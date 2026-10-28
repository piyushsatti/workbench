package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"

	"workspaces/internal/config"
	"workspaces/internal/launchlog"
	"workspaces/internal/policy"
	"workspaces/internal/records"
)

const spaceArgHelp = `Without a space, the space holding the current folder is used; failing that,
a picker opens on a terminal, and without one the command exits 64. "." means
the space holding the current folder.
`

const usageSpace = `Usage: workspaces space list
       workspaces space show [space]
       workspaces space add <name> [root]
       workspaces space rm [--yes] [space]
       workspaces space attach [space] [path]
       workspaces space detach [--yes] [space] [path]

list prints one line per space: root, number of attach paths, todo state
("N items", "TODO.md only" when only an unread TODO.md exists, "none"), log state and
last launch. show prints one space in full: its paths, the check result for
its root and each attach path, and its last five launches.
show without a space uses the space holding the current folder, then a picker
on a terminal, and exits 64 without one; "." means that space.

add takes the root from the current folder when none is given and refuses a
root that "workspaces run" would refuse (exit 77). attach takes the path from the
current folder and refuses one under deny_access, global or the space's (77).
Without a space, rm, attach and detach open a picker on a terminal and exit 64
without one; there "." means the space holding the current folder. With one
argument, attach and detach read it as a path when it starts with ".", "~" or
"/" or contains "/", and as a space name otherwise. detach without a path
picks from the attach list. rm and detach ask for confirmation on a terminal;
--yes skips it, and without a terminal and without --yes they exit 64.
` + writeRulesHelp

const writeRulesHelp = `
Writes change only the entry named, keep the rest of the file as written
(paths stay in the form written, keys in their order), and indent the file by
two spaces. The result is validated before anything is written (exit 78 when
it is invalid, file untouched). The old file is kept as <file>.prev, and the
new one replaces it atomically. Writes are not recorded in the launch log.
`

const usageTodo = `Usage: workspaces todo list [--json] [space]

Lists the space's todo file (default <root>/TODO.jsonl) grouped by domain,
major_category or category, whichever a record has first: id, status, then
[tag] and title. A record without an id shows its line number as #N; one
without a title shows its text or id. --json prints the raw lines. A bad line
is reported with its line number and skipped.
` + spaceArgHelp

const usageLog = `Usage: workspaces log list [-n N] [--json] [space]

Shows the last N records of the space's log file (default <root>/LOGS.jsonl),
newest last (default 20): date (or the date of ts), kind, group (domain,
major_category or category), status, then [tag] and title. --json prints the
raw lines. A bad line is reported with its line number and skipped.
` + spaceArgHelp

const (
	showLaunches = 5
	// Bad lines reported one by one per file before a summary line.
	maxBadLines = 5
	// Widths in runes.
	titleWidth = 72
	fieldWidth = 32
)

func (a *App) spaceCmd(args []string) int {
	return a.dispatch("space", usageSpace, []verb{
		{"list", a.spaceList}, {"show", a.spaceShow}, {"add", a.spaceAdd},
		{"rm", a.spaceRm}, {"attach", a.spaceAttach}, {"detach", a.spaceDetach},
	}, args)
}

func (a *App) todoCmd(args []string) int {
	return a.dispatch("todo", usageTodo, []verb{{"list", a.todoList}}, args)
}

func (a *App) logCmd(args []string) int {
	return a.dispatch("log", usageLog, []verb{{"list", a.logList}}, args)
}

func (a *App) loadSpacesOnly() (*config.Spaces, int) {
	path, _, err := a.configPaths()
	var cfg *config.Spaces
	if err == nil {
		cfg, _, err = config.LoadSpaces(path, a.home())
	}
	if err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: %v\n", err)
		return nil, exitConfig
	}
	return cfg, exitOK
}

