// Package mailsync runs the daemon's own IMAP sync for the accounts that ask for it
package mailsync

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/imapsync"
	"github.com/choice404/symphony/internal/oauth"
)

/**
 * Run
 * Syncs one account once, logging in the way its config says
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return imapsync.Result, error
 **/
func Run(ctx context.Context, a config.Account, logf func(string, ...interface{})) (imapsync.Result, error) {
	// The login
	auth, err := login(ctx, a)
	if err != nil {
		return imapsync.Result{}, err
	}
	// The sync
	acc := imapsync.Account{Name: a.Name, Dir: a.Maildir, Host: a.IMAPHost(), Keep: a.FetchCount()}
	res, err := imapsync.Sync(ctx, acc, auth, logf)
	if err != nil {
		return res, err
	}
	logf("%s: synced, %d added, %d removed, %d flag changes, %d kept", a.Name, res.Added, res.Removed, res.Updated, res.Total)
	return res, nil
}

/**
 * login
 * Builds the SASL client for an account from its auth setting
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @return sasl.Client, error
 **/
func login(ctx context.Context, a config.Account) (sasl.Client, error) {
	// Every synced account needs a user
	if a.User == "" {
		return nil, fmt.Errorf("%s: set user to the full address", a.Name)
	}
	// Dispatch on the auth kind
	switch a.Auth {
	case "oauth":
		tok, err := oauth.AccessToken(ctx, a.Name)
		if err != nil {
			return nil, err
		}
		host := a.IMAPHost()
		if i := strings.Index(host, ":"); i >= 0 {
			host = host[:i]
		}
		return imapsync.OAuthBearer(a.User, tok, host), nil
	case "password":
		if a.PassFile == "" {
			return nil, fmt.Errorf("%s: set pass_file", a.Name)
		}
		data, err := os.ReadFile(a.PassFile)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Name, err)
		}
		return imapsync.Plain(a.User, strings.TrimSpace(string(data))), nil
	}
	return nil, fmt.Errorf("%s: auth is %q, set it to oauth or password", a.Name, a.Auth)
}

/**
 * Loop
 * Syncs every account with an interval on its own clock until the context ends, the first sync right away
 * @param ctx {context.Context} - the context
 * @param load {func() config.Config} - returns the current config, read at every tick so an edit lands
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return void
 **/
func Loop(ctx context.Context, load func() config.Config, logf func(string, ...interface{})) {
	// One goroutine per account named at start, a new account in the config takes a restart
	var wg sync.WaitGroup
	for _, a := range load().Mail.All() {
		if !a.Synced() || a.SyncEvery() == 0 {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			tick(ctx, name, load, logf)
		}(a.Name)
	}
	wg.Wait()
}

/**
 * tick
 * Syncs one account on its interval, reading the account fresh from the config each time
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name
 * @param load {func() config.Config} - returns the current config
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return void
 **/
func tick(ctx context.Context, name string, load func() config.Config, logf func(string, ...interface{})) {
	// Loop until the context ends
	for {
		// The account as configured right now
		a, ok := find(load(), name)
		if !ok || !a.Synced() {
			return
		}
		if _, err := Run(ctx, a, logf); err != nil {
			logf("%s: sync failed: %v", name, err)
		}
		// Wait for the next tick
		every := a.SyncEvery()
		if every == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

/**
 * find
 * Finds an account by name in a config
 * @param cfg {config.Config} - the config
 * @param name {string} - the name
 * @return config.Account, bool
 **/
func find(cfg config.Config, name string) (config.Account, bool) {
	for _, a := range cfg.Mail.All() {
		if a.Name == name {
			return a, true
		}
	}
	return config.Account{}, false
}
