"""The pinned tmux plugins: the lock matches the Dotfiles config, and tmux-resurrect gets its local patches."""

import importlib.util
import re
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("workstation_plugins", ROOT / "scripts/workstation.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
TMUX_CONF = ROOT.parent / "dotfiles/dot_tmux.conf.tmpl"
PATCH = ROOT.parent / "dotfiles/.patches/tmux-resurrect.diff"


def unpatched_files():
    """Stand-ins for the upstream scripts: each hunk's context and removed lines, which git apply locates by content."""
    files, name = {}, None
    for line in PATCH.read_text().splitlines():
        if line.startswith("diff --git "):
            name = line.split(" b/", 1)[1]
            files[name] = []
        elif line.startswith(("---", "+++", "@@", "index ")):
            continue
        elif line.startswith((" ", "-")):
            files[name].append(line[1:])
    return {name: "\n".join(lines) + "\n" for name, lines in files.items()}


def write_tree(tree, files):
    for name, text in files.items():
        (tree / name).parent.mkdir(parents=True, exist_ok=True)
        (tree / name).write_text(text)


class PluginLockTests(unittest.TestCase):
    def test_every_configured_plugin_is_pinned_and_every_pin_is_configured(self):
        configured = re.findall(r"^set -g @plugin '([^']+)'$", TMUX_CONF.read_text(), re.MULTILINE)
        self.assertEqual(len(configured), len(set(configured)))
        self.assertEqual(sorted(configured), sorted(plugin["repo"] for plugin in MODULE.pinned_plugins()))
        for plugin in MODULE.pinned_plugins():
            # TPM loads ~/.tmux/plugins/<repository name>, so that is where the installer must put it.
            self.assertEqual(plugin["name"], plugin["repo"].split("/")[1])
            self.assertRegex(plugin["commit"], r"^[0-9a-f]{40}$")


class ResurrectPatchTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.home = Path(temp.name).resolve()
        # A repository above the plugin tree: without the ceiling, git apply would resolve paths against it.
        subprocess.run(["git", "init", "--quiet", str(self.home)], check=True)

    def test_patch_applies_once_and_verifies(self):
        tree = self.home / ".tmux/plugins/tmux-resurrect"
        write_tree(tree, unpatched_files())
        self.assertFalse(MODULE.resurrect_patch(tree, "--reverse", "--check"))
        MODULE.patch_resurrect(tree)
        patched = {name: (tree / name).read_text() for name in unpatched_files()}
        self.assertIn('[ ! -s "$resurrect_file" ]', patched["scripts/restore.sh"])
        self.assertIn('[ -s "$resurrect_file_path" ]', patched["scripts/save.sh"])
        self.assertTrue(MODULE.resurrect_patch(tree, "--reverse", "--check"))
        MODULE.patch_resurrect(tree)
        self.assertEqual({name: (tree / name).read_text() for name in patched}, patched)

    def test_upstream_that_no_longer_matches_fails(self):
        tree = self.home / ".tmux/plugins/tmux-resurrect"
        write_tree(tree, {name: "changed upstream\n" for name in unpatched_files()})
        with self.assertRaisesRegex(RuntimeError, "no longer apply"):
            MODULE.patch_resurrect(tree)

    def test_install_patches_the_copy_and_leaves_the_pinned_source_clean(self):
        sources = self.home / "sources"

        def checkout(name, url, commit):
            source = sources / name
            source.mkdir(parents=True, exist_ok=True)
            if name == "tmux-resurrect":
                write_tree(source, unpatched_files())
            if name == "tmux-thumbs":
                write_tree(source, {"target/release/tmux-thumbs": ""})
            return source

        run = self.home / "recovery"
        run.mkdir()
        with (
            mock.patch.object(MODULE, "HOME", self.home),
            mock.patch.object(MODULE, "RUN", run),
            mock.patch.object(MODULE, "checkout", checkout),
        ):
            MODULE.plugins()
            installed = self.home / ".tmux/plugins/tmux-resurrect"
            self.assertTrue(MODULE.resurrect_patch(installed, "--reverse", "--check"))
            self.assertFalse(MODULE.resurrect_patch(sources / "tmux-resurrect", "--reverse", "--check"))
            # A repeat run finds the revision marker and the patch already in place.
            MODULE.plugins()
            self.assertTrue(MODULE.resurrect_patch(installed, "--reverse", "--check"))


if __name__ == "__main__":
    unittest.main()
