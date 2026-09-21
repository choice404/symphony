package calendar

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/gcal"
)

// Cached is what one fetch left behind for one account
type Cached struct {
	// When the fetch ran
	FetchedAt time.Time `json:"fetched_at"`
	// The window it covered
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	// The events in start order
	Events []gcal.Event `json:"events"`
}

// Store keeps one cache file per account under a directory so pages read from disk and never wait on the network
type Store struct {
	// The directory
	dir string
	// Guards the files
	mu sync.Mutex
}

/**
 * NewStore
 * Builds a store under a directory
 * @param dir {string} - the directory, created on the first save
 * @return *Store
 **/
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

/**
 * path
 * Returns an account's cache file
 * @param account {string} - the account name
 * @return string
 **/
func (s *Store) path(account string) string {
	return filepath.Join(s.dir, account+".json")
}

/**
 * Load
 * Reads an account's cache, an empty one with no fetch time when there is none
 * @param account {string} - the account name
 * @return Cached, error
 **/
func (s *Store) Load(account string) (Cached, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Read when present
	data, err := os.ReadFile(s.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return Cached{}, nil
	}
	if err != nil {
		return Cached{}, fmt.Errorf("calendar cache: %w", err)
	}
	var c Cached
	if err := json.Unmarshal(data, &c); err != nil {
		return Cached{}, fmt.Errorf("calendar cache: %w", err)
	}
	return c, nil
}

/**
 * Save
 * Writes an account's cache through a temp file
 * @param account {string} - the account name
 * @param c {Cached} - the cache
 * @return error
 **/
func (s *Store) Save(account string, c Cached) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The directory
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("calendar cache: %w", err)
	}
	// Encode and write
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(account) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("calendar cache: %w", err)
	}
	return os.Rename(tmp, s.path(account))
}
