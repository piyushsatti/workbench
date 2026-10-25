package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileCreatesDirAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf", "workspaces", "spaces.json")
	if err := WriteFile(path, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir: %v %v, want mode 0700", dirInfo, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file: %v %v, want mode 0600", info, err)
	}
	if _, err := os.Stat(path + ".prev"); !os.IsNotExist(err) {
		t.Errorf("a first write left a .prev: %v", err)
	}
}

func TestWriteFileKeepsPrevAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spaces.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Errorf("file = %q", got)
	}
	if got, _ := os.ReadFile(path + ".prev"); string(got) != "old\n" {
		t.Errorf(".prev = %q, want the old bytes", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want the original 0640", info.Mode().Perm())
	}

	if err := WriteFile(path, []byte("newer\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path + ".prev"); string(got) != "new\n" {
		t.Errorf(".prev = %q, want the previous version only", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("directory holds %d entries, want the file and its .prev: %v", len(entries), entries)
	}
}

func TestWriteFileFailsWhenDirIsAFile(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(blocker, "x.json"), []byte("x")); err == nil {
		t.Error("want an error")
	}
}

func TestParseSpacesAndHarnessesMatchLoaders(t *testing.T) {
	home := tempHome(t)
	if _, err := ParseSpaces([]byte(`{"deny_acess":[]}`), home); err == nil {
		t.Error("ParseSpaces accepted an unknown key")
	}
	if _, err := ParseHarnesses([]byte(`{"default":"x","harnesses":{"claude":{"adapter":"claude"}}}`), home, onlyClaude); err == nil {
		t.Error("ParseHarnesses accepted a default naming no harness")
	}
	h, err := ParseHarnesses([]byte(DefaultHarnessesJSON), home, onlyClaude)
	if err != nil || h.Default != "claude" || len(h.Harnesses) != 1 {
		t.Errorf("DefaultHarnessesJSON = %+v, %v", h, err)
	}
}

func TestReplaceLeavesNoTempFileWhenTheRenameFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "spaces.json")
	if err := os.MkdirAll(filepath.Join(target, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replace(target, []byte("x"), 0o600); err == nil {
		t.Fatal("want an error renaming over a directory")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "spaces.json" {
		t.Errorf("directory holds %v, want only the target", entries)
	}
}

func TestWriteFileKeepsASymlinkLinked(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "spaces.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "spaces.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(link, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link replaced: %v %v", info, err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new\n" {
		t.Errorf("target = %q, want new", b)
	}
	if b, _ := os.ReadFile(target + ".prev"); string(b) != "old\n" {
		t.Errorf("prev = %q, want old beside the target", b)
	}
}
