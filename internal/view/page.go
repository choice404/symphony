// Package view is the shape every app shows through nvim, a page of lines with a key per line and actions on a key
package view

import "context"

// Page is one screen of content the plugin turns into a buffer, it is built once and never changed
type Page struct {
	// The view name, the buffer is symphony://Name
	Name string
	// The title shown in the status area
	Title string
	// The lines of the buffer
	Lines []string
	// The key of every line, same length as Lines, empty for a line with no item
	Keys []string
	// The zero based line the cursor starts on
	Cursor int
	// The filetype suffix, the buffer gets symphony-Filetype
	Filetype string
}

// Kind is what the plugin does with a Response
type Kind string

const (
	// KindNone means nothing to do
	KindNone Kind = "none"
	// KindPage means show the page in its own buffer
	KindPage Kind = "page"
	// KindNotify means show the text as a message
	KindNotify Kind = "notify"
)

// Response is what an action hands back to the plugin
type Response struct {
	// What to do
	Kind Kind
	// The page to show for KindPage
	Page Page
	// The message text for KindNotify
	Text string
	// Whether the message is an error
	Error bool
}

// View is one app as the plugin sees it
type View interface {
	// The name used in symphony://name and :Symphony open name
	Name() string
	// The page to show when the view is opened
	Render(ctx context.Context) (Page, error)
	// The response to an action on a key, the key is empty on a line with no item
	Act(ctx context.Context, action, key string) (Response, error)
}

/**
 * Notify
 * Builds a message response
 * @param text {string} - the message
 * @return Response
 **/
func Notify(text string) Response {
	return Response{Kind: KindNotify, Text: text}
}

/**
 * Fail
 * Builds an error message response
 * @param text {string} - the message
 * @return Response
 **/
func Fail(text string) Response {
	return Response{Kind: KindNotify, Text: text, Error: true}
}

/**
 * Show
 * Builds a page response
 * @param p {Page} - the page
 * @return Response
 **/
func Show(p Page) Response {
	return Response{Kind: KindPage, Page: p}
}

/**
 * ToMap
 * Turns a page into the plain map the rpc layer sends to lua
 * @return map[string]interface{}
 **/
func (p Page) ToMap() map[string]interface{} {
	// Copy the slices so the map never shares memory with the page, never nil so lua sees a table not nil
	lines := append(make([]string, 0, len(p.Lines)), p.Lines...)
	keys := append(make([]string, 0, len(p.Lines)), p.Keys...)
	// Pad the keys so lua always finds one per line
	for len(keys) < len(lines) {
		keys = append(keys, "")
	}
	// Return the map
	return map[string]interface{}{
		"name":     p.Name,
		"title":    p.Title,
		"lines":    lines,
		"keys":     keys,
		"cursor":   p.Cursor,
		"filetype": p.Filetype,
	}
}

/**
 * ToMap
 * Turns a response into the plain map the rpc layer sends to lua
 * @return map[string]interface{}
 **/
func (r Response) ToMap() map[string]interface{} {
	// The base map
	m := map[string]interface{}{"kind": string(r.Kind)}
	// Add the page or the text by kind
	switch r.Kind {
	case KindPage:
		m["page"] = r.Page.ToMap()
	case KindNotify:
		m["text"] = r.Text
		m["error"] = r.Error
	}
	// Return the map
	return m
}
