//go:build !geas

package host

import (
	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/view"
)

/**
 * Views
 * Builds every view from the config, this build reads the Maildir straight from Go
 * @param cfg {config.Config} - the config
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return view.Registry, func(), error
 **/
func Views(cfg config.Config, logf func(string, ...interface{})) (view.Registry, func(), error) {
	// Assemble around the plain mail view
	reg, err := assemble(mail.NewView(cfg.Mail.Maildir))
	if err != nil {
		return view.Registry{}, nil, err
	}
	// Nothing to close
	return reg, func() {}, nil
}
