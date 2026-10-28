# Wardrobe — the shared hat library

The wardrobe is a **reference library, not a workflow.** It holds one file per professional lens (`hats/<name>.md`). Those files are the single source of truth for what each lens cares about; the review modes *consume* them.

- **consult** loads exactly one hat → partner-stance dialogue through that lens.
- **hats** loads 3–4 distinct hats → parallel blind panel, each hat + neutral stance.
- **red-vs-blue** optionally loads one hat → both red and blue wear the same lens.

Defining a persona once here and composing it everywhere is the whole point: the architect lens is never re-described in three modes that can drift apart. A hat file carries *what to look at*; each consumer supplies *how to engage* (its own stance protocol). Red-vs-blue "wearing a hat" is the same mechanism — the stance role reads the lens file into its prompt; no nested subagents.

## How a consumer loads a hat

1. Resolve the roster (below) or ask the user which lens fits.
2. Read `references/wardrobe/hats/<name>.md` (paths are relative to the crucible skill folder).
3. Compose: the hat file's **body** (the `## Role` … `## Severity anchors` sections) + the consumer's own stance rules → the dispatch prompt (or the in-thread persona for consult). **Exclude the YAML frontmatter** — `overlaps` is registry metadata for the panel composer and the overlap eval; naming other hats in the payload is irrelevant detail and measured downside, so it never reaches the reviewing model [D16, D13]. The shared severity taxonomy lives in `references/wardrobe/hats/HATS.md`, not in each hat — a consumer that needs the level definitions reads them from there.
4. If no roster hat fits, draft one inline for the run **and flag it for wardrobe addition** — do not silently invent a throwaway lens.

## Roster (v1)

| Hat | Catches (one line) |
|-----|--------------------|
| `architect` | boundary/coupling mistakes, single points of failure, painted-into-corner evolution |
| `senior-engineer` | hidden implementation complexity, maintainability debt, won't-survive-the-codebase |
| `security` | trust-boundary violations, authn/z gaps, data exposure, supply chain |
| `sre` | operability: observability, rollback, failure recovery, on-call reality |
| `frontend` | interface/UX flaws, state-management traps, accessibility, perceived performance |
| `product` | user-value gaps, wrong sequencing, does-anyone-need-this |
| `coverage` | missing requirements, unstated assumptions, checklist integrity (D9 counterweight) |
| `simplifier` | YAGNI, deletable scope, cheaper paths to the same outcome |
| `finance` | cost/ROI, runway impact, build-vs-buy |
| `coach` | personal/life decisions: sustainability, values alignment, motivation reality |

**Agentic-infrastructure lenses** (for reviewing skills/agents/prompts/harness):

| Hat | Catches (one line) |
|-----|--------------------|
| `prompt-engineer` | weak triggers, workflow-in-description, missing rationalization counters, ambiguous instructions, context bloat |
| `harness-engineer` | dispatch topology, hook-event mismatch, MCP/sandbox wiring, model/effort misroute, cross-host fragility, failure handling |
| `skill-designer` | discoverability (SDO), wrong primitive (skill vs hook vs agent), progressive disclosure, handoff/routing, registry overlap |
| `eval-engineer` | no ground truth, non-discriminating/saturated metrics, judge bias, untested claims, overfitting to one fixture |

## Authoring or extending a hat

The authoring contract (frontmatter + the six body sections), the shared severity taxonomy, the anti-flavor rule, the minting rule (a new hat must **declare** a failure class no existing hat lists — uniqueness is then verified by the overlap eval, not assumed — else extend an existing hat), and the versioning note all live in `references/wardrobe/hats/HATS.md`. Read it before adding or editing any hat.

The D-numbers cited throughout are in `references/wardrobe/adversarial-review-methodology.md`.

## Not a workflow
Nothing here is invoked directly. If you want to *use* a lens:
- one perspective, conversational → **consult**
- 3–4 perspectives, structured panel → **hats**
- adversarial attack/verify → **red-vs-blue**

## Status
Hat files are **v1** (`status: v1` in each file's frontmatter), rebuilt to the persona/contract research findings (D12–D17). Rubrics and failure classes are expected to be revised after real review batches [D15] — bump `status` and run the overlap eval by hand (`hats/HATS.md`, Running the evals) when a definition changes.
