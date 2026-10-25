package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"workspaces/internal/config"
	"workspaces/internal/harness"
	"workspaces/internal/launchlog"
	"workspaces/internal/policy"
	"workspaces/internal/tmux"
)

const (
	reasonConfigError       = "config_error"
	reasonUnknownSpace      = "unknown_space"
	reasonUnknownHarness    = "unknown_harness"
	reasonHarnessNotFound   = "harness_not_found"
	reasonSettingsCollision = "settings_collision"
	reasonNotInTmux         = "not_in_tmux"
	reasonBackgroundRefused = "background_refused"
	reasonTmuxError         = "tmux_error"
)

type refusal struct {
	// reason is empty for outcomes that are not logged: usage errors, a
	// cancelled picker, internal failures.
	reason string
	rule   string
	code   int
	msg    string
}

type tmuxState struct {
	queried bool
	pane    *tmux.Pane
	socket  string
	err     error
}

type bootstrapPlan struct {
	tmuxPath   string
	session    string
	window     string
	exists     bool
	newSession []string
	newWindow  []string
}

type launchPlan struct {
	spaceArg    string
	harnessName string
	userArgs    []string
	tty         bool

	cfg       *config.Spaces
	cfgHash   string
	harnesses *config.Harnesses
	cwd       policy.Cwd
	space     *config.Space
	launchDir string
	harness   config.Harness
	adapter   harness.Adapter
	decision  policy.Decision
	mode      harness.Mode
	bin       string
	binTarget string
	launchID  string
	addDirs   []string
	denyPaths []string
	// unenforced counts deny_access paths the harness cannot enforce.
	unenforced int
	envAdded   map[string]string
	argv       []string
	env        []string

	tmux      tmuxState
	bootstrap *bootstrapPlan
	refusal   *refusal
}

func (p *launchPlan) refuse(reason, rule string, code int, format string, args ...any) {
	if p.refusal == nil {
		p.refusal = &refusal{reason: reason, rule: rule, code: code, msg: fmt.Sprintf(format, args...)}
	}
}

// unenforcedWarning is empty when the harness enforces every deny_access path.
func (p *launchPlan) unenforcedWarning() string {
	if p.unenforced == 0 {
		return ""
	}
	paths := "paths are"
	if p.unenforced == 1 {
		paths = "path is"
	}
	return fmt.Sprintf("the %s adapter cannot enforce deny_access: %d %s not denied inside this session",
		p.harness.Adapter, p.unenforced, paths)
}

func (p *launchPlan) spaceName() string {
	if p.space == nil {
		return ""
	}
	return p.space.Name
}

const usageRun = `Usage: workspaces run [space] [harness] [harness-args...]

Checks the folder, logs the launch and starts the harness inside tmux.
space is "." for here, or a space name (the harness then starts in its root).
Everything after harness goes to the harness unchanged.
`

const usageCheck = `Usage: workspaces check [--json] [space] [harness] [harness-args...]

Shows what "workspaces run" would do with the same arguments; writes nothing.
space defaults to "." and harness to the default harness.
`

func (a *App) runCmd(args []string) int {
	fs := a.flagSet("run", usageRun)
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	p := a.plan(fs.Args(), true)
	if p.refusal == nil {
		a.planTmux(p)
	}
	var code int
	switch {
	case p.refusal != nil:
		code = a.refuseRun(p)
	case p.bootstrap != nil:
		code = a.bootstrap(p)
	default:
		code = a.launchInPlace(p)
	}
	// tmux closes a bootstrapped window as soon as workspaces exits, which would
	// take any error message with it. A background refusal never runs in one:
	// the outer workspaces refuses the same arguments.
	if code != exitOK && (p.refusal == nil || p.refusal.reason != reasonBackgroundRefused) {
		a.queryTmux(p)
		if a.inBootstrapPane(p) {
			fmt.Fprint(a.Stderr, "Press Enter to close this window.")
			a.WaitEnter()
		}
	}
	return code
}

func (a *App) flagSet(name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.Usage = func() { fmt.Fprint(a.Stderr, usage) }
	return fs
}

func flagExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitUsage
}

