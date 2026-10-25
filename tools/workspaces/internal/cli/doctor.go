package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"

	"workspaces/internal/config"
	"workspaces/internal/harness"
	"workspaces/internal/policy"
	"workspaces/internal/tmux"
)

const usageDoctor = `Usage: workspaces doctor

Checks that workspaces is set up: the config files, the paths they name, each
space's launch check, harness binaries and their config dirs, the wrappers in
~/.bash_aliases, the protection entries and the SessionStart hook in
~/.claude/settings.json (read only), that the launch log is writable, that
workspaces (or its awm link) is ~/.local/bin/workspaces on PATH, tmux, and which spaces still keep their
todo list in TODO.md.

One row per check: OK, WARN or FAIL, the check, the detail. Missing protection
entries are WARN, since adding them is your choice. Exit 0 when nothing FAILs,
78 otherwise. Writes nothing.
`

const (
	levelOK   = "OK"
	levelWarn = "WARN"
	levelFail = "FAIL"
)

type doctorRow struct{ level, check, detail string }

// The section "protection" entries for ~/.claude/settings.json.
var (
	requiredDeny    = []string{"Edit(~/.config/workspaces/**)", "Edit(~/.local/bin/workspaces)", "Edit(~/.local/bin/awm)"}
	recommendedDeny = []string{
		"Edit(~/.local/state/workspaces/**)", "Edit(~/.bash_aliases)", "Edit(~/.bashrc)", "Edit(~/base.bashrc)",
		"Edit(~/.tmux.conf)", "Edit(~/.tmux.base.conf)", "Edit(~/.tmux/**)",
	}
	optionalDeny       = []string{"Edit(~/.claude/settings.json)"}
	recommendedSandbox = []string{"~/.config/workspaces", "~/.local/bin/workspaces", "~/.local/bin/awm", "~/.local/state/workspaces"}
)

const sessionHookCommand = "workspaces hook session-start"

// accessW is W_OK for access(2).
const accessW = 2

func (a *App) doctorCmd(args []string) int {
	fs := a.flagSet("doctor", usageDoctor)
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() > 0 {
		fmt.Fprint(a.Stderr, usageDoctor)
		return exitUsage
	}
	rows := a.doctorRows()
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	code := exitOK
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.level, r.check, clean(r.detail))
		if r.level == levelFail {
			code = exitConfig
		}
	}
	tw.Flush()
	return code
}

func (a *App) doctorRows() []doctorRow {
	var rows []doctorRow
	add := func(level, check, format string, args ...any) {
		rows = append(rows, doctorRow{level, check, fmt.Sprintf(format, args...)})
	}

	cfg, hs, err := a.doctorConfig()
	if err != nil {
		add(levelFail, "config", "%v", err)
	} else {
		add(levelOK, "config", "%d spaces, %d harnesses, default %s", len(cfg.Spaces), len(hs.Harnesses), hs.Default)
	}
	if cfg != nil {
		rows = append(rows, a.checkPaths(cfg), a.checkSpaceRoots(cfg), a.checkAttach(cfg))
	}
	if hs != nil {
		rows = append(rows, a.checkBinaries(hs), a.checkConfigDirs(hs), a.checkWrappers(hs))
	}
	rows = append(rows, a.checkSettings(), a.checkHook(), a.checkLogWritable(), a.checkAgentPath(), a.checkTmux())
	if cfg != nil {
		rows = append(rows, checkTodo(cfg))
	}
	return rows
}

// doctorConfig loads both files. When one is invalid the other is still
// returned, so the checks that need only it can run.
func (a *App) doctorConfig() (*config.Spaces, *config.Harnesses, error) {
	spacesPath, harnessPath, err := a.configPaths()
	if err != nil {
		return nil, nil, err
	}
	cfg, _, spacesErr := config.LoadSpaces(spacesPath, a.home())
	hs, harnessErr := config.LoadHarnesses(harnessPath, a.home(), knownAdapter)
	return cfg, hs, errors.Join(spacesErr, harnessErr)
}

func (a *App) checkPaths(cfg *config.Spaces) doctorRow {
	if missing := cfg.Missing(); len(missing) > 0 {
		return doctorRow{levelWarn, "paths", strings.Join(missing, "; ")}
	}
	return doctorRow{levelOK, "paths", "every configured path exists"}
}

