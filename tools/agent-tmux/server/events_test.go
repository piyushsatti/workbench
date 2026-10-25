package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ownTag marks a pane as owned by the harness's agent the way create does: the owner string in both options.
func (h *layoutHarness) ownTag(id string) {
	p := h.f.pane(id)
	p[ownerOpt] = fakeServerPID + ":%1"
	p[ownedPaneOpt] = fakeServerPID + ":%1"
}

// movePane puts a pane in another session and window, as the human's move-window or join-pane would.
func (h *layoutHarness) movePane(id, sess, win, winIdx string) {
	p := h.f.pane(id)
	p["session_id"], p["window_id"], p["window_index"] = sess, win, winIdx
	p["session_name"], p["window_name"] = "s"+sess[1:], "w"+win[1:]
}

func readEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("event line is not JSON: %q", l)
		}
		out = append(out, m)
	}
	return out
}

func evKinds(evs []map[string]any) string {
	var ks []string
	for _, e := range evs {
		ks = append(ks, fmt.Sprint(e["ev"], ":", e["pane"]))
	}
	return strings.Join(ks, " ")
}

// A reply is the logged record plus legacy keys, so every key of the last line must read the same in the reply.
func wantReplyIsRecord(t *testing.T, label string, reply map[string]any, last map[string]any) {
	t.Helper()
	for k, v := range last {
		if fmt.Sprint(reply[k]) != fmt.Sprint(v) {
			t.Errorf("%s: reply %s = %v, log line has %v", label, k, reply[k], v)
		}
	}
}

func TestEventLogFileAndReplies(t *testing.T) {
	h := newLayoutHarness(t)
	h.f.pane("%5")["content"] = "PANEOUTPUT-MUST-NOT-BE-LOGGED\n"
	h.ownTag("%5")

	created := h.ok(t, "tmux_new_window", `{"name":"build"}`)
	pane := created["pane_id"].(string)
	evs := readEvents(t, h.eventsPath)
	if len(evs) != 1 || evs[0]["ev"] != "created" || evs[0]["pane"] != pane {
		t.Fatalf("after new_window: %v", evs)
	}
	wantReplyIsRecord(t, "new_window", created, evs[0])
	for _, k := range []string{"name", "window", "session", "at", "size", "owner", "ts"} {
		if created[k] == nil || created[k] == "" {
			t.Errorf("new_window reply lacks %s: %v", k, created)
		}
	}
	if created["name"] != "build" || created["session"] != "s1" || !strings.HasSuffix(created["window"].(string), ":build") || created["size"] != "120x40" {
		t.Errorf("new_window reply: %v", created)
	}

	split := h.ok(t, "tmux_split", `{"target":"%5","direction":"h"}`)
	evs = readEvents(t, h.eventsPath)
	wantReplyIsRecord(t, "split", split, evs[len(evs)-1])
	if split["pos"] != "right of %5" || split["name"] != "pane-"+split["pane_id"].(string)[1:] {
		t.Errorf("split reply: %v", split)
	}

	sent := h.ok(t, "tmux_send", sendArgs("%5", "make test"))
	evs = readEvents(t, h.eventsPath)
	wantReplyIsRecord(t, "send", sent, evs[len(evs)-1])
	if sent["ev"] != "typed" || sent["cmd"] != "make test" || sent["state"] != "waiting_for_enter" || sent["pane"] != "%5" {
		t.Errorf("send reply: %v", sent)
	}

	label := h.ok(t, "tmux_label", `{"target":"%5","title":"tests"}`)
	evs = readEvents(t, h.eventsPath)
	wantReplyIsRecord(t, "label", label, evs[len(evs)-1])
	if label["ev"] != "labelled" || label["name"] != "tests" || h.f.pane("%5")[nameOpt] != "tests" {
		t.Errorf("label reply: %v, name option %q", label, h.f.pane("%5")[nameOpt])
	}

	h.f.pane(pane)["content"] = ""
	killed := h.ok(t, "tmux_kill", js(map[string]string{"target": pane}))
	evs = readEvents(t, h.eventsPath)
	last := evs[len(evs)-1]
	if killed["killed"] != pane || last["ev"] != "killed" || last["pane"] != pane {
		t.Errorf("kill reply %v, last event %v", killed, last)
	}
	if got, _ := killed["events"].([]any); len(got) != 1 || fmt.Sprint(got[0].(map[string]any)["pane"]) != pane {
		t.Errorf("kill reply events: %v", killed["events"])
	}

	// One line per event, no pane output, and a 0600 file.
	raw, _ := os.ReadFile(h.eventsPath)
	if strings.Contains(string(raw), "PANEOUTPUT") {
		t.Errorf("pane output in the event log:\n%s", raw)
	}
	if n := strings.Count(string(raw), "\n"); n != len(evs) {
		t.Errorf("%d newlines for %d events", n, len(evs))
	}
	st, err := os.Stat(h.eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("event log mode %v, want 0600", st.Mode().Perm())
	}
	for _, e := range evs {
		if e["ts"] == nil || e["ev"] == nil || e["owner"] != "%1" {
			t.Errorf("event lacks ts, ev or owner: %v", e)
		}
	}
}

func TestEventLogAppendsAndRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "events.jsonl")
	l := NewEventLog(path)
	for i := 0; i < 3; i++ {
		if err := l.Append(record{TS: "t", Ev: "created", Pane: "%1"}); err != nil {
			t.Fatal(err)
		}
	}
	if evs := readEvents(t, path); len(evs) != 3 {
		t.Errorf("want 3 lines, got %d", len(evs))
	}
	// A very long command is cut so the line stays under the size one write keeps whole.
	if err := l.Append(record{TS: "t", Ev: "typed", Pane: "%1", Cmd: strings.Repeat("x", 6000)}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if len(line) > maxEventLine {
			t.Errorf("line of %d bytes", len(line))
		}
	}
	// The log is never followed through a symlink.
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := NewEventLog(link).Append(record{TS: "t", Ev: "created"}); err == nil {
		t.Errorf("appended through a symlink")
	}
	if NewEventLog("") != nil {
		t.Errorf("an empty path should give no log")
	}
	if err := (*EventLog)(nil).Append(record{}); err == nil {
		t.Errorf("a nil log should report that it did not write")
	}
}

func TestEventLogPath(t *testing.T) {
	for _, c := range []struct{ home, override, want string }{
		{"/home/u", "", "/home/u/.local/state/agent-tmux/events.jsonl"},
		{"/home/u", "/tmp/x/e.jsonl", "/tmp/x/e.jsonl"},
		{"/home/u", "rel/e.jsonl", "/home/u/.local/state/agent-tmux/events.jsonl"},
		{"", "", ""},
		{"", "/tmp/x/e.jsonl", "/tmp/x/e.jsonl"},
	} {
		if got := eventLogPath(c.home, c.override); got != c.want {
			t.Errorf("eventLogPath(%q, %q) = %q, want %q", c.home, c.override, got, c.want)
		}
	}
}

