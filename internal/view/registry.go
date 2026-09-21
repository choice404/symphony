package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Registry holds every view by name and is built once
type Registry struct {
	// The views keyed by name
	views map[string]View
}

/**
 * NewRegistry
 * Builds a registry from views, a repeated name is an error
 * @param views {...View} - the views
 * @return Registry, error
 **/
func NewRegistry(views ...View) (Registry, error) {
	// The map
	m := make(map[string]View, len(views))
	// Loop over every view
	for _, v := range views {
		// Refuse a repeated name
		if _, dup := m[v.Name()]; dup {
			return Registry{}, fmt.Errorf("view %q registered twice", v.Name())
		}
		// Store it
		m[v.Name()] = v
	}
	// Return the registry
	return Registry{views: m}, nil
}

/**
 * Names
 * Returns every view name sorted
 * @return []string
 **/
func (r Registry) Names() []string {
	// The names
	names := make([]string, 0, len(r.views))
	// Loop over every view
	for name := range r.views {
		names = append(names, name)
	}
	// Sort for stable completion
	sort.Strings(names)
	return names
}

/**
 * Get
 * Finds a view by name
 * @param name {string} - the view name
 * @return View, error
 **/
func (r Registry) Get(name string) (View, error) {
	// Look it up
	v, ok := r.views[name]
	// Report an unknown name
	if !ok {
		return nil, fmt.Errorf("no view named %q", name)
	}
	return v, nil
}

/**
 * Split
 * Splits a page name into its view and the path below it
 * @param name {string} - the page name such as mail/school/sent
 * @return string, string
 **/
func Split(name string) (string, string) {
	// Cut at the first slash
	if i := strings.Index(name, "/"); i >= 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

/**
 * Render
 * Renders a page by name, a path below the view goes to the view's RenderPath
 * @param ctx {context.Context} - the context
 * @param name {string} - the page name
 * @return Page, error
 **/
func (r Registry) Render(ctx context.Context, name string) (Page, error) {
	// Find the view
	base, path := Split(name)
	v, err := r.Get(base)
	if err != nil {
		return Page{}, err
	}
	// A path needs a pager
	if path != "" {
		p, ok := v.(Pager)
		if !ok {
			return Page{}, fmt.Errorf("%s has no page %q", base, path)
		}
		return p.RenderPath(ctx, path)
	}
	// Render the view itself
	return v.Render(ctx)
}

/**
 * Act
 * Runs an action on a view by name
 * @param ctx {context.Context} - the context
 * @param name {string} - the view name
 * @param a {Action} - the action
 * @return Response, error
 **/
func (r Registry) Act(ctx context.Context, name string, a Action) (Response, error) {
	// Find the view
	base, _ := Split(name)
	v, err := r.Get(base)
	if err != nil {
		return Response{}, err
	}
	// Run the action
	return v.Act(ctx, a)
}
