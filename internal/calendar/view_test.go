package calendar

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/choice404/symphony/internal/gcal"
	"github.com/choice404/symphony/internal/view"
)

// stamp formats a local time the way the API sends it
func stamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// sample builds a fetch service that answers with a fixed set of events around now
func sample(t *testing.T) (Services, *[]gcal.Event, *[]string) {
	t.Helper()
	now := time.Now()
	today := dayStart(now)
	events := []gcal.Event{
		{ID: "past", Summary: "Yesterday", Start: when(stamp(today.Add(-10 * time.Hour))), End: when(stamp(today.Add(-9 * time.Hour))), Calendar: "primary"},
		{ID: "soon", Summary: "Standup", Location: "room 1", Start: when(stamp(today.Add(25 * time.Hour))), End: when(stamp(today.Add(26 * time.Hour))), Calendar: "primary"},
		{ID: "trip", Summary: "Trip", Start: gcal.NewEvent("", today.AddDate(0, 0, 3), today.AddDate(0, 0, 5), true).Start, End: gcal.NewEvent("", today.AddDate(0, 0, 3), today.AddDate(0, 0, 5), true).End, Calendar: "primary"},
	}
	var inserted []gcal.Event
	var deleted []string
	svc := Services{
		Fetch: func(_ context.Context, a Account, from, to time.Time) ([]gcal.Event, error) {
			if a.Name == "school" {
				return nil, fmt.Errorf("token expired")
			}
			// The server knows what was inserted, like the real one
			return append(append([]gcal.Event{}, events...), inserted...), nil
		},
		Insert: func(_ context.Context, a Account, e gcal.Event) (gcal.Event, error) {
			e.ID = fmt.Sprintf("new%d", len(inserted)+1)
			e.Calendar = "primary"
			inserted = append(inserted, e)
			return e, nil
		},
		Delete: func(_ context.Context, a Account, calendar, id string) error {
			deleted = append(deleted, calendar+"/"+id)
			return nil
		},
	}
	return svc, &inserted, &deleted
}

// when builds a timed start or end from an RFC 3339 string through the API's own shape
func when(rfc string) (w struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}) {
	w.DateTime = rfc
	return w
}

// two builds a view over two accounts, the second one failing to fetch
func two(t *testing.T) (*Calendar, *[]gcal.Event, *[]string) {
	t.Helper()
	svc, inserted, deleted := sample(t)
	src := func() Settings {
		return Settings{Accounts: []Account{{Name: "personal", User: "me@example.com", Online: true}, {Name: "school", User: "me@school.edu", Online: true}}, Days: 30}
	}
	return New(src, NewStore(t.TempDir()), svc), inserted, deleted
}

func TestAgendaGroupsByDayAndCursor(t *testing.T) {
	c, _, _ := two(t)
	ctx := context.Background()
	p, err := c.RenderPath(ctx, "personal/agenda")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(p.Lines, "\n")
	// Yesterday is outside the window, Standup and the trip are in it, grouped under day headers
	if strings.Contains(text, "Yesterday") || !strings.Contains(text, "Standup") || !strings.Contains(text, "all day      Trip") {
		t.Fatalf("agenda = %s", text)
	}
	if !strings.Contains(text, "@ room 1") {
		t.Fatal("location missing")
	}
	// The cursor sits on the first event and its key is the account and id
	if p.Keys[p.Cursor] != "personal/soon" {
		t.Fatalf("cursor key = %q at %d", p.Keys[p.Cursor], p.Cursor)
	}
	if !strings.HasPrefix(p.Lines[0], "personal agenda") || !strings.Contains(p.Lines[0], "2 events") {
		t.Fatalf("header = %q", p.Lines[0])
	}
}

func TestAllMergesAndTagsFailures(t *testing.T) {
	c, _, _ := two(t)
	p, err := c.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "calendar/all/agenda" || !strings.Contains(p.Lines[0], "[school token expired]") {
		t.Fatalf("header = %q", p.Lines[0])
	}
	// The account column shows on the all page
	if !strings.Contains(strings.Join(p.Lines, "\n"), "personal  Standup") {
		t.Fatalf("lines = %v", p.Lines)
	}
}

func TestEntriesAndSummary(t *testing.T) {
	c, _, _ := two(t)
	e := c.Entries()
	if len(e) != 3 || e[0].Name != "calendar/personal/agenda" || e[2].Name != "calendar" {
		t.Fatalf("entries = %+v", e)
	}
	got := e[0].Summary(context.Background())
	if !strings.HasPrefix(got, "0 today, next ") || !strings.Contains(got, "Standup") {
		t.Fatalf("summary = %q", got)
	}
	if got := e[1].Summary(context.Background()); got != "token expired" {
		t.Fatalf("school summary = %q", got)
	}
}

