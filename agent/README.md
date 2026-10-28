# agent

Shared agent configuration for Claude Code, Codex and Cursor ([decisions](../docs/decisions.md#harnesses)). This is the source. Nothing here is live until `workbench deploy` (`scripts/deploy_agent.py`) copies it into place.

| Path | What | Deploys to |
|---|---|---|
| `AGENTS.md` | canonical global rules | `~/.agents/AGENTS.md`; read through the one-line `~/.claude/CLAUDE.md` shim (`@~/.agents/AGENTS.md`), `~/.codex/AGENTS.md`, and Cursor user rules |
| `rulebooks/` | topic rulebooks (documentation, engineering, governance, orchestration, security, tooling) | per-repository bound copies in `docs/standards/`, amended here first |
| `subagents/` | architect and scrutineer review agents | `~/.claude/agents/`; Codex `~/.codex/agents/*.toml` and Cursor need a converted form (see below) |
| `sandbox/` | `strict` and `permissive` Claude Code profiles | merged into a repository's `.claude/settings.json` |
| `skills/` | the global skills (see its README) | `~/.agents/skills/` and `~/.claude/skills/` |
| `hooks/` | `git-guard.py`, a git safety hook, with its configs and tests | `hooks/install.json` puts `git-guard` in `~/.local/bin`, outside agent-writable paths |
| `bin/` | `lint-agent-files.sh` | `~/.agents/bin/` |
| `docs/harness-parity.md` | Claude Code, Codex and Cursor capability matrix | reference only |

## Rules that hold here

- **Global skills are capped at 12** ([decisions](../docs/decisions.md#agent-config)). `skills/` holds 7, listed in `skills/README.md`.
- **Hooks are guarantees, rules are requests.** Anything that must hold goes in a git hook, the sandbox or a permission rule, not in prose here.
- **Executables never run from an agent-writable path.** Deploy copies scripts to `~/.local/bin` or `~/.tmux/claude`.

## Known gaps

- Cursor imports Claude's hook settings (user and project) by default, so registering the same hooks in Cursor fires them twice; see `docs/harness-parity.md`.
- `hooks/git-guard.py` is built, tested and installed to `~/.local/bin/git-guard` by deploy, but not registered anywhere; hook configs are merged by hand.
- git-guard reads commands word by word, so it does not see a commit that another program builds or runs: text built at run time and piped into a shell (`cat cmd.txt | sh`), command runners (`uv run git …`, `find -exec git …`, `arch -arm64 git …`), `$(which git)`, `git rebase -x`, `git submodule foreach` and `git filter-branch`. Words produced by expansion (`$'-n'`, `{-n,-m,x}`, `$f`) are not expanded either. Register it knowing it stops the plain forms only.
