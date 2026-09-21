package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/view"
)

// plain is a simple message
const plain = "From: Ada <ada@example.com>\r\nTo: me@example.com\r\nSubject: hello there\r\nDate: Mon, 02 Jan 2006 15:04:05 -0700\r\nMessage-ID: <1@example.com>\r\n\r\nfirst line\r\n\r\n\r\n\r\nlast line\r\n"

// encoded has an encoded subject and a quoted printable body
const encoded = "From: =?UTF-8?B?QmrDtnJu?= <bjorn@example.com>\r\nSubject: =?UTF-8?Q?caf=C3=A9_plans?=\r\nDate: Tue, 03 Jan 2006 10:00:00 +0000\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\ncaf=C3=A9 at noon\r\n"

// multi is multipart with html first and plain second, plus an attachment
const multi = "From: Cy <cy@example.com>\r\nSubject: multi\r\nDate: Wed, 04 Jan 2006 10:00:00 +0000\r\nContent-Type: multipart/alternative; boundary=\"bb\"\r\n\r\n--bb\r\nContent-Type: text/html\r\n\r\n<p>hello <b>html</b></p>\r\n--bb\r\nContent-Type: text/plain\r\n\r\nhello plain\r\n--bb\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"x.pdf\"\r\n\r\nPDFBYTES\r\n--bb--\r\n"

// htmlOnly has no plain part
const htmlOnly = "From: Di <di@example.com>\r\nSubject: html only\r\nDate: Thu, 05 Jan 2006 10:00:00 +0000\r\nContent-Type: text/html\r\n\r\n<div>line one</div><div>line &amp; two</div>\r\n"

// fixture writes a Maildir with the sample messages and returns its root
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"new", "cur", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"cur/1.plain:2,S":    plain,
		"cur/2.encoded:2,FS": encoded,
		"new/3.multi":        multi,
		"cur/4.html:2,RS":    htmlOnly,
		"cur/.hidden":        plain,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScanOrderFlagsAndDecoding(t *testing.T) {
	msgs, err := Scan(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	// Four messages, the dot file skipped, newest first
	if len(msgs) != 4 {
		t.Fatalf("count = %d", len(msgs))
	}
	if msgs[0].ID != "4.html" || msgs[3].ID != "1.plain" {
		t.Fatalf("order = %s .. %s", msgs[0].ID, msgs[3].ID)
	}
	// Flags read from the file name
	if msgs[3].Seen != true || msgs[1].Seen != false {
		t.Fatalf("seen flags wrong: %+v %+v", msgs[3], msgs[1])
	}
	if !msgs[2].Flagged || !msgs[0].Replied {
		t.Fatalf("flagged or replied wrong: %+v %+v", msgs[2], msgs[0])
	}
	// Encoded words decoded
	if msgs[2].From != "Björn <bjorn@example.com>" || msgs[2].Subject != "café plans" {
		t.Fatalf("decoded = %q %q", msgs[2].From, msgs[2].Subject)
	}
	if Unread(msgs) != 1 {
		t.Fatalf("unread = %d", Unread(msgs))
	}
}

func TestScanMissingDir(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing maildir")
	}
}

