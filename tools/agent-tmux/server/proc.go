package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// maxProcs caps how many processes readProcTree reads; past it the scan fails, and so does whatever relied on it.
const maxProcs = 4096

// procTree is a snapshot of the process table: each PID's comm and process group, and the PIDs whose parent it is.
type procTree struct {
	comm     map[int]string
	pgrp     map[int]int
	children map[int][]int
}

// readProcTree reads <root>/<pid>/stat for every process. A process that exits mid-scan is skipped; anything
// else unreadable or malformed fails the whole scan.
func readProcTree(root string) (procTree, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return procTree{}, err
	}
	t := procTree{comm: map[int]string{}, pgrp: map[int]int{}, children: map[int][]int{}}
	n := 0
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if n++; n > maxProcs {
			return procTree{}, fmt.Errorf("more than %d processes", maxProcs)
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "stat"))
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return procTree{}, err
		}
		ppid, comm, err := parseStat(string(b))
		if err == nil {
			t.pgrp[pid], err = statGroup(string(b))
		}
		if err != nil {
			return procTree{}, fmt.Errorf("pid %d: %w", pid, err)
		}
		t.comm[pid] = comm
		t.children[ppid] = append(t.children[ppid], pid)
	}
	return t, nil
}

// statGroup returns pgrp, field 5 of /proc/<pid>/stat.
func statGroup(data string) (int, error) {
	_, tail, err := statTail(data)
	if err != nil {
		return 0, err
	}
	if len(tail) < 3 {
		return 0, errors.New("malformed stat: short tail")
	}
	pgrp, err := strconv.Atoi(tail[2])
	if err != nil || pgrp < 0 {
		return 0, errors.New("malformed stat: pgrp")
	}
	return pgrp, nil
}

// groupMate returns the lowest PID other than pgid in process group pgid, or 0 when the group holds no other.
func (t procTree) groupMate(pgid int) int {
	mate := 0
	for pid, g := range t.pgrp {
		if g == pgid && pid != pgid && (mate == 0 || pid < mate) {
			mate = pid
		}
	}
	return mate
}

// hasComm reports whether pid or any process descended from it has this comm.
func (t procTree) hasComm(pid int, comm string) bool {
	seen := map[int]bool{}
	stack := []int{pid}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] {
			continue
		}
		seen[p] = true
		if t.comm[p] == comm {
			return true
		}
		stack = append(stack, t.children[p]...)
	}
	return false
}

// foregroundGroup returns tpgid, field 8 of /proc/<pid>/stat: the process group in the foreground of pid's terminal.
func foregroundGroup(root string, pid int) (int, error) {
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	_, tail, err := statTail(string(b))
	if err != nil {
		return 0, err
	}
	if len(tail) < 6 {
		return 0, errors.New("malformed stat: short tail")
	}
	tpgid, err := strconv.Atoi(tail[5])
	if err != nil {
		return 0, errors.New("malformed stat: tpgid")
	}
	return tpgid, nil
}

// shellAtPrompt checks a pane whose command already reads as a shell. pane_current_command alone cannot tell a
// shell at its prompt from `bash -c ...` started there, which reads as bash too. So the shell must own the
// terminal (tpgid equals its pid, so no job it started is in the foreground), it must lead its own process group
// and be alone in it (a command substitution, or any command with job control off, runs its child in the shell's
// group while the shell keeps the terminal), and no process named claude may run anywhere under it. Any read or
// parse error refuses.
func (s *Server) shellAtPrompt(p paneRow, procs procTree) error {
	if procs.hasComm(p.PanePID, "claude") {
		return &toolErr{"refused", "pane " + p.PaneID + " has a claude process under its shell"}
	}
	fg, err := foregroundGroup(s.procRoot, p.PanePID)
	if err != nil {
		return &toolErr{"refused", "cannot tell whether pane " + p.PaneID + "'s shell is at its prompt: " + err.Error()}
	}
	if fg != p.PanePID {
		return &toolErr{"refused", "pane " + p.PaneID + "'s shell is not at its prompt: another program has the terminal"}
	}
	if procs.pgrp[p.PanePID] != p.PanePID {
		return &toolErr{"refused", "pane " + p.PaneID + "'s shell is not at its prompt: it does not lead its own process group"}
	}
	if mate := procs.groupMate(p.PanePID); mate != 0 {
		return &toolErr{"refused", fmt.Sprintf("pane %s's shell is not at its prompt: process %d runs in its process group", p.PaneID, mate)}
	}
	return nil
}
