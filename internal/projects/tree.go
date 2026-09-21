package projects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/choice404/symphony/internal/view"
)

// treePrefix is what a tree page name starts with, the absolute project path follows
const treePrefix = "tree/"

// treeHint is the second line of a tree page
const treeHint = "  <CR> open or expand  l expand  h collapse  . dotfiles  f find  r refresh  q back to projects"

// treeState is what is expanded and whether dotfiles show, one per project
type treeState struct {
	// The expanded directories by path
	expanded map[string]bool
	// Whether dotfiles are listed
	hidden bool
}

/**
 * treePage
 * Builds the file tree of a project, the root listing with every expanded directory listed under its line
 * @param ctx {context.Context} - the context
 * @param root {string} - the project path
 * @param st {treeState} - what is expanded
 * @param git {string} - the git tag for the header, empty outside git
 * @return view.Page, error
 **/
func treePage(ctx context.Context, root string, st treeState, git string) (view.Page, error) {
	// The root must exist
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return view.Page{}, fmt.Errorf("projects: %s is not a directory", root)
	}
	// The header
	header := filepath.Base(root) + "  " + root
	if git != "" {
		header += "  [" + git + "]"
	}
	lines := []string{header, treeHint, ""}
	keys := []string{"", "", ""}
	// The listing
	l, k := listDir(root, 0, st)
	lines = append(lines, l...)
	keys = append(keys, k...)
	if len(l) == 0 {
		lines = append(lines, "  (empty)")
		keys = append(keys, "")
	}
	return view.Page{
		Name:     ViewName + "/" + treePrefix + root,
		Title:    filepath.Base(root),
		Lines:    lines,
		Keys:     keys,
		Key:      root,
		Cursor:   3,
		Filetype: "tree",
	}, nil
}

/**
 * listDir
 * Lists one directory, directories first, then files, each expanded directory followed by its own listing indented
 * @param dir {string} - the directory
 * @param depth {int} - how deep, for the indent
 * @param st {treeState} - what is expanded
 * @return []string, []string
 **/
func listDir(dir string, depth int, st treeState) ([]string, []string) {
	// Read it
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{strings.Repeat("  ", depth+1) + "cannot read: " + err.Error()}, []string{""}
	}
	// Directories first, then files, both by name
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	lines := make([]string, 0, len(entries))
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		// Dotfiles only when asked
		if !st.hidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		lines = append(lines, entryLine(path, e, depth, st.expanded[path]))
		keys = append(keys, path)
		// An expanded directory lists under its line
		if e.IsDir() && st.expanded[path] {
			l, k := listDir(path, depth+1, st)
			lines = append(lines, l...)
			keys = append(keys, k...)
		}
	}
	return lines, keys
}

/**
 * entryLine
 * Formats one entry the way ls -al does, the mode, the size, the time, then the indented name with a marker on a directory
 * @param path {string} - the entry path
 * @param e {os.DirEntry} - the entry
 * @param depth {int} - the indent depth
 * @param expanded {bool} - whether a directory is open
 * @return string
 **/
func entryLine(path string, e os.DirEntry, depth int, expanded bool) string {
	// The stat, a broken entry still gets a line
	info, err := e.Info()
	mode, size, when := "??????????", "", "            "
	if err == nil {
		mode = info.Mode().String()
		when = info.ModTime().Format("Jan 02 15:04")
		if !e.IsDir() {
			size = human(info.Size())
		}
	}
	// A link says where it goes
	name := e.Name()
	if e.Type()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			name += " -> " + target
		}
	}
	// A directory carries a marker and a slash
	marker := "  "
	if e.IsDir() {
		marker = "+ "
		if expanded {
			marker = "- "
		}
		name += "/"
	}
	return fmt.Sprintf("%s %6s  %s  %s%s%s", mode, size, when, strings.Repeat("    ", depth), marker, name)
}

/**
 * human
 * Formats a byte count in one unit
 * @param n {int64} - the count
 * @return string
 **/
func human(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1fM", float64(n)/1024/1024)
	default:
		return fmt.Sprintf("%.1fG", float64(n)/1024/1024/1024)
	}
}

/**
 * gitTag
 * Formats the branch and dirty count of a project for the tree header, empty outside git
 * @param ctx {context.Context} - the context
 * @param s {*Scanner} - the scanner with its git cache
 * @param root {string} - the project path
 * @return string
 **/
