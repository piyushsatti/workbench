package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"workspaces/internal/launchlog"
	"workspaces/internal/picker"
	"workspaces/internal/sessions"
)

var sessionBase = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// transcript writes a session under <config>/projects/<project>/ whose first
// record carries cwd, then the extra lines, and sets its mtime to sessionBase
// plus age. config is relative to HOME: ".claude" for the default harness.
func (f *fixture) transcript(config, project, id, cwd string, age time.Duration, extra ...string) string {
	f.t.Helper()
	dir := filepath.Join(f.home, config, "projects", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{"type": "user", "cwd": cwd, "sessionId": id})
	lines := append([]string{`{"type":"mode"}`, string(b)}, extra...)
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chtimes(path, sessionBase.Add(age), sessionBase.Add(age)); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func aiTitle(s string) string { return `{"type":"ai-title","aiTitle":` + jsonString(s) + `}` }

func customTitle(s string) string {
	return `{"type":"custom-title","customTitle":` + jsonString(s) + `}`
}

func lastPrompt(s string) string {
	return `{"type":"last-prompt","lastPrompt":` + jsonString(s) + `}`
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// squash folds each output line's whitespace, so tests do not depend on how
// tabwriter pads columns.
func squash(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		out = append(out, strings.Join(strings.Fields(line), " "))
	}
	return out
}

func (f *fixture) root() string { return filepath.Join(f.home, "Source/Projects/a") }

const (
	idA = "aaaaaaaa-1111-4111-8111-111111111111"
	idB = "bbbbbbbb-2222-4222-8222-222222222222"
	idC = "cccccccc-3333-4333-8333-333333333333"
	idD = "dddddddd-4444-4444-8444-444444444444"
)

func TestSessionListTable(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "proj-a", idA, f.root(), 1*time.Hour, aiTitle("Old work"))
	f.transcript(".claude", "proj-a", idB, f.root(), 3*time.Hour, aiTitle("AI one"), customTitle("My rename"))
	// A worktree under the root has a folder of its own.
	f.transcript(".claude", "proj-a-wt", idC, f.root()+"/.claude/worktrees/x", 2*time.Hour, lastPrompt("fix the thing"))
	// Another space's session, and one outside every space.
	f.transcript(".claude", "proj-b", idD, filepath.Join(f.home, "Source/Projects/b"), 9*time.Hour, aiTitle("Not mine"))
	if code := f.run("session", "list", "a"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	want := []string{
		"MODIFIED ID TITLE TMUX",
		"2026-10-01 15:00 bbbbbbbb My rename -",
		"2026-10-01 14:00 cccccccc fix the thing -",
		"2026-10-01 13:00 aaaaaaaa Old work -",
	}
	if got := squash(f.stdout.String()); !slices.Equal(got, want) {
		t.Errorf("output:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSessionListLimit(t *testing.T) {
	f := newFixture(t)
	for i := range 25 {
		f.transcript(".claude", "p", fmt.Sprintf("%08d-0000-4000-8000-000000000000", i), f.root(), time.Duration(i)*time.Minute, aiTitle(fmt.Sprint("t", i)))
	}
	for _, tt := range []struct {
		args []string
		rows int
		top  string
	}{
		{[]string{"session", "list", "a"}, 20, "00000024"},
		{[]string{"session", "list", "-n", "3", "a"}, 3, "00000024"},
		{[]string{"session", "list", "a", "-n", "1"}, 1, "00000024"},
		{[]string{"session", "list", "-n", "100", "a"}, 25, "00000024"},
	} {
		f.stdout.Reset()
		if code := f.run(tt.args...); code != exitOK {
			t.Fatalf("%q: exit %d", tt.args, code)
		}
		rows := squash(f.stdout.String())[1:]
		if len(rows) != tt.rows || !strings.Contains(rows[0], tt.top) {
			t.Errorf("%q: %d rows, first %q; want %d rows, first with %s", tt.args, len(rows), rows[0], tt.rows, tt.top)
		}
	}
	for _, n := range []string{"0", "-1"} {
		f.stderr.Reset()
		if code := f.run("session", "list", "-n", n, "a"); code != exitUsage {
			t.Errorf("-n %s: exit %d, want %d", n, code, exitUsage)
		}
	}
}

func TestSessionListJSON(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), time.Hour, aiTitle("with\x1b[31m escape"))
	f.transcript(".claude", "p", idB, f.root()+"/sub", 2*time.Hour, aiTitle("B"))
	if code := f.run("session", "list", "--json", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(f.stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %q", lines)
	}
	var first struct {
		ID, Modified, Title, Cwd string
		Tmux                     *launchlog.Tmux
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first.ID != idB || first.Modified != "2026-10-01T14:00:00.000Z" || first.Title != "B" || first.Cwd != f.root()+"/sub" || first.Tmux != nil {
		t.Errorf("first = %+v", first)
	}
	if !strings.Contains(lines[1], `"title":"with�[31m escape"`) {
		t.Errorf("control characters must not reach the output: %s", lines[1])
	}
}

func TestSessionListTitleCleaning(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), time.Hour, aiTitle("tab\there\x1b[2Jbidi‮end  spaced\nnewline"))
	f.transcript(".claude", "p", idB, f.root(), 2*time.Hour, aiTitle(strings.Repeat("w", 100)))
	if code := f.run("session", "list", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	out := f.stdout.String()
	if strings.ContainsAny(out, "\x1b‮") {
		t.Errorf("output holds control or format characters: %q", out)
	}
	rows := squash(out)
	if !strings.Contains(rows[2], "tab here�[2Jbidi�end spaced newline") {
		t.Errorf("cleaned row = %q", rows[2])
	}
	if want := strings.Repeat("w", 69) + "..."; !strings.Contains(rows[1], want) || strings.Contains(rows[1], "w"+want) {
		t.Errorf("long title row = %q, want the title cut to 72 characters", rows[1])
	}
}

func TestSessionListHarnessChoosesConfigDir(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), 0, aiTitle("default dir"))
	f.transcript(".claude-alt", "p", idB, f.root(), 0, aiTitle("alt dir"))
	f.transcript("custom-env", "p", idC, f.root(), 0, aiTitle("env dir"))
	for _, tt := range []struct {
		name string
		env  string
		args []string
		want string
	}{
		{"default harness, no variable", "", []string{"session", "list", "a"}, "default dir"},
		{"harness env wins", "", []string{"session", "list", "--harness", "claude-alt", "a"}, "alt dir"},
		{"explicit default harness", "", []string{"session", "list", "--harness", "claude", "a"}, "default dir"},
		{"environment variable when the harness has none", filepath.Join(f.home, "custom-env"), []string{"session", "list", "a"}, "env dir"},
		{"harness env beats the environment", filepath.Join(f.home, "custom-env"), []string{"session", "list", "--harness", "claude-alt", "a"}, "alt dir"},
		{"a relative variable is ignored", "custom-env", []string{"session", "list", "a"}, "default dir"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f.stdout.Reset()
			f.env["CLAUDE_CONFIG_DIR"] = tt.env
			if code := f.run(tt.args...); code != exitOK {
				t.Fatalf("exit %d; %s", code, f.stderr.String())
			}
			rows := squash(f.stdout.String())
			if len(rows) != 2 || !strings.Contains(rows[1], tt.want) {
				t.Errorf("rows = %q, want %q", rows, tt.want)
			}
		})
	}
}

