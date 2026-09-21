// Package discordweb reads Discord's own web client through the browser and shows it as pages, so it is your account with the official client doing every request
package discordweb

import (
	"fmt"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/browser"
)

// base is where the web client lives, a variable so a test can point it at a local server
var base = "https://discord.com"

// tabName is the browser tab reserved for discord
const tabName = "discord"

// loadWait is how long the client may take to show either the app or the login form
const loadWait = 25 * time.Second

// State is what the client shows right now
type State string

const (
	// StateApp is the logged in client
	StateApp State = "app"
	// StateLogin is the login form
	StateLogin State = "login"
	// StateUnknown is neither yet
	StateUnknown State = "unknown"
)

// Guild is one server in the sidebar
type Guild struct {
	// The id
	ID string `json:"id"`
	// The name
	Name string `json:"name"`
}

// Channel is one channel or direct message in the list
type Channel struct {
	// The id
	ID string `json:"id"`
	// The server, @me for a direct message
	GuildID string `json:"guild"`
	// The name
	Name string `json:"name"`
	// Whether the client marks it unread
	Unread bool `json:"unread"`
}

// Message is one message in the chat
type Message struct {
	// The id
	ID string `json:"id"`
	// The author's display name
	Author string `json:"author"`
	// The text
	Content string `json:"content"`
	// The time as the client stamps it, RFC 3339
	At string `json:"at"`
	// Whether it carries an attachment or an embed
	Extra bool `json:"extra"`
}

// stateScript says whether the app or the login form is showing
const stateScript = `(() => {
  if (document.querySelector('[data-list-id="guildsnav"], nav[aria-label*="Servers"], [class*="guilds"] [data-list-item-id]')) return 'app';
  if (location.pathname.startsWith('/login') || document.querySelector('form input[name="email"], input[type="password"]')) return 'login';
  return 'unknown';
})()`

// guildsScript lists the servers from the sidebar, home is the direct message list and is skipped
const guildsScript = `(() => {
  const out = [];
  const seen = new Set();
  for (const el of document.querySelectorAll('[data-list-item-id^="guildsnav___"]')) {
    const id = el.getAttribute('data-list-item-id').replace('guildsnav___', '');
    if (!/^\d+$/.test(id) || seen.has(id)) continue;
    seen.add(id);
    let name = el.getAttribute('aria-label') || '';
    if (!name) { const inner = el.querySelector('[aria-label]'); if (inner) name = inner.getAttribute('aria-label'); }
    if (!name) { const img = el.querySelector('img[alt]'); if (img) name = img.alt; }
    name = name.replace(/,\s*\d+\s+(mention|unread).*$/i, '').trim();
    out.push({ id: id, name: name || id });
  }
  return out;
})()`

// channelsScript lists the channels of the current server, or the direct messages on the home view
const channelsScript = `((guild) => {
  const out = [];
  const seen = new Set();
  const prefix = '/channels/' + guild + '/';
  const roots = document.querySelectorAll('[data-list-id="channels"], [data-list-id^="private-channels"], nav[aria-label*="Direct"], nav[aria-label*="channels" i]');
  const scope = roots.length ? Array.from(roots) : [document];
  for (const root of scope) {
    for (const a of root.querySelectorAll('a[href^="' + prefix + '"]')) {
      const id = a.getAttribute('href').slice(prefix.length).split('/')[0];
      if (!/^\d+$/.test(id) || seen.has(id)) continue;
      seen.add(id);
      let name = a.getAttribute('aria-label') || (a.innerText || '').replace(/\s+/g, ' ').trim();
      name = name.replace(/\s*\((text|voice|announcement|forum|stage) channel\)/i, '').replace(/,\s*(unread|\d+\s+mention).*$/i, '').trim();
      const unread = /unread|mention/i.test(a.getAttribute('aria-label') || '') || !!a.closest('li')?.querySelector('[class*="unread"], [class*="mentionsBadge"], [class*="numberBadge"]');
      out.push({ id: id, guild: guild, name: name || id, unread: unread });
    }
  }
  return out;
})`

// messagesScript reads the visible chat, a message without a name belongs to the author above it
const messagesScript = `(() => {
  const out = [];
  let author = '';
  const items = document.querySelectorAll('ol[data-list-id="chat-messages"] > li[id^="chat-messages-"], [data-list-id="chat-messages"] li');
  for (const li of items) {
    const id = (li.id || '').replace(/^chat-messages-\d+-/, '') || String(out.length);
    const nameEl = li.querySelector('[id^="message-username-"]');
    if (nameEl) author = (nameEl.innerText || nameEl.textContent || '').replace(/\s+/g, ' ').trim();
    const bodyEl = li.querySelector('[id^="message-content-"]');
    const timeEl = li.querySelector('time[datetime]');
    let content = bodyEl ? (bodyEl.innerText || bodyEl.textContent || '') : '';
    content = content.replace(/\r/g, '').replace(/[ \t]+\n/g, '\n').trim();
    const extra = !!li.querySelector('[class*="attachment"], [class*="embed"], article');
    if (!content && !extra) continue;
    out.push({ id: id, author: author, content: content, at: timeEl ? timeEl.getAttribute('datetime') : '', extra: extra });
  }
  return out;
})()`

// titleScript is the client's own title, which names the channel and the server
const titleScript = `document.title`

