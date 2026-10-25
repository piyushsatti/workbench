"""Per-machine git identity: the chezmoi prompt, ~/.gitconfig, and the global pre-commit hook.

The hook is run directly inside a throwaway repository (cwd = the repo, HOME = a temp dir), the
way git runs it, with identity varied through repo config, GIT_CONFIG_PARAMETERS (what `git -c`
exports) and GIT_AUTHOR_* / GIT_COMMITTER_* (what `--author` and the environment set). No commit
is made: nothing is staged or committed in any repository.
"""

import io
import json
import os
import shutil
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CHEZMOI = shutil.which("chezmoi")
GIT = shutil.which("git")
NAME = "Pat O'Brien"
EMAIL = "pat@example.com"


def chezmoi(*args, stdin=None):
    return subprocess.run([CHEZMOI, "--source", str(ROOT), "--config", "/dev/null", "--config-format", "toml", *args],
                          input=stdin, check=True, capture_output=True, text=True).stdout


def render(path, data):
    return chezmoi("--override-data", json.dumps(data), "execute-template", stdin=(ROOT / path).read_text())


def clean_env(home, **extra):
    env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
    env.update(HOME=str(home), XDG_CONFIG_HOME=str(Path(home) / ".config"), GIT_CONFIG_NOSYSTEM="1",
               LC_ALL="C", **extra)
    return env


