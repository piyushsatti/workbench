package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func tempHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func onlyClaude(name string) bool { return name == "claude" }

func TestLoadSpacesRejects(t *testing.T) {
	home := tempHome(t)
	tests := []struct {
		name string
		body string
		want string
	}{
		{"relative path", `{"allow_launch":["Source"]}`, "must start with"},
		{"dollar HOME", `{"allow_launch":["$HOME/x"]}`, "must start with"},
		{"tilde user", `{"allow_launch":["~user/x"]}`, "must start with"},
		{"empty path", `{"deny_launch":[""]}`, "must start with"},
		{"double star", `{"deny_launch":["~/Source/**"]}`, "** is not supported"},
		{"glob in deny_access", `{"deny_access":["~/Source/*"]}`, "globs are not allowed"},
		{"glob in space root", `{"spaces":{"a":{"root":"~/Source/*"}}}`, "globs are not allowed"},
		{"glob in attach", `{"spaces":{"a":{"root":"~/a","attach":["~/b*"]}}}`, "globs are not allowed"},
		{"glob in a space's deny_access", `{"spaces":{"a":{"root":"~/a","deny_access":["~/a/*"]}}}`, "globs are not allowed"},
		{"bad pattern", `{"deny_launch":["~/x/[a"]}`, "syntax error in pattern"},
		{"unknown key", `{"deny_acess":["~/x"]}`, "unknown field"},
		{"unknown space key", `{"spaces":{"a":{"root":"~/a","atach":[]}}}`, "unknown field"},
		{"space name with dot", `{"spaces":{"a.b":{"root":"~/a"}}}`, "space name"},
		{"space name with colon", `{"spaces":{"a:b":{"root":"~/a"}}}`, "space name"},
		{"space name upper case", `{"spaces":{"A":{"root":"~/a"}}}`, "space name"},
		{"space name too long", `{"spaces":{"` + strings.Repeat("a", 33) + `":{"root":"~/a"}}}`, "space name"},
		{"space without root", `{"spaces":{"a":{}}}`, "root is required"},
		{"not json", `{"allow_launch":`, "unexpected EOF"},
		{"trailing data", `{} {}`, "unexpected data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, tt.body)
			_, hash, err := LoadSpaces(path, home)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
			if hash == "" {
				t.Error("hash of an invalid but readable file should still be set")
			}
		})
	}
}

func TestLoadSpacesMissing(t *testing.T) {
	_, hash, err := LoadSpaces(filepath.Join(t.TempDir(), "spaces.json"), tempHome(t))
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("err = %v, want ErrMissing", err)
	}
	if hash != "" {
		t.Errorf("hash = %q, want empty for a missing file", hash)
	}
}

func TestLoadSpacesNormalisesPaths(t *testing.T) {
	home := tempHome(t)
	for _, d := range []string{"Source/x", "real"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(home, "real"), filepath.Join(home, "link")); err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, `{
	 "allow_launch": ["~/Source/", "~/Source/x/../x", "~", "/"],
	 "deny_launch": ["~/link/*", "~/nope/*"],
	 "deny_access": ["~/does-not-exist"],
	 "spaces": {"s": {"root": "~/Source/x/", "todo": "notes/TODO.jsonl", "log": "~/l.jsonl"}}
	}`)
	cfg, _, err := LoadSpaces(path, home)
	if err != nil {
		t.Fatal(err)
	}
	wantAllow := []string{filepath.Join(home, "Source"), filepath.Join(home, "Source/x"), home, "/"}
	for i, e := range cfg.AllowLaunch {
		if e.Path != wantAllow[i] || !e.Exists || e.Glob {
			t.Errorf("allow_launch[%d] = %+v, want path %q, existing, plain", i, e, wantAllow[i])
		}
	}
	if e := cfg.DenyLaunch[0]; e.Path != filepath.Join(home, "real/*") || !e.Glob || !e.Exists {
		t.Errorf("deny_launch[0] = %+v, want the literal part resolved through the symlink", e)
	}
	if e := cfg.DenyLaunch[1]; e.Path != filepath.Join(home, "nope/*") || e.Exists {
		t.Errorf("deny_launch[1] = %+v, want cleaned form, not existing", e)
	}
	if e := cfg.DenyAccess[0]; e.Path != filepath.Join(home, "does-not-exist") || e.Exists {
		t.Errorf("deny_access[0] = %+v, want cleaned form, not existing", e)
	}
	sp, ok := cfg.Space("s")
	if !ok {
		t.Fatal("space s missing")
	}
	if sp.Todo != filepath.Join(home, "Source/x/notes/TODO.jsonl") {
		t.Errorf("todo = %q, want it relative to root", sp.Todo)
	}
	if sp.Log != filepath.Join(home, "l.jsonl") {
		t.Errorf("log = %q, want tilde expanded", sp.Log)
	}

	missing := strings.Join(cfg.Missing(), "\n")
	for _, w := range []string{"~/nope/*", "~/does-not-exist"} {
		if !strings.Contains(missing, w) {
			t.Errorf("Missing() = %q, want it to name %s", missing, w)
		}
	}
}

