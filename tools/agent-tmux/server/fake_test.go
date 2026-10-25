package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeTmux answers the commands agent-tmux issues from an in-memory pane table. User options live at three levels,
// as in tmux: on the pane (keys starting with '@' in its map), on the window (winOpts) and global (globalOpts).
// A -F format resolves an option through all three; show-options without -A reads one level only.
type fakeTmux struct {
	mu         sync.Mutex
	panes      []map[string]string
	winOpts    map[string]map[string]string
	globalOpts map[string]string
	clients    []map[string]string
	calls      [][]string
	nextID     int
	// misplace, when set, makes new-window and split-window put the new pane in this session instead.
	misplace string
	// proc, when set, is the fake /proc where each new pane's shell appears, at its prompt.
	proc string
	// env holds show-environment answers, keyed "-g" or a session ID, then the variable; a value of "-"
	// stands for a variable removed at that level, printed by tmux as "-NAME".
	env map[string]map[string]string
}

var reFmtVar = regexp.MustCompile(`#\{([^}]+)\}`)

func (f *fakeTmux) expand(format string, p map[string]string) string {
	return reFmtVar.ReplaceAllStringFunc(format, func(m string) string {
		k := m[2 : len(m)-1]
		if v := p[k]; v != "" || !strings.HasPrefix(k, "@") {
			return v
		}
		if v := f.winOpts[p["window_id"]][k]; v != "" {
			return v
		}
		return f.globalOpts[k]
	})
}

// locked runs fn under the fake's lock, for tests that change the fake while a call is in flight.
func (f *fakeTmux) locked(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeTmux) window(id string) map[string]string {
	for _, p := range f.panes {
		if p["window_id"] == id {
			return p
		}
	}
	return nil
}

