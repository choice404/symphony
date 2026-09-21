package view

import (
	"context"
	"strings"
	"testing"
)

// fake is a view that records the last action and serves one path
type fake struct {
	name string
	last Action
}

func (f *fake) Name() string { return f.name }

func (f *fake) Render(context.Context) (Page, error) {
	return Page{Name: f.name, Lines: []string{"a", "b"}, Keys: []string{"ka"}}, nil
}

func (f *fake) RenderPath(_ context.Context, path string) (Page, error) {
	return Page{Name: f.name + "/" + path, Lines: []string{"path " + path}}, nil
}

func (f *fake) Act(_ context.Context, a Action) (Response, error) {
	f.last = a
	return Notify("did " + a.Name), nil
}

// plain is a view with no pages below it
type plain struct{}

func (plain) Name() string                                  { return "plain" }
func (plain) Render(context.Context) (Page, error)          { return Page{Name: "plain"}, nil }
func (plain) Act(context.Context, Action) (Response, error) { return Response{}, nil }

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
	resp, err := r.Act(context.Background(), "x/some/path", Action{Name: "open", Key: "ka", Page: "x/some/path"})
	if err != nil || resp.Kind != KindNotify || resp.Text != "did open" {
		t.Fatalf("act = %+v %v", resp, err)
	}
	if f.last.Key != "ka" || f.last.Page != "x/some/path" {
		t.Fatalf("last = %+v", f.last)
	}
	if _, err := r.Render(context.Background(), "nope"); err == nil {
		t.Fatal("expected unknown view error")
	}
}

func TestRegistryRenderPath(t *testing.T) {
	r, _ := NewRegistry(&fake{name: "x"}, plain{})
	p, err := r.Render(context.Background(), "x/school/sent")
	if err != nil || p.Name != "x/school/sent" || p.Lines[0] != "path school/sent" {
		t.Fatalf("path render = %+v %v", p, err)
	}
	if _, err := r.Render(context.Background(), "plain/anything"); err == nil {
		t.Fatal("a view without pages should refuse a path")
	}
}

func TestSplit(t *testing.T) {
	if v, p := Split("mail/school/sent"); v != "mail" || p != "school/sent" {
		t.Fatalf("split = %q %q", v, p)
	}
	if v, p := Split("home"); v != "home" || p != "" {
		t.Fatalf("split = %q %q", v, p)
	}
}

func TestPageToMapPadsKeys(t *testing.T) {
	p := Page{Name: "x", Lines: []string{"a", "b"}, Keys: []string{"ka"}, Key: "pk", Editable: true}
	m := p.ToMap()
	keys := m["keys"].([]string)
	if len(keys) != 2 || keys[0] != "ka" || keys[1] != "" {
		t.Fatalf("keys = %v", keys)
	}
	if m["key"] != "pk" || m["editable"] != true {
		t.Fatalf("map = %v", m)
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
	r := Show(Page{Name: "p"})
	r.Close = "x/compose/1"
	m = r.ToMap()
	if m["kind"] != "page" || m["page"].(map[string]interface{})["name"] != "p" || m["close"] != "x/compose/1" {
		t.Fatalf("show map = %v", m)
	}
}

func TestHomeListsAndOpens(t *testing.T) {
	f := &fake{name: "mail"}
	// The registry is assigned after the home view so the opener closes over it
	var r Registry
	open := func(ctx context.Context, name string) (Page, error) { return r.Render(ctx, name) }
	entries := func() []Entry {
		return []Entry{{Name: "mail/school/inbox", Label: "mail school", Summary: func(context.Context) string { return "3 unread" }}}
	}
	home := NewHome(open, entries)
	var err error
	r, err = NewRegistry(f, home)
	if err != nil {
		t.Fatal(err)
	}
	p, err := home.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The entry line carries the label, the summary, and the page name as its key
	if !strings.Contains(p.Lines[2], "mail school") || !strings.Contains(p.Lines[2], "3 unread") || p.Keys[2] != "mail/school/inbox" {
		t.Fatalf("entry line = %q key = %q", p.Lines[2], p.Keys[2])
	}
	if p.Cursor != 2 {
		t.Fatalf("cursor = %d", p.Cursor)
	}
	// Opening the entry renders the mail page under its path
	resp, err := home.Act(context.Background(), Action{Name: "open", Key: "mail/school/inbox"})
	if err != nil || resp.Kind != KindPage || resp.Page.Name != "mail/school/inbox" {
		t.Fatalf("open = %+v %v", resp, err)
	}
	// Opening a header line does nothing
	resp, _ = home.Act(context.Background(), Action{Name: "open"})
	if resp.Kind != KindNone {
		t.Fatalf("header open = %+v", resp)
	}
	// An unknown view is a message not a crash
	resp, _ = home.Act(context.Background(), Action{Name: "open", Key: "ghost"})
	if resp.Kind != KindNotify || !resp.Error {
		t.Fatalf("ghost open = %+v", resp)
	}
}
