---
name: crucible
description: "Lens-based review in four modes. consult: the user wants to think an idea through with one professional perspective (\"think this through with me\", \"be my architect\", \"what would a security person think\"), in-thread, as a partner. audit: one independent reviewer, one findings report (audit, second opinion, strict auditor, spec audit, regression check). hats: \"what am I missing?\", different hats or perspectives, panel review: 3-4 distinct lenses as a blind parallel panel plus synthesis. red-vs-blue: \"will this break?\", red team, attack this plan, adversarial review of a high-stakes artifact: attack + verify pair and an adjudicated verdict, gated by a worthiness check. Also `crucible <mode>`."
---

# Crucible

Every reviewer is a lens (what it cares about, from the shared wardrobe of hats) plus a stance (how it engages: partner, neutral, attack or verify). Four modes compose them.

| Mode | Stance | Ask | Reference |
|------|--------|-----|-----------|
| `consult` | partner | "Think this through with me": one lens, in-thread, conversational | `references/consult.md` |
| `audit` | neutral | One reviewer, one findings report | `references/audit.md` |
| `hats` | neutral | "What am I missing?": 3-4 distinct lenses, blind parallel panel, structured synthesis | `references/hats.md` |
| `red-vs-blue` | attack + verify | "Will this break?": attack and verify pair plus an adjudicated verdict, gated by a worthiness check | `references/red-vs-blue.md` |

## Route

1. If the request names a mode (`crucible hats`, "red-team this", "audit this"), use it.
2. Otherwise pick from the ask: one perspective to talk with → `consult`; one findings report → `audit`; several perspectives at once → `hats`; attack it to see if it breaks → `red-vs-blue`.
3. If two modes fit equally, ask the user which.
4. Read that mode's reference file and follow it. Don't load the others.

All paths in this skill are relative to the skill folder. Lenses live in `references/wardrobe/` (start at `references/wardrobe/README.md`), the subagent role prompts in `references/roles/`, and the D-numbers every file cites in `references/wardrobe/adversarial-review-methodology.md`.
