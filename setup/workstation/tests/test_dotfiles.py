"""Canonical Dotfiles dependency, target, and lock tests."""

import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location(
    "workstation_dotfiles",
    Path(__file__).resolve().parents[1] / "scripts/workstation.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DotfilesTests(unittest.TestCase):
    def setUp(self):
        MODULE.select_profile("linux-full")

    def marker(self, path):
        path.mkdir(parents=True)
        for name in (".chezmoiignore.tmpl", "dot_bashrc.tmpl", "dot_tmux.conf.tmpl"):
            (path / name).write_text("")

    def test_resolves_normal_shared_root(self):
        with tempfile.TemporaryDirectory() as temp:
            outer = Path(temp)
            source = outer / "dotfiles"
            self.marker(source)
            with (
                mock.patch.object(MODULE, "ROOT", outer / "workstation"),
                mock.patch.dict(os.environ, {}, clear=True),
            ):
                self.assertEqual(MODULE.resolve_dotfiles_source(), source.resolve())

    def test_resolves_shared_worktree_root(self):
        with tempfile.TemporaryDirectory() as temp:
            outer = Path(temp)
            source = outer / "dotfiles"
            self.marker(source)
            root = outer / "worktrees" / "change"
            with (
                mock.patch.object(MODULE, "ROOT", root),
                mock.patch.dict(os.environ, {}, clear=True),
            ):
                self.assertEqual(MODULE.resolve_dotfiles_source(), source.resolve())

    def test_explicit_source_wins(self):
        with tempfile.TemporaryDirectory() as temp:
            source = Path(temp) / "paired-dotfiles"
            self.marker(source)
            with mock.patch.dict(
                os.environ, {"WORKSTATION_DOTFILES_SOURCE": str(source)}, clear=True
            ):
                self.assertEqual(MODULE.resolve_dotfiles_source(), source.resolve())

    def test_missing_source_fails(self):
        with (
            tempfile.TemporaryDirectory() as temp,
            mock.patch.object(MODULE, "ROOT", Path(temp) / "workstation"),
            mock.patch.dict(os.environ, {}, clear=True),
            self.assertRaisesRegex(RuntimeError, "source not found"),
        ):
            MODULE.resolve_dotfiles_source()

    def test_paired_source_matches_target_lock(self):
        source, rendered, digest = MODULE.render_dotfiles()
        self.assertEqual(source, MODULE.resolve_dotfiles_source())
        self.assertEqual(set(rendered), set(MODULE.dotfiles_lock()["targets"]))
        self.assertEqual(digest, MODULE.dotfiles_lock()["rendered_sha256"])
        tmux = rendered[".tmux.conf"][0].decode()
        self.assertIn("set -g @continuum-boot 'on'", tmux)  # linux-full starts tmux at boot
        self.assertIn("set -g @continuum-restore 'on'", tmux)

    def test_linux_renders_its_own_lock_without_gui_settings(self):
        MODULE.select_profile("linux")
        _, rendered, digest = MODULE.render_dotfiles()
        lock = MODULE.dotfiles_lock()
        self.assertEqual(lock["profile"], "linux")
        self.assertEqual(set(rendered), set(lock["targets"]))
        self.assertEqual(digest, lock["rendered_sha256"])
        self.assertFalse({".config/ghostty/config", MODULE.VSCODE_SETTINGS} & set(rendered))
        self.assertIn("set -g @continuum-boot 'off'", rendered[".tmux.conf"][0].decode())  # linux doesn't start tmux at boot

    def test_render_ignores_the_installers_strict_umask(self):
        # install() runs under umask 077; the render must still match the lock.
        previous = os.umask(0o077)
        try:
            for profile in ("linux-full", "linux"):
                MODULE.select_profile(profile)
                _, rendered, digest = MODULE.render_dotfiles()
                self.assertEqual(digest, MODULE.dotfiles_lock()["rendered_sha256"], profile)
                self.assertEqual(rendered[".bashrc"][1], 0o644)
        finally:
            os.umask(previous)

    def test_lock_writes_the_vscode_settings_later_stages_read(self):
        # A fresh home has no settings.json; the VS Code stage and verify both read it, so the lock must write it.
        _, rendered, _ = MODULE.render_dotfiles()
        self.assertIn(MODULE.VSCODE_SETTINGS, MODULE.dotfiles_lock()["targets"])
        settings = json.loads(rendered[MODULE.VSCODE_SETTINGS][0])
        self.assertEqual(settings["workbench.colorTheme"], "Nord")

    def test_installer_owned_vscode_key_passes_the_dotfile_check(self):
        _, rendered, _ = MODULE.render_dotfiles()
        locked = rendered[MODULE.VSCODE_SETTINGS][0]
        settings = json.loads(locked)
        settings["yaml.schemas"] = {"file:///home/u/.vscode/extensions/continue/schema.json": [".continue/**/*.yaml"]}
        self.assertTrue(MODULE.dotfile_matches(MODULE.VSCODE_SETTINGS, locked, json.dumps(settings, indent=2).encode()))
        settings["workbench.colorTheme"] = "Default Dark+"
        self.assertFalse(MODULE.dotfile_matches(MODULE.VSCODE_SETTINGS, locked, json.dumps(settings).encode()))
        tmux = rendered[".tmux.conf"][0]
        self.assertFalse(MODULE.dotfile_matches(".tmux.conf", tmux, tmux + b"\n"))

    def test_digest_mismatch_fails_before_write(self):
        bad_lock = dict(MODULE.dotfiles_lock(), rendered_sha256="0" * 64)
        with (
            mock.patch.object(MODULE, "dotfiles_lock", return_value=bad_lock),
            self.assertRaisesRegex(
                RuntimeError, "differs from the Ubuntu profile lock"
            ),
        ):
            MODULE.render_dotfiles()

    def test_configuration_records_and_backs_up_managed_targets(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            run = home / "recovery"
            state = home / ".local/state/workstation"
            run.mkdir()
            original = home / ".bashrc"
            original.write_text("original bashrc\n")
            with (
                mock.patch.object(MODULE, "HOME", home),
                mock.patch.object(MODULE, "RUN", run),
                mock.patch.object(MODULE, "STATE", state),
            ):
                MODULE.config()
            manifest = json.loads((run / "dotfiles-manifest.json").read_text())
            self.assertEqual(
                {item["path"] for item in manifest["targets"]},
                set(MODULE.dotfiles_lock()["targets"]),
            )
            bashrc = next(
                item for item in manifest["targets"] if item["path"] == ".bashrc"
            )
            self.assertEqual(bashrc["before_kind"], "file")
            self.assertNotEqual(bashrc["before_sha256"], bashrc["after_sha256"])
            self.assertEqual((run / "files/.bashrc").read_text(), "original bashrc\n")
            self.assertTrue(all(item["applied"] for item in manifest["targets"]))

    def test_partial_failure_retains_complete_preimage_journal(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            run = home / "recovery"
            state = home / ".local/state/workstation"
            run.mkdir()
            real_write = MODULE.write

            def fail_on_second_target(path, content, mode=0o644, seed=False):
                if path == home / ".bash_profile":
                    raise PermissionError("deliberate middle-target failure")
                return real_write(path, content, mode, seed)

            with (
                mock.patch.object(MODULE, "HOME", home),
                mock.patch.object(MODULE, "RUN", run),
                mock.patch.object(MODULE, "STATE", state),
                mock.patch.object(MODULE, "write", fail_on_second_target),
                self.assertRaisesRegex(PermissionError, "middle-target failure"),
            ):
                MODULE.config()

            manifest = json.loads((run / "dotfiles-manifest.json").read_text())
            entries = {item["path"]: item for item in manifest["targets"]}
            self.assertEqual(set(entries), set(MODULE.dotfiles_lock()["targets"]))
            self.assertEqual(entries[".bash_aliases"]["before_kind"], "absent")
            self.assertTrue(entries[".bash_aliases"]["applied"])
            self.assertTrue((home / ".bash_aliases").is_file())
            self.assertEqual(entries[".bash_profile"]["before_kind"], "absent")
            self.assertFalse(entries[".bash_profile"]["applied"])
            self.assertFalse(entries[".bashrc"]["applied"])


if __name__ == "__main__":
    unittest.main()
