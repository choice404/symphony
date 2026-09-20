// The symphony TUI, an embedded nvim drawn through bubbletea
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/choice404/symphony"
	"github.com/choice404/symphony/internal/nvim"
	"github.com/choice404/symphony/internal/ui"
)

/**
 * main
 * Installs the plugin, starts nvim, runs the program, and always closes nvim on the way out
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
 * The whole program so main can turn its error into an exit code
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
	// Open the log file next to the plugin
	logger, closeLog, err := openLog(pluginDir)
	if err != nil {
		return err
	}
	defer closeLog()
	// The program, set once it exists so the redraw goroutine can reach it
	var prog atomic.Pointer[tea.Program]
	// Hands a message to the program when it exists
	send := func(msg tea.Msg) {
		if p := prog.Load(); p != nil {
			p.Send(msg)
		}
	}
	// Start nvim with the plugin on its runtimepath
	sess, err := nvim.Start(ctx, nvim.Options{
		PluginDir: pluginDir,
		Args:      os.Args[1:],
		OnFlush:   func(s nvim.Screen) { send(ui.FlushMsg{Screen: s}) },
		OnExit:    func(err error) { send(ui.ExitMsg{Err: err}) },
		Logf:      logger.Printf,
	})
	if err != nil {
		return err
	}
	// Always close nvim, whatever way the program ends
	defer func() { _ = sess.Close() }()
	// Build the program on the alternate screen
	p := tea.NewProgram(ui.New(sess), tea.WithAltScreen())
	prog.Store(p)
	// Run it
	final, err := p.Run()
	if err != nil {
		return err
	}
	// Surface the error that ended the model, a closed pipe on :q is normal and not reported
	if m, ok := final.(ui.Model); ok && m.Err != nil {
		logger.Printf("exit: %v", m.Err)
	}
	return nil
}

/**
 * openLog
 * Opens the log file under the cache directory
 * @param pluginDir {string} - the installed plugin path, the log sits two levels up from it
 * @return *log.Logger, func(), error
 **/
func openLog(pluginDir string) (*log.Logger, func(), error) {
	// The log path, cache/symphony/symphony.log
	path := filepath.Join(filepath.Dir(filepath.Dir(pluginDir)), "symphony.log")
	// Open for append
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	// Return the logger and its closer
	return log.New(f, "", log.LstdFlags), func() { _ = f.Close() }, nil
}