// plan runs everything up to the tmux stage. It stops at a refusal that
// leaves nothing further to compute, and otherwise keeps the first refusal
// while filling in what check displays.
func (a *App) plan(pos []string, interactive bool) *launchPlan {
	p := &launchPlan{tty: a.IsTTY(), launchID: launchlog.NewID()}
	if len(pos) > 0 {
		p.spaceArg = pos[0]
	}
	if len(pos) > 1 {
		p.harnessName = pos[1]
		p.userArgs = pos[2:]
	}
	if interactive && !p.tty && p.spaceArg == "" {
		p.refuse("", "", exitUsage, "missing space; no TTY")
		return p
	}

	wd, err := a.Getwd()
	if err == nil {
		p.cwd, err = policy.ResolveCwd(wd)
	}
	if err != nil {
		p.refuse("", "", exitInternal, "cannot resolve the current directory: %v", err)
		return p
	}

	if !a.loadConfig(p) {
		return p
	}
	if !a.chooseSpace(p, interactive) {
		return p
	}

	if p.spaceArg == "." {
		p.space = policy.SelectSpace(p.cfg, p.cwd.Physical)
		p.launchDir = p.cwd.Physical
	} else {
		sp, ok := p.cfg.Space(p.spaceArg)
		if !ok {
			hint := ""
			if _, isHarness := p.harnesses.Harnesses[p.spaceArg]; isHarness {
				hint = fmt.Sprintf("; did you mean: workspaces run . %s", p.spaceArg)
			}
			p.refuse(reasonUnknownSpace, "", exitUsage, "unknown space %q%s", p.spaceArg, hint)
			return p
		}
		p.space = sp
		p.launchDir = sp.Root.Path
	}

	if !a.chooseHarness(p, interactive) {
		return p
	}
	h, ok := p.harnesses.Harnesses[p.harnessName]
	if !ok {
		p.refuse(reasonUnknownHarness, "", exitUsage, "unknown harness %q; configured: %s",
			p.harnessName, strings.Join(p.harnesses.Names(), ", "))
		return p
	}
	p.harness = h
	p.adapter, _ = harness.Lookup(h.Adapter)

	checked := p.cwd
	if p.spaceArg != "." {
		checked = policy.Cwd{Logical: p.launchDir, Physical: p.launchDir}
	}
	p.decision = policy.Decide(checked, p.cfg, p.space)
	if !p.decision.Allow {
		p.refuse(p.decision.Reason, p.decision.Rule, exitRefused, "%s", refusalText(p, checked))
	}
	if p.space != nil {
		for _, e := range p.space.Attach {
			p.addDirs = append(p.addDirs, e.Path)
		}
	}

	p.denyPaths = effectiveDenyPaths(p.cfg, p.space)

	p.mode = p.adapter.Mode(p.userArgs)
	// A utility command starts no agent loop, so nothing it runs reads files.
	if limited, ok := p.adapter.(harness.DenyLimited); ok && p.mode != harness.ModeUtility {
		p.unenforced = limited.UnenforcedDeny(p.denyPaths)
	}

	name := h.Bin
	if name == "" {
		name = p.adapter.Binary()
	}
	if bin, err := a.LookPath(name); err != nil {
		p.refuse(reasonHarnessNotFound, "", exitNotFound, "harness binary %q not found: %v", name, err)
	} else {
		p.bin = bin
		p.binTarget = bin
		if target, err := filepath.EvalSymlinks(bin); err == nil {
			p.binTarget = target
		}
	}

	p.envAdded = map[string]string{}
	for k, v := range h.Env {
		p.envAdded[k] = v
	}
	p.envAdded["AGENT_SPACE"] = p.spaceName()
	p.envAdded["AGENT_HARNESS"] = p.harnessName
	env := map[string]string{"AGENT_LAUNCH_ID": p.launchID}
	for k, v := range p.envAdded {
		env[k] = v
	}
	p.argv, p.env, err = p.adapter.Command(harness.Launch{
		Dir: p.launchDir, AddDirs: p.addDirs, DenyAccess: p.denyPaths, UserArgs: p.userArgs, Env: env, BaseEnv: a.Environ(),
	})
	switch {
	case errors.Is(err, harness.ErrSettingsCollision):
		p.refuse(reasonSettingsCollision, "", exitUsage,
			"refused (settings_collision): workspaces passes --settings for attach paths and deny_access, so the harness arguments cannot carry their own")
	case err != nil:
		p.refuse("", "", exitInternal, "building the harness command: %v", err)
	}
	return p
}

