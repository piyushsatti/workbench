package main

import (
	"strings"
	"testing"
)

func TestParseStat(t *testing.T) {
	cases := []struct {
		in   string
		ppid int
		comm string
	}{
		{"1000 (bash) S 800 1000 1000 0", 800, "bash"},
		{"1000 (a) (b c) S 42 1 1", 42, "a) (b c"},
		{"1000 (x) S 7 (y) S 9) R 55 1 1", 55, "x) S 7 (y) S 9"},
		{"1000 (tmux: server) S 1 1 1", 1, "tmux: server"},
	}
	for _, c := range cases {
		ppid, comm, err := parseStat(c.in)
		if err != nil || ppid != c.ppid || comm != c.comm {
			t.Errorf("%q: got %d %q %v", c.in, ppid, comm, err)
		}
	}
	for _, bad := range []string{"", "1000 bash S 1", "1000 (bash)", "1000 (bash) S x", "1000 (bash) S -3"} {
		if _, _, err := parseStat(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestAncestryTrickyComm(t *testing.T) {
	root := fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1000: {950, "agent-tmux-mcp"}, 950: {800, ") (evil 1 2"}, 800: {700, "bash"}, 700: {1, "tmux"}})
	chain, parent, err := ancestors(root, 1000)
	if err != nil || parent != ") (evil 1 2" || len(chain) != 3 || chain[0] != 950 || chain[1] != 800 || chain[2] != 700 {
		t.Fatalf("chain %v parent %q err %v", chain, parent, err)
	}
	// The parent check refuses anything but "claude", tricky names included.
	if h := DetectHome(tmux{standardFake()}, root, 1000); h.Err == nil {
		t.Errorf("tricky parent comm must be refused: %+v", h)
	}
	root = fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1000: {950, "agent-tmux-mcp"}, 950: {800, "claude"}, 800: {700, "bash"}, 700: {1, "tmux"}})
	if h := DetectHome(tmux{standardFake()}, root, 1000); h.Err != nil || h.PaneID != "%1" || h.ParentComm != "claude" {
		t.Errorf("home through claude parent: %+v", h)
	}
}

func TestAncestryFailsClosed(t *testing.T) {
	// Broken link mid-chain.
	root := fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1000: {950, "agent-tmux-mcp"}})
	if h := DetectHome(tmux{standardFake()}, root, 1000); h.Err == nil {
		t.Errorf("missing parent stat must fail closed")
	}
	// No ancestor is a pane.
	root = fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1000: {950, "agent-tmux-mcp"}, 950: {1, "claude"}})
	if h := DetectHome(tmux{standardFake()}, root, 1000); h.Err == nil || !strings.Contains(h.Err.Error(), "no ancestor") {
		t.Errorf("no match: %+v", h)
	}
	// A pid loop never escapes the 64-hop cap.
	root = fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1000: {1001, "a"}, 1001: {1000, "b"}})
	if h := DetectHome(tmux{standardFake()}, root, 1000); h.Err == nil {
		t.Errorf("loop: want no home")
	}
}

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\x1b[1;31mred\x1b[0m":                       "red",
		"a\x1b]0;title\x07b":                         "ab",
		"a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b": "alinkb",
		"a\x1bPq#0;2;0\x1b\\b":                       "ab",
		"x\x1b(By":                                   "xy",
		"x\x1b7y\x1b8z":                              "xyz",
		"c1\u009b31mcsi\u009d0;t\u009cend":           "c1csiend",
		"tab\tnl\ncr\rbell\x07del\x7f":               "tab\tnl\ncrbelldel",
		"bad\xffutf8":                                "bad\uFFFDutf8",
		"unterminated \x1b[12":                       "unterminated ",
		"héllo ✓ 日本":                                 "héllo ✓ 日本",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckArgv(t *testing.T) {
	f := standardFake()
	tm := tmux{f}
	for _, args := range [][]string{{"display-message", ";"}, {"display-message", "-p", "a;"}, {"list-panes", "-t", "%1;"}, {}} {
		if _, err := tm.run(args...); err == nil {
			t.Errorf("%q: want rejection", args)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("runner reached despite rejection: %v", f.calls)
	}
	if _, err := (ExecRunner{Home: "/nonexistent"}).Run([]string{"kill-server", ";"}); err == nil || !strings.Contains(err.Error(), "';'") {
		t.Errorf("ExecRunner must reject before exec: %v", err)
	}
}

func TestParseRows(t *testing.T) {
	f := []string{"a", "b"}
	if rows, err := parseRows("T\x1f1\x1f2\x1fT\nT\x1f3\x1f4\x1fT\n", f, "T"); err != nil || len(rows) != 2 || rows[1]["b"] != "4" {
		t.Errorf("good rows: %v %v", rows, err)
	}
	for _, bad := range []string{
		"T\x1f1\x1fT\n", "T\x1f1\x1f2\x1f3\x1fT\n", "T\x1f1\x1f2\x1fT\n\n", "1\x1f2\n", "U\x1f1\x1f2\x1fU\n",
		// b holds a newline: its row ends early, and the line after it, forged whole, needs the token to pass.
		"T\x1f1\x1fx\x1fG\nG\x1f9\x1f9\x1fT\n",
	} {
		if _, err := parseRows(bad, f, "T"); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
	if !strings.HasPrefix(socketPath, "/tmp/tmux-") || !strings.HasSuffix(socketPath, "/default") {
		t.Errorf("socket path %s", socketPath)
	}
}

func TestClientPath(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"split-window", "-d", "-v", "-c", "/h", "-e", "PATH=/a:/b"}, "/a:/b"},
		{[]string{"new-window", "-d", "-e", "PATH=/a", "-n", "x"}, "/a"},
		{[]string{"new-window", "-d", "-c", "/h"}, "/usr/bin:/bin"},
		{[]string{"new-window", "-n", "PATH=/a"}, "/usr/bin:/bin"},
		{[]string{"new-window", "-e", "HOME=/x"}, "/usr/bin:/bin"},
		{[]string{"send-keys", "-e", "PATH=/a"}, "/usr/bin:/bin"},
		{[]string{"split-window", "-e"}, "/usr/bin:/bin"},
	} {
		if got := clientPath(c.args); got != c.want {
			t.Errorf("clientPath(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}
