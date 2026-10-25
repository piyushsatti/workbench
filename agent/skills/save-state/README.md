# save-state

Save State is the deliberate, manual checkpoint for a session that is finished, paused, handed off or about to be abandoned. It turns the useful parts of the visible session into a durable Markdown state node with provenance, worktree facts, verification evidence, open decisions and a resume path. The recipe is in [SKILL.md](SKILL.md), and the node shape with good and bad examples is in [references/examples.md](references/examples.md).

The skill doesn't change the worktree. It doesn't commit, stage, reset or delete a worktree, update canonical memory, or publish anywhere, and it doesn't claim a raw transcript was archived when only a summary was written. Each run writes one new node and never rewrites an older one.

It was the save-state plugin and is now a global skill in `agent/skills/` ([decisions](../../../docs/decisions.md#what-each-piece-is)).

## Harness support

This is one Agent Skill and nothing else: no hooks, MCP server, commands, agents or executables. The source claimed Codex, Claude Code and Pi Agent and declared nothing unsupported. Pi Agent isn't claimed here. Cursor is added because skills map to it natively.

| Harness | What works | Reduced | Missing |
|---|---|---|---|
| Claude Code | The `save-state` skill (`/save-state`), writing nodes to `~/.local/state/save-state/`. | Nothing. | Nothing. |
| Codex | The same skill, from `~/.agents/skills`. `agents/openai.yaml` keeps it explicit-only (`allow_implicit_invocation: false`) and gives the display name and default prompt. | Nothing. | Nothing. |
| Cursor | The same skill, from `~/.agents/skills`. | Cursor doesn't read `agents/openai.yaml`, so nothing marks the skill explicit-only there. Its description still says when to use it. | Nothing. |

Not verified in any live harness yet.

## Data: `~/.local/state/save-state/`

```text
~/.local/state/save-state/
└── YYYY-MM-DDTHH-mm-ssZ--<project-slug>--<topic-slug>.md
```

The skill folder holds only the skill. Nodes are written to `~/.local/state/save-state/`, with no `state/` subfolder. Node frontmatter records `state_root` as that path.

**Changed from the source.** The source wrote nodes to `$PI_HOME/save-state/state/`, falling back to the `.pi/save-state/state/` folder in your home directory when `PI_HOME` was unset. That folder belongs to Pi Agent ([decisions](../../../docs/decisions.md#plugins-and-marketplace)), so the port moves the data, and `PI_HOME` is no longer read.

**Migrating existing nodes.** Copy them across; the old folder is left as it is, because archiving `.pi/` needs its own go-ahead:

```sh
mkdir -p ~/.local/state/save-state
cp -Rp "$HOME"/.pi/save-state/state/. ~/.local/state/save-state/
```

If you had set `PI_HOME`, copy from `"$PI_HOME"/save-state/state/` instead. Old nodes keep their old `state_root` value in frontmatter; that is a historical record and needn't be rewritten. A new node can point at an old one with `prior_node`.

## Tests

None. The source shipped no tests and the skill has no executable code; `agent/skills/tests` checks its frontmatter.

## Licence

MIT, as the source.
