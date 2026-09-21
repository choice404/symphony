package projects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the projects view
const ViewName = "projects"

// listHint is the second line of the projects page
const listHint = "  <CR> open  c new  D forget  r refresh  q back"

// newHint is the first line of the new project editor
const newHint = "  gs create  q back without creating"

// newMarker ends the fields of the new project editor
const newMarker = "--- nothing below this line is read ---"

// Settings is what the view reads from the config on every render
type Settings struct {
	// The root directories
	Roots []string
}

// Source returns the settings as configured right now
type Source func() Settings

// Projects is the projects app
type Projects struct {
	// Returns the settings as configured right now
	src Source
	// Reads directories and git
	scanner *Scanner
	// Remembers what was opened
	recents *Recents
	// Guards the last list and the editors
	mu sync.Mutex
	// The last list shown, so open finds a key
	last []Project
	// The open editors by number
	editors map[int]bool
	// The next editor number
	next int
	// The tree state per project path
	trees map[string]treeState
}

/**
 * New
 * Builds the projects view
 * @param src {Source} - the settings, read on every render
 * @param scanner {*Scanner} - reads directories and git
 * @param recents {*Recents} - remembers what was opened
 * @return *Projects
 **/
func New(src Source, scanner *Scanner, recents *Recents) *Projects {
	return &Projects{src: src, scanner: scanner, recents: recents, editors: map[int]bool{}, next: 1, trees: map[string]treeState{}}
}

/**
 * Name
 * Returns projects
 * @return string
 **/
func (p *Projects) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the one home entry
 * @return []view.Entry
 **/
func (p *Projects) Entries() []view.Entry {
	return []view.Entry{{Name: ViewName, Label: "projects", Summary: p.summary}}
}

/**
 * summary
 * Returns the count and the most recently opened project
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (p *Projects) summary(ctx context.Context) string {
	list, errs := p.scanner.Scan(ctx, p.src().Roots, p.recents)
	if len(list) == 0 && len(errs) > 0 {
		for root, err := range errs {
			return root + ": " + err.Error()
		}
	}
	out := fmt.Sprintf("%d projects", len(list))
	if len(list) > 0 && !list[0].Opened.IsZero() {
		out += ", last " + list[0].Name
	}
	return out
}

/**
 * Render
 * Lists every project
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (p *Projects) Render(ctx context.Context) (view.Page, error) {
	// The list
	list, errs := p.scanner.Scan(ctx, p.src().Roots, p.recents)
	p.mu.Lock()
	p.last = list
	p.mu.Unlock()
	// The header with any root that failed
	header := fmt.Sprintf("projects  %d", len(list))
	roots := make([]string, 0, len(errs))
	for root := range errs {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		header += "  [" + root + " " + errs[root].Error() + "]"
	}
	lines := []string{header, listHint, ""}
	keys := []string{"", "", ""}
	// One line per project
	for _, pr := range list {
		lines = append(lines, projectLine(pr))
		keys = append(keys, pr.Path)
	}
	if len(list) == 0 {
		lines = append(lines, "  nothing under the roots, press c to make a project")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName, Title: "projects", Lines: lines, Keys: keys, Cursor: 3, Filetype: "projects"}, nil
}

/**
 * projectLine
 * Formats one project line, the name, the branch and dirty count, the last commit, and when it was opened
 * @param pr {Project} - the project
 * @return string
 **/
func projectLine(pr Project) string {
	// The git column
	git := "          "
	if pr.Git {
		branch := pr.Branch
		if branch == "" {
			branch = "detached"
		}
		if pr.Dirty > 0 {
			branch += fmt.Sprintf(" +%d", pr.Dirty)
		}
		git = fmt.Sprintf("%-10s", clip(branch, 10))
	}
	// The last commit, clipped
	last := clip(pr.Last, 40)
	if pr.Last != "" && !pr.LastAt.IsZero() {
		last = ago(pr.LastAt) + "  " + last
	}
	// The opened column
	opened := ""
	if !pr.Opened.IsZero() {
		opened = "opened " + ago(pr.Opened)
	}
	return fmt.Sprintf("  %-24s %s  %-52s %s", clip(pr.Name, 24), git, last, opened)
}

