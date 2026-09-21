package mail

import (
	"strings"
	"testing"
	"time"
)

func TestParseComposeBuildsMessage(t *testing.T) {
	text := strings.Join([]string{
		composeHint,
		"From: Me <me@example.com>",
		"To: Ada <ada@example.com>, bo@example.com",
		"Cc: cy@example.com",
		"Subject: héllo",
		composeMarker,
		"line one",
		"line two",
		"",
	}, "\n")
	d := Draft{Account: "p", InReplyTo: "<1@example.com>", References: "<0@example.com> <1@example.com>"}
	out, err := parseCompose(text, d, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if out.From != "me@example.com" || strings.Join(out.Recipients, ",") != "ada@example.com,bo@example.com,cy@example.com" || out.Subject != "héllo" {
		t.Fatalf("out = %+v", out)
	}
	raw := string(out.Raw)
	for _, want := range []string{
		"From: Me <me@example.com>\r\n",
		"To: Ada <ada@example.com>, bo@example.com\r\n",
		"Cc: cy@example.com\r\n",
		"Subject: =?utf-8?q?h=C3=A9llo?=\r\n",
		"Date: Fri, 02 Jan 2026 03:04:05 +0000\r\n",
		"In-Reply-To: <1@example.com>\r\n",
		"References: <0@example.com> <1@example.com>\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\nline one\r\nline two\r\n",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw missing %q in %q", want, raw)
		}
	}
	if !strings.Contains(raw, "Message-ID: <") || !strings.Contains(raw, "@example.com>\r\n") {
		t.Fatalf("no message id in %q", raw)
	}
}

func TestParseComposeErrors(t *testing.T) {
	base := composeHint + "\nFrom: me@example.com\nTo: %s\nCc: \nSubject: x\n" + composeMarker + "\nhi\n"
	if _, err := parseCompose(strings.Replace(base, "%s", "", 1), Draft{}, time.Now()); err == nil || !strings.Contains(err.Error(), "to is empty") {
		t.Fatalf("empty to = %v", err)
	}
	if _, err := parseCompose(strings.Replace(base, "%s", "not an address", 1), Draft{}, time.Now()); err == nil {
		t.Fatal("bad address accepted")
	}
	if _, err := parseCompose("From: me@example.com\nTo: a@b.c\nSubject: x\nhi\n", Draft{}, time.Now()); err == nil || !strings.Contains(err.Error(), "marker") {
		t.Fatalf("missing marker = %v", err)
	}
}

func TestSubjectsAndBodies(t *testing.T) {
	if replySubject("hello") != "Re: hello" || replySubject("RE: hello") != "RE: hello" {
		t.Fatal("reply subject")
	}
	if forwardSubject("hello") != "Fwd: hello" || forwardSubject("Fwd: hello") != "Fwd: hello" {
		t.Fatal("forward subject")
	}
	o := Opened{Message: Message{From: "Ada <ada@example.com>", Subject: "s", Date: time.Unix(0, 0).UTC()}, To: "me", Body: "a\nb"}
	if q := replyBody(o); !strings.Contains(q, "> a\n> b\n") || !strings.Contains(q, "Ada <ada@example.com> wrote:") {
		t.Fatalf("reply body = %q", q)
	}
	if f := forwardBody(o); !strings.Contains(f, "Forwarded message") || !strings.Contains(f, "From: Ada") || !strings.HasSuffix(f, "a\nb\n") {
		t.Fatalf("forward body = %q", f)
	}
}
