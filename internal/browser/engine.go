// Package browser drives a headless Chromium with a profile that keeps logins, and renders its pages as text
package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// navTimeout is how long a navigation may take
const navTimeout = 30 * time.Second

// actTimeout is how long a click, a keystroke, or an extraction may take
const actTimeout = 15 * time.Second

// candidates are the browsers looked for on the path, in order
var candidates = []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser", "brave", "chrome"}

/**
 * FindChrome
 * Returns the first Chromium on the path, empty when there is none
 * @return string
 **/
func FindChrome() string {
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// Tab is one open page
type Tab struct {
	// The id
	ID int
	// The name a caller reserved it under, empty for a plain tab
	Name string
	// The chromedp context of the tab
	ctx context.Context
	// Cancels the tab
	cancel context.CancelFunc
}

// Engine is one browser process and its tabs, started on first use and stopped on demand so a headed login can borrow the profile
type Engine struct {
	// The browser binary
	exec string
	// The user data directory that keeps logins
	profile string
	// Guards everything below
	mu sync.Mutex
	// The allocator, nil until started
	alloc context.Context
	// Cancels the allocator
	allocCancel context.CancelFunc
	// The first context, the browser itself, every tab derives from it
	root context.Context
	// Cancels the root
	rootCancel context.CancelFunc
	// The tabs by id
	tabs map[int]*Tab
	// The next tab id
	next int
}

/**
 * New
 * Builds an engine, nothing runs until the first tab opens
 * @param exec {string} - the browser binary
 * @param profile {string} - the user data directory
 * @return *Engine
 **/
func New(exec, profile string) *Engine {
	return &Engine{exec: exec, profile: profile, tabs: map[int]*Tab{}, next: 1}
}

/**
 * Profile
 * Returns the user data directory
 * @return string
 **/
func (e *Engine) Profile() string {
	return e.profile
}

/**
 * Exec
 * Returns the browser binary
 * @return string
 **/
func (e *Engine) Exec() string {
	return e.exec
}

/**
 * start
 * Launches the browser when it is not running, with the lock held
 * @return error
 **/
func (e *Engine) start() error {
	if e.root != nil {
		return nil
	}
	if e.exec == "" {
		return errors.New("browser: no chromium found, install google-chrome, chromium, or brave, or set chrome under [browser]")
	}
	if err := os.MkdirAll(e.profile, 0o700); err != nil {
		return fmt.Errorf("browser: profile: %w", err)
	}
	// The options, headless with the profile and a desktop sized window so sites lay out fully
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.ExecPath(e.exec),
		chromedp.UserDataDir(e.profile),
		chromedp.WindowSize(1280, 1024),
		chromedp.Flag("headless", "new"),
		// The back forward cache restores a page without a load event, which leaves a back navigation waiting forever
		chromedp.Flag("disable-features", "site-per-process,Translate,BlinkGenPropertyTrees,BackForwardCache"),
	)
	e.alloc, e.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	e.root, e.rootCancel = chromedp.NewContext(e.alloc)
	// Bring the browser up on the root itself, the browser lives as long as the context of its first run
	if err := chromedp.Run(e.root); err != nil {
		e.stopLocked()
		return fmt.Errorf("browser: start: %w", err)
	}
	return nil
}

/**
 * stopLocked
 * Closes every tab and the browser, with the lock held
 * @return void
 **/
func (e *Engine) stopLocked() {
	for id, t := range e.tabs {
		t.cancel()
		delete(e.tabs, id)
	}
	// Cancel waits for the browser to exit so the profile is free for a headed window
	if e.root != nil {
		_ = chromedp.Cancel(e.root)
	}
	if e.rootCancel != nil {
		e.rootCancel()
	}
	if e.allocCancel != nil {
		e.allocCancel()
	}
	e.root, e.rootCancel, e.alloc, e.allocCancel = nil, nil, nil, nil
}

/**
 * Stop
 * Closes every tab and the browser, so a headed window can use the profile, the next open starts it again
 * @return void
 **/
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
}

/**
 * Running
 * Reports whether the browser is up
 * @return bool
 **/
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.root != nil
}

/**
 * Open
 * Opens a new tab at a url, starting the browser when needed
 * @param url {string} - the url
 * @return *Tab, error
 **/
func (e *Engine) Open(url string) (*Tab, error) {
	return e.open("", url)
}

/**
 * Named
 * Returns the tab reserved under a name, opening it at the url when there is none yet
 * @param name {string} - the name
 * @param url {string} - the url for a new tab
 * @return *Tab, error
 **/
func (e *Engine) Named(name, url string) (*Tab, error) {
	e.mu.Lock()
	for _, t := range e.tabs {
		if t.Name == name {
			e.mu.Unlock()
			return t, nil
		}
	}
	e.mu.Unlock()
	return e.open(name, url)
}

/**
 * open
 * Opens a tab, named or not
 * @param name {string} - the name, empty for none
 * @param url {string} - the url
 * @return *Tab, error
 **/
func (e *Engine) open(name, url string) (*Tab, error) {
	e.mu.Lock()
	if err := e.start(); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	ctx, cancel := chromedp.NewContext(e.root)
	t := &Tab{ID: e.next, Name: name, ctx: ctx, cancel: cancel}
	e.next++
	e.tabs[t.ID] = t
	e.mu.Unlock()
	// The tab lives as long as the context of its first run, so that run is the plain one
	if err := chromedp.Run(ctx); err != nil {
		e.Close(t.ID)
		return nil, fmt.Errorf("browser: new tab: %w", err)
	}
	// Navigate, a failure closes the tab again
	if err := t.Navigate(url); err != nil {
		e.Close(t.ID)
		return nil, err
	}
	return t, nil
}

