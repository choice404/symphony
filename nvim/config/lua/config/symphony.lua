-- What the config knows about the symphony that runs it
local M = {}

-- Returns the directory holding the symphony plugin, the one the binary wrote or the cache path a standalone run finds it at
function M.plugin_dir()
  return vim.env.SYMPHONY_PLUGIN or (vim.fn.stdpath("cache") .. "/symphony.nvim")
end

return M
