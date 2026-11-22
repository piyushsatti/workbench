# opencode

OpenCode's part of the agent layer, kept so `workbench deploy --harness opencode` still works. `harness.json` tells `scripts/deploy_agent.py` to convert the common subagents to OpenCode frontmatter in `~/.config/opencode/agents` and to copy the rules to `~/.config/opencode/AGENTS.md`. `workbench install` does not install OpenCode; the Ubuntu workstation (`workbench setup`) still does, with its deny-by-default config.
