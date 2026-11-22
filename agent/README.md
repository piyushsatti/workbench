# agent

The agent layer for Claude Code, Codex and Cursor ([decisions](../docs/decisions.md#agent-layer)): a common part every harness reads, plus a part per harness. This is the source. Nothing here is live until `workbench deploy` (`scripts/deploy_agent.py`) copies it into place, or `workbench install` does it as the `agent-layer` item.

```
agent/
├─ common/      what every harness reads
├─ claude/      Claude Code's part
├─ codex/       Codex's part
├─ cursor/      Cursor's part
└─ opencode/    OpenCode's part (deploy still supports it; the installer does not install OpenCode)
```

## common/

| Path | What | Deploys to |
|---|---|---|
| `AGENTS.md` | canonical global rules | `~/.agents/AGENTS.md`, and copied where a harness reads only a literal file (see each harness part) |
| `skills/` | the global skills (see its README) | `~/.agents/skills/` (Codex, Cursor, OpenCode read it), and `~/.claude/skills/` through Claude's part |
| `subagents/` | architect and scrutineer review agents | each harness's agents folder, converted where the harness needs it |
| `hooks/` | `git-guard.py`, a git safety hook, with its configs and tests | `hooks/install.json` puts `git-guard` in `~/.local/bin`, outside agent-writable paths |
| `sandbox/` | `strict` and `permissive` Claude Code profiles | merged by hand into a repository's `.claude/settings.json` |
| `bin/` | `lint-agent-files.sh` | `~/.agents/bin/` |
| `docs/harness-parity.md` | Claude Code, Codex and Cursor capability matrix | `~/.agents/docs/` (reference) |

`~/.agents` is a deploy target, not a checkout: edit here and deploy.

## Harness parts

Each harness folder has a `harness.json` that `deploy_agent.py` reads: which common folders go where, which converter turns a subagent into the harness's format, whether the rules need a literal copy, and what deploy can only report.

| Part | Holds | Rules reach the harness through |
|---|---|---|
| `claude/` | `harness.json`; `CLAUDE.md`, the one-line shim; `statusline/` and the `cc-status` mod, placed by its `install.json` | `~/.claude/CLAUDE.md` importing `@~/.agents/AGENTS.md`. Deploy checks the file and never writes it: its format is yours |
| `codex/` | `harness.json`: subagents as TOML | a literal copy at `~/.codex/AGENTS.md` (Codex follows no import) |
| `cursor/` | `harness.json`: subagents with Cursor frontmatter | Cursor Settings → Rules, pasted by hand (not a file) |
| `opencode/` | `harness.json`: subagents with OpenCode frontmatter | a literal copy at `~/.config/opencode/AGENTS.md` |

## Rules that hold here

- **Global skills are capped at 12** ([decisions](../docs/decisions.md#agent-layer)). `common/skills/` holds 7, listed in its README.
- **No knowledge-base content.** Workbench ships no rulebooks and names no notes folder. A skill that can use one reads its location from its own config (dream) or asks.
- **Hooks are guarantees, rules are requests.** Anything that must hold goes in a git hook, the sandbox or a permission rule, not in prose here.
- **Executables never run from an agent-writable path.** Deploy copies scripts to `~/.local/bin` or `~/.local/share/claude-mods`.

## Known gaps

- Cursor imports Claude's hook settings (user and project) by default, so registering the same hooks in Cursor fires them twice; see `common/docs/harness-parity.md`.
- `common/hooks/git-guard.py` is built, tested and installed to `~/.local/bin/git-guard` by deploy, but not registered anywhere; hook configs are merged by hand.
- git-guard reads commands word by word, so it does not see a commit that another program builds or runs: text built at run time and piped into a shell (`cat cmd.txt | sh`), command runners (`uv run git …`, `find -exec git …`, `arch -arm64 git …`), `$(which git)`, `git rebase -x`, `git submodule foreach` and `git filter-branch`. Words produced by expansion (`$'-n'`, `{-n,-m,x}`, `$f`) are not expanded either. Register it knowing it stops the plain forms only.

Tests: `uv run python -B -m unittest discover -s scripts/tests` (deploy and the harness parts), plus the suites in `common/skills/README.md`, `common/hooks/tests` and `claude/README.md`.
