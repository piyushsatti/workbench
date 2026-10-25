package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"workspaces/internal/picker"
)

func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// denyJSON is the deny and sandbox part of the --settings JSON for paths, none
// of which is a symlink.
func denyJSON(paths ...string) string {
	var deny, quoted []string
	for _, p := range paths {
		for _, r := range []string{"Read(/" + p + ")", "Read(/" + p + "/**)", "Edit(/" + p + ")", "Edit(/" + p + "/**)"} {
			deny = append(deny, `"`+r+`"`)
		}
		quoted = append(quoted, `"`+p+`"`)
	}
	list := strings.Join(quoted, ",")
	return `"deny":[` + strings.Join(deny, ",") + `]},"sandbox":{"enabled":true,"filesystem":{"denyRead":[` + list + `],"denyWrite":[` + list + `]}}}`
}

// globalDeny and spaceADeny are the deny_access paths of testSpaces.
func globalDeny(home string) []string {
	return []string{home + "/Source/Projects/_archive", home + "/.ssh"}
}

func spaceADeny(home string) []string {
	return append(globalDeny(home), home+"/Source/Projects/a/secrets")
}

func TestRunPassesHarnessArgsVerbatim(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	if code := f.run("run", ".", "claude", "--resume", "x", "-p", "y z"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.execs) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(f.execs))
	}
	want := []string{"claude", "--settings", `{"permissions":{` + denyJSON(globalDeny(f.home)...), "--resume", "x", "-p", "y z"}
	if got := f.execs[0].argv; !slices.Equal(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if got := f.onlyRecord().Args; !slices.Equal(got, want[3:]) {
		t.Errorf("record args = %q, want %q", got, want[1:])
	}
}

func TestRunMissingArgumentWithoutTTY(t *testing.T) {
	for _, tt := range []struct {
		args   []string
		single bool
		msg    string
	}{
		{[]string{"run"}, false, "missing space; no TTY"},
		{[]string{"run", "."}, false, "missing harness; no TTY"},
		{[]string{"run", "."}, true, "missing harness; no TTY"},
	} {
		f := newFixture(t)
		f.tty = false
		if tt.single {
			f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude"}}}`)
		}
		if code := f.run(tt.args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", tt.args, code, exitUsage)
		}
		if len(f.picks) != 0 {
			t.Errorf("%q: picker called without a TTY", tt.args)
		}
		if !strings.Contains(f.stderr.String(), tt.msg) {
			t.Errorf("%q: stderr = %q, want %q", tt.args, f.stderr.String(), tt.msg)
		}
		if f.logExists() {
			t.Errorf("%q: a usage error wrote a record", tt.args)
		}
	}
}

func TestRunRefusalWritesOneRecordAndNeverExecs(t *testing.T) {
	f := newFixture(t)
	f.cwd = filepath.Join(f.home, "outside")
	if code := f.run("run", ".", "claude"); code != exitRefused {
		t.Fatalf("exit %d, want %d", code, exitRefused)
	}
	if len(f.execs) != 0 {
		t.Fatal("exec called after a refusal")
	}
	rec := f.onlyRecord()
	if rec.Decision != "refuse" || rec.Reason != "not_allowed" || rec.Rule != "" {
		t.Errorf("record = %+v", rec)
	}
	if !strings.Contains(f.stderr.String(), "not_allowed") {
		t.Errorf("stderr = %q, want the reason", f.stderr.String())
	}
}

func TestRunRefusalRecordFields(t *testing.T) {
	f := newFixture(t)
	f.cwd = filepath.Join(f.home, "Source/private/drafts")
	if code := f.run("run", ".", "claude-alt", "hi"); code != exitRefused {
		t.Fatalf("exit %d", code)
	}
	rec := f.onlyRecord()
	checks := map[string][2]string{
		"ts":         {rec.TS, "2026-10-04T22:31:05.000Z"},
		"agent":      {rec.Agent, "2026.10.05"},
		"decision":   {rec.Decision, "refuse"},
		"reason":     {rec.Reason, "deny_launch"},
		"rule":       {rec.Rule, "~/Source/private/*"},
		"cwd":        {rec.Cwd, f.cwd},
		"launch_dir": {rec.LaunchDir, f.cwd},
		"harness":    {rec.Harness, "claude-alt"},
		"adapter":    {rec.Adapter, "claude"},
		"mode":       {rec.Mode, "session"},
		"bin":        {rec.Bin, filepath.Join(f.home, "bin/claude")},
		"bin_target": {rec.BinTarget, filepath.Join(f.home, "bin/versions/2.1.289")},
	}
	for field, gw := range checks {
		if gw[0] != gw[1] {
			t.Errorf("%s = %q, want %q", field, gw[0], gw[1])
		}
	}
	if len(rec.ID) != 16 || rec.ConfigSHA256 == "" || rec.Space != nil || rec.Tmux != nil || !rec.TTY ||
		rec.PID != testPid || rec.PPID != 1 {
		t.Errorf("record = %+v", rec)
	}
}

