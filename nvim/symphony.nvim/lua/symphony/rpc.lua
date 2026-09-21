-- The transport between the plugin and the daemon, a msgpack rpc channel over the daemon's unix socket
local M = {}

-- The error raised when no daemon can be reached
M.not_attached = "symphony: not attached to a host"

-- Returns the socket path, the env override, then the runtime dir, then the cache dir
function M.socket_path()
  -- The override wins
  local override = vim.env.SYMPHONY_SOCKET
  if override and override ~= "" then
    return override
  end
  -- The runtime dir when there is one
  local runtime = vim.env.XDG_RUNTIME_DIR
  if runtime and runtime ~= "" then
    return runtime .. "/symphony/symphonyd.sock"
  end
  -- The cache dir otherwise, the same one the daemon uses
  local cache = vim.env.XDG_CACHE_HOME
  if not cache or cache == "" then
    cache = vim.env.HOME .. "/.cache"
  end
  return cache .. "/symphony/symphonyd.sock"
end

-- Reports whether a channel id still points at an open channel
local function open(chan)
  -- Anything but a positive number is closed
  if type(chan) ~= "number" or chan <= 0 then
    return false
  end
  -- A closed channel has no info
  local ok, info = pcall(vim.api.nvim_get_chan_info, chan)
  return ok and info ~= nil and next(info) ~= nil
end

-- Dials the daemon socket and remembers the channel, returns the channel or nil and a message
function M.connect(path)
  -- Already attached
  local have = vim.g.symphony_channel
  if open(have) then
    return have
  end
  -- The path to dial
  path = path or M.socket_path()
  -- Dial as an rpc channel
  local ok, chan = pcall(vim.fn.sockconnect, "pipe", path, { rpc = true })
  -- Report a failure
  if not ok or type(chan) ~= "number" or chan == 0 then
    return nil, "symphony: no daemon at " .. path .. ", start symphonyd"
  end
  -- Remember it
  vim.g.symphony_channel = chan
  return chan
end

-- Drops the remembered channel
function M.disconnect()
  -- Close it when it is open
  local chan = vim.g.symphony_channel
  if open(chan) then
    pcall(vim.fn.chanclose, chan)
  end
  vim.g.symphony_channel = nil
end

-- Returns the daemon channel, dialing the default socket when none is remembered
function M.channel()
  -- The remembered channel when it is still open
  local chan = vim.g.symphony_channel
  if open(chan) then
    return chan
  end
  -- Forget a dead one
  vim.g.symphony_channel = nil
  -- Try the default socket
  return (M.connect())
end

-- Sends a request to the daemon and returns its reply, raising when there is no daemon
function M.request(method, ...)
  -- Find the daemon
  local chan = M.channel()
  -- Raise when there is none
  if not chan then
    error(M.not_attached, 0)
  end
  -- Forward the request
  local ok, result = pcall(vim.rpcrequest, chan, method, ...)
  if ok then
    return result
  end
  -- A dead channel is dialed once more before giving up
  if tostring(result):lower():find("channel", 1, true) then
    vim.g.symphony_channel = nil
    chan = M.channel()
    if chan then
      return vim.rpcrequest(chan, method, ...)
    end
    error(M.not_attached, 0)
  end
  -- Anything else is the daemon's own error
  error(result, 0)
end

-- Sends a notification to the daemon, doing nothing when there is none
function M.notify(method, ...)
  -- Find the daemon
  local chan = M.channel()
  -- Drop the notification when there is none
  if not chan then
    return false
  end
  -- Forward the notification
  vim.rpcnotify(chan, method, ...)
  return true
end

return M
