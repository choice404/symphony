// The symphony daemon, it holds the views and answers every nvim that dials its socket
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/choice404/symphony/internal/browser"
	"github.com/choice404/symphony/internal/calsync"
	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/host"
	"github.com/choice404/symphony/internal/launch"
	"github.com/choice404/symphony/internal/mailsync"
	"github.com/choice404/symphony/internal/oauth"
	"github.com/choice404/symphony/internal/rpc"
)

/**
 * main
 * Runs the subcommand and turns its error into an exit code
 * @return void
 **/
func main() {
	// The subcommand, run when none was given
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	// Dispatch
	var err error
	switch cmd {
	case "run":
		err = run()
	case "stop":
		err = stop()
	case "status":
		err = status()
	case "mail":
		err = mailCmd(os.Args[2:])
	case "authorize":
		err = mailCmd(append([]string{"authorize"}, os.Args[2:]...))
	case "calendar":
		err = calendarCmd(os.Args[2:])
	case "browser":
		err = browserCmd(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q, symphonyd takes run, stop, status, authorize, mail, or calendar", cmd)
	}
	// Report
	if err != nil {
		fmt.Fprintln(os.Stderr, "symphonyd:", err)
		os.Exit(1)
	}
}

/**
 * run
 * Serves the socket until a signal or a stop request
 * @return error
 **/
func run() error {
	// Stop on ctrl c or a terminate
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The log goes to stderr, the launcher points that at a file
	logger := log.New(os.Stderr, "", log.LstdFlags)
	// Read the config once so a broken file is reported at start
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Every later read goes back to the file, a bad edit keeps the last good config and is logged
	var lastMu sync.Mutex
	last := cfg
	load := func() config.Config {
		lastMu.Lock()
		defer lastMu.Unlock()
		c, err := config.Load()
		if err != nil {
			logger.Printf("config: %v, keeping the last good one", err)
			return last
		}
		last = c
		return c
	}
	// The server is filled in below, the reloader needs it to swap the views
	var srv *host.Server
	var closeViews func()
	var rebuildMu sync.Mutex
	// A reload checks the file, builds the views again, swaps them in, and closes the old ones
	var reload func() error
	reload = func() error {
		rebuildMu.Lock()
		defer rebuildMu.Unlock()
		if _, err := config.Load(); err != nil {
			return err
		}
		next, closeNext, err := host.Views(load, logger.Printf, reload)
		if err != nil {
			return err
		}
		old := closeViews
		srv.Swap(next)
		closeViews = closeNext
		old()
		logger.Printf("config reloaded")
		return nil
	}
	// Build the views and keep their closer for the way out
	reg, closeFirst, err := host.Views(load, logger.Printf, reload)
	if err != nil {
		return err
	}
	closeViews = closeFirst
	defer func() { closeViews() }()
	// Listen
	sock, err := rpc.SocketPath()
	if err != nil {
		return err
	}
	ln, err := rpc.Listen(sock)
	if err != nil {
		return err
	}
	logger.Printf("listening on %s", sock)
	// Sync the accounts that ask for it on their own clocks, mail and calendars
	go mailsync.Loop(ctx, load, logger.Printf)
	go calsync.Loop(ctx, load, host.CalendarStore(), logger.Printf)
	// Serve, reporting this binary's build id so a TUI can tell a stale daemon apart
	srv = host.NewServer(reg, logger.Printf)
	srv.Reload = reload
	if exe, err := os.Executable(); err == nil {
		srv.Version, _ = launch.BuildID(exe)
	}
	err = srv.Serve(ctx, ln)
	// Leave nothing behind
	_ = os.Remove(sock)
	logger.Printf("stopped")
	return err
}

/**
 * stop
 * Asks the running daemon to exit
 * @return error
 **/
func stop() error {
	// Connect
	c, err := connect()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	// Ask
	var ok bool
	if err := c.Call(host.StopMethod, &ok); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	fmt.Println("stopping")
	return nil
}

/**
 * status
 * Pings the running daemon
 * @return error
 **/
func status() error {
	// Connect
	c, err := connect()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	// Ping
	var reply string
	if err := c.Call(host.PingMethod, &reply); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	// Report the socket, the views, and the build
	sock, _ := rpc.SocketPath()
	var names []string
	_ = c.Call(host.ViewsMethod, &names)
	var version string
	_ = c.Call(host.VersionMethod, &version)
	fmt.Printf("running on %s, views: %v, build %s\n", sock, names, version)
	return nil
}

/**
 * mailCmd
 * Runs the mail subcommands, authorize <account> for the browser login and sync [account] for a sync now
 * @param args {[]string} - the words after mail
 * @return error
 **/
func mailCmd(args []string) error {
	// The subcommand
	if len(args) == 0 {
		return fmt.Errorf("symphonyd mail takes authorize <account> or sync [account]")
	}
	// The config and a logger to stderr
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := log.New(os.Stderr, "", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Dispatch
	switch args[0] {
	case "authorize":
		if len(args) < 2 {
			return fmt.Errorf("symphonyd mail authorize <account>")
		}
		a, ok := account(cfg, args[1])
		if !ok {
			return fmt.Errorf("no account named %q in the config", args[1])
		}
		if a.User == "" {
			return fmt.Errorf("%s: set user to the full address before authorizing", a.Name)
		}
		return oauth.Authorize(ctx, a.Name, a.User, os.Stdout)
	case "sync":
		// One account or every synced one
		var failed int
		for _, a := range cfg.Mail.All() {
			if len(args) > 1 && a.Name != args[1] {
				continue
			}
			if !a.Synced() {
				if len(args) > 1 {
					return fmt.Errorf("%s has no auth set, the daemon does not sync it", a.Name)
				}
				continue
			}
			if _, err := mailsync.Run(ctx, a, logger.Printf); err != nil {
				logger.Printf("%s: sync failed: %v", a.Name, err)
				failed++
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d account(s) failed", failed)
		}
		return nil
	}
	return fmt.Errorf("unknown mail command %q, symphonyd mail takes authorize or sync", args[0])
}

/**
 * browserCmd
 * Runs the browser subcommands, login [url] opens a real window on the daemon's profile after asking the daemon to let go of it
 * @param args {[]string} - the words after browser
 * @return error
 **/
func browserCmd(args []string) error {
	if len(args) == 0 || args[0] != "login" {
		return fmt.Errorf("symphonyd browser takes login [url]")
	}
	target := "https://discord.com/login"
	if len(args) > 1 {
		target = args[1]
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	chrome := cfg.Browser.Chrome
	if chrome == "" {
		chrome = browser.FindChrome()
	}
	if chrome == "" {
		return fmt.Errorf("no chromium found, install google-chrome, chromium, or brave, or set chrome under [browser]")
	}
	// The daemon's headless browser holds the profile, ask it to let go
	if c, err := connect(); err == nil {
		var ok bool
		_ = c.Call(host.BrowserStopMethod, &ok)
		_ = c.Close()
	}
	home, _ := os.UserHomeDir()
	profile := filepath.Join(home, ".local", "share", "symphony", "browser")
	fmt.Println("a browser window is opening on symphony's profile, log in there, then close the window")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := browser.RunHeaded(ctx, chrome, profile, target); err != nil {
		return err
	}
	fmt.Println("done, the daemon starts its browser again on the next page")
	return nil
}

/**
 * calendarCmd
 * Runs the calendar subcommands, sync [account] warms the cache now
 * @param args {[]string} - the words after calendar
 * @return error
 **/
func calendarCmd(args []string) error {
	if len(args) == 0 || args[0] != "sync" {
		return fmt.Errorf("symphonyd calendar takes sync [account]")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := log.New(os.Stderr, "", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store := host.CalendarStore()
	failed := 0
	for _, a := range cfg.Mail.All() {
		if len(args) > 1 && a.Name != args[1] {
			continue
		}
		if a.Auth != "oauth" {
			if len(args) > 1 {
				return fmt.Errorf("%s is not an oauth account, calendars need one", a.Name)
			}
			continue
		}
		if err := calsync.Warm(ctx, a, cfg.Calendar.Ahead(), store, logger.Printf); err != nil {
			logger.Printf("%s: calendar sync failed: %v", a.Name, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d account(s) failed", failed)
	}
	return nil
}

/**
 * account
 * Finds an account by name
 * @param cfg {config.Config} - the config
 * @param name {string} - the name
 * @return config.Account, bool
 **/
func account(cfg config.Config, name string) (config.Account, bool) {
	for _, a := range cfg.Mail.All() {
		if a.Name == name {
			return a, true
		}
	}
	return config.Account{}, false
}

/**
 * connect
 * Connects to the daemon socket
 * @return *rpc.Client, error
 **/
func connect() (*rpc.Client, error) {
	// The socket
	sock, err := rpc.SocketPath()
	if err != nil {
		return nil, err
	}
	// Dial
	c, err := rpc.Connect(sock)
	if err != nil {
		return nil, fmt.Errorf("no daemon on %s", sock)
	}
	return c, nil
}
