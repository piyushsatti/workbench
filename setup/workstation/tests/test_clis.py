"""CLIs the installer manages: an installed one is kept; a missing one gets the latest."""

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location(
    "workstation_clis", Path(__file__).resolve().parents[1] / "scripts/workstation.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class CliTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.home = Path(temp.name)
        (self.home / ".local/bin").mkdir(parents=True)
        self.calls = []
        for name, value in (("HOME", self.home), ("DATA", self.home / ".local/share/workstation"),
                            ("REPORT", {"pending": []}), ("SELECTED", MODULE.PROFILES.expand("linux-full"))):
            patcher = mock.patch.object(MODULE, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)
        run = mock.patch.object(MODULE, "run", side_effect=self.fake_run)
        run.start()
        self.addCleanup(run.stop)
        which = mock.patch.object(MODULE.shutil, "which", return_value=None)
        which.start()
        self.addCleanup(which.stop)

    def fake_run(self, *args, **kwargs):
        self.calls.append([str(a) for a in args])
        if args[:2] == ("npm", "install"):
            bin_dir = Path(args[args.index("--prefix") + 1]) / "node_modules/.bin"
            bin_dir.mkdir(parents=True, exist_ok=True)
            for spec in args[args.index("--prefix") + 2:]:
                name = next(n for n, p in MODULE.HARNESS_PACKAGES.items() if spec == f"{p}@latest")
                (bin_dir / name).write_text("")
        return ""

    def installed(self, name):
        real = self.home / "elsewhere" / name
        real.parent.mkdir(exist_ok=True)
        real.write_text("#!/bin/sh\n")
        path = self.home / ".local/bin" / name
        path.symlink_to(real)
        return path

    def test_a_dangling_link_or_our_broken_wrapper_is_reinstalled(self):
        (self.home / ".local/bin/claude").symlink_to("/opt/gone/claude")
        target = MODULE.DATA / "npm/node_modules/.bin/codex"
        (self.home / ".local/bin/codex").write_text(MODULE.npm_wrapper(target))  # its npm install was cleared
        self.installed("opencode")
        with mock.patch.object(MODULE, "write"):
            MODULE.harness_clis()
        [npm] = self.calls
        self.assertEqual(npm[-2:], ["@anthropic-ai/claude-code@latest", "@openai/codex@latest"])

    def test_installed_clis_are_kept_and_reported(self):
        for name in ("claude", "codex", "opencode", "uv", "chezmoi"):
            self.installed(name)
        MODULE.harness_clis()
        MODULE.uv_tool()
        MODULE.chezmoi_tool()
        self.assertEqual(self.calls, [])
        self.assertTrue((self.home / ".local/bin/claude").is_symlink())
        self.assertEqual(len(MODULE.REPORT["kept"]), 5)

    def test_missing_chezmoi_is_checked_against_the_pinned_checksum_only(self):
        downloads = []

        def download(url, target, checksum=None):
            downloads.append((url, checksum))
            raise RuntimeError("stop before extracting")

        with (
            mock.patch.object(MODULE, "download", side_effect=download),
            mock.patch.object(MODULE.os, "uname", return_value=mock.Mock(machine="aarch64")),
            self.assertRaisesRegex(RuntimeError, "stop before extracting"),
        ):
            MODULE.chezmoi_tool()
        version = MODULE.VERSIONS["chezmoi"]
        self.assertEqual(downloads, [(
            f"https://github.com/twpayne/chezmoi/releases/download/v{version}/chezmoi_{version}_linux_arm64.tar.gz",
            MODULE.VERSIONS["chezmoiSha256"]["arm64"],
        )])

    def test_chezmoi_pins_every_architecture_and_refuses_an_unknown_one(self):
        self.assertEqual(set(MODULE.VERSIONS["chezmoiSha256"]), {"amd64", "arm64"})
        for digest in MODULE.VERSIONS["chezmoiSha256"].values():
            self.assertRegex(digest, r"^[0-9a-f]{64}$")
        with (
            mock.patch.object(MODULE.os, "uname", return_value=mock.Mock(machine="riscv64")),
            self.assertRaisesRegex(RuntimeError, "no pinned chezmoi for riscv64"),
        ):
            MODULE.chezmoi_tool()

    def test_only_missing_agent_clis_get_the_latest(self):
        self.installed("claude")
        with mock.patch.object(MODULE, "write") as write:
            MODULE.harness_clis()
        [npm] = self.calls
        self.assertEqual(npm[:2], ["npm", "install"])
        self.assertEqual(npm[-2:], ["@openai/codex@latest", "opencode-ai@latest"])
        self.assertEqual({c.args[0].name for c in write.call_args_list}, {"codex", "opencode"})

    def test_only_the_profiles_harnesses_are_installed_and_verified(self):
        with mock.patch.dict(MODULE.SELECTED, harnesses=["claude"]), mock.patch.object(MODULE, "write"):
            MODULE.harness_clis()
            self.assertEqual(MODULE.installed_clis(), ["uv", "claude"])
        [npm] = self.calls
        self.assertEqual([arg for arg in npm if arg.endswith("@latest")], ["@anthropic-ai/claude-code@latest"])

    def test_profile_harnesses_are_verified_with_uv(self):
        self.assertEqual(MODULE.installed_clis(), ["uv", "claude", "codex", "opencode"])

    def test_harness_without_a_known_package_fails(self):
        with mock.patch.dict(MODULE.SELECTED, harnesses=["cursor"]), self.assertRaisesRegex(RuntimeError, "cursor"):
            MODULE.harness_clis()

    def test_missing_uv_gets_the_latest(self):
        MODULE.uv_tool()
        self.assertEqual(self.calls, [["pipx", "install", "uv"]])

    def test_a_cli_on_path_counts_as_installed(self):
        with mock.patch.object(MODULE.shutil, "which", return_value="/usr/bin/uv"):
            MODULE.uv_tool()
        self.assertEqual(self.calls, [])


class ClaudePluginTests(unittest.TestCase):
    WANTED = ["a@claude-plugins-official", "b@claude-plugins-official"]

    def setUp(self):
        self.calls = []
        self.installed = []
        self.marketplaces = ["claude-plugins-official"]
        for name, value in (("SELECTED", {"claude_plugins": self.WANTED}),
                            ("cli_path", lambda name: Path("/opt/bin/claude"))):
            patcher = mock.patch.object(MODULE, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)
        run = mock.patch.object(MODULE, "run", side_effect=self.fake_run)
        run.start()
        self.addCleanup(run.stop)

    def fake_run(self, *args, **kwargs):
        args = [str(a) for a in args]
        if args[1:3] == ["plugin", "list"]:
            return json.dumps([{"id": plugin} for plugin in self.installed])
        if args[1:4] == ["plugin", "marketplace", "list"]:
            return json.dumps([{"name": name} for name in self.marketplaces])
        self.calls.append(args[1:])
        return ""

    def test_only_missing_plugins_are_installed(self):
        self.installed = ["a@claude-plugins-official"]
        MODULE.claude_plugins()
        self.assertEqual(self.calls, [["plugin", "install", "--scope", "user", "b@claude-plugins-official"]])

    def test_nothing_runs_when_all_are_installed(self):
        self.installed = list(self.WANTED)
        MODULE.claude_plugins()
        self.assertEqual(self.calls, [])

    def test_a_missing_marketplace_is_added_first(self):
        self.marketplaces = []
        MODULE.claude_plugins()
        self.assertEqual(self.calls[0], ["plugin", "marketplace", "add", "anthropics/claude-plugins-official"])
        self.assertEqual(len(self.calls), 3)

    def test_no_claude_means_nothing_to_do(self):
        with mock.patch.object(MODULE, "cli_path", return_value=None):
            MODULE.claude_plugins()
        self.assertEqual(self.calls, [])



class DockerGroupTests(unittest.TestCase):
    def pending(self, members, gid, my_groups):
        group = mock.Mock(gr_mem=members, gr_gid=gid)
        with mock.patch.object(MODULE.grp, "getgrnam", return_value=group), \
                mock.patch.object(MODULE.os, "getgroups", return_value=my_groups), \
                mock.patch.dict(MODULE.os.environ, {"USER": "me"}):
            return MODULE.docker_group_pending()

    def test_joined_but_not_yet_logged_in_again(self):
        self.assertTrue(self.pending(["me"], 999, [20, 1000]))

    def test_session_already_has_the_group(self):
        self.assertFalse(self.pending(["me"], 999, [20, 999]))

    def test_not_a_member(self):
        self.assertFalse(self.pending(["someone"], 999, [20]))

if __name__ == "__main__":
    unittest.main()
