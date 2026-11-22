"""Tests for dream's executables, on synthetic data only.

Run: python3 -m unittest discover -s tests (from the plugin folder).
Every test points HOME at a temporary directory, so nothing real is read.
"""

import contextlib
import datetime
import importlib.machinery
import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

PLUGIN = Path(__file__).resolve().parents[1]
BIN = PLUGIN / "bin"


def load(name):
    """Import an extensionless executable from bin/ as a module."""
    path = BIN / name
    loader = importlib.machinery.SourceFileLoader(name.replace("-", "_"), str(path))
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    loader.exec_module(module)
    return module


replay = load("dream-replay")
catalog = load("dream-catalog")
lint = load("dream-lint-memory")
config = load("dream-config")


def run_quiet(func, *args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = func(*args)
    return code, out.getvalue()


def jsonl(path, records, trailing_partial=""):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a") as handle:
        for record in records:
            handle.write(json.dumps(record) + "\n")
        handle.write(trailing_partial)


def user(uuid, text, ts="2026-10-20T10:00:00Z", **extra):
    return {"type": "user", "uuid": uuid, "timestamp": ts, "message": {"content": text}, **extra}


def assistant(uuid, blocks, ts="2026-10-20T10:01:00Z"):
    return {"type": "assistant", "uuid": uuid, "timestamp": ts, "message": {"content": blocks}}


class HomeCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.home = Path(self.tmp.name)
        self.env = mock_env({"HOME": str(self.home), "DREAM_CONFIG": None})
        self.env.__enter__()

    def tearDown(self):
        self.env.__exit__(None, None, None)
        self.tmp.cleanup()


@contextlib.contextmanager
def mock_env(values):
    old = {k: os.environ.get(k) for k in values}
    for key, value in values.items():
        if value is None:
            os.environ.pop(key, None)
        else:
            os.environ[key] = value
    try:
        yield
    finally:
        for key, value in old.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value


class ReplayTest(HomeCase):
    def setUp(self):
        super().setUp()
        self.projects = self.home / ".claude/projects"
        self.transcripts = self.projects / "-work-space"
        self.state = self.home / ".local/state/dream/-work-space/state"
        self.out = self.home / "run/digests"
        memory_path = str(self.projects / "-work-space/memory/2026-10-01-user_x.md")
        jsonl(
            self.transcripts / "s1.jsonl",
            [
                user("u1", "Please fix the build. Remember: always run the lint."),
                user("m1", "<system-reminder>noise</system-reminder>", isMeta=True),
                {"type": "summary", "uuid": "x"},
                assistant(
                    "a1",
                    [
                        {"type": "text", "text": "Reading memory."},
                        {"type": "tool_use", "id": "t1", "name": "Read", "input": {"file_path": memory_path}},
                        {"type": "tool_use", "id": "t2", "name": "Bash", "input": {"command": "export API_KEY=abc123\nmore"}},
                    ],
                ),
                user("u2", [
                    {"type": "tool_result", "tool_use_id": "t1", "content": "ok"},
                    {"type": "tool_result", "tool_use_id": "t2", "is_error": True, "content": "Exit code 1\nno such file\nthird"},
                ]),
                assistant("a2", [{"type": "text", "text": "Done.\n**Deferred**\n- wire the cache later"}]),
                user("u1", "a duplicate copied in by resume"),
            ],
            trailing_partial='{"type": "user", "uuid": "half"',
        )
        jsonl(
            self.transcripts / "s1/subagents/agent-1.jsonl",
            [
                assistant("sa1", [{"type": "tool_use", "id": "x1", "name": "Grep", "input": {"pattern": "foo"}}]),
                user("su1", [{"type": "tool_result", "tool_use_id": "x1", "is_error": True, "content": "bad regex"}]),
            ],
        )

    def digest(self):
        files = list(self.out.glob("*.md"))
        self.assertEqual(len(files), 1, files)
        return files[0]

    def test_run_writes_digest_recall_and_staged_state(self):
        code, printed = run_quiet(replay.run, self.transcripts, self.state, self.out, "")
        self.assertEqual(code, 0)
        self.assertIn("digests=1", printed)
        self.assertIn("dupes=1", printed)
        self.assertIn("memory_reads=1", printed)
        digest = self.digest()
        self.assertTrue(digest.name.startswith("s1-L1-L"))
        text = digest.read_text()
        self.assertIn("always run the lint", text)
        self.assertNotIn("noise", text)
        self.assertIn("[Read]", text)
        self.assertIn("API_KEY=***", text)
        self.assertNotIn("abc123", text)
        self.assertIn("ERROR: Exit code 1 | no such file", text)
        self.assertNotIn("third", text)
        self.assertIn("## Open loops at the end of this window", text)
        self.assertIn("wire the cache later", text)
        self.assertIn("agent-1: [Grep] foo → ERROR: bad regex", text)
        self.assertNotIn("duplicate copied", text)
        recall = (self.state / "recall.next.tsv").read_text().split("\t")
        self.assertEqual(recall, ["2026-10-20", "s1", "Read", "-work-space", "2026-10-01-user_x.md"])
        self.assertTrue((self.state / "state.next.json").exists())
        self.assertFalse((self.state / "state.json").exists())

    def test_uncommitted_run_replays_the_same_lines(self):
        run_quiet(replay.run, self.transcripts, self.state, self.out, "")
        _, printed = run_quiet(replay.run, self.transcripts, self.state, self.out, "")
        self.assertIn("digests=1", printed)

    def test_commit_then_rerun_reads_only_new_lines(self):
        run_quiet(replay.run, self.transcripts, self.state, self.out, "")
        run_quiet(replay.commit, self.state)
        self.assertTrue((self.state / "state.json").exists())
        self.assertFalse(list(self.state.glob("*.next.*")))
        _, printed = run_quiet(replay.run, self.transcripts, self.state, self.home / "run2", "")
        self.assertIn("digests=0", printed)
        # The half-written line is finished and a new one arrives.
        with (self.transcripts / "s1.jsonl").open("a") as handle:
            handle.write(', "timestamp": "2026-10-21T09:00:00Z", "message": {"content": "finished line"}}\n')
        jsonl(self.transcripts / "s1.jsonl", [user("u9", "brand new question", ts="2026-10-21T09:05:00Z")])
        _, printed = run_quiet(replay.run, self.transcripts, self.state, self.home / "run3", "")
        self.assertIn("digests=1", printed)
        text = next((self.home / "run3").glob("*.md")).read_text()
        self.assertIn("finished line", text)
        self.assertIn("brand new question", text)
        self.assertNotIn("always run the lint", text)

    def test_since_floor_is_stored_and_drops_older_records(self):
        _, printed = run_quiet(replay.run, self.transcripts, self.state, self.out, "2026-10-21")
        self.assertIn("digests=0", printed)
        self.assertEqual(list(self.out.glob("*.md")), [])
        state = json.loads((self.state / "state.next.json").read_text())
        self.assertEqual(state["since"], "2026-10-21")

    def test_executable_runs_with_isolated_python(self):
        result = subprocess.run(
            [str(BIN / "dream-replay"), "run", str(self.transcripts), str(self.state), str(self.out)],
            capture_output=True, text=True, env={**os.environ, "HOME": str(self.home)},
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("digests=1", result.stdout)

    def test_bad_arguments_print_usage(self):
        result = subprocess.run([str(BIN / "dream-replay"), "nope"], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("dream-replay run", result.stderr)


def memory_file(path, created, kind="feedback", body=None, extra=""):
    path.parent.mkdir(parents=True, exist_ok=True)
    body = body if body is not None else "Rule.\n\n**Why:** it broke once.\n\n**How to apply:** check first.\n"
    path.write_text(
        "---\n"
        f"name: {path.stem.split('_', 1)[-1]}\n"
        f"id: {path.stem}\n"
        "description: one line\n"
        f"type: {kind}\n"
        f"created: {created}\n"
        f"last_updated: {created}\n"
        "scope: {key: workbench, depth: 1, role: project}\n"
        "genericity: specific\n"
        "provenance: a session\n"
        "status: active\n"
        f"{extra}"
        "---\n" + body
    )


class CatalogTest(HomeCase):
    def test_rows_tiers_staleness_and_recall(self):
        projects = self.home / ".claude/projects"
        today = datetime.datetime.now(datetime.UTC).date()
        old = (today - datetime.timedelta(days=90)).isoformat()
        fresh = today.isoformat()
        store = projects / "-a/memory"
        memory_file(store / f"{old}-feedback_old.md", old)
        memory_file(store / f"{old}-feedback_recalled.md", old)
        memory_file(store / f"{fresh}-feedback_new.md", fresh)
        memory_file(store / "archive" / f"{old}-feedback_gone.md", old)
        (store / "MEMORY.md").write_text("")
        log = self.home / "recall.tsv"
        log.write_text(f"{fresh}\ts1\tRead\t-a\t{old}-feedback_recalled.md\n")
        code, printed = run_quiet(catalog.main, projects, [str(log)])
        self.assertEqual(code, 0)
        rows = [line.split("\t") for line in printed.splitlines()]
        self.assertEqual(tuple(rows[0]), catalog.COLUMNS)
        by_file = {row[2]: row for row in rows[1:]}
        self.assertNotIn("MEMORY.md", by_file)
        self.assertEqual(by_file[f"{old}-feedback_old.md"][7], "stale")
        self.assertEqual(by_file[f"{old}-feedback_recalled.md"][6], fresh)
        self.assertEqual(by_file[f"{old}-feedback_recalled.md"][7], "")
        self.assertEqual(by_file[f"{fresh}-feedback_new.md"][7], "")
        self.assertEqual(by_file[f"{old}-feedback_gone.md"][1], "archive")
        self.assertEqual(by_file[f"{old}-feedback_gone.md"][7], "")

    def test_a_missing_recall_log_means_nothing_recalled(self):
        # First run: SKILL.md stage 3 passes <state>/recall.next.tsv before any commit has written recall.tsv.
        projects = self.home / ".claude/projects"
        old = (datetime.datetime.now(datetime.UTC).date() - datetime.timedelta(days=90)).isoformat()
        memory_file(projects / "-a/memory" / f"{old}-feedback_old.md", old)
        code, printed = run_quiet(catalog.main, projects, [str(self.home / "state/recall.next.tsv")])
        self.assertEqual(code, 0)
        self.assertEqual(printed.splitlines()[1].split("\t")[7], "stale")


class LintTest(HomeCase):
    def setUp(self):
        super().setUp()
        self.store = self.home / "memory"
        memory_file(self.store / "2026-10-01-feedback_alpha.md", "2026-10-01", body="Use [[beta]].\n\n**Why:** x\n\n**How to apply:** y\n")
        memory_file(self.store / "2026-10-02-reference_beta.md", "2026-10-02", kind="reference", body="A pointer.\n")
        (self.store / "MEMORY.md").write_text(
            "- [Alpha](2026-10-01-feedback_alpha.md) — the alpha rule\n"
            "- [Beta](2026-10-02-reference_beta.md) — the beta pointer\n"
        )

    def test_clean_store_passes(self):
        code, printed = run_quiet(lint.main, str(self.store))
        self.assertEqual(code, 0, printed)
        self.assertIn("2 indexed + 0 archived files, 0 errors, 0 warnings", printed)

    def test_unindexed_file_secret_and_broken_link_are_errors(self):
        memory_file(
            self.store / "2026-10-03-project_gamma.md", "2026-10-03", kind="project",
            body="password: hunter22 and [[nowhere]]\n",
        )
        code, printed = run_quiet(lint.main, str(self.store))
        self.assertEqual(code, 1)
        self.assertIn("ERROR 2026-10-03-project_gamma.md: not in MEMORY.md", printed)
        self.assertIn("looks like a secret", printed)
        self.assertIn("[[nowhere]] matches no memory", printed)
        self.assertIn("WARN  2026-10-03-project_gamma.md: no **Why:**", printed)

    def test_a_file_in_the_older_format_only_warns(self):
        path = self.store / "2026-10-05-reference_old.md"
        path.write_text("---\nname: old\ndescription: d\ntype: reference\ncreated: 2026-10-05\n"
                        "last_updated: 2026-10-05\nscope: {depth: 1, role: project}\ngenericity: specific\n"
                        "schema_version: 2\n---\nBody\n")
        with (self.store / "MEMORY.md").open("a") as index:
            index.write("- [Old](2026-10-05-reference_old.md) — an older file\n")
        code, printed = run_quiet(lint.main, str(self.store))
        self.assertEqual(code, 0, printed)
        for field in ("id", "provenance", "status"):
            self.assertIn(f"WARN  2026-10-05-reference_old.md: missing rulebook field {field}", printed)
        self.assertIn("WARN  2026-10-05-reference_old.md: scope has no key", printed)

    def test_metadata_type_and_fields_are_accepted(self):
        path = self.store / "archive/2026-10-04-user_delta.md"
        path.parent.mkdir()
        path.write_text(
            "---\nname: delta\ndescription: d\nmetadata:\n  type: user\n  created: 2026-10-04\n"
            "  last_updated: 2026-10-04\n  scope: {key: x, depth: 1, role: project}\n  genericity: specific\n"
            "  id: 2026-10-04-user_delta\n  provenance: p\n  status: active\n---\nBody\n"
        )
        code, printed = run_quiet(lint.main, str(self.store))
        self.assertEqual(code, 0, printed)
        self.assertIn("1 archived", printed)


class ConfigTest(HomeCase):
    def test_missing_file_gives_defaults(self):
        settings = config.load(key="-work-space", date="2026-10-22")
        self.assertFalse(settings["configured"])
        self.assertEqual(settings["config_path"], str(self.home / ".local/state/dream/config.json"))
        self.assertEqual(settings["state_dir"], str(self.home / ".local/state/dream/-work-space/state"))
        self.assertEqual(settings["run_dir"], str(self.home / ".local/state/dream/-work-space/runs/2026-10-22-dream"))
        self.assertEqual(settings["routes"], [])
        self.assertEqual(settings["trash_command"], "trash-put")

    def test_file_is_merged_and_paths_expanded(self):
        path = self.home / ".local/state/dream/config.json"
        path.parent.mkdir(parents=True)
        path.write_text(json.dumps({
            "routes": [{"item": "Hosts", "to": "~/notes/infra/"}],
            "read_first": [{"path": "~/notes/rules.md", "for": "x"}],
            "state_dir": "/srv/state/<key>",
        }))
        settings = config.load(key="k")
        self.assertTrue(settings["configured"])
        self.assertEqual(settings["routes"][0]["to"], f"{self.home}/notes/infra/")
        self.assertEqual(settings["read_first"][0]["path"], str(self.home / "notes/rules.md"))
        self.assertEqual(settings["state_dir"], "/srv/state/k")
        self.assertEqual(config.missing(settings), [str(self.home / "notes/rules.md"), f"{self.home}/notes/infra/"])
        (self.home / "notes/infra").mkdir(parents=True)
        (self.home / "notes/rules.md").write_text("")
        self.assertEqual(config.missing(settings), [])

    def test_space_placeholder_keeps_the_old_in_space_layout(self):
        path = self.home / ".local/state/dream/config.json"
        path.parent.mkdir(parents=True)
        path.write_text(json.dumps({
            "state_dir": "<space>/artifacts/dream/state",
            "run_dir": "<space>/artifacts/<date>-dream",
        }))
        settings = config.load(key="k", date="2026-10-22", space="/work/space/")
        self.assertEqual(settings["state_dir"], "/work/space/artifacts/dream/state")
        self.assertEqual(settings["run_dir"], "/work/space/artifacts/2026-10-22-dream")

    def test_unknown_key_is_refused(self):
        path = self.home / "custom.json"
        path.write_text(json.dumps({"rutes": []}))
        with mock_env({"DREAM_CONFIG": str(path)}):
            err = io.StringIO()
            with contextlib.redirect_stderr(err), contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(config.main([]), 2)
        self.assertIn("unknown keys ['rutes']", err.getvalue())

    def test_example_config_is_valid_and_neutral(self):
        example = PLUGIN / "config.example.json"
        with mock_env({"DREAM_CONFIG": str(example)}):
            settings = config.load(key="k", date="2026-10-22")
        self.assertTrue(settings["configured"])
        self.assertTrue(settings["routes"])
        for route in settings["routes"]:
            self.assertTrue(route["to"].startswith(str(self.home / "notes")), route)

    def test_check_flag_reports_missing_paths(self):
        path = self.home / ".local/state/dream/config.json"
        path.parent.mkdir(parents=True)
        path.write_text(json.dumps({"rulebooks_dir": "~/nope/"}))
        result = subprocess.run(
            [str(BIN / "dream-config"), "--key", "k", "--check"],
            capture_output=True, text=True, env={**os.environ, "HOME": str(self.home)},
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn(f"MISSING {self.home}/nope/", result.stderr)
        self.assertTrue(json.loads(result.stdout)["configured"])


class PluginShapeTest(unittest.TestCase):
    def test_install_json_covers_every_executable(self):
        install = json.loads((PLUGIN / "install.json").read_text())
        srcs = {entry["src"] for entry in install["files"]}
        self.assertEqual(srcs, {f"bin/{p.name}" for p in BIN.iterdir()})
        for entry in install["files"]:
            self.assertEqual(entry["dest"], "~/.local/bin/" + Path(entry["src"]).name)
            self.assertTrue(os.access(PLUGIN / entry["src"], os.X_OK))

    def test_skill_calls_only_installed_names(self):
        skill = (PLUGIN / "SKILL.md").read_text()
        self.assertNotIn("scripts/", skill)
        self.assertNotIn(".py", skill)
        for name in ("dream-replay", "dream-catalog", "dream-lint-memory", "dream-config"):
            self.assertIn(name, skill)


if __name__ == "__main__":
    unittest.main()
