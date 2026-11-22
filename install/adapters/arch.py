"""Arch Linux and Omarchy: pacman for the official repositories, an AUR helper (yay or paru) for the AUR.

Installs use `pacman -S --needed`, the form Omarchy's own omarchy-pkg-add uses. Arch does not support partial
upgrades, so the update verb upgrades the whole system (`pacman -Syu`, or the AUR helper's `-Syu`, which also
upgrades AUR packages). The installer never builds an AUR helper itself: without yay or paru, AUR items are
reported and skipped. Omarchy ships yay.
"""

import shutil
import subprocess

from adapters import Action

HELPERS = ("yay", "paru")


class Arch:
    name = "arch"
    managers = ("pacman", "aur")

    def helper(self):
        return next((h for h in HELPERS if shutil.which(h)), None)

    def problems(self):
        return [] if shutil.which("pacman") else ["pacman is missing; this does not look like Arch"]

    def prepare(self):
        return []

    def resolve(self, argv):
        return list(argv)

    def unavailable(self, manager):
        """Why a manager cannot install here, or None."""
        if manager == "aur" and not self.helper():
            return "needs an AUR helper (yay or paru), which is not installed"
        return None

    def installed(self, manager, names):
        if not names or not shutil.which("pacman"):
            return set()
        result = subprocess.run(["pacman", "-Q", *names], capture_output=True, text=True, check=False)
        return {line.split()[0] for line in result.stdout.splitlines() if line.strip()}

    def install(self, manager, names):
        if manager == "aur":
            helper = self.helper() or "yay"
            return [Action(f"{helper} -S --needed {' '.join(names)}", [helper, "-S", "--needed", "--noconfirm", *names])]
        return [Action(f"pacman -S --needed {' '.join(names)}", ["pacman", "-S", "--needed", "--noconfirm", *names],
                       sudo=True)]

    def upgrade(self, packages):
        helper = self.helper()
        if helper:
            return [Action(f"{helper} -Syu (whole system and AUR; Arch has no partial upgrades)",
                           [helper, "-Syu", "--noconfirm"])]
        return [Action("pacman -Syu (whole system; Arch has no partial upgrades)", ["pacman", "-Syu", "--noconfirm"],
                       sudo=True)]
