# workspaces

`workspaces` (also `awm`, agent workspace manager) starts an AI-agent harness only from allowed folders, always inside tmux, and logs every launch to `~/.local/state/workspaces/launches.jsonl`. It ships two adapters, `claude` (Claude Code) and `codex` (see Codex below); the built-in default harness is `claude`. To route a harness through it, define a bash function such as `claude() { workspaces run . claude "$@"; }`. `command claude` goes around the check.

It was called `agent-launcher` (binary `agent`) until 2026-10-24. Cursor's CLI installs itself as `~/.local/bin/agent`, so the two kept replacing each other.

```
workspaces run [space] [harness] [harness-args...]    check, log, then exec the harness
workspaces check [--json] [space] [harness] [args...] show what run would do; writes nothing
workspaces launch list [-n N] [--json]                the last N launch records
workspaces space list                                 each space: root, attach count, todo, log, last launch
workspaces space show [space]                         one space: paths, check results, last five launches
workspaces space add <name> [root]                    add a space; the root must pass the launch check
workspaces space rm [--yes] [space]                   remove a space
workspaces space attach [space] [path]                attach a folder to a space
workspaces space detach [--yes] [space] [path]        detach an attached folder
workspaces harness list                               each harness: adapter, bin, env keys; the default marked
workspaces harness add <name> --adapter A [--bin P] [--env K=V]...
workspaces harness rm [--yes] [harness]               remove a harness other than the default
workspaces policy list                                the global allow_launch, deny_launch and deny_access lists
workspaces policy add <list> <path>                   add a path to a global list
workspaces policy rm [--yes] <list> [path]            remove a path from a global list
workspaces todo list [--json] [space]                 the space's TODO.jsonl, grouped by domain
workspaces log list [-n N] [--json] [space]           the last N records of the space's LOGS.jsonl
workspaces session list [--harness H] [-n N] [--json] [space]
                                                 a space's Claude sessions, newest first
workspaces session resume [--harness H] [space] [session]
                                                 resume one through the same path as run --resume
workspaces session prune [--harness H] [--yes] [space]
                                                 delete chosen sessions' transcripts
workspaces doctor                                     one row per setup check: OK, WARN or FAIL
workspaces hook session-start                         for Claude Code's SessionStart hook (below)
workspaces --version                               CalVer of the build, e.g. 2026.10.05
```

`.` as the space means "here". Inside tmux the harness runs in the current pane. Outside tmux, a session launch opens a new window in the tmux session named after the space (or `main`) and attaches to it. Print mode outside tmux, and background sessions (`--bg`, `respawn`), are refused.

## Views

`space`, `todo` and `log` only read; they never write the launch log. Without a space argument they use the space whose root holds the current folder (the longest root wins), then a picker on a terminal; without a terminal they exit 64.

A space's todo file is `<root>/TODO.jsonl` and its log file `<root>/LOGS.jsonl`, unless `todo` or `log` in `spaces.json` names another (relative paths resolve against the root). A path that is not a regular file is unreadable (exit 74). Records are read loosely: unknown fields are kept, missing ones show as `-`, and a `{"_meta": true, ...}` record is skipped. A line that is not a JSON object is reported on stderr with its line number and skipped, without quoting it; after five in one file, one summary line counts the rest.

`todo list` groups by the first of `domain`, `major_category` and `category` a record has, and shows `id` (or `#<line>` when there is none), `status`, then `[tag]` and `title`. `log list` shows `date` (or the date part of `ts`), `kind`, the same group key, `status`, then `[tag]` and `title`. A record without a title shows its `text`, then its `id`. Titles are cut to 72 characters and other fields to 32; in the table views, whitespace runs fold to one space and control and format characters (such as bidi overrides) show as U+FFFD, so an escape sequence in a record never reaches the terminal. `space show` lists the space's own deny_access entries and the global ones separately. A space with only a `TODO.md` beside its todo path shows as "TODO.md only"; TODO.md is not read, use the JSONL todo file. `--json` prints the raw record lines, unfiltered, for piping.

