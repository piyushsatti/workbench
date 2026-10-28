"""lint-agent-files.sh: third-party plugin clones are skipped, and paths with spaces are linted whole."""

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "agent/bin/lint-agent-files.sh"
TOO_LONG = "".join(f"line {n}\n" for n in range(30))  # over the 20-line budget for a project AGENTS.md


def lint(root, bash="bash"):
    env = {**os.environ, "AGENT_LINT_ROOT": str(root), "HOME": str(root.parent)}
    return subprocess.run([bash, str(SCRIPT)], capture_output=True, text=True, env=env)


class LintAgentFilesTests(unittest.TestCase):
    def test_third_party_clones_are_skipped_and_spaced_paths_are_linted(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "Source"
            (root / "workbench").mkdir(parents=True)
            (root / "workbench/plugins.json").write_text(json.dumps({"owner": "me", "plugins": [
                {"name": "theirs", "github": "them/theirs", "sha": "a"}, {"name": "mine", "github": "me/mine", "sha": "b"}]}))
            for folder in ("theirs", "mine", "My Notes"):
                (root / folder).mkdir()
                (root / folder / "AGENTS.md").write_text(TOO_LONG)
            done = lint(root)
            self.assertNotIn("theirs/AGENTS.md", done.stdout)
            self.assertIn("Source/mine/AGENTS.md", done.stdout)
            self.assertIn("Source/My Notes/AGENTS.md", done.stdout)
            self.assertEqual(done.returncode, 1)  # the over-budget files are majors

    def test_no_plugins_json_on_the_script_shebang_bash(self):
        # The script runs as #!/bin/bash, which is bash 3.2 on macOS: an empty array under set -u must not fail.
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "Source"
            (root / "mine").mkdir(parents=True)
            (root / "mine/AGENTS.md").write_text(TOO_LONG)
            done = lint(root, "/bin/bash")
            self.assertNotIn("unbound variable", done.stderr)
            self.assertIn("Source/mine/AGENTS.md", done.stdout)


if __name__ == "__main__":
    unittest.main()
