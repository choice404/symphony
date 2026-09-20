package view

import (
	"context"
	"fmt"
)

// HomeName is the name of the home view
const HomeName = "home"

// Opener renders a view by name, the home view uses it so it never has to hold the registry that holds it
type Opener func(ctx context.Context, name string) (Page, error)

// Entry is one app listed on the home page
type Entry struct {
	// The view name the entry opens
	Name string
	// The label shown before the summary
	Label string
	// Builds a one line summary such as a count, nil for none
	Summary func(ctx context.Context) string
}

// Home is the landing page listing every app
type Home struct {
	// The entries in display order
	entries []Entry
	// Renders the view an entry opens
	open Opener
}

/**
 * NewHome
 * Builds the home view
 * @param open {Opener} - renders the view an entry opens
 * @param entries {...Entry} - the entries in display order
 * @return *Home
 **/
func NewHome(open Opener, entries ...Entry) *Home {
	// Copy the entries so the caller's slice is not shared
	return &Home{entries: append([]Entry(nil), entries...), open: open}
}

/**
 * Name
 * Returns home
 * @return string
 **/
func (h *Home) Name() string {
	return HomeName
}

/**
 * Render
 * Lists every entry with its summary
 * @param ctx {context.Context} - the context
 * @return Page, error
 **/
func (h *Home) Render(ctx context.Context) (Page, error) {
	// The header lines with no key
	lines := []string{"symphony", ""}
	keys := []string{"", ""}
	// Loop over every entry
	for _, e := range h.entries {
		// The summary when there is one
		summary := ""
		if e.Summary != nil {
			summary = e.Summary(ctx)
		}
		// Add the line and its key
		lines = append(lines, fmt.Sprintf("  %-12s %s", e.Label, summary))
		keys = append(keys, e.Name)
	}
	// The footer
	lines = append(lines, "", "  <CR> open   r refresh   q back   gh home")
	keys = append(keys, "", "")
	// Return the page with the cursor on the first entry
	return Page{Name: HomeName, Title: "symphony", Lines: lines, Keys: keys, Cursor: 2, Filetype: "home"}, nil
}

/**
 * Act
 * Opens the entry under the cursor
 * @param ctx {context.Context} - the context
 * @param action {string} - open or refresh
 * @param key {string} - the entry's view name
 * @return Response, error
 **/
func (h *Home) Act(ctx context.Context, action, key string) (Response, error) {
	// Dispatch on the action
	switch action {
	case "refresh":
		// Render again
		p, err := h.Render(ctx)
		if err != nil {
			return Response{}, err
		}
		return Show(p), nil
	case "open":
		// Nothing to open on a header line
		if key == "" {
			return Response{Kind: KindNone}, nil
		}
		// Render the entry's view
		p, err := h.open(ctx, key)
		if err != nil {
			return Fail(err.Error()), nil
		}
		return Show(p), nil
	}
	// Anything else is unknown
	return Fail("home: unknown action " + action), nil
}
