package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Pane options inherit from window options (tmux(1), OPTIONS), so the window tag has its own name:
// a window-level @agent-tmux_owned would make a human's later split in that window read as owned.
const (
	ownedPaneOpt   = "@agent-tmux_owned"
	ownedWindowOpt = "@agent-tmux_owned_window"
)

const (
	patAnyTarget    = `^([0-9]{1,4}|[0-9]{1,4}\.[0-9]{1,4}|%[0-9]{1,6}|@[0-9]{1,6})$`
	patPaneTarget   = `^(%[0-9]{1,6}|[0-9]{1,4}\.[0-9]{1,4})$`
	patWindowTarget = `^(@[0-9]{1,6}|[0-9]{1,4})$`
	noteDelayMS     = "3000"
	minCells        = 5
	maxCells        = 500
)

// reClientName accepts only pty clients; control-mode or unnamed clients are never targeted.
var reClientName = regexp.MustCompile(`^/dev/pts/[0-9]{1,9}$`)

var createdFields = []string{"window_id", "pane_id"}

func hashOpt(rec *auditRec, name string, v *string) {
	if v != nil {
		rec.args = append(rec.args, hashArg(name, *v))
	}
}

// orRefused returns err when a tag could not be read, else a refusal with msg.
func orRefused(err error, msg string) error {
	if err != nil {
		return err
	}
	return &toolErr{"refused", msg}
}

// intent writes the audit line before a mutation, so a log that cannot be written refuses the call before tmux
// is touched. A call cancelled by then stops here with nothing changed. Past this line a call runs to the end, so
// a cancel never leaves an untagged pane or an intent line without its outcome.
func (s *Server) intent(ctx context.Context, rec auditRec) error {
	if ctx.Err() != nil {
		return &toolErr{"cancelled", "call cancelled before anything was changed"}
	}
	rec.result = "intent"
	if err := s.auditWrite(rec); err != nil {
		return &toolErr{"audit_unavailable", "audit log could not be written; nothing was changed"}
	}
	return nil
}

func windowPanes(panes []paneRow, winID string) []paneRow {
	var out []paneRow
	for _, p := range panes {
		if p.WindowID == winID {
			out = append(out, p)
		}
	}
	return out
}

// tagValue reads a agent-tmux tag set on the object itself: level "-p" for a pane, "-w" for a window. It never passes
// -A or -g and never reads the tag through a -F format, because those resolve it through the window, session,
// global and server options too: a hand-set global tag would make every pane read as owned.
func (s *Server) tagValue(level, id, name string) (string, error) {
	out, err := s.tm.run("show-options", level, "-q", "-v", "-t", id, name)
	if err != nil {
		return "", &toolErr{"tmux_error", "could not read " + name + " of " + id}
	}
	return strings.TrimSuffix(out, "\n"), nil
}

// ownerValue is what this agent writes in @agent-tmux_owner: the tmux server's pid and the home pane's id. The pid is
// read fresh each time, so a pane id that a restarted server hands out again does not look owned, and the home pane
// is the one this server serves, so another agent's panes do not either.
func (s *Server) ownerValue() (string, error) {
	f := []string{"pid"}
	rows, err := s.tm.query(f, "display-message", "-p", "-t", s.home.PaneID, formatOf(f))
	if err != nil || len(rows) != 1 {
		return "", &toolErr{"tmux_error", "could not read the tmux server pid"}
	}
	if _, err := parseUint(rows[0]["pid"]); err != nil {
		return "", &toolErr{"tmux_error", "tmux server pid is unreadable"}
	}
	return rows[0]["pid"] + ":" + s.home.PaneID, nil
}

