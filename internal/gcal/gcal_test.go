package gcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fake serves the API from a handler and points Base at it
func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	old := Base
	Base = srv.URL
	t.Cleanup(func() { Base = old })
	return New(srv.Client())
}

func TestEventsPagesAndTags(t *testing.T) {
	calls := 0
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("singleEvents") != "true" || q.Get("orderBy") != "startTime" || q.Get("timeMin") == "" {
			t.Errorf("query = %v", q)
		}
		if !strings.HasPrefix(r.URL.Path, "/calendars/primary/events") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if q.Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"items":[{"id":"a","summary":"Standup","start":{"dateTime":"2026-01-02T09:00:00-08:00"},"end":{"dateTime":"2026-01-02T09:30:00-08:00"}}],"nextPageToken":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"b","summary":"Holiday","start":{"date":"2026-01-03"},"end":{"date":"2026-01-04"}}]}`))
	})
	got, err := c.Events(context.Background(), Primary, time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" || got[1].Calendar != Primary {
		t.Fatalf("events = %+v calls = %d", got, calls)
	}
	if got[0].AllDay() || !got[1].AllDay() {
		t.Fatal("all day flags wrong")
	}
	if got[0].StartTime().IsZero() || got[1].StartTime().Day() != 3 {
		t.Fatalf("times = %v %v", got[0].StartTime(), got[1].StartTime())
	}
}

func TestInsertAndDelete(t *testing.T) {
	var posted Event
	deleted := ""
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&posted)
			posted.ID = "new1"
			_ = json.NewEncoder(w).Encode(posted)
		case http.MethodDelete:
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}
	})
	start := time.Date(2026, 1, 2, 9, 0, 0, 0, time.Local)
	e, err := c.Insert(context.Background(), Primary, NewEvent("Dentist", start, start.Add(time.Hour), false))
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "new1" || posted.Summary != "Dentist" || posted.Start.DateTime == "" || posted.Start.Date != "" {
		t.Fatalf("insert = %+v posted = %+v", e, posted)
	}
	if err := c.Delete(context.Background(), Primary, "new1"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/calendars/primary/events/new1" {
		t.Fatalf("deleted = %s", deleted)
	}
}

func TestErrorMessage(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"Insufficient Permission"}}`))
	})
	_, err := c.Calendars(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Insufficient Permission") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewEventAllDay(t *testing.T) {
	d := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)
	e := NewEvent("Trip", d, d.AddDate(0, 0, 2), true)
	if e.Start.Date != "2026-01-02" || e.End.Date != "2026-01-04" || e.Start.DateTime != "" {
		t.Fatalf("event = %+v", e)
	}
}
