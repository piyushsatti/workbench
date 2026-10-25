package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// In newLayoutHarness, %5 is a bash pane tagged agent-tmux-owned in window @4, which carries the window tag:
// the one pane send may type into.

func sendArgs(pane, text string) string { return js(map[string]string{"pane": pane, "text": text}) }

func (f *fakeTmux) sendCalls() [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c[0] == "send-keys" {
			out = append(out, c)
		}
	}
	return out
}

func TestSendTypesLiterally(t *testing.T) {
	h := newLayoutHarness(t)
	for _, c := range []struct{ pane, text string }{
		{"%5", "echo hi"},
		{"3.0", "ls -la; echo done # inner ';' and '#' are literal here"},
		{"%5", "-n starts with a dash"},
		{"%5", "x"},
		{"%5", strings.Repeat("y", maxSendLen)},
	} {
		n := len(h.f.sendCalls())
		m := h.ok(t, "tmux_send", sendArgs(c.pane, c.text))
		if m["pane_id"] != "%5" || m["bytes"] != float64(len(c.text)) {
			t.Errorf("send %q: result %v", c.text, m)
		}
		got := h.f.sendCalls()[n:]
		want := []string{"send-keys", "-l", "-t", "%5", "--", c.text}
		if len(got) != 1 || !slices.Equal(got[0], want) {
			t.Errorf("send %q: argv %q, want exactly %q", c.text, got, want)
		}
	}
	if typed := h.f.pane("%5")["typed"]; strings.ContainsAny(typed, "\r\n") {
		t.Errorf("a line ending reached the pane: %q", typed)
	}
}

// Every send-keys agent-tmux issued, on every path, is literal and carries no key that could run the line.
func TestSendNeverPressesEnter(t *testing.T) {
	h := newLayoutHarness(t)
	h.call1(t, "tmux_send", sendArgs("%5", "make test"))
	h.call1(t, "tmux_send", sendArgs("3.0", "Enter"))
	h.call1(t, "tmux_send", sendArgs("%5", "C-m"))
	h.call1(t, "tmux_send", sendArgs("%5", "echo a\nb"))
	h.call1(t, "tmux_send", sendArgs("%5", "echo a\rb"))
	h.call1(t, "tmux_send", sendArgs("%5", "\x1b#"))
	h.call1(t, "tmux_send", sendArgs("%2", "x"))
	h.f.panes = append(h.f.panes, mkPane("830", "%30", "$1", "@4", 3, 1, false, ownedPaneOpt, "1"))
	h.call1(t, "tmux_send", sendArgs("%30", "y"))
	calls := h.f.sendCalls()
	if len(calls) != 4 {
		t.Fatalf("want 4 sends (the ASCII ones to owned panes), got %q", calls)
	}
	for _, c := range calls {
		if len(c) != 6 || c[1] != "-l" || c[2] != "-t" || !rePaneID.MatchString(c[3]) || c[4] != "--" {
			t.Errorf("send-keys argv outside the one allowed shape: %q", c)
		}
		if strings.ContainsFunc(c[5], func(r rune) bool { return r < 0x20 || r > 0x7e }) {
			t.Errorf("control or non-ASCII byte sent: %q", c)
		}
	}
	// Key names may appear only as the literal text of a -l send, where tmux types them letter by letter.
	for _, c := range h.f.calls {
		for i, a := range c {
			if (a == "Enter" || a == "C-m" || a == "C-j" || a == "KPEnter") && !(c[0] == "send-keys" && i == 5) {
				t.Errorf("key name %q in %q", a, c)
			}
		}
	}
}

// An owned pane split into the home window takes text, as one in an owned window does; an unowned one there
// does not.
func TestSendHomeWindow(t *testing.T) {
	h := newLayoutHarness(t)
	text, isErr := h.call1(t, "tmux_send", sendArgs("%2", "x"))
	wantCode(t, "unowned pane in the home window", text, isErr, "refused")
	h.f.pane("%2")[ownedPaneOpt] = "1"
	if m := h.ok(t, "tmux_send", sendArgs("%2", "echo hi")); m["pane_id"] != "%2" {
		t.Errorf("owned pane in the home window: %v", m)
	}
	if h.f.winOpts["@1"][ownedWindowOpt] != "" {
		t.Errorf("the home window must stay untagged")
	}
}

