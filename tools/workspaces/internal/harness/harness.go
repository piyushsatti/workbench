package harness

import (
	"errors"
	"sort"
	"strings"
)

type Mode string

const (
	ModeSession    Mode = "session"
	ModePrint      Mode = "print"
	ModeUtility    Mode = "utility"
	ModeBackground Mode = "background"
)

var ErrSettingsCollision = errors.New("--settings given while workspaces also has to pass one")

type Launch struct {
	Dir     string // physical launch dir
	AddDirs []string
	// DenyAccess holds absolute logical paths; an adapter adds each one's
	// physical form itself.
	DenyAccess []string
	UserArgs   []string
	// Env is laid over BaseEnv; PWD is then set to Dir.
	Env     map[string]string
	BaseEnv []string
}

type Adapter interface {
	// Binary is the name looked up on PATH when harness.json has no bin.
	Binary() string
	Mode(userArgs []string) Mode
	Command(l Launch) (argv []string, env []string, err error)
}

// DenyLimited is implemented by an adapter that cannot express every
// deny_access path inside the session.
type DenyLimited interface {
	// UnenforcedDeny returns how many of the paths the harness cannot deny.
	UnenforcedDeny(paths []string) int
}

var registry = map[string]Adapter{
	"claude": Claude{},
	"codex":  Codex{},
}

func Lookup(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}

func Known(name string) bool {
	_, ok := registry[name]
	return ok
}

// launchEnv is the harness env laid over the base env, with PWD set to the
// launch dir.
func launchEnv(l Launch) []string {
	over := make(map[string]string, len(l.Env)+1)
	for k, v := range l.Env {
		over[k] = v
	}
	over["PWD"] = l.Dir
	return OverlayEnv(l.BaseEnv, over)
}

// Keys new to base are appended in sorted order so the result is
// deterministic.
func OverlayEnv(base []string, over map[string]string) []string {
	out := make([]string, 0, len(base)+len(over))
	seen := make(map[string]bool, len(over))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if v, ok := over[k]; ok {
			if !seen[k] {
				out = append(out, k+"="+v)
				seen[k] = true
			}
			continue
		}
		out = append(out, kv)
	}
	keys := make([]string, 0, len(over))
	for k := range over {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+over[k])
	}
	return out
}

// beforeDoubleDash returns the args a harness would read as its own options;
// everything after "--" belongs to the prompt.
func beforeDoubleDash(args []string) []string {
	for i, a := range args {
		if a == "--" {
			return args[:i]
		}
	}
	return args
}