// effectiveDenyPaths lists the global deny_access paths, then the space's,
// each once.
func effectiveDenyPaths(cfg *config.Spaces, sp *config.Space) []string {
	entries := cfg.DenyAccess
	if sp != nil {
		entries = append(slices.Clone(entries), sp.DenyAccess...)
	}
	var out []string
	for _, e := range entries {
		if !slices.Contains(out, e.Logical) {
			out = append(out, e.Logical)
		}
	}
	return out
}

func (a *App) loadConfig(p *launchPlan) bool {
	spacesPath, harnessPath, err := a.configPaths()
	if err == nil {
		p.cfg, p.cfgHash, err = config.LoadSpaces(spacesPath, a.home())
	}
	if err == nil {
		p.harnesses, err = config.LoadHarnesses(harnessPath, a.home(), harness.Known)
	}
	if err != nil {
		p.refuse(reasonConfigError, "", exitConfig, "refused (config_error): %v", err)
		return false
	}
	return true
}

// chooseSpace and chooseHarness fill a missing argument: from a picker for
// run, from defaults for check, which answers "what would a launch from here
// get".
func (a *App) chooseSpace(p *launchPlan, interactive bool) bool {
	if p.spaceArg != "" {
		return true
	}
	if !interactive || len(p.cfg.Spaces) == 0 {
		p.spaceArg = "."
		return true
	}
	items := []string{". (here: " + p.cwd.Logical + ")"}
	for _, n := range p.cfg.SpaceNames() {
		items = append(items, n+"  "+p.cfg.Spaces[n].Root.Written)
	}
	choice, code := a.pick("space", items)
	if code != exitOK {
		p.refuse("", "", code, "")
		return false
	}
	p.spaceArg = choice
	return true
}

func (a *App) chooseHarness(p *launchPlan, interactive bool) bool {
	if p.harnessName != "" {
		return true
	}
	names := p.harnesses.Names()
	switch {
	case !interactive:
		p.harnessName = p.harnesses.Default
	case !p.tty:
		p.refuse("", "", exitUsage, "missing harness; no TTY")
		return false
	case len(names) == 1:
		p.harnessName = names[0]
	default:
		choice, code := a.pick("harness", names)
		if code != exitOK {
			p.refuse("", "", code, "")
			return false
		}
		p.harnessName = choice
	}
	return true
}

func refusalText(p *launchPlan, checked policy.Cwd) string {
	return decisionText(p.decision, checked, p.spaceName())
}

func decisionText(d policy.Decision, checked policy.Cwd, space string) string {
	where := checked.Physical
	if checked.Logical != checked.Physical {
		where = checked.Logical + " (" + checked.Physical + ")"
	}
	switch d.Reason {
	case policy.ReasonNotAllowed:
		return fmt.Sprintf("refused (not_allowed): no allow_launch entry covers %s", checked.Physical)
	case policy.ReasonAttachDenied:
		return fmt.Sprintf("refused (attach_denied): an attach path of space %s matches deny_access entry %s", space, d.Rule)
	default:
		return fmt.Sprintf("refused (%s): %s matches %s entry %s", d.Reason, where, d.Reason, d.Rule)
	}
}

// planTmux decides where the harness runs. Background mode is refused before
// any tmux call; outside tmux only a session with a terminal is bootstrapped.
func (a *App) planTmux(p *launchPlan) {
	switch {
	case p.mode == harness.ModeBackground:
		p.refuse(reasonBackgroundRefused, "", exitRefused,
			"refused (background_refused): a background session would run outside any tmux pane")
	case a.Getenv("TMUX") != "":
		a.queryTmux(p)
		if p.tmux.err != nil && p.mode != harness.ModeUtility {
			p.refuse(reasonTmuxError, "", exitUnavailable,
				"refused (tmux_error): $TMUX is set but its server does not answer: %v", p.tmux.err)
		}
	case p.mode == harness.ModeUtility:
	case p.mode == harness.ModePrint || !p.tty:
		p.refuse(reasonNotInTmux, "", exitRefused,
			"refused (not_in_tmux): %s, and agent sessions run only inside tmux.\nStart tmux, then run the same command again.", notInTmuxWhy(p))
	default:
		a.planBootstrap(p)
	}
}

func notInTmuxWhy(p *launchPlan) string {
	if p.mode == harness.ModePrint {
		return "print mode cannot be moved into a new tmux window"
	}
	return "there is no terminal to attach tmux to"
}