`check` warns about an attach path (`attach_denied <rule>`, as `space show` puts it) only when it falls under `deny_access`, global or the space's own, because that is the only rule `run` applies to attach paths.

## Sessions

`session list|resume|prune` work on Claude sessions only. A harness whose adapter is not `claude` is refused (64). The space is the argument, else the space holding the current folder, else a picker on a terminal (64 without one). The harness is `--harness`, else the default harness.

Transcripts live in `<config dir>/projects/<folder>/<id>.jsonl`. The config dir is the harness's `CLAUDE_CONFIG_DIR`, else the one in the environment, else `~/.claude`. Claude Code's folder-name encoding is not reliable (a dot is sometimes kept and sometimes turned into a dash), so `workspaces` never works out a folder name. It scans every `projects/*/` and reads each transcript only up to its first record that has a `cwd`; a transcript belongs to a space when that cwd is the space root or below it, so worktrees under the root count. Files are read newest first, so `-n` stops early.

- `session list` prints `MODIFIED  ID  TITLE  TMUX`, newest first, 20 by default (`-n N`, at least 1). The id is the first 8 characters. The title is the last `custom-title`, else the first `ai-title`, else the last `last-prompt` cut to 60 characters, then cleaned and cut like the views' titles; with none of them it is `(empty)` for a transcript under ten lines and `(untitled)` otherwise. TMUX is `session:window %pane` from the launch log, and `-` when the session is not joined to a launch. `--json` prints one object per line: `id` (full), `modified`, `title`, `cwd` and `tmux` (the launch log's object, or null).
- `session resume [space] [session]` takes an id or a unique start of one, and picks on a terminal when it is missing (64 without one). With one argument, it is the space when it names a space or is `.`, otherwise the session. It then runs exactly `workspaces run <space> <harness> --resume <id>`: the policy check, the launch log and the tmux rules all apply. It starts in the space root; when the session ran elsewhere (a worktree, say), a warning says Claude Code may not find it from there.
- `session prune [space]` opens a multi-select (`fzf --multi`; `gum filter --no-limit` without fzf), lists what it will delete, asks `gum confirm` (`--yes` skips; declined is 130), then deletes each chosen `<id>.jsonl` and a real folder `<id>/` beside it and prints one `deleted <id>` line each. Nothing else under the config dir is touched. The picker needs a terminal, so without one it exits 64, with or without `--yes`. Run it from a terminal you type in: deletions under `~/.claude*` fail inside a session's sandbox.

### Joining sessions to launches

`workspaces` exports `AGENT_LAUNCH_ID` to every harness it starts. `workspaces hook session-start` reads the SessionStart hook's JSON on stdin and, when that variable is set, appends `{"ts","launch_id","session_id","cwd","source"}` to `~/.local/state/workspaces/sessions.jsonl` in the way the launch log is written. `session list` joins that file to the launch log for its TMUX column. The hook prints nothing on stdout (Claude Code would add it to the session) and always exits 0; any error goes to stderr. It does nothing without `AGENT_LAUNCH_ID`, so sessions started with `command claude` are not recorded. `source` is `startup`, `resume`, `clear`, `compact` or `fork`.

`workspaces` does not edit Claude Code's settings. To install the hook, add this to `~/.claude/settings.json` (merge it with the `hooks` already there):

```json
{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"workspaces hook session-start"}]}]}}
```

It relies on a hook inheriting the session's environment. That holds unless `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB` is set to `1`, which strips injected variables and would leave `sessions.jsonl` empty; `doctor` does not test for that.

## Doctor

`workspaces doctor` prints one row per check, `OK`, `WARN` or `FAIL`, then the check and a detail, and exits 0 when nothing FAILs and 78 otherwise. It writes nothing and never changes a file.

