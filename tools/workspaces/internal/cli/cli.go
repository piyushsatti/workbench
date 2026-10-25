package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"workspaces/internal/config"
	"workspaces/internal/picker"
	"workspaces/internal/tmux"
)

// Exit codes are sysexits values so they never collide with the harness's own.
const (
	exitOK          = 0
	exitUsage       = 64
	exitUnavailable = 69
	exitInternal    = 70
	exitIOErr       = 74
	exitRefused     = 77
	exitConfig      = 78
	exitNotFound    = 127
	exitCancelled   = 130
)

// App holds every effect the commands reach for, so tests can swap exec, the
// picker and tmux for fakes.
type App struct {
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Getenv     func(string) string
	Environ    func() []string
	Getwd      func() (string, error)
	Chdir      func(string) error
	LookPath   func(string) (string, error)
	Executable func() (string, error)
	IsTTY      func() bool
	Pick       func(prompt string, items []string) (string, error)
	PickMulti  func(prompt string, items []string) ([]string, error)
	Confirm    func(prompt string) (bool, error)
	Tmux       tmux.Runner
	// Exec replaces the process on success and so only ever returns an error.
	Exec      func(path string, argv, env []string) error
	WaitEnter func()
	Now       func() time.Time
	Pid       int
	Ppid      int
	Version   string
}

func Main(args []string) int {
	return New().Run(args)
}

func New() *App {
	a := &App{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Getenv:     os.Getenv,
		Environ:    os.Environ,
		Getwd:      os.Getwd,
		Chdir:      os.Chdir,
		LookPath:   exec.LookPath,
		Executable: os.Executable,
		IsTTY:      picker.IsTTY,
		Pick:       picker.Pick,
		PickMulti:  picker.PickMulti,
		Confirm:    picker.Confirm,
		Tmux:       tmux.Run,
		Exec:       syscall.Exec,
		Now:        time.Now,
		Pid:        os.Getpid(),
		Ppid:       os.Getppid(),
		Version:    buildVersion(),
	}
	a.WaitEnter = func() { _, _ = bufio.NewReader(a.Stdin).ReadString('\n') }
	return a
}

const usageTop = `Usage: workspaces [--version] <command> [arguments]

workspaces (also awm, agent workspace manager) starts an agent harness only from
allowed folders, always inside tmux, and logs every launch.

Commands:
  run [space] [harness] [harness-args...]    check, log and start the harness
  check [space] [harness] [harness-args...]  show what run would do; writes nothing
  launch list [-n N] [--json]                show the launch log
  space list                                 list the spaces with their todo and log state
  space show [space]                         show one space: paths, check results, launches
  space add <name> [root]                    add a space; the root must pass the launch check
  space rm [space] [--yes]                   remove a space
  space attach [space] [path]                attach a folder to a space
  space detach [space] [path] [--yes]        detach an attached folder
  harness list                               list the harnesses
  harness add <name> --adapter A [--bin P] [--env K=V]...
                                             add a harness
  harness rm [harness] [--yes]               remove a harness other than the default
  policy list                                list allow_launch, deny_launch and deny_access
  policy add <list> <path>                   add a path to a global list
  policy rm <list> [path] [--yes]            remove a path from a global list
  todo list [--json] [space]                 list a space's TODO.jsonl, grouped by domain
  log list [-n N] [--json] [space]           show a space's LOGS.jsonl
  session list [--harness H] [-n N] [--json] [space]
                                             list a space's Claude sessions, newest first
  session resume [--harness H] [space] [session]
                                             resume one, checked and logged like run
  session prune [--harness H] [--yes] [space]
                                             delete chosen sessions' transcripts
  doctor                                     check config, paths, binaries, wrappers and protection
  hook session-start                         for Claude Code's SessionStart hook; see the README

A space of "." means here. The space, harness and policy commands that write
keep the previous file as <file>.prev. With no command and a terminal, a picker opens.
"workspaces <command> -h" shows a command's help.
`

