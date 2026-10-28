package cli

import (
	"fmt"

	"workspaces/internal/config"
	"workspaces/internal/jsonedit"
	"workspaces/internal/policy"
)

func (a *App) spaceAdd(args []string) int {
	fs := a.flagSet("space add", usageSpace)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprint(a.Stderr, usageSpace)
		return exitUsage
	}
	name := argAt(pos, 0)
	if !config.ValidName(name) {
		return a.fail(exitUsage, "space name %q: must match [a-z0-9][a-z0-9_-]* and be at most 32 characters", name)
	}
	written, code := a.pathArg(argAt(pos, 1))
	if code != exitOK {
		return code
	}
	return a.editSpaces(func(cur *config.Spaces, top *jsonedit.Object) (string, int) {
		if _, taken := cur.Space(name); taken {
			return "", a.fail(exitUsage, "space %q already exists", name)
		}
		root, code := a.parseDir(written)
		if code != exitOK {
			return "", code
		}
		// Named-space launches check the root as both its logical and physical path.
		checked := policy.Cwd{Logical: root.Path, Physical: root.Path}
		d := policy.Decide(checked, cur, &config.Space{Name: name, Root: root})
		if !d.Allow {
			hint := ""
			if d.Reason == policy.ReasonNotAllowed {
				hint = "; allow it with: workspaces policy add allow_launch <path>"
			}
			return "", a.fail(exitRefused, "%s%s", decisionText(d, checked, name), hint)
		}
		entry := jsonedit.New()
		entry.Set("root", jsonedit.String(written), spaceKeyOrder)
		return "added space " + name + " (" + written + ")", a.setSpace(top, name, entry)
	})
}

func (a *App) spaceRm(args []string) int {
	fs := a.flagSet("space rm", usageSpace)
	yes := fs.Bool("yes", false, "skip the confirmation")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 1 {
		fmt.Fprint(a.Stderr, usageSpace)
		return exitUsage
	}
	return a.editSpaces(func(cur *config.Spaces, top *jsonedit.Object) (string, int) {
		sp, code := a.writeSpace(cur, argAt(pos, 0))
		if code != exitOK {
			return "", code
		}
		if code := a.confirm(*yes, fmt.Sprintf("Remove space %s (%s)?", sp.Name, sp.Root.Written)); code != exitOK {
			return "", code
		}
		spaces, err := top.Child("spaces")
		if err != nil {
			return "", a.fail(exitConfig, "%v", err)
		}
		spaces.Delete(sp.Name)
		top.Set("spaces", spaces.Marshal(), spacesKeyOrder)
		return "removed space " + sp.Name, exitOK
	})
}

func (a *App) spaceAttach(args []string) int {
	fs := a.flagSet("space attach", usageSpace)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 2 {
		fmt.Fprint(a.Stderr, usageSpace)
		return exitUsage
	}
	spaceArg, pathArg := splitSpacePath(pos)
	written, code := a.pathArg(pathArg)
	if code != exitOK {
		return code
	}
	return a.editSpaces(func(cur *config.Spaces, top *jsonedit.Object) (string, int) {
		sp, code := a.writeSpace(cur, spaceArg)
		if code != exitOK {
			return "", code
		}
		e, code := a.parseDir(written)
		if code != exitOK {
			return "", code
		}
		for _, have := range sp.Attach {
			if have.Path == e.Path {
				return "", a.fail(exitUsage, "%s is already attached to space %s as %s", written, sp.Name, have.Written)
			}
		}
		if m, denied := policy.DenyAccessMatch(cur, sp, e.Path, e.Logical); denied {
			return "", a.fail(exitRefused, "refused (%s): %s matches deny_access entry %s", policy.ReasonAttachDenied, written, m.Written)
		}
		return a.editSpaceList(top, sp.Name, "attach", func(list jsonedit.Array) (jsonedit.Array, int) {
			return append(list, jsonedit.String(written)), exitOK
		}, "attached "+written+" to space "+sp.Name)
	})
}

func (a *App) spaceDetach(args []string) int {
	fs := a.flagSet("space detach", usageSpace)
	yes := fs.Bool("yes", false, "skip the confirmation")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if len(pos) > 2 {
		fmt.Fprint(a.Stderr, usageSpace)
		return exitUsage
	}
	spaceArg, pathArg := splitSpacePath(pos)
	return a.editSpaces(func(cur *config.Spaces, top *jsonedit.Object) (string, int) {
		sp, code := a.writeSpace(cur, spaceArg)
		if code != exitOK {
			return "", code
		}
		if pathArg == "" {
			var choice string
			if choice, code = a.pickEntry("attach", sp.Attach, "space "+sp.Name); code != exitOK {
				return "", code
			}
			pathArg = choice
		}
		found := a.findEntries(sp.Attach, pathArg)
		if len(found) == 0 {
			return "", a.fail(exitUsage, "%s is not attached to space %s", pathArg, sp.Name)
		}
		e := found[0]
		if code := a.confirm(*yes, fmt.Sprintf("Detach %s from space %s?", e.Written, sp.Name)); code != exitOK {
			return "", code
		}
		return a.editSpaceList(top, sp.Name, "attach", func(list jsonedit.Array) (jsonedit.Array, int) {
			return list.WithoutStrings(writtenForms(found)), exitOK
		}, "detached "+e.Written+" from space "+sp.Name)
	})
}

// setSpace puts a space's raw object under spaces, adding spaces when the file
// has none.
func (a *App) setSpace(top *jsonedit.Object, name string, sp *jsonedit.Object) int {
	spaces, err := top.Child("spaces")
	if err != nil {
		return a.fail(exitConfig, "%v", err)
	}
	spaces.Set(name, sp.Marshal(), nil)
	top.Set("spaces", spaces.Marshal(), spacesKeyOrder)
	return exitOK
}

// editSpaceList changes one list of a space and returns the line to print.
func (a *App) editSpaceList(top *jsonedit.Object, space, key string, change func(jsonedit.Array) (jsonedit.Array, int), msg string) (string, int) {
	spaces, err := top.Child("spaces")
	if err != nil {
		return "", a.fail(exitConfig, "%v", err)
	}
	sp, err := spaces.Child(space)
	if err != nil {
		return "", a.fail(exitConfig, "%v", err)
	}
	raw, _ := sp.Get(key)
	list, err := jsonedit.ParseArray(raw)
	if err != nil {
		return "", a.fail(exitConfig, "spaces.%s.%s: %v", space, key, err)
	}
	list, code := change(list)
	if code != exitOK {
		return "", code
	}
	sp.Set(key, list.Marshal(), spaceKeyOrder)
	return msg, a.setSpace(top, space, sp)
}
