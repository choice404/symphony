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

-- The keymaps every page gets, then the ones its filetype adds, each sends an action with an explicit key or the line's key
local keymaps = {
  common = {
    ["<CR>"] = { "open" },
    ["r"] = { "refresh" },
  },
  mail = {
    ["c"] = { "compose" },
    ["R"] = { "reply" },
    ["F"] = { "forward" },
    ["D"] = { "trash" },
    ["]a"] = { "next" },
    ["[a"] = { "prev" },
    ["ga"] = { "all" },
    ["gi"] = { "folder", "inbox" },
    ["gs"] = { "folder", "sent" },
    ["gS"] = { "folder", "spam" },
  },
  message = {
    ["R"] = { "reply" },
    ["F"] = { "forward" },
    ["D"] = { "trash" },
  },
  spam = {
    ["d"] = { "mark" },
    ["x"] = { "purge" },
    ["gi"] = { "folder", "inbox" },
    ["]a"] = { "next" },
    ["[a"] = { "prev" },
  },
  compose = {
    ["gs"] = { "send" },
  },
  calendar = {
    ["c"] = { "new" },
    ["D"] = { "delete" },
    ["]a"] = { "next" },
    ["[a"] = { "prev" },
    ["ga"] = { "all" },
    ["]d"] = { "later" },
    ["[d"] = { "earlier" },
    ["gt"] = { "today" },
  },
  event = {
    ["D"] = { "delete" },
  },
  eventedit = {
    ["gs"] = { "save" },
  },
  projects = {
    ["c"] = { "new" },
    ["D"] = { "forget" },
  },
  projectnew = {
    ["gs"] = { "save" },
  },
  tree = {
    ["l"] = { "expand" },
    ["h"] = { "collapse" },
    ["."] = { "hidden" },
  },
}

-- Sets the buffer local keymaps for a page's filetype
function M.keymaps(buf, filetype)
  -- Maps one key in normal mode for this buffer only
  local function map(lhs, spec)
    vim.keymap.set("n", lhs, function()
      M.act(spec[1], spec[2])
    end, { buffer = buf, nowait = true, silent = true })
  end
  -- The common ones
  for lhs, spec in pairs(keymaps.common) do
    map(lhs, spec)
  end
  -- The filetype's own
  for lhs, spec in pairs(keymaps[filetype] or {}) do
    map(lhs, spec)
  end
  -- Back and home on every page, the tree page's q and f belong to project mode
  if filetype == "tree" then
    vim.keymap.set("n", "q", function()
      require("symphony.project").leave(false)
    end, { buffer = buf, nowait = true, silent = true })
    vim.keymap.set("n", "f", function()
      require("symphony.project").pick()
    end, { buffer = buf, nowait = true, silent = true })
  else
    vim.keymap.set("n", "q", M.back, { buffer = buf, nowait = true, silent = true })
  end
  vim.keymap.set("n", "gh", function()
    M.open("home")
  end, { buffer = buf, nowait = true, silent = true })
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
  -- The filetype suffix
  local filetype = page.filetype
  if filetype == vim.NIL or filetype == nil or filetype == "" then
    filetype = "page"
  end
  -- Replace the content while the buffer is writable, a compose page stays writable
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = page.editable == true
  -- The options of a view buffer
  vim.bo[buf].buftype = "nofile"
  vim.bo[buf].bufhidden = "hide"
  vim.bo[buf].swapfile = false
  vim.bo[buf].filetype = "symphony-" .. filetype
  -- Remember the view, the page, the keys, the page key, and the title on the buffer
  vim.b[buf].symphony_view = view_of(page.name)
  vim.b[buf].symphony_page = page.name
  vim.b[buf].symphony_keys = keys
  vim.b[buf].symphony_key = (page.key ~= vim.NIL and page.key) or ""
  vim.b[buf].symphony_title = page.title
  -- Set the keymaps
  M.keymaps(buf, filetype)
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

-- Closes a page's buffer by name, used after a send or a trash
function M.close(name)
  -- Nothing to close
  if name == nil or name == vim.NIL or name == "" then
    return
  end
  local buf = vim.fn.bufnr(M.prefix .. name)
  if buf == -1 or not vim.api.nvim_buf_is_valid(buf) then
    return
  end
  -- Leave it first when it is on screen
  if buf == vim.api.nvim_get_current_buf() then
    M.back()
  end
  pcall(vim.api.nvim_buf_delete, buf, { force = true })
end

-- Applies a response from the host
function M.apply(resp)
  -- Nothing to do
  if resp == nil or resp == vim.NIL or resp.kind == "none" then
    return
  end
  -- A page is shown, then anything the host wants closed goes
  if resp.kind == "page" then
    M.show(resp.page)
    M.close(resp.close)
    if resp.text and resp.text ~= vim.NIL and resp.text ~= "" then
      vim.notify(resp.text)
    end
    return
  end
  -- A message is shown, then anything the host wants closed goes
  if resp.kind == "notify" then
    local level = resp.error and vim.log.levels.ERROR or vim.log.levels.INFO
    vim.notify(resp.text, level)
    M.close(resp.close)
    return
  end
  -- A directory is entered as a project
  if resp.kind == "enter" then
    M.close(resp.close)
    require("symphony.project").enter(resp.path, resp.text)
    return
  end
  -- A file is opened in the editor
  if resp.kind == "edit" then
    vim.cmd.edit(vim.fn.fnameescape(resp.path))
  end
end

-- Asks the host for a page and shows it
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

-- Sends an action to the host and applies the reply, the key is the one given, else the cursor line's, else the page's
function M.act(action, key)
  -- The current buffer and its view
  local buf = vim.api.nvim_get_current_buf()
  local view = vim.b[buf].symphony_view
  -- Not a symphony buffer
  if not view then
    return
  end
  -- The key
  if key == nil then
    local line = vim.api.nvim_win_get_cursor(0)[1]
    local keys = vim.b[buf].symphony_keys or {}
    key = keys[line]
    if key == nil or key == "" then
      key = vim.b[buf].symphony_key or ""
    end
  end
  -- The body, the whole buffer for a send
  local body = ""
  if action == "send" then
    body = table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n")
  end
  -- Ask
  local page = vim.b[buf].symphony_page or ""
  local ok, resp = pcall(rpc.request, "symphony.action", view, action, key, page, body)
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
