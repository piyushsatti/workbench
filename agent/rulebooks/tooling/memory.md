# Memory

Route by behavior:

| Material | Home |
|---|---|
| proactive rule | canonical role standard |
| project-specific rule or fact | project-owned documentation |
| follow-up task | owning task ledger: `<project-root>/TODO.jsonl` |
| recalled fact, reference, or past context | memory |
| symbol, architecture, or code decision coupled to code | semantic code tools and repository docs |

Retain useful, recurring facts with their scope and provenance. An ordinary memory save does not need a separate proposal or approval step under these standards. Never place an always-on rule or follow-up task in memory. Tasks belong in the owning TODO.jsonl, as defined by [governance](../governance/authority-and-changes.md#task-ownership-and-memory).

## Canonical records and native projections

Each harness keeps memory in its own native store (for Claude Code, the per-project memory under `~/.claude/projects/`). The Dream skill (`agent/skills/dream`) consolidates it on request. Authored knowledge bases, standards and project documentation remain external.

A memory record carries `created`, `last_updated`, `type`, `scope`, `genericity`, a stable `id`, `provenance`, and lifecycle `status` (`active`, `deprecated`, or `tombstone`). Scope contains `key`, `depth`, and `role`: `key` identifies the logical owner independently of its machine path. The home key is `{key: home, depth: 0, role: home}`. Never infer project identity from depth alone.

The format has no version number. A record missing a field gets it the next time it is rewritten; when its owner cannot be resolved, it goes to review instead of a guessed key. Name each file `<created>-<type>_<slug>.md` and update `last_updated` whenever memory changes.

Writes require explicit scope. Replacing a record requires its expected content hash, checked under the write lock; conflicting updates must not overwrite each other. Preserve stable identity and provenance through merges and retirement. Retain preimages and tombstones so retries cannot resurrect retired knowledge.

Treat `genericity` as an export boundary; when unsure, use `specific`. Sharing between two local harnesses does not authorize broader scope or export. Project facts belong in project-owned documentation; reusable behavioral rules are promoted to their canonical role standard first.

Reconciliation uses a concrete reviewed manifest. Write canonical records first, then track each native projection separately. Conflicting edits and disappearance are review candidates, never automatic last-write-wins or deletion. Repeating an operation must be idempotent. Use the selected tool’s supported workflow and access constraints; this standard does not impose a separate blanket memory-save approval gate.

Claude projections may target its native project memory directories. Codex native intake is global: submit only global-safe summaries or scoped pointers through its supported ad-hoc note path, following its explicit-user-request requirement. Never edit Codex-generated memory files or internal databases. A submitted note remains pending until managed consolidation is verified; scoped recall remains available through Memory and project documentation.

Hooks must not apply memory changes. Projection receipts and continuity state are operational data, partitioned by logical project, harness, and worktree/session; they are not authored knowledge.

Documentation rules live in `../documentation/documentation.md`. Repeatable
planning procedures live in `../orchestration/procedures.md` and role-owned
skills.
