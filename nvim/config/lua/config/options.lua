-- Options on top of LazyVim's, the editor is a workspace so the chrome stays quiet
local opt = vim.opt

-- No swap or backup files in a session that holds mail and chat buffers
opt.swapfile = false
opt.backup = false
-- Wrap long lines in the pages, they are text
opt.wrap = true
opt.linebreak = true
-- The mouse works inside tmux
opt.mouse = "a"
-- A short update time so the pages refresh feel immediate
opt.updatetime = 200
-- Snacks animations off, the grid is redrawn by the host
vim.g.snacks_animate = false
-- LazyVim's default picker and explorer
vim.g.lazyvim_picker = "snacks"
vim.g.lazyvim_explorer = "snacks"