// paneOwned: the pane carries this agent's owner tag itself, wherever it is now (session is its session id). A
// pane with no owner tag and the exact pre-2026.10.13 tag @agent-tmux_owned=1 is still owned while it sits in the home
// session, which is all that tag ever meant. New panes never reach that path: they carry the owner string in
// both options, and a revoke clears both.
func (s *Server) paneOwned(id, session string) (bool, error) {
	v, err := s.tagValue("-p", id, ownerOpt)
	if err != nil {
		return false, err
	}
	if v != "" {
		mine, err := s.ownerValue()
		return err == nil && v == mine, err
	}
	if session != s.homeSession() {
		return false, nil
	}
	old, err := s.tagValue("-p", id, ownedPaneOpt)
	return old == "1", err
}

// windowAllOwned: every pane of the window is owned. This replaces the old window tag, which a window moved or a
// pane joined into it could make wrong. It does not look at the home pane; callers refuse that separately.
func (s *Server) windowAllOwned(rows []paneRow) (bool, error) {
	for _, p := range rows {
		if ok, err := s.paneOwned(p.PaneID, p.SessionID); err != nil || !ok {
			return false, err
		}
	}
	return len(rows) > 0, nil
}

func windowSynced(rows []paneRow) bool {
	return slices.ContainsFunc(rows, func(p paneRow) bool { return p.Synced })
}

// homeWindow returns the window the home pane is in now, which may differ from the one cached at startup.
func (s *Server) homeWindow(panes []paneRow) (string, error) {
	for _, p := range panes {
		if p.PaneID == s.home.PaneID {
			return p.WindowID, nil
		}
	}
	return "", &toolErr{"no_home", "home pane missing from the home session listing"}
}

// paneAllowed: the home pane or a pane that carries the agent-tmux tag itself.
func (s *Server) paneAllowed(p paneRow) (bool, error) {
	if p.PaneID == s.home.PaneID {
		return true, nil
	}
	return s.paneOwned(p.PaneID, p.SessionID)
}

// parsePaneTarget accepts only the pane forms %N and N.M.
func parsePaneTarget(arg string) (target, error) {
	tg, err := parseTarget(arg)
	if err != nil {
		return target{}, err
	}
	if tg.kind != tPaneID && tg.kind != tWindowPane {
		return target{}, &toolErr{"bad_target", "target must be a pane: %N or N.M"}
	}
	return tg, nil
}

// paneTarget resolves a pane-form target (default: the home pane) against a fresh home listing.
func (s *Server) paneTarget(arg *string) (paneRow, []paneRow, error) {
	tg := target{kind: tPaneID, id: s.home.PaneID}
	if arg != nil {
		var err error
		if tg, err = parsePaneTarget(*arg); err != nil {
			return paneRow{}, nil, err
		}
	}
	return s.resolvePane(tg)
}

// resolveWindow finds a window by index or ID in the home listing and returns its panes.
func resolveWindow(t target, panes []paneRow) (string, []paneRow, error) {
	id := ""
	for _, p := range panes {
		if (t.kind == tWindowIndex && p.WindowIndex == t.win) || (t.kind == tWindowID && p.WindowID == t.id) {
			id = p.WindowID
			break
		}
	}
	if id == "" {
		return "", nil, &toolErr{"out_of_scope", "target is not a window in the home session"}
	}
	rows := windowPanes(panes, id)
	for _, p := range rows {
		if p.WindowLinked {
			return "", nil, &toolErr{"refused", "target window is linked into another session"}
		}
	}
	return id, rows, nil
}

// startDir is the -c value for a new pane: the agent's dir, or else the home pane's current path, checked the same way.
func (s *Server) startDir(arg *string) (string, error) {
	if arg != nil {
		return checkDir(*arg, s.userHome)
	}
	f := []string{"pane_current_path"}
	rows, err := s.tm.query(f, "display-message", "-p", "-t", s.home.PaneID, formatOf(f))
	if err != nil || len(rows) != 1 {
		return "", &toolErr{"tmux_error", "could not read the home pane's current directory"}
	}
	d, err := checkDir(rows[0]["pane_current_path"], s.userHome)
	if err != nil {
		te := asToolErr(err, "bad_input")
		return "", &toolErr{te.code, "home pane's current directory is not usable (" + te.msg + "); pass dir"}
	}
	return d, nil
}

