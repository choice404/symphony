package calendar

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/gcal"
	"github.com/choice404/symphony/internal/view"
)

// stale is how old a cache may be before a page render fetches again when it can
const stale = 10 * time.Minute

// Calendar is the calendar app, agendas per account and merged, event pages, and an editor
type Calendar struct {
	// Returns the settings as configured right now
	src Source
	// The cache
	store *Store
	// What the daemon does on the network
	svc Services
	// Guards the offsets and the editors
	mu sync.Mutex
	// The window offset in weeks per agenda page name
	offsets map[string]int
	// The open editors by number, the account each belongs to
	editors map[int]string
	// The next editor number
	next int
}

/**
 * New
 * Builds the calendar view
 * @param src {Source} - the settings, read on every render
 * @param store {*Store} - the cache
 * @param svc {Services} - what the daemon does on the network
 * @return *Calendar
 **/
func New(src Source, store *Store, svc Services) *Calendar {
	return &Calendar{src: src, store: store, svc: svc, offsets: map[string]int{}, editors: map[int]string{}, next: 1}
}

/**
 * Name
 * Returns calendar
 * @return string
 **/
func (c *Calendar) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the home page entries, one per account then one for all of them when there are several
 * @return []view.Entry
 **/
func (c *Calendar) Entries() []view.Entry {
	accs := c.src().Accounts
	out := make([]view.Entry, 0, len(accs)+1)
	for _, a := range accs {
		a := a
		out = append(out, view.Entry{
			Name:    pageName(a.Name),
			Label:   "calendar " + a.Name,
			Summary: func(ctx context.Context) string { return c.summary(ctx, a) },
		})
	}
	if len(accs) > 1 {
		out = append(out, view.Entry{Name: ViewName, Label: "calendar all", Summary: func(ctx context.Context) string { return c.summaryAll(ctx) }})
	}
	if len(accs) == 0 {
		out = append(out, view.Entry{Name: ViewName, Label: "calendar", Summary: func(context.Context) string { return "no oauth accounts" }})
	}
	return out
}

/**
 * summary
 * Returns one account's line for home, today's count and the next event
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @return string
 **/
func (c *Calendar) summary(ctx context.Context, a Account) string {
	from, to := c.window(pageName(a.Name))
	entries, err := c.events(ctx, a, from, to)
	if err != nil {
		return err.Error()
	}
	return describe(entries)
}

/**
 * summaryAll
 * Returns the merged line for home
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (c *Calendar) summaryAll(ctx context.Context) string {
	from, to := c.window(pageName(AllAccounts))
	entries, tags := c.merged(ctx, c.src().Accounts, from, to)
	return describe(entries) + notes(tags)
}

/**
 * describe
 * Formats today's count and the next upcoming event
 * @param entries {[]entry} - the events in start order
 * @return string
 **/
func describe(entries []entry) string {
	// Today's count and the next one from now
	now := time.Now()
	today := dayStart(now)
	tomorrow := today.AddDate(0, 0, 1)
	count := 0
	next := ""
	for _, e := range entries {
		start := e.Event.StartTime()
		if !start.Before(today) && start.Before(tomorrow) {
			count++
		}
		if next == "" && start.After(now) {
			when := start.Format("Mon 15:04")
			if e.Event.AllDay() {
				when = start.Format("Mon") + " all day"
			}
			next = when + " " + e.Event.Summary
		}
	}
	out := fmt.Sprintf("%d today", count)
	if next != "" {
		out += ", next " + next
	}
	return out
}

/**
 * Render
 * Shows the merged agenda
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (c *Calendar) Render(ctx context.Context) (view.Page, error) {
	return c.RenderPath(ctx, AllAccounts+"/agenda")
}

/**
 * RenderPath
 * Shows an agenda, account/agenda, or an event, account/event/id
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below calendar
 * @return view.Page, error
 **/
func (c *Calendar) RenderPath(ctx context.Context, path string) (view.Page, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return view.Page{}, fmt.Errorf("calendar: a page is account/agenda or account/event/id, not %q", path)
	}
	switch parts[1] {
	case "agenda":
		return c.agenda(ctx, parts[0])
	case "event":
		if len(parts) < 3 {
			return view.Page{}, fmt.Errorf("calendar: an event page needs an id")
		}
		return c.eventPage(ctx, parts[0]+"/"+strings.Join(parts[2:], "/"))
	case "new":
		return view.Page{}, fmt.Errorf("calendar: a new event page is opened with c")
	}
	return view.Page{}, fmt.Errorf("calendar: no page %q", path)
}

