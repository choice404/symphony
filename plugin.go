// Package symphony holds the embedded nvim plugin so the binary can install it on its own
package symphony

import "embed"

// PluginRoot is the path of the plugin inside PluginFS
const PluginRoot = "nvim/symphony.nvim"

// PluginFS is the lua plugin tree, embedded so the TUI never depends on an install
//
//go:embed nvim/symphony.nvim/lua nvim/symphony.nvim/plugin
var PluginFS embed.FS

// ConfigRoot is the path of the editor config inside ConfigFS
const ConfigRoot = "nvim/config"

// ConfigFS is the editor config, a LazyVim based distro the binary writes out and runs under its own NVIM_APPNAME
//
//go:embed nvim/config
var ConfigFS embed.FS