// rePathValue is a PATH value agent-tmux will hand to tmux, up to maxPathLen: printable ASCII, not ending in ';'
// (checkArgv).
var rePathValue = regexp.MustCompile(`^[\x20-\x7e]*[\x20-\x3a\x3c-\x7e]$`)

const maxPathLen = 4096

// pathEnv is the -e PATH=... for a new pane: the PATH a pane split by hand would get, the home session's, else
// the global one. tmux overrides -e with the detached client's own PATH, so ExecRunner also runs the command
// with this PATH (clientPath). If neither reads cleanly it returns nothing and the pane gets /usr/bin:/bin.
func (s *Server) pathEnv() []string {
	for _, scope := range [][]string{{"-t", s.homeSession()}, {"-g"}} {
		out, err := s.tm.run(append(append([]string{"show-environment"}, scope...), "PATH")...)
		v, ok := strings.CutPrefix(strings.TrimSuffix(out, "\n"), "PATH=")
		if err == nil && ok && len(v) <= maxPathLen && rePathValue.MatchString(v) {
			return []string{"-e", "PATH=" + v}
		}
	}
	return nil
}

// create runs new-window/split-window (argv ends with -P -F), then checks the new pane is in wantSession and
// wantWindow (empty: the window tmux reports) before tagging and naming it. A misplaced pane is left alone:
// untagged, so no agent-tmux tool can act on it, and not killed. name is the pane's @agent-tmux_name; empty makes
// "pane-N" from its id, so every pane agent-tmux creates has one.
func (s *Server) create(rec *auditRec, wantSession, wantWindow, name string, tagWindow bool, args ...string) (string, string, paneDesc, error) {
	rows, err := s.tm.query(createdFields, args...)
	if err != nil || len(rows) != 1 {
		return "", "", paneDesc{}, &toolErr{"tmux_error", fmt.Sprint(args[0], " failed: ", err)}
	}
	win, pane := rows[0]["window_id"], rows[0]["pane_id"]
	if checkID(reWindowID, win) != nil || checkID(rePaneID, pane) != nil {
		return "", "", paneDesc{}, &toolErr{"tmux_error", args[0] + " returned malformed ids"}
	}
	rec.created = win + "," + pane
	if wantWindow == "" {
		wantWindow = win
	}
	f := []string{"pane_id", "window_id", "session_id", "window_linked"}
	got, err := s.tm.query(f, "display-message", "-p", "-t", pane, formatOf(f))
	if err != nil || len(got) != 1 || got[0]["pane_id"] != pane {
		return "", "", paneDesc{}, &toolErr{"tmux_error", "new pane " + pane + " could not be re-read; it was left untagged"}
	}
	if got[0]["session_id"] != wantSession || got[0]["window_id"] != wantWindow || got[0]["window_linked"] != "0" {
		return "", "", paneDesc{}, &toolErr{"out_of_scope", "new pane " + pane + " is not in the expected window of the expected session; it was left untagged and nothing was killed"}
	}
	owner, err := s.ownerValue()
	if err != nil {
		return "", "", paneDesc{}, &toolErr{"tmux_error", "pane " + pane + " was created but could not be tagged agent-tmux-owned: " + err.Error()}
	}
	// The owner string goes in both options. @agent-tmux_owned keeps the current border working and makes an older
	// binary, which wants exactly "1", refuse the pane instead of owning it.
	for _, opt := range []string{ownerOpt, ownedPaneOpt} {
		if _, err := s.tm.run("set-option", "-p", "-t", pane, opt, owner); err != nil {
			return "", "", paneDesc{}, &toolErr{"tmux_error", "pane " + pane + " was created but could not be tagged agent-tmux-owned"}
		}
	}
	if tagWindow {
		if _, err := s.tm.run("set-option", "-w", "-t", win, ownedWindowOpt, "1"); err != nil {
			return "", "", paneDesc{}, &toolErr{"tmux_error", "pane " + pane + " is tagged but its window " + win + " could not be tagged agent-tmux-owned"}
		}
	}
	if name == "" {
		name = "pane-" + pane[1:]
	}
	// The title too, for tools that show it; the border reads @agent-tmux_name, which a program in the pane cannot reset.
	if _, err := s.tm.run("set-option", "-p", "-t", pane, nameOpt, name); err != nil {
		return "", "", paneDesc{}, &toolErr{"tmux_error", "pane " + pane + " is tagged but could not be named"}
	}
	if _, err := s.tm.run("select-pane", "-T", name, "-t", pane); err != nil {
		return "", "", paneDesc{}, &toolErr{"tmux_error", "pane " + pane + " is tagged but its title could not be set"}
	}
	d := s.describe(pane)
	s.track(pane, d)
	return win, pane, d, nil
}

