package discord

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/choice404/symphony/internal/view"
)

// guildsHint is the second line of the servers page
const guildsHint = "  <CR> open  r refresh  q back"

// channelsHint is the second line of a channels page
const channelsHint = "  <CR> open  r refresh  q back to servers"

// messagesHint is the second line of a messages page
const messagesHint = "  i say something  r refresh  q back to channels"

// Discord is the discord app
type Discord struct {
	// The API, nil when no token is configured
	api API
	// The messages the gateway delivered and what was read
	store *Store
	// Guards the cached names
	mu map[string]string
}

/**
 * New
 * Builds the discord view
 * @param api {API} - the API, nil when no token is configured
 * @param store {*Store} - the message store the gateway feeds
 * @return *Discord
 **/
func New(api API, store *Store) *Discord {
	return &Discord{api: api, store: store, mu: map[string]string{}}
}

/**
 * Name
 * Returns discord
 * @return string
 **/
func (d *Discord) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the one home entry
 * @return []view.Entry
 **/
func (d *Discord) Entries() []view.Entry {
	return []view.Entry{{Name: ViewName, Label: "discord", Summary: d.summary}}
}

/**
 * summary
 * Returns the server count and the unread total for home
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (d *Discord) summary(ctx context.Context) string {
	if d.api == nil {
		return "no bot token"
	}
	guilds, err := d.api.Guilds(ctx)
	if err != nil {
		return err.Error()
	}
	out := fmt.Sprintf("%d servers", len(guilds))
	if n := d.store.UnreadAll(); n > 0 {
		out += fmt.Sprintf(", %d unread", n)
	}
	return out
}

/**
 * Render
 * Lists the servers
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (d *Discord) Render(ctx context.Context) (view.Page, error) {
	if d.api == nil {
		return view.Page{
			Name:     ViewName,
			Title:    "discord",
			Lines:    []string{"discord has no bot token", "", "put the bot token in ~/.config/symphony/discord.token mode 600 and restart the daemon"},
			Keys:     []string{"", "", ""},
			Filetype: "page",
		}, nil
	}
	guilds, err := d.api.Guilds(ctx)
	if err != nil {
		return view.Page{}, err
	}
	sort.SliceStable(guilds, func(i, j int) bool { return strings.ToLower(guilds[i].Name) < strings.ToLower(guilds[j].Name) })
	lines := []string{fmt.Sprintf("discord  %d servers", len(guilds)), guildsHint, ""}
	keys := []string{"", "", ""}
	for _, g := range guilds {
		unread := ""
		if n := d.store.UnreadGuild(g.ID); n > 0 {
			unread = fmt.Sprintf("  %d unread", n)
		}
		lines = append(lines, "  "+g.Name+unread)
		keys = append(keys, g.ID)
	}
	if len(guilds) == 0 {
		lines = append(lines, "  the bot is in no servers yet, invite it from the developer portal")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName, Title: "discord", Lines: lines, Keys: keys, Cursor: 3, Filetype: "discordguilds"}, nil
}

/**
 * RenderPath
 * Renders a channels page, guild, or a messages page, guild/channel
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below discord
 * @return view.Page, error
 **/
func (d *Discord) RenderPath(ctx context.Context, path string) (view.Page, error) {
	if d.api == nil {
		return d.Render(ctx)
	}
	guild, channel, _ := strings.Cut(path, "/")
	if channel == "" {
		return d.channels(ctx, guild)
	}
	return d.messages(ctx, guild, channel)
}

/**
 * channels
 * Lists a server's text channels grouped by category
 * @param ctx {context.Context} - the context
 * @param guildID {string} - the server
 * @return view.Page, error
 **/
func (d *Discord) channels(ctx context.Context, guildID string) (view.Page, error) {
	chans, err := d.api.Channels(ctx, guildID)
	if err != nil {
		return view.Page{}, err
	}
	sort.SliceStable(chans, func(i, j int) bool {
		if chans[i].Category != chans[j].Category {
			return chans[i].Category < chans[j].Category
		}
		return chans[i].Position < chans[j].Position
	})
	lines := []string{d.guildName(ctx, guildID) + "  " + fmt.Sprintf("%d channels", len(chans)), channelsHint}
	keys := []string{"", ""}
	last := "\x00"
	for _, c := range chans {
		if c.Category != last {
			title := c.Category
			if title == "" {
				title = "channels"
			}
			lines = append(lines, "", title)
			keys = append(keys, "", "")
			last = c.Category
		}
		unread := ""
		if n := d.store.Unread(c.ID); n > 0 && d.store.Loaded(c.ID) {
			unread = fmt.Sprintf("  %d unread", n)
		} else if n > 0 {
			unread = fmt.Sprintf("  %d new", n)
		}
		lines = append(lines, "  #"+c.Name+unread)
		keys = append(keys, guildID+"/"+c.ID)
	}
	cursor := 3
	for i, k := range keys {
		if k != "" {
			cursor = i
			break
		}
	}
	return view.Page{Name: ViewName + "/" + guildID, Title: d.guildName(ctx, guildID), Lines: lines, Keys: keys, Key: guildID, Cursor: cursor, Filetype: "discordchannels"}, nil
}