func TestSessionRefusesOtherAdapters(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("harness.json", `{"default":"claude","harnesses":{"claude":{"adapter":"claude"},"codex":{"adapter":"codex"}}}`)
	for _, args := range [][]string{
		{"session", "list", "--harness", "codex", "a"},
		{"session", "resume", "--harness", "codex", "a", idA},
		{"session", "prune", "--harness", "codex", "--yes", "a"},
		{"session", "list", "--harness", "nope", "a"},
	} {
		f.stderr.Reset()
		if code := f.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
	}
	f.stderr.Reset()
	f.run("session", "list", "--harness", "codex", "a")
	if !strings.Contains(f.stderr.String(), "Claude sessions only") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
	f.writeConfig("harness.json", `{"default":"codex","harnesses":{"codex":{"adapter":"codex"}}}`)
	f.stderr.Reset()
	if code := f.run("session", "list", "a"); code != exitUsage || !strings.Contains(f.stderr.String(), "codex adapter") {
		t.Errorf("default harness is codex: exit %d, stderr %q", code, f.stderr.String())
	}
	if len(f.execs) != 0 || f.logExists() {
		t.Error("a refused session command launched or logged")
	}
}

func TestSessionSpaceResolution(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), 0, aiTitle("in a"))

	f.cwd = f.root() + "/sub"
	if code := f.run("session", "list"); code != exitOK || !strings.Contains(f.stdout.String(), "in a") {
		t.Errorf("cwd in a space: exit %d, out %q", code, f.stdout.String())
	}
	f.stdout.Reset()
	f.run("session", "list", ".")
	if !strings.Contains(f.stdout.String(), "in a") {
		t.Errorf(`"." out = %q`, f.stdout.String())
	}

	f.cwd = filepath.Join(f.home, "outside")
	f.tty = false
	if code := f.run("session", "list"); code != exitUsage {
		t.Errorf("no space and no TTY: exit %d, want %d", code, exitUsage)
	}
	f.tty = true
	f.pickReply = []string{"a  ~/Source/Projects/a"}
	f.stdout.Reset()
	if code := f.run("session", "list"); code != exitOK || !strings.Contains(f.stdout.String(), "in a") {
		t.Errorf("picker: exit %d, out %q", code, f.stdout.String())
	}
	if code := f.run("session", "list", "nope"); code != exitUsage {
		t.Errorf("unknown space: exit %d", code)
	}
	if code := f.run("session", "list", "a", "extra"); code != exitUsage {
		t.Errorf("extra argument: exit %d", code)
	}
}

