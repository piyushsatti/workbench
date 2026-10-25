# agent-tmux spec (rev 2, 2026-10-04; updated for rev 3, phase 0 and phase 2 the same day, and for phases 3 and 4 on 2026-10-05)

> **Rev 3 direction (2026-10-04): Go MCP server, not a Bash wrapper.** The second review (G4) showed the Bash wrapper's entry hardening runs too late under `bash <path>`, so exposure moves to a stdio MCP server that Claude Code launches outside the sandbox. The sections below on "Invocation", "Entry hardening" and step 0 are superseded by this box and marked *Historical*; they are kept for the record. The verb rules, target grammar, string hygiene and owned-pane model still apply, tightened by G1–G13.
>
> - **Language:** Go 1.26 (`/usr/bin/go`), standard library only (no `go.mod` requires), MCP stdio JSON-RPC written by hand. One static binary, no interpreter or runtime environment to hijack.
> - **Source:** `server/` (agent-writable, never executed from there).
> - **Binary:** `~/.tmux/claude/agent-tmux-mcp` (sandbox cannot write). Built **by the user** from reviewed source in an unsandboxed shell, straight into that path, so no agent-writable binary is ever installed: `! cd <src> && GOTOOLCHAIN=local GOFLAGS='-trimpath -buildvcs=false' CGO_ENABLED=0 go build -o ~/.tmux/claude/agent-tmux-mcp . && sha256sum ~/.tmux/claude/agent-tmux-mcp`.
> - **Agent-side builds and tests** (sandboxed) use `GOCACHE=$TMPDIR/gocache GOTELEMETRY=off GOTOOLCHAIN=local` (the default cache and telemetry dirs are read-only in the sandbox; `GOTOOLCHAIN=local` stops any toolchain download).
> - **Registration:** user scope only (`~/.claude.json`, plain `claude` profile), command = absolute binary path, `env` set explicitly. Never a project `.mcp.json`.
> - **Process:** ignores its cwd; reads nothing but `/proc`, its config in `~/.tmux/claude/`, and tmux output; rebuilds its own env for each `exec.Command("/usr/bin/tmux", "-S", <const>, ...)`.
> - **Home pane:** resolved once at startup from the parent PID chain and cached for the server's lifetime; every tool call re-checks that it still exists in the same session, unlinked and ungrouped. The parent-comm check (direct parent must be the `claude` process, `requiredParentComm` in `home.go`) is **on**, confirmed as `claude` from a live server's startup line.
> - **State (2026-10-05):** phase 0 is done (results below). Phases 1 to 4 are built as server 2026.10.05. It carries the fixes from the first security review of 0.4.0: `send` and `kill` check that the shell is at its prompt, the wait pattern has a size cap, JSON keys must match exactly, every tmux query is framed by a row token, and the audit line carries a call number. and the fixes from the second review: the untrusted block carries a per-call token, the prompt check also requires the shell to be alone in its process group, and a refused `kill` is audited against its resolved target. 2026.10.05 is the first version that is a date; 0.4.0 was the last semver one. See Build order.

Lets a Claude Code agent see and change **its own** tmux session. Revs 1 and 2 specified a bash wrapper over the tmux CLI; rev 3 (box above) builds the same rules as a Go MCP server, and phases 1 and 2 of it are live. Rev 1 was audited (security lens); every rev 1 finding is mapped to its fix in the table at the end. The second review's findings (G1–G13) are folded into the sections below, and the ones still open are under Residual risks.

## Threat model

- The agent is semi-trusted. It runs in the Claude Code Bash sandbox, which can write only to the project folder and `$TMPDIR`, and cannot reach the tmux socket `/tmp/tmux-<uid>/default`.
- The tmux socket controls the whole server: every session, `run-shell`, `#()`. Reaching it at all is unsandboxed code execution. agent-tmux is the only path to it, and agent-tmux runs unsandboxed.
- So agent-tmux must not let the agent (a) run any command, (b) touch another session, or (c) cause typed input to execute in an unsandboxed shell or another agent.
- Text that comes back from `capture` is untrusted. It can contain prompt injection aimed at the agent.

## Files and install

| What | Where | Why |
|---|---|---|
| Binary | `~/.tmux/claude/agent-tmux-mcp` (sandbox cannot write; built by the user) | The agent must not be able to edit what runs unsandboxed |
| Script (*historical*, rev 2 Bash design) | `~/.tmux/claude/agent-tmux` (sandbox cannot write) | Same |
| Config | `~/.tmux/claude/agent-tmux.conf` (sandbox cannot write). Not built yet: phases 1–2 use constants in the source | Same: the agent must not flip its own guards |
| Audit log | `~/.claude/logs/agent-tmux.log` (sandbox-denied) | The agent cannot rewrite history |
| Source | `tools/agent-tmux/server/` | Build input and reference only, never executed from there |