func (s *Server) toolNewWindow(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Name *string `json:"name"`
		Dir  *string `json:"dir"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "name", a.Name)
	hashOpt(rec, "dir", a.Dir)
	if err != nil {
		return "", err
	}
	home := s.homeSession()
	rec.target = home
	if a.Name != nil {
		if err := checkText("name", *a.Name, maxNameLen); err != nil {
			return "", err
		}
	}
	dir, err := s.startDir(a.Dir)
	if err != nil {
		return "", err
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	// No shell-command argument: tmux starts the default shell. -c is always explicit because
	// without it the window starts in the tmux client's cwd, which for this server is "/".
	args := []string{"new-window", "-d", "-P", "-F", formatOf(createdFields), "-t", home + ":", "-c", dir}
	args = append(args, s.pathEnv()...)
	name := ""
	if a.Name != nil {
		args = append(args, "-n", *a.Name)
		name = *a.Name
	}
	win, pane, d, err := s.create(rec, home, "", name, true, args...)
	if err != nil {
		return "", err
	}
	r := record{Ev: "created", Pane: pane, By: "tmux_new_window", Pos: "new window " + d.window}
	d.apply(&r)
	// dir may be the home pane's cwd as tmux reports it, so it is neutralised like whoami's cwd.
	return s.report(r, map[string]any{"window_id": win, "pane_id": pane, "dir": neutralise(dir)})
}

func (s *Server) toolSplit(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Target    *string `json:"target"`
		Direction *string `json:"direction"`
		Dir       *string `json:"dir"`
		Percent   *int    `json:"percent"`
		Name      *string `json:"name"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "target", a.Target)
	hashOpt(rec, "dir", a.Dir)
	hashOpt(rec, "name", a.Name)
	if err != nil {
		return "", err
	}
	name := ""
	if a.Name != nil {
		if err := checkText("name", *a.Name, maxNameLen); err != nil {
			return "", err
		}
		name = *a.Name
	}
	if a.Direction == nil || (*a.Direction != "h" && *a.Direction != "v") {
		return "", badInput(`direction is required: "h" (side by side) or "v" (stacked)`)
	}
	if a.Percent != nil && (*a.Percent < 10 || *a.Percent > 90) {
		return "", badInput("percent must be 10..90")
	}
	p, panes, err := s.paneTarget(a.Target)
	if err != nil {
		return "", err
	}
	rec.target = p.PaneID
	if ok, err := s.paneAllowed(p); err != nil || !ok {
		return "", orRefused(err, "split is limited to the home pane and agent-tmux-owned panes")
	}
	if windowSynced(windowPanes(panes, p.WindowID)) {
		return "", &toolErr{"refused", "target window has synchronize-panes on"}
	}
	dir, err := s.startDir(a.Dir)
	if err != nil {
		return "", err
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	args := []string{"split-window", "-d", "-" + *a.Direction}
	if a.Percent != nil {
		args = append(args, "-l", strconv.Itoa(*a.Percent)+"%")
	}
	args = append(args, "-P", "-F", formatOf(createdFields), "-t", p.PaneID, "-c", dir)
	args = append(args, s.pathEnv()...)
	win, pane, d, err := s.create(rec, p.SessionID, p.WindowID, name, false, args...)
	if err != nil {
		return "", err
	}
	// h puts the new pane to the right of the split pane, v below it.
	side := map[string]string{"h": "right of ", "v": "below "}[*a.Direction]
	r := record{Ev: "created", Pane: pane, By: "tmux_split", SplitFrom: p.PaneID, Direction: *a.Direction, Pos: side + p.PaneID}
	if p.PaneID == s.home.PaneID {
		r.Pos += " (the home pane)"
	}
	d.apply(&r)
	return s.report(r, map[string]any{"window_id": win, "pane_id": pane, "dir": neutralise(dir)})
}