func TestSessionListEmptyAndMissingConfigDir(t *testing.T) {
	f := newFixture(t)
	if code := f.run("session", "list", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if got := squash(f.stdout.String()); len(got) != 1 || got[0] != "MODIFIED ID TITLE TMUX" {
		t.Errorf("output = %q, want the header only", got)
	}
	if err := os.WriteFile(filepath.Join(f.home, ".claude-alt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Mkdir(filepath.Join(f.home, ".claude"), 0o755)
	os.WriteFile(filepath.Join(f.home, ".claude/projects"), nil, 0o644)
	if code := f.run("session", "list", "a"); code != exitIOErr {
		t.Errorf("unreadable projects dir: exit %d, want %d", code, exitIOErr)
	}
}

func TestSessionListTmuxJoin(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), 4*time.Hour, aiTitle("joined"))
	f.transcript(".claude", "p", idB, f.root(), 3*time.Hour, aiTitle("resumed twice"))
	f.transcript(".claude", "p", idC, f.root(), 2*time.Hour, aiTitle("refused launch"))
	f.transcript(".claude", "p", idD, f.root(), 1*time.Hour, aiTitle("no join"))
	idx3, idx5 := 3, 5
	logPath := filepath.Join(f.home, ".local/state/workspaces/launches.jsonl")
	for _, r := range []launchlog.Record{
		{ID: "L1", Decision: "allow", Tmux: &launchlog.Tmux{Session: "main", WindowIndex: &idx3, PaneID: "%30"}},
		{ID: "L2", Decision: "allow", Tmux: &launchlog.Tmux{Session: "old", WindowIndex: &idx3, PaneID: "%1"}},
		{ID: "L3", Decision: "allow", Tmux: &launchlog.Tmux{Session: "a", WindowIndex: &idx5, PaneID: "%44"}},
		{ID: "L4", Decision: "refuse", Tmux: &launchlog.Tmux{Session: "refused", PaneID: "%9"}},
		{ID: "L5", Decision: "allow"},
	} {
		if err := launchlog.Append(logPath, r); err != nil {
			t.Fatal(err)
		}
	}
	sessPath := filepath.Join(f.home, ".local/state/workspaces/sessions.jsonl")
	for _, r := range []launchlog.SessionRecord{
		{LaunchID: "L1", SessionID: idA},
		{LaunchID: "L2", SessionID: idB},
		{LaunchID: "L3", SessionID: idB},
		{LaunchID: "L4", SessionID: idC},
		{LaunchID: "L5", SessionID: idD},
		{LaunchID: "gone", SessionID: idD},
	} {
		if err := launchlog.AppendSession(sessPath, r); err != nil {
			t.Fatal(err)
		}
	}
	f.run("session", "list", "a")
	want := []string{
		"MODIFIED ID TITLE TMUX",
		"2026-10-01 13:00 dddddddd no join -",
		"2026-10-01 14:00 cccccccc refused launch -",
		"2026-10-01 15:00 bbbbbbbb resumed twice a:5 %44",
		"2026-10-01 16:00 aaaaaaaa joined main:3 %30",
	}
	want = []string{want[0], want[4], want[3], want[2], want[1]}
	if got := squash(f.stdout.String()); !slices.Equal(got, want) {
		t.Errorf("output:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	f.stdout.Reset()
	f.run("session", "list", "--json", "-n", "1", "a")
	if !strings.Contains(f.stdout.String(), `"tmux":{"socket":"","session_id":"","session":"main","window_id":"","window_index":3,"window":"","pane_id":"%30","entry":""}`) {
		t.Errorf("json = %s", f.stdout.String())
	}
}

func TestSessionListSurvivesBadJoinFiles(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), 0, aiTitle("t"))
	dir := filepath.Join(f.home, ".local/state/workspaces")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "sessions.jsonl"), []byte("garbage\n{\"launch_id\":\"x\",\"session_id\":\""+idA+"\"}\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "launches.jsonl"), []byte("garbage\n"), 0o600)
	if code := f.run("session", "list", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(f.stderr.String(), "skipped 1 malformed line(s) in sessions.jsonl") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
	if got := squash(f.stdout.String()); len(got) != 2 || !strings.HasSuffix(got[1], " t -") {
		t.Errorf("output = %q", got)
	}
	// A sessions.jsonl that cannot be read is reported and the list still shows.
	os.Remove(filepath.Join(dir, "sessions.jsonl"))
	os.Mkdir(filepath.Join(dir, "sessions.jsonl"), 0o700)
	f.stdout.Reset()
	f.stderr.Reset()
	if code := f.run("session", "list", "a"); code != exitOK || len(squash(f.stdout.String())) != 2 {
		t.Errorf("exit %d, out %q", code, f.stdout.String())
	}
	if !strings.Contains(f.stderr.String(), "reading sessions.jsonl") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

// launchView is what a launch leaves, minus what differs from call to call.
type launchView struct {
	path   string
	argv   []string
	env    []string
	chdirs []string
	record launchlog.Record
}

// text renders the view with home replaced, for comparing two fixtures.
func (v launchView) text(home string) string {
	b, err := json.Marshal([]any{v.path, v.argv, v.env, v.chdirs, v.record})
	if err != nil {
		panic(err)
	}
	return strings.ReplaceAll(string(b), home, "$HOME")
}

func (f *fixture) launchView() launchView {
	f.t.Helper()
	if len(f.execs) != 1 {
		f.t.Fatalf("exec calls = %d, want 1; stderr %s", len(f.execs), f.stderr.String())
	}
	e := f.execs[0]
	var env []string
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "AGENT_LAUNCH_ID=") {
			env = append(env, kv)
		}
	}
	rec := f.onlyRecord()
	rec.ID = ""
	return launchView{e.path, e.argv, env, f.chdirs, rec}
}

func TestSessionResumeIsRunResume(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		run  []string
	}{
		{"full id", []string{"session", "resume", "a", idB}, []string{"run", "a", "claude", "--resume", idB}},
		{"unique prefix", []string{"session", "resume", "a", "bbbb"}, []string{"run", "a", "claude", "--resume", idB}},
		{"harness flag", []string{"session", "resume", "--harness", "claude-alt", "a", idB}, []string{"run", "a", "claude-alt", "--resume", idB}},
		{"flags after the positionals", []string{"session", "resume", "a", idB, "--harness", "claude-alt"}, []string{"run", "a", "claude-alt", "--resume", idB}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setup := func() *fixture {
				f := newFixture(t)
				f.inTmux("999")
				for _, dir := range []string{".claude", ".claude-alt"} {
					f.transcript(dir, "p", idA, f.root(), 0, aiTitle("A"))
					f.transcript(dir, "p", idB, f.root(), time.Hour, aiTitle("B"))
				}
				return f
			}
			viaSession, viaRun := setup(), setup()
			if code := viaSession.run(tt.args...); code != exitOK {
				t.Fatalf("session resume: exit %d; stderr %s", code, viaSession.stderr.String())
			}
			if code := viaRun.run(tt.run...); code != exitOK {
				t.Fatalf("run: exit %d", code)
			}
			got, want := viaSession.launchView(), viaRun.launchView()
			// The two fixtures have different temporary homes.
			gotText := got.text(viaSession.home)
			wantText := want.text(viaRun.home)
			if gotText != wantText {
				t.Errorf("session resume:\n%s\nrun --resume:\n%s", gotText, wantText)
			}
			if got.argv[len(got.argv)-2] != "--resume" || got.argv[len(got.argv)-1] != idB {
				t.Errorf("argv = %q", got.argv)
			}
			if got.record.Decision != "allow" || !slices.Equal(got.record.Args, []string{"--resume", idB}) {
				t.Errorf("record = %+v", got.record)
			}
		})
	}
}