- *Historical (rev 2 Bash design, superseded by the MCP server in the Rev 3 box).* **Invocation:** the agent runs `bash ~/.tmux/claude/agent-tmux <verb> ...`. Sandbox exception and permission rule are written against that exact prefix. (The exec bit cannot be set from the sandbox, and `bash <path>` avoids PATH lookup of `agent-tmux`.)
- *Historical (rev 2 Bash design; never run, moot under the MCP design).* **Open: does the sandbox exception hold?** Before any verb ships, test empirically against the installed Claude Code version that each of these is either sandboxed or denied, never run unsandboxed: `bash ~/.tmux/claude/agent-tmux whoami; id > $TMPDIR/p`, `... && id`, `... | sh`, `$(...)` around it, `X=1 bash ~/.tmux/claude/agent-tmux whoami`, `env X=1 bash ...`, `BASH_ENV=$TMPDIR/x bash ...`. If any one escapes, agent-tmux does not ship until that is closed.

## Entry hardening (before anything else in the script)

*Historical: rev 2 Bash design, superseded by the Rev 3 box.* Items 2–4 carry over to the MCP server unchanged in substance: a fixed `-S` socket and absolute tmux path with a rebuilt env per exec, the home pane from process ancestry, and refusal of linked or grouped home windows (checked at startup and again on every call). Item 1 is replaced by the server running from a user-built binary outside the sandbox.

1. Shebang `#!/bin/bash -p`; first line re-execs once under `env -i PATH=/usr/bin:/bin HOME="$HOME" /bin/bash -p "$0" "$@"` with a guard variable, so `BASH_ENV`, `ENV`, exported functions, `PATH`, `IFS`, `TMUX`, and `TMUX_PANE` from the caller are all discarded. (With the `bash <path>` invocation, `BASH_ENV` would be read by the outer bash before the script starts, so the sandbox-exception test above must prove that env prefixes are refused.)
2. tmux is always `/usr/bin/tmux -S /tmp/tmux-<uid>/default` (`-S` from a constant, not from `$TMUX`).
3. **Home pane = process ancestry, not env.** Walk the script's parent PIDs (`/proc/<pid>/stat`) and match the first ancestor PID against `#{pane_pid}` from `list-panes -a -F '#{pane_pid} #{pane_id}'`. No match → exit 4. Home session = that pane's `#{session_id}`.
4. Refuse if the home pane's window is linked into more than one session (`#{window_linked}` = 1) or the home session is in a session group (`#{session_grouped}` = 1): the scope is ambiguous there.

## Targets

- Accepted grammar only: `^[0-9]{1,4}$` (window index), `^[0-9]{1,4}\.[0-9]{1,4}$` (window.pane), `^%[0-9]{1,6}$` (pane ID), `^@[0-9]{1,6}$` (window ID). Everything else (names, `{last}`, `!`, `+1`, `~`, `{marked}`, `{mouse}`, `=`, `:`) is refused as `bad_target`. Pane tools (`split`, `label`, `resize`, `wait`, `send`) take only the pane forms `%N` and `N.M`; `rename_window` takes only the window forms `@N` and `N`; `capture` and `kill` take all four.
- **Targets are resolved in Go, never by tmux.** The server lists the home session (`list-panes -s -t $<home>`, strict `\x1f`-separated fields), resolves the agent's target against that session-qualified list, and hands tmux only the immutable `%pane` / `@window` ID. An index target never reaches tmux. The one session-qualified target is `new-window -t $<home>:`, which means "next free index in the home session".
- Resolve once to an immutable `%pane` / `@window` ID, check `session_id` equals home and `window_linked` is 0, then act on that ID only. Check and act cannot be one `tmux` invocation (`display -p -t %X '#{session_id}' \; kill-pane -t %X` is not conditional), so: resolve, check, act, re-verify. `capture` re-reads the pane after `capture-pane` and withholds the output if it left home. `kill` resolves twice, back to back, immediately before acting, and refuses if the two results differ.
- Every user string reaches tmux as a single argv element, never word-split. `;` is rejected anywhere in it, because an element ending in `;` splits into a second tmux command even after `--` (Q7). `--` is still passed where the command takes a positional argument, and a leading `-` is rejected, so a value is never read as an option. `send` text is the one exception, with narrower rules of its own (see Input).

## String hygiene (all agent-supplied text)

