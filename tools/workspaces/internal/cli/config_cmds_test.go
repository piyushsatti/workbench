package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const builtinHarness = "{\n  \"default\": \"claude\",\n  \"harnesses\": {\n    \"claude\": {\n      \"adapter\": \"claude\"\n    }\n  }\n}\n"

func TestHarnessList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		harness string
		want    []string
	}{
		{"configured", "", []string{
			"HARNESS ADAPTER BIN ENV",
			"claude (default) claude - -",
			"claude-alt claude - CLAUDE_CONFIG_DIR",
		}},
		{"missing file shows the built-in default", missing, []string{
			"HARNESS ADAPTER BIN ENV",
			"claude (default) claude - -",
		}},
		{"bin, sorted env keys, another default first", `{"default":"alt","harnesses":{"claude":{"adapter":"claude"},"alt":{"adapter":"claude","bin":"~/bin/claude","env":{"Z":"1","A":"2"}}}}`, []string{
			"HARNESS ADAPTER BIN ENV",
			"alt (default) claude ~/bin/claude A,Z",
			"claude claude - -",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if tc.harness == missing {
				os.Remove(filepath.Join(f.home, ".config/workspaces/harness.json"))
			} else if tc.harness != "" {
				f.writeConfig("harness.json", tc.harness)
			}
			lines := outLines(f.runOK("harness", "list"))
			if len(lines) != len(tc.want) {
				t.Fatalf("got %q, want %d lines", lines, len(tc.want))
			}
			for i, w := range tc.want {
				if got := strings.Join(strings.Fields(lines[i]), " "); got != w {
					t.Errorf("line %d = %q, want %q", i, got, w)
				}
			}
		})
	}
}

func TestHarnessListErrors(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("harness.json", `{"default":"x","harnesses":{"claude":{"adapter":"claude"}}}`)
	if code := f.run("harness", "list"); code != exitConfig {
		t.Errorf("invalid file: exit %d, want %d", code, exitConfig)
	}
	if code := f.run("harness", "list", "x"); code != exitUsage {
		t.Errorf("extra argument: exit %d", code)
	}
}

