//go:build !geas

package host

import (
	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/view"
)

/**
 * Views
 * Builds every view over a config loader, this build reads the Maildirs straight from Go
 * @param load {func() config.Config} - returns the current config, called on every render so an edit lands without a restart
 * @param logf {func(string, ...interface{})} - where log lines go
 * @param reload {func() error} - reads the config again and rebuilds the views, nil when not wired
 * @return view.Registry, func(), error
 **/
func Views(load func() config.Config, logf func(string, ...interface{}), reload func() error) (view.Registry, func(), error) {
	// Assemble around the plain backend, discord's gateway and the browser are what closes
	dv, closeDiscord := newDiscord(load, logf)
	eng := newBrowser(load, logf)
	reg, err := assemble(mail.New(accounts(load), mail.Plain{}, services(load, logf)), newCalendar(load), newProjects(load), dv, eng, load().Browser.Search, reload)
	if err != nil {
		closeDiscord()
		return view.Registry{}, nil, err
	}
	return reg, func() { closeDiscord(); eng.Stop() }, nil
}
