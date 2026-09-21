package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/view"
)

// site serves two pages and a form
func site(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Home</title><style>.x{display:none}</style></head><body>
<h1>Welcome</h1><p>Some <b>bold</b> text and <a href="/two">a link</a>.</p>
<div class="x">hidden text</div><script>var a=1;</script>
<form action="/hello"><label>Name</label><input name="who" placeholder="your name"><button type="submit">Go</button></form>
<img alt="a picture"><ul><li>one</li><li>two</li></ul></body></html>`))
	})
	mux.HandleFunc("/two", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Two</title></head><body><p>Second page</p><a href="/">home</a></body></html>`))
	})
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><head><title>Hello</title></head><body><p>hello " + r.URL.Query().Get("who") + "</p></body></html>"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// engine builds an engine over a scratch profile, skipping without a browser
func engine(t *testing.T) *Engine {
	t.Helper()
	exec := FindChrome()
	if exec == "" {
		t.Skip("no chromium on PATH")
	}
	e := New(exec, t.TempDir())
	t.Cleanup(e.Stop)
	return e
}

func TestRenderWrapsAndKeys(t *testing.T) {
	d := Doc{
		Items:  []item{{T: "text", V: "# "}, {T: "text", V: "Title"}, {T: "break"}, {T: "text", V: "see"}, {T: "link", V: "1"}, {T: "text", V: "here"}, {T: "break"}, {T: "break"}, {T: "field", V: "1"}, {T: "break"}},
		Links:  []Link{{N: 1, Text: "here", Href: "http://x/"}},
		Inputs: []Field{{N: 1, Type: "text", Name: "who", Placeholder: "your name"}},
	}
	lines := d.Render()
	if len(lines) != 4 || lines[0].Text != "# Title" || lines[1].Text != "see [1] here" || lines[1].Key != "link:1" {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[2].Text != "" || lines[3].Text != "{1 your name: _}" || lines[3].Key != "field:1" {
		t.Fatalf("field line = %+v", lines[3])
	}
	long := strings.Repeat("word ", 60)
	if got := wrap(long, 20); len(got) < 10 || len([]rune(got[0])) > 20 {
		t.Fatalf("wrap = %v", got)
	}
}

func TestNormalize(t *testing.T) {
	if normalize("example.com") != "https://example.com" || normalize("http://a.b/c") != "http://a.b/c" {
		t.Fatal("host or url")
	}
	if !strings.HasPrefix(normalize("go tui"), searchURL) || !strings.HasPrefix(normalize("golang"), searchURL) {
		t.Fatal("phrase should search")
	}
}

func TestOpenReadFollowFill(t *testing.T) {
	srv := site(t)
	e := engine(t)
	b := NewView(e)
	ctx := context.Background()
	// A url from the tabs page opens a tab
	r, _ := b.Act(ctx, view.Action{Name: "url", Page: "browser", Body: srv.URL})
	if r.Kind != view.KindPage || r.Page.Name != "browser/tab/1" {
		t.Fatalf("open = %+v", r)
	}
	text := strings.Join(r.Page.Lines, "\n")
	if !strings.Contains(text, "# Welcome") || !strings.Contains(text, "Some bold text and [1] a link") || strings.Contains(text, "hidden text") || strings.Contains(text, "var a") {
		t.Fatalf("page = %s", text)
	}
	if !strings.Contains(text, "{1 your name: _}") || !strings.Contains(text, "{2 Go}") || !strings.Contains(text, "[a picture]") {
		t.Fatalf("fields = %s", text)
	}
	// The link line's key follows it
	var linkKey string
	for i, l := range r.Page.Lines {
		if strings.Contains(l, "[1] a link") {
			linkKey = r.Page.Keys[i]
		}
	}
	r, _ = b.Act(ctx, view.Action{Name: "open", Key: linkKey, Page: "browser/tab/1"})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "Second page") {
		t.Fatalf("follow = %v", r.Page.Lines)
	}
	// Back returns, then a field is filled and submitted
	r, _ = b.Act(ctx, view.Action{Name: "back", Page: "browser/tab/1"})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "# Welcome") {
		t.Fatalf("back = %v", r.Page.Lines)
	}
	r, _ = b.Act(ctx, view.Action{Name: "fill", Key: "field:1", Page: "browser/tab/1", Body: "ada"})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "{1 your name: ada}") {
		t.Fatalf("fill = %v", r.Page.Lines)
	}
	r, _ = b.Act(ctx, view.Action{Name: "submit", Key: "field:1", Page: "browser/tab/1"})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "hello ada") {
		t.Fatalf("submit = %v", r.Page.Lines)
	}
	// The tabs page lists it and close drops it
	p, _ := b.Render(ctx)
	if len(p.Lines) != 4 || p.Keys[3] != "tab:1" {
		t.Fatalf("tabs = %v", p.Lines)
	}
	r, _ = b.Act(ctx, view.Action{Name: "close", Page: "browser/tab/1"})
	if r.Close != "browser/tab/1" || len(e.Tabs()) != 0 {
		t.Fatalf("close = %+v", r)
	}
}

func TestStopAndRestart(t *testing.T) {
	srv := site(t)
	e := engine(t)
	if _, err := e.Open(srv.URL); err != nil {
		t.Fatal(err)
	}
	e.Stop()
	if e.Running() || len(e.Tabs()) != 0 {
		t.Fatal("still running after stop")
	}
	tab, err := e.Named("again", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e.Named("again", ""); got != tab {
		t.Fatal("named tab not reused")
	}
}