func TestHarnessAdd(t *testing.T) {
	addEng := edit(baseHarness, "    }\n  }\n}\n", "    },\n    \"claude-two\": {\n      \"adapter\": \"claude\",\n      \"bin\": \"~/bin/claude\",\n      \"env\": {\n        \"CLAUDE_CONFIG_DIR\": \"~/.claude-two\",\n        \"A_FLAG\": \"x=y&z\"\n      }\n    }\n  }\n}\n")
	addPlain := edit(baseHarness, "    }\n  }\n}\n", "    },\n    \"x\": {\n      \"adapter\": \"claude\"\n    }\n  }\n}\n")
	runWriteCases(t, []writeCase{
		{name: "adapter, bin and env as written", args: []string{"harness", "add", "claude-two", "--adapter", "claude", "--bin", "~/bin/claude", "--env", "CLAUDE_CONFIG_DIR=~/.claude-two", "--env", "A_FLAG=x=y&z"},
			wantHarness: addEng, out: "added harness claude-two (claude)"},
		{name: "flags before the name", args: []string{"harness", "add", "--env", "CLAUDE_CONFIG_DIR=~/.claude-two", "--env", "A_FLAG=x=y&z", "--bin", "~/bin/claude", "--adapter", "claude", "claude-two"}, wantHarness: addEng},
		{name: "adapter only", args: []string{"harness", "add", "x", "--adapter", "claude"}, wantHarness: addPlain},
		{name: "bare bin name", args: []string{"harness", "add", "x", "--adapter", "claude", "--bin", "claude-2"},
			wantHarness: edit(addPlain, `"adapter": "claude"
    }
  }`, "\"adapter\": \"claude\",\n      \"bin\": \"claude-2\"\n    }\n  }")},
		{name: "empty env value", args: []string{"harness", "add", "x", "--adapter", "claude", "--env", "E="},
			wantHarness: edit(addPlain, `"adapter": "claude"
    }
  }`, "\"adapter\": \"claude\",\n      \"env\": {\n        \"E\": \"\"\n      }\n    }\n  }")},
		{name: "creates harness.json from the built-in default", harness: missing, args: []string{"harness", "add", "x", "--adapter", "claude"},
			wantHarness: edit(builtinHarness, "    }\n  }\n}\n", "    },\n    \"x\": {\n      \"adapter\": \"claude\"\n    }\n  }\n}\n")},
		{name: "compact input is reformatted", harness: `{"default":"claude","harnesses":{"claude":{"adapter":"claude"}}}`, args: []string{"harness", "add", "x", "--adapter", "claude"},
			wantHarness: edit(builtinHarness, "    }\n  }\n}\n", "    },\n    \"x\": {\n      \"adapter\": \"claude\"\n    }\n  }\n}\n")},

		{name: "unknown adapter fails validation", args: []string{"harness", "add", "x", "--adapter", "gemini"}, code: exitConfig, errHas: "not a compiled-in adapter"},
		{name: "relative bin fails validation", args: []string{"harness", "add", "x", "--adapter", "claude", "--bin", "./claude"}, code: exitConfig, errHas: "not written"},
		{name: "invalid result on a missing file creates nothing", harness: missing, args: []string{"harness", "add", "x", "--adapter", "claude", "--bin", "./claude"}, code: exitConfig},
		{name: "adapter is required", args: []string{"harness", "add", "x"}, code: exitUsage, errHas: "--adapter is required"},
		{name: "name is required", args: []string{"harness", "add", "--adapter", "claude"}, code: exitUsage},
		{name: "-- ends the flags", args: []string{"harness", "add", "--", "x", "--adapter", "claude"}, code: exitUsage},
		{name: "two names", args: []string{"harness", "add", "x", "y", "--adapter", "claude"}, code: exitUsage},
		{name: "duplicate", args: []string{"harness", "add", "claude-alt", "--adapter", "claude"}, code: exitUsage, errHas: "already exists"},
		{name: "duplicate of the built-in", harness: missing, args: []string{"harness", "add", "claude", "--adapter", "claude"}, code: exitUsage},
		{name: "invalid name", args: []string{"harness", "add", "Bad.name", "--adapter", "claude"}, code: exitUsage, errHas: "harness name"},
		{name: "env without equals", args: []string{"harness", "add", "x", "--adapter", "claude", "--env", "NOEQ"}, code: exitUsage, errHas: "K=V"},
		{name: "env with an empty key", args: []string{"harness", "add", "x", "--adapter", "claude", "--env", "=v"}, code: exitUsage},
		{name: "env key with a bad start", args: []string{"harness", "add", "x", "--adapter", "claude", "--env", "1A=v"}, code: exitUsage},
		{name: "env key given twice", args: []string{"harness", "add", "x", "--adapter", "claude", "--env", "A=1", "--env", "A=2"}, code: exitUsage, errHas: "twice"},
		{name: "existing file invalid", harness: `{"default":"x","harnesses":{"claude":{"adapter":"claude"}}}`, args: []string{"harness", "add", "y", "--adapter", "claude"}, code: exitConfig, errHas: "fix it by hand"},
	})
}

