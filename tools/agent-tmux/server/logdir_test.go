package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A log folder that already exists with a looser mode is tightened to 0700 on the next write: MkdirAll alone
// leaves an existing folder as it is.
func TestLogFoldersAreTightenedTo0700(t *testing.T) {
	for name, write := range map[string]func(path string) error{
		"audit":  func(path string) error { return NewAudit(path).Write(auditRec{tool: "tmux_list", result: "ok"}) },
		"events": func(path string) error { return NewEventLog(path).Append(record{Ev: "created", Pane: "%1"}) },
	} {
		dir := filepath.Join(t.TempDir(), "logs")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := write(filepath.Join(dir, "log")); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: folder mode %v (err %v), want 0700", name, fi.Mode().Perm(), err)
		}
	}
}
