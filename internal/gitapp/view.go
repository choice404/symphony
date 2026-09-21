package gitapp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the git view
const ViewName = "git"

// statusHint is the second line of a status page
const statusHint = "  <CR> diff  s stage  u unstage  c commit  l log  p push  P pull  r refresh  q back"

// logHint is the second line of a log page
const logHint = "  <CR> show  gs status  r refresh  q back"

// commitHint is the first line of a commit page
const commitHint = "  gs commit  q back without committing, the message goes above the marker"

// commitMarker ends the message on a commit page
const commitMarker = "--- staged changes below, not part of the message ---"

// Git is the git app, pages under a repository path
type Git struct {
	// Guards the editors
	mu sync.Mutex
	// The open commit editors by number, the repository each is for
	editors map[int]string
	// The next editor number
	next int
}

/**
 * New
 * Builds the git view
 * @return *Git
 **/
func New() *Git {
	return &Git{editors: map[int]string{}, next: 1}
}

/**
 * Name
 * Returns git
 * @return string
 **/
func (g *Git) Name() string {
	return ViewName
}

/**
 * Render
 * Explains that git pages live under a repository
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (g *Git) Render(ctx context.Context) (view.Page, error) {
	return view.Page{
		Name:     ViewName,
		Title:    "git",
		Lines:    []string{"git pages live under a project, open one from the projects page and press g on its tree"},
		Keys:     []string{""},
		Filetype: "page",
	}, nil
}

// pageOf splits a page path into its kind and the repository, status/<repo>, log/<repo>, diff/<repo>//<file>, show/<repo>//<hash>
func pageOf(path string) (kind, repo, rest string) {
	kind, path, _ = strings.Cut(path, "/")
	repo, rest, _ = strings.Cut(path, "//")
	return kind, repo, rest
}

/**
 * RenderPath
 * Renders a page under a repository
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below git
 * @return view.Page, error
 **/
func (g *Git) RenderPath(ctx context.Context, path string) (view.Page, error) {
	kind, repo, rest := pageOf(path)
	if repo == "" {
		return view.Page{}, fmt.Errorf("git: a page needs a repository, not %q", path)
	}
	switch kind {
	case "status":
		return g.status(ctx, repo)
	case "log":
		return g.log(ctx, repo)
	case "diff":
		return g.diff(ctx, repo, rest)
	case "show":
		return g.showCommit(ctx, repo, rest)
	case "commit":
		return view.Page{}, fmt.Errorf("git: a commit page is opened with c")
	}
	return view.Page{}, fmt.Errorf("git: no page %q", path)
}

/**
 * status
 * Builds the status page, the branch line, then the staged, unstaged, and untracked sections
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return view.Page, error
 **/
func (g *Git) status(ctx context.Context, repo string) (view.Page, error) {
	st, err := ReadStatus(ctx, repo)
	if err != nil {
		return view.Page{}, err
	}
	// The header
	branch := st.Branch
	if branch == "" {
		branch = "detached"
	}
	header := fmt.Sprintf("%s  %s", branch, repo)
	if st.Ahead > 0 || st.Behind > 0 {
		header += fmt.Sprintf("  [ahead %d, behind %d]", st.Ahead, st.Behind)
	}
	lines := []string{header, statusHint}
	keys := []string{"", ""}
	// The sections, each entry keyed by its section and path
	section := func(title, prefix string, entries []Entry) {
		if len(entries) == 0 {
			return
		}
		lines = append(lines, "", title)
		keys = append(keys, "", "")
		for _, e := range entries {
			letter := e.Staged
			if prefix == "unstaged" {
				letter = e.Unstaged
			}
			if prefix == "untracked" {
				letter = '?'
			}
			lines = append(lines, fmt.Sprintf("  %c  %s", letter, e.Path))
			keys = append(keys, prefix+":"+e.Path)
		}
	}
	section("staged", "staged", st.StagedList)
	section("unstaged", "unstaged", st.UnstagedList)
	section("untracked", "untracked", st.UntrackedList)
	if len(lines) == 2 {
		lines = append(lines, "", "  nothing to commit, working tree clean")
		keys = append(keys, "", "")
	}
	// The cursor starts on the first path
	cursor := 3
	for i, k := range keys {
		if k != "" {
			cursor = i
			break
		}
	}
	return view.Page{Name: ViewName + "/status/" + repo, Title: "git " + branch, Lines: lines, Keys: keys, Key: repo, Cursor: cursor, Filetype: "gitstatus"}, nil
}

