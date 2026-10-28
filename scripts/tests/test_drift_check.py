"""drift_check.py: read-only, skips what isn't set up, and reports drift without fixing it."""

import importlib.util
import tempfile
import unittest
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location("drift_check", Path(__file__).resolve().parents[1] / "drift_check.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DriftCheckTests(unittest.TestCase):
    def test_harness_filter_reaches_the_deploy_plan(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            with mock.patch.object(MODULE, "HOME", home):
                _, everything = MODULE.check_deploy()
                _, claude_only = MODULE.check_deploy(("claude",))
            self.assertTrue(any("/.cursor/" in line for line in everything))
            self.assertFalse(any("/.cursor/" in line or "/.codex/" in line for line in claude_only))
        seen = []
        fake = lambda harnesses=None: seen.append(harnesses) or ("ok", [])  # noqa: E731
        with mock.patch.object(MODULE, "check_deploy", fake), mock.patch.object(MODULE, "CHECKS", [("deploy", fake)]), \
                redirect_stdout(StringIO()):
            self.assertEqual(MODULE.main(["--harness", "codex,claude"]), 0)
        self.assertEqual(seen, [("claude", "codex")])
        with mock.patch("sys.stderr", StringIO()), self.assertRaises(SystemExit):
            MODULE.main(["--harness", "vim"])

    def test_deploy_drift_on_an_empty_home_writes_nothing(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            with mock.patch.object(MODULE, "HOME", home):
                state, lines = MODULE.check_deploy()
            self.assertEqual(state, "drift")
            self.assertTrue(lines)
            self.assertEqual(list(home.iterdir()), [])

    def test_missing_tools_are_skipped_and_exit_follows_drift(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            with mock.patch.object(MODULE, "HOME", home), mock.patch.object(MODULE.shutil, "which", return_value=None):
                self.assertEqual(MODULE.check_chezmoi()[0], "skip")
                self.assertEqual(MODULE.check_workspaces_doctor()[0], "skip")
                checks = [("clean", lambda: ("ok", [])), ("absent", lambda: ("skip", ["x"]))]
                with mock.patch.object(MODULE, "CHECKS", checks), redirect_stdout(StringIO()) as out:
                    self.assertEqual(MODULE.main([]), 0)
                self.assertIn("no drift", out.getvalue())
                drifted = [*checks, ("drifted", lambda: ("drift", ["~/.tmux.conf differs"]))]
                with mock.patch.object(MODULE, "CHECKS", drifted), redirect_stdout(StringIO()) as out:
                    self.assertEqual(MODULE.main([]), 1)
                self.assertIn("~/.tmux.conf differs", out.getvalue())
                checks.append(("broken", lambda: 1 / 0))
                with mock.patch.object(MODULE, "CHECKS", checks), redirect_stdout(StringIO()) as out:
                    self.assertEqual(MODULE.main([]), 1)
                self.assertIn("FAIL   broken", out.getvalue())


if __name__ == "__main__":
    unittest.main()
