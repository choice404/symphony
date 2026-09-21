package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// composeMarker separates the headers a compose page shows from the body below it
const composeMarker = "--- write below this line, the lines above are headers ---"

// composeHint is the first line of a compose page
const composeHint = "  gs send  q back without sending"

// Draft is what a compose page was opened with, kept so send knows the account and the thread
type Draft struct {
	// The account the message goes out from
	Account string
	// The Message-ID being replied to, empty otherwise
	InReplyTo string
	// The References header for the thread, empty otherwise
	References string
}

/**
 * composePage
 * Builds an editable compose page with the headers filled and the body below the marker
 * @param n {int} - the draft number, names the page
 * @param a {Account} - the account
 * @param to {string} - the To header
 * @param subject {string} - the Subject header
 * @param body {string} - the body to start with
 * @return view.Page
 **/
func composePage(n int, a Account, to, subject, body string) view.Page {
	// The header block
	lines := []string{
		composeHint,
		"From: " + a.User,
		"To: " + to,
		"Cc: ",
		"Subject: " + subject,
		composeMarker,
	}
	// The body
	lines = append(lines, strings.Split(body, "\n")...)
	// The cursor goes to To when it is empty, else to the body
	cursor := 2
	if to != "" {
		cursor = 6
	}
	return view.Page{
		Name:     fmt.Sprintf("%s/%s/compose/%d", ViewName, a.Name, n),
		Title:    "compose",
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Key:      fmt.Sprintf("%d", n),
		Cursor:   cursor,
		Filetype: "compose",
		Editable: true,
	}
}

/**
 * replyBody
 * Quotes a message the way a reply does
 * @param o {Opened} - the message
 * @return string
 **/
func replyBody(o Opened) string {
	// The attribution line then every body line quoted
	var b strings.Builder
	b.WriteString("\n\nOn " + o.Date.Format(time.RFC1123) + ", " + o.From + " wrote:\n")
	for _, line := range strings.Split(o.Body, "\n") {
		b.WriteString("> " + line + "\n")
	}
	return b.String()
}

/**
 * forwardBody
 * Wraps a message the way a forward does
 * @param o {Opened} - the message
 * @return string
 **/
func forwardBody(o Opened) string {
	return "\n\n---------- Forwarded message ----------\n" +
		"From: " + o.From + "\n" +
		"Date: " + o.Date.Format(time.RFC1123) + "\n" +
		"Subject: " + o.Subject + "\n" +
		"To: " + o.To + "\n\n" +
		o.Body + "\n"
}

/**
 * replySubject
 * Prefixes a subject with Re: once
 * @param subject {string} - the subject
 * @return string
 **/
func replySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

/**
 * forwardSubject
 * Prefixes a subject with Fwd: once
 * @param subject {string} - the subject
 * @return string
 **/
func forwardSubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "fwd:") {
		return subject
	}
	return "Fwd: " + subject
}

// Outgoing is a parsed compose buffer ready to send
type Outgoing struct {
	// The From address, bare
	From string
	// Every recipient address, bare, To and Cc together
	Recipients []string
	// The raw message
	Raw []byte
	// The subject, for the notification
	Subject string
}

/**
 * parseCompose
 * Turns the text of a compose buffer into an outgoing message with the headers a mail needs
 * @param text {string} - the buffer text
 * @param d {Draft} - what the page was opened with
 * @param now {time.Time} - the Date header
 * @return Outgoing, error
 **/
func parseCompose(text string, d Draft, now time.Time) (Outgoing, error) {
	// Find the marker first, everything above it is headers and everything below is the body
	lines := strings.Split(text, "\n")
	at := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == composeMarker {
			at = i
			break
		}
	}
	if at < 0 {
		return Outgoing{}, fmt.Errorf("the marker line is missing, the body goes below it")
	}
	body := strings.Join(lines[at+1:], "\n")
	// The headers, the hint and blank lines skipped, everything else Name: value
	headers := map[string]string{}
	for _, line := range lines[:at] {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "  ") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return Outgoing{}, fmt.Errorf("header line without a colon: %q", line)
		}
		headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	// The addresses
	from, err := oneAddress(headers["from"])
	if err != nil {
		return Outgoing{}, fmt.Errorf("from: %w", err)
	}
	to, err := addresses(headers["to"])
	if err != nil {
		return Outgoing{}, fmt.Errorf("to: %w", err)
	}
	cc, err := addresses(headers["cc"])
	if err != nil {
		return Outgoing{}, fmt.Errorf("cc: %w", err)
	}
	if len(to) == 0 {
		return Outgoing{}, fmt.Errorf("to is empty")
	}
	// The recipients as bare addresses
	rcpts := make([]string, 0, len(to)+len(cc))
	for _, a := range append(append([]*mail.Address{}, to...), cc...) {
		rcpts = append(rcpts, a.Address)
	}
	// The raw message
	var b bytes.Buffer
	writeHeader(&b, "From", headers["from"])
	writeHeader(&b, "To", headers["to"])
	if headers["cc"] != "" {
		writeHeader(&b, "Cc", headers["cc"])
	}
	writeHeader(&b, "Subject", mime.QEncoding.Encode("utf-8", headers["subject"]))
	writeHeader(&b, "Date", now.Format(time.RFC1123Z))
	id, err := messageID(from.Address)
	if err != nil {
		return Outgoing{}, err
	}
	writeHeader(&b, "Message-ID", id)
	if d.InReplyTo != "" {
		writeHeader(&b, "In-Reply-To", d.InReplyTo)
		refs := d.References
		if refs == "" {
			refs = d.InReplyTo
		}
		writeHeader(&b, "References", refs)
	}
	writeHeader(&b, "MIME-Version", "1.0")
	writeHeader(&b, "Content-Type", "text/plain; charset=utf-8")
	writeHeader(&b, "Content-Transfer-Encoding", "8bit")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.TrimRight(body, "\n"), "\n", "\r\n"))
	b.WriteString("\r\n")
	return Outgoing{From: from.Address, Recipients: rcpts, Raw: b.Bytes(), Subject: headers["subject"]}, nil
}

/**
 * writeHeader
 * Writes one header line with a CRLF
 * @param b {*bytes.Buffer} - the message
 * @param name {string} - the header name
 * @param value {string} - the value
 * @return void
 **/
func writeHeader(b *bytes.Buffer, name, value string) {
	b.WriteString(name + ": " + value + "\r\n")
}

/**
 * oneAddress
 * Parses exactly one address
 * @param s {string} - the header value
 * @return *mail.Address, error
 **/
func oneAddress(s string) (*mail.Address, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("empty")
	}
	return mail.ParseAddress(s)
}

/**
 * addresses
 * Parses a list of addresses, none for an empty value
 * @param s {string} - the header value
 * @return []*mail.Address, error
 **/
func addresses(s string) ([]*mail.Address, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	return mail.ParseAddressList(s)
}

/**
 * messageID
 * Builds a Message-ID from random bytes and the sender's domain
 * @param from {string} - the bare From address
 * @return string, error
 **/
func messageID(from string) (string, error) {
	// Random bytes
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	// The domain after the at sign
	domain := "symphony"
	if i := strings.LastIndex(from, "@"); i >= 0 {
		domain = from[i+1:]
	}
	return "<" + hex.EncodeToString(b) + "@" + domain + ">", nil
}
