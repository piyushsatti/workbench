package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const healthyAliases = `# aliases
claude()          { workspaces run . claude "$@"; }
claude-alt()       { workspaces run . claude-alt "$@"; }
`

var allProtection = []string{
	"Edit(~/.config/workspaces/**)", "Edit(~/.local/bin/workspaces)", "Edit(~/.local/bin/awm)", "Edit(~/.local/state/workspaces/**)",
	"Edit(~/.bash_aliases)", "Edit(~/.bashrc)", "Edit(~/base.bashrc)", "Edit(~/.tmux.conf)", "Edit(~/.tmux.base.conf)", "Edit(~/.tmux/**)",
}

// settingsJSON builds a settings.json with the given deny entries, sandbox
// denyWrite paths and SessionStart hook commands.
func settingsJSON(deny, denyWrite, hooks []string) string {
	var groups []map[string]any
	for _, c := range hooks {
		groups = append(groups, map[string]any{"hooks": []map[string]any{{"type": "command", "command": c}}})
	}
	b, _ := json.Marshal(map[string]any{
		"permissions": map[string]any{"deny": deny, "defaultMode": "auto"},
		"sandbox":     map[string]any{"filesystem": map[string]any{"denyWrite": denyWrite}},
		"hooks":       map[string]any{"SessionStart": groups, "UserPromptSubmit": []any{}},
	})
	return string(b)
}

var healthySettings = settingsJSON(allProtection,
	[]string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}, []string{"workspaces hook session-start"})

