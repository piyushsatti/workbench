# Native plugin conventions

Every `pi` plugin repo (`~/Source/<name>`, listed in `plugins.json`) follows this layout. `scripts/lint_plugins.py` checks it (V0 on the [verification ladder](#verification-ladder)). The decisions behind it are in [decisions.md](docs/decisions.md#plugins-and-marketplace).

## Layout

```
<name>/
├── .claude-plugin/plugin.json    present if the plugin claims Claude Code
├── .codex-plugin/plugin.json     present if it claims Codex
├── .cursor-plugin/plugin.json    present if it claims Cursor
├── README.md                     what it does + a "Harness support" table
├── VERIFIED.md                   V0–V5 evidence, one row per run
├── install.json                  executables to copy outside the checkout (optional)
├── skills/<skill>/SKILL.md       Agent Skills; read by all three
├── commands/<cmd>.md             slash commands; Claude Code and Cursor only
├── agents/<agent>.md             subagents; Claude Code and Cursor only
├── hooks/claude.json             one hook config per harness, each named by its manifest
├── hooks/codex.json
├── hooks/cursor.json
├── .mcp.json · mcp.json          identical; Claude/Codex read the first, Cursor the second
├── bin/                          sources of executables named in install.json
└── tests/                        the plugin's own tests
```

## Rules

1. **Identity.** `name` is the folder name, the same in every manifest. The marketplace is `pi`, so the plugin installs as `<name>@pi`. Existing installed names are kept, and new ones need the maintainer's exact value.
2. **Version.** CalVer `YYYY.MM.DD`, identical in every manifest and in each catalog entry.
3. **A plugin claims a harness by shipping its manifest.** The README's "Harness support" table says, for each claimed harness, what works, what is reduced and what is missing.
4. **No `hooks/hooks.json`.** All three harnesses load that path automatically, and their formats differ, so each manifest names its own file in `"hooks"`.
5. **Executables run from outside agent-writable paths.** Hook and MCP commands call the installed copy, never a script inside the plugin folder. Write them as `sh -c 'exec "$HOME/.local/bin/om" hook claude Stop'`, so `$HOME` expands whether or not the harness runs commands through a shell (Codex's and Cursor's behaviour is undocumented). `install.json` lists what gets copied there:

   ```json
   {"files": [{"src": "bin/om", "dest": "~/.local/bin/om", "mode": "0755"}]}
   ```

   Because nothing depends on `${CLAUDE_PLUGIN_ROOT}`, `${PLUGIN_ROOT}` or `${CURSOR_PLUGIN_ROOT}`, `.mcp.json` and `mcp.json` are byte-identical.
6. **Data** lives in `~/.local/state/<name>/`. Never `~/.pi/` (that's Pi Agent's), never the plugin folder, and never the project.
7. **Skills** sit exactly one level under `skills/`, with frontmatter holding only `name` and `description` (plus `allowed-tools` where needed). No harness-specific keys.
8. **No machine paths, no secrets, no company content.** `$HOME` and `~` are fine; `/Users/<x>` and `/home/<x>` are not, except obvious test placeholders such as `/home/u` or `/Users/me`.

## Manifest fields

The field names come from each harness's documentation, as recorded in [harness-parity](agent/docs/harness-parity.md). Fields marked *unverified* still need a V1 or V3 check.

| Field | Claude `.claude-plugin/plugin.json` | Codex `.codex-plugin/plugin.json` | Cursor `.cursor-plugin/plugin.json` |
|---|---|---|---|
| `name`, `version`, `description`, `author`, `license` | yes | yes | yes |
| `hooks` | `"./hooks/claude.json"` | `"./hooks/codex.json"` *(unverified)* | `"./hooks/cursor.json"` |
| `mcpServers` | `"./.mcp.json"` | `"./.mcp.json"` | `"./mcp.json"` |
| `skills` | auto-discovered from `skills/` | `["./skills"]` (as installed Codex plugins use) | `"./skills/"` |
| `commands`, `agents` | auto-discovered | not supported | auto-discovered |
| `interface` | — | `displayName`, `shortDescription`, `developerName`, `category`, `capabilities` | — |

## Catalogs (workbench root)

- `.claude-plugin/marketplace.json`: `{"name": "pi", "owner": {...}, "plugins": [{"name", "description", "version", "source": {"source": "github", "repo": "<owner>/<name>", "sha"?}}]}`
- `.agents/plugins/marketplace.json` (Codex): `{"schemaVersion": 1, "name": "pi", "plugins": [{"name", "description", "version", "source": {"source": "url", "url": "https://github.com/<owner>/<name>.git", "sha"?}, "category"}]}`
- A plugin hosted in workbench (`plugins.json` entry with `path`) is cataloged by that path: `"source": "./<path>"` for Claude Code, `{"source": "local", "path": "./<path>"}` for Codex.
- Cursor: no catalog. Cursor resolves catalog entries only inside the catalog's own repo, so each plugin repo is added to Cursor directly through its `.cursor-plugin/plugin.json`.

A catalog lists a plugin only if that plugin ships the harness's manifest. `scripts/write_catalogs.py --apply` rewrites both from the manifests, and `scripts/lint_plugins.py` checks them. The manifests are the source of truth.

## Verification ladder

Each plugin climbs these levels in each harness it claims. A level is recorded only when its check has actually run. The evidence goes in the plugin's `VERIFIED.md`: date, machine, harness version, command and result.

| Level | What it proves | Claude Code | Codex | Cursor |
|---|---|---|---|---|
| **V0 lint** | The files agree with each other | `scripts/lint_plugins.py` | same | same |
| **V1 native validator** | The harness accepts the manifest | `claude plugin validate <dir>` (2.1.307 has it) | none found locally; lint checks the documented schema | none found; lint checks the documented schema |
| **V2 script tests** | Hook scripts behave on each harness's input shape | fixture stdin per event, in CI-style tests | same | same |
| **V3 loads without install** | The harness discovers it in a throwaway session | `claude -p --plugin-dir ~/Source/<x>`: session only, no settings written | `codex exec --ephemeral -p <test-profile>`; the profile file is a config change, so it needs a go-ahead | no CLI; check in the app, or install `cursor-agent` (go-ahead) |
| **V4 runtime behaviour** | It does its job | plugin-specific checks | same; hooks must first be trusted in `/hooks` (interactive, so a human does it) | same, manual |
| **V5 in use** | It works day to day | installed on a machine and used for a day | same | same |

Rules:
- **Fixtures are synthetic,** built from each harness's documented hook input. Real transcripts on disk may be read in local test runs, but they're never copied into the repository.
- **A harness counts as supported at V4,** and a plugin counts as moved at V4 in every harness it claims. Until then the old install stays.
- **An agent can run V0 to V4 for Claude locally.** A V3 or V4 run is a real `claude -p` session, so it spends tokens. For Codex and Cursor, V3 and up need the maintainer's go-ahead or hands.
