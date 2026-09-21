-- Project mode, working inside one directory with your own nvim setup, and coming back to the projects page on :q
local M = {}

-- The project being worked on, nil outside project mode
M.active = nil

-- The directory nvim was in before the project was entered
M.previous_cwd = nil

-- The pickers tried in order once a project is entered, the first command that exists wins
local pickers = {
  { cmd = "Telescope", run = "Telescope find_files" },
  { cmd = "FzfLua", run = "FzfLua files" },
  { cmd = "Oil", run = "Oil ." },
  { cmd = "Neotree", run = "Neotree reveal" },
  { cmd = "Explore", run = "Explore ." },
}

-- Opens whatever file picker this nvim has
local function pick()
  for _, p in ipairs(pickers) do
    if vim.fn.exists(":" .. p.cmd) == 2 then
      vim.cmd(p.run)
      return
    end
  end
end

-- Enters a project, cd into it, remember it, and open the picker
function M.enter(path, name)
  -- Remember where we were
  if not M.active then
    M.previous_cwd = vim.fn.getcwd()
  end
  M.active = { path = path, name = name or vim.fn.fnamemodify(path, ":t") }
  vim.g.symphony_project = path
  -- Move in and start on an empty buffer so the picker has somewhere to open into
  vim.cmd.cd(vim.fn.fnameescape(path))
  vim.cmd.enew()
  vim.bo.bufhidden = "wipe"
  vim.notify("symphony: project " .. M.active.name)
  pick()
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

-- Leaves project mode, or quits nvim for real when no project is active, bang forces past unsaved changes
function M.leave(bang)
  -- Outside a project this is an ordinary quit
  if not M.active then
    vim.cmd.quit({ bang = bang })
    return
  end
  -- A split just closes
  if #vim.api.nvim_tabpage_list_wins(0) > 1 then
    vim.cmd.close()
    return
  end
  -- Unsaved work stops a plain quit the way vim does
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
  -- Drop the project's buffers, go back to where we were, and show the projects page
  local path = M.active.path
  M.active = nil
  vim.g.symphony_project = nil
  require("symphony.view").open("projects")
  for _, buf in ipairs(buffers_under(path)) do
    pcall(vim.api.nvim_buf_delete, buf, { force = true })
  end
  if M.previous_cwd then
    pcall(vim.cmd.cd, vim.fn.fnameescape(M.previous_cwd))
  end
end

-- Writes the current buffer then leaves, what :wq means in a project
function M.write_and_leave(bang)
  if vim.bo.modifiable and vim.bo.buftype == "" and vim.api.nvim_buf_get_name(0) ~= "" then
    vim.cmd.write({ bang = bang })
  end
  M.leave(bang)
end

-- Installs the commands and the command line abbreviations that turn :q and :wq into a return to the projects page
function M.setup()
  vim.api.nvim_create_user_command("SymphonyQuit", function(opts)
    M.leave(opts.bang)
  end, { bang = true })
  vim.api.nvim_create_user_command("SymphonyWq", function(opts)
    M.write_and_leave(opts.bang)
  end, { bang = true })
  -- Only a bare q or wq at the start of the command line is taken, :qa and everything else stay vim's own
  vim.cmd([[cnoreabbrev <expr> q (getcmdtype() == ':' && getcmdline() ==# 'q') ? 'SymphonyQuit' : 'q']])
  vim.cmd([[cnoreabbrev <expr> wq (getcmdtype() == ':' && getcmdline() ==# 'wq') ? 'SymphonyWq' : 'wq']])
end

return M
