package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"workspaces/internal/launchlog"
	"workspaces/internal/picker"
)

// Record shapes the views must handle in TODO.jsonl and LOGS.jsonl: a _meta
// header, major_category with tag, domain with category, tag as a list, ts in
// place of date, null fields, and fields no view knows.
const (
	metaLine     = `{"_meta":true,"schema_version":1,"purpose":"p","taxonomy":{"x":["y"]},"fields":{"title":"str"}}`
	todoMajor    = `{"major_category":"infra","tag":"cli","status":"open","title":"Fix the loader","body":"b","blocked_by":null,"section_title":null,"section_date":"2026-10-01"}`
	todoMajor2   = `{"major_category":"docs","tag":"readme","status":"planned","title":"Write the readme","body":"b","blocked_by":"x"}`
	todoDomain   = `{"domain":"ops","category":"backup","status":"blocked","title":"Rotate keys","id":"T-7"}`
	todoCategory = `{"category":"misc","status":"done","title":"Only a category"}`
	todoBare     = `{"status":null,"title":"No group at all"}`
	logDated     = `{"date":"2026-10-01","kind":"completion","tag":"cli","title":"Shipped the loader","status":"done","body":"b"}`
	logTS        = `{"ts":"2026-10-02T01:00:00+09:30","kind":"finding","tag":["infra","net"],"text":"t","title":"Found a leak"}`
	logDomain    = `{"date":"2026-10-03","domain":"ops","origin":{"session":"s"},"status":"open","title":"Domain record"}`
	logNull      = `{"date":"2026-10-04","kind":"ruling","tag":"policy","title":"Null status","status":null}`
)

func (f *fixture) writeFile(rel string, lines ...string) string {
	f.t.Helper()
	path := filepath.Join(f.home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func (f *fixture) runOK(args ...string) string {
	f.t.Helper()
	f.stdout.Reset()
	f.stderr.Reset()
	if code := f.run(args...); code != exitOK {
		f.t.Fatalf("%q: exit %d; stderr %s", args, code, f.stderr.String())
	}
	return f.stdout.String()
}

func outLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func spaceName(s string) *string { return &s }

const twoSpaces = `{"allow_launch":["~/Source"],"deny_launch":["~/Source/private/*"],"deny_access":["~/Source/Projects/_archive"],
 "spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b","~/Source/private/drafts"],"deny_access":["~/Source/Projects/a/secrets"]},
  "b":{"root":"~/Source/Projects/b","todo":"notes/todo.jsonl","log":"~/b-logs.jsonl"},
  "c":{"root":"~/Source/Projects/_archive/old"}}}`

func TestSpaceListRows(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", twoSpaces)
	f.writeFile("Source/Projects/a/TODO.jsonl", metaLine, todoMajor, todoDomain, "not json")
	f.writeFile("Source/Projects/a/LOGS.jsonl", metaLine, logDated)
	f.writeFile("Source/Projects/b/notes/todo.jsonl", todoMajor)
	f.writeFile("b-logs.jsonl", logDated, logTS, logNull)
	f.writeFile("Source/Projects/_archive/old/TODO.md", "# todo")
	f.appendRecord(launchlog.Record{TS: "2026-10-01T10:00:00.000Z", Space: spaceName("a"), Decision: "allow"})
	f.appendRecord(launchlog.Record{TS: "2026-10-02T11:00:00.000Z", Space: spaceName("a"), Decision: "refuse"})
	f.appendRecord(launchlog.Record{TS: "2026-10-03T12:00:00.000Z", Space: spaceName("b"), Decision: "allow"})
	f.appendRecord(launchlog.Record{TS: "2026-10-04T13:00:00.000Z", Decision: "allow"})

	lines := outLines(f.runOK("space", "list"))
	want := [][]string{
		{"SPACE", "ROOT", "ATTACH", "TODO", "LOG", "LAST", "LAUNCH"},
		{"a", "~/Source/Projects/a", "2", "2", "items", "1", "record", "2026-10-02", "11:00:00"},
		{"b", "~/Source/Projects/b", "0", "1", "item", "3", "records", "2026-10-03", "12:00:00"},
		{"c", "~/Source/Projects/_archive/old", "0", "TODO.md", "only", "none", "never"},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), f.stdout.String())
	}
	for i, w := range want {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != strings.Join(w, " ") {
			t.Errorf("line %d = %q, want %q", i, got, strings.Join(w, " "))
		}
	}
	if !strings.Contains(f.stderr.String(), "Source/Projects/a/TODO.jsonl:4:") {
		t.Errorf("stderr = %q, want the bad line reported with its number", f.stderr.String())
	}
}

func TestSpaceListTodoNoneWhenNoFileAtAll(t *testing.T) {
	f := newFixture(t)
	lines := outLines(f.runOK("space", "list"))
	if got := strings.Fields(lines[1]); strings.Join(got, " ") != "a ~/Source/Projects/a 1 none none never" {
		t.Errorf("row = %q", lines[1])
	}
}

func TestSpaceListJSONLPresentIsNotMigratedEvenWithTodoMD(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.md", "# old")
	f.writeFile("Source/Projects/a/TODO.jsonl", todoMajor)
	if out := f.runOK("space", "list"); !strings.Contains(out, " 1 item ") {
		t.Errorf("output = %q, want the jsonl counted", out)
	}
}

func TestSpaceListUnreadableTodoExits74(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.home, "Source/Projects/a/TODO.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := f.run("space", "list"); code != exitIOErr {
		t.Fatalf("exit %d, want %d", code, exitIOErr)
	}
	if !strings.Contains(f.stdout.String(), "unreadable") {
		t.Errorf("stdout = %q, want the row still printed as unreadable", f.stdout.String())
	}
}

func TestViewsConfigMissingExits78(t *testing.T) {
	for _, args := range [][]string{{"space", "list"}, {"space", "show", "a"}, {"todo", "list", "a"}, {"log", "list", "a"}} {
		f := newFixture(t)
		os.Remove(filepath.Join(f.home, ".config/workspaces/spaces.json"))
		if code := f.run(args...); code != exitConfig {
			t.Errorf("%q: exit %d, want %d", args, code, exitConfig)
		}
	}
}

