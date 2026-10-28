#!/usr/bin/env python3
"""PreToolUse guard: deny shell commands that would skip or rewrite the git identity check.

The real identity check is the global git pre-commit hook (setup/dotfiles/dot_config/git/hooks),
which sees what git actually resolves on every route. This guard only closes the routes that
switch that hook off or rewrite the identity it checks:

- `git commit --no-verify` / `-n` (also bundled, quoted, abbreviated, or after `--`)
- `git -c KEY=...` / `--config-env KEY=...` for identity, hook, alias and include keys
- `git config` writes to user.*, author.*, committer.*, core.hooksPath, alias.*, include*, credential.*
- assignments to GIT_AUTHOR_*, GIT_COMMITTER_*, GIT_CONFIG* (inline, `env`, `export`)
- a git command run with HOME / XDG_CONFIG_HOME changed or the environment cleared (`env -i`),
  which hides ~/.gitconfig and with it core.hooksPath

It walks `&&`, `||`, `;`, `|`, newlines, `$(...)`, backticks, `env`, common wrappers, `sh -c`,
`eval`, shell heredocs and `... | sh`. A command it cannot tokenize that mentions git is denied.

Usage (stdin is the hook payload):  git-guard [--harness claude|codex]
Install: copy to ~/.local/bin/git-guard, mode 0755 (outside agent-writable paths); see install.json.
Standard library only. Run as `python3 -I` or via the shebang from the installed path.
"""

import json
import os
import re
import shlex
import sys

SEPARATORS = {"&&", "||", ";", "|", "&", "|&", ";;", "(", ")", "`", "\n", "{", "}", "!"}
KEYWORDS = {"if", "then", "else", "elif", "do", "while", "until", "fi", "done", "coproc"}  # ! { } are SEPARATORS
SHELLS = {"sh", "bash", "zsh", "dash", "ksh", "mksh", "fish"}
# Wrappers that run their remaining arguments as a command. Value: options that take a separate value.
WRAPPERS = {
    "command": set(), "builtin": set(), "exec": {"-a"}, "nohup": set(), "time": {"-f", "-o"},
    "noglob": set(), "nice": {"-n"}, "ionice": {"-c", "-n", "-p"}, "stdbuf": {"-i", "-o", "-e"},
    "sudo": {"-u", "-g", "-h", "-p", "-C", "-U", "-r", "-t", "-D"}, "doas": {"-u", "-C"},
    "timeout": {"-s", "-k", "--signal", "--kill-after"}, "caffeinate": {"-t", "-w"},
    "xargs": {"-I", "-L", "-n", "-P", "-s", "-d", "-E", "-e", "-a", "--delimiter", "--arg-file",
              "--max-args", "--max-procs", "--max-lines", "--max-chars", "--replace", "--eof"},
    "watch": {"-n", "-d", "--interval"}, "flock": {"-w", "-E", "-c"}, "unbuffer": set(),
}
# Wrappers whose first positional argument is not the command (timeout DURATION, flock FILE).
WRAPPER_POSITIONALS = {"timeout": 1, "flock": 1}

# --exec-path, --list-cmds and --super-prefix take a value only with `=`, so they are not here.
GIT_GLOBAL_WITH_VALUE = {"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env", "--attr-source"}
# Long options only; short ones are COMMIT_SHORT_VALUE below.
COMMIT_WITH_VALUE = {"--message", "--file", "--reuse-message",
                     "--reedit-message", "--template", "--author", "--date", "--cleanup",
                     "--fixup", "--squash", "--trailer", "--pathspec-from-file"}
# Short commit options that take a value (the rest of a bundle is that value): -m, -F, -c, -C, -t, -S, -u.
COMMIT_SHORT_VALUE = set("mFcCtSu")

# Config keys (lower-cased) that a -c / --config-env / git config write may not touch.
PROTECTED_KEY = re.compile(
    r"^(user|author|committer|alias|credential|hook)\..+$|^core\.hookspath$|^include\.|^includeif\.",
)
PROTECTED_SECTION = re.compile(r"^(user|author|committer|core|alias|credential|hook|include|includeif)(\..*)?$")
PROTECTED_ENV = re.compile(r"^(GIT_AUTHOR_\w*|GIT_COMMITTER_\w*|GIT_CONFIG\w*)$")
CONFIG_HIDING_ENV = {"HOME", "XDG_CONFIG_HOME"}
ASSIGNMENT = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*)\+?=(.*)$", re.S)

CONFIG_WRITE_FLAGS = {"--replace-all", "--add", "--unset", "--unset-all", "--rename-section",
                      "--remove-section", "--edit", "-e"}