func TestCacheServesOffline(t *testing.T) {
	svc, _, _ := sample(t)
	store := NewStore(t.TempDir())
	src := func() Settings {
		return Settings{Accounts: []Account{{Name: "personal", Online: true}}, Days: 30}
	}
	// First render fetches and caches
	c := New(src, store, svc)
	if _, err := c.RenderPath(context.Background(), "personal/agenda"); err != nil {
		t.Fatal(err)
	}
	// A view with no network reads the same cache
	offline := New(src, store, Services{})
	p, err := offline.RenderPath(context.Background(), "personal/agenda")
	if err != nil || !strings.Contains(strings.Join(p.Lines, "\n"), "Standup") {
		t.Fatalf("offline = %v %v", p.Lines, err)
	}
	// An account never synced says so
	never := New(func() Settings { return Settings{Accounts: []Account{{Name: "ghost", Online: true}}, Days: 30} }, store, Services{})
	p, _ = never.RenderPath(context.Background(), "ghost/agenda")
	if !strings.Contains(p.Lines[0], "not synced yet") {
		t.Fatalf("never = %q", p.Lines[0])
	}
}

func TestOpenEventPage(t *testing.T) {
	c, _, _ := two(t)
	ctx := context.Background()
	if _, err := c.RenderPath(ctx, "personal/agenda"); err != nil {
		t.Fatal(err)
	}
	r, _ := c.Act(ctx, view.Action{Name: "open", Key: "personal/soon", Page: "calendar/personal/agenda"})
	if r.Kind != view.KindPage || r.Page.Name != "calendar/personal/event/soon" || r.Page.Key != "personal/soon" {
		t.Fatalf("open = %+v", r)
	}
	if r.Page.Lines[0] != "Title:    Standup" || !strings.Contains(r.Page.Lines[2], "room 1") {
		t.Fatalf("event page = %v", r.Page.Lines)
	}
	if _, err := c.RenderPath(ctx, "personal/event/soon"); err != nil {
		t.Fatal(err)
	}
}

func TestNavigationAndWeeks(t *testing.T) {
	c, _, _ := two(t)
	ctx := context.Background()
	r, _ := c.Act(ctx, view.Action{Name: "next", Page: "calendar/all/agenda"})
	if r.Page.Name != "calendar/personal/agenda" {
		t.Fatalf("next = %s", r.Page.Name)
	}
	r, _ = c.Act(ctx, view.Action{Name: "prev", Page: "calendar/all/agenda"})
	if r.Page.Name != "calendar/school/agenda" {
		t.Fatalf("prev = %s", r.Page.Name)
	}
	// A week later the standup is gone, today brings it back
	r, _ = c.Act(ctx, view.Action{Name: "later", Page: "calendar/personal/agenda"})
	if strings.Contains(strings.Join(r.Page.Lines, "\n"), "Standup") {
		t.Fatal("later still shows this week")
	}
	r, _ = c.Act(ctx, view.Action{Name: "today", Page: "calendar/personal/agenda"})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "Standup") {
		t.Fatal("today lost this week")
	}
}

func TestNewSaveAndDelete(t *testing.T) {
	c, inserted, deleted := two(t)
	ctx := context.Background()
	r, _ := c.Act(ctx, view.Action{Name: "new", Page: "calendar/personal/agenda"})
	if r.Kind != view.KindPage || !r.Page.Editable || r.Page.Name != "calendar/personal/new/1" {
		t.Fatalf("new = %+v", r)
	}
	// Fill the editor and save
	lines := r.Page.Lines
	lines[1] = "Title: Dentist"
	lines[5] = "Location: downtown"
	lines = append(lines, "bring the card")
	r, _ = c.Act(ctx, view.Action{Name: "save", Key: r.Page.Key, Page: r.Page.Name, Body: strings.Join(lines, "\n")})
	if r.Kind != view.KindPage || r.Page.Name != "calendar/personal/agenda" || r.Close != "calendar/personal/new/1" {
		t.Fatalf("save = %+v", r)
	}
	if len(*inserted) != 1 || (*inserted)[0].Summary != "Dentist" || (*inserted)[0].Location != "downtown" || (*inserted)[0].Description != "bring the card" {
		t.Fatalf("inserted = %+v", *inserted)
	}
	// The new event shows on the agenda and can be deleted from its page
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "Dentist") {
		t.Fatal("saved event not on the agenda")
	}
	r, _ = c.Act(ctx, view.Action{Name: "delete", Key: "personal/new1", Page: "calendar/personal/event/new1"})
	if r.Kind != view.KindPage || r.Close != "calendar/personal/event/new1" || len(*deleted) != 1 || (*deleted)[0] != "primary/new1" {
		t.Fatalf("delete = %+v deleted = %v", r, *deleted)
	}
	if strings.Contains(strings.Join(r.Page.Lines, "\n"), "Dentist") {
		t.Fatal("deleted event still on the agenda")
	}
}

func TestParseEdit(t *testing.T) {
	text := strings.Join([]string{editHint, "Title: Trip", "Start: 2026-03-01", "End: 2026-03-02", "All day: yes", "Location: ", editMarker, "pack light"}, "\n")
	e, err := parseEdit(text)
	if err != nil {
		t.Fatal(err)
	}
	if !e.AllDay() || e.Start.Date != "2026-03-01" || e.End.Date != "2026-03-03" || e.Description != "pack light" {
		t.Fatalf("event = %+v", e)
	}
	bad := strings.Join([]string{"Title: x", "Start: 2026-03-01 10:00", "End: 2026-03-01 09:00", "All day: no", editMarker}, "\n")
	if _, err := parseEdit(bad); err == nil || !strings.Contains(err.Error(), "after start") {
		t.Fatalf("bad = %v", err)
	}
	if _, err := parseEdit("Title: \n" + editMarker); err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("empty title = %v", err)
	}
}
