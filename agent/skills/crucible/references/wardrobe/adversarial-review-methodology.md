# Adversarial & Multi-Lens Review — Methodology Basis

**Status:** active
**Grounds:** `references/red-vs-blue.md` (adversarial pair) and `references/hats.md` (multi-lens panel)

**Provenance:** synthesized from a deep-research run — 5 search angles, 26 primary sources fetched, 129 claims extracted, 25 claims put through 3-vote adversarial verification (19 confirmed, 6 refuted), merged to 11 findings. Refuted claims are listed below so they are never cited.

---

## TL;DR — what the evidence actually supports

Adversarial and multi-perspective review of artifacts **measurably beats single-reviewer and self-critique baselines** across reasoning, evaluation, and fact-checking tasks. Two hard qualifiers:

1. **Persona/lens diversity is the load-bearing ingredient.** Identical-role panels gain literally nothing over a single reviewer. N copies of the same reviewer are worthless.
2. **Multi-agent review is not a generic accuracy booster.** It does not reliably beat compute-matched self-consistency. The value case for these skills is *diverse, structured, evidence-cited findings on an artifact* — not raw ensemble accuracy.

And one domain caveat that applies to everything below: almost all quantified evidence is answer-selection on QA/reasoning/evaluation benchmarks. Critique of long-form plan files and code artifacts — our actual use case — is an extrapolation. Treat protocol parameters as tunable defaults, not laws.

---

## Design decisions

Each decision the skills make points here. Confidence reflects the verification pass, not enthusiasm.

### Use external reviewers; never rely on self-critique. (HIGH)

Debate/review by other agents beats both single-agent output and self-reflection: arithmetic 67.0% single → 72.1% self-reflection → 81.8% debate; GSM8K 77.0→85.0; MMLU 63.9→71.1; biography factuality 66.0→73.8 (Du et al., ICML 2024). Self-reflection was *worse* than single-agent on GSM8K (75.0 vs 77.0) and MMLU (57.7 vs 63.9) — independently consistent with the self-correction-limits literature (Huang et al., ICLR 2024: LLMs cannot yet reliably self-correct reasoning). Evaluator panels: FairEval accuracy 53.8→60.0 (ChatEval, ICLR 2024). Fact-checking: +7.95/+7.86 mF1 (Spanish/Italian) over single-agent CoT, shrinking to +0.48 for low-resource Arabic — gains are real but task-dependent.

### Position the skills as structured-findings generators, not accuracy boosters. (HIGH)

Smit et al. (ICML 2024): "multi-agent debating systems, in their current form, do not reliably outperform … self-consistency and ensembling," attributed to acute hyperparameter sensitivity; replicated across 9 benchmarks / 4 models (arXiv:2502.08788). Design implication: justify red-vs-blue and hats by critique quality and structure, and treat reviewer count / rounds / agreeableness as tunable.

### Lenses must be genuinely distinct. (MEDIUM)

ChatEval ablation: identical role prompts → zero gain over a single agent (53.8% vs 53.8%, kappa 0.27→0.25); diverse role prompts → 60.0% / 0.33. The 1,800-puzzle controlled study (arXiv:2511.07784) independently found reasoning strength and group diversity the dominant drivers. Medium confidence: single ~80-item benchmark ablation; later work partly credits heterogeneous model *backbones* rather than personas. Direction corroborated; magnitude unsettled. Open question: how much of the gain survives with different role prompts on one frontier backbone.

### First pass is blind and parallel; reviewers commit before seeing peers; default to stubbornness. (HIGH)

Sycophantic conformity is the dominant failure mode: facing an incorrect majority, strong models corrected the consensus 30.0–34.4% of the time; a weak model corrected itself in only **3.6%** of cases (arXiv:2511.07784). Du et al.: "stubborn" prompts → longer debates, better final solutions. Smit et al.: tuning prompted agreement intensity gave ~15% improvement (optimum is task-dependent). Implications: no reviewer sees another's output before submitting; instruct reviewers to hold positions; treat agreeableness as a knob defaulting toward stubborn for review work.

