-- Pages from the host shown as buffers, and actions on them sent back
local M = {}

-- The transport to the host
local rpc = require("symphony.rpc")

-- The prefix of every symphony buffer name
M.prefix = "symphony://"

-- The buffers left behind by show, newest last, so back climbs the way you came
M.history = {}

-- Finds the buffer for a page name or creates a scratch one
local function buffer_for(name)
  -- The buffer name
  local bufname = M.prefix .. name
  -- Reuse an existing buffer
  local existing = vim.fn.bufnr(bufname)
  if existing ~= -1 and vim.api.nvim_buf_is_valid(existing) then
    return existing
  end
  -- Create a listed scratch buffer
  local buf = vim.api.nvim_create_buf(true, true)
  vim.api.nvim_buf_set_name(buf, bufname)
  return buf
end

-- The view name of a page, the part of its name before any slash
local function view_of(name)
  return name:match("^[^/]+") or name
end

-- Sets the buffer local keymaps every page gets
function M.keymaps(buf)
  -- Maps one key in normal mode for this buffer only
  local function map(lhs, fn)
    vim.keymap.set("n", lhs, fn, { buffer = buf, nowait = true, silent = true })
  end
  -- Open the item under the cursor
  map("<CR>", function()
    M.act("open")
  end)
  -- Render the page again
  map("r", function()
    M.act("refresh")
  end)
  -- Go back to the previous buffer
  map("q", M.back)
  -- Jump home
  map("gh", function()
    M.open("home")
  end)
end

-- Shows a page in its buffer and returns the buffer number
function M.show(page)
  -- The buffer
  local buf = buffer_for(page.name)
  -- The lines and keys, empty when the host sent none
  local lines = page.lines
  if lines == vim.NIL or lines == nil then
    lines = {}
  end
  local keys = page.keys
  if keys == vim.NIL or keys == nil then
    keys = {}
  end
  -- Replace the content while the buffer is writable
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
  -- The options of a view buffer
  vim.bo[buf].buftype = "nofile"
  vim.bo[buf].bufhidden = "hide"
  vim.bo[buf].swapfile = false
  vim.bo[buf].filetype = "symphony-" .. (page.filetype or "page")
  -- Remember the view, the keys, and the title on the buffer
  vim.b[buf].symphony_view = view_of(page.name)
  vim.b[buf].symphony_keys = keys
  vim.b[buf].symphony_title = page.title
  -- Set the keymaps
  M.keymaps(buf)
  -- Remember the symphony buffer being left, a refresh of the same page leaves nothing
  local cur = vim.api.nvim_get_current_buf()
  if cur ~= buf and vim.b[cur].symphony_view then
    table.insert(M.history, cur)
  end
  -- Show it
  vim.api.nvim_set_current_buf(buf)
  -- Put the cursor on the page's line, clamped to the buffer
  local row = math.min(math.max((page.cursor or 0) + 1, 1), math.max(#lines, 1))
  vim.api.nvim_win_set_cursor(0, { row, 0 })
  return buf
end

-- Applies a response from the host
function M.apply(resp)
  -- Nothing to do
  if resp == nil or resp == vim.NIL or resp.kind == "none" then
    return
  end
  -- A page is shown
  if resp.kind == "page" then
    M.show(resp.page)
    return
  end
  -- A message is shown
  if resp.kind == "notify" then
    local level = resp.error and vim.log.levels.ERROR or vim.log.levels.INFO
    vim.notify(resp.text, level)
  end
end

-- Asks the host for a view and shows it
function M.open(name)
  -- Ask
  local ok, page = pcall(rpc.request, "symphony.render", name)
  -- Report a failure
  if not ok then
    vim.notify(tostring(page), vim.log.levels.ERROR)
    return nil
  end
  -- Show it
  return M.show(page)
end

-- Sends an action on the current line to the host and applies the reply
function M.act(action)
  -- The current buffer and its view
  local buf = vim.api.nvim_get_current_buf()
  local view = vim.b[buf].symphony_view
  -- Not a symphony buffer
  if not view then
    return
  end
  -- The key of the cursor line
  local line = vim.api.nvim_win_get_cursor(0)[1]
  local keys = vim.b[buf].symphony_keys or {}
  local key = keys[line] or ""
  -- Ask
  local ok, resp = pcall(rpc.request, "symphony.action", view, action, key)
  -- Report a failure
  if not ok then
    vim.notify(tostring(resp), vim.log.levels.ERROR)
    return
  end
  -- Apply the reply
  M.apply(resp)
end

-- Goes back along the history, or home when there is nothing left
function M.back()
  -- Pop until a live buffer turns up
  while #M.history > 0 do
    local prev = table.remove(M.history)
    if prev ~= vim.api.nvim_get_current_buf() and vim.api.nvim_buf_is_valid(prev) then
      vim.api.nvim_set_current_buf(prev)
      return
    end
  end
  -- Nothing behind, go home unless already there
  if vim.b[0].symphony_view ~= "home" then
    M.open("home")
  end
end

return M
