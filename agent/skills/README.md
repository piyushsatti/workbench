# skills

The global agent skills ([decisions](../../docs/decisions.md#what-each-piece-is)). `scripts/deploy_agent.py` copies each folder to `~/.agents/skills/<name>/`, which Codex and Cursor read, and to `~/.claude/skills/<name>/` for Claude Code. `tests/`, `bin/`, `runtime/` and `install.json` stay behind: executables never run from an agent-writable path, so a skill's `install.json` puts them in `~/.local/bin` (and `~/.local/share`) instead.

| Skill | Job | Invocation | Was |
| --- | --- | --- | --- |
| `crucible` | Review through lenses and stances: consult, audit, hats, red-vs-blue | On request | the crucible plugin (6 skills, 4 agents) |
| `dream` | Consolidate memory from Claude transcripts | On request | the dream plugin |
| `save-state` | Write a durable checkpoint node for a session | Explicit only | the save-state plugin |
| `humanizer` | Revise prose while keeping its facts and the writer's voice | On request | the workbench plugin |
| `grill-me` | Interview the user about a plan or decision | Explicit only | the workbench plugin |
| `research` | Investigate a question from primary sources and cite the findings | On request | the workbench plugin |
| `teach` | Build a stateful learning workspace with lessons and practice | Explicit only | the workbench plugin |

The global cap is 12 skills. Each folder's own README, where there is one, covers harness support, data and changes from its source.

## Rules

- `SKILL.md` frontmatter holds only `name`, `description` and `allowed-tools`, and `name` matches the folder. Claude Code's `disable-model-invocation` isn't portable, so an explicit-only skill says "Run only when the user asks for it by name." in its description, and Codex's `agents/openai.yaml` sets `allow_implicit_invocation: false`.
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
uv run python -B -m unittest discover -s agent/skills/tests
uv run python -B -m unittest discover -s agent/skills/dream/tests
```