### 3–4 reviewers, one review round + one synthesis pass; skip turn-taking machinery. (HIGH)

FairEval accuracy peaks at 3–4 roles, declines at 5 (ChatEval §4.3). More discussion turns show no upward trend — "continual discussion often leads to stagnation or even degradation" (consistent across ChatEval, Liang et al., Du et al.). In the controlled puzzle study, debate depth, confidence visibility, and debate order were all statistically insignificant (p>0.05); team size and reasoning strength/diversity were the significant drivers.

### Do not treat the attacker's expressed confidence as a severity signal. (LOW)

"The Confident Liar" (arXiv:2606.10296): in a Constructor/Auditor protocol, confidence–quality alignment was ~2× stronger for the supportive role than the adversarial one (Spearman ρ 0.335 vs 0.177); detecting critical reasoning failures from confidence: AUROC 0.804 vs 0.634. Unreviewed preprint, single domain — but the safe engineering default is cheap: severity comes from evidence and adjudication, never from how sure red sounds.

### Blue is a verifier, not a second attacker. (LOW — design precedent)

Same paper deliberately separates adversarial framing (Auditor: find flaws) from cooperative-but-skeptical framing (Verifier: "verify correctness rather than to find a flaw at all costs, since an incorrect verification is itself a failure mode"). Rationale, not measured effect — but it directly supports blue validating and defending what is sound instead of counter-attacking.

### Independence contract + structured output contract + critical-issue flag. (LOW — design precedent)

Precedented contracts (arXiv:2606.10296 §3.3.1, §3.5.3): (a) the reviewer may not restate the generator's reasoning and must cite evidence distinct from the artifact's self-justification — otherwise the two reasoning paths aren't genuinely different; (b) mandatory labeled output sections give the adjudicator scoring anchors; (c) a critical-issue flag (hallucinated evidence, internal contradiction, role violation) overrides any aggregate impression. Asserted design rationale, not ablated — cite as precedent.

### Adversarial review is precision-biased; completeness needs an explicit guard. (MEDIUM)

TriAdReview (arXiv:2606.15074): a triangular adversarial review system *degraded* requirements-analysis output −7.5% vs single-model baseline; generator output shrank ~17%; completeness score 4.4→2.8. Authors: adversarial review is "optimized for precision (removing bad content) but harms recall (covering necessary content)." Direction independently corroborated (Huang et al. ICLR 2024; arXiv:2601.11578: feedback raises precision, significantly lowers recall). Implications: blue (or synthesis) runs a mandatory coverage check; apply adversarial review cautiously to artifacts whose value is exhaustiveness (requirements docs, checklists, inventory-style plans) — or route those to a panel with a dedicated coverage lens.

### Adjudication is separate from both reviewers; the author has standing to reject. (LOW)

TriAdReview dispute topology: the judge must be a different agent than the disputed reviewer (no self-adjudication). Across 212 dispute resolutions: generator-wins 58.0%, compromise 26.9%, reviewer-wins 15.1% — but reviewer-wins concentrated on critical issues (security, over-engineering). Implications: expect most pushback against findings to be legitimate; escalate only disputed *critical-severity* findings to an independent adjudicator or the user.

### Topology: parallel-blind first pass, adjudicated synthesis second. (MEDIUM)

ChatEval found sequential one-by-one communication modestly better than simultaneous talk (60%/0.33 vs 55%/0.28) — but sequential exposure conflicts directly with blind first pass (independence against sycophancy). The defensible hybrid, and what both skills implement: parallel independent reviews, then a single synthesis/adjudication pass that is the only place cross-reviewer information flows.

---

## Failure modes these decisions guard against