func (a *App) planBootstrap(p *launchPlan) {
	tmuxPath, err := a.LookPath("tmux")
	if err != nil {
		p.refuse(reasonTmuxError, "", exitUnavailable, "refused (tmux_error): tmux not found on PATH: %v", err)
		return
	}
	self, err := a.Executable()
	if err != nil {
		p.refuse("", "", exitInternal, "cannot find the workspaces executable: %v", err)
		return
	}
	dir := p.cwd.Logical
	if p.spaceArg != "." {
		dir = p.launchDir
	}
	env := tmux.BootstrapEnv(a.Environ(), p.cwd.Logical)
	command := append([]string{self, "run", p.spaceArg, p.harnessName}, p.userArgs...)
	b := &bootstrapPlan{
		tmuxPath: tmuxPath,
		session:  tmux.SessionName(p.spaceName()),
		window:   tmux.WindowName(p.spaceName(), p.harnessName, p.launchDir),
	}
	b.newSession = tmux.NewSession(b.session, b.window, dir, env, command)
	b.newWindow = tmux.NewWindow(b.session, b.window, dir, env, command)
	for _, args := range [][]string{b.newSession, b.newWindow} {
		if err := tmux.CheckArgvSize(args, env); err != nil {
			p.refuse(reasonTmuxError, "", exitUnavailable, "refused (tmux_error): %v", err)
			return
		}
	}
	_, err = a.Tmux(tmux.HasSession(b.session))
	b.exists = err == nil
	p.bootstrap = b
}

func (a *App) queryTmux(p *launchPlan) {
	if p.tmux.queried || a.Getenv("TMUX") == "" {
		return
	}
	p.tmux.queried = true
	if pane := a.Getenv("TMUX_PANE"); pane != "" {
		out, err := a.Tmux(tmux.PaneQuery(pane))
		if err == nil {
			var pn tmux.Pane
			if pn, err = tmux.ParsePane(out); err == nil {
				p.tmux.pane = &pn
			}
		}
		p.tmux.err = err
		return
	}
	out, err := a.Tmux(tmux.SocketQuery())
	p.tmux.socket = strings.TrimSpace(out)
	p.tmux.err = err
}

// A bootstrapped pane runs workspaces itself, so its pane_pid is our pid.
func (a *App) inBootstrapPane(p *launchPlan) bool {
	return p.tmux.pane != nil && p.tmux.pane.PanePID == a.Pid
}

func (a *App) tmuxRecord(p *launchPlan) *launchlog.Tmux {
	switch {
	case !p.tmux.queried || p.tmux.err != nil:
		return nil
	case p.tmux.pane == nil:
		return &launchlog.Tmux{Socket: p.tmux.socket, Entry: "pane"}
	}
	pn := p.tmux.pane
	idx := pn.WindowIndex
	entry := "pane"
	if a.inBootstrapPane(p) {
		entry = "bootstrap"
	}
	return &launchlog.Tmux{
		Socket: pn.Socket, SessionID: pn.SessionID, Session: pn.Session, WindowID: pn.WindowID,
		WindowIndex: &idx, Window: pn.Window, PaneID: pn.PaneID, Entry: entry,
	}
}

func (a *App) record(p *launchPlan, decision, reason, rule string) launchlog.Record {
	r := launchlog.Record{
		TS:           launchlog.FormatTime(a.Now()),
		ID:           p.launchID,
		Agent:        a.Version,
		Decision:     decision,
		Reason:       reason,
		Rule:         rule,
		Cwd:          p.cwd.Physical,
		CwdLogical:   p.cwd.Logical,
		LaunchDir:    p.launchDir,
		Harness:      p.harnessName,
		Adapter:      p.harness.Adapter,
		Mode:         string(p.mode),
		Tmux:         a.tmuxRecord(p),
		Bin:          p.bin,
		BinTarget:    p.binTarget,
		Args:         p.userArgs,
		AddDirs:      p.addDirs,
		ConfigSHA256: p.cfgHash,
		TTY:          p.tty,
		PID:          a.Pid,
		PPID:         a.Ppid,

		UnenforcedDeny: p.unenforced,
	}
	if p.space != nil {
		name := p.space.Name
		r.Space = &name
	}
	return r
}

func (a *App) appendRecord(r launchlog.Record) error {
	path, err := a.logPath()
	if err != nil {
		return err
	}
	return launchlog.Append(path, r)
}

