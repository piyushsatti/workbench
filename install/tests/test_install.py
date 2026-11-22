"""install.py: the catalog holds together, picks resolve, the menu parses, the system is detected, and a plan
installs only what is missing. Nothing here runs a package manager.

Run from the repo root: uv run python -B -m unittest discover -s install/tests
"""

import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(HERE))

import steps
from adapters import SYSTEMS, Action, Unsupported, detect
from adapters.arch import Arch
from adapters.macos import Macos
from adapters.ubuntu import Ubuntu

import install

CATALOG = install.load_catalog()
APT_TXT = install.REPO / "setup/workstation/profiles/ubuntu-26.04/apt.txt"
MANAGERS = {"macos": {"brew", "cask"}, "ubuntu": {"apt"}, "arch": {"pacman", "aur"}}

UBUNTU = 'NAME="Ubuntu"\nID=ubuntu\nVERSION_ID="26.04"\n'
ARCH = 'NAME="Arch Linux"\nID=arch\n'
ENDEAVOUR = 'NAME="EndeavourOS"\nID=endeavouros\nID_LIKE=arch\n'
FEDORA = 'NAME="Fedora Linux"\nID=fedora\nPRETTY_NAME="Fedora Linux 43"\n'


class CatalogTests(unittest.TestCase):
    def test_items_are_well_formed(self):
        names = list(CATALOG["items"])
        for index, (name, item) in enumerate(CATALOG["items"].items()):
            self.assertIn(item["group"], CATALOG["groups"], name)
            self.assertTrue(item["description"], name)
            for required in item.get("requires", []):
                self.assertIn(required, names[:index], f"{name} requires {required}, which must be listed earlier")
            for system in SYSTEMS:
                for manager in item.get(system, {}):
                    self.assertIn(manager, MANAGERS[system] | {"steps", "before"}, (name, system))
            for system in SYSTEMS:
                for step in install.item_steps(CATALOG, system, name) + install.item_steps(CATALOG, system, name, "before"):
                    self.assertIn(step[0], steps.STEPS, (name, step))

    def test_bundles_name_known_items(self):
        for bundle in CATALOG["bundles"]:
            for name in install.expand_bundle(CATALOG, bundle):
                self.assertIn(name, CATALOG["items"], bundle)

    def test_tools_bundle_on_ubuntu_is_the_workstation_apt_list(self):
        selection = install.resolve(CATALOG, "ubuntu", ["tools"])
        apt = install.packages(CATALOG, "ubuntu", selection.items)["apt"]
        listed = {line.split()[0] for line in APT_TXT.read_text().splitlines() if line.strip() and not line.startswith("#")}
        self.assertEqual(set(apt), listed)
        self.assertEqual(len(apt), len(set(apt)))


class ResolveTests(unittest.TestCase):
    def test_bundle_includes_another_bundle(self):
        agent = install.expand_bundle(CATALOG, "tools+agent")
        self.assertEqual(agent[: len(install.expand_bundle(CATALOG, "tools"))], install.expand_bundle(CATALOG, "tools"))
        self.assertIn("agent-layer", agent)

    def test_requires_pull_items_in_and_keep_catalog_order(self):
        selection = install.resolve(CATALOG, "macos", picks=["claude-plugins"])
        self.assertEqual(selection.items, ["node", "claude", "claude-plugins"])

    def test_no_gui_drops_gui_from_bundles_only(self):
        selection = install.resolve(CATALOG, "ubuntu", ["tools"], picks=["ghostty"], no_gui=True)
        self.assertIn("ghostty", selection.items)
        self.assertNotIn("vscode", selection.items)
        self.assertIn(("vscode", "--no-gui"), selection.left_out)

    def test_omarchy_skips_what_it_ships_unless_picked(self):
        selection = install.resolve(CATALOG, "arch", ["tools"], flavor="omarchy")
        self.assertNotIn("dotfiles", selection.items)
        self.assertNotIn("nerd-font", selection.items)
        picked = install.resolve(CATALOG, "arch", ["tools"], picks=["dotfiles"], flavor="omarchy")
        self.assertIn("dotfiles", picked.items)

    def test_items_not_offered_are_reported(self):
        selection = install.resolve(CATALOG, "ubuntu", picks=["cursor", "git"])
        self.assertEqual(selection.items, ["git"])
        self.assertEqual([n for n, _ in selection.unavailable], ["cursor"])

    def test_a_bundle_name_in_picks_counts_as_a_bundle(self):
        self.assertIn("tmux", install.resolve(CATALOG, "arch", picks=["tools"]).items)

    def test_unknown_names_are_errors(self):
        with self.assertRaises(install.PickError):
            install.resolve(CATALOG, "macos", picks=["nope"])
        with self.assertRaises(install.PickError):
            install.resolve(CATALOG, "macos", ["nope"])


