-- The transport between the plugin and the host
local M = {}

-- The error raised when no host is attached
M.not_attached = "symphony: not attached to a host"

-- Returns the host channel id or nil when the plugin runs without a host
function M.channel()
  -- The host sets this global right after it attaches
  local chan = vim.g.symphony_channel
  -- Treat anything but a positive number as no host
  if type(chan) ~= "number" or chan <= 0 then
    return nil
  end
  -- Return the channel
  return chan
end

-- Sends a request to the host and returns its reply, raising when there is no host
function M.request(method, ...)
  -- Find the host
  local chan = M.channel()
  -- Raise when there is none
  if not chan then
    error(M.not_attached, 0)
  end
  -- Forward the request
  return vim.rpcrequest(chan, method, ...)
end

-- Sends a notification to the host, doing nothing when there is no host
function M.notify(method, ...)
  -- Find the host
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
