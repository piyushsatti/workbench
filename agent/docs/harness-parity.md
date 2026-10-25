# Harness parity: Claude Code, Codex, Cursor

What each of the three supported harnesses reads, and where `scripts/deploy_agent.py` puts the shared config for it. Pi Agent and OpenCode are out of scope: Pi Agent is not supported and OpenCode is diagnose-only.

Every cell carries one mark:

- **doc** links the official page that says it (fetched 2026-10-22).
- **obs** means observed on a test machine (Codex CLI 0.163.0, Cursor.app installed).
- **inf** means inferred and not confirmed by a doc or a test. Treat it as open.

`developers.openai.com/codex/*` pages now redirect (308) to `learn.chatgpt.com/docs/*`; the links below are the redirect targets.

## Instructions

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| Global file | `~/.claude/CLAUDE.md` ([doc][cc-memory]) | `~/.codex/AGENTS.md`, or `AGENTS.override.md` first; only the first non-empty one is read ([doc][cx-agentsmd]) | none: User Rules are set in Customize > Rules in the app, no file path is documented ([doc][cu-rules]) |
| Follows imports or pointers | yes, `@path` imports up to four hops; `~/.claude/CLAUDE.md` imports load without a dialog ([doc][cc-memory]) | no import syntax is documented; files are concatenated as literal text up to `project_doc_max_bytes` (32 KiB) ([doc][cx-agentsmd]). A one-line "Read `~/.agents/AGENTS.md`" works only if the model chooses to open the file (inf) | n/a for User Rules. Project `AGENTS.md` (root and nested) is read ([doc][cu-rules]) |
| Reads another harness's files | project `AGENTS.md` when no `CLAUDE.md` is above the working directory ([doc][cc-memory]) | none documented | Claude's `CLAUDE.md` through the third-party import setting, with reported bugs in the off switch ([forum][cu-forum-claudemd], [forum][cu-forum-optout]). Whether that covers `~/.claude/CLAUDE.md` and resolves its `@` import is not confirmed (inf) |
| Project rules | `.claude/rules/` ([doc][cc-memory]) | nested `AGENTS.md` from repo root to cwd ([doc][cx-agentsmd]) | `.cursor/rules/*.mdc` with `description`, `globs`, `alwaysApply`; plain `.md` there is ignored ([doc][cu-rules]) |
| What deploy does | checks `~/.claude/CLAUDE.md` has the `@~/.agents/AGENTS.md` shim, never writes it | copies `agent/AGENTS.md` to `~/.codex/AGENTS.md` when that file is missing or is an earlier deploy's copy; a file with its own content gets a CHECK line and is left alone. | prints a CHECK line: paste `agent/AGENTS.md` into User Rules by hand |

## Skills

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| User location | `~/.claude/skills/` only; `.agents` paths are not read ([doc][cc-skills]) | `~/.agents/skills` (USER scope), plus `/etc/codex/skills` and bundled system skills; symlinks are followed ([doc][cx-skills]) | `~/.agents/skills/` and `~/.cursor/skills/` ([doc][cu-skills]) |
| Project location | `.claude/skills/`, nested too ([doc][cc-skills]) | `.agents/skills` in every directory from cwd up to the repo root ([doc][cx-skills]) | `.agents/skills/`, `.cursor/skills/`, nested anywhere in the repo ([doc][cu-skills]) |
| Reads other harnesses' skills | no | none documented | yes: `.claude/skills/`, `~/.claude/skills/`, `.codex/skills/`, `~/.codex/skills/` ([doc][cu-skills]) |
| What deploy does | `scripts/deploy_agent.py` copies `agent/skills` into `~/.claude/skills` | `scripts/deploy_agent.py` copies `agent/skills` into `~/.agents/skills`, which it reads | nothing more: it reads both `~/.agents/skills` and `~/.claude/skills` |

Consequence: `scripts/deploy_agent.py` copies `agent/skills` to both `~/.agents/skills` and `~/.claude/skills`, and Cursor reads both, so each workbench skill may show twice there. How Cursor resolves the duplicate names is not documented (inf).

`~/.codex/skills/` was observed on a test machine with a few skills in it (obs). Codex's skills page does not list it; Cursor's does (as a compatibility path).

