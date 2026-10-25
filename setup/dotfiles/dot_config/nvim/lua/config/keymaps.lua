local map = vim.keymap.set

-- F12: go to definition (VS Code)
map("n", "<F12>", vim.lsp.buf.definition, { desc = "Go to Definition" })

-- Shift+F12: find references (VS Code)
map("n", "<S-F12>", "<cmd>Telescope lsp_references<cr>", { desc = "Find References" })

-- F2: rename symbol (VS Code)
map("n", "<F2>", vim.lsp.buf.rename, { desc = "Rename Symbol" })

-- ── Comments ───────────────────────────────────────────────────────────────
-- Ctrl+/: toggle comment (VS Code)
-- Note: terminals send Ctrl+/ as <C-_>, Kitty sends both
-- gcc = linewise (normal), gc = selection (visual)
map("n", "<C-_>", "gcc", { desc = "Toggle Comment", remap = true })
map("n", "<C-/>", "gcc", { desc = "Toggle Comment", remap = true })
map("v", "<C-_>", "gc",  { desc = "Toggle Comment", remap = true })
map("v", "<C-/>", "gc",  { desc = "Toggle Comment", remap = true })

-- ── Terminal ───────────────────────────────────────────────────────────────
-- Ctrl+`: toggle Snacks floating terminal (LazyVim built-in)
map("n", "<C-`>", function() require("snacks").terminal.toggle() end, { desc = "Toggle Terminal" })

-- ── Window Navigation ──────────────────────────────────────────────────────
-- Ctrl+hjkl to move between splits
map("n", "<C-h>", "<C-w>h", { desc = "Move to Left Window" })
map("n", "<C-j>", "<C-w>j", { desc = "Move to Bottom Window" })
map("n", "<C-k>", "<C-w>k", { desc = "Move to Top Window" })
map("n", "<C-l>", "<C-w>l", { desc = "Move to Right Window" })
