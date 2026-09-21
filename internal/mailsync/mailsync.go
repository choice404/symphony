// Package mailsync runs the daemon's own IMAP sync, SMTP send, and trash for the accounts that ask for it
package mailsync

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/imapsync"
	"github.com/choice404/symphony/internal/mail"
	"github.com/choice404/symphony/internal/oauth"
)

// The Gmail mailbox names, used when the host is Gmail
const (
	gmailSent  = "[Gmail]/Sent Mail"
	gmailTrash = "[Gmail]/Trash"
)

// triggers carries account names whose sync loop should run now
var triggers = struct {
	sync.Mutex
	chans map[string]chan struct{}
}{chans: map[string]chan struct{}{}}

/**
 * Run
 * Syncs one account once, the inbox and the sent folder, logging in the way its config says
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return imapsync.Result, error
 **/
func Run(ctx context.Context, a config.Account, logf func(string, ...interface{})) (imapsync.Result, error) {
	// The login
	auth, err := login(ctx, a, imapPort(a))
	if err != nil {
		return imapsync.Result{}, err
	}
	// The sync
	res, err := imapsync.Sync(ctx, syncAccount(a), auth, logf)
	if err != nil {
		return res, err
	}
	logf("%s: synced, %d added, %d removed, %d flag changes, %d kept", a.Name, res.Added, res.Removed, res.Updated, res.Total)
	return res, nil
}

/**
 * syncAccount
 * Builds the sync account with its two folders
 * @param a {config.Account} - the account
 * @return imapsync.Account
 **/
func syncAccount(a config.Account) imapsync.Account {
	return imapsync.Account{
		Name: a.Name,
		Host: a.IMAPHost(),
		Keep: a.FetchCount(),
		Folders: []imapsync.Folder{
			{Mailbox: "INBOX", Dir: mail.FolderDir(a.Maildir, mail.FolderInbox)},
			{Mailbox: sentMailbox(a), Dir: mail.FolderDir(a.Maildir, mail.FolderSent)},
		},
	}
}

/**
 * sentMailbox
 * Returns the server's sent mailbox, Gmail's when the host is Gmail
 * @param a {config.Account} - the account
 * @return string
 **/
func sentMailbox(a config.Account) string {
	if strings.Contains(a.IMAPHost(), "gmail.com") {
		return gmailSent
	}
	return "Sent"
}

/**
 * trashMailbox
 * Returns the server's trash mailbox, Gmail's when the host is Gmail
 * @param a {config.Account} - the account
 * @return string
 **/
func trashMailbox(a config.Account) string {
	if strings.Contains(a.IMAPHost(), "gmail.com") {
		return gmailTrash
	}
	return "Trash"
}

/**
 * imapPort
 * Returns the IMAP port from the host setting
 * @param a {config.Account} - the account
 * @return int
 **/
func imapPort(a config.Account) int {
	if strings.HasSuffix(a.IMAPHost(), ":1143") {
		return 1143
	}
	return 993
}

/**
 * login
 * Builds the SASL client for an account from its auth setting
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @param port {int} - the port the token is scoped to for OAUTHBEARER
 * @return sasl.Client, error
 **/
func login(ctx context.Context, a config.Account, port int) (sasl.Client, error) {
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
		return imapsync.OAuthBearer(a.User, tok, host, port), nil
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
 * Send
 * Sends a message over SMTP with the account's login
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @param out {mail.Outgoing} - the message
 * @return error
 **/
func Send(ctx context.Context, a config.Account, out mail.Outgoing) error {
	// The login, scoped to the SMTP port
	host := a.SMTPHost()
	port := 465
	if i := strings.LastIndex(host, ":"); i >= 0 {
		if n, err := strconv.Atoi(host[i+1:]); err == nil {
			port = n
		}
	}
	auth, err := login(ctx, a, port)
	if err != nil {
		return err
	}
	// Connect over TLS and log in
	c, err := smtp.DialTLS(host, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", host, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp login %s: %w", a.Name, err)
	}
	// Send
	if err := c.SendMail(out.From, out.Recipients, bytes.NewReader(out.Raw)); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return c.Quit()
}

/**
 * Trash
 * Moves a message to the server's trash and removes its local copy
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @param m {mail.Message} - the message
 * @return error
 **/
func Trash(ctx context.Context, a config.Account, m mail.Message) error {
	// The login
	auth, err := login(ctx, a, imapPort(a))
	if err != nil {
		return err
	}
	// The folder the message sits in
	dir := mail.FolderDir(a.Maildir, m.Folder)
	mailbox := "INBOX"
	if m.Folder == mail.FolderSent {
		mailbox = sentMailbox(a)
	}
	rel, err := relative(dir, m.Path)
	if err != nil {
		return err
	}
	return imapsync.Trash(ctx, syncAccount(a), auth, imapsync.Folder{Mailbox: mailbox, Dir: dir}, rel, trashMailbox(a))
}

/**
 * relative
 * Returns a message path relative to its folder root
 * @param dir {string} - the folder root
 * @param path {string} - the file
 * @return string, error
 **/
func relative(dir, path string) (string, error) {
	// The file must sit under the root
	if !strings.HasPrefix(path, dir+"/") {
		return "", fmt.Errorf("%s is not under %s", path, dir)
	}
	return strings.TrimPrefix(path, dir+"/"), nil
}

/**
 * Trigger
 * Asks an account's sync loop to run now, nothing happens when no loop runs for it
 * @param name {string} - the account name
 * @return void
 **/
func Trigger(name string) {
	triggers.Lock()
	ch := triggers.chans[name]
	triggers.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
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
		ch := make(chan struct{}, 1)
		triggers.Lock()
		triggers.chans[a.Name] = ch
		triggers.Unlock()
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			tick(ctx, name, ch, load, logf)
		}(a.Name)
	}
	wg.Wait()
}

/**
 * tick
 * Syncs one account on its interval or when triggered, reading the account fresh from the config each time
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name
 * @param ch {chan struct{}} - the trigger
 * @param load {func() config.Config} - returns the current config
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return void
 **/
func tick(ctx context.Context, name string, ch chan struct{}, load func() config.Config, logf func(string, ...interface{})) {
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
		// Wait for the next tick or a trigger
		every := a.SyncEvery()
		if every == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		case <-ch:
			// A send just happened, give the server a moment to file it
			time.Sleep(3 * time.Second)
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