- Every agent-supplied string must be printable ASCII 0x20–0x7E. That rejects control characters, ESC, DEL, C1 bytes and all non-ASCII; a UTF-8 window name is an accepted loss. It must also not contain `#` (format expansion, Q1–Q4) or `;` (Q7), must not start with `-`, and must not be empty where a value is required. Do not try to escape `#` to `##`: rejection is simpler to reason about.
- Length cap 40 for names and titles, 200 for note text and the `wait` pattern, 1000 for `send` text.
- Strings go only to: `send-keys -l` (text, phase 4), `display-message -l` (note), `rename-window`/`new-window -n` (name), `select-pane -T` (label). With `#` rejected, format expansion in names/titles/`-c` cannot reach `#()`.
- Two strings follow narrower rules. `send` text may hold `#`, an inner `;` and a leading `-`, because `send-keys` expands formats only with `-F` and the text always follows `--`; it must still not end in `;` (Input). The `wait` pattern may hold `#`, `;` and a leading `-`, because it is compiled in Go and never reaches tmux or any argv. Both are still printable ASCII only.
- `-c DIR` (`dir`, optional): absolute; only `[A-Za-z0-9._/-]`; no `..` segment; must exist and be a directory (`os.Stat`); then canonicalised with `filepath.EvalSymlinks`, and the character set and the root are checked again on the result; must be inside the user's home directory. When `dir` is omitted the home pane's `#{pane_current_path}` is used and checked by the same rules; if it fails, the call errors out rather than falling back. `-c` is always passed explicitly, because without it a new window starts in the tmux client's cwd (Q10), which for agent-tmux-mcp is `/` (it changes to `/` at startup).

## Verbs

### Know (read-only)

| Verb | Notes |
|---|---|
| `whoami` | home session, window, pane ID, cwd, size |
| `list` | windows and panes in home: index, ID, name, active/last flag, command, cwd, `@claude_state`, pane and window ownership, and whether the window has `synchronize-panes` on. The listing is one `list-panes -s -F` call with 18 fields. Like every multi-field query agent-tmux makes, the format is framed by a random per-call row token at both ends, and a line is accepted only as one whole row: token, 18 fields, token. Without the token, a pane cwd, a command name or `@claude_state` holding a newline and `\x1f` could forge a second row. Its ownership columns are the inherited view a `-F` format gives, so a tag set by hand on a window or globally shows there; the tools that act read ownership per level instead (Layout) |
| `capture <pane> [-n LINES]` | default 200, max 2000; ANSI stripped; output wrapped as `--- BEGIN UNTRUSTED PANE OUTPUT %X TOKEN ---` … `--- END UNTRUSTED PANE OUTPUT %X TOKEN ---`. TOKEN is 26 random characters (`rand.Text`), drawn for each call after the text is read and drawn again while the text contains it, so it occurs exactly twice. Only the END line with the BEGIN line's token ends the block, and the tool description says so. A pane can print END lines, but not the one with this call's token, which does not exist until the text is read. Text inside the output that looks like the marker is also neutralised, as a second layer for a reader that skips the token. That layer cannot be complete: it misses look-alike letters (a Cyrillic `А`, fullwidth letters), a zero-width character inside a word and separators outside its class such as `/`. The match ignores case, and between the three words it allows any run of whitespace, control characters, Unicode space separators, format characters (zero-width space, soft hyphen), `_`, `.` and `-`. Every other piece of tmux-held text a tool returns is neutralised the same way: `list` and `whoami` fields, the `dir` that `new_window` and `split` report, and the text of every error. A pane's command quoted in a refusal is neutralised before it is quoted, because quoting escapes a non-ASCII space and would hide the look-alike. Log records target and byte count. **Phase 1 scope is any pane in the home session, on permission `ask`**; owned-only scope (pane-level tag, never the home pane) is implemented for when config arrives |
| `tmux_wait(pane, pattern, timeout?, lines?)` | Built 2026-10-05 (phase 3). agent-tmux-owned panes only (pane-level tag), never the home pane. `pattern` is a Go RE2 regular expression, compiled in Go: printable ASCII, 1–200 characters, and a compile error is `bad_input`. There is no grep and no subprocess. Go's RE2 engine never backtracks exponentially, but its time is linear in the text times the compiled program, and 200 characters can compile to thousands of instructions (`(?:.*){999}Q` compiles to 2001 and takes about 0.6 s over 200 lines of 300 columns). So a pattern that compiles to more than 1000 instructions is refused at once as `bad_input`; ordinary patterns, even a 200-character literal, compile to about 400 or fewer. Polls `capture-pane -p -t %ID -S -LINES` about every 500 ms, strips ANSI, and searches the last `lines` lines (1–200, default 20), newest first. A line already on screen matches on the first poll, and the tool description warns about this. `timeout` is 1–300 s, default 30. Result `{matched, line, elapsed_ms}`, with `line` wrapped in the UNTRUSTED block with its own token and neutralised as in `capture`. A timeout is a normal result with `matched: false`. Cancellation stops the wait between polls and between lines within a poll. Every poll re-reads the pane: a pane that closes or leaves the home session during the wait is `out_of_scope`, and one that loses its own `@agent-tmux_owned` tag is `refused`. Audit: one line, with result `ok`, `timeout`, `cancelled` or the error code, the pattern as length and SHA-256, and the matched line's byte count |

`idle` is **dropped**: shell-prompt detection is unreliable (subshells, ssh, spoofed prompts) and a false "idle" leads to typing into a live program.

### Layout (phase 2)

**Owned** means agent-tmux created it. `new_window` and `split` set the pane option `@agent-tmux_owned 1` on every pane they create, and `new_window` also sets the window option `@agent-tmux_owned_window 1`. The window tag has its own name because pane options inherit from window options (tmux(1), OPTIONS): a window-level `@agent-tmux_owned` would make a human's later split in that window read as owned.