func TestHarnessRm(t *testing.T) {
	noAlt := edit(baseHarness, "    },\n    \"claude-alt\": {\n      \"adapter\": \"claude\",\n      \"env\": {\n        \"CLAUDE_CONFIG_DIR\": \"~/.claude-alt\"\n      }\n    }\n", "    }\n")
	altDefault := `{"default":"claude-alt","harnesses":{"claude":{"adapter":"claude"},"claude-alt":{"adapter":"claude"}}}`
	runWriteCases(t, []writeCase{
		{name: "--yes", args: []string{"harness", "rm", "--yes", "claude-alt"}, wantHarness: noAlt, out: "removed harness claude-alt"},
		{name: "--yes after the name", args: []string{"harness", "rm", "claude-alt", "--yes"}, wantHarness: noAlt},
		{name: "confirmed", args: []string{"harness", "rm", "claude-alt"}, tty: true, confirm: true, wantHarness: noAlt, wantConfirm: "Remove harness claude-alt?"},
		{name: "declined", args: []string{"harness", "rm", "claude-alt"}, tty: true, code: exitCancelled, wantConfirm: "Remove harness claude-alt?"},
		{name: "no TTY and no --yes", args: []string{"harness", "rm", "claude-alt"}, code: exitUsage, errHas: "--yes"},
		{name: "picker lists only the removable ones", args: []string{"harness", "rm"}, tty: true, confirm: true, picks: []string{"claude-alt"},
			wantHarness: noAlt, wantPicks: "harness|claude-alt"},
		{name: "picker cancelled", args: []string{"harness", "rm"}, tty: true, code: exitCancelled},
		{name: "omitted without a TTY", args: []string{"harness", "rm", "--yes"}, code: exitUsage, errHas: "missing harness"},
		{name: "default refused", args: []string{"harness", "rm", "--yes", "claude"}, code: exitUsage, errHas: "default"},
		{name: "default refused on a TTY before any question", args: []string{"harness", "rm", "claude"}, tty: true, confirm: true, code: exitUsage, errHas: "default"},
		{name: "default refused with no harness.json", harness: missing, args: []string{"harness", "rm", "--yes", "claude"}, code: exitUsage, errHas: "default"},
		{name: "the default is whatever the file says", harness: altDefault, args: []string{"harness", "rm", "--yes", "claude-alt"}, code: exitUsage, errHas: "default"},
		{name: "a non-default claude can go", harness: altDefault, args: []string{"harness", "rm", "--yes", "claude"},
			wantHarness: "{\n  \"default\": \"claude-alt\",\n  \"harnesses\": {\n    \"claude-alt\": {\n      \"adapter\": \"claude\"\n    }\n  }\n}\n"},
		{name: "only the default exists", harness: missing, args: []string{"harness", "rm"}, tty: true, code: exitUsage, errHas: "no harness other than the default"},
		{name: "unknown", args: []string{"harness", "rm", "--yes", "zz"}, code: exitUsage, errHas: "unknown harness"},
		{name: "unknown with no harness.json", harness: missing, args: []string{"harness", "rm", "--yes", "zz"}, code: exitUsage},
		{name: "too many arguments", args: []string{"harness", "rm", "--yes", "a", "b"}, code: exitUsage},
	})
}

const policyOnlySource = "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ]\n}\n"

func TestPolicyList(t *testing.T) {
	f := newFixture(t)
	want := []string{
		"allow_launch ~/Source",
		"deny_launch ~/Source/private/*",
		"deny_access ~/Source/Projects/_archive",
		"deny_access ~/.ssh",
	}
	lines := outLines(f.runOK("policy", "list"))
	if len(lines) != len(want) {
		t.Fatalf("got %q", lines)
	}
	for i, w := range want {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != w {
			t.Errorf("line %d = %q, want %q", i, got, w)
		}
	}

	f.writeConfig("spaces.json", `{"allow_launch":["~/Source"]}`)
	if got := strings.Join(strings.Fields(strings.ReplaceAll(f.runOK("policy", "list"), "\n", " ")), " "); got != "allow_launch ~/Source deny_launch - deny_access -" {
		t.Errorf("empty lists = %q", got)
	}
	if code := f.run("policy", "list", "x"); code != exitUsage {
		t.Errorf("extra argument: exit %d", code)
	}
	os.Remove(filepath.Join(f.home, ".config/workspaces/spaces.json"))
	if code := f.run("policy", "list"); code != exitConfig {
		t.Errorf("missing file: exit %d, want %d", code, exitConfig)
	}
}

