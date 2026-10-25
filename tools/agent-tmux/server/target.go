package main

import (
	"regexp"
	"strconv"
	"strings"
)

// Accepted target grammar (agent-tmux-SPEC "Targets"). Anything else is bad_target.
var (
	reWinIdx  = regexp.MustCompile(`^[0-9]{1,4}$`)
	reWinPane = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}$`)
	rePaneArg = regexp.MustCompile(`^%[0-9]{1,6}$`)
	reWinArg  = regexp.MustCompile(`^@[0-9]{1,6}$`)
)

type targetKind int

const (
	tWindowIndex targetKind = iota
	tWindowPane
	tPaneID
	tWindowID
)

type target struct {
	kind     targetKind
	win, pan int    // indexes for tWindowIndex / tWindowPane
	id       string // %N or @N
}

func parseTarget(s string) (target, error) {
	switch {
	case reWinIdx.MatchString(s):
		n, _ := strconv.Atoi(s)
		return target{kind: tWindowIndex, win: n}, nil
	case reWinPane.MatchString(s):
		w, p, _ := strings.Cut(s, ".")
		wn, _ := strconv.Atoi(w)
		pn, _ := strconv.Atoi(p)
		return target{kind: tWindowPane, win: wn, pan: pn}, nil
	case rePaneArg.MatchString(s):
		return target{kind: tPaneID, id: s}, nil
	case reWinArg.MatchString(s):
		return target{kind: tWindowID, id: s}, nil
	}
	return target{}, &toolErr{"bad_target", "target must be N, N.M, %N or @N"}
}

// resolve picks the pane a target names from the home session's pane list (already session-qualified).
// Window forms resolve to that window's active pane. Not found means out of scope.
func resolve(t target, panes []paneRow) (paneRow, error) {
	for _, p := range panes {
		ok := false
		switch t.kind {
		case tWindowIndex:
			ok = p.WindowIndex == t.win && p.PaneActive
		case tWindowPane:
			ok = p.WindowIndex == t.win && p.PaneIndex == t.pan
		case tPaneID:
			ok = p.PaneID == t.id
		case tWindowID:
			ok = p.WindowID == t.id && p.PaneActive
		}
		if ok {
			if p.WindowLinked {
				return paneRow{}, &toolErr{"refused", "target window is linked into another session"}
			}
			return p, nil
		}
	}
	return paneRow{}, &toolErr{"out_of_scope", "target is not a pane in the home session"}
}
