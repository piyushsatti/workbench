"""One adapter per system: what installs a package there, how to tell it is installed, how to upgrade it.

Standard library only, and Python 3.9 or later: macOS's /usr/bin/python3 from the Command Line Tools is 3.9,
and the installer runs before anything else is installed. An adapter builds Actions and never runs them;
install.py runs them, and only with --apply.
"""

import os
import platform
import shutil
from collections import namedtuple
from pathlib import Path

# label: one line for the plan. argv: a command, run as is (sudo: prefixed with sudo when not root).
# func: a Python callable for a step that writes files; it gets no arguments.
Action = namedtuple("Action", "label argv func sudo", defaults=(None, None, False))

SYSTEMS = ("macos", "ubuntu", "arch")


class Unsupported(Exception):
    pass


def os_release(text):
    """/etc/os-release as a dict; quotes removed."""
    fields = {}
    for line in text.splitlines():
        key, sep, value = line.partition("=")
        if sep and key.strip() and not key.startswith("#"):
            fields[key.strip()] = value.strip().strip('"').strip("'")
    return fields


def detect(system=None, release_text=None, home=None, which=shutil.which):
    """(system, flavor): system is macos, ubuntu or arch; flavor is "omarchy" on an Omarchy install, else None.

    Arch derivatives count as arch (ID_LIKE names arch); Ubuntu derivatives count as ubuntu (ID_LIKE names ubuntu).
    Omarchy is told by its own commands on PATH (omarchy-pkg-add ships in its bin/) or its install folder.
    """
    system = system or platform.system()
    if system == "Darwin":
        return "macos", None
    if system != "Linux":
        raise Unsupported(f"{system} is not supported; workbench installs on macOS, Ubuntu and Arch")
    if release_text is None:
        path = Path("/etc/os-release")
        release_text = path.read_text() if path.exists() else ""
    fields = os_release(release_text)
    ids = [fields.get("ID", "")] + fields.get("ID_LIKE", "").split()
    home = Path(home) if home else Path.home()
    if "arch" in ids:
        omarchy = which("omarchy-pkg-add") or (home / ".local/share/omarchy").is_dir()
        return "arch", "omarchy" if omarchy else None
    if "ubuntu" in ids:
        return "ubuntu", None
    name = fields.get("PRETTY_NAME") or fields.get("ID") or "this Linux"
    raise Unsupported(f"{name} is not supported; workbench installs on macOS, Ubuntu and Arch")


def adapter(system):
    if system == "macos":
        from adapters.macos import Macos
        return Macos()
    if system == "ubuntu":
        from adapters.ubuntu import Ubuntu
        return Ubuntu()
    if system == "arch":
        from adapters.arch import Arch
        return Arch()
    raise Unsupported(system)


def is_root():
    return hasattr(os, "geteuid") and os.geteuid() == 0
