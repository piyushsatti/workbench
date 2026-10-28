package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxHops = 64

// requiredParentComm makes DetectHome insist the direct parent's comm equals it.
// Claude Code's process comm is "claude" (seen in the server's startup log line under /mcp).
const requiredParentComm = "claude"

// Home is the pane this server serves. Err non-nil means every tool refuses with that reason.
type Home struct {
	PaneID, SessionID string
	ParentComm        string
	Err               error
}

// statTail splits /proc/<pid>/stat into comm and the fields after it, which start at field 3 (state).
// comm may contain spaces and ')', so it ends at the LAST ')'.
func statTail(data string) (comm string, tail []string, err error) {
	open := strings.IndexByte(data, '(')
	end := strings.LastIndexByte(data, ')')
	if open < 0 || end < open {
		return "", nil, errors.New("malformed stat")
	}
	return data[open+1 : end], strings.Fields(data[end+1:]), nil
}

// parseStat returns ppid (field 4) and comm from /proc/<pid>/stat.
func parseStat(data string) (ppid int, comm string, err error) {
	comm, rest, err := statTail(data)
	if err != nil {
		return 0, "", err
	}
	if len(rest) < 2 {
		return 0, "", errors.New("malformed stat: short tail")
	}
	ppid, err = strconv.Atoi(rest[1])
	if err != nil || ppid < 0 {
		return 0, "", errors.New("malformed stat: ppid")
	}
	return ppid, comm, nil
}

// ancestors returns the parent PID chain of pid (nearest first, excluding pid 1 and 0) and the direct parent's comm.
// Any unreadable link fails the whole walk.
func ancestors(procRoot string, pid int) ([]int, string, error) {
	var chain []int
	parentComm := ""
	cur := pid
	for hop := 0; hop < maxHops; hop++ {
		b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(cur), "stat"))
		if err != nil {
			return nil, "", fmt.Errorf("read stat of %d: %w", cur, err)
		}
		ppid, comm, err := parseStat(string(b))
		if err != nil {
			return nil, "", fmt.Errorf("pid %d: %w", cur, err)
		}
		if hop == 1 {
			parentComm = comm
		}
		if ppid <= 1 {
			break
		}
		chain = append(chain, ppid)
		cur = ppid
	}
	return chain, parentComm, nil
}

var homeFields = []string{"pane_pid", "pane_id", "session_id", "window_id", "window_linked", "session_grouped"}

// DetectHome matches the ancestor chain of self against every pane_pid on the server. It fails closed.
func DetectHome(t tmux, procRoot string, self int) Home {
	fail := func(f string, a ...any) Home { return Home{Err: fmt.Errorf("no home pane: "+f, a...)} }
	chain, parentComm, err := ancestors(procRoot, self)
	if err != nil {
		return fail("%v", err)
	}
	if requiredParentComm != "" && parentComm != requiredParentComm {
		return fail("parent process is %q, not %q", parentComm, requiredParentComm)
	}
	rows, err := t.query(homeFields, "list-panes", "-a", "-F", formatOf(homeFields))
	if err != nil {
		return fail("%v", err)
	}
	byPID := map[string][]map[string]string{}
	for _, r := range rows {
		byPID[r["pane_pid"]] = append(byPID[r["pane_pid"]], r)
	}
	for _, pid := range chain {
		matches := byPID[strconv.Itoa(pid)]
		if len(matches) == 0 {
			continue
		}
		for _, m := range matches {
			if m["window_linked"] != "0" {
				return fail("home window is linked into more than one session")
			}
			if m["session_grouped"] != "0" {
				return fail("home session is in a session group")
			}
		}
		if len(matches) > 1 {
			return fail("pid %d matches %d panes", pid, len(matches))
		}
		m := matches[0]
		if checkID(rePaneID, m["pane_id"]) != nil || checkID(reWindowID, m["window_id"]) != nil || checkID(reSessionID, m["session_id"]) != nil {
			return fail("malformed ids from tmux")
		}
		return Home{PaneID: m["pane_id"], SessionID: m["session_id"], ParentComm: parentComm}
	}
	return fail("no ancestor process is a tmux pane")
}
