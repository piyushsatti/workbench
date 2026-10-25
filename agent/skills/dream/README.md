# dream

Consolidates an agent's memory the way sleep consolidates the day. It replays only the transcript lines added since the last dream, extracts what mattered and what was left open, weighs it, and turns episodes into rules. Each fact is filed where it belongs and merged with what is already stored across every project. What has gone stale is demoted, and then recall is tested. The pipeline itself is in [SKILL.md](SKILL.md).

It was the dream plugin; it is now a global skill in `agent/skills/` ([decisions](../../../docs/decisions.md#what-each-piece-is)).

## Harness support

Dream's scripts read Claude Code's transcripts (`~/.claude/projects/<key>/*.jsonl`) and its memory stores (`~/.claude/projects/<key>/memory/`). The other two harnesses keep both somewhere else and in other formats, so from those harnesses dream can only consolidate the Claude data on the same machine.

| Harness | What works | Reduced | Missing |
|---|---|---|---|
| Claude Code | The `dream` skill (`/dream`) and all four executables, over Claude's transcripts and memory. Stages 2 and 10 dispatch Haiku and Sonnet subagents. | Nothing. | Nothing. |
| Codex | The skill loads as an Agent Skill. The executables run, but only over Claude's stores on the same machine. | The subagent model names (Haiku, Sonnet) are Claude's, so Codex has to pick its own equivalents. | **Claude-only: Codex's own data is not read.** Codex writes rollouts to `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`, with `session_meta`, `response_item` and `event_msg` records. `dream-replay` keeps only Claude's `user`/`assistant` records, so it writes no digests for them. Codex's memory is `~/.codex/memories/`: `MEMORY.md` grouped by task, plus `memory_summary.md`, `raw_memories.md` and `rollout_summaries/`. That isn't the frontmatter-per-file store that `dream-lint-memory` and `dream-catalog` check. |
| Cursor | The skill loads from `~/.agents/skills`. The executables run, but only over Claude's stores on the same machine. | Same model-name caveat as Codex. | **Claude-only: Cursor's own data is not read.** Cursor writes agent transcripts to `~/.cursor/projects/<key>/agent-transcripts/<id>/<id>.jsonl` as `{role, message}` records, with no `type`, `uuid` or `timestamp`. `dream-replay` skips them all. No file-based memory store was found under `~/.cursor/`, and Cursor's project rules live in `.cursor/rules/`. |

The Codex and Cursor locations above were observed locally. They haven't been checked against either harness's documentation. Supporting either one means adding a reader to `dream-replay` for that transcript shape, and deciding what "the store" is for that harness.

## Executables

`install.json` copies these to `~/.local/bin/`. The skill calls them by those names and never from the skill folder.

| Command | Does |
|---|---|
| `dream-replay run <transcripts> <state> <out> [--since YYYY-MM-DD]` | Stage 1. Writes one digest per session from the lines past the watermark, and stages the new watermark and `recall.tsv`. |
| `dream-replay commit <state>` | Stage 11. Makes the staged watermark current. |
| `dream-catalog <projects dir> [recall.tsv ...]` | Prints a TSV of every memory file in every store, with tier, dates and a `stale` flag (60 days). |
| `dream-lint-memory <memory dir>` | Stage 10 lint: frontmatter, filenames, index lines, `[[links]]` and secrets. Exits 1 on any ERROR. |
| `dream-config [--space P] [--key K] [--date D] [--check]` | Prints the settings below merged over the defaults, as JSON. `--check` lists configured paths that don't exist and exits 1 if there are any. |

They need Python 3.11 or newer, and they run isolated (`python3 -I`) through their shebang.

## Settings: `~/.local/state/dream/config.json`

Machine-specific routes live in this file, not in the skill. To start, copy [config.example.json](config.example.json) there and replace each `~/notes/...` placeholder. `DREAM_CONFIG=<path>` points at another file, which the tests use.

Every key is optional. A missing file means `"configured": false`. The skill then reads no rulebooks and files only into memory, and it lists everything else as unrouted.

| Key | Type | Default | Used for |
|---|---|---|---|
| `read_first` | list of `{path, for}` | `[]` | Files read before starting, each for the reason in `for`. They win over the skill. A glob reads its newest match. |
| `rulebooks_dir` | path | `""` | Where rulebook drafts are proposed, and what stage 6 greps for an existing rule. |
| `locate_hint` | shell command | `""` | Run when `--check` reports a missing path, to find where the knowledge base went. |
| `routes` | list of `{item, to, then?}` | `[]` | Machine-specific rows of the stage 5 routing table: what kind of item, where it goes, and what happens to it later. |
| `open_loop_tracker` | text | `""` | How a kept open loop becomes a task. Empty keeps it in the report. |
| `trash_command` | command | `trash-put` | Moves retired or replaced memory files to the trash. |
| `state_dir` | path template | `~/.local/state/dream/<key>/state` | `<state>`: the watermark, `seen.txt`, `recall.tsv` and `retired.tsv`. |
| `run_dir` | path template | `~/.local/state/dream/<key>/runs/<date>-dream` | `<run>`: digests, candidates, `plan.md`, `catalog.tsv` and `report.md`. |

`~` is expanded everywhere. `<space>` (the project space path), `<key>` and `<date>` are filled in the two templates.

**Moving state.** The skill used to keep `<state>` at `<space>/artifacts/dream/state/` and runs at `<space>/artifacts/<date>-dream/`. The default now follows the plugin rule that data lives in `~/.local/state/<name>/`. To keep an existing watermark, either copy the old `state/` folder to the new `state_dir`, or set `state_dir` to `<space>/artifacts/dream/state` (and `run_dir` to `<space>/artifacts/<date>-dream`) in the config. Without one of these, the first run replays from the `--since` floor.

## Tests

```
python3 -m unittest discover -s tests
```

Run this from this folder. The tests use synthetic transcripts and stores under a temporary `HOME` and never read real data.
