"""Ubuntu: apt, as root through sudo. The package names are those of setup/workstation/profiles/ubuntu-26.04/apt.txt.

apt-get install of a package that is already installed does not upgrade it; upgrades are the update verb's job.
Other Ubuntu releases may lack a package (ghostty, for one); apt then refuses and nothing is installed.
"""

import shutil
import subprocess

from adapters import Action

APT = ["env", "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=l", "apt-get",
       "-o", "Acquire::Retries=3", "-o", "DPkg::Lock::Timeout=120"]


class Ubuntu:
    name = "ubuntu"
    managers = ("apt",)

    def problems(self):
        return [] if shutil.which("apt-get") else ["apt-get is missing; this does not look like Ubuntu"]

    def prepare(self):
        return [Action("apt-get update", [*APT, "update"], sudo=True)]

    def resolve(self, argv):
        return list(argv)

    def installed(self, manager, names):
        if not names or not shutil.which("dpkg-query"):
            return set()
        result = subprocess.run(["dpkg-query", "-W", "-f=${Package} ${db:Status-Abbrev}\\n", *names],
                                capture_output=True, text=True, check=False)
        return {line.split()[0] for line in result.stdout.splitlines() if line.split()[1:2] == ["ii"]}

    def install(self, manager, names):
        return [Action(f"apt-get install {' '.join(names)}",
                       [*APT, "install", "--yes", "--no-install-recommends", *names], sudo=True)]

    def upgrade(self, packages):
        names = packages.get("apt", [])
        actions = [Action("apt-get update", [*APT, "update"], sudo=True)]
        if names:
            actions.append(Action(f"apt-get install --only-upgrade {' '.join(names)}",
                                  [*APT, "install", "--only-upgrade", "--yes", *names], sudo=True))
        return actions
