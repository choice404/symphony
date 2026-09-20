// Package rpc is the unix socket between the daemon and everything that talks to it
package rpc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	msgrpc "github.com/neovim/go-client/msgpack/rpc"
)

// sockRel is the socket path under the runtime directory
const sockRel = "symphony/symphonyd.sock"

// EnvSocket names the env var that overrides the socket path
const EnvSocket = "SYMPHONY_SOCKET"

/**
 * SocketPath
 * Returns the socket path, the env override, then $XDG_RUNTIME_DIR, then the cache directory
 * @return string, error
 **/
func SocketPath() (string, error) {
	// The override wins
	if p := os.Getenv(EnvSocket); p != "" {
		return p, nil
	}
	// The runtime directory when there is one
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, sockRel), nil
	}
	// The cache directory otherwise
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("socket path: %w", err)
	}
	return filepath.Join(cache, sockRel), nil
}

/**
 * Listen
 * Listens on a unix socket owned by the user alone, removing a stale socket nobody answers on
 * @param path {string} - the socket path
 * @return net.Listener, error
 **/
func Listen(path string) (net.Listener, error) {
	// The directory, private to the user
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("socket dir: %w", err)
	}
	// A socket that answers means another daemon is running
	if conn, err := net.Dial("unix", path); err == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("a daemon is already listening on %s", path)
	}
	// Anything left at the path is stale
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	// Listen
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", path, err)
	}
	// Only the user may connect
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return ln, nil
}

/**
 * Dial
 * Connects to the daemon socket
 * @param path {string} - the socket path
 * @return net.Conn, error
 **/
func Dial(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}

// Client is one msgpack-rpc connection to the daemon
type Client struct {
	// The endpoint over the connection
	ep *msgrpc.Endpoint
}

/**
 * NewClient
 * Wraps a connection and starts reading replies
 * @param conn {net.Conn} - the connection
 * @return *Client, error
 **/
func NewClient(conn net.Conn) (*Client, error) {
	// The endpoint
	ep, err := msgrpc.NewEndpoint(conn, conn, conn)
	if err != nil {
		return nil, err
	}
	// Read replies until the connection closes
	go func() { _ = ep.Serve() }()
	return &Client{ep: ep}, nil
}

/**
 * Connect
 * Dials the socket and wraps it
 * @param path {string} - the socket path
 * @return *Client, error
 **/
func Connect(path string) (*Client, error) {
	// Dial
	conn, err := Dial(path)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", path, err)
	}
	return NewClient(conn)
}

/**
 * Call
 * Sends a request and decodes the reply
 * @param method {string} - the method name
 * @param reply {interface{}} - where the reply lands, nil to drop it
 * @param args {...interface{}} - the arguments
 * @return error
 **/
func (c *Client) Call(method string, reply interface{}, args ...interface{}) error {
	return c.ep.Call(method, reply, args...)
}

/**
 * Close
 * Closes the connection
 * @return error
 **/
func (c *Client) Close() error {
	return c.ep.Close()
}
