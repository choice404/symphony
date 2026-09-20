package host

import (
	"context"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/view"
)

/**
 * Views
 * Builds every view from the config and the home page that lists them
 * @param cfg {config.Config} - the config
 * @return view.Registry, error
 **/
func Views(cfg config.Config) (view.Registry, error) {
	// The registry, assigned after home so the opener closes over it
	var reg view.Registry
	// The mail view
	mailView := mail.NewView(cfg.Mail.Maildir)
	// The home view opens entries through the registry
	open := func(ctx context.Context, name string) (view.Page, error) { return reg.Render(ctx, name) }
	home := view.NewHome(open, view.Entry{Name: mail.ViewName, Label: "mail", Summary: mailView.Summary})
	// Build the registry
	var err error
	reg, err = view.NewRegistry(home, mailView)
	if err != nil {
		return view.Registry{}, err
	}
	return reg, nil
}
