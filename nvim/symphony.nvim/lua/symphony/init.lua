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

-- Prints whether a daemon is attached and on which channel
commands.status = function()
  -- Find the daemon
  local chan = rpc.channel()
  -- Report either way
  if chan then
    vim.notify("symphony: attached on channel " .. chan)
  else
    vim.notify("symphony: no host attached")
  end
end

-- Dials the daemon, at a path when one was given
commands.connect = function(args)
  -- Dial
  local chan, err = rpc.connect(args[1])
  -- Report either way
  if chan then
    vim.notify("symphony: attached on channel " .. chan)
  else
    vim.notify(err, vim.log.levels.ERROR)
  end
end

-- Runs the health check
commands.health = function()
  vim.cmd.checkhealth("symphony")
end

-- Opens a view by name, home when none was given
commands.open = function(args)
  require("symphony.view").open(args[1] or "home")
end

-- Opens the home view
commands.home = function()
  require("symphony.view").open("home")
end

-- Opens mail, every account's inbox, or one account's folder such as :Symphony mail school sent
commands.mail = function(args)
  local name = "mail"
  if args[1] then
    name = "mail/" .. args[1] .. "/" .. (args[2] or "inbox")
  end
  require("symphony.view").open(name)
end

-- Opens the calendar, every account's agenda, or one account's such as :Symphony calendar school
commands.calendar = function(args)
  local name = "calendar"
  if args[1] then
    name = "calendar/" .. args[1] .. "/agenda"
  end
  require("symphony.view").open(name)
end

-- Opens the discord page
commands.discord = function()
  require("symphony.view").open("discord")
end

-- Opens the browser tabs page, or a url straight away
commands.browser = function(args)
  local view = require("symphony.view")
  -- The tabs page first so q from the new tab lands on it
  if view.open("browser") == nil then
    return
  end
  if args[1] then
    view.act("url", nil, args[1])
  end
end

-- Opens the projects page
commands.projects = function()
  require("symphony.view").open("projects")
end

-- Leaves project mode and comes back to the projects page
commands.leave = function()
  require("symphony.project").leave(true)
end

-- Runs one subcommand from the words after :Symphony
function M.command(fargs)
  -- The subcommand name, home when none was given
  local name = fargs[1] or "home"
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
