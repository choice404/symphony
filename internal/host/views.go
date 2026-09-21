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
 * @return view.Registry, func(), error
 **/
func Views(load func() config.Config, logf func(string, ...interface{})) (view.Registry, func(), error) {
	// Assemble around the plain backend
	reg, err := assemble(mail.New(accounts(load), mail.Plain{}, services(load, logf)))
	if err != nil {
		return view.Registry{}, nil, err
	}
	// Nothing to close
	return reg, func() {}, nil
}
