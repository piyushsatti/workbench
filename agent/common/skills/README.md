# skills

The global agent skills ([decisions](../../../docs/decisions.md#what-each-piece-is)). `scripts/deploy_agent.py` copies each folder to `~/.agents/skills/<name>/`, which Codex and Cursor read, and to `~/.claude/skills/<name>/` for Claude Code. `tests/`, `bin/`, `runtime/` and `install.json` stay behind: executables never run from an agent-writable path, so a skill's `install.json` puts them in `~/.local/bin` (and `~/.local/share`) instead.

| Skill | Job | Invocation | Was |
| --- | --- | --- | --- |
| `crucible` | Review through lenses and stances: consult, audit, hats, red-vs-blue | On request | the crucible plugin (6 skills, 4 agents) |
| `dream` | Consolidate memory from Claude transcripts | On request | the dream plugin |
| `save-state` | Write a durable checkpoint node for a session | Explicit only | the save-state plugin |
| `humanizer` | Revise prose while keeping its facts and the writer's voice | On request | the workbench plugin |
| `grill-me` | Interview the user about a plan or decision | Explicit only | the workbench plugin |
| `research` | Investigate a question from primary sources and cite the findings | On request | the workbench plugin |
| `teach` | Build a stateful learning workspace with lessons and practice | Explicit only | the workbench plugin |
| `notion-page-design` | Build or reorganise Notion hubs, databases and review queues so they scan at a glance | On request | `~/.claude/skills` only; example names made generic |
| `social-posts` | Draft LinkedIn and X posts that promote a write-up, in the author's voice rules | On request | `~/.claude/skills` only |
| `whiteboard-map` | Turn a pile of information into an Excalidraw+ board, one frame per subject | On request | `~/.claude/skills` only; new generic example spec, "Notion/Relay" became "a doc" |

The global cap is 12 skills; Claude Code's `refine` (in [`agent/claude`](../../claude/README.md)) makes 11. Each folder's own README, where there is one, covers harness support, data and changes from its source.

## Rules

- `SKILL.md` frontmatter holds only `name`, `description` and `allowed-tools`, and `name` matches the folder. Claude Code's `disable-model-invocation` isn't portable, so an explicit-only skill says "Run only when the user asks for it by name." in its description, and Codex's `agents/openai.yaml` sets `allow_implicit_invocation: false`.
- `whiteboard-map` runs `scripts/build_board.py` with `python3` from its own folder and needs the `excalidraw` CLI, signed in, for render and upload.
- `teach` writes `MISSION.md`, `RESOURCES.md`, `lessons/`, `reference/`, `learning-records/`, `assets/` and `NOTES.md` into the working directory, so start it from a dedicated learning workspace. `research` writes one cited Markdown file in the repository.

## Third-party sources

| Source | Pin | Skills | Local adjustment |
| --- | --- | --- | --- |
| [`blader/humanizer`](https://github.com/blader/humanizer) | `9862685f575c65a8247f90369951df1b3416e3d6` (`v3.0.0`) | `humanizer` | The upstream frontmatter's `license` and `metadata.version` keys became a comment at the top of the body. |
| [`mattpocock/skills`](https://github.com/mattpocock/skills) | `6acc160e4e0cd062dbbbd7a1b26ae92855edf07e` (`v1.2.3`) | `grill-me`, `research`, `teach` | Grill Me reads the bundled Grilling procedure (`grill-me/references/GRILLING.md`) instead of calling a host-specific skill command. Research works in the current session when background agents are unavailable. Grill Me and Teach dropped `disable-model-invocation`. |

Each imported skill keeps its upstream MIT licence as `LICENSE` in its folder. `save-state` is MIT, as its source.

## Tests

From the repo root:

```
uv run python -B -m unittest discover -s agent/common/skills/tests
uv run python -B -m unittest discover -s agent/common/skills/dream/tests
```
