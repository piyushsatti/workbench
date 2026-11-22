# workbench

How I set up a computer, and how I keep the AI agents on it in line. Take what's useful.

## Install

On a fresh machine, macOS, Ubuntu or Arch (Omarchy included):

```bash
# macOS: xcode-select --install   Ubuntu: sudo apt install -y git   Arch: sudo pacman -S --needed git
git clone https://github.com/piyushsatti/workbench ~/Source/workbench
~/Source/workbench/install/install.sh
```

It shows a numbered menu: pick `tools` (just the tools), `tools+agent` (the tools and the agent layer), or single items. It prints the plan and asks before doing anything. The flag form is `--bundle tools+agent`, `--pick tmux,neovim`, `--no-gui` for a headless box and `--apply` to skip the question. Later, `workbench update` pulls this repo and upgrades what it installed. Details: [install/](install/README.md).

On Ubuntu 26.04 there is also the full pinned workstation, `sudo setup/workstation/bootstrap.sh` (`workbench setup`): the same packages plus a locked dotfiles render, pinned tmux plugins, the VS Code extensions and an end-to-end verify.

### Just the dotfiles (macOS or any Linux)

```bash
sh -c "$(curl -fsLS get.chezmoi.io)" -- -b ~/.local/bin    # skip if you have chezmoi
git clone https://github.com/piyushsatti/workbench ~/Source/workbench
chezmoi init --source ~/Source/workbench/setup/dotfiles
chezmoi diff     # see what would change
chezmoi apply
```

`chezmoi init` asks once for your git name and email. They stay on your machine.

### The agent layer and tools

```bash
~/Source/workbench/tools/cli/bin/workbench deploy             # dry run: lists what it would copy
~/Source/workbench/tools/cli/bin/workbench deploy --apply
```

This copies the agent layer (rules, skills, subagents) to where Claude Code, Codex, Cursor and OpenCode look for them, and installs git-guard, snippets, the skills' helper scripts and the `workbench` command itself in `~/.local/bin`. From then on it's just `workbench install`, `workbench update`, `workbench dotfiles diff|apply`, `workbench deploy` and `workbench plugins`. `workbench install --bundle tools+agent` runs this deploy for you. It prints the build commands for the Go tools (workspaces, agent-tmux) rather than running them. Add `--harness claude` (or `codex`, `cursor`, `opencode`) to limit it to one harness.

### Plugins from the `pi` marketplace

The marketplace isn't open yet. It will carry:

- **observational-memory**: an observer model takes notes while a session runs, and puts them back after the context is compacted.
- **rpctool**: shrinks huge tool output to its head and tail, and runs commands on allowlisted machines over ssh.
- **agent-coop**: a browser page where you edit and comment on a document, and the agent picks up your changes.
- **ordinator**: runs a fleet of agent sessions, one per ticket, each taking it to a draft pull request.
- **agent-tmux**: the tmux MCP server described below. It lives in this repo rather than its own.

When it opens, swap `<plugin>` for one of those names. Each plugin's hooks and MCP server call a small program in `~/.local/bin`, kept out of the plugin folder so an agent can't edit it; the plugin's own README says how to install that part.

**Claude Code**

```bash
claude plugin marketplace add piyushsatti/workbench
claude plugin install <plugin>@pi
```

**Codex**

```bash
codex plugin marketplace add piyushsatti/workbench
codex plugin add <plugin>@pi
```

**Cursor** has no install command for personal use. Its docs load local plugins from `~/.cursor/plugins/local/`, so clone the plugin there and reload the window. This route hasn't been tested yet. For agent-tmux, copy `tools/agent-tmux` from this repo instead.

```bash
git clone https://github.com/piyushsatti/<plugin> ~/.cursor/plugins/local/<plugin>
```

**OpenCode** only loads plugins written as JavaScript modules, from npm or a local folder, so these plugins don't run there yet. It does read Agent Skills from `~/.agents/skills`, which is where `workbench deploy` puts the global skills.

