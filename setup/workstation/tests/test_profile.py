"""defaults.yml profile expansion, the ubuntu-26.04 lock equivalence, and the headless linux profile."""

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
LIST = ROOT / "profiles/ubuntu-26.04/apt.txt"
FULL = "linux-full"


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PROFILES = load("workstation_profile_helper", ROOT / "scripts/profile.py")
WORKSTATION = load("workstation_for_profiles", ROOT / "scripts/workstation.py")


class ParserTests(unittest.TestCase):
    def test_subset_structures(self):
        text = (
            "# comment\n"
            "schema: 1\n"
            "name: linux   # trailing comment\n"
            "empty: null\n"
            "flow: [a, b.c, docker.io]\n"
            "none: []\n"
            "nested:\n"
            "  inner:\n"
            "    - one\n"
            "    - two\n"
        )
        self.assertEqual(
            PROFILES.parse(text),
            {
                "schema": 1,
                "name": "linux",
                "empty": None,
                "flow": ["a", "b.c", "docker.io"],
                "none": [],
                "nested": {"inner": ["one", "two"]},
            },
        )

    def test_rejects_text_outside_the_subset(self):
        for text in (
            "key: yes\n",  # YAML 1.1 boolean
            "key: 26.04\n",  # YAML float
            "key: 'quoted'\n",
            "key: @scoped/name\n",
            "key: {a: b}\n",
            "key:\n   - odd indent\n",
            "key:\n\t- tab\n",
            "key: a\nkey: b\n",
            "key: [a, b\n",
            "a:\n  - x\n  y: z\n",
        ):
            with self.subTest(text=text), self.assertRaises(PROFILES.ProfileError):
                PROFILES.parse(text)


class ProfileTests(unittest.TestCase):
    def setUp(self):
        self.data = PROFILES.load()

    def test_lists_the_three_profiles(self):
        self.assertEqual(
            set(self.data["profiles"]),
            {"linux", FULL, "darwin"},
        )

    def test_defaults_are_consistent(self):
        PROFILES.check(self.data)

    def test_full_profile_reproduces_the_list_byte_for_byte(self):
        before = subprocess.run(
            ["awk", "NF && $1 !~ /^#/ {print $1}", str(LIST)],
            check=True,
            capture_output=True,
        ).stdout
        after = subprocess.run(
            [sys.executable, str(ROOT / "scripts/profile.py"), "apt", FULL],
            check=True,
            capture_output=True,
        ).stdout
        self.assertEqual(after, before)
        self.assertEqual(len(before.splitlines()), 51)

    def test_full_profile_matches_the_installer_locks(self):
        full = PROFILES.expand(FULL, self.data)
        self.assertEqual(
            ROOT / full["dotfiles_lock"], WORKSTATION.PROFILE / "dotfiles.lock.json"
        )
        WORKSTATION.select_profile(FULL)
        self.assertEqual(WORKSTATION.dotfiles_lock()["profile"], FULL)
        self.assertEqual(full["harnesses"], ["claude", "codex", "opencode"])
        self.assertEqual(WORKSTATION.harnesses(), full["harnesses"])
        self.assertIn("vercel@claude-plugins-official", full["claude_plugins"])
        self.assertEqual(PROFILES.expand("linux", self.data)["claude_plugins"], full["claude_plugins"])

    def test_linux_has_no_gui(self):
        vm = PROFILES.expand("linux", self.data)
        full = PROFILES.expand(FULL, self.data)
        self.assertNotIn("gui", vm["modules"])
        names = set(vm["apt_names"])
        for package in (
            "ghostty",
            "code",
            "wl-clipboard",
        ):
            self.assertNotIn(package, names)
        for tool in ("nerd-font", "vscode-extensions"):
            self.assertNotIn(tool, vm["tools"])
        # Exactly the gui packages are dropped.
        dropped = set(full["apt_names"]) - names
        expected = set(self.data["modules"]["gui"]["apt"])
        self.assertEqual(dropped, expected)
        self.assertTrue(set(vm["apt"]) < set(full["apt"]))
        self.assertIn("tmux", vm["apt"])

    def test_agents_module_references_acpx_without_a_version(self):
        agents = self.data["modules"]["agents"]
        self.assertEqual(agents["external"], ["agent_tools.acpx"])
        self.assertNotIn("version", agents)
        lock = (ROOT / self.data["external_lock"]).resolve()
        if not lock.is_file():
            self.skipTest("external/sources.lock.yaml is outside this checkout")
        text = lock.read_text()
        self.assertIn("\nagent_tools:\n", text)
        self.assertIn("\n  acpx:", text)

    def test_unknown_or_uninstallable_profile_fails(self):
        with self.assertRaisesRegex(PROFILES.ProfileError, "Unknown profile"):
            PROFILES.expand("no-such-profile", self.data)
        for name in ("darwin",):
            result = subprocess.run(
                [sys.executable, str(ROOT / "scripts/profile.py"), "apt", name],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 1)
            self.assertIn("no package lock", result.stderr)

    def test_missing_requirement_fails(self):
        data = json.loads(json.dumps(self.data))
        data["profiles"]["broken"] = {"os": "x", "locks": None, "modules": ["agents"]}
        with self.assertRaisesRegex(PROFILES.ProfileError, "requires 'dev'"):
            PROFILES.expand("broken", data)


