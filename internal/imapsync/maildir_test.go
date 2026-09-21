package imapsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
)

func TestFlagLettersSorted(t *testing.T) {
	got := flagLetters([]imap.Flag{imap.FlagSeen, imap.FlagFlagged, imap.FlagAnswered, "\\Junk"})
	if got != "FRS" {
		t.Fatalf("letters = %q", got)
	}
	if flagLetters(nil) != "" {
		t.Fatal("no flags should be empty")
	}
}

func TestFileNameAndWithFlags(t *testing.T) {
	name := fileName(42, time.Unix(1700000000, 0), "box", "S")
	if name != "1700000000.U42.box:2,S" {
		t.Fatalf("name = %q", name)
	}
	if got := withFlags("cur/"+name, "FS"); got != "cur/1700000000.U42.box:2,FS" {
		t.Fatalf("withFlags = %q", got)
	}
	if got := withFlags("cur/noflags", "S"); got != "cur/noflags:2,S" {
		t.Fatalf("withFlags on bare = %q", got)
	}
}

func TestWriteMessageLandsInCur(t *testing.T) {
	dir := t.TempDir()
	if err := ensureMaildir(dir); err != nil {
		t.Fatal(err)
	}
	rel, err := writeMessage(dir, 7, []byte("From: a\r\n\r\nhi\r\n"), time.Unix(1, 0), "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rel, "cur/") || !strings.HasSuffix(rel, ":2,") {
		t.Fatalf("rel = %q", rel)
	}
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil || string(data) != "From: a\r\n\r\nhi\r\n" {
		t.Fatalf("body = %q %v", data, err)
	}
	// tmp is left empty
	entries, _ := os.ReadDir(filepath.Join(dir, "tmp"))
	if len(entries) != 0 {
		t.Fatal("tmp not empty")
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := loadState(dir)
	if err != nil || st.UIDValidity != 0 || len(st.Files) != 0 {
		t.Fatalf("empty state = %+v %v", st, err)
	}
	st.UIDValidity = 9
	st.Files[3] = "cur/x:2,S"
	if err := saveState(dir, st); err != nil {
		t.Fatal(err)
	}
	back, err := loadState(dir)
	if err != nil || back.UIDValidity != 9 || back.Files[3] != "cur/x:2,S" {
		t.Fatalf("state = %+v %v", back, err)
	}
	// No temp file left behind
	if _, err := os.Stat(filepath.Join(dir, stateFile+".tmp")); err == nil {
		t.Fatal("temp state left behind")
	}
}
