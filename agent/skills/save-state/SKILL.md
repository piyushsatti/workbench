---
name: save-state
description: "Capture the current session's verified learnings, research, decisions, worktree state, and resumption path in a durable state node. Use when a feature or research pass is complete, paused, handed off, or a worktree is about to be abandoned; do not use for automatic context compaction."
---

# Save State

When explicitly invoked, write one truthful, resumable state node under the
Save State's data root, `~/.local/state/save-state/`. Read enough context to preserve the work,
write one new node, verify it, report its path, and stop.

Read [examples.md](references/examples.md) before writing. It contains the
node shape and concrete good/bad examples; imitate the good example's level of
specificity.

## Contract

| Concern | Rule |
|---|---|
| Storage | `~/.local/state/save-state/` (create it if missing). Never project `notes/`, `dist/`, or the skill folder. |
| Filename | `YYYY-MM-DDTHH-mm-ssZ--<project-slug>--<topic-slug>.md`; use the current clock and never overwrite an older node. |
| Status | Honor an explicit `complete`, `abandoned`, or `active`; otherwise use `paused`. Never infer completion or abandonment. |
| Evidence | Label claims `verified`, `observed`, `inferred`, `proposed`, or `unknown`. Keep fixture, screenshot, and plan evidence scoped. |
| Transcript | Say `archived` only after writing and verifying an exact raw export; otherwise use `pointer-only` or `unavailable`. |
| Worktree | Read-only inspection. Never commit, stage, reset, checkout, delete, archive, or rebind. |
| Side effects | Write the new state node only. Do not mutate memory stores, external notes apps, project notes, configuration, or remote systems. |

## Recipe

1. Snapshot the work. Read applicable instructions and relevant artifacts. When
   applicable, collect `git rev-parse --show-toplevel`, branch, `git rev-parse
   HEAD`, `git status --short`, `git diff --stat`, and the matching
   `git worktree list --porcelain` entry.
2. Extract what a future session needs: objective, actual outcome, durable
   learnings, changed artifacts, checks and receipts, limitations, open
   decisions, and the shortest safe resume path.
3. Classify every meaningful claim. Preserve failed attempts when they change
   the next move. Do not re-run broad research just to fill a section.
4. Create one Markdown node using the format and examples in `references/`.
   Include project/worktree identity as metadata even though storage is global
   to the skill.
5. Read the node back. Confirm its path, timestamp, status, state root, and
   transcript status. Re-check the worktree to confirm Save State changed only
   the state root. Report the exact node path and stop.

## Fast heuristics

- No direct evidence? Write `unknown`, not a plausible reconstruction.
- A test, fixture, screenshot, or synthetic provider? Record its exact scope;
  do not promote it to runtime, production, or adoption proof.
- A suggestion without a concrete approval? Put it under “Proposed,” not
  “Outcome” or “Next action.”
- A dirty worktree? List exact paths and leave them untouched.
- No safe transcript path? Use `unavailable`; a session ID or source path is
  `pointer-only`, not `archived`.
- An existing node for this session? Write a new snapshot and set `prior_node`.
- “Abandon” or “close”? Record that disposition; do not delete anything.
- No Git root? Save the node anyway and mark Git fields `unknown`.

Do not copy large source files into the node. Link exact paths, receipts, and
source URLs instead. Do not continue implementation after saving unless asked.
