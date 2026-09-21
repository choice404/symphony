package discord

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// fake is an API over fixed data that records sends
type fake struct {
	sent []string
}

func (f *fake) Guilds(context.Context) ([]Guild, error) {
	return []Guild{{ID: "g2", Name: "Zeta"}, {ID: "g1", Name: "Alpha"}}, nil
}

func (f *fake) Channels(_ context.Context, guildID string) ([]Channel, error) {
	if guildID != "g1" {
		return nil, fmt.Errorf("unknown guild")
	}
	return []Channel{
		{ID: "c2", GuildID: "g1", Name: "random", Position: 2, Category: "Text"},
		{ID: "c1", GuildID: "g1", Name: "general", Position: 1, Category: "Text"},
		{ID: "c3", GuildID: "g1", Name: "announce", Position: 0},
	}, nil
}

func (f *fake) Messages(_ context.Context, channelID string, limit int) ([]Message, error) {
	base := time.Date(2026, 1, 2, 9, 0, 0, 0, time.Local)
	return []Message{
		{ID: "m1", ChannelID: channelID, GuildID: "g1", Author: "ada", Content: "hello", At: base},
		{ID: "m2", ChannelID: channelID, GuildID: "g1", Author: "symphony", Content: "hi\nthere", At: base.Add(time.Minute), Bot: true},
	}, nil
}

func (f *fake) Send(_ context.Context, channelID, content string) error {
	f.sent = append(f.sent, channelID+":"+content)
	return nil
}

func (f *fake) Me() string { return "symphony" }

func TestNoToken(t *testing.T) {
	d := New(nil, NewStore())
	p, _ := d.Render(context.Background())
	if !strings.Contains(p.Lines[0], "no bot token") {
		t.Fatalf("page = %v", p.Lines)
	}
	if d.Entries()[0].Summary(context.Background()) != "no bot token" {
		t.Fatal("summary")
	}
}

func TestGuildsChannelsMessages(t *testing.T) {
	api := &fake{}
	store := NewStore()
	d := New(api, store)
	ctx := context.Background()
	// Servers sorted by name
	p, err := d.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Lines[3] != "  Alpha" || p.Keys[3] != "g1" || p.Lines[4] != "  Zeta" {
		t.Fatalf("guilds = %v", p.Lines)
	}
	// Channels grouped by category, uncategorized first, keys carry the guild
	r, _ := d.Act(ctx, view.Action{Name: "open", Key: "g1", Page: "discord"})
	if r.Kind != view.KindPage || r.Page.Name != "discord/g1" {
		t.Fatalf("channels = %+v", r)
	}
	text := strings.Join(r.Page.Lines, "\n")
	if !strings.Contains(text, "channels\n  #announce") || !strings.Contains(text, "Text\n  #general\n  #random") {
		t.Fatalf("channels page = %s", text)
	}
	if r.Page.Keys[r.Page.Cursor] != "g1/c3" {
		t.Fatalf("cursor key = %q", r.Page.Keys[r.Page.Cursor])
	}
	// Messages pull history once, group by day, mark the bot's own lines, and mark read
	r, _ = d.Act(ctx, view.Action{Name: "open", Key: "g1/c1", Page: "discord/g1"})
	if r.Kind != view.KindPage || r.Page.Name != "discord/g1/c1" || r.Page.Key != "c1" {
		t.Fatalf("messages = %+v", r)
	}
	text = strings.Join(r.Page.Lines, "\n")
	if !strings.Contains(text, "--- Fri Jan 02") || !strings.Contains(text, "09:00  ada: hello") || !strings.Contains(text, "09:01  you: hi\n         there") {
		t.Fatalf("messages page = %s", text)
	}
	if store.Unread("c1") != 0 {
		t.Fatal("not marked read")
	}
	// A gateway message shows on refresh and counts as unread on the channels page
	store.Add(Message{ID: "m3", ChannelID: "c1", GuildID: "g1", Author: "bo", Content: "new one", At: time.Now()})
	cp, _ := d.RenderPath(ctx, "g1")
	if !strings.Contains(strings.Join(cp.Lines, "\n"), "#general  1 unread") {
		t.Fatalf("unread = %v", cp.Lines)
	}
	if got := d.Entries()[0].Summary(ctx); got != "2 servers, 1 unread" {
		t.Fatalf("summary = %q", got)
	}
	r, _ = d.Act(ctx, view.Action{Name: "refresh", Page: "discord/g1/c1"})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "bo: new one") {
		t.Fatalf("refresh = %v", r.Page.Lines)
	}
	// Sending goes to the API and shows the channel again
	r, _ = d.Act(ctx, view.Action{Name: "send", Page: "discord/g1/c1", Body: "  yo  "})
	if r.Kind != view.KindPage || len(api.sent) != 1 || api.sent[0] != "c1:yo" {
		t.Fatalf("send = %+v sent = %v", r, api.sent)
	}
	r, _ = d.Act(ctx, view.Action{Name: "send", Page: "discord/g1", Body: "x"})
	if r.Kind != view.KindNotify || !r.Error {
		t.Fatalf("send from channels page = %+v", r)
	}
}

func TestStoreCapsAndDedupes(t *testing.T) {
	s := NewStore()
	for i := 0; i < keep+10; i++ {
		s.Add(Message{ID: fmt.Sprintf("m%d", i), ChannelID: "c", At: time.Unix(int64(i), 0)})
	}
	s.Add(Message{ID: "m100", ChannelID: "c"})
	got := s.Recent("c")
	if len(got) != keep || got[0].ID != "m10" {
		t.Fatalf("recent = %d first %s", len(got), got[0].ID)
	}
	// Load keeps gateway messages newer than the history
	s2 := NewStore()
	s2.Add(Message{ID: "live", ChannelID: "c", At: time.Unix(100, 0)})
	s2.Load("c", "g", []Message{{ID: "old", ChannelID: "c", At: time.Unix(1, 0)}})
	if r := s2.Recent("c"); len(r) != 2 || r[0].ID != "old" || r[1].ID != "live" {
		t.Fatalf("merged = %+v", r)
	}
	if s2.Unread("c") != 2 {
		t.Fatal("unread before read")
	}
	s2.MarkRead("c")
	if s2.Unread("c") != 0 || s2.UnreadGuild("g") != 0 {
		t.Fatal("unread after read")
	}
}
