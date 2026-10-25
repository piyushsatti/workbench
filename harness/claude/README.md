# claude

Claude Code–specific harness pieces in workbench. Each subfolder is a mod plugin loaded through `CLAUDE_CODE_PLUGIN_DIRS` in `~/.claude/settings.json`.

| Mod | What it does |
| --- | --- |
| cc-status | Footer: limits, turn/compaction counts, ctx bar, memory list (`/mem`, `/diet`) |

pm and tm live in ordinator; snippets is in `tools/snippets` ([decisions](../../docs/decisions.md#what-each-piece-is)).