| Check | FAIL | WARN |
|---|---|---|
| config | `spaces.json` or `harness.json` missing or invalid | |
| paths | | a configured path does not exist |
| space roots | a root fails the launch check (deny_launch, deny_access, not allowed) | |
| attach paths | an attach path is under deny_access | |
| harness binaries | a harness binary is not on `PATH` | |
| config dirs | | a harness's `CLAUDE_CONFIG_DIR` or `CODEX_HOME` does not exist |
| wrappers | | `~/.bash_aliases` has no `<harness>() { workspaces run . <harness> ...; }` for a harness |
| settings | | an entry of the protection list is missing from `~/.claude/settings.json` (read only); required and recommended are named separately |
| session hook | | no SessionStart hook runs `workspaces hook session-start` |
| launch log | `launches.jsonl` (or the first folder that exists above it) is not writable | |
| workspaces binary | | `workspaces` is not on `PATH`, or is not `~/.local/bin/workspaces` |
| tmux | tmux is not on `PATH`, or `$TMUX` is set and its server does not answer | a server answers oddly; no server running is OK |
| todo | | a space has only a `TODO.md` |

A missing protection entry is only a WARN because adding it is your choice. The list: required `Edit(~/.config/workspaces/**)`, `Edit(~/.local/bin/workspaces)` and `Edit(~/.local/bin/awm)`; recommended `Edit(~/.local/state/workspaces/**)`, `Edit(~/.bash_aliases)`, `Edit(~/.bashrc)`, `Edit(~/base.bashrc)`, `Edit(~/.tmux.conf)`, `Edit(~/.tmux.base.conf)`, `Edit(~/.tmux/**)`, and `sandbox.filesystem.denyWrite` for `~/.config/workspaces`, `~/.local/bin/workspaces`, `~/.local/bin/awm` and `~/.local/state/workspaces`; optional `Edit(~/.claude/settings.json)`, mentioned when absent but never a WARN.

## Codex

A harness with `"adapter": "codex"` runs the `codex` binary (or `bin`). Profiles are `CODEX_HOME` in the harness `env`:

```json
{"default": "claude",
 "harnesses": {"claude": {"adapter": "claude"},
               "codex":  {"adapter": "codex"},
               "codex-alt": {"adapter": "codex", "env": {"CODEX_HOME": "~/.codex-alt"}}}}
```

- Mode, for the tmux rules: a first argument of `exec`, `e` or `review` is print mode; `login`, `logout`, `mcp`, `plugin`, `completion`, `update`, `doctor`, `sandbox`, `debug`, `apply`, `features`, `help`, `archive`, `delete`, `unarchive`, `queue`, `migrate-rollouts` and the server commands, or `-V`, `--version`, `-h` or `--help` anywhere, are utility; everything else, including a prompt, `resume`, `fork` and any word the list does not know, is a session. Only arguments before a `--` are read.
- Attach paths become one `--add-dir <path>` each, before the user's arguments. `codex --help` shows `--add-dir <DIR>` taking one value, so a prompt after it is not swallowed. Utility launches get none. `codex review` has no `--add-dir`; the flag is passed before the subcommand and has not been tested there.
- `deny_access` cannot be enforced: Codex has no read deny. The launch goes ahead, prints `workspaces: warning: the codex adapter cannot enforce deny_access: N paths are not denied inside this session` on stderr, and writes `"unenforced_deny": N` in its launch log row (the field is left out when 0, so older rows and readers are unaffected). `workspaces check` shows the same warning and `deny_access  0 paths in session (N not enforced)`. A space that holds secrets should not be opened with Codex until this changes. Utility commands start no agent loop and get no warning.
- No flag is injected that a user argument could replace, so there is no `settings_collision` equivalent.

## Config commands

`space add|rm|attach|detach`, `harness add|rm` and `policy add|rm` edit the config files; `harness list` and `policy list` only read. Flags may come before or after the positionals.