**Ownership is read at its own level (2026-10-05).** A pane is owned when `show-options -p -q -v -t %ID @agent-tmux_owned` prints `1`; a window is owned when `show-options -w -q -v -t @ID @agent-tmux_owned_window` prints `1`. Neither read passes `-A` or `-g`, so a tag inherited from the window, the session or the global options does not count, and `-q` prints nothing for an unset option. The value must be exactly `1`: any other value, or nothing, is not owned, and a read that fails is `tmux_error`, never owned. A `-F` format such as `#{@agent-tmux_owned}` resolves the tag through all of those levels, so phase 2's format-based check counted a hand-set window or global tag. Every tool that acts on ownership now uses the per-level read: `kill`, `send`, `wait`, `split`, `label`, `resize`, `rename_window`, and `capture`'s owned scope. Only `list` still shows the inherited view.

| Tool | Notes |
|---|---|
| `tmux_new_window(name?, dir?)` | `new-window -d -P -F '#{window_id}\x1f#{pane_id}' -t $<home>: -c DIR [-n NAME]`, with no shell-command, so tmux starts the default shell. The new pane is re-read and must be in the home session and the reported window before it is tagged; otherwise it is left untagged, nothing is killed, and the call errors. Tags pane and window; returns the IDs |
| `tmux_split(target?, direction h\|v, dir?, percent?)` | target defaults to the home pane. Allowed: the home pane or an owned pane, never a human or other-Claude pane. Refused if any pane in the window has `synchronize-panes` on (Q9). `split-window -d -h\|-v [-l N%] -P -F ... -t %ID -c DIR`, percent 10–90. Tags the new pane only |
| `tmux_rename_window(target?, name)` | default: the window the home pane is in now. Allowed: the home window or an owned window. `rename-window -t @ID -- NAME` |
| `tmux_resize(target, width?, height?)` | owned panes only (resizing shared panes moves the human's layout). `resize-pane -t %ID -x N -y N`, 5–500 |
| `tmux_kill(target)` | Pane (`%N`, `N.M`): owned, not the home pane, no `@claude_state`, `pane_current_command` is `bash`, `sh` or `zsh` (agent-tmux starts only the default shell, so any other command was started by the human), the shell is at its prompt, and no process under the pane has comm `claude`. "At its prompt" means the terminal's foreground process group (`tpgid`, field 8 of `/proc/<pane_pid>/stat`, counted after the last `)`) is the shell's own pid, so no job the shell started holds the terminal; `bash -c ...` started at the prompt reads as `bash` in `pane_current_command` but fails this. The shell must also lead its own process group (`pgrp`, field 5, equals its pid) and be the only process in it, because a command substitution `$(…)`, or any command run with job control off (`set +m`), runs in the shell's group while the shell keeps the terminal. A background job started with `&` gets its own group and does not block the check. The `claude` check walks the process tree from `pane_pid` through `/proc/<pid>/stat` (parsed as in `home.go`). The scan reads at most 4096 processes system-wide, and it refuses when it reaches that cap or cannot read `/proc`. A process that exits mid-scan is skipped; any other unreadable or malformed stat refuses. Window (`@N`, `N`): the window carries its own `@agent-tmux_owned_window` tag, does not contain the home pane, `window_linked` 0, and every pane in it passes the pane rule. Resolved twice right before acting; any difference refuses. The shell and process-tree checks were added 2026-10-05 (G11), the prompt check after the first security review of 0.4.0, and the process-group check after the second |

`select` does not exist, because stealing focus from the human is disruptive. `new-window` and `split` pass `-d` so they never change focus either, and no tool runs `select-window`, or `select-pane` without `-T`. No agent argument is ever a tmux command or a shell command.

### Input

**Typed input can execute without Enter** (readline `M-1 M-#` accepts the line; Claude Code permission dialogs and `y/N` prompts act on one key; vim/less act on letters). "No Enter" is a convention, not a safety boundary. Rev 2 therefore narrows input:

| Verb | Notes |
|---|---|
| `tmux_send(pane, text)` | `send-keys -l`, never Enter. Phase 4 rules below. Text is printable ASCII only, so no ESC, no newline, no C1 |
| `keys` | **dropped.** Every allowlisted key had a hazard: `Escape` is the readline meta prefix and interrupts other agents, `C-c`/`C-d` kill or exit, arrows recall history, `q` quits programs |

**Where may `send` type?** Rev 1: any pane, including Claude panes. Audit: a single typed digit can answer another Claude's permission prompt (for example "Yes, and don't ask again"), and text left in another agent's input box is an injection channel that a human Enter later submits. Rev 2 weighed:
- **A. agent-tmux-owned panes only** (audit recommendation, rev 2 default).
- B. Any home pane except panes with `@claude_state` set, and only when `pane_current_command` is a shell.
- C. Any home pane (rev 1), accepting the above.

**Phase 4 rules (decided 2026-10-04, built 2026-10-05 in server 0.4.0; the process-group check is in server 2026.10.05): A, tightened with B's checks and the phase 0 results.** `send` resolves the pane in Go against a fresh home listing and refuses unless all of these hold:
- the pane is owned and its window is owned, each read at its own level (Layout);
- it is not the home pane;
- the pane's shell is at its prompt: `pane_current_command` is `bash`, `sh` or `zsh`, the terminal's foreground process group is the shell's own pid, the shell leads its own process group and no other process is in it (the `kill` rule in Layout), and no process named `claude` runs under it. `pane_current_command` alone is not enough, because `bash -c 'read -n1 x; ...'` started at the prompt reads as `bash`, and one typed key would answer it. The terminal alone is not enough either, because `x=$(bash -c 'read -n1 c')` runs its child in the shell's group while the shell keeps the terminal. Every read of `/proc` that fails or does not parse refuses;
- `@claude_state` is unset (not proof on its own, see G11);
- the pane is not in copy mode or another mode (`pane_in_mode` 0), where the keys would drive the mode's key bindings instead of reaching the shell (added while building);
- no pane in the window has `synchronize-panes` on, because with it on `send-keys -t <pane>` also reaches the other panes (Q9);
- the text is printable ASCII 0x20–0x7E and 1–1000 characters, because `send-keys -l` passes C1 bytes and UTF-8 through unchanged (Q8);
- the text does not end in `;`, refused as `bad_input` because tmux reads a trailing `;` as a command separator (Q7). A `;` inside the text is fine, and so are `#` and a leading `-` (String hygiene).

As built:
- **Argv.** The only argv is `send-keys -l -t %ID -- TEXT`. No path sends `Enter`, `C-m` or `C-j`, and the text cannot hold a control character, so agent-tmux never runs what it types.
- **Re-check.** After the `intent` audit line, every check runs again on a fresh listing immediately before `send-keys`. If the pane, its window, its command or the window's pane list differs, `send` refuses with "target changed while it was being checked; nothing was sent".
- **Result and audit.** The result is `{pane_id, bytes}`. The audit log gets the `intent` line, then the outcome, with the text logged as length and SHA-256.
- **Description.** The tool description says it never presses Enter and that the human decides whether to run the text.

**One human Enter in an owned pane is unsandboxed execution.** An owned pane is a normal shell outside the sandbox. Whatever `send` leaves on its command line runs as the user the moment a human presses Enter there, or any key the shell binds to accept-line. The human is the last check, and the window name and pane title are the only cues they get.

Own pane (the home pane) is always refused: self-typing would queue prompts into its own input box.

### Status

| Verb | Notes |
|---|---|
| `tmux_note(text)` | `display-message -l -d 3000 -c <client> -t %home -- TEXT`, once for each pty client attached to the home session (`list-clients -t $<home>`). `-c` is required: `display-message` draws on its target client, `-t` only supplies format context, and without `-c` tmux picks a client from the whole server, which can belong to another session. No client attached: error `no_client` |
| `tmux_label(target?, title)` | `select-pane -T TITLE -t %ID`; the home pane or owned panes; does not change focus (Q11) |

## Plumbing

- *Historical (Bash design):* `--dry` prints the exact argv it would pass to tmux.
- *Historical (Bash design):* exit codes 0 ok, 2 usage/bad input, 3 out of scope, 4 refused by rule, 5 timeout. The MCP server instead returns `isError` with a fixed code before the message: `bad_input`, `bad_target`, `out_of_scope`, `refused`, `no_home`, `no_client`, `tmux_error`, `audit_unavailable`, `cancelled`, `internal`.
- Log line: ISO time, the server's pid and a call number, home pane, tool, resolved target IDs, result code, byte count for capture, wait (the matched line) and send (the text), IDs a call created (`new=@W,%P`). A refusal that comes after the target resolves is logged against the resolved ID, so a refused `kill` of `0.1` logs `target=%2`, not `target=-`. The call number is the server's own counter, one per call, so an `intent` line and its outcome share it and calls running at the same time can be told apart; with the pid it stays unique across servers writing the same log. The client's JSON-RPC id is never logged. Besides `ok`, `intent` and the error codes above, the result can be `timeout`, for a `wait` that ran out of time. Free text (names, titles, note text, dirs, raw targets) is not logged verbatim: log its length and a SHA-256, so names cannot forge log lines.
- **If an audit line cannot be written, the call is refused.** Read-only tools withhold their result. Tools that change tmux first write an `intent` line and refuse before touching tmux if that write fails; the outcome line follows the action.
- **Concurrent calls (2026-10-05):**
  - Each `tools/call` runs in its own goroutine with its own context, so the stdin read loop never waits on a tool, and a long `wait` does not hold up `ping` or other calls.
  - Every reply is marshalled first and written as one whole message in one `Write`, under one mutex.
  - At most 16 calls run at once. A call beyond that is refused with `refused` and audited. A request that reuses the id of a call still running gets JSON-RPC error -32600 with id `null`, because a reply carrying the live id would be taken as the answer to the running call.
  - `notifications/cancelled` cancels that call's context, and the call gets no reply, as MCP specifies.
  - Stdin EOF cancels every running call. The server exits only after each one has finished, written its audit line and tried to write its reply.
  - `wait` watches its context between polls and between lines. A tool that changes tmux checks it once, just before its `intent` line: a call cancelled by then returns `cancelled` and changes nothing, and past that point it runs to the end, so a cancel can never leave a half-built window or an untagged pane. `whoami`, `list` and `capture` do not watch it. Audit ordering holds within each call: `intent` before any tmux change, the outcome after it.
  - SIGPIPE is ignored, so a client that closes its end of stdout turns reply writes into `EPIPE` errors instead of killing the server mid-call. A failed reply from a running call is logged to stderr and dropped; a failed reply from the read loop ends the loop, and the server still waits for the running calls as at EOF.
- **JSON keys must match exactly.** Go's decoder matches object keys without regard to case (Unicode folding, so `ſ` matches `s`) and the last of two equal keys wins, so one message could read one way to the client's checks and another way to agent-tmux. Before decoding, agent-tmux scans the keys of every object it reads, all of them flat; nested objects it never reads, such as `capabilities` and `_meta`, are not scanned. Tool arguments must use exactly the schema's keys, each once; the envelope, `initialize` params, `tools/call` params and `notifications/cancelled` params may carry unknown keys, but not a case or fold variant of a known key and not a key twice. A bad envelope is -32600 with id `null`, bad params -32602, bad arguments `bad_input`, and a bad cancel is ignored.
- `agent-tmux.conf` (not agent-writable, not built yet): `capture_scope = owned|session`, `capture_max_lines`, `send_scope = owned|nonclaude|any`.

## Build order

*Historical (rev 2):* 0. sandbox-exception probe; 1. entry hardening, home pane, resolver, hygiene, log, `whoami`, `list`, `--dry`; 2. `new-window`, `split`, `rename-window`, `label`, `note`; 3. `capture`, `wait`; 4. `send`, `resize`, `kill`. Rev 3 reordered it as below.

0. Phase 0: manual tmux checks on a private server. Done 2026-10-04; results below.
1. Phase 1: `tmux_whoami`, `tmux_list`, `tmux_capture`, with the ancestry home pane, target resolver and audit log. Done 2026-10-04. Capture scope is the whole home session, on permission `ask`.
2. Phase 2: layout tools `tmux_new_window`, `tmux_split`, `tmux_rename_window`, `tmux_label`, `tmux_note`, `tmux_resize`, `tmux_kill`. Implemented and tested against the fake tmux 2026-10-04. Manual tmux checks cover the live behaviours they assume. Full live pass 2026-10-05: every tool worked on owned objects and every refusal held. Kill refusals against real panes were left to the unit tests.
3. Phase 3: `tmux_wait`, with concurrent tool calls so a wait never blocks the server. Built and tested against the fake tmux 2026-10-05 (server version 0.4.0).
4. Phase 4: `tmux_send`, under the rules in Input, together with the `kill` hardening (shell-only, no `claude` process under the pane) and the per-level ownership reads. Built and tested with phase 3 in the same 0.4.0 source. Live checks on tmux 3.6 passed the same day: `send` typed `{`, `}`, `-n hi`, `#{session_name}`, `~` and `echo a; echo b` literally; a `bash -c 'read -n1 x'` child made `send` and `kill` refuse; `wait` did not match early and then matched; a 2001-instruction pattern was refused; an owned window at its prompt was killed. They also found that a refused `kill` logged `target=-`, fixed in 2026.10.05.
5. Server 2026.10.05: the fixes from the second security review (per-call token on the untrusted block, the process-group check, the `kill` audit target). Built and tested against the fake tmux 2026-10-05. Its live checks in `server-README.md` were not run; an earlier manual check showed an idle login shell passes the process-group check.

## Phase 0 results (2026-10-04, tmux 3.6, private server)

| Q | Result | What it forces |
|---|---|---|
| 1–3 | `new-window -n`, `rename-window` and `select-pane -T` format-expand their argument (`#{session_name}` became the session name). `#()` did not run in those tests; treat it as possible | `#` rejected in every name and title |
| 4a | `new-window -c` expanded the path **and executed `#()`** | `dir` limited to `[A-Za-z0-9._/-]`, checked before and after symlink resolution |
| 4b | `split-window -c` expanded the path (`#()` did not run in that test) | Same rule for `split` |
| 5 | `display-message -l` is literal; without `-l` it expands | `note` always passes `-l` |
| 6 | `#{q:...}` escapes for sh correctly (space, `'`, `$()`, `#` become `\#`) and round-trips under `sh -c` | Nothing relies on it: agent-tmux never builds shell strings |
| 7 | An argv element equal to `;` or ending in `;` splits into a second tmux command, even after `--`. Mid-string `a;b` does not split | `;` rejected anywhere in agent strings (mid-string too, for simplicity); every argv element is checked again before exec. `send` text, where a `;` inside a shell command is common, rejects only a trailing `;` |
| 8 | `send-keys -l` passes C1 bytes 0x80–0x9F and UTF-8 through unchanged | Phase 4 `send` accepts printable ASCII only |
| 9 | With `synchronize-panes` on, `send-keys -t <pane>` also reaches the other panes | `split` and phase 4 `send` refuse windows with it on |
| 10 | `new-window` without `-c` starts in the client's cwd | `new_window` and `split` always pass an explicit `-c` (agent-tmux-mcp itself runs in `/`) |
| 11 | `select-pane -T` changes neither the active pane nor the current window | `label` uses it as is |
| 12 | Not measurable headless | Nothing yet |
| 13 | The `\x1f` separator passes through `-F` unchanged | `sep` in `tmux.go` stays `\x1f` |

## Residual risks

Open after the second security review, with their state on 2026-10-05:

- **G2: owned panes are unsandboxed shells.** `new_window` and `split` start the user's default shell outside the sandbox, and `send` now puts text on their command lines. agent-tmux never presses Enter, but a human Enter in one runs whatever is there. `split` from the home pane places such a shell inside the human's own window. Mitigated outside agent-tmux (2026-10-05): the `pane-border-format` in `~/.tmux/claude/tmux-status.conf` shows a yellow `agent-tmux: check before Enter` tag on every pane where `@agent-tmux_owned` is set, so the human can see which panes agent-tmux may have typed into.
- **G3: `-c` into a directory the agent controls.** `dir` must be inside home, and the sandbox can write the project folder. A new shell started in a repo whose `.git/config` the agent wrote runs `core.fsmonitor` as soon as anything runs `git status` there, and a git-aware prompt does that at the first prompt. agent-tmux does not inspect directories for this. Accepted without a code change (2026-10-05): a agent-tmux shell there has the same exposure as any shell the human opens there.
- **G7: titles outlive the session through resurrect.** tmux-resurrect saves pane titles and folders and re-applies them at restore, and `select-pane -T` is format-expanded (Q3). A `label` title containing `#` is already rejected, but the saved titles also include Claude Code's own model-generated topic titles (and anything a program sets through the terminal title escape), which agent-tmux never sees. Mitigation outside agent-tmux: patch tmux-resurrect's `scripts/restore.sh` so `restore_pane` doubles every `#` in the saved folder and title, and a `#()` in one cannot run at restore. A TPM update of tmux-resurrect reverts the patch, so note it in your tmux config.
- **G11: a missing `@claude_state` does not mean the pane isn't Claude.** agent-tmux does not write it; whatever sets it should derive it from Claude Code's own `TMUX_PANE`, so the agent cannot forge it. But panes without it include Claude sessions where nothing sets it, `claude -p`, the time before session start, and a crash with no session end (which leaves stale state behind). Since 2026-10-05, `kill` refuses any pane whose `pane_current_command` is not `bash`, `sh` or `zsh`, and any pane with a process named `claude` anywhere under it. A window kill needs every pane to pass, and `send` requires a shell too. None of this is proof, because a process name is easy to change and the process-tree walk checks only names. `@agent-tmux_owned` also means "created by agent-tmux", not "used only by the agent": a shell the human has adopted stays killable while its shell is at its prompt.

Found while building phase 2:

- **Ownership tags are plain user options.** Anyone with tmux access can set `@agent-tmux_owned` on a pane; the agent cannot, since no tool sets arbitrary options. Since 2026-10-05 a tag counts only when it is set on the pane or window itself (Layout), so a window-level or global `@agent-tmux_owned 1` set by hand no longer makes the panes under it owned for any tool that acts. `list` still shows the inherited view.
- **`dir` is checked, then used.** The path is canonicalised before `new-window`/`split` runs, and a component the agent can write could be swapped for a symlink in between. The result is a shell starting somewhere else, which is G2/G3 territory rather than a new capability.

Found while building phases 3 and 4:

- **`send` checks, then types.** The checks re-run immediately before `send-keys`, but they are separate tmux calls. If the human starts a program in the pane between the re-check and `send-keys`, that program receives the text. The text is still literal and has no Enter.
- **Concurrent sends can follow each other.** Calls now run in parallel, so two `send` calls to one pane can both pass their checks and type one after the other on the same command line. Each is still literal printable text with no Enter.
- **The `kill` process scan is capped system-wide.** The walk reads at most 4096 processes from `/proc` to build the tree, and refuses at the cap. On a machine running more than 4096 processes every `kill` and every `send` refuses (fail closed), until the cap in `proc.go` is raised.

Found in the security reviews of 0.4.0 and left open after their fixes:

- **The prompt check reads the terminal and the process table, not the shell's state.** `send` and `kill` require the shell to own its terminal and to be alone in its own process group. A job in the foreground, a command substitution and any command run with job control off (`set +m`) break that, so server 2026.10.05 refuses them. Some input readers still pass:
  - a builtin such as `read` (or `select`, or a shell function that reads), run at the prompt, is the shell, so the check passes and one typed key can answer it;
  - a program started with `exec` under a shell's name, such as `exec bash -c 'read ...'`, is the pane's own pid and alone in its group, so it passes the same way.
  The text is still literal printable ASCII with no Enter. The group check also refuses more than it must: a prompt hook that runs a command substitution (a git-aware prompt, for one) fails it for the moment the hook runs, and a retry clears that; a helper the shell keeps running in its own group (some zsh asynchronous prompt plugins) would make every `send` and `kill` on that pane refuse. Both fail closed.
- **Readline vi command mode acts on printable keys.** In a shell set to vi editing and left in command mode, `#` comments out the line and accepts it, and with a count (`1#`) it removes a leading `#` and accepts the line, which runs it. `v` opens the line in the editor and runs it when the editor exits, so `vZZ` in one send runs the current line. Emacs mode needs ESC for the same (`M-#`), and `send` rejects ESC. agent-tmux cannot see the editing mode. The pane must already be in vi command mode, which only a human's ESC puts it in.
- **`wait` can spend CPU.** The 1000-instruction cap bounds one scan to about 0.5 s for 200 lines of 300 columns (0.53 s measured for `(?:\pL*){498}Q`). 16 concurrent waits near the cap, each polling every 500 ms for up to 300 s, can keep several cores busy for five minutes. It reads only owned panes and returns at most one line, so this costs time, not scope.

## Permissions plan (2026-10-05)

Apply this by hand to `~/.claude/settings.json`; agent-tmux does not depend on it.

| Tool | Permission | Why |
|---|---|---|
| `tmux_whoami`, `tmux_list`, `tmux_label`, `tmux_note`, `tmux_rename_window` | allow | Unchanged |
| `tmux_new_window`, `tmux_split`, `tmux_resize` | allow (was ask) | They create or move panes but type nothing, and every pane they create shows the G2 border tag |
| `tmux_wait` | allow | Reads owned panes only, and returns at most one line, in the UNTRUSTED block with its per-call token |
| `tmux_capture` | ask | Unchanged: its scope is still the whole home session |
| `tmux_send` | ask | Text it types runs on the human's next Enter, so each send is shown to the human first |
| `tmux_kill` | ask | Unchanged: the one destructive tool |

## Rev 1 finding → rev 2 fix

| Finding | Fix |
|---|---|
| F1 status-right `#()` script in sandbox-writable path | Done 2026-10-04: `tmux-status.conf` and its status script moved to `~/.tmux/claude/`, and the tmux config updated to load them from there |
| F2 `note` format expansion | `display-message -l`, `#` rejected |
| F3 `-c`/`-n`/rename/`-T` format expansion | `#` rejected in all strings; `-c` realpath-checked |
| F4 env trust (`BASH_ENV`, `PATH`, functions) | `bash -p`, `env -i` re-exec, absolute tmux, fixed `-S`; probe in step 0 |
| F5 compound commands vs `excludedCommands` | Step 0 probe; ship nothing until it passes |
| F6 `Escape` + `M-#` accepts line | `keys` dropped; control chars rejected in `send` |
| F7 typing into other Claude panes | Phase 4 rules: A (owned panes in owned windows, or in the home window since 2026-10-09), plus shell, `@claude_state`, mode and sync checks. Built 2026-10-05 |
| F8 single-key TUIs | `keys` dropped; `send` default limited to owned panes |
| F9 `TMUX_PANE` spoofing | Home pane from process ancestry; caller env discarded |
| F10 target TOCTOU and syntax | Strict grammar, session-qualified indexes, resolve to ID once |
| F11 argv ending in `;` | Rejected; one argv element per string; `--` |
| F12 linked/grouped windows | Refuse when `window_linked` or `session_grouped` |
| F13 `capture` leaks/injection | UNTRUSTED block whose BEGIN and END lines carry a per-call random token the pane cannot know, so only the real END line closes it; marker look-alikes also neutralised as a second layer; logged. Phase 1 ships session scope on permission `ask`; owned scope implemented for later |
| F14 `kill` blast radius | Owned panes only (tag read at its own level); only panes running `bash`, `sh` or `zsh`, with the shell at its prompt; never a pane with a `claude` process under it |
| F15 file locations | Fixed under `~/.tmux/claude/`; log stores hashes not raw text |
| F16 `wait` grep options / ReDoS | No grep and no subprocess: the pattern is compiled with Go's RE2 `regexp`, which never backtracks exponentially. Its time is linear in the text times the compiled program, so a pattern that compiles to more than 1000 instructions is refused; a scan of 200 lines of 300 columns then costs about 0.5 s at most (Residual risks). The timeout (1–300 s) is enforced in Go (rev 2 planned `grep -E -e` under `timeout -k` with a 600 s cap) |
| F17 `idle` unreliable | Dropped |
| F18 `send` to own pane | Refused |
| F19 `resize` shared panes | Owned panes only |
