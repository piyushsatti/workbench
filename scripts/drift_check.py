#!/usr/bin/env python3
"""Weekly drift check: report where this machine has drifted from the repository.

Read-only: every check only reads, and nothing is synced, copied or applied. Exit 1 when any check
reports drift or fails, 0 when all are clean or skipped. A skipped check (tool not installed, chezmoi
not pointed at this repo) is listed, never counted as drift.

Checks: shared agent config, skills and plugin executables (deploy_agent.py's plan, for the harnesses
--harness names), the plugins' conventions (lint_plugins.py), agent instruction files
(agent/bin/lint-agent-files.sh), dotfiles (chezmoi verify) and the launcher (workspaces doctor).
"""

import argparse
import importlib.util
import io
import shutil
import subprocess
import sys
from contextlib import redirect_stdout
from functools import partial
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HOME = Path.home()


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts" / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def check_deploy(harnesses=None):
    """Files deploy_agent.py would copy are drift: the machine no longer matches agent/ or the plugin repos in ~/Source."""
    deploy = load("deploy_agent")
    deploy.HOME = HOME
    changed = [f"{item[2]} {item[1]}" for item in deploy.plan(harnesses or deploy.HARNESSES) if item[2] != "same"]
    return ("drift", changed) if changed else ("ok", [])


def check_plugin_lint():
    lint = load("lint_plugins")
    out = io.StringIO()
    with redirect_stdout(out):
        status = lint.main([])
    lines = [line for line in out.getvalue().splitlines() if line.startswith("ERROR")]
    return ("drift", lines) if status else ("ok", [])


def run(argv, label):
    try:
        done = subprocess.run(argv, capture_output=True, text=True, timeout=120)
    except FileNotFoundError:
        return "skip", [f"{label}: {argv[0]} not found"]
    except subprocess.TimeoutExpired:
        return "fail", [f"{label}: timed out"]
    if done.returncode == 0:
        return "ok", []
    tail = (done.stdout + done.stderr).strip().splitlines()[-10:]
    return "drift", tail


def check_agent_files():
    script = ROOT / "agent/bin/lint-agent-files.sh"
    if not script.exists():
        return "skip", [f"{script} missing"]
    return run(["bash", str(script)], "lint-agent-files")


def check_chezmoi():
    chezmoi = shutil.which("chezmoi")
    if not chezmoi:
        return "skip", ["chezmoi not installed"]
    source = subprocess.run([chezmoi, "source-path"], capture_output=True, text=True)
    expected = ROOT / "setup/dotfiles"
    if source.returncode != 0 or Path(source.stdout.strip()).resolve() != expected.resolve():
        return "skip", [f"chezmoi's source is not {expected} (not initialised from this repo yet)"]
    return run([chezmoi, "verify"], "chezmoi verify")


def check_workspaces_doctor():
    launcher = HOME / ".local/bin/workspaces"
    if not launcher.exists():
        return "skip", [f"{launcher} not installed"]
    return run([str(launcher), "doctor"], "workspaces doctor")


CHECKS = [
    ("agent config, skills and plugin executables", check_deploy),
    ("plugin conventions", check_plugin_lint),
    ("agent instruction files", check_agent_files),
    ("dotfiles", check_chezmoi),
    ("launcher", check_workspaces_doctor),
]


def main(argv=None):
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("--verbose", action="store_true", help="list every finding, not just the first five per check")
    deploy = load("deploy_agent")
    parser.add_argument("--harness", type=deploy.parse_harnesses, default=deploy.HARNESSES,
                        help="comma-separated harnesses whose deploy targets count (default: claude,codex,cursor)")
    args = parser.parse_args(argv)
    bad = 0
    for label, check in CHECKS:
        if check is check_deploy:
            check = partial(check_deploy, args.harness)
        try:
            state, lines = check()
        except Exception as error:  # a broken check is a finding, not a crash
            state, lines = "fail", [f"{type(error).__name__}: {error}"]
        print(f"{state.upper():6} {label}" + (f" ({len(lines)})" if state == "drift" else ""))
        for line in lines if args.verbose else lines[:5]:
            print(f"         {line}")
        if state in ("drift", "fail"):
            bad += 1
            if not args.verbose and len(lines) > 5:
                print(f"         … {len(lines) - 5} more (--verbose)")
    print("no drift" if not bad else f"{bad} check(s) report drift")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
