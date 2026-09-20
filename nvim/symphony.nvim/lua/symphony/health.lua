-- The :checkhealth symphony report
local M = {}

-- The transport to the host
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
  -- Find the host
  local chan = rpc.channel()
  -- Report no host as a warning, the plugin still loads without one
  if not chan then
    health.warn("no host attached", { "run this nvim inside symphony or start symphonyd" })
    return
  end
  -- Report the channel
  health.ok("host attached on channel " .. chan)
  -- Ping the host
  local ok, reply = pcall(rpc.request, "symphony.ping")
  -- Report the round trip
  if ok and reply == "pong" then
    health.ok("host answered ping")
  else
    health.error("host did not answer ping: " .. tostring(reply))
  end
end

return M
