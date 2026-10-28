"""Who owns the dotfiles: the user's chezmoi when it is initialised from this repo's source, else the locked render."""

import importlib.util
import tempfile
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location(
    "workstation_chezmoi", Path(__file__).resolve().parents[1] / "scripts/workstation.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)

# A stand-in chezmoi: source-path prints $HOME/source-path, status prints $HOME/status.
FAKE_CHEZMOI = """#!/bin/sh
case "$1" in
  source-path) cat "$HOME/source-path" ;;
  status) cat "$HOME/status" ;;
  *) echo "unexpected chezmoi $*" >&2; exit 2 ;;
esac
"""


class ChezmoiOwnerTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.home = Path(temp.name).resolve()
        chezmoi = self.home / ".local/bin/chezmoi"
        chezmoi.parent.mkdir(parents=True)
        chezmoi.write_text(FAKE_CHEZMOI)
        chezmoi.chmod(0o755)
        (self.home / "status").write_text("")
        self.source = MODULE.resolve_dotfiles_source()
        for name, value in (("HOME", self.home), ("REPORT", {"pending": [], "failed": []})):
            patcher = mock.patch.object(MODULE, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)
        MODULE.select_profile("linux")

    def initialised_from(self, source):
        (self.home / "source-path").write_text(f"{source}\n")

    def test_chezmoi_initialised_from_this_repo_owns_the_dotfiles(self):
        self.initialised_from(self.source)
        self.assertEqual(MODULE.chezmoi_owns_dotfiles(), self.home / ".local/bin/chezmoi")
        with mock.patch.object(MODULE, "apply_dotfiles") as apply, mock.patch.object(MODULE, "agent_seeds"):
            MODULE.config()
        apply.assert_not_called()
        self.assertEqual(MODULE.REPORT["dotfiles"], {"owner": "chezmoi", "source": str(self.source)})
        with mock.patch.object(MODULE, "render_dotfiles") as render, mock.patch.object(MODULE, "verify_tmux") as tmux:
            checks = []
            MODULE.verify_dotfiles(checks)
        render.assert_not_called()
        tmux.assert_not_called()
        self.assertTrue(any("chezmoi has nothing to apply" in c for c in checks), checks)

    def test_verify_fails_while_chezmoi_has_changes_to_apply(self):
        self.initialised_from(self.source)
        (self.home / "status").write_text(" M .tmux.conf\n")
        with self.assertRaisesRegex(RuntimeError, r"(?s)chezmoi has changes to apply.*\.tmux\.conf"):
            MODULE.verify_dotfiles([])

    def test_without_chezmoi_from_this_repo_the_locked_render_applies_and_verifies(self):
        for source in (self.home / ".local/share/chezmoi", None):
            if source is None:
                (self.home / ".local/bin/chezmoi").unlink()
            else:
                self.initialised_from(source)
            self.assertIsNone(MODULE.chezmoi_owns_dotfiles(), source)
            with mock.patch.object(MODULE, "apply_dotfiles") as apply, mock.patch.object(MODULE, "agent_seeds"):
                MODULE.config()
            apply.assert_called_once()
            with mock.patch.object(MODULE, "verify_locked_dotfiles") as locked:
                MODULE.verify_dotfiles([])
            locked.assert_called_once()


if __name__ == "__main__":
    unittest.main()