func (s *Server) toolRenameWindow(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Target *string `json:"target"`
		Name   *string `json:"name"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "target", a.Target)
	hashOpt(rec, "name", a.Name)
	if err != nil {
		return "", err
	}
	if a.Name == nil {
		return "", badInput("name is required")
	}
	if err := checkText("name", *a.Name, maxNameLen); err != nil {
		return "", err
	}
	var tg target
	if a.Target != nil {
		if tg, err = parseTarget(*a.Target); err != nil {
			return "", err
		}
		if tg.kind != tWindowID && tg.kind != tWindowIndex {
			return "", &toolErr{"bad_target", "target must be a window: @N or N"}
		}
	}
	panes, err := s.homePanes()
	if err != nil {
		return "", err
	}
	homeWin, err := s.homeWindow(panes)
	if err != nil {
		return "", err
	}
	if a.Target == nil {
		tg = target{kind: tWindowID, id: homeWin}
	}
	id, rows, err := resolveWindow(tg, panes)
	if err != nil {
		return "", err
	}
	rec.target = id
	// A window counts as agent-tmux's when every pane in it is owned; a pane moved in or out changes that.
	if id != homeWin {
		if ok, err := s.windowAllOwned(rows); err != nil || !ok {
			return "", orRefused(err, "rename is limited to the home window and windows whose panes are all agent-tmux-owned")
		}
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	if _, err := s.tm.run("rename-window", "-t", id, "--", *a.Name); err != nil {
		return "", &toolErr{"tmux_error", err.Error()}
	}
	r := record{Ev: "renamed", Window: fmt.Sprintf("%d:%s", rows[0].WindowIndex, fixedText(*a.Name)), From: fixedText(rows[0].WindowName), To: fixedText(*a.Name)}
	return s.report(r, map[string]any{"window_id": id})
}

func (s *Server) toolLabel(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Target *string `json:"target"`
		Title  *string `json:"title"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "target", a.Target)
	hashOpt(rec, "title", a.Title)
	if err != nil {
		return "", err
	}
	if a.Title == nil {
		return "", badInput("title is required")
	}
	if err := checkText("title", *a.Title, maxNameLen); err != nil {
		return "", err
	}
	p, _, err := s.paneTarget(a.Target)
	if err != nil {
		return "", err
	}
	rec.target = p.PaneID
	if ok, err := s.paneAllowed(p); err != nil || !ok {
		return "", orRefused(err, "label is limited to the home pane and agent-tmux-owned panes")
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	// select-pane with -T only sets the title; it does not select the pane.
	if _, err := s.tm.run("select-pane", "-T", *a.Title, "-t", p.PaneID); err != nil {
		return "", &toolErr{"tmux_error", err.Error()}
	}
	// An owned pane's name follows its title, so the border and the records show the new one. The home pane
	// keeps only the title: it is not agent-tmux's pane and gets no name.
	if p.PaneID != s.home.PaneID {
		if _, err := s.tm.run("set-option", "-p", "-t", p.PaneID, nameOpt, *a.Title); err != nil {
			return "", &toolErr{"tmux_error", "title set on " + p.PaneID + " but its name could not be updated"}
		}
	}
	d := s.describe(p.PaneID)
	if p.PaneID != s.home.PaneID {
		s.track(p.PaneID, d)
	}
	r := record{Ev: "labelled", Pane: p.PaneID, Title: fixedText(*a.Title)}
	d.apply(&r)
	return s.report(r, map[string]any{"pane_id": p.PaneID})
}

