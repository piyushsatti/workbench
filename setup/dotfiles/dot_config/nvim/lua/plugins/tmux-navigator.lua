-- Ctrl+h/j/k/l move between nvim splits and on into tmux panes; ~/.tmux.conf loads the
-- tmux half of the same plugin. Replaces LazyVim's plain <C-w> window maps.
return {
  {
    "christoomey/vim-tmux-navigator",
    cmd = {
      "TmuxNavigateLeft",
      "TmuxNavigateDown",
      "TmuxNavigateUp",
      "TmuxNavigateRight",
      "TmuxNavigatePrevious",
      "TmuxNavigatorProcessList",
    },
    keys = {
      { "<C-h>", "<cmd><C-U>TmuxNavigateLeft<cr>", desc = "Window or Pane Left" },
      { "<C-j>", "<cmd><C-U>TmuxNavigateDown<cr>", desc = "Window or Pane Below" },
      { "<C-k>", "<cmd><C-U>TmuxNavigateUp<cr>", desc = "Window or Pane Above" },
      { "<C-l>", "<cmd><C-U>TmuxNavigateRight<cr>", desc = "Window or Pane Right" },
      { "<C-\\>", "<cmd><C-U>TmuxNavigatePrevious<cr>", desc = "Previous Window or Pane" },
    },
  },
}
