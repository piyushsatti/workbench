"""deploy_agent.py: dry run writes nothing, apply copies and backs up, CLAUDE.md and a hand-edited
~/.agents/AGENTS.md, ~/.codex/AGENTS.md or ~/.config/opencode/AGENTS.md are never written, and subagents convert for Codex,
Cursor and OpenCode."""

import importlib.util
import json
import tempfile
import tomllib
import unittest
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location("deploy_agent", Path(__file__).resolve().parents[1] / "deploy_agent.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DeployAgentTests(unittest.TestCase):
    def run_main(self, home, *argv):
        out = StringIO()
        with mock.patch.object(MODULE, "HOME", home), mock.patch("sys.argv", ["deploy_agent.py", *argv]), redirect_stdout(out):
            MODULE.main()
        return out.getvalue()

    def test_dry_run_writes_nothing(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            output = self.run_main(home)
            self.assertIn("NEW", output)
            self.assertIn("MISSING", output)
            self.assertEqual(list(home.iterdir()), [])

    def test_apply_copies_backs_up_and_leaves_claude_md(self):
        with tempfile.TemporaryDirectory() as temp, mock.patch.object(MODULE, "committed_rules", lambda: {b"old rules\n"}):
            home = Path(temp)
            (home / ".agents").mkdir()
            (home / ".agents/AGENTS.md").write_text("old rules\n")
            (home / ".claude").mkdir()
            (home / ".claude/CLAUDE.md").write_text("my own file\n")
            self.run_main(home, "--apply")
            self.assertEqual((home / ".agents/AGENTS.md").read_text(), (MODULE.AGENT / "AGENTS.md").read_text())
            self.assertEqual((home / ".agents/AGENTS.md.pre-deploy").read_text(), "old rules\n")
            self.assertEqual((home / ".claude/CLAUDE.md").read_text(), "my own file\n")
            self.assertTrue((home / ".claude/agents/scrutineer.md").is_file())
            self.assertIn("up to date", self.run_main(home))

    def test_hand_edited_shared_rules_are_never_overwritten(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            shared = home / ".agents/AGENTS.md"
            shared.parent.mkdir()
            edited = (MODULE.AGENT / "AGENTS.md").read_text() + "Memory: read this machine's index.\n"
            shared.write_text(edited)
            output = self.run_main(home, "--apply")
            self.assertIn(f"CHECK   {shared} has its own content, left as is", output)
            self.assertNotIn(f"CHANGED  {shared}", output)
            self.assertEqual(shared.read_text(), edited)
            self.assertFalse((home / ".agents/AGENTS.md.pre-deploy").exists())
            # The harness copies still get the canonical rules, not the local line.
            self.assertEqual((home / ".codex/AGENTS.md").read_text(), (MODULE.AGENT / "AGENTS.md").read_text())

    def test_claude_settings_are_checked_for_the_status_line_and_mods(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            settings = home / ".claude/settings.json"
            settings.parent.mkdir()
            settings.write_text('{"env": {"CLAUDE_CODE_PLUGIN_DIRS": "~/mods/other"}}')
            output = self.run_main(home, "--harness", "claude", "--apply")
            self.assertIn('"command": "$HOME/.local/bin/claude-statusline"', output)
            self.assertIn('"CLAUDE_CODE_PLUGIN_DIRS": "~/mods/other:~/.local/share/claude-mods/cc-status"', output)
            self.assertEqual(settings.read_text(), '{"env": {"CLAUDE_CODE_PLUGIN_DIRS": "~/mods/other"}}')
            self.assertTrue((home / ".local/bin/claude-statusline").stat().st_mode & 0o100)
            self.assertTrue((home / ".local/share/claude-mods/cc-status/hooks/register.tsx").is_file())
            settings.write_text(json.dumps({
                "statusLine": {"type": "command", "command": "$HOME/.local/bin/claude-statusline"},
                "env": {"CLAUDE_CODE_PLUGIN_DIRS": f"{home}/.local/share/claude-mods/cc-status"}}))
            output = self.run_main(home, "--harness", "claude")
            self.assertNotIn("status line", output)
            self.assertNotIn("does not load", output)

    def test_committed_rules_include_the_current_version(self):
        known = MODULE.committed_rules()
        if not known:
            self.skipTest("no git history in this checkout")
        self.assertIn((MODULE.AGENT / "AGENTS.md").read_bytes(), known)

    def test_skills_deploy_without_their_tests_and_executables(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            self.run_main(home, "--apply")
            for root in (".agents/skills", ".claude/skills"):
                self.assertTrue((home / root / "dream/SKILL.md").is_file(), root)
                self.assertFalse((home / root / "dream/bin").exists(), root)
                self.assertFalse((home / root / "dream/tests").exists(), root)
                self.assertFalse((home / root / "README.md").exists(), root)
                self.assertFalse((home / root / "tests").exists(), root)
            self.assertEqual((home / ".local/bin/dream-replay").stat().st_mode & 0o777, 0o755)

    def test_skill_runtime_bin_tests_and_install_json_stay_behind(self):
        with tempfile.TemporaryDirectory() as temp:
            agent = Path(temp)
            for rel in ("SKILL.md", "runtime/scripts/demo.py", "bin/demo", "tests/test_demo.py", "install.json"):
                (agent / "skills/demo" / rel).parent.mkdir(parents=True, exist_ok=True)
                (agent / "skills/demo" / rel).write_text("x")
            with mock.patch.object(MODULE, "AGENT", agent):
                deployed = [target for _, target, _ in MODULE.mapped(agent / "skills", Path("/dest"), None, None)]
            self.assertEqual(deployed, [Path("/dest/demo/SKILL.md")])

    def test_extra_files_are_reported_not_removed(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            (home / ".claude/agents").mkdir(parents=True)
            (home / ".claude/agents/mine.md").write_text("x")
            self.assertIn("EXTRA", self.run_main(home))
            self.assertTrue((home / ".claude/agents/mine.md").exists())

    def test_plugin_executables_install_with_their_mode(self):
        with tempfile.TemporaryDirectory() as temp:
            home, plugins = Path(temp) / "home", Path(temp) / "plugins"
            (home).mkdir()
            (plugins / "demo/bin").mkdir(parents=True)
            (plugins / "demo/bin/demo").write_text("#!/bin/sh\n")
            (plugins / "demo/install.json").write_text(
                '{"files": [{"src": "bin/demo", "dest": "~/.local/bin/demo", "mode": "0755"}], "builds": [{"build": "make", "dest": "~/.local/bin/x"}]}')
            with mock.patch.object(MODULE, "PLUGINS", plugins):
                dry = self.run_main(home)
                self.assertIn(f"run `make` in {plugins / 'demo'} (not run here)", dry)
                self.assertFalse((home / ".local/bin/demo").exists())
                self.run_main(home, "--apply")
            self.assertEqual((home / ".local/bin/demo").stat().st_mode & 0o777, 0o755)

    def test_git_guard_installs_from_agent_hooks(self):
        with tempfile.TemporaryDirectory() as temp:
            with mock.patch.object(MODULE, "HOME", Path(temp)):
                files = MODULE.plugin_installs()[0]
            self.assertIn((MODULE.AGENT / "hooks/git-guard.py", Path(temp) / ".local/bin/git-guard", 0o755), files)

    def test_harness_filter_limits_destinations(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            codex = self.run_main(home, "--harness", "codex")
            self.assertIn(".codex/agents/architect.toml", codex)
            self.assertIn(".agents/AGENTS.md", codex)
            self.assertNotIn(".claude/agents", codex)
            self.assertNotIn(".cursor/agents", codex)
            self.assertNotIn("CLAUDE.md", codex)
            cursor = self.run_main(home, "--harness", "cursor")
            self.assertIn(".cursor/agents/architect.md", cursor)
            self.assertIn("User Rules", cursor)
            self.assertNotIn(".codex/", cursor)
            default = self.run_main(home)
            opencode = self.run_main(home, "--harness", "opencode")
            self.assertIn(".config/opencode/agents/architect.md", opencode)
            self.assertIn(".config/opencode/AGENTS.md", opencode)
            self.assertNotIn(".codex/", opencode)
            for path in (".claude/agents/architect.md", ".codex/agents/architect.toml", ".cursor/agents/architect.md",
                         ".config/opencode/agents/architect.md"):
                self.assertIn(path, default)
            with self.assertRaises(SystemExit), mock.patch("sys.stderr", StringIO()):
                self.run_main(home, "--harness", "pi")

    def test_codex_agents_are_valid_toml_with_the_body(self):
        for source in sorted((MODULE.AGENT / "subagents").glob("*.md")):
            fields, body = MODULE.frontmatter(source.read_text())
            data = tomllib.loads(MODULE.to_codex(source.name, source.read_text()))
            self.assertEqual(data["name"], fields["name"])
            self.assertEqual(data["description"], fields["description"])
            self.assertEqual(data["developer_instructions"], body)
            self.assertNotIn("model", data)
            has_shell = "Bash" in MODULE.tools_of(fields)
            self.assertEqual(data.get("features", {}).get("shell_tool", True), has_shell)

    def test_frontmatter_reads_folded_descriptions_and_escapes_bodies(self):
        fields, _ = MODULE.frontmatter("---\nname: x\ndescription: >-\n  one\n  two: three\ntools: Read, Bash\n---\nBody\n")
        self.assertEqual(fields["description"], "one two: three")
        self.assertEqual(MODULE.tools_of(fields), {"Read", "Bash"})
        awkward = "Body ''' and \"\"\" and \\ end\n"
        data = tomllib.loads(MODULE.to_codex("x.md", "---\nname: x\ndescription: d\n---\n" + awkward))
        self.assertEqual(data["developer_instructions"], awkward)

    def test_cursor_agents_get_cursor_frontmatter(self):
        text = (MODULE.AGENT / "subagents/architect.md").read_text()
        fields, body = MODULE.frontmatter(MODULE.to_cursor("subagents/architect.md", text))
        self.assertEqual(fields["model"], "inherit")
        self.assertEqual(fields["readonly"], "true")
        self.assertNotIn("tools", fields)
        self.assertEqual(body, MODULE.frontmatter(text)[1])
        scrutineer = MODULE.to_cursor("s.md", (MODULE.AGENT / "subagents/scrutineer.md").read_text())
        self.assertIn("readonly: false", scrutineer)

    def test_opencode_agents_get_opencode_frontmatter(self):
        text = (MODULE.AGENT / "subagents/architect.md").read_text()
        converted = MODULE.to_opencode("subagents/architect.md", text)
        fields, body = MODULE.frontmatter(converted)
        self.assertEqual(fields["mode"], "subagent")
        self.assertEqual(fields["description"], MODULE.frontmatter(text)[0]["description"])
        self.assertNotIn("model", fields)
        self.assertNotIn("name", fields)
        self.assertIn("permission:\n  edit: deny\n---", converted)
        self.assertEqual(body, MODULE.frontmatter(text)[1])
        no_shell = MODULE.to_opencode("x.md", "---\nname: x\ndescription: d\ntools: Read\n---\nBody\n")
        self.assertIn("permission:\n  edit: deny\n  bash: deny\n---", no_shell)
        unrestricted = MODULE.to_opencode("x.md", "---\nname: x\ndescription: d\n---\nBody\n")
        self.assertNotIn("permission", unrestricted)

    def test_opencode_rules_follow_the_codex_rules(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            self.run_main(home, "--harness", "opencode", "--apply")
            rules = home / ".config/opencode/AGENTS.md"
            self.assertEqual(rules.read_text(), (MODULE.AGENT / "AGENTS.md").read_text())
            self.assertIn("up to date", self.run_main(home, "--harness", "opencode"))
            rules.write_text("my own rules\n")
            self.assertIn("OpenCode reads its literal text", self.run_main(home, "--harness", "opencode", "--apply"))
            self.assertEqual(rules.read_text(), "my own rules\n")

    def test_codex_agents_md_is_copied_when_missing_and_never_overwritten(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            self.run_main(home, "--harness", "codex", "--apply")
            self.assertEqual((home / ".codex/AGENTS.md").read_text(), (MODULE.AGENT / "AGENTS.md").read_text())
            self.assertFalse((home / ".codex/AGENTS.md").is_symlink())
            self.assertTrue(tomllib.loads((home / ".codex/agents/scrutineer.toml").read_text()))
            self.assertIn("up to date", self.run_main(home, "--harness", "codex"))
            (home / ".codex/AGENTS.md").write_text("Read `~/.agents/AGENTS.md`.\n")
            output = self.run_main(home, "--harness", "codex", "--apply")
            self.assertIn("has its own content", output)
            self.assertEqual((home / ".codex/AGENTS.md").read_text(), "Read `~/.agents/AGENTS.md`.\n")
            self.assertFalse((home / ".codex/AGENTS.md.pre-deploy").exists())

    def test_stale_deployed_codex_copy_is_updated_with_a_backup(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            for path in (home / ".agents/AGENTS.md", home / ".codex/AGENTS.md"):
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("old rules\n")
            self.assertIn(f"CHANGED  {home / '.codex/AGENTS.md'}", self.run_main(home, "--harness", "codex"))
            self.run_main(home, "--harness", "codex", "--apply")
            self.assertEqual((home / ".codex/AGENTS.md").read_text(), (MODULE.AGENT / "AGENTS.md").read_text())
            self.assertEqual((home / ".codex/AGENTS.md.pre-deploy").read_text(), "old rules\n")

    def test_converted_changes_back_up_and_extras_stay(self):
        with tempfile.TemporaryDirectory() as temp:
            home = Path(temp)
            (home / ".codex/agents").mkdir(parents=True)
            (home / ".codex/agents/scrutineer.toml").write_text('name = "scrutineer"\n')
            (home / ".codex/agents/mine.toml").write_text('name = "mine"\n')
            output = self.run_main(home, "--harness", "codex", "--apply")
            self.assertIn(f"CHANGED  {home / '.codex/agents/scrutineer.toml'}", output)
            self.assertIn(f"EXTRA    {home / '.codex/agents/mine.toml'}", output)
            self.assertNotIn("architect.toml  (not", output)
            self.assertEqual((home / ".codex/agents/scrutineer.toml.pre-deploy").read_text(), 'name = "scrutineer"\n')
            self.assertTrue((home / ".codex/agents/mine.toml").exists())
            self.assertIn("up to date", self.run_main(home, "--harness", "codex"))


if __name__ == "__main__":
    unittest.main()
