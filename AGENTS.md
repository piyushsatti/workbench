# workbench repository instructions

Read `~/.agents/AGENTS.md` first. This repo is an installer for a fresh machine (macOS, Ubuntu, Arch): the installer in `install/`, the dotfiles, the agent layer in `agent/` (a common part plus a part per harness), the repo's own tools, and for now the `pi` marketplace catalog, which moves to `pi-plugins` ([decisions](docs/decisions.md#repos-and-layout)). It knows nothing about the knowledge base: no rulebooks or notes paths here. No nested repositories, gitlinks, submodules or repository-pointing symlinks here.

- **Decisions** live in [docs/decisions.md](docs/decisions.md), edited in place; git is the history. No numbered ADRs, no handoff or session-state documents in the repo.
- **Identifiers:** installed plugin and product identifiers change only on an exact value from the maintainer. A folder move never changes an installed binding.
- **Generated files:** `.claude-plugin/marketplace.json` and `.agents/plugins/marketplace.json` are written by `scripts/write_catalogs.py` from `plugins.json` and each plugin's manifests. Don't edit them by hand.
- **Machine changes** (bootstrap, `chezmoi apply`, deploys, plugin installs, pushes, credentials) each need their own go-ahead.
- **Python:** repo scripts and tests run with `uv run` from the root `pyproject.toml` ([decisions](docs/decisions.md#python)). `setup/workstation/scripts/profile.py` and `install/` stay stdlib-only and run as bare `python3`; `install/` keeps to Python 3.9.
- **Before committing:** the test suites listed in each component's README pass, and `uv run python -B scripts/lint_plugins.py` reports 0 errors.
