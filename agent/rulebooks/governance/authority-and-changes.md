# Standards governance

- Canonical standards live in this rulebook set (`agent/rulebooks/`).
- Make accepted or authorized standards changes at the canonical source first. A casual question or discussion does not authorize a rewrite.
- Derived agent files and harness adapters may lag. A lagging derivative never overrides its canonical source.
- Resolve contradictions at the canonical source, then regenerate or revise affected derivatives.
- A global instruction file is optional. If present, keep it to indispensable universal behavior. It contains no identity, task routing, or standards catalog.
- Put a rule in the narrowest role standard that must load it. Executable project roots may route roles to focused standards. Do not create organizational workspace, category, or arbitrary nested-folder instruction layers.
- Project facts and operating commands belong in project-owned docs. Reusable standards belong here.

## Authority and execution

- The human decides what a thing is: splits, merges, moves, renames, new repositories, and changed contracts must be stated by the human in their own words before implementation.
- Research, proposals, and plans do not authorize writes. Execute only after an explicit instruction such as "do it", "build it", or "implement".
- An approval of a plan does not approve a structural decision hidden inside it. Surface the structural decision directly.
- Diagnose with evidence before fixing. If blocked or changing approach, stop and present two or three concrete options.
- Never invent an exception because an issue seems immaterial. Follow the governing standard or surface the ambiguity.
- Do not wait for routine review that the human did not request. Ask before committing unless the task already authorized a commit.

## Completion discipline

Before declaring completion:

1. Check for stale docs, TODOs, configuration drift, missing tests, and references to changed paths.
2. Record warranted follow-up work in the owning task ledger with the current date, under the task ownership rule below.
3. Apply the nearest documented staleness checklist.
4. State the verification boundary and any unresolved risk.

Bootstrap missing project instructions only after reporting what is missing and receiving authorization. Draft project context from its README, structure, and build system. Do not infer a new project shape during bootstrap.

## Task ownership and memory

Each project owns its task ledger at `<project-root>/TODO.jsonl`. Do not route a project's tasks to a shared workspace-root ledger or store follow-up tasks in memory files. Reconcile existing alternate trackers with their owning ledger rather than creating competing task lists.

Follow-ups have no mandatory memory-file destination, and ordinary memory saves do not require a separate approval step under these standards. Use the supported write mechanism and preserve applicable scope, provenance and conflict checks. This does not change a storage system’s own access or write constraints.

## Release ownership

- Use conventional commit prefixes.
- Never add `Co-Authored-By` trailers.
- Use `git merge --no-edit` for merges.
- Major and minor release changes require the human's explicit request. An agent may apply a patch change when shipping a fix if the project owns semantic versioning.
- Do not invent, freeze, or bump a standards version. Date the change; add a version only when the human asks for one.

## Portable templates and direction of flow

The portable layer of the code and documentation standards. These files hold the rules that do not depend on any one project: the comment tests, the error mechanism, the logging levels, the quality and review criteria, the design doc standard.

They are templates, not the enforced copies. A project does not point at this directory, because a clone of that project would not be able to reach it. Instead each repository carries its own whole document per topic, derived from the template here and bound to that project's own type names, fixtures, correlation ids, and paths. The derived copy ships with the repository, so anyone who clones it, and any agent working in it, has the standard in hand without needing anything from this workspace.

### Direction of flow

Material is authored and settled in a project's own notes, promotes up to here once the generic part of it is clear, and ships out to each repository as a bound instance. Amendments go the same way: change the template here, then apply the same change to each derived copy. The derived copy names the template and the date it derived from, so a divergence can be found by reading the two side by side.
