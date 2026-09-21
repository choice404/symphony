package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/gcal"
	"github.com/choice404/symphony/internal/view"
)

// editMarker separates the fields of an event editor from the description below it
const editMarker = "--- description below this line, the lines above are fields ---"

// editHint is the first line of an event editor
const editHint = "  gs save  q back without saving"

// timeLayout is how a start or end is typed
const timeLayout = "2006-01-02 15:04"

// dateLayout is how an all day start or end is typed
const dateLayout = "2006-01-02"

/**
 * editPage
 * Builds an editable page for a new event, the next round hour for an hour
 * @param n {int} - the editor number
 * @param a {Account} - the account
 * @param now {time.Time} - the time the page was opened
 * @return view.Page
 **/
func editPage(n int, a Account, now time.Time) view.Page {
	// The next round hour
	start := now.Local().Truncate(time.Hour).Add(time.Hour)
	end := start.Add(time.Hour)
	lines := []string{
		editHint,
		"Title: ",
		"Start: " + start.Format(timeLayout),
		"End: " + end.Format(timeLayout),
		"All day: no",
		"Location: ",
		editMarker,
		"",
	}
	return view.Page{
		Name:     fmt.Sprintf("%s/%s/new/%d", ViewName, a.Name, n),
		Title:    "new event",
		Lines:    lines,
		Keys:     make([]string, len(lines)),
		Key:      fmt.Sprintf("%d", n),
		Cursor:   1,
		Filetype: "eventedit",
		Editable: true,
	}
}

/**
 * parseEdit
 * Turns the text of an event editor into an event to insert
 * @param text {string} - the buffer text
 * @return gcal.Event, error
 **/
func parseEdit(text string) (gcal.Event, error) {
	// The marker splits fields from the description
	lines := strings.Split(text, "\n")
	at := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == editMarker {
			at = i
			break
		}
	}
	if at < 0 {
		return gcal.Event{}, fmt.Errorf("the marker line is missing, the description goes below it")
	}
	description := strings.TrimSpace(strings.Join(lines[at+1:], "\n"))
	// The fields
	fields := map[string]string{}
	for _, line := range lines[:at] {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "  ") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return gcal.Event{}, fmt.Errorf("field line without a colon: %q", line)
		}
		fields[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	if fields["title"] == "" {
		return gcal.Event{}, fmt.Errorf("title is empty")
	}
	// The times
	allDay := strings.HasPrefix(strings.ToLower(fields["all day"]), "y")
	layout := timeLayout
	if allDay {
		layout = dateLayout
	}
	start, err := time.ParseInLocation(layout, fields["start"], time.Local)
	if err != nil {
		return gcal.Event{}, fmt.Errorf("start: use %s", layout)
	}
	end, err := time.ParseInLocation(layout, fields["end"], time.Local)
	if err != nil {
		return gcal.Event{}, fmt.Errorf("end: use %s", layout)
	}
	// An all day end is the day after the last day, what the API expects
	if allDay {
		end = end.AddDate(0, 0, 1)
	}
	if !end.After(start) {
		return gcal.Event{}, fmt.Errorf("end must be after start")
	}
	// The event
	e := gcal.NewEvent(fields["title"], start, end, allDay)
	e.Location = fields["location"]
	e.Description = description
	return e, nil
}
