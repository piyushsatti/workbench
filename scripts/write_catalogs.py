#!/usr/bin/env python3
"""Rewrite the two `pi` marketplace catalogs at this repo's root (Claude Code and Codex).

The plugins live in their own repos next to this one (plugins.json). Catalog entries point at each repo
on GitHub, pinned by `sha`; one whose `sha` is still null is not pushed yet and is left out. An entry with `path` lives in this repo and is cataloged by that relative path. Cursor resolves catalog entries only inside the catalog's own repo,
so it has no `pi` catalog: Cursor users add each plugin repo directly (each ships a .cursor-plugin manifest).
A catalog lists a plugin only for the harnesses whose manifest the plugin ships (CONVENTIONS.md).

Dry run by default: prints what would change. `--apply` writes. lint_plugins.py checks the result.
"""

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import plugin_repos  # noqa: E402

ROOT = plugin_repos.ROOT
OWNER = {"name": "piyushsatti", "email": "49362334+piyushsatti@users.noreply.github.com"}
NOTE = "`pi` is unrelated to Pi Agent."
MANIFESTS = {"claude": ".claude-plugin/plugin.json", "codex": ".codex-plugin/plugin.json"}


def manifest(repo, harness):
    path = repo / MANIFESTS[harness]
    return json.loads(path.read_text()) if path.exists() else None


def pinned(entry, source):
    return {**source, "sha": entry["sha"]} if entry.get("sha") else source


def catalogs(root=ROOT, source=None):
    entries = {"claude": [], "codex": []}
    missing, unpushed = [], []
    for entry, repo in plugin_repos.repos(root, source):
        if "github" in entry and not entry.get("sha"):
            unpushed.append(entry["name"])
            continue
        if not repo.is_dir():
            missing.append(entry["name"])
            continue
        for harness in MANIFESTS:
            m = manifest(repo, harness)
            if m is None:
                continue
            base = {"name": m["name"], "description": m["description"], "version": m["version"]}
            local = f"./{entry['path']}" if "path" in entry else None
            if harness == "claude":
                source = local or pinned(entry, {"source": "github", "repo": entry["github"]})
                entries["claude"].append({**base, "source": source})
            else:
                url = f"https://github.com/{entry.get('github')}.git"
                source = {"source": "local", "path": local} if local else pinned(entry, {"source": "url", "url": url})
                entries["codex"].append({**base, "source": source,
                                         "category": m.get("interface", {}).get("category", "Productivity")})
    files = {
        ".claude-plugin/marketplace.json": {"name": "pi", "owner": OWNER, "metadata": {"description": f"Plugins for Claude Code. {NOTE}"}, "plugins": entries["claude"]},
        ".agents/plugins/marketplace.json": {"schemaVersion": 1, "name": "pi", "interface": {"displayName": "pi"}, "plugins": entries["codex"]},
    }
    return files, missing, unpushed


def main(argv=None):
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("--root", type=Path, default=ROOT, help="the workbench repo (default: this one)")
    parser.add_argument("--source", type=Path, help="the folder holding the plugin repos (default: ~/Source)")
    parser.add_argument("--apply", action="store_true", help="write the catalogs")
    args = parser.parse_args(argv)
    changed = 0
    files, missing, unpushed = catalogs(args.root, args.source)
    for name in unpushed:
        print(f"UNPUSHED     {name}: plugins.json has no sha yet; left out until it is pushed and pinned")
    for name in missing:
        print(f"MISSING      {name}: no repo at {plugin_repos.source_dir() if not args.source else args.source}/{name}; left out")
    for rel, value in files.items():
        path, text = args.root / rel, json.dumps(value, indent=2) + "\n"
        if path.exists() and path.read_text() == text:
            continue
        changed += 1
        print(f"{'WRITE' if args.apply else 'WOULD WRITE':12} {rel}  ({len(value['plugins'])} plugins)")
        if args.apply:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text)
    print("up to date" if not changed else f"{changed} change(s) {'made' if args.apply else 'pending; rerun with --apply'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
