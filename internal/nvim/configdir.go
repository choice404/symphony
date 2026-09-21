package nvim

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// AppName is the NVIM_APPNAME the editor runs under, so its config, data, and cache sit beside symphony's own
const AppName = "symphony/nvim"

// lockFile is the plugin pin list, written once and then left to lazy.nvim and the user
const lockFile = "lazy-lock.json"

// UserDir is the directory of starter files that belong to the user once written
const UserDir = "lua/user/"

/**
 * WriteOnce
 * Reports whether a shipped file is written only when missing, the lock file and everything under the user directory
 * @param path {string} - the path inside the config tree, slash separated
 * @return bool
 **/
func WriteOnce(path string) bool {
	return path == lockFile || strings.HasPrefix(path, UserDir)
}

/**
 * ConfigDir
 * Returns where the editor config lives, whether or not it has been written yet
 * @return string, error
 **/
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(base, filepath.FromSlash(AppName)), nil
}

/**
 * InstallConfig
 * Writes the embedded editor config under the user config directory and returns its path, a shipped file is written when it differs, the lock file and the user starters only when missing, and any other file in the tree is left alone so user additions survive
 * @param fsys {fs.FS} - the embedded tree
 * @param root {string} - the directory inside fsys that holds init.lua
 * @return string, error
 **/
func InstallConfig(fsys fs.FS, root string) (string, error) {
	// The destination, what NVIM_APPNAME resolves to
	dest, err := ConfigDir()
	if err != nil {
		return "", err
	}
	// Narrow the tree to the config root
	sub, err := fs.Sub(fsys, root)
	if err != nil {
		return "", fmt.Errorf("sub %s: %w", root, err)
	}
	// Walk every entry
	err = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		// Stop on a walk error
		if err != nil {
			return err
		}
		// The path on disk
		target := filepath.Join(dest, filepath.FromSlash(path))
		// Make directories
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Read the shipped file
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}
		// A write once file belongs to the user after the first write, anything else is refreshed when it changed
		have, readErr := os.ReadFile(target)
		if readErr == nil && (WriteOnce(path) || bytes.Equal(have, data)) {
			return nil
		}
		// Write the shipped file
		return os.WriteFile(target, data, 0o644)
	})
	// Fail when the walk failed
	if err != nil {
		return "", fmt.Errorf("install config: %w", err)
	}
	// Return the path
	return dest, nil
}
