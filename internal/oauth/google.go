// Package oauth logs into Google once in a browser and keeps the refresh token for the daemon
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/choice404/symphony/internal/config"
)

// clientFile is the OAuth client under the config directory
const clientFile = "google-oauth.toml"

// tokenDir is where tokens live under the config directory
const tokenDir = "tokens"

// mailScope is full IMAP access, the only scope Gmail takes for IMAP
const mailScope = "https://mail.google.com/"

// calendarScope is read and write access to the account's calendars
const calendarScope = "https://www.googleapis.com/auth/calendar"

// scopes is everything one login asks for, mail and calendar together so one browser trip covers both
var scopes = []string{mailScope, calendarScope}

// loginWait is how long the browser login may take
const loginWait = 5 * time.Minute

// Client is the OAuth client from the Google Cloud console
type Client struct {
	// The client id
	ClientID string `toml:"client_id"`
	// The client secret
	ClientSecret string `toml:"client_secret"`
}

/**
 * LoadClient
 * Reads the OAuth client from google-oauth.toml beside the config
 * @return Client, error
 **/
func LoadClient() (Client, error) {
	// The file
	dir, err := config.Dir()
	if err != nil {
		return Client{}, err
	}
	path := filepath.Join(dir, clientFile)
	// Decode
	var c Client
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Client{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return Client{}, fmt.Errorf("%s needs client_id and client_secret", path)
	}
	return c, nil
}

/**
 * tokenPath
 * Returns the token file for an account
 * @param name {string} - the account name
 * @return string, error
 **/
func tokenPath(name string) (string, error) {
	// Under tokens beside the config
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenDir, name+".json"), nil
}

/**
 * oauthConfig
 * Builds the oauth2 config for a redirect
 * @param c {Client} - the client
 * @param redirect {string} - the redirect url, empty when only refreshing
 * @return *oauth2.Config
 **/
func oauthConfig(c Client, redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
		RedirectURL:  redirect,
	}
}

/**
 * Authorize
 * Runs the browser login for an account and stores its refresh token, this is where Okta or a security key does its part
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name, names the token file
 * @param user {string} - the address, offered to Google as the login hint
 * @param out {io.Writer} - where the url and progress are printed
 * @return error
 **/
func Authorize(ctx context.Context, name, user string, out io.Writer) error {
	// The client
	client, err := LoadClient()
	if err != nil {
		return err
	}
	// A loopback listener on a free port takes the redirect
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = ln.Close() }()
	redirect := fmt.Sprintf("http://%s/", ln.Addr().String())
	cfg := oauthConfig(client, redirect)
	// The state and the PKCE verifier tie the callback to this run
	state, err := nonce()
	if err != nil {
		return err
	}
	verifier := oauth2.GenerateVerifier()
	url := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("login_hint", user),
	)
	// Show the url and try the browser
	_, _ = fmt.Fprintf(out, "open this in a browser and log in as %s:\n\n%s\n\n", user, url)
	_ = exec.Command("xdg-open", url).Start()
	// Wait for the code
	code, err := waitForCode(ctx, ln, state)
	if err != nil {
		return err
	}
	// Trade it for tokens
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fmt.Errorf("exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return errors.New("google returned no refresh token, revoke the app under myaccount.google.com/connections and authorize again")
	}
	// Keep it
	if err := saveToken(name, tok); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "authorized %s, token stored\n", name)
	return nil
}

/**
 * waitForCode
 * Serves the redirect once and returns the code it carried
 * @param ctx {context.Context} - the context
 * @param ln {net.Listener} - the loopback listener
 * @param state {string} - the state the callback must carry
 * @return string, error
 **/
func waitForCode(ctx context.Context, ln net.Listener, state string) (string, error) {
	// The code lands here
	codes := make(chan string, 1)
	fails := make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// A wrong state is someone else's callback
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		// Google reports a refusal here
		if e := q.Get("error"); e != "" {
			http.Error(w, "login failed: "+e, http.StatusBadRequest)
			fails <- fmt.Errorf("login refused: %s", e)
			return
		}
		_, _ = fmt.Fprintln(w, "symphony is authorized, you can close this tab")
		codes <- q.Get("code")
	})
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	// Wait for the code, a failure, the context, or the clock
	select {
	case code := <-codes:
		if code == "" {
			return "", errors.New("the callback carried no code")
		}
		return code, nil
	case err := <-fails:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(loginWait):
		return "", errors.New("no login within five minutes")
	}
}

/**
 * nonce
 * Returns random hex for the state
 * @return string, error
 **/
func nonce() (string, error) {
	// Sixteen random bytes
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}

/**
 * saveToken
 * Writes a token to the account's token file, private to the user
 * @param name {string} - the account name
 * @param tok {*oauth2.Token} - the token
 * @return error
 **/
func saveToken(name string, tok *oauth2.Token) error {
	// The path and its directory
	path, err := tokenPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("token dir: %w", err)
	}
	// Encode and write
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

/**
 * loadToken
 * Reads an account's token file
 * @param name {string} - the account name
 * @return *oauth2.Token, error
 **/
func loadToken(name string) (*oauth2.Token, error) {
	// The path
	path, err := tokenPath(name)
	if err != nil {
		return nil, err
	}
	// Read and decode
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no token for %s, run symphonyd mail authorize %s", name, name)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &tok, nil
}

// saving refreshes through the wrapped source and writes every new access token back to the file
type saving struct {
	// The account name
	name string
	// The refreshing source
	src oauth2.TokenSource
	// The last access token written
	last string
}

/**
 * Token
 * Returns a valid token, saving it when it was refreshed
 * @return *oauth2.Token, error
 **/
func (s *saving) Token() (*oauth2.Token, error) {
	// Refresh when needed
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	// Persist a new one
	if tok.AccessToken != s.last {
		s.last = tok.AccessToken
		if err := saveToken(s.name, tok); err != nil {
			return nil, err
		}
	}
	return tok, nil
}

/**
 * HTTPClient
 * Returns an http client that signs every request with the account's token, refreshing and saving it as needed
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name
 * @return *http.Client, error
 **/
func HTTPClient(ctx context.Context, name string) (*http.Client, error) {
	// The client and the stored token
	client, err := LoadClient()
	if err != nil {
		return nil, err
	}
	tok, err := loadToken(name)
	if err != nil {
		return nil, err
	}
	// A refreshing source that saves, wrapped in a client
	src := &saving{name: name, src: oauthConfig(client, "").TokenSource(ctx, tok), last: tok.AccessToken}
	return oauth2.NewClient(ctx, src), nil
}

/**
 * AccessToken
 * Returns a valid access token for an account, refreshing it silently when it expired
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name
 * @return string, error
 **/
func AccessToken(ctx context.Context, name string) (string, error) {
	// The client and the stored token
	client, err := LoadClient()
	if err != nil {
		return "", err
	}
	tok, err := loadToken(name)
	if err != nil {
		return "", err
	}
	// A refreshing source that saves
	src := &saving{name: name, src: oauthConfig(client, "").TokenSource(ctx, tok), last: tok.AccessToken}
	fresh, err := src.Token()
	if err != nil {
		return "", fmt.Errorf("refresh %s: %w", name, err)
	}
	return fresh.AccessToken, nil
}