func gitTag(ctx context.Context, s *Scanner, root string) string {
	p := Project{Path: root}
	s.fill(ctx, &p)
	if !p.Git {
		return ""
	}
	branch := p.Branch
	if branch == "" {
		branch = "detached"
	}
	if p.Dirty > 0 {
		branch += fmt.Sprintf(" +%d", p.Dirty)
	}
	return branch
}

/**
 * treeOf
 * Returns the project path behind a tree page name, empty when it is not one
 * @param page {string} - the page name
 * @return string
 **/
func treeOf(page string) string {
	_, path := view.Split(page)
	if !strings.HasPrefix(path, treePrefix) {
		return ""
	}
	return strings.TrimPrefix(path, treePrefix)
}

/**
 * state
 * Returns a project's tree state, an empty one the first time
 * @param root {string} - the project path
 * @return treeState
 **/
func (p *Projects) state(root string) treeState {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.trees[root]
	if !ok {
		return treeState{expanded: map[string]bool{}}
	}
	return st
}

/**
 * setState
 * Replaces a project's tree state
 * @param root {string} - the project path
 * @param st {treeState} - the state
 * @return void
 **/
func (p *Projects) setState(root string, st treeState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := make(map[string]treeState, len(p.trees)+1)
	for k, v := range p.trees {
		next[k] = v
	}
	next[root] = st
	p.trees = next
}

/**
 * toggle
 * Returns a state with one directory expanded or collapsed, collapsing also closes everything under it
 * @param st {treeState} - the state
 * @param dir {string} - the directory
 * @param open {bool} - whether to expand
 * @return treeState
 **/
func toggle(st treeState, dir string, open bool) treeState {
	expanded := make(map[string]bool, len(st.expanded)+1)
	for k, v := range st.expanded {
		if !open && (k == dir || strings.HasPrefix(k, dir+"/")) {
			continue
		}
		expanded[k] = v
	}
	if open {
		expanded[dir] = true
	}
	return treeState{expanded: expanded, hidden: st.hidden}
}

/**
 * tree
 * Renders the tree page of a project
 * @param ctx {context.Context} - the context
 * @param root {string} - the project path
 * @return view.Page, error
 **/
func (p *Projects) tree(ctx context.Context, root string) (view.Page, error) {
	return treePage(ctx, root, p.state(root), gitTag(ctx, p.scanner, root))
}

/**
 * treeAct
 * Runs an action on a tree page, open expands a directory or edits a file, l and h expand and collapse, dot toggles dotfiles
 * @param ctx {context.Context} - the context
 * @param root {string} - the project path
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (p *Projects) treeAct(ctx context.Context, root string, a view.Action) (view.Response, error) {
	// The entry under the cursor
	target := a.Key
	isDir := false
	if target != "" && target != root {
		info, err := os.Stat(target)
		if err != nil {
			return view.Fail("projects: " + err.Error()), nil
		}
		isDir = info.IsDir()
	}
	st := p.state(root)
	switch a.Name {
	case "refresh":
		return p.show(p.tree(ctx, root))
	case "hidden":
		p.setState(root, treeState{expanded: st.expanded, hidden: !st.hidden})
		return p.show(p.tree(ctx, root))
	case "open", "expand":
		if target == "" || target == root {
			return view.Response{Kind: view.KindNone}, nil
		}
		if !isDir {
			if a.Name == "expand" {
				return view.Response{Kind: view.KindNone}, nil
			}
			return view.Response{Kind: view.KindEdit, Path: target, Text: filepath.Base(target)}, nil
		}
		p.setState(root, toggle(st, target, a.Name == "expand" || !st.expanded[target]))
		return p.show(focus(p.tree(ctx, root))(target))
	case "collapse":
		// An open directory closes, anything else closes its parent
		dir := target
		if target == "" || target == root || !isDir || !st.expanded[target] {
			dir = filepath.Dir(target)
		}
		if dir == "" || dir == root || !strings.HasPrefix(dir, root+"/") {
			return view.Response{Kind: view.KindNone}, nil
		}
		p.setState(root, toggle(st, dir, false))
		return p.show(focus(p.tree(ctx, root))(dir))
	}
	return view.Fail("projects: unknown action " + a.Name), nil
}

/**
 * focus
 * Wraps a rendered page so the cursor lands on the line keyed by a path, the folder just opened or closed
 * @param pg {view.Page} - the page
 * @param err {error} - its error
 * @return func(string) (view.Page, error)
 **/
func focus(pg view.Page, err error) func(string) (view.Page, error) {
	return func(key string) (view.Page, error) {
		pg.Focus = key
		return pg, err
	}
}