// One test per send rule: each case breaks exactly one rule on an otherwise sendable pane.
func TestSendRefusals(t *testing.T) {
	cases := []struct {
		label, pane, msg string
		setup            func(h *layoutHarness)
	}{
		{"pane not owned", "%5", "pane %5 is not agent-tmux-owned", func(h *layoutHarness) { h.f.pane("%5")[ownedPaneOpt] = "" }},
		{"home pane", "%1", "home pane", func(h *layoutHarness) {
			home := h.f.pane("%1")
			home[ownedPaneOpt], home["@claude_state"], home["pane_current_command"] = "1", "", "bash"
			h.f.setWinOpt("@1", ownedWindowOpt, "1")
		}},
		{"synchronize-panes on another pane", "%5", "synchronize-panes", func(h *layoutHarness) {
			h.f.panes = append(h.f.panes, mkPane("830", "%30", "$1", "@4", 3, 1, false, ownedPaneOpt, "1", "pane_synchronized", "1"))
		}},
		{"synchronize-panes on the pane", "%5", "synchronize-panes", func(h *layoutHarness) { h.f.pane("%5")["pane_synchronized"] = "1" }},
		{"running vim", "%5", `running "vim"`, func(h *layoutHarness) { h.f.pane("%5")["pane_current_command"] = "vim" }},
		{"running claude", "%5", `running "claude"`, func(h *layoutHarness) { h.f.pane("%5")["pane_current_command"] = "claude" }},
		{"running python3", "%5", `running "python3"`, func(h *layoutHarness) { h.f.pane("%5")["pane_current_command"] = "python3" }},
		{"no command", "%5", `running ""`, func(h *layoutHarness) { h.f.pane("%5")["pane_current_command"] = "" }},
		{"@claude_state set", "%5", "@claude_state", func(h *layoutHarness) { h.f.pane("%5")["@claude_state"] = "idle" }},
		{"in copy mode", "%5", "copy mode", func(h *layoutHarness) { h.f.pane("%5")["pane_in_mode"] = "1" }},
	}
	for _, c := range cases {
		h := newLayoutHarness(t)
		h.ok(t, "tmux_send", sendArgs("%5", "x")) // the harness starts out sendable
		n := len(h.f.sendCalls())
		c.setup(h)
		text, isErr := h.call1(t, "tmux_send", sendArgs(c.pane, "echo hi"))
		wantCode(t, c.label, text, isErr, "refused")
		if !strings.Contains(text, c.msg) {
			t.Errorf("%s: message %q lacks %q", c.label, text, c.msg)
		}
		if got := h.f.sendCalls()[n:]; len(got) != 0 {
			t.Errorf("%s: refused send reached tmux: %q", c.label, got)
		}
	}
}

func TestSendTextRules(t *testing.T) {
	h := newLayoutHarness(t)
	for _, c := range []struct{ text, msg string }{
		{"", "empty"},
		{strings.Repeat("a", maxSendLen+1), "longer than 1000"},
		{"café", "printable ASCII"},
		{"c1\u0085", "printable ASCII"},
		{"tab\there", "printable ASCII"},
		{"echo a\n", "printable ASCII"},
		{"echo a\r", "printable ASCII"},
		{"esc\x1b[A", "printable ASCII"},
		{"del\x7f", "printable ASCII"},
		{"nul\x00", "printable ASCII"},
		{"ls;", "tmux reads a trailing ';' as a command separator"},
		{";", "tmux reads a trailing ';' as a command separator"},
		{`echo \;`, "tmux reads a trailing ';' as a command separator"},
	} {
		text, isErr := h.call1(t, "tmux_send", sendArgs("%5", c.text))
		wantCode(t, fmt.Sprintf("send %q", c.text), text, isErr, "bad_input")
		if !strings.Contains(text, c.msg) {
			t.Errorf("send %q: message %q lacks %q", c.text, text, c.msg)
		}
	}
	for _, c := range []struct{ args, code string }{
		{`{"pane":"%5"}`, "bad_input"},
		{`{"text":"x"}`, "bad_input"},
		{`{"pane":"%5","text":"x","enter":true}`, "bad_input"},
		{`{"pane":"3","text":"x"}`, "bad_target"},
		{`{"pane":"@4","text":"x"}`, "bad_target"},
		{`{"pane":"%9","text":"x"}`, "out_of_scope"},
	} {
		text, isErr := h.call1(t, "tmux_send", c.args)
		wantCode(t, "send "+c.args, text, isErr, c.code)
	}
	if got := h.f.sendCalls(); len(got) != 0 {
		t.Errorf("rejected text reached tmux: %q", got)
	}
}

