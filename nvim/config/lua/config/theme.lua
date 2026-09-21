-- The colors from lua/user/theme.lua, applied at start and again after every write
local M = {}

-- Reads the theme file fresh, an unreadable one is empty
function M.load()
  package.loaded["user.theme"] = nil
  local ok, theme = pcall(require, "user.theme")
  if not ok or type(theme) ~= "table" then
    return {}
  end
  return theme
end

-- Sets the highlight groups the theme names on top of the colorscheme
function M.highlights(theme)
  for name, spec in pairs(theme.highlights or {}) do
    pcall(vim.api.nvim_set_hl, 0, name, spec)
  end
end

-- Applies the background, the colorscheme, and the highlights, a missing colorscheme is reported and the current one stays
function M.apply()
  local theme = M.load()
  if theme.background then
    vim.o.background = theme.background
  end
  if theme.colorscheme then
    local ok = pcall(vim.cmd.colorscheme, theme.colorscheme)
    if not ok then
      vim.notify("symphony: no colorscheme " .. theme.colorscheme .. ", add its plugin in lua/user/plugins.lua", vim.log.levels.WARN)
    end
  end
  M.highlights(theme)
end

-- Keeps the highlights on top of whatever colorscheme loads next
vim.api.nvim_create_autocmd("ColorScheme", {
  group = vim.api.nvim_create_augroup("symphony_theme", { clear = true }),
  callback = function()
    M.highlights(M.load())
  end,
})

return M
