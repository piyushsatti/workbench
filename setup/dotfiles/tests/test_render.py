"""Render checks for the canonical cross-platform Chezmoi source."""

import hashlib
import io
import json
import shutil
import subprocess
import re
import tarfile
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CHEZMOI = shutil.which("chezmoi")
LUAC = shutil.which("luac")

GIT_HOOKS = ("applypatch-msg", "commit-msg", "post-applypatch", "post-checkout", "post-commit", "post-merge", "post-rewrite",
             "pre-applypatch", "pre-commit", "pre-merge-commit", "pre-push", "pre-rebase", "prepare-commit-msg")
GIT_HOOK_FILES = {f".config/git/hooks/{name}" for name in GIT_HOOKS}
NVIM_FILES = {
    ".config/nvim/init.lua",
    ".config/nvim/lazy-lock.json",
    ".config/nvim/lua/config/autocmds.lua",
    ".config/nvim/lua/config/keymaps.lua",
    ".config/nvim/lua/config/lazy-bootstrap.lua",
    ".config/nvim/lua/config/options.lua",
    ".config/nvim/lua/plugins/avante.lua",
    ".config/nvim/lua/plugins/colorscheme.lua",
    ".config/nvim/lua/plugins/mason-extra.lua",
    ".config/nvim/lua/plugins/project.lua",
}
# Pinned copy of catppuccin/btop themes/catppuccin_mocha.theme (see dot_config/btop/themes/.catppuccin-SOURCE).
CATPPUCCIN_BTOP_SHA256 = "4c7469ab9bda282c1ff606aca37c9c0b84cd0cb66d041e4e7e1fed39791f4644"
PROFILES = (("linux", "linux"), ("linux", "linux-full"), ("darwin", None))


def archive_files(archive):
    """name -> bytes for each file in a `chezmoi archive` tarball."""
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:*") as bundle:
        return {m.name: bundle.extractfile(m).read() for m in bundle.getmembers() if m.isfile()}


