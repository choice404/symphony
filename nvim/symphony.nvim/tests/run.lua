-- Runs the plugin tests under nvim --headless -u NONE -l tests/run.lua
-- Every test is a function in the tests table, a failure is an error, the exit code is the failure count

-- The directory this script lives in
local here = debug.getinfo(1, "S").source:sub(2):match("(.*/)") or "./"
-- The plugin root one level up
local root = vim.fn.fnamemodify(here .. "..", ":p")
-- Point the socket at a path nothing listens on so a real daemon never answers a test
vim.env.SYMPHONY_SOCKET = vim.fn.tempname() .. ".sock"
-- Put the plugin on the runtimepath
vim.opt.runtimepath:prepend(root)
-- Source the plugin file the way a real start would
vim.cmd.source(root .. "plugin/symphony.lua")

-- Every test keyed by name
local tests = {}

-- Fails unless a condition holds
local function assert_true(cond, msg)
  if not cond then
    error(msg or "assertion failed", 2)
  end
end

-- Fails unless two values are equal
local function assert_eq(got, want, msg)
  if got ~= want then
    error(string.format("%s: got %s want %s", msg or "assert_eq", vim.inspect(got), vim.inspect(want)), 2)
  end
end

-- The command registers once
tests.command_registered = function()
  local cmds = vim.api.nvim_get_commands({})
  assert_true(cmds.Symphony ~= nil, "Symphony command missing")
  assert_true(vim.g.loaded_symphony, "loaded guard not set")
end

-- No channel means no host
tests.channel_nil_without_host = function()
  vim.g.symphony_channel = nil
  assert_eq(require("symphony.rpc").channel(), nil, "channel")
  vim.g.symphony_channel = 0
  assert_eq(require("symphony.rpc").channel(), nil, "channel zero")
end

-- A request without a host raises the named error
tests.request_raises_without_host = function()
  vim.g.symphony_channel = nil
  local rpc = require("symphony.rpc")
  local ok, err = pcall(rpc.request, "symphony.ping")
  assert_eq(ok, false, "request should fail")
  assert_eq(err, rpc.not_attached, "error text")
end

-- A notify without a host is dropped and reports false
tests.notify_dropped_without_host = function()
  vim.g.symphony_channel = nil
  assert_eq(require("symphony.rpc").notify("symphony.noop"), false, "notify")
end

-- The socket path follows the override, then the runtime dir, then the cache dir
tests.socket_path_order = function()
  local rpc = require("symphony.rpc")
  local saved = { vim.env.SYMPHONY_SOCKET, vim.env.XDG_RUNTIME_DIR, vim.env.XDG_CACHE_HOME }
  vim.env.SYMPHONY_SOCKET = "/tmp/o.sock"
  assert_eq(rpc.socket_path(), "/tmp/o.sock", "override")
  vim.env.SYMPHONY_SOCKET = ""
  vim.env.XDG_RUNTIME_DIR = "/run/user/9"
  assert_eq(rpc.socket_path(), "/run/user/9/symphony/symphonyd.sock", "runtime")
  vim.env.XDG_RUNTIME_DIR = ""
  vim.env.XDG_CACHE_HOME = "/tmp/cache"
  assert_eq(rpc.socket_path(), "/tmp/cache/symphony/symphonyd.sock", "cache")
  vim.env.SYMPHONY_SOCKET, vim.env.XDG_RUNTIME_DIR, vim.env.XDG_CACHE_HOME = saved[1], saved[2], saved[3]
end

-- Connecting to a socket nobody listens on returns nil and a message naming the path
tests.connect_missing_socket = function()
  local rpc = require("symphony.rpc")
  local chan, err = rpc.connect("/nonexistent/dir/none.sock")
  assert_eq(chan, nil, "channel")
  assert_true(err:find("/nonexistent/dir/none.sock", 1, true) ~= nil, "names the path")
  assert_eq(vim.g.symphony_channel, nil, "nothing remembered")
end

-- The subcommand list is sorted and holds the known names
tests.subcommands_sorted = function()
  local names = require("symphony").subcommands()
  assert_eq(table.concat(names, ","), "calendar,connect,health,home,leave,mail,open,ping,projects,status", "names")
end

-- A fake host that answers render and action from tables
local function fake_host(pages, responses)
  local rpc = require("symphony.rpc")
  local old = rpc.request
  local calls = {}
  rpc.request = function(method, ...)
    table.insert(calls, { method = method, ... })
    if method == "symphony.render" then
      local page = pages[...]
      if not page then
        error("no view named " .. tostring(...), 0)
      end
      return page
    end
    if method == "symphony.action" then
      return responses[select(2, ...)] or { kind = "none" }
    end
    error("unexpected " .. method, 0)
  end
  return calls, function()
    rpc.request = old
  end
