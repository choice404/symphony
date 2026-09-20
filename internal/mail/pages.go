package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// listWidthFrom is how many columns the sender gets in the list
const listWidthFrom = 24

// unconfiguredPage is what the mail view shows before a maildir is set
var unconfiguredPage = view.Page{
	Name:     ViewName,
	Title:    "mail",
	Lines:    []string{"mail is not configured", "", "set maildir under [mail] in ~/.config/symphony/config.toml"},
	Keys:     []string{"", "", ""},
	Filetype: "mail",
}

/**
 * listPage
 * Builds the inbox page from messages newest first, with a header line before the list
 * @param header {string} - the first line, the counts or the health
 * @param msgs {[]Message} - the messages
 * @return view.Page
 **/
func listPage(header string, msgs []Message) view.Page {
	// The header
	lines := []string{header, ""}
	keys := []string{"", ""}
	// Loop over every message
	for _, m := range msgs {
		lines = append(lines, listLine(m))
		keys = append(keys, m.ID)
	}
	// Return the page with the cursor on the first message
	return view.Page{Name: ViewName, Title: "mail", Lines: lines, Keys: keys, Cursor: 2, Filetype: "mail"}
}

/**
 * countHeader
 * Formats the inbox counts
 * @param msgs {[]Message} - the messages
 * @return string
 **/
func countHeader(msgs []Message) string {
	return fmt.Sprintf("inbox  %d messages, %d unread", len(msgs), Unread(msgs))
}

/**
 * messagePage
 * Builds the page for one opened message, headers then body
 * @param o {Opened} - the message
 * @return view.Page
 **/
func messagePage(o Opened) view.Page {
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
	return view.Page{
		Name:     ViewName + "/" + o.ID,
		Title:    o.Subject,
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Filetype: "message",
	}
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
