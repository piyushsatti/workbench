package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// layoutHarness: standardFake plus a temp user home whose src dir is the home pane's cwd, window @4 tagged
// agent-tmux-owned, window 5 (@6) holding a human pane %6, and pty client /dev/pts/3 attached to the home session.
type layoutHarness struct {
	*harness
	home, src string
}

func newLayoutHarness(t *testing.T) *layoutHarness {
	t.Helper()
	h := newHarness(t)
	home := t.TempDir()
	src := filepath.Join(home, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	h.s.userHome = home
	h.f.panes[0]["pane_current_path"] = src
	h.f.setWinOpt("@4", ownedWindowOpt, "1")
	h.f.panes = append(h.f.panes, mkPane("803", "%6", "$1", "@6", 5, 0, true, "window_active", "0"))
	h.f.clients = []map[string]string{
		{"client_name": "/dev/pts/3", "session_id": "$1"},
		{"client_name": "client-77", "session_id": "$1"},
		{"client_name": "/dev/pts/4", "session_id": "$2"},
	}
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	return &layoutHarness{h, home, real}
}

func (h *harness) call1(t *testing.T, tool, args string) (string, bool) {
	t.Helper()
	return toolText(t, exchange(t, h.s, call(1, tool, args))[0])
}

func (h *harness) ok(t *testing.T, tool, args string) map[string]any {
	t.Helper()
	text, isErr := h.call1(t, tool, args)
	return wantOK(t, tool+" "+args, text, isErr)
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// mutations returns every call that changes tmux state or draws on a client.
func (f *fakeTmux) mutations() [][]string {
	var out [][]string
	for _, c := range f.calls {
		switch c[0] {
		case "new-window", "split-window", "set-option", "rename-window", "select-pane", "select-window",
			"resize-pane", "kill-pane", "kill-window", "send-keys":
			out = append(out, c)
		case "display-message":
			if !has(c, "-p") {
				out = append(out, c)
			}
		}
	}
	return out
}

func (f *fakeTmux) last(cmd string) []string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i][0] == cmd {
			return f.calls[i]
		}
	}
	return nil
}

func wantCode(t *testing.T, label, text string, isErr bool, code string) {
	t.Helper()
	if !isErr || !strings.HasPrefix(text, code+":") {
		t.Errorf("%s: want %s, got isError=%v %q", label, code, isErr, text)
	}
}

func wantOK(t *testing.T, label, text string, isErr bool) map[string]any {
	t.Helper()
	if isErr {
		t.Fatalf("%s: unexpected error %q", label, text)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("%s: result is not JSON: %q", label, text)
	}
	return m
}

var badStrings = []string{
	"", "caf\u00e9", "tab\there", "nl\nx", "esc\x1b[31m", "c1\u0085x", "del\x7f",
	"a#b", "#{session_name}", "#(id)", "end;", ";", "a;b", "-n", "-",
}

func TestTextRules(t *testing.T) {
	h := newLayoutHarness(t)
	type field struct{ tool, key, extra string }
	fields := []field{
		{"tmux_new_window", "name", ""},
		{"tmux_rename_window", "name", ""},
		{"tmux_label", "title", ""},
		{"tmux_note", "text", ""},
	}
	for _, f := range fields {
		over := strings.Repeat("a", maxNameLen+1)
		if f.key == "text" {
			over = strings.Repeat("a", maxNoteLen+1)
		}
		for _, v := range append(slices.Clone(badStrings), over) {
			text, isErr := h.call1(t, f.tool, js(map[string]string{f.key: v}))
			wantCode(t, fmt.Sprintf("%s %s=%q", f.tool, f.key, v), text, isErr, "bad_input")
		}
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("a rejected string reached tmux: %v", m)
	}
	for _, c := range []struct{ tool, args string }{
		{"tmux_rename_window", js(map[string]string{"name": strings.Repeat("a", maxNameLen)})},
		{"tmux_label", js(map[string]string{"title": `Build: "x" (1/2) $HOME ~ {} \ ok`})},
		{"tmux_note", js(map[string]string{"text": strings.Repeat("b", maxNoteLen)})},
		{"tmux_new_window", js(map[string]string{"name": "a-b_c.d"})},
	} {
		text, isErr := h.call1(t, c.tool, c.args)
		wantOK(t, c.tool+" at the limits", text, isErr)
	}
	for _, args := range []string{`{}`, `{"name":"x","extra":1}`, `{"name":5}`} {
		text, isErr := h.call1(t, "tmux_rename_window", args)
		wantCode(t, "rename "+args, text, isErr, "bad_input")
	}
}

func TestCheckTextMessages(t *testing.T) {
	cases := map[string]string{
		"":          "must not be empty",
		"caf\u00e9": "printable ASCII",
		"a\x1fb":    "printable ASCII",
		"a#b":       "'#'",
		"a;":        "';'",
		"a;b":       "';'",
		"-x":        "start with '-'",
		"abcdefgh":  "longer than 5",
	}
	for in, want := range cases {
		err := checkText("v", in, 5)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("checkText(%q) = %v, want %q", in, err, want)
		}
	}
	if err := checkText("v", "a b-c", 5); err != nil {
		t.Errorf("valid text rejected: %v", err)
	}
}