| Failure mode | Evidence | Guard |
|---|---|---|
| Sycophantic conformity to a (possibly wrong) majority | 3.6% self-correction rate for weak reviewers under wrong majority | blind first pass blind pass, stubbornness |
| Stagnation/degradation over rounds | ChatEval, Liang et al., Du et al. | one review round one round |
| Identical reviewers = zero value | ChatEval ablation | distinct lenses distinctness test |
| Confident-but-wrong attacker steering severity | Confident Liar ρ/AUROC asymmetry | confidence is not severity evidence-based severity |
| Flaw-hunting produces wrong verdicts on sound work | Confident Liar framing rationale | blue verifies blue verifies |
| Review shrinks completeness-oriented artifacts | TriAdReview −7.5%, completeness 4.4→2.8 | completeness guard coverage check |
| Self-adjudication | TriAdReview topology rule | separate adjudication separate judge |

---

## Refuted claims — do NOT cite these

Killed 0-3 or 1-2 in adversarial verification:

1. "Debate converges wrong-to-right even when every agent starts wrong" (attributed to Du et al.) — refuted 0-3.
2. "Performance monotonically increases with more agents and more rounds" — refuted; see one review round.
3. "Debate is upper-bounded by the strongest individual reasoner" — refuted 1-2.
4. TriAdReview's headline "+10.1% overall quality (p<0.05)" — refuted 0-3; cite TriAdReview only for the narrower results in completeness guard and separate adjudication.
5. FC-MAD's SOTA claims on ViFactCheck/FEVER — refuted 0-3; cite only the narrow mF1 gains in external reviewers.
6. "Judge-removal causes the largest ablation drop" (FC-MAD) — refuted 0-3. separate adjudication's adjudication design stands on TriAdReview's topology precedent, not on this.

---

## Open questions

1. Does any of this transfer from answer-selection benchmarks to long-form artifact critique (plans, specs, diffs)? No direct study. Our skills are the experiment; watch outcomes.
2. At matched token budgets, does red-vs-blue-with-adjudication beat N-parallel-hats-plus-synthesis? No paper compares the two topologies.
3. Is persona diversity on a single frontier backbone enough, or is model diversity doing the work in recent results? (If the latter, hats should diversify models, not just prompts.)
4. Right agreement-intensity default for artifact review? Calibrated only for QA (MedQA ~90%); uncalibrated for plan/code review.

## Evidence-quality caveats

- The strongest findings (external reviewers, findings, not accuracy, blind first pass, one review round) rest on peer-reviewed work (ICML 2024 ×2, ICLR 2024). Nearly everything specific to *adversarial pair review* (confidence is not severity, blue verifies, output contract, completeness guard, separate adjudication) comes from 2025–2026 unreviewed preprints, some tiny (n=5 per cell; FairEval ≈ 80 items, no significance tests).
- Headline effect sizes come from GPT-3.5/GPT-4o-mini-class models. Stronger 2026 models self-correct better, which may shrink debate gains and shift optimal agreeableness.
- The compute-matched caveat (findings, not accuracy) is load-bearing for honest positioning.

## Sources