func TestSessionResumeRefusesIDsThatLookLikeOptions(t *testing.T) {
	for _, id := range []string{"-dangerously-skip-permissions", "--settings=x", "a b", "a;b"} {
		t.Run(id, func(t *testing.T) {
			f := newFixture(t)
			f.inTmux("999")
			f.transcript(".claude", "p", id, f.root(), 0, aiTitle("planted"))
			if code := f.run("session", "resume", "a", id); code != exitUsage {
				t.Fatalf("exit %d, want %d; stderr %s", code, exitUsage, f.stderr.String())
			}
			if len(f.execs) != 0 {
				t.Errorf("exec called with %q", f.execs[0].argv)
			}
		})
	}
}

func TestSessionResumePickerRefusesAPlantedOptionID(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.transcript(".claude", "p", "-dangerously-skip-permissions", f.root(), 0, aiTitle("planted"))
	f.pickReply = []string{"-dangero  2026-10-01 12:00  planted"}
	if code := f.run("session", "resume", "a"); code != exitUsage {
		t.Fatalf("exit %d, want %d; stderr %s", code, exitUsage, f.stderr.String())
	}
	if !strings.Contains(f.stderr.String(), "is not a session id") {
		t.Errorf("stderr = %s", f.stderr.String())
	}
	if len(f.execs) != 0 {
		t.Errorf("exec called with %q", f.execs[0].argv)
	}
}

func TestSessionResumeIsChecked(t *testing.T) {
	f := newFixture(t)
	f.inTmux("999")
	f.transcript(".claude", "p", idA, f.root(), 0, aiTitle("A"))
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source/Projects/b"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`)
	if code := f.run("session", "resume", "a", idA); code != exitRefused {
		t.Fatalf("exit %d, want %d", code, exitRefused)
	}
	if len(f.execs) != 0 {
		t.Error("exec called after a refusal")
	}
	if rec := f.onlyRecord(); rec.Decision != "refuse" || rec.Reason != "not_allowed" || !slices.Equal(rec.Args, []string{"--resume", idA}) {
		t.Errorf("record = %+v", rec)
	}
}

