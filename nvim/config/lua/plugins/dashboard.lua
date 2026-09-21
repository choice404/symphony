-- The landing screen, the apps with their live summaries from the daemon in place of LazyVim's file keys
local header = [[
 ____                        _
/ ___| _   _ _ __ ___  _ __ | |__   ___  _ __  _   _
\___ \| | | | '_ ` _ \| '_ \| '_ \ / _ \| '_ \| | | |
 ___) | |_| | | | | | | |_) | | | | (_) | | | | |_| |
|____/ \__, |_| |_| |_| .__/|_| |_|\___/|_| |_|\__, |
       |___/          |_|                      |___/
]]

return {
  {
    "folke/snacks.nvim",
    opts = function(_, opts)
      opts.dashboard = opts.dashboard or {}
      opts.dashboard.preset = opts.dashboard.preset or {}
      opts.dashboard.preset.header = header
      -- Wide enough for a label, a summary, and the key on one line
      opts.dashboard.width = 84
      opts.dashboard.sections = {
        { section = "header" },
        -- The apps, read from the daemon each time the dashboard draws
        function()
          return require("symphony.dashboard").section()
        end,
        { section = "startup" },
      }
    end,
  },
  {
    -- No news windows, the daemon owns the surprises
    "LazyVim/LazyVim",
    opts = { news = { lazyvim = false, neovim = false } },
  },
}
