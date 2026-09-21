// Package spam asks Jev whether inbox messages are spam and remembers the answers
package spam

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/choice404/symphony/internal/jev"
	"github.com/choice404/symphony/internal/mail"
)

// cacheFile is where judged probabilities live under the cache directory
const cacheFile = "spam.json"

// snippetLen is how much of a body the judge sees
const snippetLen = 600

// parallel is how many messages are judged at once
const parallel = 4

// question is the one judgment asked per message
var question = map[string]jev.Question{
	"spam": {
		Type:         "noul",
		Instructions: "Is this email spam, unsolicited bulk mail, a scam, or a phishing attempt that the recipient would want deleted without reading?",
		Criteria: map[string]interface{}{
			"true":  "Unsolicited marketing from a sender the recipient never dealt with, a scam, a phishing lure, a fake notice, or bulk mail with no relationship behind it",
			"false": "Mail from a person, a service the recipient uses, a receipt, a notification the recipient signed up for, a newsletter the recipient subscribed to, or anything else they would expect",
		},
	},
}

// Judge scores messages and caches the scores by message key
type Judge struct {
	// The client
	client *jev.Client
	// Guards the cache
	mu sync.Mutex
	// The scores by account and key
	cache map[string]float64
	// Where the cache is written
	path string
}

/**
 * New
 * Builds a judge over a client with a cache under a directory, nil when there is no client
 * @param client {*jev.Client} - the client, nil for none
 * @param cacheDir {string} - the directory the cache file lives in
 * @return *Judge
 **/
func New(client *jev.Client, cacheDir string) *Judge {
	// No client, no judge
	if client == nil {
		return nil
	}
	j := &Judge{client: client, cache: map[string]float64{}, path: filepath.Join(cacheDir, cacheFile)}
	// Load what an earlier run judged
	if data, err := os.ReadFile(j.path); err == nil {
		_ = json.Unmarshal(data, &j.cache)
	}
	return j
}

/**
 * Score
 * Returns the spam probability of every message, asking Jev only for the ones not judged before
 * @param ctx {context.Context} - the context
 * @param a {mail.Account} - the account
 * @param msgs {[]mail.Message} - the messages
 * @return map[string]float64, error
 **/
func (j *Judge) Score(ctx context.Context, a mail.Account, msgs []mail.Message) (map[string]float64, error) {
	// What is cached and what is not
	out := map[string]float64{}
	todo := make([]mail.Message, 0, len(msgs))
	j.mu.Lock()
	for _, m := range msgs {
		if p, ok := j.cache[m.Key()]; ok {
			out[m.Key()] = p
		} else {
			todo = append(todo, m)
		}
	}
	j.mu.Unlock()
	// Judge the rest a few at a time
	type scored struct {
		key string
		p   float64
		err error
	}
	results := make(chan scored, len(todo))
	sem := make(chan struct{}, parallel)
	for _, m := range todo {
		sem <- struct{}{}
		go func(m mail.Message) {
			defer func() { <-sem }()
			p, err := j.one(ctx, a, m)
			results <- scored{key: m.Key(), p: p, err: err}
		}(m)
	}
	// Collect, the first error is reported after the rest land
	var firstErr error
	for range todo {
		r := <-results
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		out[r.key] = r.p
		j.mu.Lock()
		j.cache[r.key] = r.p
		j.mu.Unlock()
	}
	// Persist what was learned
	if len(todo) > 0 {
		j.save()
	}
	return out, firstErr
}

/**
 * one
 * Judges one message from its headers and a body snippet
 * @param ctx {context.Context} - the context
 * @param a {mail.Account} - the account
 * @param m {mail.Message} - the message
 * @return float64, error
 **/
func (j *Judge) one(ctx context.Context, a mail.Account, m mail.Message) (float64, error) {
	// The snippet, empty when the body cannot be read
	snippet := ""
	if o, err := mail.Open(m.Path); err == nil {
		snippet = o.Body
		if len(snippet) > snippetLen {
			snippet = snippet[:snippetLen]
		}
	}
	// The state the judge sees
	state := map[string]interface{}{
		"recipient": a.User,
		"from":      m.From,
		"subject":   m.Subject,
		"date":      m.Date.Format("2006-01-02"),
		"body":      snippet,
	}
	answers, err := j.client.Ask(ctx, state, question)
	if err != nil {
		return 0, err
	}
	ans, ok := answers["spam"]
	if !ok {
		return 0, fmt.Errorf("jev: no answer")
	}
	return ans.Noul, nil
}

/**
 * save
 * Writes the cache
 * @return void
 **/
func (j *Judge) save() {
	j.mu.Lock()
	data, err := json.Marshal(j.cache)
	j.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(j.path), 0o700)
	_ = os.WriteFile(j.path, data, 0o600)
}
