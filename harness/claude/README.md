# claude

Claude Code–specific harness pieces in workbench. `workbench deploy` installs them through `install.json`; Claude's settings are merged by hand, and deploy prints the exact lines when they're missing.

| Piece | What it does | Installed to | Turned on by |
| --- | --- | --- | --- |
| cc-status | Mod. Footer: limits, turn/compaction counts, ctx bar data, memory list (`/mem`, `/diet`) | `~/.local/share/claude-mods/cc-status` | `"CLAUDE_CODE_PLUGIN_DIRS": "~/.local/share/claude-mods/cc-status"` in `env` |
| statusline | Status line: folder, model, effort, context bar, cache state and the observational-memory segment. It also saves the status JSON per session that cc-status reads | `~/.local/bin/claude-statusline` | `"statusLine": {"type": "command", "command": "$HOME/.local/bin/claude-statusline"}` |

The two work together: the 5h and 7d limits show in the footer only when both are on. The status line needs `jq`.

Tests: `python3 -B -m unittest discover -s harness/claude/statusline/tests` (the status line), and `claude plugin test` in a copy of `cc-status` (it writes type files).

pm and tm live in ordinator; snippets is in `tools/snippets` ([decisions](../../docs/decisions.md#what-each-piece-is)).
