-- Project mode, a tab of its own with the project's file tree, files opened from it, and :q climbing back to the tree and then to the projects page
local M = {}

-- The project being worked on, nil outside project mode, with its path, name, and tab
M.active = nil

-- The pickers tried in order for f on the tree page, the first command that exists wins
local pickers = {
  { cmd = "Telescope", run = "Telescope find_files" },
  { cmd = "FzfLua", run = "FzfLua files" },
  { cmd = "Oil", run = "Oil ." },
  { cmd = "Neotree", run = "Neotree reveal" },
  { cmd = "Explore", run = "Explore ." },
}

-- Opens whatever file picker this nvim has
function M.pick()
  for _, p in ipairs(pickers) do
    if vim.fn.exists(":" .. p.cmd) == 2 then
      vim.cmd(p.run)
      return
    end
  end
end

-- The tree page name of a project
local function tree_page(path)
  return "projects/tree/" .. path
end

-- Enters a project in a new tab, the tab's directory set to it and its tree page shown
function M.enter(path, name)
  -- Leave whatever project was open first
  if M.active then
    M.leave(true)
  end
  vim.cmd.tabnew()
  M.active = { path = path, name = name or vim.fn.fnamemodify(path, ":t"), tab = vim.api.nvim_get_current_tabpage() }
  vim.g.symphony_project = path
  vim.cmd.tcd(vim.fn.fnameescape(path))
  -- The tree page in this tab
  require("symphony.view").open(tree_page(path))
  -- ZZ and ZQ climb back the same way :q does
  vim.keymap.set("n", "ZZ", function()
    M.write_and_leave(false)
  end, { desc = "symphony: write and go back" })
  vim.keymap.set("n", "ZQ", function()
    M.leave(true)
  end, { desc = "symphony: go back without writing" })
  vim.notify("symphony: project " .. M.active.name .. ", :q on a file returns to the tree, :q on the tree returns to projects")
end

-- The windows of the current tab that are not floating, the only ones :q cares about
local function real_windows()
  local out = {}
  for _, win in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
    if vim.api.nvim_win_get_config(win).relative == "" then
      table.insert(out, win)
    end
  end
  return out
end