CONFIG_READ_FLAGS = {"--get", "--get-all", "--get-regexp", "--get-urlmatch", "--list", "-l",
                     "--get-color", "--get-colorbool"}
CONFIG_WITH_VALUE = {"-f", "--file", "--blob", "--type", "--default", "--comment", "--value"}
CONFIG_SUBCOMMANDS_WRITE = {"set", "unset", "rename-section", "remove-section", "edit"}
CONFIG_SUBCOMMANDS_READ = {"get", "list"}

HEREDOC = re.compile(r"<<(-?)[ \t]*(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\2")


class Denied(Exception):
    pass


class Unparseable(Exception):
    pass


def basename(word):
    return word.lstrip("\\").rsplit("/", 1)[-1]


def bash_text(text):
    """The text as bash reads it before splitting words: backslash-newline joins lines (outside single quotes),
    and # starts a comment only at the start of a word. shlex would cut at any #, so `-m fix#1 --no-verify`
    hid the flag, and it keeps the newline of a continuation, so `\\<newline>-n` was not an option."""
    out, i, quote = [], 0, None
    while i < len(text):
        char = text[i]
        if quote == "'":
            quote = None if char == "'" else quote
            out.append(char)
            i += 1
        elif char == "\\" and i + 1 < len(text):
            if text[i + 1] != "\n":
                out.append(text[i:i + 2])
            i += 2
        elif quote == '"':
            quote = None if char == '"' else quote
            out.append(char)
            i += 1
        elif char in "'\"":
            quote = char
            out.append(char)
            i += 1
        elif char == "#" and (not out or out[-1] in " \t\r\n;&|()<>`"):
            end = text.find("\n", i)
            i = len(text) if end == -1 else end
        else:
            out.append(char)
            i += 1
    return "".join(out)


def tokenize(text):
    """Split shell text into words and separator tokens (quotes removed, newlines kept)."""
    lexer = shlex.shlex(bash_text(text), posix=True, punctuation_chars="();<>|&`\n")
    lexer.whitespace = " \t\r"
    lexer.whitespace_split = True
    lexer.commenters = ""
    try:
        tokens = list(lexer)
    except ValueError as error:
        raise Unparseable(str(error)) from error
    out = []
    for token in tokens:
        # punctuation_chars glues runs like ";\n" or "&&(" together; split them back into known separators.
        if token and all(c in "();<>|&`\n" for c in token) and token not in {"<", ">", "<<", ">>", "<<<", "<<-", ">&", "<&", "&>", ">|", "&>>"}:
            out.extend(split_punct(token))
        else:
            out.append(token)
    return out


def split_punct(run):
    pieces = []
    i = 0
    while i < len(run):
        two = run[i:i + 2]
        if two in {"&&", "||", ";;", "|&"}:
            pieces.append(two)
            i += 2
        else:
            pieces.append(run[i])
            i += 1
    return pieces


def heredocs(text):
    """Yield (command_line_before_marker, body) for each heredoc, and return text without the bodies."""
    found = []
    lines = text.split("\n")
    kept = []
    i = 0
    while i < len(lines):
        line = lines[i]
        kept.append(line)
        markers = [(m.group(1) == "-", m.group(3), m.start()) for m in HEREDOC.finditer(line)
                   if not line[max(0, m.start() - 1):m.start()] == "<"]
        i += 1
        for strip_tabs, marker, start in markers:
            body = []
            while i < len(lines):
                candidate = lines[i].lstrip("\t") if strip_tabs else lines[i]
                if candidate == marker:
                    i += 1
                    break
                body.append(lines[i])
                i += 1
            found.append((line[:start], "\n".join(body)))
    return "\n".join(kept), found


