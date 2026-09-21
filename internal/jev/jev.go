// Package jev asks TypeSafe's Jev model for typed judgments over a piece of state
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/choice404/symphony/internal/config"
)

// endpoint is the System One endpoint
const endpoint = "https://api.typesafe.ai/v1/systemone"

// model is the model alias every request names
const model = "jev-latest"

// keyFile is where the API key lives beside the config when the env has none
const keyFile = "typesafe.key"

// EnvKey is the env var that carries the API key
const EnvKey = "TYPESAFE_API_KEY"

// retries is how many times a rate limited request is tried
const retries = 4

// Question is one judgment to ask, a Noul when Criteria is a yes and no pair
type Question struct {
	// noul, choice, or score
	Type string `json:"type"`
	// What to decide
	Instructions interface{} `json:"instructions"`
	// What the answers mean, shape by type
	Criteria interface{} `json:"criteria,omitempty"`
}

// Answer is one judgment back
type Answer struct {
	// The type it answers
	Type string `json:"type"`
	// The probability of yes for a noul
	Noul float64 `json:"noul"`
	// The winning option for a choice
	Choice string `json:"choice"`
	// Every option's probability for a choice
	Probabilities map[string]float64 `json:"probabilities"`
	// The weighted value for a score
	Score float64 `json:"score"`
	// How concentrated a choice or score was
	Confidence float64 `json:"confidence"`
}

// Client talks to the API with one key
type Client struct {
	// The API key
	key string
	// The http client with a timeout
	http *http.Client
}

/**
 * LoadKey
 * Finds the API key in the env or in typesafe.key beside the config, empty when there is none
 * @return string
 **/
func LoadKey() string {
	// The env wins
	if k := strings.TrimSpace(os.Getenv(EnvKey)); k != "" {
		return k
	}
	// The file beside the config
	dir, err := config.Dir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, keyFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

/**
 * New
 * Builds a client, nil when there is no key so callers can leave the feature off
 * @param key {string} - the API key
 * @return *Client
 **/
func New(key string) *Client {
	if key == "" {
		return nil
	}
	return &Client{key: key, http: &http.Client{Timeout: 30 * time.Second}}
}

/**
 * Ask
 * Sends one state with its questions and returns the answers by id, backing off on a rate limit
 * @param ctx {context.Context} - the context
 * @param state {interface{}} - the state, a string or a map
 * @param questions {map[string]Question} - the questions by id
 * @return map[string]Answer, error
 **/
func (c *Client) Ask(ctx context.Context, state interface{}, questions map[string]Question) (map[string]Answer, error) {
	// The request body
	body, err := json.Marshal(map[string]interface{}{"state": state, "model": model, "questions": questions})
	if err != nil {
		return nil, err
	}
	// Try with backoff
	wait := time.Second
	for attempt := 0; attempt < retries; attempt++ {
		answers, retry, err := c.once(ctx, body)
		if err == nil {
			return answers, nil
		}
		if !retry {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
			wait *= 2
		}
	}
	return nil, errors.New("jev: rate limited, gave up")
}

/**
 * once
 * Sends the request once, the second result says whether the failure is worth a retry
 * @param ctx {context.Context} - the context
 * @param body {[]byte} - the encoded request
 * @return map[string]Answer, bool, error
 **/
func (c *Client) once(ctx context.Context, body []byte) (map[string]Answer, bool, error) {
	// The request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	// Send
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, true, err
	}
	// A rate limit or an overload is retried, anything else not ok is final
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 529 {
		return nil, true, fmt.Errorf("jev: %s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("jev: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	// The answers
	var out struct {
		Answers map[string]Answer `json:"answers"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false, fmt.Errorf("jev: %w", err)
	}
	return out.Answers, false, nil
}