/**
 * window
 * Returns the agenda window of a page, today plus the configured days, shifted by the page's week offset
 * @param page {string} - the agenda page name
 * @return time.Time, time.Time
 **/
func (c *Calendar) window(page string) (time.Time, time.Time) {
	c.mu.Lock()
	off := c.offsets[page]
	c.mu.Unlock()
	from := dayStart(time.Now()).AddDate(0, 0, 7*off)
	return from, from.AddDate(0, 0, c.src().Days)
}

/**
 * agenda
 * Builds the agenda of an account or of every account
 * @param ctx {context.Context} - the context
 * @param account {string} - the account name or all
 * @return view.Page, error
 **/
func (c *Calendar) agenda(ctx context.Context, account string) (view.Page, error) {
	accs := c.src().Accounts
	from, to := c.window(pageName(account))
	if len(accs) == 0 {
		return agendaPage(account, from, to, nil, map[string]string{"": "no account with auth = oauth, authorize one"}, false), nil
	}
	// One or all
	if account == AllAccounts {
		entries, tags := c.merged(ctx, accs, from, to)
		return agendaPage(account, from, to, entries, tags, true), nil
	}
	a, ok := findAccount(accs, account)
	if !ok {
		return view.Page{}, fmt.Errorf("calendar: no account named %q", account)
	}
	entries, err := c.events(ctx, a, from, to)
	tags := map[string]string{}
	if err != nil {
		tags[a.Name] = err.Error()
	}
	return agendaPage(account, from, to, entries, tags, false), nil
}

/**
 * events
 * Returns an account's events in a window from the cache, fetching first when the cache is missing, stale, or short of the window
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param from {time.Time} - the window start
 * @param to {time.Time} - the window end
 * @return []entry, error
 **/
func (c *Calendar) events(ctx context.Context, a Account, from, to time.Time) ([]entry, error) {
	// The cache
	cached, err := c.store.Load(a.Name)
	if err != nil {
		return nil, err
	}
	// Fetch when the cache cannot answer and the network can
	covers := !cached.FetchedAt.IsZero() && !cached.From.After(from) && !cached.To.Before(to)
	if (!covers || time.Since(cached.FetchedAt) > stale) && a.Online && c.svc.Fetch != nil {
		fresh, ferr := c.fetch(ctx, a, from, to)
		if ferr == nil {
			cached = fresh
		} else if !covers {
			return nil, ferr
		}
	}
	if cached.FetchedAt.IsZero() {
		return nil, fmt.Errorf("not synced yet")
	}
	// The window
	out := make([]entry, 0, len(cached.Events))
	for _, e := range cached.Events {
		start := e.StartTime()
		if start.Before(from) || !start.Before(to) {
			continue
		}
		out = append(out, entry{Account: a.Name, Event: e})
	}
	return out, nil
}

/**
 * fetch
 * Pulls a window wide enough for every page from the network and saves it
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param from {time.Time} - the window start the caller needs
 * @param to {time.Time} - the window end the caller needs
 * @return Cached, error
 **/
func (c *Calendar) fetch(ctx context.Context, a Account, from, to time.Time) (Cached, error) {
	// A wider window so a week either side needs no fetch
	wfrom := from.AddDate(0, 0, -14)
	wto := to.AddDate(0, 0, 14)
	events, err := c.svc.Fetch(ctx, a, wfrom, wto)
	if err != nil {
		return Cached{}, err
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].StartTime().Before(events[j].StartTime()) })
	cached := Cached{FetchedAt: time.Now(), From: wfrom, To: wto, Events: events}
	return cached, c.store.Save(a.Name, cached)
}

/**
 * merged
 * Returns every account's events in a window newest first with a tag per account that failed
 * @param ctx {context.Context} - the context
 * @param accs {[]Account} - the accounts
 * @param from {time.Time} - the window start
 * @param to {time.Time} - the window end
 * @return []entry, map[string]string
 **/
func (c *Calendar) merged(ctx context.Context, accs []Account, from, to time.Time) ([]entry, map[string]string) {
	all := make([]entry, 0, 64)
	tags := map[string]string{}
	for _, a := range accs {
		entries, err := c.events(ctx, a, from, to)
		if err != nil {
			tags[a.Name] = err.Error()
			continue
		}
		all = append(all, entries...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Event.StartTime().Before(all[j].Event.StartTime()) })
	return all, tags
}

/**
 * lookup
 * Finds an event by key in an account's cache
 * @param key {string} - the account and event id
 * @return entry, Account, error
 **/
