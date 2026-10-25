# Save State examples

Use these examples as a pattern. The node should be compact, concrete, and
honest about what the evidence does and does not prove.

## Node shape

```markdown
---
schema_version: 1
kind: session-save-state
captured_at: "2026-09-21T22-18-45Z"
title: "Add Save State skill"
status: paused
state_root: "/home/user/.local/state/save-state"
project: "my-plugin"
worktree: "/home/user/src/my-plugin"
branch: "main"
head: "abc1234"
source_session: "unknown"
transcript_status: unavailable
prior_node: "none"
---

# Objective

Create a manual skill that preserves useful session state for later resumption.

# Outcome

Implemented the skill and packaging changes. No installation or global config
change was performed. The node is paused until the user chooses the install
step.

# Durable learnings

## Verified

- `npm test` passed 31 tests — evidence: test output from this session.
- The generated plugin package contains `skills/save-state/SKILL.md` — evidence:
  `dist/save-state/skills/save-state/SKILL.md`.

## Observed

- The current compiler exposes Codex skill directories as `./skills` for this
  one-level component layout.

## Inferred

- The plugin-owned state root is the right durable boundary for checkpoints.

## Proposed

- Install the built plugin in a later, explicitly requested step.

# Changes and evidence

- Added `src/plugins/save-state/skills/save-state/SKILL.md`.
- Validation and smoke checks passed; no live host installation was tested.

# Current worktree state

- `main`, HEAD `abc1234`; pre-existing dirty paths remain untouched.

# Open questions and next gate

- Decide whether to install the generated package.

# Resume instructions

Read this node, then inspect the generated package before any install action.

# Transcript and provenance

`unavailable`: no safe raw transcript export was exposed.
```

## Good versus bad

Good:

```markdown
status: paused
transcript_status: pointer-only

## Verified

- `restore()` accepts the recorded backup ID — evidence: `tests/restore.test.ts`.

## Proposed

- Test restore against the off-host destination next; not run in this session.
```

Bad:

```markdown
status: complete
transcript_status: archived

- Everything works and the transcript is saved.
- Next, deploy the fix.
```

Why it is bad: it claims completion without acceptance evidence, claims an
archive without an exported file and verification, and turns a possible next
step into an authorized action.

## Final report shape

```text
Saved: /home/user/.local/state/save-state/2026-09-21T22-18-45Z--my-plugin--save-state.md
Status: paused
Transcript: unavailable
Next gate: read the saved node before choosing whether to install or resume.
```