// The schema pattern and checkSendText must agree.
func TestSendTextSchemaMatchesCheck(t *testing.T) {
	re := regexp.MustCompile(patSendText)
	for _, v := range []string{"x", "a;b", "a;", ";", "-x", "#{x}", " ", "café", "a\tb", "a\x7f", "", `\;`} {
		if got, want := re.MatchString(v), checkSendText(v) == nil; got != want {
			t.Errorf("%q: schema %v, checkSendText %v", v, got, want)
		}
	}
}

// callHook changes the fake right after the n-th call of cmd.
type callHook struct {
	*fakeTmux
	cmd    string
	n, got int
	change func(f *fakeTmux)
}

func (c *callHook) Run(args []string) (string, error) {
	out, err := c.fakeTmux.Run(args)
	if args[0] == c.cmd {
		c.got++
		if c.got == c.n {
			c.locked(func() { c.change(c.fakeTmux) })
		}
	}
	return out, err
}

func TestSendRecheck(t *testing.T) {
	for _, c := range []struct {
		label, msg string
		change     func(f *fakeTmux)
	}{
		{"shell changed", "target changed while it was being checked; nothing was sent", func(f *fakeTmux) { f.pane("%5")["pane_current_command"] = "zsh" }},
		{"owned pane joined the window", "target changed while it was being checked; nothing was sent", func(f *fakeTmux) {
			f.panes = append(f.panes, mkPane("830", "%30", "$1", "@4", 3, 1, false, ownedPaneOpt, "1"))
		}},
		{"program started", `running "vim"`, func(f *fakeTmux) { f.pane("%5")["pane_current_command"] = "vim" }},
		{"tag removed", "not agent-tmux-owned", func(f *fakeTmux) { f.pane("%5")[ownedPaneOpt] = "" }},
	} {
		h := newLayoutHarness(t)
		// %5 here carries only the pre-2026.10.13 tag, so the first check reads @agent-tmux_owner (empty) and then
		// @agent-tmux_owned, its second show-options; the change lands after it.
		h.s.tm = tmux{&callHook{fakeTmux: h.f, cmd: "show-options", n: 2, change: c.change}}
		before := h.f.count("list-panes")
		text, isErr := h.call1(t, "tmux_send", sendArgs("%5", "echo hi"))
		wantCode(t, c.label, text, isErr, "refused")
		if !strings.Contains(text, c.msg) {
			t.Errorf("%s: message %q lacks %q", c.label, text, c.msg)
		}
		if n := h.f.count("list-panes") - before; n != 2 || len(h.f.sendCalls()) != 0 {
			t.Errorf("%s: want a check and a re-check and no send, got %d listings, sends %q", c.label, n, h.f.sendCalls())
		}
		data, _ := os.ReadFile(h.logPath)
		if lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"); len(lines) != 2 ||
			!strings.Contains(lines[0], "target=%5 result=intent") || !strings.Contains(lines[1], "target=%5 result=refused") {
			t.Errorf("%s: want intent then refused:\n%s", c.label, data)
		}
	}
}

func TestSendAudit(t *testing.T) {
	h := newLayoutHarness(t)
	secret := "export TOKEN=hunter2"
	reply, _ := h.call1(t, "tmux_send", sendArgs("%5", secret))
	h.call1(t, "tmux_send", sendArgs("%2", secret))
	h.call1(t, "tmux_send", sendArgs("%5", secret+";"))
	// The redactor runs before the reply and the event log, not only before the audit log.
	events, err := os.ReadFile(h.eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply, "hunter2") || strings.Contains(string(events), "hunter2") {
		t.Errorf("secret in the reply or the event log:\n%s\n%s", reply, events)
	}
	if !strings.Contains(reply, "export TOKEN=***") {
		t.Errorf("reply does not echo the redacted text: %s", reply)
	}
	data, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if strings.Contains(log, "hunter2") {
		t.Errorf("raw text in log:\n%s", log)
	}
	lines := strings.Split(strings.TrimSuffix(log, "\n"), "\n")
	hash := fmt.Sprintf("text=len:%d,sha256:%x", len(secret), sha256.Sum256([]byte(secret)))
	want := [][]string{
		{"tool=tmux_send target=%5 result=intent bytes=0", hash},
		{"tool=tmux_send target=%5 result=ok bytes=20", hash},
		{"tool=tmux_send target=%2 result=refused", hash},
		{"tool=tmux_send target=- result=bad_input"},
	}
	if len(lines) != len(want) {
		t.Fatalf("want %d lines, got:\n%s", len(want), log)
	}
	for i, ws := range want {
		for _, w := range ws {
			if !strings.Contains(lines[i], w) {
				t.Errorf("line %d lacks %q: %s", i, w, lines[i])
			}
		}
	}
}

