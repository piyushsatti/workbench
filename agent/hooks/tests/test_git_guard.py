"""git-guard.py: a table of allowed and denied commands, and the deny shape for each harness."""

import importlib.util
import json
import subprocess
import sys
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "git-guard.py"
SPEC = importlib.util.spec_from_file_location("git_guard", SCRIPT)
GUARD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(GUARD)

ALLOW = [
    "git status",
    "git log --oneline -5",
    'git commit -m "feat: thing"',
    "git commit -am 'fix: x'",
    "git commit --amend --no-edit",
    "git commit -m -n",                       # -n is the message
    "git commit -F msg.txt",
    'git commit -m "mention --no-verify in prose"',
    "git commit --verbose -m x",
    "git -C repo commit -m x",
    "git -c color.ui=always log",
    "git -c core.editor=vim commit",
    "git config user.email",                  # read
    "git config --get user.email",
    "git config --global --get user.name",
    "git config --list --show-origin",
    "git config get user.email",
    "git config core.editor vim",
    "git config --global pull.rebase false",
    "git push --no-verify",                    # out of scope: identity is checked at commit
    "ls -la && git status",
    'echo "remember to git commit -n later"',
    "echo GIT_AUTHOR_NAME is unset",
    "printenv GIT_AUTHOR_EMAIL",
    "HOME=/tmp/x ls",                          # no git involved
    "cat > notes.md <<'MD'\nProse: git commit --no-verify and GIT_AUTHOR_NAME=x.\nMD\necho done",
    "grep -n 'git commit -n' docs/*.md",
    "env | sort",
    # the usual agent commit form: message in a quoted $(cat <<'EOF' ...) heredoc
    "git commit -m \"$(cat <<'EOF'\nfeat: x\n\nmentions --no-verify and GIT_AUTHOR_NAME=x\nEOF\n)\"",
    "cd x && cat <<EOF > f\ngit commit -n\nEOF",
    "npm test && git add -p",
    "python3 -c 'print(1)'",
    "",
]

DENY = [
    # --no-verify / -n on commit
    "git commit --no-verify -m x",
    "git commit -n -m x",
    "git commit -nm x",
    "git commit -anm x",
    "git commit -m x -n",
    "git commit --no-verif -m x",             # unambiguous abbreviation git accepts
    "git commit '--no-verify' -m x",          # quoted flag
    'git commit "-n" -m x',
    "git commit -m x -- --no-verify",         # after `--`: denied, fail closed
    "/usr/bin/git commit --no-verify",
    "\\git commit -n",
    "cd repo && git commit --no-verify -m x",
    "true; git commit -n -m x",
    "git status || git commit -n",
    "git add . | git commit -n",
    "if git commit -n -m x; then echo ok; fi",
    "{ git commit -n; }",
    "(cd repo && git commit -n)",
    "echo $(git commit -n -m x)",
    'echo "$(git commit -n -m x)"',
    "echo `git commit -n`",
    "git -C repo commit --no-verify",
    "git --git-dir=.git commit -n",
    "git -c core.editor=vim commit -n",
    # -c / --config-env overrides, before the subcommand
    "git -c core.hooksPath=/dev/null commit -m x",
    "git -c core.hookspath= commit -m x",
    "git -c CORE.HOOKSPATH=/x commit",
    "git -c user.email=bad@example.com commit -m x",
    "git -c user.name=Mallory commit -m x",
    "git -c author.email=bad@example.com commit",
    "git -c 'alias.ci=commit --no-verify' ci -m x",
    "git -c alias.x='!sh -c true' x",
    "git -c include.path=/tmp/evil commit",
    "git --config-env=core.hooksPath=EMPTY commit",
    "git --config-env core.hooksPath=EMPTY commit",
    # git config writes
    "git config user.email bad@example.com",
    "git config --global user.name Mallory",
    "git config --local user.email x@y",
    "git config -f ~/.gitconfig user.email x@y",
    "git config set user.email x@y",
    "git config --unset core.hooksPath",
    "git config --global --unset-all core.hooksPath",
    "git config core.hooksPath /dev/null",
    "git config --global alias.ci 'commit --no-verify'",
    "git config --global include.path /tmp/x",
    "git config credential.helper store",
    "git config --remove-section user",
    "git config --global --rename-section core old",
    "git config --global --edit",
    "git config -e",
    "git config --replace-all user.email x@y",
    # identity / config environment
    "GIT_AUTHOR_NAME=Mallory git commit -m x",
    "GIT_AUTHOR_EMAIL=bad@example.com git commit -m x",
    "GIT_COMMITTER_EMAIL=bad@example.com git commit -m x",
    "GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0= git commit",
    "GIT_CONFIG_GLOBAL=/dev/null git commit -m x",
    "GIT_CONFIG_PARAMETERS=\"'core.hookspath'=''\" git commit",
    "export GIT_AUTHOR_EMAIL=bad@example.com; git commit -m x",
    "export GIT_COMMITTER_NAME=x",
    "env GIT_AUTHOR_EMAIL=bad@example.com git commit -m x",
    "env -u PATH GIT_COMMITTER_NAME=x git commit",
    "FOO=1 GIT_AUTHOR_DATE=now git commit",
    # environment that hides ~/.gitconfig (and so core.hooksPath)
    "env -i git commit -m x",
    "env -i PATH=/usr/bin git commit -m x",
    "env --ignore-environment git commit",
    "env -u HOME git commit -m x",
    "HOME=/tmp git commit -m x",
    "XDG_CONFIG_HOME=/tmp HOME=/tmp git commit",
    "export HOME=/tmp && git commit -m x",
    "env -i sh -c 'git commit -m x'",
    # wrappers and nested shells
    "sh -c 'git commit --no-verify'",
    "bash -lc 'git commit -n -m x'",
    "bash -c \"cd repo && git commit -n\"",
    "zsh -c 'GIT_AUTHOR_NAME=x git commit'",
    "sh -c \"sh -c 'git commit -n'\"",
    "eval 'git commit -n -m x'",
    "eval git commit --no-verify",
    "command git commit -n",
    "sudo -u me git commit -n",
    "nohup git commit -n &",
    "timeout 30 git commit -n",
    "xargs -n 1 git commit -n < list",
    "env -S 'GIT_AUTHOR_NAME=x git commit'",
    "printf 'git commit -n' | sh",
    "bash <<'EOF'\ngit commit --no-verify -m x\nEOF",
    "bash <<< 'git commit -n'",
    "cd x && bash <<EOF\ngit commit -n\nEOF",
    "git commit -m \"$(cat <<'EOF'\nx\nEOF\n)\" --no-verify",
]