func TestViewsRejectExtraArguments(t *testing.T) {
	for _, args := range [][]string{
		{"space", "list", "a"}, {"space", "show", "a", "b"}, {"todo", "list", "a", "b"},
		{"log", "list", "a", "b"}, {"log", "list", "-n", "-1", "a"},
	} {
		f := newFixture(t)
		if code := f.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
		if !strings.Contains(f.stderr.String(), "Usage: workspaces "+args[0]) {
			t.Errorf("%q: stderr = %q, want usage", args, f.stderr.String())
		}
	}
}

func TestViewsNeverWriteTheLaunchLog(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", todoMajor)
	f.writeFile("Source/Projects/a/LOGS.jsonl", logDated)
	for _, args := range [][]string{{"space", "list"}, {"space", "show", "a"}, {"todo", "list", "a"}, {"log", "list", "a"}} {
		f.runOK(args...)
	}
	if f.logExists() || len(f.execs) != 0 || len(f.tmuxCalls) != 0 {
		t.Errorf("a view wrote the log (%v), exec'd (%d) or called tmux (%d)", f.logExists(), len(f.execs), len(f.tmuxCalls))
	}
}

func showRows(t *testing.T, out, key string) []string {
	t.Helper()
	var rows []string
	for _, l := range outLines(out) {
		if k, v, ok := strings.Cut(l, "  "); ok && k == key {
			rows = append(rows, strings.TrimSpace(v))
		}
	}
	return rows
}

func TestSpaceShowPathsAndStates(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", twoSpaces)
	f.writeFile("Source/Projects/b/notes/todo.jsonl", todoMajor, todoMajor2)
	out := f.runOK("space", "show", "b")
	for key, want := range map[string]string{
		"space":                "b",
		"root":                 "~/Source/Projects/b",
		"attach":               "-",
		"deny_access (space)":  "-",
		"deny_access (global)": "~/Source/Projects/_archive",
		"todo":                 "~/Source/Projects/b/notes/todo.jsonl (2 items)",
		"log":                  "~/b-logs.jsonl (none)",
		"launch":               "none",
	} {
		if got := showRows(t, out, key); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %q, want %q\n%s", key, got, want, out)
		}
	}

	out = f.runOK("space", "show", "a")
	if got := showRows(t, out, "attach"); strings.Join(got, "|") != "~/Source/Projects/b|~/Source/private/drafts" {
		t.Errorf("attach rows = %q", got)
	}
	if got := showRows(t, out, "deny_access (space)"); strings.Join(got, "|") != "~/Source/Projects/a/secrets" {
		t.Errorf("deny_access (space) rows = %q", got)
	}
	if got := showRows(t, out, "deny_access (global)"); strings.Join(got, "|") != "~/Source/Projects/_archive" {
		t.Errorf("deny_access (global) rows = %q", got)
	}
}

