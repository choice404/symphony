// Package discord shows a bot's servers and channels as pages and sends messages into them
package discord

import (
	"context"
	"time"
)

// ViewName is the name of the discord view
const ViewName = "discord"

// keep is how many messages a channel keeps in memory
const keep = 200

// fetch is how many messages a first open pulls from the API
const fetch = 50

// Guild is one server the bot is in
type Guild struct {
	// The id
	ID string
	// The name
	Name string
}

// Channel is one text channel
type Channel struct {
	// The id
	ID string
	// The server it belongs to
	GuildID string
	// The name without the hash
	Name string
	// Where it sorts
	Position int
	// The category name, empty for none
	Category string
}

// Message is one message in a channel
type Message struct {
	// The id
	ID string
	// The channel it was sent in
	ChannelID string
	// The server, empty for a direct message
	GuildID string
	// The author's display name
	Author string
	// The text
	Content string
	// When it was sent
	At time.Time
	// Whether a bot sent it
	Bot bool
}

// API is what the view needs from Discord, real over discordgo and fake in tests
type API interface {
	// The servers the bot is in
	Guilds(ctx context.Context) ([]Guild, error)
	// The text channels of a server in display order
	Channels(ctx context.Context, guildID string) ([]Channel, error)
	// The newest messages of a channel, oldest first
	Messages(ctx context.Context, channelID string, limit int) ([]Message, error)
	// Sends a message
	Send(ctx context.Context, channelID, content string) error
	// The bot's own display name, for marking its lines
	Me() string
}