## Subagents

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| User location | `~/.claude/agents/*.md` ([doc][cc-subagents]) | `~/.codex/agents/*.toml`, one agent per file; project `.codex/agents/` ([doc][cx-subagents]) | `~/.cursor/agents/*.md`; project `.cursor/agents/` ([doc][cu-subagents]) |
| Format | Markdown, YAML frontmatter `name`, `description`, `tools`, `model`, … ([doc][cc-subagents]) | TOML. Required `name`, `description`, `developer_instructions`. Optional `model`, `model_reasoning_effort`, …; `features.shell_tool` and others can be set to `false`. `sandbox_mode`, `approval_policy`, `mcp_servers` in an agent file are ignored: agents inherit the parent's ([doc][cx-subagents]) | Markdown, YAML frontmatter `name`, `description`, `model` (`inherit` or an ID), `readonly`, `is_background`; no `tools` field is documented ([doc][cu-subagents]) |
| Reads other harnesses' agents | no | none documented | yes: `~/.claude/agents/`, `~/.codex/agents/` and their project forms; `.cursor/` wins on a name clash ([doc][cu-subagents]) |
| What deploy does | copies `agent/subagents/*.md` as they are | converts to TOML: body becomes `developer_instructions`, `model` is dropped (inherit), and a `tools` list without `Bash` becomes `[features] shell_tool = false`. Other tool limits cannot be expressed | converts the frontmatter: `model: inherit`, `readonly: true` when the `tools` list has no Write, Edit, MultiEdit or NotebookEdit (today only architect), `tools` dropped, body unchanged |

Why Cursor gets its own copies although it reads `~/.claude/agents`: the Claude files carry `model: sonnet` and a Claude `tools` list, which Cursor does not document. A same-named file in `~/.cursor/agents` takes precedence, so Cursor uses the converted one (doc for the precedence, inf for how Cursor treats the Claude fields). Whether Cursor parses the Codex TOML files at all is not documented (inf).

