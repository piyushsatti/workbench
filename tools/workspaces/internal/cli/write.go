package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"workspaces/internal/config"
	"workspaces/internal/harness"
	"workspaces/internal/jsonedit"
	"workspaces/internal/picker"
	"workspaces/internal/policy"
)

// Key order for keys a write adds; keys already in a file keep their place.
var (
	spacesKeyOrder  = []string{"allow_launch", "deny_launch", "deny_access", "spaces"}
	spaceKeyOrder   = []string{"root", "attach", "deny_access", "todo", "log"}
	harnessKeyOrder = []string{"default", "harnesses"}
	entryKeyOrder   = []string{"adapter", "bin", "env"}
)

// parseMixed parses flags that come before or after the positionals, so that
// "harness add <name> --adapter claude" and "space rm <space> --yes" work. A
// "--" ends the flags.
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos, tail []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, tail = args[:i], args[i+1:]
	}
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return append(pos, tail...), nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func (a *App) fail(code int, format string, args ...any) int {
	fmt.Fprintf(a.Stderr, "workspaces: "+format+"\n", args...)
	return code
}

// editSpaces applies change to spaces.json. A missing file is an empty one.
// change sees the loaded config and the raw object it edits, and returns the
// line to print.
func (a *App) editSpaces(change func(cur *config.Spaces, top *jsonedit.Object) (string, int)) int {
	path, _, err := a.configPaths()
	if err != nil {
		return a.fail(exitConfig, "%v", err)
	}
	raw, code := a.readForEdit(path, "{}")
	if code != exitOK {
		return code
	}
	cur, err := config.ParseSpaces(raw, a.home())
	if err != nil {
		return a.fail(exitConfig, "%s: %v; fix it by hand before changing it with workspaces", path, err)
	}
	top, err := jsonedit.Parse(raw)
	if err != nil {
		return a.fail(exitConfig, "%s: %v", path, err)
	}
	msg, code := change(cur, top)
	if code != exitOK {
		return code
	}
	return a.commit(path, top, msg, func(b []byte) error {
		_, err := config.ParseSpaces(b, a.home())
		return err
	})
}

// editHarnesses is editSpaces for harness.json, whose missing file stands for
// the built-in default.
func (a *App) editHarnesses(change func(cur *config.Harnesses, top *jsonedit.Object) (string, int)) int {
	_, path, err := a.configPaths()
	if err != nil {
		return a.fail(exitConfig, "%v", err)
	}
	raw, code := a.readForEdit(path, config.DefaultHarnessesJSON)
	if code != exitOK {
		return code
	}
	cur, err := config.ParseHarnesses(raw, a.home(), harness.Known)
	if err != nil {
		return a.fail(exitConfig, "%s: %v; fix it by hand before changing it with workspaces", path, err)
	}
	top, err := jsonedit.Parse(raw)
	if err != nil {
		return a.fail(exitConfig, "%s: %v", path, err)
	}
	msg, code := change(cur, top)
	if code != exitOK {
		return code
	}
	return a.commit(path, top, msg, func(b []byte) error {
		_, err := config.ParseHarnesses(b, a.home(), harness.Known)
		return err
	})
}

func (a *App) readForEdit(path, whenMissing string) ([]byte, int) {
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return []byte(whenMissing), exitOK
	case err != nil:
		return nil, a.fail(exitIOErr, "reading %s: %v", path, err)
	}
	return raw, exitOK
}

// commit validates the result with the loader before it writes anything.
func (a *App) commit(path string, top *jsonedit.Object, msg string, validate func([]byte) error) int {
	out := top.Format()
	if err := validate(out); err != nil {
		return a.fail(exitConfig, "not written, the result is invalid: %v", err)
	}
	if err := config.WriteFile(path, out); err != nil {
		return a.fail(exitIOErr, "writing %s: %v", path, err)
	}
	fmt.Fprintln(a.Stdout, msg)
	return exitOK
}

// confirm gates a removal: --yes, or a yes from gum on a terminal.
func (a *App) confirm(yes bool, question string) int {
	if yes {
		return exitOK
	}
	if !a.IsTTY() {
		return a.fail(exitUsage, "cannot ask %q: no TTY; pass --yes", question)
	}
	ok, err := a.Confirm(question)
	switch {
	case errors.Is(err, picker.ErrNoConfirm):
		return a.fail(exitUsage, "%v; pass --yes", err)
	case err != nil:
		return a.fail(exitInternal, "confirm: %v", err)
	case !ok:
		return exitCancelled
	}
	return exitOK
}

