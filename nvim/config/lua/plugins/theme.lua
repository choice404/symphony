-- The colorscheme comes from lua/user/theme.lua rather than a name fixed here
return {
  {
    "LazyVim/LazyVim",
    opts = function(_, opts)
      opts.colorscheme = function()
        require("config.theme").apply()
      end
    end,
  },
}
