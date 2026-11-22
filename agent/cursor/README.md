# cursor

Cursor's part of the agent layer. `harness.json` tells `scripts/deploy_agent.py` to convert the common subagents to Cursor frontmatter in `~/.cursor/agents`. Cursor's User Rules live in its settings, not a file, so deploy only reminds you to paste `agent/common/AGENTS.md` there. Skills: Cursor reads `~/.agents/skills` (and `~/.claude/skills`; see the third-party import toggle in `docs/decisions.md`). The converter is `to_cursor` in `scripts/deploy_agent.py`.
