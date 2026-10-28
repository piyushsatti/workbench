package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"workspaces/internal/config"
	"workspaces/internal/harness"
	"workspaces/internal/jsonedit"
)

const usageHarness = `Usage: workspaces harness list
       workspaces harness add <name> --adapter A [--bin P] [--env K=V]...
       workspaces harness rm [--yes] [harness]

list prints each harness with its adapter, binary and the names of the
environment variables it sets; the default is marked. Without a harness.json
the only harness is the built-in claude.

add writes a harness: --adapter names a compiled-in adapter (claude), --bin an
absolute path, a ~/ path or a bare name looked up on PATH, and --env may repeat.
rm removes a harness other than the default; without a name it picks from them
on a terminal, and it asks for confirmation unless --yes is given (exit 64
without a terminal and without --yes). add creates harness.json when it is
missing, starting from the built-in claude harness.
` + writeRulesHelp

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (a *App) harnessCmd(args []string) int {
	return a.dispatch("harness", usageHarness, []verb{
		{"list", a.harnessList}, {"add", a.harnessAdd}, {"rm", a.harnessRm},
	}, args)
}

func (a *App) harnessList(args []string) int {
	fs := a.flagSet("harness list", usageHarness)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 0 {
		fmt.Fprint(a.Stderr, usageHarness)
		return exitUsage
	}
	_, path, err := a.configPaths()
	var hs *config.Harnesses
	if err == nil {
		hs, err = config.LoadHarnesses(path, a.home(), harness.Known)
	}
	if err != nil {
		return a.fail(exitConfig, "%v", err)
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "HARNESS\tADAPTER\tBIN\tENV")
	for _, name := range hs.Names() {
		h := hs.Harnesses[name]
		label := name
		if name == hs.Default {
			label += " (default)"
		}
		keys := make([]string, 0, len(h.Env))
		for k := range h.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", clean(label), clean(h.Adapter), orDash(clean(a.tildePath(h.Bin))), orDash(clean(strings.Join(keys, ","))))
	}
	tw.Flush()
	return exitOK
}

type envFlag []string

func (e *envFlag) String() string     { return strings.Join(*e, ",") }
func (e *envFlag) Set(v string) error { *e = append(*e, v); return nil }

func (a *App) harnessAdd(args []string) int {
	fs := a.flagSet("harness add", usageHarness)
	adapter := fs.String("adapter", "", "adapter name")
	bin := fs.String("bin", "", "binary path or name")
	var envs envFlag
	fs.Var(&envs, "env", "K=V, repeatable")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) != 1 {
		fmt.Fprint(a.Stderr, usageHarness)
		return exitUsage
	}
	if *adapter == "" {
		return a.fail(exitUsage, "harness add: --adapter is required")
	}
	name := argAt(pos, 0)
	if !config.ValidName(name) {
		return a.fail(exitUsage, "harness name %q: must match [a-z0-9][a-z0-9_-]* and be at most 32 characters", name)
	}
	env := jsonedit.New()
	for _, kv := range envs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !envKeyPattern.MatchString(k) {
			return a.fail(exitUsage, "--env %q: want K=V with K a variable name", kv)
		}
		if _, dup := env.Get(k); dup {
			return a.fail(exitUsage, "--env %s given twice", k)
		}
		env.Set(k, jsonedit.String(v), nil)
	}
	return a.editHarnesses(func(cur *config.Harnesses, top *jsonedit.Object) (string, int) {
		if _, taken := cur.Harnesses[name]; taken {
			return "", a.fail(exitUsage, "harness %q already exists", name)
		}
		entry := jsonedit.New()
		entry.Set("adapter", jsonedit.String(*adapter), entryKeyOrder)
		if *bin != "" {
			entry.Set("bin", jsonedit.String(*bin), entryKeyOrder)
		}
		if len(envs) > 0 {
			entry.Set("env", env.Marshal(), entryKeyOrder)
		}
		hs, err := top.Child("harnesses")
		if err != nil {
			return "", a.fail(exitConfig, "%v", err)
		}
		hs.Set(name, entry.Marshal(), nil)
		top.Set("harnesses", hs.Marshal(), harnessKeyOrder)
		return "added harness " + name + " (" + *adapter + ")", exitOK
	})
}

func (a *App) harnessRm(args []string) int {
	fs := a.flagSet("harness rm", usageHarness)
	yes := fs.Bool("yes", false, "skip the confirmation")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 1 {
		fmt.Fprint(a.Stderr, usageHarness)
		return exitUsage
	}
	return a.editHarnesses(func(cur *config.Harnesses, top *jsonedit.Object) (string, int) {
		name := argAt(pos, 0)
		if name == "" {
			var removable []string
			for _, n := range cur.Names() {
				if n != cur.Default {
					removable = append(removable, n)
				}
			}
			switch {
			case !a.IsTTY():
				return "", a.fail(exitUsage, "missing harness; no TTY")
			case len(removable) == 0:
				return "", a.fail(exitUsage, "no harness other than the default %s", cur.Default)
			}
			var code int
			if name, code = a.pick("harness", removable); code != exitOK {
				return "", code
			}
		}
		if _, ok := cur.Harnesses[name]; !ok {
			return "", a.fail(exitUsage, "unknown harness %q; configured: %s", name, strings.Join(cur.Names(), ", "))
		}
		if name == cur.Default {
			return "", a.fail(exitUsage, "harness %q is the default and cannot be removed", name)
		}
		if code := a.confirm(*yes, fmt.Sprintf("Remove harness %s?", name)); code != exitOK {
			return "", code
		}
		hs, err := top.Child("harnesses")
		if err != nil {
			return "", a.fail(exitConfig, "%v", err)
		}
		hs.Delete(name)
		top.Set("harnesses", hs.Marshal(), harnessKeyOrder)
		return "removed harness " + name, exitOK
	})
}