func TestOpenBodies(t *testing.T) {
	root := fixture(t)
	cases := map[string]string{
		"cur/1.plain:2,S":    "first line\n\nlast line",
		"cur/2.encoded:2,FS": "café at noon",
		"new/3.multi":        "hello plain",
		"cur/4.html:2,RS":    "line one\nline & two",
	}
	for name, want := range cases {
		o, err := Open(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if o.Body != want {
			t.Fatalf("%s body = %q want %q", name, o.Body, want)
		}
	}
	o, _ := Open(filepath.Join(root, "cur/1.plain:2,S"))
	if o.To != "me@example.com" {
		t.Fatalf("to = %q", o.To)
	}
}

func TestViewUnconfigured(t *testing.T) {
	v := NewView(Fixed(""))
	p, err := v.Render(context.Background())
	if err != nil || !strings.Contains(p.Lines[0], "not configured") {
		t.Fatalf("page = %+v %v", p, err)
	}
	if v.Summary(context.Background()) != "not configured" {
		t.Fatalf("summary = %q", v.Summary(context.Background()))
	}
}

func TestViewListAndOpen(t *testing.T) {
	v := NewView(Fixed(fixture(t)))
	ctx := context.Background()
	if got := v.Summary(ctx); got != "4 messages, 1 unread" {
		t.Fatalf("summary = %q", got)
	}
	p, err := v.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Two header lines then one line per message with its id as the key
	if len(p.Lines) != 6 || p.Keys[2] != "4.html" || p.Keys[5] != "1.plain" {
		t.Fatalf("lines = %d keys = %v", len(p.Lines), p.Keys)
	}
	// The new message shows N, the flagged one F
	if !strings.HasPrefix(p.Lines[3], "N ") || !strings.HasPrefix(p.Lines[4], "F ") {
		t.Fatalf("flag columns = %q %q", p.Lines[3], p.Lines[4])
	}
	if !strings.Contains(p.Lines[5], "hello there") || !strings.Contains(p.Lines[5], "Ada") {
		t.Fatalf("line = %q", p.Lines[5])
	}
	// Opening a message gives a page with headers then body
	resp, err := v.Act(ctx, "open", "1.plain")
	if err != nil || resp.Kind != view.KindPage {
		t.Fatalf("open = %+v %v", resp, err)
	}
	if resp.Page.Name != "mail/1.plain" || resp.Page.Lines[3] != "Subject: hello there" || resp.Page.Lines[5] != "first line" {
		t.Fatalf("page = %+v", resp.Page.Lines)
	}
	// A header line opens nothing
	resp, _ = v.Act(ctx, "open", "")
	if resp.Kind != view.KindNone {
		t.Fatalf("header open = %+v", resp)
	}
	// An unknown id is a message
	resp, _ = v.Act(ctx, "open", "ghost")
	if resp.Kind != view.KindNotify || !resp.Error {
		t.Fatalf("ghost open = %+v", resp)
	}
	// Refresh renders again
	resp, _ = v.Act(ctx, "refresh", "")
	if resp.Kind != view.KindPage || len(resp.Page.Lines) != 6 {
		t.Fatalf("refresh = %+v", resp)
	}
}

func TestViewFollowsDirChanges(t *testing.T) {
	// The dir function answers whatever the config says right now
	dir := ""
	v := NewView(func() Settings { return Fixed(dir)() })
	ctx := context.Background()
	if got := v.Summary(ctx); got != "not configured" {
		t.Fatalf("before = %q", got)
	}
	dir = fixture(t)
	if got := v.Summary(ctx); got != "4 messages, 1 unread" {
		t.Fatalf("after = %q", got)
	}
}

func TestViewTwoAccounts(t *testing.T) {
	// Two accounts, the second one a missing directory
	a := fixture(t)
	missing := filepath.Join(t.TempDir(), "nope")
	src := func() Settings {
		return Settings{Accounts: []Account{{Name: "personal", Dir: a}, {Name: "school", Dir: missing}}}
	}
	v := NewView(src)
	ctx := context.Background()
	// The summary counts the good one and names the failed one
	if got := v.Summary(ctx); !strings.HasPrefix(got, "4 messages, 1 unread") || !strings.Contains(got, "[school error]") {
		t.Fatalf("summary = %q", got)
	}
	p, err := v.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The header names the failed account, the lines carry the account column, the keys carry the account
	if !strings.Contains(p.Lines[0], "[school error:") {
		t.Fatalf("header = %q", p.Lines[0])
	}
	if !strings.Contains(p.Lines[2], "personal") || p.Keys[2] != "personal/4.html" {
		t.Fatalf("line = %q key = %q", p.Lines[2], p.Keys[2])
	}
	// Opening by the account key works and the page is named by it
	resp, _ := v.Act(ctx, "open", "personal/1.plain")
	if resp.Kind != view.KindPage || resp.Page.Name != "mail/personal/1.plain" {
		t.Fatalf("open = %+v", resp)
	}
}

func TestViewLimitKeepsNewest(t *testing.T) {
	a := fixture(t)
	src := func() Settings { return Settings{Accounts: []Account{{Dir: a}}, Limit: 2} }
	p, err := NewView(src).Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Two header lines then only the two newest
	if len(p.Lines) != 4 || p.Keys[2] != "4.html" || p.Keys[3] != "3.multi" {
		t.Fatalf("lines = %d keys = %v", len(p.Lines), p.Keys)
	}
}
