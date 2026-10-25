package records

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "TODO.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadMissingFileWrapsErrNotExist(t *testing.T) {
	_, _, err := Read(filepath.Join(t.TempDir(), "nope.jsonl"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestReadSkipsMetaHeader(t *testing.T) {
	path := writeFile(t, `{"_meta":true,"schema_version":1,"fields":{}}`+"\n"+`{"title":"one"}`+"\n")
	recs, bad, err := Read(path)
	if err != nil || len(bad) != 0 {
		t.Fatalf("err %v, bad %v", err, bad)
	}
	if len(recs) != 1 || recs[0].Text("title") != "one" {
		t.Fatalf("recs = %+v, want only the non-meta record", recs)
	}
}

func TestReadKeepsMetaFalse(t *testing.T) {
	recs, _, _ := Read(writeFile(t, `{"_meta":false,"title":"kept"}`+"\n"))
	if len(recs) != 1 {
		t.Fatalf("recs = %+v, want the record with _meta false kept", recs)
	}
}

func TestReadReportsBadLinesWithLineNumbersAndKeepsGoing(t *testing.T) {
	body := strings.Join([]string{
		`{"title":"first"}`,
		``,
		`{"title": broken`,
		`[1,2]`,
		`null`,
		`{"title":"last"}`,
	}, "\n")
	recs, bad, err := Read(writeFile(t, body))
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, b := range bad {
		lines = append(lines, b.Line)
		if b.Err == nil {
			t.Errorf("line %d: no error", b.Line)
		}
	}
	if len(lines) != 3 || lines[0] != 3 || lines[1] != 4 || lines[2] != 5 {
		t.Errorf("bad lines = %v, want [3 4 5] (blank lines count)", lines)
	}
	if len(recs) != 2 || recs[0].Line != 1 || recs[1].Line != 6 || recs[1].Text("title") != "last" {
		t.Errorf("recs = %+v, want lines 1 and 6", recs)
	}
}

func TestReadLastLineWithoutNewline(t *testing.T) {
	recs, _, _ := Read(writeFile(t, `{"a":"1"}`+"\n"+`{"a":"2"}`))
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
}

func TestReadKeepsRawLine(t *testing.T) {
	raw := `{"zz_future":{"x":[1]},"title":"t"}`
	recs, _, _ := Read(writeFile(t, "  "+raw+"  \n"))
	if len(recs) != 1 || recs[0].Raw != raw {
		t.Fatalf("raw = %q, want %q", recs[0].Raw, raw)
	}
	if got := recs[0].Text("zz_future"); got != `{"x":[1]}` {
		t.Errorf("unknown field = %q, want it kept", got)
	}
}

func TestTextRendersEveryShape(t *testing.T) {
	recs, _, err := Read(writeFile(t, `{"s":"str","n":null,"i":3,"f":1.50,"b":true,"l":["a","b"],"ln":["a",null,2],"o":{"k": "v"}}`))
	if err != nil || len(recs) != 1 {
		t.Fatal(err)
	}
	r := recs[0]
	for key, want := range map[string]string{
		"s": "str", "n": "", "missing": "", "i": "3", "f": "1.50", "b": "true",
		"l": "a,b", "ln": "a,2", "o": `{"k":"v"}`,
	} {
		if got := r.Text(key); got != want {
			t.Errorf("Text(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestFirstSkipsBlankAndNull(t *testing.T) {
	recs, _, _ := Read(writeFile(t, `{"domain":null,"major_category":"","category":"c"}`))
	if got := recs[0].First("domain", "major_category", "category"); got != "c" {
		t.Errorf("First = %q, want c", got)
	}
	if got := recs[0].First("nope"); got != "" {
		t.Errorf("First of missing = %q", got)
	}
}

func TestReadSkipsMetaOnAnyLine(t *testing.T) {
	recs, _, _ := Read(writeFile(t, `{"title":"a"}`+"\n"+`{"title":"b"}`+"\n"+`{"_meta":true}`+"\n"+`{"title":"c"}`))
	if len(recs) != 3 || recs[2].Text("title") != "c" {
		t.Errorf("recs = %+v, want the _meta record on line 3 skipped", recs)
	}
}

func TestReadBadLineErrorsAreFixedTexts(t *testing.T) {
	_, bad, _ := Read(writeFile(t, "{oops\n[1]\nnull\n\"s\"\n"))
	want := []error{ErrNotJSON, ErrNotObject, ErrNotObject, ErrNotObject}
	if len(bad) != len(want) {
		t.Fatalf("bad = %v", bad)
	}
	for i, w := range want {
		if bad[i].Err != w {
			t.Errorf("line %d: err %v, want %v", bad[i].Line, bad[i].Err, w)
		}
	}
}

func TestReadRefusesNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fifo, dir} {
		done := make(chan error, 1)
		go func() { _, _, err := Read(path); done <- err }()
		select {
		case err := <-done:
			if !errors.Is(err, ErrNotRegular) {
				t.Errorf("%s: err = %v, want ErrNotRegular", path, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Read blocked on %s", path)
		}
	}
}
