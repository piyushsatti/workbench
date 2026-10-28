"""Contract for the global skills in agent/skills: portable frontmatter, bundled files and licences.

Stdlib only, no network. Run from the repo root: uv run python -B -m unittest discover -s agent/skills/tests
"""

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ALLOWED_KEYS = {"name", "description", "allowed-tools"}
EXPLICIT_ONLY = ("grill-me", "save-state", "teach")


def skills():
    return sorted(p.parent.name for p in ROOT.glob("*/SKILL.md"))


def frontmatter(path):
    head = path.read_text().split("---\n")[1]
    return [m.group(1) for m in re.finditer(r"^([A-Za-z_-]+):", head, re.M)], head


class Skills(unittest.TestCase):
    def test_the_seven_skills_are_here(self):
        self.assertEqual(skills(), ["crucible", "dream", "grill-me", "humanizer", "research", "save-state", "teach"])

    def test_frontmatter_holds_only_portable_keys_and_its_own_name(self):
        for skill in skills():
            keys, head = frontmatter(ROOT / skill / "SKILL.md")
            self.assertTrue(set(keys) <= ALLOWED_KEYS, f"{skill}: {keys}")
            self.assertIn(f"name: {skill}\n", head)

    def test_bundled_files_and_licences_ship(self):
        for rel in (
            "grill-me/references/GRILLING.md",
            "teach/MISSION-FORMAT.md",
            "teach/RESOURCES-FORMAT.md",
            "teach/LEARNING-RECORD-FORMAT.md",
            "teach/GLOSSARY-FORMAT.md",
            "humanizer/LICENSE",
            "grill-me/LICENSE",
            "research/LICENSE",
            "teach/LICENSE",
        ):
            self.assertTrue((ROOT / rel).is_file(), rel)
        self.assertIn("references/GRILLING.md", (ROOT / "grill-me/SKILL.md").read_text())
        self.assertIn("If the host has no sub-agents", (ROOT / "grill-me/SKILL.md").read_text())
        self.assertIn("Otherwise, do the same research in this session.", (ROOT / "research/SKILL.md").read_text())

    def test_explicit_only_skills_keep_that_intent_on_every_harness(self):
        # Claude Code's disable-model-invocation key is gone; the description says it instead,
        # and Codex keeps its own policy in agents/openai.yaml.
        for skill in EXPLICIT_ONLY:
            _, head = frontmatter(ROOT / skill / "SKILL.md")
            self.assertIn("Run only when the user asks for it by name.", head)
            self.assertIn("allow_implicit_invocation: false", (ROOT / skill / "agents/openai.yaml").read_text())


if __name__ == "__main__":
    unittest.main()
