package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"workspaces/internal/launchlog"
	"workspaces/internal/picker"
)

const testPid = 4242

const testSpaces = `{
 "allow_launch": ["~/Source"],
 "deny_launch": ["~/Source/private/*"],
 "deny_access": ["~/Source/Projects/_archive", "~/.ssh"],
 "spaces": {"a": {"root": "~/Source/Projects/a", "attach": ["~/Source/Projects/b"], "deny_access": ["~/Source/Projects/a/secrets"]}}
}`

const testHarnesses = `{"default":"claude","harnesses":{
 "claude":{"adapter":"claude"},
 "claude-alt":{"adapter":"claude","env":{"CLAUDE_CONFIG_DIR":"~/.claude-alt"}}}}`

type execCall struct {
	path string
	argv []string
	env  []string
}

// fixture is an App wired to fakes over a temporary HOME. Nothing reaches the
// real ~/.config, ~/.local/state, tmux, a picker or exec.
type fixture struct {
	t      *testing.T
	home   string
	env    map[string]string
	cwd    string
	tty    bool
	stdout bytes.Buffer
	stderr bytes.Buffer

	lookPath  map[string]string
	execs     []execCall
	chdirs    []string
	picks     [][]string
	pickReply []string
	pickErr   error
	// pickManyReply is what the multi-select returns, lines as shown.
	pickManyReply []string
	confirms      []string
	// confirmReply is the answer every confirm gets; it defaults to a decline.
	confirmReply bool
	confirmErr   error
	tmuxCalls    [][]string
	tmuxReply    func(args []string) (string, error)
	waited       bool

	getwdErr, chdirErr, execErr, executableErr error
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		"Source/private/drafts", "Source/Projects/a/sub", "Source/Projects/a/secrets",
		"Source/Projects/b", "Source/Projects/_archive/old", "outside", "bin/versions",
	} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(home, "bin/versions/2.1.289")
	if err := os.WriteFile(target, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "bin/claude")); err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t:    t,
		home: home,
		cwd:  filepath.Join(home, "Source"),
		tty:  true,
		env: map[string]string{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
			"XDG_STATE_HOME":  filepath.Join(home, ".local/state"),
			"PATH":            "/usr/bin",
		},
		lookPath: map[string]string{
			"claude": filepath.Join(home, "bin/claude"),
			"tmux":   "/usr/bin/tmux",
		},
		tmuxReply: func([]string) (string, error) { return "", errors.New("unexpected tmux call") },
	}
	f.writeConfig("spaces.json", testSpaces)
	f.writeConfig("harness.json", testHarnesses)
	return f
}

func (f *fixture) writeConfig(name, body string) {
	f.t.Helper()
	dir := filepath.Join(f.home, ".config/workspaces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) app() *App {
	return &App{
		Stdin:  strings.NewReader(""),
		Stdout: &f.stdout,
		Stderr: &f.stderr,
		Getenv: func(k string) string { return f.env[k] },
		Environ: func() []string {
			out := make([]string, 0, len(f.env))
			for k, v := range f.env {
				out = append(out, k+"="+v)
			}
			sort.Strings(out)
			return out
		},
		Getwd: func() (string, error) { return f.cwd, f.getwdErr },
		Chdir: func(d string) error { f.chdirs = append(f.chdirs, d); return f.chdirErr },
		LookPath: func(name string) (string, error) {
			if p, ok := f.lookPath[name]; ok {
				return p, nil
			}
			return "", errors.New("executable file not found in $PATH")
		},
		Executable: func() (string, error) { return "/opt/bin/workspaces", f.executableErr },
		IsTTY:      func() bool { return f.tty },
		Pick: func(prompt string, items []string) (string, error) {
			f.picks = append(f.picks, append([]string{prompt}, items...))
			if f.pickErr != nil {
				return "", f.pickErr
			}
			if len(f.pickReply) == 0 {
				return "", picker.ErrCancelled
			}
			r := f.pickReply[0]
			f.pickReply = f.pickReply[1:]
			return r, nil
		},
		PickMulti: func(prompt string, items []string) ([]string, error) {
			f.picks = append(f.picks, append([]string{prompt}, items...))
			if f.pickErr != nil {
				return nil, f.pickErr
			}
			if len(f.pickManyReply) == 0 {
				return nil, picker.ErrCancelled
			}
			return f.pickManyReply, nil
		},
		Confirm: func(prompt string) (bool, error) {
			f.confirms = append(f.confirms, prompt)
			return f.confirmReply, f.confirmErr
		},
		Tmux: func(args []string) (string, error) {
			f.tmuxCalls = append(f.tmuxCalls, args)
			return f.tmuxReply(args)
		},
		Exec: func(path string, argv, env []string) error {
			f.execs = append(f.execs, execCall{path, argv, env})
			return f.execErr
		},
		WaitEnter: func() { f.waited = true },
		Now:       func() time.Time { return time.Date(2026, 10, 4, 22, 31, 5, 0, time.UTC) },
		Pid:       testPid,
		Ppid:      1,
		Version:   "2026.10.05",
	}
}

func (f *fixture) run(args ...string) int {
	f.t.Helper()
	return f.app().Run(args)
}

func (f *fixture) records() []launchlog.Record {
	f.t.Helper()
	lines, malformed, err := launchlog.Read(filepath.Join(f.home, ".local/state/workspaces/launches.jsonl"))
	if err != nil || malformed != 0 {
		f.t.Fatalf("reading log: %v, %d malformed", err, malformed)
	}
	var out []launchlog.Record
	for _, l := range lines {
		out = append(out, l.Record)
	}
	return out
}

func (f *fixture) onlyRecord() launchlog.Record {
	f.t.Helper()
	recs := f.records()
	if len(recs) != 1 {
		f.t.Fatalf("got %d records, want 1: %+v\nstderr: %s", len(recs), recs, f.stderr.String())
	}
	return recs[0]
}

func (f *fixture) logExists() bool {
	_, err := os.Stat(filepath.Join(f.home, ".local/state/workspaces/launches.jsonl"))
	return err == nil
}

// inTmux makes the fake tmux answer the pane query for pane %30, whose
// pane_pid is panePid.
func (f *fixture) inTmux(panePid string) {
	f.env["TMUX"] = "/tmp/tmux-1000/default,1,0"
	f.env["TMUX_PANE"] = "%30"
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "display-message" {
			return "/tmp/tmux-1000/default\n$5\nmain\n@12\n3\n%30\n" + panePid + "\nclaude\n", nil
		}
		return "", errors.New("unexpected tmux call")
	}
}

