# Delegation and tool routing

- Delegate only a concrete, bounded task that can run independently.
- The orchestrator plans substantial exploration or file work, then assigns it to an appropriate worker. A worker executes its task and does not re-delegate it.
- Prefer the cheapest capable model for delegated work. Reserve the strongest model for orchestration and reviews that need it.
- Inspect the target harness's installed models, supported effort levels, permissions, and flags before dispatch unless a maintained current record already settles them.
- Pin the model and supported effort explicitly for headless cross-harness work. Never rely on ambient defaults.
- Report the model that actually ran and the exit status.
- Give workers self-contained prompts and explicit file ownership. Never include secrets.
- In the restricted cross-harness OpenCode worker workflow, use qualifying free models only. A paid or plan-included OpenCode worker requires explicit human approval for that task. This does not restrict normal local Claude or Codex delegation under their existing account plans.

## Restricted harnesses

The supported harnesses are the ones setup installs, Claude Code, Codex, Cursor and OpenCode, and cross-harness delegation between them is allowed.

- Pi Agent is not supported.
- OpenCode runs under the deny-by-default configuration setup seeds (file inspection only). Widening it for a task needs the user's go-ahead.
- A noninteractive worker that would require a permission prompt must be treated as unable to perform that action.
- Do not trust headless agent routing that silently falls back to another agent or model. Verify actual routing from output or session metadata.

Version-specific model catalogs, free-model lists, CLI invocations, bugs, and launch recipes are historical or operational evidence. Keep them in a dated harness runbook and reverify before use rather than loading them as universal behavior.

## Tool references

- Before answering any question about an external library, framework, SDK, API, CLI tool, or cloud service, use Context7 to resolve its library identifier and query current documentation. Skip this only for the human's own code or business logic, general programming concepts, or an explicit request to answer from memory. If Context7 is unavailable, use current primary documentation and state the fallback.
- Dispatch the `scrutineer` role when asked to audit, sanity-check, or verify a research notebook, findings document, or analysis before treating it as trustworthy. Scrutineer reports a claim ledger and evidence rigor without editing the target. Use code review for a diff and a builder for implementation.
- Read a role-owned procedure before a multi-part design, architecture, or research effort when routing says that procedure applies.

## Claude Code role residue

Claude Code dispatches subagents through the Task tool. For its model classes, cheapest capable means Sonnet class and strongest means Opus class. Keep this mapping with the delegation role, not in a harness-global shim.

## Subagent dispatch

Load before dispatching an Agent or Task subagent.

- **Offload heavy file work.** For any task with substantial exploration or many file reads, writes or rewrites: plan the work in the main thread first, then dispatch a subagent to execute it and return a concise report. Raw file contents and edit churn stay in the subagent, keeping the main thread's context lean.
- **Hand long-running work to a background subagent so the main chat stays free for discussion.**
- **A stage that writes files may return a short pointer; use structured output only when the verdict itself is the deliverable.** Do not force a schema onto a stage whose product is a file on disk.
- **Haiku is the default subagent model; the dispatching session overrides it when the task needs better thinking.** This holds on every project unless the project says otherwise, and covers every model choice made on the user's behalf: a subagent's `model`, a mod's `$.model.complete`, an observer or summarizer. For judgement work (review, design, synthesis, hard debugging), set `model: "sonnet"` or `model: "opus"` on that call, with an effort level.
- **A brief that shares a worktree must forbid repository-wide git commands by name.** When two seats work one worktree on a file-scope split, every dispatch brief says the worktree is shared and names the forbidden commands: no `git stash`, `git checkout .`, `git reset`, `git clean`, `git restore`. **A file-scope instruction alone is not enough**, because a seat can honour "only edit `tests/src/`" exactly and still run a command whose blast radius is the whole tree. A stash by one seat sweeps the other seat's uncommitted work with it. A seat needing a clean tree for diagnosis gets a throwaway clone, never a stash. The merge-side half of this rule: colliding uncommitted work is never stashed or hand-resolved to land a merge.
- **Run the review gate in this order.** Put any new or changed rule in the checkout's rulebook copy before dispatching, and quote it in the implementor's brief as well as the reviewer's. Run the project's mechanical check for searchable rules before the first review round, and its gate script, and put the script's own table in the brief rather than a summary of it; name the facts the change altered and sweep them (`finding-protocol.md`, "Before round one"). A brief states only what a command in this session showed. Brief the fixer to fix each listed finding and then sweep the whole target for the same rule. A round-two failure ends the loop (`finding-protocol.md`, "Rounds"); the caller raises what is left for a ruling and does not dispatch a third.
- **The pipeline is plan, implement, review, two rounds at most** (signatures and long plan-review rounds do not earn their cost in a full agentic loop).
  1. **Plan.** Revise the design from the last review's findings. End with a self-audit: which architecture patterns the work uses, which it deliberately does not, and why.
  2. **Implement.** Build the design, and above all write tests that lock in the intended behaviour (the rulings, contracts and done-when), so it cannot drift in later rounds or later work. Run lint and tests for what it touched.
  3. **Gates.** The caller runs the gate script and pastes its table into the review brief.
  4. **Review.** Check whether each intended behaviour can be verified, more than whether the code looks right: which test or check proves it, whether that test would fail if the behaviour broke, and what only a stand-in stack or real hardware can show (`finding-protocol.md`, standing checks 7 and 8). Findings feed the next round's plan.

  Work not converged after round two goes to the human with what is left; there is no third round.
- **A subagent never re-delegates, except a named PR manager, one level down** (to cut the coordination load of parallel PRs; the main chat keeps only rulings and decisions). The default is unchanged: if you ARE a subagent, do the delegated work yourself. The one exception is a **PR manager**, which the main thread dispatches with a brief that says "you are a PR manager". A PR manager owns one worktree and one PR, and runs that PR's pipeline itself (the loop below: plan, implement, the gate script, review, two rounds at most). It dispatches only the existing worker agent types, only one level down, and never dispatches another manager. Workers never delegate. A manager does not rewrite a worker's brief into a near-identical one to pass it on; each dispatch names a different stage or a different fix list. A manager stops and reports, and does not decide, on: a ruling the finding protocol reserves for a human, a conflict with another PR, an unverified fact it cannot check, a Docker-only test, a review still failing after round two. The manager's final report lists those under `DECISION NEEDED`, or says plainly there are none.

