-- Your keymaps, loaded after symphony's and LazyVim's so yours win, this file is written once and never touched by an update
-- Edit it from the config page with K, every write applies it straight away
local map = vim.keymap.set

-- jk leaves insert mode
map("i", "jk", "<esc>")
-- A blank line below or above without leaving normal mode
map("n", "<leader>o", "o<esc>", { desc = "New line below" })
map("n", "<leader>O", "O<esc>", { desc = "New line above" })
-- Join lines and search hits keep the cursor where it was
map("n", "J", "mzJ`z")
map("n", "n", "nzzzv")
map("n", "N", "Nzzzv")
-- Move a selection up or down
map("v", "J", ":m '>+1<cr>gv=gv")
map("v", "K", ":m '<-2<cr>gv=gv")
-- Ex mode stays off
map("n", "Q", "<nop>")
