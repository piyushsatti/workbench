#!/usr/bin/env python3
"""Install what you pick on macOS, Ubuntu or Arch (Omarchy included), in one run.

Pick a bundle ("tools", or "tools+agent" for the tools and the agent layer), name items, or both. Without
picks on a terminal, a plain numbered menu asks. The plan is printed first; --apply runs it (a terminal is
asked instead). Nothing is installed or changed without --apply or a yes.

  install.py list                              the catalog for this system
  install.py install [--bundle B] [--pick a,b] [--no-gui] [--apply] [--yes]
  install.py update [--apply] [--yes] [--no-pull]   pull workbench, add what the catalog newly lists for
                                               your picks, upgrade them, redeploy the agent layer
  install.py os                                the detected system

Picks are remembered in ~/.local/state/workbench/install.json after a run with --apply, so `update` knows
what to upgrade. Standard library only, Python 3.9 or later (macOS's own python3 is 3.9).
"""

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import steps
from adapters import SYSTEMS, Action, Unsupported, adapter, detect, is_root

REPO = HERE.parent
CATALOG = HERE / "catalog.json"


class PickError(Exception):
    pass


def load_catalog(path=CATALOG):
    return json.loads(Path(path).read_text())


def expand_bundle(catalog, name, seen=()):
    """A bundle's item names; "@other" includes another bundle."""
    if name in seen:
        raise PickError(f"bundle {name} includes itself")
    bundle = catalog["bundles"].get(name)
    if bundle is None:
        raise PickError(f"unknown bundle {name!r}; bundles: {', '.join(catalog['bundles'])}")
    out = []
    for entry in bundle["items"]:
        out += expand_bundle(catalog, entry[1:], seen + (name,)) if entry.startswith("@") else [entry]
    return out


class Selection:
    def __init__(self, items, unavailable, left_out):
        self.items = items              # item names to install, in catalog order
        self.unavailable = unavailable  # (name, reason): picked but not offered on this system
        self.left_out = left_out        # (name, reason): dropped from a bundle (--no-gui, Omarchy)


def resolve(catalog, system, bundles=(), picks=(), flavor=None, no_gui=False):
    """Turn bundles and item picks into the items to install on this system.

    Bundle items drop out under --no-gui (group gui) and on Omarchy (skip_on: omarchy); an item named with
    --pick is always kept. requires pulls items in. An item with no entry for this system is reported, not
    installed. The result keeps catalog order, which lists every required item before the items needing it.
    """
    items = catalog["items"]
    explicit = []
    for name in picks:
        if name in catalog["bundles"]:
            bundles = tuple(bundles) + (name,)
        elif name in items:
            explicit.append(name)
        else:
            raise PickError(f"unknown item {name!r}; see `workbench install list`")
    chosen, left_out = set(explicit), []
    for bundle in bundles:
        for name in expand_bundle(catalog, bundle):
            if name not in items:
                raise PickError(f"bundle {bundle} names unknown item {name!r}")
            if name in chosen:
                continue
            if no_gui and items[name]["group"] == "gui":
                left_out.append((name, "--no-gui"))
            elif flavor and flavor in items[name].get("skip_on", []):
                left_out.append((name, f"{flavor} ships its own"))
            else:
                chosen.add(name)
    pending = list(chosen)
    while pending:
        for required in items[pending.pop()].get("requires", []):
            if required not in chosen:
                chosen.add(required)
                pending.append(required)
    left_out = [(n, why) for n, why in left_out if n not in chosen]
    ordered = [n for n in items if n in chosen]
    unavailable = [(n, f"not offered on {system}") for n in ordered if system not in items[n]]
    return Selection([n for n in ordered if system in items[n]], unavailable, left_out)


def packages(catalog, system, names):
    """manager -> package names for the items, in order, without repeats."""
    out = {}
    for name in names:
        for manager, values in catalog["items"][name][system].items():
            if manager in ("steps", "before"):
                continue
            bucket = out.setdefault(manager, [])
            bucket += [v for v in values if v not in bucket]
    return out


def item_steps(catalog, system, name, key="steps"):
    item = catalog["items"][name]
    return list(item.get(key, [])) + list(item.get(system, {}).get(key, []))


def plan(catalog, system, selection, os_adapter, ctx, check_installed=True):
    """(actions, notes): prepare the package manager, the `before` steps, one install per manager for the
    packages not yet installed, then each item's steps in order."""
    actions, notes = [], []
    actions += os_adapter.prepare()
    for name in selection.items:
        for step in item_steps(catalog, system, name, "before"):
            actions += steps.actions(ctx, step)
    for manager, names in packages(catalog, system, selection.items).items():
        why = getattr(os_adapter, "unavailable", lambda m: None)(manager)
        if why:
            notes.append(f"skipped {manager} packages {' '.join(names)}: {why}")
            continue
        have = os_adapter.installed(manager, names) if check_installed else set()
        if have:
            notes.append(f"already installed ({manager}): {' '.join(n for n in names if n in have)}")
        wanted = [n for n in names if n not in have]
        if wanted:
            actions += os_adapter.install(manager, wanted)
    for name in selection.items:
        for step in item_steps(catalog, system, name):
            actions += steps.actions(ctx, step)
    return actions, notes + ctx.notes