class InstallerProfileTests(unittest.TestCase):
    def setUp(self):
        self.addCleanup(setattr, WORKSTATION, "SELECTED", None)
        self.addCleanup(WORKSTATION.REPORT.update, {"pending": []})

    def stages_for(self, profile):
        ran = []
        with (
            tempfile.TemporaryDirectory() as temp,
            mock.patch.dict(os.environ, {"WORKSTATION_PROFILE": profile}),
            mock.patch.object(WORKSTATION, "STATE", Path(temp) / "state"),
            mock.patch.object(WORKSTATION, "DATA", Path(temp) / "data"),
            mock.patch.object(WORKSTATION, "stage", lambda name, _: ran.append(name)),
            mock.patch.object(sys, "argv", ["workstation.py", "install"]),
            mock.patch.object(WORKSTATION.os, "geteuid", lambda: 1000),
            mock.patch.object(WORKSTATION.os, "umask"),
            mock.patch.object(WORKSTATION, "save_report", lambda: None),
        ):
            WORKSTATION.main()
        return ran

    def test_full_profile_runs_every_stage_in_order(self):
        self.assertEqual(
            self.stages_for(FULL),
            [
                "CLI tools",
                "tmux plugins",
                "fonts and themes",
                "configuration",
                "Claude plugins",
                "VS Code extensions",
                "existing tmux configuration reload",
            ],
        )

    def test_linux_skips_gui_stages(self):
        self.assertEqual(
            self.stages_for("linux"),
            [
                "CLI tools",
                "tmux plugins",
                "configuration",
                "Claude plugins",
                "existing tmux configuration reload",
            ],
        )

    def test_linux_configuration_applies_its_dotfiles(self):
        WORKSTATION.select_profile("linux")
        with (
            mock.patch.object(WORKSTATION, "apply_dotfiles") as dotfiles,
            mock.patch.object(WORKSTATION, "agent_seeds") as seeds,
        ):
            WORKSTATION.config()
        dotfiles.assert_called_once()
        seeds.assert_called_once()
        self.assertNotIn(WORKSTATION.NO_DOTFILES_LOCK, WORKSTATION.REPORT["pending"])
        self.assertEqual(WORKSTATION.dotfiles_lock()["profile"], "linux")

    def test_linux_verifies_only_its_packages(self):
        WORKSTATION.select_profile("linux")
        self.assertFalse(WORKSTATION.has("gui"))
        self.assertTrue(WORKSTATION.dotfiles_locked())
        checked = {
            command
            for command, module in WORKSTATION.COMMANDS
            if WORKSTATION.has(module)
        }
        self.assertFalse({"ghostty", "code", "wl-copy"} & checked)
        self.assertIn("tmux", checked)

    def test_profile_without_ubuntu_lock_is_refused(self):
        with self.assertRaisesRegex(RuntimeError, "cannot be installed here"):
            WORKSTATION.select_profile("darwin")


if __name__ == "__main__":
    unittest.main()