- `space add <name> [root]`: the root defaults to the physical current folder and must pass the launch check `run` applies to a named space (refusal printed with its reason, exit 77). The name must be valid and unused (64), and the root an existing folder (64). With no `spaces.json` there is no `allow_launch`, so run `workspaces policy add allow_launch <path>` first, which creates the file.
- `space attach [space] [path]`: the path defaults to the physical current folder; one under `deny_access`, global or the space's, is refused (77), and one already attached is refused (64). With a single argument, one starting with `.`, `~` or `/`, or containing `/`, is the path; any other is the space.
- `space detach [space] [path]`: without a path, a picker over the space's attach list on a terminal, exit 64 without one.
- `harness add` takes `--adapter` (required), `--bin` and any number of `--env K=V`. It creates `harness.json` when missing, starting from the built-in `claude`. `harness rm` refuses the default harness (64). `harness list` shows the built-in default when there is no file.
- `policy add <list> <path>` and `policy rm <list> [path]` take `allow_launch`, `deny_launch` or `deny_access`, with the same entry rules as the loader: a glob in `deny_access` is refused with exit 78, the loader's error class. `policy add` creates `spaces.json` when missing. Without a path, `policy rm` picks from the list on a terminal.
- A space, harness or list omitted from `rm`, `attach` and `detach` is picked on a terminal and is exit 64 without one; `.` means the space holding the current folder. A bare path on the command line is written with `~` when it sits under the home folder.
- `rm`, `detach` and `policy rm` ask `gum confirm` on a terminal (declined or cancelled: exit 130, nothing written). `--yes` skips the question. Without a terminal and without `--yes` they exit 64.

Each write reads the file, changes only the entry named in the raw JSON, and keeps the rest as written: paths stay in the form written (`~/...` is never expanded), keys keep their order, and new keys go in the usual order. The file comes out indented by two spaces with a trailing newline. The result is validated with the same loader that reads the file at launch, and a result that fails is never written (exit 78, file untouched); so is a file that is already invalid. The old bytes are copied to `<file>.prev`, then the new ones go to a temp file in the same folder, are synced and renamed over the original, and the folder is synced. A missing config folder is created with mode 0700. A successful write prints one line, such as `added space a (~/Source/Projects/a)`, and is not recorded in the launch log.

These commands can change what `workspaces` allows, so a session that can edit `~/.config/workspaces` can change policy. If the user adds the protection entries for `~/.config/workspaces` (an `Edit` deny and a sandbox `denyWrite`) to Claude Code's settings, a session can no longer change policy, and changes then come from a terminal the user types in. Until those entries are in place, nothing stops a session from editing the files.

## Install

```
go vet ./... && go test ./...
go build -o ~/.local/bin/workspaces ./cmd/workspaces
ln -sfn workspaces ~/.local/bin/awm
```

[install.json](install.json) holds the same build line, which `scripts/deploy_agent.py` prints for you to run.

On a new machine, copy `spaces.example.json` to `~/.config/workspaces/spaces.json` and edit it, or build the config with `workspaces policy add` and `workspaces space add`.

## Config

Both files live in `~/.config/workspaces/` (or `$XDG_CONFIG_HOME/workspaces/`) and are read fresh on every call. Unknown keys are errors.

`spaces.json` is required: with no policy there is no launch. Paths start with `~`, `~/` or `/`. `allow_launch` and `deny_launch` may use `*`, `?` and `[...]`, where `*` stays inside one path segment and an entry ending in `/*` covers everything below the parent but not the parent itself.

```json
{"allow_launch": ["~/Source"],
 "deny_launch":  ["~/Source/private/*"],
 "deny_access":  ["~/.ssh"],
 "spaces": {"example": {"root": "~/Source/Projects/example-app",
                        "attach": ["~/Source/Projects/example-lib"],
                        "deny_access": []}}}
```

Every deny test runs before any allow test: `deny_access` (current folder, logical or physical path), then `deny_launch`, then the space's attach paths against `deny_access`, then `allow_launch` (physical path only). Nothing matching means refused. A named space starts in its root; `.` uses the space whose root holds the current folder, if any. Attach paths and deny_access reach Claude Code through `--settings` (see Claude settings).

`harness.json` is optional. Without it the only harness is `claude`.

