#!/usr/bin/env python3
"""Check the pi plugins against CONVENTIONS.md (V0 on the verification ladder).

The plugins are separate repos next to this one, listed in plugins.json; the Claude Code and Codex
catalogs sit at this repo's root. Cursor has no pi catalog; its per-plugin manifest is still checked.
A check, never a build: it reads files and reports problems, and writes nothing.
Exit status 1 when any plugin or catalog has an error; warnings alone exit 0.
"""

import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import plugin_repos  # noqa: E402

ROOT = plugin_repos.ROOT

MANIFESTS = {
    "claude": ".claude-plugin/plugin.json",
    "codex": ".codex-plugin/plugin.json",
    "cursor": ".cursor-plugin/plugin.json",
}
CATALOGS = {
    "claude": ".claude-plugin/marketplace.json",
    "codex": ".agents/plugins/marketplace.json",
}
HOOK_FILES = {"claude": "./hooks/claude.json", "codex": "./hooks/codex.json", "cursor": "./hooks/cursor.json"}
MCP_FILES = {"claude": "./.mcp.json", "codex": "./.mcp.json", "cursor": "./mcp.json"}
SHARED_KEYS = ("name", "version", "description")
MARKETPLACE = "pi"
CALVER = re.compile(r"^\d{4}\.\d{2}\.\d{2}$")
# Real home directories; obvious test placeholders (/home/u, /Users/me, /home/user …) are allowed.
MACHINE_PATH = re.compile(r"/(Users|home)/(?!(?:u|me|user|test|example|someone-else)\b)[A-Za-z0-9._-]+")
# Installed executables live outside agent-writable paths (CONVENTIONS rule 5).
INSTALLED = re.compile(r"(?:\$HOME|~)/(?:\.local/bin|\.tmux/claude)/[A-Za-z0-9._-]+")
PLUGIN_ROOT_VAR = re.compile(r"\$\{?(CLAUDE_PLUGIN_ROOT|PLUGIN_ROOT|CURSOR_PLUGIN_ROOT)\}?")
SECRET = re.compile(r"(sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|xox[bp]-[A-Za-z0-9-]{10,})")
TEXT_SUFFIXES = {".json", ".md", ".py", ".sh", ".ts", ".tsx", ".js", ".mjs", ".go", ".toml", ".yaml", ".yml", ".txt", ""}
SKIP_DIRS = {"node_modules", ".git", "__pycache__", ".ruff_cache"}


def load_json(path, problems, label):
    try:
        return json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as error:
        problems.append(("error", f"{label}: cannot read JSON ({error})"))
        return None


def text_files(folder):
    for path in sorted(folder.rglob("*")):
        if path.is_file() and not SKIP_DIRS.intersection(path.relative_to(folder).parts) and path.suffix in TEXT_SUFFIXES:
            yield path


def installed_dests(plugin, problems):
    """Destinations from install.json, normalised to the $HOME form hooks use."""
    path = plugin / "install.json"
    if not path.exists():
        return set()
    data = load_json(path, problems, f"{plugin.name}/install.json") or {}
    dests = set()
    for entry in data.get("files", []):
        if not (plugin / entry.get("src", "")).exists() and "build" not in entry:
            problems.append(("error", f"{plugin.name}/install.json: src {entry.get('src')!r} does not exist"))
        dests.add(entry.get("dest", "").replace("~/", "$HOME/", 1))
    # A built executable (e.g. agent-tmux) names its build command and destination instead of a src file.
    for entry in data.get("builds", []) + ([data] if "build" in data else []):
        dests.add(entry.get("dest", "").replace("~/", "$HOME/", 1))
    return dests


def commands_in(value):
    """Every `command` string inside a hooks or MCP config, however deeply nested."""
    if isinstance(value, dict):
        for key, item in value.items():
            if key == "command" and isinstance(item, str):
                yield item
            else:
                yield from commands_in(item)
    elif isinstance(value, list):
        for item in value:
            yield from commands_in(item)


def check_commands(label, config, dests, problems):
    for command in commands_in(config):
        if PLUGIN_ROOT_VAR.search(command):
            problems.append(("error", f"{label}: command runs from the plugin folder ({command!r}); call the installed copy"))
        for found in INSTALLED.findall(command):
            if found.replace("~/", "$HOME/", 1) not in dests:
                problems.append(("error", f"{label}: {found} is not installed by install.json"))


def read_manifests(plugin, problems):
    manifests = {}
    for harness, rel in MANIFESTS.items():
        if (plugin / rel).exists():
            manifests[harness] = load_json(plugin / rel, problems, f"{plugin.name}/{rel}")
    manifests = {h: m for h, m in manifests.items() if m is not None}
    if not manifests:
        problems.append(("error", f"{plugin.name}: no harness manifest"))
    return manifests


