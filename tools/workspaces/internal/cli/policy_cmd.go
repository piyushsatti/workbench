package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"workspaces/internal/config"
	"workspaces/internal/jsonedit"
)

const usagePolicy = `Usage: workspaces policy list
       workspaces policy add <list> <path>
       workspaces policy rm [--yes] <list> [path]

The global lists are allow_launch, deny_launch and deny_access. list prints
each with its entries as written. add appends a path with the loader's rules:
it starts with ~, ~/ or /, or is relative to the current folder; allow_launch
and deny_launch may hold globs (* ? [...]), deny_access plain paths only (a
glob there is refused with exit 78, the loader's error class). A path already
in the list is refused (64). rm removes a path; without one it picks from the
list on a terminal (exit 64 without a terminal), and it asks for confirmation
unless --yes is given (exit 64 without a terminal and without --yes). add
creates spaces.json when it is missing.
` + writeRulesHelp + `
Anything that can edit spaces.json can change what workspaces allows. Once the
protection entries for ~/.config/workspaces are in the user's Claude Code settings,
a session cannot change policy; the changes then come from a human terminal.
`

var policyLists = []string{"allow_launch", "deny_launch", "deny_access"}

func (a *App) policyCmd(args []string) int {
	return a.dispatch("policy", usagePolicy, []verb{
		{"list", a.policyList}, {"add", a.policyAdd}, {"rm", a.policyRm},
	}, args)
}

func globalList(cfg *config.Spaces, list string) []config.Entry {
	switch list {
	case "allow_launch":
		return cfg.AllowLaunch
	case "deny_launch":
		return cfg.DenyLaunch
	}
	return cfg.DenyAccess
}

func (a *App) checkList(list string) int {
	if !slices.Contains(policyLists, list) {
		return a.fail(exitUsage, "unknown list %q; one of %s", list, strings.Join(policyLists, ", "))
	}
	return exitOK
}

func (a *App) policyList(args []string) int {
	fs := a.flagSet("policy list", usagePolicy)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 0 {
		fmt.Fprint(a.Stderr, usagePolicy)
		return exitUsage
	}
	cfg, code := a.loadSpacesOnly()
	if code != exitOK {
		return code
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	for _, list := range policyLists {
		entries := globalList(cfg, list)
		if len(entries) == 0 {
			fmt.Fprintf(tw, "%s\t-\n", list)
		}
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\n", list, clean(e.Written))
		}
	}
	tw.Flush()
	return exitOK
}

func (a *App) policyAdd(args []string) int {
	fs := a.flagSet("policy add", usagePolicy)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) != 2 {
		fmt.Fprint(a.Stderr, usagePolicy)
		return exitUsage
	}
	list := argAt(pos, 0)
	if code := a.checkList(list); code != exitOK {
		return code
	}
	written, code := a.pathArg(argAt(pos, 1))
	if code != exitOK {
		return code
	}
	return a.editSpaces(func(cur *config.Spaces, top *jsonedit.Object) (string, int) {
		e, err := config.ParseEntry(written, a.home(), list != "deny_access")
		if err != nil {
			return "", a.fail(exitConfig, "%s: %v", list, err)
		}
		for _, have := range globalList(cur, list) {
			if have.Path == e.Path {
				return "", a.fail(exitUsage, "%s already holds %s", list, have.Written)
			}
		}
		raw, _ := top.Get(list)
		items, err := jsonedit.ParseArray(raw)
		if err != nil {
			return "", a.fail(exitConfig, "%s: %v", list, err)
		}
		top.Set(list, append(items, jsonedit.String(written)).Marshal(), spacesKeyOrder)
		return "added " + written + " to " + list, exitOK
	})
}

func (a *App) policyRm(args []string) int {
	fs := a.flagSet("policy rm", usagePolicy)
	yes := fs.Bool("yes", false, "skip the confirmation")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprint(a.Stderr, usagePolicy)
		return exitUsage
	}
	list := argAt(pos, 0)
	if code := a.checkList(list); code != exitOK {
		return code
	}
	return a.editSpaces(func(cur *config.Spaces, top *jsonedit.Object) (string, int) {
		entries := globalList(cur, list)
		arg := argAt(pos, 1)
		if arg == "" {
			var code int
			if arg, code = a.pickEntry(list, entries, list); code != exitOK {
				return "", code
			}
		}
		e, ok := a.findEntry(entries, arg)
		if !ok {
			return "", a.fail(exitUsage, "%s does not hold %s", list, arg)
		}
		if code := a.confirm(*yes, fmt.Sprintf("Remove %s from %s?", e.Written, list)); code != exitOK {
			return "", code
		}
		raw, _ := top.Get(list)
		items, err := jsonedit.ParseArray(raw)
		if err != nil {
			return "", a.fail(exitConfig, "%s: %v", list, err)
		}
		top.Set(list, items.Without(items.Index(e.Written)).Marshal(), spacesKeyOrder)
		return "removed " + e.Written + " from " + list, exitOK
	})
}
