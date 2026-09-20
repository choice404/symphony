package mail

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the mail view
const ViewName = "mail"

// listWidthFrom is how many columns the sender gets in the list
const listWidthFrom = 24

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
		return view.Page{
			Name:     ViewName,
			Title:    "mail",
			Lines:    []string{"mail is not configured", "", "set maildir under [mail] in ~/.config/symphony/config.toml"},
			Keys:     []string{"", "", ""},
			Filetype: "mail",
		}, nil
	}
	// Scan
	msgs, err := v.scan()
	if err != nil {
		return view.Page{}, err
	}
	// The header
	lines := []string{fmt.Sprintf("inbox  %d messages, %d unread", len(msgs), Unread(msgs)), ""}
	keys := []string{"", ""}
	// Loop over every message
	for _, m := range msgs {
		lines = append(lines, listLine(m))
		keys = append(keys, m.ID)
	}
	// Return the page with the cursor on the first message
	return view.Page{Name: ViewName, Title: "mail", Lines: lines, Keys: keys, Cursor: 2, Filetype: "mail"}, nil
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
	m, ok := v.find(key)
	if !ok {
		return view.Fail("mail: message not in the current list, refresh"), nil
	}
	// Read it in full
	o, err := Open(m.Path)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	// The header lines
	lines := []string{
		"From:    " + o.From,
		"To:      " + o.To,
		"Date:    " + o.Date.Format(time.RFC1123),
		"Subject: " + o.Subject,
		"",
	}
	// The body lines
	lines = append(lines, strings.Split(o.Body, "\n")...)
	// Return the page, no keys since nothing on it opens
	return view.Show(view.Page{
		Name:     ViewName + "/" + o.ID,
		Title:    o.Subject,
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Filetype: "message",
	}), nil
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
 * Finds a message by id in the last scan
 * @param id {string} - the message id
 * @return Message, bool
 **/
func (v *View) find(id string) (Message, bool) {
	// The last scan
	last := v.last.Load()
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

/**
 * listLine
 * Formats one list line, a flag column, the date, the sender, and the subject
 * @param m {Message} - the message
 * @return string
 **/
func listLine(m Message) string {
	// The flag column, N for new, F for flagged, R for replied
	flag := " "
	switch {
	case !m.Seen:
		flag = "N"
	case m.Flagged:
		flag = "F"
	case m.Replied:
		flag = "R"
	}
	// The date, blank when unknown
	date := "          "
	if !m.Date.IsZero() {
		date = m.Date.Local().Format("2006-01-02")
	}
	// Return the line
	return fmt.Sprintf("%s %s  %-*s  %s", flag, date, listWidthFrom, clip(m.From, listWidthFrom), m.Subject)
}

/**
 * clip
 * Cuts a string to a width in runes
 * @param s {string} - the string
 * @param width {int} - the width
 * @return string
 **/
func clip(s string, width int) string {
	// The runes
	r := []rune(s)
	// Return whole when it fits
	if len(r) <= width {
		return s
	}
	// Cut it
	return string(r[:width])
}
