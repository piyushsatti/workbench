# Finding protocol

How a review agent reports what it found, and how a fixing agent reads it back.

## The return

A review returns three things in this order, and nothing else:

1. a `pass:` line,
2. a `findings:` block,
3. a `## RAISE` section, only when there is something to raise.

Prose before the `pass:` line is required to carry the `checks run:` block described under "Standing checks", and is otherwise worth writing: the strongest observed rounds opened by saying what the reviewer actually did, which gates they reproduced themselves rather than took on trust, and what they could not execute. A reviewer that cannot say where it read a fact marks that finding unverified on its face.

### The pass line

```
pass: true
```

`true` or `false`, lowercase, nothing else. `pass: false` whenever there is at least one `defect`. A `plan conflict` alone does not fail a review, because it is not the author's to fix; it is routed instead.

### The findings block

One finding per line, five fields, separated by ` | `:

```
<path>:<line> | <rulebook-ref> | <type> | <subtype> | <text>
```

- **`<path>:<line>`** — the file and line the finding anchors to. A range (`:297-320`) is allowed. A finding about something a document should say and does not is anchored to the document with `(silent)` in place of the line.
- **`<rulebook-ref>`** — the rulebook and heading the finding cites, as `` `code-quality.md` § Shape ``. This field is what makes a finding actionable, so it is never blank and never a bare filename: the heading is the part a fixer reads before changing code.
- **`<type>`** — `defect`, `plan conflict`, or `nit`. See below.
- **`<subtype>`** — `rule` or `judgement`. See below.
- **`<text>`** — one paragraph, stating the defect and why the cited rule makes it one. Concrete over categorical: name the input, the state, and what goes wrong.

### Types