class MenuTests(unittest.TestCase):
    def test_numbers_ranges_and_names(self):
        _, numbered = install.menu_lines(CATALOG, "macos")
        picks = install.parse_answer("1, 3-4 tmux tools+agent", numbered, CATALOG)
        self.assertEqual(picks, [numbered[0], numbered[2], numbered[3], "tmux", "tools+agent"])

    def test_bad_answers_are_refused(self):
        _, numbered = install.menu_lines(CATALOG, "macos")
        for answer in ("0", str(len(numbered) + 1), "5-2", "zzz"):
            with self.assertRaises(install.PickError, msg=answer):
                install.parse_answer(answer, numbered, CATALOG)

    def test_menu_lists_only_this_systems_items(self):
        _, numbered = install.menu_lines(CATALOG, "ubuntu")
        self.assertNotIn("cursor", numbered)
        self.assertIn("cursor", install.menu_lines(CATALOG, "macos")[1])


class DetectTests(unittest.TestCase):
    def setUp(self):
        self.home = tempfile.TemporaryDirectory()
        self.addCleanup(self.home.cleanup)

    def detect(self, text, which=lambda name: None):
        return detect("Linux", text, self.home.name, which)

    def test_systems(self):
        self.assertEqual(detect("Darwin"), ("macos", None))
        self.assertEqual(self.detect(UBUNTU), ("ubuntu", None))
        self.assertEqual(self.detect(ARCH), ("arch", None))
        self.assertEqual(self.detect(ENDEAVOUR), ("arch", None))

    def test_omarchy_by_command_or_folder(self):
        self.assertEqual(self.detect(ARCH, lambda name: "/usr/bin/x" if name == "omarchy-pkg-add" else None),
                         ("arch", "omarchy"))
        (Path(self.home.name) / ".local/share/omarchy").mkdir(parents=True)
        self.assertEqual(self.detect(ARCH), ("arch", "omarchy"))

    def test_unsupported(self):
        with self.assertRaisesRegex(Unsupported, "Fedora Linux 43"):
            self.detect(FEDORA)
        with self.assertRaises(Unsupported):
            detect("Windows")


class FakeAdapter:
    name = "arch"
    managers = ("pacman", "aur")

    def __init__(self, have=(), aur=True):
        self.have, self.aur = set(have), aur

    def prepare(self):
        return [Action("prepare")]

    def unavailable(self, manager):
        return None if self.aur or manager != "aur" else "no helper"

    def installed(self, manager, names):
        return {n for n in names if n in self.have}

    def install(self, manager, names):
        return [Action(f"{manager} {' '.join(names)}")]


