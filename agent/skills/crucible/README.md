# crucible

Lens and stance review system, as one agent skill. Every reviewer is a tuple **(lens, stance, model, effort)**: the **lens** is what the reviewer cares about (architect, security, frontend and so on), the **stance** is how it engages (`partner`, `neutral`, `attack` or `verify`), and model and effort are recommended per role. A lens is defined once in the shared **wardrobe** and used by every mode.

## Modes

| Mode | Stance | Job |
|------|--------|-----|
| `consult` | partner | "Think this through with me": one lens, in-thread, conversational. No subagents. |
| `audit` | neutral | One reviewer, one findings report. |
| `hats` | neutral | "What am I missing?": 3-4 distinct lenses as a blind parallel panel, then a synthesis. |
| `red-vs-blue` | attack + verify | "Will this break?": red attacks, blue verifies, an adjudicator gives the verdict. Gated by a worthiness check. |

`SKILL.md` routes to a mode from the request, or from an argument such as `crucible hats`.

## Layout

```
SKILL.md                      router
references/
  consult.md audit.md hats.md red-vs-blue.md   one procedure per mode
  roles/                      subagent role prompts, launched as general-purpose subagents
    panelist.md               hats: one per lens (sonnet, medium)
    red-attacker.md           red-vs-blue: attack (opus, high)
    blue-verifier.md          red-vs-blue: verify (sonnet, high)
    adjudicator.md            red-vs-blue: verdict (opus, high)
  wardrobe/
    README.md                 what the wardrobe is, the roster, how hats are picked
    adversarial-review-methodology.md   the evidence base, cited by D-number
    hats/HATS.md              the hat authoring contract and severity taxonomy
    hats/<name>.md            14 lenses
```

The model and effort pins are recommendations and still provisional: the seeded-flaw benchmark hasn't separated the tiers. Each role file says why.

## Provenance

`audit`, `hats`, `red-vs-blue`, the wardrobe and the four roles came from an earlier MIT-licensed review library; `consult` is crucible's own. The plugin's Codex `agent-dispatch` shim, manifests, install file and plan-review hook were not carried over.