func flagVal(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func (f *fakeTmux) pane(id string) map[string]string {
	for _, p := range f.panes {
		if p["pane_id"] == id {
			return p
		}
	}
	return nil
}

func (f *fakeTmux) setWinOpt(win, k, v string) {
	if f.winOpts == nil {
		f.winOpts = map[string]map[string]string{}
	}
	if f.winOpts[win] == nil {
		f.winOpts[win] = map[string]string{}
	}
	f.winOpts[win][k] = v
}

func (f *fakeTmux) remove(keep func(p map[string]string) bool) {
	var out []map[string]string
	for _, p := range f.panes {
		if keep(p) {
			out = append(out, p)
		}
	}
	f.panes = out
}

// spawn adds a pane the way new-window (win == "") or split-window (win = the split pane's window) would.
func (f *fakeTmux) spawn(sess, win string, args []string) map[string]string {
	if f.misplace != "" {
		sess = f.misplace
	}
	f.nextID++
	id := 100 + f.nextID
	winIdx, paneIdx := 0, 0
	if win == "" {
		win = fmt.Sprint("@", id)
		for _, p := range f.panes {
			if p["session_id"] == sess {
				if n, _ := strconv.Atoi(p["window_index"]); n >= winIdx {
					winIdx = n + 1
				}
			}
		}
	} else {
		for _, p := range f.panes {
			if p["window_id"] == win {
				winIdx, _ = strconv.Atoi(p["window_index"])
				paneIdx++
			}
		}
	}
	p := mkPane(fmt.Sprint(5000+id), fmt.Sprint("%", id), sess, win, winIdx, paneIdx, false, "pane_current_path", flagVal(args, "-c"))
	if f.proc != "" {
		if err := statFile(f.proc, 5000+id, "bash", 700, 5000+id); err != nil {
			panic(err)
		}
	}
	if n := flagVal(args, "-n"); n != "" {
		p["window_name"] = n
	}
	f.panes = append(f.panes, p)
	return p
}

var reFramed = regexp.MustCompile(`^([A-Z2-7]{26})\x1f(.*)\x1f([A-Z2-7]{26})$`)

// unframe returns args with a token-framed format (see tmux.query) replaced by the bare format, for comparing argv.
func unframe(args []string) []string {
	out := slices.Clone(args)
	for i, a := range out {
		if m := reFramed.FindStringSubmatch(a); m != nil && m[1] == m[3] {
			out[i] = m[2]
		}
	}
	return out
}

// reSendKeys is the only send-keys argv agent-tmux may issue: literal, one pane ID, '--', one text argument.
var reSendKeys = regexp.MustCompile(`^send-keys\x00-l\x00-t\x00%[0-9]+\x00--\x00[\x20-\x7e]*[\x20-\x3a\x3c-\x7e]$`)

func (f *fakeTmux) Run(args []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), args...))
	var b strings.Builder
	switch args[0] {
	case "show-options":
		// agent-tmux reads a tag only as show-options -p|-w -q -v -t ID NAME.
		if len(args) != 7 || (args[1] != "-p" && args[1] != "-w") || args[2] != "-q" || args[3] != "-v" || args[4] != "-t" {
			return "", fmt.Errorf("fake: unexpected show-options argv %q", args)
		}
		id, k := args[5], args[6]
		var v string
		if args[1] == "-p" {
			p := f.pane(id)
			if p == nil {
				return "", nil // -q: an unknown target prints nothing
			}
			v = p[k]
		} else {
			w := id
			if p := f.pane(id); p != nil {
				w = p["window_id"]
			}
			if f.window(w) == nil {
				return "", nil
			}
			v = f.winOpts[w][k]
		}
		if v == "" {
			return "", nil
		}
		return v + "\n", nil
	case "show-environment":
		scope, k := "-g", args[len(args)-1]
		if args[1] == "-t" {
			scope = args[2]
		}
		v, ok := f.env[scope][k]
		switch {
		case !ok:
			return "", errors.New("unknown variable: " + k)
		case v == "-":
			return "-" + k + "\n", nil
		}
		return k + "=" + v + "\n", nil
	case "send-keys":
		if !reSendKeys.MatchString(strings.Join(args, "\x00")) {
			return "", fmt.Errorf("fake: send-keys argv outside the one allowed shape: %q", args)
		}
		p := f.pane(args[3])
		if p == nil {
			return "", errors.New("can't find pane")
		}
		p["typed"] += args[5]
		return "", nil
	case "list-panes":
		format := flagVal(args, "-F")
		sess := flagVal(args, "-t")
		for _, p := range f.panes {
			if has(args, "-a") || (has(args, "-s") && p["session_id"] == sess) {
				b.WriteString(f.expand(format, p) + "\n")
			}
		}
		return b.String(), nil
	case "display-message":
		p := f.pane(flagVal(args, "-t"))
		if p == nil {
			return "", errors.New("can't find pane")
		}
		if !has(args, "-p") {
			for _, c := range f.clients {
				if c["client_name"] == flagVal(args, "-c") {
					return "", nil
				}
			}
			return "", errors.New("can't find client")
		}
		return f.expand(args[len(args)-1], p) + "\n", nil
	case "capture-pane":
		p := f.pane(flagVal(args, "-t"))
		if p == nil {
			return "", errors.New("can't find pane")
		}
		return p["content"], nil
	case "list-clients":
		for _, c := range f.clients {
			if c["session_id"] == flagVal(args, "-t") {
				b.WriteString(f.expand(flagVal(args, "-F"), c) + "\n")
			}
		}
		return b.String(), nil
	case "new-window":
		p := f.spawn(strings.TrimSuffix(flagVal(args, "-t"), ":"), "", args)
		return f.expand(flagVal(args, "-F"), p) + "\n", nil
	case "split-window":
		src := f.pane(flagVal(args, "-t"))
		if src == nil {
			return "", errors.New("can't find pane")
		}
		p := f.spawn(src["session_id"], src["window_id"], args)
		return f.expand(flagVal(args, "-F"), p) + "\n", nil
	case "set-option":
		k, v := args[len(args)-2], args[len(args)-1]
		if has(args, "-u") {
			// set-option -p|-w -u -t ID NAME: unset at that level.
			k, v = args[len(args)-1], ""
		}
		if has(args, "-w") {
			f.setWinOpt(flagVal(args, "-t"), k, v)
			return "", nil
		}
		p := f.pane(flagVal(args, "-t"))
		if p == nil {
			return "", errors.New("can't find pane")
		}
		p[k] = v
		return "", nil
	case "rename-window":
		for _, p := range f.panes {
			if p["window_id"] == flagVal(args, "-t") {
				p["window_name"] = args[len(args)-1]
			}
		}
		return "", nil
	case "select-pane":
		if p := f.pane(flagVal(args, "-t")); p != nil {
			p["pane_title"] = flagVal(args, "-T")
		}
		return "", nil
	case "resize-pane":
		return "", nil
	case "kill-pane":
		id := flagVal(args, "-t")
		f.remove(func(p map[string]string) bool { return p["pane_id"] != id })
		return "", nil
	case "kill-window":
		id := flagVal(args, "-t")
		f.remove(func(p map[string]string) bool { return p["window_id"] != id })
		return "", nil
	}
	return "", fmt.Errorf("fake: unhandled %v", args)
}

func (f *fakeTmux) count(cmd string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c[0] == cmd {
			n++
		}
	}
	return n
}

