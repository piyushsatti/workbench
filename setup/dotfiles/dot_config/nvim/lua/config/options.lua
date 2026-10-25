local opt = vim.opt

-- Line numbers
opt.number = true
opt.relativenumber = true

-- Indentation (match VS Code defaults)
opt.tabstop = 4
opt.shiftwidth = 4
opt.expandtab = true
opt.smartindent = true

-- Search
opt.ignorecase = true
opt.smartcase = true
-- hlsearch is managed by LazyVim (auto-clears on cursor move) — don't override it

-- UI
opt.scrolloff = 8
opt.sidescrolloff = 8
opt.cursorline = true
opt.colorcolumn = "100"
opt.signcolumn = "yes"
opt.termguicolors = true

-- Files
opt.undofile = true
opt.swapfile = false
opt.backup = false

-- Splits open right and below (VS Code behaviour)
opt.splitright = true
opt.splitbelow = true

-- Python: use pyright for types, ruff for linting/formatting
vim.g.lazyvim_python_lsp = "pyright"
vim.g.lazyvim_python_ruff = "ruff"
