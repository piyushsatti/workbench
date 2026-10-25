# workbench

Machine setup and the `pi` plugin marketplace. The marketplace is named `pi` (unrelated to Pi Agent); this repo is `workbench`. What's decided is in [docs/decisions.md](docs/decisions.md).

Only the plugins have their own repos, next to this one in `~/Source`: the six listed in `plugins.json` (observational-memory, rpctool, agent-coop, ordinator, scriptorium, manifold).

- `setup/workstation/`: OS packages, CLI tools and profiles (`defaults.yml`, `bootstrap`, `verify`).
- `setup/dotfiles/`: the chezmoi source (shell, git, tmux, themes).
- `agent/`: shared rules, rulebooks, subagents, sandbox profiles and the git-guard hook. `agent/skills/` holds the global skills, deployed to `~/.agents/skills`.
- `tools/`: the repo's own tools: the `workbench` CLI (`cli/`), agent-tmux, workspaces (`awm`, the agent workspace manager), snippets.
- `harness/claude/`: Claude Code–only pieces (cc-status). pm and tm live in ordinator (`~/Source/ordinator/internal/`).
- `plugins.json`: the plugins in the marketplace, one repo each in `~/Source/<name>`, except agent-tmux, which is cataloged from `tools/agent-tmux`.
- `.claude-plugin/`, `.agents/plugins/`: the Claude Code and Codex catalogs, written by `scripts/write_catalogs.py`. Don't edit them by hand. Cursor has no `pi` catalog: add each plugin repo to Cursor directly.
- [CONVENTIONS.md](CONVENTIONS.md): what every plugin repo ships.
- `scripts/`: `write_catalogs.py`, `lint_plugins.py`, `deploy_agent.py`, `drift_check.py`. Tests are in `scripts/tests/`. Python **≥ 3.11** via root `pyproject.toml` and `uv run`; bootstrap still uses bare `python3` for `profile.py`.
- [Decisions](docs/decisions.md): what's true now and what's open.

After changing a plugin repo:

```bash
uv run python -B scripts/write_catalogs.py --apply && uv run python -B scripts/lint_plugins.py
# or: workbench plugins sync
```
