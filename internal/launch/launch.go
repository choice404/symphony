// Package launch starts the daemon when nothing is listening on its socket, and replaces one built from an older binary
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

// VersionMethod asks a daemon for the build id of the binary it runs from
const VersionMethod = "symphony.version"

// StopMethod asks a daemon to exit, the same name the host package serves
const StopMethod = "symphony.stop"

// waitFor is how long to wait for a spawned daemon to answer
const waitFor = 5 * time.Second

// pollEvery is how often to try the socket while waiting
const pollEvery = 50 * time.Millisecond

/**
 * BuildID
 * Identifies a binary by its size and modification time, enough to tell a rebuilt daemon from the one still running
 * @param path {string} - the binary
 * @return string, error
 **/
func BuildID(path string) (string, error) {
	// Stat the file
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("build id: %w", err)
	}
	// Size and mtime as one string
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()), nil
}

/**
 * EnsureDaemon
 * Returns once a daemon built from the current binary answers on the socket, spawning one when none does and replacing a stale one
 * @param sock {string} - the socket path
 * @param logPath {string} - where the spawned daemon's output goes
 * @return error
 **/
func EnsureDaemon(sock, logPath string) error {
	// Find the daemon binary
	bin, err := findDaemon()
	if err != nil {
		// Something alive is still usable when there is nothing to spawn
		if alive(sock) {
			return nil
		}
		return err
	}
	// Done when the running daemon is this binary
	if alive(sock) {
		if !Stale(sock, bin) {
			return nil
		}
		// Replace it
		if err := stopAndWait(sock); err != nil {
			return err
		}
	}
	// Spawn
	if err := spawn(bin, sock, logPath); err != nil {
		return err
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
 * Stale
 * Reports whether the daemon on the socket runs an older build than the binary, a daemon that cannot say is stale
 * @param sock {string} - the socket path
 * @param bin {string} - the daemon binary that would be spawned
 * @return bool
 **/
func Stale(sock, bin string) bool {
	// The id of the binary on disk
	want, err := BuildID(bin)
	if err != nil {
		return false
	}
	// The id the daemon reports
	c, err := rpc.Connect(sock)
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	var got string
	if err := c.Call(VersionMethod, &got); err != nil {
		return true
	}
	return got != want
}

/**
 * stopAndWait
 * Asks the daemon on the socket to exit and waits until nothing answers
 * @param sock {string} - the socket path
 * @return error
 **/
func stopAndWait(sock string) error {
	// Ask
	c, err := rpc.Connect(sock)
	if err != nil {
		return nil
	}
	var ok bool
	_ = c.Call(StopMethod, &ok)
	_ = c.Close()
	// Wait for the socket to go quiet
	deadline := time.Now().Add(waitFor)
	for time.Now().Before(deadline) {
		if !alive(sock) {
			return nil
		}
		time.Sleep(pollEvery)
	}
	return fmt.Errorf("the daemon on %s did not stop", sock)
}

/**
 * spawn
 * Starts the daemon in its own session with its output in a log
 * @param bin {string} - the binary
 * @param sock {string} - the socket path handed over in the env
 * @param logPath {string} - the log file
 * @return error
 **/
func spawn(bin, sock, logPath string) error {
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
	return nil
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
 * Finds the daemon binary beside this executable first, then on the path
 * @return string, error
 **/
func findDaemon() (string, error) {
	// Beside this binary, so a fresh build wins over an older install on the path
	self, err := os.Executable()
	if err == nil {
		beside := filepath.Join(filepath.Dir(self), daemonName)
		if _, err := os.Stat(beside); err == nil {
			return beside, nil
		}
	}
	// On the path
	if p, err := exec.LookPath(daemonName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found beside %s or on PATH", daemonName, self)
}
