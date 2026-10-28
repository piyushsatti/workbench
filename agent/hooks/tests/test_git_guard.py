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
    "git merge -n feature",
    "git merge --no-verify-signatures feature",
    "unset GIT_PAGER; git commit -m x",
    "export -n FOO; git commit -m x",
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
    "git status # a comment that isn't checked",
    "git commit -m x  # done",
    "echo ${#PATH} && git status",
    "env -v git status",
]

# Each DENY row names the rule that must refuse it: a fragment of that rule's reason.
MERGE_NO_VERIFY = "pre-merge-commit"
COMMIT_N = "includes -n"
COMMIT_NO_VERIFY = "skips the pre-commit"
CONFIG_HIDDEN = "hides ~/.gitconfig"
CONFIG_OVERRIDE = "for one command"
CONFIG_EDIT = "config --edit"
CONFIG_SECTION = "touches identity"
CONFIG_WRITE = "would write"
ENV_IDENTITY = "through the environment"

DENY = [
    # --no-verify / -n on commit
    ("git commit --no-verify -m x", COMMIT_NO_VERIFY),
    ("git commit -n -m x", COMMIT_N),
    ("git commit -nm x", COMMIT_N),
    ("git commit -anm x", COMMIT_N),
    ("git commit -m x -n", COMMIT_N),
    ("git commit --no-verif -m x", COMMIT_NO_VERIFY),             # unambiguous abbreviation git accepts
    ("git commit '--no-verify' -m x", COMMIT_NO_VERIFY),          # quoted flag
    ('git commit "-n" -m x', COMMIT_N),
    ("git commit -m x -- --no-verify", COMMIT_NO_VERIFY),         # after `--`: denied, fail closed
    ("/usr/bin/git commit --no-verify", COMMIT_NO_VERIFY),
    ("\\git commit -n", COMMIT_N),
    ("cd repo && git commit --no-verify -m x", COMMIT_NO_VERIFY),
    ("true; git commit -n -m x", COMMIT_N),
    ("git status || git commit -n", COMMIT_N),
    ("git add . | git commit -n", COMMIT_N),
    ("if git commit -n -m x; then echo ok; fi", COMMIT_N),
    ("{ git commit -n; }", COMMIT_N),
    ("(cd repo && git commit -n)", COMMIT_N),
    ("echo $(git commit -n -m x)", COMMIT_N),
    ('echo "$(git commit -n -m x)"', COMMIT_N),
    ("echo `git commit -n`", COMMIT_N),
    ("git -C repo commit --no-verify", COMMIT_NO_VERIFY),
    ("git --git-dir=.git commit -n", COMMIT_N),
    ("git -c core.editor=vim commit -n", COMMIT_N),
    # -c / --config-env overrides, before the subcommand
    ("git -c core.hooksPath=/dev/null commit -m x", CONFIG_OVERRIDE),
    ("git -c core.hookspath= commit -m x", CONFIG_OVERRIDE),
    ("git -c CORE.HOOKSPATH=/x commit", CONFIG_OVERRIDE),
    ("git -c user.email=bad@example.com commit -m x", CONFIG_OVERRIDE),
    ("git -c user.name=Mallory commit -m x", CONFIG_OVERRIDE),
    ("git -c author.email=bad@example.com commit", CONFIG_OVERRIDE),
    ("git -c 'alias.ci=commit --no-verify' ci -m x", CONFIG_OVERRIDE),
    ("git -c alias.x='!sh -c true' x", CONFIG_OVERRIDE),
    ("git -c include.path=/tmp/evil commit", CONFIG_OVERRIDE),
    ("git --config-env=core.hooksPath=EMPTY commit", CONFIG_OVERRIDE),
    ("git --config-env core.hooksPath=EMPTY commit", CONFIG_OVERRIDE),
    # git config writes
    ("git config user.email bad@example.com", CONFIG_WRITE),
    ("git config --global user.name Mallory", CONFIG_WRITE),
    ("git config --local user.email x@y", CONFIG_WRITE),
    ("git config -f ~/.gitconfig user.email x@y", CONFIG_WRITE),
    ("git config set user.email x@y", CONFIG_WRITE),
    ("git config --unset core.hooksPath", CONFIG_WRITE),
    ("git config --global --unset-all core.hooksPath", CONFIG_WRITE),
    ("git config core.hooksPath /dev/null", CONFIG_WRITE),
    ("git config --global alias.ci 'commit --no-verify'", CONFIG_WRITE),
    ("git config --global include.path /tmp/x", CONFIG_WRITE),
    ("git config credential.helper store", CONFIG_WRITE),
    ("git config --remove-section user", CONFIG_SECTION),
    ("git config --global --rename-section core old", CONFIG_SECTION),
    ("git config --global --edit", CONFIG_EDIT),
    ("git config -e", CONFIG_EDIT),
    ("git config --replace-all user.email x@y", CONFIG_WRITE),
    # identity / config environment
    ("GIT_AUTHOR_NAME=Mallory git commit -m x", ENV_IDENTITY),
    ("GIT_AUTHOR_EMAIL=bad@example.com git commit -m x", ENV_IDENTITY),
    ("GIT_COMMITTER_EMAIL=bad@example.com git commit -m x", ENV_IDENTITY),
    ("GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0= git commit", ENV_IDENTITY),
    ("GIT_CONFIG_GLOBAL=/dev/null git commit -m x", ENV_IDENTITY),
    ("GIT_CONFIG_PARAMETERS=\"'core.hookspath'=''\" git commit", ENV_IDENTITY),
    ("export GIT_AUTHOR_EMAIL=bad@example.com; git commit -m x", ENV_IDENTITY),
    ("export GIT_COMMITTER_NAME=x", ENV_IDENTITY),
    ("env GIT_AUTHOR_EMAIL=bad@example.com git commit -m x", ENV_IDENTITY),
    ("env -u PATH GIT_COMMITTER_NAME=x git commit", ENV_IDENTITY),
    ("FOO=1 GIT_AUTHOR_DATE=now git commit", ENV_IDENTITY),
    # environment that hides ~/.gitconfig (and so core.hooksPath)
    ("env -i git commit -m x", CONFIG_HIDDEN),
    ("env -i PATH=/usr/bin git commit -m x", CONFIG_HIDDEN),
    ("env --ignore-environment git commit", CONFIG_HIDDEN),
    ("env -u HOME git commit -m x", CONFIG_HIDDEN),
    ("HOME=/tmp git commit -m x", CONFIG_HIDDEN),
    ("XDG_CONFIG_HOME=/tmp HOME=/tmp git commit", CONFIG_HIDDEN),
    ("export HOME=/tmp && git commit -m x", CONFIG_HIDDEN),
    ("env -i sh -c 'git commit -m x'", CONFIG_HIDDEN),
    # wrappers and nested shells
    ("sh -c 'git commit --no-verify'", COMMIT_NO_VERIFY),
    ("bash -lc 'git commit -n -m x'", COMMIT_N),
    ("bash -c \"cd repo && git commit -n\"", COMMIT_N),
    ("zsh -c 'GIT_AUTHOR_NAME=x git commit'", ENV_IDENTITY),
    ("sh -c \"sh -c 'git commit -n'\"", COMMIT_N),
    ("eval 'git commit -n -m x'", COMMIT_N),
    ("eval git commit --no-verify", COMMIT_NO_VERIFY),
    ("command git commit -n", COMMIT_N),
    ("sudo -u me git commit -n", COMMIT_N),
    ("nohup git commit -n &", COMMIT_N),
    ("timeout 30 git commit -n", COMMIT_N),
    ("xargs -n 1 git commit -n < list", COMMIT_N),
    ("env -S 'GIT_AUTHOR_NAME=x git commit'", ENV_IDENTITY),
    ("printf 'git commit -n' | sh", COMMIT_N),
    ("bash <<'EOF'\ngit commit --no-verify -m x\nEOF", COMMIT_NO_VERIFY),
    ("bash <<< 'git commit -n'", COMMIT_N),
    ("cd x && bash <<EOF\ngit commit -n\nEOF", COMMIT_N),
    ("git commit -m \"$(cat <<'EOF'\nx\nEOF\n)\" --no-verify", COMMIT_NO_VERIFY),
    # From the 2026-10-27 audit: each of these was allowed before.
    ("git commit -m fix#1 --no-verify", COMMIT_NO_VERIFY),         # a # inside a word is not a comment
    ("echo a#b && git commit --no-verify -m x", COMMIT_NO_VERIFY),
    ("git commit \\\n-n -m x", COMMIT_N),                 # backslash-newline joins the lines
    ("git commit -a --untracked-files -n -m x", COMMIT_N), # --untracked-files takes no separate value
    ("git config --rename-section evil core", CONFIG_SECTION),   # the new name counts too
    ("env -S'git commit -n -m x'", COMMIT_N),              # -S with an attached script, not -i
    ("env -P /usr/bin git commit -n -m x", COMMIT_N),      # -P takes a value
    ("env -iS 'git commit --no-verify -m x'", COMMIT_NO_VERIFY),   # a flag bundle ending in -S
    ("env -vS'git commit -n -m x'", COMMIT_N),
    ("env -iC /tmp git commit --no-verify -m x", COMMIT_NO_VERIFY),  # a flag bundle ending in -C
    ("env -iu HOME git commit -m x", CONFIG_HIDDEN),            # still env -i
    ("bash -o pipefail -c 'git commit -n -m x'", COMMIT_N),
    ("bash -O extglob -c 'git commit -n -m x'", COMMIT_N),
    ("git config -- user.email evil@example.com", CONFIG_WRITE),
    ("git config -- core.hooksPath /dev/null", CONFIG_WRITE),
    ("git config --global -- user.name x", CONFIG_WRITE),
    ("unset HOME; git commit -m x", CONFIG_HIDDEN),
    ("unset -v XDG_CONFIG_HOME HOME && git commit -m x", CONFIG_HIDDEN),
    ("export -n HOME; git commit -m x", CONFIG_HIDDEN),
    ("declare +x HOME; git commit -m x", CONFIG_HIDDEN),
    ("typeset -g +x HOME; git commit -m x", CONFIG_HIDDEN),
    ("git merge --no-verify feature", MERGE_NO_VERIFY),
    ("git -C repo pull --no-verify origin main", MERGE_NO_VERIFY),
    ("coproc git commit -n -m x", COMMIT_N),
    ("coproc worker { git commit --no-verify -m x; }", COMMIT_NO_VERIFY),
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
        for command, rule in DENY:
            with self.subTest(command=command):
                reason = GUARD.decide(command)
                self.assertIsNotNone(reason)
                self.assertTrue(reason.startswith("git-guard: "))
                self.assertIn(rule, reason)

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