- **`defect`** — the code breaches a named rule. The author fixes it without asking. This is the overwhelming majority of findings.
- **`plan conflict`** — the only fix would change a signed contract (a plan's `## Signatures` section, a binding amendment, a ratified schema), or two binding documents prescribe different things and the code cannot satisfy both. **Reserved for a human.** See below.
- **`nit`** — a real but trivial breach. Report it; never let it fail a review.

### Subtypes

- **`rule`** — a named rulebook heading says so, and the finding quotes it.
- **`judgement`** — no named rulebook covers the point, so the reviewer applied its own judgement. A `judgement` finding must say so in its text and must also be raised under `## RAISE`, because a rule needed for a finding that exists nowhere in the named rulebooks is a gap in the rulebooks.

## Reserved for a human

**A `plan conflict` is never fixed by an agent.** A fixing agent skips every one of them and records `not fixed: plan conflict <file>:<line>`; the calling session routes it for a ruling, and the ruling is written to the project's decision record before any code changes.

The reason is the whole point of the type: the fix would change what was signed, so applying it silently converts a decision someone owns into a change nobody reviewed. The observed practice is to raise these *before* code review where they are already known, so a reviewer does not spend a round rediscovering them as signature drift.

Everything else — `defect` and `nit` — is the author's or the fixing agent's to apply, in the order given.

## The RAISE section

Headed `## RAISE`, at the end, present only when it has content. It carries what the reviewer could not settle inside its own scope:

- two rulebooks contradicting each other on a point, quoting both;
- a rule needed for a finding that exists nowhere in the named rulebooks;
- the target contradicting a decisions document the caller named;
- symbol tooling unavailable for a file type, so textual search was used: `symbol tooling unavailable for <ext>; textual search used`.

A RAISE never blocks the review. The reviewer files what it can and raises the rest.

## Rounds

Work runs as a loop of plan, implement and review, at most **two** rounds, with no signature stage. Round two plans from round one's findings, implements, and is reviewed again. A second round that still does not pass ends the loop rather than opening a third: something upstream is wrong, and the caller takes it to the human. Reopening is a ruling, and the failing rounds stay in the record as history rather than being erased. A ruling to reopen names what went wrong to reach the cap (which brief, rule or agent let the defects through), and the caller records it in the project's log before the next round.

A round narrower than the last is titled `scoped confirmation` and says in its header what it was scoped to, so a later reader cannot mistake a confirmation for a full pass.

Each round is written to its own file, `review-code-r<N>.md`, and carries a header line naming the agent, model, head commit, the diff expression, and the dispatch and return timestamps:

```
`agent: <reviewer>` `model: <model>` `head: <sha>` `diff: git diff origin/main...HEAD` `dispatched: <UTC time>` `returned: <UTC time>`
```

The agent's return is written **verbatim**. Where the calling session edits it at all — unescaping HTML entities is the only observed case — the edit is declared immediately under the header, naming exactly what changed.

## Rules that reach every line

A rule that applies to every line of the target (a path form, a naming form, a banned string) is a class of finding, not a single one. Reviewing it one sample at a time costs a round per sample.

- **A script checks it, not the reviewer.** When the project has a mechanical check for the class, the caller runs it before round one and clears what it reports. A review round is never spent on what a search would have found.
- **The reviewer lists every instance.** For a class with no script, the reviewer searches the whole target, names the search it ran, and reports every hit. It does not report a sample.
- **The fixer fixes the class.** After the listed lines, the fixer runs the same search across the whole target and fixes each remaining hit of that rule. A hit outside the diff is raised, not edited.
- **A rule made mid-review lands first.** A ruling that adds or changes a rule goes into the rulebook template and the checkout's copy before the next round. Until it does, the caller quotes the rule in every dispatch, the implementor's included.

## Before round one

Two things are done by the author before the first dispatch, so no round is spent on what the author could have found.

- **Run the gate script and quote its table.** The caller runs the project's gate script on the checkout being reviewed. The brief says which gates passed by pasting the script's table, and claims nothing the table does not show. A gate the table lacks, or marks not run, is unverified and the brief says so. The same script runs again after every fix, by the fixer and then by the caller.
- **Sweep the facts the change altered.** When a change alters what a state set, a column, a field or a setting means, or what "absent" or "never" holds true for, the author names the identifiers and lists every absolute claim about them. Each line is either fixed because the change made it false, or listed in the brief as checked and still true. The reviewer's standing check 5 finds what the author missed; it is not the first search.
- **A fixer's edit is swept too.** A sentence the fixer writes or rewrites gets the same sweep on its names, and the fixer never runs a formatter over Markdown or with any config but the project's, and never restores a file to undo one. A file the fixer made noisy is reported, not repaired.

## Standing checks

Every review runs all of these on the whole target in round one, whatever the caller's brief says, and the implementor and fixer run them on their own changes before reporting. A check that finds nothing still gets its line in the return. Each round after the first repeats only the checks whose lines changed.

1. **Rule classes.** Every rule that reaches every line (see above): search the whole target, name the search, report every hit.
2. **Enumerations.** For each new or changed setting, flag, route, error type, log event or table, search by its kind for every list that enumerates that kind (README, runbooks, config examples, docs, fixtures, tests that count them) and confirm each one is true after the change. A count in prose ("the six variables") is an enumeration.
3. **Caught exceptions.** Every new `except` or `catch` either re-raises or logs once where it is handled, and the event is in the docs' log list.
4. **Claims in prose.** Every sentence the change adds or edits about who uses, reads, calls or emits something is checked against a search for every user of it. Docstrings and module docs state the function's logging and raising where it logs or raises.
5. **Stale neighbours.** Comments and docs outside the diff that the change's behaviour makes false are found by searching for the names the change touches, and are fixed or raised.
6. **Defaults that block.** A new default, limit or gate is run against every shipped configuration and fixture (local stack, dev, examples): does the existing flow still work with it unset?
7. **Acceptance and verifiability.** Each ticket requirement, and each behaviour the plan set out to lock in, names the test or check that proves it. The reviewer says whether that test would fail if the behaviour broke, and lists what only a stand-in stack or real hardware can show. Each new error path has its own test.
8. **Human sequences.** Every command sequence the work hands to a human (a runbook, a stand-in sequence, a pasteable block) is checked step by step against what it calls:
   - each command and flag exists in the script or tool it names (read its usage, or run it with `--help` or `--dry-run`);
   - each step says what the human should see, and that is what the code prints;
   - every step the author could run where they work was run, and its output is quoted;
   - a step that needs root or the human's own terminal says so;
   - a step that depends on state left by an earlier step names that step;
   - a claim the sequence makes about the system (what a route answers, what a count includes) cites the line it was read from, or is marked unverified.

   A step only the human can run is listed as such, with what would show it failed.

