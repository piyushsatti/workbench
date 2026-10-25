# workbench

One front door to the workbench repo ([decisions](../../docs/decisions.md#what-each-piece-is)). Each verb calls an existing script and adds no logic of its own.

| Verb | Runs |
|---|---|
| `workbench setup [--profile NAME]` | `sudo setup/workstation/bootstrap.sh`, which verifies at the end. Ubuntu 26.04 only; refuses elsewhere. |
| `workbench dotfiles [diff\|apply\|status\|verify]` | `chezmoi --source setup/dotfiles <sub>`. Default `diff`. |
| `workbench plugins [check\|sync\|lint]` | `scripts/write_catalogs.py` (dry run, or `--apply` for `sync`), then `lint_plugins.py`. Default `check`. |
| `workbench deploy [--apply] [--harness …]` | `scripts/deploy_agent.py`, arguments passed through. Dry run unless `--apply`. |
| `workbench drift [--verbose] [--harness …]` | `scripts/drift_check.py`, read-only, arguments passed through. It also runs `workspaces doctor`. |
| `workbench doctor` | `verify.sh` (Ubuntu only) and the drift check, in one report. |

`PI_ROOT` selects the workbench checkout (default `~/Source/workbench`). `workbench deploy --apply` installs this script to `~/.local/bin/workbench` from `install.json`.

For `plugins`, `deploy`, `drift`, and `doctor` (drift leg), the CLI runs repo scripts with `uv run --project "$PI_ROOT" python` when `uv` is on PATH and `pyproject.toml` exists; otherwise `python3`. Set `PYTHON=` to override (single executable path).

Exit codes: 64 for an unknown verb, 78 when there's no workbench checkout; otherwise the called script's.

Tests (from repo root): `uv run python -B -m unittest discover -s tools/cli/tests`. Fallback: `python3 -B -m unittest discover -s tests` from this folder.
