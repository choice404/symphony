package discord

import (
	"sort"
	"sync"
)

// Store keeps the messages the gateway delivered and what has been read, all in memory
type Store struct {
	// Guards everything below
	mu sync.Mutex
	// The messages per channel, oldest first, at most keep
	messages map[string][]Message
	// Whether a channel was ever loaded from the API
	loaded map[string]bool
	// The id of the last message read per channel
	read map[string]string
	// The guild of every channel seen, for unread counts per server
	guildOf map[string]string
}

/**
 * NewStore
 * Builds an empty store
 * @return *Store
 **/
func NewStore() *Store {
	return &Store{messages: map[string][]Message{}, loaded: map[string]bool{}, read: map[string]string{}, guildOf: map[string]string{}}
}

/**
 * Add
 * Appends a message the gateway delivered, dropping the oldest past the cap and ignoring a repeat
 * @param m {Message} - the message
 * @return void
 **/
func (s *Store) Add(m Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.messages[m.ChannelID]
	for _, have := range list {
		if have.ID == m.ID {
			return
		}
	}
	list = append(append([]Message{}, list...), m)
	if len(list) > keep {
		list = list[len(list)-keep:]
	}
	s.messages[m.ChannelID] = list
	if m.GuildID != "" {
		s.guildOf[m.ChannelID] = m.GuildID
	}
}

/**
 * Load
 * Replaces a channel's history with what the API returned, keeping anything newer the gateway already delivered
 * @param channelID {string} - the channel
 * @param guildID {string} - the server
 * @param msgs {[]Message} - the history oldest first
 * @return void
 **/
func (s *Store) Load(channelID, guildID string, msgs []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// What the gateway delivered that the history does not have
	seen := map[string]bool{}
	for _, m := range msgs {
		seen[m.ID] = true
	}
	merged := append([]Message{}, msgs...)
	for _, m := range s.messages[channelID] {
		if !seen[m.ID] {
			merged = append(merged, m)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].At.Before(merged[j].At) })
	if len(merged) > keep {
		merged = merged[len(merged)-keep:]
	}
	s.messages[channelID] = merged
	s.loaded[channelID] = true
	s.guildOf[channelID] = guildID
}

/**
 * Loaded
 * Reports whether a channel's history was ever pulled
 * @param channelID {string} - the channel
 * @return bool
 **/
func (s *Store) Loaded(channelID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loaded[channelID]
}

/**
 * Recent
 * Returns a channel's messages oldest first
 * @param channelID {string} - the channel
 * @return []Message
 **/
func (s *Store) Recent(channelID string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message{}, s.messages[channelID]...)
}

/**
 * MarkRead
 * Records that everything in a channel so far was seen
 * @param channelID {string} - the channel
 * @return void
 **/
func (s *Store) MarkRead(channelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.messages[channelID]
	if len(list) > 0 {
		s.read[channelID] = list[len(list)-1].ID
	}
}

/**
 * Unread
 * Counts the messages in a channel after the last one read, everything when nothing was read
 * @param channelID {string} - the channel
 * @return int
 **/
func (s *Store) Unread(channelID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unreadLocked(channelID)
}

/**
 * unreadLocked
 * Counts unread messages with the lock held
 * @param channelID {string} - the channel
 * @return int
 **/
func (s *Store) unreadLocked(channelID string) int {
	list := s.messages[channelID]
	last := s.read[channelID]
	if last == "" {
		return len(list)
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].ID == last {
			return len(list) - 1 - i
		}
	}
	return len(list)
}

/**
 * UnreadGuild
 * Sums the unread messages of every channel seen in a server
 * @param guildID {string} - the server
 * @return int
 **/
func (s *Store) UnreadGuild(guildID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for ch, g := range s.guildOf {
		if g == guildID {
			n += s.unreadLocked(ch)
		}
	}
	return n
}

/**
 * UnreadAll
 * Sums the unread messages of every channel seen
 * @return int
 **/
func (s *Store) UnreadAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for ch := range s.messages {
		n += s.unreadLocked(ch)
	}
	return n
}
