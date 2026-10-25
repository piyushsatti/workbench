package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"workspaces/internal/launchlog"
)

func TestLaunchList(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	for _, args := range [][]string{
		{"run", ".", "claude", "--resume", strings.Repeat("x", 60)},
		{"run", "a", "claude-alt"},
	} {
		if code := f.run(args...); code != exitOK {
			t.Fatalf("%q: exit %d; stderr %s", args, code, f.stderr.String())
		}
	}
	f.cwd = filepath.Join(f.home, "outside")
	f.run("run", ".", "claude")

	logPath := filepath.Join(f.home, ".local/state/workspaces/launches.jsonl")
	fh, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fh.WriteString("garbage\n")
	fh.Close()

	f.stdout.Reset()
	f.stderr.Reset()
	if code := f.run("launch", "list"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(f.stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), f.stdout.String())
	}
	first := strings.Fields(lines[0])
	if want := []string{"2026-10-04", "22:31:05", "allow", "allow_launch", "-", "claude", "~/Source"}; strings.Join(first[:7], " ") != strings.Join(want, " ") {
		t.Errorf("first line = %q", lines[0])
	}
	if !strings.HasSuffix(lines[0], "...") {
		t.Errorf("long args not cut: %q", lines[0])
	}
	if !strings.Contains(lines[1], "a  ") || !strings.Contains(lines[1], "~/Source/Projects/a") {
		t.Errorf("second line = %q", lines[1])
	}
	if !strings.Contains(lines[2], "refuse") || !strings.Contains(lines[2], "not_allowed") {
		t.Errorf("newest record should be last: %q", lines[2])
	}
	if !strings.Contains(f.stderr.String(), "skipped 1 malformed line") {
		t.Errorf("stderr = %q, want the malformed count", f.stderr.String())
	}

	f.stdout.Reset()
	if code := f.run("launch", "list", "-n", "1", "--json"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	out := strings.TrimRight(f.stdout.String(), "\n")
	if strings.Count(out, "\n") != 0 || !strings.HasPrefix(out, `{"ts":`) || !strings.Contains(out, `"not_allowed"`) {
		t.Errorf("--json -n 1 = %q, want the newest raw line", out)
	}
}

func TestLaunchListDefaultsToTwenty(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	for i := range 25 {
		if code := f.run("run", ".", "claude", strconv.Itoa(i)); code != exitOK {
			t.Fatalf("exit %d", code)
		}
	}
	f.stdout.Reset()
	if code := f.run("launch", "list"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(f.stdout.String(), "\n"), "\n")
	if len(lines) != 20 {
		t.Fatalf("got %d lines, want 20", len(lines))
	}
	if !strings.HasSuffix(lines[0], " 5") || !strings.HasSuffix(lines[19], " 24") {
		t.Errorf("first %q, last %q; want records 5 to 24", lines[0], lines[19])
	}
}

func TestLaunchListReadErrorExits74(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.home, ".local/state/workspaces/launches.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code := f.run("launch", "list"); code != exitIOErr {
		t.Fatalf("exit %d, want %d; stderr %s", code, exitIOErr, f.stderr.String())
	}
}

func TestLaunchListWithoutHomeExits78(t *testing.T) {
	f := newFixture(t)
	delete(f.env, "HOME")
	delete(f.env, "XDG_STATE_HOME")
	if code := f.run("launch", "list"); code != exitConfig {
		t.Fatalf("exit %d, want %d", code, exitConfig)
	}
}

func TestLaunchVerbPicking(t *testing.T) {
	f := newFixture(t)
	if code := f.run("launch"); code != exitOK {
		t.Errorf("with a TTY the only verb is picked automatically; exit %d", code)
	}
	if len(f.picks) != 0 {
		t.Error("picker opened for a single verb")
	}
	f.tty = false
	if code := f.run("launch"); code != exitUsage {
		t.Errorf("without a TTY: exit %d, want %d", code, exitUsage)
	}
}

func (f *fixture) logFile() string {
	return filepath.Join(f.home, ".local/state/workspaces/launches.jsonl")
}

func (f *fixture) appendRecord(r launchlog.Record) {
	f.t.Helper()
	if err := launchlog.Append(f.logFile(), r); err != nil {
		f.t.Fatal(err)
	}
}

// listArgsColumn returns the args column of the only line "launch list" prints.
func listArgsColumn(t *testing.T, f *fixture) string {
	t.Helper()
	f.stdout.Reset()
	if code := f.run("launch", "list"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	out := strings.TrimRight(f.stdout.String(), "\n")
	if !utf8.ValidString(out) {
		t.Fatalf("output is not valid UTF-8: %q", out)
	}
	fields := strings.Fields(out)
	return fields[len(fields)-1]
}

func TestLaunchListRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{{"launch", "list", "-n", "-1"}, {"launch", "list", "extra"}} {
		f := newFixture(t)
		if code := f.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
		if !strings.Contains(f.stderr.String(), "Usage: workspaces launch list") {
			t.Errorf("%q: stderr = %q, want usage", args, f.stderr.String())
		}
	}
}

func TestLaunchListJSONEchoesRawLine(t *testing.T) {
	f := newFixture(t)
	raw := `{"zz_future":[1,2],"decision":"allow","ts":"2026-10-04T22:31:05.000Z","args":[]}`
	if err := os.MkdirAll(filepath.Dir(f.logFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.logFile(), []byte(raw+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := f.run("launch", "list", "--json"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if got := f.stdout.String(); got != raw+"\n" {
		t.Errorf("--json = %q, want %q", got, raw+"\n")
	}
}

func TestLaunchListCutsArgsToForty(t *testing.T) {
	f := newFixture(t)
	f.appendRecord(launchlog.Record{TS: "2026-10-04T22:31:05.000Z", Args: []string{strings.Repeat("x", 50)}})
	col := listArgsColumn(t, f)
	if want := strings.Repeat("x", 37) + "..."; col != want {
		t.Errorf("args column = %q, want %q", col, want)
	}
}

func TestLaunchListCutsArgsByRune(t *testing.T) {
	f := newFixture(t)
	f.appendRecord(launchlog.Record{TS: "2026-10-04T22:31:05.000Z", Args: []string{strings.Repeat("é", 45)}})
	col := listArgsColumn(t, f)
	// shellQuote wraps non-ASCII in single quotes, so the opening quote takes one of the 40 runes.
	if want := "'" + strings.Repeat("é", 36) + "..."; col != want {
		t.Errorf("args column = %q, want %q", col, want)
	}
}

func TestLaunchListShowsUnparsableTimeAsIs(t *testing.T) {
	f := newFixture(t)
	f.appendRecord(launchlog.Record{TS: "garbage", Decision: "allow"})
	f.run("launch", "list")
	if got := f.stdout.String(); !strings.HasPrefix(got, "garbage ") {
		t.Errorf("line = %q, want it to start with the raw ts", got)
	}
}
