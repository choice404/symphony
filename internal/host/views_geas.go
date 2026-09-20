//go:build geas

package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/geas"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/view"
)

// mailModule is the file name of the compiled mail contract
const mailModule = "libmail.geas.so"

/**
 * Views
 * Builds every view from the config, this build runs mail through its contract when the module is found and falls back to plain Go when it is not
 * @param cfg {config.Config} - the config
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return view.Registry, func(), error
 **/
func Views(cfg config.Config, logf func(string, ...interface{})) (view.Registry, func(), error) {
	// Find the module
	module, err := findModule(cfg)
	if err != nil {
		logf("geas: %v, mail runs without the contract", err)
		reg, err := assemble(mail.NewView(cfg.Mail.Maildir))
		return reg, func() {}, err
	}
	// Bring up the runtime and load it
	rt, err := geas.Init()
	if err != nil {
		return view.Registry{}, nil, err
	}
	if err := rt.Load(module); err != nil {
		rt.Shutdown()
		return view.Registry{}, nil, err
	}
	// Bind the host pledges, then the dusk body when this build carries it
	if err := mail.Bind(rt); err != nil {
		rt.Shutdown()
		return view.Registry{}, nil, err
	}
	labels := true
	if err := mail.BindDusk(rt); err != nil {
		logf("geas: %v, inbox lines carry no label", err)
		labels = false
	}
	if err := rt.Freeze(); err != nil {
		rt.Shutdown()
		return view.Registry{}, nil, err
	}
	logf("geas: mail runs through %s", module)
	// Assemble around the contract view
	reg, err := assemble(mail.NewContract(rt, cfg.Mail.Maildir, labels))
	if err != nil {
		rt.Shutdown()
		return view.Registry{}, nil, err
	}
	return reg, rt.Shutdown, nil
}

/**
 * findModule
 * Finds the compiled mail contract, the config dir first, then beside the binary, then the data dir
 * @param cfg {config.Config} - the config
 * @return string, error
 **/
func findModule(cfg config.Config) (string, error) {
	// The places to look
	dirs := make([]string, 0, 3)
	if cfg.Geas.Contracts != "" {
		dirs = append(dirs, cfg.Geas.Contracts)
	}
	if self, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(self), "contracts"))
	}
	if data, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(data, ".local", "share", "symphony", "contracts"))
	}
	// The first hit wins
	for _, d := range dirs {
		p := filepath.Join(d, mailModule)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Nothing found
	if len(dirs) == 0 {
		return "", errors.New("no contract directories to search")
	}
	return "", fmt.Errorf("%s not found in %v", mailModule, dirs)
}