func (a *App) Run(args []string) int {
	fs := flag.NewFlagSet("workspaces", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.Usage = func() { fmt.Fprint(a.Stderr, usageTop) }
	showVersion := fs.Bool("version", false, "print the CalVer build version")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *showVersion {
		fmt.Fprintln(a.Stdout, a.Version)
		return exitOK
	}

	rest := fs.Args()
	if len(rest) == 0 {
		if !a.IsTTY() {
			fmt.Fprint(a.Stderr, usageTop)
			return exitUsage
		}
		noun, code := a.pick("workspaces", []string{"run", "check", "launch", "space", "harness", "policy", "todo", "log", "session", "doctor"})
		if code != exitOK {
			return code
		}
		rest = []string{noun}
	}

	switch rest[0] {
	case "run":
		return a.runCmd(rest[1:])
	case "check":
		return a.checkCmd(rest[1:])
	case "launch":
		return a.launchCmd(rest[1:])
	case "space":
		return a.spaceCmd(rest[1:])
	case "harness":
		return a.harnessCmd(rest[1:])
	case "policy":
		return a.policyCmd(rest[1:])
	case "todo":
		return a.todoCmd(rest[1:])
	case "log":
		return a.logCmd(rest[1:])
	case "session":
		return a.sessionCmd(rest[1:])
	case "doctor":
		return a.doctorCmd(rest[1:])
	case "hook":
		return a.hookCmd(rest[1:])
	default:
		fmt.Fprintf(a.Stderr, "workspaces: unknown command %q\n\n%s", rest[0], usageTop)
		return exitUsage
	}
}

// pick returns the first field of the chosen line, so items can carry a
// description after the value.
func (a *App) pick(prompt string, items []string) (string, int) {
	choice, code := a.pickLine(prompt, items)
	if code != exitOK {
		return "", code
	}
	fields := strings.Fields(choice)
	if len(fields) == 0 {
		return "", exitCancelled
	}
	return fields[0], exitOK
}

// pickLine returns the whole chosen line, for items such as paths that may
// hold spaces.
func (a *App) pickLine(prompt string, items []string) (string, int) {
	choice, err := a.Pick(prompt, items)
	switch {
	case errors.Is(err, picker.ErrCancelled):
		return "", exitCancelled
	case errors.Is(err, picker.ErrNoPicker):
		fmt.Fprintf(a.Stderr, "workspaces: %v; pass the %s as an argument\n", err, prompt)
		return "", exitUsage
	case err != nil:
		fmt.Fprintf(a.Stderr, "workspaces: picker: %v\n", err)
		return "", exitInternal
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return "", exitCancelled
	}
	return choice, exitOK
}

// pickMany returns every chosen line; cancelling or choosing nothing is exit
// 130.
func (a *App) pickMany(prompt string, items []string) ([]string, int) {
	choices, err := a.PickMulti(prompt, items)
	switch {
	case errors.Is(err, picker.ErrCancelled):
		return nil, exitCancelled
	case errors.Is(err, picker.ErrNoPicker):
		fmt.Fprintf(a.Stderr, "workspaces: %v; nothing to pick the %s with\n", err, prompt)
		return nil, exitUsage
	case err != nil:
		fmt.Fprintf(a.Stderr, "workspaces: picker: %v\n", err)
		return nil, exitInternal
	case len(choices) == 0:
		return nil, exitCancelled
	}
	return choices, exitOK
}

func (a *App) home() string {
	return a.Getenv("HOME")
}

func (a *App) configPaths() (spaces, harness string, err error) {
	dir, err := config.ConfigDir(a.Getenv)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "spaces.json"), filepath.Join(dir, "harness.json"), nil
}

func (a *App) logPath() (string, error) {
	dir, err := config.StateDir(a.Getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "launches.jsonl"), nil
}

func (a *App) tildePath(p string) string {
	home := a.home()
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}

// Go stamps the vcs.* settings only when the module sits in a checkout whose
// .git is a directory, so a build inside a linked git worktree reports "devel".
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return calver(info.Settings)
}

// calver renders the commit date in UTC as YYYY.0M.0D; every build from the
// same day shares one version.
func calver(settings []debug.BuildSetting) string {
	var rev, when string
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			when = s.Value
		}
	}
	t, err := time.Parse(time.RFC3339, when)
	if rev == "" || err != nil {
		return "devel"
	}
	return t.UTC().Format("2006.01.02")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-./=:@%+,", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, s := range args {
		q[i] = shellQuote(s)
	}
	return strings.Join(q, " ")
}
