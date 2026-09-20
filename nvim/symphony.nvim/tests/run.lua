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
  assert_eq(table.concat(names, ","), "connect,health,home,mail,open,ping,status", "names")
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

-- An action sends the view, the action, and the key of the cursor line, then applies the page that comes back
tests.act_sends_key_and_applies_page = function()
  local mail_page = { name = "mail", lines = { "inbox" }, keys = { "" }, filetype = "mail" }
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
  assert_eq(vim.b[0].symphony_view, "mail", "mail page shown")
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

-- Back returns to the previous listed buffer
tests.back_goes_to_previous = function()
  local view = require("symphony.view")
  local first = view.show(home_page)
  local second = view.show({ name = "mail", lines = { "inbox" }, keys = { "" }, filetype = "mail" })
  assert_eq(vim.api.nvim_get_current_buf(), second, "on second")
  view.back()
  assert_eq(vim.api.nvim_get_current_buf(), first, "back on first")
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