func (a *App) refuseRun(p *launchPlan) int {
	r := p.refusal
	if r.reason != "" {
		// Background mode is refused before any tmux call; see runCmd.
		if r.reason != reasonBackgroundRefused {
			a.queryTmux(p)
		}
		if err := a.appendRecord(a.record(p, "refuse", r.reason, r.rule)); err != nil {
			fmt.Fprintf(a.Stderr, "workspaces: warning: could not log this refusal: %v\n", err)
		}
	}
	if r.msg != "" {
		fmt.Fprintf(a.Stderr, "workspaces: %s\n", r.msg)
	}
	return r.code
}

func (a *App) launchInPlace(p *launchPlan) int {
	if err := a.appendRecord(a.record(p, "allow", p.decision.Reason, p.decision.Rule)); err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: cannot write the launch log, nothing launched: %v\n", err)
		return exitIOErr
	}
	if w := p.unenforcedWarning(); w != "" {
		fmt.Fprintf(a.Stderr, "workspaces: warning: %s\n", w)
	}
	if p.spaceArg != "." {
		if err := a.Chdir(p.launchDir); err != nil {
			fmt.Fprintf(a.Stderr, "workspaces: %v\n", err)
			return exitInternal
		}
	}
	if err := a.Exec(p.bin, p.argv, p.env); err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: exec %s: %v\n", p.bin, err)
		return exitInternal
	}
	return exitOK
}

// bootstrap starts "workspaces run" again as the process of a new tmux window, then
// attaches to it. The inner workspaces re-checks and writes the allow record, so the
// outer writes one only when tmux fails.
func (a *App) bootstrap(p *launchPlan) int {
	b := p.bootstrap
	var err error
	if b.exists {
		_, err = a.Tmux(b.newWindow)
	} else {
		_, err = a.Tmux(b.newSession)
		if tmux.IsDuplicateSession(err) {
			_, err = a.Tmux(b.newWindow)
		}
	}
	if err != nil {
		p.refuse(reasonTmuxError, "", exitUnavailable, "refused (tmux_error): %v", err)
		return a.refuseRun(p)
	}
	if err := a.Exec(b.tmuxPath, tmux.AttachArgv(b.session), a.Environ()); err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: exec tmux attach-session: %v\n", err)
		return exitInternal
	}
	return exitOK
}

type checkReport struct {
	Decision   string            `json:"decision"`
	Reason     string            `json:"reason"`
	Rule       string            `json:"rule"`
	Message    string            `json:"message,omitempty"`
	CwdLogical string            `json:"cwd_logical"`
	Cwd        string            `json:"cwd"`
	Space      *string           `json:"space"`
	LaunchDir  string            `json:"launch_dir"`
	Harness    string            `json:"harness"`
	Adapter    string            `json:"adapter"`
	Bin        string            `json:"bin"`
	BinTarget  string            `json:"bin_target"`
	Mode       string            `json:"mode"`
	Tmux       string            `json:"tmux"`
	Argv       []string          `json:"argv"`
	DenyCount  int               `json:"deny_access_paths"`
	Unenforced int               `json:"unenforced_deny,omitempty"`
	Env        map[string]string `json:"env"`
	Warnings   []string          `json:"warnings"`
}

func (a *App) checkCmd(args []string) int {
	fs := a.flagSet("check", usageCheck)
	jsonOut := fs.Bool("json", false, "print the result as one JSON object")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	p := a.plan(fs.Args(), false)
	if p.refusal == nil {
		a.planTmux(p)
	}
	rep := a.report(p)
	if *jsonOut {
		enc := json.NewEncoder(a.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return exitInternal
		}
	} else {
		a.printReport(rep)
	}
	if p.refusal != nil {
		return p.refusal.code
	}
	return exitOK
}

