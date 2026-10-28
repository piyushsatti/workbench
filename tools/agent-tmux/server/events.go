package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Pane options agent-tmux writes. ownerOpt is the authority: "<tmux server pid>:<home pane id>", so a pane id reused
// by a restarted server, or a pane owned by another agent, does not read as owned. ownedPaneOpt (layout.go) carries
// the same value for the pane border and for rollback; only an exact "1" there is a pre-2026.10.13 tag.
const (
	ownerOpt = "@agent-tmux_owner"
	nameOpt  = "@agent-tmux_name"
	typedOpt = "@agent-tmux_typed"
)

// maxEventLine keeps every appended line under 4 KB so a single O_APPEND write stays whole.
const maxEventLine = 4000

var errNoEventLog = errors.New("event log path unknown")

// record is one event. The line appended to events.jsonl and the reply to a tool call are the same object, so
// they cannot disagree; a reply adds a few legacy keys beside it. A record carries ids, numbers, names agent-tmux
// validated and the redacted text of a command agent-tmux typed. It never carries pane output.
type record struct {
	TS    string `json:"ts"`
	Ev    string `json:"ev"`
	Pane  string `json:"pane,omitempty"`
	Owner string `json:"owner"`
	// Name is the pane's @agent-tmux_name. Window, Session and At come from tmux and pass through fixedText.
	Name    string `json:"name,omitempty"`
	Window  string `json:"window,omitempty"` // "3:jobs": index and name
	Session string `json:"session,omitempty"`
	At      string `json:"at,omitempty"`  // "3.2": window.pane number
	Pos     string `json:"pos,omitempty"` // in words: "right of %51"
	Size    string `json:"size,omitempty"`
	// SplitFrom and Direction describe a split; From and To a move or a rename.
	SplitFrom string `json:"split_from,omitempty"`
	Direction string `json:"direction,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	By        string `json:"by,omitempty"`
	Title     string `json:"title,omitempty"`
	Text      string `json:"text,omitempty"`
	// Cmd is the redacted text of a typed command, or "[private]" for one that began with a space.
	Cmd     string `json:"cmd,omitempty"`
	TypedID uint64 `json:"typed_id,omitempty"`
	State   string `json:"state,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	// Appended: the pane already held an earlier typed line that had not been run.
	Appended bool `json:"appended_to_pending,omitempty"`
}

// EventLog appends one JSON line per event to a file that is 0600 and written with O_APPEND, one Write per line.
// Several servers (one per Claude Code session) can append to the same file; a line is never split.
type EventLog struct {
	path string
	mu   sync.Mutex
}

func NewEventLog(path string) *EventLog {
	if path == "" {
		return nil
	}
	return &EventLog{path: path}
}