class Guard:
    def __init__(self):
        self.git_seen = False
        self.config_hidden = None  # reason string once HOME/XDG_CONFIG_HOME changes or env is cleared

    # ---- entry points -------------------------------------------------------------------------

    def check(self, text, depth=0):
        if depth > 8:
            raise Unparseable("nesting too deep")
        text, docs = heredocs(text)
        for before, body in docs:
            try:
                # The text before `<<` can end inside an open quote, as in the usual
                # git commit -m "$(cat <<'EOF' ... EOF\n)" form; that is a data heredoc.
                words = []
                for token in tokenize(before):
                    words = [] if token in SEPARATORS else words + [token]
            except Unparseable:
                words = []
            if words and self.runs_code(words):
                self.check(body, depth + 1)
        tokens = tokenize(text)
        segment, previous, piped = [], None, False
        for token in tokens + [";"]:
            if token in SEPARATORS:
                if segment:
                    self.segment(segment, depth, previous if piped else None)
                    previous = segment
                piped = token in {"|", "|&"}
                segment = []
                continue
            segment.append(token)
        if self.git_seen and self.config_hidden:
            raise Denied(f"git runs with {self.config_hidden}, which hides ~/.gitconfig and its core.hooksPath "
                         "(the identity check). Run git with the normal environment.")

    def runs_code(self, words):
        name, args = self.unwrap(words)
        return name in SHELLS or name == "eval"

    # ---- one simple command -------------------------------------------------------------------

    def segment(self, words, depth, piped=None):
        words = [w for w in words if w not in {"<", ">", ">>", "<<", "<<-", ">&", "<&", "&>", ">|", "&>>"}]
        # Words that carry command substitution: inspect their contents too.
        for word in words:
            for inner in substitutions(word):
                self.check(inner, depth + 1)
        name, args = self.unwrap(words)
        if name is None:
            return
        if name in {"export", "declare", "typeset", "local", "readonly", "set", "unset"} and args is not None:
            # unset, export -n and declare/typeset +x take HOME out of git's environment, like env -u.
            unexports = name == "unset" or any(
                a.startswith("-" if name == "export" else "+") and ("n" if name == "export" else "x") in a
                for a in args if name in {"export", "declare", "typeset"})
            for arg in args:
                if unexports and arg in CONFIG_HIDING_ENV:
                    self.config_hidden = f"{arg} unset"
                self.assignment(arg)
            return
        if name == "env":
            # unwrap() already handled env; reaching here means `env` alone (prints the env).
            return
        if name in SHELLS:
            script = shell_script(args)
            if script is not None:
                self.check(script, depth + 1)
            elif piped and not any(not a.startswith("-") for a in args):
                # `printf 'git commit -n' | sh`: the upstream command's arguments are the script.
                self.check(" ".join(piped[1:]), depth + 1)
            if "<<<" in words:
                at = words.index("<<<")
                if at + 1 < len(words):
                    self.check(words[at + 1], depth + 1)
            return
        if name == "eval":
            self.check(" ".join(args), depth + 1)
            return
        if name == "git":
            self.git_seen = True
            self.git(args)

    def assignment(self, word):
        match = ASSIGNMENT.match(word)
        if not match:
            return False
        var = match.group(1)
        if PROTECTED_ENV.match(var):
            raise Denied(f"sets {var}. Commit identity and git config come from this machine's chezmoi data; "
                         "do not override them through the environment.")
        if var in CONFIG_HIDING_ENV:
            self.config_hidden = f"{var} changed"
        return True

    def unwrap(self, words):
        """Strip assignments, env and wrapper commands. Return (command basename, its args)."""
        i = 0
        while i < len(words):
            word = words[i]
            if self.assignment(word) or word in KEYWORDS:
                i += 1
                continue
            name = basename(word)
            if name == "env":
                i += 1
                while i < len(words):
                    arg = words[i]
                    flags = re.match(r"-([0iv]+)(.*)", arg, re.S)
                    if arg in {"-i", "--ignore-environment", "-"}:
                        self.config_hidden = "the environment cleared (env -i)"
                        i += 1
                    elif flags:
                        # A bundle: the flags -0, -i and -v first, then maybe one option that takes a value
                        # (-iS'git …', -iC dir), which is parsed as its own word. `-S'git …'` holds an i too,
                        # but after the S, so it is a script, handled below.
                        if "i" in flags[1]:
                            self.config_hidden = "the environment cleared (env -i)"
                        if flags[2]:
                            words = words[:i] + ["-" + flags[2]] + words[i + 1:]
                        else:
                            i += 1
                    elif arg in {"-u", "--unset"} and i + 1 < len(words):
                        if words[i + 1] in CONFIG_HIDING_ENV:
                            self.config_hidden = f"{words[i + 1]} unset"
                        i += 2
                    elif arg.startswith("--unset=") or (arg.startswith("-u") and len(arg) > 2):
                        var = arg.split("=", 1)[1] if arg.startswith("--unset=") else arg[2:]
                        if var in CONFIG_HIDING_ENV:
                            self.config_hidden = f"{var} unset"
                        i += 1
                    elif arg in {"-S", "--split-string"} and i + 1 < len(words):
                        return self.unwrap(tokenize(words[i + 1]) + words[i + 2:])
                    elif arg.startswith("--split-string=") or (arg.startswith("-S") and len(arg) > 2):
                        rest = arg.split("=", 1)[1] if arg.startswith("--") else arg[2:]
                        return self.unwrap(tokenize(rest) + words[i + 1:])
                    elif arg in {"-C", "--chdir", "-P"}:  # -P altpath (BSD env) takes a value too
                        i += 2
                    elif arg == "--":
                        i += 1
                        break
                    elif arg.startswith("-"):
                        i += 1
                    elif self.assignment(arg):
                        i += 1
                    else:
                        break
                if i >= len(words):
                    return "env", []
                continue
            if name in WRAPPERS:
                takes_value = WRAPPERS[name]
                positionals = WRAPPER_POSITIONALS.get(name, 0)
                i += 1
                while i < len(words):
                    arg = words[i]
                    if arg == "--":
                        i += 1
                        break
                    if arg.startswith("-") and len(arg) > 1:
                        i += 2 if arg in takes_value else 1
                        continue
                    if positionals:
                        positionals -= 1
                        i += 1
                        continue
                    break
                if name == "flock" and "-c" in words:
                    at = words.index("-c")
                    if at + 1 < len(words):
                        return "sh", ["-c", words[at + 1]]
                if i >= len(words):
                    return name, []
                continue
            return name, words[i + 1:]
        return None, None

    # ---- git ----------------------------------------------------------------------------------

    def git(self, args):
        i = 0
        while i < len(args):
            arg = args[i]
            if not arg.startswith("-") or arg == "-":
                break
            opt, eq, value = arg.partition("=")
            if opt == "-c" or (opt.startswith("-c") and len(opt) > 2 and not opt.startswith("--")):
                if opt == "-c" and not eq:
                    value = args[i + 1] if i + 1 < len(args) else ""
                    i += 1
                elif opt != "-c":
                    value = arg[2:]
                self.config_key(value.partition("=")[0], f"git -c {value}")
            elif opt == "--config-env":
                if not eq:
                    value = args[i + 1] if i + 1 < len(args) else ""
                    i += 1
                self.config_key(value.partition("=")[0], f"git --config-env {value}")
            elif opt in GIT_GLOBAL_WITH_VALUE and not eq:
                i += 1
            i += 1
        if i >= len(args):
            return
        sub, rest = args[i], args[i + 1:]
        if sub == "commit":
            self.commit(rest)
        elif sub == "config":
            self.config(rest)
        elif sub in {"merge", "pull"} and "--no-verify" in rest:
            # Exact word only: every shorter prefix is ambiguous with --no-verify-signatures, and -n means --no-stat.
            raise Denied(f"`git {sub} --no-verify` skips the pre-merge-commit identity check.")

    def config_key(self, key, shown):
        if PROTECTED_KEY.match(key.strip().lower()):
            raise Denied(f"`{shown}` overrides identity, hook, alias or include config for one command, "
                         "which can skip or fool the identity check.")

    def commit(self, args):
        i = 0
        while i < len(args):
            arg = args[i]
            if arg.startswith("--"):
                opt, eq, _ = arg.partition("=")
                if len(opt) >= len("--no-v") and "--no-verify".startswith(opt):
                    raise Denied(f"`git commit {arg}` skips the pre-commit identity check.")
                if opt in COMMIT_WITH_VALUE and not eq:
                    i += 1
            elif arg.startswith("-") and len(arg) > 1:
                for position, flag in enumerate(arg[1:]):
                    if flag == "n":
                        raise Denied(f"`git commit {arg}` includes -n (--no-verify), which skips the pre-commit "
                                     "identity check.")
                    if flag in COMMIT_SHORT_VALUE:
                        if position == len(arg) - 2 and flag in "mFcCt":
                            i += 1  # value is the next word
                        break
            i += 1
        # Anything after `--` is a pathspec to git, but a literal --no-verify / -n there is denied too
        # (handled above, since the loop does not stop at `--`): fail closed on the odd case.

    def config(self, args):
        positionals, write_flag, read_flag = [], None, False
        i = 0
        while i < len(args):
            arg = args[i]
            opt = arg.partition("=")[0]
            if arg in CONFIG_WRITE_FLAGS:
                write_flag = arg
            elif arg in CONFIG_READ_FLAGS:
                read_flag = True
            elif opt in CONFIG_WITH_VALUE:
                if "=" not in arg:
                    i += 1
            elif arg == "--":
                positionals.extend(args[i + 1:])  # git config reads every word after -- as a name or value
                break
            elif arg.startswith("-") and len(arg) > 1:
                pass
            else:
                positionals.append(arg)
            i += 1
        sub = positionals[0] if positionals and positionals[0] in CONFIG_SUBCOMMANDS_WRITE | CONFIG_SUBCOMMANDS_READ else None
        if sub:
            positionals = positionals[1:]
            if sub in CONFIG_SUBCOMMANDS_READ:
                return
            write_flag = sub
        elif read_flag and not write_flag:
            return
        elif not write_flag and len(positionals) < 2:
            return  # `git config user.email` reads
        if write_flag in {"--edit", "-e", "edit"}:
            raise Denied("`git config --edit` can rewrite identity or core.hooksPath; it is not allowed from an agent.")
        if not positionals:
            return
        key = positionals[0].lower()
        if write_flag in {"--rename-section", "--remove-section", "rename-section", "remove-section"}:
            # Both names count: renaming `evil` (holding hookspath) to `core` writes core.hookspath.
            for section in positionals[:2]:
                if PROTECTED_SECTION.match(section.lower()):
                    raise Denied(f"`git config {write_flag} {' '.join(positionals[:2])}` touches identity, hook, "
                                 "alias, include or credential config.")
            return
        if PROTECTED_KEY.match(key) or key == "core.hookspath":
            raise Denied(f"`git config` would write {positionals[0]}. Identity and core.hooksPath come from this "
                         "machine's chezmoi data; aliases, includes and credential helpers are not set by agents.")


