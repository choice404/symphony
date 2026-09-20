package launch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	msgrpc "github.com/neovim/go-client/msgpack/rpc"

	"github.com/choice404/symphony/internal/rpc"
)

// fakeDaemon listens on a socket answering version with the given id, and records a stop by closing the listener
func fakeDaemon(t *testing.T, path, version string, withVersion bool) chan struct{} {
	t.Helper()
	ln, err := rpc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			ep, _ := msgrpc.NewEndpoint(conn, conn, conn)
			// An old daemon has no version to give, the endpoint needs a handler either way so it answers with an error
			if withVersion {
				_ = ep.Register(VersionMethod, func() (string, error) { return version, nil })
			} else {
				_ = ep.Register(VersionMethod, func() (string, error) { return "", errors.New("no such method") })
			}
			_ = ep.Register(StopMethod, func() (bool, error) {
				go func() {
					time.Sleep(50 * time.Millisecond)
					_ = ln.Close()
					_ = os.Remove(path)
					close(stopped)
				}()
				return true, nil
			})
			go func() { _ = ep.Serve() }()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return stopped
}

// fakeBinary writes a file to stand in for the daemon binary and returns its path and id
func fakeBinary(t *testing.T) (string, string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "symphonyd")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := BuildID(bin)
	if err != nil {
		t.Fatal(err)
	}
	return bin, id
}

func TestStaleByVersion(t *testing.T) {
	bin, id := fakeBinary(t)
	// The same id is current
	sock := filepath.Join(t.TempDir(), "s.sock")
	fakeDaemon(t, sock, id, true)
	if Stale(sock, bin) {
		t.Fatal("matching build reported stale")
	}
	// A different id is stale
	sock2 := filepath.Join(t.TempDir(), "s2.sock")
	fakeDaemon(t, sock2, "older", true)
	if !Stale(sock2, bin) {
		t.Fatal("older build reported current")
	}
	// A daemon with no version method is stale
	sock3 := filepath.Join(t.TempDir(), "s3.sock")
	fakeDaemon(t, sock3, "", false)
	if !Stale(sock3, bin) {
		t.Fatal("daemon without version reported current")
	}
}

func TestEnsureDaemonKeepsCurrentOne(t *testing.T) {
	bin, id := fakeBinary(t)
	t.Setenv("PATH", filepath.Dir(bin))
	sock := filepath.Join(t.TempDir(), "s.sock")
	stopped := fakeDaemon(t, sock, id, true)
	if err := EnsureDaemon(sock, filepath.Join(t.TempDir(), "d.log")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
		t.Fatal("current daemon was stopped")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestEnsureDaemonStopsStaleOne(t *testing.T) {
	bin, _ := fakeBinary(t)
	t.Setenv("PATH", filepath.Dir(bin))
	sock := filepath.Join(t.TempDir(), "s.sock")
	stopped := fakeDaemon(t, sock, "older", true)
	// The stale one is asked to stop, then the fake binary exits at once so the spawn never answers
	err := EnsureDaemon(sock, filepath.Join(t.TempDir(), "d.log"))
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("stale daemon was not stopped")
	}
	if err == nil {
		t.Fatal("expected an error since the fake binary answers nothing")
	}
}

func TestEnsureDaemonFailsWithoutBinary(t *testing.T) {
	// No daemon on an empty PATH and none beside the test binary
	t.Setenv("PATH", t.TempDir())
	path := filepath.Join(t.TempDir(), "s.sock")
	if err := EnsureDaemon(path, filepath.Join(t.TempDir(), "d.log")); err == nil {
		t.Fatal("expected an error when the daemon binary is missing")
	}
}
