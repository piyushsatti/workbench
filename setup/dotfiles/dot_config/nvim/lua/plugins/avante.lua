-- avante.nvim: Claude AI sidebar (VS Code Copilot equivalent)
-- Uses claude-code ACP provider — authenticates via existing Claude Code OAuth session.
-- No separate API key needed. Requires: npx @zed-industries/claude-code-acp (auto-fetched by npx)
return {
  {
    "yetone/avante.nvim",
    event = "VeryLazy",
    version = false,
    opts = {
      provider = "claude-code",
      acp_providers = {
        ["claude-code"] = {
          command = "npx",
          args = { "@zed-industries/claude-code-acp" },
          env = {
            NODE_NO_WARNINGS = "1",
          },
        },
      },
      mappings = {
        toggle  = { default = "<C-a>" },  -- bare <C-a>: open/close sidebar
        ask     = "<C-a>a",               -- ask a question
        edit    = "<C-a>e",               -- inline edit
        refresh = "<C-a>r",               -- refresh
        focus   = "<C-a>f",               -- focus sidebar
      },
      hints = { enabled = true },
    },
    build = "make",
    dependencies = {
      "nvim-treesitter/nvim-treesitter",
      "stevearc/dressing.nvim",
      "nvim-lua/plenary.nvim",
      "MunifTanjim/nui.nvim",
      "nvim-tree/nvim-web-devicons",
      {
        "HakonHarnes/img-clip.nvim",
        event = "VeryLazy",
        opts = {
          default = {
            embed_image_as_base64 = false,
            prompt_for_file_name = false,
            drag_and_drop = { insert_mode = true },
          },
        },
      },
      {
        "MeanderingProgrammer/render-markdown.nvim",
        opts = { file_types = { "markdown", "Avante" } },
        ft = { "markdown", "Avante" },
      },
    },
  },
}
