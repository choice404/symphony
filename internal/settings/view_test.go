package settings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/view"
)

func TestMissingFileThenTemplate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	s := New(func() (string, error) { return p, nil }, nil).WithTemplate([]byte("[calendar]\ndays = 0\n[projects]\nroots = []\n"))
	ctx := context.Background()
	// The home entry and the page say there is no file
	if got := s.Entries()[0].Summary(ctx); got != "none yet, open to write one" {
		t.Fatalf("summary = %q", got)
	}
	page, err := s.Render(ctx)
	if err != nil || !strings.Contains(page.Lines[3], "no config file yet") {
		t.Fatalf("page = %v err %v", page.Lines, err)
	}
	// Edit writes the template with owner only permissions and opens it with the reload tag
	r, _ := s.Act(ctx, view.Action{Name: "edit"})
	if r.Kind != view.KindEdit || r.Path != p || r.Text != "reload" {
		t.Fatalf("edit = %+v", r)
	}
	st, err := os.Stat(p)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v err %v", st, err)
	}
	// The template parses and shows as ok with the defaults
	page, _ = s.Render(ctx)
	if page.Lines[3] != "status: ok" || !strings.Contains(strings.Join(page.Lines, "\n"), "projects roots: ~/projects") || !strings.Contains(strings.Join(page.Lines, "\n"), "editor: symphony's own") {
		t.Fatalf("page = %v", page.Lines)
	}
}

func TestBadFileAndReload(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte("[mail\nlimit = 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := New(func() (string, error) { return p, nil }, func() error {
		calls++
		if calls == 1 {
			return errors.New("still broken")
		}
		return nil
	})
	ctx := context.Background()
	page, _ := s.Render(ctx)
	if !strings.HasPrefix(page.Lines[3], "status: does not parse") || s.Entries()[0].Summary(ctx) != "does not parse" {
		t.Fatalf("page = %v", page.Lines)
	}
	// A failed reload reports the error, a good one shows the page again with a note
	r, _ := s.Act(ctx, view.Action{Name: "reload"})
	if r.Kind != view.KindNotify || r.Text != "still broken" {
		t.Fatalf("reload = %+v", r)
	}
	if err := os.WriteFile(p, []byte("[calendar]\ndays = 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Act(ctx, view.Action{Name: "reload"})
	if r.Kind != view.KindPage || r.Text != "config reloaded" || !strings.Contains(strings.Join(r.Page.Lines, "\n"), "calendar days: 7") {
		t.Fatalf("reload = %+v", r)
	}
	// Without a reloader the action says so
	r, _ = New(func() (string, error) { return p, nil }, nil).Act(ctx, view.Action{Name: "reload"})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("no reloader = %+v", r)
	}
}

func TestEditorFilesWriteStarterOnce(t *testing.T) {
	dir := t.TempDir()
	s := New(func() (string, error) { return filepath.Join(dir, "config.toml"), nil }, nil).WithEditor(
		func() (string, error) { return filepath.Join(dir, "nvim"), nil },
		func(name string) ([]byte, error) { return []byte("-- starter " + name), nil },
	)
	ctx := context.Background()
	page, _ := s.Render(ctx)
	if !strings.Contains(page.Lines[2], "k keymaps") {
		t.Fatalf("hint = %v", page.Lines)
	}
	// The first edit writes the starter, the second finds the file as it stands
	r, _ := s.Act(ctx, view.Action{Name: "keymaps"})
	want := filepath.Join(dir, "nvim", "lua", "user", "keymaps.lua")
	if r.Kind != view.KindEdit || r.Path != want || r.Text != "source" {
		t.Fatalf("keymaps = %+v", r)
	}
	if got, _ := os.ReadFile(want); string(got) != "-- starter keymaps.lua" {
		t.Fatalf("starter = %q", got)
	}
	if err := os.WriteFile(want, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Act(ctx, view.Action{Name: "keymaps"})
	if got, _ := os.ReadFile(r.Path); string(got) != "mine" {
		t.Fatalf("kept = %q", got)
	}
	// The theme and plugins files carry their own write hints
	r, _ = s.Act(ctx, view.Action{Name: "theme"})
	if r.Text != "theme" {
		t.Fatalf("theme = %+v", r)
	}
	r, _ = s.Act(ctx, view.Action{Name: "plugins"})
	if r.Text != "restart" {
		t.Fatalf("plugins = %+v", r)
	}
	// Without the editor wired the action says so
	r, _ = New(func() (string, error) { return "", nil }, nil).Act(ctx, view.Action{Name: "options"})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("unwired = %+v", r)
	}
}