| Work | Venue | Used for |
|---|---|---|
| Du et al., *Improving Factuality and Reasoning in LMs through Multiagent Debate* — [arXiv:2305.14325](https://arxiv.org/abs/2305.14325) | ICML 2024 | external reviewers, blind first pass |
| Chan et al., *ChatEval* — [arXiv:2308.07201](https://arxiv.org/abs/2308.07201) | ICLR 2024 | external reviewers, distinct lenses, one review round, blind-then-adjudicated topology |
| Smit et al., *Should we be going MAD?* — [arXiv:2311.17371](https://arxiv.org/abs/2311.17371) | ICML 2024 | findings, not accuracy, blind first pass |
| Huang et al., *LLMs Cannot Self-Correct Reasoning Yet* — [arXiv:2310.01798](https://arxiv.org/abs/2310.01798) | ICLR 2024 | external reviewers, completeness guard |
| Controlled 1,800-puzzle debate study — [arXiv:2511.07784](https://arxiv.org/abs/2511.07784) | preprint | distinct lenses, blind first pass, one review round |
| Keramati et al., *The Confident Liar* — [arXiv:2606.10296](https://arxiv.org/pdf/2606.10296) | preprint 2026 | confidence is not severity, blue verifies, output contract |
| TriAdReview — [arXiv:2606.15074](https://arxiv.org/pdf/2606.15074) | preprint 2026 | completeness guard, separate adjudication |
| FC-MAD fact-checking MAD — [S2405959526000883](https://www.sciencedirect.com/science/article/pii/S2405959526000883) | journal 2026 | external reviewers (narrow) |
| Liang et al., *Divergent Thinking in MAD* — [arXiv:2305.19118](https://arxiv.org/abs/2305.19118) | EMNLP 2024 | one review round |
| Madaan et al., *Self-Refine* — [arXiv:2303.17651](https://arxiv.org/abs/2303.17651) | NeurIPS 2023 | self-correction limits context |
| Verga et al., *Panel of LLM Judges (PoLL)* — [arXiv:2404.18796](https://arxiv.org/abs/2404.18796) | preprint | judge-panel context |
| MAD replication — [arXiv:2502.08788](https://arxiv.org/abs/2502.08788) | preprint | findings, not accuracy replication |

Supporting/corroborating: arXiv:2509.23055 (sycophancy in MAD), arXiv:2601.11578 (feedback precision/recall), arXiv:2306.05685 (LLM-as-judge biases), TACL 2024.tacl-1.78 (self-correction survey).

---

# Part 2 — Persona & contract layer

Grounds the wardrobe hat files and output contracts.

### A hat is a checklist-contract with a thin domain header, not a character. (HIGH on parts, triangulated as a whole)

Identity personas alone are null-to-harmful: 162 roles × 9 models, no average accuracy gain, best-persona selection no better than random (Zheng et al., EMNLP Findings 2024). What carries effect: task-coupled role framing — what to *do*, not who to *be* (Kong et al., NAACL 2024; contested on some reasoning sets), domain-matched framing (small on recall, material on open-ended review-type generation — Salewski et al., NeurIPS 2023), and explicit decomposed criteria (§anchored severity). Honest caveat: no factorial study pits concern-lists against identity attributes directly; the checklist-centric design is a triangulated bridge across the persona, judge, and defect-detection literatures.

### Strip names, backstories, demographics, and "world-class" flattery. (HIGH)

Irrelevant persona detail is measured *harm*, not neutral filler: performance drops up to ~30pp (Araujo et al. 2025, 9 LLMs × 27 tasks). Demographic markers cause systematic reasoning bias and stereotyped abstentions (Gupta et al., ICLR 2024; corroborated by Zheng and Salewski; unreplicated on 2025-26 frontier models — declared gap). Every non-task-relevant word in a hat file is downside exposure.

### Free-prose Analysis first, structured Findings last; never verdict-first. (HIGH)

The "format restrictions degrade reasoning" dispute resolves to a mechanism both camps endorse: answer-before-reasoning ordering is what hurts (Tam et al. EMNLP 2024 found 100% of JSON-mode responses put the answer key first; dottxt's matched-prompt rebuttal and CRANE's ICML 2025 theory converge on reasoning-unconstrained-then-structure). Corollaries: section/field names are instructions (key wording measurably steers generation, arXiv:2604.14862); schema compliance ≠ correctness (arXiv:2604.25359).

### Shared anchored severity taxonomy; binary-checkable criteria; evidence before severity. (HIGH direction)

Binary-checkable criteria lift evaluator agreement dramatically (CheckEval: α ~0.05–0.09 → ~0.45–0.67); anchored per-level rubric text is load-bearing (Prometheus ablation); ~5 levels best-evidenced granularity (Jan 2026 preprint, medium); evidence-before-verdict is the best-quantified judge mitigation (Wang et al. ACL 2024 +10–14pt; MT-Bench reference-grounding cut math-judging failure 70%→15%). Severity is assigned only after the evidence field is filled; unevidenced findings are capped a band down (cap rule: Rulers, Jan 2026 preprint — emerging, not consensus). Expect criteria drift: rubrics are versioned and revised after real batches (Shankar et al., UIST 2024).

### Distinctness is measured, not asserted; prefer cross-model diversity where possible. (MEDIUM, lean cautionary)

Same-model personas frequently collapse toward one voice (Chameleon's Limit: in one model 83% of variation was stochastic; corroborating preprint shows weak cross-persona differentiation). Hence: hats declare *responsibility* (failure classes), never "uniquely caught" — uniqueness comes from the overlap eval (run two hats on one artifact, compare finding-overlap against declarations, plus a no-persona control the hat must beat on its own classes). The `overlaps` field is registry metadata for the panel composer — kept OUT of the prompt payload (no persona flattery exposure). Cross-model diversity is better evidenced than persona diversity for catching correlated errors (CVE-pipeline case study) — vary the model per hat when feasible.

### Personas drift in ~8 turns; CoT makes it worse; re-inject in long sessions. (HIGH, scope-caveated)

Instruction drift within ~8 rounds from attention decay; system-prompt re-injection is the robust fix (Li et al.). Chain-of-thought *increases* persona response variability (PERSIST, 25 models). Fresh-subagent review dispatches (hats, red-vs-blue) are naturally immune; the consult partner is not — re-state the hat roughly every 6–8 turns [schedule is a DEFAULT derived from the 8-round finding].

### Hat evals (from the research; published designs + declared novelty)

Adherence probes (InCharacter/PersonaGym patterns — anonymize hat names for LLM judges); seeded-defect probes per declared failure class (Deterministic Integrity Gates design: specialized detectors 100% recall vs generic reviewer 41%); distinctness classifier hat-vs-hat (method published for models, application to same-model personas is ours — a genuine gap we're filling, say so); regression re-run after every hat edit (tooling practice, one silent-regression existence proof — a "helpful assistant" wrapper improved one metric +13% while silently degrading two others).

---

# Part 3 — Local benchmark evidence

The literature above is external. This section records what an unpublished seeded-defect benchmark measured. Both runs (model×effort and hat distinctness) are single-run, small-N, directional — not settled.

### L1 — Hat distinctness is real (lane discipline perfect; all hats beat a no-persona control). (measured, N=1 fixture)

On a shared multi-lens fixture, architect/security/coverage/senior-engineer each produced **0 out-of-class findings** (no wandering into another hat's taxonomy) and each surfaced ≥1 in-class defect a no-persona control missed. This is direct local evidence for measured distinctness (distinctness measured, not asserted) and against same-model voice-collapse. Soft spot: on a security-heavy fixture, security↔coverage overlapped most and the security hat had no *fully-unique* catches (the generic control already finds loud security bugs) — so the panel's lift came mainly from architect (design-time lock-in) and senior-engineer (operational edges), which **confirms distinct lenses** (diversity is the load-bearing ingredient) in a measurement. Caveat: one fixture can't separate "hats overlap" from "artifact is security-shaped" — re-run on a non-security fixture before generalizing.

### L2 — On an easy corpus, capability tier does not move recall; max effort inflates false positives on adversarial review. (measured, N=1 corpus)

Red (security) and panelist (architect) both hit **100% recall across all four configs** (sonnet/high, sonnet/max, opus/medium, opus/high) — the corpus was too easy to separate tiers on detection. The only discriminating signal: **sonnet/max raised 10 false-positive defect claims on clean artifacts vs 7 for every other config** — more adversarial effort bought noise, not catches. This is a local instance of the completeness guard precision-bias: cranking attacker aggression trades precision for false alarms. **Actionable outcomes:** (a) do **not** use `max` effort for the red/attacker role; (b) the provisional agent pins (red/adjudicator opus·high, blue sonnet·high, panelist sonnet·medium) are **retained on the research prior, not overturned** — a non-discriminating benchmark is not license to change them; note that **sonnet/medium itself was never one of the four tested configs** (sonnet/high, sonnet/max, opus/medium, opus/high were), so the panelist pin is retained on the prior, not validated by this run; also note the panel slice ran with **no clean controls**, so its "0 FP" figures are structural (zero-by-construction) and were never evidence for the blue-verifier pin either; (c) the adjudicator — the role whose pin most needs data — remains **un-benchmarked** (needs red+blue pairs) and is the top follow-up; (d) corpus v2 needs defects requiring multi-step reasoning so the benchmark can actually separate sonnet from opus.
