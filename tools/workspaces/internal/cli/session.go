package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"workspaces/internal/config"
	"workspaces/internal/launchlog"
	"workspaces/internal/sessions"
)

const usageSession = `Usage: workspaces session list [--harness H] [-n N] [--json] [space]
       workspaces session resume [--harness H] [space] [session]
       workspaces session prune [--harness H] [--yes] [space]

Claude sessions only: a harness whose adapter is not claude is refused (64).
A space's sessions are the transcripts in <config dir>/projects whose cwd is
the space root or below it, worktrees included. The config dir is the
harness's CLAUDE_CONFIG_DIR, then the environment's, then ~/.claude; --harness
picks the harness (default: the default harness).

list prints the newest N sessions (default 20): last-modified time, short id,
title and, for a session started through workspaces with the SessionStart hook
installed, where it ran in tmux (session:window and pane id). The title is the
custom title, else the AI title, else the last prompt. --json prints one
object per line.

resume starts the session with "workspaces run <space> <harness> --resume <id>", so
it is checked, logged and placed in tmux like any launch. The session is an
id, or a unique start of one. A missing session is picked on a terminal; one
argument is the space when it names a space or is ".", otherwise the session.
It starts in the space root, which may differ from where the session ran.

prune picks sessions with a multi-select, asks for confirmation, then deletes
each chosen <id>.jsonl and a folder <id>/ beside it; nothing else under the
config dir is touched. It needs a terminal to pick on (exit 64 without one);
--yes skips the confirmation. Declining is exit 130.
` + spaceArgHelp

const listSessions = 20

// sessionTimeLayout is shown in the zone of the app's clock.
const sessionTimeLayout = "2006-01-02 15:04"

func (a *App) sessionCmd(args []string) int {
	return a.dispatch("session", usageSession, []verb{
		{"list", a.sessionList}, {"resume", a.sessionResume}, {"prune", a.sessionPrune},
	}, args)
}

// sessionScope is where one space's sessions live for one harness.
type sessionScope struct {
	space     *config.Space
	harness   string
	configDir string
	roots     []string
}

// sessionSpace resolves the harness and the space of a session command from the
// loaded config, with harnessName and spaceArg as the user gave them.
func (a *App) sessionSpace(cfg *config.Spaces, hs *config.Harnesses, harnessName, spaceArg string) (*sessionScope, int) {
	if harnessName == "" {
		harnessName = hs.Default
	}
	h, ok := hs.Harnesses[harnessName]
	if !ok {
		return nil, a.fail(exitUsage, "unknown harness %q; configured: %s", harnessName, strings.Join(hs.Names(), ", "))
	}
	if h.Adapter != "claude" {
		return nil, a.fail(exitUsage, "harness %s uses the %s adapter; session commands read Claude sessions only", harnessName, h.Adapter)
	}
	sp, code := a.viewSpace(cfg, spaceArg)
	if code != exitOK {
		return nil, code
	}
	dir := h.Env["CLAUDE_CONFIG_DIR"]
	if dir == "" {
		if env := a.Getenv("CLAUDE_CONFIG_DIR"); filepath.IsAbs(env) {
			dir = env
		} else {
			dir = filepath.Join(a.home(), ".claude")
		}
	}
	roots := []string{sp.Root.Path}
	if sp.Root.Logical != sp.Root.Path {
		roots = append(roots, sp.Root.Logical)
	}
	return &sessionScope{space: sp, harness: harnessName, configDir: dir, roots: roots}, exitOK
}

func (a *App) loadBoth() (*config.Spaces, *config.Harnesses, int) {
	cfg, code := a.loadSpacesOnly()
	if code != exitOK {
		return nil, nil, code
	}
	_, path, err := a.configPaths()
	var hs *config.Harnesses
	if err == nil {
		hs, err = config.LoadHarnesses(path, a.home(), knownAdapter)
	}
	if err != nil {
		return nil, nil, a.fail(exitConfig, "%v", err)
	}
	return cfg, hs, exitOK
}

