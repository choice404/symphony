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
	// BrowserStopMethod closes the daemon's browser so a headed window can use its profile
	BrowserStopMethod = "symphony.browser.stop"
	// ReloadMethod reads the config again and rebuilds the views
	ReloadMethod = "symphony.reload"
	// EntriesMethod lists the home entries with their summaries, for the dashboard
	EntriesMethod = "symphony.entries"
)

// stopDelay gives the stop reply time to reach the caller before the listener closes
const stopDelay = 100 * time.Millisecond

// Server answers requests from every connection with one registry
type Server struct {
	// The views, swapped whole on a reload
	reg view.Registry
	// Guards reg
	mu sync.RWMutex
	// Reads the config again and rebuilds the views, nil when the daemon did not wire it
	Reload func() error
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
 * Swap
 * Replaces the views, every later request goes to the new ones
 * @param reg {view.Registry} - the new views
 * @return void
 **/
func (s *Server) Swap(reg view.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reg = reg
}

/**
 * views
 * Returns the current views
 * @return view.Registry
 **/
func (s *Server) views() view.Registry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reg
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
			p, err := s.views().Render(ctx, name)
			if err != nil {
				return nil, err
			}
			return p.ToMap(), nil
		}),
		ep.Register(ActionMethod, func(name, action, key, page, body string) (map[string]interface{}, error) {
			r, err := s.views().Act(ctx, name, view.Action{Name: action, Key: key, Page: page, Body: body})
			if err != nil {
				return nil, err
			}
			return r.ToMap(), nil
		}),
		ep.Register(ViewsMethod, func() ([]string, error) { return s.views().Names(), nil }),
		ep.Register(EntriesMethod, func() ([]map[string]interface{}, error) {
			v, err := s.views().Get("home")
			if err != nil {
				return nil, err
			}
			h, ok := v.(*view.Home)
			if !ok {
				return nil, errors.New("entries: home is not the home view")
			}
			return h.Items(ctx), nil
		}),
		ep.Register(ReloadMethod, func() (bool, error) {
			if s.Reload == nil {
				return false, errors.New("reload: not available")
			}
			if err := s.Reload(); err != nil {
				return false, err
			}
			return true, nil
		}),
		ep.Register(VersionMethod, func() (string, error) { return s.Version, nil }),
		ep.Register(BrowserStopMethod, func() (bool, error) {
			if BrowserEngine != nil {
				BrowserEngine.Stop()
			}
			return true, nil
		}),
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
