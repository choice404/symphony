package rpc

import (
	"os"
	"path/filepath"
	"testing"

	msgrpc "github.com/neovim/go-client/msgpack/rpc"
)

func TestSocketPathOrder(t *testing.T) {
	t.Setenv(EnvSocket, "/tmp/override.sock")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/9")
	if p, _ := SocketPath(); p != "/tmp/override.sock" {
		t.Fatalf("override = %q", p)
	}
	t.Setenv(EnvSocket, "")
	if p, _ := SocketPath(); p != "/run/user/9/symphony/symphonyd.sock" {
		t.Fatalf("runtime = %q", p)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/cache")
	if p, _ := SocketPath(); p != "/tmp/cache/symphony/symphonyd.sock" {
		t.Fatalf("cache = %q", p)
	}
}

func TestListenPermissionsAndStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d", "s.sock")
	// A stale file at the path is removed
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	// The socket is private
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	// A second listen on a live socket is refused
	if _, err := Listen(path); err == nil {
		t.Fatal("expected error for a live socket")
	}
}

func TestClientRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	ln, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	// A server that echoes one method
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		ep, _ := msgrpc.NewEndpoint(conn, conn, conn)
		_ = ep.Register("echo", func(s string) (string, error) { return "got " + s, nil })
		_ = ep.Serve()
	}()
	c, err := Connect(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	var reply string
	if err := c.Call("echo", &reply, "hi"); err != nil {
		t.Fatal(err)
	}
	if reply != "got hi" {
		t.Fatalf("reply = %q", reply)
	}
}
