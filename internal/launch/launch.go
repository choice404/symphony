// Package launch starts the daemon when nothing is listening on its socket
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/choice404/symphony/internal/rpc"
)

// daemonName is the daemon binary
const daemonName = "symphonyd"

// waitFor is how long to wait for a spawned daemon to answer
const waitFor = 5 * time.Second

// pollEvery is how often to try the socket while waiting
const pollEvery = 50 * time.Millisecond

/**
 * EnsureDaemon
 * Returns once a daemon answers on the socket, spawning one detached when none does
 * @param sock {string} - the socket path
 * @param logPath {string} - where the spawned daemon's output goes
 * @return error
 **/
func EnsureDaemon(sock, logPath string) error {
	// Done when something already answers
	if alive(sock) {
		return nil
	}
	// Find the daemon binary
	bin, err := findDaemon()
	if err != nil {
		return err
	}
	// Open the log for the child
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("daemon log: %w", err)
	}
	defer func() { _ = logf.Close() }()
	// Spawn it in its own session so it outlives the TUI
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), rpc.EnvSocket+"="+sock)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}
	// Let it go
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release %s: %w", bin, err)
	}
	// Wait for it to answer
	deadline := time.Now().Add(waitFor)
	for time.Now().Before(deadline) {
		if alive(sock) {
			return nil
		}
		time.Sleep(pollEvery)
	}
	return fmt.Errorf("%s did not answer on %s, see %s", bin, sock, logPath)
}

/**
 * alive
 * Reports whether something accepts a connection on the socket
 * @param sock {string} - the socket path
 * @return bool
 **/
func alive(sock string) bool {
	// Dial and close
	conn, err := rpc.Dial(sock)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

/**
 * findDaemon
 * Finds the daemon binary on the path or beside this executable
 * @return string, error
 **/
func findDaemon() (string, error) {
	// On the path
	if p, err := exec.LookPath(daemonName); err == nil {
		return p, nil
	}
	// Beside this binary
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find %s: %w", daemonName, err)
	}
	beside := filepath.Join(filepath.Dir(self), daemonName)
	if _, err := os.Stat(beside); err == nil {
		return beside, nil
	}
	return "", fmt.Errorf("%s not found on PATH or beside %s", daemonName, self)
}