func TestSessionResumeBootstrapsOutsideTmux(t *testing.T) {
	f := newFixture(t)
	f.transcript(".claude", "p", idA, f.root(), 0, aiTitle("A"))
	f.tmuxReply = func(args []string) (string, error) { return "", nil }
	if code := f.run("session", "resume", "a", idA); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.tmuxCalls) < 2 || f.tmuxCalls[1][0] != "new-window" {
		t.Fatalf("tmux calls = %q", f.tmuxCalls)
	}
	cmd := strings.Join(f.tmuxCalls[1], " ")
	if !strings.Contains(cmd, "-- /opt/bin/workspaces run a claude --resume "+idA) {
		t.Errorf("new-window = %s", cmd)
	}
}

func TestSessionResumeArguments(t *testing.T) {
	setup := func() *fixture {
		f := newFixture(t)
		f.inTmux("999")
		f.transcript(".claude", "p", "abcd1111-0000-4000-8000-000000000000", f.root(), 2*time.Hour, aiTitle("first"))
		f.transcript(".claude", "p", "abcd2222-0000-4000-8000-000000000000", f.root(), time.Hour, aiTitle("second"))
		f.transcript(".claude", "p", "ffff0000-0000-4000-8000-000000000000", f.root(), 0, aiTitle("third"))
		return f
	}
	resumed := func(f *fixture) string {
		f.t.Helper()
		if len(f.execs) != 1 {
			f.t.Fatalf("exec calls = %d; stderr %s", len(f.execs), f.stderr.String())
		}
		argv := f.execs[0].argv
		return argv[len(argv)-1]
	}

	t.Run("one argument naming a space is the space, then a picker", func(t *testing.T) {
		f := setup()
		f.pickReply = []string{"ffff0000  2026-10-01 12:00  third"}
		if code := f.run("session", "resume", "a"); code != exitOK {
			t.Fatalf("exit %d; %s", code, f.stderr.String())
		}
		if got := resumed(f); got != "ffff0000-0000-4000-8000-000000000000" {
			t.Errorf("resumed %s", got)
		}
		if len(f.picks) != 1 || len(f.picks[0]) != 4 {
			t.Fatalf("picks = %q", f.picks)
		}
		wantItems := []string{"session", "abcd1111  2026-10-01 14:00  first", "abcd2222  2026-10-01 13:00  second", "ffff0000  2026-10-01 12:00  third"}
		if !slices.Equal(f.picks[0], wantItems) {
			t.Errorf("picker items = %q, want %q", f.picks[0], wantItems)
		}
	})
	t.Run("one argument that is not a space is the session", func(t *testing.T) {
		f := setup()
		f.cwd = f.root()
		if code := f.run("session", "resume", "ffff"); code != exitOK {
			t.Fatalf("exit %d; %s", code, f.stderr.String())
		}
		if got := resumed(f); got != "ffff0000-0000-4000-8000-000000000000" {
			t.Errorf("resumed %s", got)
		}
		if len(f.picks) != 0 {
			t.Errorf("picker opened: %q", f.picks)
		}
	})
	t.Run("a dot is the space", func(t *testing.T) {
		f := setup()
		f.cwd = f.root()
		f.pickReply = []string{"abcd1111  2026-10-01 14:00  first"}
		if code := f.run("session", "resume", "."); code != exitOK {
			t.Fatalf("exit %d; %s", code, f.stderr.String())
		}
		if got := resumed(f); got != "abcd1111-0000-4000-8000-000000000000" {
			t.Errorf("resumed %s", got)
		}
	})
	t.Run("no argument, cwd in the space", func(t *testing.T) {
		f := setup()
		f.cwd = f.root()
		f.pickReply = []string{"abcd2222  2026-10-01 13:00  second"}
		if code := f.run("session", "resume"); code != exitOK {
			t.Fatalf("exit %d; %s", code, f.stderr.String())
		}
		if got := resumed(f); got != "abcd2222-0000-4000-8000-000000000000" {
			t.Errorf("resumed %s", got)
		}
	})
	for _, tt := range []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"ambiguous prefix", []string{"session", "resume", "a", "abcd"}, exitUsage, "ambiguous"},
		{"no such session", []string{"session", "resume", "a", "0000"}, exitUsage, `no session "0000"`},
		{"an id from outside the space", []string{"session", "resume", "a", "../x"}, exitUsage, "no session"},
		{"a flag is not a session", []string{"session", "resume", "a", "--", "--help"}, exitUsage, "no session"},
		{"too many arguments", []string{"session", "resume", "a", "x", "y"}, exitUsage, "Usage"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := setup()
			if code := f.run(tt.args...); code != tt.code {
				t.Fatalf("exit %d, want %d", code, tt.code)
			}
			if !strings.Contains(f.stderr.String(), tt.msg) {
				t.Errorf("stderr = %q, want %q", f.stderr.String(), tt.msg)
			}
			if len(f.execs) != 0 || f.logExists() {
				t.Error("launched or logged")
			}
		})
	}
	t.Run("no session and no TTY", func(t *testing.T) {
		f := setup()
		f.tty = false
		if code := f.run("session", "resume", "a"); code != exitUsage || !strings.Contains(f.stderr.String(), "missing session; no TTY") {
			t.Errorf("exit %d, stderr %q", code, f.stderr.String())
		}
		if len(f.picks) != 0 {
			t.Error("picker opened without a TTY")
		}
	})
	t.Run("picker cancelled", func(t *testing.T) {
		f := setup()
		if code := f.run("session", "resume", "a"); code != exitCancelled {
			t.Errorf("exit %d, want %d", code, exitCancelled)
		}
		if f.logExists() || len(f.execs) != 0 {
			t.Error("a cancelled picker launched or logged")
		}
	})
	t.Run("no sessions", func(t *testing.T) {
		f := newFixture(t)
		if code := f.run("session", "resume", "a", idA); code != exitUsage || !strings.Contains(f.stderr.String(), "has no sessions") {
			t.Errorf("exit %d, stderr %q", code, f.stderr.String())
		}
	})
}