// doctorFixture is a setup in which every check passes.
func doctorFixture(t *testing.T) *fixture {
	f := newFixture(t)
	for _, d := range []string{".ssh", ".claude-alt", ".claude"} {
		if err := os.MkdirAll(filepath.Join(f.home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.write(".bash_aliases", healthyAliases)
	f.write(".claude/settings.json", healthySettings)
	f.lookPath["workspaces"] = filepath.Join(f.home, ".local/bin/workspaces")
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "list-sessions" {
			return "main\na\n", nil
		}
		return "", errors.New("unexpected tmux call")
	}
	return f
}

func (f *fixture) write(rel, body string) {
	f.t.Helper()
	p := filepath.Join(f.home, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// doctorRowOf returns the level and detail of the row for check.
func doctorRowOf(t *testing.T, out, check string) (level, detail string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
		if strings.HasPrefix(rest, check+"  ") || rest == check {
			return fields[0], strings.TrimSpace(strings.TrimPrefix(rest, check))
		}
	}
	t.Fatalf("no row for %q in:\n%s", check, out)
	return "", ""
}

var allChecks = []string{
	"config", "paths", "space roots", "attach paths", "harness binaries", "config dirs", "wrappers",
	"settings", "session hook", "launch log", "workspaces binary", "tmux", "todo",
}

func TestDoctorHealthy(t *testing.T) {
	f := doctorFixture(t)
	if code := f.run("doctor"); code != exitOK {
		t.Fatalf("exit %d\n%s%s", code, f.stdout.String(), f.stderr.String())
	}
	rows := strings.Split(strings.TrimRight(f.stdout.String(), "\n"), "\n")
	if len(rows) != len(allChecks) {
		t.Fatalf("%d rows, want %d:\n%s", len(rows), len(allChecks), f.stdout.String())
	}
	for i, check := range allChecks {
		level, detail := doctorRowOf(t, rows[i], check)
		if level != "OK" {
			t.Errorf("%s: %s %s, want OK", check, level, detail)
		}
	}
	if _, d := doctorRowOf(t, f.stdout.String(), "config"); d != "1 spaces, 2 harnesses, default claude" {
		t.Errorf("config detail = %q", d)
	}
	if _, d := doctorRowOf(t, f.stdout.String(), "tmux"); !strings.Contains(d, "server answers, 2 sessions") {
		t.Errorf("tmux detail = %q", d)
	}
	if f.logExists() {
		t.Error("doctor wrote the launch log")
	}
	if exists(filepath.Join(f.home, ".local/state")) {
		t.Error("doctor created the state dir")
	}
	if len(f.execs) != 0 {
		t.Error("doctor launched something")
	}
}

func TestDoctorRowFormat(t *testing.T) {
	f := doctorFixture(t)
	f.run("doctor")
	for _, line := range strings.Split(strings.TrimRight(f.stdout.String(), "\n"), "\n") {
		if !strings.HasPrefix(line, "OK  ") && !strings.HasPrefix(line, "WARN  ") && !strings.HasPrefix(line, "FAIL  ") {
			t.Errorf("row %q does not start with a level and two spaces", line)
		}
	}
}

// verdict is what one row of the doctor output must say after a change.
type verdict struct {
	check, level, detail string
}

func TestDoctorChecks(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *fixture)
		want  []verdict
		code  int
	}{
		{"spaces.json invalid", func(f *fixture) { f.writeConfig("spaces.json", `{"deny_acess":[]}`) },
			[]verdict{{"config", "FAIL", "unknown field"}}, exitConfig},
		{"spaces.json missing", func(f *fixture) { os.Remove(filepath.Join(f.home, ".config/workspaces/spaces.json")) },
			[]verdict{{"config", "FAIL", "config file missing"}}, exitConfig},
		{"harness.json invalid", func(f *fixture) {
			f.writeConfig("harness.json", `{"default":"nope","harnesses":{"claude":{"adapter":"claude"}}}`)
		},
			[]verdict{{"config", "FAIL", `default "nope" names no harness`}}, exitConfig},
		{"path missing", func(f *fixture) { os.Remove(filepath.Join(f.home, ".ssh")) },
			[]verdict{{"paths", "WARN", "deny_access ~/.ssh does not exist"}, {"config", "OK", ""}}, exitOK},
		{"space root fails the launch check", func(f *fixture) {
			f.writeConfig("spaces.json", `{"allow_launch":["~/Source/Projects/b"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`)
		}, []verdict{{"space roots", "FAIL", "a: not_allowed"}}, exitConfig},
		{"space root under deny_launch", func(f *fixture) {
			f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_launch":["~/Source/Projects/a"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`)
		}, []verdict{{"space roots", "FAIL", "a: deny_launch ~/Source/Projects/a"}}, exitConfig},
		{"space root under deny_access", func(f *fixture) {
			f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/a"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`)
		}, []verdict{{"space roots", "FAIL", "a: deny_access ~/Source/Projects/a"}}, exitConfig},
		{"attach under deny_access", func(f *fixture) {
			f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/b"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b"]}}}`)
		}, []verdict{
			{"attach paths", "FAIL", "a attach ~/Source/Projects/b is under deny_access ~/Source/Projects/b"},
			// attach_denied belongs to the attach row, not the root row.
			{"space roots", "OK", ""},
		}, exitConfig},
		{"harness binary missing", func(f *fixture) { delete(f.lookPath, "claude") },
			[]verdict{{"harness binaries", "FAIL", "not found: claude (claude), claude-alt (claude)"}}, exitConfig},
		{"harness bin override looked up", func(f *fixture) {
			f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude","bin":"my-claude"}}}`)
		}, []verdict{{"harness binaries", "FAIL", "claude (my-claude)"}}, exitConfig},
		{"config dir missing", func(f *fixture) { os.Remove(filepath.Join(f.home, ".claude-alt")) },
			[]verdict{{"config dirs", "WARN", "claude-alt: CLAUDE_CONFIG_DIR=~/.claude-alt"}}, exitOK},
		{"config dir is a file", func(f *fixture) {
			os.Remove(filepath.Join(f.home, ".claude-alt"))
			f.write(".claude-alt", "")
		}, []verdict{{"config dirs", "WARN", "claude-alt"}}, exitOK},
		{"codex home missing", func(f *fixture) {
			f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude"},"codex-alt":{"adapter":"codex","env":{"CODEX_HOME":"~/.codex-alt"}}}}`)
			f.lookPath["codex"] = "/usr/bin/codex"
		}, []verdict{{"config dirs", "WARN", "codex-alt: CODEX_HOME=~/.codex-alt"}}, exitOK},
		{"wrappers file missing", func(f *fixture) { os.Remove(filepath.Join(f.home, ".bash_aliases")) },
			[]verdict{{"wrappers", "WARN", "cannot read ~/.bash_aliases"}}, exitOK},
		{"one wrapper missing", func(f *fixture) { f.write(".bash_aliases", `claude() { workspaces run . claude "$@"; }`+"\n") },
			[]verdict{{"wrappers", "WARN", "for: claude-alt"}}, exitOK},
		{"wrapper for the wrong harness", func(f *fixture) {
			f.write(".bash_aliases", `claude() { workspaces run . claude-alt "$@"; }`+"\n"+`claude-alt() { workspaces run . claude-alt "$@"; }`+"\n")
		}, []verdict{{"wrappers", "WARN", "for: claude$"}}, exitOK},
		{"wrapper commented out", func(f *fixture) {
			f.write(".bash_aliases", "# claude() { workspaces run . claude \"$@\"; }\n"+`claude-alt() { workspaces run . claude-alt "$@"; }`+"\n")
		}, []verdict{{"wrappers", "WARN", "for: claude$"}}, exitOK},
		{"wrapper without agent", func(f *fixture) {
			f.write(".bash_aliases", `claude() { command claude "$@"; }`+"\n"+`claude-alt() { workspaces run . claude-alt "$@"; }`+"\n")
		}, []verdict{{"wrappers", "WARN", "for: claude$"}}, exitOK},
		{"multi-line wrapper form", func(f *fixture) {
			f.write(".bash_aliases", "claude() { workspaces run . claude \"$@\"\n}\nclaude-alt () { workspaces run . claude-alt; }\n")
		}, []verdict{{"wrappers", "OK", ""}}, exitOK},
		{"codex harness needs its wrapper", func(f *fixture) {
			f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude"},"codex":{"adapter":"codex"}}}`)
			f.lookPath["codex"] = "/usr/bin/codex"
		}, []verdict{{"wrappers", "WARN", "for: codex"}, {"harness binaries", "OK", "codex=/usr/bin/codex"}}, exitOK},
		{"settings file missing", func(f *fixture) { os.Remove(filepath.Join(f.home, ".claude/settings.json")) },
			[]verdict{
				{"settings", "WARN", "missing required: Edit(~/.config/workspaces/**), Edit(~/.local/bin/workspaces), Edit(~/.local/bin/awm)"},
				{"session hook", "WARN", "no SessionStart hook"},
			}, exitOK},
		{"settings not json", func(f *fixture) { f.write(".claude/settings.json", "{oops") },
			[]verdict{{"settings", "WARN", "cannot read ~/.claude/settings.json"}, {"session hook", "WARN", "cannot read"}}, exitOK},
		{"required entry missing", func(f *fixture) {
			f.write(".claude/settings.json", settingsJSON(allProtection[1:], []string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}, []string{"workspaces hook session-start"}))
		}, []verdict{{"settings", "WARN", "missing required: Edit(~/.config/workspaces/**)$"}}, exitOK},
		{"only recommended entries missing", func(f *fixture) {
			f.write(".claude/settings.json", settingsJSON(allProtection[:3], []string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}, []string{"workspaces hook session-start"}))
		}, []verdict{{"settings", "WARN", "missing recommended: Edit(~/.local/state/workspaces/**), Edit(~/.bash_aliases), Edit(~/.bashrc), Edit(~/base.bashrc), Edit(~/.tmux.conf), Edit(~/.tmux.base.conf), Edit(~/.tmux/**)$"}}, exitOK},
		{"sandbox denyWrite missing", func(f *fixture) {
			f.write(".claude/settings.json", settingsJSON(allProtection, []string{"~/.config/workspaces"}, []string{"workspaces hook session-start"}))
		}, []verdict{{"settings", "WARN", "missing recommended: sandbox denyWrite ~/.local/bin/workspaces, sandbox denyWrite ~/.local/bin/awm, sandbox denyWrite ~/.local/state/workspaces$"}}, exitOK},
		{"optional entry present", func(f *fixture) {
			f.write(".claude/settings.json", settingsJSON(append(allProtection, "Edit(~/.claude/settings.json)"),
				[]string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}, []string{"workspaces hook session-start"}))
		}, []verdict{{"settings", "OK", "all required and recommended entries present$"}}, exitOK},
		{"optional entry absent is still OK", func(f *fixture) {}, []verdict{{"settings", "OK", "optional, not set: Edit(~/.claude/settings.json)"}}, exitOK},
		{"hook command with a path prefix", func(f *fixture) {
			f.write(".claude/settings.json", settingsJSON(allProtection,
				[]string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}, []string{"other", "/home/u/.local/bin/workspaces hook session-start"}))
		}, []verdict{{"session hook", "OK", ""}}, exitOK},
		{"hook absent", func(f *fixture) {
			f.write(".claude/settings.json", settingsJSON(allProtection, []string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}, []string{"bash other.sh"}))
		}, []verdict{{"session hook", "WARN", "no SessionStart hook runs `workspaces hook session-start`"}}, exitOK},
		{"state dir blocked", func(f *fixture) {
			f.write("blocker", "")
			f.env["XDG_STATE_HOME"] = filepath.Join(f.home, "blocker")
		}, []verdict{{"launch log", "FAIL", "is not writable"}}, exitConfig},
		{"launch log is a directory", func(f *fixture) {
			os.MkdirAll(filepath.Join(f.home, ".local/state/workspaces/launches.jsonl"), 0o700)
		}, []verdict{{"launch log", "FAIL", "is not writable"}}, exitConfig},
		{"launch log exists and is writable", func(f *fixture) {
			f.write(".local/state/workspaces/launches.jsonl", "")
		}, []verdict{{"launch log", "OK", "~/.local/state/workspaces/launches.jsonl is writable"}}, exitOK},
		{"HOME not absolute", func(f *fixture) { delete(f.env, "XDG_STATE_HOME"); f.env["HOME"] = "relative" },
			[]verdict{{"launch log", "FAIL", "HOME is not set"}}, exitConfig},
		{"agent not on PATH", func(f *fixture) { delete(f.lookPath, "workspaces") },
			[]verdict{{"workspaces binary", "WARN", "workspaces is not on PATH; expected ~/.local/bin/workspaces"}}, exitOK},
		{"agent elsewhere", func(f *fixture) { f.lookPath["workspaces"] = "/usr/local/bin/agent" },
			[]verdict{{"workspaces binary", "WARN", "command -v workspaces is /usr/local/bin/agent, expected ~/.local/bin/workspaces"}}, exitOK},
		{"tmux not installed", func(f *fixture) { delete(f.lookPath, "tmux") },
			[]verdict{{"tmux", "FAIL", "tmux is not on PATH"}}, exitConfig},
		{"no tmux server running", func(f *fixture) {
			f.tmuxReply = func([]string) (string, error) {
				return "", errors.New("tmux list-sessions: no server running on /tmp/tmux-1000/default")
			}
		}, []verdict{{"tmux", "OK", "/usr/bin/tmux; no server running yet"}}, exitOK},
		{"tmux socket gone", func(f *fixture) {
			f.tmuxReply = func([]string) (string, error) {
				return "", errors.New("tmux list-sessions: error connecting to /tmp/tmux-1000/default (No such file or directory)")
			}
		}, []verdict{{"tmux", "OK", "no server running yet"}}, exitOK},
		{"tmux answers oddly", func(f *fixture) {
			f.tmuxReply = func([]string) (string, error) { return "", errors.New("tmux list-sessions: protocol version mismatch") }
		}, []verdict{{"tmux", "WARN", "the server did not answer: tmux list-sessions: protocol version mismatch"}}, exitOK},
		{"TMUX set and the server answers", func(f *fixture) { f.env["TMUX"] = "/tmp/tmux-1000/default,1,0" },
			[]verdict{{"tmux", "OK", "server answers"}}, exitOK},
		{"TMUX set and the server is dead", func(f *fixture) {
			f.env["TMUX"] = "/tmp/tmux-1000/default,1,0"
			f.tmuxReply = func([]string) (string, error) {
				return "", errors.New("tmux list-sessions: no server running on /tmp/tmux-1000/default")
			}
		}, []verdict{{"tmux", "FAIL", "$TMUX is set but the server does not answer"}}, exitConfig},
		{"todo still on TODO.md", func(f *fixture) { f.write("Source/Projects/a/TODO.md", "- [ ] x\n") },
			[]verdict{{"todo", "WARN", "still on TODO.md: a$"}}, exitOK},
		{"todo migrated", func(f *fixture) {
			f.write("Source/Projects/a/TODO.md", "- [ ] x\n")
			f.write("Source/Projects/a/TODO.jsonl", "{}\n")
		}, []verdict{{"todo", "OK", ""}}, exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := doctorFixture(t)
			tt.setup(f)
			code := f.run("doctor")
			if code != tt.code {
				t.Errorf("exit %d, want %d\n%s", code, tt.code, f.stdout.String())
			}
			for _, v := range tt.want {
				level, detail := doctorRowOf(t, f.stdout.String(), v.check)
				if level != v.level {
					t.Errorf("%s: level %s, want %s (%s)", v.check, level, v.level, detail)
				}
				if v.detail != "" && !matchDetail(detail, v.detail) {
					t.Errorf("%s: detail %q, want it to contain %q", v.check, detail, v.detail)
				}
			}
		})
	}
}