@unittest.skipUnless(CHEZMOI, "chezmoi is required for render tests")
class RenderTests(unittest.TestCase):
    def render(self, os_name, profile=None):
        if os_name != "darwin" and profile is None:
            profile = "linux"
        data = {"chezmoi": {"os": os_name}}
        if profile:
            data["workstation"] = {"profile": profile}
        result = self.raw_archive(data)
        result.check_returncode()
        archive = result.stdout
        files = {}
        modes = {}
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:*") as bundle:
            for member in bundle.getmembers():
                if member.isfile():
                    files[member.name] = bundle.extractfile(member).read()
                    modes[member.name] = member.mode & 0o777
        return files, modes

    def test_linux_profile_target_and_shell_syntax(self):
        files, _ = self.render("linux", "linux-full")
        self.assertEqual(
            set(files),
            {
                "20-patch-tmux-resurrect.sh",
                ".bash_aliases",
                ".bash_profile",
                ".bashrc",
                ".config/Code/User/keybindings.json",
                ".config/Code/User/settings.json",
                ".config/btop/btop.conf",
                ".config/ghostty/config",
                ".config/starship.toml",
                ".tmux.conf",
                ".tmux/claude/claude-tmux-summary.sh",
                ".tmux/claude/pane-title.sh",
                ".tmux/claude/tmux-status.conf",
                ".config/btop/themes/catppuccin_mocha.theme",
                ".gitconfig",
            }
            | NVIM_FILES
            | GIT_HOOK_FILES,
        )
        for name in (".bash_aliases", ".bash_profile", ".bashrc"):
            subprocess.run(["bash", "-n"], input=files[name], check=True)
        bashrc = files[".bashrc"].decode()
        self.assertIn("if type -P pyenv", bashrc)
        self.assertIn("FZF_DEFAULT_COMMAND", bashrc)
        tmux = files[".tmux.conf"].decode()
        self.assertIn("set -g @continuum-boot 'on'", tmux)
        self.assertIn("set -g @continuum-restore 'on'", tmux)
        self.assertIn("bind S display-popup", tmux)

    def test_mac_installs_each_tmux_plugin_at_its_pinned_commit(self):
        pins = json.loads((ROOT / ".chezmoidata/tmux_plugins.json").read_text())["tmuxPlugins"]
        configured = re.findall(r"^set -g @plugin '([^']+)'$", (ROOT / "dot_tmux.conf.tmpl").read_text(), re.MULTILINE)
        self.assertEqual(sorted(configured), sorted(p["repo"] for p in pins))
        files, _ = self.render("darwin")
        script = files["10-install-tmux-plugins.sh"]
        subprocess.run(["sh", "-n"], input=script, check=True)
        for p in pins:
            self.assertIn(f'pin "{p["name"]}" "{p["repo"]}" "{p["commit"]}"', script.decode())
        self.assertNotIn("10-install-tmux-plugins.sh", self.render("linux", "linux-full")[0])

    def test_headless_linux_profile_is_a_small_override_without_gui_settings(self):
        default, _ = self.render("linux", "linux-full")
        profile, _ = self.render("linux", "linux")
        self.assertEqual(set(default) - set(profile), {".config/ghostty/config"} | {n for n in default if n.startswith(".config/Code/")})
        tmux = profile[".tmux.conf"].decode()
        self.assertIn("set -g @continuum-boot 'off'", tmux)
        self.assertIn("set -g @continuum-restore 'on'", tmux)
        self.assertIn("bind S display-popup", tmux)
        self.assertIn(" %Y-%m-%d ", tmux)
        self.assertIn(" %H:%M ", tmux)
        self.assertIn("#H", tmux)
        bashrc = profile[".bashrc"].decode()
        self.assertIn('export BAT_THEME="Nord"', bashrc)
        self.assertIn("fdfind --type f", bashrc)

    def render_data(self, data):
        data = json.loads(json.dumps(data))
        if data.get("chezmoi", {}).get("os") != "darwin" and "profile" not in data.get("workstation", {}):
            data.setdefault("workstation", {})["profile"] = "linux"
        result = self.raw_archive(data)
        result.check_returncode()
        return archive_files(result.stdout)

    def raw_archive(self, data, env=None):
        with tempfile.TemporaryDirectory(prefix="dotfiles-render-") as home:
            return subprocess.run(
                [CHEZMOI, "--source", str(ROOT), "--destination", home, "--config", "/dev/null",
                 "--config-format", "toml", "--override-data", json.dumps(data), "archive"],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env,
            )

    def test_linux_profile_is_required_and_environment_profile_works(self):
        prefix = "workstation profile is not set: run chezmoi init and choose linux or linux-full, set [data.workstation] profile in ~/.config/chezmoi/chezmoi.toml, or export WORKSTATION_PROFILE=linux|linux-full (got \""
        for profile, value in ((None, ""), ("foo", "foo")):
            data = {"chezmoi": {"os": "linux"}, "theme": "nord"}
            if profile is not None:
                data["workstation"] = {"profile": profile}
            result = self.raw_archive(data)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn((prefix + value + '\")').encode(), result.stderr)
        linux_data = {"chezmoi": {"os": "linux"}, "workstation": {"profile": "linux"}, "theme": "nord"}
        expected = self.raw_archive(linux_data)
        env = {**__import__("os").environ, "WORKSTATION_PROFILE": "linux"}
        actual = self.raw_archive({"chezmoi": {"os": "linux"}, "theme": "nord"}, env)
        self.assertEqual(actual.returncode, 0, actual.stderr)
        self.assertEqual(archive_files(actual.stdout), archive_files(expected.stdout))

    def test_linux_full_profile_keeps_the_tmux_behaviour(self):
        files, modes = self.render("linux", "linux-full")
        self.assertIn("fdfind --type f", files[".bashrc"].decode())
        tmux = files[".tmux.conf"].decode()
        self.assertIn("set -g @continuum-boot 'on'", tmux)
        self.assertIn("set -g @continuum-restore 'on'", tmux)
        self.assertIn("bind S display-popup", tmux)
        self.assertIn("set -s copy-command 'wl-copy'", tmux)
        self.assertIn("set -g status 2", tmux)
        self.assertIn("source-file -q ~/.tmux/claude/tmux-status.conf", tmux)
        self.assertNotIn("@claude-resurrect-claude-dir", tmux)
        self.assertEqual(modes[".tmux/claude/pane-title.sh"], 0o755)
        self.assertTrue(files[".tmux/claude/pane-title.sh"].startswith(b"#!/usr/bin/env bash\n#"))

    def test_editor_is_installed_on_each_profile(self):
        # The headless linux profile has no VS Code, so `code --wait` there broke git commit and crontab -e.
        for os_name, profile, editor in (("linux", "linux", "nvim"), ("linux", "linux-full", "code --wait"),
                                         ("darwin", None, "code --wait")):
            bashrc = self.render(os_name, profile)[0][".bashrc"].decode()
            self.assertIn(f'export EDITOR="{editor}"', bashrc, profile)
            self.assertIn(f'export VISUAL="{editor}"', bashrc, profile)

    def test_rendered_files_carry_no_home_paths(self):
        for theme in ("nord", "catppuccin-mocha"):
            for os_name, profile in PROFILES:
                data = {"chezmoi": {"os": os_name}, "theme": theme}
                if profile:
                    data["workstation"] = {"profile": profile}
                for name, content in self.render_data(data).items():
                    self.assertNotIn(str(Path.home()).encode(), content, name)
                    self.assertIsNone(re.search(rb"/home/[a-z]|/Users/[a-z]", content), name)

    def test_starship_renders_under_both_themes(self):
        for theme in ("nord", "catppuccin-mocha"):
            files = self.render_data({"chezmoi": {"os": "linux"}, "theme": theme})
            config = tomllib.loads(files[".config/starship.toml"].decode())
            palette = config["palettes"][config["palette"]]
            self.assertEqual(config["palette"], theme)
            expected = {"nord": "#88C0D0", "catppuccin-mocha": "#89DCEB"}[theme]
            self.assertEqual(palette["accent"], expected)
            for key in ("bg", "fg", "red", "yellow", "green", "purple"):
                self.assertRegex(palette[key], r"^#[0-9A-Fa-f]{6}$")
            self.assertEqual(config["username"]["style_user"], "bold accent")
            self.assertIn("$character", config["format"])

    def test_unknown_themes_fall_back_to_nord(self):
        for os_name in ("linux", "darwin"):
            nord = self.render_data({"chezmoi": {"os": os_name}, "theme": "nord"})
            for theme in ("", "no-such-theme"):
                with self.subTest(os=os_name, theme=theme):
                    self.assertEqual(
                        self.render_data({"chezmoi": {"os": os_name}, "theme": theme}),
                        nord,
                    )

    def test_nvim_colorscheme_follows_theme(self):
        expected = {"nord": ('"nordtheme/vim"', 'colorscheme = "nord"'),
                    "catppuccin-mocha": ('flavour = "mocha"', 'colorscheme = "catppuccin"')}
        for theme, (plugin, colorscheme) in expected.items():
            for os_name, profile in PROFILES:
                data = {"chezmoi": {"os": os_name}, "theme": theme}
                if profile:
                    data["workstation"] = {"profile": profile}
                files = self.render_data(data)
                self.assertLessEqual(NVIM_FILES, set(files))
                lua = files[".config/nvim/lua/plugins/colorscheme.lua"].decode()
                self.assertIn(plugin, lua)
                self.assertIn(colorscheme, lua)
            json.loads(files[".config/nvim/lazy-lock.json"])
            if LUAC:
                for name in NVIM_FILES - {".config/nvim/lazy-lock.json"}:
                    subprocess.run([LUAC, "-p", "-"], input=files[name], check=True)

    def test_btop_picks_the_catppuccin_theme_file(self):
        nord = self.render_data({"chezmoi": {"os": "linux"}})
        mocha = self.render_data({"chezmoi": {"os": "linux"}, "theme": "catppuccin-mocha"})
        self.assertIn(b'color_theme = "nord"', nord[".config/btop/btop.conf"])
        self.assertIn(b'color_theme = "catppuccin_mocha"', mocha[".config/btop/btop.conf"])
        theme = mocha[".config/btop/themes/catppuccin_mocha.theme"]
        self.assertEqual(hashlib.sha256(theme).hexdigest(), CATPPUCCIN_BTOP_SHA256)
        self.assertIn(b'theme[main_bg]="#1e1e2e"', theme)
        self.assertFalse(any("LICENSE" in name or "SOURCE" in name for name in mocha))

    def test_claude_resurrect_paths_come_from_machine_data(self):
        files = self.render_data({
            "chezmoi": {"os": "linux"},
            "tmux": {"claudeResurrectDir": "/home/u/.claude-alt", "claudeResurrectCommand": "/home/u/.local/bin/claude"},
        })
        tmux = files[".tmux.conf"].decode()
        self.assertIn("set -g @claude-resurrect-claude-dir '/home/u/.claude-alt'", tmux)
        self.assertIn("set -g @claude-resurrect-command '/home/u/.local/bin/claude'", tmux)

    def test_theme_switch_reaches_every_themed_file(self):
        nord = self.render_data({"chezmoi": {"os": "darwin"}})
        mocha = self.render_data({"chezmoi": {"os": "darwin"}, "theme": "catppuccin-mocha"})
        self.assertIn(b"theme = Nord", nord[".config/ghostty/config"])
        self.assertIn(b"theme = Catppuccin Mocha", mocha[".config/ghostty/config"])
        self.assertIn(b'BAT_THEME="Catppuccin Mocha"', mocha[".bashrc"])
        self.assertIn(b"bg=#F9E2AF", mocha[".tmux.conf"])
        self.assertIn(b"#F9E2AF", mocha[".tmux/claude/tmux-status.conf"])
        for editor in ("Code", "Cursor"):
            settings = json.loads(mocha[f"Library/Application Support/{editor}/User/settings.json"])
            self.assertEqual(settings["workbench.colorTheme"], "Catppuccin Mocha")

    def test_editor_settings_per_platform(self):
        linux, _ = self.render("linux", "linux-full")
        darwin, _ = self.render("darwin")
        self.assertNotIn(".config/Cursor/User/settings.json", linux)
        self.assertFalse(any(name.startswith("Library/") for name in linux))
        self.assertFalse(any(name.startswith(".config/Code") for name in darwin))
        code = json.loads(darwin["Library/Application Support/Code/User/settings.json"])
        cursor = json.loads(darwin["Library/Application Support/Cursor/User/settings.json"])
        self.assertTrue(code["chat.disableAIFeatures"])
        self.assertNotIn("chat.disableAIFeatures", cursor)
        self.assertTrue(cursor["terminal.integrated.macOptionIsMeta"])
        self.assertIn("terminal.integrated.defaultProfile.linux", json.loads(linux[".config/Code/User/settings.json"]))
        for files in (linux, darwin):
            for name, content in files.items():
                if name.endswith(".json"):
                    json.loads(content)

    def test_patch_sources_stay_out_of_the_target(self):
        files, _ = self.render("linux")
        self.assertFalse(any(".patches" in name for name in files))
        script = files["20-patch-tmux-resurrect.sh"].decode()
        self.assertIn("diff --git a/scripts/restore.sh", script)
        subprocess.run(["sh", "-n"], input=files["20-patch-tmux-resurrect.sh"], check=True)

    def test_ghostty_platform_options(self):
        linux, _ = self.render("linux", "linux-full")
        darwin, _ = self.render("darwin")
        linux_ghostty = linux[".config/ghostty/config"].decode()
        darwin_ghostty = darwin[".config/ghostty/config"].decode()
        self.assertNotIn("macos-titlebar-style", linux_ghostty)
        self.assertNotIn("macos-option-as-alt", linux_ghostty)
        self.assertIn("macos-titlebar-style = tabs", darwin_ghostty)
        self.assertIn("macos-option-as-alt = true", darwin_ghostty)

    def test_existing_pyenv_function_does_not_recreate_lazy_wrappers(self):
        files, _ = self.render("linux")
        with tempfile.TemporaryDirectory(prefix="dotfiles-bash-") as temp:
            root = Path(temp)
            bashrc = root / "bashrc"
            bashrc.write_bytes(files[".bashrc"])
            result = subprocess.run(
                [
                    "bash",
                    "--noprofile",
                    "--norc",
                    "-ic",
                    'pyenv() { :; }; source "$1"; type -t _lazy_pyenv 2>/dev/null || true',
                    "bash",
                    str(bashrc),
                ],
                env={"HOME": temp, "PATH": "/usr/bin:/bin"},
                check=True,
                capture_output=True,
                text=True,
            )
        self.assertNotEqual(result.stdout.strip().splitlines()[-1:], ["function"])

    def test_pyenv_executable_keeps_lazy_wrapper_behavior(self):
        files, _ = self.render("linux")
        with tempfile.TemporaryDirectory(prefix="dotfiles-bash-") as temp:
            root = Path(temp)
            bashrc = root / "bashrc"
            bashrc.write_bytes(files[".bashrc"])
            binary_dir = root / "bin"
            binary_dir.mkdir()
            pyenv = binary_dir / "pyenv"
            pyenv.write_text("#!/bin/sh\nexit 0\n")
            pyenv.chmod(0o755)
            result = subprocess.run(
                [
                    "bash",
                    "--noprofile",
                    "--norc",
                    "-ic",
                    'source "$1"; type -t _lazy_pyenv 2>/dev/null || true',
                    "bash",
                    str(bashrc),
                ],
                env={"HOME": temp, "PATH": f"{binary_dir}:/usr/bin:/bin"},
                check=True,
                capture_output=True,
                text=True,
            )
        self.assertEqual(result.stdout.strip().splitlines()[-1:], ["function"])


class SourceTests(unittest.TestCase):
    def test_source_holds_no_logs_or_unmarked_empty_files(self):
        # chezmoi skips an empty source file unless its name has the empty_ attribute, so one without it is
        # stray output (an editor log, say), never a target. Logs are runtime output wherever they land.
        for path in ROOT.rglob("*"):
            if not path.is_file() or {".git", "__pycache__"} & set(path.relative_to(ROOT).parts):
                continue
            rel = path.relative_to(ROOT)
            self.assertNotEqual(path.suffix, ".log", rel)
            if path.stat().st_size == 0:
                self.assertRegex(path.name, r"^(?:(?:private|readonly|executable|encrypted)_)*empty_", rel)


if __name__ == "__main__":
    unittest.main()