func (c *Calendar) lookup(key string) (entry, Account, error) {
	name, id, ok := strings.Cut(key, "/")
	if !ok {
		return entry{}, Account{}, fmt.Errorf("calendar: nothing under the cursor")
	}
	a, found := findAccount(c.src().Accounts, name)
	if !found {
		return entry{}, Account{}, fmt.Errorf("calendar: no account named %q", name)
	}
	cached, err := c.store.Load(name)
	if err != nil {
		return entry{}, Account{}, err
	}
	for _, e := range cached.Events {
		if e.ID == id {
			return entry{Account: name, Event: e}, a, nil
		}
	}
	return entry{}, Account{}, fmt.Errorf("calendar: event not in the cache, refresh")
}

/**
 * eventPage
 * Builds the page of one event by key
 * @param ctx {context.Context} - the context
 * @param key {string} - the account and event id
 * @return view.Page, error
 **/
func (c *Calendar) eventPage(ctx context.Context, key string) (view.Page, error) {
	e, a, err := c.lookup(key)
	if err != nil {
		return view.Page{}, err
	}
	return eventPage(e, a.User), nil
}

/**
 * Act
 * Runs an action from a calendar page
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (c *Calendar) Act(ctx context.Context, a view.Action) (view.Response, error) {
	// Where the action came from
	_, path := view.Split(a.Page)
	parts := strings.Split(path, "/")
	account := AllAccounts
	if len(parts) >= 1 && parts[0] != "" {
		account = parts[0]
	}
	// Dispatch
	switch a.Name {
	case "open":
		if a.Key == "" {
			return view.Response{Kind: view.KindNone}, nil
		}
		return c.show(c.eventPage(ctx, a.Key))
	case "refresh":
		return c.refresh(ctx, account)
	case "next", "prev":
		return c.show(c.agenda(ctx, c.neighbor(account, a.Name == "next")))
	case "all":
		return c.show(c.agenda(ctx, AllAccounts))
	case "later", "earlier", "today":
		return c.shift(ctx, account, a.Name)
	case "new":
		return c.newEvent(account)
	case "save":
		return c.save(ctx, a)
	case "delete":
		return c.delete(ctx, a.Key, account)
	}
	return view.Fail("calendar: unknown action " + a.Name), nil
}

/**
 * show
 * Turns a page or its error into a response
 * @param p {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (c *Calendar) show(p view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(p), nil
}

/**
 * refresh
 * Fetches every account behind an agenda now and shows it again
 * @param ctx {context.Context} - the context
 * @param account {string} - the account name or all
 * @return view.Response, error
 **/
func (c *Calendar) refresh(ctx context.Context, account string) (view.Response, error) {
	accs := c.src().Accounts
	from, to := c.window(pageName(account))
	for _, acc := range accs {
		if account != AllAccounts && acc.Name != account {
			continue
		}
		if acc.Online && c.svc.Fetch != nil {
			if _, err := c.fetch(ctx, acc, from, to); err != nil {
				return view.Fail("calendar: " + acc.Name + ": " + err.Error()), nil
			}
		}
	}
	return c.show(c.agenda(ctx, account))
}

/**
 * neighbor
 * Returns the account after or before one in the cycle all, then each account in config order
 * @param account {string} - the current account or all
 * @param forward {bool} - whether to step forward
 * @return string
 **/
func (c *Calendar) neighbor(account string, forward bool) string {
	names := []string{AllAccounts}
	for _, a := range c.src().Accounts {
		names = append(names, a.Name)
	}
	at := 0
	for i, n := range names {
		if n == account {
			at = i
		}
	}
	if forward {
		return names[(at+1)%len(names)]
	}
	return names[(at-1+len(names))%len(names)]
}

/**
 * shift
 * Moves an agenda's window a week either way or back to today and shows it again
 * @param ctx {context.Context} - the context
 * @param account {string} - the account name or all
 * @param how {string} - later, earlier, or today
 * @return view.Response, error
 **/
func (c *Calendar) shift(ctx context.Context, account, how string) (view.Response, error) {
	page := pageName(account)
	c.mu.Lock()
	next := make(map[string]int, len(c.offsets)+1)
	for k, v := range c.offsets {
		next[k] = v
	}
	switch how {
	case "later":
		next[page]++
	case "earlier":
		next[page]--
	default:
		delete(next, page)
	}
	c.offsets = next
	c.mu.Unlock()
	return c.show(c.agenda(ctx, account))
}