end

-- A page for the fake host
local home_page = {
  name = "home",
  title = "symphony",
  lines = { "symphony", "", "  Mail  3 unread" },
  keys = { "", "", "mail" },
  cursor = 2,
  filetype = "home",
}

-- Showing a page fills a scratch buffer with the page's options
tests.show_fills_buffer = function()
  local view = require("symphony.view")
  local buf = view.show(home_page)
  assert_eq(vim.api.nvim_buf_get_name(buf):match("symphony://home$") ~= nil, true, "buffer name")
  assert_eq(table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "|"), "symphony||  Mail  3 unread", "lines")
  assert_eq(vim.bo[buf].modifiable, false, "modifiable")
  assert_eq(vim.bo[buf].buftype, "nofile", "buftype")
  assert_eq(vim.bo[buf].filetype, "symphony-home", "filetype")
  assert_eq(vim.b[buf].symphony_view, "home", "view var")
  assert_eq(vim.b[buf].symphony_keys[3], "mail", "keys var")
  assert_eq(vim.api.nvim_win_get_cursor(0)[1], 3, "cursor row")
  -- Showing again reuses the buffer
  assert_eq(view.show(home_page), buf, "reused buffer")
end

-- A page under a view keeps the view name before the slash
tests.show_subpage_view_name = function()
  local view = require("symphony.view")
  local buf = view.show({ name = "mail/1.plain", lines = { "From: x" }, keys = { "" }, filetype = "message" })
  assert_eq(vim.b[buf].symphony_view, "mail", "view var")
end

