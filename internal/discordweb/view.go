package discordweb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the discord web view
const ViewName = "discordweb"

// homeHint is the second line of the home page
const homeHint = "  <CR> open, on a folder open or close it  r refresh  q back"

// chatHint is the second line of a chat page
const chatHint = "  i say something  r refresh  q back"

// loginLines is what every page says when the client wants a login
var loginLines = []string{
	"discord wants you to log in",
	"",
	"the login page has a captcha and maybe a passkey, so it needs a real window once:",
	"",
	"    symphonyd browser login https://discord.com/login",
	"",
	"log in there, close the window, and press r here",
}

// Web is the discord web app
type Web struct {
	// The client over the browser
	client *Client
}

/**
 * New
 * Builds the view
 * @param client {*Client} - the client
 * @return *Web
 **/
func New(client *Client) *Web {
	return &Web{client: client}
}

/**
 * Name
 * Returns discordweb
 * @return string
 **/
func (w *Web) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the one home entry, which never starts the browser on its own
 * @return []view.Entry
 **/
func (w *Web) Entries() []view.Entry {
	return []view.Entry{{Name: ViewName, Label: "discord (you)", Summary: func(context.Context) string {
		if !w.client.Running() {
			return "your own account through the browser, open to start"
		}
		return "open"
	}}}
}

/**
 * Render
 * Shows the home view, direct messages and servers
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (w *Web) Render(ctx context.Context) (view.Page, error) {
	st, err := w.client.Go("/channels/@me")
	if err != nil {
		return view.Page{}, err
	}
	if st != StateApp {
		return loginPage(ViewName), nil
	}
	guilds, err := w.client.Guilds()
	if err != nil {
		return view.Page{}, err
	}
	dms, _ := w.client.Channels("@me")
	lines := []string{"discord  " + fmt.Sprintf("%d servers, %d direct messages", len(guilds), len(dms)), homeHint, "", "direct messages"}
	keys := []string{"", "", "", ""}
	for _, d := range dms {
		lines = append(lines, "  "+d.Name+mark(d.Unread))
		keys = append(keys, "dm/"+d.ID)
	}
	if len(dms) == 0 {
		lines = append(lines, "  none listed")
		keys = append(keys, "")
	}
	lines = append(lines, "", "servers")
	keys = append(keys, "", "")
	for _, g := range guilds {
		lines = append(lines, guildLine(g))
		if g.Folder {
			keys = append(keys, "f/"+g.ID)
			continue
		}
		keys = append(keys, "s/"+g.ID)
	}
	return view.Page{Name: ViewName, Title: "discord", Lines: lines, Keys: keys, Cursor: 4, Filetype: "discordweb"}, nil
}

/**
 * RenderPath
 * Shows a server's channels, s/<guild>, or a chat, c/<guild>/<channel> or dm/<channel>
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below discordweb
 * @return view.Page, error
 **/
func (w *Web) RenderPath(ctx context.Context, path string) (view.Page, error) {
	parts := strings.Split(path, "/")
	switch {
	case parts[0] == "s" && len(parts) == 2:
		return w.channels(parts[1])
	case parts[0] == "c" && len(parts) == 3:
		return w.chat(parts[1], parts[2], "c/"+parts[1]+"/"+parts[2])
	case parts[0] == "dm" && len(parts) == 2:
		return w.chat("@me", parts[1], "dm/"+parts[1])
	}
	return view.Page{}, fmt.Errorf("discordweb: no page %q", path)
}

/**
 * channels
 * Shows a server's channels
 * @param guild {string} - the server id
 * @return view.Page, error
 **/
func (w *Web) channels(guild string) (view.Page, error) {
	st, err := w.client.Go("/channels/" + guild)
	if err != nil {
		return view.Page{}, err
	}
	if st != StateApp {
		return loginPage(ViewName + "/s/" + guild), nil
	}
	chans, err := w.client.Channels(guild)
	if err != nil {
		return view.Page{}, err
	}
	var title string
	_, title, _ = w.client.Messages()
	lines := []string{serverName(title) + "  " + fmt.Sprintf("%d channels", len(chans)), homeHint, ""}
	keys := []string{"", "", ""}
	for _, c := range chans {
		lines = append(lines, "  #"+c.Name+mark(c.Unread))
		keys = append(keys, "c/"+guild+"/"+c.ID)
	}
	if len(chans) == 0 {
		lines = append(lines, "  no channels read, press r once the server has loaded")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName + "/s/" + guild, Title: serverName(title), Lines: lines, Keys: keys, Key: guild, Cursor: 3, Filetype: "discordweb"}, nil
}

/**
 * chat
 * Shows a channel or direct message and its visible history
 * @param guild {string} - the server id or @me
 * @param channel {string} - the channel id
 * @param path {string} - the page path below discordweb
 * @return view.Page, error
 **/
