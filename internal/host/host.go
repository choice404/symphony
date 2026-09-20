// Package host serves the views to every nvim that dials the daemon socket
package host

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	msgrpc "github.com/neovim/go-client/msgpack/rpc"

	"github.com/choice404/symphony/internal/view"
)

// The request names the plugin uses
const (
	// PingMethod answers pong
	PingMethod = "symphony.ping"
	// RenderMethod renders a view by name
	RenderMethod = "symphony.render"
	// ActionMethod runs an action on a view
	ActionMethod = "symphony.action"
	// ViewsMethod lists the view names
	ViewsMethod = "symphony.views"
	// StopMethod asks the daemon to exit
	StopMethod = "symphony.stop"
	// VersionMethod answers the build id of the binary the daemon runs from
	VersionMethod = "symphony.version"
)

// stopDelay gives the stop reply time to reach the caller before the listener closes
const stopDelay = 100 * time.Millisecond

// Server answers requests from every connection with one registry
type Server struct {
	// The views
	reg view.Registry
	// Where log lines go
	logf func(string, ...interface{})
	// Closed once when Stop is called
	stop chan struct{}
	// Guards the close of stop
	once sync.Once
	// The build id the daemon reports, empty when unknown
	Version string
}

/**
 * NewServer
 * Builds a server over a registry
 * @param reg {view.Registry} - the views
 * @param logf {func(string, ...interface{})} - where log lines go, nil for none
 * @return *Server
 **/
func NewServer(reg view.Registry, logf func(string, ...interface{})) *Server {
	// A silent logger when none was given
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	return &Server{reg: reg, logf: logf, stop: make(chan struct{})}
}

/**
 * Stop
 * Asks Serve to return, safe to call more than once
 * @return void
 **/
func (s *Server) Stop() {
	s.once.Do(func() { close(s.stop) })
}

/**
 * Serve
 * Accepts connections until the context ends or Stop is called, then closes the listener
 * @param ctx {context.Context} - the context
 * @param ln {net.Listener} - the socket
 * @return error
 **/
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	// Close the listener when told to stop, which ends the accept loop
	go func() {
		select {
		case <-ctx.Done():
		case <-s.stop:
		}
		_ = ln.Close()
	}()
	// Accept forever
	for {
		conn, err := ln.Accept()
		if err != nil {
			// A closed listener is the normal end
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		// Serve the connection on its own goroutine
		go s.handle(ctx, conn)
	}
}

/**
 * handle
 * Registers every method on one connection and serves it until it closes
 * @param ctx {context.Context} - the context every request runs under
 * @param conn {net.Conn} - the connection
 * @return void
 **/
func (s *Server) handle(ctx context.Context, conn net.Conn) {
	// The endpoint over the connection
	ep, err := msgrpc.NewEndpoint(conn, conn, conn, msgrpc.WithLogf(s.logf))
	if err != nil {
		s.logf("endpoint: %v", err)
		_ = conn.Close()
		return
	}
	// Register every method, stopping at the first failure
	err = errors.Join(
		ep.Register(PingMethod, func() (string, error) { return "pong", nil }),
		ep.Register(RenderMethod, func(name string) (map[string]interface{}, error) {
			p, err := s.reg.Render(ctx, name)
			if err != nil {
				return nil, err
			}
			return p.ToMap(), nil
		}),
		ep.Register(ActionMethod, func(name, action, key string) (map[string]interface{}, error) {
			r, err := s.reg.Act(ctx, name, action, key)
			if err != nil {
				return nil, err
			}
			return r.ToMap(), nil
		}),
		ep.Register(ViewsMethod, func() ([]string, error) { return s.reg.Names(), nil }),
		ep.Register(VersionMethod, func() (string, error) { return s.Version, nil }),
		ep.Register(StopMethod, func() (bool, error) {
			time.AfterFunc(stopDelay, s.Stop)
			return true, nil
		}),
	)
	if err != nil {
		s.logf("register: %v", err)
		_ = conn.Close()
		return
	}
	// Serve until the connection closes
	if err := ep.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
		s.logf("connection: %v", err)
	}
}
