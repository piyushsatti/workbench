# dotfiles

Terminal configuration for macOS and Linux (Ubuntu), managed
with [chezmoi](https://www.chezmoi.io/).

This component is the canonical authored source for shared terminal files. The
sibling Workstation installer renders a locked target subset for each Linux
profile, `linux` and `linux-full`, and each profile has its own lock; it does not
maintain another copy of these files. Workstation continues to own packages, pinned tmux plugins, recovery and
runtime verification.

## What's here

There are two switchable themes: **Nord** (the default) and **Catppuccin Mocha** ([decisions](../../docs/decisions.md#look)). Both palettes live in `.chezmoidata/theme.yaml`, and every themed file reads them; an empty or unknown theme falls back to Nord. The login shell is Homebrew bash 5.x.

| Area | Files | Notes |
|---|---|---|
| bash | `.bashrc`, `.bash_profile`, `.bash_aliases` | login shell; templated per OS; lazy-loads pyenv and zoxide; bash-completion@2; `BAT_THEME` from the theme |
| zsh | `.zshrc`, `.zprofile` | retained, but no longer the login shell |
| prompt | `.config/starship.toml` | palette from the theme; used by bash on all machines |
| tmux | `.tmux.conf`, `~/.tmux/claude/*` | settled look (see below); tpm, resurrect, continuum, claude-resurrect |
| ghostty | `.config/ghostty/config` | theme from the palette; JetBrainsMono Nerd Font Mono |
| btop | `.config/btop/btop.conf`, `.config/btop/themes/catppuccin_mocha.theme` | built-in `nord`, or the pinned official catppuccin/btop Mocha file (see below) |
| neovim | `.config/nvim/` | LazyVim config; only `lua/plugins/colorscheme.lua` is templated (see below) |
| editors | VS Code and Cursor `settings.json` and `keybindings.json` | one shared template in `.chezmoitemplates/`; `~/.config/Code` on Linux, `~/Library/Application Support/{Code,Cursor}` on macOS; Cursor on macOS only |

## Per-machine settings

On Linux, `chezmoi init` asks once for the required `linux` (headless) or `linux-full` (GUI) profile; you can instead set `WORKSTATION_PROFILE` to either value. macOS needs no profile.

```toml
[data]
  theme = "nord"                  # or "catppuccin-mocha"
[data.git]                        # asked once by `chezmoi init` (.chezmoi.toml.tmpl); never in the repo
  name = "Your Name"
  email = "you@example.com"
  signingkey = ""                 # optional
[data.tmux]                       # only where several Claude logins share one ~/.claude tree
  claudeResurrectDir = "/home/you/.claude-alt"
  claudeResurrectCommand = "/home/you/.local/bin/claude"
```

`git` drives `~/.gitconfig` (`[user]`, `core.hooksPath = ~/.config/git/hooks`, plus the shared settings every machine gets: default branch, pull/fetch/push/merge/rerere/diff, the editor by OS and profile, and delta and `~/.gitignore_global` only where they exist) and the global `pre-commit` hook, which refuses a commit whose `git var GIT_AUTHOR_IDENT` / `GIT_COMMITTER_IDENT` differs from it, then runs the repo's own `.git/hooks/pre-commit`. Machine-only git settings (credential helpers, `url.insteadOf`) go in `~/.gitconfig.local`, which `~/.gitconfig` includes first, so it cannot override the identity or hooks path. Move them there before the first `chezmoi apply` replaces `~/.gitconfig`.

Rendered files carry no home-directory paths (a test checks this), so the Ubuntu render lock stays the same on every machine.

## Install

```sh
# macOS
brew install chezmoi
# Linux: Workstation (setup/workstation) installs a pinned chezmoi checked against the SHA-256 in
# profiles/ubuntu-26.04/versions.json. Without it, download that version's release tarball and check
# its SHA-256 against versions.json before extracting it to ~/.local/bin.

chezmoi init --source ~/Source/workbench/setup/dotfiles
chezmoi diff       # review what would change
chezmoi apply
```

On macOS, `chezmoi apply` installs each tmux plugin at the commit pinned in `.chezmoidata/tmux_plugins.json` (Workstation installs the same list on Ubuntu). Don't press `Ctrl+Space` then `I` or `U`: tpm would move the plugins to upstream HEAD.

## Machine-local extension points

Anything machine-specific (hardware quirks, ssh mounts, one-off tooling)
stays out of this repo. These files are sourced if present, never managed:

- `~/.bashrc.local`
- `~/.bash_profile.local`
- `~/.bash_aliases.local`
- `~/.zshrc.local`

## tmux status bar

Keep these properties when editing:
- The layout comes from the nordtheme/tmux plugin. The window and status formats are restated after the plugin loads, so the foreground uses the theme's hex value.
- A two-line status area: line 0 is blank in the terminal background, so there's a gap above the bar.
- Fixed-width tabs. The Claude badge (from the `@claude_state` pane option) is one glyph in a one-character slot, drawn by `~/.tmux/claude/tmux-status.conf`. That file is sourced last, with `source-file -q`, so a machine without it still loads.
- No state-driven terminal title.

`run_after_20-patch-tmux-resurrect.sh` re-applies the local tmux-resurrect patches in `.patches/` to a TPM checkout after every apply. A TPM update reverts them. Workstation installs carry no `.git`, so the script skips them; the installer applies the same patch itself.

## btop Catppuccin theme

btop ships `nord` but no Catppuccin theme. `dot_config/btop/themes/catppuccin_mocha.theme` is the official
file from [catppuccin/btop](https://github.com/catppuccin/btop), copied byte for byte at commit
`f437574b600f1c6d932627050b15ff5153b58fa3` (MIT, Copyright (c) 2021 Catppuccin). The licence and the pin
sit next to it as `.catppuccin-LICENSE` and `.catppuccin-SOURCE`; dot-prefixed files stay in the source and
are not applied. A render test checks the file's SHA-256, so an update means changing the pin and the test together.

## Neovim

`.config/nvim/` is a LazyVim config. It's copied as-is except:
- `lua/plugins/colorscheme.lua` is a template: Catppuccin Mocha (`catppuccin/nvim`) or Nord
  (`nordtheme/vim`, the official port), from the theme.
- `init.lua` is the LazyVim starter's one line, loading `config.lazy-bootstrap`.

`lazy-lock.json` is kept because it pins plugins. `lazyvim.json` (LazyVim's own news and install-version state) is
not managed: LazyVim rewrites it. lazy.nvim also rewrites `lazy-lock.json` on update, so `chezmoi diff` shows that
drift. Re-add it with `chezmoi add` when the new pins are wanted.

## Dependencies

`ghostty`, `starship`, `tmux`, `btop`, `neovim`, `fzf`, `eza`, `bat`, `zoxide`, `fd`,
`ripgrep`, `lazygit`, the JetBrainsMono Nerd Font, and on macOS: homebrew,
`bash` 5.x plus `bash-completion@2`.

Everything above installs via Homebrew. Node comes from the `node` formula;
nvm and fnm are not used.

## Workstation consumer profile

The Ubuntu Workstation profiles consume Bash, tmux, Starship and btop from this
source; `linux-full` also takes Ghostty and the VS Code `settings.json` and
`keybindings.json`. Both keep Continuum restore enabled; automatic tmux boot is on
for `linux-full` and off for `linux`. They exclude the TPM run-once script
because Workstation pins every plugin itself.

Render tests do not apply files:

```sh
python3 -m unittest discover -s setup/dotfiles/tests   # from the repo root
```
