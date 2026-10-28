# Tmux Command Reference

Your tmux is configured with:
- **Prefix key**: `Ctrl+Space` (instead of default `Ctrl+B`)
- **Mouse support**: Enabled
- **Copy mode**: Vi-style bindings
- **Theme**: Nord, from the nordtheme/tmux plugin; `.tmux.conf` restates the window and status formats after it loads
- **Session persistence**: Auto-save every 10 minutes

---

## Panes

### Pane Selection (Vim-style)

| Command | Action |
|---------|--------|
| `<prefix> h` | Move to pane on the left |
| `<prefix> j` | Move to pane below |
| `<prefix> k` | Move to pane above |
| `<prefix> l` | Move to pane on the right |

### Pane Selection (Alt+Arrow) - No Prefix Needed

| Command | Action |
|---------|--------|
| `Alt+Left` | Move to pane on the left |
| `Alt+Right` | Move to pane on the right |
| `Alt+Up` | Move to pane above |
| `Alt+Down` | Move to pane below |

### Creating Panes

| Command | Action |
|---------|--------|
| `<prefix> \` | Split pane vertically (side-by-side) |
| `<prefix> -` | Split pane horizontally (stacked) |

### Closing & Moving Panes

| Command | Action |
|---------|--------|
| `<prefix> x` | Close current pane |
| `<prefix> !` | Move current pane to new window |
| `<prefix> {` | Swap pane with previous |
| `<prefix> }` | Swap pane with next |
| `<prefix> z` | Toggle pane zoom (fullscreen) |
| `<prefix> <space>` | Cycle through pane layouts |

### Pane Synchronization

| Command | Action |
|---------|--------|
| `<prefix> a` | Toggle synchronized input (send commands to all panes) |

Shows status: "Sync ON" or "Sync OFF". Useful for multi-server administration.

**Note**: New panes inherit the current working directory.

---

## Windows

### Window Navigation

| Command | Action |
|---------|--------|
| `Shift+Left` | Previous window |
| `Shift+Right` | Next window |
| `Alt+H` | Previous window (Vim-style) |
| `Alt+L` | Next window (Vim-style) |

### Creating & Managing Windows

| Command | Action |
|---------|--------|
| `<prefix> c` | Create new window |
| `<prefix> ,` | Rename current window |
| `<prefix> &` | Close current window |

### Window Reordering

| Command | Action |
|---------|--------|
| `Ctrl+Shift+Left` | Move current window left |
| `Ctrl+Shift+Right` | Move current window right |

**Note**: New windows inherit the current working directory.

---

## Copy/Paste (Vi-style Copy Mode)

### Entering & Exiting Copy Mode

| Command | Action |
|---------|--------|
| `Alt+V` | Enter copy mode |
| `Escape` | Exit copy mode |

### Copy Mode Navigation & Selection

| Command | Action |
|---------|--------|
| `v` | Begin selection |
| `C-v` | Toggle rectangle (block) selection |
| `y` | Copy selection and leave copy mode |
| Mouse drag | Copy the dragged selection and leave copy mode |
| `Double-click` | Select word and copy to clipboard |
| `Triple-click` | Select line and copy; the selection stays visible |

**Note**: `y` and a mouse drag use `copy-pipe-and-cancel`, so copy mode ends; only a triple-click keeps the selection visible.

---

## Session Management

### Fuzzy Session Switcher

| Command | Action |
|---------|--------|
| `<prefix> S` | Open fuzzy finder to switch sessions (Linux only) |

Uses `fzf` for interactive session selection. On macOS, use `<prefix> s` (tmux-sessionx).

---

## Special Features

### Floating Scratch Terminal

| Command | Action |
|---------|--------|
| `Alt+G` | Toggle floating scratch terminal (80% width/height) |

Useful for quick commands without minimizing your main layout. Opens in current directory.

---

## Session Persistence

Your tmux automatically:
- **Saves state** every 10 minutes
- **Restores sessions** on startup (Linux only; continuum restore is off on macOS)
- **Preserves**: Pane contents, windows, directories
- **Preserves processes**: vim, nvim, less, man, htop

No manual action needed — your sessions survive shell restarts.

---

## Other Commands

| Command | Action |
|---------|--------|
| `<prefix> ?` | List all key bindings |
| `<prefix> :` | Command prompt |
| `<prefix> d` | Detach from session |
| `<prefix> $` | Rename current session |

---

## Plugins Installed

Each is installed at the commit pinned in `.chezmoidata/tmux_plugins.json`: by `chezmoi apply` on macOS and by Workstation on Ubuntu. `<prefix> I` and `<prefix> U` would move them to upstream HEAD, so they are not the way to install or update.

1. **tpm** - Plugin manager
2. **tmux-sensible** - Sensible defaults
3. **vim-tmux-navigator** - Seamless vim/tmux navigation
4. **nordtheme/tmux** - Nord theme and status bar layout
5. **tmux-prefix-highlight** - Prefix indicator for the Nord theme
6. **tmux-cpu** - CPU/system monitor; installed but not currently in the status bar
7. **tmux-resurrect** - Session persistence, with local patches (`.patches/tmux-resurrect.diff`)
8. **tmux-claude-resurrect** - Restores Claude Code panes with their `--resume` session
9. **tmux-continuum** - Automatic session save/restore
10. **tmux-thumbs** - Hint-based copy of on-screen strings
11. **tmux-sessionx** - Session switcher (prefix + s)
12. **tmux-toggle-popup** - Floating scratch terminal (Alt-g)

---

## Tips

- **Mouse**: Click panes to switch, scroll in copy mode, drag pane dividers
- **Search in copy mode**: `/` searches forward, `?` backward; `<prefix> f` is tmux's find-window, which searches window names and contents
- **Quick refresh**: `<prefix> :` then type `source-file ~/.tmux.conf` and press Enter
- **List all sessions**: `tmux list-sessions` or `<prefix> s`