## Why this exists

It started as a pile of dotfiles. Then I began working with coding agents most of the day, across Claude Code, Codex and Cursor, on a Mac and a couple of Ubuntu boxes, and the pile grew a second half: the rules those agents follow, the guards that stop them doing something dumb, and the small tools I wrote when an agent kept tripping over the same thing. Both halves live here because they change together.

Nothing here assumes you're me, but it is opinionated.

## What's here

### The dotfiles

[`setup/dotfiles/`](setup/dotfiles/) is a [chezmoi](https://www.chezmoi.io/) source for bash, zsh, git, tmux, Neovim (LazyVim), Ghostty, btop and the editor settings VS Code and Cursor share.

Everything reads one palette file, so switching between Nord (the default) and Catppuccin Mocha is a one-line change that recolours the terminal, tmux, the prompt, `bat` and the editors together. tmux has a two-line status bar and `Ctrl-Space` as its prefix. It can badge the panes that have Claude running in them, if a Claude Code hook sets the `@claude_state` pane option (that hook isn't in this repo yet).

Anything that only belongs on one machine goes in a `*.local` file next to the managed one (`~/.bashrc.local`, `~/.gitconfig.local`), which chezmoi leaves alone.

### Living with agents

[`agent/`](agent/) is the agent layer: a common part every harness reads, and a small part per harness (Claude Code, Codex, Cursor) that says where things go and how they convert.

- **Skills** ([`agent/common/skills/`](agent/common/skills/)): seven global skills. `crucible` reviews a plan or a diff through different professional lenses, up to a red team against a blue team. `dream` mines old transcripts for things worth remembering. `save-state` writes a checkpoint before you walk away. `humanizer`, `grill-me`, `research` and `teach` are borrowed, with credit, from people who wrote them better.
- **git-guard** ([`agent/common/hooks/`](agent/common/hooks/)): a hook that stops an agent from slipping past the git identity check with `--no-verify`, `-c user.email=…`, a swapped `HOME` and the other ways round it.
- **Sandbox profiles** ([`agent/common/sandbox/`](agent/common/sandbox/)): `strict` for repos where a human makes every commit, `permissive` for ones where the agent can do more but still can't force-push or rewrite history.

### Tools I wrote

- **[workspaces](tools/workspaces/)** starts an agent only from folders I've allowed, always inside tmux, and logs every launch. It's a small Go binary that sits in front of `claude` and `codex`.
- **[agent-tmux](tools/agent-tmux/)** is an MCP server that lets an agent open its own tmux windows and panes and do its work where I can watch it. It can type into its own panes but never presses Enter, and it won't touch a pane it didn't open.
- **[snippets](tools/snippets/)** are small reusable instructions ("don't commit", "plan first") that ride along with every prompt until you switch them off.
- **[workbench](tools/cli/)** is the front door: `workbench install`, `workbench update`, `workbench dotfiles diff`, `workbench deploy`.
- **[cc-status](agent/claude/)** is a Claude Code footer showing rate limits, context use and what's loaded into memory.

### The `pi` marketplace

Some of this grew big enough to be a plugin someone else might install. Each one lives in its own repo and is written once for every harness that can load it: one manifest per harness, shared skills, the same MCP config, hooks written per harness. This repo hosts the catalog, named `pi` (no relation to Pi Agent). The rules every plugin repo follows are in [CONVENTIONS.md](CONVENTIONS.md).

## Working on it

Decisions, and the questions that are still open, are in [docs/decisions.md](docs/decisions.md). Python runs through `uv` from the root `pyproject.toml`. Each folder's README lists its tests; the repo-level ones are:

```bash
uv run python -B -m unittest discover -s scripts/tests
uv run python -B -m unittest discover -s install/tests
uv run python -B scripts/lint_plugins.py
```