func TestRunLogWriteFailureExits74(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	blocker := filepath.Join(f.home, "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f.env["XDG_STATE_HOME"] = blocker
	if code := f.run("run", ".", "claude"); code != exitIOErr {
		t.Fatalf("exit %d, want %d", code, exitIOErr)
	}
	if len(f.execs) != 0 {
		t.Fatal("exec called although the log write failed")
	}
}

func TestRunHarnessBinaryNotFound(t *testing.T) {
	f := newFixture(t)
	delete(f.lookPath, "claude")
	if code := f.run("run", ".", "claude"); code != exitNotFound {
		t.Fatalf("exit %d, want %d", code, exitNotFound)
	}
	if len(f.execs) != 0 {
		t.Fatal("exec called")
	}
	if rec := f.onlyRecord(); rec.Reason != "harness_not_found" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestRunPickerCancelled(t *testing.T) {
	for _, err := range []error{picker.ErrCancelled, nil} {
		f := newFixture(t)
		f.pickErr = err
		if code := f.run("run"); code != exitCancelled {
			t.Errorf("exit %d, want %d", code, exitCancelled)
		}
		if f.logExists() || len(f.execs) != 0 {
			t.Error("a cancelled picker wrote a record or launched")
		}
	}
}

func TestRunNoPickerInstalled(t *testing.T) {
	f := newFixture(t)
	f.pickErr = picker.ErrNoPicker
	if code := f.run("run"); code != exitUsage {
		t.Errorf("exit %d, want %d", code, exitUsage)
	}
}

func TestRunSkipsSpacePickerWithoutSpaces(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"]}`)
	f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude"}}}`)
	if code := f.run("run"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.picks) != 0 {
		t.Errorf("picks = %q, want none with no spaces and one harness", f.picks)
	}
}

func TestCheckNeverWrites(t *testing.T) {
	f := newFixture(t)
	for _, dir := range []string{"Source", "outside", "Source/private/drafts"} {
		f.cwd = filepath.Join(f.home, dir)
		f.run("check")
		f.run("check", "--json", ".", "claude", "-p", "x")
	}
	f.run("check", "a", "claude")
	if f.logExists() {
		t.Error("check wrote the launch log")
	}
	if len(f.execs) != 0 {
		t.Error("check called exec")
	}
}

func TestCheckOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		dir    string
		args   []string
		code   int
		reason string
		tmux   string
	}{
		{"allowed, bootstrap planned", "Source", nil, exitOK, "allow_launch", "bootstrap: new session main with window Source"},
		{"deny_launch", "Source/private/drafts", nil, exitRefused, "deny_launch", ""},
		{"deny_access", "Source/Projects/_archive/old", nil, exitRefused, "deny_access", ""},
		{"not_allowed", "", nil, exitRefused, "not_allowed", ""},
		{"print mode outside tmux", "Source", []string{".", "claude", "-p", "x"}, exitRefused, "not_in_tmux", ""},
		{"unknown space", "Source", []string{"nope"}, exitUsage, "unknown_space", ""},
		{"unknown harness", "Source", []string{".", "nope"}, exitUsage, "unknown_harness", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.tmuxReply = func([]string) (string, error) { return "", errors.New("no server running") }
			f.cwd = filepath.Join(f.home, tt.dir)
			code := f.run(append([]string{"check", "--json"}, tt.args...)...)
			if code != tt.code {
				t.Fatalf("exit %d, want %d; stderr %s", code, tt.code, f.stderr.String())
			}
			var rep checkReport
			if err := json.Unmarshal(f.stdout.Bytes(), &rep); err != nil {
				t.Fatalf("%v: %s", err, f.stdout.String())
			}
			if rep.Reason != tt.reason || rep.Tmux != tt.tmux {
				t.Errorf("reason, tmux = %q, %q; want %q, %q", rep.Reason, rep.Tmux, tt.reason, tt.tmux)
			}
		})
	}
}

func TestCheckNamedSpaceText(t *testing.T) {
	f := newFixture(t)
	f.cwd = filepath.Join(f.home, "outside")
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "has-session" {
			return "", nil
		}
		return "", errors.New("unexpected")
	}
	if code := f.run("check", "a", "claude-alt"); code != exitOK {
		t.Fatalf("exit %d; stdout %s stderr %s", code, f.stdout.String(), f.stderr.String())
	}
	out := f.stdout.String()
	root := filepath.Join(f.home, "Source/Projects/a")
	for _, want := range []string{
		"decision      allow",
		"launch dir    " + root,
		"CLAUDE_CONFIG_DIR=" + f.home + "/.claude-alt",
		`--settings '{"permissions":{"additionalDirectories":["` + filepath.Join(f.home, "Source/Projects/b") + `"],` + denyJSON(spaceADeny(f.home)...) + `'`,
		"deny_access   3 paths in session",
		"bootstrap: new window claude-alt in session a (session exists)",
		"mode          session",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("check output lacks %q:\n%s", want, out)
		}
	}
}

