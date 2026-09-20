package nvim

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pluginDir is the plugin source tree relative to this package
const pluginDir = "../../nvim/symphony.nvim"

// startTest spawns a real nvim with the plugin and a clean config, skipping when nvim is missing
func startTest(t *testing.T) (*Session, chan Screen) {
	t.Helper()
	// Skip without nvim
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not on PATH")
	}
	// The plugin path made absolute
	plugin, err := filepath.Abs(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	// Every flush lands here, buffered so the redraw goroutine never blocks
	flushes := make(chan Screen, 64)
	// Start with no user config so the test is the same on every machine
	sess, err := Start(context.Background(), Options{
		PluginDir: plugin,
		Args:      []string{"--clean"},
		OnFlush:   func(s Screen) { flushes <- s },
		Logf:      t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Close on cleanup
	t.Cleanup(func() { _ = sess.Close() })
	// Attach at a small size
	if err := sess.Attach(40, 10); err != nil {
		t.Fatal(err)
	}
	return sess, flushes
}

// waitFor drains flushes until one contains the text or the deadline passes
func waitFor(t *testing.T, flushes chan Screen, text string) Screen {
	t.Helper()
	// The deadline
	deadline := time.After(5 * time.Second)
	// Loop until found
	for {
		select {
		case s := <-flushes:
			// Check every row
			for r := 0; r < s.Height; r++ {
				if strings.Contains(s.Line(r), text) {
					return s
				}
			}
		case <-deadline:
			t.Fatalf("no flush containing %q", text)
		}
	}
}

func TestSessionEchoRendersOnScreen(t *testing.T) {
	sess, flushes := startTest(t)
	// Run an echo through the command line
	if err := sess.Input(":echo 'symphony-echo-ok'<CR>"); err != nil {
		t.Fatal(err)
	}
	// The message line shows it
	s := waitFor(t, flushes, "symphony-echo-ok")
	if s.Width != 40 || s.Height != 10 {
		t.Fatalf("size = %dx%d", s.Width, s.Height)
	}
}

func TestSessionInsertTextAndCursor(t *testing.T) {
	sess, flushes := startTest(t)
	// Type a word in insert mode and leave
	if err := sess.Input("ihello<Esc>"); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, flushes, "hello")
	// The cursor sits on the last letter in normal mode
	if s.CursorRow != 0 || s.CursorCol != 4 {
		t.Fatalf("cursor = %d,%d", s.CursorRow, s.CursorCol)
	}
	if s.Mode != "normal" {
		t.Fatalf("mode = %q", s.Mode)
	}
}

func TestSessionResize(t *testing.T) {
	sess, flushes := startTest(t)
	if err := sess.Resize(60, 12); err != nil {
		t.Fatal(err)
	}
	// Wait for a flush at the new size
	deadline := time.After(5 * time.Second)
	for {
		select {
		case s := <-flushes:
			if s.Width == 60 && s.Height == 12 {
				return
			}
		case <-deadline:
			t.Fatal("no flush at 60x12")
		}
	}
}

func TestSessionPluginPingRoundTrip(t *testing.T) {
	sess, _ := startTest(t)
	// The plugin asks the host and the host answers
	var reply string
	if err := sess.Exec(`return require("symphony.rpc").request("symphony.ping")`, &reply); err != nil {
		t.Fatal(err)
	}
	if reply != "pong" {
		t.Fatalf("reply = %q", reply)
	}
}

func TestSessionPluginCommandRegistered(t *testing.T) {
	sess, _ := startTest(t)
	// The :Symphony command exists once the plugin loaded from the runtimepath
	var exists bool
	if err := sess.Exec(`return vim.api.nvim_get_commands({}).Symphony ~= nil`, &exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Symphony command missing")
	}
}

func TestSessionExitReported(t *testing.T) {
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not on PATH")
	}
	plugin, _ := filepath.Abs(pluginDir)
	// The exit lands here
	exited := make(chan error, 1)
	sess, err := Start(context.Background(), Options{
		PluginDir: plugin,
		Args:      []string{"--clean"},
		OnExit:    func(err error) { exited <- err },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Attach(40, 10); err != nil {
		t.Fatal(err)
	}
	// Quit from inside nvim
	if err := sess.Input(":qa!<CR>"); err != nil {
		t.Fatal(err)
	}
	// The serve loop ends
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("OnExit never fired")
	}
}

func TestInstallPluginWritesTree(t *testing.T) {
	// Point the cache at a scratch directory
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir, err := InstallPlugin(os.DirFS(filepath.Clean(pluginDir)), ".")
	if err != nil {
		t.Fatal(err)
	}
	// The plugin file and the lua entry point landed
	for _, rel := range []string{"plugin/symphony.lua", "lua/symphony/init.lua"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("%s missing: %v", rel, err)
		}
	}
}
