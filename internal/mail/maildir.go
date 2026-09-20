// Package mail reads a Maildir and shows it as a view
package mail

import (
	"fmt"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Message is one message as the list shows it, read once from disk
type Message struct {
	// The path of the file
	Path string
	// The key used in the list, the file name without its flags
	ID string
	// The From header decoded
	From string
	// The Subject header decoded
	Subject string
	// The Date header parsed, zero when missing or unreadable
	Date time.Time
	// Whether the file carries the seen flag
	Seen bool
	// Whether the file carries the flagged flag
	Flagged bool
	// Whether the file carries the replied flag
	Replied bool
}

// decoder decodes RFC 2047 encoded words in headers
var decoder = mime.WordDecoder{}

/**
 * Scan
 * Reads every message in the cur and new folders of a Maildir sorted newest first
 * @param dir {string} - the Maildir root
 * @return []Message, error
 **/
func Scan(dir string) ([]Message, error) {
	// Fail when the root is missing
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("maildir %s: %w", dir, err)
	}
	// The messages collected so far
	msgs := make([]Message, 0, 64)
	// Loop over the two folders that hold messages
	for _, sub := range []string{"new", "cur"} {
		// Read the folder, a missing one is fine
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		// Loop over every file
		for _, e := range entries {
			// Skip directories and dot files
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			// Read the message headers
			m, err := readMessage(filepath.Join(dir, sub, e.Name()))
			if err != nil {
				continue
			}
			msgs = append(msgs, m)
		}
	}
	// Newest first, path as the tie break so the order is stable
	sort.SliceStable(msgs, func(i, j int) bool {
		if !msgs[i].Date.Equal(msgs[j].Date) {
			return msgs[i].Date.After(msgs[j].Date)
		}
		return msgs[i].Path < msgs[j].Path
	})
	return msgs, nil
}

/**
 * readMessage
 * Reads the headers and flags of one file
 * @param path {string} - the file
 * @return Message, error
 **/
func readMessage(path string) (Message, error) {
	// Open the file
	f, err := os.Open(path)
	if err != nil {
		return Message{}, err
	}
	defer func() { _ = f.Close() }()
	// Parse the headers only
	parsed, err := mail.ReadMessage(f)
	if err != nil {
		return Message{}, fmt.Errorf("%s: %w", path, err)
	}
	// The id and flags from the file name
	id, flags := splitFlags(filepath.Base(path))
	// The date, zero when unreadable
	date, _ := parsed.Header.Date()
	// Build the message
	return Message{
		Path:    path,
		ID:      id,
		From:    decodeHeader(parsed.Header.Get("From")),
		Subject: decodeHeader(parsed.Header.Get("Subject")),
		Date:    date,
		Seen:    strings.Contains(flags, "S"),
		Flagged: strings.Contains(flags, "F"),
		Replied: strings.Contains(flags, "R"),
	}, nil
}

/**
 * splitFlags
 * Splits a Maildir file name into its unique part and its flag letters
 * @param name {string} - the file name such as 1234.abc:2,FS
 * @return string, string
 **/
func splitFlags(name string) (string, string) {
	// The flags sit after :2,
	i := strings.Index(name, ":2,")
	// No marker means no flags
	if i < 0 {
		return name, ""
	}
	// Return both parts
	return name[:i], name[i+3:]
}

/**
 * decodeHeader
 * Decodes encoded words in a header and falls back to the raw text
 * @param raw {string} - the header value
 * @return string
 **/
func decodeHeader(raw string) string {
	// Try the decoder
	s, err := decoder.DecodeHeader(raw)
	if err != nil {
		return raw
	}
	return s
}

/**
 * Unread
 * Counts the messages without the seen flag
 * @param msgs {[]Message} - the messages
 * @return int
 **/
func Unread(msgs []Message) int {
	// The count
	n := 0
	// Loop over every message
	for _, m := range msgs {
		if !m.Seen {
			n++
		}
	}
	return n
}
