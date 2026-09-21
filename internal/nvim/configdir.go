package nvim

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AppName is the NVIM_APPNAME the editor runs under, so its config, data, and cache sit beside symphony's own
const AppName = "symphony/nvim"

// lockFile is the plugin pin list, written once and then left to lazy.nvim and the user
const lockFile = "lazy-lock.json"

/**
 * InstallConfig
 * Writes the embedded editor config under the user config directory and returns its path, a shipped file is written when it differs, the lock file only when missing, and any other file in the tree is left alone so user additions survive
 * @param fsys {fs.FS} - the embedded tree
 * @param root {string} - the directory inside fsys that holds init.lua
 * @return string, error
 **/
func InstallConfig(fsys fs.FS, root string) (string, error) {
	// Find the config directory
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	// The destination, what NVIM_APPNAME resolves to
	dest := filepath.Join(base, filepath.FromSlash(AppName))
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
		// The lock file is written once, later pins belong to lazy.nvim and the user
		have, readErr := os.ReadFile(target)
		if readErr == nil && (path == lockFile || bytes.Equal(have, data)) {
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