func (a *App) report(p *launchPlan) checkReport {
	rep := checkReport{
		Decision:   "allow",
		Reason:     p.decision.Reason,
		Rule:       p.decision.Rule,
		CwdLogical: p.cwd.Logical,
		Cwd:        p.cwd.Physical,
		LaunchDir:  p.launchDir,
		Harness:    p.harnessName,
		Adapter:    p.harness.Adapter,
		Bin:        p.bin,
		BinTarget:  p.binTarget,
		Mode:       string(p.mode),
		Tmux:       a.tmuxPlanText(p),
		Argv:       p.argv,
		DenyCount:  len(p.denyPaths),
		Unenforced: p.unenforced,
	}
	if p.space != nil {
		name := p.space.Name
		rep.Space = &name
	}
	if r := p.refusal; r != nil {
		rep.Decision = "refuse"
		rep.Reason = r.reason
		rep.Rule = r.rule
		rep.Message = r.msg
	}
	if p.envAdded != nil {
		rep.Env = map[string]string{"PWD": p.launchDir}
		for k, v := range p.envAdded {
			rep.Env[k] = v
		}
	}
	if p.cfg != nil {
		rep.Warnings = p.cfg.Missing()
		if w := p.unenforcedWarning(); w != "" {
			rep.Warnings = append(rep.Warnings, w)
		}
		for _, name := range p.cfg.SpaceNames() {
			sp := p.cfg.Spaces[name]
			root := policy.Cwd{Logical: sp.Root.Path, Physical: sp.Root.Path}
			if d := policy.Decide(root, p.cfg, sp); !d.Allow {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("space %s fails the check: %s", name, reasonRule(d)))
			}
			for _, e := range sp.Attach {
				if rule, denied := attachDenied(p.cfg, sp, e); denied {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("space %s attach %s fails the check: %s %s", name, e.Written, policy.ReasonAttachDenied, rule))
				}
			}
		}
	}
	return rep
}

// attachDenied mirrors the only rule run applies to attach paths (refusing
// with attach_denied): deny_access, global or the space's own. allow_launch
// and deny_launch govern where a session starts, not what it may attach.
func attachDenied(cfg *config.Spaces, sp *config.Space, e config.Entry) (string, bool) {
	m, ok := policy.DenyAccessMatch(cfg, sp, e.Path)
	return m.Written, ok
}

func reasonRule(d policy.Decision) string {
	if d.Rule == "" {
		return d.Reason
	}
	return d.Reason + " " + d.Rule
}

func (a *App) tmuxPlanText(p *launchPlan) string {
	switch {
	case p.bootstrap != nil && p.bootstrap.exists:
		return fmt.Sprintf("bootstrap: new window %s in session %s (session exists)", p.bootstrap.window, p.bootstrap.session)
	case p.bootstrap != nil:
		return fmt.Sprintf("bootstrap: new session %s with window %s", p.bootstrap.session, p.bootstrap.window)
	case p.refusal != nil:
		return ""
	case p.tmux.pane != nil:
		return fmt.Sprintf("in place, pane %s in session %s", p.tmux.pane.PaneID, p.tmux.pane.Session)
	case p.tmux.queried && p.tmux.err == nil:
		return "in place, socket " + p.tmux.socket
	case p.tmux.queried:
		return "in place; tmux does not answer"
	default:
		return "in place, outside tmux"
	}
}

func (a *App) printReport(rep checkReport) {
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) {
		if v == "" {
			v = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\n", k, v)
	}
	row("decision", rep.Decision)
	row("reason", rep.Reason)
	row("rule", rep.Rule)
	if rep.Message != "" {
		row("message", strings.ReplaceAll(rep.Message, "\n", " "))
	}
	row("cwd", rep.CwdLogical)
	row("cwd physical", rep.Cwd)
	space := ""
	if rep.Space != nil {
		space = *rep.Space
	}
	row("space", space)
	row("launch dir", rep.LaunchDir)
	harnessText := rep.Harness
	if rep.Adapter != "" {
		harnessText += " (adapter " + rep.Adapter + ")"
	}
	row("harness", harnessText)
	binText := rep.Bin
	if rep.BinTarget != "" && rep.BinTarget != rep.Bin {
		binText += " -> " + rep.BinTarget
	}
	row("binary", binText)
	row("mode", rep.Mode)
	row("tmux", rep.Tmux)
	row("argv", shellJoin(rep.Argv))
	denyText := fmt.Sprintf("%d paths in session", rep.DenyCount-rep.Unenforced)
	if rep.Unenforced > 0 {
		denyText += fmt.Sprintf(" (%d not enforced)", rep.Unenforced)
	}
	row("deny_access", denyText)
	keys := make([]string, 0, len(rep.Env))
	for k := range rep.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		row("env", k+"="+shellQuote(rep.Env[k]))
	}
	for _, w := range rep.Warnings {
		row("warning", w)
	}
	tw.Flush()
}
