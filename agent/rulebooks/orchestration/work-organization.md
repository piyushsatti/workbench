# Rulebook: work organization

Load when work is being captured, filed or picked up.

All outstanding work lives in a `TODO.jsonl` in the project root, filed by category and tag, never as a flat list. Closed work and findings go to `LOGS.jsonl` beside it. Facts go to reference docs, as the routing table in [memory.md](../tooling/memory.md) routes them.

- **Capture on arrival, file by domain.** An item being mentioned is not a reason to act on it.
- **Two levels, split by scope.** Cross-cutting work goes in the top-level list. Work belonging to a single project goes in that project's own `TODO.jsonl`, in the project root. When a top-level item turns out to be project-scoped, delegate it: strike it through and point at the project's file instead of deleting it, so the trail from discovery to owner stays readable.
- **Work that fits no existing domain opens a NEW domain section.** Never append loose items to whichever domain is closest. A poor fit is the signal that a domain is missing.
- **Open a domain deliberately, then work its whole list as a set.** Read every item in the domain before starting any of them. Never work a domain item-by-item or ad hoc.
- **Blocked domains stay shut.** If a domain waits on a decision only the user can make, mark it as blocked, name the decision, and leave the domain alone.
- **`TODO.jsonl` holds work, not truth.** Facts discovered along the way go to reference docs with a `verified:` date. They never accumulate in the work list.

## The routing table outranks any step that names a path

**A step in this or any other rulebook, skill or procedure that hardcodes one output path never overrides the routing table** in [memory.md](../tooling/memory.md). The table routes by content type; a fixed path cannot, so where the two disagree the table wins and the step is a defect to be fixed rather than followed. A procedure naming one output path for every input is a defect.

**Retiring a write destination needs a tombstone and a later sweep, not just the edit.** An amendment does not reach sessions already running: they hold the old text for as long as they live and will keep writing to the retired path. Worse, deleting the file underneath them is indistinguishable from data loss, and the conservative reading — rewriting it from what was read — is the correct one for a session that cannot tell the difference. So when a destination is retired: leave a marker where the file was, saying it was retired and where its content now goes; put the same note in the index that pointed at it; and sweep again some hours later, once the sessions holding the old text have ended.

## End of task

Before declaring any task complete:

1. Check whether the work created follow-up needs (stale docs, TODOs, config drift, missing tests).
2. If yes, **route each one by type through the routing table in [memory.md](../tooling/memory.md)**, which for a follow-up need means the owning `TODO.jsonl`, filed by domain, dated, and project-scoped or cross-cutting per the two-levels rule above. A fact learned along the way is not a follow-up: it goes to a reference doc with a `verified:` date. Never open a work list inside `memory/`.
3. If config files, docs or build scripts reference files or paths that changed, flag it immediately.
4. When writing or updating any memory file, set `last_updated: YYYY-MM-DD` in its YAML frontmatter (new files include it from the start).

Each project's own CLAUDE.md defines which files to scan for ripple effects. Check it for that project's staleness list.

## Mirror the open list onto the harness task list

**Working a `TODO.jsonl` domain, or any task of three or more steps, is tracked on the harness task list as well as in the file.** The file is the durable record and stays authoritative; the harness list is the live view of the same items, so progress is visible in the session rather than only on disk.

- **On opening a domain**, call `TaskList` first to avoid duplicates, then `TaskCreate` one task per item you are actually taking on in this sitting. Not the whole domain unless the whole domain is the sitting. Deliberate opening still comes first: read every item in the domain before creating any task.
- **`TaskUpdate` to `in_progress` before starting an item, to `completed` only when it is fully done.** A partial result, a failing test or an unresolved error leaves it `in_progress`; when something blocks it, create a task naming the blocker rather than closing the item.
- **Use `addBlockedBy` where the file records a dependency**, so a gating decision is visible as a blocked task instead of a buried sentence.
- **The file is updated at the same time, not afterwards.** A harness task closed without its `TODO.jsonl` line being struck or amended is progress that dies with the session. If the two disagree, the file is right and the list is stale.
- Skip it for a single trivial item, which the harness's own guidance also excludes; a two-step errand does not need a list.

## Plans

Every plan file includes a blast-radius section: artifacts by scope, what is untouched, and reversibility.

Non-trivial work gets a durable plan; for a high-stakes plan, run the crucible skill in red-vs-blue mode before implementation.

## Worktrees

All non-trivial work happens in a git worktree, never the main checkout. The primary clone stays clean on the default branch.

- Layout: primary clone `<project>/<project-name>/`, worktrees beside it at `<project>/worktrees/<branch-slug>/` (not nested inside the repo), shared content at `<project>/artifacts/`.
- Branch naming: a branch named after the issue, or `<descriptive-kebab>`. Slashes are fine and are slugged into the directory name.
- **Base a new worktree on the latest `origin/main` by default; fetch first and state the commit you branched from.** Do not ask which base, and do not take a base recorded in an issue, which is stale from when it was drafted. The one other base is a feature branch being actively worked on, which announces itself because the work is explicitly stacked on it. **Creating a worktree still needs the user's ask.** Only the base question is settled; whether to create one is not.
- **Validate in a worktree before filing a ticket.** Open a worktree, run and fix the actual script or component there, and only then file the ticket in the issue tracker and write the documentation. A local draft is welcome at any point, since it can be rewritten freely; a filed issue cannot be unfiled. Never file a ticket whose behaviour is still inferred from reading rather than from running.