def substitutions(word):
    """Contents of $(...) and `...` inside a word (shlex keeps them inside double-quoted words)."""
    found = []
    i = 0
    while i < len(word):
        if word.startswith("$(", i) and not word.startswith("$((", i):
            depth, j = 1, i + 2
            while j < len(word) and depth:
                depth += {"(": 1, ")": -1}.get(word[j], 0)
                j += 1
            found.append(word[i + 2:j - 1] if depth == 0 else word[i + 2:])
            i = j
        elif word[i] == "`":
            j = word.find("`", i + 1)
            found.append(word[i + 1:] if j < 0 else word[i + 1:j])
            i = len(word) if j < 0 else j + 1
        else:
            i += 1
    return found


def shell_script(args):
    """The -c script of `sh -c SCRIPT` (also -lc, -ec, -c after other options), or None."""
    skip = False
    for i, arg in enumerate(args):
        if skip:
            skip = False
            continue
        if arg == "--":
            return None
        if arg in {"-o", "-O", "+o", "+O"}:  # set -o / shopt -O take an option name: `bash -o pipefail -c …`
            skip = True
            continue
        if arg.startswith("-") and not arg.startswith("--") and "c" in arg[1:]:
            rest = [a for a in args[i + 1:] if not (a.startswith("-") and len(a) > 1)]
            return rest[0] if rest else ""
        if not arg.startswith("-") and not arg.startswith("+"):
            return None
    return None


