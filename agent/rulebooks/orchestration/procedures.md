# Proven procedures

Role and skill routing loads this file when a multi-part design or architecture effort begins. Add a procedure only after it was performed and worked; do not add aspirations.

## Roadmap and todo before design

This procedure applies when design spans more than one subsystem, sitting, or session. It does not apply to a single bounded change.

Set up two records before design discussion starts: the effort's open items as entries in the project's `TODO.jsonl` (the project's task ledger, see [work organization](work-organization.md)), and `ROADMAP.md`, a design document inside the repository.

The split is strict: the `TODO.jsonl` entries say what remains undecided; `ROADMAP.md` says in what order to resolve it. Priority, tiers, and sequencing do not belong in the entries. Each entry records `Blocks` and `Blocked by` as facts; the roadmap consumes those facts to derive order.

### ROADMAP.md is navigation and position

| Element | Purpose |
|---|---|
| verdict block, four to six bullets | enough to decide without reading further |
| progress table | one row per bucket with pages, requirement count, open-decision count, and status |
| governing rules | standing rules made visible for this effort |
| one section per bucket | clickable links to the real documents and one-line coverage descriptions |
| already-decided table | locked decisions that must not reopen silently |
| where things live | branch, directory, and stale-state record |

The roadmap links to the actual design documents. It does not summarize them, because a duplicate summary will drift.

### The TODO.jsonl entries are the decision ledger

Record every open fork as one entry, filed under its bucket, with a stable descriptive ID and one status: `open`, `decided`, `deferred`, or `owner`. IDs make dependencies referenceable, but never use a bare ID as the subject of prose. Decisions are not tasks. An unresolved fork blocks its dependent requirement. Use `owner` for product decisions engineering must not guess.

### Sequence

1. Inventory existing pages, requirements, and open questions.
2. Add the `TODO.jsonl` entries: extract every open fork, assign stable IDs, and file each under its bucket.
3. Write `ROADMAP.md`: add bucket counts from the entries and link every real document.
4. Ask the owner for cross-bucket order. This is a value decision about which surface reaches a user soonest.
5. Derive within-bucket order from the entries' `Blocks` and `Blocked by` facts. This is a dependency decision. Record the result in the roadmap with one Mermaid diagram per bucket.
6. Only then begin resolving decisions.

Do not skip within-bucket ordering after receiving cross-bucket priority. During dependency analysis, identify homeless decisions that every document disclaims and promote them to cross-cutting rows. Also identify inversions where a high-value bucket depends on a lower-value bucket.

The proof that this procedure works is enumeration: counts reveal whether an architecture that appears written is still mostly undecided.

