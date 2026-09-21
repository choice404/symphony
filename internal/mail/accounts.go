package mail

import (
	"sort"
)

// Account is one mailbox, a name for the column and a Maildir it syncs into
type Account struct {
	// The name shown beside a message, empty for a lone unnamed account
	Name string
	// The Maildir root, empty when not configured
	Dir string
}

// Settings is what the mail views read from the config on every render
type Settings struct {
	// The accounts
	Accounts []Account
	// How many of the newest messages the inbox shows, 0 for all
	Limit int
}

// Source returns the settings as configured right now, called on every render so a config edit lands without a restart
type Source func() Settings

/**
 * Fixed
 * Wraps one Maildir as a source with a single unnamed account and no limit that never changes, an empty dir is still one account so a contract can report it
 * @param dir {string} - the Maildir root, empty when not configured
 * @return Source
 **/
func Fixed(dir string) Source {
	return func() Settings {
		return Settings{Accounts: []Account{{Dir: dir}}}
	}
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
 * Key
 * The key of a message in a page, the account and the id so two accounts never collide
 * @return string
 **/
func (m Message) Key() string {
	// A lone unnamed account uses the bare id
	if m.Account == "" {
		return m.ID
	}
	return m.Account + "/" + m.ID
}

/**
 * ScanAccounts
 * Scans every account into one list newest first, tagging each message with its account, and returns the failures by name
 * @param accs {[]Account} - the accounts
 * @return []Message, map[string]error
 **/
func ScanAccounts(accs []Account) ([]Message, map[string]error) {
	// The merged list and the failures
	all := make([]Message, 0, 64)
	errs := map[string]error{}
	// Loop over every account
	for _, a := range accs {
		msgs, err := Scan(a.Dir)
		if err != nil {
			errs[a.Name] = err
			continue
		}
		for _, m := range msgs {
			m.Account = a.Name
			all = append(all, m)
		}
	}
	// Newest first across accounts, path as the tie break
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Date.Equal(all[j].Date) {
			return all[i].Date.After(all[j].Date)
		}
		return all[i].Path < all[j].Path
	})
	return all, errs
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
