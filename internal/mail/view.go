package mail

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the mail view
const ViewName = "mail"

// View shows every account's Maildir as one inbox page and opens messages as pages
type View struct {
	// Returns the accounts as configured right now
	src Source
	// The last scan, replaced whole on every refresh and never edited
	last atomic.Pointer[[]Message]
}

/**
 * NewView
 * Builds the mail view over a source of accounts
 * @param src {Source} - returns the accounts, called on every render so a config edit lands without a restart
 * @return *View
 **/
func NewView(src Source) *View {
	return &View{src: src}
}

/**
 * Name
 * Returns mail
 * @return string
 **/
func (v *View) Name() string {
	return ViewName
}

/**
 * Summary
 * Returns the one line count the home page shows, with any account that failed named
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (v *View) Summary(ctx context.Context) string {
	// Say so when nothing is configured
	set := v.src()
	accs := Configured(set.Accounts)
	if len(accs) == 0 {
		return "not configured"
	}
	// Scan
	msgs, errs := v.scan(accs, set.Limit)
	// A lone account that failed is just the error
	if len(accs) == 1 && len(errs) == 1 {
		for _, err := range errs {
			return "error: " + err.Error()
		}
	}
	// The counts, then every failed account
	out := fmt.Sprintf("%d messages, %d unread", len(msgs), Unread(msgs))
	if len(errs) > 0 {
		failed := map[string]string{}
		for name := range errs {
			failed[name] = "error"
		}
		out += notes(failed)
	}
	return out
}

/**
 * Render
 * Lists every account's inbox newest first
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (v *View) Render(ctx context.Context) (view.Page, error) {
	// Explain the config when there is no account
	set := v.src()
	accs := Configured(set.Accounts)
	if len(accs) == 0 {
		return unconfiguredPage, nil
	}
	// Scan
	msgs, errs := v.scan(accs, set.Limit)
	// A lone account that failed is an error page
	if len(accs) == 1 && len(errs) == 1 {
		for _, err := range errs {
			return view.Page{}, err
		}
	}
	// The header carries the counts and every failed account
	header := countHeader(msgs)
	if len(errs) > 0 {
		failed := map[string]string{}
		for name, err := range errs {
			failed[name] = "error: " + err.Error()
		}
		header += notes(failed)
	}
	// Return the list
	return listPage(header, msgs, len(accs) > 1), nil
}

/**
 * Act
 * Opens a message or refreshes the list
 * @param ctx {context.Context} - the context
 * @param action {string} - open or refresh
 * @param key {string} - the message key
 * @return view.Response, error
 **/
func (v *View) Act(ctx context.Context, action, key string) (view.Response, error) {
	// Dispatch on the action
	switch action {
	case "refresh":
		p, err := v.Render(ctx)
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return view.Show(p), nil
	case "open":
		return v.open(key)
	}
	return view.Fail("mail: unknown action " + action), nil
}

/**
 * open
 * Builds the page for one message by key
 * @param key {string} - the message key
 * @return view.Response, error
 **/
func (v *View) open(key string) (view.Response, error) {
	// Nothing to open on a header line
	if key == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	// Find the message in the last scan
	last := v.last.Load()
	if last == nil {
		return view.Fail("mail: message not in the current list, refresh"), nil
	}
	m, ok := findKey(*last, key)
	if !ok {
		return view.Fail("mail: message not in the current list, refresh"), nil
	}
	// Read it in full
	o, err := Open(m.Path)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	o.Account = m.Account
	return view.Show(messagePage(o)), nil
}

/**
 * scan
 * Reads every account, keeps the newest up to the limit, and remembers the result for open
 * @param accs {[]Account} - the configured accounts
 * @param limit {int} - how many to keep, 0 for all
 * @return []Message, map[string]error
 **/
func (v *View) scan(accs []Account, limit int) ([]Message, map[string]error) {
	// Scan and cut
	msgs, errs := ScanAccounts(accs)
	msgs = newest(msgs, limit)
	// Remember the new slice, the old one is left alone
	v.last.Store(&msgs)
	return msgs, errs
}