/**
 * ago
 * Says how long ago a time was in one unit
 * @param t {time.Time} - the time
 * @return string
 **/
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
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

/**
 * RenderPath
 * Renders a tree page, tree/ followed by the project path
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below projects
 * @return view.Page, error
 **/
func (p *Projects) RenderPath(ctx context.Context, path string) (view.Page, error) {
	if strings.HasPrefix(path, treePrefix) {
		return p.tree(ctx, strings.TrimPrefix(path, treePrefix))
	}
	if strings.HasPrefix(path, "new/") {
		return view.Page{}, fmt.Errorf("projects: a new project page is opened with c")
	}
	return view.Page{}, fmt.Errorf("projects: no page %q", path)
}

/**
 * Act
 * Runs an action from a projects page, tree pages have their own set
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (p *Projects) Act(ctx context.Context, a view.Action) (view.Response, error) {
	if root := treeOf(a.Page); root != "" {
		return p.treeAct(ctx, root, a)
	}
	switch a.Name {
	case "refresh":
		return p.show(p.Render(ctx))
	case "open":
		return p.open(a.Key)
	case "new":
		return p.newProject()
	case "save":
		return p.create(ctx, a)
	case "forget":
		if a.Key == "" {
			return view.Response{Kind: view.KindNone}, nil
		}
		p.recents.Forget(a.Key)
		return p.show(p.Render(ctx))
	}
	return view.Fail("projects: unknown action " + a.Name), nil
}

/**
 * show
 * Turns a page or its error into a response
 * @param pg {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (p *Projects) show(pg view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(pg), nil
}

/**
 * open
 * Records the open and tells the plugin to enter the directory
 * @param path {string} - the project path
 * @return view.Response, error
 **/
func (p *Projects) open(path string) (view.Response, error) {
	if path == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return view.Fail("projects: " + path + " is not a directory any more, refresh"), nil
	}
	p.recents.Touch(path)
	return view.Response{Kind: view.KindEnter, Path: path, Text: filepath.Base(path)}, nil
}

/**
 * newProject
 * Opens the editor for a new project under the first root
 * @return view.Response, error
 **/
func (p *Projects) newProject() (view.Response, error) {
	roots := p.src().Roots
	if len(roots) == 0 {
		return view.Fail("projects: set roots under [projects] in the config"), nil
	}
	p.mu.Lock()
	n := p.next
	p.next++
	p.editors[n] = true
	p.mu.Unlock()
	lines := []string{
		newHint,
		"Name: ",
		"Root: " + roots[0],
		"Git: yes",
		newMarker,
	}
	return view.Show(view.Page{
		Name:     fmt.Sprintf("%s/new/%d", ViewName, n),
		Title:    "new project",
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Key:      strconv.Itoa(n),
		Cursor:   1,
		Filetype: "projectnew",
		Editable: true,
	}), nil
}

/**
 * create
 * Parses the editor, makes the project, and enters it
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the save action with the buffer text
 * @return view.Response, error
 **/
func (p *Projects) create(ctx context.Context, a view.Action) (view.Response, error) {
	n, err := strconv.Atoi(a.Key)
	if err != nil {
		return view.Fail("projects: create works from a new project page"), nil
	}
	p.mu.Lock()
	open := p.editors[n]
	p.mu.Unlock()
	if !open {
		return view.Fail("projects: this editor is not open any more, press c again"), nil
	}
	// The fields
	fields := map[string]string{}
	for _, line := range strings.Split(a.Body, "\n") {
		if strings.TrimSpace(line) == newMarker {
			break
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "  ") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	name := fields["name"]
	if name == "" || strings.ContainsAny(name, "/\\") {
		return view.Fail("projects: name must be a plain directory name"), nil
	}
	root := fields["root"]
	if root == "" {
		return view.Fail("projects: root is empty"), nil
	}
	initGit := strings.HasPrefix(strings.ToLower(fields["git"]), "y")
	// Make it
	path := filepath.Join(root, name)
	if err := Create(ctx, path, name, initGit); err != nil {
		return view.Fail("projects: " + err.Error()), nil
	}
	p.mu.Lock()
	delete(p.editors, n)
	p.mu.Unlock()
	// Enter it and drop the editor
	resp, err := p.open(path)
	resp.Close = a.Page
	return resp, err
}
