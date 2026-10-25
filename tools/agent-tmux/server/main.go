// agent-tmux-mcp: MCP server (stdio) that lets a Claude Code agent see and lay out its own tmux session. See ../docs/agent-tmux-SPEC.md.
package main

import (
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"
)

// sweepInterval is how often the server looks for panes that moved, closed or lost their tag.
const sweepInterval = 5 * time.Second

func main() {
	logger := log.New(os.Stderr, "agent-tmux: ", log.LstdFlags)
	// Never use the inherited cwd.
	if err := os.Chdir("/"); err != nil {
		logger.Fatalf("chdir /: %v", err)
	}
	home := homeDir()
	logPath := ""
	if home != "" {
		logPath = filepath.Join(home, ".claude", "logs", "agent-tmux.log")
	} else {
		logger.Printf("no home directory: audit log unavailable, all tools will refuse")
	}
	r := ExecRunner{Home: home}
	h := DetectHome(tmux{r}, defaultProcRoot, os.Getpid())
	if h.Err != nil {
		logger.Printf("%v (all tools will refuse)", h.Err)
	} else {
		logger.Printf("home pane %s in session %s (parent comm %q)", h.PaneID, h.SessionID, h.ParentComm)
	}
	srv := NewServer(r, h, logPath, logger)
	srv.userHome = home
	if evPath := eventLogPath(home, os.Getenv("AGENT_TMUX_EVENT_LOG")); evPath != "" {
		srv.events = NewEventLog(evPath)
	} else {
		logger.Printf("no event log path: events will not be recorded")
	}
	srv.sweepEvery = sweepInterval
	if err := serveStdio(srv); err != nil {
		logger.Fatalf("serve: %v", err)
	}
}

// serveStdio serves on stdin and stdout. Go kills a program by SIGPIPE when a write to a closed stdout fails, and
// that could happen while another call sits between its intent line and its outcome. Ignored, the write just
// returns EPIPE: Serve stops reading, and every call still running finishes and writes its audit line first.
func serveStdio(srv *Server) error {
	signal.Ignore(syscall.SIGPIPE)
	return srv.Serve(os.Stdin, os.Stdout)
}

// eventLogPath is ~/.local/state/agent-tmux/events.jsonl unless override is an absolute path (AGENT_TMUX_EVENT_LOG, meant
// for tests). A relative override is ignored: the server's cwd is "/", so it would not mean what the writer meant.
func eventLogPath(home, override string) string {
	if filepath.IsAbs(override) {
		return override
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "agent-tmux", "events.jsonl")
}

// homeDir prefers the passwd entry over $HOME; both must be absolute.
func homeDir() string {
	if u, err := user.Current(); err == nil && filepath.IsAbs(u.HomeDir) {
		return u.HomeDir
	}
	if h := os.Getenv("HOME"); filepath.IsAbs(h) {
		return h
	}
	return ""
}
