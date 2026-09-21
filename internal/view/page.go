// Package view is the shape every app shows through nvim, a page of lines with a key per line and actions on a key
package view

import "context"

// Page is one screen of content the plugin turns into a buffer, it is built once and never changed
type Page struct {
	// The page name, the buffer is symphony://Name, the part before the first slash is the view
	Name string
	// The title shown in the status area
	Title string
	// The lines of the buffer
	Lines []string
	// The key of every line, same length as Lines, empty for a line with no item
	Keys []string
	// The key of the page as a whole, used by an action on a line with no key, such as reply on a message
	Key string
	// The zero based line the cursor starts on
	Cursor int
	// The filetype suffix, the buffer gets symphony-Filetype
	Filetype string
	// Whether the buffer stays writable, for a compose page
	Editable bool
}

// Action is one request from the plugin against a view
type Action struct {
	// The action name such as open, refresh, reply, or send
	Name string
	// The key of the line the action was run on, or the page key when the line has none
	Key string
	// The page name the action was run from
	Page string
	// The buffer text for an action that submits it, such as send
	Body string
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
	// KindEnter means leave the pages and work in a directory, project mode, Path says where and Text names it
	KindEnter Kind = "enter"
	// KindEdit means open a file in the editor, Path says which
	KindEdit Kind = "edit"
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
	// A buffer name to close after applying the response, such as a sent compose page
	Close string
	// The directory to enter for KindEnter
	Path string
}

// View is one app as the plugin sees it
type View interface {
	// The name used in symphony://name and :Symphony open name
	Name() string
	// The page to show when the view is opened with no path
	Render(ctx context.Context) (Page, error)
	// The response to an action
	Act(ctx context.Context, a Action) (Response, error)
}

// Pager is a view that also renders pages under a path, such as mail/school/sent
type Pager interface {
	// The page for a path below the view name
	RenderPath(ctx context.Context, path string) (Page, error)
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
		"key":      p.Key,
		"cursor":   p.Cursor,
		"filetype": p.Filetype,
		"editable": p.Editable,
	}
}

/**
 * ToMap
 * Turns a response into the plain map the rpc layer sends to lua
 * @return map[string]interface{}
 **/
func (r Response) ToMap() map[string]interface{} {
	// The base map
	m := map[string]interface{}{"kind": string(r.Kind), "close": r.Close}
	// Add the page or the text by kind
	switch r.Kind {
	case KindPage:
		m["page"] = r.Page.ToMap()
	case KindNotify:
		m["text"] = r.Text
		m["error"] = r.Error
	case KindEnter, KindEdit:
		m["text"] = r.Text
		m["path"] = r.Path
	}
	// Return the map
	return m
}