/**
 * log
 * Builds the log page
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return view.Page, error
 **/
func (g *Git) log(ctx context.Context, repo string) (view.Page, error) {
	commits, err := Log(ctx, repo)
	if err != nil {
		return view.Page{}, err
	}
	lines := []string{fmt.Sprintf("log  %s  %d commits", repo, len(commits)), logHint, ""}
	keys := []string{"", "", ""}
	for _, c := range commits {
		lines = append(lines, fmt.Sprintf("  %s  %-14s %-16s %s", c.Hash, clip(c.When, 14), clip(c.Author, 16), c.Subject))
		keys = append(keys, c.Hash)
	}
	if len(commits) == 0 {
		lines = append(lines, "  no commits yet")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName + "/log/" + repo, Title: "git log", Lines: lines, Keys: keys, Key: repo, Cursor: 3, Filetype: "gitlog"}, nil
}

/**
 * diff
 * Builds the diff page of one path, staged and unstaged parts
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param key {string} - the section and path from the status page
 * @return view.Page, error
 **/
func (g *Git) diff(ctx context.Context, repo, key string) (view.Page, error) {
	section, rel, ok := strings.Cut(key, ":")
	if !ok {
		return view.Page{}, fmt.Errorf("git: nothing to diff")
	}
	staged, unstaged, err := Diff(ctx, repo, rel, section == "untracked")
	if err != nil {
		return view.Page{}, err
	}
	lines := []string{"diff  " + rel, "  q back", ""}
	if staged != "" {
		lines = append(lines, "# staged")
		lines = append(lines, strings.Split(strings.TrimRight(staged, "\n"), "\n")...)
		lines = append(lines, "")
	}
	if unstaged != "" {
		lines = append(lines, "# unstaged")
		lines = append(lines, strings.Split(strings.TrimRight(unstaged, "\n"), "\n")...)
	}
	if staged == "" && unstaged == "" {
		lines = append(lines, "  no differences")
	}
	return view.Page{Name: ViewName + "/diff/" + repo + "//" + key, Title: "diff " + rel, Lines: lines, Keys: make([]string, len(lines)), Key: key, Filetype: "diff"}, nil
}

/**
 * showCommit
 * Builds the page of one commit
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param hash {string} - the commit
 * @return view.Page, error
 **/