// findSessions lists the scope's sessions, newest first.
func (a *App) findSessions(sc *sessionScope, limit int) ([]sessions.Session, int) {
	found, err := sessions.Find(sc.configDir, sc.roots, limit)
	if err != nil {
		return nil, a.fail(exitIOErr, "reading %s: %v", a.tildePath(sc.configDir), err)
	}
	return found, exitOK
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// tmuxPlace is where a session ran, as session:window %pane.
func tmuxPlace(t *launchlog.Tmux) string {
	if t == nil || t.Session == "" {
		return ""
	}
	place := t.Session
	if t.WindowIndex != nil {
		place += fmt.Sprintf(":%d", *t.WindowIndex)
	}
	if t.PaneID != "" {
		place += " " + t.PaneID
	}
	return place
}

// sessionPlaces maps a session id to the tmux place of its latest launch that
// has one. It joins sessions.jsonl, written by the SessionStart hook, to the
// launch log by launch id. A log that cannot be read leaves the places blank,
// and is reported on stderr.
func (a *App) sessionPlaces() map[string]*launchlog.Tmux {
	dir, err := config.StateDir(a.Getenv)
	if err != nil {
		return nil
	}
	joins, bad, err := launchlog.ReadSessions(filepath.Join(dir, "sessions.jsonl"))
	if err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: reading sessions.jsonl: %v\n", err)
		return nil
	}
	if bad > 0 {
		fmt.Fprintf(a.Stderr, "workspaces: skipped %d malformed line(s) in sessions.jsonl\n", bad)
	}
	if len(joins) == 0 {
		return nil
	}
	lines, code := a.launches()
	if code != exitOK {
		return nil
	}
	byID := make(map[string]*launchlog.Tmux, len(lines))
	for _, l := range lines {
		if l.Record.Decision == "allow" && l.Record.Tmux != nil {
			byID[l.Record.ID] = l.Record.Tmux
		}
	}
	places := map[string]*launchlog.Tmux{}
	for i := len(joins) - 1; i >= 0; i-- {
		j := joins[i]
		if _, done := places[j.SessionID]; done {
			continue
		}
		if t, ok := byID[j.LaunchID]; ok {
			places[j.SessionID] = t
		}
	}
	return places
}

type sessionJSON struct {
	ID       string          `json:"id"`
	Modified string          `json:"modified"`
	Title    string          `json:"title"`
	Cwd      string          `json:"cwd"`
	Tmux     *launchlog.Tmux `json:"tmux"`
}

func (a *App) sessionList(args []string) int {
	fs := a.flagSet("session list", usageSession)
	harnessName := fs.String("harness", "", "harness whose config dir holds the sessions")
	n := fs.Int("n", listSessions, "number of sessions")
	jsonOut := fs.Bool("json", false, "print one JSON object per line")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 1 || *n < 1 {
		fmt.Fprint(a.Stderr, usageSession)
		return exitUsage
	}
	cfg, hs, code := a.loadBoth()
	if code != exitOK {
		return code
	}
	sc, code := a.sessionSpace(cfg, hs, *harnessName, argAt(pos, 0))
	if code != exitOK {
		return code
	}
	found, code := a.findSessions(sc, *n)
	if code != exitOK {
		return code
	}
	places := a.sessionPlaces()
	loc := a.Now().Location()

	if *jsonOut {
		enc := json.NewEncoder(a.Stdout)
		enc.SetEscapeHTML(false)
		for _, s := range found {
			if err := enc.Encode(sessionJSON{
				ID: s.ID, Modified: launchlog.FormatTime(s.Modified.In(loc)), Title: clean(sessions.Title(s.Path)),
				Cwd: s.Cwd, Tmux: places[s.ID],
			}); err != nil {
				return a.fail(exitInternal, "%v", err)
			}
		}
		return exitOK
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODIFIED\tID\tTITLE\tTMUX")
	for _, s := range found {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Modified.In(loc).Format(sessionTimeLayout), shortID(s.ID),
			cut(clean(sessions.Title(s.Path)), titleWidth), orDash(clean(tmuxPlace(places[s.ID]))))
	}
	tw.Flush()
	return exitOK
}

// sessionItems renders the picker lines for ss and maps each back to its
// session. Two sessions that render alike are told apart by a suffix.
func (a *App) sessionItems(ss []sessions.Session) ([]string, map[string]sessions.Session) {
	loc := a.Now().Location()
	items := make([]string, 0, len(ss))
	byLine := make(map[string]sessions.Session, len(ss))
	for _, s := range ss {
		line := fmt.Sprintf("%s  %s  %s", shortID(s.ID), s.Modified.In(loc).Format(sessionTimeLayout), cut(clean(sessions.Title(s.Path)), titleWidth))
		for i := 2; ; i++ {
			if _, taken := byLine[line]; !taken {
				break
			}
			line = fmt.Sprintf("%s  #%d", line, i)
		}
		items = append(items, line)
		byLine[line] = s
	}
	return items, byLine
}