// marshalLine encodes v on one line without HTML escaping, so a reply reads the way the file does.
func marshalLine(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (l *EventLog) Append(r record) error {
	if l == nil {
		return errNoEventLog
	}
	line, err := marshalLine(r)
	if err != nil {
		return err
	}
	if len(line) > maxEventLine {
		// Only the command can be long; the rest is capped by the field rules.
		r.Cmd = r.Cmd[:min(len(r.Cmd), 200)] + "...[cut]"
		if line, err = marshalLine(r); err != nil || len(line) > maxEventLine {
			return errors.New("event line too long")
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := privateDir(filepath.Dir(l.path)); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if n, err := f.Write(line); err != nil || n != len(line) {
		f.Close()
		return errors.Join(err, errors.New("short event write"))
	}
	return f.Close()
}

// stamp fills the fields every event carries.
func (s *Server) stamp(r record) record {
	r.TS = time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	r.Owner = s.home.PaneID
	return r
}

// logEvent appends an event. A failed write never undoes what already happened in tmux; callers say so in the reply.
func (s *Server) logEvent(r record) bool {
	if err := s.events.Append(r); err != nil {
		s.log.Printf("event log write failed: %v", err)
		return false
	}
	return true
}

func recordMap(r record) (map[string]any, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}

// report writes r to the event log and returns it as the tool's reply, with extra keys beside it.
func (s *Server) report(r record, extra map[string]any) (string, error) {
	r = s.stamp(r)
	logged := s.logEvent(r)
	m, err := recordMap(r)
	if err != nil {
		return "", &toolErr{"internal", err.Error()}
	}
	maps.Copy(m, extra)
	if !logged {
		m["event_logged"] = false
	}
	return jsonText(m)
}

// paneDesc says where a pane is now, for a record. Every text comes from fixedText.
type paneDesc struct {
	sessID, winID             string
	name, window, session, at string
	size                      string
	known                     bool
}

var describeFields = []string{"pane_id", "session_id", "session_name", "window_id", "window_index", "window_name", "pane_index", "pane_width", "pane_height", nameOpt}

// describe reads one pane. If tmux cannot answer, the record just has less in it.
func (s *Server) describe(id string) paneDesc {
	rows, err := s.tm.query(describeFields, "display-message", "-p", "-t", id, formatOf(describeFields))
	if err != nil || len(rows) != 1 || rows[0]["pane_id"] != id {
		return paneDesc{}
	}
	r := rows[0]
	wi, e1 := parseUint(r["window_index"])
	pi, e2 := parseUint(r["pane_index"])
	w, e3 := parseUint(r["pane_width"])
	h, e4 := parseUint(r["pane_height"])
	if errors.Join(e1, e2, e3, e4, checkID(reWindowID, r["window_id"]), checkID(reSessionID, r["session_id"])) != nil {
		return paneDesc{}
	}
	return paneDesc{sessID: r["session_id"], winID: r["window_id"], name: fixedText(r[nameOpt]),
		window: fmt.Sprintf("%d:%s", wi, fixedText(r["window_name"])), session: fixedText(r["session_name"]),
		at: fmt.Sprintf("%d.%d", wi, pi), size: fmt.Sprintf("%dx%d", w, h), known: true}
}

func (d paneDesc) apply(r *record) {
	r.Name, r.Window, r.Session, r.At, r.Size = d.name, d.window, d.session, d.at, d.size
}

func (d paneDesc) loc() string { return d.session + ":" + d.at }

// tracked is what agent-tmux remembers about a pane it owns, in memory only, to notice moves and closes between calls.
type tracked struct {
	sessID, winID, loc, name string
	// typed is the text of the line agent-tmux typed and has not seen run; it is compared with the screen and never logged.
	typed string
	// closing: agent-tmux is killing it, so its disappearance is not an observation.
	closing bool
}

// track remembers a pane's place.
func (s *Server) track(id string, d paneDesc) {
	if !d.known {
		return
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	if t := s.reg[id]; t != nil {
		t.sessID, t.winID, t.loc, t.name = d.sessID, d.winID, d.loc(), d.name
		return
	}
	s.reg[id] = &tracked{sessID: d.sessID, winID: d.winID, loc: d.loc(), name: d.name}
}

func (s *Server) setTyped(id, text string) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	if t := s.reg[id]; t != nil {
		t.typed = text
	}
}

func (s *Server) typedText(id string) string {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	if t := s.reg[id]; t != nil {
		return t.typed
	}
	return ""
}

// markClosing flags panes agent-tmux is about to kill, so a sweep does not report them as closed by someone else.
// It is undone if the kill fails.
func (s *Server) markClosing(ids []string, closing bool) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	for _, id := range ids {
		if t := s.reg[id]; t != nil {
			t.closing = closing
		}
	}
}

func (s *Server) forget(ids []string) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	for _, id := range ids {
		delete(s.reg, id)
	}
}

var sweepFields = []string{"pid", "pane_id", "session_id", "session_name", "window_id", "window_index", "window_name", "pane_index", nameOpt, ownerOpt}

// sweep compares the panes this agent owns with what it last saw, and writes `moved` (another window or
// session), `revoked` (the pane is still there but no longer carries this agent's tag) and `closed` (gone;
// agent-tmux cannot tell a human closing it from the shell exiting, so by is "unknown"). A pane seen for the first
// time is remembered without an event. It only reads tmux and writes events: nothing it does changes tmux.
func (s *Server) sweep() {
	if s.home.Err != nil {
		return
	}
	rows, err := s.tm.query(sweepFields, "list-panes", "-a", "-F", formatOf(sweepFields))
	if err != nil || len(rows) == 0 {
		return
	}
	if _, err := parseUint(rows[0]["pid"]); err != nil {
		return
	}
	mine := rows[0]["pid"] + ":" + s.home.PaneID
	present := map[string]bool{}
	var evs []record
	s.regMu.Lock()
	seen := map[string]bool{}
	for _, r := range rows {
		id := r["pane_id"]
		present[id] = true
		// The -F view reads tags through inheritance, so a hand-set global tag shows on every pane. It only picks
		// candidates: the tag on the pane itself decides, and the home pane is never agent-tmux's to track.
		if r[ownerOpt] != mine || id == s.home.PaneID || checkID(rePaneID, id) != nil {
			continue
		}
		own, err := s.tagValue("-p", id, ownerOpt)
		if err != nil {
			// Cannot tell: keep what is remembered and report nothing for this pane.
			seen[id] = true
			continue
		}
		if own != mine {
			continue
		}
		wi, e1 := parseUint(r["window_index"])
		pi, e2 := parseUint(r["pane_index"])
		if errors.Join(e1, e2, checkID(reWindowID, r["window_id"]), checkID(reSessionID, r["session_id"])) != nil {
			continue
		}
		seen[id] = true
		d := paneDesc{sessID: r["session_id"], winID: r["window_id"], name: fixedText(r[nameOpt]),
			window: fmt.Sprintf("%d:%s", wi, fixedText(r["window_name"])), session: fixedText(r["session_name"]),
			at: fmt.Sprintf("%d.%d", wi, pi), known: true}
		t := s.reg[id]
		switch {
		case t == nil:
			s.reg[id] = &tracked{sessID: d.sessID, winID: d.winID, loc: d.loc(), name: d.name}
		case t.sessID != d.sessID || t.winID != d.winID:
			// Keyed on the ids: renumber-windows or a sibling closing changes indexes without moving the pane.
			ev := record{Ev: "moved", Pane: id, From: t.loc, To: d.loc()}
			d.apply(&ev)
			evs = append(evs, ev)
			t.sessID, t.winID, t.loc, t.name = d.sessID, d.winID, d.loc(), d.name
		default:
			t.loc, t.name = d.loc(), d.name
		}
	}
	for id, t := range s.reg {
		if seen[id] {
			continue
		}
		delete(s.reg, id)
		switch {
		case t.closing:
		case present[id]:
			evs = append(evs, record{Ev: "revoked", Pane: id, Name: t.name, From: t.loc})
		default:
			evs = append(evs, record{Ev: "closed", Pane: id, Name: t.name, From: t.loc, By: "unknown"})
		}
	}
	// Written under the lock so two sweeps cannot both report one change, and events keep their order.
	for _, ev := range evs {
		s.logEvent(s.stamp(ev))
	}
	s.regMu.Unlock()
}

// lineHolds reports whether the last non-blank line of the pane still ends with the text agent-tmux typed: typed text
// waits at the line's end, after the prompt, so a prompt that merely contains it ("~/tools $" for "ls") does not
// count. The screen is compared here and never sent anywhere. -J joins lines the terminal wrapped.
func (s *Server) lineHolds(id, text string) (bool, error) {
	want := strings.TrimSpace(text)
	if want == "" {
		return true, nil
	}
	out, err := s.tm.run("capture-pane", "-p", "-J", "-t", id)
	if err != nil {
		return false, err
	}
	ls := strings.Split(lastLines(stripANSI(out), 1), "\n")
	return strings.HasSuffix(strings.TrimRight(ls[len(ls)-1], " \t"), want), nil
}

var settleFields = []string{"pane_id", "session_id", "pane_current_command", typedOpt}

// settleTyped clears @agent-tmux_typed on owned panes whose typed line is no longer waiting: a program other than a
// shell is running there, or the shell's last line no longer holds the text. It is a guess from the screen, which
// is why tmux_list says "unverified" when agent-tmux has no text to compare. It runs only inside a tool call, and
// writes the call's intent line before the first change (intentDone says the caller already wrote it), so no
// tmux change happens without one.
func (s *Server) settleTyped(ctx context.Context, rec *auditRec, intentDone bool) error {
	rows, err := s.tm.query(settleFields, "list-panes", "-a", "-F", formatOf(settleFields))
	if err != nil {
		return nil
	}
	var stale []string
	for _, r := range rows {
		id := r["pane_id"]
		if r[typedOpt] == "" || checkID(rePaneID, id) != nil || checkID(reSessionID, r["session_id"]) != nil {
			continue
		}
		if ok, err := s.paneOwned(id, r["session_id"]); err != nil || !ok {
			continue
		}
		if shells[r["pane_current_command"]] {
			text := s.typedText(id)
			if text == "" {
				continue // no text to compare: stays, and tmux_list calls it unverified
			}
			if held, err := s.lineHolds(id, text); err != nil || held {
				continue
			}
		}
		stale = append(stale, id)
	}
	if len(stale) == 0 {
		return nil
	}
	if !intentDone {
		if err := s.intent(ctx, *rec); err != nil {
			return err
		}
	}
	for _, id := range stale {
		if _, err := s.tm.run("set-option", "-p", "-u", "-t", id, typedOpt); err == nil {
			s.setTyped(id, "")
		}
	}
	return nil
}
