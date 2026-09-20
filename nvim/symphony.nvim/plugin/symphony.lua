-- Load once
if vim.g.loaded_symphony then
  return
end
vim.g.loaded_symphony = true

-- The :Symphony command hands its words to the lua module
vim.api.nvim_create_user_command("Symphony", function(opts)
  require("symphony").command(opts.fargs)
end, {
  nargs = "*",
  complete = function()
    return require("symphony").subcommands()
  end,
})
