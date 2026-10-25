package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"workspaces/internal/config"
	"workspaces/internal/launchlog"
)

const usageHook = `Usage: workspaces hook session-start

Run by Claude Code's SessionStart hook, not by hand. Reads the hook's JSON on
stdin and, when AGENT_LAUNCH_ID is set (a session agent launched), appends
{"ts","launch_id","session_id","cwd","source"} to
~/.local/state/workspaces/sessions.jsonl, which "workspaces session list" uses to show
where a session ran. It prints nothing on stdout, which Claude Code would add
to the session, and always exits 0: a hook must never fail a session. Errors go
to stderr.
`

// maxHookInput bounds what the hook reads from stdin; the real payload is a
// few hundred bytes.
const maxHookInput = 1 << 20

func (a *App) hookCmd(args []string) int {
	return a.dispatch("hook", usageHook, []verb{{"session-start", a.hookSessionStart}}, args)
}

func (a *App) hookSessionStart(args []string) int {
	if len(args) > 0 {
		fmt.Fprint(a.Stderr, usageHook)
		return exitUsage
	}
	id := a.Getenv("AGENT_LAUNCH_ID")
	if id == "" {
		return exitOK
	}
	if err := a.recordSession(id); err != nil {
		fmt.Fprintf(a.Stderr, "workspaces hook session-start: %v\n", err)
	}
	return exitOK
}

func (a *App) recordSession(launchID string) error {
	var in struct {
		SessionID string `json:"session_id"`
		Cwd       string `json:"cwd"`
		Source    string `json:"source"`
	}
	data, err := io.ReadAll(io.LimitReader(a.Stdin, maxHookInput))
	if err == nil {
		err = json.Unmarshal(data, &in)
	}
	if err != nil {
		return fmt.Errorf("reading the hook input: %w", err)
	}
	if in.SessionID == "" {
		return fmt.Errorf("the hook input has no session_id")
	}
	dir, err := config.StateDir(a.Getenv)
	if err != nil {
		return err
	}
	return launchlog.AppendSession(filepath.Join(dir, "sessions.jsonl"), launchlog.SessionRecord{
		TS:        launchlog.FormatTime(a.Now()),
		LaunchID:  launchID,
		SessionID: in.SessionID,
		Cwd:       in.Cwd,
		Source:    in.Source,
	})
}
