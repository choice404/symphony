// The symphony TUI, an embedded nvim drawn through bubbletea and connected to the daemon
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/choice404/symphony"
	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/launch"
	"github.com/choice404/symphony/internal/nvim"
	"github.com/choice404/symphony/internal/rpc"
	"github.com/choice404/symphony/internal/ui"
)

// startLua dials the daemon from inside nvim and opens home unless a dashboard already stands in for it or a file was named
const startLua = `
local path, open_home = ...
local chan, err = require("symphony").start(path, open_home)
if not chan then error(err, 0) end
`

/**
 * main
 * Runs the program and turns its error into an exit code
 * @return void
 **/
func main() {
	// Run and report
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "symphony:", err)
		os.Exit(1)
	}
}

/**
 * run
 * Makes sure the daemon is up, installs the plugin, starts nvim, and runs the program, always closing nvim on the way out
 * @return error
 **/
func run() error {
	// The context that kills nvim when we leave
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Put the embedded plugin where nvim can load it
	pluginDir, err := nvim.InstallPlugin(symphony.PluginFS, symphony.PluginRoot)
	if err != nil {
		return err
	}
	// The cache directory the plugin sits under, logs go beside it
	cacheDir := filepath.Dir(filepath.Dir(pluginDir))
	// Open the log file
	logger, closeLog, err := openLog(filepath.Join(cacheDir, "symphony.log"))
	if err != nil {
		return err
	}
	defer closeLog()
	// Make sure a daemon answers on the socket
	sock, err := rpc.SocketPath()
	if err != nil {
		return err
	}
	if err := launch.EnsureDaemon(sock, filepath.Join(cacheDir, "symphonyd.log")); err != nil {
		return err
	}
	// The program, set once it exists so the redraw goroutine can reach it
	var prog atomic.Pointer[tea.Program]
	// Hands a message to the program when it exists
	send := func(msg tea.Msg) {
		if p := prog.Load(); p != nil {
			p.Send(msg)
		}
	}
	// The first run writes a config with every key at its default so there is always a file to edit
	if path, created, err := config.Ensure(symphony.ExampleConfig); err != nil {
		return err
	} else if created {
		logger.Printf("wrote %s", path)
	}
	// The shipped editor config runs under its own app name unless the config asks for your own
	env := []string{rpc.EnvSocket + "=" + sock, "SYMPHONY_PLUGIN=" + pluginDir}
	cfg, _ := config.Load()
	if !cfg.Editor.Own() {
		if _, err := nvim.InstallConfig(symphony.ConfigFS, symphony.ConfigRoot); err != nil {
			return err
		}
		env = append(env, "NVIM_APPNAME="+nvim.AppName)
	}
	// Start nvim with the plugin on its runtimepath
	sess, err := nvim.Start(ctx, nvim.Options{
		PluginDir: pluginDir,
		Args:      os.Args[1:],
		Env:       env,
		OnFlush:   func(s nvim.Screen) { send(ui.FlushMsg{Screen: s}) },
		OnExit:    func(err error) { send(ui.ExitMsg{Err: err}) },
		Logf:      logger.Printf,
	})
	if err != nil {
		return err
	}
	// Always close nvim, whatever way the program ends
	defer func() { _ = sess.Close() }()
	// After attach the plugin dials the daemon and opens home, unless a file was named on the command line or the dashboard is up
	onAttach := func() error { return sess.Exec(startLua, nil, sock, !hasFileArg(os.Args[1:])) }
	// Build the program on the alternate screen
	p := tea.NewProgram(ui.New(sess, onAttach), tea.WithAltScreen())
	prog.Store(p)
	// Run it
	final, err := p.Run()
	if err != nil {
		return err
	}
	// A failed host call is reported, a closed pipe on :q is normal and only logged
	if m, ok := final.(ui.Model); ok {
		if m.ExitErr != nil {
			logger.Printf("nvim exit: %v", m.ExitErr)
		}
		if m.Err != nil {
			logger.Printf("exit: %v", m.Err)
			return m.Err
		}
	}
	return nil
}

/**
 * hasFileArg
 * Reports whether any argument names a file rather than a flag
 * @param args {[]string} - the arguments after the binary name
 * @return bool
 **/
func hasFileArg(args []string) bool {
	// Loop over every argument
	for i := 0; i < len(args); i++ {
		// Skip flags and the value of a flag that takes one
		if strings.HasPrefix(args[i], "-") {
			if args[i] == "-u" || args[i] == "-i" || args[i] == "--cmd" || args[i] == "-c" || args[i] == "--listen" {
				i++
			}
			continue
		}
		return true
	}
	return false
}

/**
 * openLog
 * Opens a log file for append
 * @param path {string} - the file
 * @return *log.Logger, func(), error
 **/
func openLog(path string) (*log.Logger, func(), error) {
	// Open for append
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	// Return the logger and its closer
	return log.New(f, "", log.LstdFlags), func() { _ = f.Close() }, nil
}
