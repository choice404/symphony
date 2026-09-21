package host

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/choice404/symphony/internal/nvim"
	"github.com/choice404/symphony/internal/rpc"
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

func (fake) Act(_ context.Context, a view.Action) (view.Response, error) {
	if a.Name == "open" {
		return view.Show(view.Page{Name: "fake/" + a.Key, Lines: []string{"opened " + a.Key + " from " + a.Page}, Filetype: "item"}), nil
	}
	return view.Notify("did " + a.Name), nil
}

// serve starts a server on a scratch socket and returns the socket path and the server
func serve(t *testing.T) (string, *Server) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.sock")
	ln, err := rpc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := view.NewRegistry(fake{})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(reg, t.Logf)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), ln) }()
	t.Cleanup(func() {
		srv.Stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("serve did not return after stop")
		}
	})
	return path, srv
}

func TestGoClientRenderAndAct(t *testing.T) {
	path, _ := serve(t)
	c, err := rpc.Connect(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	var pong string
	if err := c.Call(PingMethod, &pong); err != nil || pong != "pong" {
		t.Fatalf("ping = %q %v", pong, err)
	}
	var page map[string]interface{}
	if err := c.Call(RenderMethod, &page, "fake"); err != nil {
		t.Fatal(err)
	}
	if page["name"] != "fake" {
		t.Fatalf("page = %v", page)
	}
	var resp map[string]interface{}
	if err := c.Call(ActionMethod, &resp, "fake", "refresh", "", "fake", ""); err != nil {
		t.Fatal(err)
	}
	if resp["kind"] != "notify" || resp["text"] != "did refresh" {
		t.Fatalf("resp = %v", resp)
	}
	var names []string
	if err := c.Call(ViewsMethod, &names); err != nil || len(names) != 1 || names[0] != "fake" {
		t.Fatalf("names = %v %v", names, err)
	}
	// An unknown view is an rpc error
	if err := c.Call(RenderMethod, &page, "ghost"); err == nil {
		t.Fatal("expected error for unknown view")
	}
}

func TestStopEndsServe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	ln, err := rpc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := view.NewRegistry(fake{})
	srv := NewServer(reg, t.Logf)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), ln) }()
	c, err := rpc.Connect(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	var ok bool
	if err := c.Call(StopMethod, &ok); err != nil || !ok {
		t.Fatalf("stop = %v %v", ok, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after stop request")
	}
}

func TestNvimDialsDaemonAndShowsPage(t *testing.T) {
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not on PATH")
	}
	path, _ := serve(t)
	plugin, _ := filepath.Abs(pluginDir)
	sess, err := nvim.Start(context.Background(), nvim.Options{PluginDir: plugin, Args: []string{"--clean"}, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	if err := sess.Attach(60, 20); err != nil {
		t.Fatal(err)
	}
	// The plugin dials the socket, asks for the page, and fills a buffer
	var lines []string
	err = sess.Exec(`
		local path = ...
		local chan, err = require("symphony.rpc").connect(path)
		if not chan then error(err) end
		local buf = require("symphony.view").open("fake")
		return vim.api.nvim_buf_get_lines(buf, 0, -1, false)
	`, &lines, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("lines = %v", lines)
	}
	// An action on the second line opens the item
	err = sess.Exec(`
		local view = require("symphony.view")
		vim.api.nvim_win_set_cursor(0, { 2, 0 })
		view.act("open")
		return vim.api.nvim_buf_get_lines(0, 0, -1, false)
	`, &lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "opened k2 from fake" {
		t.Fatalf("lines = %v", lines)
	}
}
