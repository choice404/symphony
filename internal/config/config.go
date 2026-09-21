// Package config reads ~/.config/symphony/config.toml
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// fileRel is the config path under the user config directory
const fileRel = "symphony/config.toml"

// Config is everything the app reads from the file
type Config struct {
	// The mail section
	Mail Mail `toml:"mail"`
	// The geas section
	Geas Geas `toml:"geas"`
}

// Geas is the [geas] section
type Geas struct {
	// The directory holding compiled contracts, ~ is expanded
	Contracts string `toml:"contracts"`
}

// Mail is the [mail] section, one maildir for a single account or a list of accounts
type Mail struct {
	// The Maildir root of a single account, ~ is expanded, ignored when accounts are listed
	Maildir string `toml:"maildir"`
	// The accounts, each a [[mail.accounts]] table
	Accounts []Account `toml:"accounts"`
	// How many of the newest messages the inbox shows, 500 when unset
	Limit int `toml:"limit"`
}

// DefaultLimit is how many messages the inbox shows when the config says nothing
const DefaultLimit = 500

/**
 * ShowLimit
 * Returns the inbox limit, the default when unset or negative
 * @return int
 **/
func (m Mail) ShowLimit() int {
	if m.Limit <= 0 {
		return DefaultLimit
	}
	return m.Limit
}

// Account is one [[mail.accounts]] table
type Account struct {
	// The name shown in the inbox and on home, such as personal or school
	Name string `toml:"name"`
	// The Maildir root the account syncs into, ~ is expanded
	Maildir string `toml:"maildir"`
	// The login, the full address, needed when the daemon syncs the account itself
	User string `toml:"user"`
	// The IMAP host, imap.gmail.com when unset
	Host string `toml:"host"`
	// How the daemon logs in, oauth for a Google OAuth2 login, password for a password file, empty when something else such as mbsync fills the maildir
	Auth string `toml:"auth"`
	// The file holding the password for auth = password, ~ is expanded
	PassFile string `toml:"pass_file"`
	// How many of the newest messages the daemon keeps locally, 2000 when unset
	Fetch int `toml:"fetch"`
	// How often the daemon syncs, such as 15m, 0 or unset for only on demand
	Sync string `toml:"sync"`
	// The SMTP host, smtp.gmail.com:465 when unset
	SMTP string `toml:"smtp"`
}

// DefaultSMTP is the SMTP host when the account says nothing
const DefaultSMTP = "smtp.gmail.com"

/**
 * SMTPHost
 * Returns the SMTP host with its port, the default when unset
 * @return string
 **/
func (a Account) SMTPHost() string {
	host := a.SMTP
	if host == "" {
		host = DefaultSMTP
	}
	if !strings.Contains(host, ":") {
		host += ":465"
	}
	return host
}

// DefaultFetch is how many messages the daemon keeps when the account says nothing
const DefaultFetch = 2000

// DefaultHost is the IMAP host when the account says nothing
const DefaultHost = "imap.gmail.com"

/**
 * Synced
 * Reports whether the daemon syncs this account itself
 * @return bool
 **/
func (a Account) Synced() bool {
	return a.Auth == "oauth" || a.Auth == "password"
}

/**
 * IMAPHost
 * Returns the IMAP host with its port, the default when unset
 * @return string
 **/
func (a Account) IMAPHost() string {
	host := a.Host
	if host == "" {
		host = DefaultHost
	}
	if !strings.Contains(host, ":") {
		host += ":993"
	}
	return host
}

/**
 * FetchCount
 * Returns how many messages to keep, the default when unset
 * @return int
 **/
func (a Account) FetchCount() int {
	if a.Fetch <= 0 {
		return DefaultFetch
	}
	return a.Fetch
}

/**
 * SyncEvery
 * Parses the sync interval, zero when unset or unreadable
 * @return time.Duration
 **/
func (a Account) SyncEvery() time.Duration {
	d, err := time.ParseDuration(a.Sync)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

/**
 * All
 * Returns every account, the single maildir counts as one unnamed account when no list is given
 * @return []Account
 **/
func (m Mail) All() []Account {
	// The list wins
	if len(m.Accounts) > 0 {
		return append([]Account(nil), m.Accounts...)
	}
	// The single maildir is one account with no name
	if m.Maildir != "" {
		return []Account{{Maildir: m.Maildir}}
	}
	return nil
}

/**
 * Dir
 * Returns the config directory, where tokens and client files live beside the config
 * @return string, error
 **/
func Dir() (string, error) {
	// The config file's directory
	p, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

/**
 * Path
 * Returns the config file path
 * @return string, error
 **/
func Path() (string, error) {
	// The user config directory
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(dir, fileRel), nil
}

/**
 * Load
 * Reads the config file, a missing file is the empty config and a bad one is an error
 * @return Config, error
 **/
func Load() (Config, error) {
	// Find the file
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return LoadFile(path)
}

/**
 * LoadFile
 * Reads one config file by path
 * @param path {string} - the file
 * @return Config, error
 **/
func LoadFile(path string) (Config, error) {
	// The decoded config
	var c Config
	// Decode, a missing file is fine
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	// Expand the home directory in every path
	c.Mail.Maildir = expand(c.Mail.Maildir)
	for i := range c.Mail.Accounts {
		c.Mail.Accounts[i].Maildir = expand(c.Mail.Accounts[i].Maildir)
		c.Mail.Accounts[i].PassFile = expand(c.Mail.Accounts[i].PassFile)
	}
	c.Geas.Contracts = expand(c.Geas.Contracts)
	return c, nil
}

/**
 * expand
 * Replaces a leading ~ with the home directory
 * @param p {string} - the path
 * @return string
 **/
func expand(p string) string {
	// Leave anything that does not start with ~
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	// Find home, leave the path alone when it is unknown
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	// Join the rest onto home
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
