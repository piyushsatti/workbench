package harness

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
)

type Claude struct{}

// Written against `claude --help` of Claude Code 2.1.289. An unknown word
// falls through to session mode, which requires tmux.
var claudeUtilitySubcommands = []string{
	"agents", "auth", "auto-mode", "doctor", "gateway", "import", "install",
	"logs", "mcp", "plugin", "plugins", "purge", "rm", "setup-token", "stop",
	"kill", "ultrareview", "update", "upgrade",
}

func (Claude) Binary() string { return "claude" }

func (Claude) Mode(userArgs []string) Mode {
	args := beforeDoubleDash(userArgs)
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	has := func(flags ...string) bool {
		return slices.ContainsFunc(args, func(a string) bool { return slices.Contains(flags, a) })
	}
	switch {
	case slices.Contains(claudeUtilitySubcommands, first), has("-v", "--version", "-h", "--help"):
		return ModeUtility
	case first == "respawn", has("--bg", "--background"):
		return ModeBackground
	case has("-p", "--print"):
		return ModePrint
	default:
		return ModeSession
	}
}

// Command passes attach paths and deny_access through --settings rather than
// --add-dir, because --add-dir takes several values and would swallow a
// prompt that follows it.
func (c Claude) Command(l Launch) ([]string, []string, error) {
	argv := []string{c.Binary()}
	if len(l.AddDirs) > 0 || len(l.DenyAccess) > 0 {
		if hasSettingsFlag(l.UserArgs) {
			return nil, nil, ErrSettingsCollision
		}
		settings, err := claudeSettings(l.AddDirs, l.DenyAccess)
		if err != nil {
			return nil, nil, err
		}
		argv = append(argv, "--settings", settings)
	}
	argv = append(argv, l.UserArgs...)

	return argv, launchEnv(l), nil
}

func hasSettingsFlag(userArgs []string) bool {
	for _, a := range beforeDoubleDash(userArgs) {
		if a == "--settings" || strings.HasPrefix(a, "--settings=") {
			return true
		}
	}
	return false
}

// claudeSettings builds the --settings JSON. Field order is the key order.
func claudeSettings(addDirs, denyAccess []string) (string, error) {
	type permissions struct {
		AdditionalDirectories []string `json:"additionalDirectories,omitempty"`
		Deny                  []string `json:"deny,omitempty"`
	}
	type filesystem struct {
		DenyRead  []string `json:"denyRead"`
		DenyWrite []string `json:"denyWrite"`
	}
	type sandbox struct {
		Enabled    bool       `json:"enabled"`
		Filesystem filesystem `json:"filesystem"`
	}
	v := struct {
		Permissions permissions `json:"permissions"`
		Sandbox     *sandbox    `json:"sandbox,omitempty"`
	}{Permissions: permissions{AdditionalDirectories: addDirs}}
	if paths := denyPaths(denyAccess); len(paths) > 0 {
		for _, p := range paths {
			// Permission rules spell an absolute path with a leading //.
			v.Permissions.Deny = append(v.Permissions.Deny,
				"Read(/"+p+")", "Read(/"+p+"/**)", "Edit(/"+p+")", "Edit(/"+p+"/**)")
		}
		v.Sandbox = &sandbox{Enabled: true, Filesystem: filesystem{DenyRead: paths, DenyWrite: paths}}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// denyPaths returns each path followed by its physical form when that exists
// and differs, without repeats. A symlinked path must be denied under both
// names: the sandbox matches the name it is given.
func denyPaths(paths []string) []string {
	var out []string
	add := func(p string) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	for _, p := range paths {
		add(p)
		if phys, err := filepath.EvalSymlinks(p); err == nil && phys != p {
			add(phys)
		}
	}
	return out
}