class PlanTests(unittest.TestCase):
    def context(self, system, items, adapter, which=lambda name: None):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        return steps.Context(install.REPO, temp.name, system, None, adapter, items, which)

    def test_only_missing_packages_install(self):
        adapter = FakeAdapter(have={"git", "tmux"})
        selection = install.resolve(CATALOG, "arch", picks=["git", "jq", "tmux"])
        actions, notes = install.plan(CATALOG, "arch", selection, adapter, self.context("arch", selection.items, adapter))
        self.assertEqual([a.label for a in actions], ["prepare", "pacman jq"])
        self.assertIn("already installed (pacman): git tmux", notes)

    def test_aur_without_a_helper_is_skipped_with_a_note(self):
        adapter = FakeAdapter(aur=False)
        selection = install.resolve(CATALOG, "arch", picks=["cursor"])
        actions, notes = install.plan(CATALOG, "arch", selection, adapter, self.context("arch", selection.items, adapter))
        self.assertEqual([a.label for a in actions], ["prepare"])
        self.assertTrue(any("cursor-bin" in n and "no helper" in n for n in notes))

    def test_before_steps_come_before_packages_and_steps_after(self):
        adapter = Ubuntu()
        selection = install.resolve(CATALOG, "ubuntu", picks=["vscode", "fd"])
        with mock.patch.object(steps, "CODE_SOURCES", ()):
            actions, _ = install.plan(CATALOG, "ubuntu", selection, adapter,
                                      self.context("ubuntu", selection.items, adapter), check_installed=False)
        labels = [a.label for a in actions]
        self.assertEqual(labels[0], "apt-get update")
        self.assertTrue(labels[1].startswith("add Microsoft's VS Code apt repository"))
        self.assertEqual(labels[2], "apt-get install fd-find code python3-json5")
        self.assertEqual(labels[3], "write ~/.local/bin/fd -> fdfind")
        self.assertTrue(actions[2].sudo)

    def test_agent_layer_deploys_for_the_picked_harnesses(self):
        ctx = self.context("macos", ["claude", "agent-layer"], Macos())
        argv = steps.agent_layer(ctx)[0].argv
        self.assertEqual(argv[-2:], ["--harness", "claude"])
        self.assertEqual(steps.agent_layer(self.context("macos", ["agent-layer"], Macos()))[0].argv[-1], "claude,codex,cursor")

    def test_an_installed_cli_is_kept(self):
        ctx = self.context("macos", ["claude"], Macos(), which=lambda name: "/usr/local/bin/claude")
        self.assertEqual(steps.npm_cli(ctx, "claude", "@anthropic-ai/claude-code"), [])
        self.assertIn("kept the installed claude", ctx.notes[0])

    def test_claude_plugin_list_comes_from_the_workstation_defaults(self):
        wanted = steps.claude_plugins_wanted(install.REPO)
        self.assertIn("skill-creator@claude-plugins-official", wanted)


class AdapterTests(unittest.TestCase):
    def test_arch_upgrades_the_whole_system(self):
        with mock.patch("shutil.which", lambda name: None):
            self.assertEqual(Arch().upgrade({"pacman": ["git"]})[0].argv, ["pacman", "-Syu", "--noconfirm"])
        with mock.patch("shutil.which", lambda name: "/usr/bin/yay" if name == "yay" else None):
            self.assertEqual(Arch().upgrade({})[0].argv, ["yay", "-Syu", "--noconfirm"])
            self.assertEqual(Arch().install("aur", ["cursor-bin"])[0].argv[:3], ["yay", "-S", "--needed"])

    def test_macos_finds_brew_at_run_time(self):
        with mock.patch.object(Macos, "brew", lambda self: "/opt/homebrew/bin/brew"):
            self.assertEqual(Macos().resolve(["brew", "install", "git"]), ["/opt/homebrew/bin/brew", "install", "git"])
            self.assertEqual(Macos().prepare(), [])

    def test_ubuntu_upgrade_only_upgrades(self):
        self.assertIn("--only-upgrade", Ubuntu().upgrade({"apt": ["git"]})[1].argv)


class StateTests(unittest.TestCase):
    def test_picks_are_remembered_and_merged(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            install.write_state(home, "macos", ["git", "tmux"])
            install.write_state(home, "macos", ["tmux", "jq"])
            state = json.loads(install.state_path(home).read_text())
            self.assertEqual(state["items"], ["git", "tmux", "jq"])
            self.assertEqual(install.read_state(home / "missing"), {})


if __name__ == "__main__":
    unittest.main()
