-- Runs the plugin tests under nvim --headless -u NONE -l tests/run.lua
-- Every test is a function in the tests table, a failure is an error, the exit code is the failure count

-- The directory this script lives in
local here = debug.getinfo(1, "S").source:sub(2):match("(.*/)") or "./"
-- The plugin root one level up
local root = vim.fn.fnamemodify(here .. "..", ":p")
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

-- The subcommand list is sorted and holds the known names
tests.subcommands_sorted = function()
  local names = require("symphony").subcommands()
  assert_eq(table.concat(names, ","), "health,ping,status", "names")
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
  require("symphony").command({})
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