/**
 * Tab
 * Finds a tab by id
 * @param id {int} - the id
 * @return *Tab, bool
 **/
func (e *Engine) Tab(id int) (*Tab, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t, ok := e.tabs[id]
	return t, ok
}

/**
 * Tabs
 * Returns every tab by id
 * @return []*Tab
 **/
func (e *Engine) Tabs() []*Tab {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*Tab, 0, len(e.tabs))
	for _, t := range e.tabs {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

/**
 * Close
 * Closes a tab
 * @param id {int} - the id
 * @return void
 **/
func (e *Engine) Close(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t, ok := e.tabs[id]; ok {
		t.cancel()
		delete(e.tabs, id)
	}
}

/**
 * run
 * Runs actions on the tab under a timeout
 * @param limit {time.Duration} - the timeout
 * @param actions {...chromedp.Action} - the actions
 * @return error
 **/
func (t *Tab) run(limit time.Duration, actions ...chromedp.Action) error {
	ctx, cancel := context.WithTimeout(t.ctx, limit)
	defer cancel()
	return chromedp.Run(ctx, actions...)
}

/**
 * Navigate
 * Loads a url and waits for the document
 * @param url {string} - the url
 * @return error
 **/
func (t *Tab) Navigate(url string) error {
	if err := t.run(navTimeout, chromedp.Navigate(url), chromedp.WaitReady("body")); err != nil {
		return fmt.Errorf("browser: %s: %w", url, err)
	}
	return nil
}

/**
 * Back
 * Goes back in the tab's history
 * @return error
 **/
func (t *Tab) Back() error {
	return t.run(navTimeout, chromedp.NavigateBack(), chromedp.WaitReady("body"))
}

/**
 * Forward
 * Goes forward in the tab's history
 * @return error
 **/
func (t *Tab) Forward() error {
	return t.run(navTimeout, chromedp.NavigateForward(), chromedp.WaitReady("body"))
}

/**
 * Reload
 * Loads the page again
 * @return error
 **/
func (t *Tab) Reload() error {
	return t.run(navTimeout, chromedp.Reload(), chromedp.WaitReady("body"))
}

/**
 * Location
 * Returns the tab's url and title
 * @return string, string
 **/
func (t *Tab) Location() (string, string) {
	var url, title string
	_ = t.run(actTimeout, chromedp.Location(&url), chromedp.Title(&title))
	return url, title
}

/**
 * Eval
 * Runs a script in the tab and decodes its result
 * @param script {string} - the script, an expression
 * @param out {interface{}} - where the result lands, nil to drop it
 * @return error
 **/
func (t *Tab) Eval(script string, out interface{}) error {
	if out == nil {
		var sink interface{}
		out = &sink
	}
	return t.run(actTimeout, chromedp.Evaluate(script, out))
}

/**
 * Wait
 * Waits for a selector to be visible
 * @param selector {string} - the selector
 * @param limit {time.Duration} - how long
 * @return error
 **/
func (t *Tab) Wait(selector string, limit time.Duration) error {
	return t.run(limit, chromedp.WaitVisible(selector, chromedp.ByQuery))
}

/**
 * Type
 * Focuses an element, clears it when it is a field, and types text into it
 * @param selector {string} - the selector
 * @param text {string} - the text
 * @return error
 **/
func (t *Tab) Type(selector, text string) error {
	clear := fmt.Sprintf(`(() => { const el = document.querySelector(%q); if (el && 'value' in el) { el.value = ''; el.dispatchEvent(new Event('input', {bubbles: true})); } return true; })()`, selector)
	return t.run(actTimeout, chromedp.Focus(selector, chromedp.ByQuery), chromedp.Evaluate(clear, nil), chromedp.SendKeys(selector, text, chromedp.ByQuery))
}

/**
 * Insert
 * Inserts text into whatever has focus as one edit, the way a paste lands, which rich editors take better than key by key typing
 * @param text {string} - the text
 * @return error
 **/
func (t *Tab) Insert(text string) error {
	return t.run(actTimeout, input.InsertText(text))
}

/**
 * Key
 * Sends a key to whatever has focus, Enter or Escape by name, anything else as typed
 * @param key {string} - the key name
 * @return error
 **/
func (t *Tab) Key(key string) error {
	switch key {
	case "Enter":
		key = kb.Enter
	case "Escape":
		key = kb.Escape
	}
	return t.run(actTimeout, chromedp.KeyEvent(key))
}

/**
 * Press
 * Sends a key such as Enter to an element
 * @param selector {string} - the selector
 * @param key {string} - the key, a chromedp key string
 * @return error
 **/
func (t *Tab) Press(selector, key string) error {
	return t.run(actTimeout, chromedp.Focus(selector, chromedp.ByQuery), chromedp.KeyEvent(key))
}

/**
 * Click
 * Clicks an element
 * @param selector {string} - the selector
 * @return error
 **/
func (t *Tab) Click(selector string) error {
	return t.run(actTimeout, chromedp.Click(selector, chromedp.ByQuery))
}

/**
 * Settle
 * Gives a page a moment after an action before it is read again
 * @param d {time.Duration} - how long
 * @return void
 **/
func (t *Tab) Settle(d time.Duration) {
	_ = t.run(d+time.Second, chromedp.Sleep(d))
}