func TestPolicyAdd(t *testing.T) {
	runWriteCases(t, []writeCase{
		{name: "allow_launch as written", args: []string{"policy", "add", "allow_launch", "~/Work"},
			wantSpaces: edit(baseSpaces, "    \"~/Source\"\n", "    \"~/Source\",\n    \"~/Work\"\n"), out: "added ~/Work to allow_launch"},
		{name: "allow_launch glob", args: []string{"policy", "add", "allow_launch", "~/Work/*"},
			wantSpaces: edit(baseSpaces, "    \"~/Source\"\n", "    \"~/Source\",\n    \"~/Work/*\"\n")},
		{name: "deny_launch glob", args: []string{"policy", "add", "deny_launch", "~/Source/scratch/*"},
			wantSpaces: edit(baseSpaces, "    \"~/Source/private/*\"\n", "    \"~/Source/private/*\",\n    \"~/Source/scratch/*\"\n")},
		{name: "deny_access plain path", args: []string{"policy", "add", "deny_access", "~/.gnupg"},
			wantSpaces: edit(baseSpaces, "    \"~/.ssh\"\n", "    \"~/.ssh\",\n    \"~/.gnupg\"\n")},
		{name: "absolute path under HOME is written with a tilde", args: []string{"policy", "add", "deny_access", "{HOME}/.gnupg"},
			wantSpaces: edit(baseSpaces, "    \"~/.ssh\"\n", "    \"~/.ssh\",\n    \"~/.gnupg\"\n")},
		{name: "absolute path outside HOME stays", args: []string{"policy", "add", "deny_access", "/etc/ssh/"},
			wantSpaces: edit(baseSpaces, "    \"~/.ssh\"\n", "    \"~/.ssh\",\n    \"/etc/ssh\"\n")},
		{name: "relative path", args: []string{"policy", "add", "allow_launch", "Projects"}, cwd: "Source",
			wantSpaces: edit(baseSpaces, "    \"~/Source\"\n", "    \"~/Source\",\n    \"~/Source/Projects\"\n")},
		{name: "path after --", args: []string{"policy", "add", "allow_launch", "--", "-x"}, cwd: "Source",
			wantSpaces: edit(baseSpaces, "    \"~/Source\"\n", "    \"~/Source\",\n    \"~/Source/-x\"\n")},
		{name: "missing list key is inserted in order", spaces: `{"allow_launch":["~/Source"],"deny_access":["~/.ssh"]}`, args: []string{"policy", "add", "deny_launch", "~/x"},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ],\n  \"deny_launch\": [\n    \"~/x\"\n  ],\n  \"deny_access\": [\n    \"~/.ssh\"\n  ]\n}\n"},
		{name: "list key before spaces", spaces: `{"spaces":{}}`, args: []string{"policy", "add", "allow_launch", "~/x"},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"~/x\"\n  ],\n  \"spaces\": {}\n}\n"},
		{name: "creates spaces.json", spaces: missing, args: []string{"policy", "add", "allow_launch", "~/Source"}, wantSpaces: policyOnlySource},
		{name: "creates spaces.json for deny_access", spaces: missing, args: []string{"policy", "add", "deny_access", "~/.ssh"},
			wantSpaces: "{\n  \"deny_access\": [\n    \"~/.ssh\"\n  ]\n}\n"},
		{name: "empty list gains an entry", spaces: `{"allow_launch":[]}`, args: []string{"policy", "add", "allow_launch", "~/Source"}, wantSpaces: policyOnlySource},

		{name: "glob in deny_access is refused as a config error", args: []string{"policy", "add", "deny_access", "~/x/*"}, code: exitConfig, errHas: "globs are not allowed"},
		{name: "glob in deny_access on a missing file creates nothing", spaces: missing, args: []string{"policy", "add", "deny_access", "~/x/?"}, code: exitConfig},
		{name: "** is refused", args: []string{"policy", "add", "allow_launch", "~/x/**"}, code: exitConfig, errHas: "**"},
		{name: "bad glob syntax is refused", args: []string{"policy", "add", "allow_launch", "~/x/[a"}, code: exitConfig},
		{name: "duplicate", args: []string{"policy", "add", "allow_launch", "~/Source"}, code: exitUsage, errHas: "already holds"},
		{name: "duplicate in another form", args: []string{"policy", "add", "allow_launch", "{HOME}/Source/"}, code: exitUsage, errHas: "already holds"},
		{name: "duplicate through a symlink", args: []string{"policy", "add", "deny_access", "~/link-ssh"}, code: exitUsage, errHas: "already holds"},
		{name: "same path in another list is fine", args: []string{"policy", "add", "deny_launch", "~/Source"},
			wantSpaces: edit(baseSpaces, "    \"~/Source/private/*\"\n", "    \"~/Source/private/*\",\n    \"~/Source\"\n")},
		{name: "unknown list", args: []string{"policy", "add", "allow", "~/x"}, code: exitUsage, errHas: "unknown list"},
		{name: "~user path", args: []string{"policy", "add", "allow_launch", "~root/x"}, code: exitUsage},
		{name: "missing path", args: []string{"policy", "add", "allow_launch"}, code: exitUsage},
		{name: "too many arguments", args: []string{"policy", "add", "allow_launch", "~/a", "~/b"}, code: exitUsage},
		{name: "existing file invalid", spaces: `{"deny_acess":[]}`, args: []string{"policy", "add", "allow_launch", "~/x"}, code: exitConfig, errHas: "fix it by hand"},
	})
}

