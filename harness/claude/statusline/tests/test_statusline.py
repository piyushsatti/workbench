"""claude-statusline: two lines, with the harness's data printed as is, never read as escape codes."""

import json
import os
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "claude-statusline"


def run(payload, settings=None):
    with tempfile.TemporaryDirectory() as home:
        if settings is not None:
            Path(home, ".claude").mkdir()
            Path(home, ".claude/settings.json").write_text(json.dumps(settings))
        env = {**os.environ, "HOME": home, "XDG_STATE_HOME": f"{home}/state", "XDG_CONFIG_HOME": f"{home}/config"}
        done = subprocess.run(["bash", str(SCRIPT)], input=json.dumps(payload), capture_output=True,
                              text=True, env=env, cwd=home, timeout=30)
    return done.stdout


class StatuslineTests(unittest.TestCase):
    def test_backslashes_in_the_data_are_printed_not_interpreted(self):
        out = run({
            "session_id": "0000-test",
            "cwd": "/tmp/a\\cb",
            "workspace": {"current_dir": "/tmp/a\\cb"},
            "model": {"id": "claude-test", "display_name": "Test"},
            "agent": {"name": "rev\\033[2J"},
        })
        self.assertEqual(len(out.splitlines()), 2, out)
        self.assertIn("a\\cb", out)
        self.assertNotIn("\x1b[2J", out)
        self.assertIn("\x1b[", out)  # the colours themselves still work

    def test_cache_segment_follows_claude_codes_prompt_cache(self):
        base = {"session_id": "0000-test", "cwd": "/tmp", "model": {"id": "claude-test", "display_name": "Test"}}
        warm = run({**base, "prompt_cache": {"warm": True, "ttl": "5m", "expires_at": int(time.time()) + 240}})
        self.assertIn("warm 4m", warm)  # a 5-minute TTL, not the old fixed hour
        cold = run({**base, "prompt_cache": {"warm": False, "ttl": "5m", "expires_at": int(time.time()) - 60}})
        self.assertIn("cold", cold)
        self.assertNotIn("cache", run(base))  # older Claude Code: no prompt_cache, no segment
        self.assertNotIn("cache", run({**base, "prompt_cache": {"expires_at": int(time.time()) + 240}}))  # no warm

    def test_sidecar_is_the_status_json_with_local_reset_times_only(self):
        payload = {"session_id": "0000-test", "cwd": "/tmp", "model": {"id": "claude-test", "display_name": "Test"},
                   "rate_limits": {"five_hour": {"used_percentage": 10, "resets_at": 1767225600}}}
        with tempfile.TemporaryDirectory() as home:
            Path(home, ".claude").mkdir()
            Path(home, ".claude/settings.json").write_text(json.dumps({"autoCompactWindow": 160000}))
            env = {**os.environ, "HOME": home, "XDG_STATE_HOME": f"{home}/state"}
            subprocess.run(["bash", str(SCRIPT)], input=json.dumps(payload), capture_output=True, text=True,
                           env=env, cwd=home, timeout=30, check=True)
            sidecar = json.loads(Path(home, "state/claude/statusline/0000-test.json").read_text())
        # cc-status reads only the harness's fields and `local`; the status line itself reads settings.json.
        self.assertEqual(set(sidecar) - set(payload), {"local"})
        self.assertRegex(sidecar["local"]["five_hour_reset"], r"^\d\d:\d\d$")

    def test_autocompact_window_counts_only_when_it_is_a_positive_integer_below_the_model_window(self):
        payload = {"session_id": "0000-test", "cwd": "/tmp", "model": {"id": "claude-test", "display_name": "Test"},
                   "context_window": {"used_percentage": 95, "total_input_tokens": 190000, "context_window_size": 200000}}
        native = run(payload)
        self.assertIn("95%", native)
        self.assertIn("190k/200k", native)
        # Bigger than the model's window, or not a positive integer: the model's window stands.
        for window in (500000, "abc", -5, 0, 1.5, "", None, "1e5", "100000 x"):
            out = run(payload, {"autoCompactWindow": window})
            self.assertIn("95%", out, window)
            self.assertIn("190k/200k", out, window)
        smaller = run(payload, {"autoCompactWindow": 160000})
        self.assertIn("118%", smaller)
        self.assertIn("190k/160k", smaller)


if __name__ == "__main__":
    unittest.main()