func mkPane(pid, paneID, sess, win string, winIdx, paneIdx int, active bool, extra ...string) map[string]string {
	a := "0"
	if active {
		a = "1"
	}
	p := map[string]string{
		"pane_pid": pid, "pane_id": paneID, "session_id": sess, "session_name": "s" + sess[1:],
		"window_id": win, "window_index": fmt.Sprint(winIdx), "window_name": "w" + win[1:],
		"window_linked": "0", "session_grouped": "0", "window_active": "1", "window_last_flag": "0",
		"pane_index": fmt.Sprint(paneIdx), "pane_active": a, "@agent-tmux_owned": "", "@agent-tmux_owned_window": "", "@claude_state": "",
		"pid": fakeServerPID, "@agent-tmux_owner": "", "@agent-tmux_name": "", "@agent-tmux_typed": "",
		"pane_synchronized": "0", "pane_in_mode": "0",
		"pane_current_command": "bash", "pane_current_path": "/home/u/src", "pane_width": "120", "pane_height": "40",
		"content": "",
	}
	for i := 0; i+1 < len(extra); i += 2 {
		p[extra[i]] = extra[i+1]
	}
	return p
}

// standardFake: home session $1 has window 0 (@1: %1 home, %2) and window 3 (@4: %5 owned); session $2 has %9.
func standardFake() *fakeTmux {
	return &fakeTmux{panes: []map[string]string{
		mkPane("800", "%1", "$1", "@1", 0, 0, true, "@claude_state", "working", "pane_current_command", "claude"),
		mkPane("801", "%2", "$1", "@1", 0, 1, false, "content", "\x1b[31mred\x1b[0m line\nsecond\n\n\n"),
		mkPane("802", "%5", "$1", "@4", 3, 0, true, "@agent-tmux_owned", "1", "window_active", "0", "window_last_flag", "1", "content", "a\nb\nc\nd\n"),
		mkPane("900", "%9", "$2", "@7", 0, 0, true, "content", "secret"),
	}, env: map[string]map[string]string{"-g": {"PATH": fakePath}}}
}

// fakeServerPID is the tmux server pid every fake pane reports as #{pid}.
const fakeServerPID = "4242"

// fakePath is the global PATH in standardFake.
const fakePath = "/home/u/.local/bin:/usr/local/bin:/usr/bin:/bin"

// fakeProc writes /proc-style stat files: chain maps pid -> {ppid, comm}. Each process leads its own process
// group and session and holds its terminal's foreground (pgrp, session and tpgid all equal its pid), as a shell
// at its prompt does; setTPGID changes that.
func fakeProc(t *testing.T, chain map[int]struct {
	ppid int
	comm string
}) string {
	t.Helper()
	root := t.TempDir()
	for pid, v := range chain {
		writeStat(t, root, pid, v.comm, v.ppid, pid)
	}
	return root
}

func statFile(root string, pid int, comm string, ppid, tpgid int) error {
	return groupStatFile(root, pid, comm, ppid, pid, tpgid)
}

// groupStatFile writes a stat whose process is in process group pgrp; its session is still its own pid.
func groupStatFile(root string, pid int, comm string, ppid, pgrp, tpgid int) error {
	d := filepath.Join(root, fmt.Sprint(pid))
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	stat := fmt.Sprintf("%d (%s) S %d %d %d 34816 %d 4194560 0 0 0 0 0 0 0 0 20 0 1 0", pid, comm, ppid, pgrp, pid, tpgid)
	return os.WriteFile(filepath.Join(d, "stat"), []byte(stat), 0o644)
}

func writeStat(t *testing.T, root string, pid int, comm string, ppid, tpgid int) {
	t.Helper()
	if err := statFile(root, pid, comm, ppid, tpgid); err != nil {
		t.Fatal(err)
	}
}

func writeGroupStat(t *testing.T, root string, pid int, comm string, ppid, pgrp, tpgid int) {
	t.Helper()
	if err := groupStatFile(root, pid, comm, ppid, pgrp, tpgid); err != nil {
		t.Fatal(err)
	}
}

// setTPGID puts process group tpgid in the foreground of pid's terminal, as when its shell runs a job.
func setTPGID(t *testing.T, root string, pid, tpgid int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, fmt.Sprint(pid), "stat"))
	if err != nil {
		t.Fatal(err)
	}
	ppid, comm, err := parseStat(string(b))
	if err != nil {
		t.Fatal(err)
	}
	writeStat(t, root, pid, comm, ppid, tpgid)
}

// standardProc: agent-tmux (1000) <- claude (950) <- bash (800, the home pane_pid) <- tmux server (700) <- init, and
// a shell at its prompt for each other pane the fakes use: 801 (%2), 802 (%5), 803 (%6), 830 (%30), 900 (%9).
func standardProc(t *testing.T) string {
	return fakeProc(t, map[int]struct {
		ppid int
		comm string
	}{1000: {950, "agent-tmux-mcp"}, 950: {800, "claude"}, 800: {700, "bash"}, 700: {1, "tmux: server"},
		801: {700, "bash"}, 802: {700, "bash"}, 803: {700, "bash"}, 830: {700, "zsh"}, 900: {700, "bash"}})
}
