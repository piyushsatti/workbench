package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestProcTree(t *testing.T) {
	root := fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1: {0, "init"}, 10: {1, "tmux: server"}, 20: {10, "bash"}, 30: {20, "make"}, 40: {30, "sh"}, 50: {40, ") claude (x"},
		60: {10, "bash"}, 70: {60, "claude"}})
	// A process that exited between readdir and read leaves a directory without stat.
	if err := os.Mkdir(filepath.Join(root, "99"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	pt, err := readProcTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if pt.hasComm(20, "claude") {
		t.Errorf("a comm that only contains claude must not match")
	}
	if !pt.hasComm(60, "claude") || !pt.hasComm(70, "claude") || !pt.hasComm(10, "claude") {
		t.Errorf("claude below 60 not found")
	}
	if !pt.hasComm(20, ") claude (x") || pt.hasComm(30, "bash") {
		t.Errorf("descendant walk wrong")
	}
	// A ppid loop ends.
	loop := fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{5: {6, "a"}, 6: {5, "b"}})
	if pt, err := readProcTree(loop); err != nil || pt.hasComm(5, "claude") || !pt.hasComm(5, "b") {
		t.Errorf("loop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "20", "stat"), []byte("20 bash S"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readProcTree(root); err == nil || !strings.Contains(err.Error(), "pid 20") {
		t.Errorf("malformed stat must fail the scan: %v", err)
	}
}

// Only a process that exited mid-scan may be skipped. Any other unreadable stat fails the scan, and kill and
// send refuse, since the unread process could be a claude under the target shell.
func TestProcTreeUnreadableStat(t *testing.T) {
	for _, c := range []struct {
		label string
		spoil func(t *testing.T, stat string)
	}{
		{"stat is a directory", func(t *testing.T, stat string) {
			if err := os.Mkdir(stat, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"stat not readable", func(t *testing.T, stat string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads a mode 000 file")
			}
			if err := os.WriteFile(stat, []byte("4242 (claude) S 802 4242 4242 34816 802 0"), 0o000); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		h := newLayoutHarness(t)
		if err := os.Mkdir(filepath.Join(h.proc, "4242"), 0o755); err != nil {
			t.Fatal(err)
		}
		c.spoil(t, filepath.Join(h.proc, "4242", "stat"))
		if _, err := readProcTree(h.proc); err == nil {
			t.Errorf("%s: the scan must fail", c.label)
		}
		for _, k := range []struct{ tool, args, msg string }{
			{"tmux_kill", `{"target":"%5"}`, "cannot check the panes' processes"},
			{"tmux_kill", `{"target":"@4"}`, "cannot check the panes' processes"},
			{"tmux_send", `{"pane":"%5","text":"x"}`, "cannot check the pane's processes"},
		} {
			text, isErr := h.call1(t, k.tool, k.args)
			wantCode(t, c.label+": "+k.tool, text, isErr, "refused")
			if !strings.Contains(text, k.msg) {
				t.Errorf("%s: %s message %q lacks %q", c.label, k.tool, text, k.msg)
			}
		}
		if m := h.f.mutations(); len(m) != 0 {
			t.Fatalf("%s: acted without a full process table: %v", c.label, m)
		}
	}
}

// The real /proc parses: kernel threads sit in process group 0, and this test's own group is read right.
func TestProcTreeRealProc(t *testing.T) {
	// agent-tmux is Linux-only (it reads /proc); on other systems there is no real tree to read.
	if _, err := os.Stat(defaultProcRoot); os.IsNotExist(err) {
		t.Skipf("%s does not exist on %s", defaultProcRoot, runtime.GOOS)
	}
	procs, err := readProcTree(defaultProcRoot)
	if err != nil {
		t.Fatalf("reading %s: %v", defaultProcRoot, err)
	}
	if g, ok := procs.pgrp[os.Getpid()]; !ok || g != syscall.Getpgrp() {
		t.Errorf("own process group read as %d (present %v), want %d", g, ok, syscall.Getpgrp())
	}
}