def run(command, harness="claude", raw=None):
    payload = raw if raw is not None else json.dumps({"tool_name": "Bash", "cwd": "/tmp",
                                                      "tool_input": {"command": command}})
    return subprocess.run([sys.executable, "-I", str(SCRIPT), "--harness", harness], input=payload,
                          capture_output=True, text=True)


class DecisionTable(unittest.TestCase):
    def test_allowed(self):
        for command in ALLOW:
            with self.subTest(command=command):
                self.assertIsNone(GUARD.decide(command))

    def test_denied(self):
        for command in DENY:
            with self.subTest(command=command):
                reason = GUARD.decide(command)
                self.assertIsNotNone(reason)
                self.assertTrue(reason.startswith("git-guard: "))

    def test_unparseable_git_command_fails_closed(self):
        self.assertIn("could not parse", GUARD.decide("git commit -m 'unterminated"))
        self.assertIsNone(GUARD.decide("echo 'unterminated"))

    def test_argv_form_command(self):
        # Codex's tool_input.command type is not documented; an argv list is handled too.
        self.assertIsNotNone(GUARD.decide(["git", "commit", "--no-verify"]))
        self.assertIsNone(GUARD.decide(["git", "status"]))
        self.assertIsNotNone(GUARD.decide(["bash", "-lc", "git commit -n"]))


class OutputShape(unittest.TestCase):
    def check_deny(self, harness):
        result = run("git commit --no-verify -m x", harness)
        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        self.assertEqual(set(output), {"hookSpecificOutput"})
        specific = output["hookSpecificOutput"]
        self.assertEqual(specific["hookEventName"], "PreToolUse")
        self.assertEqual(specific["permissionDecision"], "deny")
        self.assertIn("no-verify", specific["permissionDecisionReason"])

    def check_allow(self, harness):
        result = run("git status", harness)
        self.assertEqual((result.returncode, result.stdout, result.stderr), (0, "", ""))

    def test_claude_shape(self):
        self.check_deny("claude")
        self.check_allow("claude")

    def test_codex_shape(self):
        self.check_deny("codex")
        self.check_allow("codex")

    def test_bad_payload_blocks_with_exit_2(self):
        for harness in ("claude", "codex"):
            result = run(None, harness, raw="not json")
            self.assertEqual(result.returncode, 2)
            self.assertIn("unreadable hook payload", result.stderr)

    def test_unknown_harness_is_an_error(self):
        self.assertEqual(run("git status", "cursor").returncode, 2)

    def test_sample_configs_call_the_installed_path(self):
        for harness in ("claude", "codex"):
            config = json.loads((SCRIPT.parent / f"git-guard.{harness}.json").read_text())
            group = config["hooks"]["PreToolUse"][0]
            self.assertIn("Bash", group["matcher"])
            command = group["hooks"][0]["command"]
            self.assertEqual(command, f'sh -c \'exec "$HOME/.local/bin/git-guard" --harness {harness}\'')


if __name__ == "__main__":
    unittest.main()
