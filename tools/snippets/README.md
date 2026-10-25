# snippets

Prompt snippets: Markdown files whose text rides along with every prompt you send, until you turn them off. A `UserPromptSubmit` hook adds the snippets that are on as context next to the prompt. The prompt you typed is never changed, and slash commands go through without snippets.

It replaces the Claude mod of the same name, which had a picker pane and a footer label. Both of those came from the mod engine, so this version uses commands instead.

## Use

```bash
snippets list            # every snippet, [x] when on
snippets show verify     # one snippet's text
snippets on verify       # turn it on
snippets off --all       # turn everything off
```

Defaults ship in [library/](library/) and install to `~/.local/share/snippets/`:

| Snippet | Side | What it says |
|---|---|---|
| `verify` | after | check each claim against code or docs; say what couldn't be checked |
| `brief` | after | short reply, answer first, at most one question |
| `plan-first` | before | read first and say what you'll change; prefer existing solutions |
| `no-commit` | after | never git add, commit or push |
| `act-and-report` | after | do reversible steps yourself and report; ask only before one-way ones |

Nothing is on until you turn it on. Add your own as `~/.config/snippets/<slug>.md`; one with the same slug as a default replaces it. The format:

```markdown
---
title: Verify
description: check claims before stating them
position: append          # or prepend; default append
---
Check each claim against the code or the docs before you state it.
```

Which snippets are on is kept in `~/.local/state/snippets/enabled.json`. `SNIPPETS_LIBRARY`, `SNIPPETS_DIR` and `SNIPPETS_STATE` override the three paths; the tests use them.

## Harnesses

| Harness | Works | How |
|---|---|---|
| Claude Code | yes | merge [hook.json](hook.json) into `~/.claude/settings.json` |
| Codex | yes | merge [hook.json](hook.json) into `~/.codex/hooks.json`, then trust it once in `/hooks` |
| Cursor | no | `beforeSubmitPrompt` can only allow or block a prompt, not add context. Cursor imports Claude's hooks by default and runs this one there too; it adds nothing |

Both harnesses take the same output: `hookSpecificOutput.additionalContext`. Claude Code shows only a preview of context over 10,000 characters, and `snippets list` warns when the snippets that are on go over that. The hook never blocks a prompt: a bad payload, a broken state file or an unreadable snippet means it adds nothing.

## Install

`scripts/deploy_agent.py` copies `snippets.py` to `~/.local/bin/snippets` and the library to `~/.local/share/snippets/` (from [install.json](install.json)), outside agent-writable paths. Add a library snippet to `install.json` too; a test checks that. Merging the hook config is done by hand.

## Test

```bash
uv run python -B -m unittest discover -s tools/snippets/tests
```
