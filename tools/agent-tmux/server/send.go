package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const maxSendLen = 1000

// patSendText mirrors checkSendText for the JSON schema: printable ASCII, last character not ';'.
const patSendText = `^[\x20-\x7e]*[\x20-\x3a\x3c-\x7e]$`

// checkSendText allows '#' and a leading '-', unlike checkText: send-keys expands no formats without -F, and
// the text always follows '--'. Printable ASCII only, because send-keys -l passes C1 bytes and UTF-8 through
// unchanged, and a control character could act as Enter.
func checkSendText(v string) error {
	if v == "" {
		return badInput("text must not be empty")
	}
	if len(v) > maxSendLen {
		return badInput(fmt.Sprintf("text is longer than %d characters", maxSendLen))
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return badInput("text may contain only printable ASCII (0x20-0x7E)")
		}
	}
	// tmux splits an argv element that ends in ';' into a second tmux command, even after '--'; a ';'
	// inside the element does not split (both measured on tmux 3.6).
	if strings.HasSuffix(v, ";") {
		return badInput("text must not end with ';': tmux reads a trailing ';' as a command separator")
	}
	return nil
}

// sendCheck is everything send's rules looked at. Two checks that differ in any field refuse the send.
type sendCheck struct {
	pane, session, window, command string
	windowPanes                    string
}

// planSend applies every send rule to a fresh listing. The returned pane is set once the target resolves. The
// pane may be in another session than the home one, if it is owned: the rules look at the pane, not its place.
func (s *Server) planSend(tg target) (sendCheck, error) {
	p, panes, err := s.resolvePane(tg)
	if err != nil {
		return sendCheck{}, err
	}
	c := sendCheck{pane: p.PaneID, session: p.SessionID, window: p.WindowID, command: p.Command}
	rows := windowPanes(panes, p.WindowID)
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.PaneID
	}
	c.windowPanes = strings.Join(ids, ",")
	switch {
	case p.PaneID == s.home.PaneID:
		return c, &toolErr{"refused", "never types into the home pane"}
	case p.ClaudeState != "":
		return c, &toolErr{"refused", "pane " + p.PaneID + " has @claude_state set"}
	case !shells[p.Command]:
		return c, &toolErr{"refused", fmt.Sprintf("pane %s is running %s; send types only at a bash, sh or zsh prompt", p.PaneID, quoteHeld(p.Command))}
	case p.InMode:
		// In copy mode the keys would drive the mode's key bindings instead of reaching the shell.
		return c, &toolErr{"refused", "pane " + p.PaneID + " is in copy mode or another mode"}
	case windowSynced(rows):
		return c, &toolErr{"refused", "window " + p.WindowID + " has synchronize-panes on, so the text would reach other panes too"}
	}
	// Owned is a property of the pane, so one that was moved to another window or session still takes text. The
	// window it sits in no longer matters: every pane rule above holds wherever it is, and the home pane is
	// refused first.
	if ok, err := s.paneOwned(p.PaneID, p.SessionID); err != nil || !ok {
		return c, orRefused(err, "pane "+p.PaneID+" is not agent-tmux-owned")
	}
	procs, err := readProcTree(s.procRoot)
	if err != nil {
		return c, &toolErr{"refused", "cannot check the pane's processes: " + err.Error()}
	}
	return c, s.shellAtPrompt(p, procs)
}

// toolSend types text into an owned shell and never presses Enter: the argv is always
// send-keys -l -t %ID -- TEXT, and TEXT holds no control character.
func (s *Server) toolSend(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Pane *string `json:"pane"`
		Text *string `json:"text"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "pane", a.Pane)
	hashOpt(rec, "text", a.Text)
	if err != nil {
		return "", err
	}
	if a.Pane == nil || a.Text == nil {
		return "", badInput("pane and text are required")
	}
	if err := checkSendText(*a.Text); err != nil {
		return "", err
	}
	tg, err := parsePaneTarget(*a.Pane)
	if err != nil {
		return "", err
	}
	first, err := s.planSend(tg)
	rec.target = first.pane
	if err != nil {
		return "", err
	}
	if err := s.intent(ctx, *rec); err != nil {
		return "", err
	}
	// Re-read the pane immediately before typing; any difference refuses.
	again, err := s.planSend(tg)
	if err != nil {
		return "", err
	}
	if again != first {
		return "", &toolErr{"refused", "target changed while it was being checked; nothing was sent"}
	}
	// The intent line is written, so this may drop the "typed" marker of any pane whose line is gone. Then the
	// marker on this pane, if still set, means an earlier line is waiting and this text goes after it.
	if err := s.settleTyped(ctx, rec, true); err != nil {
		return "", err
	}
	pending, err := s.tagValue("-p", first.pane, typedOpt)
	if err != nil {
		return "", err
	}
	if _, err := s.tm.run("send-keys", "-l", "-t", first.pane, "--", *a.Text); err != nil {
		return "", &toolErr{"tmux_error", err.Error()}
	}
	rec.bytes = len(*a.Text)
	// From here the text is typed; nothing below may refuse the call. The marker shows the human a typed line is
	// waiting. It is a display flag, and the reply says if it could not be set.
	id := s.typedSeq.Add(1)
	_, markErr := s.tm.run("set-option", "-p", "-t", first.pane, typedOpt, strconv.FormatUint(id, 10))
	d := s.describe(first.pane)
	s.track(first.pane, d)
	s.setTyped(first.pane, *a.Text)
	r := record{Ev: "typed", Pane: first.pane, Cmd: redactCmd(*a.Text), TypedID: id, State: "waiting_for_enter",
		Bytes: len(*a.Text), Appended: pending != ""}
	d.apply(&r)
	extra := map[string]any{"pane_id": first.pane}
	if markErr != nil {
		extra["marker_set"] = false
	}
	return s.report(r, extra)
}