// matchDetail reports whether detail contains want, where a trailing $ in
// want anchors it to the end of the detail.
func matchDetail(detail, want string) bool {
	if rest, ok := strings.CutSuffix(want, "$"); ok {
		return strings.HasSuffix(detail, rest)
	}
	return strings.Contains(detail, want)
}

func TestDoctorSkipsWhatNeedsConfig(t *testing.T) {
	f := doctorFixture(t)
	f.writeConfig("spaces.json", "{")
	if code := f.run("doctor"); code != exitConfig {
		t.Fatalf("exit %d", code)
	}
	out := f.stdout.String()
	for _, check := range []string{"paths", "space roots", "attach paths", "todo"} {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "  "+check+"  ") {
				t.Errorf("%s ran without spaces.json: %q", check, line)
			}
		}
	}
	// The harness half and the rest still run.
	for _, check := range []string{"config", "harness binaries", "wrappers", "settings", "tmux", "launch log"} {
		doctorRowOf(t, out, check)
	}

	f = doctorFixture(t)
	f.writeConfig("harness.json", "{")
	if code := f.run("doctor"); code != exitConfig {
		t.Fatalf("exit %d", code)
	}
	out = f.stdout.String()
	if strings.Contains(out, "wrappers") || strings.Contains(out, "harness binaries") {
		t.Errorf("harness checks ran without harness.json:\n%s", out)
	}
	doctorRowOf(t, out, "space roots")
}

