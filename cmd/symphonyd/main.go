// The symphony daemon, it holds the views and answers every nvim that dials its socket
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/host"
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
	default:
		err = fmt.Errorf("unknown command %q, symphonyd takes run, stop, or status", cmd)
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
	// Read the config
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Build the views
	reg, err := host.Views(cfg)
	if err != nil {
		return err
	}
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
	// Serve
	srv := host.NewServer(reg, logger.Printf)
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
	// Report the socket and the views
	sock, _ := rpc.SocketPath()
	var names []string
	_ = c.Call(host.ViewsMethod, &names)
	fmt.Printf("running on %s, views: %v\n", sock, names)
	return nil
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