func TestSpaceShowCheckResults(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_launch":["~/Source/private/*"],"deny_access":["~/Source/Projects/_archive"],
	 "spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b","~/Source/private/drafts","/etc","~/Source/Projects/_archive/old"],"deny_access":["~/Source/Projects/b"]},
	  "k":{"root":"~/Source/private/drafts"}}}`)
	out := f.runOK("space", "show", "a")
	if got := showRows(t, out, "check root"); len(got) != 1 || got[0] != "allow allow_launch ~/Source" {
		t.Errorf("check root = %q, want the root's own check, attach paths apart", got)
	}
	want := []string{
		"~/Source/Projects/b: attach_denied ~/Source/Projects/b",
		"~/Source/private/drafts: ok",
		"/etc: ok",
		"~/Source/Projects/_archive/old: attach_denied ~/Source/Projects/_archive",
	}
	if got := showRows(t, out, "check attach"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("check attach = %q, want %q", got, want)
	}

	if got := showRows(t, f.runOK("space", "show", "k"), "check root"); len(got) != 1 || got[0] != "refuse deny_launch ~/Source/private/*" {
		t.Errorf("check root of k = %q", got)
	}
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source/Projects/b"],"deny_access":["~/Source/Projects/b"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b"]}}}`)
	if got := showRows(t, f.runOK("space", "show", "a"), "check root"); len(got) != 1 || got[0] != "refuse not_allowed" {
		t.Errorf("check root of a refused root with a denied attach path = %q", got)
	}
	f.writeConfig("spaces.json", testSpaces)
	if got := showRows(t, f.runOK("space", "show", "a"), "check root"); len(got) != 1 || got[0] != "allow allow_launch ~/Source" {
		t.Errorf("check root of allowed a = %q", got)
	}
}

func TestSpaceShowLastFiveLaunchesOfThatSpace(t *testing.T) {
	f := newFixture(t)
	for i := range 7 {
		f.appendRecord(launchlog.Record{TS: "2026-10-04T22:31:05.000Z", Space: spaceName("a"), Decision: "allow", Reason: "allow_launch", Harness: "claude", Args: []string{"n" + strconv.Itoa(i)}})
		f.appendRecord(launchlog.Record{TS: "2026-10-04T22:31:05.000Z", Space: spaceName("other"), Args: []string{"x"}})
		f.appendRecord(launchlog.Record{TS: "2026-10-04T22:31:05.000Z", Args: []string{"y"}})
	}
	rows := showRows(t, f.runOK("space", "show", "a"), "launch")
	if len(rows) != 5 {
		t.Fatalf("launch rows = %d, want 5:\n%s", len(rows), f.stdout.String())
	}
	for i, r := range rows {
		fields := strings.Fields(r)
		if want := "n" + strconv.Itoa(i+2); fields[len(fields)-1] != want || fields[4] != "a" {
			t.Errorf("row %d = %q, want space a, args %s (newest last)", i, r, want)
		}
	}
}

func TestViewsUseTheSpaceHoldingTheCwd(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a"},"inner":{"root":"~/Source/Projects/a/sub"}}}`)
	f.writeFile("Source/Projects/a/sub/TODO.jsonl", todoDomain)
	f.writeFile("Source/Projects/a/sub/LOGS.jsonl", logDomain)
	f.cwd = filepath.Join(f.home, "Source/Projects/a/sub")
	f.tty = false
	if got := showRows(t, f.runOK("space", "show"), "space"); len(got) != 1 || got[0] != "inner" {
		t.Errorf("space show = %q, want the longest root holding the cwd", got)
	}
	if got := showRows(t, f.runOK("space", "show", "."), "space"); len(got) != 1 || got[0] != "inner" {
		t.Errorf("space show . = %q", got)
	}
	if out := f.runOK("todo", "list"); !strings.Contains(out, "Rotate keys") {
		t.Errorf("todo list = %q", out)
	}
	if out := f.runOK("log", "list"); !strings.Contains(out, "Domain record") {
		t.Errorf("log list = %q", out)
	}
	if len(f.picks) != 0 {
		t.Error("picker opened although a space holds the cwd")
	}
}

func TestViewsSelectSpaceByPhysicalCwd(t *testing.T) {
	f := newFixture(t)
	link := filepath.Join(f.home, "link")
	if err := os.Symlink(filepath.Join(f.home, "Source/Projects/a"), link); err != nil {
		t.Fatal(err)
	}
	f.cwd = link
	f.tty = false
	if got := showRows(t, f.runOK("space", "show"), "space"); len(got) != 1 || got[0] != "a" {
		t.Errorf("space = %q, want a through the symlink", got)
	}
}

func TestViewsPickSpaceOnTTY(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"space", "show"}, "~/Source/Projects/b/notes/todo.jsonl"},
		{[]string{"todo", "list"}, "Only a category"},
		{[]string{"log", "list"}, "Null status"},
	} {
		args := tt.args
		f := newFixture(t)
		f.writeConfig("spaces.json", twoSpaces)
		f.writeFile("Source/Projects/b/notes/todo.jsonl", todoCategory)
		f.writeFile("b-logs.jsonl", logNull)
		f.pickReply = []string{"b  ~/Source/Projects/b"}
		out := f.runOK(args...)
		if len(f.picks) != 1 {
			t.Fatalf("%q: picks = %q, want one", args, f.picks)
		}
		want := "space|a  ~/Source/Projects/a|b  ~/Source/Projects/b|c  ~/Source/Projects/_archive/old"
		if got := strings.Join(f.picks[0], "|"); got != want {
			t.Errorf("%q: picker = %s, want %s", args, got, want)
		}
		if !strings.Contains(out, tt.want) {
			t.Errorf("%q: output %q is not space b's", args, out)
		}
	}
}

func TestViewsMissingSpaceWithoutTTYExits64(t *testing.T) {
	for _, args := range [][]string{{"space", "show"}, {"todo", "list"}, {"log", "list"}} {
		f := newFixture(t)
		f.tty = false
		if code := f.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
		if len(f.picks) != 0 {
			t.Errorf("%q: picker opened without a TTY", args)
		}
		if !strings.Contains(f.stderr.String(), "missing space") {
			t.Errorf("%q: stderr = %q", args, f.stderr.String())
		}
	}
}

func TestViewsSpaceResolutionFailures(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		spaces string
		pick   error
		code   int
		msg    string
	}{
		{"unknown name", []string{"todo", "list", "nope"}, testSpaces, nil, exitUsage, `unknown space "nope"`},
		{"dot outside every space", []string{"space", "show", "."}, testSpaces, nil, exitUsage, "no space holds"},
		{"no spaces configured", []string{"log", "list"}, `{"allow_launch":["~/Source"]}`, nil, exitUsage, "no spaces configured"},
		{"picker cancelled", []string{"space", "show"}, testSpaces, picker.ErrCancelled, exitCancelled, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.writeConfig("spaces.json", tt.spaces)
			f.pickErr = tt.pick
			if code := f.run(tt.args...); code != tt.code {
				t.Fatalf("exit %d, want %d; stderr %s", code, tt.code, f.stderr.String())
			}
			if !strings.Contains(f.stderr.String(), tt.msg) {
				t.Errorf("stderr = %q, want %q", f.stderr.String(), tt.msg)
			}
			if f.stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", f.stdout.String())
			}
		})
	}
}

func TestTodoListGroupsByFirstPresentKey(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl",
		metaLine,
		todoMajor,
		todoDomain,
		todoMajor2,
		todoCategory,
		todoBare,
		`{"major_category":"infra","status":"open","title":"Second infra"}`,
		`{"domain":"ops","major_category":"infra","title":"Domain beats major_category"}`,
		`{"domain":null,"major_category":"","category":"misc","title":"Null domain falls through"}`,
	)
	out := f.runOK("todo", "list", "a")
	want := []string{
		"infra",
		"#2 open [cli] Fix the loader",
		"#7 open Second infra",
		"ops",
		"T-7 blocked Rotate keys",
		"#8 - Domain beats major_category",
		"docs",
		"#4 planned [readme] Write the readme",
		"misc",
		"#5 done Only a category",
		"#9 - Null domain falls through",
		"-",
		"#6 - No group at all",
	}
	got := outLines(out)
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i, w := range want {
		if g := strings.Join(strings.Fields(got[i]), " "); g != w {
			t.Errorf("line %d = %q, want %q", i, g, w)
		}
	}
	if !strings.HasPrefix(got[1], "  #2") {
		t.Errorf("item line %q is not indented under its group", got[1])
	}
	if f.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", f.stderr.String())
	}
}

func TestTodoListTagAsListAndFlattenedTitle(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", `{"domain":"d","tag":["x","y"],"status":"open","title":"two\nlines\tand tab"}`)
	got := outLines(f.runOK("todo", "list", "a"))
	if len(got) != 2 || strings.Join(strings.Fields(got[1]), " ") != "#1 open [x,y] two lines and tab" {
		t.Errorf("output = %q", got)
	}
}

func TestTodoListReportsBadLinesAndListsTheRest(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", metaLine, `{"title": oops`, todoMajor, `[1]`, todoDomain)
	out := f.runOK("todo", "list", "a")
	if !strings.Contains(out, "Fix the loader") || !strings.Contains(out, "Rotate keys") {
		t.Errorf("stdout = %q, want the good records listed", out)
	}
	errs := f.stderr.String()
	for _, want := range []string{"~/Source/Projects/a/TODO.jsonl:2:", "~/Source/Projects/a/TODO.jsonl:4:"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr = %q, want %q", errs, want)
		}
	}
	if strings.Count(errs, "skipped") != 2 {
		t.Errorf("stderr = %q, want two skipped lines", errs)
	}
}

func TestTodoListJSONPrintsRawLines(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", metaLine, todoMajor, "bad", todoDomain)
	if got := f.runOK("todo", "list", "--json", "a"); got != todoMajor+"\n"+todoDomain+"\n" {
		t.Errorf("--json = %q, want the raw record lines without _meta", got)
	}
}

func TestTodoListMissingFile(t *testing.T) {
	f := newFixture(t)
	if out := f.runOK("todo", "list", "a"); out != "" {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(f.stderr.String(), "space a has no file at ~/Source/Projects/a/TODO.jsonl") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestTodoListNeverParsesTodoMD(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.md", `{"domain":"d","title":"looks like a record"}`)
	if out := f.runOK("todo", "list", "a"); out != "" {
		t.Errorf("stdout = %q, want TODO.md left unparsed", out)
	}
	if !strings.Contains(f.stderr.String(), "space a has only TODO.md") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestTodoListReadErrorExits74(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.home, "Source/Projects/a/TODO.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := f.run("todo", "list", "a"); code != exitIOErr {
		t.Fatalf("exit %d, want %d", code, exitIOErr)
	}
	if !strings.Contains(f.stderr.String(), "reading ") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestLogListColumns(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/LOGS.jsonl", metaLine, logDated, logTS, logDomain, logNull,
		`{"date":"2026-10-05","kind":"event","major_category":"infra","title":"Major category record","status":"done"}`,
		`{"title":"Bare"}`)
	want := []string{
		"2026-10-01 completion - done [cli] Shipped the loader",
		"2026-10-02 finding - - [infra,net] Found a leak",
		"2026-10-03 - ops open Domain record",
		"2026-10-04 ruling - - [policy] Null status",
		"2026-10-05 event infra done Major category record",
		"- - - - Bare",
	}
	got := outLines(f.runOK("log", "list", "a"))
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), f.stdout.String())
	}
	for i, w := range want {
		if g := strings.Join(strings.Fields(got[i]), " "); g != w {
			t.Errorf("line %d = %q, want %q", i, g, w)
		}
	}
}

func TestLogListUnparsableTSShownAsIs(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/LOGS.jsonl", `{"ts":"last tuesday","title":"t"}`)
	if got := f.runOK("log", "list", "a"); !strings.HasPrefix(got, "last tuesday ") {
		t.Errorf("line = %q", got)
	}
}

func TestLogListCountAndJSON(t *testing.T) {
	f := newFixture(t)
	lines := []string{metaLine}
	for i := range 25 {
		lines = append(lines, `{"date":"2026-10-01","kind":"k","title":"r`+strconv.Itoa(i)+`"}`)
	}
	f.writeFile("Source/Projects/a/LOGS.jsonl", lines...)

	got := outLines(f.runOK("log", "list", "a"))
	if len(got) != 20 || !strings.HasSuffix(got[0], " r5") || !strings.HasSuffix(got[19], " r24") {
		t.Errorf("default = %d lines from %q to %q, want r5 to r24", len(got), got[0], got[len(got)-1])
	}
	got = outLines(f.runOK("log", "list", "-n", "2", "a"))
	if len(got) != 2 || !strings.HasSuffix(got[0], " r23") || !strings.HasSuffix(got[1], " r24") {
		t.Errorf("-n 2 = %q", got)
	}
	if out := f.runOK("log", "list", "-n", "0", "a"); out != "" {
		t.Errorf("-n 0 = %q", out)
	}
	if out := f.runOK("log", "list", "-n", "2", "--json", "a"); out != lines[24]+"\n"+lines[25]+"\n" {
		t.Errorf("--json -n 2 = %q, want the last two raw lines", out)
	}
	if out := f.runOK("log", "list", "-n", "100", "--json", "a"); strings.Contains(out, `"_meta"`) || strings.Count(out, "\n") != 25 {
		t.Errorf("--json printed the _meta line or the wrong count")
	}
}

func TestLogListReportsBadLinesAndListsTheRest(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/LOGS.jsonl", metaLine, logDated, "{nope", logNull)
	got := outLines(f.runOK("log", "list", "a"))
	if len(got) != 2 {
		t.Errorf("stdout = %q, want two records", got)
	}
	if !strings.Contains(f.stderr.String(), "~/Source/Projects/a/LOGS.jsonl:3:") {
		t.Errorf("stderr = %q, want line 3 reported", f.stderr.String())
	}
}

func TestLogListMissingFile(t *testing.T) {
	f := newFixture(t)
	if out := f.runOK("log", "list", "a"); out != "" {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(f.stderr.String(), "space a has no file at ~/Source/Projects/a/LOGS.jsonl") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestLogListHonoursConfiguredPath(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", twoSpaces)
	f.writeFile("b-logs.jsonl", logDated)
	if out := f.runOK("log", "list", "b"); !strings.Contains(out, "Shipped the loader") {
		t.Errorf("stdout = %q", out)
	}
}

func TestViewVerbPicking(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", todoMajor)
	f.pickReply = []string{"list"}
	f.runOK("space")
	if len(f.picks) != 1 || strings.Join(f.picks[0], "|") != "space|list|show|add|rm|attach|detach" {
		t.Errorf("space verb picker = %q", f.picks)
	}
	if !strings.HasPrefix(f.stdout.String(), "SPACE") {
		t.Errorf("picked list printed %q", f.stdout.String())
	}

	f.picks = nil
	f.cwd = filepath.Join(f.home, "Source/Projects/a")
	if out := f.runOK("todo"); !strings.Contains(out, "Fix the loader") {
		t.Errorf("todo = %q", out)
	}
	f.runOK("log")
	if len(f.picks) != 0 {
		t.Errorf("picker opened for a single verb: %q", f.picks)
	}

	f.tty = false
	for _, noun := range []string{"space", "todo", "log"} {
		if code := f.run(noun); code != exitUsage {
			t.Errorf("%s without a TTY: exit %d, want %d", noun, code, exitUsage)
		}
	}
}

func TestViewHelpAndUnknownVerb(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{{"space", "-h"}, {"space", "show", "-h"}, {"todo", "list", "-h"}, {"log", "-h"}, {"log", "list", "-h"}} {
		f.stderr.Reset()
		if code := f.run(args...); code != exitOK {
			t.Errorf("%q: exit %d", args, code)
		}
		if !strings.Contains(f.stderr.String(), "Usage: workspaces "+args[0]) {
			t.Errorf("%q: stderr = %q", args, f.stderr.String())
		}
	}
	for _, noun := range []string{"space", "todo", "log"} {
		if code := f.run(noun, "bogus"); code != exitUsage {
			t.Errorf("%s bogus: exit %d", noun, code)
		}
	}
}

func TestNounPickerReachesViews(t *testing.T) {
	f := newFixture(t)
	f.pickReply = []string{"space", "list"}
	if out := f.runOK(); !strings.HasPrefix(out, "SPACE") {
		t.Errorf("stdout = %q", out)
	}
}

func TestUsageListsViews(t *testing.T) {
	f := newFixture(t)
	f.run("-h")
	for _, want := range []string{"space list", "space show [space]", "todo list [--json] [space]", "log list [-n N] [--json] [space]"} {
		if !strings.Contains(f.stderr.String(), want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

// Planted terminal sequences: SGR, OSC 0 (window title) ended by BEL, OSC 52
// (clipboard write), a C1 CSI and DEL. JSON carries them as \u escapes.
const (
	escSGR  = `\u001b[31m`
	escOSC0 = `\u001b]0;pwned\u0007`
	escOSC  = `\u001b]52;c;Zm9v\u0007`
	escC1   = `\u009b31m\u007f`
)

func assertNoControls(t *testing.T, what, out string) {
	t.Helper()
	for _, r := range out {
		if r != '\n' && unicode.IsControl(r) {
			t.Errorf("%s: control character %U reached the output: %q", what, r, out)
			return
		}
	}
	if !strings.ContainsRune(out, unicode.ReplacementChar) {
		t.Errorf("%s: no U+FFFD in %q", what, out)
	}
}

func TestViewsReplaceControlCharacters(t *testing.T) {
	planted := escSGR + escOSC0 + escOSC + escC1
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl",
		`{"domain":"d`+planted+`","id":"i`+planted+`","status":"s`+planted+`","tag":"g`+planted+`","title":"t`+planted+`"}`)
	f.writeFile("Source/Projects/a/LOGS.jsonl",
		`{"ts":"x`+planted+`","kind":"k`+planted+`","major_category":"m`+planted+`","status":"s`+planted+`","tag":"g`+planted+`","title":"t`+planted+`"}`,
		`{"date":"d`+planted+`","text":"x`+planted+`"}`)
	assertNoControls(t, "todo list", f.runOK("todo", "list", "a"))
	assertNoControls(t, "log list", f.runOK("log", "list", "a"))

	dec := "\x1b[31m\x1b]0;pwned\x07\x1b]52;c;Zm9v\x07\u009b31m\x7f"
	f.appendRecord(launchlog.Record{TS: "t" + dec, Decision: "allow" + dec, Reason: "r" + dec, Space: spaceName("a"),
		Harness: "h" + dec, LaunchDir: "/d" + dec, Args: []string{"x" + dec}})
	assertNoControls(t, "launch list", f.runOK("launch", "list"))
	assertNoControls(t, "space show", f.runOK("space", "show", "a"))
}

func TestCleanFoldsThenReplaces(t *testing.T) {
	for in, want := range map[string]string{
		"a\tb\n c":         "a b c",
		"\x1b[31mred":      "�[31mred",
		"bel\x07":          "bel�",
		"del\x7f":          "del�",
		"c1\u009b":         "c1�",
		"  pad  ":          "pad",
		"plain é unicode":  "plain é unicode",
		"\u202egnp.exe":    "�gnp.exe",
		"iso\u2066x\u2069": "iso�x�",
		"zw\u200bj":        "zw�j",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func runeOffset(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:i])
}

func TestLogListWidthAndStatusColumn(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("é", 300)
	f.writeFile("Source/Projects/a/LOGS.jsonl",
		`{"date":"2026-10-01","kind":"k","domain":"d","status":"S1","title":"`+long+`"}`,
		`{"date":"2026-10-02","kind":"completion","domain":"operations","status":"S2","tag":"t","title":"short"}`,
		`{"date":"2026-10-03","kind":"`+strings.Repeat("k", 300)+`","domain":"`+strings.Repeat("g", 300)+`","status":"S3","title":"x"}`)
	got := outLines(f.runOK("log", "list", "a"))
	// date, kind, group and status at most fieldWidth each, two-space gaps, then the title.
	maxWidth := 4*(fieldWidth+2) + titleWidth
	for i, l := range got {
		if n := utf8.RuneCountInString(l); n > maxWidth {
			t.Errorf("row %d is %d runes, over %d", i, n, maxWidth)
		}
	}
	if !strings.HasSuffix(got[0], strings.Repeat("é", titleWidth-3)+"...") {
		t.Errorf("long title not cut to %d runes: %q", titleWidth, got[0])
	}
	pos := runeOffset(got[0], "S1")
	for i, l := range got {
		if p := runeOffset(l, "S"+strconv.Itoa(i+1)); p != pos {
			t.Errorf("row %d status at %d, row 0 at %d", i, p, pos)
		}
	}
}

func TestTodoListWidthAndColumnsAcrossGroups(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl",
		`{"domain":"one","status":"S1","title":"`+strings.Repeat("x", 300)+`"}`,
		`{"domain":"one","id":"`+strings.Repeat("i", 300)+`","status":"S3","title":"t"}`,
		`{"domain":"two","id":"LONG-IDENTIFIER-1","status":"S2","title":"t"}`)
	got := outLines(f.runOK("todo", "list", "a"))
	if len(got) != 5 {
		t.Fatalf("output = %q", got)
	}
	maxWidth := 2 + 2*(fieldWidth+2) + titleWidth
	for i, l := range got {
		if n := utf8.RuneCountInString(l); n > maxWidth {
			t.Errorf("row %d is %d runes, over %d", i, n, maxWidth)
		}
	}
	if !strings.HasSuffix(got[1], strings.Repeat("x", titleWidth-3)+"...") {
		t.Errorf("long title not cut: %q", got[1])
	}
	pos := runeOffset(got[1], "S1")
	for _, st := range []string{"S2", "S3"} {
		for _, l := range got {
			if strings.Contains(l, st) && runeOffset(l, st) != pos {
				t.Errorf("%s at %d, S1 at %d; columns differ across groups:\n%s", st, runeOffset(l, st), pos, f.stdout.String())
			}
		}
	}
}

// Two groups whose names share their first fieldWidth runes stay apart; only
// the header is cut.
func TestTodoListGroupsByTheFullName(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("g", fieldWidth+5)
	f.writeFile("Source/Projects/a/TODO.jsonl",
		`{"domain":"`+long+`-one","status":"s","title":"first"}`,
		`{"domain":"`+long+`-two","status":"s","title":"second"}`)
	got := outLines(f.runOK("todo", "list", "a"))
	if len(got) != 4 || got[0] != field(long+"-one") || got[2] != field(long+"-two") {
		t.Errorf("output = %q, want two groups with cut headers", got)
	}
}

func TestTitleFallsBackToTextThenID(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl",
		`{"domain":"d","status":"s","text":"from text","id":"I-1"}`,
		`{"domain":"d","status":"s","id":"I-2"}`,
		`{"domain":"d","status":"s"}`,
		`{"domain":"d","status":"s","tag":"g"}`,
		`{"domain":"d","status":"s","title":"","text":null,"tag":"g","id":"I-5"}`)
	want := []string{"d", "I-1 s from text", "I-2 s I-2", "#3 s -", "#4 s [g] -", "I-5 s [g] I-5"}
	got := outLines(f.runOK("todo", "list", "a"))
	if len(got) != len(want) {
		t.Fatalf("output = %q", got)
	}
	for i, w := range want {
		if g := strings.Join(strings.Fields(got[i]), " "); g != w {
			t.Errorf("line %d = %q, want %q", i, g, w)
		}
	}
	f.writeFile("Source/Projects/a/LOGS.jsonl", `{"date":"2026-10-01","text":"log text"}`, `{"date":"2026-10-01","tag":"g"}`)
	got = outLines(f.runOK("log", "list", "a"))
	if !strings.HasSuffix(got[0], " log text") || !strings.HasSuffix(got[1], " [g] -") {
		t.Errorf("log list = %q", got)
	}
}

func TestBadLinesNeverQuoteTheFileAndAreCapped(t *testing.T) {
	f := newFixture(t)
	lines := []string{todoMajor, `{"title": SECRET-BYTES`, `["SECRET-ARRAY"]`}
	for range 6 {
		lines = append(lines, "garbage-SECRET")
	}
	f.writeFile("Source/Projects/a/TODO.jsonl", lines...)
	f.runOK("todo", "list", "a")
	errs := f.stderr.String()
	if strings.Contains(errs, "SECRET") {
		t.Errorf("stderr quotes the file: %q", errs)
	}
	want := []string{
		"workspaces: ~/Source/Projects/a/TODO.jsonl:2: not valid JSON; line skipped",
		"workspaces: ~/Source/Projects/a/TODO.jsonl:3: not a JSON object; line skipped",
		"workspaces: ~/Source/Projects/a/TODO.jsonl:4: not valid JSON; line skipped",
		"workspaces: ~/Source/Projects/a/TODO.jsonl:5: not valid JSON; line skipped",
		"workspaces: ~/Source/Projects/a/TODO.jsonl:6: not valid JSON; line skipped",
		"workspaces: ~/Source/Projects/a/TODO.jsonl: 3 more bad lines skipped",
	}
	if got := outLines(errs); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("stderr =\n%s\nwant\n%s", errs, strings.Join(want, "\n"))
	}

	f.writeFile("Source/Projects/a/TODO.jsonl", "x", "x", "x", "x", "x")
	f.runOK("todo", "list", "a")
	if strings.Contains(f.stderr.String(), "more bad lines") || strings.Count(f.stderr.String(), "\n") != 5 {
		t.Errorf("five bad lines should all be listed with no summary: %q", f.stderr.String())
	}
}

func TestViewsRefuseAFIFO(t *testing.T) {
	cases := []struct {
		file string
		args []string
	}{
		{"Source/Projects/a/TODO.jsonl", []string{"todo", "list", "a"}},
		{"Source/Projects/a/LOGS.jsonl", []string{"log", "list", "a"}},
		{"Source/Projects/a/TODO.jsonl", []string{"space", "list"}},
		{"Source/Projects/a/LOGS.jsonl", []string{"space", "show", "a"}},
		{".local/state/workspaces/launches.jsonl", []string{"space", "list"}},
	}
	for _, c := range cases {
		f := newFixture(t)
		path := filepath.Join(f.home, c.file)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		done := make(chan int, 1)
		go func() { done <- f.run(c.args...) }()
		select {
		case code := <-done:
			if code != exitIOErr {
				t.Errorf("%q on a FIFO %s: exit %d, want %d", c.args, c.file, code, exitIOErr)
			}
			if !strings.Contains(f.stderr.String(), "not a regular file") {
				t.Errorf("%q: stderr = %q", c.args, f.stderr.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q blocked on a FIFO at %s", c.args, c.file)
		}
	}
}

func TestUsageLogMatchesColumns(t *testing.T) {
	f := newFixture(t)
	f.run("log", "list", "-h")
	usage := strings.Join(strings.Fields(f.stderr.String()), " ")
	if !strings.Contains(usage, "date (or the date of ts), kind, group (domain, major_category or category), status, then [tag] and title") {
		t.Errorf("log usage does not list the columns in order: %q", usage)
	}
	f.stderr.Reset()
	f.run("todo", "list", "-h")
	if usage := strings.Join(strings.Fields(f.stderr.String()), " "); !strings.Contains(usage, "id, status, then [tag] and title") {
		t.Errorf("todo usage = %q", usage)
	}
}

func TestViewGetwdErrorExits70(t *testing.T) {
	f := newFixture(t)
	f.getwdErr = os.ErrPermission
	if code := f.run("todo", "list"); code != exitInternal {
		t.Fatalf("exit %d, want %d", code, exitInternal)
	}
	if !strings.Contains(f.stderr.String(), "cannot resolve the current directory") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestSpaceListStateDirErrorExits78(t *testing.T) {
	f := newFixture(t)
	f.env["HOME"] = "relative/home"
	delete(f.env, "XDG_STATE_HOME")
	f.writeConfig("spaces.json", `{"allow_launch":["/"],"spaces":{"a":{"root":"/"}}}`)
	if code := f.run("space", "list"); code != exitConfig {
		t.Fatalf("exit %d, want %d; stderr %s", code, exitConfig, f.stderr.String())
	}
	if !strings.Contains(f.stderr.String(), "HOME is not set to an absolute path") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestSpaceViewsLaunchLogProblems(t *testing.T) {
	for _, args := range [][]string{{"space", "list"}, {"space", "show", "a"}} {
		f := newFixture(t)
		if err := os.MkdirAll(f.logFile(), 0o700); err != nil {
			t.Fatal(err)
		}
		if code := f.run(args...); code != exitIOErr {
			t.Errorf("%q with launches.jsonl a directory: exit %d, want %d", args, code, exitIOErr)
		}
		if f.stdout.Len() != 0 {
			t.Errorf("%q: stdout = %q, want nothing", args, f.stdout.String())
		}

		f = newFixture(t)
		f.appendRecord(launchlog.Record{TS: "2026-10-04T22:31:05.000Z", Space: spaceName("a")})
		fh, err := os.OpenFile(f.logFile(), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		fh.WriteString("garbage\n")
		fh.Close()
		f.runOK(args...)
		if !strings.Contains(f.stderr.String(), "skipped 1 malformed line(s)") {
			t.Errorf("%q: stderr = %q, want the malformed count", args, f.stderr.String())
		}
		if !strings.Contains(f.stdout.String(), "2026-10-04 22:31:05") {
			t.Errorf("%q: the good launch record is missing:\n%s", args, f.stdout.String())
		}
	}
}

func TestSpaceViewsUnreadableFiles(t *testing.T) {
	for _, file := range []string{"TODO.jsonl", "LOGS.jsonl"} {
		for _, args := range [][]string{{"space", "list"}, {"space", "show", "a"}} {
			f := newFixture(t)
			if err := os.MkdirAll(filepath.Join(f.home, "Source/Projects/a", file), 0o755); err != nil {
				t.Fatal(err)
			}
			if code := f.run(args...); code != exitIOErr {
				t.Errorf("%s a directory, %q: exit %d, want %d", file, args, code, exitIOErr)
			}
			if !strings.Contains(f.stdout.String(), "unreadable") {
				t.Errorf("%s, %q: stdout = %q, want unreadable", file, args, f.stdout.String())
			}
			if !strings.Contains(f.stderr.String(), file+": not a regular file") {
				t.Errorf("%s, %q: stderr = %q, want the error", file, args, f.stderr.String())
			}
			if args[1] == "list" && !strings.Contains(f.stderr.String(), "space a: ") {
				t.Errorf("%s: stderr = %q, want the space named", file, f.stderr.String())
			}
		}
	}
}

func TestLogListReadErrorExits74(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.home, "Source/Projects/a/LOGS.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := f.run("log", "list", "a"); code != exitIOErr {
		t.Fatalf("exit %d, want %d", code, exitIOErr)
	}
	if !strings.Contains(f.stderr.String(), "reading ") {
		t.Errorf("stderr = %q", f.stderr.String())
	}
}

func TestNotMigratedLooksBesideTheTodoPath(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", twoSpaces)
	f.writeFile("Source/Projects/b/notes/TODO.md", "# old")
	if out := f.runOK("space", "list"); !strings.Contains(outLines(out)[2], "TODO.md only") {
		t.Errorf("TODO.md beside the configured todo path: %q", outLines(out)[2])
	}
	os.Remove(filepath.Join(f.home, "Source/Projects/b/notes/TODO.md"))
	f.writeFile("Source/Projects/b/TODO.md", "# old")
	row := strings.Fields(outLines(f.runOK("space", "list"))[2])
	if row[3] != "none" {
		t.Errorf("TODO.md at the root only, todo elsewhere: row %q, want todo none", row)
	}
}

func TestZeroCounts(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", metaLine)
	if err := os.WriteFile(filepath.Join(f.home, "Source/Projects/a/LOGS.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	row := strings.Join(strings.Fields(outLines(f.runOK("space", "list"))[1]), " ")
	if !strings.Contains(row, " 0 items 0 records ") {
		t.Errorf("row = %q, want 0 items and 0 records", row)
	}
}

func TestLogListMissingWithTodoMDSaysNoFile(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.md", "# old")
	f.runOK("log", "list", "a")
	if errs := f.stderr.String(); !strings.Contains(errs, "has no file at ~/Source/Projects/a/LOGS.jsonl") || strings.Contains(errs, "TODO.md only") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestMajorCategoryBeatsCategory(t *testing.T) {
	f := newFixture(t)
	rec := `{"major_category":"major","category":"minor","status":"s","title":"t"}`
	f.writeFile("Source/Projects/a/TODO.jsonl", rec)
	f.writeFile("Source/Projects/a/LOGS.jsonl", rec)
	if got := outLines(f.runOK("todo", "list", "a")); got[0] != "major" {
		t.Errorf("todo group = %q", got[0])
	}
	if got := strings.Fields(f.runOK("log", "list", "a")); got[2] != "major" {
		t.Errorf("log group = %q", got)
	}
}

func TestEveryFieldStaysOnOneLine(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl", `{"domain":"g\nroup","id":"i\td","status":"st\natus","tag":"t\nag","title":"ti\ntle"}`)
	out := f.runOK("todo", "list", "a")
	if strings.Contains(out, "\t") {
		t.Errorf("todo list printed a tab: %q", out)
	}
	got := outLines(out)
	if len(got) != 2 || got[0] != "g roup" || strings.Join(strings.Fields(got[1]), " ") != "i d st atus [t ag] ti tle" {
		t.Errorf("todo list = %q", got)
	}
	f.writeFile("Source/Projects/a/LOGS.jsonl",
		`{"date":"da\nte","kind":"ki\tnd","category":"gr\noup","status":"st\natus","tag":"t\nag","title":"ti\ttle"}`,
		`{"ts":"t\ns","title":"x"}`)
	got = outLines(f.runOK("log", "list", "a"))
	if len(got) != 2 || strings.Join(strings.Fields(got[0]), " ") != "da te ki nd gr oup st atus [t ag] ti tle" || !strings.HasPrefix(got[1], "t s ") {
		t.Errorf("log list = %q", got)
	}
}

func TestViewHelpFlagSpellings(t *testing.T) {
	for _, args := range [][]string{{"space", "--help"}, {"todo", "-help"}, {"log", "--help"}} {
		f := newFixture(t)
		if code := f.run(args...); code != exitOK {
			t.Errorf("%q: exit %d", args, code)
		}
		if !strings.Contains(f.stderr.String(), "Usage: workspaces "+args[0]) {
			t.Errorf("%q: stderr = %q", args, f.stderr.String())
		}
	}
}

// "[" is a glob character, which config paths refuse, so no SGR here.
func TestSpaceViewsReplaceControlCharactersFromConfig(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"],"deny_access":["~/g`+escOSC+`"],
	 "spaces":{"a":{"root":"~/Source/Projects/a`+escOSC0+`","attach":["~/x`+escOSC+`"],"deny_access":["~/y`+escC1+`"]}}}`)
	assertNoControls(t, "space list", f.runOK("space", "list"))
	assertNoControls(t, "space show", f.runOK("space", "show", "a"))
}

// The cut widths are fixed by design, so the numbers are written out rather
// than read from the constants.
func TestCutWidthsAreFixed(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/LOGS.jsonl",
		`{"date":"2026-10-01","kind":"`+strings.Repeat("k", 32)+`","domain":"`+strings.Repeat("g", 33)+`","status":"s","title":"`+strings.Repeat("t", 72)+`"}`,
		`{"date":"2026-10-02","kind":"k","domain":"d","status":"s","title":"`+strings.Repeat("u", 73)+`"}`)
	got := outLines(f.runOK("log", "list", "a"))
	if len(got) != 2 {
		t.Fatalf("output = %q", got)
	}
	if !strings.Contains(got[0], " "+strings.Repeat("k", 32)+" ") {
		t.Errorf("32-rune kind was cut: %q", got[0])
	}
	if !strings.Contains(got[0], " "+strings.Repeat("g", 29)+"... ") {
		t.Errorf("33-rune domain not cut to 29 plus ...: %q", got[0])
	}
	if !strings.HasSuffix(got[0], " "+strings.Repeat("t", 72)) {
		t.Errorf("72-rune title was cut: %q", got[0])
	}
	if !strings.HasSuffix(got[1], " "+strings.Repeat("u", 69)+"...") {
		t.Errorf("73-rune title not cut to 69 plus ...: %q", got[1])
	}
}

func TestTodoColumnsAlignWithUnevenWidths(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/TODO.jsonl",
		`{"domain":"one","id":"Ü-12","status":"open","title":"alpha"}`,
		`{"domain":"one","id":"A-22","status":"blocked","title":"beta"}`,
		`{"domain":"two","id":"B-3","status":"done","title":"gamma"}`)
	got := outLines(f.runOK("todo", "list", "a"))
	if len(got) != 5 {
		t.Fatalf("output = %q", got)
	}
	rows := map[string]string{"open": "alpha", "blocked": "beta", "done": "gamma"}
	statusPos, titlePos := -1, -1
	for _, l := range got {
		for st, title := range rows {
			if !strings.Contains(l, title) {
				continue
			}
			sp, tp := runeOffset(l, st), runeOffset(l, title)
			if statusPos < 0 {
				statusPos, titlePos = sp, tp
			}
			if sp != statusPos || tp != titlePos {
				t.Errorf("row %q: status at %d, title at %d; want %d, %d", l, sp, tp, statusPos, titlePos)
			}
		}
	}
	if !strings.Contains(f.stdout.String(), "  A-22  blocked  beta\n") {
		t.Errorf("want two-space gaps: %q", f.stdout.String())
	}
}

func TestLogTitleColumnAligns(t *testing.T) {
	f := newFixture(t)
	f.writeFile("Source/Projects/a/LOGS.jsonl",
		`{"date":"2026-10-01","kind":"k","domain":"d","status":"open","title":"alpha"}`,
		`{"date":"2026-10-02","kind":"k","domain":"d","status":"awaiting","title":"beta"}`)
	got := outLines(f.runOK("log", "list", "a"))
	if len(got) != 2 || runeOffset(got[0], "alpha") != runeOffset(got[1], "beta") {
		t.Errorf("title columns differ: %q", got)
	}
}