func TestDoctorHarnessDefaultsWithoutFile(t *testing.T) {
	f := doctorFixture(t)
	os.Remove(filepath.Join(f.home, ".config/workspaces/harness.json"))
	f.write(".bash_aliases", `claude() { workspaces run . claude "$@"; }`+"\n")
	if code := f.run("doctor"); code != exitOK {
		t.Fatalf("exit %d\n%s", code, f.stdout.String())
	}
	if level, _ := doctorRowOf(t, f.stdout.String(), "wrappers"); level != "OK" {
		t.Errorf("wrappers = %s", level)
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
	f := doctorFixture(t)
	f.write(".claude/settings.json", settingsJSON(nil, nil, nil))
	before := snapshot(t, f.home)
	f.run("doctor")
	f.env["XDG_STATE_HOME"] = filepath.Join(f.home, "fresh/state")
	f.run("doctor")
	if after := snapshot(t, f.home); after != before {
		t.Errorf("doctor changed files under HOME:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// snapshot lists every path under dir with its size and mtime.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			b.WriteString(p + " " + info.ModTime().String() + " " + info.Mode().String() + "\n")
		}
		return nil
	})
	return b.String()
}

func TestDoctorLogWritableByPermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	f := doctorFixture(t)
	dir := filepath.Join(f.home, ".local/state/workspaces")
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	if code := f.run("doctor"); code != exitConfig {
		t.Errorf("read-only state dir: exit %d, want %d", code, exitConfig)
	}
	os.Chmod(dir, 0o700)
	log := filepath.Join(dir, "launches.jsonl")
	os.WriteFile(log, nil, 0o400)
	f.stdout.Reset()
	if code := f.run("doctor"); code != exitConfig {
		t.Errorf("read-only launch log: exit %d, want %d", code, exitConfig)
	}
	// The nearest folder that exists decides when the state dir is not there yet.
	os.Remove(log)
	os.Remove(dir)
	os.Chmod(filepath.Join(f.home, ".local/state"), 0o500)
	defer os.Chmod(filepath.Join(f.home, ".local/state"), 0o700)
	f.stdout.Reset()
	if code := f.run("doctor"); code != exitConfig {
		t.Errorf("read-only parent of the state dir: exit %d, want %d", code, exitConfig)
	}
}

func TestDoctorArguments(t *testing.T) {
	f := doctorFixture(t)
	if code := f.run("doctor", "extra"); code != exitUsage {
		t.Errorf("extra argument: exit %d", code)
	}
	if code := f.run("doctor", "--bogus"); code != exitUsage {
		t.Errorf("bad flag: exit %d", code)
	}
	f.stderr.Reset()
	if code := f.run("doctor", "-h"); code != exitOK || !strings.Contains(f.stderr.String(), "Exit 0 when nothing FAILs") {
		t.Errorf("-h: exit %d, stderr %q", code, f.stderr.String())
	}
}

func TestDoctorCleansDetails(t *testing.T) {
	f := doctorFixture(t)
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b\u001b[2J"]}}}`)
	f.run("doctor")
	if strings.Contains(f.stdout.String(), "\x1b") {
		t.Errorf("escape sequence reached the output: %q", f.stdout.String())
	}
}
