# Red vs Blue

Use for adversarial stress-testing of a high-stakes artifact — architecture, contracts, launch/no-go decisions, or any plan where a hidden flaw is expensive. Trigger when the user says red team, blue team, attack this plan, or adversarial review. Routine review belongs to audit or hats.

Two independent reviewers with opposite framings on the same artifact, plus a separate adjudication pass.

- **Red attacks** — assumes the plan fails in production and hunts for how.
- **Blue verifies** — cooperative but skeptical: confirms what is actually sound with evidence, and runs the coverage check that flaw-hunting structurally misses.
- **Synthesis adjudicates** — the main thread (or a third agent) reconciles the two. Blue is *not* red's judge and never sees red's findings.

Reserved for **high-stakes artifacts**. For routine review use **audit** or **hats**.

<HARD-GATE>
Neither side implements fixes. Neither side sees the other's output. Adjudication happens only in synthesis, by the main thread or a third agent — never by red or blue themselves. No exceptions for "it's a one-line fix" or "the user obviously wants it" — findings route to the user/synthesis, not to your own edits.
</HARD-GATE>

## Why this shape

D-numbers cite `references/wardrobe/adversarial-review-methodology.md`:

- External adversarial critique beats self-critique and single review; but the payoff is structured findings, not raw accuracy.
- Red and blue run **parallel and blind** — exposure breeds sycophantic convergence.
- **Blue verifies rather than counter-attacks** — pure flaw-hunting produces incorrect verdicts on sound work as its own failure mode.
- Adversarial review is **precision-biased**: it removes bad content but shrinks coverage (measured −7.5% on completeness-oriented artifacts). Blue's mandatory coverage check is the counterweight.
- **Adjudication is separate** from both reviewers; the plan's author has standing to reject findings — most rejections are legitimate (58% in the one measured system), but disputed critical findings escalate.
- **One round.** No red-on-blue rebuttals; more rounds stagnate or degrade.

## Worthiness gate (before dispatch)

Red-vs-blue is expensive. Before dispatching the pair, run a **cheap in-thread check** — no subagent, seconds — on three axes:

- **Stakes** — what's the blast radius if a hidden flaw ships? Low blast radius → probably not worth the pair.
- **Maturity** — is the artifact complete enough to attack? Attacking a stub or a sketch wastes the pair.
- **Fit** — is this completeness-shaped work (requirements, checklist, inventory) better served by **hats** with a coverage lens?

If any check fails, tell the user and ask — never block:

> "This doesn't look worth a full red-vs-blue because ⟨reason⟩ — audit or a single consult would do. Continue anyway?"

The user's call is final. The gate **informs, never blocks** — if they say go, go.

## Protocol

1. Confirm the artifact — plan, design, spec, decision. Run the **worthiness gate** above.
2. **Optional lens.** If the user names a lens ("red-team this as a security reviewer") or one clearly fits, load that hat from the wardrobe (`references/wardrobe/hats/<name>.md`) and give **both** red and blue the **same** hat **body** — red attacks through it, blue verifies through it. Pass the body only (`## Role` through `## Severity anchors`); exclude the frontmatter — `overlaps` is registry metadata and irrelevant detail in the payload. No lens → both run the general reviewer stance below. One shared lens for the pair, not one each, keeps them on the same surface.
3. Dispatch red and blue as parallel subagents, blind to each other. **Launch them from the `red-attacker` and `blue-verifier` roles** (`references/roles/red-attacker.md`, `references/roles/blue-verifier.md`): each a **general-purpose** subagent whose prompt is the role file's prompt section plus the artifact (+ the hat body, if a lens was chosen). The roles carry the Red / Blue instructions below. Use the model and effort each role file recommends; the user may override.

**Run from the main thread only.** This mode fans out to parallel subagents; a subagent has no dispatch tool, so if you are yourself a dispatched subagent, do NOT try to fan out — review the artifact in-thread through the relevant lens(es) and say explicitly that you ran single-threaded.
4. **Both-returned gate — an empty side is a failure, not a clean bill.** Before adjudicating, confirm **each** subagent returned a substantive, well-formed report (an `## Analysis` plus its required sections). A silent death, a timeout, an error, or an empty/truncated reply is **not** "found nothing" — a missing blue reads as *false "sound"* and a missing red as *false "no attacks."* If either side is missing or malformed, **re-dispatch that one side once**; if it fails again, **halt and tell the user which side failed** — never emit a `proceed` verdict on a half-run pair. Record which side(s) actually ran in the synthesis header.
5. Adjudicate in synthesis — **prefer the `adjudicator` role** (`references/roles/adjudicator.md`, launched as a general-purpose subagent with both reports and the artifact, at the model and effort it recommends); else the main thread (or a third general-purpose agent for maximum independence). Never red or blue.

## Red instructions

