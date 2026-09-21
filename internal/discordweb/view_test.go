package discordweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/browser"
	"github.com/choice404/symphony/internal/view"
)

// app is a stand in for the client's DOM, the pieces the scripts read
const app = `<html><head><title>Discord | #general | Demo Server</title></head><body>
<nav aria-label="Servers sidebar"><div data-list-id="guildsnav">
  <div data-list-item-id="guildsnav___home" aria-label="Direct Messages"></div>
  <div data-list-item-id="guildsnav___111" aria-label="Demo Server, 2 unread"></div>
  <div data-list-item-id="guildsnav___222"><img alt="Other"></div>
</div></nav>
<ul data-list-id="private-channels-uid_1">
  <li><a href="/channels/@me/900" aria-label="ada (direct message)">ada</a></li>
</ul>
<div data-list-id="channels">
  <li><a href="/channels/111/10" aria-label="general (text channel)">general</a></li>
  <li><a href="/channels/111/11" aria-label="random (text channel), unread">random</a></li>
  <li><a href="/channels/111/12" aria-label="voice (voice channel)">voice</a></li>
</div>
<ol data-list-id="chat-messages">
  <li id="chat-messages-10-1"><h3><span id="message-username-1">ada</span></h3><time datetime="2026-01-02T17:00:00.000Z"></time><div id="message-content-1">hello there</div></li>
  <li id="chat-messages-10-2"><time datetime="2026-01-02T17:01:00.000Z"></time><div id="message-content-2">second line<br>more</div></li>
  <li id="chat-messages-10-3"><h3><span id="message-username-3">bo</span></h3><time datetime="2026-01-03T09:00:00.000Z"></time><div id="message-content-3">look</div><div class="attachment-x">img</div></li>
</ol>
<div role="textbox" data-slate-editor="true" contenteditable="true" aria-label="Message #general"></div>
<script>
document.querySelector('[data-slate-editor]').addEventListener('keydown', (e) => {
  if (e.key === 'Enter') {
    const li = document.createElement('li'); li.id = 'chat-messages-10-9';
    li.innerHTML = '<h3><span id="message-username-9">you</span></h3><time datetime="2026-01-03T09:05:00.000Z"></time><div id="message-content-9">' + e.target.textContent + '</div>';
    document.querySelector('ol').appendChild(li); e.target.textContent = '';
  }
});
</script>
</body></html>`

// login is the login form
const login = `<html><head><title>Discord</title></head><body><form><input name="email"><input type="password" name="password"></form></body></html>`

// serve stands in for discord.com, base is pointed at it
func serve(t *testing.T, loggedIn bool) *browser.Engine {
	t.Helper()
	exec := browser.FindChrome()
	if exec == "" {
		t.Skip("no chromium on PATH")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if loggedIn {
			_, _ = w.Write([]byte(app))
			return
		}
		_, _ = w.Write([]byte(login))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := base
	base = srv.URL
	t.Cleanup(func() { base = old })
	e := browser.New(exec, t.TempDir())
	t.Cleanup(e.Stop)
	return e
}

func TestLoginPage(t *testing.T) {
	w := New(NewClient(serve(t, false)))
	p, err := w.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Lines[0], "log in") || !strings.Contains(strings.Join(p.Lines, "\n"), "browser login") {
		t.Fatalf("page = %v", p.Lines)
	}
}

func TestHomeChannelsChatAndSay(t *testing.T) {
	w := New(NewClient(serve(t, true)))
	ctx := context.Background()
	// Home lists the direct messages and the servers, with names cleaned of the client's suffixes
	p, err := w.Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(p.Lines, "\n")
	if !strings.Contains(text, "direct messages\n  ada (direct message)") && !strings.Contains(text, "direct messages\n  ada") {
		t.Fatalf("dms = %s", text)
	}
	if !strings.Contains(text, "servers\n  Demo Server\n  Other") {
		t.Fatalf("servers = %s", text)
	}
	if p.Keys[4] != "dm/900" {
		t.Fatalf("dm key = %q", p.Keys[4])
	}
	// The channels page reads the list, marks unread, and keys by server and channel
	r, _ := w.Act(ctx, view.Action{Name: "open", Key: "s/111", Page: "discordweb"})
	if r.Kind != view.KindPage || r.Page.Name != "discordweb/s/111" {
		t.Fatalf("channels = %+v", r)
	}
	text = strings.Join(r.Page.Lines, "\n")
	if !strings.HasPrefix(r.Page.Lines[0], "Demo Server") || !strings.Contains(text, "#general\n  #random  *\n  #voice") || r.Page.Keys[3] != "c/111/10" {
		t.Fatalf("channels page = %s keys = %v", text, r.Page.Keys)
	}
	// The chat groups by day, carries authors down, keeps line breaks, and marks attachments
	r, _ = w.Act(ctx, view.Action{Name: "open", Key: "c/111/10", Page: "discordweb/s/111"})
	if r.Kind != view.KindPage || r.Page.Name != "discordweb/c/111/10" {
		t.Fatalf("chat = %+v", r)
	}
	text = strings.Join(r.Page.Lines, "\n")
	if !strings.Contains(text, "ada: hello there") || !strings.Contains(text, "ada: second line\n") || !strings.Contains(text, "  more") || !strings.Contains(text, "bo: look  [attachment]") {
		t.Fatalf("chat page = %s", text)
	}
	if strings.Count(text, "--- ") != 2 || !strings.HasPrefix(r.Page.Lines[0], "#general  Demo Server") {
		t.Fatalf("days or title = %s", text)
	}
	// Saying something types into the box and the new line shows
	r, _ = w.Act(ctx, view.Action{Name: "send", Page: "discordweb/c/111/10", Body: "hi all"})
	if r.Kind != view.KindPage || !strings.Contains(strings.Join(r.Page.Lines, "\n"), "you: hi all") {
		t.Fatalf("say = %+v", r)
	}
}