/**
 * newEvent
 * Opens an editor for a new event on an account, the first when opened from all
 * @param account {string} - the account name or all
 * @return view.Response, error
 **/
func (c *Calendar) newEvent(account string) (view.Response, error) {
	accs := c.src().Accounts
	if len(accs) == 0 {
		return view.Fail("calendar: no account with auth = oauth"), nil
	}
	a := accs[0]
	if account != AllAccounts {
		found, ok := findAccount(accs, account)
		if !ok {
			return view.Fail("calendar: no account named " + account), nil
		}
		a = found
	}
	if !a.Online || c.svc.Insert == nil {
		return view.Fail("calendar: " + a.Name + " is not authorized, run symphonyd authorize " + a.Name), nil
	}
	c.mu.Lock()
	n := c.next
	c.next++
	c.editors[n] = a.Name
	c.mu.Unlock()
	return view.Show(editPage(n, a, time.Now())), nil
}

/**
 * save
 * Parses an editor and creates the event, then shows the account's agenda
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the save action with the buffer text
 * @return view.Response, error
 **/
func (c *Calendar) save(ctx context.Context, a view.Action) (view.Response, error) {
	n, err := strconv.Atoi(a.Key)
	if err != nil {
		return view.Fail("calendar: save works from a new event page"), nil
	}
	c.mu.Lock()
	name, ok := c.editors[n]
	c.mu.Unlock()
	if !ok {
		return view.Fail("calendar: this editor is not open any more, press c again"), nil
	}
	acc, found := findAccount(c.src().Accounts, name)
	if !found || c.svc.Insert == nil {
		return view.Fail("calendar: " + name + " is not available"), nil
	}
	e, err := parseEdit(a.Body)
	if err != nil {
		return view.Fail("calendar: " + err.Error()), nil
	}
	created, err := c.svc.Insert(ctx, acc, e)
	if err != nil {
		return view.Fail("calendar: save failed, " + err.Error()), nil
	}
	// Put it in the cache so the agenda shows it right away
	c.addToCache(acc.Name, created)
	c.mu.Lock()
	delete(c.editors, n)
	c.mu.Unlock()
	resp, err := c.show(c.agenda(ctx, acc.Name))
	if err == nil && resp.Kind == view.KindPage {
		resp.Close = a.Page
		resp.Text = "calendar: saved " + strconv.Quote(created.Summary)
	}
	return resp, err
}

/**
 * addToCache
 * Inserts a created event into the account's cache in start order
 * @param account {string} - the account name
 * @param e {gcal.Event} - the event
 * @return void
 **/
func (c *Calendar) addToCache(account string, e gcal.Event) {
	cached, err := c.store.Load(account)
	if err != nil {
		return
	}
	cached.Events = append(cached.Events, e)
	sort.SliceStable(cached.Events, func(i, j int) bool { return cached.Events[i].StartTime().Before(cached.Events[j].StartTime()) })
	_ = c.store.Save(account, cached)
}

/**
 * delete
 * Deletes an event on the server and from the cache, then shows the agenda the action came from
 * @param ctx {context.Context} - the context
 * @param key {string} - the account and event id
 * @param account {string} - the account of the page the action came from
 * @return view.Response, error
 **/
func (c *Calendar) delete(ctx context.Context, key, account string) (view.Response, error) {
	if key == "" {
		return view.Fail("calendar: nothing under the cursor to delete"), nil
	}
	e, a, err := c.lookup(key)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	if !a.Online || c.svc.Delete == nil {
		return view.Fail("calendar: " + a.Name + " is not authorized"), nil
	}
	if err := c.svc.Delete(ctx, a, e.Event.Calendar, e.Event.ID); err != nil {
		return view.Fail("calendar: delete failed, " + err.Error()), nil
	}
	// Drop it from the cache
	cached, err := c.store.Load(a.Name)
	if err == nil {
		kept := make([]gcal.Event, 0, len(cached.Events))
		for _, ev := range cached.Events {
			if ev.ID != e.Event.ID {
				kept = append(kept, ev)
			}
		}
		cached.Events = kept
		_ = c.store.Save(a.Name, cached)
	}
	// The agenda again, an event page closed
	agendaOf := account
	if agendaOf == "event" || agendaOf == "" {
		agendaOf = a.Name
	}
	resp, err := c.show(c.agenda(ctx, agendaOf))
	if err == nil && resp.Kind == view.KindPage {
		resp.Close = ViewName + "/" + a.Name + "/event/" + e.Event.ID
		resp.Text = "calendar: deleted " + strconv.Quote(e.Event.Summary)
	}
	return resp, err
}