// toolNote shows text on every pty client attached to the home session. display-message draws on its
// target client, and without -c tmux guesses one from the whole server, which can be another session's.
func (s *Server) toolNote(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Text *string `json:"text"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "text", a.Text)
	if err != nil {
		return "", err
	}
	if a.Text == nil {
		return "", badInput("text is required")
	}
	if err := checkText("text", *a.Text, maxNoteLen); err != nil {
		return "", err
	}
	f := []string{"client_name", "session_id"}
	home := s.homeSession()
	rows, err := s.tm.query(f, "list-clients", "-t", home, "-F", formatOf(f))
	if err != nil {
		return "", &toolErr{"tmux_error", err.Error()}
	}
	var clients []string
	for _, r := range rows {
		if r["session_id"] == home && reClientName.MatchString(r["client_name"]) {
			clients = append(clients, r["client_name"])
		}
	}
	if len(clients) == 0 {
		return "", &toolErr{"no_client", "no terminal is attached to the home session; note not shown"}
	}
	rec.target = strings.Join(clients, ",")
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	for _, c := range clients {
		if _, err := s.tm.run("display-message", "-l", "-d", noteDelayMS, "-c", c, "-t", s.home.PaneID, "--", *a.Text); err != nil {
			return "", &toolErr{"tmux_error", err.Error()}
		}
	}
	return s.report(record{Ev: "noted", Pane: s.home.PaneID, Text: redact(*a.Text)}, map[string]any{"clients": clients})
}

func (s *Server) toolResize(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Target *string `json:"target"`
		Width  *int    `json:"width"`
		Height *int    `json:"height"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "target", a.Target)
	if err != nil {
		return "", err
	}
	if a.Target == nil {
		return "", badInput("target is required")
	}
	if a.Width == nil && a.Height == nil {
		return "", badInput("width or height is required")
	}
	for _, v := range []*int{a.Width, a.Height} {
		if v != nil && (*v < minCells || *v > maxCells) {
			return "", badInput(fmt.Sprintf("width and height must be %d..%d", minCells, maxCells))
		}
	}
	p, _, err := s.paneTarget(a.Target)
	if err != nil {
		return "", err
	}
	rec.target = p.PaneID
	if p.PaneID == s.home.PaneID {
		return "", &toolErr{"refused", "resize is limited to agent-tmux-owned panes"}
	}
	if ok, err := s.paneOwned(p.PaneID, p.SessionID); err != nil || !ok {
		return "", orRefused(err, "resize is limited to agent-tmux-owned panes")
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	args := []string{"resize-pane", "-t", p.PaneID}
	if a.Width != nil {
		args = append(args, "-x", strconv.Itoa(*a.Width))
	}
	if a.Height != nil {
		args = append(args, "-y", strconv.Itoa(*a.Height))
	}
	if _, err := s.tm.run(args...); err != nil {
		return "", &toolErr{"tmux_error", err.Error()}
	}
	d := s.describe(p.PaneID)
	s.track(p.PaneID, d)
	r := record{Ev: "resized", Pane: p.PaneID}
	d.apply(&r)
	return s.report(r, map[string]any{"pane_id": p.PaneID})
}

type killPlan struct {
	window bool
	id     string
	panes  []string
}

// shells are the only programs agent-tmux ever starts in a pane, and the only ones send types into or kill closes.
var shells = map[string]bool{"bash": true, "sh": true, "zsh": true}

