// Package host answers the plugin's requests for pages and actions
package host

import (
	"context"
	"fmt"

	"github.com/choice404/symphony/internal/nvim"
	"github.com/choice404/symphony/internal/view"
)

// The request names the plugin uses
const (
	// renderMethod renders a view by name
	renderMethod = "symphony.render"
	// actionMethod runs an action on a view
	actionMethod = "symphony.action"
	// viewsMethod lists the view names
	viewsMethod = "symphony.views"
)

/**
 * Register
 * Wires the registry to the session so the plugin can render views and run actions
 * @param ctx {context.Context} - the context every request runs under
 * @param sess {*nvim.Session} - the embedded nvim
 * @param reg {view.Registry} - the views
 * @return error
 **/
func Register(ctx context.Context, sess *nvim.Session, reg view.Registry) error {
	// Render a view by name
	err := sess.Handle(renderMethod, func(name string) (map[string]interface{}, error) {
		p, err := reg.Render(ctx, name)
		if err != nil {
			return nil, err
		}
		return p.ToMap(), nil
	})
	if err != nil {
		return fmt.Errorf("register %s: %w", renderMethod, err)
	}
	// Run an action on a view
	err = sess.Handle(actionMethod, func(name, action, key string) (map[string]interface{}, error) {
		r, err := reg.Act(ctx, name, action, key)
		if err != nil {
			return nil, err
		}
		return r.ToMap(), nil
	})
	if err != nil {
		return fmt.Errorf("register %s: %w", actionMethod, err)
	}
	// List the views
	err = sess.Handle(viewsMethod, func() ([]string, error) {
		return reg.Names(), nil
	})
	if err != nil {
		return fmt.Errorf("register %s: %w", viewsMethod, err)
	}
	return nil
}

/**
 * OpenHome
 * Asks the plugin to show the home view
 * @param sess {*nvim.Session} - the embedded nvim
 * @return error
 **/
func OpenHome(sess *nvim.Session) error {
	// Run the plugin's open through lua
	var buf int
	return sess.Exec(`return require("symphony.view").open("home")`, &buf)
}
