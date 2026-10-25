"""Recovery and repeat-run behavior for the files the installer manages."""

import importlib.util
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "workstation", Path(__file__).resolve().parents[1] / "scripts/workstation.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ConfigurationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        MODULE.HOME = Path(self.temp.name)
        MODULE.RUN = MODULE.HOME / "recovery"
        MODULE.RUN.mkdir()
        MODULE.STATE = MODULE.HOME / ".local/state/workstation"

    def test_replacement_retains_original_and_repeat_run_is_unchanged(self):
        target = MODULE.HOME / ".tmux.conf"
        target.write_text("old config")
        MODULE.write(target, "new config")
        stamp = target.stat().st_mtime_ns
        MODULE.write(target, "new config")
        self.assertEqual(target.stat().st_mtime_ns, stamp)
        self.assertEqual((MODULE.RUN / "files/.tmux.conf").read_text(), "old config")

    def test_seed_preserves_existing_notes(self):
        target = MODULE.HOME / "notes.md"
        target.write_text("my edited notes")
        MODULE.write(target, "default notes", seed=True)
        self.assertEqual(target.read_text(), "my edited notes")

    def test_symlink_target_is_not_overwritten(self):
        original = MODULE.HOME / "other.conf"
        original.write_text("outside managed configuration")
        target = MODULE.HOME / ".tmux.conf"
        target.symlink_to(original)
        MODULE.write(target, "new config")
        self.assertEqual(original.read_text(), "outside managed configuration")
        self.assertFalse(target.is_symlink())
        self.assertTrue((MODULE.RUN / "files/.tmux.conf").is_symlink())

    def test_report_symlink_is_refused_without_changing_target(self):
        outside = Path(self.temp.name).parent / f"{Path(self.temp.name).name}-outside-report"
        outside.write_text("keep me")
        self.addCleanup(outside.unlink, missing_ok=True)
        state = MODULE.HOME / ".local/state/workstation"
        state.mkdir(parents=True)
        (state / "report.json").symlink_to(outside)
        MODULE.STATE = state
        with self.assertRaisesRegex(RuntimeError, "symlink"):
            MODULE.save_report()
        self.assertEqual(outside.read_text(), "keep me")

    def test_incomplete_checkout_is_retried_from_bare_remote(self):
        root = MODULE.HOME / "git-fixture"
        remote = root / "remote.git"
        seed = root / "seed"
        subprocess.run(
            ["git", "init", "--bare", str(remote)], check=True, capture_output=True
        )
        seed.mkdir(parents=True)
        subprocess.run(["git", "init", str(seed)], check=True, capture_output=True)
        subprocess.run(
            ["git", "-C", str(seed), "config", "user.email", "test@example.invalid"],
            check=True,
        )
        subprocess.run(
            ["git", "-C", str(seed), "config", "user.name", "Test"], check=True
        )
        (seed / "content").write_text("pinned\n")
        subprocess.run(["git", "-C", str(seed), "add", "content"], check=True)
        # Without the machine's global config, whose hooks may refuse the fixture identity.
        subprocess.run(
            ["git", "-C", str(seed), "commit", "-m", "fixture"],
            check=True,
            capture_output=True,
            env={**os.environ, "GIT_CONFIG_GLOBAL": os.devnull},
        )
        commit = subprocess.run(
            ["git", "-C", str(seed), "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
        subprocess.run(
            ["git", "-C", str(seed), "push", str(remote), "HEAD"],
            check=True,
            capture_output=True,
        )

        MODULE.DATA = root / "data"
        MODULE.RUN = MODULE.HOME / "recovery"
        MODULE.RUN.mkdir(exist_ok=True)
        target = MODULE.DATA / "sources" / "fixture" / commit
        target.mkdir(parents=True)
        subprocess.run(["git", "init", "--quiet", str(target)], check=True)

        result = MODULE.checkout("fixture", str(remote), commit)
        self.assertEqual(result, target)
        self.assertEqual(
            subprocess.run(
                ["git", "rev-parse", "HEAD"],
                cwd=target,
                check=True,
                capture_output=True,
                text=True,
            ).stdout.strip(),
            commit,
        )
        self.assertTrue(
            (MODULE.RUN / "incomplete-checkouts/fixture" / commit / ".git").is_dir()
        )


if __name__ == "__main__":
    unittest.main()
