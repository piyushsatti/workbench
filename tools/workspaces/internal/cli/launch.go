package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"workspaces/internal/launchlog"
)

const usageLaunch = `Usage: workspaces launch list [-n N] [--json]

Shows the last N launch records, newest last (default 20). --json prints the
raw log lines.
`

const argsWidth = 40

func (a *App) launchCmd(args []string) int {
	return a.dispatch("launch", usageLaunch, []verb{{"list", a.launchList}}, args)
}

type verb struct {
	name string
	run  func(args []string) int
}

// dispatch runs a noun's verb. A missing verb is picked on a terminal,
// automatically when the noun has only one.
func (a *App) dispatch(noun, usage string, verbs []verb, args []string) int {
	if len(args) == 0 {
		if !a.IsTTY() {
			fmt.Fprint(a.Stderr, usage)
			return exitUsage
		}
		choice := verbs[0].name
		if len(verbs) > 1 {
			names := make([]string, len(verbs))
			for i, v := range verbs {
				names[i] = v.name
			}
			var code int
			if choice, code = a.pick(noun, names); code != exitOK {
				return code
			}
		}
		args = []string{choice}
	}
	switch args[0] {
	case "-h", "-help", "--help":
		fmt.Fprint(a.Stderr, usage)
		return exitOK
	}
	for _, v := range verbs {
		if v.name == args[0] {
			return v.run(args[1:])
		}
	}
	fmt.Fprintf(a.Stderr, "workspaces: unknown %s verb %q\n\n%s", noun, args[0], usage)
	return exitUsage
}

func (a *App) launchList(args []string) int {
	fs := a.flagSet("launch list", usageLaunch)
	n := fs.Int("n", 20, "number of records")
	jsonOut := fs.Bool("json", false, "print the raw log lines")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	if fs.NArg() > 0 || *n < 0 {
		fmt.Fprint(a.Stderr, usageLaunch)
		return exitUsage
	}
	path, err := a.logPath()
	if err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: %v\n", err)
		return exitConfig
	}
	lines, malformed, err := launchlog.Read(path)
	if err != nil {
		fmt.Fprintf(a.Stderr, "workspaces: reading %s: %v\n", path, err)
		return exitIOErr
	}
	if malformed > 0 {
		fmt.Fprintf(a.Stderr, "workspaces: skipped %d malformed line(s) in %s\n", malformed, path)
	}
	if len(lines) > *n {
		lines = lines[len(lines)-*n:]
	}

	if *jsonOut {
		for _, l := range lines {
			fmt.Fprintln(a.Stdout, l.Raw)
		}
		return exitOK
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	for _, l := range lines {
		fmt.Fprintln(tw, a.launchRow(l.Record))
	}
	tw.Flush()
	return exitOK
}

// launchRow is one launch record as tab-separated cells.
func (a *App) launchRow(r launchlog.Record) string {
	space := "-"
	if r.Space != nil {
		space = *r.Space
	}
	cells := []string{shortTime(r.TS), r.Decision, r.Reason, space, r.Harness, a.tildePath(r.LaunchDir), cut(clean(shellJoin(r.Args)), argsWidth)}
	for i, c := range cells {
		cells[i] = clean(c)
	}
	return strings.Join(cells, "\t")
}

func shortTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04:05")
}

func cut(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-3]) + "..."
}
