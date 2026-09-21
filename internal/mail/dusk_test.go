//go:build geas && dusk

package mail

import (
	"context"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/geas"
)

// duskRuntime is the mail runtime with the dusk body bound to classify
func duskRuntime(t *testing.T) *geas.Runtime {
	t.Helper()
	rt := mailRuntime(t)
	if err := BindDusk(rt); err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestDuskClassifyLabels(t *testing.T) {
	rt := duskRuntime(t)
	inst, err := rt.Sign(ContractName, map[string]geas.Value{"maildir": "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ sender, subject, want string }{
		{"Ada <ada@example.com>", "lunch tomorrow", "personal"},
		{"noreply@github.com", "your build passed", "notice"},
		{"News <news@list.example.com>", "Weekly Newsletter, unsubscribe below", "list"},
		{"billing@shop.example.com", "Your invoice is ready", "money"},
		{"boss@example.com", "URGENT: action required", "urgent"},
	}
	for _, c := range cases {
		v, err := inst.Fulfill("classify", c.sender, c.subject)
		if err != nil {
			t.Fatalf("%s: %v", c.subject, err)
		}
		r, ok := v.(geas.Result)
		if !ok || !r.Ok || r.Value != c.want {
			t.Fatalf("classify(%q, %q) = %#v want %q", c.sender, c.subject, v, c.want)
		}
	}
}

func TestDuskLabelsInInbox(t *testing.T) {
	rt := duskRuntime(t)
	m := New(Fixed(fixture(t)), NewContract(rt, true), Services{})
	p, err := m.RenderPath(context.Background(), "mail/inbox")
	if err != nil {
		t.Fatal(err)
	}
	// Every message line carries a label column from the plugin
	for _, line := range p.Lines[3:] {
		if !strings.Contains(line, "personal") {
			t.Fatalf("line without label: %q", line)
		}
	}
}
