"""macOS: Homebrew formulae (brew) and casks (cask).

Homebrew is installed with its official script when missing (https://brew.sh); brew itself refuses to run as
root, so the installer must run as the user. The compiler and git come from the Xcode Command Line Tools.
"""

import shutil
import subprocess
from pathlib import Path

from adapters import Action, is_root

BREW_INSTALL = 'curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh'
# Where the Homebrew installer puts brew: Apple silicon, then Intel. A fresh install is not on PATH yet.
BREW_PATHS = ("/opt/homebrew/bin/brew", "/usr/local/bin/brew")


class Macos:
    name = "macos"
    managers = ("brew", "cask")

    def brew(self):
        found = shutil.which("brew")
        if found:
            return found
        return next((p for p in BREW_PATHS if Path(p).exists()), None)

    def problems(self):
        out = []
        if is_root():
            out.append("run the installer as your own user, not root: Homebrew refuses to run as root")
        if subprocess.run(["xcode-select", "-p"], capture_output=True, check=False).returncode:
            out.append("the Xcode Command Line Tools are missing: run `xcode-select --install`, then run this again")
        return out

    def prepare(self):
        if self.brew():
            return []
        return [Action("install Homebrew (official script from brew.sh)", ["/bin/bash", "-c", f'/bin/bash -c "$({BREW_INSTALL})"'])]

    def resolve(self, argv):
        """brew in an Action is found at run time: Homebrew may have been installed earlier in the same run."""
        if argv and argv[0] == "brew":
            return [self.brew() or "brew"] + list(argv[1:])
        return list(argv)

    def installed(self, manager, names):
        brew = self.brew()
        if not brew:
            return set()
        kind = "--cask" if manager == "cask" else "--formula"
        result = subprocess.run([brew, "list", kind, "-1"], capture_output=True, text=True, check=False)
        have = set(result.stdout.split()) if result.returncode == 0 else set()
        return {n for n in names if n in have}

    def install(self, manager, names):
        if manager == "cask":
            return [Action(f"brew install --cask {' '.join(names)}", ["brew", "install", "--cask", *names])]
        return [Action(f"brew install {' '.join(names)}", ["brew", "install", *names])]

    def upgrade(self, packages):
        actions = [Action("brew update", ["brew", "update"])]
        if packages.get("brew"):
            actions.append(Action(f"brew upgrade {' '.join(packages['brew'])}", ["brew", "upgrade", *packages["brew"]]))
        if packages.get("cask"):
            actions.append(Action(f"brew upgrade --cask {' '.join(packages['cask'])}",
                                  ["brew", "upgrade", "--cask", *packages["cask"]]))
        return actions