func (w *Web) chat(guild, channel, path string) (view.Page, error) {
	st, err := w.client.Go("/channels/" + guild + "/" + channel)
	if err != nil {
		return view.Page{}, err
	}
	if st != StateApp {
		return loginPage(ViewName + "/" + path), nil
	}
	msgs, title, err := w.client.Messages()
	if err != nil {
		return view.Page{}, err
	}
	lines := []string{chatTitle(title), chatHint, ""}
	keys := []string{"", "", ""}
	lastDay := ""
	for _, m := range msgs {
		stamp := ""
		if t, err := time.Parse(time.RFC3339, m.At); err == nil {
			day := t.Local().Format("Mon Jan 02")
			if day != lastDay {
				lines = append(lines, "--- "+day)
				keys = append(keys, "")
				lastDay = day
			}
			stamp = t.Local().Format("15:04") + "  "
		}
		text := strings.Split(m.Content, "\n")
		first := text[0]
		if m.Extra {
			first += "  [attachment]"
		}
		lines = append(lines, stamp+m.Author+": "+first)
		keys = append(keys, m.ID)
		for _, more := range text[1:] {
			lines = append(lines, strings.Repeat(" ", len(stamp))+"  "+more)
			keys = append(keys, m.ID)
		}
	}
	if len(msgs) == 0 {
		lines = append(lines, "  nothing read yet, press r once the chat has loaded")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName + "/" + path, Title: chatTitle(title), Lines: lines, Keys: keys, Key: channel, Cursor: len(lines) - 1, Filetype: "discordchat"}, nil
}

/**
 * Act
 * Runs an action from a discord web page
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (w *Web) Act(ctx context.Context, a view.Action) (view.Response, error) {
	_, path := view.Split(a.Page)
	switch a.Name {
	case "refresh":
		if path == "" {
			return w.show(w.Render(ctx))
		}
		return w.show(w.RenderPath(ctx, path))
	case "open":
		if a.Key == "" || !strings.Contains(a.Key, "/") {
			return view.Response{Kind: view.KindNone}, nil
		}
		// A folder opens or closes in place and the home page reads again
		if strings.HasPrefix(a.Key, "f/") {
			if err := w.client.ToggleFolder(strings.TrimPrefix(a.Key, "f/")); err != nil {
				return view.Fail(err.Error()), nil
			}
			return w.show(w.Render(ctx))
		}
		return w.show(w.RenderPath(ctx, a.Key))
	case "send":
		if !strings.HasPrefix(path, "c/") && !strings.HasPrefix(path, "dm/") {
			return view.Fail("discord: say something from a chat page"), nil
		}
		text := strings.TrimSpace(a.Body)
		if text == "" {
			return view.Fail("discord: nothing to send"), nil
		}
		if err := w.client.Say(text); err != nil {
			return view.Fail(err.Error()), nil
		}
		return w.show(w.RenderPath(ctx, path))
	}
	return view.Fail("discordweb: unknown action " + a.Name), nil
}

/**
 * show
 * Turns a page or its error into a response
 * @param p {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (w *Web) show(p view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(p), nil
}

/**
 * loginPage
 * Builds the page that asks for a headed login
 * @param name {string} - the page name to keep
 * @return view.Page
 **/
func loginPage(name string) view.Page {
	return view.Page{Name: name, Title: "discord login", Lines: loginLines, Keys: make([]string, len(loginLines)), Filetype: "discordweb"}
}

/**
 * guildLine
 * Formats a sidebar entry, a folder with its open or closed sign, a server inside a folder indented
 * @param g {Guild} - the entry
 * @return string
 **/
func guildLine(g Guild) string {
	if g.Folder {
		sign := "+ "
		if g.Open {
			sign = "- "
		}
		return "  " + sign + g.Name + mark(g.Unread)
	}
	if g.Inside {
		return "      " + g.Name + mark(g.Unread)
	}
	return "  " + g.Name + mark(g.Unread)
}

/**
 * mark
 * Returns the unread tag for a line
 * @param unread {bool} - whether unread
 * @return string
 **/
func mark(unread bool) string {
	if unread {
		return "  *"
	}
	return ""
}

/**
 * serverName
 * Pulls the server out of the client's title, Discord | #channel | Server
 * @param title {string} - the title
 * @return string
 **/
func serverName(title string) string {
	parts := strings.Split(title, "|")
	if len(parts) >= 3 {
		return strings.TrimSpace(parts[len(parts)-1])
	}
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(title)
}

/**
 * chatTitle
 * Pulls the channel and the server out of the client's title
 * @param title {string} - the title
 * @return string
 **/
func chatTitle(title string) string {
	parts := strings.Split(title, "|")
	if len(parts) >= 2 {
		out := make([]string, 0, 2)
		for _, p := range parts[1:] {
			out = append(out, strings.TrimSpace(p))
		}
		return strings.Join(out, "  ")
	}
	return strings.TrimSpace(title)
}