def info_of(manifests):
    first = next(iter(manifests.values()))
    return {"version": first.get("version"), "description": first.get("description"), "harnesses": set(manifests)}


def lint_third_party(plugin):
    """Someone else's plugin: only what the catalogs need (its manifests), not this repo's conventions."""
    problems = []
    manifests = read_manifests(plugin, problems)
    return problems, info_of(manifests) if manifests else {}


def lint_plugin(plugin):
    problems = []
    name = plugin.name
    manifests = read_manifests(plugin, problems)
    if not manifests:
        return problems, {}

    for harness, manifest in manifests.items():
        label = f"{name}/{MANIFESTS[harness]}"
        if manifest.get("name") != name:
            problems.append(("error", f"{label}: name {manifest.get('name')!r} is not the folder name {name!r}"))
        if not CALVER.match(str(manifest.get("version", ""))):
            problems.append(("error", f"{label}: version {manifest.get('version')!r} is not CalVer YYYY.MM.DD"))
        for key in ("description", "author", "license"):
            if key not in manifest:
                problems.append(("warning", f"{label}: no {key}"))
        if "hooks" in manifest and manifest["hooks"] != HOOK_FILES[harness]:
            problems.append(("error", f"{label}: hooks should be {HOOK_FILES[harness]!r}, not {manifest['hooks']!r}"))
        if "mcpServers" in manifest and manifest["mcpServers"] != MCP_FILES[harness]:
            problems.append(("error", f"{label}: mcpServers should be {MCP_FILES[harness]!r}"))
    first = next(iter(manifests.values()))
    for key in SHARED_KEYS:
        values = {h: m.get(key) for h, m in manifests.items()}
        if len(set(map(json.dumps, values.values()))) > 1:
            problems.append(("error", f"{name}: {key} differs between manifests {values}"))

    if (plugin / "hooks/hooks.json").exists():
        problems.append(("error", f"{name}/hooks/hooks.json: every harness auto-loads this path; use hooks/<harness>.json"))
    dests = installed_dests(plugin, problems)
    for harness, manifest in manifests.items():
        if isinstance(manifest.get("hooks"), str):
            path = plugin / manifest["hooks"]
            if not path.exists():
                problems.append(("error", f"{name}: {manifest['hooks']} named by the {harness} manifest does not exist"))
            else:
                config = load_json(path, problems, f"{name}/{manifest['hooks']}")
                if config is not None:
                    check_commands(f"{name}/{manifest['hooks']}", config, dests, problems)
    for path in sorted(plugin.glob("hooks/*.json")):
        harness = path.stem
        if harness in MANIFESTS and harness not in manifests:
            problems.append(("warning", f"{name}/hooks/{path.name}: no {harness} manifest claims it"))

    mcp_files = [plugin / ".mcp.json", plugin / "mcp.json"]
    present = [p for p in mcp_files if p.exists()]
    if len(present) == 1:
        problems.append(("error", f"{name}: {present[0].name} exists without its twin; ship both, identical"))
    elif len(present) == 2:
        if present[0].read_bytes() != present[1].read_bytes():
            problems.append(("error", f"{name}: .mcp.json and mcp.json differ"))
        config = load_json(present[0], problems, f"{name}/.mcp.json")
        if config is not None:
            check_commands(f"{name}/.mcp.json", config, dests, problems)

    skills = plugin / "skills"
    if skills.is_dir():
        for skill_md in sorted(skills.rglob("SKILL.md")):
            depth = len(skill_md.relative_to(skills).parts)
            if depth != 2:
                problems.append(("error", f"{name}/{skill_md.relative_to(plugin)}: a skill sits exactly one level under skills/"))
            front = skill_md.read_text().split("---")
            if len(front) < 3 or "name:" not in front[1] or "description:" not in front[1]:
                problems.append(("error", f"{name}/{skill_md.relative_to(plugin)}: frontmatter needs name and description"))
    # Codex reads skills from the roots listed in its manifest; installed Codex plugins use ["./skills"].
    if "codex" in manifests and skills.is_dir() and manifests["codex"].get("skills") != ["./skills"]:
        problems.append(("error", f"{name}/.codex-plugin/plugin.json: skills should be [\"./skills\"]"))

    for required in ("README.md", "VERIFIED.md"):
        if not (plugin / required).exists():
            problems.append(("error", f"{name}: no {required}"))
    if (plugin / "README.md").exists() and "Harness support" not in (plugin / "README.md").read_text():
        problems.append(("error", f"{name}/README.md: no 'Harness support' table"))

    for path in text_files(plugin):
        text = path.read_text(errors="ignore")
        rel = path.relative_to(plugin)
        machine = MACHINE_PATH.search(text)
        if machine:
            problems.append(("error", f"{name}/{rel}: machine path {machine.group(0)!r}"))
        if SECRET.search(text):
            problems.append(("error", f"{name}/{rel}: looks like a secret"))
        if re.search(r"(?<![\w.])~/\.pi/|\$HOME/\.pi/", text):
            problems.append(("error", f"{name}/{rel}: uses ~/.pi/, which belongs to Pi Agent; use ~/.local/state/{name}/"))

    return problems, info_of(manifests)


