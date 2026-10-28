#!/usr/bin/env python3
"""Prompt snippets: Markdown files whose text rides along with every prompt you send.

A snippet is a Markdown file, optionally with frontmatter. Defaults ship in ~/.local/share/snippets/;
your own go in ~/.config/snippets/, and one of yours wins over a default with the same name:

    ---
    title: Verify
    description: Do not assume
    position: append        # or prepend; default append
    ---
    Check each claim.

Turn snippets on and off here; the UserPromptSubmit hook adds the ones that are on as context next to
the prompt (Claude Code and Codex). The prompt text itself is never changed. Slash commands go through
without snippets. Cursor's beforeSubmitPrompt can't add context, so Cursor gets nothing.

Usage:
    snippets list                  every snippet, [x] when on
    snippets show SLUG             one snippet's text
    snippets on SLUG...            turn snippets on
    snippets off SLUG... | --all   turn snippets off
    snippets hook                  the UserPromptSubmit hook (stdin: the hook payload)

Folders: $SNIPPETS_LIBRARY (default ~/.local/share/snippets), $SNIPPETS_DIR (default ~/.config/snippets)
and $SNIPPETS_STATE (default ~/.local/state/snippets/enabled.json). Standard library only.
"""

import json
import os
import re
import sys
from pathlib import Path

FRONTMATTER = re.compile(r"^---\r?\n(.*?)\r?\n---\r?\n?(.*)$", re.S)
# Claude Code saves context over 10,000 characters to a file and shows only a preview.
CONTEXT_LIMIT = 10_000


def library_dir():
    return Path(os.environ.get("SNIPPETS_LIBRARY") or Path.home() / ".local/share/snippets")


def snippets_dir():
    return Path(os.environ.get("SNIPPETS_DIR") or Path.home() / ".config/snippets")


def state_file():
    return Path(os.environ.get("SNIPPETS_STATE") or Path.home() / ".local/state/snippets/enabled.json")


def parse(slug, source):
    """A snippet dict from a file's text. Unknown keys are ignored; no frontmatter means all body."""
    m = FRONTMATTER.match(source)
    meta = {}
    for line in (m.group(1) if m else "").splitlines():
        kv = re.match(r"^(\w+):\s*(.*)$", line)
        if kv:
            # A YAML comment starts at a '#' after whitespace; "C# tips" keeps its '#'.
            meta[kv.group(1)] = re.sub(r"\s+#.*$", "", kv.group(2)).strip()
    return {
        "slug": slug,
        "title": meta.get("title") or slug,
        "description": meta.get("description", ""),
        "position": "prepend" if meta.get("position") == "prepend" else "append",
        "body": (m.group(2) if m else source).strip(),
    }


def catalog():
    """Every readable snippet, prepends first, then by slug. Your folder wins over the library."""
    found = {}
    for folder in (library_dir(), snippets_dir()):
        for path in sorted(folder.glob("*.md")) if folder.is_dir() else []:
            try:
                found[path.stem] = parse(path.stem, path.read_text(encoding="utf-8"))
            except (OSError, UnicodeDecodeError):
                continue
    return sorted(sorted(found.values(), key=lambda s: s["slug"]), key=lambda s: s["position"] != "prepend")


def enabled():
    try:
        data = json.loads(state_file().read_text())
        return [s for s in data if isinstance(s, str)] if isinstance(data, list) else []
    except (OSError, ValueError):
        return []


def save(slugs):
    path = state_file()
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(sorted(set(slugs))) + "\n")
    os.replace(tmp, path)


def context_for(snippets, slugs):
    """The context strings for the snippets that are on, prepends first."""
    on = [s for s in snippets if s["slug"] in slugs]
    return [f"Read this before the message above:\n\n{s['body']}" for s in on if s["position"] == "prepend"] + [
        f"Apply this to the message above:\n\n{s['body']}" for s in on if s["position"] == "append"
    ]


def hook(stdin):
    """Hook output for one payload, or None for nothing. Never blocks the prompt."""
    try:
        prompt = json.loads(stdin or "{}").get("prompt", "")
    except (ValueError, AttributeError):
        return None
    # A slash command is one /word (with : . - allowed) then a space or the end; a pasted path such as
    # /Users/me/app/log.txt has a second / and is an ordinary prompt.
    if not isinstance(prompt, str) or re.match(r"/[\w:.-]+(\s|$)", prompt.lstrip()):
        return None
    extra = context_for(catalog(), enabled())
    if not extra:
        return None
    return {"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": "\n\n".join(extra)}}


def main(argv):
    cmd, args = (argv[0], argv[1:]) if argv else ("list", [])
    if cmd == "hook":
        try:
            out = hook(sys.stdin.read())
        except Exception:  # a broken snippet must never stop a prompt
            out = None
        if out:
            print(json.dumps(out))
        return 0

    snippets = {s["slug"]: s for s in catalog()}
    on = enabled()
    if cmd == "list":
        if not snippets:
            print(f"No snippets in {library_dir()}/ or {snippets_dir()}/ yet.")
        for s in snippets.values():
            mark = "[x]" if s["slug"] in on else "[ ]"
            side = "before" if s["position"] == "prepend" else "after"
            desc = f" — {s['description']}" if s["description"] else ""
            print(f"{mark} {s['slug']:<16} {side:<6} {s['title']}{desc}")
        size = len("\n\n".join(context_for(list(snippets.values()), on)))
        if size > CONTEXT_LIMIT:
            print(f"warning: the snippets that are on add {size} characters; Claude Code shows only a preview over {CONTEXT_LIMIT}")
        return 0
    if cmd == "show" and len(args) == 1:
        if args[0] not in snippets:
            print(f"snippets: no snippet {args[0]!r}; see snippets list", file=sys.stderr)
            return 1
        print(snippets[args[0]]["body"])
        return 0
    if cmd == "on" and args:
        missing = [a for a in args if a not in snippets]
        if missing:
            print(f"snippets: no snippet {', '.join(missing)}; see snippets list", file=sys.stderr)
            return 1
        save(on + args)
        return 0
    if cmd == "off" and args:
        save([] if args == ["--all"] else [s for s in on if s not in args])
        return 0
    print((__doc__ or "").split("Usage:")[1].split("Folders:")[0].rstrip(), file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