func TestPolicyRm(t *testing.T) {
	noSource := edit(baseSpaces, "  \"allow_launch\": [\n    \"~/Source\"\n  ],", "  \"allow_launch\": [],")
	noGlob := edit(baseSpaces, "  \"deny_launch\": [\n    \"~/Source/private/*\"\n  ],", "  \"deny_launch\": [],")
	two := `{"allow_launch":["~/a","/opt/b","~/c"]}`
	runWriteCases(t, []writeCase{
		{name: "--yes", args: []string{"policy", "rm", "--yes", "allow_launch", "~/Source"}, wantSpaces: noSource, out: "removed ~/Source from allow_launch"},
		{name: "--yes first", args: []string{"policy", "rm", "allow_launch", "~/Source", "--yes"}, wantSpaces: noSource},
		{name: "glob entry by its written form", args: []string{"policy", "rm", "--yes", "deny_launch", "~/Source/private/*"}, wantSpaces: noGlob},
		{name: "absolute form", args: []string{"policy", "rm", "--yes", "allow_launch", "{HOME}/Source"}, wantSpaces: noSource},
		{name: "relative form", args: []string{"policy", "rm", "--yes", "allow_launch", "."}, cwd: "Source", wantSpaces: noSource},
		{name: "confirmed", args: []string{"policy", "rm", "allow_launch", "~/Source"}, tty: true, confirm: true, wantSpaces: noSource,
			wantConfirm: "Remove ~/Source from allow_launch?"},
		{name: "declined", args: []string{"policy", "rm", "allow_launch", "~/Source"}, tty: true, code: exitCancelled, wantConfirm: "Remove ~/Source from allow_launch?"},
		{name: "no TTY and no --yes", args: []string{"policy", "rm", "allow_launch", "~/Source"}, code: exitUsage, errHas: "--yes"},
		{name: "picker over the list", spaces: `{"deny_access":["~/Source/Projects/_archive","~/.ssh"]}`, args: []string{"policy", "rm", "deny_access"}, tty: true, confirm: true, picks: []string{"~/Source/Projects/_archive"},
			wantSpaces: "{\n  \"deny_access\": [\n    \"~/.ssh\"\n  ]\n}\n", wantPicks: "deny_access|~/Source/Projects/_archive|~/.ssh"},
		{name: "picker cancelled", args: []string{"policy", "rm", "deny_access"}, tty: true, code: exitCancelled},
		{name: "path omitted without a TTY", args: []string{"policy", "rm", "--yes", "deny_access"}, code: exitUsage, errHas: "missing path"},
		{name: "empty list", spaces: `{"allow_launch":[]}`, args: []string{"policy", "rm", "allow_launch"}, tty: true, code: exitUsage, errHas: "no entries"},
		{name: "not in the list", args: []string{"policy", "rm", "--yes", "allow_launch", "~/Work"}, code: exitUsage, errHas: "does not hold"},
		{name: "in another list only", args: []string{"policy", "rm", "--yes", "deny_launch", "~/Source"}, code: exitUsage},
		{name: "unknown list", args: []string{"policy", "rm", "--yes", "allow", "~/Source"}, code: exitUsage, errHas: "unknown list"},
		{name: "list omitted", args: []string{"policy", "rm", "--yes"}, code: exitUsage},
		{name: "too many arguments", args: []string{"policy", "rm", "--yes", "allow_launch", "~/a", "~/b"}, code: exitUsage},
		{name: "missing spaces.json", spaces: missing, args: []string{"policy", "rm", "--yes", "allow_launch", "~/Source"}, code: exitUsage},
		{name: "middle entry, others keep their form", spaces: two, args: []string{"policy", "rm", "--yes", "allow_launch", "/opt/b"},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"~/a\",\n    \"~/c\"\n  ]\n}\n"},
		{name: "last entry leaves an empty list", spaces: `{"allow_launch":["~/a"]}`, args: []string{"policy", "rm", "--yes", "allow_launch", "~/a"},
			wantSpaces: "{\n  \"allow_launch\": []\n}\n"},
		{name: "every duplicate", spaces: `{"allow_launch":["~/a","/opt/b","~/a"]}`,
			args: []string{"policy", "rm", "--yes", "allow_launch", "~/a"},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"/opt/b\"\n  ]\n}\n"},
	})
}