var errPathForm = errors.New("paths start with ~/, / or are relative to the current folder")

// writtenPath is how a command-line path goes into a config file: ~ forms and
// absolute paths as given, a relative path made absolute against the cwd, and
// an empty one the physical cwd; anything under HOME is written with ~.
func (a *App) writtenPath(arg string) (string, error) {
	switch {
	case arg == "~" || strings.HasPrefix(arg, "~/") || filepath.IsAbs(arg):
	case strings.HasPrefix(arg, "~") || strings.HasPrefix(arg, "$"):
		return "", fmt.Errorf("%q: %w", arg, errPathForm)
	default:
		wd, err := a.Getwd()
		if err == nil && arg == "" {
			var cwd policy.Cwd
			if cwd, err = policy.ResolveCwd(wd); err == nil {
				wd = cwd.Physical
			}
		}
		if err != nil {
			return "", fmt.Errorf("cannot resolve the current directory: %w", err)
		}
		arg = filepath.Join(wd, arg)
	}
	return a.tildePath(filepath.Clean(arg)), nil
}

// pathArg is writtenPath reporting its failure.
func (a *App) pathArg(arg string) (string, int) {
	written, err := a.writtenPath(arg)
	switch {
	case errors.Is(err, errPathForm):
		return "", a.fail(exitUsage, "%v", err)
	case err != nil:
		return "", a.fail(exitInternal, "%v", err)
	}
	return written, exitOK
}

// parseDir parses a path written for a root or attach entry and requires it
// to be a folder that exists.
func (a *App) parseDir(written string) (config.Entry, int) {
	e, err := config.ParseEntry(written, a.home(), false)
	if err != nil {
		return e, a.fail(exitConfig, "%v", err)
	}
	if info, err := os.Stat(e.Path); err != nil || !info.IsDir() {
		return e, a.fail(exitUsage, "%s is not a folder", written)
	}
	return e, exitOK
}

// writeSpace resolves the space argument of a write: a name, "." for the space
// holding the cwd, or a picker. Unlike the views it never guesses from the cwd.
func (a *App) writeSpace(cfg *config.Spaces, arg string) (*config.Space, int) {
	if arg == "" {
		switch {
		case !a.IsTTY():
			return nil, a.fail(exitUsage, "missing space; no TTY")
		case len(cfg.Spaces) == 0:
			return nil, a.fail(exitUsage, "no spaces configured")
		}
		choice, code := a.pick("space", spaceItems(cfg))
		if code != exitOK {
			return nil, code
		}
		arg = choice
	}
	return a.viewSpace(cfg, arg)
}

// splitSpacePath reads the optional [space] [path] of attach and detach. One
// argument is a path when it looks like one, which no space name can.
func splitSpacePath(pos []string) (space, path string) {
	switch len(pos) {
	case 1:
		if strings.HasPrefix(pos[0], ".") || strings.HasPrefix(pos[0], "~") || strings.Contains(pos[0], "/") {
			return "", pos[0]
		}
		return pos[0], ""
	case 2:
		return pos[0], pos[1]
	}
	return "", ""
}

// findEntry returns the entry that arg names, comparing paths once expanded and
// resolved through symlinks.
func (a *App) findEntry(entries []config.Entry, arg string) (config.Entry, bool) {
	written, err := a.writtenPath(arg)
	if err != nil {
		return config.Entry{}, false
	}
	parsed, err := config.ParseEntry(written, a.home(), true)
	if err != nil {
		return config.Entry{}, false
	}
	for _, e := range entries {
		if e.Path == parsed.Path {
			return e, true
		}
	}
	return config.Entry{}, false
}

// pickEntry picks one of entries by its written form; without a terminal the
// path has to be an argument.
func (a *App) pickEntry(prompt string, entries []config.Entry, what string) (string, int) {
	switch {
	case !a.IsTTY():
		return "", a.fail(exitUsage, "missing path; no TTY")
	case len(entries) == 0:
		return "", a.fail(exitUsage, "%s has no entries", what)
	}
	items := make([]string, len(entries))
	for i, e := range entries {
		items[i] = e.Written
	}
	return a.pickLine(prompt, items)
}

func argAt(pos []string, i int) string {
	if i < len(pos) {
		return pos[i]
	}
	return ""
}
