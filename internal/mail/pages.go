package mail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// listWidthFrom is how many columns the sender gets in the list
const listWidthFrom = 24

// listWidthAccount is how many columns the account name gets in the list
const listWidthAccount = 8

// listHint is the second line of every list, the keys that move around
const listHint = "  <CR> open  c compose  R reply  F forward  D trash  ]a [a account  ga all  gi inbox  gs sent  gS spam  r refresh"

// messageHint is the footer of a message page
const messageHint = "  R reply  F forward  D trash  q back"

// unconfiguredPage is what the mail view shows before any account is set
var unconfiguredPage = view.Page{
	Name:  ViewName,
	Title: "mail",
	Lines: []string{
		"mail is not configured",
		"",
		"set maildir under [mail] in ~/.config/symphony/config.toml for one account,",
		"or one [[mail.accounts]] table per account with a name and a maildir",
	},
	Keys:     []string{"", "", "", ""},
	Filetype: "mail",
}

/**
 * pageName
 * Builds the page name of a list
 * @param account {string} - the account name or all
 * @param folder {string} - the folder
 * @return string
 **/
func pageName(account, folder string) string {
	return ViewName + "/" + account + "/" + folder
}

/**
 * listPage
 * Builds a list page, a header, the hint line, then one line per message
 * @param account {string} - the account name or all
 * @param folder {string} - the folder
 * @param header {string} - the first line, the counts and the health tags
 * @param msgs {[]Message} - the messages
 * @param multi {bool} - whether to show the account column
 * @return view.Page
 **/
func listPage(account, folder, header string, msgs []Message, multi bool) view.Page {
	// The header and the hint
	lines := []string{header, listHint, ""}
	keys := []string{"", "", ""}
	// Loop over every message
	for _, m := range msgs {
		lines = append(lines, listLine(m, multi))
		keys = append(keys, m.Key())
	}
	// Return the page with the cursor on the first message
	return view.Page{
		Name:     pageName(account, folder),
		Title:    account + " " + folder,
		Lines:    lines,
		Keys:     keys,
		Cursor:   3,
		Filetype: "mail",
	}
}

/**
 * errorPage
 * Builds a list page whose content is one error, for a lone account that cannot be read
 * @param account {string} - the account name
 * @param folder {string} - the folder
 * @param text {string} - the error
 * @return view.Page
 **/
func errorPage(account, folder, text string) view.Page {
	return view.Page{
		Name:     pageName(account, folder),
		Title:    account + " " + folder,
		Lines:    []string{"mail " + text, listHint, "", "fix the config and press r"},
		Keys:     []string{"", "", "", ""},
		Filetype: "mail",
	}
}

/**
 * countHeader
 * Formats a list header, the account, the folder, and the counts
 * @param account {string} - the account name or all
 * @param folder {string} - the folder
 * @param msgs {[]Message} - the messages shown
 * @return string
 **/
func countHeader(account, folder string, msgs []Message) string {
	return fmt.Sprintf("%s %s  %d messages, %d unread", account, folder, len(msgs), Unread(msgs))
}

/**
 * notes
 * Formats per account notes such as a state or an error as bracketed tags in name order
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
		b.WriteString("  [")
		if n != "" {
			b.WriteString(n + " ")
		}
		b.WriteString(byName[n])
		b.WriteString("]")
	}
	return b.String()
}

/**
 * messagePage
 * Builds the page for one opened message, headers then body then the hint, keyed as a whole by the message
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
	// The body lines and the hint
	lines = append(lines, strings.Split(o.Body, "\n")...)
	lines = append(lines, "", messageHint)
	// Return the page, no line keys since the page as a whole is the message
	return view.Page{
		Name:     ViewName + "/" + o.Key(),
		Title:    o.Subject,
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Key:      o.Key(),
		Filetype: "message",
	}
}

/**
 * listLine
 * Formats one list line, a flag column, the date, the account when asked, the label when set, the sender, and the subject
 * @param m {Message} - the message
 * @param multi {bool} - whether to show the account column
 * @return string
 **/
func listLine(m Message, multi bool) string {
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
	// The account column, only with more than one account
	account := ""
	if multi {
		account = fmt.Sprintf("%-*s  ", listWidthAccount, clip(m.Account, listWidthAccount))
	}
	// The label column, only when a classifier ran
	label := ""
	if m.Label != "" {
		label = fmt.Sprintf("%-8s  ", clip(m.Label, 8))
	}
	// Return the line
	return fmt.Sprintf("%s %s  %s%s%-*s  %s", flag, date, account, label, listWidthFrom, clip(m.From, listWidthFrom), m.Subject)
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