func (a *App) checkSpaceRoots(cfg *config.Spaces) doctorRow {
	var bad []string
	for _, name := range cfg.SpaceNames() {
		sp := cfg.Spaces[name]
		root := policy.Cwd{Logical: sp.Root.Path, Physical: sp.Root.Path}
		// attach_denied is the attach paths' check, reported on its own row.
		if d := policy.Decide(root, cfg, sp); !d.Allow && d.Reason != policy.ReasonAttachDenied {
			bad = append(bad, name+": "+reasonRule(d))
		}
	}
	if len(bad) > 0 {
		return doctorRow{levelFail, "space roots", strings.Join(bad, "; ")}
	}
	return doctorRow{levelOK, "space roots", plural(len(cfg.Spaces), "space") + " pass the launch check"}
}

func (a *App) checkAttach(cfg *config.Spaces) doctorRow {
	var bad []string
	for _, name := range cfg.SpaceNames() {
		sp := cfg.Spaces[name]
		for _, e := range sp.Attach {
			if rule, denied := attachDenied(cfg, sp, e); denied {
				bad = append(bad, fmt.Sprintf("%s attach %s is under deny_access %s", name, e.Written, rule))
			}
		}
	}
	if len(bad) > 0 {
		return doctorRow{levelFail, "attach paths", strings.Join(bad, "; ")}
	}
	return doctorRow{levelOK, "attach paths", "none is under deny_access"}
}

func (a *App) checkBinaries(hs *config.Harnesses) doctorRow {
	var found, missing []string
	for _, name := range hs.Names() {
		h := hs.Harnesses[name]
		bin := h.Bin
		if bin == "" {
			if ad, ok := harness.Lookup(h.Adapter); ok {
				bin = ad.Binary()
			}
		}
		if path, err := a.LookPath(bin); err != nil {
			missing = append(missing, name+" ("+bin+")")
		} else {
			found = append(found, name+"="+a.tildePath(path))
		}
	}
	if len(missing) > 0 {
		return doctorRow{levelFail, "harness binaries", "not found: " + strings.Join(missing, ", ")}
	}
	return doctorRow{levelOK, "harness binaries", strings.Join(found, ", ")}
}

// configDirVars are the harness env variables that name a profile's folder.
var configDirVars = []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME"}

func (a *App) checkConfigDirs(hs *config.Harnesses) doctorRow {
	var bad []string
	checked := 0
	for _, name := range hs.Names() {
		for _, key := range configDirVars {
			dir, ok := hs.Harnesses[name].Env[key]
			if !ok {
				continue
			}
			checked++
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				bad = append(bad, fmt.Sprintf("%s: %s=%s", name, key, a.tildePath(dir)))
			}
		}
	}
	if len(bad) > 0 {
		return doctorRow{levelWarn, "config dirs", "missing: " + strings.Join(bad, ", ")}
	}
	return doctorRow{levelOK, "config dirs", fmt.Sprintf("%d set, all exist", checked)}
}

