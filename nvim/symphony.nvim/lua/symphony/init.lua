-- The entry point of the plugin
local M = {}

-- The transport to the host
local rpc = require("symphony.rpc")

-- Every subcommand keyed by name
local commands = {}

-- Prints the host reply to a ping
commands.ping = function()
  -- Ask the host
  local ok, reply = pcall(rpc.request, "symphony.ping")
  -- Show the error when the request failed
  if not ok then
    vim.notify(tostring(reply), vim.log.levels.ERROR)
    return
  end
  -- Show the reply
  vim.notify("symphony: " .. tostring(reply))
end

-- Prints whether a host is attached and on which channel
commands.status = function()
  -- Find the host
  local chan = rpc.channel()
  -- Report either way
  if chan then
    vim.notify("symphony: attached on channel " .. chan)
  else
    vim.notify("symphony: no host attached")
  end
end

-- Runs the health check
commands.health = function()
  vim.cmd.checkhealth("symphony")
end

-- Runs one subcommand from the words after :Symphony
function M.command(fargs)
  -- The subcommand name, status when none was given
  local name = fargs[1] or "status"
  -- Find it
  local fn = commands[name]
  -- Report an unknown name
  if not fn then
    vim.notify("symphony: unknown subcommand " .. name, vim.log.levels.ERROR)
    return
  end
  -- Run it with the rest of the words
  fn(vim.list_slice(fargs, 2))
end

-- Returns the subcommand names for completion
function M.subcommands()
  -- The names collected so far
  local names = {}
  -- Loop over every command
  for name in pairs(commands) do
    table.insert(names, name)
  end
  -- Sort so completion is stable
  table.sort(names)
  return names
end

-- Setup for people who call it from their config, nothing to configure yet
function M.setup(opts)
  M.opts = opts or {}
end

return M
