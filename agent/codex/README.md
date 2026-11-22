# codex

Codex's part of the agent layer. `harness.json` tells `scripts/deploy_agent.py` to convert the common subagents to Codex TOML in `~/.codex/agents` and to copy `agent/common/AGENTS.md` to `~/.codex/AGENTS.md`, because Codex reads that file's literal text and follows no import. Skills need nothing here: Codex reads `~/.agents/skills`. The converter is `to_codex` in `scripts/deploy_agent.py`; sources are in `agent/common/docs/harness-parity.md`.
