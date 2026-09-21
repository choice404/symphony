package calendar

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/gcal"
	"github.com/choice404/symphony/internal/view"
)

// agendaHint is the second line of an agenda
const agendaHint = "  <CR> open  c new  D delete  ]d [d week  gt today  ]a [a account  ga all  r refresh"

// eventHint is the footer of an event page
const eventHint = "  D delete  q back"

// dayFormat is how a day header reads
const dayFormat = "Mon Jan 02 2006"

// entry is one event with the account it came from
type entry struct {
	// The account name
	Account string
	// The event
	Event gcal.Event
}

/**
 * pageName
 * Builds the page name of an agenda
 * @param account {string} - the account name or all
 * @return string
 **/
func pageName(account string) string {
	return ViewName + "/" + account + "/agenda"
}

/**
 * agendaPage
 * Builds an agenda, a header, the hint, then the events grouped by day with the cursor on the first event of today or later
 * @param account {string} - the account name or all
 * @param from {time.Time} - the window start
 * @param to {time.Time} - the window end
 * @param entries {[]entry} - the events in start order
 * @param tags {map[string]string} - a note per account for the header
 * @param multi {bool} - whether to show the account column
 * @return view.Page
 **/
func agendaPage(account string, from, to time.Time, entries []entry, tags map[string]string, multi bool) view.Page {
	// The header
	header := fmt.Sprintf("%s agenda  %s to %s, %d events", account, from.Format("Jan 02"), to.AddDate(0, 0, -1).Format("Jan 02"), len(entries)) + notes(tags)
	lines := []string{header, agendaHint}
	keys := []string{"", ""}
	// The events by day
	cursor := 2
	today := dayStart(time.Now())
	lastDay := ""
	for _, e := range entries {
		// A new day gets a blank line and a header
		day := dayStart(e.Event.StartTime()).Format(dayFormat)
		if day != lastDay {
			lines = append(lines, "", day)
			keys = append(keys, "", "")
			lastDay = day
		}
		// The first event on or after today takes the cursor
		if cursor == 2 && !e.Event.StartTime().Before(today) {
			cursor = len(lines)
		}
		lines = append(lines, eventLine(e, multi))
		keys = append(keys, Key(e.Account, e.Event))
	}
	// Nothing at all
	if len(entries) == 0 {
		lines = append(lines, "", "  nothing scheduled")
		keys = append(keys, "", "")
	}
	return view.Page{Name: pageName(account), Title: account + " agenda", Lines: lines, Keys: keys, Cursor: cursor, Filetype: "calendar"}
}

/**
 * eventLine
 * Formats one agenda line, the time span, the account when asked, the title, and the place
 * @param e {entry} - the event
 * @param multi {bool} - whether to show the account column
 * @return string
 **/
func eventLine(e entry, multi bool) string {
	// The span
	span := "all day      "
	if !e.Event.AllDay() {
		span = e.Event.StartTime().Format("15:04") + "-" + e.Event.EndTime().Format("15:04") + "  "
	}
	// The account column
	account := ""
	if multi {
		account = fmt.Sprintf("%-8s  ", clip(e.Account, 8))
	}
	// The place
	where := ""
	if e.Event.Location != "" {
		where = "  @ " + e.Event.Location
	}
	// A cancelled event says so
	title := e.Event.Summary
	if title == "" {
		title = "(no title)"
	}
	if e.Event.Status == "cancelled" {
		title = "[cancelled] " + title
	}
	return "  " + span + account + title + where
}

/**
 * eventPage
 * Builds the page of one event
 * @param e {entry} - the event
 * @param self {string} - the account's address, to mark the reader among the attendees
 * @return view.Page
 **/
func eventPage(e entry, self string) view.Page {
	// The when line
	when := e.Event.StartTime().Format("Mon Jan 02 2006 15:04") + " to " + e.Event.EndTime().Format("15:04")
	if e.Event.AllDay() {
		when = e.Event.StartTime().Format("Mon Jan 02 2006") + " all day"
		if days := int(e.Event.EndTime().Sub(e.Event.StartTime()).Hours() / 24); days > 1 {
			when += fmt.Sprintf(", %d days", days)
		}
	}
	// The lines
	title := e.Event.Summary
	if title == "" {
		title = "(no title)"
	}
	lines := []string{
		"Title:    " + title,
		"When:     " + when,
		"Where:    " + e.Event.Location,
		"Calendar: " + e.Event.Calendar,
		"Account:  " + e.Account,
	}
	if len(e.Event.Attendees) > 0 {
		lines = append(lines, "Who:")
		for _, a := range e.Event.Attendees {
			mark := "  "
			if a.Self || a.Email == self {
				mark = "* "
			}
			lines = append(lines, fmt.Sprintf("  %s%s  %s", mark, a.Email, a.ResponseStatus))
		}
	}
	if e.Event.Description != "" {
		lines = append(lines, "", strings.TrimSpace(e.Event.Description))
		lines = strings.Split(strings.Join(lines, "\n"), "\n")
	}
	if e.Event.HTMLLink != "" {
		lines = append(lines, "", "Link:     "+e.Event.HTMLLink)
	}
	lines = append(lines, "", eventHint)
	return view.Page{
		Name:     ViewName + "/" + e.Account + "/event/" + e.Event.ID,
		Title:    title,
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Key:      Key(e.Account, e.Event),
		Filetype: "event",
	}
}

/**
 * notes
 * Formats per account notes as bracketed tags in name order
 * @param byName {map[string]string} - the note per account name
 * @return string
 **/
func notes(byName map[string]string) string {
	// The names sorted for a stable line
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	// The tags
	var b strings.Builder
	for _, n := range names {
		b.WriteString("  [" + n + " " + byName[n] + "]")
	}
	return b.String()
}

/**
 * clip
 * Cuts a string to a width in runes
 * @param s {string} - the string
 * @param width {int} - the width
 * @return string
 **/
func clip(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}
