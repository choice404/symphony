package view

import (
	"context"
	"strings"
	"testing"
)

// fake is a view that records the last action
type fake struct {
	name string
	last string
}

func (f *fake) Name() string { return f.name }

func (f *fake) Render(context.Context) (Page, error) {
	return Page{Name: f.name, Lines: []string{"a", "b"}, Keys: []string{"ka"}}, nil
}

func (f *fake) Act(_ context.Context, action, key string) (Response, error) {
	f.last = action + ":" + key
	return Notify("did " + action), nil
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	if _, err := NewRegistry(&fake{name: "x"}, &fake{name: "x"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegistryNamesSorted(t *testing.T) {
	r, err := NewRegistry(&fake{name: "zeta"}, &fake{name: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Names(), ","); got != "alpha,zeta" {
		t.Fatalf("names = %s", got)
	}
}

func TestRegistryRenderAndAct(t *testing.T) {
	f := &fake{name: "x"}
	r, _ := NewRegistry(f)
	p, err := r.Render(context.Background(), "x")
	if err != nil || len(p.Lines) != 2 {
		t.Fatalf("render = %+v %v", p, err)
	}
	resp, err := r.Act(context.Background(), "x", "open", "ka")
	if err != nil || resp.Kind != KindNotify || resp.Text != "did open" {
		t.Fatalf("act = %+v %v", resp, err)
	}
	if f.last != "open:ka" {
		t.Fatalf("last = %s", f.last)
	}
	if _, err := r.Render(context.Background(), "nope"); err == nil {
		t.Fatal("expected unknown view error")
	}
}

func TestPageToMapPadsKeys(t *testing.T) {
	p := Page{Name: "x", Lines: []string{"a", "b"}, Keys: []string{"ka"}}
	m := p.ToMap()
	keys := m["keys"].([]string)
	if len(keys) != 2 || keys[0] != "ka" || keys[1] != "" {
		t.Fatalf("keys = %v", keys)
	}
	// The map does not share the page's slice
	keys[0] = "changed"
	if p.Keys[0] != "ka" {
		t.Fatal("ToMap shared the keys slice")
	}
}

func TestResponseToMap(t *testing.T) {
	m := Fail("bad").ToMap()
	if m["kind"] != "notify" || m["text"] != "bad" || m["error"] != true {
		t.Fatalf("fail map = %v", m)
	}
	m = Show(Page{Name: "p"}).ToMap()
	if m["kind"] != "page" || m["page"].(map[string]interface{})["name"] != "p" {
		t.Fatalf("show map = %v", m)
	}
}

func TestHomeListsAndOpens(t *testing.T) {
	f := &fake{name: "mail"}
	// The registry is assigned after the home view so the opener closes over it
	var r Registry
	open := func(ctx context.Context, name string) (Page, error) { return r.Render(ctx, name) }
	home := NewHome(open, Entry{Name: "mail", Label: "Mail", Summary: func(context.Context) string { return "3 unread" }})
	var err error
	r, err = NewRegistry(f, home)
	if err != nil {
		t.Fatal(err)
	}
	p, err := home.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The entry line carries the label, the summary, and the view name as its key
	if !strings.Contains(p.Lines[2], "Mail") || !strings.Contains(p.Lines[2], "3 unread") || p.Keys[2] != "mail" {
		t.Fatalf("entry line = %q key = %q", p.Lines[2], p.Keys[2])
	}
	if p.Cursor != 2 {
		t.Fatalf("cursor = %d", p.Cursor)
	}
	// Opening the entry renders the mail view
	resp, err := home.Act(context.Background(), "open", "mail")
	if err != nil || resp.Kind != KindPage || resp.Page.Name != "mail" {
		t.Fatalf("open = %+v %v", resp, err)
	}
	// Opening a header line does nothing
	resp, _ = home.Act(context.Background(), "open", "")
	if resp.Kind != KindNone {
		t.Fatalf("header open = %+v", resp)
	}
	// An unknown view is a message not a crash
	resp, _ = home.Act(context.Background(), "open", "ghost")
	if resp.Kind != KindNotify || !resp.Error {
		t.Fatalf("ghost open = %+v", resp)
	}
}