@unittest.skipUnless(CHEZMOI and GIT, "chezmoi and git are required")
class IdentityTests(unittest.TestCase):
    DATA = {"chezmoi": {"os": "linux"}, "workstation": {"profile": "linux"},
            "git": {"name": NAME, "email": EMAIL, "signingkey": ""}}

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="identity-")
        self.home = Path(self.temp.name) / "home"
        self.repo = Path(self.temp.name) / "repo"
        self.home.mkdir()
        (self.home / ".gitconfig").write_text(render("dot_gitconfig.tmpl", self.DATA))
        self.hook = self.home / ".config/git/hooks/pre-commit"
        self.hook.parent.mkdir(parents=True)
        self.hook.write_text(render("dot_config/git/hooks/executable_pre-commit.tmpl", self.DATA))
        self.hook.chmod(0o755)
        subprocess.run([GIT, "init", "-q", str(self.repo)], check=True, env=clean_env(self.home))

    def tearDown(self):
        self.temp.cleanup()

    def run_hook(self, hook=None, **env):
        return subprocess.run([str(hook or self.hook)], cwd=self.repo, env=clean_env(self.home, **env),
                              capture_output=True, text=True)

    # ---- rendering ----------------------------------------------------------------------------

    def test_config_template_prompts_once_for_identity(self):
        out = chezmoi("execute-template", "--init",
                      "--promptString", f"Git commit name={NAME}",
                      "--promptString", f"Git commit email={EMAIL}",
                      "--promptString", "Git signing key (blank for none)=",
                      "--override-data", json.dumps({"chezmoi": {"os": "linux"}}),
                      "--promptChoice", "Workstation profile=linux",
                      stdin=(ROOT / ".chezmoi.toml.tmpl").read_text())
        self.assertIn("[data.workstation]", out)
        self.assertIn('profile = "linux"', out)
        self.assertIn("[data.git]", out)
        self.assertIn('name = "Pat O\'Brien"', out)
        self.assertIn(f'email = "{EMAIL}"', out)
        self.assertIn('signingkey = ""', out)

    def test_config_template_keeps_stored_answers(self):
        with tempfile.NamedTemporaryFile("w", suffix=".toml") as config:
            config.write('[data.workstation]\nprofile = "linux-full"\n[data.git]\nname = "Stored"\nemail = "stored@example.com"\nsigningkey = ""\n')
            config.flush()
            out = subprocess.run([CHEZMOI, "--source", str(ROOT), "--config", config.name, "--config-format", "toml", "execute-template",
                                  "--init", "--no-tty", "--override-data", json.dumps({"chezmoi": {"os": "linux"}})], input=(ROOT / ".chezmoi.toml.tmpl").read_text(),
                                 env={**os.environ, "WORKSTATION_PROFILE": "linux-full"},
                                 check=True, capture_output=True, text=True).stdout
        self.assertIn('name = "Stored"', out)
        self.assertIn('email = "stored@example.com"', out)
        self.assertIn('profile = "linux-full"', out)

    def test_config_template_profile_environment_skips_prompt(self):
        env = {**os.environ, "WORKSTATION_PROFILE": "linux-full"}
        out = subprocess.run([CHEZMOI, "--source", str(ROOT), "--config", "/dev/null", "--config-format", "toml", "execute-template",
                              "--init", "--no-tty", "--override-data", json.dumps({"chezmoi": {"os": "linux"}})],
                             input=(ROOT / ".chezmoi.toml.tmpl").read_text(), env=env, check=True,
                             capture_output=True, text=True).stdout
        self.assertIn('profile = "linux-full"', out)

    def test_config_template_does_not_ask_profile_on_darwin(self):
        out = subprocess.run([CHEZMOI, "--source", str(ROOT), "--config", "/dev/null", "--config-format", "toml", "execute-template",
                              "--init", "--no-tty", "--override-data", json.dumps({"chezmoi": {"os": "darwin"}})],
                             input=(ROOT / ".chezmoi.toml.tmpl").read_text(), check=True,
                             capture_output=True, text=True).stdout
        self.assertNotIn("[data.workstation]", out)

    def test_gitconfig_sets_identity_and_hooks_path(self):
        env = clean_env(self.home)
        get = lambda key: subprocess.run([GIT, "config", "--global", "--get", key], env=env,
                                         capture_output=True, text=True).stdout.strip()
        self.assertEqual(get("user.name"), NAME)
        self.assertEqual(get("user.email"), EMAIL)
        self.assertEqual(get("core.hooksPath"), "~/.config/git/hooks")
        self.assertEqual(get("user.signingkey"), "")
        hooks = subprocess.run([GIT, "rev-parse", "--git-path", "hooks"], cwd=self.repo, env=env,
                               capture_output=True, text=True).stdout.strip()
        self.assertEqual(Path(hooks).resolve(), self.hook.parent.resolve())

    def render_with(self, data, path_dirs):
        env = {**os.environ, "PATH": os.pathsep.join(path_dirs)}
        return subprocess.run([CHEZMOI, "--source", str(ROOT), "--config", "/dev/null", "--config-format", "toml",
                               "--override-data", json.dumps(data), "execute-template"],
                              input=(ROOT / "dot_gitconfig.tmpl").read_text(), env=env,
                              check=True, capture_output=True, text=True).stdout

    def git_get(self, text, key):
        path = self.home / "rendered.gitconfig"
        path.write_text(text)
        done = subprocess.run(["git", "config", "--file", str(path), "--get", key], capture_output=True, text=True)
        return done.stdout.strip() if done.returncode == 0 else None

    def test_shared_settings_are_on_every_machine(self):
        text = render("dot_gitconfig.tmpl", self.DATA)
        for key, value in (("init.defaultbranch", "main"), ("pull.rebase", "false"), ("fetch.prune", "true"),
                           ("push.autosetupremote", "true"), ("merge.conflictstyle", "zdiff3"),
                           ("rerere.enabled", "true"), ("diff.colormoved", "default")):
            self.assertEqual(self.git_get(text, key), value, key)

    def test_editor_follows_os_and_profile(self):
        for os_name, profile, editor in (("darwin", "", "code --wait"), ("linux", "linux-full", "code --wait"),
                                         ("linux", "linux", "nvim")):
            data = {**self.DATA, "chezmoi": {"os": os_name}, "workstation": {"profile": profile}}
            self.assertEqual(self.git_get(render("dot_gitconfig.tmpl", data), "core.editor"), editor, (os_name, profile))

    def test_delta_settings_only_where_delta_is_installed(self):
        without = self.render_with(self.DATA, ["/usr/bin", "/bin"])
        self.assertIsNone(self.git_get(without, "core.pager"))
        self.assertIsNone(self.git_get(without, "delta.navigate"))
        fake = self.home / "fakebin"
        fake.mkdir()
        (fake / "delta").write_text("#!/bin/sh\n")
        (fake / "delta").chmod(0o755)
        with_delta = self.render_with(self.DATA, [str(fake), "/usr/bin", "/bin"])
        self.assertEqual(self.git_get(with_delta, "core.pager"), "delta")
        self.assertEqual(self.git_get(with_delta, "interactive.difffilter"), "delta --color-only")
        self.assertEqual(self.git_get(with_delta, "delta.side-by-side"), "true")

    def test_signing_key_is_optional(self):
        data = {**self.DATA, "git": {**self.DATA["git"], "signingkey": "ABC123"}}
        self.assertIn('signingkey = "ABC123"', render("dot_gitconfig.tmpl", data))
        self.assertNotIn("signingkey", render("dot_gitconfig.tmpl", self.DATA))

    def test_local_include_cannot_override_identity_or_hooks_path(self):
        (self.home / ".gitconfig.local").write_text("[user]\n\temail = other@example.com\n[core]\n\thooksPath = /dev/null\n")
        env = clean_env(self.home)
        get = lambda key: subprocess.run([GIT, "config", "--get", key], cwd=self.repo, env=env,
                                         capture_output=True, text=True).stdout.strip()
        self.assertEqual(get("user.email"), EMAIL)
        self.assertEqual(get("core.hooksPath"), "~/.config/git/hooks")

    def test_archive_places_an_executable_hook_and_gitconfig(self):
        with tempfile.TemporaryDirectory() as dest:
            archive = subprocess.run([CHEZMOI, "--source", str(ROOT), "--destination", dest, "--config", "/dev/null",
                                      "--config-format", "toml", "--override-data", json.dumps(self.DATA), "archive"],
                                     check=True, stdout=subprocess.PIPE).stdout
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:*") as bundle:
            members = {m.name: m for m in bundle.getmembers() if m.isfile()}
        self.assertIn(".gitconfig", members)
        self.assertEqual(members[".config/git/hooks/pre-commit"].mode & 0o777, 0o755)

    def test_rendered_files_carry_no_identity_without_data(self):
        bare = {"chezmoi": {"os": "linux"}, "workstation": {"profile": "linux"}}
        self.assertNotIn("[user]", render("dot_gitconfig.tmpl", bare))
        hook = render("dot_config/git/hooks/executable_pre-commit.tmpl", bare)
        self.assertIn("expected_email=''", hook)
        subprocess.run(["sh", "-n"], input=hook, text=True, check=True)

    # ---- the hook -----------------------------------------------------------------------------

    def test_matching_identity_passes(self):
        result = self.run_hook()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_email_case_is_ignored(self):
        result = self.run_hook(GIT_AUTHOR_EMAIL=EMAIL.upper(), GIT_COMMITTER_EMAIL=EMAIL.upper())
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_repo_config_mismatch_is_refused(self):
        subprocess.run([GIT, "config", "user.email", "bad@example.com"], cwd=self.repo, check=True,
                       env=clean_env(self.home))
        result = self.run_hook()
        self.assertEqual(result.returncode, 1)
        self.assertIn("author would be Pat O'Brien <bad@example.com>", result.stderr)
        self.assertIn(f"expected identity is {NAME} <{EMAIL}>", result.stderr)

    def test_dash_c_override_is_refused(self):
        result = self.run_hook(GIT_CONFIG_PARAMETERS="'user.name'='Mallory'")
        self.assertEqual(result.returncode, 1)
        self.assertIn("Mallory", result.stderr)

    def test_author_override_is_refused(self):
        result = self.run_hook(GIT_AUTHOR_NAME="B", GIT_AUTHOR_EMAIL="bad@example.com")
        self.assertEqual(result.returncode, 1)
        self.assertIn("author would be B <bad@example.com>", result.stderr)

    def test_committer_override_is_refused(self):
        result = self.run_hook(GIT_COMMITTER_EMAIL="bad@example.com")
        self.assertEqual(result.returncode, 1)
        self.assertIn("committer would be", result.stderr)

    def test_no_identity_configured_is_refused(self):
        bare = self.home / "bare-pre-commit"
        bare.write_text(render("dot_config/git/hooks/executable_pre-commit.tmpl", {"chezmoi": {"os": "linux"}}))
        bare.chmod(0o755)
        result = self.run_hook(hook=bare)
        self.assertEqual(result.returncode, 1)
        self.assertIn("no expected git identity", result.stderr)

    def test_chains_to_the_repo_hook(self):
        repo_hook = self.repo / ".git/hooks/pre-commit"
        marker = Path(self.temp.name) / "ran"
        repo_hook.write_text(f'#!/bin/sh\ntouch "{marker}"\nexit 3\n')
        repo_hook.chmod(0o755)
        result = self.run_hook()
        self.assertEqual(result.returncode, 3)
        self.assertTrue(marker.exists())

    def test_repo_hook_is_not_run_on_mismatch(self):
        repo_hook = self.repo / ".git/hooks/pre-commit"
        marker = Path(self.temp.name) / "ran"
        repo_hook.write_text(f'#!/bin/sh\ntouch "{marker}"\n')
        repo_hook.chmod(0o755)
        result = self.run_hook(GIT_AUTHOR_EMAIL="bad@example.com")
        self.assertEqual(result.returncode, 1)
        self.assertFalse(marker.exists())

    def test_non_executable_repo_hook_is_skipped(self):
        (self.repo / ".git/hooks/pre-commit").write_text("#!/bin/sh\nexit 3\n")
        self.assertEqual(self.run_hook().returncode, 0)


if __name__ == "__main__":
    unittest.main()
