"""workbench dispatches each verb to the right workbench script, passes arguments through, and refuses unknown ones."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

CLI = Path(__file__).resolve().parents[1] / "bin" / "workbench"
STUB = '#!/bin/sh\necho "$(basename "$0") $*" >> "$WORKBENCH_LOG"\n'


class WorkbenchTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.pi, self.log, stubs = root / "workbench", root / "calls.log", root / "stubs"
        for rel in ["scripts/write_catalogs.py", "scripts/lint_plugins.py", "scripts/deploy_agent.py",
                    "scripts/drift_check.py", "setup/workstation/verify.sh", "setup/workstation/bootstrap.sh",
                    "stubs/chezmoi", "stubs/python3", "stubs/sudo", "stubs/uname"]:
            path = (root if rel.startswith("stubs") else self.pi) / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(STUB)
            path.chmod(0o755)
        (self.pi / "setup/dotfiles").mkdir(parents=True)
        # sudo runs its command; uname says Linux, so `setup` gets past its OS check.
        (stubs / "sudo").write_text('#!/bin/sh\necho "sudo $(basename "$1")" >> "$WORKBENCH_LOG"\nexec "$@"\n')
        (stubs / "uname").write_text("#!/bin/sh\necho Linux\n")
        # python3 stub: log the script it was given, minus -B.
        (stubs / "python3").write_text('#!/bin/sh\nshift\nscript=$(basename "$1")\nshift\necho "$script${*:+ $*}" >> "$WORKBENCH_LOG"\n')
        self.env = {**os.environ, "PI_ROOT": str(self.pi), "WORKBENCH_LOG": str(self.log),
                    "PATH": f"{stubs}:{os.environ['PATH']}", "PYTHON": str(stubs / "python3")}

    def tearDown(self):
        self.temp.cleanup()

    def run_cli(self, *args):
        done = subprocess.run([str(CLI), *args], env=self.env, capture_output=True, text=True)
        calls = self.log.read_text().splitlines() if self.log.exists() else []
        return done, calls

    def test_plugins_check_is_a_dry_run_then_lint(self):
        done, calls = self.run_cli("plugins")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(calls, ["write_catalogs.py", "lint_plugins.py"])

    def test_plugins_sync_applies(self):
        _, calls = self.run_cli("plugins", "sync")
        self.assertEqual(calls[0].split()[:2], ["write_catalogs.py", "--apply"])

    def test_deploy_and_drift_pass_arguments_through(self):
        _, calls = self.run_cli("deploy", "--apply", "--harness", "claude")
        self.assertEqual(calls, ["deploy_agent.py --apply --harness claude"])
        self.log.unlink()
        _, calls = self.run_cli("drift", "--verbose")
        self.assertEqual(calls, ["drift_check.py --verbose"])

    def test_dotfiles_default_is_diff_against_workbench_source(self):
        _, calls = self.run_cli("dotfiles")
        self.assertEqual(calls, [f"chezmoi --source {self.pi}/setup/dotfiles diff"])

    def test_setup_runs_bootstrap_once_and_no_second_verify(self):
        done, calls = self.run_cli("setup", "--profile", "linux")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(calls, ["sudo bootstrap.sh", "bootstrap.sh --profile linux"])

    def test_unknown_verbs_are_refused(self):
        for args in (["bogus"], ["dotfiles", "nuke"], ["plugins", "publish"]):
            done, calls = self.run_cli(*args)
            self.assertEqual(done.returncode, 64, args)
            self.assertEqual(calls, [])

    def test_missing_checkout_is_a_config_error(self):
        self.env["PI_ROOT"] = str(Path(self.temp.name) / "absent")
        done, _ = self.run_cli("drift")
        self.assertEqual(done.returncode, 78)


if __name__ == "__main__":
    unittest.main()
