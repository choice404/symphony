package mail

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"regexp"
	"strings"
)

// maxBody caps how much of a body is read so a huge attachment never lands in a buffer
const maxBody = 4 << 20

// tags matches an html tag
var tags = regexp.MustCompile(`(?s)<[^>]*>`)

// blankRuns collapses three or more newlines
var blankRuns = regexp.MustCompile(`\n{3,}`)

// Opened is a message read in full
type Opened struct {
	// The list entry
	Message
	// The To header decoded
	To string
	// The body as plain text
	Body string
}

/**
 * Open
 * Reads one message file in full and picks a plain text body out of it
 * @param path {string} - the file
 * @return Opened, error
 **/
func Open(path string) (Opened, error) {
	// Read the headers
	m, err := readMessage(path)
	if err != nil {
		return Opened{}, err
	}
	// Open the file again for the body
	f, err := os.Open(path)
	if err != nil {
		return Opened{}, err
	}
	defer func() { _ = f.Close() }()
	// Parse it
	parsed, err := mail.ReadMessage(io.LimitReader(f, maxBody))
	if err != nil {
		return Opened{}, fmt.Errorf("%s: %w", path, err)
	}
	// Pull the body out of the parts
	body, err := bodyOf(parsed.Header.Get("Content-Type"), parsed.Header.Get("Content-Transfer-Encoding"), parsed.Body)
	if err != nil {
		return Opened{}, err
	}
	// Return the opened message
	return Opened{Message: m, To: decodeHeader(parsed.Header.Get("To")), Body: body}, nil
}

/**
 * bodyOf
 * Walks a part and returns its text, the first text/plain wins and text/html is stripped as a fallback
 * @param ctype {string} - the Content-Type header
 * @param encoding {string} - the Content-Transfer-Encoding header
 * @param r {io.Reader} - the part body
 * @return string, error
 **/
func bodyOf(ctype, encoding string, r io.Reader) (string, error) {
	// Parse the media type, a missing one is plain text
	media, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		media = "text/plain"
	}
	// Recurse into a multipart
	if strings.HasPrefix(media, "multipart/") {
		return multipartBody(params["boundary"], r)
	}
	// Decode the transfer encoding
	data, err := io.ReadAll(decode(encoding, r))
	if err != nil {
		return "", err
	}
	// Return by type
	switch media {
	case "text/plain":
		return normalize(string(data)), nil
	case "text/html":
		return normalize(stripHTML(string(data))), nil
	}
	// Anything else is not text
	return "", nil
}

/**
 * multipartBody
 * Reads every part of a multipart and prefers plain text over html
 * @param boundary {string} - the boundary
 * @param r {io.Reader} - the body
 * @return string, error
 **/
func multipartBody(boundary string, r io.Reader) (string, error) {
	// No boundary means nothing to read
	if boundary == "" {
		return "", nil
	}
	// The reader over the parts
	mr := multipart.NewReader(r, boundary)
	// The html fallback if no plain part shows up
	fallback := ""
	// Loop over every part
	for {
		// Next part, EOF ends the loop
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fallback, nil
		}
		// The part's type
		ctype := p.Header.Get("Content-Type")
		// Skip attachments
		if strings.HasPrefix(strings.ToLower(p.Header.Get("Content-Disposition")), "attachment") {
			continue
		}
		// Read the part recursively
		text, err := bodyOf(ctype, p.Header.Get("Content-Transfer-Encoding"), p)
		if err != nil {
			continue
		}
		// Plain text wins right away
		if strings.HasPrefix(strings.ToLower(ctype), "text/plain") || strings.HasPrefix(strings.ToLower(ctype), "multipart/") {
			if text != "" {
				return text, nil
			}
			continue
		}
		// Keep the first non plain text as the fallback
		if fallback == "" && text != "" {
			fallback = text
		}
	}
	return fallback, nil
}

/**
 * decode
 * Wraps a reader in the transfer decoding it needs
 * @param encoding {string} - the Content-Transfer-Encoding header
 * @param r {io.Reader} - the raw body
 * @return io.Reader
 **/
func decode(encoding string, r io.Reader) io.Reader {
	// Dispatch on the encoding
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	}
	// Anything else is passed through
	return r
}

/**
 * stripHTML
 * Drops tags and unescapes entities so html reads as text
 * @param s {string} - the html
 * @return string
 **/
func stripHTML(s string) string {
	// Turn block ends into newlines first
	s = regexp.MustCompile(`(?i)</(p|div|br|tr|li|h[1-6])\s*>|<br\s*/?>`).ReplaceAllString(s, "\n")
	// Drop every other tag
	s = tags.ReplaceAllString(s, "")
	// Unescape entities
	return html.UnescapeString(s)
}

/**
 * normalize
 * Unifies line endings and trims runs of blank lines
 * @param s {string} - the text
 * @return string
 **/
func normalize(s string) string {
	// Unix line endings
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Collapse long blank runs
	s = blankRuns.ReplaceAllString(s, "\n\n")
	// Trim the ends
	return strings.TrimSpace(s)
}
