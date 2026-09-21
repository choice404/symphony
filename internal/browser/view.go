package browser

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the browser view
const ViewName = "browser"

// tabsHint is the second line of the tabs page
const tabsHint = "  <CR> open tab  o url  / search  x close  r refresh  q back"

// pageHint is the second line of a tab page
const pageHint = "  <CR> follow or fill  f link number  gs submit  b back  F forward  r reload  o url  / search  x close  q tabs"

// searchURL is where a search goes, the html version renders as text
const searchURL = "https://html.duckduckgo.com/html/?q="

// Browser is the browser app over an engine
type Browser struct {
	// The engine
	engine *Engine
}

/**
 * NewView
 * Builds the browser view
 * @param engine {*Engine} - the engine
 * @return *Browser
 **/
func NewView(engine *Engine) *Browser {
	return &Browser{engine: engine}
}

/**
 * Name
 * Returns browser
 * @return string
 **/
func (b *Browser) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the one home entry
 * @return []view.Entry
 **/
func (b *Browser) Entries() []view.Entry {
	return []view.Entry{{Name: ViewName, Label: "browser", Summary: func(context.Context) string {
		if b.engine.Exec() == "" {
			return "no chromium found"
		}
		n := len(b.engine.Tabs())
		if n == 0 {
			return "no tabs"
		}
		return fmt.Sprintf("%d tabs", n)
	}}}
}

/**
 * Render
 * Lists the open tabs
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (b *Browser) Render(ctx context.Context) (view.Page, error) {
	tabs := b.engine.Tabs()
	lines := []string{fmt.Sprintf("browser  %d tabs", len(tabs)), tabsHint, ""}
	keys := []string{"", "", ""}
	for _, t := range tabs {
		u, title := t.Location()
		if title == "" {
			title = u
		}
		lines = append(lines, fmt.Sprintf("  %d  %s  %s", t.ID, title, u))
		keys = append(keys, "tab:"+strconv.Itoa(t.ID))
	}
	if len(tabs) == 0 {
		lines = append(lines, "  no tabs, press o for a url or / to search")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName, Title: "browser", Lines: lines, Keys: keys, Cursor: 3, Filetype: "browser"}, nil
}

/**
 * RenderPath
 * Renders a tab page, tab/<id>
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below browser
 * @return view.Page, error
 **/
func (b *Browser) RenderPath(ctx context.Context, path string) (view.Page, error) {
	if !strings.HasPrefix(path, "tab/") {
		return view.Page{}, fmt.Errorf("browser: no page %q", path)
	}
	id, err := strconv.Atoi(strings.TrimPrefix(path, "tab/"))
	if err != nil {
		return view.Page{}, fmt.Errorf("browser: bad tab %q", path)
	}
	t, ok := b.engine.Tab(id)
	if !ok {
		return view.Page{}, fmt.Errorf("browser: tab %d is closed", id)
	}
	return b.tabPage(t)
}

/**
 * tabPage
 * Reads a tab and builds its page
 * @param t {*Tab} - the tab
 * @return view.Page, error
 **/
func (b *Browser) tabPage(t *Tab) (view.Page, error) {
	d, err := t.Extract()
	if err != nil {
		return view.Page{}, err
	}
	title := d.Title
	if title == "" {
		title = d.URL
	}
	lines := []string{title + "  " + d.URL, pageHint, ""}
	keys := []string{"", "", ""}
	for _, l := range d.Render() {
		lines = append(lines, l.Text)
		keys = append(keys, l.Key)
	}
	return view.Page{Name: ViewName + "/tab/" + strconv.Itoa(t.ID), Title: title, Lines: lines, Keys: keys, Key: "tab:" + strconv.Itoa(t.ID), Cursor: 3, Filetype: "browsertab"}, nil
}

/**
 * Act
 * Runs an action from a browser page
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (b *Browser) Act(ctx context.Context, a view.Action) (view.Response, error) {
	_, path := view.Split(a.Page)
	tab, _ := b.tabOf(path)
	switch a.Name {
	case "refresh":
		if tab != nil {
			return b.show(b.tabPage(tab))
		}
		return b.show(b.Render(ctx))
	case "open":
		return b.open(ctx, tab, a)
	case "url":
		return b.navigate(ctx, tab, normalize(a.Body))
	case "search":
		if strings.TrimSpace(a.Body) == "" {
			return view.Response{Kind: view.KindNone}, nil
		}
		return b.navigate(ctx, tab, searchURL+url.QueryEscape(strings.TrimSpace(a.Body)))
	case "follow":
		if tab == nil {
			return view.Fail("browser: follow works on a tab page"), nil
		}
		return b.follow(tab, strings.TrimSpace(a.Body))
	case "fill":
		if tab == nil {
			return view.Fail("browser: fill works on a tab page"), nil
		}
		return b.fill(tab, a.Key, a.Body)
	case "submit":
		if tab == nil {
			return view.Fail("browser: submit works on a tab page"), nil
		}
		return b.submit(tab, a.Key)
	case "back", "forward", "reload":
		if tab == nil {
			return view.Fail("browser: " + a.Name + " works on a tab page"), nil
		}
		var err error
		switch a.Name {
		case "back":
			err = tab.Back()
		case "forward":
			err = tab.Forward()
		default:
			err = tab.Reload()
		}
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return b.show(b.tabPage(tab))
	case "close":
		return b.closeTab(ctx, tab, a)
	}
	return view.Fail("browser: unknown action " + a.Name), nil
}

/**
 * tabOf
 * Finds the tab a page path names
 * @param path {string} - the path below browser
 * @return *Tab, int
 **/