func TestSessionResumeWarnsWhenItRanElsewhere(t *testing.T) {
	for _, tt := range []struct {
		cwd  string
		warn bool
	}{
		{"", false},
		{"/.claude/worktrees/x", true},
		{"/sub", true},
	} {
		f := newFixture(t)
		f.inTmux("999")
		f.transcript(".claude", "p", idA, f.root()+tt.cwd, 0, aiTitle("A"))
		if code := f.run("session", "resume", "a", idA); code != exitOK {
			t.Fatalf("exit %d", code)
		}
		got := f.stderr.String()
		if tt.warn != strings.Contains(got, "warning: session aaaaaaaa ran in ~/Source/Projects/a"+tt.cwd) {
			t.Errorf("cwd %q: stderr = %q, want warning %v", tt.cwd, got, tt.warn)
		}
		if tt.warn && !strings.Contains(got, "resume starts in the space root ~/Source/Projects/a,") {
			t.Errorf("stderr = %q", got)
		}
	}
}

// pruneFixture holds three sessions of space a, one with a folder of its own.
func pruneFixture(t *testing.T) (*fixture, map[string]string) {
	f := newFixture(t)
	paths := map[string]string{
		idA: f.transcript(".claude", "p", idA, f.root(), 3*time.Hour, aiTitle("keep me")),
		idB: f.transcript(".claude", "p", idB, f.root(), 2*time.Hour, aiTitle("delete me")),
		idC: f.transcript(".claude", "p-wt", idC, f.root()+"/wt", time.Hour, aiTitle("delete me too")),
	}
	for _, id := range []string{idB, idC} {
		dir := filepath.Dir(paths[id])
		if err := os.MkdirAll(filepath.Join(dir, id, "subagents"), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, id, "subagents", "x.jsonl"), []byte("{}\n"), 0o644)
	}
	return f, paths
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestSessionPruneDeletesOnlyTheChosen(t *testing.T) {
	f, paths := pruneFixture(t)
	// Things under the config dir that are not sessions, and the unchosen one's folder.
	os.MkdirAll(filepath.Join(f.home, ".claude/projects/p/memory"), 0o755)
	keepers := []string{
		filepath.Join(f.home, ".claude/projects/p/memory/note.md"),
		filepath.Join(f.home, ".claude/settings.json"),
		filepath.Join(filepath.Dir(paths[idA]), idA, "tool-results.txt"),
		filepath.Join(f.home, ".claude/projects/p/other.txt"),
	}
	for _, k := range keepers {
		os.MkdirAll(filepath.Dir(k), 0o755)
		os.WriteFile(k, []byte("keep"), 0o644)
	}
	f.confirmReply = true
	f.pickManyReply = []string{
		"bbbbbbbb  2026-10-01 14:00  delete me",
		"cccccccc  2026-10-01 13:00  delete me too",
	}
	if code := f.run("session", "prune", "a"); code != exitOK {
		t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
	}
	if len(f.picks) != 1 || f.picks[0][0] != "sessions" || len(f.picks[0]) != 4 {
		t.Fatalf("picks = %q", f.picks)
	}
	if want := "aaaaaaaa  2026-10-01 15:00  keep me"; f.picks[0][1] != want {
		t.Errorf("first item = %q, want %q (newest first)", f.picks[0][1], want)
	}
	for _, id := range []string{idB, idC} {
		if exists(paths[id]) || exists(filepath.Join(filepath.Dir(paths[id]), id)) {
			t.Errorf("session %s was not deleted", id)
		}
	}
	if !exists(paths[idA]) {
		t.Error("the unchosen session was deleted")
	}
	for _, k := range keepers {
		if !exists(k) {
			t.Errorf("%s was deleted", k)
		}
	}
	want := []string{
		"will delete " + idB + " delete me",
		"will delete " + idC + " delete me too",
		"deleted " + idB + " and its folder",
		"deleted " + idC + " and its folder",
	}
	if got := squash(f.stdout.String()); !slices.Equal(got, want) {
		t.Errorf("output:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(f.confirms) != 1 || f.confirms[0] != "Delete 2 sessions?" {
		t.Errorf("confirms = %q", f.confirms)
	}
	if f.logExists() {
		t.Error("prune wrote the launch log")
	}
}

func TestSessionPruneWithoutAFolder(t *testing.T) {
	f, paths := pruneFixture(t)
	f.confirmReply = true
	f.pickManyReply = []string{"aaaaaaaa  2026-10-01 15:00  keep me"}
	if code := f.run("session", "prune", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if exists(paths[idA]) || !exists(paths[idB]) {
		t.Error("wrong files deleted")
	}
	if got := squash(f.stdout.String()); got[len(got)-1] != "deleted "+idA {
		t.Errorf("output = %q", got)
	}
	if len(f.confirms) != 1 || f.confirms[0] != "Delete 1 session?" {
		t.Errorf("confirms = %q", f.confirms)
	}
}

func TestSessionPruneDeclined(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(f *fixture)
		code int
	}{
		{"declined", func(f *fixture) { f.confirmReply = false }, exitCancelled},
		{"no gum", func(f *fixture) { f.confirmErr = picker.ErrNoConfirm }, exitUsage},
		{"confirm fails", func(f *fixture) { f.confirmErr = os.ErrPermission }, exitInternal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, paths := pruneFixture(t)
			f.pickManyReply = []string{"bbbbbbbb  2026-10-01 14:00  delete me"}
			tt.set(f)
			if code := f.run("session", "prune", "a"); code != tt.code {
				t.Fatalf("exit %d, want %d", code, tt.code)
			}
			for id, p := range paths {
				if !exists(p) {
					t.Errorf("%s was deleted", id)
				}
			}
			if strings.Contains(f.stdout.String(), "deleted") {
				t.Errorf("output = %q", f.stdout.String())
			}
		})
	}
}

