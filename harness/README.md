# harness

Harness-specific extensions in workbench, separate from marketplace `@pi` plugins and from cross-harness `tools/`.

| Path | Role |
| --- | --- |
| `claude/` | Claude Code–only pieces (today: `cc-status` and the status line) |

pm and tm live in the ordinator plugin. snippets is a plain hook in `tools/snippets` ([decisions](../docs/decisions.md#what-each-piece-is)).

`scripts/deploy_agent.py` scans each **direct child** of `harness/` for `install.json` (none today). Mod folders under `claude/` install through Claude settings, not deploy.
