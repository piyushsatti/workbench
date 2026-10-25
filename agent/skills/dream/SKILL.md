---
name: dream
description: |
  Consolidate a project's Claude memory the way sleep consolidates the day: replay only the
  transcript lines added since the last dream, extract what mattered and what was left open,
  weigh it, turn episodes into rules, file each fact where it belongs, merge with what is
  stored across every project, demote what has gone stale, then test recall. Use when asked to
  "dream", consolidate or audit memories, or mine transcripts for learnings. The argument `validate`
  runs only stage 10 against the current store.
---

# Dream: consolidate memory from transcripts

Work from the **project space**, never a clone or worktree: only the space's store is durable. The store is `~/.claude/projects/<key>/memory/`, where `<key>` is the space path with `/` turned into `-`. Transcripts are the `*.jsonl` files beside it, with subagent transcripts under `<session>/subagents/`. The scripts read Claude Code's transcript and memory format only; from another harness, dream still works on the Claude stores on this machine and on nothing else.

## Settings

Everything specific to this machine (the rulebooks, the knowledge-base routes, where state goes) lives in `~/.local/state/dream/config.json`, never in this skill. Load it first:

```bash
dream-config --space <space> --key <key> --check
```

It prints the settings as JSON, with `~`, `<space>`, `<key>` and today's `<date>` filled in, and lists any configured path that is missing on stderr.

