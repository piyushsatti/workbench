# HATS.md — the hat-file authoring contract

A hat file defines one professional review lens. It is consumed by `consult`, `hats`, and `red-vs-blue`, which compose its **body** with their own stance rules. This file is the contract every hat must satisfy.

A hat is a **checklist-and-contract document with a thin domain header — not a character.** "Be thorough", "think like an expert", "world-class architect" are worthless: identity personas alone buy no accuracy and often *hurt*, and identical-in-effect lenses add literally zero over a single reviewer ([D12], [D3] in `references/wardrobe/adversarial-review-methodology.md`). A hat earns its place by the failure classes it is responsible for and the binary-checkable questions it always asks — not by who it claims to be.

## Terminology (the word "hat" is overloaded — pin these)

Three distinct things get called "hat" in casual speech. This repo disambiguates:

| Term | Means | Not |
|------|-------|-----|
| **lens** (a **hat file**, `references/wardrobe/hats/<name>.md`) | one review perspective — the checklist-and-contract document this file governs | the skill, the collection |
| **wardrobe** | the shared *collection* of lens files, drawn from by every consumer | any single lens |
| **`hats`** (code font — the **skill**) | the consumer that wears **3–4 lenses at once** as a blind parallel panel | a single lens; the wardrobe |

Rule of thumb: a *lens* is worn; the **`hats`** skill wears several; they all live in the **wardrobe**. `consult` wears **one** lens, `red-vs-blue` wears **one** shared lens across its pair — only **`hats`** wears many. When a sentence could mean more than one, say "lens" for the file and reserve bare **`hats`** (code font) for the skill.

## Frontmatter (registry metadata — NOT sent to the model)

```yaml
name: <hat-name>
status: v1
overlaps:
  - <other-hat>: "one-line boundary — where this hat ends and that one begins"
  - <other-hat>: "..."
```

- **`name`** — the roster key.
- **`status`** — `v1` after this pass; bump when a rubric or failure class is revised.
- **`overlaps`** — each near-neighbour hat plus the one-line boundary between them. This field is **registry metadata for the panel composer and the overlap eval only. Consumers MUST exclude the frontmatter from the dispatch prompt** — every non-task word in the payload is measured downside, and overlaps naming *other* hats is exactly that irrelevant detail [D16, D13]. The panel composer reads it to decide which hat pairs to run the distinctness test on; the reviewing model never sees it.

## Required body sections (in this order)

1. **`## Role`** — ONE sentence, task-coupled, demographically unmarked. Form: *"You are a [domain] reviewer. You review [artifact type] for [failure family] and always [core procedure]."* The domain header is the cheapest, weakest ingredient — worth one sentence, never more [D12].
2. **`## Failure classes`** — an enumerated list of defect classes this hat is **responsible** for catching, each with a one-line operational definition. These are responsibilities, **not** "uniquely caught": uniqueness is an empirical claim measured by the overlap eval, never asserted at authoring time [D16].
3. **`## Always ask`** — 3–6 questions, **each answerable yes/no/n-a against the artifact**. No open-ended "consider the architecture" prompts — binary-checkable items are what lift evaluator agreement; open ones forfeit the gain [D15].
4. **`## Evidence demands`** — what a finding from this lens must cite: a **location (file/section/step) plus a verbatim quote** from the artifact, before the finding is valid. A finding with no quoted evidence is **capped at the lowest severity band (nit)** [D15].
5. **`## Blind spots`** — the failure families this hat does **not** cover, each with which hat owns them. Keep terse — this section is also payload exposure [D13].
6. **`## Severity anchors`** — 1–2 hat-specific exemplar findings per severity level, so the shared taxonomy below has concrete anchors for this lens (e.g. "unauthenticated SQL injection on a reachable path = blocker") [D15].

Target ≤60 lines. Concrete, not aspirational.

## Shared severity taxonomy (lives here; every hat and consumer uses it)

Four levels, each a decision rule with binary-checkable conditions [D15]:

| Severity | Decision rule (binary-checkable) |
|----------|----------------------------------|
| **blocker** | Ships incorrect behavior, data loss, or a security breach on a reachable path — and the evidence is quoted. |
| **major** | Violates a stated requirement or load-bearing decision, but a workaround or non-default path exists. |
| **minor** | A real defect with bounded impact that does not block the artifact's stated goal. |
| **nit** | Correctness-neutral improvement (clarity, consistency, style). |

**Evidence gate [D15]:** severity is assigned **only after** the evidence field is filled with a verbatim quote + location. A finding lacking quoted evidence is mechanically **capped at `nit`** regardless of how bad it sounds. Per-hat `Severity anchors` specialize this table with concrete examples; they never redefine the levels.

## Anti-flavor rule [D13]

Ban from every hat file: names, backstories, demographics (gender, age, race, seniority-years), and "world-class / distinguished / 10x" intensifiers. This is not style preference — irrelevant persona detail is **measured harm up to ~30pp**, and demographic markers bias reasoning in the wrong direction. Every non-task-relevant word is downside exposure, not neutral filler. The only identity a hat gets is its one-sentence `Role`.

## Minting rule [D16]

> A new hat must declare at least one failure class **no existing hat lists.** If it cannot, it is a duplicate — **extend an existing hat instead.**

The declaration is the authoring bar; the *guarantee* of distinctness is not asserted — it is **verified by the overlap eval** (run two-plus hats on one artifact, compare finding-overlap against their declarations, and require each hat to beat a no-persona control on its own classes). Before adding a hat, read the roster in `references/wardrobe/README.md`, open the two or three nearest hats, and confirm the new lens owns a failure class none of them do.

**Running the evals.** The overlap eval is a procedure you run by hand, not a tool: crucible ships no fixture, script or stored result for it. Dispatch the hats and a no-persona control as panelists on one shared multi-lens fixture of your own, then count, per hat, in-class vs out-of-class findings, pairwise overlap, and whether it beat the control on its own classes. Recall and model×effort were measured the same way with an unpublished seeded-defect benchmark. After editing a hat, run it again and compare against your last recorded result, if you keep one — a gain on the edited lens does not clear the change; lane discipline and beats-control must still hold [D16].

## Versioning note [D15]

Rubrics and failure-class definitions are **expected to be revised after real review batches** — "grading outputs helps users define criteria" (Shankar et al., UIST 2024). A locked-forever rubric contradicts the best evidence. Bump `status` and re-run the hat evals when definitions change; do not treat v1 as final.

## Composition note (for consumers, not authors)

The hat supplies *what to look at* (its body). The consumer supplies *how to engage*:
- **consult** → partner stance (brainstorm/validate, conversational).
- **hats** → neutral stance (raise-don't-fix, blind, reasoning-first findings contract).
- **red-vs-blue** → attack stance (red) and verify stance (blue), same hat both.

Never bake a stance into a hat file — that would break sharing. And never feed the frontmatter to the model — body only [D16, D13].
