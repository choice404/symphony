//go:build geas

package mail

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/geas"
	"github.com/choice404/symphony/internal/view"
)

// mailRuntime builds the mail module, loads it, and binds the host pledges, skipping without the compiler
func mailRuntime(t *testing.T) *geas.Runtime {
	t.Helper()
	if _, err := exec.LookPath("geas"); err != nil {
		t.Skip("geas not on PATH")
	}
	src, _ := filepath.Abs("../../contracts/mail.geas")
	dir := t.TempDir()
	cmd := exec.Command("geas", "build", src)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("geas build: %v\n%s", err, out)
	}
	rt, err := geas.Init()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Shutdown)
	if err := rt.Load(filepath.Join(dir, "target", "geas-out", "libmail.geas.so")); err != nil {
		t.Fatal(err)
	}
	if err := Bind(rt); err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestContractListsOpensAndStates(t *testing.T) {
	rt := mailRuntime(t)
	m := New(Fixed(fixture(t)), NewContract(rt, false), Services{})
	ctx := context.Background()
	// The summary carries the runtime state and the counts
	if got := m.Entries()[0].Summary(ctx); got != "partial, 4 messages, 1 unread" {
		t.Fatalf("summary = %q", got)
	}
	p, err := m.RenderPath(ctx, "mail/inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lines) != 7 || !strings.Contains(p.Lines[0], "[mail partial]") || p.Keys[6] != "mail/inbox/1.plain" {
		t.Fatalf("page = %v keys = %v", p.Lines, p.Keys)
	}
	// The sent folder goes through the same pledge with a folder argument
	p, _ = m.RenderPath(ctx, "mail/sent")
	if len(p.Lines) != 4 || p.Keys[3] != "mail/sent/9.sent" {
		t.Fatalf("sent = %v", p.Lines)
	}
	// Opening goes through the contract and fulfills the reading subcontract
	r, _ := m.Act(ctx, view.Action{Name: "open", Key: "mail/inbox/1.plain", Page: "mail/mail/inbox"})
	if r.Kind != view.KindPage || r.Page.Lines[3] != "Subject: hello there" || r.Page.Lines[5] != "first line" {
		t.Fatalf("open = %+v", r)
	}
	if got := m.Entries()[0].Summary(ctx); got != "fulfilled, 4 messages, 1 unread" {
		t.Fatalf("summary after open = %q", got)
	}
	// A reply through the contract has the thread headers
	r, _ = m.Act(ctx, view.Action{Name: "reply", Key: "mail/inbox/1.plain", Page: "mail/mail/inbox"})
	if r.Kind != view.KindPage || r.Page.Lines[2] != "To: Ada <ada@example.com>" {
		t.Fatalf("reply = %+v", r)
	}
}

func TestContractNotConfiguredBreaks(t *testing.T) {
	rt := mailRuntime(t)
	m := New(Fixed(""), NewContract(rt, false), Services{})
	ctx := context.Background()
	// An empty maildir is unconfigured before the contract is even asked
	if got := m.Entries()[0].Summary(ctx); got != "not configured" {
		t.Fatalf("summary = %q", got)
	}
	// The contract itself reports it when asked through the backend
	_, _, err := NewContract(rt, false).List(ctx, Account{Name: "x"}, FolderInbox)
	if err == nil || !strings.HasPrefix(err.Error(), "broken:") || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("list = %v", err)
	}
}

func TestContractMissingMaildirBreaksAndRefreshResigns(t *testing.T) {
	rt := mailRuntime(t)
	missing := filepath.Join(t.TempDir(), "nope")
	m := New(Fixed(missing), NewContract(rt, false), Services{})
	ctx := context.Background()
	// list errs with Missing and the inbox subcontract breaks the contract
	got := m.Entries()[0].Summary(ctx)
	if !strings.HasPrefix(got, "broken:") || !strings.Contains(got, missing) {
		t.Fatalf("summary = %q", got)
	}
	// Refresh signs again, still broken since nothing changed, but through a fresh instance
	r, _ := m.Act(ctx, view.Action{Name: "refresh", Page: "mail/mail/inbox"})
	if r.Kind != view.KindPage || !strings.Contains(r.Page.Lines[0], "broken") {
		t.Fatalf("refresh = %+v", r)
	}
}

func TestMessageRoundTrip(t *testing.T) {
	msgs, _ := Scan(fixture(t))
	back := decodeMessages(encodeMessages(msgs))
	if len(back) != len(msgs) {
		t.Fatalf("count = %d", len(back))
	}
	for i := range msgs {
		if back[i].ID != msgs[i].ID || back[i].From != msgs[i].From || back[i].Seen != msgs[i].Seen || back[i].MessageID != msgs[i].MessageID || !back[i].Date.Equal(msgs[i].Date.Truncate(0)) {
			t.Fatalf("message %d = %+v want %+v", i, back[i], msgs[i])
		}
	}
}