func TestSpaceFileDefaults(t *testing.T) {
	home := tempHome(t)
	cfg, _, err := LoadSpaces(writeFile(t, `{"spaces":{"s":{"root":"~/r"}}}`), home)
	if err != nil {
		t.Fatal(err)
	}
	sp := cfg.Spaces["s"]
	if sp.Todo != filepath.Join(home, "r/TODO.jsonl") || sp.Log != filepath.Join(home, "r/LOGS.jsonl") {
		t.Errorf("todo, log = %q, %q; want <root>/TODO.jsonl and <root>/LOGS.jsonl", sp.Todo, sp.Log)
	}
}

func TestLoadHarnesses(t *testing.T) {
	home := tempHome(t)
	t.Run("missing file gives built-in default", func(t *testing.T) {
		h, err := LoadHarnesses(filepath.Join(t.TempDir(), "harness.json"), home, onlyClaude)
		if err != nil {
			t.Fatal(err)
		}
		if h.Default != "claude" || len(h.Harnesses) != 1 || h.Harnesses["claude"].Adapter != "claude" {
			t.Errorf("got %+v, want the built-in default", h)
		}
	})
	t.Run("env tilde expansion and default first", func(t *testing.T) {
		path := writeFile(t, `{"default":"claude","harnesses":{
		 "claude-alt":{"adapter":"claude","env":{"CLAUDE_CONFIG_DIR":"~/.claude-alt","PLAIN":"$HOME/x"}},
		 "claude":{"adapter":"claude"},
		 "alt":{"adapter":"claude","bin":"~/bin/claude"},
		 "abs":{"adapter":"claude","bin":"/opt/claude"},
		 "bare":{"adapter":"claude","bin":"claude-dev"}}}`)
		h, err := LoadHarnesses(path, home, onlyClaude)
		if err != nil {
			t.Fatal(err)
		}
		env := h.Harnesses["claude-alt"].Env
		if env["CLAUDE_CONFIG_DIR"] != home+"/.claude-alt" || env["PLAIN"] != "$HOME/x" {
			t.Errorf("env = %v, want tilde expanded and nothing else", env)
		}
		if got := h.Harnesses["alt"].Bin; got != home+"/bin/claude" {
			t.Errorf("bin = %q", got)
		}
		if got := h.Harnesses["abs"].Bin + " " + h.Harnesses["bare"].Bin; got != "/opt/claude claude-dev" {
			t.Errorf("bins = %q, want absolute and bare names kept", got)
		}
		if got := strings.Join(h.Names(), ","); got != "claude,abs,alt,bare,claude-alt" {
			t.Errorf("Names() = %s, want the default first", got)
		}
	})
	rejects := []struct {
		name, body, want string
	}{
		{"unknown adapter", `{"default":"x","harnesses":{"x":{"adapter":"gemini"}}}`, "not a compiled-in adapter"},
		{"missing adapter", `{"default":"x","harnesses":{"x":{}}}`, "adapter is required"},
		{"unknown key", `{"default":"x","harnesses":{"x":{"adapter":"claude","envs":{}}}}`, "unknown field"},
		{"bad harness name", `{"default":"x.y","harnesses":{"x.y":{"adapter":"claude"}}}`, "harness name"},
		{"default names nothing", `{"default":"y","harnesses":{"x":{"adapter":"claude"}}}`, "names no harness"},
		{"no harnesses", `{"default":"x","harnesses":{}}`, "harnesses is empty"},
		{"relative bin", `{"default":"x","harnesses":{"x":{"adapter":"claude","bin":"bin/claude"}}}`, "harnesses.x.bin"},
		{"dot-relative bin", `{"default":"x","harnesses":{"x":{"adapter":"claude","bin":"./claude"}}}`, "harnesses.x.bin"},
		{"other user's home bin", `{"default":"x","harnesses":{"x":{"adapter":"claude","bin":"~bob/claude"}}}`, "harnesses.x.bin"},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadHarnesses(writeFile(t, tt.body), home, onlyClaude)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestDirs(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name               string
		vars               map[string]string
		wantConfig, wantSt string
	}{
		{"defaults", map[string]string{"HOME": "/h"}, "/h/.config/workspaces", "/h/.local/state/workspaces"},
		{"xdg set", map[string]string{"HOME": "/h", "XDG_CONFIG_HOME": "/c", "XDG_STATE_HOME": "/s"}, "/c/workspaces", "/s/workspaces"},
		{"relative xdg ignored", map[string]string{"HOME": "/h", "XDG_CONFIG_HOME": "c", "XDG_STATE_HOME": "s"}, "/h/.config/workspaces", "/h/.local/state/workspaces"},
	}
	for _, tt := range tests {
		c, err := ConfigDir(env(tt.vars))
		if err != nil || c != tt.wantConfig {
			t.Errorf("%s: ConfigDir = %q, %v; want %q", tt.name, c, err, tt.wantConfig)
		}
		s, err := StateDir(env(tt.vars))
		if err != nil || s != tt.wantSt {
			t.Errorf("%s: StateDir = %q, %v; want %q", tt.name, s, err, tt.wantSt)
		}
	}
	if _, err := ConfigDir(env(map[string]string{})); err == nil {
		t.Error("ConfigDir with no HOME should fail")
	}
}

func TestShippedExampleLoads(t *testing.T) {
	cfg, _, err := LoadSpaces(filepath.Join("..", "..", "spaces.example.json"), tempHome(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Space("example"); !ok {
		t.Error("spaces.example.json lost its example space")
	}
}

func TestValidNameLength(t *testing.T) {
	if !ValidName(strings.Repeat("a", 32)) {
		t.Error("a 32-character name is rejected")
	}
	if ValidName(strings.Repeat("a", 33)) {
		t.Error("a 33-character name is accepted")
	}
}

func TestRelativeHomeIsAnError(t *testing.T) {
	getenv := func(k string) string {
		if k == "HOME" {
			return "rel"
		}
		return ""
	}
	if dir, err := ConfigDir(getenv); err == nil {
		t.Errorf("ConfigDir with HOME=rel = %s, want an error", dir)
	}
	if dir, err := StateDir(getenv); err == nil {
		t.Errorf("StateDir with HOME=rel = %s, want an error", dir)
	}
}

func TestHarnessEnvBareTilde(t *testing.T) {
	home := tempHome(t)
	h, err := LoadHarnesses(writeFile(t, `{"default":"c","harnesses":{"c":{"adapter":"claude","env":{"X":"~"}}}}`), home, onlyClaude)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Harnesses["c"].Env["X"]; got != home {
		t.Errorf("X = %q, want %q", got, home)
	}
}

func TestParseEntryCleans(t *testing.T) {
	home := tempHome(t)
	e, err := ParseEntry("~/nope/../nope2/", home, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "nope2"); e.Path != want {
		t.Errorf("Path = %s, want %s", e.Path, want)
	}
}

func TestMissingAttachPath(t *testing.T) {
	home := tempHome(t)
	if err := os.MkdirAll(filepath.Join(home, "Source"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, _, err := LoadSpaces(writeFile(t, `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source","attach":["~/missing"]}}}`), home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Missing(); !slices.Equal(got, []string{"spaces.a.attach ~/missing does not exist"}) {
		t.Errorf("Missing() = %q", got)
	}
}

func TestParseEntryKeepsLogicalPath(t *testing.T) {
	home := tempHome(t)
	if err := os.Mkdir(filepath.Join(home, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "real"), filepath.Join(home, "link")); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ written, logical, path string }{
		{"~/link", filepath.Join(home, "link"), filepath.Join(home, "real")},
		{"~/link/../link/", filepath.Join(home, "link"), filepath.Join(home, "real")},
		{"~/nope", filepath.Join(home, "nope"), filepath.Join(home, "nope")},
		{"/etc/", "/etc", "/etc"},
	}
	for _, tt := range tests {
		e, err := ParseEntry(tt.written, home, false)
		if err != nil {
			t.Fatal(err)
		}
		if e.Logical != tt.logical || (tt.written != "/etc/" && e.Path != tt.path) {
			t.Errorf("ParseEntry(%q) = logical %q path %q, want logical %q path %q", tt.written, e.Logical, e.Path, tt.logical, tt.path)
		}
	}
}