func (g *Git) showCommit(ctx context.Context, repo, hash string) (view.Page, error) {
	out, err := Show(ctx, repo, hash)
	if err != nil {
		return view.Page{}, err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	return view.Page{Name: ViewName + "/show/" + repo + "//" + hash, Title: hash, Lines: lines, Keys: make([]string, len(lines)), Key: hash, Filetype: "diff"}, nil
}

/**
 * Act
 * Runs an action from a git page
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (g *Git) Act(ctx context.Context, a view.Action) (view.Response, error) {
	_, path := view.Split(a.Page)
	kind, repo, _ := pageOf(path)
	if repo == "" {
		return view.Fail("git: no repository on this page"), nil
	}
	switch a.Name {
	case "refresh":
		if kind == "log" {
			return g.show(g.log(ctx, repo))
		}
		return g.show(g.status(ctx, repo))
	case "open":
		if a.Key == "" || a.Key == repo {
			return view.Response{Kind: view.KindNone}, nil
		}
		if kind == "log" {
			return g.show(g.showCommit(ctx, repo, a.Key))
		}
		return g.show(g.diff(ctx, repo, a.Key))
	case "status":
		return g.show(g.status(ctx, repo))
	case "log":
		return g.show(g.log(ctx, repo))
	case "stage", "unstage":
		return g.stageOrUnstage(ctx, repo, a.Name, a.Key)
	case "commit":
		return g.commitPage(ctx, repo)
	case "save":
		return g.commit(ctx, repo, a)
	case "push", "pull":
		return g.remote(ctx, repo, a.Name)
	}
	return view.Fail("git: unknown action " + a.Name), nil
}

/**
 * show
 * Turns a page or its error into a response
 * @param p {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (g *Git) show(p view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(p), nil
}

/**
 * stageOrUnstage
 * Stages or unstages the path under the cursor and shows the status again
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param what {string} - stage or unstage
 * @param key {string} - the section and path
 * @return view.Response, error
 **/
func (g *Git) stageOrUnstage(ctx context.Context, repo, what, key string) (view.Response, error) {
	_, rel, ok := strings.Cut(key, ":")
	if !ok || key == repo {
		return view.Response{Kind: view.KindNone}, nil
	}
	var err error
	if what == "stage" {
		err = Stage(ctx, repo, rel)
	} else {
		err = Unstage(ctx, repo, rel)
	}
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return g.show(g.status(ctx, repo))
}

/**
 * commitPage
 * Opens an editable page for the commit message with the staged stat below the marker
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return view.Response, error
 **/
func (g *Git) commitPage(ctx context.Context, repo string) (view.Response, error) {
	stat, err := StagedStat(ctx, repo)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	if strings.TrimSpace(stat) == "" {
		return view.Fail("git: nothing staged, press s on a path first"), nil
	}
	g.mu.Lock()
	n := g.next
	g.next++
	g.editors[n] = repo
	g.mu.Unlock()
	lines := []string{commitHint, "", commitMarker}
	lines = append(lines, strings.Split(strings.TrimRight(stat, "\n"), "\n")...)
	return view.Show(view.Page{
		Name:     fmt.Sprintf("%s/commit/%s//%d", ViewName, repo, n),
		Title:    "commit",
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Key:      strconv.Itoa(n),
		Cursor:   1,
		Filetype: "gitcommit",
		Editable: true,
	}), nil
}

/**
 * commit
 * Commits with the message above the marker and shows the status again
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param a {view.Action} - the save action with the buffer text
 * @return view.Response, error
 **/
func (g *Git) commit(ctx context.Context, repo string, a view.Action) (view.Response, error) {
	n, err := strconv.Atoi(a.Key)
	if err != nil {
		return view.Fail("git: commit works from a commit page"), nil
	}
	g.mu.Lock()
	open := g.editors[n] == repo
	g.mu.Unlock()
	if !open {
		return view.Fail("git: this commit page is not open any more, press c again"), nil
	}
	// The message is everything above the marker without the hint
	var msg []string
	for _, line := range strings.Split(a.Body, "\n") {
		if strings.TrimSpace(line) == commitMarker {
			break
		}
		if strings.HasPrefix(line, "  ") && len(msg) == 0 {
			continue
		}
		msg = append(msg, line)
	}
	out, err := Commit(ctx, repo, strings.TrimSpace(strings.Join(msg, "\n"))+"\n")
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	g.mu.Lock()
	delete(g.editors, n)
	g.mu.Unlock()
	resp, err := g.show(g.status(ctx, repo))
	if err == nil && resp.Kind == view.KindPage {
		resp.Close = a.Page
		resp.Text = "git: " + firstLine(out)
	}
	return resp, err
}

/**
 * remote
 * Pushes or pulls and shows what git said on a page
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param what {string} - push or pull
 * @return view.Response, error
 **/
func (g *Git) remote(ctx context.Context, repo, what string) (view.Response, error) {
	var out string
	var err error
	if what == "push" {
		out, err = Push(ctx, repo)
	} else {
		out, err = Pull(ctx, repo)
	}
	text := strings.TrimSpace(out)
	if err != nil {
		text = strings.TrimSpace(text + "\n" + err.Error())
	}
	if text == "" {
		text = what + " done"
	}
	lines := []string{what + "  " + repo, "  q back", ""}
	lines = append(lines, strings.Split(text, "\n")...)
	return view.Show(view.Page{Name: ViewName + "/output/" + repo, Title: "git " + what, Lines: lines, Keys: make([]string, len(lines)), Filetype: "page"}), nil
}

/**
 * firstLine
 * Returns the first line of a string
 * @param s {string} - the string
 * @return string
 **/
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

/**
 * clip
 * Cuts a string to a width in runes
 * @param s {string} - the string
 * @param width {int} - the width
 * @return string
 **/
func clip(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}
