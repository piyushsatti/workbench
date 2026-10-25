package harness

import (
	"slices"
)

type Codex struct{}

// Written against `codex --help` and `codex <command> --help`, checked
// 2026-10-05. Everything that starts an agent loop is left out: resume and
// fork reopen a session, and agents is a session browser, so all three fall
// through to session mode with any word this list does not know.
var codexUtilitySubcommands = []string{
	"login", "logout", "mcp", "plugin", "completion", "update", "doctor",
	"sandbox", "debug", "apply", "a", "features", "help", "archive", "delete",
	"unarchive", "queue", "migrate-rollouts", "app-server", "remote-control",
	"mcp-server", "exec-server",
}

func (Codex) Binary() string { return "codex" }

func (Codex) Mode(userArgs []string) Mode {
	args := beforeDoubleDash(userArgs)
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	has := func(flags ...string) bool {
		return slices.ContainsFunc(args, func(a string) bool { return slices.Contains(flags, a) })
	}
	switch {
	case slices.Contains(codexUtilitySubcommands, first), has("-V", "--version", "-h", "--help"):
		return ModeUtility
	case first == "exec", first == "e", first == "review":
		return ModePrint
	default:
		return ModeSession
	}
}

// Command passes each attach path as its own --add-dir. `codex --help` shows
// the option as `--add-dir <DIR>`, one value, where the multi-value option
// `-i, --image <FILE>...` has the ellipsis; so a prompt that follows cannot be
// swallowed, and no -c writable_roots list is needed. Utility commands get
// nothing: no agent loop starts, and `codex review` has no --add-dir.
//
// No flag is injected that a user argument could replace, so there is no
// collision rule. Profiles are not a flag either: CODEX_HOME comes in through
// the harness env.
func (c Codex) Command(l Launch) ([]string, []string, error) {
	argv := []string{c.Binary()}
	if c.Mode(l.UserArgs) != ModeUtility {
		for _, p := range l.AddDirs {
			argv = append(argv, "--add-dir", p)
		}
	}
	argv = append(argv, l.UserArgs...)
	return argv, launchEnv(l), nil
}

// UnenforcedDeny is every path: Codex has no read deny, and its docs name no
// key that -c can set for one. The sandbox limits writes, so a path under
// deny_access can still be read.
func (Codex) UnenforcedDeny(paths []string) int { return len(paths) }