-- The listed buffers whose file sits under a path
local function buffers_under(path)
  local out = {}
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    local name = vim.api.nvim_buf_get_name(buf)
    if vim.bo[buf].buflisted and name ~= "" and name:sub(1, #path + 1) == path .. "/" then
      table.insert(out, buf)
    end
  end
  return out
end

-- Whether the current buffer is the active project's tree page
local function on_tree()
  return M.active ~= nil and vim.b[0].symphony_page == tree_page(M.active.path)
end

-- Closes the project for good, its tab, its buffers, its maps, and shows the projects page where the tab came from
local function close_project()
  local path = M.active.path
  local tab = M.active.tab
  M.active = nil
  vim.g.symphony_project = nil
  pcall(vim.keymap.del, "n", "ZZ")
  pcall(vim.keymap.del, "n", "ZQ")
  for _, buf in ipairs(buffers_under(path)) do
    pcall(vim.api.nvim_buf_delete, buf, { force = true })
  end
  -- Close the tab when more than one exists, otherwise just reuse it
  if vim.api.nvim_tabpage_is_valid(tab) and #vim.api.nvim_list_tabpages() > 1 then
    local cur = vim.api.nvim_get_current_tabpage()
    vim.api.nvim_set_current_tabpage(tab)
    vim.cmd.tabclose()
    if cur ~= tab and vim.api.nvim_tabpage_is_valid(cur) then
      vim.api.nvim_set_current_tabpage(cur)
    end
  end
  require("symphony.view").open("projects")
end

-- Leaves one level, a file goes back to the tree and the tree goes back to the projects page, bang forces past unsaved changes, no project means an ordinary quit
function M.leave(bang)
  -- Outside a project, a file opened from a page goes back to that page, anything else is an ordinary quit
  if not M.active then
    if vim.b[0].symphony_return then
      M.return_to_page(bang)
      return
    end
    vim.cmd.quit({ bang = bang })
    return
  end
  -- Another tab is not the project's business
  if vim.api.nvim_get_current_tabpage() ~= M.active.tab then
    vim.cmd.quit({ bang = bang })
    return
  end
  -- A floating window such as a picker or a popup just closes, and a split just closes
  if vim.api.nvim_win_get_config(0).relative ~= "" then
    pcall(vim.api.nvim_win_close, 0, bang)
    return
  end
  if #real_windows() > 1 then
    vim.cmd.close({ bang = bang })
    return
  end
  -- On the tree page, leaving means closing the project
  if on_tree() then
    local dirty = {}
    for _, buf in ipairs(buffers_under(M.active.path)) do
      if vim.bo[buf].modified then
        table.insert(dirty, vim.fn.fnamemodify(vim.api.nvim_buf_get_name(buf), ":."))
      end
    end
    if #dirty > 0 and not bang then
      vim.notify("E37: No write since last change in " .. table.concat(dirty, ", ") .. " (add ! to override)", vim.log.levels.ERROR)
      return
    end
    close_project()
    return
  end
  -- On a file, leaving means back to the tree, the file's buffer dropped
  local buf = vim.api.nvim_get_current_buf()
  if vim.bo[buf].modified and not bang then
    vim.notify("E37: No write since last change (add ! to override)", vim.log.levels.ERROR)
    return
  end
  require("symphony.view").open(tree_page(M.active.path))
  if vim.bo[buf].buftype == "" and vim.api.nvim_buf_get_name(buf) ~= "" then
    pcall(vim.api.nvim_buf_delete, buf, { force = true })
  end
end

-- Leaves a file that a page opened, such as the config, back to that page with the file's buffer dropped, bang forces past unsaved changes
function M.return_to_page(bang)
  -- A floating window or a split just closes
  if vim.api.nvim_win_get_config(0).relative ~= "" then
    pcall(vim.api.nvim_win_close, 0, bang)
    return
  end
  if #real_windows() > 1 then
    vim.cmd.close({ bang = bang })
    return
  end
  local buf = vim.api.nvim_get_current_buf()
  if vim.bo[buf].modified and not bang then
    vim.notify("E37: No write since last change (add ! to override)", vim.log.levels.ERROR)
    return
  end
  local page = vim.b[buf].symphony_return
  require("symphony.view").open(page)
  if vim.bo[buf].buftype == "" and vim.api.nvim_buf_get_name(buf) ~= "" then
    pcall(vim.api.nvim_buf_delete, buf, { force = true })
  end
end

-- Writes the current buffer then leaves one level, what :wq means in a project
function M.write_and_leave(bang)
  if vim.bo.modifiable and vim.bo.buftype == "" and vim.api.nvim_buf_get_name(0) ~= "" then
    vim.cmd.write({ bang = bang })
  end
  M.leave(bang)
end

-- Installs the commands and the command line abbreviations that turn :q, :wq, and :x into a climb back
function M.setup()
  vim.api.nvim_create_user_command("SymphonyQuit", function(opts)
    M.leave(opts.bang)
  end, { bang = true })
  vim.api.nvim_create_user_command("SymphonyWq", function(opts)
    M.write_and_leave(opts.bang)
  end, { bang = true })
  -- Only a bare q, wq, or x at the start of the command line is taken, :qa and everything else stay vim's own
  vim.cmd([[cnoreabbrev <expr> q (getcmdtype() == ':' && getcmdline() ==# 'q') ? 'SymphonyQuit' : 'q']])
  vim.cmd([[cnoreabbrev <expr> wq (getcmdtype() == ':' && getcmdline() ==# 'wq') ? 'SymphonyWq' : 'wq']])
  vim.cmd([[cnoreabbrev <expr> x (getcmdtype() == ':' && getcmdline() ==# 'x') ? 'SymphonyWq' : 'x']])
end

return M