// checkWrappers wants a bash function per harness that calls workspaces run with
// the harness's name, in ~/.bash_aliases.
func (a *App) checkWrappers(hs *config.Harnesses) doctorRow {
	path := filepath.Join(a.home(), ".bash_aliases")
	data, err := os.ReadFile(path)
	if err != nil {
		return doctorRow{levelWarn, "wrappers", fmt.Sprintf("cannot read %s: %v", a.tildePath(path), err)}
	}
	var missing []string
	for _, name := range hs.Names() {
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*\(\)\s*\{[^}\n]*\b(?:workspaces|awm) run \. ` + regexp.QuoteMeta(name) + `(\s|;|$)`)
		if !re.Match(data) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return doctorRow{levelWarn, "wrappers", "no workspaces wrapper in ~/.bash_aliases for: " + strings.Join(missing, ", ")}
	}
	return doctorRow{levelOK, "wrappers", "~/.bash_aliases wraps " + strings.Join(hs.Names(), ", ")}
}

// settingsFile is the part of ~/.claude/settings.json that doctor reads.
type settingsFile struct {
	Permissions struct {
		Deny []string `json:"deny"`
	} `json:"permissions"`
	Sandbox struct {
		Filesystem struct {
			DenyWrite []string `json:"denyWrite"`
		} `json:"filesystem"`
	} `json:"sandbox"`
	Hooks map[string][]struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// readSettings reads ~/.claude/settings.json. A missing file is a file with
// nothing in it.
func (a *App) readSettings() (settingsFile, string, error) {
	path := filepath.Join(a.home(), ".claude", "settings.json")
	var s settingsFile
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, path, nil
	case err != nil:
		return s, path, err
	}
	return s, path, json.Unmarshal(data, &s)
}

func (a *App) checkSettings() doctorRow {
	s, path, err := a.readSettings()
	if err != nil {
		return doctorRow{levelWarn, "settings", fmt.Sprintf("cannot read %s: %v", a.tildePath(path), err)}
	}
	missing := func(want, have []string) []string {
		var out []string
		for _, w := range want {
			if !slices.Contains(have, w) {
				out = append(out, w)
			}
		}
		return out
	}
	required := missing(requiredDeny, s.Permissions.Deny)
	recommended := missing(recommendedDeny, s.Permissions.Deny)
	for _, p := range missing(recommendedSandbox, s.Sandbox.Filesystem.DenyWrite) {
		recommended = append(recommended, "sandbox denyWrite "+p)
	}
	optional := missing(optionalDeny, s.Permissions.Deny)

	var parts []string
	if len(required) > 0 {
		parts = append(parts, "missing required: "+strings.Join(required, ", "))
	}
	if len(recommended) > 0 {
		parts = append(parts, "missing recommended: "+strings.Join(recommended, ", "))
	}
	if len(parts) > 0 {
		return doctorRow{levelWarn, "settings", a.tildePath(path) + " " + strings.Join(parts, "; ")}
	}
	detail := "all required and recommended entries present"
	if len(optional) > 0 {
		detail += " (optional, not set: " + strings.Join(optional, ", ") + ")"
	}
	return doctorRow{levelOK, "settings", detail}
}

func (a *App) checkHook() doctorRow {
	s, path, err := a.readSettings()
	if err != nil {
		return doctorRow{levelWarn, "session hook", fmt.Sprintf("cannot read %s: %v", a.tildePath(path), err)}
	}
	for _, group := range s.Hooks["SessionStart"] {
		for _, h := range group.Hooks {
			if strings.Contains(h.Command, sessionHookCommand) {
				return doctorRow{levelOK, "session hook", "SessionStart runs " + sessionHookCommand}
			}
		}
	}
	return doctorRow{levelWarn, "session hook", "no SessionStart hook runs `" + sessionHookCommand + "`; session list cannot show where a session ran"}
}

func (a *App) checkLogWritable() doctorRow {
	dir, err := config.StateDir(a.Getenv)
	if err != nil {
		return doctorRow{levelFail, "launch log", err.Error()}
	}
	path := filepath.Join(dir, "launches.jsonl")
	if err := writable(path); err != nil {
		return doctorRow{levelFail, "launch log", fmt.Sprintf("%s is not writable, so nothing can launch: %v", a.tildePath(path), err)}
	}
	return doctorRow{levelOK, "launch log", a.tildePath(path) + " is writable"}
}

// writable tests a file without creating it: an existing one is opened for
// append, a missing one needs a writable folder, the nearest that exists.
func writable(path string) error {
	if _, err := os.Lstat(path); err == nil {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		return f.Close()
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a folder", dir)
			}
			return syscall.Access(dir, accessW)
		}
		if parent := filepath.Dir(dir); parent == dir {
			return err
		}
	}
}

func (a *App) checkAgentPath() doctorRow {
	want := filepath.Join(a.home(), ".local", "bin", "workspaces")
	got, err := a.LookPath("workspaces")
	switch {
	case err != nil:
		return doctorRow{levelWarn, "workspaces binary", "workspaces is not on PATH; expected " + a.tildePath(want)}
	case filepath.Clean(got) != want:
		return doctorRow{levelWarn, "workspaces binary", fmt.Sprintf("command -v workspaces is %s, expected %s", a.tildePath(got), a.tildePath(want))}
	}
	return doctorRow{levelOK, "workspaces binary", a.tildePath(got)}
}

func (a *App) checkTmux() doctorRow {
	path, err := a.LookPath("tmux")
	if err != nil {
		return doctorRow{levelFail, "tmux", "tmux is not on PATH, so no session can launch"}
	}
	out, err := a.Tmux(tmux.ListSessions())
	switch {
	case err == nil:
		return doctorRow{levelOK, "tmux", fmt.Sprintf("%s; server answers, %s", path, plural(len(strings.Fields(out)), "session"))}
	case a.Getenv("TMUX") != "":
		return doctorRow{levelFail, "tmux", "$TMUX is set but the server does not answer, so launches are refused: " + err.Error()}
	case tmux.IsNoServer(err):
		return doctorRow{levelOK, "tmux", path + "; no server running yet"}
	}
	return doctorRow{levelWarn, "tmux", path + "; the server did not answer: " + err.Error()}
}

func checkTodo(cfg *config.Spaces) doctorRow {
	var old []string
	for _, name := range cfg.SpaceNames() {
		sp := cfg.Spaces[name]
		if _, err := os.Stat(sp.Todo); errors.Is(err, os.ErrNotExist) && onlyTodoMD(sp) {
			old = append(old, name)
		}
	}
	if len(old) > 0 {
		return doctorRow{levelWarn, "todo", "still on TODO.md: " + strings.Join(old, ", ")}
	}
	return doctorRow{levelOK, "todo", "no space is on TODO.md alone"}
}