func TestPolicyAddCreatesConfigDirWithMode0700(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.home, "fresh")
	f.env["XDG_CONFIG_HOME"] = dir
	f.runOK("policy", "add", "allow_launch", "~/Source")
	info, err := os.Stat(filepath.Join(dir, "workspaces"))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("config dir: %v, %v, want mode 0700", info, err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "workspaces/spaces.json"))
	if err != nil || string(got) != policyOnlySource {
		t.Errorf("spaces.json = %q, %v", got, err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "workspaces/spaces.json")); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestHarnessAddCreatesConfigDir(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.home, "fresh")
	f.env["XDG_CONFIG_HOME"] = dir
	f.runOK("harness", "add", "x", "--adapter", "claude")
	if _, err := os.Stat(filepath.Join(dir, "workspaces/harness.json")); err != nil {
		t.Error(err)
	}
}

func TestWriteFailureExits74(t *testing.T) {
	f := newFixture(t)
	// A directory where the temp file must go makes the write fail.
	if err := os.Chmod(filepath.Join(f.home, ".config/workspaces"), 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(f.home, ".config/workspaces"), 0o700)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	before, _ := os.ReadFile(filepath.Join(f.home, ".config/workspaces/spaces.json"))
	if code := f.run("policy", "add", "allow_launch", "~/x"); code != exitIOErr {
		t.Fatalf("exit %d, want %d; stderr %s", code, exitIOErr, f.stderr.String())
	}
	if after, _ := os.ReadFile(filepath.Join(f.home, ".config/workspaces/spaces.json")); string(before) != string(after) {
		t.Error("file changed after a failed write")
	}
}