func TestSessionPruneYesSkipsTheQuestion(t *testing.T) {
	f, paths := pruneFixture(t)
	f.pickManyReply = []string{"bbbbbbbb  2026-10-01 14:00  delete me"}
	if code := f.run("session", "prune", "--yes", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if len(f.confirms) != 0 {
		t.Errorf("asked %q despite --yes", f.confirms)
	}
	if exists(paths[idB]) || !exists(paths[idA]) {
		t.Error("wrong files deleted")
	}
	f2, _ := pruneFixture(t)
	f2.pickManyReply = []string{"bbbbbbbb  2026-10-01 14:00  delete me"}
	if code := f2.run("session", "prune", "a", "--yes"); code != exitOK || len(f2.confirms) != 0 {
		t.Errorf("--yes after the space: exit %d, confirms %q", code, f2.confirms)
	}
}

func TestSessionPruneWithoutATTY(t *testing.T) {
	for _, args := range [][]string{{"session", "prune", "a"}, {"session", "prune", "--yes", "a"}} {
		f, paths := pruneFixture(t)
		f.tty = false
		f.pickManyReply = []string{"bbbbbbbb  2026-10-01 14:00  delete me"}
		if code := f.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
		if len(f.picks) != 0 || len(f.confirms) != 0 {
			t.Errorf("%q: picker or confirm used without a TTY", args)
		}
		for id, p := range paths {
			if !exists(p) {
				t.Errorf("%q: %s was deleted", args, id)
			}
		}
	}
}

func TestSessionPrunePickerOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		err   error
		reply []string
		code  int
	}{
		{"cancelled", picker.ErrCancelled, nil, exitCancelled},
		{"nothing chosen", nil, nil, exitCancelled},
		{"no picker installed", picker.ErrNoPicker, nil, exitUsage},
		{"picker fails", os.ErrPermission, nil, exitInternal},
		{"lines that are not items", nil, []string{"made up", ""}, exitCancelled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, paths := pruneFixture(t)
			f.pickErr = tt.err
			f.pickManyReply = tt.reply
			f.confirmReply = true
			if code := f.run("session", "prune", "a"); code != tt.code {
				t.Fatalf("exit %d, want %d", code, tt.code)
			}
			for id, p := range paths {
				if !exists(p) {
					t.Errorf("%s was deleted", id)
				}
			}
			if len(f.confirms) != 0 {
				t.Error("asked for confirmation with nothing chosen")
			}
		})
	}
}

