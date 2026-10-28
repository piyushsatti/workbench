# agent-tmux

A stdio MCP server that lets an agent see and lay out its **own** tmux session, so its work happens in visible panes instead of hidden forked subagents ([decisions](../../docs/decisions.md#what-each-piece-is)). It can list and capture panes, open windows and splits, rename and label them, resize and kill panes it owns, wait for a pattern, and type into panes it owns. It never presses Enter.

One guard depends on your tmux setup: `tmux_send` and `tmux_kill` refuse any pane that carries the `@claude_state` pane option, but agent-tmux does not write that option itself. If nothing in your setup sets it, that guard marks no Claude panes. The other guard, refusing a pane with a `claude` process under its shell, still holds, but one of the two layers is missing.

A rename of `agent-tmux` changes the tool folder, the manifest `name`, the MCP server key in `.mcp.json`/`mcp.json` and every permission rule that names `mcp__agent-tmux__*`, so it lands as one change.

## Tools

`tmux_whoami`, `tmux_list`, `tmux_capture`, `tmux_new_window`, `tmux_split`, `tmux_rename_window`, `tmux_label`, `tmux_note`, `tmux_resize`, `tmux_kill`, `tmux_wait`, `tmux_send`. The rules for each one (ownership, prompt checks, the untrusted-output block, the audit and event logs) are in [docs/server-README.md](docs/server-README.md) and [docs/agent-tmux-SPEC.md](docs/agent-tmux-SPEC.md).

## Harness support

| Harness | Claimed | Works | Reduced | Missing |
|---|---|---|---|---|
| Claude Code | yes (`.claude-plugin/plugin.json`) | every tool, on Linux, when the session runs inside a tmux pane | — | — |
| Codex | **no** | — | — | every tool refuses: the server requires its direct parent process to be named `claude` (`requiredParentComm` in `server/home.go`) |
| Cursor | **no** | — | — | same as Codex |

Why only Claude Code, although MCP is shared: the server is plain stdio JSON-RPC, but it is not harness-neutral.

- **Parent process.** `DetectHome` refuses unless the direct parent's comm is `claude`. Under Codex or Cursor that check fails and every tool returns `no_home`.
- **Claude-specific paths and names.** The audit log is `~/.claude/logs/agent-tmux.log`. `tmux_send` and `tmux_kill` refuse a pane with a `claude` process under it, and refuse a pane carrying the `@claude_state` pane option, which agent-tmux does not set itself.
- **Linux only.** It reads `/proc/<pid>/stat` for the home pane and the prompt checks, runs `/usr/bin/tmux`, and uses the socket `/tmp/tmux-<uid>/default`. On macOS there is no `/proc`, so every tool refuses (fails closed). Linux is the intended host.

Claiming Codex or Cursor would need a configurable parent name and harness-neutral log paths in the Go source. That's a source change, so it isn't made here. `mcp.json` ships anyway because the layout requires it to match `.mcp.json`.

## Install

Nothing is installed by this folder. The binary is built from reviewed source by you in an unsandboxed shell, straight into a path the agent sandbox can't write (agent-tmux runs from `~/.tmux/claude/`; see [decisions](../../docs/decisions.md#repos-and-layout)).

`install.json` uses a `build` form instead of `files`, because the executable is compiled rather than copied:

```json
{
  "build": "GOTOOLCHAIN=local GOFLAGS='-trimpath -buildvcs=false' CGO_ENABLED=0 go build -C server -o ~/.tmux/claude/agent-tmux-mcp .",
  "dest": "~/.tmux/claude/agent-tmux-mcp",
  "files": []
}
```

- `build` is a shell command run from the tool folder. `go build -C server` changes into `server/` first, because `go.mod` lives there. The flags are the ones the original build used.
- `dest` is the one path the command writes, so an installer or reviewer can check it and record its sha256 afterwards (`sha256sum ~/.tmux/claude/agent-tmux-mcp`).
- `files` is empty: nothing is copied as-is.

The MCP entry (`.mcp.json`, byte-identical to `mcp.json`) runs `$HOME/.tmux/claude/agent-tmux-mcp` with no arguments, and sets `PATH=/usr/bin:/bin` and `LANG=C.UTF-8` explicitly, as the original user-scope registration did. The server takes no flags. It reads one optional variable, `AGENT_TMUX_EVENT_LOG`, an absolute path that overrides `~/.local/state/agent-tmux/events.jsonl`.

A running session keeps the old server process after a rebuild until `/mcp` reconnects it.

## Installing

agent-tmux is a `pi` plugin, cataloged from this folder rather than its own repo. It ships manifests for Claude Code, Codex and Cursor; each only registers the MCP server.

- **Remove the old user-scope registration first.** If you registered it at user scope in `~/.claude.json` (as `agent-tmux`), remove that first; enabling the plugin as well would start a second server under a different tool prefix.
- **The command is fixed:** `.mcp.json` runs only `$HOME/.tmux/claude/agent-tmux-mcp`, a path outside agent-writable folders. The checkout itself is agent-writable, so a changed `command` would run on the next start; review changes to `.mcp.json` like code.
- **`$HOME` expansion** in `command` follows [CONVENTIONS.md](../../CONVENTIONS.md). It hasn't been checked at V3.

## Layout

- `server/`: the Go source, which lives here. Standard library only, so there is no `go.sum`, and there was no `testdata/`.
- `docs/`: the design doc (`agent-tmux-SPEC.md`) and the server's own README as `server-README.md`. Home paths are written as `~` or `<plugin>`/`<agent-tmux-source>` placeholders.

## Tests

```
cd server && GOCACHE=$TMPDIR/gocache go test ./...
```

The tests use a fake tmux and a fake `/proc`. `TestProcTreeRealProc` reads the real `/proc` and skips where there is none, as on macOS. Results are in [VERIFIED.md](VERIFIED.md).
