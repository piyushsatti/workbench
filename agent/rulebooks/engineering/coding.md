# Coding and repositories

- Diagnose before editing. Reproduce or inspect evidence at the narrowest responsible layer.
- Compose shared behavior instead of copying it. Use explicit dependencies and clear module boundaries.
- Duplication, vendoring, and private copies require written justification.
- Stop when the structure is wrong; obtain the human's decision on the new shape before restructuring.
- Keep tests in top-level `tests/`, mirroring source paths, unless the project has an explicit different convention.
- Run `ruff format` and `ruff check` before committing Python changes.
- In uv projects, pin the interpreter in `.python-version`.

## Worktrees

- Perform all non-trivial repository work in a worktree. Keep the primary checkout clean on its default branch.
- Create worktrees with `git worktree`. Pass a base branch only when the conversation already identifies one.
- Keep a project's repository, worktrees, artifacts, history, research, notes, and data under one project-owned boundary.
- Reach shared artifacts through documented relative paths, not symlink wiring.
- Track any instruction needed in every worktree inside the repository, or route it through the canonical role standards.

## Semantic navigation

Use an available language server for symbol questions: definitions, references, calls, implementations, types, and file symbols. Use textual search for strings, comments, config, logs, TODOs, and unsupported languages.

Validate language-server scope before trusting counts. For Python, confirm repository root, workspace diagnostics, and the project environment. Cross-check load-bearing counts with text search. If a language server is unavailable or returns implausible results, say so.

## Coding preferences

Load when writing code, laying out tests, or committing.

### Language and tooling

- **The project standard is the baseline.** The repository's own `pyproject.toml`, the project's standard, and the existing format of the code outrank a generic skill's recommendations. Do not add lint rule sets or tooling a skill suggests unless the standard asks for them.

### Commits

- Conventional commits: `feat:` / `fix:` / `chore:` / `docs:` / `refactor:` / `test:`.
- Committing is situational, not a standing rule. Default to committing only when asked, unless an agreed workflow says otherwise.

### Python module layout

Module-level constants and globals (for example `BATCH_SIZE = 1000`) go at the top of the file, directly after the imports, before the first function, never between functions.

### Test file layout

Use the layout the language's own documentation prescribes.

- Go: `foo_test.go` beside `foo.go`, in the same directory (`go help test`).
- Python: tests live in the top-level `tests/` directory, mirroring the source path of what they test: source `path/to/foo` becomes `tests/path/to/test_foo`, or `tests/unit/path/to/test_foo` and `tests/integration/path/to/test_foo` where the project splits its suite by whether a test needs an external service. Never colocate tests beside the source under a nested `tests/` subdir.

Framework and exact naming are project-specific and live in that project's CLAUDE.md.

### Comments

See `code-comments.md` in this directory. It is the authoritative statement, including the no-ticket-numbers rule.

### Pasteable command blocks

Pure commands only. No inline `#` comments, no backticks inside the block. The TUI line-wraps multi-line pastes, and a wrapped inline comment or a backticked word lands on its own line and executes as a stray command. Put all explanation in prose before or after the block. If a step needs a note, use a separate prose line, not an inline comment.

#### Blocks handed to a human to run

- **Real values, named machine, tested from where the human runs it.** Fill in real paths, hosts and values, never placeholders such as `<data dir>`. Say which machine the block runs on. Check it works from the directory and environment the human will use (worktree or project root, venv, module path, env file) before handing it over.
- **Each `!` or Bash command is a fresh shell.** An `export` in one is gone in the next. Put the variables in front of the same command or pass `--env-file`, and from another directory use `uv run --directory <worktree>`.
- **One machine, one block, one message.** Say what the block changes, hand nothing else with it, and wait for its output before the next. Prose between blocks can be copied along with them, and an apostrophe in it can open a quote that breaks the heredoc; a guard protects only a block pasted whole.
- **Host guard.** A script that targets a specific machine first checks it is on that machine and aborts the whole block on a mismatch. Wrap the block as `bash -euo pipefail <<'EOF' ... EOF` with the guard first, so the abort stops everything after it without killing the login shell. Say what the guard prints on success.
- **A block that asks for a sudo or ssh password runs in a real terminal**, not through `!`.
- **`</dev/null` on stdin readers.** Inside the heredoc the script is bash's stdin, so `docker compose exec -T`, `docker exec -i`, `ssh` and `read` swallow the rest of the script and the block ends early with no error. Give each `</dev/null` (`</dev/tty` for a prompt).
- **Nothing after a prompting `sudo`.** A pasted block feeds the lines after `sudo` into its password prompt as bogus attempts, which looks exactly like a wrong password. Run `sudo -v` alone as its own block to cache the credential, or give every `sudo` command its own block.
- **Leading `-` in a name needs `./`.** A folder name that starts with `-` (the encoded `~/.claude/projects` keys do) is parsed as an option or predicate by `find` and `ls`; write `./-home-user-...`.

### Versions

Version the tools, servers and mods this repo owns as CalVer `YYYY.0M.0D` (zero-padded, for example `2026.10.05`). Builds on the same day share the version, with no `.2` or `-2` suffixes. Past semver numbers stay as history. Use SemVer only when told to, and for third-party packages that require it.

### Agents, skills and workflow prompts carry behaviour, never facts

They are tools we deploy. A tool checks information at run time from the source the caller names; it does not recall it, because a recalled fact that has drifted corrupts every decision downstream. Bodies hold Purpose, Blocking requirements, Raise to the calling session, Method, and Output contract, and are project-neutral. Rules, style and project facts live once, in rulebooks or decision docs, reached by a pointer under progressive disclosure. The same intent applies to CLAUDE.md files: pointers and layout only, nothing a rulebook, ROADMAP or notes already hold.

- **Rulebook paths are input, not hardcoded.** An agent or workflow takes the rulebook paths from its caller. A workflow that builds paths from a hardcoded rulebook root breaks when the rulebooks move.
- **Cite rulebooks relative to the checkout in play.** A container agent file names `docs/standards/` of the checkout it is working in, never the primary clone's `<project>/docs/standards/`, or a session in a worktree silently reads the primary clone's copy from another branch.
