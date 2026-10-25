# Decisions

What is decided now. When a decision changes, edit it here in place; git is the history. Open questions are at the bottom.

## Repos and layout

- `~/Source/workbench` holds machine setup, shared agent config, the repo's own tools and the `@pi` marketplace catalog. `PI_ROOT` defaults to it.
- A separate repo is only for something another person could install. Four are plugins in the `pi` marketplace, each a sibling in `~/Source`: observational-memory, rpctool, agent-coop and ordinator. A fifth, agent-tmux, is a `pi` plugin too but stays in `tools/agent-tmux` (its `plugins.json` entry has a `path`, and the catalogs point at that folder). No nested repos, submodules or gitlinks in workbench.
- Folders are lowercase.

| Folder | Holds |
|---|---|
| `setup/` | `workstation/` (profiles, bootstrap) and `dotfiles/` (chezmoi source) |
| `tools/` | the repo's cross-harness tools: the `workbench` CLI, agent-tmux, workspaces (`awm`), snippets |
| `harness/claude/` | Claude Code–only pieces: cc-status |
| `agent/` | shared rules, rulebooks, subagents, hooks, sandbox profiles, and the global skills in `skills/` |
| `scripts/` | catalog writer, plugin lint, deploy, drift check |
| `external/` | pinned upstreams; change the pin or fork, never edit in place |

- Tools install outside agent-writable paths (agent-tmux to `~/.tmux/claude/`, workspaces to `~/.local/bin`). Cursor's CLI owns `~/.local/bin/agent`, so nothing of ours uses that name.

## Harnesses

- The supported harnesses are the ones setup installs: Claude Code, Codex, Cursor and OpenCode. Everything shared works in each; where a harness can't express something, say so; don't approximate it.
- OpenCode is seeded with a deny-by-default config (only file inspection allowed).
- Pi Agent is not supported and is not part of the setup.

## Plugins and marketplace

- A plugin is one folder that Claude Code, Codex and Cursor read directly, with no build step: one manifest per harness, shared `skills/`, identical `.mcp.json` and `mcp.json`, and hooks written per harness. `scripts/lint_plugins.py` checks this ([CONVENTIONS.md](../CONVENTIONS.md)).
- The marketplace is `pi`, unrelated to Pi Agent. Plugins install as `<name>@pi`.
- `plugins.json` lists each plugin, its GitHub repo and an optional `sha` pin. `scripts/write_catalogs.py` writes the Claude Code and Codex catalogs at the repo root. Cursor only resolves catalog paths inside the catalog's repo, so there is no Cursor catalog and no copies: Cursor users add each plugin repo directly.
- Plugin data lives in `~/.local/state/<plugin>/`, never `~/.pi/`. An existing `~/.pi/` is never touched.
- Installed identifiers change only on an exact value from the maintainer. Moving a plugin from an older marketplace to `@pi` means a reinstall, a data copy and permission updates on every machine, so each move needs its own go-ahead.

## What each piece is

- **observational-memory**: plain hooks (a plugin module in OpenCode), works in all four harnesses, on from install in `both` mode.
- **rpctool** (`rpctool@pi`): runs tool calls, including remote execution.
- **ordinator**: owns orchestration-shaped work, including pm and tm as internals (in ordinator's `internal/`). Review capabilities stay with the crucible skill.
- **agent-coop**: human↔harness coordination (a shared document-review surface), separate from ordinator and from `workspaces`, which only gates where a harness may launch.
- **snippets** (`tools/snippets`): a plain `UserPromptSubmit` hook, not a Claude mod. It adds the snippets that are on as context next to the prompt in Claude Code and Codex; Cursor can't.
- **Global skills** live in `agent/skills/` and deploy to `~/.agents/skills` (Codex, Cursor) and `~/.claude/skills` (Claude Code): crucible (one skill, was a plugin), dream, save-state, and the loose bundle grill-me, humanizer, research and teach.
- Cursor also loads `~/.claude/skills` and `~/.codex/skills` for compatibility and doesn't de-duplicate, so on a machine with Cursor its "Include Third-Party Plugins, Skills, and Other Configs" toggle (Cursor Settings → Agents → Third-Party Imports) is off. That toggle covers the editor only; Cursor's CLI still loads both folders.
- **`workbench` CLI** (`tools/cli/`) is the one front door: `setup`, `dotfiles`, `plugins`, `deploy`, `drift`, `doctor`. Each verb calls an existing script.

## Setup

- chezmoi applies dotfiles. `setup/workstation/defaults.yml` holds profiles and package lists.
- Profiles are `linux` (headless) and `linux-full` (`linux` plus `gui`), both on Ubuntu 26.04, plus `darwin`, which has no package lock, so bootstrap refuses it.
- Setup never installs the OS, never customises the desktop and never installs Homebrew. `workbench setup` refuses on anything but Ubuntu 26.04.
- No exact version pins where something else keeps things current: apt packages are listed by name and install at Ubuntu's version; CLIs (chezmoi, uv, Claude Code, Codex, OpenCode) install at their latest only when missing, and one already installed is never replaced. Pins stay only for checksummed downloads, tmux plugin commits and VS Code extensions.
- The `agents` module installs the official Claude plugins listed in `claude_plugins` (defaults.yml) when missing, for the user; one already installed is left as is.
- Per-profile differences go in templates. Rendered files contain no machine paths. Each Linux profile has a lock of its exact dotfile targets and their rendered digest; change a template, re-render and update the digest.

## Look

- Two switchable themes, official Nord (the default) and Catppuccin Mocha, from one palette file, `setup/dotfiles/.chezmoidata/theme.yaml`. Every themed config reads it.
- One tmux look on every profile: `nordtheme/tmux`, two-line status.
- JetBrainsMono Nerd Font Mono. `BAT_THEME` follows the theme. VS Code and Cursor share one settings template.

## Agent config

- At most 12 global skills. Everything else lives in the project that uses it.
- One rulebook set, structured by topic folders.
- LSP plugins are off globally and on per repo.
- Hooks are reviewed before they are added. The `permissive` sandbox denies every git option that runs a program.

## Python

- Repo scripts and tests run with `uv run` from the root `pyproject.toml` (Python 3.11 or later, stdlib-only at runtime).
- Bootstrap runs before uv exists, so `setup/workstation/scripts/profile.py` stays stdlib-only and is called as bare `python3`.

## Open

- LSP plugins: decided off globally, but `claude_plugins` in `defaults.yml` installs pyright-lsp and typescript-lsp, which install enabled. Drop them from the list or change the rule.
- Plugin data roots: agent-coop keeps per-project state in `<project>/.local-state/<name>/`, while the rule is `~/.local/state/<plugin>/`. Pick one.
- Crucible's plan-review hook (`crucible-plan-review-nudge`): dropped with the plugin; bring it back as a reviewed hook if wanted.
- The permissive sandbox's structural fix: keep git inside the sandbox instead of excluding `git *`.
- A pinned tmux-resurrect fork. Until then `run_after_20-patch-tmux-resurrect.sh` re-applies the patch.
