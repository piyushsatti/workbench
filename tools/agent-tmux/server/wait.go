package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWaitSecs  = 30
	maxWaitSecs      = 300
	defaultWaitLines = 20
	maxWaitLines     = 200
	maxPatternLen    = 200
	defaultWaitPoll  = 500 * time.Millisecond
	// maxPatternInsts caps the compiled program. RE2 time is linear in the text but also in the program, and
	// 200 characters can expand to thousands of instructions: (?:.*){999}Q compiles to 2001. Ordinary patterns,
	// even a 200-character literal, stay under about 400.
	maxPatternInsts = 1000
)

// patPattern mirrors checkPattern for the JSON schema. '#', ';' and a leading '-' are fine here: the pattern
// is compiled in Go and never reaches tmux or any argv.
const patPattern = `^[\x20-\x7e]+$`

func checkPattern(v string) (*regexp.Regexp, error) {
	if v == "" {
		return nil, badInput("pattern must not be empty")
	}
	if len(v) > maxPatternLen {
		return nil, badInput(fmt.Sprintf("pattern is longer than %d characters", maxPatternLen))
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return nil, badInput("pattern may contain only printable ASCII (0x20-0x7E)")
		}
	}
	parsed, err := syntax.Parse(v, syntax.Perl)
	if err != nil {
		return nil, badInput("pattern does not compile: " + err.Error())
	}
	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return nil, badInput("pattern does not compile: " + err.Error())
	}
	if len(prog.Inst) > maxPatternInsts {
		return nil, badInput(fmt.Sprintf("pattern is too complex: it compiles to %d instructions, more than %d; use fewer or smaller repeats", len(prog.Inst), maxPatternInsts))
	}
	re, err := regexp.Compile(v)
	if err != nil {
		return nil, badInput("pattern does not compile: " + err.Error())
	}
	return re, nil
}

func intArg(v *int, def, lo, hi int, name string) (int, error) {
	if v == nil {
		return def, nil
	}
	if *v < lo || *v > hi {
		return 0, badInput(fmt.Sprintf("%s must be %d..%d", name, lo, hi))
	}
	return *v, nil
}

// toolWait polls a agent-tmux-owned pane until one of its last lines matches. A timeout is a result, not an error;
// cancellation (notifications/cancelled or stdin EOF) ends the wait at the next tick or the next line scanned.
func (s *Server) toolWait(ctx context.Context, raw json.RawMessage, rec *auditRec) (string, error) {
	var a struct {
		Pane    *string `json:"pane"`
		Pattern *string `json:"pattern"`
		Timeout *int    `json:"timeout"`
		Lines   *int    `json:"lines"`
	}
	err := decodeArgs(raw, &a)
	hashOpt(rec, "pane", a.Pane)
	hashOpt(rec, "pattern", a.Pattern)
	if err != nil {
		return "", err
	}
	if a.Pane == nil || a.Pattern == nil {
		return "", badInput("pane and pattern are required")
	}
	secs, err := intArg(a.Timeout, defaultWaitSecs, 1, maxWaitSecs, "timeout")
	if err != nil {
		return "", err
	}
	lines, err := intArg(a.Lines, defaultWaitLines, 1, maxWaitLines, "lines")
	if err != nil {
		return "", err
	}
	re, err := checkPattern(*a.Pattern)
	if err != nil {
		return "", err
	}
	tg, err := parsePaneTarget(*a.Pane)
	if err != nil {
		return "", err
	}
	p, _, err := s.resolvePane(tg)
	if err != nil {
		return "", err
	}
	rec.target = p.PaneID
	if p.PaneID == s.home.PaneID {
		return "", &toolErr{"refused", "never waits on the home pane"}
	}
	if ok, err := s.paneOwned(p.PaneID, p.SessionID); err != nil || !ok {
		return "", orRefused(err, "wait is limited to agent-tmux-owned panes")
	}
	start := time.Now()
	deadline := start.Add(time.Duration(secs) * time.Second)
	for {
		line, found, err := s.scanPane(ctx, p.PaneID, re, lines)
		if err != nil {
			return "", err
		}
		elapsed := time.Since(start).Milliseconds()
		if found {
			rec.bytes = len(line)
			return jsonText(map[string]any{"matched": true, "line": wrapUntrusted(p.PaneID, neutralise(line)), "elapsed_ms": elapsed})
		}
		left := time.Until(deadline)
		if left <= 0 {
			rec.result = "timeout"
			return jsonText(map[string]any{"matched": false, "line": "", "elapsed_ms": elapsed})
		}
		tick := time.NewTimer(min(s.waitPoll, left))
		select {
		case <-ctx.Done():
			tick.Stop()
			return "", errWaitCancelled
		case <-tick.C:
		}
	}
}

var errWaitCancelled = &toolErr{"cancelled", "wait cancelled before a match"}

// scanPane captures the pane, checks it is still in the home session and still carries the agent-tmux tag itself,
// and returns the newest of its last n lines that matches.
func (s *Server) scanPane(ctx context.Context, id string, re *regexp.Regexp, n int) (string, bool, error) {
	out, capErr := s.tm.run("capture-pane", "-p", "-t", id, "-S", "-"+strconv.Itoa(n))
	sess, inScope := s.paneInScope(id)
	if !inScope {
		return "", false, &toolErr{"out_of_scope", "pane " + id + " closed or left the home session during the wait"}
	}
	if ok, err := s.paneOwned(id, sess); err != nil || !ok {
		return "", false, orRefused(err, "pane "+id+" stopped being agent-tmux-owned during the wait")
	}
	if capErr != nil {
		return "", false, &toolErr{"tmux_error", capErr.Error()}
	}
	ls := strings.Split(lastLines(stripANSI(out), n), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return "", false, errWaitCancelled
		}
		if re.MatchString(ls[i]) {
			return ls[i], true, nil
		}
	}
	return "", false, nil
}
