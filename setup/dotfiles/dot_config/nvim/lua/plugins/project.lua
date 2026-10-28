-- Project root detection + recent projects list
return {
  {
    "ahmedkhalf/project.nvim",
    opts = {
      -- No "lsp": this pin's lsp method calls the deprecated vim.lsp.buf_get_clients().
      detection_methods = { "pattern" },
      patterns = { ".git", "pyproject.toml", "package.xml", "CMakeLists.txt", "Makefile" },
      show_hidden = false,
      silent_chdir = true,
      scope_chdir = "global",
    },
    config = function(_, opts)
      require("project_nvim").setup(opts)
      require("telescope").load_extension("projects")
    end,
  },

  -- Add <Space>fp keymap to open project picker
  {
    "nvim-telescope/telescope.nvim",
    keys = {
      { "<leader>fp", "<cmd>Telescope projects<cr>", desc = "Projects" },
    },
  },
}
