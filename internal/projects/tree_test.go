package projects

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/view"
)

// treeRoot makes a project with a nested directory, a dotfile, and a file
func treeRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "demo")
	for _, d := range []string{"src/pkg", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"README.md": "# demo\n", "src/main.go": "package main\n", "src/pkg/x.go": "package pkg\n", ".env": "x=1\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTreeListsLikeLs(t *testing.T) {
	root := treeRoot(t)
	p := fresh(t, filepath.Dir(root))
	pg, err := p.RenderPath(context.Background(), "tree/"+root)
	if err != nil {
		t.Fatal(err)
	}
	if pg.Name != "projects/tree/"+root || pg.Key != root || pg.Filetype != "tree" {
		t.Fatalf("page = %+v", pg)
	}
	// Directories first with a plus, then files with a size, dotfiles hidden
	if !strings.Contains(pg.Lines[3], "+ docs/") || !strings.Contains(pg.Lines[4], "+ src/") || !strings.Contains(pg.Lines[5], "README.md") {
		t.Fatalf("lines = %v", pg.Lines)
	}
	if !strings.HasPrefix(pg.Lines[5], "-rw-") || !strings.Contains(pg.Lines[5], "7B") {
		t.Fatalf("file line = %q", pg.Lines[5])
	}
	if strings.Contains(strings.Join(pg.Lines, "\n"), ".env") {
		t.Fatal("dotfile shown by default")
	}
	if pg.Keys[4] != filepath.Join(root, "src") || pg.Keys[5] != filepath.Join(root, "README.md") {
		t.Fatalf("keys = %v", pg.Keys)
	}
}

func TestTreeExpandCollapseAndDotfiles(t *testing.T) {
	root := treeRoot(t)
	p := fresh(t, filepath.Dir(root))
	ctx := context.Background()
	page := "projects/tree/" + root
	src := filepath.Join(root, "src")
	// Enter on a directory expands it and its children indent under it
	r, _ := p.Act(ctx, view.Action{Name: "open", Key: src, Page: page})
	if r.Kind != view.KindPage || !strings.Contains(r.Page.Lines[4], "- src/") || !strings.Contains(r.Page.Lines[5], "    + pkg/") || !strings.Contains(r.Page.Lines[6], "    main.go") {
		t.Fatalf("expanded = %v", r.Page.Lines)
	}
	// l on the nested directory expands it too
	r, _ = p.Act(ctx, view.Action{Name: "expand", Key: filepath.Join(src, "pkg"), Page: page})
	if !strings.Contains(r.Page.Lines[6], "        x.go") {
		t.Fatalf("nested = %v", r.Page.Lines)
	}
	// h on a file inside collapses its parent, and the nested one closes with it
	r, _ = p.Act(ctx, view.Action{Name: "collapse", Key: filepath.Join(src, "main.go"), Page: page})
	if !strings.Contains(r.Page.Lines[4], "+ src/") || len(r.Page.Lines) != 6 {
		t.Fatalf("collapsed = %v", r.Page.Lines)
	}
	r, _ = p.Act(ctx, view.Action{Name: "open", Key: src, Page: page})
	if !strings.Contains(r.Page.Lines[5], "    + pkg/") {
		t.Fatal("nested directory stayed open after its parent collapsed")
	}
	// Dot toggles dotfiles
	r, _ = p.Act(ctx, view.Action{Name: "hidden", Page: page})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), ".env") {
		t.Fatal("dotfile not shown after toggle")
	}
}

func TestTreeOpensFile(t *testing.T) {
	root := treeRoot(t)
	p := fresh(t, filepath.Dir(root))
	ctx := context.Background()
	page := "projects/tree/" + root
	r, _ := p.Act(ctx, view.Action{Name: "open", Key: filepath.Join(root, "README.md"), Page: page})
	if r.Kind != view.KindEdit || r.Path != filepath.Join(root, "README.md") || r.Text != "README.md" {
		t.Fatalf("edit = %+v", r)
	}
	// A header line does nothing, a vanished path is a message
	r, _ = p.Act(ctx, view.Action{Name: "open", Page: page})
	if r.Kind != view.KindNone {
		t.Fatalf("header = %+v", r)
	}
	r, _ = p.Act(ctx, view.Action{Name: "open", Key: filepath.Join(root, "gone"), Page: page})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("gone = %+v", r)
	}
	if _, err := p.RenderPath(ctx, "tree/"+filepath.Join(root, "nope")); err == nil {
		t.Fatal("missing root rendered")
	}
}
