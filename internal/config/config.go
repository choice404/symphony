// Package config reads ~/.config/symphony/config.toml
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// fileRel is the config path under the user config directory
const fileRel = "symphony/config.toml"

// Config is everything the app reads from the file
type Config struct {
	// The mail section
	Mail Mail `toml:"mail"`
	// The geas section
	Geas Geas `toml:"geas"`
}

// Geas is the [geas] section
type Geas struct {
	// The directory holding compiled contracts, ~ is expanded
	Contracts string `toml:"contracts"`
}

// Mail is the [mail] section
type Mail struct {
	// The Maildir root, ~ is expanded
	Maildir string `toml:"maildir"`
}

/**
 * Path
 * Returns the config file path
 * @return string, error
 **/
func Path() (string, error) {
	// The user config directory
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(dir, fileRel), nil
}

/**
 * Load
 * Reads the config file, a missing file is the empty config and a bad one is an error
 * @return Config, error
 **/
func Load() (Config, error) {
	// Find the file
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return LoadFile(path)
}

/**
 * LoadFile
 * Reads one config file by path
 * @param path {string} - the file
 * @return Config, error
 **/
func LoadFile(path string) (Config, error) {
	// The decoded config
	var c Config
	// Decode, a missing file is fine
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	// Expand the home directory in paths
	c.Mail.Maildir = expand(c.Mail.Maildir)
	c.Geas.Contracts = expand(c.Geas.Contracts)
	return c, nil
}

/**
 * expand
 * Replaces a leading ~ with the home directory
 * @param p {string} - the path
 * @return string
 **/
func expand(p string) string {
	// Leave anything that does not start with ~
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	// Find home, leave the path alone when it is unknown
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	// Join the rest onto home
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
