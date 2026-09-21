// Package imapsync pulls the newest messages of an IMAP inbox into a Maildir and keeps their flags in step
package imapsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
)

// stateFile is the sync state kept at the Maildir root
const stateFile = ".symphony-sync.json"

// State is what the last sync left behind, the mailbox identity and the file each uid landed in
type State struct {
	// The UIDVALIDITY the files belong to, a change throws them all away
	UIDValidity uint32 `json:"uidvalidity"`
	// The file of every uid, relative to the Maildir root
	Files map[uint32]string `json:"files"`
}

/**
 * loadState
 * Reads the state file, an empty state when there is none
 * @param dir {string} - the Maildir root
 * @return State, error
 **/
func loadState(dir string) (State, error) {
	// The empty state
	st := State{Files: map[uint32]string{}}
	// Read when present
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("sync state: %w", err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("sync state: %w", err)
	}
	if st.Files == nil {
		st.Files = map[uint32]string{}
	}
	return st, nil
}

/**
 * saveState
 * Writes the state file
 * @param dir {string} - the Maildir root
 * @param st {State} - the state
 * @return error
 **/
func saveState(dir string, st State) error {
	// Encode
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	// Write through a temp file so a crash never leaves half a state
	tmp := filepath.Join(dir, stateFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("sync state: %w", err)
	}
	return os.Rename(tmp, filepath.Join(dir, stateFile))
}

/**
 * ensureMaildir
 * Creates the cur, new, and tmp folders
 * @param dir {string} - the Maildir root
 * @return error
 **/
func ensureMaildir(dir string) error {
	// Loop over the three folders
	for _, sub := range []string{"cur", "new", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return fmt.Errorf("maildir: %w", err)
		}
	}
	return nil
}

/**
 * flagLetters
 * Turns IMAP flags into the sorted Maildir flag letters
 * @param flags {[]imap.Flag} - the flags
 * @return string
 **/
func flagLetters(flags []imap.Flag) string {
	// The letters
	letters := make([]string, 0, 5)
	for _, f := range flags {
		switch f {
		case imap.FlagSeen:
			letters = append(letters, "S")
		case imap.FlagAnswered:
			letters = append(letters, "R")
		case imap.FlagFlagged:
			letters = append(letters, "F")
		case imap.FlagDeleted:
			letters = append(letters, "T")
		case imap.FlagDraft:
			letters = append(letters, "D")
		}
	}
	// Maildir wants them sorted
	sort.Strings(letters)
	return strings.Join(letters, "")
}

/**
 * fileName
 * Builds a Maildir file name for a uid, unique through the time and the uid
 * @param uid {uint32} - the uid
 * @param when {time.Time} - the delivery time
 * @param host {string} - this machine's name
 * @param flags {string} - the flag letters
 * @return string
 **/
func fileName(uid uint32, when time.Time, host, flags string) string {
	return fmt.Sprintf("%d.U%d.%s:2,%s", when.Unix(), uid, host, flags)
}

/**
 * withFlags
 * Returns a Maildir file path with its flag letters replaced
 * @param rel {string} - the file relative to the root, such as cur/123.U5.host:2,S
 * @param flags {string} - the new letters
 * @return string
 **/
func withFlags(rel string, flags string) string {
	// Cut at the flag marker when present
	if i := strings.Index(rel, ":2,"); i >= 0 {
		rel = rel[:i]
	}
	return rel + ":2," + flags
}

/**
 * writeMessage
 * Writes a message into the Maildir through tmp and returns its file relative to the root
 * @param dir {string} - the Maildir root
 * @param uid {uint32} - the uid
 * @param body {[]byte} - the raw message
 * @param when {time.Time} - the delivery time
 * @param host {string} - this machine's name
 * @param flags {string} - the flag letters
 * @return string, error
 **/
func writeMessage(dir string, uid uint32, body []byte, when time.Time, host, flags string) (string, error) {
	// The name, then the temp path and the final path
	name := fileName(uid, when, host, flags)
	tmp := filepath.Join(dir, "tmp", name)
	rel := filepath.Join("cur", name)
	// Write to tmp
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	// Move into cur
	if err := os.Rename(tmp, filepath.Join(dir, rel)); err != nil {
		return "", fmt.Errorf("move %s: %w", name, err)
	}
	return rel, nil
}

/**
 * hostName
 * Returns this machine's name for file names, localhost when unknown
 * @return string
 **/
func hostName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "localhost"
	}
	return h
}
