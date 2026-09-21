package mail

import (
	"path/filepath"
	"sort"
	"strings"
)

// The folders a page can show
const (
	// FolderInbox is the account's Maildir root
	FolderInbox = "inbox"
	// FolderSent is the Maildir++ subfolder the sent sync fills
	FolderSent = "sent"
	// FolderSpam is the review page of likely spam, derived from the inbox
	FolderSpam = "spam"
)

// AllAccounts is the pseudo account that merges every account
const AllAccounts = "all"

// Account is one mailbox, a name for the column, a Maildir it syncs into, and the address mail is sent from
type Account struct {
	// The name shown beside a message and in page names
	Name string
	// The Maildir root, empty when not configured
	Dir string
	// The address, used as From when composing
	User string
}

// Settings is what the mail view reads from the config on every render
type Settings struct {
	// The accounts
	Accounts []Account
	// How many of the newest messages a list shows, 0 for all
	Limit int
}

// Source returns the settings as configured right now, called on every render so a config edit lands without a restart
type Source func() Settings

/**
 * Fixed
 * Wraps one Maildir as a source with a single account named mail and no limit, an empty dir is still one account so a contract can report it
 * @param dir {string} - the Maildir root, empty when not configured
 * @return Source
 **/
func Fixed(dir string) Source {
	return func() Settings {
		return Settings{Accounts: []Account{{Name: "mail", Dir: dir, User: "me@example.com"}}}
	}
}

/**
 * Configured
 * Drops the accounts with no Maildir
 * @param accs {[]Account} - the accounts
 * @return []Account
 **/
func Configured(accs []Account) []Account {
	// The ones with a directory
	out := make([]Account, 0, len(accs))
	for _, a := range accs {
		if a.Dir != "" {
			out = append(out, a)
		}
	}
	return out
}

/**
 * FolderDir
 * Returns the directory a folder lives in under a Maildir root, the root for the inbox and a Maildir++ subfolder for the rest
 * @param dir {string} - the Maildir root
 * @param folder {string} - inbox or sent
 * @return string
 **/
func FolderDir(dir, folder string) string {
	// The inbox is the root itself
	if folder == FolderInbox || folder == "" {
		return dir
	}
	// Everything else is a dot folder with a capital name
	return filepath.Join(dir, "."+strings.ToUpper(folder[:1])+folder[1:])
}

/**
 * Key
 * The key of a message in a page, account, folder, and id, so nothing collides
 * @return string
 **/
func (m Message) Key() string {
	return m.Account + "/" + m.Folder + "/" + m.ID
}

/**
 * newest
 * Keeps the first n messages of a list already sorted newest first, all of them when n is 0
 * @param msgs {[]Message} - the sorted list
 * @param n {int} - the limit
 * @return []Message
 **/
func newest(msgs []Message, n int) []Message {
	if n <= 0 || len(msgs) <= n {
		return msgs
	}
	return msgs[:n]
}

/**
 * sortNewest
 * Sorts messages newest first with the path as the tie break
 * @param msgs {[]Message} - the messages, sorted in place
 * @return void
 **/
func sortNewest(msgs []Message) {
	sort.SliceStable(msgs, func(i, j int) bool {
		if !msgs[i].Date.Equal(msgs[j].Date) {
			return msgs[i].Date.After(msgs[j].Date)
		}
		return msgs[i].Path < msgs[j].Path
	})
}

/**
 * findKey
 * Finds a message by page key in a list
 * @param msgs {[]Message} - the list
 * @param key {string} - the key
 * @return Message, bool
 **/
func findKey(msgs []Message, key string) (Message, bool) {
	// Loop over every message
	for _, m := range msgs {
		if m.Key() == key {
			return m, true
		}
	}
	return Message{}, false
}

/**
 * findAccount
 * Finds an account by name
 * @param accs {[]Account} - the accounts
 * @param name {string} - the name
 * @return Account, bool
 **/
func findAccount(accs []Account, name string) (Account, bool) {
	for _, a := range accs {
		if a.Name == name {
			return a, true
		}
	}
	return Account{}, false
}
