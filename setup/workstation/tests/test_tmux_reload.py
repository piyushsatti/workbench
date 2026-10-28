"""Exercise the real reload function against an isolated tmux server."""

import importlib.util
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location(
    "workstation", Path(__file__).resolve().parents[1] / "scripts/workstation.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


@unittest.skipUnless(shutil.which("tmux"), "tmux is needed for the integration check")
class TmuxReloadTest(unittest.TestCase):
    def test_verifier_rejects_startup_configuration_errors(self):
        with tempfile.TemporaryDirectory(prefix="workstation-invalid-config-") as temp:
            previous_home = MODULE.HOME
            MODULE.HOME = Path(temp)
            MODULE.SELECTED = None  # no profile, so the boot check is skipped and tmux's own error surfaces
            (MODULE.HOME / ".tmux.conf").write_text(
                "set -g workstation-deliberately-invalid-option none\n"
            )
            try:
                with self.assertRaises(subprocess.CalledProcessError) as caught:
                    MODULE.verify_tmux()
                self.assertIn("invalid option", caught.exception.stderr)
            finally:
                MODULE.HOME = previous_home

    def test_live_session_survives_and_configuration_is_reloaded(self):
        with tempfile.TemporaryDirectory(prefix="workstation-reload-") as temp:
            root = Path(temp)
            socket = str(root / "socket")
            previous_tmux = os.environ.get("TMUX")
            previous_home = MODULE.HOME
            MODULE.HOME = root
            (root / ".tmux.conf").write_text(
                "set -g status-right WORKSTATION_RELOAD_TEST\n"
                "set -g status-position bottom\n"
                "set -g @observed-restore-delay '#{@continuum-restore-max-delay}'\n"
            )
            subprocess.run(
                [
                    "tmux",
                    "-S",
                    socket,
                    "-f",
                    "/dev/null",
                    "new-session",
                    "-d",
                    "-s",
                    "sentinel",
                    "sleep 60",
                ],
                check=True,
            )
            os.environ["TMUX"] = f"{socket},0,0"
            MODULE.select_profile("linux-full")
            try:
                MODULE.reload_tmux()
                subprocess.run(["tmux", "has-session", "-t", "sentinel"], check=True)
                value = subprocess.check_output(
                    ["tmux", "show-option", "-gv", "status-right"], text=True
                ).strip()
                self.assertEqual(value, "WORKSTATION_RELOAD_TEST")
                delay = subprocess.check_output(
                    ["tmux", "show-option", "-gqv", "@continuum-restore-max-delay"],
                    text=True,
                ).strip()
                self.assertEqual(delay, "")
            finally:
                subprocess.run(["tmux", "-S", socket, "kill-server"], check=False)
                MODULE.HOME = previous_home
                if previous_tmux is None:
                    os.environ.pop("TMUX", None)
                else:
                    os.environ["TMUX"] = previous_tmux

    def test_profile_without_locked_dotfiles_leaves_a_live_server_alone(self):
        # Without a dotfiles lock no ~/.tmux.conf is written, so a running server must not source a missing file.
        with tempfile.TemporaryDirectory(prefix="workstation-noreload-") as temp:
            root = Path(temp)
            socket = str(root / "socket")
            previous_tmux, previous_home = os.environ.get("TMUX"), MODULE.HOME
            MODULE.HOME = root
            subprocess.run(["tmux", "-S", socket, "-f", "/dev/null", "new-session", "-d", "-s", "sentinel", "sleep 60"], check=True)
            os.environ["TMUX"] = f"{socket},0,0"
            MODULE.select_profile("linux")
            try:
                with mock.patch.object(MODULE, "dotfiles_locked", return_value=False):
                    MODULE.reload_tmux()
                subprocess.run(["tmux", "-S", socket, "has-session", "-t", "sentinel"], check=True)
                self.assertFalse((root / ".tmux.conf").exists())
            finally:
                subprocess.run(["tmux", "-S", socket, "kill-server"], check=False)
                MODULE.HOME = previous_home
                if previous_tmux is None:
                    os.environ.pop("TMUX", None)
                else:
                    os.environ["TMUX"] = previous_tmux


class LockedTmuxConfigTest(unittest.TestCase):
    """The verifier's expectations against the real locked linux-full .tmux.conf, not a hand-written one."""

    @classmethod
    def setUpClass(cls):
        # Named, not taken from $WORKSTATION_PROFILE: these tests read the linux-full config.
        MODULE.select_profile("linux-full")
        _, rendered, _ = MODULE.render_dotfiles()
        cls.config = rendered[".tmux.conf"][0].decode()

    def test_boot_value_matches_each_profile(self):
        MODULE.check_tmux_boot(self.config, "linux-full")
        with self.assertRaisesRegex(RuntimeError, "must be off"):
            MODULE.check_tmux_boot(self.config, "linux")
        with self.assertRaisesRegex(RuntimeError, "found none"):
            MODULE.check_tmux_boot("set -g status 2\n", "linux-full")

    def test_isolated_server_never_loads_continuum(self):
        # tpm reads ~/.tmux.conf from disk, so the isolated file must not run tpm at all: continuum's boot
        # handler would disable the real tmux boot unit (#1 of the 2026-10-27 audit).
        isolated = MODULE.isolated_tmux_config(self.config)
        self.assertNotIn(MODULE.TPM_RUN, isolated)
        self.assertNotIn("plugins/tmux-continuum/*.tmux", isolated)
        self.assertIn("""run 'for f in ~/.tmux/plugins/tmux-toggle-popup/*.tmux; do "$f"; done'""", isolated)

    @unittest.skipUnless(shutil.which("tmux"), "tmux is needed for the integration check")
    def test_locked_config_has_the_options_the_verifier_expects(self):
        # Plugins aren't installed here, so drop the lines that run them; the options under test are plain sets.
        text = "\n".join(line for line in MODULE.isolated_tmux_config(self.config).splitlines()
                          if not line.startswith("run ") and "source-file -q" not in line)
        name = f"workstation-locked-{os.getpid()}"
        with tempfile.TemporaryDirectory(prefix="workstation-locked-") as temp:
            path = Path(temp) / "tmux.conf"
            path.write_text(text + "\n")
            subprocess.run(["tmux", "-L", name, "-f", str(path), "new-session", "-d", "sleep 60"], check=True)
            try:
                for option, value in MODULE.TMUX_EXPECTED:
                    actual = subprocess.check_output(["tmux", "-L", name, "show-option", "-gv", option], text=True).strip()
                    self.assertEqual(actual, value, option)
            finally:
                subprocess.run(["tmux", "-L", name, "kill-server"], capture_output=True, check=False)


if __name__ == "__main__":
    unittest.main()