# ---------------------------------------------------------------------------
# The menu
# ---------------------------------------------------------------------------


def menu_lines(catalog, system):
    """The numbered menu: bundles first, then the items offered on this system, by group."""
    lines = ["Bundles:"]
    for name, bundle in catalog["bundles"].items():
        lines.append(f"  {name:<14} {bundle['description']}")
    lines.append("Items:")
    numbered = [n for n in catalog["items"] if system in catalog["items"][n]]
    for group in catalog["groups"]:
        members = [n for n in numbered if catalog["items"][n]["group"] == group]
        if members:
            lines.append(f" [{group}]")
            lines += [f"  {numbered.index(n) + 1:>2}) {n:<15} {catalog['items'][n]['description']}" for n in members]
    return lines, numbered


def parse_answer(answer, numbered, catalog):
    """Menu input to picks: bundle names, item names, numbers and ranges (3-7), separated by spaces or commas."""
    picks = []
    for word in answer.replace(",", " ").split():
        if word in catalog["bundles"] or word in catalog["items"]:
            picks.append(word)
        elif "-" in word and all(part.isdigit() for part in word.split("-", 1)):
            low, high = (int(part) for part in word.split("-", 1))
            if not 1 <= low <= high <= len(numbered):
                raise PickError(f"{word} is outside 1-{len(numbered)}")
            picks += numbered[low - 1:high]
        elif word.isdigit() and 1 <= int(word) <= len(numbered):
            picks.append(numbered[int(word) - 1])
        else:
            raise PickError(f"{word!r} is not a bundle, an item or a number from the menu")
    return picks


def ask_picks(catalog, system):
    lines, numbered = menu_lines(catalog, system)
    print("\n".join(lines))
    while True:
        answer = input("\nPick a bundle, or item numbers (e.g. tools, or 2 5 9-12): ").strip()
        if not answer:
            raise SystemExit("nothing picked")
        try:
            return parse_answer(answer, numbered, catalog)
        except PickError as error:
            print(error)


# ---------------------------------------------------------------------------
# Running
# ---------------------------------------------------------------------------


def run_actions(actions, os_adapter, home):
    env = dict(os.environ)
    env["PATH"] = f"{home / '.local/bin'}{os.pathsep}{env.get('PATH', '')}"
    for action in actions:
        print(f"==> {action.label}", flush=True)
        if action.func:
            action.func()
            continue
        argv = os_adapter.resolve(action.argv)
        if action.sudo and not is_root():
            argv = ["sudo", *argv]
        subprocess.run(argv, check=True, env=env)


def state_path(home):
    return home / ".local/state/workbench/install.json"


def read_state(home):
    path = state_path(home)
    try:
        return json.loads(path.read_text())
    except (OSError, ValueError):
        return {}


def write_state(home, system, items):
    path = state_path(home)
    state = read_state(home)
    merged = list(dict.fromkeys(list(state.get("items", [])) + list(items)))
    path.parent.mkdir(parents=True, exist_ok=True)
    # timezone.utc, not datetime.UTC: that alias is Python 3.11+, and this runs on 3.9.
    now = datetime.now(timezone.utc)  # noqa: UP017
    path.write_text(json.dumps({"system": system, "items": merged,
                                "updated": now.strftime("%Y-%m-%dT%H:%M:%SZ")}, indent=2) + "\n")


def confirm(args):
    if args.apply or args.yes:
        return True
    if not sys.stdin.isatty():
        print("\nDry run: nothing was changed. Rerun with --apply to do it.")
        return False
    return input("\nDo this now? [y/N] ").strip().lower() in ("y", "yes")


def print_plan(selection, actions, notes):
    print(f"Items: {' '.join(selection.items) or '(none)'}")
    for name, why in selection.left_out:
        print(f"LEFT OUT  {name}: {why}")
    for name, why in selection.unavailable:
        print(f"NOT HERE  {name}: {why}")
    for line in notes:
        print(f"NOTE      {line}")
    if actions:
        print("Plan:")
        for number, action in enumerate(actions, 1):
            print(f"  {number:>2}. {action.label}{' (sudo)' if action.sudo and not is_root() else ''}")
    else:
        print("Nothing to do.")


def environment():
    system, flavor = detect()
    os_adapter = adapter(system)
    problems = os_adapter.problems()
    if problems:
        raise SystemExit("workbench install: " + "; ".join(problems))
    return system, flavor, os_adapter


