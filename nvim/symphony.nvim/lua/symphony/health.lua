-- The :checkhealth symphony report
local M = {}

-- The transport to the daemon
local rpc = require("symphony.rpc")

-- Runs every check
function M.check()
  -- The health api
  local health = vim.health
  -- Start the section
  health.start("symphony")
  -- Report the nvim version
  local v = vim.version()
  health.info(string.format("nvim %d.%d.%d", v.major, v.minor, v.patch))
  -- Report the socket
  health.info("socket " .. rpc.socket_path())
  -- Find the daemon
  local chan = rpc.channel()
  -- Report no daemon as a warning, the plugin still loads without one
  if not chan then
    health.warn("no daemon reachable", { "start symphonyd or run this nvim inside symphony" })
    return
  end
  -- Report the channel
  health.ok("daemon attached on channel " .. chan)
  -- Ping the daemon
  local ok, reply = pcall(rpc.request, "symphony.ping")
  -- Report the round trip
  if ok and reply == "pong" then
    health.ok("daemon answered ping")
  else
    health.error("daemon did not answer ping: " .. tostring(reply))
  end
end

return M