- `"configured": false`: there is no config file. Tell the user once that the dream skill folder ships `config.example.json` to copy there. Until it exists, read no rulebooks, file only into memory (stage 5's memory rows), and keep every other item in the end-of-run table as unrouted.
- `read_first`: read each path before starting, for the reason its `for` gives. These win over this skill where they differ. For a glob, read the newest match.
- `rulebooks_dir`: where rulebook drafts go and what stage 6 greps.
- `routes`: the machine's own rows for the stage 5 table.
- `open_loop_tracker`: how a kept open loop becomes a task. Empty means the loop stays in the report.
- `trash_command`: the command that moves a file to the trash.
- `state_dir` and `run_dir`: `<state>` and `<run>` below.

If a configured path has moved, `--check` reports it as missing. Then run `locate_hint` when it is set, and give the user the corrected value for `config.json`. Don't edit the config yourself.

Go one stage at a time and report after each, unless the user asked for the whole run.

## The pipeline

Every stage reads files and writes files, so a failed run restarts from the stage that failed. Only stages 2 and 3 use a model.

| Stage | Who | Reads | Writes |
| --- | --- | --- | --- |
| 1. Replay | `dream-replay` | new transcript lines | `digests/`, staged watermark |
| 2. Extract | Haiku, one subagent per batch | `digests/` | `candidates/*.jsonl` |
| 3. Weigh and tier | this session (Sonnet medium if delegated) | candidates, `catalog.tsv` | `plan.md` |
| 4–8. Generalize to recombine | this session | `plan.md`, the store, rulebooks | `plan.md` |
| 9. Load | this session, Write and Edit | `plan.md` | the store, gated drafts |
| 10. Validate | lint script and two subagents | the store | `report.md` |
| 11. Commit | `dream-replay commit` | staged watermark | `state/` |

Paths: `<state>` is the settings' `state_dir` and persists between runs. `<run>` is the settings' `run_dir`, one per run. Provenance never goes in `memory/`.

## 1. Replay

```bash
dream-replay run ~/.claude/projects/<key> <state> <run>/digests
```

On the first run in a space, add `--since <YYYY-MM-DD>`: the date of the last dream if one is recorded, else 30 days ago. The floor is stored and applies to every later run.

The script reads only the lines added since the last commit, drops duplicate records copied in by resume or fork, strips harness wrappers, and keeps subagent reports as conversation text. Each tool call becomes one line (`[Bash] Run the lint → ok`), and an error keeps its first two lines. A digest ends with the open loops the session's last reply listed. Subagent transcripts contribute only their tool errors. Every read or edit of a memory file goes to `<state>/recall.tsv`.

Report the stats line it prints. If it wrote no digests, stop: there is nothing to dream about.

## 2. Extract

Batch the digests at 100–150 KB and give each batch to its own subagent (Haiku, the dispatch default; use Sonnet at low effort if a batch comes back thin or wrong). It writes `<run>/candidates/<batch>.jsonl` and returns only the path and a count. One JSON object per line:

- `kind`: preference, correction, project fact, reference, library gotcha, environment, **open loop** or **recall check**;
- `title`, `fact`, `date` (absolute), `evidence` (a short quote), `session`;
- `signal`: correction, repeated, explicit ("remember", "always", "never", "from now on"), decision, or error.

Brief it on the two kinds a plain read misses:
- **Open loops.** Start from each digest's "Open loops" section, then add what the user deferred in their own words ("later", "next time", "remind me", "after X") and questions that were never answered. Drop a loop that later text in the batch shows was closed.
- **Recall checks.** Give it the rows of `recall.tsv` for its sessions. For each memory a session read, say whether the session confirmed it, contradicted it (quote the evidence), or never tested it.

## 3. Weigh and tier

Score each candidate on two axes and multiply:
- **Recurrence:** 1 for one session; 2 for two sessions; 3 for three or more, or an explicit "always/never".
- **Cost of a miss:** 1 if cosmetic; 2 if it costs rework; 3 if it caused a correction, visible frustration, data loss or a security risk.

| Score | Tier |
| --- | --- |
| 4–9 | **index**: a file in `memory/` with a line in `MEMORY.md` |
| 2–3 | **archive**: a file in `memory/archive/`, searched when needed, never loaded |
| 1 | drop |

Apply the save gate to every survivor: is it worth recording, and will it come up again? Drop any candidate that matches a row of `<state>/retired.tsv`: it was retired once, and the old transcript does not revive it.

Open loops skip scoring. Each one goes to the end-of-run table for the user to keep or drop.

Build the cross-project view once per run:

```bash
dream-catalog ~/.claude/projects <state>/recall.tsv > <run>/catalog.tsv
```

Write the survivors, with their scores and tiers, to `<run>/plan.md`.

## 4. Generalize

Write each survivor as a rule or fact, then `**Why:**` (the incident in one line, with its absolute date), then `**How to apply:**`. Leave the story in the transcript. Use they/them for anyone whose pronouns aren't stated.

Write the `MEMORY.md` hook so that the line alone is enough to act on: sessions load the index every time and open a memory's body only now and then.

## 5. Route

File each item by the filing rules in `read_first`. Don't decide by habit. These rows hold everywhere:

| Item | Goes to |
| --- | --- |
| Always-apply behaviour that holds in every project | a rulebook in `rulebooks_dir`: draft the text, show it, and write nothing until the user approves |
| A rule bound to one project, by the user's choice | that project's memory |
| A recalled fact, a person, a pointer, project state | memory |
| An open loop the user keeps | `open_loop_tracker`, after approval; the report when it is empty |
| Something the repo already records | nowhere |

Every other kind of item goes by the settings' `routes`: each row's `item` says what it takes and `to` where it goes, and `then`, when present, says what happens to it later. An item that no route takes, and that isn't memory, goes to the end-of-run table as unrouted. Don't invent a destination.

Never store secrets. A host name and a role name are fine; a password is not.

## 6. Integrate

Compare every item with the existing memories in both tiers, the rulebooks (grep `rulebooks_dir`), the repo docs and `catalog.tsv`. Give it one verdict:
- **NEW**: a new file in the tier stage 3 chose.
- **FIX**: correct an existing file.
- **CONFIRM**: a recall check confirmed it; set `last_confirmed` to the session's date.
- **MERGE**: fold it into another file.
- **PROMOTE**: draft it into a rulebook. Propose this when the catalog shows the same rule in two or more other stores.
- **PROMOTED**: a rulebook already holds it.
- **MOVE**: it belongs in another store.
- **DEMOTE**: move it from the index to `memory/archive/`.
- **RETIRE**: it is no longer true or useful.
- **KEEP**: no change.

Extending an existing file beats adding one. Link related memories with `[[name]]`, across tiers.

## 7. Prune and settle

- Check every named file, branch, path and number against the live state before keeping it.
- A recall check that found a contradiction makes the memory a FIX or a RETIRE. Quote the evidence.
- If two memories conflict, the newer dated one wins. CLAUDE.md and the rulebooks beat memory.
- A conflict you can't settle goes to the user as a question; don't write either side.
- Rows marked `stale` in `catalog.tsv` for this store are DEMOTE by default. Show them as a table with the default, and ask for "approve" or "except N".
- A RETIRE adds a row to `<state>/retired.tsv`: date, file, the claim in one line, and why. The file itself goes to the trash with `trash_command`, with the exact list shown first. If the sandbox blocks writes in `~/.claude/projects`, give the user one `!` command.

## 8. Recombine

Group the rules by the principle behind them. When several items share one principle, write it once and keep the specifics as bullets under it. Use `catalog.tsv` to look across projects as well as inside this one. List the gaps: corrections that came up more than once and that no memory or rule covers.

## 9. Load

Write the plan into the store. Rulebook drafts, retirements, demotions and tracker tasks wait for the user's approval.

- **Filename:** `<created>-<type>_<slug>.md`, where `<created>` is the date the fact was first recorded.
- **Frontmatter:** `name` (the slug), `description` (one line, used for recall), `created`, `last_updated`, `last_confirmed` (when set), `type` (user, feedback, project or reference), `scope` (`{depth: N, role: project}`), `genericity` (`specific` when unsure), `schema_version: 2`, and `metadata: {type: ...}`.
- **Tiers:** index files sit in `memory/` and each has one `MEMORY.md` line, `- [Title](file.md) — hook`. Archive files sit in `memory/archive/`, with the same frontmatter and no index line. No content goes in `MEMORY.md`.
- Write files with Write and Edit. Bash usually cannot write in `~/.claude/projects`.
- A harness hook may move the rulebook fields under `metadata:` and add `originSessionId` after a write. That is fine, and the lint accepts both places.
- A renamed, merged or demoted file is written as a new file. Then hand the user one `trash_command` command for the old files, after `ls` shows the exact list.

## 10. Validate

Run all three checks, then report each one's result.

1. **Lint.** Run `dream-lint-memory <memory dir>` and fix every ERROR. Fix each WARN too, or say why it stays.
2. **Recall test.** A fresh Haiku subagent gets only `MEMORY.md` and 10–15 situations written from the index memories without their wording (for example "about to add an index to a column"). It names the file for each situation. A miss means the hook needs rewriting. Rewrite it and test again.
3. **Quality review.** A Sonnet subagent reads each changed file next to its sources (candidates, digests, live repo state) and checks it for:
   - accuracy, absolute dates and no stale claims;
   - Why and How present;
   - the right store under stage 5 and the right tier under stage 3, and no duplicate of a rulebook rule;
   - no secrets.

   It returns a pass or fail per file, with the reason.

Write the results to `<run>/report.md`.

## 11. Commit

Only after stage 10 passes:

```bash
dream-replay commit <state>
```

Until then the next run replays the same lines, so a failed run loses nothing.

Finish with a table: file, verdict, tier, and check results. Then list the open loops for the user to keep or drop, the open questions, and every gated item still waiting on the user (rulebook drafts, demotions, retirements, deletes, tracker tasks).
