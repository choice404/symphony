-- The colorscheme picker and the theme file it writes to
local M = {}

-- Rewrites the colorscheme line of a theme file's text, adding one when there is none
function M.rewrite(text, name)
  local line = 'colorscheme = "' .. name .. '",'
  local out, n = text:gsub('colorscheme%s*=%s*"[^"]*",?', line, 1)
  if n == 0 then
    out = text:gsub("return%s*{", "return {\n  " .. line, 1)
  end
  return out
end

-- Writes the chosen colorscheme into lua/user/theme.lua under the running config
function M.save(name)
  local path = vim.fn.stdpath("config") .. "/lua/user/theme.lua"
  local lines = vim.fn.filereadable(path) == 1 and vim.fn.readfile(path) or { "return {", "}" }
  local text = M.rewrite(table.concat(lines, "\n"), name)
  vim.fn.mkdir(vim.fn.fnamemodify(path, ":h"), "p")
  vim.fn.writefile(vim.split(text, "\n"), path)
end

-- Opens the picker, the chosen colorscheme applies and is written to the theme file
function M.pick()
  if not (Snacks and Snacks.picker) then
    vim.ui.input({ prompt = "colorscheme: ", completion = "color" }, function(name)
      if name and name ~= "" then
        M.choose(name)
      end
    end)
    return
  end
  Snacks.picker.colorschemes({
    confirm = function(picker, item)
      picker:close()
      if item then
        M.choose(item.text)
      end
    end,
  })
end

-- Applies a colorscheme and remembers it when the running config is symphony's
function M.choose(name)
  local ok = pcall(vim.cmd.colorscheme, name)
  if not ok then
    vim.notify("symphony: no colorscheme " .. name, vim.log.levels.ERROR)
    return
  end
  if vim.g.symphony_dashboard then
    M.save(name)
    vim.notify("symphony: colorscheme " .. name .. " saved to lua/user/theme.lua")
  end
end

return M