/**
 * messages
 * Shows a channel's messages oldest first, pulling history on the first open, and marks it read
 * @param ctx {context.Context} - the context
 * @param guildID {string} - the server
 * @param channelID {string} - the channel
 * @return view.Page, error
 **/
func (d *Discord) messages(ctx context.Context, guildID, channelID string) (view.Page, error) {
	if !d.store.Loaded(channelID) {
		history, err := d.api.Messages(ctx, channelID, fetch)
		if err != nil {
			return view.Page{}, err
		}
		d.store.Load(channelID, guildID, history)
	}
	msgs := d.store.Recent(channelID)
	d.store.MarkRead(channelID)
	lines := []string{"#" + d.channelName(ctx, guildID, channelID) + "  " + d.guildName(ctx, guildID), messagesHint, ""}
	keys := []string{"", "", ""}
	me := d.api.Me()
	lastDay := ""
	for _, m := range msgs {
		day := m.At.Local().Format("Mon Jan 02")
		if day != lastDay {
			lines = append(lines, "--- "+day)
			keys = append(keys, "")
			lastDay = day
		}
		author := m.Author
		if m.Bot && m.Author == me {
			author = "you"
		}
		text := strings.Split(m.Content, "\n")
		lines = append(lines, fmt.Sprintf("%s  %s: %s", m.At.Local().Format("15:04"), author, text[0]))
		keys = append(keys, m.ID)
		for _, more := range text[1:] {
			lines = append(lines, "         "+more)
			keys = append(keys, m.ID)
		}
	}
	if len(msgs) == 0 {
		lines = append(lines, "  nothing here yet")
		keys = append(keys, "")
	}
	return view.Page{Name: ViewName + "/" + guildID + "/" + channelID, Title: "#" + d.channelName(ctx, guildID, channelID), Lines: lines, Keys: keys, Key: channelID, Cursor: len(lines) - 1, Filetype: "discordmessages"}, nil
}

/**
 * guildName
 * Returns a server's name, remembered from the last listing
 * @param ctx {context.Context} - the context
 * @param guildID {string} - the server
 * @return string
 **/
func (d *Discord) guildName(ctx context.Context, guildID string) string {
	if name, ok := d.mu["g:"+guildID]; ok {
		return name
	}
	guilds, err := d.api.Guilds(ctx)
	if err == nil {
		for _, g := range guilds {
			d.mu["g:"+g.ID] = g.Name
		}
	}
	if name, ok := d.mu["g:"+guildID]; ok {
		return name
	}
	return guildID
}

/**
 * channelName
 * Returns a channel's name, remembered from the last listing
 * @param ctx {context.Context} - the context
 * @param guildID {string} - the server
 * @param channelID {string} - the channel
 * @return string
 **/
func (d *Discord) channelName(ctx context.Context, guildID, channelID string) string {
	if name, ok := d.mu["c:"+channelID]; ok {
		return name
	}
	chans, err := d.api.Channels(ctx, guildID)
	if err == nil {
		for _, c := range chans {
			d.mu["c:"+c.ID] = c.Name
		}
	}
	if name, ok := d.mu["c:"+channelID]; ok {
		return name
	}
	return channelID
}

/**
 * Act
 * Runs an action from a discord page
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (d *Discord) Act(ctx context.Context, a view.Action) (view.Response, error) {
	if d.api == nil {
		return view.Fail("discord: no bot token"), nil
	}
	_, path := view.Split(a.Page)
	guild, channel, _ := strings.Cut(path, "/")
	switch a.Name {
	case "refresh":
		return d.show(d.RenderPath(ctx, path))
	case "open":
		if a.Key == "" {
			return view.Response{Kind: view.KindNone}, nil
		}
		if channel != "" {
			return view.Response{Kind: view.KindNone}, nil
		}
		return d.show(d.RenderPath(ctx, a.Key))
	case "send":
		if channel == "" {
			return view.Fail("discord: say something from a channel page"), nil
		}
		text := strings.TrimSpace(a.Body)
		if text == "" {
			return view.Fail("discord: nothing to send"), nil
		}
		if err := d.api.Send(ctx, channel, text); err != nil {
			return view.Fail("discord: " + err.Error()), nil
		}
		return d.show(d.messages(ctx, guild, channel))
	}
	return view.Fail("discord: unknown action " + a.Name), nil
}

/**
 * show
 * Turns a page or its error into a response
 * @param p {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (d *Discord) show(p view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(p), nil
}
