// The symphony TUI, an embedded nvim drawn through bubbletea with every app shown as a buffer
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
	"github.com/choice404/symphony/internal/host"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/nvim"
	"github.com/choice404/symphony/internal/ui"
	"github.com/choice404/symphony/internal/view"
)

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
 * Loads the config, installs the plugin, starts nvim, wires the views, runs the program, and always closes nvim on the way out
 * @return error
 **/
func run() error {
	// The context that kills nvim when we leave
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Read the config
	cfg, err := config.Load()
	if err != nil {
		return err
	}
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
	// Build the views and answer the plugin from them
	reg, err := buildViews(cfg)
	if err != nil {
		return err
	}
	if err := host.Register(ctx, sess, reg); err != nil {
		return err
	}
	// Open home after attach unless a file was named on the command line
	var onAttach func() error
	if !hasFileArg(os.Args[1:]) {
		onAttach = func() error { return host.OpenHome(sess) }
	}
	// Build the program on the alternate screen
	p := tea.NewProgram(ui.New(sess, onAttach), tea.WithAltScreen())
	prog.Store(p)
	// Run it
	final, err := p.Run()
	if err != nil {
		return err
	}
	// Log the error that ended the model, a closed pipe on :q is normal and not reported
	if m, ok := final.(ui.Model); ok && m.Err != nil {
		logger.Printf("exit: %v", m.Err)
	}
	return nil
}

/**
 * buildViews
 * Builds every view and the home page that lists them
 * @param cfg {config.Config} - the config
 * @return view.Registry, error
 **/
func buildViews(cfg config.Config) (view.Registry, error) {
	// The registry, assigned after home so the opener closes over it
	var reg view.Registry
	// The mail view
	mailView := mail.NewView(cfg.Mail.Maildir)
	// The home view opens entries through the registry
	open := func(ctx context.Context, name string) (view.Page, error) { return reg.Render(ctx, name) }
	home := view.NewHome(open, view.Entry{Name: mail.ViewName, Label: "mail", Summary: mailView.Summary})
	// Build the registry
	var err error
	reg, err = view.NewRegistry(home, mailView)
	if err != nil {
		return view.Registry{}, err
	}
	return reg, nil
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
