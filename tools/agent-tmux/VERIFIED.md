# agent-tmux: verification

Levels are defined in [verification ladder](../../CONVENTIONS.md#verification-ladder). Claimed harness: Claude Code only.

| Level | Harness | Date | Machine | Version | Command | Result |
|---|---|---|---|---|---|---|
| V0 lint | Claude Code | 2026-10-22 | macOS (arm64) | lint_plugins.py as of 2026-10-22 | `python3 -B scripts/lint_plugins.py` | pass: 5 plugins, 0 errors, 0 warnings, catalogs included. Historical: run when agent-tmux was under `plugins/`; it is not in `plugins.json` now, so `lint_plugins.py` no longer covers it. |
| V1 native validator | Claude Code | 2026-10-22 | macOS (arm64) | Claude Code 2.1.307 | `claude plugin validate tools/agent-tmux` | pass |
| V2 script tests | Claude Code | 2026-10-22 | macOS (darwin/arm64) | go1.27.3 | `cd server && GOCACHE=$TMPDIR/gocache go test -count=1 -v ./...` | 116 top-level tests pass, 1 fails: `TestProcTreeRealProc` (`reading /proc: open /proc: no such file or directory`), which is expected on macOS. With `-skip TestProcTreeRealProc`: `ok agent-tmux-mcp`. `go vet ./...` is clean. The build command in `install.json` was also run with `-o $TMPDIR/...` and produced a binary, which was then deleted. Nothing was installed. Not run on Linux, where the real-`/proc` test would apply. |
| V2 script tests | Claude Code | 2026-10-22 (rerun) | macOS (arm64) | go1.27.3 | `cd server && GOCACHE=$TMPDIR/gocache go test -count=1 ./...` | `ok`: `TestProcTreeRealProc` now skips where /proc doesn't exist |
| V3 loads without install | Claude Code | — | — | — | `claude -p --plugin-dir tools/agent-tmux` | not run |
| V4 runtime behaviour | Claude Code | — | — | — | live checks in `docs/server-README.md` | not run |
| V5 in use | Claude Code | — | — | — | — | not run |
