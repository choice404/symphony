package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/choice404/symphony/internal/nvim"
)

// Host is the embedded nvim as the model sees it
type Host interface {
	// Attach the ui at a size
	Attach(width, height int) error
	// Resize the ui
	Resize(width, height int) error
	// Send keys in nvim notation
	Input(keys string) error
	// Send raw pasted text
	Paste(text string) error
}

// FlushMsg carries a fresh screen snapshot from the redraw goroutine
type FlushMsg struct {
	// The snapshot to draw
	Screen nvim.Screen
}

// ExitMsg says nvim went away
type ExitMsg struct {
	// The serve error if any
	Err error
}

// errMsg carries a failed host call
type errMsg struct {
	// The error
	err error
}

// Model is the bubbletea model that draws nvim and forwards keys to it
type Model struct {
	// The embedded nvim
	host Host
	// Runs once right after the ui attaches, nil for nothing
	onAttach func() error
	// The last snapshot
	screen nvim.Screen
	// Whether the ui is attached yet
	attached bool
	// The error that ended the program if any
	Err error
}

/**
 * New
 * Builds a model around a host
 * @param host {Host} - the embedded nvim
 * @param onAttach {func() error} - runs once after the ui attaches, nil for nothing
 * @return Model
 **/
func New(host Host, onAttach func() error) Model {
	// Return the model, nothing attached yet
	return Model{host: host, onAttach: onAttach}
}

/**
 * Init
 * Nothing to start, the first window size does the attach
 * @return tea.Cmd
 **/
func (m Model) Init() tea.Cmd {
	return nil
}

/**
 * Update
 * Handles a message and returns the next model, keys go to nvim in order so they are sent here not in a Cmd
 * @param msg {tea.Msg} - the message
 * @return tea.Model, tea.Cmd
 **/
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Dispatch on the message type
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onSize(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	case FlushMsg:
		// Take the new snapshot
		next := m
		next.screen = msg.Screen
		return next, nil
	case ExitMsg:
		// Keep the error and stop
		next := m
		next.Err = msg.Err
		return next, tea.Quit
	case errMsg:
		// A failed host call ends the program with its error
		next := m
		next.Err = msg.err
		return next, tea.Quit
	}
	// Ignore everything else
	return m, nil
}

/**
 * onSize
 * Attaches on the first size and resizes after that
 * @param msg {tea.WindowSizeMsg} - the new size
 * @return tea.Model, tea.Cmd
 **/
func (m Model) onSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	// The next model
	next := m
	// The call to make
	var err error
	if !m.attached {
		err = m.host.Attach(msg.Width, msg.Height)
		next.attached = err == nil
		// Run the attach hook once the ui is up
		if err == nil && m.onAttach != nil {
			err = m.onAttach()
		}
	} else {
		err = m.host.Resize(msg.Width, msg.Height)
	}
	// Report a failure
	if err != nil {
		return next, func() tea.Msg { return errMsg{err} }
	}
	return next, nil
}

/**
 * onKey
 * Translates a key and sends it to nvim right away so order is kept
 * @param msg {tea.KeyMsg} - the key
 * @return tea.Model, tea.Cmd
 **/
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Translate the key
	in := Translate(msg)
	// The call to make
	var err error
	if in.Paste != "" {
		err = m.host.Paste(in.Paste)
	} else if in.Keys != "" {
		err = m.host.Input(in.Keys)
	}
	// Report a failure
	if err != nil {
		return m, func() tea.Msg { return errMsg{err} }
	}
	return m, nil
}

/**
 * View
 * Draws the last snapshot, or a waiting line before the first flush
 * @return string
 **/
func (m Model) View() string {
	// Nothing to draw before the first flush
	if m.screen.Height == 0 {
		return "starting nvim..."
	}
	// Draw the grid
	return nvim.Render(m.screen)
}