-- Nil lines from the host become an empty buffer instead of an error
tests.show_nil_lines = function()
  local view = require("symphony.view")
  local buf = view.show({ name = "empty", lines = vim.NIL, keys = vim.NIL, filetype = "page" })
  assert_eq(#vim.api.nvim_buf_get_lines(buf, 0, -1, false), 1, "one blank line")
end

-- Open asks the host for the view and shows the page it returns
tests.open_renders_from_host = function()
  local calls, restore = fake_host({ home = home_page }, {})
  local view = require("symphony.view")
  local buf = view.open("home")
  restore()
  assert_eq(calls[1].method, "symphony.render", "method")
  assert_eq(calls[1][1], "home", "view name")
  assert_eq(vim.b[buf].symphony_view, "home", "shown")
end

-- Open reports an unknown view through vim.notify
tests.open_unknown_notifies = function()
  local _, restore = fake_host({}, {})
  local seen
  local old = vim.notify
  vim.notify = function(msg, level)
    seen = { msg = msg, level = level }
  end
  require("symphony.view").open("ghost")
  vim.notify = old
  restore()
  assert_eq(seen.level, vim.log.levels.ERROR, "level")
  assert_true(seen.msg:find("ghost", 1, true) ~= nil, "names the view")
end

-- An action sends the view, the action, the key of the cursor line, the page, and an empty body, then applies the page that comes back
tests.act_sends_key_and_applies_page = function()
  local mail_page = { name = "mail/all/inbox", lines = { "inbox" }, keys = { "" }, filetype = "mail" }
  local calls, restore = fake_host({ home = home_page }, { open = { kind = "page", page = mail_page } })
  local view = require("symphony.view")
  view.open("home")
  vim.api.nvim_win_set_cursor(0, { 3, 0 })
  view.act("open")
  restore()
  local c = calls[2]
  assert_eq(c.method, "symphony.action", "method")
  assert_eq(c[1], "home", "view")
  assert_eq(c[2], "open", "action")
  assert_eq(c[3], "mail", "key")
  assert_eq(c[4], "home", "page")
  assert_eq(c[5], "", "body")
  assert_eq(vim.b[0].symphony_view, "mail", "mail page shown")
  assert_eq(vim.b[0].symphony_page, "mail/all/inbox", "page var")
end

-- An explicit key wins over the line, and a line without a key falls back to the page key
tests.act_key_fallbacks = function()
  local calls, restore = fake_host({}, {})
  local view = require("symphony.view")
  view.show({ name = "mail/p/inbox/1", lines = { "From: x", "body" }, keys = { "", "" }, key = "p/inbox/1", filetype = "message" })
  view.act("reply")
  view.act("folder", "sent")
  restore()
  assert_eq(calls[1][3], "p/inbox/1", "page key used")
  assert_eq(calls[2][2], "folder", "action")
  assert_eq(calls[2][3], "sent", "explicit key used")
end

-- A send carries the whole buffer as the body, and the close in the reply removes the compose buffer
tests.send_carries_body_and_closes = function()
  local calls, restore = fake_host({ home = home_page }, { send = { kind = "notify", text = "sent", close = "mail/p/compose/1" } })
  local view = require("symphony.view")
  view.open("home")
  local buf = view.show({ name = "mail/p/compose/1", lines = { "To: a@b.c", "---", "hi" }, keys = { "", "", "" }, key = "1", filetype = "compose", editable = true })
  assert_eq(vim.bo[buf].modifiable, true, "compose is editable")
  vim.api.nvim_buf_set_lines(buf, 2, 3, false, { "hello there" })
  local old = vim.notify
  vim.notify = function() end
  view.act("send")
  vim.notify = old
  restore()
  assert_eq(calls[2][2], "send", "action")
  assert_eq(calls[2][5], "To: a@b.c\n---\nhello there", "body")
  assert_eq(vim.api.nvim_buf_is_valid(buf), false, "compose buffer closed")
  assert_eq(vim.b[0].symphony_view, "home", "back home after close")
end

-- A notify response goes through vim.notify with the right level
tests.act_applies_notify = function()
  local _, restore = fake_host({ home = home_page }, { refresh = { kind = "notify", text = "nope", error = true } })
  local view = require("symphony.view")
  view.open("home")
  local seen
  local old = vim.notify
  vim.notify = function(msg, level)
    seen = { msg = msg, level = level }
  end
  view.act("refresh")
  vim.notify = old
  restore()
  assert_eq(seen.msg, "nope", "text")
  assert_eq(seen.level, vim.log.levels.ERROR, "level")
end

-- Back climbs the history the way you came, never toggling between the last two
tests.back_climbs_history = function()
  local view = require("symphony.view")
  local home = view.show(home_page)
  view.history = {}
  local inbox = view.show({ name = "mail", lines = { "inbox" }, keys = { "" }, filetype = "mail" })
  local msg = view.show({ name = "mail/1", lines = { "From: x" }, keys = { "" }, filetype = "message" })
  assert_eq(vim.api.nvim_get_current_buf(), msg, "on message")
  view.back()
  assert_eq(vim.api.nvim_get_current_buf(), inbox, "back on inbox")
  view.back()
  assert_eq(vim.api.nvim_get_current_buf(), home, "back on home")
  -- A refresh of the same page adds nothing to the history
  view.show(home_page)
  assert_eq(#view.history, 0, "history empty after refresh")
end

-- An unknown subcommand reports an error through vim.notify
tests.unknown_subcommand_notifies = function()
  local seen
  local old = vim.notify
  vim.notify = function(msg, level)
    seen = { msg = msg, level = level }
  end
  require("symphony").command({ "bogus" })
  vim.notify = old
  assert_true(seen ~= nil, "nothing notified")
  assert_eq(seen.level, vim.log.levels.ERROR, "level")
  assert_true(seen.msg:find("bogus", 1, true) ~= nil, "message names the subcommand")
end

-- Status without a host says so
tests.status_without_host = function()
  vim.g.symphony_channel = nil
  local seen
  local old = vim.notify
  vim.notify = function(msg)
    seen = msg
  end
  require("symphony").command({ "status" })
  vim.notify = old
  assert_eq(seen, "symphony: no host attached", "status text")
end


-- A fake host that also serves tree pages for any project path
local function project_host()
  local tree = { name = "", lines = { "demo  /x", "hint", "", "-rw- a.txt" }, keys = { "", "", "", "" }, filetype = "tree" }
  local rpc = require("symphony.rpc")
  local old = rpc.request
  rpc.request = function(method, name)
    if method == "symphony.render" then
      if name == "projects" then
        return { name = "projects", lines = { "projects" }, keys = { "" }, filetype = "projects" }
      end
      if name:sub(1, 14) == "projects/tree/" then
        local page = vim.deepcopy(tree)
        page.name = name
        page.key = name:sub(15)
        return page
      end
      error("no view named " .. tostring(name), 0)
    end
    return { kind = "none" }
  end
  return function()
    rpc.request = old
  end
end

-- Entering a project opens a tab with the tree page and sets the tab's directory
tests.project_enter_opens_tab = function()
  local project = require("symphony.project")
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir .. "/src", "p")
  local before_tabs = #vim.api.nvim_list_tabpages()
  local before_cwd = vim.fn.getcwd(-1, 0)
  local restore = project_host()
  project.enter(dir, "demo")
  assert_eq(#vim.api.nvim_list_tabpages(), before_tabs + 1, "a tab was opened")
  assert_eq(vim.fn.getcwd(), dir, "tab cwd")
  assert_eq(vim.b[0].symphony_page, "projects/tree/" .. dir, "tree page shown")
  assert_eq(vim.g.symphony_project, dir, "global")
  -- The commands exist from setup
  local cmds = vim.api.nvim_get_commands({})
  assert_true(cmds.SymphonyQuit ~= nil and cmds.SymphonyWq ~= nil, "quit commands missing")
  project.leave(true)
  restore()
  assert_eq(project.active, nil, "left")
  assert_eq(#vim.api.nvim_list_tabpages(), before_tabs, "tab closed")
  assert_eq(vim.fn.getcwd(-1, 0), before_cwd, "first tab cwd untouched")
  assert_eq(vim.b[0].symphony_view, "projects", "projects page shown")
end

-- :q on a file goes back to the tree and drops the file, :q on the tree closes the project
tests.project_leave_climbs = function()
  local project = require("symphony.project")
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  vim.fn.writefile({ "hello" }, dir .. "/a.txt")
  local restore = project_host()
  project.enter(dir, "demo")
  vim.cmd.edit(dir .. "/a.txt")
  local fbuf = vim.api.nvim_get_current_buf()
  project.leave(false)
  assert_true(project.active ~= nil, "still in the project after leaving a file")
  assert_eq(vim.b[0].symphony_page, "projects/tree/" .. dir, "back on the tree")
  assert_eq(vim.api.nvim_buf_is_valid(fbuf), false, "file buffer dropped")
  project.leave(false)
  restore()
  assert_eq(project.active, nil, "closed from the tree")
  assert_eq(vim.b[0].symphony_view, "projects", "projects page shown")
end

-- A floating window on screen does not count as a split
tests.project_leave_ignores_floating = function()
  local project = require("symphony.project")
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  vim.fn.writefile({ "x" }, dir .. "/c.txt")
  local restore = project_host()
  project.enter(dir, "f")
  vim.cmd.edit(dir .. "/c.txt")
  local fbuf = vim.api.nvim_create_buf(false, true)
  local float = vim.api.nvim_open_win(fbuf, false, { relative = "editor", width = 10, height = 2, row = 1, col = 1 })
  assert_true(#vim.api.nvim_tabpage_list_wins(0) > 1, "float counted by nvim")
  project.leave(false)
  pcall(vim.api.nvim_win_close, float, true)
  assert_eq(vim.b[0].symphony_page, "projects/tree/" .. dir, "back on the tree despite the float")
  project.leave(true)
  restore()
  assert_eq(project.active, nil, "closed")
end

-- Unsaved changes stop a plain leave, a bang forces it
tests.project_leave_refuses_dirty = function()
  local project = require("symphony.project")
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  vim.fn.writefile({ "x" }, dir .. "/b.txt")
  local restore = project_host()
  project.enter(dir, "d")
  vim.cmd.edit(dir .. "/b.txt")
  vim.api.nvim_buf_set_lines(0, 0, -1, false, { "changed" })
  local seen
  local old = vim.notify
  vim.notify = function(msg, level)
    if level == vim.log.levels.ERROR then
      seen = msg
    end
  end
  project.leave(false)
  vim.notify = old
  assert_true(project.active ~= nil, "still active after refused leave")
  assert_true(seen ~= nil and seen:find("E37", 1, true) ~= nil, "E37 shown")
  project.leave(true)
  project.leave(true)
  restore()
  assert_eq(project.active, nil, "left with bang")
end

-- Any action from an editable page carries the buffer text, a read only page sends none
tests.editable_page_sends_body = function()
  local calls, restore = fake_host({}, {})
  local view = require("symphony.view")
  view.show({ name = "git/commit/x//1", lines = { "msg", "---" }, keys = { "", "" }, key = "1", filetype = "gitcommit", editable = true })
  view.act("save")
  view.show({ name = "git/log/x", lines = { "log" }, keys = { "" }, filetype = "gitlog" })
  view.act("refresh")
  restore()
  assert_eq(calls[1][5], "msg\n---", "editable body sent")
  assert_eq(calls[2][5], "", "read only sends no body")
end

-- An edit response opens the file
tests.apply_edit_opens_file = function()
  local view = require("symphony.view")
  local file = vim.fn.tempname()
  vim.fn.writefile({ "content" }, file)
  view.apply({ kind = "edit", path = file, text = "x" })
  assert_eq(vim.api.nvim_buf_get_name(0), file, "file opened")
  assert_eq(vim.api.nvim_buf_get_lines(0, 0, 1, false)[1], "content", "file content")
end

-- Run every test in name order
local names = vim.tbl_keys(tests)
table.sort(names)
local failed = 0
for _, name in ipairs(names) do
  local ok, err = pcall(tests[name])
  if ok then
    io.stdout:write("ok   " .. name .. "\n")
  else
    failed = failed + 1
    io.stdout:write("FAIL " .. name .. ": " .. tostring(err) .. "\n")
  end
end
io.stdout:write(string.format("%d tests, %d failed\n", #names, failed))
os.exit(failed)