def cmd_install(args, home=None):
    home = home or Path.home()
    catalog = load_catalog()
    system, flavor, os_adapter = environment()
    picks = [p.strip() for p in (args.pick or "").split(",") if p.strip()]
    bundles = [args.bundle] if args.bundle else []
    if not picks and not bundles:
        if not sys.stdin.isatty():
            raise SystemExit("workbench install: name --bundle or --pick (no terminal to ask on)")
        picks = ask_picks(catalog, system)
    selection = resolve(catalog, system, bundles, picks, flavor, args.no_gui)
    ctx = steps.Context(REPO, home, system, flavor, os_adapter, selection.items)
    actions, notes = plan(catalog, system, selection, os_adapter, ctx)
    print(f"System: {system}{' (' + flavor + ')' if flavor else ''}")
    print_plan(selection, actions, notes)
    if actions and confirm(args):
        run_actions(actions, os_adapter, home)
        write_state(home, system, selection.items)
        print("\nDone. Open a new terminal so PATH and group changes apply.")
    return 0


def cmd_update(args, home=None):
    """Pull workbench, then install what the catalog newly lists for the remembered picks, upgrade their
    packages, update the agent CLIs this installer put in place, and redeploy the agent layer."""
    home = home or Path.home()
    if not args.no_pull and (REPO / ".git").exists():
        print("==> git pull --ff-only (workbench)")
        if args.apply or args.yes or (sys.stdin.isatty() and input("Pull workbench first? [y/N] ").lower() in ("y", "yes")):
            subprocess.run(["git", "-C", str(REPO), "pull", "--ff-only"], check=True)
            # The catalog and this file may have changed; run the rest from the pulled copy.
            os.execv(sys.executable, [sys.executable, "-B", str(Path(__file__)), "update", "--no-pull",
                                      *(["--apply"] if args.apply else []), *(["--yes"] if args.yes else [])])
    catalog = load_catalog()
    system, flavor, os_adapter = environment()
    remembered = [n for n in read_state(home).get("items", []) if n in catalog["items"]]
    if not remembered:
        raise SystemExit("workbench update: no remembered picks; run `workbench install` first")
    selection = resolve(catalog, system, (), remembered, flavor)
    ctx = steps.Context(REPO, home, system, flavor, os_adapter, selection.items)
    actions, notes = plan(catalog, system, selection, os_adapter, ctx)
    actions = [a for a in actions if not a.label.startswith("deploy the agent layer")]
    actions += os_adapter.upgrade(packages(catalog, system, selection.items))
    npm = ctx.data / "npm"
    for name in selection.items:
        for step in item_steps(catalog, system, name):
            if step[0] == "npm-cli" and (npm / "node_modules/.bin" / step[1]).exists():
                actions.append(Action(f"npm install {step[2]}@latest (ours, in ~/.local/share/workbench/npm)",
                                      ["npm", "install", "--no-audit", "--no-fund", "--prefix", str(npm), f"{step[2]}@latest"]))
    if "agent-layer" in selection.items:
        actions += steps.agent_layer(ctx)
    print(f"System: {system}{' (' + flavor + ')' if flavor else ''}")
    print_plan(selection, actions, notes)
    if actions and confirm(args):
        run_actions(actions, os_adapter, home)
        write_state(home, system, selection.items)
    return 0


def cmd_list(args):
    catalog = load_catalog()
    system = args.system or environment()[0]
    print("\n".join(menu_lines(catalog, system)[0]))
    return 0


def main(argv=None):
    parser = argparse.ArgumentParser(prog="workbench install", description=(__doc__ or "").splitlines()[0])
    sub = parser.add_subparsers(dest="command")
    install = sub.add_parser("install", help="install picked items (default)")
    install.add_argument("--bundle", help="tools, or tools+agent")
    install.add_argument("--pick", help="comma-separated items or bundles")
    install.add_argument("--no-gui", action="store_true", help="leave the gui group out of bundles (a headless box)")
    update = sub.add_parser("update", help="pull, then upgrade what was installed")
    update.add_argument("--no-pull", action="store_true", help="do not git pull workbench first")
    for p in (install, update):
        p.add_argument("--apply", action="store_true", help="do it (default: print the plan)")
        p.add_argument("--yes", action="store_true", help="do not ask")
    listing = sub.add_parser("list", help="the catalog for this system")
    listing.add_argument("--system", choices=SYSTEMS, help="show another system's catalog")
    sub.add_parser("os", help="the detected system")
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0] not in ("install", "update", "list", "os", "-h", "--help"):
        argv = ["install", *argv]
    args = parser.parse_args(argv)
    try:
        if args.command == "os":
            system, flavor = detect()
            print(system + (f" ({flavor})" if flavor else ""))
            return 0
        if args.command == "list":
            return cmd_list(args)
        if args.command == "update":
            return cmd_update(args)
        return cmd_install(args)
    except (PickError, Unsupported) as error:
        print(f"workbench install: {error}", file=sys.stderr)
        return 2
    except subprocess.CalledProcessError as error:
        print(f"workbench install: `{' '.join(map(str, error.cmd))}` failed (exit {error.returncode}); "
              "fix that and run the same command again", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
