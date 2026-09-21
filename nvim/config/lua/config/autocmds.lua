-- Autocmds on top of LazyVim's

-- Symphony pages are text, no spell or line numbers on them, their filetypes all start with symphony-, and the options are window ones so they are set when the page lands in a window
local function quiet(win)
  vim.wo[win].number = false
  vim.wo[win].relativenumber = false
  vim.wo[win].spell = false
  vim.wo[win].signcolumn = "no"
end
vim.api.nvim_create_autocmd({ "FileType", "BufWinEnter" }, {
  callback = function(ev)
    if not (vim.bo[ev.buf].filetype or ""):match("^symphony%-") then
      return
    end
    for _, win in ipairs(vim.fn.win_findbuf(ev.buf)) do
      quiet(win)
    end
  end,
})