// killablePane: owned, not home, a shell at its prompt, and no sign of Claude. A missing @claude_state alone does
// not prove a pane is not Claude, and agent-tmux cannot start a program in its panes, so anything else running there
// was started by the human.
func (s *Server) killablePane(p paneRow, procs procTree) error {
	switch {
	case p.PaneID == s.home.PaneID:
		return &toolErr{"refused", "never kills the home pane"}
	case p.ClaudeState != "":
		return &toolErr{"refused", "pane " + p.PaneID + " has @claude_state set"}
	case !shells[p.Command]:
		return &toolErr{"refused", fmt.Sprintf("pane %s is running %s, not bash, sh or zsh, so the human started it", p.PaneID, quoteHeld(p.Command))}
	}
	if ok, err := s.paneOwned(p.PaneID, p.SessionID); err != nil || !ok {
		return orRefused(err, "pane "+p.PaneID+" is not agent-tmux-owned")
	}
	return s.shellAtPrompt(p, procs)
}

// planKill applies every kill rule to a fresh listing. The returned id is set once the target resolves, so a
// refusal is audited against it; panes is complete only when the error is nil.
func (s *Server) planKill(tg target) (killPlan, error) {
	var plan killPlan
	var rows []paneRow
	if tg.kind == tPaneID || tg.kind == tWindowPane {
		p, _, err := s.resolvePane(tg)
		if err != nil {
			return killPlan{}, err
		}
		plan.id, rows = p.PaneID, []paneRow{p}
	} else {
		panes, err := s.homePanes()
		if err != nil {
			return killPlan{}, err
		}
		id, ws, err := resolveWindow(tg, panes)
		if err != nil {
			return killPlan{}, err
		}
		plan = killPlan{window: true, id: id}
		rows = ws
		// A window may go only when every pane in it is agent-tmux's, which killablePane checks for each pane below.
		for _, p := range rows {
			if p.PaneID == s.home.PaneID {
				return plan, &toolErr{"refused", "window " + id + " contains the home pane"}
			}
		}
	}
	procs, err := readProcTree(s.procRoot)
	if err != nil {
		return plan, &toolErr{"refused", "cannot check the panes' processes for claude: " + err.Error()}
	}
	for _, p := range rows {
		if err := s.killablePane(p, procs); err != nil {
			return plan, err
		}
		plan.panes = append(plan.panes, p.PaneID)
	}
	return plan, nil
}

func (s *Server) toolKill(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Target *string `json:"target"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "target", a.Target)
	if err != nil {
		return "", err
	}
	if a.Target == nil {
		return "", badInput("target is required")
	}
	tg, err := parseTarget(*a.Target)
	if err != nil {
		return "", err
	}
	first, err := s.planKill(tg)
	rec.target = first.id
	if err != nil {
		return "", err
	}
	// Re-resolve immediately before acting; any difference refuses.
	again, err := s.planKill(tg)
	if err != nil {
		return "", err
	}
	if again.window != first.window || again.id != first.id || !slices.Equal(again.panes, first.panes) {
		return "", &toolErr{"refused", "target changed while it was being checked; nothing was killed"}
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	cmd := "kill-pane"
	if first.window {
		cmd = "kill-window"
	}
	// Describe before killing, so each record can say where the pane was. markClosing keeps a sweep from also
	// reporting the disappearance as someone else's.
	descs := make([]paneDesc, len(first.panes))
	for i, id := range first.panes {
		descs[i] = s.describe(id)
	}
	s.markClosing(first.panes, true)
	if _, err := s.tm.run(cmd, "-t", first.id); err != nil {
		s.markClosing(first.panes, false)
		return "", &toolErr{"tmux_error", err.Error()}
	}
	s.forget(first.panes)
	logged := true
	records := make([]map[string]any, len(first.panes))
	for i, id := range first.panes {
		r := s.stamp(record{Ev: "killed", Pane: id, By: "tmux_kill"})
		descs[i].apply(&r)
		r.Size = ""
		logged = s.logEvent(r) && logged
		m, err := recordMap(r)
		if err != nil {
			return "", &toolErr{"internal", err.Error()}
		}
		records[i] = m
	}
	reply := map[string]any{"killed": first.id, "panes": first.panes, "events": records}
	if !logged {
		reply["event_logged"] = false
	}
	return jsonText(reply)
}
