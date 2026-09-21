-- The symphony plugin itself, from the directory the binary wrote, connected to the daemon as soon as it loads so the dashboard can ask it for counts
return {
  {
    dir = require("config.symphony").plugin_dir(),
    name = "symphony.nvim",
    lazy = false,
    priority = 1000,
    config = function()
      -- The dashboard stands in for the home page
      vim.g.symphony_dashboard = true
      require("symphony").attach()
    end,
  },
}
