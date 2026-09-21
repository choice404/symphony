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

// sentOne is a message in the sent folder
const sentOne = "From: me@example.com\r\nTo: Ada <ada@example.com>\r\nSubject: my reply\r\nDate: Fri, 06 Jan 2006 10:00:00 +0000\r\n\r\nsent body\r\n"

// fixture writes a Maildir with the sample messages and a sent folder and returns its root
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"new", "cur", "tmp", ".Sent/cur", ".Sent/new", ".Sent/tmp"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"cur/1.plain:2,S":      plain,
		"cur/2.encoded:2,FS":   encoded,
		"new/3.multi":          multi,
		"cur/4.html:2,RS":      htmlOnly,
		"cur/.hidden":          plain,
		".Sent/cur/9.sent:2,S": sentOne,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// two builds a source with two accounts, the first on the fixture and the second on a path
func two(t *testing.T, second string) Source {
	t.Helper()
	a := fixture(t)
	return func() Settings {
		return Settings{Accounts: []Account{{Name: "personal", Dir: a, User: "me@example.com"}, {Name: "school", Dir: second, User: "me@school.edu"}}}
	}
}

func TestScanOrderFlagsAndDecoding(t *testing.T) {
	msgs, err := Scan(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	// Four messages, the dot file and the sent folder skipped, newest first
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
	// Encoded words decoded, the message id kept
	if msgs[2].From != "Björn <bjorn@example.com>" || msgs[2].Subject != "café plans" {
		t.Fatalf("decoded = %q %q", msgs[2].From, msgs[2].Subject)
	}
	if msgs[3].MessageID != "<1@example.com>" {
		t.Fatalf("message id = %q", msgs[3].MessageID)
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

func TestFolderDir(t *testing.T) {
	if FolderDir("/m", FolderInbox) != "/m" || FolderDir("/m", FolderSent) != "/m/.Sent" {
		t.Fatalf("folder dirs = %q %q", FolderDir("/m", FolderInbox), FolderDir("/m", FolderSent))
	}
}

func TestUnconfigured(t *testing.T) {
	m := New(func() Settings { return Settings{} }, Plain{}, Services{})
	p, err := m.Render(context.Background())
	if err != nil || !strings.Contains(p.Lines[0], "not configured") {
		t.Fatalf("page = %+v %v", p, err)
	}
	if e := m.Entries(); len(e) != 1 || e[0].Summary(context.Background()) != "not configured" {
		t.Fatalf("entries = %+v", e)
	}
}

func TestEntriesPerAccountAndAll(t *testing.T) {
	m := New(two(t, filepath.Join(t.TempDir(), "nope")), Plain{}, Services{})
	ctx := context.Background()
	e := m.Entries()
	if len(e) != 3 || e[0].Name != "mail/personal/inbox" || e[1].Name != "mail/school/inbox" || e[2].Name != "mail" {
		t.Fatalf("entries = %+v", e)
	}
	if got := e[0].Summary(ctx); got != "4 messages, 1 unread" {
		t.Fatalf("personal = %q", got)
	}
	if got := e[1].Summary(ctx); !strings.Contains(got, "maildir") {
		t.Fatalf("school = %q", got)
	}
	if got := e[2].Summary(ctx); !strings.HasPrefix(got, "4 messages, 1 unread") || !strings.Contains(got, "[school ") {
		t.Fatalf("all = %q", got)
	}
}

func TestListPagesAndNavigation(t *testing.T) {
	m := New(two(t, filepath.Join(t.TempDir(), "nope")), Plain{}, Services{})
	ctx := context.Background()
	// The merged inbox carries the account column and a tag for the failed account
	p, err := m.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "mail/all/inbox" || !strings.Contains(p.Lines[0], "[school ") || !strings.Contains(p.Lines[3], "personal") || p.Keys[3] != "personal/inbox/4.html" {
		t.Fatalf("all page = %v keys = %v", p.Lines[:4], p.Keys[:4])
	}
	// One account has no account column and its own name
	p, _ = m.RenderPath(ctx, "personal/inbox")
	if p.Name != "mail/personal/inbox" || strings.Contains(p.Lines[3], "personal  ") || !strings.HasPrefix(p.Lines[0], "personal inbox  4 messages") {
		t.Fatalf("personal page = %v", p.Lines[:4])
	}
	// The sent folder
	p, _ = m.RenderPath(ctx, "personal/sent")
	if len(p.Lines) != 4 || !strings.Contains(p.Lines[3], "my reply") || p.Keys[3] != "personal/sent/9.sent" {
		t.Fatalf("sent page = %v keys = %v", p.Lines, p.Keys)
	}
	// A broken lone account is an error page
	p, _ = m.RenderPath(ctx, "school/inbox")
	if !strings.HasPrefix(p.Lines[0], "mail ") || !strings.Contains(p.Lines[0], "maildir") {
		t.Fatalf("school page = %v", p.Lines)
	}
	// Next cycles all, personal, school, all
	r, _ := m.Act(ctx, view.Action{Name: "next", Page: "mail/all/inbox"})
	if r.Page.Name != "mail/personal/inbox" {
		t.Fatalf("next from all = %s", r.Page.Name)
	}
	r, _ = m.Act(ctx, view.Action{Name: "next", Page: "mail/school/inbox"})
	if r.Page.Name != "mail/all/inbox" {
		t.Fatalf("next from school = %s", r.Page.Name)
	}
	r, _ = m.Act(ctx, view.Action{Name: "prev", Page: "mail/all/inbox"})
	if r.Page.Name != "mail/school/inbox" {
		t.Fatalf("prev from all = %s", r.Page.Name)
	}
	// Folder and all keep the account or the folder
	r, _ = m.Act(ctx, view.Action{Name: "folder", Key: "sent", Page: "mail/personal/inbox"})
	if r.Page.Name != "mail/personal/sent" {
		t.Fatalf("folder = %s", r.Page.Name)
	}
	r, _ = m.Act(ctx, view.Action{Name: "all", Page: "mail/personal/sent"})
	if r.Page.Name != "mail/all/sent" {
		t.Fatalf("all = %s", r.Page.Name)
	}
}

func TestOpenAndMessagePage(t *testing.T) {
	m := New(two(t, ""), Plain{}, Services{})
	ctx := context.Background()
	if _, err := m.Render(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ := m.Act(ctx, view.Action{Name: "open", Key: "personal/inbox/1.plain", Page: "mail/all/inbox"})
	if r.Kind != view.KindPage || r.Page.Name != "mail/personal/inbox/1.plain" || r.Page.Key != "personal/inbox/1.plain" {
		t.Fatalf("open = %+v", r)
	}
	if r.Page.Lines[3] != "Subject: hello there" || r.Page.Lines[5] != "first line" {
		t.Fatalf("message page = %v", r.Page.Lines)
	}
	// The same page by name
	p, err := m.RenderPath(ctx, "personal/inbox/1.plain")
	if err != nil || p.Name != "mail/personal/inbox/1.plain" {
		t.Fatalf("by name = %+v %v", p, err)
	}
	// A header line opens nothing and an unknown key is a message
	r, _ = m.Act(ctx, view.Action{Name: "open", Page: "mail/all/inbox"})
	if r.Kind != view.KindNone {
		t.Fatalf("header open = %+v", r)
	}
	r, _ = m.Act(ctx, view.Action{Name: "open", Key: "personal/inbox/ghost", Page: "mail/all/inbox"})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("ghost open = %+v", r)
	}
}

func TestLimitKeepsNewest(t *testing.T) {
	a := fixture(t)
	src := func() Settings { return Settings{Accounts: []Account{{Name: "p", Dir: a}}, Limit: 2} }
	p, err := New(src, Plain{}, Services{}).RenderPath(context.Background(), "p/inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lines) != 5 || p.Keys[3] != "p/inbox/4.html" || p.Keys[4] != "p/inbox/3.multi" {
		t.Fatalf("lines = %d keys = %v", len(p.Lines), p.Keys)
	}
}

func TestComposeReplyForwardSend(t *testing.T) {
	var sent []Outgoing
	var synced []string
	svc := Services{
		Send:    func(_ context.Context, a Account, out Outgoing) error { sent = append(sent, out); return nil },
		SyncNow: func(name string) { synced = append(synced, name) },
	}
	m := New(two(t, ""), Plain{}, svc)
	ctx := context.Background()
	if _, err := m.Render(ctx); err != nil {
		t.Fatal(err)
	}
	// A fresh compose page from the personal inbox
	r, _ := m.Act(ctx, view.Action{Name: "compose", Page: "mail/personal/inbox"})
	if r.Kind != view.KindPage || !r.Page.Editable || r.Page.Name != "mail/personal/compose/1" || r.Page.Lines[1] != "From: me@example.com" {
		t.Fatalf("compose = %+v", r)
	}
	// A reply carries the address, the subject, the quote, and the thread
	r, _ = m.Act(ctx, view.Action{Name: "reply", Key: "personal/inbox/1.plain", Page: "mail/all/inbox"})
	if r.Kind != view.KindPage || r.Page.Lines[2] != "To: Ada <ada@example.com>" || r.Page.Lines[4] != "Subject: Re: hello there" {
		t.Fatalf("reply = %v", r.Page.Lines[:6])
	}
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "> first line") {
		t.Fatal("reply has no quote")
	}
	replyPage := r.Page
	// A forward has an empty To and the original inside
	r, _ = m.Act(ctx, view.Action{Name: "forward", Key: "personal/inbox/1.plain", Page: "mail/personal/inbox/1.plain"})
	if r.Page.Lines[2] != "To: " || r.Page.Lines[4] != "Subject: Fwd: hello there" || !strings.Contains(strings.Join(r.Page.Lines, "\n"), "Forwarded message") {
		t.Fatalf("forward = %v", r.Page.Lines[:6])
	}
	// Sending the reply parses the buffer, sends, closes the page, and asks for a sync
	body := strings.Join(replyPage.Lines, "\n") + "\nthanks\n"
	r, _ = m.Act(ctx, view.Action{Name: "send", Key: replyPage.Key, Page: replyPage.Name, Body: body})
	if r.Kind != view.KindNotify || r.Error || r.Close != replyPage.Name {
		t.Fatalf("send = %+v", r)
	}
	if len(sent) != 1 || sent[0].From != "me@example.com" || sent[0].Recipients[0] != "ada@example.com" || sent[0].Subject != "Re: hello there" {
		t.Fatalf("sent = %+v", sent)
	}
	raw := string(sent[0].Raw)
	if !strings.Contains(raw, "In-Reply-To: <1@example.com>") || !strings.Contains(raw, "\r\n\r\n") || !strings.Contains(raw, "thanks") {
		t.Fatalf("raw = %q", raw)
	}
	if len(synced) != 1 || synced[0] != "personal" {
		t.Fatalf("synced = %v", synced)
	}
	// Sending again is refused since the draft is gone
	r, _ = m.Act(ctx, view.Action{Name: "send", Key: replyPage.Key, Page: replyPage.Name, Body: body})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("second send = %+v", r)
	}
}

func TestSendWithoutService(t *testing.T) {
	m := New(two(t, ""), Plain{}, Services{})
	ctx := context.Background()
	r, _ := m.Act(ctx, view.Action{Name: "compose", Page: "mail/personal/inbox"})
	r, _ = m.Act(ctx, view.Action{Name: "send", Key: r.Page.Key, Page: r.Page.Name, Body: strings.Join(r.Page.Lines, "\n")})
	if r.Kind != view.KindNotify || !r.Error || !strings.Contains(r.Text, "not available") {
		t.Fatalf("send = %+v", r)
	}
}

func TestTrash(t *testing.T) {
	var trashed []string
	svc := Services{Trash: func(_ context.Context, a Account, m Message) error {
		trashed = append(trashed, a.Name+":"+m.ID)
		return nil
	}}
	m := New(two(t, ""), Plain{}, svc)
	ctx := context.Background()
	if _, err := m.Render(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ := m.Act(ctx, view.Action{Name: "trash", Key: "personal/inbox/1.plain", Page: "mail/personal/inbox/1.plain"})
	if r.Kind != view.KindPage || r.Page.Name != "mail/personal/inbox" || r.Close != "mail/personal/inbox/1.plain" {
		t.Fatalf("trash = %+v", r)
	}
	if len(trashed) != 1 || trashed[0] != "personal:1.plain" {
		t.Fatalf("trashed = %v", trashed)
	}
}

func TestSpamReview(t *testing.T) {
	var trashed []string
	svc := Services{
		Spam: func(_ context.Context, a Account, msgs []Message) (map[string]float64, error) {
			out := map[string]float64{}
			for _, m := range msgs {
				switch m.ID {
				case "2.encoded":
					out[m.Key()] = 0.93
				case "4.html":
					out[m.Key()] = 0.61
				default:
					out[m.Key()] = 0.05
				}
			}
			return out, nil
		},
		Trash: func(_ context.Context, a Account, m Message) error { trashed = append(trashed, m.ID); return nil },
	}
	m := New(two(t, ""), Plain{}, svc)
	ctx := context.Background()
	// The review page lists the two likely ones, most likely first
	p, err := m.RenderPath(ctx, "personal/spam")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lines) != 5 || !strings.HasPrefix(p.Lines[3], "  0.93") || !strings.HasPrefix(p.Lines[4], "  0.61") || p.Keys[3] != "personal/inbox/2.encoded" {
		t.Fatalf("spam page = %v keys = %v", p.Lines, p.Keys)
	}
	// Marking flips the column and the count
	r, _ := m.Act(ctx, view.Action{Name: "mark", Key: "personal/inbox/2.encoded", Page: "mail/personal/spam"})
	if !strings.HasPrefix(r.Page.Lines[3], "* 0.93") || !strings.Contains(r.Page.Lines[0], "1 marked") {
		t.Fatalf("marked = %v", r.Page.Lines[:4])
	}
	// Purge trashes only the marked one
	r, _ = m.Act(ctx, view.Action{Name: "purge", Page: "mail/personal/spam"})
	if len(trashed) != 1 || trashed[0] != "2.encoded" || !strings.Contains(r.Text, "trashed 1") {
		t.Fatalf("purge = %v %+v", trashed, r)
	}
}

func TestSpamWithoutJudge(t *testing.T) {
	m := New(two(t, ""), Plain{}, Services{})
	p, _ := m.RenderPath(context.Background(), "personal/spam")
	if !strings.Contains(p.Lines[0], "judge") {
		t.Fatalf("page = %v", p.Lines)
	}
}
