package discord

import (
	"context"
	"fmt"
	"sort"

	"github.com/bwmarrin/discordgo"
)

// Bot is the API over discordgo, REST for lists and history and the gateway for live messages
type Bot struct {
	// The session
	s *discordgo.Session
	// The store the gateway feeds
	store *Store
	// The bot's display name
	me string
}

/**
 * Connect
 * Logs the bot in, opens the gateway so new messages flow into the store, and returns the API
 * @param token {string} - the bot token
 * @param store {*Store} - where gateway messages land
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return *Bot, error
 **/
func Connect(token string, store *Store, logf func(string, ...interface{})) (*Bot, error) {
	// The session with the intents the pages need, message content is a privileged one the portal has to allow
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	s.Identify.Intents = discordgo.IntentGuilds | discordgo.IntentGuildMessages | discordgo.IntentMessageContent
	// Every new message goes into the store
	s.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Message == nil {
			return
		}
		store.Add(convert(m.Message))
	})
	s.AddHandler(func(_ *discordgo.Session, r *discordgo.Ready) {
		logf("discord: gateway ready as %s", r.User.Username)
	})
	// Who we are
	me, err := s.User("@me")
	if err != nil {
		return nil, fmt.Errorf("discord: token refused: %w", err)
	}
	b := &Bot{s: s, store: store, me: me.Username}
	// The gateway, a failure here still leaves the REST side working
	if err := s.Open(); err != nil {
		logf("discord: gateway failed, live messages are off: %v", err)
	}
	return b, nil
}

/**
 * Close
 * Closes the gateway
 * @return void
 **/
func (b *Bot) Close() {
	_ = b.s.Close()
}

/**
 * Me
 * Returns the bot's username
 * @return string
 **/
func (b *Bot) Me() string {
	return b.me
}

/**
 * Guilds
 * Lists the servers the bot is in
 * @param ctx {context.Context} - the context
 * @return []Guild, error
 **/
func (b *Bot) Guilds(ctx context.Context) ([]Guild, error) {
	got, err := b.s.UserGuilds(100, "", "", false, discordgo.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	out := make([]Guild, 0, len(got))
	for _, g := range got {
		out = append(out, Guild{ID: g.ID, Name: g.Name})
	}
	return out, nil
}

/**
 * Channels
 * Lists a server's text channels with their category names
 * @param ctx {context.Context} - the context
 * @param guildID {string} - the server
 * @return []Channel, error
 **/
func (b *Bot) Channels(ctx context.Context, guildID string) ([]Channel, error) {
	got, err := b.s.GuildChannels(guildID, discordgo.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	// Categories by id for the names
	cats := map[string]*discordgo.Channel{}
	for _, c := range got {
		if c.Type == discordgo.ChannelTypeGuildCategory {
			cats[c.ID] = c
		}
	}
	out := make([]Channel, 0, len(got))
	for _, c := range got {
		if c.Type != discordgo.ChannelTypeGuildText && c.Type != discordgo.ChannelTypeGuildNews {
			continue
		}
		ch := Channel{ID: c.ID, GuildID: guildID, Name: c.Name, Position: c.Position}
		if cat, ok := cats[c.ParentID]; ok {
			ch.Category = cat.Name
			ch.Position += cat.Position * 1000
		}
		out = append(out, ch)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

/**
 * Messages
 * Pulls the newest messages of a channel and returns them oldest first
 * @param ctx {context.Context} - the context
 * @param channelID {string} - the channel
 * @param limit {int} - how many
 * @return []Message, error
 **/
func (b *Bot) Messages(ctx context.Context, channelID string, limit int) ([]Message, error) {
	got, err := b.s.ChannelMessages(channelID, limit, "", "", "", discordgo.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	// The API answers newest first
	out := make([]Message, 0, len(got))
	for i := len(got) - 1; i >= 0; i-- {
		out = append(out, convert(got[i]))
	}
	return out, nil
}

/**
 * Send
 * Sends a message into a channel
 * @param ctx {context.Context} - the context
 * @param channelID {string} - the channel
 * @param content {string} - the text
 * @return error
 **/
func (b *Bot) Send(ctx context.Context, channelID, content string) error {
	_, err := b.s.ChannelMessageSend(channelID, content, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	return nil
}

/**
 * convert
 * Turns a discordgo message into the app's message
 * @param m {*discordgo.Message} - the message
 * @return Message
 **/
func convert(m *discordgo.Message) Message {
	out := Message{ID: m.ID, ChannelID: m.ChannelID, GuildID: m.GuildID, Content: m.Content, At: m.Timestamp}
	if m.Author != nil {
		out.Author = m.Author.Username
		if m.Author.GlobalName != "" {
			out.Author = m.Author.GlobalName
		}
		out.Bot = m.Author.Bot
	}
	return out
}