func TestCheckWarnsAboutMissingPaths(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_launch":["~/Source/private/*"],"deny_access":["~/nope"],
	 "spaces":{"gone":{"root":"~/Source/gone"},"drafts":{"root":"~/Source/private/drafts"}}}`)
	if code := f.run("check"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	out := f.stdout.String()
	for _, want := range []string{
		"deny_access ~/nope does not exist",
		"spaces.gone.root ~/Source/gone does not exist",
		"space drafts fails the check: deny_launch",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("check output lacks %q:\n%s", want, out)
		}
	}
}

// run refuses an attach path only when it falls under deny_access (global or
// the space's own), so check warns about that and nothing else.
func TestCheckWarnsAboutAttachPaths(t *testing.T) {
	tests := []struct {
		name   string
		attach string
		warn   string
	}{
		{"under global deny_access", "~/Source/Projects/_archive/old", "space a attach ~/Source/Projects/_archive/old fails the check: attach_denied ~/Source/Projects/_archive"},
		{"under the space's own deny_access", "~/Source/Projects/b", "space a attach ~/Source/Projects/b fails the check: attach_denied ~/Source/Projects/b"},
		{"outside allow_launch", "/etc", ""},
		{"under deny_launch", "~/Source/private/drafts", ""},
		{"allowed", "~/Source/Projects/a/sub", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.inTmux("999")
			f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_launch":["~/Source/private/*"],"deny_access":["~/Source/Projects/_archive"],
			 "spaces":{"a":{"root":"~/Source/Projects/a","attach":["`+tt.attach+`"],"deny_access":["~/Source/Projects/b"]}}}`)
			if code := f.run("check"); code != exitOK {
				t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
			}
			var attachWarnings []string
			for _, w := range checkJSON(t, f).Warnings {
				if strings.Contains(w, " attach ") {
					attachWarnings = append(attachWarnings, w)
				}
			}
			switch {
			case tt.warn == "" && len(attachWarnings) != 0:
				t.Errorf("warnings = %q, want no attach warning", attachWarnings)
			case tt.warn != "" && (len(attachWarnings) != 1 || attachWarnings[0] != tt.warn):
				t.Errorf("warnings = %q, want exactly %q", attachWarnings, tt.warn)
			}
		})
	}
}

func TestConfigErrors(t *testing.T) {
	tests := []struct {
		name   string
		spaces string
		harn   string
	}{
		{"missing spaces.json", "", testHarnesses},
		{"invalid spaces.json", `{"deny_acess":[]}`, testHarnesses},
		{"invalid harness.json", testSpaces, `{"default":"x","harnesses":{"x":{"adapter":"gemini"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.spaces == "" {
				os.Remove(filepath.Join(f.home, ".config/workspaces/spaces.json"))
			} else {
				f.writeConfig("spaces.json", tt.spaces)
			}
			f.writeConfig("harness.json", tt.harn)
			if code := f.run("run", ".", "claude"); code != exitConfig {
				t.Fatalf("exit %d, want %d", code, exitConfig)
			}
			if len(f.execs) != 0 {
				t.Fatal("exec called")
			}
			rec := f.onlyRecord()
			if rec.Reason != "config_error" {
				t.Errorf("reason = %s", rec.Reason)
			}
			if (tt.spaces == "") != (rec.ConfigSHA256 == "") {
				t.Errorf("config_sha256 = %q; want empty exactly when spaces.json is missing", rec.ConfigSHA256)
			}
		})
	}
}

func TestMissingHarnessJSONUsesBuiltIn(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	os.Remove(filepath.Join(f.home, ".config/workspaces/harness.json"))
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if code := f.run("run", ".", "claude-alt"); code != exitUsage {
		t.Errorf("claude-alt without harness.json: exit %d, want %d", code, exitUsage)
	}
}

func TestRunUnknownSpaceSuggestsDot(t *testing.T) {
	f := newFixture(t)
	f.tty = false
	if code := f.run("run", "claude"); code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(f.stderr.String(), "workspaces run . claude") {
		t.Errorf("stderr = %q, want the suggestion", f.stderr.String())
	}
	if rec := f.onlyRecord(); rec.Reason != "unknown_space" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestRunUnknownHarness(t *testing.T) {
	f := newFixture(t)
	if code := f.run("run", ".", "nope"); code != exitUsage {
		t.Fatalf("exit %d", code)
	}
	if rec := f.onlyRecord(); rec.Reason != "unknown_harness" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestRunSettingsCollision(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	if code := f.run("run", "a", "claude", "--settings", "x.json"); code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if len(f.execs) != 0 {
		t.Fatal("exec called")
	}
	if rec := f.onlyRecord(); rec.Reason != "settings_collision" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestRunNamedSpaceFromElsewhere(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.cwd = filepath.Join(f.home, "outside")
	if code := f.run("run", "a", "claude-alt", "--resume", "abc"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	root := filepath.Join(f.home, "Source/Projects/a")
	attach := filepath.Join(f.home, "Source/Projects/b")
	if !slices.Equal(f.chdirs, []string{root}) {
		t.Errorf("chdirs = %q, want [%s]", f.chdirs, root)
	}
	ex := f.execs[0]
	wantArgv := []string{"claude", "--settings", `{"permissions":{"additionalDirectories":["` + attach + `"],` + denyJSON(spaceADeny(f.home)...), "--resume", "abc"}
	if !slices.Equal(ex.argv, wantArgv) {
		t.Errorf("argv = %q, want %q", ex.argv, wantArgv)
	}
	if ex.path != filepath.Join(f.home, "bin/claude") {
		t.Errorf("exec path = %s, want the PATH result, not its target", ex.path)
	}
	rec := f.onlyRecord()
	for k, want := range map[string]string{
		"CLAUDE_CONFIG_DIR": f.home + "/.claude-alt",
		"AGENT_SPACE":       "a",
		"AGENT_HARNESS":     "claude-alt",
		"AGENT_LAUNCH_ID":   rec.ID,
		"PWD":               root,
		"HOME":              f.home,
	} {
		if got, _ := envValue(ex.env, k); got != want {
			t.Errorf("env %s = %q, want %q", k, got, want)
		}
	}
	if rec.LaunchDir != root || rec.Space == nil || *rec.Space != "a" || !slices.Equal(rec.AddDirs, []string{attach}) {
		t.Errorf("record = %+v", rec)
	}
}

func TestRunDotStaysInPlace(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.cwd = filepath.Join(f.home, "Source/Projects/a/sub")
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.chdirs) != 0 {
		t.Errorf("chdirs = %q, want none for .", f.chdirs)
	}
	if got, _ := envValue(f.execs[0].env, "AGENT_SPACE"); got != "a" {
		t.Errorf("AGENT_SPACE = %q, want the space selected by cwd", got)
	}
}

// Tmux cases, all on the fake tmux runner.

func TestTmuxInsidePane(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.tmuxCalls) != 1 || f.tmuxCalls[0][0] != "display-message" {
		t.Errorf("tmux calls = %q, want only the pane query", f.tmuxCalls)
	}
	if len(f.execs) != 1 {
		t.Fatal("exec not called")
	}
	tm := f.onlyRecord().Tmux
	if tm == nil {
		t.Fatal("tmux is null")
	}
	if tm.Socket != "/tmp/tmux-1000/default" || tm.SessionID != "$5" || tm.Session != "main" || tm.WindowID != "@12" ||
		tm.WindowIndex == nil || *tm.WindowIndex != 3 || tm.Window != "claude" || tm.PaneID != "%30" || tm.Entry != "pane" {
		t.Errorf("tmux = %+v", tm)
	}
}

func TestTmuxBootstrapPaneEntry(t *testing.T) {
	f := newFixture(t)
	f.inTmux("4242")
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if tm := f.onlyRecord().Tmux; tm == nil || tm.Entry != "bootstrap" {
		t.Errorf("tmux = %+v, want entry bootstrap", tm)
	}
}

func TestTmuxBootstrapPaneRefusalWaits(t *testing.T) {
	f := newFixture(t)
	f.inTmux("4242")
	f.cwd = filepath.Join(f.home, "outside")
	if code := f.run("run", ".", "claude"); code != exitRefused {
		t.Fatalf("exit %d", code)
	}
	if !f.waited {
		t.Error("a refusal in a bootstrap pane did not wait for Enter")
	}
	if tm := f.onlyRecord().Tmux; tm == nil || tm.Entry != "bootstrap" {
		t.Errorf("tmux = %+v", tm)
	}

	g := newFixture(t)
	g.inTmux("999")
	g.cwd = filepath.Join(g.home, "outside")
	g.run("run", ".", "claude")
	if g.waited {
		t.Error("a refusal in an ordinary pane waited for Enter")
	}
}

func TestTmuxServerNotAnswering(t *testing.T) {
	f := newFixture(t)
	f.env["TMUX"] = "/nonexistent,1,0"
	f.env["TMUX_PANE"] = "%1"
	f.tmuxReply = func([]string) (string, error) { return "", errors.New("no server running on /nonexistent") }
	if code := f.run("run", ".", "claude"); code != exitUnavailable {
		t.Fatalf("exit %d, want %d", code, exitUnavailable)
	}
	if len(f.execs) != 0 {
		t.Fatal("exec called")
	}
	if rec := f.onlyRecord(); rec.Reason != "tmux_error" || rec.Tmux != nil {
		t.Errorf("record = %+v", rec)
	}
}

func TestTmuxSocketOnlyWithoutPane(t *testing.T) {
	f := newFixture(t)
	f.env["TMUX"] = "/tmp/tmux-1000/default,1,0"
	f.tmuxReply = func(args []string) (string, error) {
		if slices.Equal(args, []string{"display-message", "-p", "#{socket_path}"}) {
			return "/tmp/tmux-1000/default\n", nil
		}
		return "", errors.New("unexpected")
	}
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	tm := f.onlyRecord().Tmux
	if tm == nil || tm.Socket != "/tmp/tmux-1000/default" || tm.PaneID != "" || tm.WindowIndex != nil || tm.Entry != "pane" {
		t.Errorf("tmux = %+v, want the socket only", tm)
	}
}

func bootstrapEnvFor(f *fixture) []string {
	var out []string
	for _, kv := range f.app().Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != "PWD" {
			out = append(out, "-e", kv)
		}
	}
	return append(out, "-e", "PWD="+f.cwd)
}

func TestTmuxBootstrapNewSession(t *testing.T) {
	f := newFixture(t)
	f.cwd = filepath.Join(f.home, "outside")
	f.tmuxReply = func(args []string) (string, error) {
		switch args[0] {
		case "has-session":
			return "", errors.New("can't find session: a")
		case "new-session":
			return "", nil
		}
		return "", errors.New("unexpected")
	}
	if code := f.run("run", "a", "claude", "fix it", "--", "-p"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	root := filepath.Join(f.home, "Source/Projects/a")
	wantNew := append([]string{"new-session", "-d", "-s", "a", "-n", "claude", "-c", root}, bootstrapEnvFor(f)...)
	wantNew = append(wantNew, "--", "/opt/bin/workspaces", "run", "a", "claude", "fix it", "--", "-p")
	want := [][]string{{"has-session", "-t", "=a"}, wantNew}
	if len(f.tmuxCalls) != 2 || !slices.Equal(f.tmuxCalls[0], want[0]) || !slices.Equal(f.tmuxCalls[1], want[1]) {
		t.Errorf("tmux calls =\n  %q\nwant\n  %q", f.tmuxCalls, want)
	}
	if len(f.execs) != 1 || f.execs[0].path != "/usr/bin/tmux" ||
		!slices.Equal(f.execs[0].argv, []string{"tmux", "attach-session", "-t", "=a"}) {
		t.Errorf("execs = %+v, want tmux attach-session", f.execs)
	}
	if f.logExists() {
		t.Error("the outer workspaces wrote a record on a successful bootstrap")
	}
}

func TestTmuxBootstrapSessionExists(t *testing.T) {
	f := newFixture(t)
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "has-session" || args[0] == "new-window" {
			return "", nil
		}
		return "", errors.New("unexpected")
	}
	if code := f.run("run", "a", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.tmuxCalls) != 2 || !slices.Equal(f.tmuxCalls[1][:3], []string{"new-window", "-t", "=a:"}) {
		t.Errorf("tmux calls = %q, want has-session then new-window -t =a:", f.tmuxCalls)
	}
}

func TestTmuxBootstrapDuplicateSessionRetriesOnce(t *testing.T) {
	f := newFixture(t)
	f.tmuxReply = func(args []string) (string, error) {
		switch args[0] {
		case "has-session":
			return "", errors.New("can't find session")
		case "new-session":
			return "", errors.New("tmux new-session: duplicate session: a")
		case "new-window":
			return "", nil
		}
		return "", errors.New("unexpected")
	}
	if code := f.run("run", "a", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	var verbs []string
	for _, c := range f.tmuxCalls {
		verbs = append(verbs, c[0])
	}
	if !slices.Equal(verbs, []string{"has-session", "new-session", "new-window"}) {
		t.Errorf("tmux verbs = %q", verbs)
	}
}

func TestTmuxBootstrapFailureLogsTmuxError(t *testing.T) {
	f := newFixture(t)
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "has-session" {
			return "", nil
		}
		return "", errors.New("tmux new-window: create window failed")
	}
	if code := f.run("run", "a", "claude"); code != exitUnavailable {
		t.Fatalf("exit %d, want %d", code, exitUnavailable)
	}
	if len(f.execs) != 0 {
		t.Fatal("exec called")
	}
	if rec := f.onlyRecord(); rec.Reason != "tmux_error" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestTmuxBootstrapNoSpaceUsesMain(t *testing.T) {
	f := newFixture(t)
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "has-session" {
			return "", nil
		}
		return "", nil
	}
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if !slices.Equal(f.tmuxCalls[0], []string{"has-session", "-t", "=main"}) {
		t.Errorf("first tmux call = %q", f.tmuxCalls[0])
	}
	nw := f.tmuxCalls[1]
	if !slices.Equal(nw[:7], []string{"new-window", "-t", "=main:", "-n", "Source", "-c", f.cwd}) {
		t.Errorf("new-window = %q, want window named after the launch dir", nw[:7])
	}
	if tail := nw[len(nw)-4:]; !slices.Equal(tail, []string{"/opt/bin/workspaces", "run", ".", "claude"}) {
		t.Errorf("command = %q, want the space argument kept as .", tail)
	}
}

func TestTmuxRefusalsWithoutTmuxCalls(t *testing.T) {
	tests := []struct {
		name   string
		inside bool
		tty    bool
		args   []string
		reason string
	}{
		{"outside, print mode", false, true, []string{"-p", "x"}, "not_in_tmux"},
		{"outside, no TTY", false, false, nil, "not_in_tmux"},
		{"background outside", false, true, []string{"--bg"}, "background_refused"},
		{"background inside", true, true, []string{"--bg"}, "background_refused"},
		{"respawn inside", true, true, []string{"respawn"}, "background_refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.inside {
				f.inTmux("999")
			}
			f.tty = tt.tty
			if code := f.run(append([]string{"run", ".", "claude"}, tt.args...)...); code != exitRefused {
				t.Fatalf("exit %d, want %d", code, exitRefused)
			}
			if len(f.tmuxCalls) != 0 {
				t.Errorf("tmux calls = %q, want none", f.tmuxCalls)
			}
			if len(f.execs) != 0 {
				t.Error("exec called")
			}
			if rec := f.onlyRecord(); rec.Reason != tt.reason {
				t.Errorf("reason = %s, want %s", rec.Reason, tt.reason)
			}
		})
	}
}

func TestTmuxNotInTmuxHint(t *testing.T) {
	f := newFixture(t)
	f.run("run", ".", "claude", "-p", "hi")
	for _, want := range []string{"print mode cannot be moved into a new tmux window", "Start tmux, then run the same command again"} {
		if !strings.Contains(f.stderr.String(), want) {
			t.Errorf("stderr = %q, want %q", f.stderr.String(), want)
		}
	}
}

func TestTmuxUtilityRunsInPlace(t *testing.T) {
	f := newFixture(t)
	f.tty = false
	if code := f.run("run", ".", "claude", "--version"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.tmuxCalls) != 0 || len(f.execs) != 1 {
		t.Errorf("tmux calls %q, execs %d; want none and one", f.tmuxCalls, len(f.execs))
	}
	if rec := f.onlyRecord(); rec.Mode != "utility" || rec.Tmux != nil {
		t.Errorf("record = %+v", rec)
	}
}

func TestTmuxUtilityNeverFailsOnTmux(t *testing.T) {
	f := newFixture(t)
	f.env["TMUX"] = "/nonexistent,1,0"
	f.env["TMUX_PANE"] = "%1"
	f.tmuxReply = func([]string) (string, error) { return "", errors.New("no server running") }
	if code := f.run("run", ".", "claude", "auth", "status"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.execs) != 1 || f.onlyRecord().Tmux != nil {
		t.Error("utility launch should exec with tmux null")
	}
}

func TestTmuxNotOnPath(t *testing.T) {
	f := newFixture(t)
	delete(f.lookPath, "tmux")
	if code := f.run("run", ".", "claude"); code != exitUnavailable {
		t.Fatalf("exit %d, want %d", code, exitUnavailable)
	}
	if len(f.tmuxCalls) != 0 || len(f.execs) != 0 {
		t.Error("tmux or exec called")
	}
	if rec := f.onlyRecord(); rec.Reason != "tmux_error" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestTmuxBootstrapEnv(t *testing.T) {
	f := newFixture(t)
	for k, v := range map[string]string{
		"TMUX_PANE": "%9", "TERM": "xterm-256color", "TERM_PROGRAM": "x", "TERM_PROGRAM_VERSION": "1",
		"SHLVL": "2", "OLDPWD": "/old", "_": "/usr/bin/workspaces", "PWD": "/stale", "CLAUDE_CODE_X": "kept",
	} {
		f.env[k] = v
	}
	link := filepath.Join(f.home, "srclink")
	if err := os.Symlink(filepath.Join(f.home, "Source"), link); err != nil {
		t.Fatal(err)
	}
	f.cwd = link
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "has-session" {
			return "", errors.New("no session")
		}
		return "", nil
	}
	if code := f.run("run", ".", "claude", "two words", `it's "q"`); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	ns := f.tmuxCalls[1]
	var envs []string
	sep := slices.Index(ns, "--")
	for i := 0; i < sep; i++ {
		if ns[i] == "-e" {
			envs = append(envs, ns[i+1])
		}
	}
	for _, kv := range envs {
		k, _, _ := strings.Cut(kv, "=")
		if slices.Contains([]string{"TMUX", "TMUX_PANE", "TERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "SHLVL", "OLDPWD", "_"}, k) {
			t.Errorf("bootstrap passes %s", kv)
		}
	}
	if v, _ := envValue(envs, "PWD"); v != link {
		t.Errorf("PWD = %q, want the logical cwd %s", v, link)
	}
	if v, _ := envValue(envs, "CLAUDE_CODE_X"); v != "kept" {
		t.Errorf("CLAUDE_CODE_X = %q, want it passed through", v)
	}
	if i := slices.Index(ns, "-c"); ns[i+1] != link {
		t.Errorf("-c %s, want the logical cwd", ns[i+1])
	}
	if tail := ns[sep+1:]; !slices.Equal(tail, []string{"/opt/bin/workspaces", "run", ".", "claude", "two words", `it's "q"`}) {
		t.Errorf("command = %q, want user args as separate elements", tail)
	}
}

func TestTmuxBootstrapArgvTooLong(t *testing.T) {
	f := newFixture(t)
	f.env["HUGE_A"] = strings.Repeat("a", 6000)
	f.env["HUGE_B"] = strings.Repeat("b", 5000)
	f.env["HUGE_C"] = strings.Repeat("c", 3000)
	if code := f.run("run", ".", "claude"); code != exitUnavailable {
		t.Fatalf("exit %d, want %d", code, exitUnavailable)
	}
	if len(f.tmuxCalls) != 0 || len(f.execs) != 0 {
		t.Error("tmux or exec called")
	}
	msg := f.stderr.String()
	for _, k := range []string{"HUGE_A", "HUGE_B", "HUGE_C"} {
		if !strings.Contains(msg, k) {
			t.Errorf("stderr = %q, want it to name %s", msg, k)
		}
	}
	if rec := f.onlyRecord(); rec.Reason != "tmux_error" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestTmuxBootstrapEscapesTmuxSyntax(t *testing.T) {
	f := newFixture(t)
	f.cwd = filepath.Join(f.home, "Source", "odd #{x};")
	if err := os.Mkdir(f.cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	f.env["SEMI"] = "v;"
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "new-session" {
			return "", nil
		}
		return "", errors.New("can't find session: main")
	}
	if code := f.run("run", ".", "claude", "fix it;", ";"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.tmuxCalls) != 2 {
		t.Fatalf("tmux calls = %q, want has-session and new-session", f.tmuxCalls)
	}
	ns := f.tmuxCalls[1]
	if i := slices.Index(ns, "-n"); ns[i+1] != `odd ##{x}\;` {
		t.Errorf("-n %q, want # doubled and the final ; escaped", ns[i+1])
	}
	wantDir := filepath.Join(f.home, "Source", `odd ##{x}\;`)
	if i := slices.Index(ns, "-c"); ns[i+1] != wantDir {
		t.Errorf("-c %q, want %q", ns[i+1], wantDir)
	}
	pwd := "PWD=" + filepath.Join(f.home, "Source", `odd #{x}\;`)
	if !slices.Contains(ns, `SEMI=v\;`) || !slices.Contains(ns, pwd) {
		t.Errorf("new-session = %q, want -e values with the final ; escaped and # kept", ns)
	}
	sep := slices.Index(ns, "--")
	if tail := ns[sep+1:]; !slices.Equal(tail, []string{"/opt/bin/workspaces", "run", ".", "claude", `fix it\;`, `\;`}) {
		t.Errorf("command = %q, want each final ; escaped", tail)
	}
}

func TestRunFirstRefusalWins(t *testing.T) {
	tests := []struct {
		name  string
		tty   bool
		setup func(f *fixture)
		args  []string
	}{
		{"missing binary", true, func(f *fixture) { delete(f.lookPath, "claude") }, nil},
		{"settings collision", true, nil, []string{"--settings", "x"}},
		{"background", true, nil, []string{"--bg"}},
		{"no TTY outside tmux", false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.cwd = filepath.Join(f.home, "Source/Projects/a/secrets")
			f.tty = tt.tty
			if tt.setup != nil {
				tt.setup(f)
			}
			if code := f.run(append([]string{"run", ".", "claude"}, tt.args...)...); code != exitRefused {
				t.Fatalf("exit %d, want %d; stderr %s", code, exitRefused, f.stderr.String())
			}
			if rec := f.onlyRecord(); rec.Reason != "deny_access" || rec.Rule != "~/Source/Projects/a/secrets" {
				t.Errorf("record %s %s, want deny_access ~/Source/Projects/a/secrets", rec.Reason, rec.Rule)
			}
			if len(f.tmuxCalls) != 0 || len(f.execs) != 0 {
				t.Errorf("tmux calls %q, execs %d; want none", f.tmuxCalls, len(f.execs))
			}
		})
	}
}

func TestRunDeniedOutsideTmuxMakesNoTmuxCalls(t *testing.T) {
	for _, dir := range []string{"outside", "Source/private/drafts"} {
		f := newFixture(t)
		f.cwd = filepath.Join(f.home, dir)
		if code := f.run("run", ".", "claude"); code != exitRefused {
			t.Fatalf("%s: exit %d, want %d", dir, code, exitRefused)
		}
		if len(f.tmuxCalls) != 0 {
			t.Errorf("%s: tmux calls = %q, want none", dir, f.tmuxCalls)
		}
	}
}

func TestRunInternalErrorsExit70(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(f *fixture)
		args      []string
		wantExecs int
		wantLog   bool
	}{
		{"getwd fails", func(f *fixture) { f.getwdErr = errors.New("gone") }, []string{".", "claude"}, 0, false},
		{"chdir fails", func(f *fixture) { f.inTmux("999"); f.chdirErr = errors.New("gone") }, []string{"a", "claude"}, 0, true},
		{"exec fails", func(f *fixture) { f.inTmux("999"); f.execErr = errors.New("ENOEXEC") }, []string{".", "claude"}, 1, true},
		{"cwd does not resolve", func(f *fixture) { f.cwd = filepath.Join(f.home, "deleted") }, []string{".", "claude"}, 0, false},
		{"executable unknown", func(f *fixture) { f.executableErr = errors.New("no /proc") }, []string{".", "claude"}, 0, false},
		{"picker fails", func(f *fixture) { f.pickErr = errors.New("fzf crashed") }, nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.setup(f)
			if code := f.run(append([]string{"run"}, tt.args...)...); code != exitInternal {
				t.Fatalf("exit %d, want %d; stderr %s", code, exitInternal, f.stderr.String())
			}
			if len(f.execs) != tt.wantExecs {
				t.Errorf("execs = %d, want %d", len(f.execs), tt.wantExecs)
			}
			if f.logExists() != tt.wantLog {
				t.Errorf("log written = %v, want %v", f.logExists(), tt.wantLog)
			}
			for _, c := range f.tmuxCalls {
				if c[0] == "new-session" || c[0] == "new-window" {
					t.Errorf("tmux %s called", c[0])
				}
			}
		})
	}
}

func TestTmuxAttachExecFailureExits70(t *testing.T) {
	f := newFixture(t)
	f.execErr = errors.New("ENOEXEC")
	f.tmuxReply = func(args []string) (string, error) {
		if args[0] == "new-session" {
			return "", nil
		}
		return "", errors.New("can't find session: main")
	}
	if code := f.run("run", ".", "claude"); code != exitInternal {
		t.Fatalf("exit %d, want %d", code, exitInternal)
	}
	if len(f.execs) != 1 || f.execs[0].path != "/usr/bin/tmux" {
		t.Errorf("execs = %+v, want one tmux attach attempt", f.execs)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("EPIPE") }

func TestCheckJSONWriteErrorExits70(t *testing.T) {
	f := newFixture(t)
	a := f.app()
	a.Stdout = failingWriter{}
	if code := a.Run([]string{"check", "--json"}); code != exitInternal {
		t.Fatalf("exit %d, want %d", code, exitInternal)
	}
}

func TestRunHomeUnsetIsConfigError(t *testing.T) {
	f := newFixture(t)
	delete(f.env, "HOME")
	delete(f.env, "XDG_CONFIG_HOME")
	if code := f.run("run", ".", "claude"); code != exitConfig {
		t.Fatalf("exit %d, want %d", code, exitConfig)
	}
	if len(f.execs) != 0 {
		t.Error("exec called")
	}
}

func TestTmuxBootstrapPaneWaitsOnEveryFailure(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *fixture)
		args  []string
		want  int
	}{
		{"chdir fails", func(f *fixture) { f.chdirErr = errors.New("gone") }, []string{"a", "claude"}, exitInternal},
		{"exec fails", func(f *fixture) { f.execErr = errors.New("ENOEXEC") }, []string{".", "claude"}, exitInternal},
		{"log write fails", func(f *fixture) {
			blocker := filepath.Join(f.home, "not-a-dir")
			if err := os.WriteFile(blocker, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			f.env["XDG_STATE_HOME"] = blocker
		}, []string{".", "claude"}, exitIOErr},
		{"unlogged usage error", func(f *fixture) { f.tty = false }, []string{"."}, exitUsage},
		{"success", func(*fixture) {}, []string{".", "claude"}, exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.inTmux("4242")
			tt.setup(f)
			if code := f.run(append([]string{"run"}, tt.args...)...); code != tt.want {
				t.Fatalf("exit %d, want %d; stderr %s", code, tt.want, f.stderr.String())
			}
			wantWait := tt.want != exitOK
			if f.waited != wantWait {
				t.Errorf("waited = %v, want %v", f.waited, wantWait)
			}
			if prompted := strings.Contains(f.stderr.String(), "Press Enter to close this window."); prompted != wantWait {
				t.Errorf("Enter prompt shown = %v, want %v", prompted, wantWait)
			}
		})
	}
}

func TestRunDotInsideSpaceAttaches(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.cwd = filepath.Join(f.home, "Source/Projects/a/sub")
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	b := filepath.Join(f.home, "Source/Projects/b")
	want := []string{"--settings", `{"permissions":{"additionalDirectories":["` + b + `"],` + denyJSON(spaceADeny(f.home)...)}
	if got := f.execs[0].argv; len(got) < 3 || !slices.Equal(got[1:3], want) {
		t.Errorf("argv = %q, want argv[1:3] %q", got, want)
	}
	if got := f.onlyRecord().AddDirs; !slices.Equal(got, []string{b}) {
		t.Errorf("add_dirs = %q, want [%s]", got, b)
	}
}

// linkIntoArchive makes ~/Source/Projects/_archive/link point at ~/Source/Projects/b,
// an allowed folder, and returns the link.
func linkIntoArchive(t *testing.T, f *fixture) string {
	t.Helper()
	link := filepath.Join(f.home, "Source/Projects/_archive/link")
	if err := os.Symlink("../b", link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestRunDeniesLogicalCwd(t *testing.T) {
	f := newFixture(t)
	f.cwd = linkIntoArchive(t, f)
	if code := f.run("run", ".", "claude"); code != exitRefused {
		t.Fatalf("exit %d, want %d", code, exitRefused)
	}
	if rec := f.onlyRecord(); rec.Reason != "deny_access" || rec.Rule != "~/Source/Projects/_archive" {
		t.Errorf("record %s %s, want deny_access ~/Source/Projects/_archive", rec.Reason, rec.Rule)
	}
	if len(f.execs) != 0 {
		t.Error("exec called")
	}
	want := f.cwd + " (" + filepath.Join(f.home, "Source/Projects/b") + ")"
	if !strings.Contains(f.stderr.String(), want) {
		t.Errorf("stderr = %q, want it to show %q", f.stderr.String(), want)
	}
}

// From ~/alink/sub, where ~/alink points at space a's root.
func runThroughSpaceLink(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.inTmux("999")
	if err := os.Symlink(filepath.Join(f.home, "Source/Projects/a"), filepath.Join(f.home, "alink")); err != nil {
		t.Fatal(err)
	}
	f.cwd = filepath.Join(f.home, "alink/sub")
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	return f
}

func TestRunSelectsSpaceByPhysicalCwd(t *testing.T) {
	f := runThroughSpaceLink(t)
	if v, ok := envValue(f.execs[0].env, "AGENT_SPACE"); !ok || v != "a" {
		t.Errorf("AGENT_SPACE = %q (set %v), want a", v, ok)
	}
	if rec := f.onlyRecord(); rec.Space == nil || *rec.Space != "a" {
		t.Errorf("record space = %v, want a", rec.Space)
	}
}

func TestRunRecordsPhysicalLaunchDir(t *testing.T) {
	f := runThroughSpaceLink(t)
	rec := f.onlyRecord()
	if want := filepath.Join(f.home, "Source/Projects/a/sub"); rec.LaunchDir != want || rec.Cwd != want {
		t.Errorf("launch_dir %s, cwd %s; want both %s", rec.LaunchDir, rec.Cwd, want)
	}
	if want := filepath.Join(f.home, "alink/sub"); rec.CwdLogical != want {
		t.Errorf("cwd_logical = %s, want %s", rec.CwdLogical, want)
	}
}

func TestRunUsesConfiguredBin(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	other := filepath.Join(f.home, "bin/other")
	f.lookPath = map[string]string{other: other, "tmux": "/usr/bin/tmux"}
	f.writeConfig("harness.json", `{"default":"c","harnesses":{"c":{"adapter":"claude","bin":"~/bin/other"}}}`)
	if code := f.run("run", ".", "c"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if f.execs[0].path != other {
		t.Errorf("exec path = %s, want %s", f.execs[0].path, other)
	}
	if rec := f.onlyRecord(); rec.Bin != other {
		t.Errorf("record bin = %s, want %s", rec.Bin, other)
	}

	g := newFixture(t)
	g.writeConfig("harness.json", `{"default":"c","harnesses":{"c":{"adapter":"claude","bin":"rel/x"}}}`)
	if code := g.run("run", ".", "c"); code != exitConfig {
		t.Errorf("relative bin: exit %d, want %d", code, exitConfig)
	}
}

func TestRunNoSpaceSetsEmptyAgentSpace(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.cwd = filepath.Join(f.home, "Source/Projects/b")
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if v, ok := envValue(f.execs[0].env, "AGENT_SPACE"); !ok || v != "" {
		t.Errorf("AGENT_SPACE = %q (set %v), want present and empty", v, ok)
	}
}

func TestRunAgentEnvBeatsHarnessEnv(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.cwd = filepath.Join(f.home, "Source/Projects/a")
	f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude","env":{"AGENT_SPACE":"x","AGENT_HARNESS":"y"}}}}`)
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	env := f.execs[0].env
	if v, _ := envValue(env, "AGENT_SPACE"); v != "a" {
		t.Errorf("AGENT_SPACE = %q, want a", v)
	}
	if v, _ := envValue(env, "AGENT_HARNESS"); v != "claude" {
		t.Errorf("AGENT_HARNESS = %q, want claude", v)
	}
}

func TestRunRecordsTTY(t *testing.T) {
	for _, tty := range []bool{false, true} {
		f := newFixture(t)
		f.inTmux("999")
		f.tty = tty
		if code := f.run("run", ".", "claude"); code != exitOK {
			t.Fatalf("tty %v: exit %d; stderr %s", tty, code, f.stderr.String())
		}
		if rec := f.onlyRecord(); rec.TTY != tty {
			t.Errorf("record tty = %v, want %v", rec.TTY, tty)
		}
	}
}

func checkJSON(t *testing.T, f *fixture, args ...string) checkReport {
	t.Helper()
	f.stdout.Reset()
	f.run(append([]string{"check", "--json"}, args...)...)
	var rep checkReport
	if err := json.Unmarshal(f.stdout.Bytes(), &rep); err != nil {
		t.Fatalf("check --json output %q: %v", f.stdout.String(), err)
	}
	return rep
}

func TestCheckUsesDefaultHarness(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	rep := checkJSON(t, f)
	if rep.Harness != "claude" || len(rep.Argv) == 0 || rep.Argv[0] != "claude" {
		t.Errorf("harness %s, argv %q; want claude", rep.Harness, rep.Argv)
	}
}

func TestCheckEnvPWDIsLaunchDir(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	rep := checkJSON(t, f, "a", "claude")
	if want := filepath.Join(f.home, "Source/Projects/a"); rep.Env["PWD"] != want {
		t.Errorf("env.PWD = %q, want %q", rep.Env["PWD"], want)
	}
}

func TestCheckRootWarningHasNoTrailingSpace(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"spaces":{"o":{"root":"~/outside"}}}`)
	rep := checkJSON(t, f)
	if !slices.Contains(rep.Warnings, "space o fails the check: not_allowed") {
		t.Errorf("warnings = %q, want exactly %q", rep.Warnings, "space o fails the check: not_allowed")
	}
}

func TestRunRefusalLogFailureStillRefuses(t *testing.T) {
	f := newFixture(t)
	f.cwd = filepath.Join(f.home, "outside")
	blocker := filepath.Join(f.home, "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f.env["XDG_STATE_HOME"] = blocker
	if code := f.run("run", ".", "claude"); code != exitRefused {
		t.Fatalf("exit %d, want %d", code, exitRefused)
	}
	if len(f.execs) != 0 {
		t.Error("exec called")
	}
	if !strings.Contains(f.stderr.String(), "could not log this refusal") {
		t.Errorf("stderr = %q, want the log warning", f.stderr.String())
	}
}

func TestTmuxUtilityRecordsPaneIDs(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	if code := f.run("run", ".", "claude", "--version"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if tm := f.onlyRecord().Tmux; tm == nil || tm.PaneID != "%30" || tm.Entry != "pane" {
		t.Errorf("tmux = %+v, want pane %%30, entry pane", tm)
	}
}

func TestRunBlankPickIsCancel(t *testing.T) {
	f := newFixture(t)
	f.pickReply = []string{"   "}
	if code := f.run("run"); code != exitCancelled {
		t.Fatalf("exit %d, want %d", code, exitCancelled)
	}
	if f.logExists() {
		t.Error("a blank pick wrote a record")
	}
}

func TestRunRelativeHomeIsConfigError(t *testing.T) {
	f := newFixture(t)
	f.env["HOME"] = "rel"
	delete(f.env, "XDG_CONFIG_HOME")
	if code := f.run("run", ".", "claude"); code != exitConfig {
		t.Fatalf("exit %d, want %d", code, exitConfig)
	}
	if !strings.Contains(f.stderr.String(), "HOME is not set to an absolute path") {
		t.Errorf("stderr = %q, want the HOME error", f.stderr.String())
	}
}

func execArgvSettings(t *testing.T, f *fixture) string {
	t.Helper()
	if len(f.execs) != 1 {
		t.Fatalf("exec calls = %d, want 1; stderr %s", len(f.execs), f.stderr.String())
	}
	argv := f.execs[0].argv
	if len(argv) < 3 || argv[1] != "--settings" {
		t.Fatalf("argv = %q, want --settings first", argv)
	}
	return argv[2]
}

func TestRunDenyAccessSettings(t *testing.T) {
	tests := []struct {
		name   string
		spaces func(home string) string
		args   []string
		cwd    string
		want   func(home string) string
	}{
		{
			name:   "global only without a space",
			spaces: func(string) string { return testSpaces },
			args:   []string{"run", ".", "claude"},
			cwd:    "Source",
			want:   func(h string) string { return `{"permissions":{` + denyJSON(globalDeny(h)...) },
		},
		{
			name:   "space entries follow global ones",
			spaces: func(string) string { return testSpaces },
			args:   []string{"run", "a", "claude"},
			cwd:    "outside",
			want: func(h string) string {
				return `{"permissions":{"additionalDirectories":["` + h + `/Source/Projects/b"],` + denyJSON(spaceADeny(h)...)
			},
		},
		{
			name: "deny only, space without attach",
			spaces: func(string) string {
				return `{"allow_launch":["~/Source"],"deny_access":["~/.ssh"],"spaces":{"b":{"root":"~/Source/Projects/b","deny_access":["~/Source/Projects/b/x"]}}}`
			},
			args: []string{"run", "b", "claude"},
			cwd:  "outside",
			want: func(h string) string {
				return `{"permissions":{` + denyJSON(h+"/.ssh", h+"/Source/Projects/b/x")
			},
		},
		{
			name: "repeated entries appear once, global position wins",
			spaces: func(string) string {
				return `{"allow_launch":["~/Source"],"deny_access":["~/.ssh","~/.ssh/","~/.aws"],
				 "spaces":{"b":{"root":"~/Source/Projects/b","deny_access":["~/.aws","~/.gnupg","~/.ssh"]}}}`
			},
			args: []string{"run", "b", "claude"},
			cwd:  "outside",
			want: func(h string) string {
				return `{"permissions":{` + denyJSON(h+"/.ssh", h+"/.aws", h+"/.gnupg")
			},
		},
		{
			name: "attach only when deny_access is empty",
			spaces: func(string) string {
				return `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b"]}}}`
			},
			args: []string{"run", "a", "claude"},
			cwd:  "outside",
			want: func(h string) string {
				return `{"permissions":{"additionalDirectories":["` + h + `/Source/Projects/b"]}}`
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.inTmux("999")
			f.cwd = filepath.Join(f.home, tt.cwd)
			f.writeConfig("spaces.json", tt.spaces(f.home))
			if code := f.run(tt.args...); code != exitOK {
				t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
			}
			if got, want := execArgvSettings(t, f), tt.want(f.home); got != want {
				t.Errorf("settings =\n  %s\nwant\n  %s", got, want)
			}
		})
	}
}

func TestRunNoDenyAccessNoSettings(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"]}`)
	if code := f.run("run", ".", "claude", "--settings", "mine.json"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if want := []string{"claude", "--settings", "mine.json"}; !slices.Equal(f.execs[0].argv, want) {
		t.Errorf("argv = %q, want %q", f.execs[0].argv, want)
	}
}

// An entry that is a symlink is denied under both names, the written path
// first.
func TestRunDenyAccessSymlinkedEntry(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	real := filepath.Join(f.home, "outside")
	link := filepath.Join(f.home, "ssh-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_access":["~/ssh-link"]}`)
	if code := f.run("run", ".", "claude"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	want := `{"permissions":{` + denyJSON(link, real)
	if got := execArgvSettings(t, f); got != want {
		t.Errorf("settings =\n  %s\nwant\n  %s", got, want)
	}
}

func TestRunSettingsCollisionWithDenyOnly(t *testing.T) {
	for _, arg := range [][]string{{"--settings", "x.json"}, {"--settings=x.json"}} {
		f := newFixture(t)
		f.inTmux("999")
		f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_access":["~/.ssh"]}`)
		if code := f.run(append([]string{"run", ".", "claude"}, arg...)...); code != exitUsage {
			t.Fatalf("args %q: exit %d, want %d", arg, code, exitUsage)
		}
		if len(f.execs) != 0 {
			t.Fatal("exec called")
		}
		if rec := f.onlyRecord(); rec.Reason != "settings_collision" {
			t.Errorf("reason = %s", rec.Reason)
		}
		if !strings.Contains(f.stderr.String(), "attach paths and deny_access") {
			t.Errorf("stderr = %q, want the message to name deny_access", f.stderr.String())
		}
	}
}

func TestCheckDenyAccessCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
		cwd  string
		want int
	}{
		{"global only", []string{"check", "."}, "Source", 2},
		{"global plus space", []string{"check", "a"}, "outside", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.inTmux("999")
			f.cwd = filepath.Join(f.home, tt.cwd)
			if code := f.run(tt.args...); code != exitOK {
				t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
			}
			line := fmt.Sprintf("deny_access   %d paths in session\n", tt.want)
			if !strings.Contains(f.stdout.String(), line) {
				t.Errorf("check output lacks %q:\n%s", line, f.stdout.String())
			}
			f.stdout.Reset()
			if code := f.run(append([]string{"check", "--json"}, tt.args[1:]...)...); code != exitOK {
				t.Fatalf("json exit %d", code)
			}
			var rep checkReport
			if err := json.Unmarshal(f.stdout.Bytes(), &rep); err != nil {
				t.Fatal(err)
			}
			if rep.DenyCount != tt.want {
				t.Errorf("deny_access_paths = %d, want %d", rep.DenyCount, tt.want)
			}
		})
	}
}

func TestCheckDenyAccessCountZero(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"]}`)
	if code := f.run("check"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if want := "deny_access   0 paths in session\n"; !strings.Contains(f.stdout.String(), want) {
		t.Errorf("check output lacks %q:\n%s", want, f.stdout.String())
	}
}

func TestCheckDenyAccessCountIgnoresRepeats(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_access":["~/.ssh","~/.ssh/"],
	 "spaces":{"b":{"root":"~/Source/Projects/b","deny_access":["~/.ssh","~/.aws"]}}}`)
	if code := f.run("check", "b"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if want := "deny_access   2 paths in session\n"; !strings.Contains(f.stdout.String(), want) {
		t.Errorf("check output lacks %q:\n%s", want, f.stdout.String())
	}
}
