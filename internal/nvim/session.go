package nvim

import (
	"context"
	"fmt"
	"os"

	client "github.com/neovim/go-client/nvim"
)

// Options is everything Start needs to spawn and wire an nvim
type Options struct {
	// The plugin directory prepended to the runtimepath, empty to skip
	PluginDir string
	// Extra arguments after --embed
	Args []string
	// Extra environment entries for nvim, NAME=value, on top of the current one
	Env []string
	// Called with a snapshot after every flush, on the redraw goroutine
	OnFlush func(Screen)
	// Called once when nvim goes away, with the serve error if any
	OnExit func(error)
	// Where log lines go, nil for none
	Logf func(string, ...interface{})
}

// Session is one embedded nvim, its grid state, and the rpc connection to it
type Session struct {
	// The rpc client
	v *client.Nvim
	// The grid state, owned by the redraw goroutine
	st *state
	// The options Start was given
	opts Options
	// Cancels the child process context
	cancel context.CancelFunc
}

/**
 * Start
 * Spawns nvim --embed, registers the redraw handler, and starts serving rpc
 * @param ctx {context.Context} - cancelling it kills nvim
 * @param opts {Options} - the session options
 * @return *Session, error
 **/
func Start(ctx context.Context, opts Options) (*Session, error) {
	// Use a silent logger when none was given
	if opts.Logf == nil {
		opts.Logf = func(string, ...interface{}) {}
	}
	// A child context so Close can kill the process
	ctx, cancel := context.WithCancel(ctx)
	// The arguments, --embed then the plugin path then anything extra
	args := []string{"--embed"}
	if opts.PluginDir != "" {
		args = append(args, "--cmd", "lua vim.opt.runtimepath:prepend([["+opts.PluginDir+"]])")
	}
	args = append(args, opts.Args...)
	// Spawn nvim without the client serving on its own, so the serve error is ours to read
	v, err := client.NewChildProcess(
		client.ChildProcessContext(ctx),
		client.ChildProcessArgs(args...),
		client.ChildProcessEnv(append(os.Environ(), opts.Env...)),
		client.ChildProcessServe(false),
		client.ChildProcessLogf(opts.Logf),
	)
	// Fail when the spawn failed
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start nvim: %w", err)
	}
	// The session
	s := &Session{v: v, st: newState(), opts: opts, cancel: cancel}
	// Register the redraw handler before any message can arrive
	if err := v.RegisterHandler("redraw", s.onRedraw); err != nil {
		cancel()
		return nil, fmt.Errorf("register redraw: %w", err)
	}
	// Serve rpc on its own goroutine and report when it ends
	go func() {
		err := v.Serve()
		if opts.OnExit != nil {
			opts.OnExit(err)
		}
	}()
	// Return the session
	return s, nil
}

/**
 * onRedraw
 * Applies every update of a redraw notification and hands out a snapshot on flush
 * @param updates {...[]interface{}} - the updates, each a name then arg tuples
 * @return void
 **/
func (s *Session) onRedraw(updates ...[]interface{}) {
	// Loop over every update
	for _, u := range updates {
		// Read the name
		name, _ := asString(argOrNil(u, 0))
		// A flush means the state is consistent, snapshot it
		if name == "flush" {
			if s.opts.OnFlush != nil {
				s.opts.OnFlush(s.st.snapshot())
			}
			continue
		}
		// Apply everything else and log a bad event rather than dropping the connection
		if err := s.st.apply(u); err != nil {
			s.opts.Logf("redraw: %v", err)
		}
	}
}

/**
 * Attach
 * Attaches as a linegrid ui at a size and makes sure the plugin is loaded
 * @param width {int} - the width in columns
 * @param height {int} - the height in rows
 * @return error
 **/
func (s *Session) Attach(width, height int) error {
	// The ui options, true color and the line grid protocol
	opts := map[string]interface{}{"rgb": true, "ext_linegrid": true}
	// Attach
	if err := s.v.AttachUI(width, height, opts); err != nil {
		return fmt.Errorf("attach ui: %w", err)
	}
	// Put the plugin back on the runtimepath, a plugin manager such as lazy.nvim resets it during startup
	if s.opts.PluginDir != "" {
		if err := s.v.ExecLua(ensurePluginLua, nil, s.opts.PluginDir); err != nil {
			return fmt.Errorf("ensure plugin: %w", err)
		}
	}
	return nil
}

// ensurePluginLua adds the plugin directory to the runtimepath when it is missing and sources its plugin file once
const ensurePluginLua = `
local dir = ...
local rtp = vim.opt.runtimepath:get()
if not vim.tbl_contains(rtp, dir) then
  vim.opt.runtimepath:prepend(dir)
end
if not vim.g.loaded_symphony then
  vim.cmd.source(dir .. "/plugin/symphony.lua")
end
`

/**
 * Resize
 * Asks nvim to resize the grid
 * @param width {int} - the width in columns
 * @param height {int} - the height in rows
 * @return error
 **/
func (s *Session) Resize(width, height int) error {
	// Forward the resize
	return s.v.TryResizeUI(width, height)
}

/**
 * Input
 * Sends keys in nvim notation
 * @param keys {string} - the keys
 * @return error
 **/
func (s *Session) Input(keys string) error {
	// Forward the keys, the count written is not needed
	_, err := s.v.Input(keys)
	return err
}

/**
 * Paste
 * Sends raw text as one paste
 * @param text {string} - the text
 * @return error
 **/
func (s *Session) Paste(text string) error {
	// Forward the text as a single phase paste
	_, err := s.v.Paste(text, true, -1)
	return err
}

/**
 * Exec
 * Runs a lua chunk with arguments and decodes its result
 * @param code {string} - the lua chunk, the arguments arrive as ...
 * @param result {interface{}} - where the return value lands, nil to drop it
 * @param args {...interface{}} - the arguments
 * @return error
 **/
func (s *Session) Exec(code string, result interface{}, args ...interface{}) error {
	// Forward to nvim_exec_lua
	return s.v.ExecLua(code, result, args...)
}

/**
 * Handle
 * Registers a handler the plugin can reach with rpcrequest or rpcnotify
 * @param method {string} - the method name
 * @param fn {interface{}} - the handler
 * @return error
 **/
func (s *Session) Handle(method string, fn interface{}) error {
	// Forward the registration
	return s.v.RegisterHandler(method, fn)
}

/**
 * Close
 * Closes the connection and kills nvim if it is still running
 * @return error
 **/
func (s *Session) Close() error {
	// Kill the process first so the wait inside Close never hangs on a stuck nvim
	s.cancel()
	// Close the pipes and reap the process
	return s.v.Close()
}