def entry_source(harness, entry):
    """Where a catalog entry says the plugin lives, as (kind, value)."""
    source = entry.get("source")
    if isinstance(source, str):
        return "path", source
    if isinstance(source, dict) and source.get("source") == "local":
        return "path", source.get("path")
    if isinstance(source, dict) and source.get("source") == "github":
        return "github", source.get("repo")
    if isinstance(source, dict) and source.get("source") == "url":
        return "github", (source.get("url") or "").removeprefix("https://github.com/").removesuffix(".git")
    return "other", json.dumps(source)


def lint_catalogs(root, plugins, listed_repos):
    """plugins: name -> lint info for each repo found; listed_repos: name -> plugins.json entry."""
    problems = []
    for harness, rel in CATALOGS.items():
        claiming = {n: info for n, info in plugins.items() if harness in info["harnesses"]}
        path = root / rel
        if not path.exists():
            if claiming:
                problems.append(("error", f"{rel}: missing, but {sorted(claiming)} claim {harness}"))
            continue
        catalog = load_json(path, problems, rel)
        if catalog is None:
            continue
        if catalog.get("name") != MARKETPLACE:
            problems.append(("error", f"{rel}: marketplace name {catalog.get('name')!r} is not {MARKETPLACE!r}"))
        listed = {}
        for entry in catalog.get("plugins", []):
            name = entry.get("name")
            kind, value = entry_source(harness, entry)
            listed_entry = listed_repos.get(name) or {}
            want = ("path", f"./{listed_entry['path']}") if "path" in listed_entry else ("github", listed_entry.get("github"))
            if (kind, value) != want:
                problems.append(("error", f"{rel}: {name} source {value!r} should be {want[1]!r}"))
            sha = (listed_repos.get(name) or {}).get("sha")
            if sha and (entry.get("source") or {}).get("sha") != sha:
                problems.append(("error", f"{rel}: {name} is not pinned to plugins.json sha {sha}"))
            listed[name] = entry
        for name in sorted(set(listed) - set(claiming)):
            problems.append(("error", f"{rel}: lists {name}, which ships no {harness} manifest"))
        for name in sorted(set(claiming) - set(listed)):
            problems.append(("error", f"{rel}: does not list {name}"))
        for name in sorted(set(listed) & set(claiming)):
            if listed[name].get("version") != claiming[name]["version"]:
                problems.append(("error", f"{rel}: {name} version {listed[name].get('version')!r} != manifest {claiming[name]['version']!r}"))
    return problems


def main(argv=None):
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("--root", type=Path, default=ROOT, help="the workbench repo (default: this one)")
    parser.add_argument("--source", type=Path, help="the folder holding the plugin repos (default: ~/Source)")
    parser.add_argument("--plugin", action="append", help="lint only these plugins, and skip the catalogs")
    args = parser.parse_args(argv)

    listed = {entry["name"]: entry for entry in plugin_repos.entries(args.root)}
    owner = json.loads((args.root / "plugins.json").read_text()).get("owner")
    repos = {entry["name"]: repo for entry, repo in plugin_repos.repos(args.root, args.source)}
    report, infos = [], {}
    for name in sorted(repos):
        if args.plugin and name not in args.plugin:
            continue
        if not repos[name].is_dir():
            report.append(("warning", f"{name}: listed in plugins.json but no repo at {repos[name]}"))
            continue
        github_owner = listed[name].get("github", "").split("/")[0]
        third_party = bool(owner and github_owner and github_owner != owner)
        problems, info = (lint_third_party if third_party else lint_plugin)(repos[name])
        report += problems
        if info:
            infos[name] = info
    if not args.plugin:
        report += lint_catalogs(args.root, infos, listed)

    for level, message in report:
        print(f"{level.upper():8} {message}")
    errors = sum(1 for level, _ in report if level == "error")
    print(f"{len(infos)} plugin(s), {errors} error(s), {len(report) - errors} warning(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