func TestVersion(t *testing.T) {
	f := newFixture(t)
	if code := f.run("--version"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if got := f.stdout.String(); got != "2026.10.05\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestCalver(t *testing.T) {
	set := func(kv ...string) []debug.BuildSetting {
		var out []debug.BuildSetting
		for i := 0; i < len(kv); i += 2 {
			out = append(out, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return out
	}
	const rev = "abc1234def5678"
	cases := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"clean", set("vcs.revision", rev, "vcs.time", "2026-10-05T04:58:00Z", "vcs.modified", "false"), "2026.10.05"},
		{"dirty ignored", set("vcs.revision", rev, "vcs.time", "2026-10-05T04:58:00Z", "vcs.modified", "true"), "2026.10.05"},
		{"date is UTC", set("vcs.revision", rev, "vcs.time", "2026-10-06T08:00:00+09:30"), "2026.10.05"},
		{"zero-padded month and day", set("vcs.revision", rev, "vcs.time", "2027-01-02T00:00:00Z"), "2027.01.02"},
		{"same day, other commit", set("vcs.revision", "fff0000", "vcs.time", "2026-10-05T23:59:59Z"), "2026.10.05"},
		{"no vcs settings", nil, "devel"},
		{"revision without time", set("vcs.revision", rev), "devel"},
		{"time without revision", set("vcs.time", "2026-10-05T00:00:00Z"), "devel"},
		{"unparsable time", set("vcs.revision", rev, "vcs.time", "yesterday"), "devel"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := calver(c.settings); got != c.want {
				t.Errorf("calver = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNoNounWithoutTTY(t *testing.T) {
	f := newFixture(t)
	f.tty = false
	if code := f.run(); code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if len(f.picks) != 0 {
		t.Error("picker called without a TTY")
	}
	if !strings.Contains(f.stderr.String(), "Usage: workspaces") {
		t.Errorf("stderr = %q, want usage", f.stderr.String())
	}
}

func TestNounPickerLeadsToCommandPickers(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.pickReply = []string{"run", ".  (here)", "claude-alt"}
	if code := f.run(); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.picks) != 3 {
		t.Fatalf("picks = %q, want noun, space and harness pickers", f.picks)
	}
	if got := strings.Join(f.picks[0], "|"); got != "workspaces|run|check|launch|space|harness|policy|todo|log|session|doctor" {
		t.Errorf("noun picker = %s", got)
	}
	wantSpace := "space|. (here: " + f.cwd + ")|a  ~/Source/Projects/a"
	if got := strings.Join(f.picks[1], "|"); got != wantSpace {
		t.Errorf("space picker = %s, want %s", got, wantSpace)
	}
	if got := strings.Join(f.picks[2], "|"); got != "harness|claude|claude-alt" {
		t.Errorf("harness picker = %s, want the default first", got)
	}
	if rec := f.onlyRecord(); rec.Harness != "claude-alt" {
		t.Errorf("harness = %s", rec.Harness)
	}
}

func TestHelpAndUnknownCommand(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{{"-h"}, {"run", "-h"}, {"check", "-h"}, {"launch", "list", "-h"}} {
		if code := f.run(args...); code != exitOK {
			t.Errorf("%q: exit %d, want 0", args, code)
		}
	}
	if code := f.run("bogus"); code != exitUsage {
		t.Errorf("unknown command: exit %d", code)
	}
	if code := f.run("launch", "bogus"); code != exitUsage {
		t.Errorf("unknown verb: exit %d", code)
	}
	if f.logExists() {
		t.Error("help wrote a launch record")
	}
}

func TestGetwdReturnsPWDThroughSymlink(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Chdir(link)
	t.Setenv("PWD", link)
	got, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got != link {
		t.Errorf("os.Getwd = %s, want the logical path %s", got, link)
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"":             "''",
		"plain/path-1": "plain/path-1",
		"two words":    "'two words'",
		"it's":         `'it'\''s'`,
		`{"a":["/b"]}`: `'{"a":["/b"]}'`,
		"--settings=x": "--settings=x",
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