// The schema pattern and checkText must agree, so the client-side hint never contradicts the server.
func TestTextPatternMatchesCheckText(t *testing.T) {
	re := regexp.MustCompile(patText)
	for _, v := range append(slices.Clone(badStrings), "ok", "a-b", "x y", `"'$~{}\`, "@#", "0;", " lead", "!") {
		if got, want := re.MatchString(v), checkText("v", v, 1000) == nil; got != want {
			t.Errorf("%q: pattern %v, checkText %v", v, got, want)
		}
	}
}

func TestDirRules(t *testing.T) {
	h := newLayoutHarness(t)
	outside := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Symlink(outside, filepath.Join(h.home, "esc")))
	must(os.Symlink(filepath.Join(h.home, "src"), filepath.Join(h.home, "link")))
	must(os.Mkdir(filepath.Join(h.home, "a b"), 0o755))
	must(os.WriteFile(filepath.Join(h.home, "file"), nil, 0o644))
	cases := []struct{ dir, code, msg string }{
		{"src", "bad_input", "absolute"},
		{"~/src", "bad_input", "absolute"},
		{"", "bad_input", "absolute"},
		{h.home + "/src/../src", "bad_input", "'..'"},
		{h.home + "/a b", "bad_input", "absolute"},
		{h.home + "/x#y", "bad_input", "absolute"},
		{h.home + "/src;", "bad_input", "absolute"},
		{h.home + "/nope", "bad_input", "does not exist"},
		{h.home + "/file", "bad_input", "not a directory"},
		{h.home + "/esc", "out_of_scope", "resolves outside"},
		{"/etc", "out_of_scope", "inside the user's home"},
		{outside, "out_of_scope", "inside the user's home"},
		{"/" + strings.Repeat("a", maxDirLen), "bad_input", "absolute"},
	}
	for _, c := range cases {
		for _, tool := range []string{"tmux_new_window", "tmux_split"} {
			args := map[string]string{"dir": c.dir}
			if tool == "tmux_split" {
				args["direction"] = "h"
			}
			text, isErr := h.call1(t, tool, js(args))
			wantCode(t, tool+" dir="+c.dir, text, isErr, c.code)
			if !strings.Contains(text, c.msg) {
				t.Errorf("%s dir=%q: message %q lacks %q", tool, c.dir, text, c.msg)
			}
		}
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("a rejected dir reached tmux: %v", m)
	}
	// A symlink that stays inside home is passed on in canonical form.
	text, isErr := h.call1(t, "tmux_new_window", js(map[string]string{"dir": h.home + "/link/"}))
	if m := wantOK(t, "dir via inside symlink", text, isErr); m["dir"] != h.src || flagVal(h.f.last("new-window"), "-c") != h.src {
		t.Errorf("want canonical %s, got %v / %v", h.src, m["dir"], h.f.last("new-window"))
	}
}

func TestDefaultDirFromHomePane(t *testing.T) {
	h := newLayoutHarness(t)
	for _, bad := range []string{"/etc", h.home + "/a b", h.home + "/gone", ""} {
		h.f.panes[0]["pane_current_path"] = bad
		text, isErr := h.call1(t, "tmux_new_window", `{}`)
		if !isErr || !strings.Contains(text, "home pane's current directory") && !strings.Contains(text, "could not read") {
			t.Errorf("home cwd %q: want refusal, got %v %q", bad, isErr, text)
		}
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("an unusable home cwd reached tmux: %v", m)
	}
	h.s.userHome = ""
	text, isErr := h.call1(t, "tmux_new_window", js(map[string]string{"dir": h.src}))
	wantCode(t, "no user home", text, isErr, "refused")
}

func TestNewWindow(t *testing.T) {
	h := newLayoutHarness(t)
	m := h.ok(t, "tmux_new_window", `{"name":"build"}`)
	win, pane := m["window_id"].(string), m["pane_id"].(string)
	want := []string{"new-window", "-d", "-P", "-F", formatOf(createdFields), "-t", "$1:", "-c", h.src, "-e", "PATH=" + fakePath, "-n", "build"}
	if got := unframe(h.f.last("new-window")); !slices.Equal(got, want) {
		t.Errorf("new-window argv\n got %q\nwant %q", got, want)
	}
	p := h.f.pane(pane)
	owner := fakeServerPID + ":%1"
	if p == nil || p["session_id"] != "$1" || p["window_id"] != win || p[ownerOpt] != owner || p[ownedPaneOpt] != owner || p[nameOpt] != "build" {
		t.Fatalf("new pane not created and tagged in home: %v", p)
	}
	if h.f.winOpts[win][ownedWindowOpt] != "1" || h.f.winOpts[win][ownedPaneOpt] != "" {
		t.Errorf("window tags: %v", h.f.winOpts[win])
	}
	wantSets := [][]string{
		{"set-option", "-p", "-t", pane, "@agent-tmux_owner", owner},
		{"set-option", "-p", "-t", pane, "@agent-tmux_owned", owner},
		{"set-option", "-w", "-t", win, "@agent-tmux_owned_window", "1"},
		{"set-option", "-p", "-t", pane, "@agent-tmux_name", "build"},
	}
	var sets [][]string
	for _, c := range h.f.calls {
		if c[0] == "set-option" {
			sets = append(sets, c)
		}
	}
	if !slices.EqualFunc(sets, wantSets, slices.Equal) {
		t.Errorf("set-option calls %q", sets)
	}
	// Without a name: still -d and an explicit -c, and never a shell-command argument.
	h.ok(t, "tmux_new_window", "")
	want = []string{"new-window", "-d", "-P", "-F", formatOf(createdFields), "-t", "$1:", "-c", h.src, "-e", "PATH=" + fakePath}
	if got := unframe(h.f.last("new-window")); !slices.Equal(got, want) {
		t.Errorf("unnamed new-window argv %q", got)
	}
}

// A new pane gets the PATH a hand-made split would: the home session's, else the global one, never
// ExecRunner's bare /usr/bin:/bin. A PATH that is missing or unusable at both levels adds no -e.
func TestNewPanePath(t *testing.T) {
	for _, c := range []struct {
		name    string
		session map[string]string
		global  map[string]string
		want    string
	}{
		{"global only", nil, map[string]string{"PATH": fakePath}, "PATH=" + fakePath},
		{"session wins", map[string]string{"PATH": "/s/bin:/usr/bin"}, map[string]string{"PATH": fakePath}, "PATH=/s/bin:/usr/bin"},
		{"session removed", map[string]string{"PATH": "-"}, map[string]string{"PATH": fakePath}, "PATH=" + fakePath},
		{"bad session", map[string]string{"PATH": "/a\n/b"}, map[string]string{"PATH": fakePath}, "PATH=" + fakePath},
		{"ends in semicolon", nil, map[string]string{"PATH": "/usr/bin;"}, ""},
		{"empty", nil, map[string]string{"PATH": ""}, ""},
		{"unset", nil, nil, ""},
	} {
		h := newLayoutHarness(t)
		h.f.env = map[string]map[string]string{"$1": c.session, "-g": c.global}
		h.ok(t, "tmux_split", `{"direction":"v"}`)
		h.ok(t, "tmux_new_window", `{}`)
		for _, cmd := range []string{"split-window", "new-window"} {
			if got := flagVal(h.f.last(cmd), "-e"); got != c.want {
				t.Errorf("%s %s: -e %q, want %q", c.name, cmd, got, c.want)
			}
		}
	}
}

func TestNewPaneMisplaced(t *testing.T) {
	h := newLayoutHarness(t)
	h.f.misplace = "$2"
	for _, c := range []struct{ tool, args string }{{"tmux_new_window", `{}`}, {"tmux_split", `{"direction":"v"}`}} {
		text, isErr := h.call1(t, c.tool, c.args)
		wantCode(t, c.tool+" misplaced", text, isErr, "out_of_scope")
		if !strings.Contains(text, "untagged") {
			t.Errorf("message should say the pane was left untagged: %q", text)
		}
	}
	for _, c := range h.f.mutations() {
		if c[0] != "new-window" && c[0] != "split-window" {
			t.Errorf("misplaced pane must be left alone, got %q", c)
		}
	}
}

func TestSplit(t *testing.T) {
	h := newLayoutHarness(t)
	m := h.ok(t, "tmux_split", `{"direction":"h","percent":30}`)
	want := []string{"split-window", "-d", "-h", "-l", "30%", "-P", "-F", formatOf(createdFields), "-t", "%1", "-c", h.src, "-e", "PATH=" + fakePath}
	if got := unframe(h.f.last("split-window")); !slices.Equal(got, want) {
		t.Errorf("split argv\n got %q\nwant %q", got, want)
	}
	if p := h.f.pane(m["pane_id"].(string)); p == nil || p[ownerOpt] != fakeServerPID+":%1" || p["window_id"] != "@1" {
		t.Errorf("split pane: %v", p)
	}
	if h.f.winOpts["@1"][ownedWindowOpt] != "" {
		t.Errorf("split must not tag the window it splits into")
	}
	h.ok(t, "tmux_split", `{"target":"%5","direction":"v"}`)
	if got := h.f.last("split-window"); has(got, "-l") || flagVal(got, "-t") != "%5" || !has(got, "-v") {
		t.Errorf("split owned argv %q", got)
	}

	n := len(h.f.mutations())
	h.f.panes[1]["@claude_state"] = "idle"
	for _, c := range []struct{ args, code string }{
		{`{"target":"%2","direction":"h"}`, "refused"},
		{`{"target":"0.1","direction":"h"}`, "refused"},
		{`{"target":"%6","direction":"h"}`, "refused"},
		{`{"target":"%9","direction":"h"}`, "out_of_scope"},
		{`{"target":"@4","direction":"h"}`, "bad_target"},
		{`{"target":"3","direction":"h"}`, "bad_target"},
		{`{"direction":"x"}`, "bad_input"},
		{`{}`, "bad_input"},
		{`{"direction":"h","percent":9}`, "bad_input"},
		{`{"direction":"h","percent":91}`, "bad_input"},
		{`{"direction":"h","size":30}`, "bad_input"},
	} {
		text, isErr := h.call1(t, "tmux_split", c.args)
		wantCode(t, "split "+c.args, text, isErr, c.code)
	}
	h.f.panes[2]["pane_synchronized"] = "1"
	text, isErr := h.call1(t, "tmux_split", `{"target":"%5","direction":"h"}`)
	wantCode(t, "split in sync window", text, isErr, "refused")
	if !strings.Contains(text, "synchronize-panes") {
		t.Errorf("sync refusal message: %q", text)
	}
	if got := len(h.f.mutations()); got != n {
		t.Errorf("refused splits reached tmux: %v", h.f.mutations()[n:])
	}
}

// A human pane in a agent-tmux-owned window inherits the window tag but must never read as an owned pane. The window
// itself is owned only if all its panes are, so it reads as not owned.
func TestHumanPaneInOwnedWindow(t *testing.T) {
	h := newLayoutHarness(t)
	m := h.ok(t, "tmux_new_window", `{}`)
	win := m["window_id"].(string)
	h.f.panes = append(h.f.panes, mkPane("990", "%90", "$1", win, 9, 1, false))
	text, _ := h.call1(t, "tmux_list", "")
	var l struct {
		Windows []struct {
			ID    string `json:"id"`
			Owned bool   `json:"agent-tmux_owned"`
			Panes []struct {
				ID    string `json:"id"`
				Owned bool   `json:"agent-tmux_owned"`
			} `json:"panes"`
		} `json:"windows"`
	}
	if err := json.Unmarshal([]byte(text), &l); err != nil {
		t.Fatal(err)
	}
	for _, w := range l.Windows {
		for _, p := range w.Panes {
			if p.ID == "%90" && (p.Owned || w.Owned) {
				t.Errorf("human pane in owned window: window owned=%v pane owned=%v", w.Owned, p.Owned)
			}
		}
	}
	for _, c := range []struct{ tool, args string }{
		{"tmux_kill", `{"target":"%90"}`},
		{"tmux_kill", js(map[string]string{"target": win})},
		{"tmux_label", `{"target":"%90","title":"x"}`},
		{"tmux_resize", `{"target":"%90","width":40}`},
		{"tmux_split", `{"target":"%90","direction":"h"}`},
	} {
		text, isErr := h.call1(t, c.tool, c.args)
		wantCode(t, c.tool+" "+c.args, text, isErr, "refused")
	}
}

func TestRenameWindow(t *testing.T) {
	h := newLayoutHarness(t)
	h.ok(t, "tmux_rename_window", `{"name":"main"}`)
	if got, want := h.f.last("rename-window"), []string{"rename-window", "-t", "@1", "--", "main"}; !slices.Equal(got, want) {
		t.Errorf("rename argv %q", got)
	}
	h.ok(t, "tmux_rename_window", `{"target":"3","name":"x"}`)
	if got := flagVal(h.f.last("rename-window"), "-t"); got != "@4" {
		t.Errorf("index 3 must reach tmux as @4, got %s", got)
	}
	n := len(h.f.mutations())
	for _, c := range []struct{ args, code string }{
		{`{"target":"@6","name":"x"}`, "refused"},
		{`{"target":"5","name":"x"}`, "refused"},
		{`{"target":"@7","name":"x"}`, "out_of_scope"},
		{`{"target":"%5","name":"x"}`, "bad_target"},
		{`{"target":"0.1","name":"x"}`, "bad_target"},
	} {
		text, isErr := h.call1(t, "tmux_rename_window", c.args)
		wantCode(t, "rename "+c.args, text, isErr, c.code)
	}
	h.f.panes[2]["window_linked"] = "1"
	text, isErr := h.call1(t, "tmux_rename_window", `{"target":"@4","name":"x"}`)
	wantCode(t, "rename linked", text, isErr, "refused")
	if got := len(h.f.mutations()); got != n {
		t.Errorf("refused renames reached tmux: %v", h.f.mutations()[n:])
	}
}

func TestLabel(t *testing.T) {
	h := newLayoutHarness(t)
	h.ok(t, "tmux_label", `{"title":"agent"}`)
	if got, want := h.f.last("select-pane"), []string{"select-pane", "-T", "agent", "-t", "%1"}; !slices.Equal(got, want) {
		t.Errorf("label argv %q", got)
	}
	h.ok(t, "tmux_label", `{"target":"3.0","title":"w"}`)
	if got := flagVal(h.f.last("select-pane"), "-t"); got != "%5" {
		t.Errorf("3.0 must reach tmux as %%5, got %s", got)
	}
	n := len(h.f.mutations())
	for _, c := range []struct{ args, code string }{
		{`{"target":"%2","title":"x"}`, "refused"},
		{`{"target":"%6","title":"x"}`, "refused"},
		{`{"target":"%9","title":"x"}`, "out_of_scope"},
		{`{"target":"@4","title":"x"}`, "bad_target"},
		{`{"target":"%5"}`, "bad_input"},
	} {
		text, isErr := h.call1(t, "tmux_label", c.args)
		wantCode(t, "label "+c.args, text, isErr, c.code)
	}
	if got := len(h.f.mutations()); got != n {
		t.Errorf("refused labels reached tmux: %v", h.f.mutations()[n:])
	}
}

func TestNote(t *testing.T) {
	h := newLayoutHarness(t)
	m := h.ok(t, "tmux_note", `{"text":"build done"}`)
	want := []string{"display-message", "-l", "-d", "3000", "-c", "/dev/pts/3", "-t", "%1", "--", "build done"}
	notes := h.f.mutations()
	if len(notes) != 1 || !slices.Equal(notes[0], want) {
		t.Errorf("note calls %q, want one %q", notes, want)
	}
	if c := m["clients"].([]any); len(c) != 1 || c[0] != "/dev/pts/3" {
		t.Errorf("clients %v", m["clients"])
	}
	h.f.clients = h.f.clients[1:]
	text, isErr := h.call1(t, "tmux_note", `{"text":"x"}`)
	wantCode(t, "note without a pty client", text, isErr, "no_client")
	if len(h.f.mutations()) != 1 {
		t.Errorf("note drew without a home client: %v", h.f.mutations())
	}
}

func TestResize(t *testing.T) {
	h := newLayoutHarness(t)
	h.ok(t, "tmux_resize", `{"target":"%5","width":80,"height":20}`)
	if got, want := h.f.last("resize-pane"), []string{"resize-pane", "-t", "%5", "-x", "80", "-y", "20"}; !slices.Equal(got, want) {
		t.Errorf("resize argv %q", got)
	}
	h.ok(t, "tmux_resize", `{"target":"3.0","height":5}`)
	if got := h.f.last("resize-pane"); has(got, "-x") || flagVal(got, "-y") != "5" {
		t.Errorf("height-only argv %q", got)
	}
	n := len(h.f.mutations())
	h.f.panes[0][ownedPaneOpt] = "1"
	for _, c := range []struct{ args, code string }{
		{`{"target":"%1","width":80}`, "refused"},
		{`{"target":"%2","width":80}`, "refused"},
		{`{"target":"%9","width":80}`, "out_of_scope"},
		{`{"target":"@4","width":80}`, "bad_target"},
		{`{"target":"%5","width":4}`, "bad_input"},
		{`{"target":"%5","height":501}`, "bad_input"},
		{`{"target":"%5"}`, "bad_input"},
		{`{"width":80}`, "bad_input"},
	} {
		text, isErr := h.call1(t, "tmux_resize", c.args)
		wantCode(t, "resize "+c.args, text, isErr, c.code)
	}
	if got := len(h.f.mutations()); got != n {
		t.Errorf("refused resizes reached tmux: %v", h.f.mutations()[n:])
	}
}

func TestKill(t *testing.T) {
	h := newLayoutHarness(t)
	h.f.setWinOpt("@1", ownedWindowOpt, "1")
	h.f.panes[0][ownedPaneOpt] = "1"
	// Owned panes that are Claude: one reports @claude_state, one only shows claude as its command.
	h.f.panes = append(h.f.panes,
		mkPane("810", "%10", "$1", "@10", 7, 0, true, ownedPaneOpt, "1", "@claude_state", "idle"),
		mkPane("811", "%11", "$1", "@11", 8, 0, true, ownedPaneOpt, "1", "pane_current_command", "claude"),
		mkPane("812", "%12", "$1", "@12", 9, 0, true, ownedPaneOpt, "1", "window_linked", "1"),
	)
	h.f.setWinOpt("@10", ownedWindowOpt, "1")
	h.f.setWinOpt("@12", ownedWindowOpt, "1")
	for _, c := range []struct{ target, code, msg string }{
		{"%1", "refused", "home pane"},
		{"0.0", "refused", "home pane"},
		{"@1", "refused", "contains the home pane"},
		{"0", "refused", "contains the home pane"},
		{"%10", "refused", "@claude_state"},
		{"@10", "refused", "@claude_state"},
		{"%11", "refused", `running "claude"`},
		{"%2", "refused", "not agent-tmux-owned"},
		{"@6", "refused", "not agent-tmux-owned"},
		{"%12", "refused", "linked"},
		{"@12", "refused", "linked"},
		{"%9", "out_of_scope", ""},
		{"@7", "out_of_scope", ""},
		{"{last}", "bad_target", ""},
	} {
		text, isErr := h.call1(t, "tmux_kill", js(map[string]string{"target": c.target}))
		wantCode(t, "kill "+c.target, text, isErr, c.code)
		if !strings.Contains(text, c.msg) {
			t.Errorf("kill %s: message %q lacks %q", c.target, text, c.msg)
		}
	}
	// Mixed window: owned window, owned pane, plus one human pane.
	h.f.panes = append(h.f.panes, mkPane("820", "%20", "$1", "@4", 3, 1, false))
	for _, tg := range []string{"@4", "3"} {
		text, isErr := h.call1(t, "tmux_kill", js(map[string]string{"target": tg}))
		wantCode(t, "kill mixed "+tg, text, isErr, "refused")
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("refused kills reached tmux: %v", m)
	}
	h.f.remove(func(p map[string]string) bool { return p["pane_id"] != "%20" })

	h.ok(t, "tmux_kill", `{"target":"%5"}`)
	if got := h.f.last("kill-pane"); !slices.Equal(got, []string{"kill-pane", "-t", "%5"}) {
		t.Errorf("kill-pane argv %q", got)
	}
	m := h.ok(t, "tmux_new_window", `{}`)
	win := m["window_id"].(string)
	idx := h.f.pane(m["pane_id"].(string))["window_index"]
	h.ok(t, "tmux_kill", js(map[string]string{"target": idx}))
	if got := h.f.last("kill-window"); !slices.Equal(got, []string{"kill-window", "-t", win}) {
		t.Errorf("kill-window argv %q", got)
	}
}

// changingFake adds a pane to @4 after the first list-panes call: the kill re-check must notice.
type changingFake struct {
	*fakeTmux
	extra   map[string]string
	listed  int
	trigger int
}

func (c *changingFake) Run(args []string) (string, error) {
	out, err := c.fakeTmux.Run(args)
	if args[0] == "list-panes" {
		c.listed++
		if c.listed == c.trigger {
			c.panes = append(c.panes, c.extra)
		}
	}
	return out, err
}

func TestKillRecheck(t *testing.T) {
	for _, c := range []struct {
		label, msg string
		extra      map[string]string
	}{
		{"owned pane appears", "target changed", mkPane("830", "%30", "$1", "@4", 3, 1, false, ownedPaneOpt, "1")},
		{"human pane appears", "not agent-tmux-owned", mkPane("831", "%31", "$1", "@4", 3, 1, false)},
	} {
		h := newLayoutHarness(t)
		f := &changingFake{fakeTmux: h.f, extra: c.extra, trigger: 1}
		h.s.tm = tmux{f}
		text, isErr := h.call1(t, "tmux_kill", `{"target":"@4"}`)
		wantCode(t, c.label, text, isErr, "refused")
		if !strings.Contains(text, c.msg) {
			t.Errorf("%s: message %q lacks %q", c.label, text, c.msg)
		}
		if f.listed != 2 || len(h.f.mutations()) != 0 {
			t.Errorf("%s: listed %d times, mutations %v", c.label, f.listed, h.f.mutations())
		}
	}
}

func TestNoFocusChange(t *testing.T) {
	h := newLayoutHarness(t)
	for _, c := range []struct{ tool, args string }{
		{"tmux_new_window", `{"name":"a"}`},
		{"tmux_split", `{"direction":"h"}`},
		{"tmux_split", `{"target":"%5","direction":"v","percent":50}`},
		{"tmux_rename_window", `{"name":"b"}`},
		{"tmux_label", `{"title":"c"}`},
		{"tmux_note", `{"text":"d"}`},
		{"tmux_resize", `{"target":"%5","width":50}`},
		{"tmux_kill", `{"target":"%5"}`},
	} {
		text, isErr := h.call1(t, c.tool, c.args)
		wantOK(t, c.tool, text, isErr)
	}
	for _, c := range h.f.calls {
		switch c[0] {
		case "select-window", "switch-client", "last-pane", "last-window":
			t.Errorf("focus-changing command %q", c)
		case "select-pane":
			if !has(c, "-T") {
				t.Errorf("select-pane without -T %q", c)
			}
		case "new-window", "split-window":
			if !has(c, "-d") || flagVal(c, "-c") == "" {
				t.Errorf("creation without -d and -c %q", c)
			}
		}
	}
}

func TestLayoutAudit(t *testing.T) {
	h := newLayoutHarness(t)
	h.call1(t, "tmux_new_window", js(map[string]string{"name": "secret-name", "dir": h.src}))
	h.call1(t, "tmux_note", `{"text":"hello there"}`)
	h.call1(t, "tmux_rename_window", `{"name":"bad#name"}`)
	data, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	lines := strings.Split(strings.TrimSuffix(log, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("want 5 lines (intent+ok, intent+ok, rejection), got %d:\n%s", len(lines), log)
	}
	for _, raw := range []string{"secret-name", h.src, "hello there", "bad#name"} {
		if strings.Contains(log, raw) {
			t.Errorf("raw free text %q in log:\n%s", raw, log)
		}
	}
	hash := func(name, v string) string {
		return fmt.Sprintf("%s=len:%d,sha256:%x", name, len(v), sha256.Sum256([]byte(v)))
	}
	checks := []struct {
		line int
		want []string
	}{
		{0, []string{"tool=tmux_new_window target=$1 result=intent", hash("name", "secret-name"), hash("dir", h.src)}},
		{1, []string{"tool=tmux_new_window target=$1 result=ok", "new=@101,%101", hash("name", "secret-name")}},
		{2, []string{"tool=tmux_note target=/dev/pts/3 result=intent", hash("text", "hello there")}},
		{3, []string{"tool=tmux_note target=/dev/pts/3 result=ok"}},
		{4, []string{"tool=tmux_rename_window target=- result=bad_input", hash("name", "bad#name")}},
	}
	for _, c := range checks {
		for _, w := range c.want {
			if !strings.Contains(lines[c.line], w) {
				t.Errorf("line %d lacks %q: %s", c.line, w, lines[c.line])
			}
		}
	}
}

// mutatingCalls holds one call per tool that changes tmux, each valid on a fresh layoutHarness.
var mutatingCalls = []struct{ tool, args string }{
	{"tmux_new_window", `{}`},
	{"tmux_split", `{"direction":"h"}`},
	{"tmux_rename_window", `{"name":"x"}`},
	{"tmux_label", `{"title":"x"}`},
	{"tmux_note", `{"text":"x"}`},
	{"tmux_resize", `{"target":"%5","width":50}`},
	{"tmux_kill", `{"target":"%5"}`},
	{"tmux_send", `{"pane":"%5","text":"x"}`},
}

func logLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// A call cancelled before its intent line changes nothing.
func TestCancelBeforeIntent(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range mutatingCalls {
		h := newLayoutHarness(t)
		res := h.s.callTool(cancelled, c.tool, json.RawMessage(c.args))
		wantCode(t, "cancelled "+c.tool, res.Content[0].Text, res.IsError, "cancelled")
		if m := h.f.mutations(); len(m) != 0 {
			t.Errorf("%s: a cancelled call reached tmux: %v", c.tool, m)
		}
		if l := logLines(t, h.logPath); len(l) != 1 || !strings.Contains(l[0], "tool="+c.tool) || !strings.Contains(l[0], "result=cancelled") {
			t.Errorf("%s: want one cancelled line and no intent:\n%s", c.tool, strings.Join(l, "\n"))
		}
		// The same call uncancelled goes through, so the cancel is what stopped it.
		if res := h.s.callTool(context.Background(), c.tool, json.RawMessage(c.args)); res.IsError {
			t.Errorf("%s uncancelled: %s", c.tool, res.Content[0].Text)
		}
	}
}

// cancelAfterIntent cancels the call at its first tmux command after the intent line is written.
type cancelAfterIntent struct {
	*fakeTmux
	log    string
	cancel context.CancelFunc
}

func (c *cancelAfterIntent) Run(args []string) (string, error) {
	if b, _ := os.ReadFile(c.log); strings.Contains(string(b), "result=intent") {
		c.cancel()
	}
	return c.fakeTmux.Run(args)
}

// Past the intent line a cancel changes nothing: the call finishes, so no pane is left untagged and no intent
// line lacks its outcome.
func TestCancelAfterIntentCompletes(t *testing.T) {
	for _, c := range mutatingCalls {
		h := newLayoutHarness(t)
		ctx, cancel := context.WithCancel(context.Background())
		h.s.tm = tmux{&cancelAfterIntent{h.f, h.logPath, cancel}}
		res := h.s.callTool(ctx, c.tool, json.RawMessage(c.args))
		if res.IsError || ctx.Err() == nil {
			t.Errorf("%s: want ok after a cancel past intent, got %q (cancelled: %v)", c.tool, res.Content[0].Text, ctx.Err())
		}
		if l := logLines(t, h.logPath); len(l) != 2 || !strings.Contains(l[0], "result=intent") || !strings.Contains(l[1], "result=ok") {
			t.Errorf("%s: want intent then ok:\n%s", c.tool, strings.Join(l, "\n"))
		}
		cancel()
	}
}

// Each call gets a server-side number, shared by its intent and outcome lines. The client's id is not logged.
func TestAuditCallNumbers(t *testing.T) {
	h := newLayoutHarness(t)
	exchange(t, h.s,
		`{"jsonrpc":"2.0","id":"client-id-7f3a","method":"tools/call","params":{"name":"tmux_new_window","arguments":{}}}`,
		call(1, "tmux_whoami", ""),
		call(1, "tmux_send", sendArgs("%5", "x")),
		call(1, "tmux_kill", `{"target":"%5"}`),
		call(1, "tmux_nope", ""),
	)
	lines := logLines(t, h.logPath)
	want := []string{
		"call=1 home=%1 tool=tmux_new_window target=$1 result=intent",
		"call=1 home=%1 tool=tmux_new_window target=$1 result=ok",
		"call=2 home=%1 tool=tmux_whoami target=%1 result=ok",
		"call=3 home=%1 tool=tmux_send target=%5 result=intent",
		"call=3 home=%1 tool=tmux_send target=%5 result=ok",
		"call=4 home=%1 tool=tmux_kill target=%5 result=intent",
		"call=4 home=%1 tool=tmux_kill target=%5 result=ok",
		"call=5 home=%1 tool=unknown target=- result=no_tool",
	}
	if len(lines) != len(want) {
		t.Fatalf("want %d lines, got:\n%s", len(want), strings.Join(lines, "\n"))
	}
	pid := fmt.Sprintf(" pid=%d ", os.Getpid())
	for i, w := range want {
		if !strings.Contains(lines[i], pid+w+" ") {
			t.Errorf("line %d lacks %q: %s", i, pid+w, lines[i])
		}
		if strings.Contains(lines[i], "client-id") {
			t.Errorf("client id logged: %s", lines[i])
		}
	}
}

func TestLayoutAuditUnavailable(t *testing.T) {
	h := newLayoutHarness(t)
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h.s.audit = NewAudit(filepath.Join(notDir, "agent-tmux.log"))
	for _, c := range mutatingCalls {
		text, isErr := h.call1(t, c.tool, c.args)
		wantCode(t, c.tool, text, isErr, "audit_unavailable")
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("tmux changed while the audit log was unwritable: %v", m)
	}
}

func TestToolsListSchemas(t *testing.T) {
	h := newLayoutHarness(t)
	rs := exchange(t, h.s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var res struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				AdditionalProperties *bool                     `json:"additionalProperties"`
				Required             []string                  `json:"required"`
				Properties           map[string]map[string]any `json:"properties"`
			} `json:"inputSchema"`
			Annotations map[string]any `json:"annotations"`
		} `json:"tools"`
	}
	b, _ := json.Marshal(rs[0]["result"])
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	type spec struct {
		required []string
		props    []string
		texts    map[string]float64
	}
	want := map[string]spec{
		"tmux_new_window":    {nil, []string{"dir", "name"}, map[string]float64{"name": maxNameLen}},
		"tmux_split":         {[]string{"direction"}, []string{"dir", "direction", "name", "percent", "target"}, map[string]float64{"name": maxNameLen}},
		"tmux_rename_window": {[]string{"name"}, []string{"name", "target"}, map[string]float64{"name": maxNameLen}},
		"tmux_label":         {[]string{"title"}, []string{"target", "title"}, map[string]float64{"title": maxNameLen}},
		"tmux_note":          {[]string{"text"}, []string{"text"}, map[string]float64{"text": maxNoteLen}},
		"tmux_resize":        {[]string{"target"}, []string{"height", "target", "width"}, nil},
		"tmux_kill":          {[]string{"target"}, []string{"target"}, nil},
		"tmux_send":          {[]string{"pane", "text"}, []string{"pane", "text"}, nil},
	}
	seen := 0
	for _, td := range res.Tools {
		s := td.InputSchema
		if s.AdditionalProperties == nil || *s.AdditionalProperties {
			t.Errorf("%s: additionalProperties must be false", td.Name)
		}
		w, ok := want[td.Name]
		if !ok {
			continue
		}
		seen++
		var props []string
		for k := range s.Properties {
			props = append(props, k)
		}
		slices.Sort(props)
		if !slices.Equal(s.Required, w.required) || !slices.Equal(props, w.props) {
			t.Errorf("%s: required %v props %v", td.Name, s.Required, props)
		}
		for k, max := range w.texts {
			if s.Properties[k]["maxLength"] != max || s.Properties[k]["pattern"] != patText {
				t.Errorf("%s.%s: %v", td.Name, k, s.Properties[k])
			}
		}
		if p, ok := s.Properties["dir"]; ok && p["pattern"] != patDir {
			t.Errorf("%s.dir pattern %v", td.Name, p["pattern"])
		}
		if td.Annotations["readOnlyHint"] != false || td.Annotations["destructiveHint"] != (td.Name == "tmux_kill") {
			t.Errorf("%s annotations %v", td.Name, td.Annotations)
		}
	}
	if seen != len(want) {
		t.Errorf("tools/list has %d of %d phase 2 and 4 tools", seen, len(want))
	}
	for _, td := range res.Tools {
		p := td.InputSchema.Properties
		switch td.Name {
		case "tmux_send":
			if p["text"]["maxLength"] != float64(maxSendLen) || p["text"]["pattern"] != patSendText || p["pane"]["pattern"] != patPaneTarget {
				t.Errorf("tmux_send schema %v", p)
			}
			if !strings.Contains(td.Description, "Never presses Enter") {
				t.Errorf("tmux_send must say it never presses Enter: %q", td.Description)
			}
		case "tmux_wait":
			if !slices.Equal(td.InputSchema.Required, []string{"pane", "pattern"}) || td.Annotations["readOnlyHint"] != true ||
				p["pattern"]["maxLength"] != float64(maxPatternLen) || p["pattern"]["pattern"] != patPattern || p["pane"]["pattern"] != patPaneTarget ||
				p["timeout"]["maximum"] != float64(maxWaitSecs) || p["lines"]["maximum"] != float64(maxWaitLines) {
				t.Errorf("tmux_wait schema %v %v", td.InputSchema, td.Annotations)
			}
			if !strings.Contains(td.Description, "Returns at once if a line on screen already matches") {
				t.Errorf("tmux_wait must warn that a line already on screen matches at once: %q", td.Description)
			}
		}
		if td.Name == "tmux_capture" || td.Name == "tmux_wait" {
			if !strings.Contains(strings.ToLower(td.Description), "only the end line with the begin line's token ends the block") {
				t.Errorf("%s must say only the END line with the BEGIN line's token ends the block: %q", td.Name, td.Description)
			}
		}
	}
}

// agent-tmux never starts a program in its panes, so a pane running anything but a shell was started by the human.
func TestKillOnlyShells(t *testing.T) {
	h := newLayoutHarness(t)
	for _, cmd := range []string{"vim", "python3", "node", "", "claude", "bash2"} {
		h.f.pane("%5")["pane_current_command"] = cmd
		for _, tg := range []string{"%5", "@4"} {
			text, isErr := h.call1(t, "tmux_kill", js(map[string]string{"target": tg}))
			wantCode(t, "kill "+tg+" running "+cmd, text, isErr, "refused")
			if want := fmt.Sprintf("running %q", cmd); !strings.Contains(text, want) {
				t.Errorf("kill %s: message %q lacks %q", tg, text, want)
			}
		}
	}
	// One non-shell pane refuses the whole window.
	h.f.pane("%5")["pane_current_command"] = "bash"
	h.f.panes = append(h.f.panes, mkPane("830", "%30", "$1", "@4", 3, 1, false, ownedPaneOpt, "1", "pane_current_command", "htop"))
	text, isErr := h.call1(t, "tmux_kill", `{"target":"@4"}`)
	wantCode(t, "kill window with htop", text, isErr, "refused")
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("refused kills reached tmux: %v", m)
	}
	h.f.pane("%5")["pane_current_command"] = "sh"
	h.f.pane("%30")["pane_current_command"] = "zsh"
	h.ok(t, "tmux_kill", `{"target":"@4"}`)
}

func TestKillClaudeDescendant(t *testing.T) {
	type proc = struct {
		ppid int
		comm string
	}
	base := map[int]proc{1000: {950, "agent-tmux-mcp"}, 950: {800, "claude"}, 800: {700, "bash"}, 700: {1, "tmux: server"}, 802: {700, "bash"}}
	with := func(extra map[int]proc) map[int]proc {
		m := map[int]proc{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	h := newLayoutHarness(t)
	// %5's shell (pid 802) has a background job whose child is claude.
	h.s.procRoot = fakeProc(t, with(map[int]proc{2000: {802, "node"}, 2001: {2000, "claude"}, 2002: {802, "sleep"}}))
	for _, tg := range []string{"%5", "3.0", "@4", "3"} {
		text, isErr := h.call1(t, "tmux_kill", js(map[string]string{"target": tg}))
		wantCode(t, "kill "+tg+" with claude below", text, isErr, "refused")
		if !strings.Contains(text, "claude process under its shell") {
			t.Errorf("kill %s: message %q", tg, text)
		}
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("refused kills reached tmux: %v", m)
	}
	// Unreadable or oversized process tables fail closed.
	h.s.procRoot = filepath.Join(t.TempDir(), "missing")
	text, isErr := h.call1(t, "tmux_kill", `{"target":"%5"}`)
	wantCode(t, "kill without /proc", text, isErr, "refused")
	big := t.TempDir()
	for pid := 1; pid <= maxProcs+1; pid++ {
		if err := os.Mkdir(filepath.Join(big, fmt.Sprint(pid)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h.s.procRoot = big
	text, isErr = h.call1(t, "tmux_kill", `{"target":"%5"}`)
	wantCode(t, "kill past the process cap", text, isErr, "refused")
	if !strings.Contains(text, fmt.Sprintf("more than %d processes", maxProcs)) {
		t.Errorf("cap message %q", text)
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("refused kills reached tmux: %v", m)
	}
	// Other programs under the shell, and a claude elsewhere on the machine, do not refuse.
	h.s.procRoot = fakeProc(t, with(map[int]proc{2000: {802, "node"}, 2001: {2000, "claude-helper"}, 2002: {700, "claude"}}))
	h.ok(t, "tmux_kill", `{"target":"%5"}`)
}

// Ownership grants an action only when the tag is set on the pane or window itself. A -F format would also see
// a tag inherited from the window or set globally by hand, and tmux_list shows that view, but no tool acts on it.
func TestOwnershipNotInherited(t *testing.T) {
	for _, c := range []struct {
		label   string
		tag     func(f *fakeTmux)
		winSeen bool
	}{
		{"global tags", func(f *fakeTmux) { f.globalOpts = map[string]string{ownedPaneOpt: "1", ownedWindowOpt: "1"} }, true},
		{"pane tag set on the window", func(f *fakeTmux) { f.setWinOpt("@6", ownedPaneOpt, "1") }, true},
	} {
		h := newLayoutHarness(t)
		c.tag(h.f)
		text, _ := h.call1(t, "tmux_list", "")
		var l struct {
			Windows []struct {
				ID    string `json:"id"`
				Owned bool   `json:"agent-tmux_owned"`
				Panes []struct {
					Owned bool `json:"agent-tmux_owned"`
				} `json:"panes"`
			} `json:"windows"`
		}
		if err := json.Unmarshal([]byte(text), &l); err != nil {
			t.Fatal(err)
		}
		for _, w := range l.Windows {
			if w.ID == "@6" && (!w.Panes[0].Owned || w.Owned != c.winSeen) {
				t.Fatalf("%s: the inherited view should show %%6 as owned: %s", c.label, text)
			}
		}
		calls := []struct{ tool, args string }{
			{"tmux_kill", `{"target":"%6"}`},
			{"tmux_kill", `{"target":"5.0"}`},
			{"tmux_kill", `{"target":"@6"}`},
			{"tmux_send", `{"pane":"%6","text":"x"}`},
			{"tmux_wait", `{"pane":"%6","pattern":"x"}`},
			{"tmux_label", `{"target":"%6","title":"x"}`},
			{"tmux_resize", `{"target":"%6","width":40}`},
			{"tmux_split", `{"target":"%6","direction":"h"}`},
			{"tmux_rename_window", `{"target":"@6","name":"x"}`},
		}
		for _, k := range calls {
			text, isErr := h.call1(t, k.tool, k.args)
			wantCode(t, c.label+": "+k.tool+" "+k.args, text, isErr, "refused")
		}
		if m := h.f.mutations(); len(m) != 0 || h.f.count("capture-pane") != 0 {
			t.Fatalf("%s: inherited ownership reached tmux: %v", c.label, m)
		}
		for _, k := range h.f.calls {
			if k[0] == "show-options" && (has(k, "-A") || has(k, "-g")) {
				t.Errorf("%s: ownership read through inheritance: %q", c.label, k)
			}
		}
		// The same calls succeed once the tags sit on the pane and the window themselves.
		h.f.pane("%6")[ownedPaneOpt] = "1"
		h.f.setWinOpt("@6", ownedWindowOpt, "1")
		h.f.pane("%6")["content"] = "x\n"
		for _, k := range calls[3:] {
			var text string
			var isErr bool
			if k.tool == "tmux_wait" {
				text, isErr = liveCall(t, h.s, k.tool, k.args, 2*time.Second)
			} else {
				text, isErr = h.call1(t, k.tool, k.args)
			}
			wantOK(t, c.label+", tagged: "+k.tool, text, isErr)
		}
		h.ok(t, "tmux_kill", `{"target":"@6"}`)
	}
}

// kill applies the same at-its-prompt rule as send, to every pane it would close.
func TestKillShellAtPrompt(t *testing.T) {
	for _, c := range []struct {
		label, msg string
		setup      func(t *testing.T, h *layoutHarness)
	}{
		{"job in the foreground", "not at its prompt", func(t *testing.T, h *layoutHarness) {
			writeStat(t, h.proc, 2001, "bash", 802, 2001)
			setTPGID(t, h.proc, 802, 2001)
		}},
		{"shell's stat gone", "cannot tell whether", func(t *testing.T, h *layoutHarness) {
			if err := os.Remove(filepath.Join(h.proc, "802", "stat")); err != nil {
				t.Fatal(err)
			}
		}},
		{"command substitution child", "process 2001 runs in its process group", func(t *testing.T, h *layoutHarness) {
			writeGroupStat(t, h.proc, 2001, "bash", 802, 802, 802)
		}},
		{"grandchild in the shell's group", "process 2002 runs in its process group", func(t *testing.T, h *layoutHarness) {
			writeGroupStat(t, h.proc, 2001, "bash", 802, 2001, 802)
			writeGroupStat(t, h.proc, 2002, "cat", 2001, 802, 802)
		}},
	} {
		h := newLayoutHarness(t)
		c.setup(t, h)
		for _, tg := range []string{"%5", "@4"} {
			text, isErr := h.call1(t, "tmux_kill", js(map[string]string{"target": tg}))
			wantCode(t, "kill "+tg+", "+c.label, text, isErr, "refused")
			if !strings.Contains(text, c.msg) {
				t.Errorf("kill %s, %s: message %q lacks %q", tg, c.label, text, c.msg)
			}
			// The refusal is audited against the resolved target.
			if ls := logLines(t, h.logPath); !strings.Contains(ls[len(ls)-1], "tool=tmux_kill target="+tg+" result=refused") {
				t.Errorf("kill %s, %s: audit line %q", tg, c.label, ls[len(ls)-1])
			}
		}
		if m := h.f.mutations(); len(m) != 0 {
			t.Fatalf("%s: refused kills reached tmux: %v", c.label, m)
		}
	}
}

// A window kill needs every pane in it to be owned, and nothing else: the window tag is no longer read, since a
// pane moved into or out of a window made it wrong.
func TestKillWindowNeedsAllPanesOwned(t *testing.T) {
	h := newLayoutHarness(t)
	h.f.setWinOpt("@4", ownedWindowOpt, "")
	h.f.panes = append(h.f.panes, mkPane("990", "%90", "$1", "@4", 3, 1, false))
	for _, tg := range []string{"@4", "3"} {
		text, isErr := h.call1(t, "tmux_kill", js(map[string]string{"target": tg}))
		wantCode(t, "kill window with a human pane "+tg, text, isErr, "refused")
		if !strings.Contains(text, "window @4 is not agent-tmux-owned") && !strings.Contains(text, "not agent-tmux-owned") {
			t.Errorf("kill %s: message %q", tg, text)
		}
	}
	if m := h.f.mutations(); len(m) != 0 {
		t.Fatalf("refused kills reached tmux: %v", m)
	}
	// With the human pane gone, the window goes without any window tag.
	h.f.remove(func(p map[string]string) bool { return p["pane_id"] != "%90" })
	h.ok(t, "tmux_kill", `{"target":"@4"}`)
}

// ownerCalls are calls that act only on a pane or window carrying its own tag, as it stands in layoutHarness.
var ownerCalls = []struct{ tool, args string }{
	{"tmux_kill", `{"target":"%5"}`},
	{"tmux_kill", `{"target":"@4"}`},
	{"tmux_send", `{"pane":"%5","text":"x"}`},
	{"tmux_wait", `{"pane":"%5","pattern":"x"}`},
	{"tmux_label", `{"target":"%5","title":"x"}`},
	{"tmux_resize", `{"target":"%5","width":40}`},
	{"tmux_split", `{"target":"%5","direction":"h"}`},
	{"tmux_rename_window", `{"target":"@4","name":"x"}`},
}

// Only the exact value "1" marks a pane or window as owned.
func TestOwnTagValue(t *testing.T) {
	for _, v := range []string{"0", "yes", "true", "on", " 1", "1 ", "01", "11"} {
		h := newLayoutHarness(t)
		h.f.pane("%5")[ownedPaneOpt] = v
		h.f.setWinOpt("@4", ownedWindowOpt, v)
		for _, c := range ownerCalls {
			text, isErr := h.call1(t, c.tool, c.args)
			wantCode(t, fmt.Sprintf("tag %q: %s %s", v, c.tool, c.args), text, isErr, "refused")
		}
		if m := h.f.mutations(); len(m) != 0 || h.f.count("capture-pane") != 0 {
			t.Fatalf("tag %q: acted on a pane not tagged \"1\": %v", v, m)
		}
	}
}

// failRunner fails every command named cmd with err.
type failRunner struct {
	*fakeTmux
	cmd string
	err error
}

func (f *failRunner) Run(args []string) (string, error) {
	if args[0] == f.cmd {
		f.locked(func() { f.calls = append(f.calls, append([]string(nil), args...)) })
		return "", f.err
	}
	return f.fakeTmux.Run(args)
}

// A tag that cannot be read counts as not set.
func TestOwnTagReadError(t *testing.T) {
	h := newLayoutHarness(t)
	h.s.tm = tmux{&failRunner{h.f, "show-options", errors.New("tmux show-options failed: server exited unexpectedly")}}
	for _, c := range ownerCalls {
		text, isErr := h.call1(t, c.tool, c.args)
		wantCode(t, c.tool+" "+c.args, text, isErr, "tmux_error")
		if !strings.Contains(text, "could not read @agent-tmux_owner") {
			t.Errorf("%s %s: message %q", c.tool, c.args, text)
		}
	}
	if m := h.f.mutations(); len(m) != 0 || h.f.count("capture-pane") != 0 {
		t.Fatalf("acted on a pane whose tag could not be read: %v", m)
	}
}

// Every refusal that comes after the target resolves is audited against the resolved ID, not "-". new_window's
// target, the home session, is known before any check.
func TestRefusalsAuditTarget(t *testing.T) {
	for _, c := range []struct {
		tool, args, target, result string
		setup                      func(t *testing.T, h *layoutHarness)
	}{
		{"tmux_kill", `{"target":"0.1"}`, "%2", "refused", nil},
		{"tmux_kill", `{"target":"0"}`, "@1", "refused", nil},
		{"tmux_kill", `{"target":"@6"}`, "@6", "refused", nil},
		{"tmux_kill", `{"target":"3.0"}`, "%5", "refused", func(t *testing.T, h *layoutHarness) { setTPGID(t, h.proc, 802, 2001) }},
		{"tmux_kill", `{"target":"%5"}`, "%5", "refused", func(t *testing.T, h *layoutHarness) {
			h.s.procRoot = filepath.Join(t.TempDir(), "missing")
		}},
		{"tmux_kill", `{"target":"@4"}`, "@4", "refused", func(t *testing.T, h *layoutHarness) {
			h.s.procRoot = filepath.Join(t.TempDir(), "missing")
		}},
		{"tmux_send", sendArgs("0.1", "x"), "%2", "refused", nil},
		{"tmux_send", sendArgs("%5", "x"), "%5", "refused", func(t *testing.T, h *layoutHarness) { setTPGID(t, h.proc, 802, 2001) }},
		{"tmux_split", `{"target":"%2","direction":"h"}`, "%2", "refused", nil},
		{"tmux_rename_window", `{"target":"@6","name":"x"}`, "@6", "refused", nil},
		{"tmux_label", `{"target":"%2","title":"x"}`, "%2", "refused", nil},
		{"tmux_resize", `{"target":"%2","width":50}`, "%2", "refused", nil},
		{"tmux_wait", `{"pane":"%2","pattern":"x"}`, "%2", "refused", nil},
		{"tmux_new_window", `{"dir":"/"}`, "$1", "out_of_scope", nil},
	} {
		h := newLayoutHarness(t)
		if c.setup != nil {
			c.setup(t, h)
		}
		text, isErr := h.call1(t, c.tool, c.args)
		wantCode(t, c.tool+" "+c.args, text, isErr, c.result)
		want := "tool=" + c.tool + " target=" + c.target + " result=" + c.result
		if ls := logLines(t, h.logPath); !strings.Contains(ls[len(ls)-1], want) {
			t.Errorf("%s %s: audit line %q lacks %q", c.tool, c.args, ls[len(ls)-1], want)
		}
	}
}
