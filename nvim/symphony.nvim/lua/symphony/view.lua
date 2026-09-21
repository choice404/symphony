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
  gitstatus = {
    ["s"] = { "stage" },
    ["u"] = { "unstage" },
    ["c"] = { "commit" },
    ["l"] = { "log" },
    ["p"] = { "push" },
    ["P"] = { "pull" },
  },
  gitlog = {
    ["gs"] = { "status" },
  },
  gitcommit = {
    ["gs"] = { "save" },
  },
  discordmessages = {
    ["i"] = { "send", nil, "say: " },
  },
  discordweb = {},
  config = {
    ["e"] = { "edit" },
    ["R"] = { "reload" },
    ["k"] = { "keymaps" },
    ["o"] = { "options" },
    ["p"] = { "plugins" },
    ["t"] = { "theme" },
  },
  discordchat = {
    ["i"] = { "send", nil, "say: " },
  },
  browser = {
    ["o"] = { "url", nil, "url: " },
    ["/"] = { "search", nil, "search: " },
    ["x"] = { "close" },
  },
  browsertab = {
    ["o"] = { "url", nil, "url: " },
    ["/"] = { "search", nil, "search: " },
    ["f"] = { "follow", nil, "link: " },
    ["gs"] = { "submit" },
    ["b"] = { "back" },
    ["F"] = { "forward" },
    ["x"] = { "close" },
  },
}

-- Sets the buffer local keymaps for a page's filetype
function M.keymaps(buf, filetype)
  -- Maps one key in normal mode for this buffer only, a spec with a prompt asks for a line and sends it as the body
  local function map(lhs, spec)
    vim.keymap.set("n", lhs, function()
      if spec[3] then
        vim.ui.input({ prompt = spec[3] }, function(text)
          if text and text ~= "" then
            M.act(spec[1], spec[2], text)
          end
        end)
        return
      end
      M.act(spec[1], spec[2])
    end, { buffer = buf, nowait = true, silent = true })
  end
  -- The common ones, on a browser page enter asks for a value when the line is a field
  for lhs, spec in pairs(keymaps.common) do
    map(lhs, spec)
  end
  -- The config page picks a colorscheme with T
  if filetype == "config" then
    vim.keymap.set("n", "T", function()
      require("symphony").command({ "theme" })
    end, { buffer = buf, nowait = true, silent = true })
  end
  if filetype == "browsertab" then
    vim.keymap.set("n", "<CR>", function()
      local line = vim.api.nvim_win_get_cursor(0)[1]
      local key = (vim.b[buf].symphony_keys or {})[line] or ""
      if key:sub(1, 6) == "field:" then
        vim.ui.input({ prompt = "value: " }, function(text)
          if text ~= nil then
            M.act("open", key, text)
          end
        end)
        return
      end
      M.act("open")
    end, { buffer = buf, nowait = true, silent = true })
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
    vim.keymap.set("n", "g", function()
      M.open("git/status/" .. (vim.b[buf].symphony_key or ""))
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
  -- Diff pages get diff colors
  if filetype == "diff" then
    vim.bo[buf].syntax = "diff"
  end
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
  -- A file is opened in the editor, and the host's text says what a write of it should do, reload the daemon, source the file, apply the theme, or note a restart
  if resp.kind == "edit" then
    vim.cmd.edit(vim.fn.fnameescape(resp.path))
    local buf = vim.api.nvim_get_current_buf()
    if resp.text == "reload" then
      M.reload_on_write(buf)
    elseif resp.text == "source" or resp.text == "theme" or resp.text == "restart" then
      M.apply_on_write(buf, resp.text)
    end
  end
end

-- Makes every write of the buffer ask the daemon to read the config again, the answer is shown either way
function M.reload_on_write(buf)
  local group = vim.api.nvim_create_augroup("symphony_config_" .. buf, { clear = true })
  vim.api.nvim_create_autocmd("BufWritePost", {
    group = group,
    buffer = buf,
    callback = function()
      local ok, err = pcall(rpc.request, "symphony.reload")
      -- After the written line has drawn, so the two do not stack into a prompt
      vim.schedule(function()
        vim.cmd.redraw()
        if ok then
          vim.notify("symphony: config reloaded")
        else
          vim.notify("symphony: " .. tostring(err), vim.log.levels.ERROR)
        end
      end)
    end,
  })
end

-- Makes every write of the buffer apply the file, source runs it, theme applies the colors, restart only says so
function M.apply_on_write(buf, how)
  local group = vim.api.nvim_create_augroup("symphony_apply_" .. buf, { clear = true })
  vim.api.nvim_create_autocmd("BufWritePost", {
    group = group,
    buffer = buf,
    callback = function()
      local path = vim.api.nvim_buf_get_name(buf)
      local ok, err = true, nil
      if how == "source" then
        ok, err = pcall(dofile, path)
      elseif how == "theme" then
        ok, err = pcall(function()
          require("config.theme").apply()
        end)
      end
      vim.schedule(function()
        vim.cmd.redraw()
        if not ok then
          vim.notify("symphony: " .. tostring(err), vim.log.levels.ERROR)
        elseif how == "restart" then
          vim.notify("symphony: new plugins load on the next start, :Lazy installs them now")
        else
          vim.notify("symphony: applied " .. vim.fn.fnamemodify(path, ":t"))
        end
      end)
    end,
  })
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

-- Sends an action to the host and applies the reply, the key is the one given, else the cursor line's, else the page's, the body is the one given, else the buffer of an editable page
function M.act(action, key, body)
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
  -- The body, the one given, else the whole buffer whenever the page is one you edit
  if body == nil then
    body = ""
    if vim.bo[buf].modifiable then
      body = table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n")
    end
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
  -- Nothing behind, go home unless already there, the dashboard when the config has one
  if vim.g.symphony_dashboard and Snacks and Snacks.dashboard then
    Snacks.dashboard()
    return
  end
  if vim.b[0].symphony_view ~= "home" then
    M.open("home")
  end
end

return M
