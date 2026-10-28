"""Tests for tools/snippets: run the script as a subprocess against throwaway folders."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "snippets.py"
sys.path.insert(0, str(SCRIPT.parent))
import snippets  # noqa: E402

VERIFY = "---\ntitle: Verify\ndescription: Do not assume\nposition: append\n---\nCheck each claim.\n"
KICKOFF = "---\ntitle: Kickoff\nposition: prepend\n---\nGet oriented first.\n"


class ParseTest(unittest.TestCase):
    def test_frontmatter_then_body(self):
        self.assertEqual(snippets.parse("verify", VERIFY), {
            "slug": "verify", "title": "Verify", "description": "Do not assume",
            "position": "append", "body": "Check each claim.",
        })

    def test_no_frontmatter_is_all_body_appended(self):
        self.assertEqual(snippets.parse("plain", "Just this.\n"), {
            "slug": "plain", "title": "plain", "description": "", "position": "append", "body": "Just this.",
        })

    def test_context_prepends_first_only_enabled(self):
        all_ = [snippets.parse("verify", VERIFY), snippets.parse("kickoff", KICKOFF), snippets.parse("plain", "x")]
        self.assertEqual(snippets.context_for(all_, ["verify", "kickoff"]), [
            "Read this before the message above:\n\nGet oriented first.",
            "Apply this to the message above:\n\nCheck each claim.",
        ])


class CliTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.dir = Path(tmp.name) / "snippets"
        self.dir.mkdir()
        (self.dir / "verify.md").write_text(VERIFY)
        (self.dir / "kickoff.md").write_text(KICKOFF)
        self.library = Path(tmp.name) / "library"
        self.library.mkdir()
        self.state = Path(tmp.name) / "state" / "enabled.json"
        self.env = {**os.environ, "SNIPPETS_DIR": str(self.dir), "SNIPPETS_LIBRARY": str(self.library),
                    "SNIPPETS_STATE": str(self.state)}

    def run_(self, *args, stdin=""):
        return subprocess.run([sys.executable, "-I", str(SCRIPT), *args], input=stdin,
                              capture_output=True, text=True, env=self.env)

    def hook(self, prompt):
        out = self.run_("hook", stdin=json.dumps({"hook_event_name": "UserPromptSubmit", "prompt": prompt}))
        self.assertEqual(out.returncode, 0)
        return json.loads(out.stdout) if out.stdout.strip() else None

    def test_nothing_on_means_no_output(self):
        self.assertIsNone(self.hook("hello"))

    def test_on_then_hook_adds_context(self):
        self.assertEqual(self.run_("on", "verify", "kickoff").returncode, 0)
        ctx = self.hook("hello")["hookSpecificOutput"]
        self.assertEqual(ctx["hookEventName"], "UserPromptSubmit")
        self.assertTrue(ctx["additionalContext"].startswith("Read this before the message above:\n\nGet oriented first."))
        self.assertIn("Check each claim.", ctx["additionalContext"])

    def test_slash_commands_get_nothing(self):
        self.run_("on", "verify")
        self.assertIsNone(self.hook("  /compact"))
        self.assertIsNone(self.hook("/ponytail:ponytail-audit the repo"))

    def test_a_prompt_starting_with_a_path_still_gets_snippets(self):
        self.run_("on", "verify")
        self.assertIsNotNone(self.hook("/Users/me/app/log.txt why does this crash?"))

    def test_off_and_off_all(self):
        self.run_("on", "verify", "kickoff")
        self.run_("off", "verify")
        self.assertEqual(json.loads(self.state.read_text()), ["kickoff"])
        self.run_("off", "--all")
        self.assertIsNone(self.hook("hello"))

    def test_on_unknown_slug_fails_and_changes_nothing(self):
        out = self.run_("on", "nope")
        self.assertEqual(out.returncode, 1)
        self.assertFalse(self.state.exists())

    def test_deleted_file_is_no_longer_added(self):
        self.run_("on", "verify")
        (self.dir / "verify.md").unlink()
        self.assertIsNone(self.hook("hello"))

    def test_bad_payload_and_bad_state_never_block(self):
        self.run_("on", "verify")
        out = self.run_("hook", stdin="not json")
        self.assertEqual((out.returncode, out.stdout), (0, ""))
        self.state.write_text("{broken")
        self.assertIsNone(self.hook("hello"))

    def test_list_marks_enabled(self):
        self.run_("on", "verify")
        lines = self.run_("list").stdout.splitlines()
        self.assertTrue(lines[0].startswith("[ ] kickoff"))
        self.assertTrue(lines[1].startswith("[x] verify"))

    def test_library_snippets_show_and_yours_win(self):
        (self.library / "brief.md").write_text("Keep it short.")
        (self.library / "verify.md").write_text("Library verify.")
        self.assertEqual(self.run_("show", "brief").stdout.strip(), "Keep it short.")
        self.assertEqual(self.run_("show", "verify").stdout.strip(), "Check each claim.")
        self.run_("on", "brief")
        self.assertIn("Keep it short.", self.hook("hello")["hookSpecificOutput"]["additionalContext"])

    def test_hook_configs_call_the_installed_path(self):
        config = json.loads((SCRIPT.parent / "hook.json").read_text())
        command = config["hooks"]["UserPromptSubmit"][0]["hooks"][0]["command"]
        dest = json.loads((SCRIPT.parent / "install.json").read_text())["files"][0]["dest"]
        self.assertIn(dest.replace("~", "$HOME"), command)


class LibraryTest(unittest.TestCase):
    """The shipped snippets: each parses to a titled snippet with a body, and install.json installs each one."""

    def test_every_library_snippet_is_complete_and_installed(self):
        library = SCRIPT.parent / "library"
        installed = {e["src"] for e in json.loads((SCRIPT.parent / "install.json").read_text())["files"]}
        names = sorted(p.name for p in library.glob("*.md"))
        self.assertTrue(names)
        for name in names:
            s = snippets.parse(name[:-3], (library / name).read_text())
            self.assertNotEqual(s["title"], s["slug"], name)
            self.assertTrue(s["description"] and s["body"], name)
            self.assertIn(f"library/{name}", installed)


if __name__ == "__main__":
    unittest.main()
