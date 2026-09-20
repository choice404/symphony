package host

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/choice404/symphony/internal/nvim"
	"github.com/choice404/symphony/internal/view"
)

// pluginDir is the plugin source tree relative to this package
const pluginDir = "../../nvim/symphony.nvim"

// fake is a view whose page and action are fixed
type fake struct{}

func (fake) Name() string { return "fake" }

func (fake) Render(context.Context) (view.Page, error) {
	return view.Page{Name: "fake", Lines: []string{"one", "two"}, Keys: []string{"k1", "k2"}, Cursor: 1, Filetype: "fake"}, nil
}

func (fake) Act(_ context.Context, action, key string) (view.Response, error) {
	if action == "open" {
		return view.Show(view.Page{Name: "fake/" + key, Lines: []string{"opened " + key}, Filetype: "item"}), nil
	}
	return view.Notify("did " + action), nil
}

// start spawns a real nvim with the plugin and the fake view registered
func start(t *testing.T) *nvim.Session {
	t.Helper()
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not on PATH")
	}
	plugin, _ := filepath.Abs(pluginDir)
	sess, err := nvim.Start(context.Background(), nvim.Options{PluginDir: plugin, Args: []string{"--clean"}, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	reg, err := view.NewRegistry(fake{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Register(context.Background(), sess, reg); err != nil {
		t.Fatal(err)
	}
	if err := sess.Attach(60, 20); err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestOpenShowsPageInBuffer(t *testing.T) {
	sess := start(t)
	// The plugin asks the host and fills a buffer
	var lines []string
	err := sess.Exec(`
		local buf = require("symphony.view").open("fake")
		return vim.api.nvim_buf_get_lines(buf, 0, -1, false)
	`, &lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("lines = %v", lines)
	}
	// The cursor landed on the page's line
	var row int
	if err := sess.Exec(`return vim.api.nvim_win_get_cursor(0)[1]`, &row); err != nil {
		t.Fatal(err)
	}
	if row != 2 {
		t.Fatalf("row = %d", row)
	}
}

func TestActionOpensItem(t *testing.T) {
	sess := start(t)
	// Open the page, move to the second line, and press enter through the action path
	var lines []string
	err := sess.Exec(`
		local view = require("symphony.view")
		view.open("fake")
		vim.api.nvim_win_set_cursor(0, { 2, 0 })
		view.act("open")
		return vim.api.nvim_buf_get_lines(0, 0, -1, false)
	`, &lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "opened k2" {
		t.Fatalf("lines = %v", lines)
	}
	// The buffer is named after the item
	var name string
	if err := sess.Exec(`return vim.api.nvim_buf_get_name(0)`, &name); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(name) != "k2" {
		t.Fatalf("name = %q", name)
	}
}

func TestViewsListed(t *testing.T) {
	sess := start(t)
	var names []string
	if err := sess.Exec(`return require("symphony.rpc").request("symphony.views")`, &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "fake" {
		t.Fatalf("names = %v", names)
	}
}

func TestUnknownViewIsAnError(t *testing.T) {
	sess := start(t)
	var msg string
	err := sess.Exec(`
		local ok, err = pcall(require("symphony.rpc").request, "symphony.render", "ghost")
		return tostring(err)
	`, &msg)
	if err != nil {
		t.Fatal(err)
	}
	if msg == "" || msg == "nil" {
		t.Fatalf("expected an error message, got %q", msg)
	}
}