// A log that cannot be written never undoes or refuses what tmux already did; the reply says so.
func TestEventLogFailureDoesNotRefuse(t *testing.T) {
	h := newLayoutHarness(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h.s.events = NewEventLog(filepath.Join(blocker, "events.jsonl"))
	m := h.ok(t, "tmux_new_window", `{}`)
	if m["event_logged"] != false || h.f.pane(m["pane_id"].(string)) == nil {
		t.Errorf("new_window with a broken log: %v", m)
	}
	h.ownTag("%5")
	m = h.ok(t, "tmux_send", sendArgs("%5", "echo hi"))
	if m["event_logged"] != false || h.f.pane("%5")["typed"] != "echo hi" {
		t.Errorf("send with a broken log: %v", m)
	}
	// No log at all (the server's default in NewServer) is the same.
	h.s.events = nil
	if m := h.ok(t, "tmux_label", `{"target":"%5","title":"x"}`); m["event_logged"] != false {
		t.Errorf("label with no log: %v", m)
	}
	if _, err := os.Stat(h.eventsPath); err == nil {
		t.Errorf("the harness log was written while it was replaced")
	}
}

// An owned pane stays owned wherever the human puts it, and every tool finds it by %N.
func TestOwnershipFollowsPane(t *testing.T) {
	h := newLayoutHarness(t)
	h.ownTag("%5")
	h.movePane("%5", "$2", "@20", "4")
	h.f.pane("%5")["content"] = "x\ny\n"

	if m := h.ok(t, "tmux_send", sendArgs("%5", "echo hi")); m["session"] != "s2" || m["window"] != "4:w20" || m["at"] != "4.0" {
		t.Errorf("send reply does not say where the pane is: %v", m)
	}
	text, isErr := h.call1(t, "tmux_capture", `{"pane":"%5"}`)
	if isErr || !strings.Contains(text, "x\ny") {
		t.Errorf("capture of a moved pane: %v %q", isErr, text)
	}
	if w := waitOK(t, h.s, `{"pane":"%5","pattern":"^y$","timeout":2}`, 3*time.Second); !w.Matched {
		t.Errorf("wait on a moved pane: %+v", w)
	}
	if m := h.ok(t, "tmux_label", `{"target":"%5","title":"moved"}`); m["session"] != "s2" {
		t.Errorf("label: %v", m)
	}
	h.ok(t, "tmux_resize", `{"target":"%5","width":40}`)
	sp := h.ok(t, "tmux_split", `{"target":"%5","direction":"v"}`)
	np := h.f.pane(sp["pane_id"].(string))
	if np["session_id"] != "$2" || np["window_id"] != "@20" || np[ownerOpt] != fakeServerPID+":%1" {
		t.Errorf("split of a moved pane: %v", np)
	}

	// The list shows it under owned_elsewhere.
	var l struct {
		Elsewhere []struct {
			ID, Name, Session, Window, At string
		} `json:"owned_elsewhere"`
	}
	text, _ = h.call1(t, "tmux_list", "")
	if err := json.Unmarshal([]byte(text), &l); err != nil {
		t.Fatal(err)
	}
	ids := ""
	for _, e := range l.Elsewhere {
		ids += e.ID + " "
	}
	if len(l.Elsewhere) != 2 || !strings.Contains(ids, "%5 ") || !strings.Contains(ids, sp["pane_id"].(string)) {
		t.Errorf("owned_elsewhere: %s", text)
	}

	k := h.ok(t, "tmux_kill", `{"target":"%5"}`)
	if k["killed"] != "%5" || h.f.pane("%5") != nil {
		t.Errorf("kill of a moved pane: %v", k)
	}
}

// What does not count as ownership, in another session: the same out_of_scope as for a pane that does not exist.
func TestForeignPaneRefusals(t *testing.T) {
	for _, c := range []struct {
		label string
		setup func(h *layoutHarness)
	}{
		{"another agent's tag", func(h *layoutHarness) { h.f.pane("%5")[ownerOpt] = fakeServerPID + ":%77" }},
		{"stale server pid", func(h *layoutHarness) { h.f.pane("%5")[ownerOpt] = "9999:%1" }},
		{"owner string without a pid", func(h *layoutHarness) { h.f.pane("%5")[ownerOpt] = "%1" }},
		{"legacy tag outside the home session", func(h *layoutHarness) {
			h.f.pane("%5")[ownerOpt], h.f.pane("%5")[ownedPaneOpt] = "", "1"
		}},
		{"global tag only", func(h *layoutHarness) {
			h.f.pane("%5")[ownerOpt], h.f.pane("%5")[ownedPaneOpt] = "", ""
			h.f.globalOpts = map[string]string{ownerOpt: fakeServerPID + ":%1", ownedPaneOpt: "1"}
		}},
		{"no tag", func(h *layoutHarness) { h.f.pane("%5")[ownerOpt], h.f.pane("%5")[ownedPaneOpt] = "", "" }},
	} {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.movePane("%5", "$2", "@20", "4")
		c.setup(h)
		for _, k := range []struct{ tool, args string }{
			{"tmux_send", `{"pane":"%5","text":"x"}`},
			{"tmux_capture", `{"pane":"%5"}`},
			{"tmux_wait", `{"pane":"%5","pattern":"x"}`},
			{"tmux_label", `{"target":"%5","title":"x"}`},
			{"tmux_resize", `{"target":"%5","width":40}`},
			{"tmux_split", `{"target":"%5","direction":"h"}`},
			{"tmux_kill", `{"target":"%5"}`},
		} {
			text, isErr := h.call1(t, k.tool, k.args)
			wantCode(t, c.label+": "+k.tool, text, isErr, "out_of_scope")
		}
		if m := h.f.mutations(); len(m) != 0 || h.f.count("capture-pane") != 0 {
			t.Errorf("%s: reached tmux: %v", c.label, m)
		}
		// And it is not listed as this agent's.
		text, _ := h.call1(t, "tmux_list", "")
		if !strings.Contains(text, `"owned_elsewhere": []`) {
			t.Errorf("%s: listed as owned elsewhere: %s", c.label, text)
		}
	}
}

// The same tags in the home session: another agent's owner string or a stale pid is refused there too, and the
// legacy "1" still works there only.
func TestOwnerValueInHomeSession(t *testing.T) {
	for _, c := range []struct {
		label, owner, owned string
		want                bool
	}{
		{"this agent", fakeServerPID + ":%1", fakeServerPID + ":%1", true},
		{"another agent", fakeServerPID + ":%77", fakeServerPID + ":%77", false},
		{"stale pid", "9999:%1", "9999:%1", false},
		{"legacy", "", "1", true},
		{"half revoked: owner gone, owner string left", "", fakeServerPID + ":%1", false},
		{"half revoked: owned gone", fakeServerPID + ":%77", "", false},
	} {
		h := newLayoutHarness(t)
		h.f.pane("%5")[ownerOpt], h.f.pane("%5")[ownedPaneOpt] = c.owner, c.owned
		text, isErr := h.call1(t, "tmux_label", `{"target":"%5","title":"x"}`)
		if c.want {
			wantOK(t, c.label, text, isErr)
		} else {
			wantCode(t, c.label, text, isErr, "refused")
		}
	}
}

func TestForeignLinkedPaneRefused(t *testing.T) {
	h := newLayoutHarness(t)
	h.ownTag("%5")
	h.movePane("%5", "$2", "@20", "4")
	h.f.pane("%5")["window_linked"] = "1"
	text, isErr := h.call1(t, "tmux_send", `{"pane":"%5","text":"x"}`)
	wantCode(t, "linked foreign pane", text, isErr, "refused")
	if m := h.f.mutations(); len(m) != 0 {
		t.Errorf("reached tmux: %v", m)
	}
}

// A secret in the home session's neighbour stays unreachable: an unowned pane elsewhere reads as missing.
func TestForeignUnownedLooksMissing(t *testing.T) {
	h := newLayoutHarness(t)
	a, _ := h.call1(t, "tmux_capture", `{"pane":"%9"}`)
	b, _ := h.call1(t, "tmux_capture", `{"pane":"%88"}`)
	if a != b || !strings.HasPrefix(a, "out_of_scope") {
		t.Errorf("unowned %q vs missing %q", a, b)
	}
}

// New panes follow the home pane when it is moved to another session.
func TestCreateAfterHomeSessionChange(t *testing.T) {
	h := newLayoutHarness(t)
	h.f.panes[0]["session_id"] = "$2"
	h.f.panes[0]["session_name"] = "s2"
	m := h.ok(t, "tmux_new_window", `{}`)
	if m["session"] != "s2" || h.f.pane(m["pane_id"].(string))["session_id"] != "$2" {
		t.Errorf("new window after the home pane moved: %v", m)
	}
	if got := flagVal(h.f.last("new-window"), "-t"); got != "$2:" {
		t.Errorf("new-window -t %q", got)
	}
	sp := h.ok(t, "tmux_split", `{"direction":"h"}`)
	if sp["session"] != "s2" || sp["pos"] != "right of %1 (the home pane)" {
		t.Errorf("split after the home pane moved: %v", sp)
	}
}

// The sweep notices what the human did to an owned pane, once, and keys a move on ids.
func TestSweepEvents(t *testing.T) {
	h := newLayoutHarness(t)
	a := h.ok(t, "tmux_new_window", `{"name":"a"}`)["pane_id"].(string)
	b := h.ok(t, "tmux_new_window", `{"name":"b"}`)["pane_id"].(string)
	c := h.ok(t, "tmux_new_window", `{"name":"c"}`)["pane_id"].(string)
	h.call1(t, "tmux_list", "")
	if evs := readEvents(t, h.eventsPath); len(evs) != 3 {
		t.Fatalf("a quiet list must add no event: %s", evKinds(evs))
	}

	// A move to another window of the home session.
	h.f.pane(a)["window_id"], h.f.pane(a)["window_index"] = "@50", "7"
	h.call1(t, "tmux_list", "")
	evs := readEvents(t, h.eventsPath)
	mv := evs[len(evs)-1]
	if mv["ev"] != "moved" || mv["pane"] != a || !strings.HasSuffix(mv["to"].(string), ":7.0") || mv["from"] == mv["to"] {
		t.Fatalf("moved: %v", mv)
	}
	h.call1(t, "tmux_list", "")
	if n := len(readEvents(t, h.eventsPath)); n != len(evs) {
		t.Errorf("a move was reported twice")
	}

	// Renumbering changes the index and not the window: no move.
	h.f.pane(a)["window_index"] = "2"
	h.call1(t, "tmux_list", "")
	if n := len(readEvents(t, h.eventsPath)); n != len(evs) {
		t.Errorf("renumbering reported as a move: %s", evKinds(readEvents(t, h.eventsPath)))
	}

	// A move to another session.
	h.movePane(b, "$2", "@60", "1")
	h.call1(t, "tmux_list", "")
	evs = readEvents(t, h.eventsPath)
	if mv := evs[len(evs)-1]; mv["ev"] != "moved" || mv["pane"] != b || mv["session"] != "s2" {
		t.Errorf("cross-session move: %v", mv)
	}

	// The tag taken off: the pane is still there.
	h.f.pane(c)[ownerOpt] = ""
	h.call1(t, "tmux_list", "")
	evs = readEvents(t, h.eventsPath)
	if rv := evs[len(evs)-1]; rv["ev"] != "revoked" || rv["pane"] != c {
		t.Errorf("revoked: %v", rv)
	}

	// The pane gone: closed by someone unknown.
	h.f.remove(func(p map[string]string) bool { return p["pane_id"] != b })
	n := len(readEvents(t, h.eventsPath))
	h.call1(t, "tmux_list", "")
	h.call1(t, "tmux_list", "")
	evs = readEvents(t, h.eventsPath)
	if len(evs) != n+1 || evs[n]["ev"] != "closed" || evs[n]["pane"] != b || evs[n]["by"] != "unknown" {
		t.Errorf("closed: %s", evKinds(evs[n:]))
	}
}

// A kill agent-tmux made is reported once, as killed, and the sweep does not call it closed.
func TestKillNotReportedTwice(t *testing.T) {
	h := newLayoutHarness(t)
	p := h.ok(t, "tmux_new_window", `{}`)["pane_id"].(string)
	h.ok(t, "tmux_kill", js(map[string]string{"target": p}))
	h.call1(t, "tmux_list", "")
	h.call1(t, "tmux_list", "")
	n := 0
	for _, e := range readEvents(t, h.eventsPath) {
		if e["pane"] == p && e["ev"] != "created" {
			n++
			if e["ev"] != "killed" {
				t.Errorf("unexpected %v", e)
			}
		}
	}
	if n != 1 {
		t.Errorf("want one killed event, got %d: %s", n, evKinds(readEvents(t, h.eventsPath)))
	}
	// A window kill reports each of its panes.
	w := h.ok(t, "tmux_new_window", `{}`)
	h.ok(t, "tmux_split", js(map[string]string{"target": w["pane_id"].(string), "direction": "h"}))
	before := len(readEvents(t, h.eventsPath))
	k := h.ok(t, "tmux_kill", js(map[string]string{"target": w["window_id"].(string)}))
	h.call1(t, "tmux_list", "")
	evs := readEvents(t, h.eventsPath)
	if got, _ := k["events"].([]any); len(got) != 2 || len(evs) != before+2 {
		t.Errorf("window kill: reply events %v, log %s", k["events"], evKinds(evs[before:]))
	}
}

func TestSendPrivateAndAppended(t *testing.T) {
	h := newLayoutHarness(t)
	h.ownTag("%5")
	m := h.ok(t, "tmux_send", sendArgs("%5", " echo hush"))
	if m["cmd"] != "[private]" {
		t.Errorf("leading space: %v", m)
	}
	raw, _ := os.ReadFile(h.eventsPath)
	if strings.Contains(string(raw), "hush") {
		t.Errorf("private text in the log: %s", raw)
	}
	if m["appended_to_pending"] != nil {
		t.Errorf("first line is not appended: %v", m)
	}
	// The line is still on the screen, so a second send goes after it.
	h.f.pane("%5")["content"] = "$ echo hush\n"
	m = h.ok(t, "tmux_send", sendArgs("%5", "echo more"))
	if m["appended_to_pending"] != true {
		t.Errorf("second send should say it was appended: %v", m)
	}
}

func TestTypedMarker(t *testing.T) {
	typed := func(h *layoutHarness) string { return h.f.pane("%5")[typedOpt] }
	t.Run("set by send, shown by list, kept while the line is there", func(t *testing.T) {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.ok(t, "tmux_send", sendArgs("%5", "echo hi"))
		if typed(h) == "" {
			t.Fatalf("send did not set the marker")
		}
		h.f.pane("%5")["content"] = "$ echo hi\n\n"
		text, _ := h.call1(t, "tmux_list", "")
		if !strings.Contains(text, `"typed": "waiting_for_enter"`) || typed(h) == "" {
			t.Errorf("marker should stay while the line holds the text: %s", text)
		}
	})
	t.Run("cleared once the line is gone", func(t *testing.T) {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.ok(t, "tmux_send", sendArgs("%5", "echo hi"))
		h.f.pane("%5")["content"] = "$ echo hi\nhi\n$ \n"
		before := len(logLines(t, h.logPath))
		text, _ := h.call1(t, "tmux_list", "")
		if typed(h) != "" || strings.Contains(text, `"typed"`) {
			t.Errorf("marker should clear: %q %s", typed(h), text)
		}
		// The clearing is a tmux change, so its call wrote an intent line before it.
		ls := logLines(t, h.logPath)[before:]
		if len(ls) != 2 || !strings.Contains(ls[0], "tool=tmux_list") || !strings.Contains(ls[0], "result=intent") {
			t.Errorf("want intent then ok for the list, got %q", ls)
		}
		var unset int
		for _, c := range h.f.calls {
			if c[0] == "set-option" && has(c, "-u") {
				unset++
			}
		}
		if unset != 1 {
			t.Errorf("want one set-option -u, got %d", unset)
		}
	})
	t.Run("cleared when a program other than a shell runs", func(t *testing.T) {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.ok(t, "tmux_send", sendArgs("%5", "vim x"))
		h.f.pane("%5")["content"] = "$ vim x\n"
		h.f.pane("%5")["pane_current_command"] = "vim"
		h.call1(t, "tmux_list", "")
		if typed(h) != "" {
			t.Errorf("marker should clear when vim runs")
		}
	})
	t.Run("a quiet list changes nothing and writes no intent line", func(t *testing.T) {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.ok(t, "tmux_send", sendArgs("%5", "echo hi"))
		h.f.pane("%5")["content"] = "$ echo hi\n"
		before := len(h.f.mutations())
		n := len(logLines(t, h.logPath))
		h.call1(t, "tmux_list", "")
		if len(h.f.mutations()) != before {
			t.Errorf("a list changed tmux: %v", h.f.mutations()[before:])
		}
		if ls := logLines(t, h.logPath)[n:]; len(ls) != 1 || strings.Contains(ls[0], "intent") {
			t.Errorf("a quiet list should write one outcome line: %q", ls)
		}
	})
	t.Run("a marker with no text to compare is unverified and stays", func(t *testing.T) {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.f.pane("%5")[typedOpt] = "7"
		text, _ := h.call1(t, "tmux_list", "")
		if !strings.Contains(text, `"typed": "unverified"`) || typed(h) != "7" {
			t.Errorf("unverified: %q %s", typed(h), text)
		}
	})
	t.Run("a marker on a pane that is not ours is left alone", func(t *testing.T) {
		h := newLayoutHarness(t)
		h.f.pane("%2")[typedOpt] = "7"
		h.f.pane("%2")["pane_current_command"] = "vim"
		h.call1(t, "tmux_list", "")
		if h.f.pane("%2")[typedOpt] != "7" {
			t.Errorf("cleared the marker of a pane agent-tmux does not own")
		}
	})
}

// The sweep runs on a ticker inside Serve when the server asks for one, and stops with it.
func TestSweepTicker(t *testing.T) {
	h := newLayoutHarness(t)
	if h.s.sweepEvery != 0 {
		t.Fatalf("NewServer must not start a ticker by default")
	}
	pane := h.ok(t, "tmux_new_window", `{}`)["pane_id"].(string)
	h.s.sweepEvery = 5 * time.Millisecond
	l := serveLive(t, h.s)
	h.f.locked(func() { h.f.remove(func(p map[string]string) bool { return p["pane_id"] != pane }) })
	deadline := time.Now().Add(3 * time.Second)
	for {
		evs := readEvents(t, h.eventsPath)
		if last := evs[len(evs)-1]; last["ev"] == "closed" && last["pane"] == pane {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the ticker never reported the closed pane: %s", evKinds(evs))
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Serve returns, and with it the ticker goroutine, when stdin closes.
	l.close(t, 2*time.Second)
	n := len(readEvents(t, h.eventsPath))
	h.f.locked(func() { h.f.panes = append(h.f.panes, mkPane("991", "%91", "$1", "@91", 9, 0, false)) })
	time.Sleep(30 * time.Millisecond)
	if len(readEvents(t, h.eventsPath)) != n {
		t.Errorf("events after Serve returned")
	}
}

// A tag set by hand on the whole server reads as an owner tag on every pane in the listing, the home pane too. The
// sweep must not track any pane for it, or name the home pane in a revoked event when it is taken off.
func TestSweepIgnoresHandSetGlobalTag(t *testing.T) {
	h := newLayoutHarness(t)
	h.f.globalOpts = map[string]string{ownerOpt: fakeServerPID + ":%1"}
	h.call1(t, "tmux_list", "")
	h.call1(t, "tmux_list", "")
	h.f.globalOpts = nil
	h.call1(t, "tmux_list", "")
	if raw, err := os.ReadFile(h.eventsPath); err == nil {
		t.Errorf("a hand-set global tag produced events:\n%s", raw)
	}
}

// When the server pid cannot be read as digits, no owner string can be formed, and nothing may read as owned: not
// even a pane tagged with the string an unchecked empty pid would give.
func TestOwnerPidUnreadableFailsClosed(t *testing.T) {
	for _, pid := range []string{"", "abc", "12 3", "-1"} {
		h := newLayoutHarness(t)
		h.ownTag("%5")
		h.f.pane("%5")[ownerOpt] = pid + ":%1"
		h.f.pane("%5")[ownedPaneOpt] = pid + ":%1"
		for _, p := range h.f.panes {
			p["pid"] = pid
		}
		for _, c := range ownerCalls {
			text, isErr := h.call1(t, c.tool, c.args)
			if !isErr || !strings.HasPrefix(text, "refused:") && !strings.HasPrefix(text, "tmux_error:") {
				t.Errorf("pid %q: %s %s: want refused or tmux_error, got %v %q", pid, c.tool, c.args, isErr, text)
			}
		}
		if m := h.f.mutations(); len(m) != 0 || h.f.count("capture-pane") != 0 {
			t.Errorf("pid %q: reached tmux: %v", pid, m)
		}
		if text, isErr := h.call1(t, "tmux_list", ""); !isErr && !strings.Contains(text, `"owned_elsewhere": []`) {
			t.Errorf("pid %q: list shows owned panes: %s", pid, text)
		}
	}
}