// viewSpace resolves the space argument of a view: a name, or when it is
// omitted the space holding the cwd, then a picker.
func (a *App) viewSpace(cfg *config.Spaces, arg string) (*config.Space, int) {
	if arg == "" || arg == "." {
		wd, err := a.Getwd()
		var cwd policy.Cwd
		if err == nil {
			cwd, err = policy.ResolveCwd(wd)
		}
		if err != nil {
			fmt.Fprintf(a.Stderr, "workspaces: cannot resolve the current directory: %v\n", err)
			return nil, exitInternal
		}
		if sp := policy.SelectSpace(cfg, cwd.Physical); sp != nil {
			return sp, exitOK
		}
		switch {
		case arg == ".":
			fmt.Fprintf(a.Stderr, "workspaces: no space holds %s\n", cwd.Logical)
			return nil, exitUsage
		case !a.IsTTY():
			fmt.Fprintf(a.Stderr, "workspaces: missing space; no space holds %s and there is no TTY\n", cwd.Logical)
			return nil, exitUsage
		case len(cfg.Spaces) == 0:
			fmt.Fprintln(a.Stderr, "workspaces: no spaces configured")
			return nil, exitUsage
		}
		choice, code := a.pick("space", spaceItems(cfg))
		if code != exitOK {
			return nil, code
		}
		arg = choice
	}
	sp, ok := cfg.Space(arg)
	if !ok {
		fmt.Fprintf(a.Stderr, "workspaces: unknown space %q; configured: %s\n", arg, strings.Join(cfg.SpaceNames(), ", "))
		return nil, exitUsage
	}
	return sp, exitOK
}

func spaceItems(cfg *config.Spaces) []string {
	var items []string
	for _, n := range cfg.SpaceNames() {
		items = append(items, n+"  "+cfg.Spaces[n].Root.Written)
	}
	return items
}

// readRecords reports bad lines on stderr, never quoting their bytes, and
// returns the rest. A read error names the path.
func (a *App) readRecords(path string) ([]records.Record, error) {
	recs, bad, err := records.Read(path)
	for i, b := range bad {
		if i == maxBadLines {
			fmt.Fprintf(a.Stderr, "workspaces: %s: %d more bad lines skipped\n", a.tildePath(path), len(bad)-maxBadLines)
			break
		}
		fmt.Fprintf(a.Stderr, "workspaces: %s:%d: %v; line skipped\n", a.tildePath(path), b.Line, b.Err)
	}
	if err != nil {
		err = fmt.Errorf("reading %s: %w", a.tildePath(path), err)
	}
	return recs, err
}

// launches returns the launch log, reporting malformed lines as launch list
// does. A missing log is empty.
func (a *App) launches() ([]launchlog.Line, int) {
	path, err := a.logPath()
	if err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: %v\n", err)
		return nil, exitConfig
	}
	lines, malformed, err := launchlog.Read(path)
	if err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: reading %s: %v\n", path, err)
		return nil, exitIOErr
	}
	if malformed > 0 {
		fmt.Fprintf(a.Stderr, "workspaces: skipped %d malformed line(s) in %s\n", malformed, path)
	}
	return lines, exitOK
}

func launchesOf(lines []launchlog.Line, space string) []launchlog.Record {
	var out []launchlog.Record
	for _, l := range lines {
		if l.Record.Space != nil && *l.Record.Space == space {
			out = append(out, l.Record)
		}
	}
	return out
}

type fileState struct {
	text string
	err  error
}

func (a *App) todoState(sp *config.Space) fileState {
	recs, err := a.readRecords(sp.Todo)
	switch {
	case errors.Is(err, os.ErrNotExist) && onlyTodoMD(sp):
		return fileState{text: "TODO.md only"}
	case errors.Is(err, os.ErrNotExist):
		return fileState{text: "none"}
	case err != nil:
		return fileState{text: "unreadable", err: err}
	}
	return fileState{text: plural(len(recs), "item")}
}

func (a *App) logState(sp *config.Space) fileState {
	recs, err := a.readRecords(sp.Log)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fileState{text: "none"}
	case err != nil:
		return fileState{text: "unreadable", err: err}
	}
	return fileState{text: plural(len(recs), "record")}
}

