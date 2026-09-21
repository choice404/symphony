package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve stands in for the API, answering from a handler at a test endpoint
func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("k")
	c.http = srv.Client()
	// Point every request at the test server by rewriting the transport
	c.http.Transport = rewrite{base: srv.URL, next: srv.Client().Transport}
	return c
}

// rewrite sends every request to the test server whatever host it named
type rewrite struct {
	base string
	next http.RoundTripper
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme = "http"
	u.Host = strings.TrimPrefix(r.base, "http://")
	req.URL = &u
	return r.next.RoundTrip(req)
}

func TestNewWithoutKey(t *testing.T) {
	if New("") != nil {
		t.Fatal("no key should give no client")
	}
}

func TestAskDecodesAnswers(t *testing.T) {
	var got map[string]interface{}
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"model":"jev-1","answers":{"spam":{"type":"noul","noul":0.91}},"usage":{}}`))
	})
	answers, err := c.Ask(context.Background(), map[string]string{"subject": "buy now"}, map[string]Question{
		"spam": {Type: "noul", Instructions: "Is this spam?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if answers["spam"].Noul != 0.91 {
		t.Fatalf("answers = %+v", answers)
	}
	if got["model"] != "jev-latest" || got["state"].(map[string]interface{})["subject"] != "buy now" {
		t.Fatalf("request = %v", got)
	}
}

func TestAskRetriesRateLimit(t *testing.T) {
	calls := 0
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"answers":{"q":{"type":"noul","noul":0.2}}}`))
	})
	answers, err := c.Ask(context.Background(), "x", map[string]Question{"q": {Type: "noul", Instructions: "?"}})
	if err != nil || answers["q"].Noul != 0.2 || calls != 2 {
		t.Fatalf("answers = %+v err = %v calls = %d", answers, err, calls)
	}
}

func TestAskFinalError(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	if _, err := c.Ask(context.Background(), "x", map[string]Question{"q": {Type: "noul", Instructions: "?"}}); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("err = %v", err)
	}
}
