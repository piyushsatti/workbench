package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var base = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// write puts a transcript of lines in <config>/projects/<project>/<id>.jsonl
// and sets its modification time to base plus age.
func write(t *testing.T, config, project, id string, age time.Duration, lines ...string) string {
	t.Helper()
	dir := filepath.Join(config, "projects", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base.Add(age), base.Add(age)); err != nil {
		t.Fatal(err)
	}
	return path
}

func cwdLine(cwd string) string {
	b, _ := json.Marshal(map[string]string{"type": "user", "cwd": cwd})
	return string(b)
}

func titleLine(typ, key, value string) string {
	b, _ := json.Marshal(map[string]string{"type": typ, key: value})
	return string(b)
}

func ids(ss []Session) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func TestTitle(t *testing.T) {
	filler := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = `{"type":"assistant"}`
		}
		return out
	}
	custom := func(s string) string { return titleLine("custom-title", "customTitle", s) }
	ai := func(s string) string { return titleLine("ai-title", "aiTitle", s) }
	prompt := func(s string) string { return titleLine("last-prompt", "lastPrompt", s) }
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"custom beats ai and prompt", []string{ai("A"), prompt("P"), custom("C")}, "C"},
		{"ai beats prompt", []string{prompt("P"), ai("A")}, "A"},
		{"prompt alone", []string{prompt("P")}, "P"},
		{"last custom wins", []string{custom("one"), custom("two")}, "two"},
		{"first ai wins", []string{ai("first"), ai("second")}, "first"},
		{"last prompt wins", []string{prompt("one"), prompt("two")}, "two"},
		{"prompt cut to 60 runes", []string{prompt(strings.Repeat("é", 70))}, strings.Repeat("é", 60)},
		{"prompt of exactly 60 kept", []string{prompt(strings.Repeat("x", 60))}, strings.Repeat("x", 60)},
		{"custom drops the caveat and after", []string{custom("  Fix bug<local-command-caveat>junk")}, "Fix bug"},
		{"custom drops a branch suffix", []string{custom("Fix bug (Branch 2) more")}, "Fix bug"},
		{"custom trims leading space", []string{custom("   hello")}, "hello"},
		{"ai is not stripped", []string{ai("Fix (Branch x)")}, "Fix (Branch x)"},
		{"blank custom falls to untitled, not to ai", []string{custom(""), ai("A")}, Untitled},
		{"no titles, short file", filler(3), Empty},
		{"no titles, nine lines", filler(9), Empty},
		{"no titles, ten lines", filler(10), Untitled},
		{"short file with a title is not empty", []string{ai("A")}, "A"},
		{"keys in message text are ignored", []string{`{"type":"user","message":"the ai-title and last-prompt records"}`}, Empty},
		{"a bad line with a key is skipped", []string{`{"type":"ai-title"`, prompt("P")}, "P"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, t.TempDir(), "p", "s", 0, tt.lines...)
			if got := Title(path); got != tt.want {
				t.Errorf("Title = %q, want %q", got, tt.want)
			}
		})
	}
	if got := Title(filepath.Join(t.TempDir(), "missing.jsonl")); got != Untitled {
		t.Errorf("Title of a missing file = %q, want %q", got, Untitled)
	}
}

func TestTitleLongLine(t *testing.T) {
	path := write(t, t.TempDir(), "p", "s", 0,
		`{"type":"user","message":"`+strings.Repeat("x", 3<<20)+`"}`, titleLine("ai-title", "aiTitle", "after the long line"))
	if got := Title(path); got != "after the long line" {
		t.Errorf("Title = %q", got)
	}
}

func TestFind(t *testing.T) {
	cfg := t.TempDir()
	root := "/h/Source/a"
	write(t, cfg, "-h-Source-a", "root-old", 1*time.Hour, cwdLine(root))
	write(t, cfg, "-h-Source-a", "root-new", 5*time.Hour, cwdLine(root))
	// A worktree has its own project folder under another name.
	write(t, cfg, "-h-Source-a--claude-worktrees-x", "worktree", 3*time.Hour, cwdLine(root+"/.claude/worktrees/x"))
	// The cwd decides, not the folder name: this folder is named for a.
	write(t, cfg, "-h-Source-a-lookalike", "elsewhere", 4*time.Hour, cwdLine("/h/Source/b"))
	write(t, cfg, "-h-Source-ab", "sibling-prefix", 6*time.Hour, cwdLine(root+"b"))
	// And a folder with an unrelated name can hold a session of the root.
	write(t, cfg, "renamed-folder", "odd-name", 2*time.Hour, cwdLine(root))
	write(t, cfg, "-h-Source-a", "no-cwd", 7*time.Hour, `{"type":"mode"}`)

	got, err := Find(cfg, []string{root}, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"root-new", "worktree", "odd-name", "root-old"}
	if !slices.Equal(ids(got), want) {
		t.Fatalf("ids = %q, want %q", ids(got), want)
	}
	if got[1].Cwd != root+"/.claude/worktrees/x" || got[0].Cwd != root {
		t.Errorf("cwds = %q, %q", got[0].Cwd, got[1].Cwd)
	}
	if !got[0].Modified.Equal(base.Add(5 * time.Hour)) {
		t.Errorf("Modified = %v", got[0].Modified)
	}
	if want := filepath.Join(cfg, "projects/-h-Source-a/root-new.jsonl"); got[0].Path != want {
		t.Errorf("Path = %q, want %q", got[0].Path, want)
	}
}