## Hooks

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| Config | `hooks` in `~/.claude/settings.json`, project `.claude/settings.json`, `.claude/settings.local.json` ([doc][cc-hooks]) | `~/.codex/hooks.json` or `[hooks]` in `~/.codex/config.toml`, plus the project forms; all sources load ([doc][cx-hooks]) | `~/.cursor/hooks.json`, project `.cursor/hooks.json`, enterprise and team ([doc][cu-hooks]) |
| Events | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `PreCompact`, … ([doc][cc-hooks]) | `SessionStart`, `SessionEnd`, `SubagentStart/Stop`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact/PostCompact`, `UserPromptSubmit`, `Stop`, `Interrupt` ([doc][cx-hooks]) | `sessionStart`, `preToolUse`, `beforeShellExecution`, `beforeSubmitPrompt`, `afterFileEdit`, `stop`, … ([doc][cu-hooks]) |
| Trust | none beyond settings | non-managed hooks need a `/hooks` review pinned to the hook's hash; plugin hooks are not auto-trusted ([doc][cx-hooks]) | none documented |
| Reads Claude's hooks | n/a | no: only `hooks.json` and `config.toml` layers are documented; it sets `CLAUDE_PLUGIN_ROOT`/`CLAUDE_PLUGIN_DATA` for plugin hooks ([doc][cx-hooks]) | **yes, on by default**: `~/.claude/settings.json`, `.claude/settings.json`, `.claude/settings.local.json`, under Settings > Agents > Third-Party Imports, "Include Third-Party Plugins, Skills, and Other Configs". It maps `UserPromptSubmit -> beforeSubmitPrompt`, `Bash -> Shell` ([doc][cu-thirdparty]) |
| Duplicates | n/a | all sources run | "All matching hooks from every source run" and responses merge, deny over ask over allow ([doc][cu-hooks]) |
| What deploy does | nothing: hooks are merged by hand | nothing | nothing |

Cursor imports Claude's hooks, so register nothing in `~/.cursor/hooks.json` that is already in `~/.claude/settings.json`, or it fires twice. The import covers the project `.claude/settings*.json` files too, not only the user file. It does not hold for Codex: a hook wanted in Codex has to be registered there, in its own file, and trusted once. Open: whether Cursor rewrites a `Bash` matcher to `Shell` (the page lists the tool mapping, not matcher rewriting), and `beforeSubmitPrompt` cannot inject context, so a Claude hook that injects context on prompt submit is a no-op in Cursor.

## MCP

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| Config | `claude mcp add`, `~/.claude.json`, project `.mcp.json`, plugin `.mcp.json` ([doc][cc-mcp]) | `[mcp_servers.<name>]` in `~/.codex/config.toml` (`command`, `args`, `env`, `env_vars`); `/mcp` lists them ([doc][cx-mcp]) | `~/.cursor/mcp.json`, project `.cursor/mcp.json`, marketplace ([doc][cu-mcp]) |
| Reads other harnesses' servers | no | no | none documented ([doc][cu-mcp]); plugins imported by the third-party setting may carry servers (inf) |
| What deploy does | nothing | nothing | nothing |

Server definitions stay in each plugin's `.mcp.json`. Adding a server everywhere means one entry per harness file.

## Sandbox and permissions

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| Sandbox | `sandbox` block in settings (Seatbelt on macOS) ([doc][cc-sandbox]); repo profiles in `agent/sandbox/` | `sandbox_mode = read-only \| workspace-write \| danger-full-access`, `[sandbox_workspace_write]` ([doc][cx-config]) | `~/.cursor/sandbox.json` and `<repo>/.cursor/sandbox.json`, merged, repo wins ([doc][cu-sandbox]) |
| Permissions | `permissions.allow/ask/deny` rules in settings ([doc][cc-settings]) | `approval_policy = on-request \| never \| { granular }` ([doc][cx-config]) | CLI `permissions.allow/deny` with `Shell(...)`, `Read(...)`, `Write(...)`, `WebFetch(...)` in `~/.cursor/cli-config.json` or `<repo>/.cursor/cli.json` ([doc][cu-perms]) |
| Subagents | per-agent `tools` | inherit the parent's sandbox and approvals ([doc][cx-subagents]) | `readonly` per agent ([doc][cu-subagents]) |
| What deploy does | nothing: profiles merge by hand per repo | nothing | nothing |

The Claude profiles in `agent/sandbox/` have no Codex or Cursor equivalent yet. Writing them is separate work.

## Statusline

| | Claude Code | Codex | Cursor |
|---|---|---|---|
| Mechanism | `statusLine` command, session JSON on stdin ([doc][cc-statusline]) | `[tui] status_line`: an ordered list of built-in item identifiers, `null` disables it; no command form ([doc][cx-config]). Command items were rejected upstream (issue #17827, from the 2026-08-22 version of this page, not rechecked) | CLI only: `statusLine` with `type: "command"` in `~/.cursor/cli-config.json`, JSON on stdin, "aligned with Claude Code's status line" (obs: Cursor's bundled `~/.cursor/skills-cursor/statusline/SKILL.md`; the official configuration page lists only display toggles ([doc][cu-cliconfig])). A custom line replaces both native footer rows ([forum][cu-forum-statusline]) |
| What deploy does | nothing | nothing | nothing |

## Sources

The link definitions below hold the URLs the tables cite (read the raw file to see them).

[cc-memory]: https://code.claude.com/docs/en/memory
[cc-skills]: https://code.claude.com/docs/en/skills
[cc-subagents]: https://code.claude.com/docs/en/sub-agents
[cc-hooks]: https://code.claude.com/docs/en/hooks
[cc-mcp]: https://code.claude.com/docs/en/mcp
[cc-sandbox]: https://code.claude.com/docs/en/sandboxing
[cc-settings]: https://code.claude.com/docs/en/settings
[cc-statusline]: https://code.claude.com/docs/en/statusline
[cx-agentsmd]: https://learn.chatgpt.com/docs/agent-configuration/agents-md
[cx-skills]: https://learn.chatgpt.com/docs/build-skills
[cx-subagents]: https://learn.chatgpt.com/docs/agent-configuration/subagents
[cx-hooks]: https://learn.chatgpt.com/docs/hooks
[cx-mcp]: https://developers.openai.com/codex/mcp
[cx-config]: https://learn.chatgpt.com/docs/config-file/config-reference
[cu-rules]: https://cursor.com/docs/context/rules
[cu-skills]: https://cursor.com/docs/context/skills
[cu-subagents]: https://cursor.com/docs/context/subagents
[cu-hooks]: https://cursor.com/docs/hooks
[cu-thirdparty]: https://cursor.com/docs/reference/third-party-hooks
[cu-mcp]: https://cursor.com/docs/context/mcp
[cu-sandbox]: https://cursor.com/docs/reference/sandbox
[cu-perms]: https://cursor.com/docs/cli/reference/permissions
[cu-cliconfig]: https://cursor.com/docs/cli/reference/configuration
[cu-forum-claudemd]: https://forum.cursor.com/t/cursor-loads-claude-md-even-when-the-third-party-rules-toggle-is-turned-off/149974
[cu-forum-optout]: https://forum.cursor.com/t/no-way-to-opt-out-of-third-party-claude-code-codex-config-loading-the-flag-is-hardcoded/166558
[cu-forum-statusline]: https://forum.cursor.com/t/cursor-cli-custom-statusline-replaces-native-footer-and-cannot-preserve-auto-review-state/166364

Not fetched this pass, linked from memory of the page names: `cc-subagents`, `cc-hooks`, `cc-mcp`, `cc-sandbox`, `cc-settings`, `cc-statusline` (Cursor's bundled skill links `cc-statusline` itself), `cu-sandbox` and `cu-perms` (seen in search results only). The `cx-config` link is the rendered form of the `config-reference.md` page that was fetched.