func (a *App) sessionResume(args []string) int {
	fs := a.flagSet("session resume", usageSession)
	harnessName := fs.String("harness", "", "harness to resume with")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 2 {
		fmt.Fprint(a.Stderr, usageSession)
		return exitUsage
	}
	cfg, hs, code := a.loadBoth()
	if code != exitOK {
		return code
	}
	spaceArg, sessionArg := argAt(pos, 0), argAt(pos, 1)
	if len(pos) == 1 {
		if _, isSpace := cfg.Space(pos[0]); !isSpace && pos[0] != "." {
			spaceArg, sessionArg = "", pos[0]
		}
	}
	sc, code := a.sessionSpace(cfg, hs, *harnessName, spaceArg)
	if code != exitOK {
		return code
	}
	found, code := a.findSessions(sc, 0)
	if code != exitOK {
		return code
	}
	if len(found) == 0 {
		return a.fail(exitUsage, "space %s has no sessions in %s", sc.space.Name, a.tildePath(sc.configDir))
	}

	var chosen sessions.Session
	if sessionArg == "" {
		if !a.IsTTY() {
			return a.fail(exitUsage, "missing session; no TTY")
		}
		items, byLine := a.sessionItems(found)
		line, code := a.pickLine("session", items)
		if code != exitOK {
			return code
		}
		chosen = byLine[line]
	} else {
		var matches []sessions.Session
		for _, s := range found {
			if s.ID == sessionArg {
				matches = []sessions.Session{s}
				break
			}
			if strings.HasPrefix(s.ID, sessionArg) {
				matches = append(matches, s)
			}
		}
		switch len(matches) {
		case 0:
			return a.fail(exitUsage, "no session %q in space %s", sessionArg, sc.space.Name)
		case 1:
			chosen = matches[0]
		default:
			return a.fail(exitUsage, "session %q is ambiguous: %d sessions start with it", sessionArg, len(matches))
		}
	}

	if !slicesContainsPath(sc.roots, chosen.Cwd) {
		fmt.Fprintf(a.Stderr, "workspaces: warning: session %s ran in %s; resume starts in the space root %s, where Claude Code may not find it\n",
			shortID(chosen.ID), a.tildePath(chosen.Cwd), a.tildePath(sc.space.Root.Path))
	}
	// The id comes from a transcript file name, so it becomes an argument only when it cannot be
	// read as an option or carry anything but an id's characters (Claude Code ids are UUIDs).
	if !resumableID.MatchString(chosen.ID) {
		return a.fail(exitUsage, "session file name %q is not a session id; refusing to pass it to the harness", chosen.ID)
	}
	return a.runCmd([]string{sc.space.Name, sc.harness, "--resume", chosen.ID})
}

var resumableID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func slicesContainsPath(roots []string, p string) bool {
	p = filepath.Clean(p)
	for _, r := range roots {
		if filepath.Clean(r) == p {
			return true
		}
	}
	return false
}

func (a *App) sessionPrune(args []string) int {
	fs := a.flagSet("session prune", usageSession)
	harnessName := fs.String("harness", "", "harness whose config dir holds the sessions")
	yes := fs.Bool("yes", false, "skip the confirmation")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 1 {
		fmt.Fprint(a.Stderr, usageSession)
		return exitUsage
	}
	cfg, hs, code := a.loadBoth()
	if code != exitOK {
		return code
	}
	sc, code := a.sessionSpace(cfg, hs, *harnessName, argAt(pos, 0))
	if code != exitOK {
		return code
	}
	found, code := a.findSessions(sc, 0)
	if code != exitOK {
		return code
	}
	if len(found) == 0 {
		fmt.Fprintf(a.Stdout, "no sessions to prune in space %s\n", sc.space.Name)
		return exitOK
	}
	if !a.IsTTY() {
		return a.fail(exitUsage, "missing selection; no TTY to pick sessions on")
	}

	items, byLine := a.sessionItems(found)
	lines, code := a.pickMany("sessions", items)
	if code != exitOK {
		return code
	}
	var chosen []sessions.Session
	for _, l := range lines {
		if s, ok := byLine[strings.TrimSpace(l)]; ok {
			chosen = append(chosen, s)
		}
	}
	if len(chosen) == 0 {
		return exitCancelled
	}
	for _, s := range chosen {
		fmt.Fprintf(a.Stdout, "will delete %s  %s\n", s.ID, cut(clean(sessions.Title(s.Path)), titleWidth))
	}
	if code := a.confirm(*yes, "Delete "+plural(len(chosen), "session")+"?"); code != exitOK {
		return code
	}

	code = exitOK
	for _, s := range chosen {
		folder, err := deleteSession(s)
		if err != nil {
			fmt.Fprintf(a.Stderr, "workspaces: %v\n", err)
			code = exitIOErr
			continue
		}
		line := "deleted " + s.ID
		if folder {
			line += " and its folder"
		}
		fmt.Fprintln(a.Stdout, line)
	}
	return code
}

// deleteSession removes the transcript and a folder named after the session
// beside it, and reports whether there was a folder. The folder is removed
// only when it is a real directory, never a symlink to one.
func deleteSession(s sessions.Session) (folder bool, err error) {
	if err := os.Remove(s.Path); err != nil {
		return false, err
	}
	dir := filepath.Join(filepath.Dir(s.Path), s.ID)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return false, nil
	}
	return true, os.RemoveAll(dir)
}