def decide(command):
    """Return None to allow, or a deny reason."""
    if isinstance(command, list):
        command = shlex.join(str(part) for part in command)
    if not isinstance(command, str) or not command.strip():
        return None
    guard = Guard()
    try:
        guard.check(command)
    except Denied as denial:
        return f"git-guard: {denial}"
    except (Unparseable, RecursionError) as error:
        if re.search(r"\bgit\b|GIT_", command):
            return f"git-guard: could not parse a command that mentions git ({error}); rewrite it more plainly."
    return None


def deny_output(reason, harness):
    # Claude Code and Codex document the same PreToolUse deny shape (hookSpecificOutput,
    # exit 0). Kept per harness so one can change without touching the other.
    if harness in {"claude", "codex"}:
        return {"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny",
                                       "permissionDecisionReason": reason}}
    raise ValueError(harness)


def main(argv=None):
    argv = sys.argv[1:] if argv is None else argv
    harness = "claude"
    if argv[:1] == ["--harness"] and len(argv) > 1:
        harness = argv[1]
    elif argv and argv[0].startswith("--harness="):
        harness = argv[0].split("=", 1)[1]
    if harness not in {"claude", "codex"}:
        print(f"git-guard: unknown harness {harness!r}", file=sys.stderr)
        return 2
    try:
        payload = json.load(sys.stdin)
        tool_input = payload.get("tool_input") or {}
        command = tool_input.get("command", "") if isinstance(tool_input, dict) else ""
    except (ValueError, AttributeError) as error:
        # Exit 2 blocks in both harnesses; stderr is the reason.
        print(f"git-guard: unreadable hook payload ({error}); refusing.", file=sys.stderr)
        return 2
    reason = decide(command)
    if reason:
        json.dump(deny_output(reason, harness), sys.stdout)
        sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
