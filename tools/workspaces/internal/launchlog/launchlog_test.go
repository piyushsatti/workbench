package launchlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestAppendConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "workspaces", "launches.jsonl")
	const writers = 50
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Append(path, Record{ID: NewID(), Decision: "allow", Args: []string{fmt.Sprint(i), strings.Repeat("x", 2000)}})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	lines, malformed, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != writers || malformed != 0 {
		t.Fatalf("got %d valid lines and %d malformed, want %d and 0", len(lines), malformed, writers)
	}
	ids := map[string]bool{}
	for _, l := range lines {
		ids[l.Record.ID] = true
	}
	if len(ids) != writers {
		t.Errorf("got %d distinct ids, want %d", len(ids), writers)
	}
}

func TestAppendWriteError(t *testing.T) {
	if f, err := os.OpenFile("/dev/full", os.O_WRONLY|os.O_APPEND, 0); err != nil {
		t.Skipf("/dev/full not writable here: %v", err)
	} else {
		f.Close()
	}
	if err := Append("/dev/full", Record{ID: NewID()}); err == nil {
		t.Fatal("Append to /dev/full succeeded, want the write error")
	}
}

func TestAppendPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workspaces")
	path := filepath.Join(dir, "launches.jsonl")
	if err := Append(path, Record{}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got&^want != 0 {
			t.Errorf("%s mode = %o, want at most %o", p, got, want)
		}
	}
}

func TestAppendFieldShapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launches.jsonl")
	if err := Append(path, Record{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	for _, want := range []string{`"space":null`, `"tmux":null`, `"args":[]`, `"add_dirs":[]`, `"rule":""`} {
		if !strings.Contains(line, want) {
			t.Errorf("record %s lacks %s", line, want)
		}
	}
	if !strings.HasSuffix(line, "}\n") || strings.Count(line, "\n") != 1 {
		t.Errorf("record is not exactly one line: %q", line)
	}
}

func TestReadSkipsMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launches.jsonl")
	body := `{"id":"a","decision":"allow"}
not json
{"id":"b",
` + "\n" + `{"id":"c","decision":"refuse"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, malformed, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if malformed != 2 {
		t.Errorf("malformed = %d, want 2", malformed)
	}
	if len(lines) != 2 || lines[0].Record.ID != "a" || lines[1].Record.ID != "c" {
		t.Errorf("lines = %+v, want records a and c", lines)
	}
}

func TestReadMissingFile(t *testing.T) {
	lines, malformed, err := Read(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil || len(lines) != 0 || malformed != 0 {
		t.Errorf("Read(missing) = %v, %d, %v; want empty", lines, malformed, err)
	}
}

func TestFormatTimeAndID(t *testing.T) {
	ts := FormatTime(time.Date(2026, 10, 4, 22, 31, 5, 123456789, time.FixedZone("", 9*3600+1800)))
	if ts != "2026-10-04T22:31:05.123+09:30" {
		t.Errorf("FormatTime = %s", ts)
	}
	id := NewID()
	if len(id) != 16 || strings.Trim(id, "0123456789abcdef") != "" {
		t.Errorf("NewID = %q, want 16 hex characters", id)
	}
}

func TestReadLongRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launches.jsonl")
	if err := Append(path, Record{ID: "long", Args: []string{strings.Repeat("x", 100*1024)}}); err != nil {
		t.Fatal(err)
	}
	lines, malformed, err := Read(path)
	if err != nil || malformed != 0 || len(lines) != 1 {
		t.Fatalf("Read = %d lines, %d malformed, %v; want 1, 0, nil", len(lines), malformed, err)
	}
}

func TestReadRefusesAFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launches.jsonl")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := Read(path); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("err = %v, want not a regular file", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read blocked on a FIFO")
	}
}

func TestUnenforcedDenyField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launches.jsonl")
	if err := Append(path, Record{ID: "zero"}); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, Record{ID: "two", UnenforcedDeny: 2}); err != nil {
		t.Fatal(err)
	}
	lines, _, err := Read(path)
	if err != nil || len(lines) != 2 {
		t.Fatalf("Read = %d lines, %v", len(lines), err)
	}
	if strings.Contains(lines[0].Raw, "unenforced_deny") {
		t.Errorf("a zero count must be left out: %s", lines[0].Raw)
	}
	if !strings.Contains(lines[1].Raw, `"unenforced_deny":2`) || lines[1].Record.UnenforcedDeny != 2 || lines[0].Record.UnenforcedDeny != 0 {
		t.Errorf("rows = %s / %s", lines[0].Raw, lines[1].Raw)
	}
}

func TestReadRowsWrittenBeforeTheNewField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launches.jsonl")
	old := `{"ts":"2026-10-04T22:31:05.123Z","id":"abc","agent":"2026.10.04","decision":"allow","reason":"allow_launch","rule":"~/Source","cwd":"/h","cwd_logical":"/h","launch_dir":"/h","space":null,"harness":"claude","adapter":"claude","mode":"session","tmux":null,"bin":"/b","bin_target":"/b","args":[],"add_dirs":[],"config_sha256":"","tty":true,"pid":1,"ppid":2}` + "\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, malformed, err := Read(path)
	if err != nil || malformed != 0 || len(lines) != 1 {
		t.Fatalf("Read = %d lines, %d malformed, %v", len(lines), malformed, err)
	}
	if r := lines[0].Record; r.ID != "abc" || r.UnenforcedDeny != 0 || r.Harness != "claude" {
		t.Errorf("record = %+v", r)
	}
}

func TestSessionRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "workspaces", "sessions.jsonl")
	want := []SessionRecord{
		{TS: "2026-10-05T10:00:00.000Z", LaunchID: "l1", SessionID: "s1", Cwd: "/r", Source: "startup"},
		{TS: "2026-10-05T11:00:00.000Z", LaunchID: "l2", SessionID: "s1", Cwd: "/r", Source: "resume"},
	}
	for _, r := range want {
		if err := AppendSession(path, r); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(path)
	if first, _, _ := strings.Cut(string(raw), "\n"); first != `{"ts":"2026-10-05T10:00:00.000Z","launch_id":"l1","session_id":"s1","cwd":"/r","source":"startup"}` {
		t.Errorf("first line = %s", first)
	}
	info, _ := os.Stat(path)
	dir, _ := os.Stat(filepath.Dir(path))
	if info.Mode().Perm() != 0o600 || dir.Mode().Perm() != 0o700 {
		t.Errorf("modes: file %v, dir %v", info.Mode().Perm(), dir.Mode().Perm())
	}
	got, malformed, err := ReadSessions(path)
	if err != nil || malformed != 0 || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ReadSessions = %+v, %d, %v", got, malformed, err)
	}
}

func TestReadSessionsMissingAndMalformed(t *testing.T) {
	dir := t.TempDir()
	if got, bad, err := ReadSessions(filepath.Join(dir, "none")); got != nil || bad != 0 || err != nil {
		t.Errorf("missing: %v, %d, %v", got, bad, err)
	}
	path := filepath.Join(dir, "s.jsonl")
	os.WriteFile(path, []byte("not json\n\n{\"session_id\":\"s\"}\n"), 0o600)
	got, bad, err := ReadSessions(path)
	if err != nil || bad != 1 || len(got) != 1 || got[0].SessionID != "s" {
		t.Errorf("ReadSessions = %+v, %d, %v", got, bad, err)
	}
	if _, _, err := ReadSessions(dir); err == nil {
		t.Error("a directory must be an error")
	}
}
