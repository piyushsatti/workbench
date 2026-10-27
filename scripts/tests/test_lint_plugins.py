"""lint_plugins.py: a good plugin passes, and each convention it breaks is reported as an error."""

import importlib.util
import json
import subprocess
import tempfile
import unittest
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parents[1]


def load(name):
    spec = importlib.util.spec_from_file_location(name, SCRIPTS / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


MODULE = load("lint_plugins")
WRITE = load("write_catalogs")

VERSION = "2026.10.22"


def write(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content if isinstance(content, str) else json.dumps(content, indent=2))


def sync(root):
    """Write the catalogs the way write_catalogs.py --apply does."""
    with redirect_stdout(StringIO()):
        WRITE.main(["--root", str(root / "pi"), "--source", str(root), "--apply"])


def good_plugin(root, name="demo"):
    """A plugin that follows every rule: three manifests, hooks, MCP, a skill, install.json, catalogs."""
    plugin = root / name
    base = {"name": name, "version": VERSION, "description": "Demo.", "author": {"name": "P"}, "license": "MIT"}
    write(plugin / ".claude-plugin/plugin.json", {**base, "hooks": "./hooks/claude.json", "mcpServers": "./.mcp.json"})
    write(plugin / ".codex-plugin/plugin.json", {**base, "hooks": "./hooks/codex.json", "mcpServers": "./.mcp.json", "skills": ["./skills"]})
    write(plugin / ".cursor-plugin/plugin.json", {**base, "hooks": "./hooks/cursor.json", "mcpServers": "./mcp.json"})
    hook = {"hooks": {"Stop": [{"hooks": [{"type": "command", "command": f'"$HOME/.local/bin/{name}" hook claude stop'}]}]}}
    for harness in ("claude", "codex", "cursor"):
        write(plugin / f"hooks/{harness}.json", hook)
    mcp = json.dumps({"mcpServers": {name: {"command": f"$HOME/.local/bin/{name}", "args": ["mcp"]}}})
    write(plugin / ".mcp.json", mcp)
    write(plugin / "mcp.json", mcp)
    write(plugin / f"skills/{name}/SKILL.md", f"---\nname: {name}\ndescription: Demo skill.\n---\nBody.\n")
    write(plugin / f"bin/{name}", "#!/usr/bin/env python3\n")
    write(plugin / "install.json", {"files": [{"src": f"bin/{name}", "dest": f"~/.local/bin/{name}", "mode": "0755"}]})
    write(plugin / "README.md", "# demo\n\n## Harness support\n")
    write(plugin / "VERIFIED.md", "# Verified\n")
    subprocess.run(["git", "init", "-q", str(plugin)], check=True)
    pi = root / "pi"
    write(pi / "plugins.json", {"plugins": [{"name": name, "github": f"me/{name}", "sha": None}]})
    sync(root)
    return plugin


class LintPluginsTests(unittest.TestCase):
    def run_lint(self, root):
        out = StringIO()
        with redirect_stdout(out):
            status = MODULE.main(["--root", str(root / "pi"), "--source", str(root)])
        return status, out.getvalue()

    def assert_error(self, root, fragment):
        status, output = self.run_lint(root)
        self.assertEqual(status, 1, output)
        self.assertIn(fragment, output)

    def test_good_plugin_passes(self):
        with tempfile.TemporaryDirectory() as temp:
            good_plugin(Path(temp))
            status, output = self.run_lint(Path(temp))
            self.assertEqual(status, 0, output)
            self.assertIn("0 error(s)", output)

    def test_versions_must_agree(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            manifest = json.loads((plugin / ".cursor-plugin/plugin.json").read_text())
            write(plugin / ".cursor-plugin/plugin.json", {**manifest, "version": "2026.10.23"})
            self.assert_error(Path(temp), "version differs")

    def test_name_must_match_folder(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            manifest = json.loads((plugin / ".claude-plugin/plugin.json").read_text())
            write(plugin / ".claude-plugin/plugin.json", {**manifest, "name": "other"})
            self.assert_error(Path(temp), "is not the folder name")

    def test_shared_hooks_json_is_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "hooks/hooks.json", {})
            self.assert_error(Path(temp), "auto-loads this path")

    def test_mcp_twins_must_match(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "mcp.json", "{}")
            self.assert_error(Path(temp), ".mcp.json and mcp.json differ")

    def test_commands_must_not_run_from_plugin_folder(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "hooks/claude.json", {"hooks": {"Stop": [{"hooks": [{"command": "${CLAUDE_PLUGIN_ROOT}/bin/demo"}]}]}})
            self.assert_error(Path(temp), "runs from the plugin folder")

    def test_installed_command_needs_install_entry(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "install.json", {"files": []})
            self.assert_error(Path(temp), "is not installed by install.json")

    def test_skill_depth_and_codex_listing(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "skills/group/nested/SKILL.md", "---\nname: n\ndescription: d\n---\n")
            self.assert_error(Path(temp), "exactly one level under skills/")
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            manifest = json.loads((plugin / ".codex-plugin/plugin.json").read_text())
            write(plugin / ".codex-plugin/plugin.json", {**manifest, "skills": ["./skills/demo"]})
            self.assert_error(Path(temp), 'skills should be ["./skills"]')

    def test_machine_paths_secrets_and_pi_home(self):
        for content, fragment in (
            ("see /Users/someone/notes", "machine path"),
            ("see /home/jdoe/notes", "machine path"),
            ("token ghp_" + "a" * 30, "looks like a secret"),
            ("data in ~/.pi/demo", "belongs to Pi Agent"),
        ):
            with tempfile.TemporaryDirectory() as temp:
                plugin = good_plugin(Path(temp))
                write(plugin / "docs/note.md", content)
                self.assert_error(Path(temp), fragment)

    def test_catalogs_must_match_manifests(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            plugin = good_plugin(root)
            (plugin / ".codex-plugin/plugin.json").unlink()
            (plugin / "hooks/codex.json").unlink()
            self.assert_error(root, "lists demo, which ships no codex manifest")
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            good_plugin(root)
            write(root / "pi/.claude-plugin/marketplace.json", {"name": "commons", "plugins": []})
            status, output = self.run_lint(root)
            self.assertIn("is not 'pi'", output)
            self.assertIn("does not list demo", output)

    def test_remote_entries_point_at_the_listed_repo_and_pin(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            good_plugin(root)
            catalog = json.loads((root / "pi/.claude-plugin/marketplace.json").read_text())
            self.assertEqual(catalog["plugins"][0]["source"], {"source": "github", "repo": "me/demo"})
            codex = json.loads((root / "pi/.agents/plugins/marketplace.json").read_text())
            self.assertEqual(codex["plugins"][0]["source"], {"source": "url", "url": "https://github.com/me/demo.git"})
            write(root / "pi/plugins.json", {"plugins": [{"name": "demo", "github": "me/demo", "sha": "abc123"}]})
            self.assert_error(root, "not pinned to plugins.json sha abc123")
            sync(root)
            self.assertEqual(self.run_lint(root)[0], 0)

    def test_third_party_plugins_are_checked_only_against_the_catalogs(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            plugin = root / "other"
            base = {"name": "other", "version": "5.3.2", "description": "Theirs."}
            write(plugin / ".claude-plugin/plugin.json", {**base, "hooks": ["./hooks/a.json", "./hooks/b.json"]})
            write(plugin / ".codex-plugin/plugin.json", {**base, "hooks": "./hooks/a.json"})
            subprocess.run(["git", "init", "-q", str(plugin)], check=True)
            write(root / "pi/plugins.json", {"owner": "me", "plugins": [{"name": "other", "github": "them/other", "sha": "abc123"}]})
            sync(root)
            status, output = self.run_lint(root)
            self.assertEqual(status, 0, output)
            write(root / "pi/plugins.json", {"owner": "me", "plugins": [{"name": "other", "github": "me/other", "sha": "abc123"}]})
            self.assert_error(root, "is not CalVer")

    def test_cursor_gets_no_catalog_and_no_copies(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            good_plugin(root)
            self.assertFalse((root / "pi/.cursor-plugin").exists())
            self.assertFalse((root / "pi/plugins").exists())
            self.assertEqual(self.run_lint(root)[0], 0)

    def test_placeholder_home_paths_are_allowed(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "tests/fixture.md", "home is /home/u and /Users/me")
            sync(Path(temp))
            status, output = self.run_lint(Path(temp))
            self.assertEqual(status, 0, output)

    def test_readme_needs_harness_table(self):
        with tempfile.TemporaryDirectory() as temp:
            plugin = good_plugin(Path(temp))
            write(plugin / "README.md", "# demo\n")
            self.assert_error(Path(temp), "Harness support")


if __name__ == "__main__":
    unittest.main()
