package host

import (
	"context"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/view"
)

/**
 * accounts
 * Turns the config loader into a mail source, one mail account per configured account plus the inbox limit
 * @param load {func() config.Config} - returns the current config
 * @return mail.Source
 **/
func accounts(load func() config.Config) mail.Source {
	return func() mail.Settings {
		// The accounts and the limit as configured right now
		m := load().Mail
		cfg := m.All()
		out := make([]mail.Account, 0, len(cfg))
		for _, a := range cfg {
			out = append(out, mail.Account{Name: a.Name, Dir: a.Maildir})
		}
		return mail.Settings{Accounts: out, Limit: m.ShowLimit()}
	}
}

// mailSource is a mail view that can also summarize itself for the home page
type mailSource interface {
	view.View
	Summary(ctx context.Context) string
}

/**
 * assemble
 * Builds the registry from a mail source and the home page that lists it
 * @param mailView {mailSource} - the mail view
 * @return view.Registry, error
 **/
func assemble(mailView mailSource) (view.Registry, error) {
	// The registry, assigned after home so the opener closes over it
	var reg view.Registry
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