func TestSessionPruneNothingToPrune(t *testing.T) {
	f := newFixture(t)
	if code := f.run("session", "prune", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(f.stdout.String(), "no sessions to prune in space a") || len(f.picks) != 0 {
		t.Errorf("output %q, picks %q", f.stdout.String(), f.picks)
	}
}

func TestSessionPruneKeepsASymlinkedFolderTarget(t *testing.T) {
	f := newFixture(t)
	path := f.transcript(".claude", "p", idA, f.root(), 0, aiTitle("linked"))
	target := t.TempDir()
	os.WriteFile(filepath.Join(target, "precious"), []byte("x"), 0o644)
	if err := os.Symlink(target, filepath.Join(filepath.Dir(path), idA)); err != nil {
		t.Fatal(err)
	}
	f.confirmReply = true
	f.pickManyReply = []string{"aaaaaaaa  2026-10-01 12:00  linked"}
	if code := f.run("session", "prune", "a"); code != exitOK {
		t.Fatalf("exit %d; %s", code, f.stderr.String())
	}
	if exists(path) {
		t.Error("transcript not deleted")
	}
	if !exists(filepath.Join(target, "precious")) {
		t.Error("the symlink's target was deleted")
	}
	if strings.Contains(f.stdout.String(), "and its folder") {
		t.Errorf("output = %q", f.stdout.String())
	}
}

func TestSessionPruneDeleteFailureContinues(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can delete from a read-only folder")
	}
	f, paths := pruneFixture(t)
	f.confirmReply = true
	f.pickManyReply = []string{"bbbbbbbb  2026-10-01 14:00  delete me", "cccccccc  2026-10-01 13:00  delete me too"}
	locked := filepath.Dir(paths[idB])
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	if code := f.run("session", "prune", "a"); code != exitIOErr {
		t.Errorf("exit %d, want %d", code, exitIOErr)
	}
	if !exists(paths[idB]) {
		t.Error("the transcript in the read-only folder is gone")
	}
	if exists(paths[idC]) {
		t.Error("the second session was not deleted after the first failed")
	}
	if !strings.Contains(f.stdout.String(), "deleted "+idC) || strings.Contains(f.stdout.String(), "deleted "+idB) {
		t.Errorf("output = %q", f.stdout.String())
	}
}

func TestSessionPruneIdenticalLinesAreToldApart(t *testing.T) {
	f := newFixture(t)
	// Two sessions whose first eight characters, time and title all agree.
	p1 := f.transcript(".claude", "p", "aaaaaaaa-0000-4000-8000-000000000001", f.root(), 0, aiTitle("same"))
	p2 := f.transcript(".claude", "p", "aaaaaaaa-0000-4000-8000-000000000002", f.root(), 0, aiTitle("same"))
	f.confirmReply = true
	f.pickManyReply = []string{"aaaaaaaa  2026-10-01 12:00  same  #2"}
	if code := f.run("session", "prune", "a"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if exists(p1) == exists(p2) {
		t.Errorf("exactly one of the two should be gone: first %v, second %v", exists(p1), exists(p2))
	}
	if len(f.picks[0]) != 3 || f.picks[0][1] == f.picks[0][2] {
		t.Errorf("items = %q", f.picks[0])
	}
}

func TestSessionUsageAndNounPicker(t *testing.T) {
	f := newFixture(t)
	f.tty = false
	if code := f.run("session"); code != exitUsage || !strings.Contains(f.stderr.String(), "Claude sessions only") {
		t.Errorf("exit %d, stderr %q", code, f.stderr.String())
	}
	if code := f.run("session", "nope"); code != exitUsage {
		t.Errorf("unknown verb: exit %d", code)
	}
	f.stderr.Reset()
	if code := f.run("session", "-h"); code != exitOK || !strings.Contains(f.stderr.String(), "session prune") {
		t.Errorf("-h: exit %d", code)
	}
	for _, args := range [][]string{{"session", "list", "-h"}, {"session", "resume", "-h"}, {"session", "prune", "-h"}} {
		if code := f.run(args...); code != exitOK {
			t.Errorf("%q: exit %d", args, code)
		}
	}
	for _, args := range [][]string{{"session", "list", "--bogus"}, {"session", "resume", "--bogus"}, {"session", "prune", "--bogus"}} {
		if code := f.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d", args, code)
		}
	}
	if code := f.run("session", "prune", "a", "b"); code != exitUsage {
		t.Errorf("prune with two spaces: exit %d", code)
	}
	// With a terminal the noun picker offers the new nouns and not hook.
	f.tty = true
	f.pickReply = []string{"session", "list"}
	f.run()
	if got := f.picks[0]; !slices.Contains(got, "session") || !slices.Contains(got, "doctor") || slices.Contains(got, "hook") {
		t.Errorf("noun picker = %q", got)
	}
}

func TestSessionConfigErrors(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("harness.json", `{"default":"x"}`)
	if code := f.run("session", "list", "a"); code != exitConfig {
		t.Errorf("bad harness.json: exit %d, want %d", code, exitConfig)
	}
	f.writeConfig("harness.json", testHarnesses)
	os.Remove(filepath.Join(f.home, ".config/workspaces/spaces.json"))
	if code := f.run("session", "list", "a"); code != exitConfig {
		t.Errorf("no spaces.json: exit %d, want %d", code, exitConfig)
	}
}

func TestDeleteSessionRefusesANameThatIsNotAnID(t *testing.T) {
	projects := filepath.Join(t.TempDir(), "projects")
	project := filepath.Join(projects, "p")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(projects, "other", "keep.jsonl")
	os.MkdirAll(filepath.Dir(other), 0o755)
	os.WriteFile(other, []byte("{}"), 0o644)
	path := filepath.Join(project, "...jsonl") // a transcript named ...jsonl reads as id ".."
	os.WriteFile(path, []byte("{}"), 0o644)
	if _, err := deleteSession(sessions.Session{ID: "..", Path: path}); err == nil {
		t.Error("deleteSession accepted id \"..\"")
	}
	if !exists(other) || !exists(path) {
		t.Error("deleteSession removed files for id \"..\"")
	}
}
