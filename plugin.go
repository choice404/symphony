// Package symphony holds the embedded nvim plugin so the binary can install it on its own
package symphony

import "embed"

// PluginRoot is the path of the plugin inside PluginFS
const PluginRoot = "nvim/symphony.nvim"

// PluginFS is the lua plugin tree, embedded so the TUI never depends on an install
//
//go:embed nvim/symphony.nvim/lua nvim/symphony.nvim/plugin
var PluginFS embed.FS
