-- Keymaps on top of LazyVim's, the leader is space as LazyVim sets it, symphony lives under leader y since LazyVim has leader s for search
local map = vim.keymap.set

-- The apps under leader s for symphony
map("n", "<leader>yh", "<cmd>Symphony home<cr>", { desc = "Symphony home" })
map("n", "<leader>ym", "<cmd>Symphony mail<cr>", { desc = "Symphony mail" })
map("n", "<leader>yc", "<cmd>Symphony calendar<cr>", { desc = "Symphony calendar" })
map("n", "<leader>yp", "<cmd>Symphony projects<cr>", { desc = "Symphony projects" })
map("n", "<leader>yd", "<cmd>Symphony discord<cr>", { desc = "Symphony discord" })
map("n", "<leader>yu", "<cmd>Symphony open discordweb<cr>", { desc = "Symphony discord, your account" })
map("n", "<leader>yb", "<cmd>Symphony browser<cr>", { desc = "Symphony browser" })
map("n", "<leader>ye", "<cmd>Symphony config<cr>", { desc = "Symphony config" })
map("n", "<leader>yl", "<cmd>Symphony leave<cr>", { desc = "Symphony leave project" })

-- Yours, last so they win
pcall(require, "user.keymaps")