// send types only when the pane's shell is at its prompt: it holds the terminal's foreground and has no claude
// under it. A program started from the shell, even `bash -c ...` that reads as bash, takes the foreground.
func TestSendShellAtPrompt(t *testing.T) {
	cases := []struct {
		label, msg string
		setup      func(t *testing.T, h *layoutHarness)
	}{
		{"bash -c in the foreground", "not at its prompt", func(t *testing.T, h *layoutHarness) {
			writeStat(t, h.proc, 2001, "bash", 802, 2001)
			setTPGID(t, h.proc, 802, 2001)
		}},
		{"no terminal", "not at its prompt", func(t *testing.T, h *layoutHarness) { setTPGID(t, h.proc, 802, -1) }},
		{"fields faked inside comm", "not at its prompt", func(t *testing.T, h *layoutHarness) {
			writeStat(t, h.proc, 802, "a) S 700 802 802 34816 802 (b", 700, 9999)
		}},
		{"claude under the shell", "claude process under its shell", func(t *testing.T, h *layoutHarness) {
			writeStat(t, h.proc, 2001, "node", 802, 802)
			writeStat(t, h.proc, 2002, "claude", 2001, 802)
		}},
		{"shell's stat gone", "cannot tell whether pane %5's shell is at its prompt", func(t *testing.T, h *layoutHarness) {
			if err := os.Remove(filepath.Join(h.proc, "802", "stat")); err != nil {
				t.Fatal(err)
			}
		}},
		{"tpgid not a number", "cannot tell whether", func(t *testing.T, h *layoutHarness) {
			if err := os.WriteFile(filepath.Join(h.proc, "802", "stat"), []byte("802 (bash) S 700 802 802 34816 x 0"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"stat cut short", "cannot tell whether", func(t *testing.T, h *layoutHarness) {
			if err := os.WriteFile(filepath.Join(h.proc, "802", "stat"), []byte("802 (bash) S 700 802"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"no process table", "cannot check the pane's processes", func(t *testing.T, h *layoutHarness) {
			h.s.procRoot = filepath.Join(t.TempDir(), "missing")
		}},
		// x=$(bash -c 'read -n1 c; echo $c'): the child joins the shell's group, and the shell keeps the terminal.
		{"command substitution child", "process 2001 runs in its process group", func(t *testing.T, h *layoutHarness) {
			writeGroupStat(t, h.proc, 2001, "bash", 802, 802, 802)
		}},
		{"grandchild in the shell's group", "process 2002 runs in its process group", func(t *testing.T, h *layoutHarness) {
			writeGroupStat(t, h.proc, 2001, "bash", 802, 2001, 802)
			writeGroupStat(t, h.proc, 2002, "cat", 2001, 802, 802)
		}},
		{"shell not its group's leader", "does not lead its own process group", func(t *testing.T, h *layoutHarness) {
			writeGroupStat(t, h.proc, 802, "bash", 700, 700, 802)
		}},
		{"pgrp not a number", "cannot check the pane's processes", func(t *testing.T, h *layoutHarness) {
			if err := os.MkdirAll(filepath.Join(h.proc, "2001"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(h.proc, "2001", "stat"), []byte("2001 (sleep) S 802 x 2001 34816 802 0"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"another stat cut short", "cannot check the pane's processes", func(t *testing.T, h *layoutHarness) {
			if err := os.MkdirAll(filepath.Join(h.proc, "2001"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(h.proc, "2001", "stat"), []byte("2001 (sleep) S 802"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		h := newLayoutHarness(t)
		c.setup(t, h)
		text, isErr := h.call1(t, "tmux_send", sendArgs("%5", "y"))
		wantCode(t, "send, "+c.label, text, isErr, "refused")
		if !strings.Contains(text, c.msg) {
			t.Errorf("%s: message %q lacks %q", c.label, text, c.msg)
		}
		if got := h.f.sendCalls(); len(got) != 0 {
			t.Errorf("%s: refused send reached tmux: %q", c.label, got)
		}
	}
	// Back at the prompt, with a background job and its child in the job's own group, a kernel thread in group 0
	// and a claude elsewhere, the send goes through.
	h := newLayoutHarness(t)
	writeStat(t, h.proc, 2001, "sleep", 802, 802)
	writeGroupStat(t, h.proc, 2003, "cat", 2001, 2001, 802)
	writeStat(t, h.proc, 2002, "claude", 700, 2002)
	writeGroupStat(t, h.proc, 2, "kthreadd", 0, 0, -1)
	h.ok(t, "tmux_send", sendArgs("%5", "y"))
}
