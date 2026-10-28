"""claude-statusline: two lines, with the harness's data printed as is, never read as escape codes."""

import json
import os
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "claude-statusline"


def run(payload):
    with tempfile.TemporaryDirectory() as home:
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


if __name__ == "__main__":
    unittest.main()
