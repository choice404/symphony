-- Name the symphony group so which-key shows it under leader y
return {
  {
    "folke/which-key.nvim",
    opts = {
      spec = {
        { "<leader>y", group = "symphony" },
      },
    },
  },
}
