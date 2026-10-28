# Sandbox and permission profiles

There are two Claude Code profiles, and each repository picks one. These are templates: merge one into a repository's `.claude/settings.json`. They contain only `sandbox` and `permissions`, so they don't clash with hooks or other keys.

| Profile | Shape |
|---|---|
| `strict.json` | The sandbox is mandatory (`failIfUnavailable`, no unsandboxed commands). Network is limited to the package index. The agent can read and inspect, and a human commits: the sandbox blocks every write to the project's `.git` (`denyWrite: ["./.git"]`), so branch deletes, commits, resets and the like fail at the OS level whatever the command looks like, and the deny list also stops the commands that rewrite working files instead (`checkout`, `restore`, `clean`, `stash`, `revert`, `cherry-pick`, `switch`). `.env` is denied for reading and editing. |
| `permissive.json` | The sandbox is on and sandboxed Bash runs without asking. `git` and `gh` run outside the sandbox so worktrees and credentials work. Read-only git and gh commands are allowed, every write asks, and destructive history operations (force push, branch or tag delete, `reset --hard`, `clean`, forced checkout or switch, writes to `.git/`) are denied. |

Add project tools and MCP read tools to `permissions.allow` per repository.

To use a profile:
- **strict:** add the project's own lint, test and sync commands to `allow`, and its package hosts to `sandbox.network.allowedDomains`.
- **permissive:** add any project MCP read tools to `allow`.

Security note for `permissive`. Because `git` and `gh` run outside the sandbox, any git option that runs a program is a way out of it:
- every `git config` form is denied, reads too, because a local `core.fsmonitor`, `core.pager` or diff driver runs on the next allowed `git status` or `git diff`
- `GIT_*=` and `env` prefixes are denied
- the extra denies block `-c`, `--config-env`, `--upload-pack`, `--receive-pack`, `--ext-diff`, `--textconv`, `--exec`, `grep -O` and global or system `git config` writes
- `sandbox.filesystem.denyWrite` stops sandboxed commands from planting a pager, diff driver or hook in `.git/config`, `.git/hooks` or the user git config

In a project's `.claude/settings.json`, `./.git` in `denyWrite` means that project's `.git` and everything under it; a single leading `/` would mean the filesystem root ([settings reference, sandbox path prefixes](https://code.claude.com/docs/en/settings-reference.md#sandbox-path-prefixes)). In a git worktree `.git` is a file pointing at the main repository's git folder, which this rule does not cover. Some risk remains: a repository's checked-in `.gitattributes` can name a diff driver, but only `.git/config` can define one. Prefer `strict` unless a repository needs git outside the sandbox. A structural fix is open: keep git inside the sandbox (drop `git *` from `excludedCommands`) and allow writes only to the worktree backing-store paths.

Notes:
- Permissions match command prefixes, and prefix rules miss `git -c …` and `cd x && git …`. The deny lists here are a convenience, not a guarantee. Guarantees belong in git hooks and remote branch protection.
- Codex and Cursor have their own sandbox models, so these files are Claude Code only. Codex uses `sandbox_mode` and `approval_policy` in `config.toml`.
