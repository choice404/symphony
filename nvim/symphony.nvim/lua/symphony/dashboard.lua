-- The dashboard items, the daemon's home entries as keys with their live summaries
local M = {}

-- The longest summary shown, the page has the whole thing
local summary_max = 60

-- The key each app answers to on the dashboard, anything unlisted takes its first free letter
local preferred = {
  projects = "p",
  mail = "m",
  calendar = "c",
  discord = "d",
  discordweb = "u",
  browser = "b",
  config = "e",
}

-- Assigns a key to every entry, preferred first, then the first letter of the name not yet taken
function M.keys(entries)
  local taken = {}
  local out = {}
  for _, e in ipairs(entries) do
    local key = preferred[e.name]
    if key and not taken[key] then
      taken[key] = true
      out[e.name] = key
    end
  end
  for _, e in ipairs(entries) do
    if not out[e.name] then
      for ch in (e.name .. "abcdefghijklmnopqrstuvwxyz"):gmatch("[a-z]") do
        if not taken[ch] then
          taken[ch] = true
          out[e.name] = ch
          break
        end
      end
    end
  end
  return out
end

-- Builds the dashboard items from the daemon's entries, an unreachable daemon shows one line saying so
function M.items()
  local rpc = require("symphony.rpc")
  local ok, entries = pcall(rpc.request, "symphony.entries")
  if not ok or type(entries) ~= "table" then
    return {
      { key = "r", desc = "no daemon, press r to try again", action = function() require("symphony.rpc").connect() Snacks.dashboard.update() end },
    }
  end
  local keys = M.keys(entries)
  local width = 0
  for _, e in ipairs(entries) do
    width = math.max(width, #e.label)
  end
  local items = {}
  for _, e in ipairs(entries) do
    local summary = e.summary
    if summary == vim.NIL or summary == nil then
      summary = ""
    end
    if #summary > summary_max then
      summary = summary:sub(1, summary_max - 1) .. "…"
    end
    local name = e.name
    table.insert(items, {
      key = keys[name],
      desc = e.label .. string.rep(" ", width - #e.label + 2) .. summary,
      action = function()
        require("symphony.view").open(name)
      end,
    })
  end
  table.insert(items, { key = "l", desc = "plugins", action = ":Lazy" })
  table.insert(items, { key = "q", desc = "quit", action = ":qa" })
  return items
end

-- The section snacks draws, the items above with a blank line after them
function M.section()
  local items = M.items()
  if #items > 0 then
    items[#items].padding = 1
  end
  return items
end

return M