// focusBoxScript finds the message box, the slate editor labelled Message, focuses it with the caret at the end, and says whether it found one
const focusBoxScript = `(() => {
  const box = document.querySelector('[data-slate-editor="true"][aria-label^="Message"]') || document.querySelector('form [data-slate-editor="true"]') || document.querySelector('[data-slate-editor="true"][role="textbox"]');
  if (!box) return false;
  box.focus();
  const sel = window.getSelection();
  if (sel) { const range = document.createRange(); range.selectNodeContents(box); range.collapse(false); sel.removeAllRanges(); sel.addRange(range); }
  return true;
})()`

// boxTextScript is what the message box holds, empty once a message went out
const boxTextScript = `(() => {
  const box = document.querySelector('[data-slate-editor="true"][aria-label^="Message"]') || document.querySelector('form [data-slate-editor="true"]') || document.querySelector('[data-slate-editor="true"][role="textbox"]');
  return box ? (box.textContent || '').trim() : '';
})()`

// sendWait is how long a message may take to leave the box
const sendWait = 6 * time.Second

// Client drives the discord tab
type Client struct {
	// The engine
	engine *browser.Engine
}

/**
 * NewClient
 * Builds a client over the engine
 * @param engine {*browser.Engine} - the engine
 * @return *Client
 **/
func NewClient(engine *browser.Engine) *Client {
	return &Client{engine: engine}
}

/**
 * tab
 * Returns the discord tab, opened at the home view when there is none
 * @return *browser.Tab, error
 **/
func (c *Client) tab() (*browser.Tab, error) {
	return c.engine.Named(tabName, base+"/channels/@me")
}

/**
 * Running
 * Reports whether the browser is up, so a home line can avoid starting it
 * @return bool
 **/
func (c *Client) Running() bool {
	return c.engine.Running()
}

/**
 * Go
 * Navigates the discord tab to a path under discord.com when it is not there already and waits for the client to settle
 * @param path {string} - the path such as /channels/@me
 * @return State, error
 **/
func (c *Client) Go(path string) (State, error) {
	t, err := c.tab()
	if err != nil {
		return StateUnknown, err
	}
	// Move when the tab is elsewhere
	cur, _ := t.Location()
	if !strings.HasPrefix(cur, base+path) {
		if err := t.Navigate(base + path); err != nil {
			return StateUnknown, err
		}
	}
	// Wait for the app or the login form
	deadline := time.Now().Add(loadWait)
	for {
		var st string
		if err := t.Eval(stateScript, &st); err == nil && st != "unknown" {
			if st == "app" {
				// The chat and lists render a moment after the shell
				t.Settle(700 * time.Millisecond)
			}
			return State(st), nil
		}
		if time.Now().After(deadline) {
			return StateUnknown, fmt.Errorf("discord: the client did not load in time")
		}
		t.Settle(400 * time.Millisecond)
	}
}

/**
 * Guilds
 * Lists the servers in the sidebar
 * @return []Guild, error
 **/
func (c *Client) Guilds() ([]Guild, error) {
	t, err := c.tab()
	if err != nil {
		return nil, err
	}
	var out []Guild
	if err := t.Eval(guildsScript, &out); err != nil {
		return nil, fmt.Errorf("discord: read servers: %w", err)
	}
	return out, nil
}

/**
 * Channels
 * Lists the channels of the current server, or the direct messages when guild is @me
 * @param guild {string} - the server id or @me
 * @return []Channel, error
 **/
func (c *Client) Channels(guild string) ([]Channel, error) {
	t, err := c.tab()
	if err != nil {
		return nil, err
	}
	var out []Channel
	if err := t.Eval(channelsScript+fmt.Sprintf("(%q)", guild), &out); err != nil {
		return nil, fmt.Errorf("discord: read channels: %w", err)
	}
	return out, nil
}

/**
 * Messages
 * Reads the visible chat
 * @return []Message, string, error
 **/
func (c *Client) Messages() ([]Message, string, error) {
	t, err := c.tab()
	if err != nil {
		return nil, "", err
	}
	var out []Message
	if err := t.Eval(messagesScript, &out); err != nil {
		return nil, "", fmt.Errorf("discord: read messages: %w", err)
	}
	var title string
	_ = t.Eval(titleScript, &title)
	return out, title, nil
}

/**
 * Say
 * Puts a line into the message box as one insert, closes any autocomplete the text raised, presses Enter, and waits for the box to empty
 * @param text {string} - the message
 * @return error
 **/
func (c *Client) Say(text string) error {
	t, err := c.tab()
	if err != nil {
		return err
	}
	// Focus the box, a chat page always has one, a server page does not
	var found bool
	if err := t.Eval(focusBoxScript, &found); err != nil || !found {
		return fmt.Errorf("discord: no message box here")
	}
	// One insert lands like a paste, which the editor takes whole, key by key typing trips its shortcuts
	if err := t.Insert(text); err != nil {
		return fmt.Errorf("discord: type: %w", err)
	}
	// A trailing emoji or mention token opens a picker that would eat the Enter, Escape closes it and does nothing otherwise
	t.Settle(150 * time.Millisecond)
	_ = t.Key("Escape")
	if err := t.Key("Enter"); err != nil {
		return fmt.Errorf("discord: send: %w", err)
	}
	// The box empties once the client accepted the message
	deadline := time.Now().Add(sendWait)
	for {
		var left string
		if err := t.Eval(boxTextScript, &left); err == nil && left == "" {
			t.Settle(600 * time.Millisecond)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("discord: the message stayed in the box, it was not sent")
		}
		t.Settle(300 * time.Millisecond)
	}
}
