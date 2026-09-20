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

func TestContractListsAndOpens(t *testing.T) {
	rt := mailRuntime(t)
	c := NewContract(rt, fixture(t))
	ctx := context.Background()
	// The summary carries the runtime state and the counts
	if got := c.Summary(ctx); got != "partial, 4 messages, 1 unread" {
		t.Fatalf("summary = %q", got)
	}
	p, err := c.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lines) != 6 || !strings.Contains(p.Lines[0], "[partial]") || p.Keys[5] != "1.plain" {
		t.Fatalf("page = %v keys = %v", p.Lines, p.Keys)
	}
	// Opening goes through the contract and fulfills the reading subcontract
	resp, err := c.Act(ctx, "open", "1.plain")
	if err != nil || resp.Kind != view.KindPage {
		t.Fatalf("open = %+v %v", resp, err)
	}
	if resp.Page.Lines[3] != "Subject: hello there" || resp.Page.Lines[5] != "first line" {
		t.Fatalf("message page = %v", resp.Page.Lines)
	}
	if got := c.Summary(ctx); got != "fulfilled, 4 messages, 1 unread" {
		t.Fatalf("summary after open = %q", got)
	}
}

func TestContractNotConfiguredBreaks(t *testing.T) {
	rt := mailRuntime(t)
	c := NewContract(rt, "")
	ctx := context.Background()
	// The configured pledge errs and the requirements break the contract
	got := c.Summary(ctx)
	if !strings.HasPrefix(got, "broken:") || !strings.Contains(got, "not configured") {
		t.Fatalf("summary = %q", got)
	}
	p, _ := c.Render(ctx)
	if !strings.Contains(p.Lines[0], "not configured") {
		t.Fatalf("page = %v", p.Lines)
	}
	// Opening on a broken contract is a message not a crash
	resp, _ := c.Act(ctx, "open", "x")
	if resp.Kind != view.KindNotify || !resp.Error {
		t.Fatalf("open = %+v", resp)
	}
}

func TestContractMissingMaildirBreaksAndRefreshResigns(t *testing.T) {
	rt := mailRuntime(t)
	missing := filepath.Join(t.TempDir(), "nope")
	c := NewContract(rt, missing)
	ctx := context.Background()
	// list errs with Missing and the inbox subcontract breaks the contract
	got := c.Summary(ctx)
	if !strings.HasPrefix(got, "broken:") || !strings.Contains(got, missing) {
		t.Fatalf("summary = %q", got)
	}
	// Refresh signs again, still broken since nothing changed, but through a fresh instance
	resp, _ := c.Act(ctx, "refresh", "")
	if resp.Kind != view.KindPage || !strings.Contains(resp.Page.Lines[0], "broken") {
		t.Fatalf("refresh = %+v", resp)
	}
}

func TestMessageRoundTrip(t *testing.T) {
	msgs, _ := Scan(fixture(t))
	back := decodeMessages(encodeMessages(msgs))
	if len(back) != len(msgs) {
		t.Fatalf("count = %d", len(back))
	}
	for i := range msgs {
		if back[i].ID != msgs[i].ID || back[i].From != msgs[i].From || back[i].Seen != msgs[i].Seen || !back[i].Date.Equal(msgs[i].Date.Truncate(0)) {
			t.Fatalf("message %d = %+v want %+v", i, back[i], msgs[i])
		}
	}
}