- **Reason first.** Open with `## Analysis` as free prose — hunt for how this fails, quote what you find, explore — before writing any structured finding. Never assign a severity answer-first.
- Assume the plan fails in production. Find how. Attack vectors: missing edge cases, false claims, hidden dependencies, untested assumptions, single points of failure, spec/intent drift, cost-explosion paths.
- **Independence contract:** do not restate the artifact's own reasoning as findings. Every finding cites evidence distinct from the artifact's self-justification — a location, a contradiction, a missing case.
- **Evidence before severity:** assign a finding's severity **only after** you have quoted its evidence (verbatim quote + location). A finding with no quoted evidence is capped at the lowest severity band (nit). Tag each finding with a `failure_class` and a `suggested_probe` (how the adjudicator/author could verify it).
- Commit to positions; no diplomacy, no "overall this looks great". Mark speculation explicitly.
- **If given a lens** (a wardrobe hat body): its `## Failure classes` are your priority attack vectors and its `## Evidence demands` is your bar — but you are still red. Attack through the lens; don't just describe it.

## Blue instructions

- **Reason first.** Open with `## Analysis` as free prose — read the artifact independently, quote what holds and what worries you — before the structured sections below.
- Full independent read. You will never see red's output — you are not red's judge.
- **Verify, don't flaw-hunt:** identify which load-bearing claims and decisions actually hold, with evidence. An incorrect "this is fine" is a failure equal to a missed flaw.
- Defend sound decisions with evidence — not everything; only what earns it.
- **Coverage check (mandatory):** what is missing for this artifact to be complete — failure paths, rollback, monitoring, requirements the artifact never mentions?
- **If given a lens** (the same wardrobe hat as red): verify through it — confirm the lens's load-bearing concerns actually hold — but keep the mandatory coverage check regardless of lens.

## Output contracts

Both open with `## Analysis` (free prose) before any structured section — reasoning first, structure last. Severity is the shared taxonomy from `references/wardrobe/hats/HATS.md`.

```markdown
# Red team: [subject]

## Analysis
[Free prose — how this fails if we're unlucky. Reason, quote the artifact, explore.
No format requirements; this comes before any finding so severity is never
assigned answer-first.]

## Findings
[Severity set ONLY after evidence is quoted; unevidenced findings capped at the
lowest band (nit).]
- id:              R1
  failure_class:   [attack vector / class]
  severity:        blocker · major · minor · nit
  evidence:        "[verbatim quote]" — [location] (distinct from the artifact's own reasoning)
  claim:           [one sentence: how it fails]
  suggested_probe: [how the adjudicator/author could verify it]
Prefix an id with CRITICAL- for override-level issues.

## Assumptions to kill     [assumption → what happens when it breaks]
## Unreviewed attack surface
```

```markdown
# Blue team: [subject]

## Analysis
[Free prose — what is sound and why, what worries you. Reason and quote before the
structured sections.]

## Verified-sound decisions
| Decision | Evidence it holds (verbatim quote + location) |
## Coverage gaps           [mandatory — may be "none found", never omitted]
## Risks worth accepting   [with rationale]
```

## Synthesis — main thread or third agent, never red or blue

Adjudicate each red finding against blue's independent read: **valid / false-alarm / acceptable-tradeoff**. Severity comes from evidence, not from how confident red sounds. **Re-check each red finding's evidence field** — a finding whose evidence is missing or is just a restatement of the artifact's own reasoning is capped at the lowest severity band (nit). Reason through the reconciliation before stating the verdict — verdict last, not first. If the plan's author (user or agent) rejects a finding, expect that to often be legitimate; escalate only disputed CRITICAL/blocker findings to the user.

```markdown
# Red vs Blue synthesis: [subject]
## Reviewers run           [red: ok/failed · blue: ok/failed — if either failed, verdict is halt-incomplete, not a clean bill]
## Analysis                [reconcile red vs blue, re-check evidence — before the verdict]
## Verdict                 [proceed / proceed with fixes / halt and redesign / halt — incomplete review (a side failed)]
## Must address            [red valid + blue's read supports]
## Acceptable as-is        [blue defended with evidence]
## Coverage gaps           [from blue — the completeness counterweight]
## Disputed — user decides
| Issue | Red | Blue | Why unresolved |
## Recommended next steps
```

One round: do not loop red on blue's output or re-run the pair on the same artifact version.

## After synthesis
Present to user. Wait for direction. Do not implement. No exceptions for "it's a one-line fix" or "the user obviously wants it" — findings route to the user/synthesis, not to your own edits. Suggest mode **audit** or **hats** when one fits. Suggest, do not auto-chain.

## Not this mode
- **audit** — single neutral reviewer. Suggest mode audit.
- **hats** — 3–4 professional lenses, not adversarial pairing; prefer it for completeness-oriented artifacts (requirements docs, checklists). Suggest mode hats.
- **grill-me** — plan interview, not attack/verify

## Evidence basis
Full findings, refuted claims, and caveats: `references/wardrobe/adversarial-review-methodology.md`
