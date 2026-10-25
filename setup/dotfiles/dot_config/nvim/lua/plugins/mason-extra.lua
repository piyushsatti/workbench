-- Mason: ensure bash and yaml language servers are installed
return {
  {
    "mason-org/mason.nvim",
    opts = function(_, opts)
      opts.ensure_installed = opts.ensure_installed or {}
      vim.list_extend(opts.ensure_installed, {
        "bash-language-server",
        "yaml-language-server",
      })
    end,
  },
}
