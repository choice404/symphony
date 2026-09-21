// Package gcal talks to the Google Calendar API over plain http with an authenticated client
package gcal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Base is the API root, a var so a test can point it at a fake
var Base = "https://www.googleapis.com/calendar/v3"

// Primary is the calendar id of the account's own calendar
const Primary = "primary"

// Calendar is one calendar the account can see
type Calendar struct {
	// The calendar id
	ID string `json:"id"`
	// The name
	Summary string `json:"summary"`
	// Whether it is the account's own
	Primary bool `json:"primary"`
}

// when is a start or end as the API spells it, a date for an all day event or a dateTime otherwise
type when struct {
	// The day for an all day event, 2006-01-02
	Date string `json:"date,omitempty"`
	// The instant otherwise, RFC 3339
	DateTime string `json:"dateTime,omitempty"`
	// The zone name
	TimeZone string `json:"timeZone,omitempty"`
}

// Event is one event as the API sends it, only the fields the app reads
type Event struct {
	// The event id
	ID string `json:"id,omitempty"`
	// The title
	Summary string `json:"summary,omitempty"`
	// Where
	Location string `json:"location,omitempty"`
	// The notes
	Description string `json:"description,omitempty"`
	// The start
	Start when `json:"start"`
	// The end
	End when `json:"end"`
	// confirmed, tentative, or cancelled
	Status string `json:"status,omitempty"`
	// The link to the web page
	HTMLLink string `json:"htmlLink,omitempty"`
	// Who is invited
	Attendees []Attendee `json:"attendees,omitempty"`
	// The calendar it came from, filled by the client and kept in the cache, never sent to the API
	Calendar string `json:"calendar,omitempty"`
}

// Attendee is one invitee
type Attendee struct {
	// The address
	Email string `json:"email"`
	// accepted, declined, tentative, or needsAction
	ResponseStatus string `json:"responseStatus,omitempty"`
	// Whether this is the account itself
	Self bool `json:"self,omitempty"`
}

/**
 * AllDay
 * Reports whether the event spans whole days
 * @return bool
 **/
func (e Event) AllDay() bool {
	return e.Start.Date != ""
}

/**
 * StartTime
 * Returns the start as a time in the local zone, midnight for an all day event
 * @return time.Time
 **/
func (e Event) StartTime() time.Time {
	return parseWhen(e.Start)
}

/**
 * EndTime
 * Returns the end as a time in the local zone
 * @return time.Time
 **/
func (e Event) EndTime() time.Time {
	return parseWhen(e.End)
}

/**
 * parseWhen
 * Turns a start or end into a local time, zero when unreadable
 * @param w {when} - the start or end
 * @return time.Time
 **/
func parseWhen(w when) time.Time {
	// An all day event is a date in the local zone
	if w.Date != "" {
		t, err := time.ParseInLocation("2006-01-02", w.Date, time.Local)
		if err != nil {
			return time.Time{}
		}
		return t
	}
	// Otherwise an instant
	t, err := time.Parse(time.RFC3339, w.DateTime)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

/**
 * NewEvent
 * Builds an event to insert from local times, all day when both fall on midnight and the flag says so
 * @param summary {string} - the title
 * @param start {time.Time} - the start
 * @param end {time.Time} - the end
 * @param allDay {bool} - whether the event spans whole days
 * @return Event
 **/
func NewEvent(summary string, start, end time.Time, allDay bool) Event {
	// An all day event carries dates, the end exclusive
	if allDay {
		return Event{Summary: summary, Start: when{Date: start.Format("2006-01-02")}, End: when{Date: end.Format("2006-01-02")}}
	}
	return Event{Summary: summary, Start: when{DateTime: start.Format(time.RFC3339)}, End: when{DateTime: end.Format(time.RFC3339)}}
}

// Client is one account's calendar access
type Client struct {
	// The authenticated http client
	http *http.Client
}

/**
 * New
 * Builds a client over an authenticated http client
 * @param h {*http.Client} - a client that signs requests
 * @return *Client
 **/
func New(h *http.Client) *Client {
	return &Client{http: h}
}

/**
 * Calendars
 * Lists the calendars the account can see
 * @param ctx {context.Context} - the context
 * @return []Calendar, error
 **/
func (c *Client) Calendars(ctx context.Context) ([]Calendar, error) {
	// The list
	var out struct {
		Items []Calendar `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/users/me/calendarList?minAccessRole=reader", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

/**
 * Events
 * Lists the events of a calendar between two instants, recurring ones expanded, in start order
 * @param ctx {context.Context} - the context
 * @param calendar {string} - the calendar id
 * @param from {time.Time} - the window start
 * @param to {time.Time} - the window end
 * @return []Event, error
 **/
func (c *Client) Events(ctx context.Context, calendar string, from, to time.Time) ([]Event, error) {
	// Page through the list
	all := make([]Event, 0, 64)
	token := ""
	for {
		q := url.Values{}
		q.Set("timeMin", from.UTC().Format(time.RFC3339))
		q.Set("timeMax", to.UTC().Format(time.RFC3339))
		q.Set("singleEvents", "true")
		q.Set("orderBy", "startTime")
		q.Set("maxResults", "250")
		if token != "" {
			q.Set("pageToken", token)
		}
		var out struct {
			Items         []Event `json:"items"`
			NextPageToken string  `json:"nextPageToken"`
		}
		path := "/calendars/" + url.PathEscape(calendar) + "/events?" + q.Encode()
		if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}
		for _, e := range out.Items {
			e.Calendar = calendar
			all = append(all, e)
		}
		if out.NextPageToken == "" {
			return all, nil
		}
		token = out.NextPageToken
	}
}

/**
 * Insert
 * Creates an event on a calendar and returns it with its id
 * @param ctx {context.Context} - the context
 * @param calendar {string} - the calendar id
 * @param e {Event} - the event
 * @return Event, error
 **/
func (c *Client) Insert(ctx context.Context, calendar string, e Event) (Event, error) {
	// Post it without the client side calendar field
	e.Calendar = ""
	var out Event
	path := "/calendars/" + url.PathEscape(calendar) + "/events"
	if err := c.do(ctx, http.MethodPost, path, e, &out); err != nil {
		return Event{}, err
	}
	out.Calendar = calendar
	return out, nil
}

/**
 * Delete
 * Removes an event from a calendar
 * @param ctx {context.Context} - the context
 * @param calendar {string} - the calendar id
 * @param id {string} - the event id
 * @return error
 **/
func (c *Client) Delete(ctx context.Context, calendar, id string) error {
	path := "/calendars/" + url.PathEscape(calendar) + "/events/" + url.PathEscape(id)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

/**
 * do
 * Sends one request with a JSON body and decodes a JSON reply
 * @param ctx {context.Context} - the context
 * @param method {string} - the http method
 * @param path {string} - the path under the base
 * @param body {interface{}} - the body to encode, nil for none
 * @param out {interface{}} - where the reply lands, nil to drop it
 * @return error
 **/
func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	// The body
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(data)
	}
	// The request
	req, err := http.NewRequestWithContext(ctx, method, Base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Send
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calendar: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	// Anything but success is the API's own message
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("calendar: %s: %s", resp.Status, apiMessage(data))
	}
	// Decode when asked
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("calendar: %w", err)
		}
	}
	return nil
}

/**
 * apiMessage
 * Pulls the message out of an API error body, or the body itself
 * @param data {[]byte} - the body
 * @return string
 **/
func apiMessage(data []byte) string {
	// The error shape
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return strings.TrimSpace(string(data))
}
