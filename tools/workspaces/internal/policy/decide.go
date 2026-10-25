package policy

import (
	"workspaces/internal/config"
)

const (
	ReasonAllowLaunch  = "allow_launch"
	ReasonDenyAccess   = "deny_access"
	ReasonDenyLaunch   = "deny_launch"
	ReasonAttachDenied = "attach_denied"
	ReasonNotAllowed   = "not_allowed"
)

type Decision struct {
	Allow  bool
	Reason string
	// Rule is the matching config entry as written; empty when none matched.
	Rule string
}

// Decide runs every deny test before any allow test, so a narrower allow
// never reopens a deny. Deny matches either cwd form; allow uses the
// physical path only.
func Decide(cwd Cwd, cfg *config.Spaces, space *config.Space) Decision {
	denyAccess := effectiveDenyAccess(cfg, space)
	var attach []config.Entry
	if space != nil {
		attach = space.Attach
	}

	if e, ok := firstMatch(denyAccess, cwd.Logical, cwd.Physical); ok {
		return Decision{Reason: ReasonDenyAccess, Rule: e.Written}
	}
	if e, ok := firstMatch(cfg.DenyLaunch, cwd.Logical, cwd.Physical); ok {
		return Decision{Reason: ReasonDenyLaunch, Rule: e.Written}
	}
	for _, a := range attach {
		if e, ok := firstMatch(denyAccess, a.Path); ok {
			return Decision{Reason: ReasonAttachDenied, Rule: e.Written}
		}
	}
	if e, ok := firstMatch(cfg.AllowLaunch, cwd.Physical); ok {
		return Decision{Allow: true, Reason: ReasonAllowLaunch, Rule: e.Written}
	}
	return Decision{Reason: ReasonNotAllowed}
}

// DenyAccessMatch returns the first global or space deny_access entry that
// matches any of paths. space may be nil.
func DenyAccessMatch(cfg *config.Spaces, space *config.Space, paths ...string) (config.Entry, bool) {
	return firstMatch(effectiveDenyAccess(cfg, space), paths...)
}

func effectiveDenyAccess(cfg *config.Spaces, space *config.Space) []config.Entry {
	if space == nil {
		return cfg.DenyAccess
	}
	return append(append([]config.Entry{}, cfg.DenyAccess...), space.DenyAccess...)
}

// SelectSpace returns the space whose root holds the physical cwd, the longest
// root winning, or nil. Attach paths never select a space.
func SelectSpace(cfg *config.Spaces, physical string) *config.Space {
	var best *config.Space
	for _, name := range cfg.SpaceNames() {
		sp := cfg.Spaces[name]
		if Under(physical, sp.Root.Path) && (best == nil || len(sp.Root.Path) > len(best.Root.Path)) {
			best = sp
		}
	}
	return best
}

func firstMatch(entries []config.Entry, paths ...string) (config.Entry, bool) {
	for _, e := range entries {
		for _, p := range paths {
			if Matches(p, e) {
				return e, true
			}
		}
	}
	return config.Entry{}, false
}
