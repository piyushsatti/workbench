# Hats

Use when a plan, design, decision, or document should be reviewed from several distinct professional perspectives before proceeding — the user says different hats, different perspectives, panel review, or names roles to apply; or when a single reviewer's lens would miss whole failure classes (security vs ops vs UX vs domain).

Fan out **3–4 parallel reviewers**, each a genuinely distinct professional lens on the same artifact, each blind to the others. One review round, one synthesis pass. The output is diverse, structured, evidence-cited findings — not a consensus vote.

<HARD-GATE>
Reviewers raise, don't fix. No hat sees another hat's output before submitting its own. Synthesis surfaces agreement AND disagreement — it does not rewrite the artifact and does not manufacture consensus. No exceptions for "it's a one-line fix" or "the user obviously wants it" — findings route to the user/synthesis, not to your own edits.
</HARD-GATE>

## Why this shape

Every rule below cites a design decision (D-number) in `references/wardrobe/adversarial-review-methodology.md`:

- **3–4 hats, not 5+** — panel quality peaks at 3–4 roles and declines at 5; extra rounds stagnate or degrade. [D5]
- **Lenses must be genuinely distinct** — identical-role panels gain *zero* over a single reviewer; diversity is the load-bearing ingredient. [D3]
- **Blind, parallel first pass** — reviewers who see peers conform sycophantically (weak reviewers correct a wrong majority in only 3.6% of cases). [D4, D11]
- **External review, not self-critique** — self-critique underperforms even a single fresh reviewer. [D1]
- Panels are for richer findings, not raw accuracy — don't sell this as an accuracy booster. [D2]

## Protocol

### 1. Scope
Confirm the artifact — plan, design doc, code, architecture, decision.

### 2. Pick 3–4 hats — from the wardrobe
Hats are **chosen from the shared wardrobe** (`references/wardrobe/`), not invented per-run. Propose 3–4 roster hats that fit the artifact and confirm; the user may name others. **Distinctness test** [D3/D16, formalized in `references/wardrobe/hats/HATS.md`]: each hat must own a failure class the others do not — compare each candidate's `## Failure classes` (body) against the `overlaps` field (frontmatter); if two would flag the same issues, swap one out. Distinctness is declared here, then *measured* by the overlap eval (a procedure run by hand when a hat is added or changed, `references/wardrobe/hats/HATS.md`; nothing runs it automatically) — never assumed [D16]. If the artifact's value is exhaustiveness (requirements, checklist, migration inventory), one hat must be the **coverage** lens [D9].

A requested lens that isn't in the wardrobe: draft it inline for this run following the HATS.md contract, and flag it for wardrobe addition — don't silently improvise a vibe.

Roster (full table in `references/wardrobe/README.md`) — general software: `architect`, `senior-engineer`, `security`, `sre`, `frontend`, `product`, `coverage`, `simplifier`, `finance`, `coach`; agentic-infra (skills/agents/prompts/harness): `prompt-engineer`, `harness-engineer`, `skill-designer`, `eval-engineer`. Example sets — API design: security, sre, senior-engineer, coverage. Life/personal decision: finance, coach, product. Reviewing a skill/plugin: prompt-engineer, harness-engineer, skill-designer, eval-engineer.

### 3. Dispatch — parallel, blind
Each hat runs as its own subagent, all in parallel, blind to each other.

**Run from the main thread only.** This mode fans out to parallel subagents; a subagent has no dispatch tool, so if you are yourself a dispatched subagent, do NOT try to fan out — review the artifact in-thread through the relevant lens(es) and say explicitly that you ran single-threaded.

**A dropped hat is a missing lens, not a clean one.** If a dispatched panelist dies, times out, errors, or returns empty, do not silently synthesize the survivors as if the panel were complete — a missing lens means its whole failure class went unreviewed. Re-dispatch that one hat once; if it fails again, name it in the synthesis as **not reviewed** and do not imply its classes are clean.

**Launch each panelist from the `panelist` role** (`references/roles/panelist.md`): a **general-purpose** subagent whose prompt is the role file's prompt section plus the composed task — the **hat file's body** (`references/wardrobe/hats/<name>.md`, `## Role` through `## Severity anchors` — **exclude the frontmatter**; `overlaps` is registry metadata and irrelevant detail in the payload [D16, D13]) + the artifact (or paths to read). The role carries the neutral-stance protocol and the output contract below.

Use the model and effort the role file recommends; the user may override — e.g. diversify models across hats, which may matter as much as personas [D3, D16, open Q3].

Every panelist prompt carries the artifact and enforces:

- Evaluate only through your lens (the hat body); cite evidence (location + verbatim quote) for every finding [D15].
- Commit to your positions — do not hedge toward what other reviewers might think [D4].
- Raise, don't fix.
- **Reason first, structure last** — fill `## Analysis` as free prose before any finding; assign severity only after the evidence is quoted [D14, D15].
- Use the CRITICAL flag for hallucinated claims, internal contradictions, or showstoppers.

### 4. Per-hat output contract — reasoning-first [D14]

Analysis comes first as free prose; structured findings come last. Never assign a severity or verdict before the evidence is quoted [D14, D15].

```markdown
# [Hat] review

## Analysis
[Free prose — no format requirements. Reason through the artifact in this lens,
quote what you see, explore. This section exists before any finding so severity is
never assigned answer-first [D14].]

## Findings
[One block per finding. Severity is set ONLY after evidence is quoted; a finding
with no verbatim-quote evidence is capped at the lowest band (nit) [D15].]
- id:              F1
  failure_class:   [from this hat's declared `## Failure classes`]
  severity:        blocker · major · minor · nit   (shared taxonomy — HATS.md)
  evidence:        "[verbatim quote]" — [location: file/section/line]
  claim:           [one sentence]
  suggested_probe: [how the synthesizer/author could verify this]
Prefix an id with CRITICAL- for override-level issues (hallucinated claim,
internal contradiction, showstopper) [D8].

## Not found
[One explicit "checked X — clean" line per `## Always ask` item, so completeness
is visible, not inferred.]

## Blind-spot flags
[Anything observed outside this hat's declared classes — routed to synthesis, not
scored [D13].]

## Confidence
[high/medium/low — and what would change it]
```

### 5. Synthesis — main thread, never one of the hats [D10]

```markdown
# Hats synthesis: [artifact]
## Executive summary
[verdict + the 3 things that matter most]
## Where hats agree
## Where hats disagree
| Topic | Hat A | Hat B | Tension |
Disagreement is signal — present it, don't average it away.
## Severity rollup
| ID | Severity | Finding | Hats flagging |
## Recommended next steps
```

Rules: CRITICAL findings surface in the summary even if only one hat raised them [D8]. Treat a hat's stated confidence as calibration context, not severity — severity comes from evidence [D6]. One round: do not send synthesis back to the hats for another pass [D5].

## After synthesis
Present to user. Wait for direction. Do not implement. No exceptions for "it's a one-line fix" or "the user obviously wants it" — findings route to the user/synthesis, not to your own edits. Suggest mode **audit** or **red-vs-blue** when one fits. Suggest, do not auto-chain.

## Not this mode
- **audit** — one neutral reviewer, one findings report. Suggest mode audit.
- **red-vs-blue** — adversarial attack/verify pair for high-stakes artifacts. Suggest mode red-vs-blue.
- **grill-me** — interactive interview to sharpen a plan

## Evidence basis
Full findings, refuted claims, and caveats: `references/wardrobe/adversarial-review-methodology.md`