// onlyTodoMD reports a space whose todo list still lives in a TODO.md beside
// where the todo file would be; TODO.md itself is never parsed.
func onlyTodoMD(sp *config.Space) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(sp.Todo), "TODO.md"))
	return err == nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func (a *App) spaceList(args []string) int {
	fs := a.flagSet("space list", usageSpace)
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() > 0 {
		fmt.Fprint(a.Stderr, usageSpace)
		return exitUsage
	}
	cfg, code := a.loadSpacesOnly()
	if code != exitOK {
		return code
	}
	lines, code := a.launches()
	if code != exitOK {
		return code
	}
	code = exitOK
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SPACE\tROOT\tATTACH\tTODO\tLOG\tLAST LAUNCH")
	for _, name := range cfg.SpaceNames() {
		sp := cfg.Spaces[name]
		last := "never"
		if recs := launchesOf(lines, name); len(recs) > 0 {
			last = shortTime(recs[len(recs)-1].TS)
		}
		todo, log := a.todoState(sp), a.logState(sp)
		for _, s := range []fileState{todo, log} {
			if s.err != nil {
				fmt.Fprintf(a.Stderr, "workspaces: space %s: %v\n", name, s.err)
				code = exitIOErr
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", name, clean(sp.Root.Written), len(sp.Attach), todo.text, log.text, clean(last))
	}
	tw.Flush()
	return code
}

func (a *App) spaceShow(args []string) int {
	fs := a.flagSet("space show", usageSpace)
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() > 1 {
		fmt.Fprint(a.Stderr, usageSpace)
		return exitUsage
	}
	cfg, code := a.loadSpacesOnly()
	if code != exitOK {
		return code
	}
	sp, code := a.viewSpace(cfg, fs.Arg(0))
	if code != exitOK {
		return code
	}
	lines, code := a.launches()
	if code != exitOK {
		return code
	}

	code = exitOK
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) { fmt.Fprintf(tw, "%s\t%s\n", k, clean(v)) }
	list := func(k string, es []config.Entry) {
		if len(es) == 0 {
			row(k, "-")
		}
		for _, e := range es {
			row(k, e.Written)
		}
	}
	row("space", sp.Name)
	row("root", sp.Root.Written)
	list("attach", sp.Attach)
	list("deny_access (space)", sp.DenyAccess)
	list("deny_access (global)", cfg.DenyAccess)
	for _, f := range []struct {
		key, path string
		state     fileState
	}{{"todo", sp.Todo, a.todoState(sp)}, {"log", sp.Log, a.logState(sp)}} {
		row(f.key, a.tildePath(f.path)+" ("+f.state.text+")")
		if f.state.err != nil {
			fmt.Fprintf(a.Stderr, "workspaces: %v\n", f.state.err)
			code = exitIOErr
		}
	}

	d := rootDecision(cfg, sp)
	decision := "refuse"
	if d.Allow {
		decision = "allow"
	}
	row("check root", decision+" "+reasonRule(d))
	for _, e := range sp.Attach {
		result := "ok"
		if rule, denied := attachDenied(cfg, sp, e); denied {
			result = policy.ReasonAttachDenied + " " + rule
		}
		row("check attach", e.Written+": "+result)
	}

	recs := launchesOf(lines, sp.Name)
	if len(recs) > showLaunches {
		recs = recs[len(recs)-showLaunches:]
	}
	if len(recs) == 0 {
		row("launch", "none")
	}
	for _, r := range recs {
		fmt.Fprintf(tw, "launch\t%s\n", a.launchRow(r))
	}
	tw.Flush()
	return code
}

func (a *App) todoList(args []string) int {
	fs := a.flagSet("todo list", usageTodo)
	jsonOut := fs.Bool("json", false, "print the raw lines")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() > 1 {
		fmt.Fprint(a.Stderr, usageTodo)
		return exitUsage
	}
	recs, code := a.spaceRecords(fs.Arg(0), func(sp *config.Space) string { return sp.Todo })
	if code != exitOK {
		return code
	}
	if *jsonOut {
		for _, r := range recs {
			fmt.Fprintln(a.Stdout, r.Raw)
		}
		return exitOK
	}

	var order []string
	groups := map[string][][3]string{}
	var idW, statusW int
	for _, r := range recs {
		g := groupOf(r)
		if _, seen := groups[g]; !seen {
			order = append(order, g)
		}
		id := r.Text("id")
		if id == "" {
			id = "#" + strconv.Itoa(r.Line)
		}
		row := [3]string{field(id), field(r.Text("status")), taggedTitle(r)}
		idW = max(idW, utf8.RuneCountInString(row[0]))
		statusW = max(statusW, utf8.RuneCountInString(row[1]))
		groups[g] = append(groups[g], row)
	}
	// Padded by hand rather than with tabwriter, which restarts its columns
	// at every header line, so the columns line up across groups.
	for _, g := range order {
		fmt.Fprintln(a.Stdout, field(g))
		for _, row := range groups[g] {
			fmt.Fprintf(a.Stdout, "  %s  %s  %s\n", padRight(row[0], idW), padRight(row[1], statusW), row[2])
		}
	}
	return exitOK
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

func (a *App) logList(args []string) int {
	fs := a.flagSet("log list", usageLog)
	n := fs.Int("n", 20, "number of records")
	jsonOut := fs.Bool("json", false, "print the raw lines")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() > 1 || *n < 0 {
		fmt.Fprint(a.Stderr, usageLog)
		return exitUsage
	}
	recs, code := a.spaceRecords(fs.Arg(0), func(sp *config.Space) string { return sp.Log })
	if code != exitOK {
		return code
	}
	if len(recs) > *n {
		recs = recs[len(recs)-*n:]
	}
	if *jsonOut {
		for _, r := range recs {
			fmt.Fprintln(a.Stdout, r.Raw)
		}
		return exitOK
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	for _, r := range recs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			field(logDate(r)), field(r.Text("kind")), field(groupOf(r)), field(r.Text("status")), taggedTitle(r))
	}
	tw.Flush()
	return exitOK
}

// spaceRecords loads the config, resolves the space and reads the file that
// file names. A missing file is reported on stderr and is not an error.
func (a *App) spaceRecords(arg string, file func(*config.Space) string) ([]records.Record, int) {
	cfg, code := a.loadSpacesOnly()
	if code != exitOK {
		return nil, code
	}
	sp, code := a.viewSpace(cfg, arg)
	if code != exitOK {
		return nil, code
	}
	path := file(sp)
	recs, err := a.readRecords(path)
	switch {
	case errors.Is(err, os.ErrNotExist) && path == sp.Todo && onlyTodoMD(sp):
		fmt.Fprintf(a.Stderr, "workspaces: space %s has only TODO.md beside %s; TODO.md is not read, use the JSONL todo file\n", sp.Name, a.tildePath(path))
		return nil, exitOK
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(a.Stderr, "workspaces: space %s has no file at %s\n", sp.Name, a.tildePath(path))
		return nil, exitOK
	case err != nil:
		fmt.Fprintf(a.Stderr, "workspaces: %v\n", err)
		return nil, exitIOErr
	}
	return recs, exitOK
}

func groupOf(r records.Record) string {
	return r.First("domain", "major_category", "category")
}

// taggedTitle is "[tag] title", cut to titleWidth. A record without a title
// shows its text, then its id.
func taggedTitle(r records.Record) string {
	title := orDash(r.First("title", "text", "id"))
	if tag := r.Text("tag"); tag != "" {
		title = "[" + tag + "] " + title
	}
	return cut(clean(title), titleWidth)
}

// logDate shows a record's date, or the date part of an RFC 3339 ts, in the
// offset the record was written with.
func logDate(r records.Record) string {
	if d := r.Text("date"); d != "" {
		return d
	}
	ts := r.Text("ts")
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format("2006-01-02")
	}
	return ts
}

// field is a record value as one table cell: cleaned, cut, "-" when blank.
func field(s string) string {
	return orDash(cut(clean(s), fieldWidth))
}

// clean folds every whitespace run to one space, so a field cannot break a
// row, and replaces control and format characters with U+FFFD. Records and
// the launch log are written by agent sessions, so an escape sequence or a
// bidi override planted in one must never reach the terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return unicode.ReplacementChar
		}
		return r
	}, strings.Join(strings.Fields(s), " "))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
