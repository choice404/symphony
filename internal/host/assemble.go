package host

import (
	"context"
	"os"
	"path/filepath"

	"github.com/choice404/symphony/internal/calendar"
	"github.com/choice404/symphony/internal/calsync"
	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/jev"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/mailsync"
	"github.com/choice404/symphony/internal/projects"
	"github.com/choice404/symphony/internal/spam"
	"github.com/choice404/symphony/internal/view"
)

/**
 * newProjects
 * Builds the projects view over the configured roots with recents under the cache directory
 * @param load {func() config.Config} - returns the current config
 * @return *projects.Projects
 **/
func newProjects(load func() config.Config) *projects.Projects {
	cacheDir, _ := os.UserCacheDir()
	src := func() projects.Settings { return projects.Settings{Roots: load().Projects.RootDirs()} }
	return projects.New(src, projects.NewScanner(), projects.NewRecents(filepath.Join(cacheDir, "symphony")))
}

/**
 * calendarSource
 * Turns the config loader into a calendar source, one account per oauth account
 * @param load {func() config.Config} - returns the current config
 * @return calendar.Source
 **/
func calendarSource(load func() config.Config) calendar.Source {
	return func() calendar.Settings {
		cfg := load()
		out := make([]calendar.Account, 0, 4)
		for _, a := range cfg.Mail.All() {
			if a.Auth == "oauth" {
				out = append(out, calendar.Account{Name: a.Name, User: a.User, Online: true})
			}
		}
		return calendar.Settings{Accounts: out, Days: cfg.Calendar.Ahead()}
	}
}

/**
 * CalendarStore
 * Returns the calendar cache under the cache directory
 * @return *calendar.Store
 **/
func CalendarStore() *calendar.Store {
	cacheDir, _ := os.UserCacheDir()
	return calendar.NewStore(filepath.Join(cacheDir, "symphony", "calendar"))
}

/**
 * newCalendar
 * Builds the calendar view over the daemon's network services
 * @param load {func() config.Config} - returns the current config
 * @return *calendar.Calendar
 **/
func newCalendar(load func() config.Config) *calendar.Calendar {
	svc := calendar.Services{Fetch: calsync.Fetch, Insert: calsync.Insert, Delete: calsync.Delete}
	return calendar.New(calendarSource(load), CalendarStore(), svc)
}

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
 * Builds the registry from the mail, calendar, and projects views and the home page that lists their entries
 * @param mv {*mail.Mail} - the mail view
 * @param cv {*calendar.Calendar} - the calendar view
 * @param pv {*projects.Projects} - the projects view
 * @return view.Registry, error
 **/
func assemble(mv *mail.Mail, cv *calendar.Calendar, pv *projects.Projects) (view.Registry, error) {
	// The registry, assigned after home so the opener closes over it
	var reg view.Registry
	// The home view opens entries through the registry and lists every app's entries, projects first since it is the door to work
	open := func(ctx context.Context, name string) (view.Page, error) { return reg.Render(ctx, name) }
	entries := func() []view.Entry {
		out := append([]view.Entry{}, pv.Entries()...)
		out = append(out, mv.Entries()...)
		return append(out, cv.Entries()...)
	}
	home := view.NewHome(open, entries)
	// Build the registry
	var err error
	reg, err = view.NewRegistry(home, mv, cv, pv)
	if err != nil {
		return view.Registry{}, err
	}
	return reg, nil
}