func (b *Browser) tabOf(path string) (*Tab, int) {
	if !strings.HasPrefix(path, "tab/") {
		return nil, 0
	}
	id, err := strconv.Atoi(strings.TrimPrefix(path, "tab/"))
	if err != nil {
		return nil, 0
	}
	t, ok := b.engine.Tab(id)
	if !ok {
		return nil, id
	}
	return t, id
}

/**
 * show
 * Turns a page or its error into a response
 * @param p {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (b *Browser) show(p view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(p), nil
}

/**
 * open
 * Enter on a tab line opens it, on a link line follows it, on a field line fills it with the body the prompt gave
 * @param ctx {context.Context} - the context
 * @param tab {*Tab} - the tab of the page, nil on the tabs page
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (b *Browser) open(ctx context.Context, tab *Tab, a view.Action) (view.Response, error) {
	kind, value, _ := strings.Cut(a.Key, ":")
	switch kind {
	case "tab":
		id, _ := strconv.Atoi(value)
		t, ok := b.engine.Tab(id)
		if !ok {
			return view.Fail("browser: that tab is closed"), nil
		}
		return b.show(b.tabPage(t))
	case "link":
		if tab == nil {
			return view.Response{Kind: view.KindNone}, nil
		}
		return b.follow(tab, value)
	case "field":
		if tab == nil {
			return view.Response{Kind: view.KindNone}, nil
		}
		return b.fill(tab, a.Key, a.Body)
	}
	return view.Response{Kind: view.KindNone}, nil
}

/**
 * navigate
 * Loads a url in the tab, or in a new tab from the tabs page
 * @param ctx {context.Context} - the context
 * @param tab {*Tab} - the tab, nil for a new one
 * @param target {string} - the url
 * @return view.Response, error
 **/
func (b *Browser) navigate(ctx context.Context, tab *Tab, target string) (view.Response, error) {
	if target == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	if tab == nil {
		t, err := b.engine.Open(target)
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return b.show(b.tabPage(t))
	}
	if err := tab.Navigate(target); err != nil {
		return view.Fail(err.Error()), nil
	}
	return b.show(b.tabPage(tab))
}

/**
 * follow
 * Follows a numbered link in the tab
 * @param tab {*Tab} - the tab
 * @param number {string} - the link number
 * @return view.Response, error
 **/
func (b *Browser) follow(tab *Tab, number string) (view.Response, error) {
	n, err := strconv.Atoi(number)
	if err != nil {
		return view.Fail("browser: a link is a number"), nil
	}
	d, err := tab.Extract()
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	l, ok := d.Link(n)
	if !ok {
		return view.Fail(fmt.Sprintf("browser: no link %d", n)), nil
	}
	if err := tab.Navigate(l.Href); err != nil {
		return view.Fail(err.Error()), nil
	}
	return b.show(b.tabPage(tab))
}

/**
 * fill
 * Types into a field, clicks a button, or toggles a box, then reads the page again
 * @param tab {*Tab} - the tab
 * @param key {string} - field:n
 * @param text {string} - what to type
 * @return view.Response, error
 **/
func (b *Browser) fill(tab *Tab, key, text string) (view.Response, error) {
	_, value, ok := strings.Cut(key, ":")
	if !ok {
		return view.Fail("browser: nothing to fill under the cursor"), nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return view.Fail("browser: bad field"), nil
	}
	d, err := tab.Extract()
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	f, ok := d.Field(n)
	if !ok {
		return view.Fail(fmt.Sprintf("browser: no field %d", n)), nil
	}
	switch f.Type {
	case "button", "submit", "checkbox", "radio":
		err = tab.Click(FieldSelector(n))
	default:
		err = tab.Type(FieldSelector(n), text)
	}
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	tab.Settle(500 * time.Millisecond)
	return b.show(b.tabPage(tab))
}

/**
 * submit
 * Presses Enter in a field, the one under the cursor or the last one focused
 * @param tab {*Tab} - the tab
 * @param key {string} - field:n or anything else
 * @return view.Response, error
 **/
func (b *Browser) submit(tab *Tab, key string) (view.Response, error) {
	selector := ":focus"
	if _, value, ok := strings.Cut(key, ":"); ok && strings.HasPrefix(key, "field:") {
		if n, err := strconv.Atoi(value); err == nil {
			selector = FieldSelector(n)
		}
	}
	if err := tab.Press(selector, "\r"); err != nil {
		return view.Fail(err.Error()), nil
	}
	tab.Settle(time.Second)
	return b.show(b.tabPage(tab))
}

/**
 * closeTab
 * Closes the tab of the page, or the tab under the cursor on the tabs page, and shows the tabs
 * @param ctx {context.Context} - the context
 * @param tab {*Tab} - the tab of the page, nil on the tabs page
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (b *Browser) closeTab(ctx context.Context, tab *Tab, a view.Action) (view.Response, error) {
	id := 0
	if tab != nil {
		id = tab.ID
	} else if _, value, ok := strings.Cut(a.Key, ":"); ok {
		id, _ = strconv.Atoi(value)
	}
	if id == 0 {
		return view.Response{Kind: view.KindNone}, nil
	}
	b.engine.Close(id)
	resp, err := b.show(b.Render(ctx))
	if err == nil && resp.Kind == view.KindPage {
		resp.Close = ViewName + "/tab/" + strconv.Itoa(id)
	}
	return resp, err
}

/**
 * normalize
 * Turns what was typed into a url, a bare host gets https and a phrase becomes a search
 * @param s {string} - the input
 * @return string
 **/
func normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		return s
	}
	if strings.Contains(s, " ") || !strings.Contains(s, ".") {
		return searchURL + url.QueryEscape(s)
	}
	return "https://" + s
}
