package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runHook runs "workspaces hook session-start" with stdin and AGENT_LAUNCH_ID set.
func (f *fixture) runHook(launchID, stdin string) int {
	f.t.Helper()
	if launchID != "" {
		f.env["AGENT_LAUNCH_ID"] = launchID
	}
	app := f.app()
	app.Stdin = strings.NewReader(stdin)
	return app.Run([]string{"hook", "session-start"})
}

func (f *fixture) sessionsFile() string {
	return filepath.Join(f.home, ".local/state/workspaces/sessions.jsonl")
}

const hookPayload = `{"session_id":"s-123","transcript_path":"/t/x.jsonl","cwd":"/h/Source/a","hook_event_name":"SessionStart","permission_mode":"auto","source":"startup","model":"m","agent_type":"","session_title":"t"}`

func TestHookAppendsTheJoinRecord(t *testing.T) {
	f := newFixture(t)
	if code := f.runHook("abc123", hookPayload); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	b, err := os.ReadFile(f.sessionsFile())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ts":"2026-10-04T22:31:05.000Z","launch_id":"abc123","session_id":"s-123","cwd":"/h/Source/a","source":"startup"}` + "\n"
	if string(b) != want {
		t.Errorf("sessions.jsonl = %q, want %q", b, want)
	}
	if f.stdout.Len() != 0 || f.stderr.Len() != 0 {
		t.Errorf("stdout %q, stderr %q: the hook must print nothing", f.stdout.String(), f.stderr.String())
	}
	info, _ := os.Stat(f.sessionsFile())
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if f.logExists() {
		t.Error("the hook wrote the launch log")
	}
}

func TestHookAppendsEveryStart(t *testing.T) {
	f := newFixture(t)
	f.runHook("l1", hookPayload)
	f.runHook("l2", strings.Replace(hookPayload, `"startup"`, `"resume"`, 1))
	lines := strings.Split(strings.TrimRight(readFile(t, f.sessionsFile()), "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"launch_id":"l1"`) || !strings.Contains(lines[1], `"launch_id":"l2"`) ||
		!strings.Contains(lines[1], `"source":"resume"`) {
		t.Errorf("lines = %q", lines)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHookWithoutLaunchIDDoesNothing(t *testing.T) {
	f := newFixture(t)
	if code := f.runHook("", hookPayload); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".local/state")); err == nil {
		t.Error("the hook created the state dir without a launch id")
	}
	if f.stdout.Len() != 0 || f.stderr.Len() != 0 {
		t.Errorf("stdout %q, stderr %q", f.stdout.String(), f.stderr.String())
	}
	// An empty value counts as unset.
	f.env["AGENT_LAUNCH_ID"] = ""
	f.app().Run([]string{"hook", "session-start"})
	if exists(f.sessionsFile()) {
		t.Error("an empty AGENT_LAUNCH_ID was recorded")
	}
}

func TestHookNeverFailsTheSession(t *testing.T) {
	big := `{"session_id":"s","pad":"` + strings.Repeat("x", 2<<20) + `"}`
	tests := []struct {
		name  string
		stdin string
		msg   string
	}{
		{"not json", "this is not json", "reading the hook input"},
		{"empty stdin", "", "reading the hook input"},
		{"json of the wrong shape", `["session_id"]`, "reading the hook input"},
		{"no session id", `{"cwd":"/x","source":"startup"}`, "no session_id"},
		{"blank session id", `{"session_id":""}`, "no session_id"},
		{"truncated json", `{"session_id":"s",`, "reading the hook input"},
		{"input over the limit", big, "reading the hook input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if code := f.runHook("abc", tt.stdin); code != exitOK {
				t.Fatalf("exit %d, want 0", code)
			}
			if !strings.Contains(f.stderr.String(), "workspaces hook session-start: ") || !strings.Contains(f.stderr.String(), tt.msg) {
				t.Errorf("stderr = %q, want %q", f.stderr.String(), tt.msg)
			}
			if f.stdout.Len() != 0 {
				t.Errorf("stdout = %q", f.stdout.String())
			}
			if exists(f.sessionsFile()) {
				t.Error("a record was written")
			}
		})
	}
}

func TestHookUnwritableState(t *testing.T) {
	f := newFixture(t)
	blocker := filepath.Join(f.home, "not-a-dir")
	os.WriteFile(blocker, nil, 0o600)
	f.env["XDG_STATE_HOME"] = blocker
	if code := f.runHook("abc", hookPayload); code != exitOK {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(f.stderr.String(), "workspaces hook session-start: ") || f.stdout.Len() != 0 {
		t.Errorf("stderr %q, stdout %q", f.stderr.String(), f.stdout.String())
	}

	f = newFixture(t)
	delete(f.env, "HOME")
	delete(f.env, "XDG_STATE_HOME")
	if code := f.runHook("abc", hookPayload); code != exitOK || !strings.Contains(f.stderr.String(), "HOME") {
		t.Errorf("no HOME: exit %d, stderr %q", code, f.stderr.String())
	}
}

func TestHookReadOnlyStateDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	f := newFixture(t)
	dir := filepath.Join(f.home, ".local/state/workspaces")
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	if code := f.runHook("abc", hookPayload); code != exitOK || f.stderr.Len() == 0 {
		t.Errorf("exit %d, stderr %q", code, f.stderr.String())
	}
}

func TestHookArguments(t *testing.T) {
	f := newFixture(t)
	f.env["AGENT_LAUNCH_ID"] = "abc"
	if code := f.run("hook", "session-start", "extra"); code != exitUsage || exists(f.sessionsFile()) {
		t.Errorf("extra argument: exit %d", code)
	}
	f.tty = false
	if code := f.run("hook"); code != exitUsage {
		t.Errorf("no verb: exit %d", code)
	}
	if code := f.run("hook", "other"); code != exitUsage {
		t.Errorf("unknown verb: exit %d", code)
	}
	f.stderr.Reset()
	if code := f.run("hook", "-h"); code != exitOK || !strings.Contains(f.stderr.String(), "always exits 0") {
		t.Errorf("-h: exit %d, stderr %q", code, f.stderr.String())
	}
}
