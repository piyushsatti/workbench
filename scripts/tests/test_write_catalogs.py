"""write_catalogs.py: dry run writes nothing; apply writes catalogs that lint_plugins.py accepts."""

import importlib.util
import json
import tempfile
import unittest
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


WRITE = load("write_catalogs", HERE.parent / "write_catalogs.py")
LINT = load("lint_plugins", HERE.parent / "lint_plugins.py")
FIXTURE = load("test_lint_plugins", HERE / "test_lint_plugins.py")


class WriteCatalogsTests(unittest.TestCase):
    def test_dry_run_then_apply_then_lint_clean(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            FIXTURE.good_plugin(root)
            pi, args = root / "pi", ["--root", str(root / "pi"), "--source", str(root)]
            for rel in WRITE.catalogs(pi, root)[0]:
                (pi / rel).unlink()
            with redirect_stdout(StringIO()) as out:
                WRITE.main(args)
            self.assertIn("pending", out.getvalue())
            self.assertFalse((pi / ".claude-plugin/marketplace.json").exists())
            with redirect_stdout(StringIO()):
                WRITE.main([*args, "--apply"])
                self.assertEqual(LINT.main(args), 0)
            with redirect_stdout(StringIO()) as out:
                WRITE.main(args)
            self.assertIn("up to date", out.getvalue())

    def test_missing_repo_is_left_out(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            FIXTURE.good_plugin(root)
            pi, args = root / "pi", ["--root", str(root / "pi"), "--source", str(root)]
            FIXTURE.write(pi / "plugins.json", {"plugins": [{"name": "demo", "github": "me/demo", "sha": None},
                                                            {"name": "absent", "github": "me/absent", "sha": None}]})
            with redirect_stdout(StringIO()) as out:
                WRITE.main([*args, "--apply"])
            self.assertIn("MISSING      absent", out.getvalue())


    def test_a_plugin_hosted_in_this_repo_is_cataloged_by_path(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            FIXTURE.good_plugin(root)
            pi, args = root / "pi", ["--root", str(root / "pi"), "--source", str(root)]
            (root / "demo").rename(pi / "demo")
            FIXTURE.write(pi / "plugins.json", {"plugins": [{"name": "demo", "path": "demo"}]})
            with redirect_stdout(StringIO()):
                WRITE.main([*args, "--apply"])
                self.assertEqual(LINT.main(args), 0)
            claude = json.loads((pi / ".claude-plugin/marketplace.json").read_text())
            codex = json.loads((pi / ".agents/plugins/marketplace.json").read_text())
            self.assertEqual(claude["plugins"][0]["source"], "./demo")
            self.assertEqual(codex["plugins"][0]["source"], {"source": "local", "path": "./demo"})

if __name__ == "__main__":
    unittest.main()
