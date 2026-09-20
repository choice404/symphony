package nvim

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// pluginRel is where the embedded plugin lands under the cache directory
const pluginRel = "symphony/nvim/symphony.nvim"

/**
 * InstallPlugin
 * Writes the embedded plugin tree into the cache directory and returns its path
 * @param fsys {fs.FS} - the embedded tree
 * @param root {string} - the directory inside fsys that holds lua and plugin
 * @return string, error
 **/
func InstallPlugin(fsys fs.FS, root string) (string, error) {
	// Find the cache directory
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}
	// The destination
	dest := filepath.Join(cache, pluginRel)
	// Drop whatever an older binary left there
	if err := os.RemoveAll(dest); err != nil {
		return "", fmt.Errorf("clear %s: %w", dest, err)
	}
	// Narrow the tree to the plugin root
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
		// Read the file
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}
		// Write it
		return os.WriteFile(target, data, 0o644)
	})
	// Fail when the walk failed
	if err != nil {
		return "", fmt.Errorf("install plugin: %w", err)
	}
	// Return the path
	return dest, nil
}
