package projects

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/view"
)

// roots makes a root with one git repo, one plain dir, and a dot dir
func roots(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	for _, d := range []string{"alpha", "beta", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// alpha is a repository with one commit and one dirty file
	repo := filepath.Join(root, "alpha")
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"commit", "--allow-empty", "-q", "-m", "first commit"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// fresh builds a view over a root with empty recents
func fresh(t *testing.T, root string) *Projects {
	t.Helper()
	src := func() Settings { return Settings{Roots: []string{root}} }
	return New(src, NewScanner(), NewRecents(t.TempDir()))
}

func TestScanListsGitState(t *testing.T) {
	root := roots(t)
	list, errs := NewScanner().Scan(context.Background(), []string{root, filepath.Join(root, "nope")}, NewRecents(t.TempDir()))
	if len(errs) != 1 {
		t.Fatalf("errs = %v", errs)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("list = %+v", list)
	}
	a := list[0]
	if !a.Git || a.Branch != "main" || a.Dirty != 1 || a.Last != "first commit" || a.LastAt.IsZero() {
		t.Fatalf("alpha = %+v", a)
	}
	if list[1].Git {
		t.Fatal("beta should not be git")
	}
}

func TestRecentsOrderAndForget(t *testing.T) {
	root := roots(t)
	rec := NewRecents(t.TempDir())
	rec.Touch(filepath.Join(root, "beta"))
	list, _ := NewScanner().Scan(context.Background(), []string{root}, rec)
	if list[0].Name != "beta" || list[0].Opened.IsZero() {
		t.Fatalf("recent first = %+v", list[0])
	}
	// Recents survive a reload from the same directory
	again := NewRecents(filepath.Dir(rec.path))
	if again.When(filepath.Join(root, "beta")).IsZero() {
		t.Fatal("recents not persisted")
	}
	rec.Forget(filepath.Join(root, "beta"))
	if !rec.When(filepath.Join(root, "beta")).IsZero() {
		t.Fatal("forget did not drop it")
	}
}

func TestRenderOpenAndEntries(t *testing.T) {
	root := roots(t)
	p := fresh(t, root)
	ctx := context.Background()
	pg, err := p.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pg.Lines[0], "projects  2") || pg.Keys[3] != filepath.Join(root, "alpha") {
		t.Fatalf("page = %v keys = %v", pg.Lines, pg.Keys)
	}
	if !strings.Contains(pg.Lines[3], "main +1") || !strings.Contains(pg.Lines[3], "first commit") {
		t.Fatalf("alpha line = %q", pg.Lines[3])
	}
	// Opening enters the directory and records it
	r, _ := p.Act(ctx, view.Action{Name: "open", Key: pg.Keys[3], Page: "projects"})
	if r.Kind != view.KindEnter || r.Path != pg.Keys[3] || r.Text != "alpha" {
		t.Fatalf("open = %+v", r)
	}
	if got := p.Entries()[0].Summary(ctx); got != "2 projects, last alpha" {
		t.Fatalf("summary = %q", got)
	}
	// A header line opens nothing, a vanished dir is a message
	r, _ = p.Act(ctx, view.Action{Name: "open", Page: "projects"})
	if r.Kind != view.KindNone {
		t.Fatalf("header open = %+v", r)
	}
	r, _ = p.Act(ctx, view.Action{Name: "open", Key: filepath.Join(root, "gone"), Page: "projects"})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("gone open = %+v", r)
	}
}

func TestNewAndCreate(t *testing.T) {
	root := roots(t)
	p := fresh(t, root)
	ctx := context.Background()
	r, _ := p.Act(ctx, view.Action{Name: "new", Page: "projects"})
	if r.Kind != view.KindPage || !r.Page.Editable || r.Page.Lines[2] != "Root: "+root {
		t.Fatalf("new = %+v", r)
	}
	lines := r.Page.Lines
	lines[1] = "Name: gamma"
	r, _ = p.Act(ctx, view.Action{Name: "save", Key: r.Page.Key, Page: r.Page.Name, Body: strings.Join(lines, "\n")})
	if r.Kind != view.KindEnter || r.Path != filepath.Join(root, "gamma") || r.Close != "projects/new/1" {
		t.Fatalf("save = %+v", r)
	}
	// The directory, its README, and its repository exist
	if _, err := os.Stat(filepath.Join(root, "gamma", ".git")); err != nil {
		t.Fatal("no git repository")
	}
	data, _ := os.ReadFile(filepath.Join(root, "gamma", "README.md"))
	if string(data) != "# gamma\n" {
		t.Fatalf("readme = %q", data)
	}
	// Creating it again is refused
	r, _ = p.Act(ctx, view.Action{Name: "new", Page: "projects"})
	lines = r.Page.Lines
	lines[1] = "Name: gamma"
	r, _ = p.Act(ctx, view.Action{Name: "save", Key: r.Page.Key, Page: r.Page.Name, Body: strings.Join(lines, "\n")})
	if r.Kind != view.KindNotify || !r.Error || !strings.Contains(r.Text, "exists") {
		t.Fatalf("second save = %+v", r)
	}
	// A bad name is refused
	lines[1] = "Name: ../evil"
	r, _ = p.Act(ctx, view.Action{Name: "save", Key: "3", Page: "projects/new/3", Body: strings.Join(lines, "\n")})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("bad name = %+v", r)
	}
}
