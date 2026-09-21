// Package projects lists the directories under the project roots with their git state and remembers which were opened
package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// gitTimeout is how long one git command may take
const gitTimeout = 3 * time.Second

// gitCacheFor is how long git state is trusted before it is read again
const gitCacheFor = 30 * time.Second

// workers is how many git reads run at once
const workers = 8

// recentsFile is where opened projects are remembered under the cache directory
const recentsFile = "projects.json"

// Project is one directory under a root
type Project struct {
	// The directory name
	Name string
	// The full path
	Path string
	// Whether it is a git repository
	Git bool
	// The current branch, empty outside git or on a detached head
	Branch string
	// How many paths git status reports
	Dirty int
	// The last commit subject, empty outside git
	Last string
	// When the last commit was made
	LastAt time.Time
	// When it was last opened through symphony, zero for never
	Opened time.Time
}

// gitState is what one read of a repository gave
type gitState struct {
	// When it was read
	at time.Time
	// The branch
	branch string
	// The dirty count
	dirty int
	// The last subject
	last string
	// The last commit time
	lastAt time.Time
}

// Recents remembers when each project was last opened
type Recents struct {
	// The file
	path string
	// Guards the map
	mu sync.Mutex
	// The last open time by path
	opened map[string]time.Time
}

/**
 * NewRecents
 * Loads the recents file under a directory, empty when there is none
 * @param dir {string} - the cache directory
 * @return *Recents
 **/
func NewRecents(dir string) *Recents {
	r := &Recents{path: filepath.Join(dir, recentsFile), opened: map[string]time.Time{}}
	if data, err := os.ReadFile(r.path); err == nil {
		_ = json.Unmarshal(data, &r.opened)
	}
	return r
}

/**
 * Touch
 * Records that a project was opened now and saves
 * @param path {string} - the project path
 * @return void
 **/
func (r *Recents) Touch(path string) {
	r.mu.Lock()
	next := make(map[string]time.Time, len(r.opened)+1)
	for k, v := range r.opened {
		next[k] = v
	}
	next[path] = time.Now()
	r.opened = next
	r.mu.Unlock()
	r.save()
}

/**
 * Forget
 * Drops a project from the recents and saves
 * @param path {string} - the project path
 * @return void
 **/
func (r *Recents) Forget(path string) {
	r.mu.Lock()
	next := make(map[string]time.Time, len(r.opened))
	for k, v := range r.opened {
		if k != path {
			next[k] = v
		}
	}
	r.opened = next
	r.mu.Unlock()
	r.save()
}

/**
 * When
 * Returns when a project was last opened, zero for never
 * @param path {string} - the project path
 * @return time.Time
 **/
func (r *Recents) When(path string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.opened[path]
}

/**
 * save
 * Writes the recents file
 * @return void
 **/
func (r *Recents) save() {
	r.mu.Lock()
	data, err := json.MarshalIndent(r.opened, "", "  ")
	r.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(r.path), 0o700)
	_ = os.WriteFile(r.path, data, 0o600)
}

// Scanner lists projects under roots with their git state, reading git in parallel and remembering it for a while
type Scanner struct {
	// Guards the git cache
	mu sync.Mutex
	// The git state by path
	git map[string]gitState
}

/**
 * NewScanner
 * Builds a scanner with an empty git cache
 * @return *Scanner
 **/
func NewScanner() *Scanner {
	return &Scanner{git: map[string]gitState{}}
}

/**
 * Scan
 * Lists every directory one level under the roots, sorted by last opened then name, with git state filled in
 * @param ctx {context.Context} - the context
 * @param roots {[]string} - the root directories
 * @param recents {*Recents} - when each was opened
 * @return []Project, map[string]error
 **/
func (s *Scanner) Scan(ctx context.Context, roots []string, recents *Recents) ([]Project, map[string]error) {
	// The directories
	found := make([]Project, 0, 64)
	errs := map[string]error{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			errs[root] = err
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			path := filepath.Join(root, e.Name())
			found = append(found, Project{Name: e.Name(), Path: path, Opened: recents.When(path)})
		}
	}
	// Git state a few at a time
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i := range found {
		wg.Add(1)
		sem <- struct{}{}
		go func(p *Project) {
			defer wg.Done()
			defer func() { <-sem }()
			s.fill(ctx, p)
		}(&found[i])
	}
	wg.Wait()
	// Recently opened first, then by name
	sort.SliceStable(found, func(i, j int) bool {
		if !found[i].Opened.Equal(found[j].Opened) {
			return found[i].Opened.After(found[j].Opened)
		}
		return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name)
	})
	return found, errs
}

/**
 * fill
 * Fills a project's git fields from the cache or from git
 * @param ctx {context.Context} - the context
 * @param p {*Project} - the project, written in place before it is handed out
 * @return void
 **/
func (s *Scanner) fill(ctx context.Context, p *Project) {
	// Not a repository
	if _, err := os.Stat(filepath.Join(p.Path, ".git")); err != nil {
		return
	}
	p.Git = true
	// The cache
	s.mu.Lock()
	st, ok := s.git[p.Path]
	s.mu.Unlock()
	if !ok || time.Since(st.at) > gitCacheFor {
		st = readGit(ctx, p.Path)
		s.mu.Lock()
		s.git[p.Path] = st
		s.mu.Unlock()
	}
	p.Branch, p.Dirty, p.Last, p.LastAt = st.branch, st.dirty, st.last, st.lastAt
}

/**
 * readGit
 * Runs the three git reads for one repository
 * @param ctx {context.Context} - the context
 * @param path {string} - the repository
 * @return gitState
 **/
func readGit(ctx context.Context, path string) gitState {
	st := gitState{at: time.Now()}
	// The branch, empty when detached
	if out, err := git(ctx, path, "symbolic-ref", "--short", "HEAD"); err == nil {
		st.branch = out
	}
	// The dirty count
	if out, err := git(ctx, path, "status", "--porcelain"); err == nil && out != "" {
		st.dirty = len(strings.Split(out, "\n"))
	}
	// The last commit
	if out, err := git(ctx, path, "log", "-1", "--format=%ct %s"); err == nil && out != "" {
		when, subject, _ := strings.Cut(out, " ")
		if n, err := strconv.ParseInt(when, 10, 64); err == nil {
			st.lastAt = time.Unix(n, 0)
		}
		st.last = subject
	}
	return st
}

/**
 * git
 * Runs one git command in a repository with a timeout and returns its trimmed output
 * @param ctx {context.Context} - the context
 * @param path {string} - the repository
 * @param args {...string} - the git arguments
 * @return string, error
 **/
func git(ctx context.Context, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

/**
 * Create
 * Makes a new project directory, with a git repository and a README when asked
 * @param ctx {context.Context} - the context
 * @param path {string} - the directory to create
 * @param name {string} - the project name for the README
 * @param initGit {bool} - whether to git init
 * @return error
 **/
func Create(ctx context.Context, path, name string, initGit bool) error {
	// Refuse an existing directory
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Make it with a README
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		return err
	}
	// A repository when asked
	if initGit {
		if _, err := git(ctx, path, "init", "-q"); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
	}
	return nil
}
