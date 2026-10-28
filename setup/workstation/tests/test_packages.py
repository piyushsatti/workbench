"""Regression tests for the package bootstrap and profile errors."""

import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
LIST = ROOT / "profiles/ubuntu-26.04/apt.txt"


def modern_bash():
    """A bash with mapfile (4+): packages.sh needs it. macOS's /bin/bash is 3.2, so look past it."""
    for candidate in (shutil.which("bash"), "/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/usr/bin/bash"):
        if candidate and os.access(candidate, os.X_OK):
            major = subprocess.run([candidate, "-c", "echo ${BASH_VERSINFO[0]}"], capture_output=True, text=True).stdout.strip()
            if major.isdigit() and int(major) >= 4:
                return candidate
    return None


BASH = modern_bash()


@unittest.skipUnless(BASH, "packages.sh needs bash 4 or later")
class PackagesTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "scripts").mkdir()
        (self.root / "profiles/ubuntu-26.04").mkdir(parents=True)
        shutil.copy(ROOT / "scripts/profile.py", self.root / "scripts/profile.py")
        shutil.copy(ROOT / "defaults.yml", self.root / "defaults.yml")
        shutil.copy(LIST, self.root / "profiles/ubuntu-26.04/apt.txt")

        script = (ROOT / "scripts/packages.sh").read_text()
        # Permit exercising the script's pre-install behavior without root.
        script = script.replace("[[ $EUID == 0 ]] || exit 1\n", "")
        self.script = self.root / "scripts/packages.sh"
        self.script.write_text(script)

        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "commands.log"
        self._stub(
            "timeout",
            'printf "timeout %s\\n" "$*" >> "$COMMAND_LOG"\n'
            'shift\nexec "$@"\n',
        )
        self._stub(
            "apt-get",
            'printf "apt-get %s\\n" "$*" >> "$COMMAND_LOG"\n',
        )
        self.env = os.environ.copy()
        self.env.update(
            PATH=f"{self.bin}:{os.environ['PATH']}", COMMAND_LOG=str(self.log)
        )

    def _stub(self, name, body):
        path = self.bin / name
        path.write_text(f"#!{BASH}\n{body}")
        path.chmod(0o755)

    def _run(self, profile):
        return subprocess.run(
            [BASH, str(self.script), profile],
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_bootstrap_installs_by_name_without_versions(self):
        result = self._run("linux")
        self.assertEqual(result.returncode, 0, result.stderr)
        install_lines = [
            line for line in self.log.read_text().splitlines()
            if line.startswith("apt-get ") and "install --yes" in line
        ]
        self.assertTrue(install_lines)
        self.assertTrue(all(f" {name}" in install_lines[0] for name in ("ca-certificates", "curl", "gnupg")), install_lines)
        pinned = [w for line in install_lines for w in line.split() if re.match(r"^[a-z0-9][a-z0-9.+-]*=", w)]
        self.assertEqual(pinned, [])

    def _stub_code_repository(self, fingerprint):
        """Stubs for adding the VS Code repository: the key download, gpg reporting `fingerprint`, and root-only tools."""
        self._stub("curl", 'printf "curl %s\\n" "$*" >> "$COMMAND_LOG"\nwhile [ $# -gt 1 ]; do shift; done\n: > "$1"\n')
        self._stub(
            "gpg",
            'printf "gpg %s\\n" "$*" >> "$COMMAND_LOG"\n'
            f'case " $* " in *" --show-keys "*) printf "pub:-:2048:1:X::::\\nfpr:::::::::{fingerprint}:\\n";;\n'
            '*" --dearmor "*) while [ "$1" != --output ]; do shift; done; : > "$2";; esac\n',
        )
        self._stub("install", 'printf "install %s\\n" "$*" >> "$COMMAND_LOG"\n')
        temp = self.root / "work"
        self._stub("mktemp", f'mkdir -p "{temp}" && echo "{temp}"\n')
        self._stub("dpkg", "echo amd64\n")

    def test_code_repository_key_must_have_microsofts_fingerprint(self):
        for fingerprint, ok in (("BC528686B50D79E339D3721CEB3E94ADBE1229CF", True), ("0" * 40, False)):
            self.log.unlink(missing_ok=True)
            self._stub_code_repository(fingerprint)
            result = self._run("linux-full")
            keyring = [line for line in self.log.read_text().splitlines()
                       if line.startswith("install ") and "workstation-microsoft.gpg" in line]
            if ok:
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(keyring)
            else:
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("fingerprint", result.stderr)
                self.assertEqual(keyring, [])

    def test_unknown_profile_preserves_profile_error(self):
        result = self._run("unknown-profile")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Unknown profile 'unknown-profile'", result.stderr)
        self.assertNotIn("selected no packages", result.stderr)


if __name__ == "__main__":
    unittest.main()