```json
{"default": "claude",
 "harnesses": {"claude":     {"adapter": "claude"},
               "claude-alt": {"adapter": "claude", "env": {"CLAUDE_CONFIG_DIR": "~/.claude-alt"}}}}
```

`adapter` must be a compiled-in adapter (`claude` or `codex`). `bin` overrides the binary: an absolute path, a `~/` path, or a bare name looked up on `PATH`; a relative path is a config error. `env` values get tilde expansion only. Space and harness names match `[a-z0-9][a-z0-9_-]*`, at most 32 characters.

## Claude settings

`workspaces` passes one inline `--settings` JSON, placed before the user's arguments. It carries the space's attach paths and the effective `deny_access` list: the global entries, then the space's own, each path once, in config order. `.` outside any space gets the global entries only.

For each deny path P, `permissions.deny` gets `Read(//P)`, `Read(//P/**)`, `Edit(//P)` and `Edit(//P/**)`, and `sandbox.filesystem.denyRead` and `denyWrite` each get P. If P is a symlink (or sits under one), the same entries follow for its resolved path; a path that does not exist is passed as written. A launch with attach `~/Source/Projects/b` and deny paths `/home/u/.ssh` and `/home/u/.aws` emits:

```json
{"permissions":{"additionalDirectories":["/home/u/Source/Projects/b"],"deny":["Read(//home/u/.ssh)","Read(//home/u/.ssh/**)","Edit(//home/u/.ssh)","Edit(//home/u/.ssh/**)","Read(//home/u/.aws)","Read(//home/u/.aws/**)","Edit(//home/u/.aws)","Edit(//home/u/.aws/**)"]},"sandbox":{"enabled":true,"filesystem":{"denyRead":["/home/u/.ssh","/home/u/.aws"],"denyWrite":["/home/u/.ssh","/home/u/.aws"]}}}
```

- `sandbox.enabled` is set to `true` whenever there are deny paths. `--settings` outranks a project's `.claude` settings, so a repo cannot turn the sandbox off in a session `workspaces` started; only managed settings can. Arrays such as `denyRead` merge with the user's own settings rather than replacing them.
- Because the global `deny_access` is normally not empty, nearly every launch passes `--settings`. A launch whose own arguments also contain `--settings` (before any `--`) is refused with `settings_collision` (exit 64), logged, and nothing starts. Put such settings in a settings file instead.
- `workspaces check` prints `deny_access  N paths in session` (`deny_access_paths` in `--json`); the full JSON is in its `argv` row.
- tmux-claude-resurrect saves and replays `--settings`, so a restored pane keeps the rules as they stood at the last save. It does not pick up later changes to `deny_access`, and it skips `workspaces`'s check and log.

Manual check, in a launched session:

- Ask it to read `~/.ssh/config` with the Read tool: refused.
- Ask it to run `cat ~/.ssh/config`: refused.
- `/permissions` lists the injected rules.
- `/sandbox` shows the sandbox enabled, with the user-level keys (such as `allowUnsandboxedCommands: false`) still present.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success; for `check`, the launch would be allowed |
| 64 | usage: missing argument without a TTY, unknown space or harness, `--settings` collision; for `session`, a harness that is not Claude, an unknown or ambiguous session, no TTY to pick on; for the config commands also a duplicate or invalid name, a duplicate entry, the default harness, and a removal with no TTY and no `--yes` |
| 69 | tmux missing, not answering, failing, or the bootstrap command too long |
| 70 | internal error |
| 74 | the launch log could not be written, nothing launched; a config file could not be written; or a session folder could not be read or a transcript deleted |
| 77 | refused: by policy, `not_in_tmux` or `background_refused`; for `space add` and `space attach`, a root or path the launch check refuses |
| 78 | config missing or invalid; for the config commands also a change whose result would be invalid; for `doctor`, at least one FAIL |
| 127 | harness binary not found |
| 130 | picker cancelled, or a removal or prune not confirmed |

After a launch, the shell sees the harness's own exit status, or the tmux client's after a bootstrap.
