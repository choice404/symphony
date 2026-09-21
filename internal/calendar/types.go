// Package calendar shows Google calendars as agenda pages, event pages, and an event editor
package calendar

import (
	"context"
	"time"

	"github.com/choice404/symphony/internal/gcal"
)

// ViewName is the name of the calendar view
const ViewName = "calendar"

// AllAccounts is the pseudo account that merges every account
const AllAccounts = "all"

// Account is one Google account with calendars
type Account struct {
	// The name shown beside an event and in page names
	Name string
	// The address, for the attendee line
	User string
	// Whether the daemon can reach its calendars, an authorized oauth account
	Online bool
}

// Settings is what the view reads from the config on every render
type Settings struct {
	// The accounts
	Accounts []Account
	// How many days ahead the agenda shows
	Days int
}

// Source returns the settings as configured right now
type Source func() Settings

// Services is what the daemon does on the network for the view, nil when it cannot
type Services struct {
	// Fetches every event of every calendar of an account in a window
	Fetch func(ctx context.Context, a Account, from, to time.Time) ([]gcal.Event, error)
	// Creates an event on the account's own calendar
	Insert func(ctx context.Context, a Account, e gcal.Event) (gcal.Event, error)
	// Deletes an event from a calendar
	Delete func(ctx context.Context, a Account, calendar, id string) error
}

/**
 * Key
 * The key of an event in a page, the account and the event id
 * @param account {string} - the account name
 * @param e {gcal.Event} - the event
 * @return string
 **/
func Key(account string, e gcal.Event) string {
	return account + "/" + e.ID
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

/**
 * dayStart
 * Returns midnight of a time's day in the local zone
 * @param t {time.Time} - the time
 * @return time.Time
 **/
func dayStart(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}
