package mail

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the mail view
const ViewName = "mail"

// View shows a Maildir as an inbox page and opens messages as pages
type View struct {
	// The Maildir root, empty when not configured
	dir string
	// The last scan, replaced whole on every refresh and never edited
	last atomic.Pointer[[]Message]
}

/**
 * NewView
 * Builds the mail view over a Maildir
 * @param dir {string} - the Maildir root, empty when not configured
 * @return *View
 **/
func NewView(dir string) *View {
	return &View{dir: dir}
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
 * Returns the one line count the home page shows
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (v *View) Summary(ctx context.Context) string {
	// Say so when nothing is configured
	if v.dir == "" {
		return "not configured"
	}
	// Scan
	msgs, err := v.scan()
	if err != nil {
		return "error: " + err.Error()
	}
	// Report the counts
	return fmt.Sprintf("%d messages, %d unread", len(msgs), Unread(msgs))
}

/**
 * Render
 * Lists the inbox newest first
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (v *View) Render(ctx context.Context) (view.Page, error) {
	// Explain the config when there is no Maildir
	if v.dir == "" {
		return unconfiguredPage, nil
	}
	// Scan
	msgs, err := v.scan()
	if err != nil {
		return view.Page{}, err
	}
	// Return the list
	return listPage(countHeader(msgs), msgs), nil
}

/**
 * Act
 * Opens a message or refreshes the list
 * @param ctx {context.Context} - the context
 * @param action {string} - open or refresh
 * @param key {string} - the message id
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
 * Builds the page for one message by id
 * @param key {string} - the message id
 * @return view.Response, error
 **/
func (v *View) open(key string) (view.Response, error) {
	// Nothing to open on a header line
	if key == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	// Find the message in the last scan
	m, ok := find(v.last.Load(), key)
	if !ok {
		return view.Fail("mail: message not in the current list, refresh"), nil
	}
	// Read it in full
	o, err := Open(m.Path)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(messagePage(o)), nil
}

/**
 * scan
 * Reads the Maildir and remembers the result for open
 * @return []Message, error
 **/
func (v *View) scan() ([]Message, error) {
	// Scan
	msgs, err := Scan(v.dir)
	if err != nil {
		return nil, err
	}
	// Remember the new slice, the old one is left alone
	v.last.Store(&msgs)
	return msgs, nil
}

/**
 * find
 * Finds a message by id in a scan
 * @param last {*[]Message} - the scan, nil for none
 * @param id {string} - the message id
 * @return Message, bool
 **/
func find(last *[]Message, id string) (Message, bool) {
	// No scan yet
	if last == nil {
		return Message{}, false
	}
	// Loop over every message
	for _, m := range *last {
		if m.ID == id {
			return m, true
		}
	}
	return Message{}, false
}