func TestFindLimit(t *testing.T) {
	cfg := t.TempDir()
	for i := range 5 {
		write(t, cfg, "p", fmt.Sprintf("s%d", i), time.Duration(i)*time.Hour, cwdLine("/r"))
	}
	// Newer than every match, but outside the root: it must not use up the limit.
	write(t, cfg, "q", "other", 9*time.Hour, cwdLine("/elsewhere"))
	for _, tt := range []struct {
		limit int
		want  []string
	}{
		{0, []string{"s4", "s3", "s2", "s1", "s0"}},
		{1, []string{"s4"}},
		{3, []string{"s4", "s3", "s2"}},
		{5, []string{"s4", "s3", "s2", "s1", "s0"}},
		{9, []string{"s4", "s3", "s2", "s1", "s0"}},
	} {
		got, err := Find(cfg, []string{"/r"}, tt.limit)
		if err != nil || !slices.Equal(ids(got), tt.want) {
			t.Errorf("limit %d: ids = %q, %v; want %q", tt.limit, ids(got), err, tt.want)
		}
	}
}

func TestFindTieBreaksByID(t *testing.T) {
	cfg := t.TempDir()
	for _, id := range []string{"c", "a", "b"} {
		write(t, cfg, "p", id, 0, cwdLine("/r"))
	}
	got, _ := Find(cfg, []string{"/r"}, 0)
	if !slices.Equal(ids(got), []string{"a", "b", "c"}) {
		t.Errorf("ids = %q", ids(got))
	}
}

func TestFindRoots(t *testing.T) {
	cfg := t.TempDir()
	write(t, cfg, "p", "logical", 0, cwdLine("/link/a"))
	write(t, cfg, "p", "physical", time.Hour, cwdLine("/real/a/sub"))
	write(t, cfg, "p", "neither", 2*time.Hour, cwdLine("/other"))
	got, _ := Find(cfg, []string{"/real/a", "/link/a"}, 0)
	if !slices.Equal(ids(got), []string{"physical", "logical"}) {
		t.Errorf("ids = %q", ids(got))
	}
	got, _ = Find(cfg, []string{"/real/a/"}, 0)
	if !slices.Equal(ids(got), []string{"physical"}) {
		t.Errorf("trailing slash root: ids = %q", ids(got))
	}
}

func TestFindReadsOnlyUntilTheFirstCwd(t *testing.T) {
	cfg := t.TempDir()
	// The first cwd decides; a later one in another folder is never read.
	write(t, cfg, "p", "moved", 0, `{"type":"mode"}`, cwdLine("/r"), cwdLine("/elsewhere"))
	write(t, cfg, "p", "moved-out", time.Hour, cwdLine("/elsewhere"), cwdLine("/r"))
	// A line holding the key as text, not a cwd field, is passed over.
	write(t, cfg, "p", "text", 2*time.Hour, `{"type":"user","message":"the \"cwd\" key"}`, cwdLine("/r"))
	// A cwd record after a megabyte-long line.
	write(t, cfg, "p", "long", 3*time.Hour, `{"type":"x","m":"`+strings.Repeat("y", 2<<20)+`"}`, cwdLine("/r"))
	// An empty cwd is not a cwd.
	write(t, cfg, "p", "empty", 4*time.Hour, cwdLine(""), cwdLine("/r"))
	// A last line without a newline still counts.
	path := filepath.Join(cfg, "projects/p/unterminated.jsonl")
	if err := os.WriteFile(path, []byte(cwdLine("/r")), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := Find(cfg, []string{"/r"}, 0)
	have := ids(got)
	slices.Sort(have)
	if want := []string{"empty", "long", "moved", "text", "unterminated"}; !slices.Equal(have, want) {
		t.Errorf("ids = %q, want %q", have, want)
	}
}

func TestFindSkipsWhatIsNotATranscript(t *testing.T) {
	cfg := t.TempDir()
	write(t, cfg, "p", "good", 0, cwdLine("/r"))
	// A sibling folder named like a session, other extensions, a stray file in projects/.
	if err := os.MkdirAll(filepath.Join(cfg, "projects/p/good/subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"projects/p/notes.txt", "projects/p/good.jsonl.bak", "projects/stray.jsonl"} {
		if err := os.WriteFile(filepath.Join(cfg, name), []byte(cwdLine("/r")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(cfg, "projects/p/dir.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A symlinked project folder and a symlinked transcript are not followed.
	outside := t.TempDir()
	write(t, outside, "x", "linked", 0, cwdLine("/r"))
	if err := os.Symlink(filepath.Join(outside, "projects/x"), filepath.Join(cfg, "projects/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "projects/x/linked.jsonl"), filepath.Join(cfg, "projects/p/linked-file.jsonl")); err != nil {
		t.Fatal(err)
	}
	got, err := Find(cfg, []string{"/r"}, 0)
	if err != nil || !slices.Equal(ids(got), []string{"good"}) {
		t.Errorf("ids = %q, %v", ids(got), err)
	}
}

func TestFindMissingAndBrokenConfigDirs(t *testing.T) {
	if got, err := Find(filepath.Join(t.TempDir(), "none"), []string{"/r"}, 0); got != nil || err != nil {
		t.Errorf("missing dir: %v, %v", got, err)
	}
	cfg := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg, "projects"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(cfg, []string{"/r"}, 0); err == nil {
		t.Error("projects is a file: want an error")
	}
	// An unreadable transcript is skipped, not fatal.
	cfg = t.TempDir()
	write(t, cfg, "p", "ok", 0, cwdLine("/r"))
	bad := write(t, cfg, "p", "locked", time.Hour, cwdLine("/r"))
	if err := os.Chmod(bad, 0); err != nil {
		t.Fatal(err)
	}
	got, err := Find(cfg, []string{"/r"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 && !slices.Equal(ids(got), []string{"ok"}) {
		t.Errorf("ids = %q", ids(got))
	}
}
