-- Where lazy.nvim lives under this app's data directory
local lazypath = vim.fn.stdpath("data") .. "/lazy/lazy.nvim"

-- Clone lazy.nvim on the first run, a failure is reported and the editor carries on with the symphony plugin alone
if not (vim.uv or vim.loop).fs_stat(lazypath) then
  local out = vim.fn.system({ "git", "clone", "--filter=blob:none", "--branch=stable", "https://github.com/folke/lazy.nvim.git", lazypath })
  if vim.v.shell_error ~= 0 then
    vim.schedule(function()
      vim.notify("symphony: could not fetch lazy.nvim, the editor runs bare until the network is back\n" .. out, vim.log.levels.ERROR)
    end)
    vim.opt.runtimepath:prepend(require("config.symphony").plugin_dir())
    return
  end
end
vim.opt.runtimepath:prepend(lazypath)

-- The plugins, LazyVim first so its defaults load before anything overrides them, then symphony's own specs, then anything you drop into lua/plugins
require("lazy").setup({
  spec = {
    { "LazyVim/LazyVim", import = "lazyvim.plugins" },
    { import = "plugins" },
  },
  defaults = {
    -- LazyVim plugins lazy load, symphony's own load at start
    lazy = false,
    -- Pin by lazy-lock.json rather than a release tag
    version = false,
  },
  install = { colorscheme = { "tokyonight", "habamax" } },
  checker = {
    -- Look for updates quietly, :Lazy shows them
    enabled = true,
    notify = false,
  },
  performance = {
    rtp = {
      disabled_plugins = { "gzip", "tarPlugin", "tohtml", "tutor", "zipPlugin" },
    },
  },
})
