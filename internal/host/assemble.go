package host

import (
	"context"
	"os"
	"path/filepath"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/jev"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/mailsync"
	"github.com/choice404/symphony/internal/spam"
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
			out = append(out, mail.Account{Name: a.Name, Dir: a.Maildir, User: a.User})
		}
		return mail.Settings{Accounts: out, Limit: m.ShowLimit()}
	}
}

/**
 * services
 * Wires the daemon's network work into the mail view, send and trash for accounts the daemon logs into, and the spam judge when a key is set
 * @param load {func() config.Config} - returns the current config
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return mail.Services
 **/
func services(load func() config.Config, logf func(string, ...interface{})) mail.Services {
	// The config account behind a mail account, only when the daemon syncs it
	synced := func(a mail.Account) (config.Account, bool) {
		for _, c := range load().Mail.All() {
			if c.Name == a.Name && c.Synced() {
				return c, true
			}
		}
		return config.Account{}, false
	}
	// The judge, nil without a key
	cacheDir, _ := os.UserCacheDir()
	judge := spam.New(jev.New(jev.LoadKey()), filepath.Join(cacheDir, "symphony"))
	if judge == nil {
		logf("spam: no TypeSafe key, the spam page is off")
	}
	svc := mail.Services{
		Send: func(ctx context.Context, a mail.Account, out mail.Outgoing) error {
			c, ok := synced(a)
			if !ok {
				return errNotSynced(a)
			}
			return mailsync.Send(ctx, c, out)
		},
		Trash: func(ctx context.Context, a mail.Account, m mail.Message) error {
			c, ok := synced(a)
			if !ok {
				return errNotSynced(a)
			}
			return mailsync.Trash(ctx, c, m)
		},
		SyncNow: mailsync.Trigger,
	}
	if judge != nil {
		svc.Spam = judge.Score
	}
	return svc
}

// notSynced says an account is filled by something other than the daemon
type notSynced struct{ name string }

func (e notSynced) Error() string {
	return e.name + " is not synced by the daemon, set auth on it in the config"
}

func errNotSynced(a mail.Account) error { return notSynced{name: a.Name} }

/**
 * assemble
 * Builds the registry from the mail view and the home page that lists its entries
 * @param mv {*mail.Mail} - the mail view
 * @return view.Registry, error
 **/
func assemble(mv *mail.Mail) (view.Registry, error) {
	// The registry, assigned after home so the opener closes over it
	var reg view.Registry
	// The home view opens entries through the registry and lists mail's entries
	open := func(ctx context.Context, name string) (view.Page, error) { return reg.Render(ctx, name) }
	home := view.NewHome(open, mv.Entries)
	// Build the registry
	var err error
	reg, err = view.NewRegistry(home, mv)
	if err != nil {
		return view.Registry{}, err
	}
	return reg, nil
}
