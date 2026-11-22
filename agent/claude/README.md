# claude

Claude Code's part of the agent layer. `harness.json` tells `scripts/deploy_agent.py` where the common subagents and skills go (`~/.claude/agents`, `~/.claude/skills`) and which line `~/.claude/CLAUDE.md` must hold; `CLAUDE.md` here is that one-line shim, for reference (deploy checks the file and never writes it). It also places this folder's own `skills/refine` and `subagents/refiner.md`: `/refine` rewrites a rough prompt with a Haiku subagent, and `bin/claude-refine-hook` (installed to `~/.local/bin` by `install.json`) runs it for any prompt that starts with `refine:`. The hook needs a `UserPromptSubmit` entry in `~/.claude/settings.json`, which deploy reports when missing and never writes.

The Claude-only pieces below install through `install.json`; Claude's settings are merged by hand, and deploy prints the exact lines when they're missing.

| Piece | What it does | Installed to | Turned on by |
| --- | --- | --- | --- |
| cc-status | Mod. Footer: limits, turn/compaction counts, ctx bar data, memory list (`/mem`, `/diet`) | `~/.local/share/claude-mods/cc-status` | `"CLAUDE_CODE_PLUGIN_DIRS": "~/.local/share/claude-mods/cc-status"` in `env` |
| statusline | Status line: folder, model, effort, context bar, cache state and the observational-memory segment. It also saves the status JSON per session that cc-status reads | `~/.local/bin/claude-statusline` | `"statusLine": {"type": "command", "command": "$HOME/.local/bin/claude-statusline"}` |

The two work together: the 5h and 7d limits show in the footer only when both are on. The status line needs `jq`.

Tests: `python3 -B -m unittest discover -s agent/claude/statusline/tests` (the status line), and `claude plugin test` in a copy of `cc-status` (it writes type files).

pm and tm live in ordinator; snippets is in `tools/snippets` ([decisions](../../docs/decisions.md#what-each-piece-is)).
