package view

import (
	"context"
	"fmt"
)

// HomeName is the name of the home view
const HomeName = "home"

// Opener renders a page by name, the home view uses it so it never has to hold the registry that holds it
type Opener func(ctx context.Context, name string) (Page, error)

// Entry is one line on the home page
type Entry struct {
	// The page name the entry opens
	Name string
	// The label shown before the summary
	Label string
	// Builds a one line summary such as a count, nil for none
	Summary func(ctx context.Context) string
}

// Home is the landing page listing every app
type Home struct {
	// Returns the entries in display order, called on every render
	entries func() []Entry
	// Renders the page an entry opens
	open Opener
}

/**
 * NewHome
 * Builds the home view
 * @param open {Opener} - renders the page an entry opens
 * @param entries {func() []Entry} - returns the entries in display order, called on every render
 * @return *Home
 **/
func NewHome(open Opener, entries func() []Entry) *Home {
	return &Home{entries: entries, open: open}
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
	for _, e := range h.entries() {
		// The summary when there is one
		summary := ""
		if e.Summary != nil {
			summary = e.Summary(ctx)
		}
		// Add the line and its key
		lines = append(lines, fmt.Sprintf("  %-14s %s", e.Label, summary))
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
 * @param a {Action} - open or refresh
 * @return Response, error
 **/
func (h *Home) Act(ctx context.Context, a Action) (Response, error) {
	// Dispatch on the action
	switch a.Name {
	case "refresh":
		// Render again
		p, err := h.Render(ctx)
		if err != nil {
			return Response{}, err
		}
		return Show(p), nil
	case "open":
		// Nothing to open on a header line
		if a.Key == "" {
			return Response{Kind: KindNone}, nil
		}
		// Render the entry's page
		p, err := h.open(ctx, a.Key)
		if err != nil {
			return Fail(err.Error()), nil
		}
		return Show(p), nil
	}
	// Anything else is unknown
	return Fail("home: unknown action " + a.Name), nil
}
