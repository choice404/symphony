package launch

import (
	"path/filepath"
	"testing"

	"github.com/choice404/symphony/internal/rpc"
)

func TestEnsureDaemonReturnsWhenAlive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	// Something listening is enough, no daemon is spawned
	ln, err := rpc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if err := EnsureDaemon(path, filepath.Join(t.TempDir(), "d.log")); err != nil {
		t.Fatal(err)
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
