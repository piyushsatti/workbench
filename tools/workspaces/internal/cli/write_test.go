package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workspaces/internal/picker"
)

// baseSpaces and baseHarness are in the form workspaces writes, so an expected file
// is the base with one edit.
const baseSpaces = `{
  "allow_launch": [
    "~/Source"
  ],
  "deny_launch": [
    "~/Source/private/*"
  ],
  "deny_access": [
    "~/.ssh"
  ],
  "spaces": {
    "a": {
      "root": "~/Source/Projects/a",
      "attach": [
        "~/Source/Projects/b"
      ],
      "deny_access": [
        "~/Source/Projects/a/secrets"
      ]
    },
    "b": {
      "root": "~/Source/Projects/b"
    }
  }
}
`

const baseHarness = `{
  "default": "claude",
  "harnesses": {
    "claude": {
      "adapter": "claude"
    },
    "claude-alt": {
      "adapter": "claude",
      "env": {
        "CLAUDE_CONFIG_DIR": "~/.claude-alt"
      }
    }
  }
}
`

const aBlock = `    "a": {
      "root": "~/Source/Projects/a",
      "attach": [
        "~/Source/Projects/b"
      ],
      "deny_access": [
        "~/Source/Projects/a/secrets"
      ]
    },
`

// edit returns s with the one occurrence of old replaced.
func edit(s, old, repl string) string {
	if strings.Count(s, old) != 1 {
		panic("edit: " + old + " must occur once")
	}
	return strings.Replace(s, old, repl, 1)
}

const (
	missing    = "-"
	prevMarker = "sentinel .prev, must survive a refused write\n"
)

type writeCase struct {
	name string
	// Arguments may spell the home folder as {HOME}.
	args []string
	// Initial files: "" is the base, missing means the file does not exist.
	spaces, harness string
	cwd             string // relative to HOME; default Source
	tty, confirm    bool
	picks           []string
	setup           func(f *fixture)
	code            int
	// Expected files: "" means untouched, missing means still absent.
	wantSpaces, wantHarness string
	out                     string // expected stdout line
	errHas                  string
	wantPicks               string // the first picker's prompt and items, "|"-joined
	wantConfirm             string
}

func runWriteCases(t *testing.T, cases []writeCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			for _, d := range []string{".ssh/sub", "Source/Projects/c", "Source/Projects/d"} {
				if err := os.MkdirAll(filepath.Join(f.home, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for link, target := range map[string]string{"link-b": "Source/Projects/b", "link-ssh": ".ssh", "link-c": "Source/Projects/c"} {
				if err := os.Symlink(filepath.Join(f.home, target), filepath.Join(f.home, link)); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(f.home, ".config/workspaces")
			initial := map[string]string{"spaces.json": c.spaces, "harness.json": c.harness}
			base := map[string]string{"spaces.json": baseSpaces, "harness.json": baseHarness}
			for name, body := range initial {
				switch body {
				case "":
					body = base[name]
				case missing:
					os.Remove(filepath.Join(dir, name))
					continue
				}
				initial[name] = body
				f.writeConfig(name, body)
				f.writeConfig(name+".prev", prevMarker)
			}
			if c.cwd != "" {
				f.cwd = filepath.Join(f.home, c.cwd)
			}
			f.tty, f.confirmReply, f.pickReply = c.tty, c.confirm, c.picks
			if c.setup != nil {
				c.setup(f)
			}
			args := make([]string, len(c.args))
			for i, a := range c.args {
				args[i] = strings.ReplaceAll(a, "{HOME}", f.home)
			}

			code := f.run(args...)
			if code != c.code {
				t.Fatalf("%q: exit %d, want %d; stderr %q", args, code, c.code, f.stderr.String())
			}
			if c.out != "" && strings.TrimSpace(f.stdout.String()) != c.out {
				t.Errorf("stdout = %q, want %q", f.stdout.String(), c.out)
			}
			if c.code != exitOK && f.stdout.Len() != 0 {
				t.Errorf("a failed command printed %q on stdout", f.stdout.String())
			}
			if c.errHas != "" && !strings.Contains(f.stderr.String(), c.errHas) {
				t.Errorf("stderr = %q, want it to contain %q", f.stderr.String(), c.errHas)
			}
			if c.wantPicks != "" && (len(f.picks) == 0 || strings.Join(f.picks[0], "|") != c.wantPicks) {
				t.Errorf("picks = %q, want first %q", f.picks, c.wantPicks)
			}
			if c.wantConfirm != "" && (len(f.confirms) != 1 || f.confirms[0] != c.wantConfirm) {
				t.Errorf("confirms = %q, want [%q]", f.confirms, c.wantConfirm)
			}
			for name, want := range map[string]string{"spaces.json": c.wantSpaces, "harness.json": c.wantHarness} {
				checkFile(t, dir, name, initial[name], want)
			}
			if f.logExists() {
				t.Error("a config write was recorded in the launch log")
			}
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				switch e.Name() {
				case "spaces.json", "spaces.json.prev", "harness.json", "harness.json.prev":
				default:
					t.Errorf("stray file %s left in the config dir", e.Name())
				}
			}
		})
	}
}

func checkFile(t *testing.T, dir, name, initial, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	prev, prevErr := os.ReadFile(filepath.Join(dir, name+".prev"))
	switch {
	case want == "" && initial == missing:
		if err == nil {
			t.Errorf("%s was created: %q", name, got)
		}
	case want == "":
		if string(got) != initial {
			t.Errorf("%s changed:\n%s\nwant it untouched:\n%s", name, got, initial)
		}
		if string(prev) != prevMarker {
			t.Errorf("%s.prev = %q, want it untouched", name, prev)
		}
	default:
		if err != nil || string(got) != want {
			t.Errorf("%s =\n%s\nwant\n%s (err %v)", name, got, want, err)
		}
		switch {
		case initial == missing && prevErr == nil:
			t.Errorf("%s.prev exists after the first write: %q", name, prev)
		case initial != missing && string(prev) != initial:
			t.Errorf("%s.prev = %q, want the bytes before the write %q", name, prev, initial)
		}
	}
}

func TestSpaceAdd(t *testing.T) {
	addC := edit(baseSpaces, "    }\n  }\n}\n", "    },\n    \"c\": {\n      \"root\": \"~/Source/Projects/c\"\n    }\n  }\n}\n")
	onlyC := "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ],\n  \"spaces\": {\n    \"c\": {\n      \"root\": \"~/Source/Projects/c\"\n    }\n  }\n}\n"
	outside := t.TempDir()
	runWriteCases(t, []writeCase{
		{name: "tilde root as written", args: []string{"space", "add", "c", "~/Source/Projects/c"}, wantSpaces: addC,
			out: "added space c (~/Source/Projects/c)"},
		{name: "trailing slash cleaned", args: []string{"space", "add", "c", "~/Source/Projects/c/"}, wantSpaces: addC},
		{name: "absolute root under HOME is written with a tilde", args: []string{"space", "add", "c", "{HOME}/Source/Projects/c"}, wantSpaces: addC},
		{name: "relative root", args: []string{"space", "add", "c", "../c"}, cwd: "Source/Projects/a", wantSpaces: addC},
		{name: "default root is the physical cwd", args: []string{"space", "add", "c"}, cwd: "Source/Projects/c", wantSpaces: addC},
		{name: "default root through a symlinked cwd is written resolved", args: []string{"space", "add", "c"}, cwd: "link-c", wantSpaces: addC},
		{name: "root through a symlink is written as typed", args: []string{"space", "add", "c", "~/link-c"},
			wantSpaces: edit(addC, `"~/Source/Projects/c"`, `"~/link-c"`), out: "added space c (~/link-c)"},
		{name: "new space in a file with no spaces key", spaces: `{"allow_launch":["~/Source"]}`, args: []string{"space", "add", "c", "~/Source/Projects/c"}, wantSpaces: onlyC},
		{name: "spaces null is an empty object", spaces: `{"allow_launch":["~/Source"],"spaces":null}`, args: []string{"space", "add", "c", "~/Source/Projects/c"}, wantSpaces: onlyC},
		{name: "compact input is reformatted, content kept",spaces: `{"allow_launch":["~/Source"],"spaces":{}}`, args: []string{"space", "add", "c", "~/Source/Projects/c"}, wantSpaces: onlyC},
		{name: "root outside HOME stays absolute", spaces: `{"allow_launch":["/"]}`, args: []string{"space", "add", "c", outside},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"/\"\n  ],\n  \"spaces\": {\n    \"c\": {\n      \"root\": \"" + outside + "\"\n    }\n  }\n}\n",
			out:        "added space c (" + outside + ")"},

		{name: "refused by deny_launch", args: []string{"space", "add", "c", "~/Source/private/drafts"}, code: exitRefused, errHas: "deny_launch"},
		{name: "refused by global deny_access", args: []string{"space", "add", "c", "~/.ssh"}, code: exitRefused, errHas: "deny_access"},
		{name: "refused by deny_access through a symlink", args: []string{"space", "add", "c", "~/link-ssh"}, code: exitRefused, errHas: "deny_access"},
		{name: "refused by a deny_access entry inside an allowed folder", spaces: `{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/c"]}`,
			args: []string{"space", "add", "c", "~/Source/Projects/c"}, code: exitRefused, errHas: "deny_access"},
		{name: "refused: not allowed", args: []string{"space", "add", "c", "~/outside"}, code: exitRefused, errHas: "not_allowed"},
		{name: "refused: no spaces.json means nothing is allowed", spaces: missing, args: []string{"space", "add", "c", "~/Source/Projects/c"},
			code: exitRefused, errHas: "workspaces policy add allow_launch"},
		{name: "duplicate name", args: []string{"space", "add", "a", "~/Source/Projects/c"}, code: exitUsage, errHas: "already exists"},
		{name: "invalid name", args: []string{"space", "add", "A", "~/Source/Projects/c"}, code: exitUsage, errHas: "space name"},
		{name: "name with a dot", args: []string{"space", "add", "a.b", "~/Source/Projects/c"}, code: exitUsage},
		{name: "name too long", args: []string{"space", "add", strings.Repeat("a", 33), "~/Source/Projects/c"}, code: exitUsage},
		{name: "root does not exist", args: []string{"space", "add", "c", "~/Source/Projects/zzz"}, code: exitUsage, errHas: "not a folder"},
		{name: "root is a file", args: []string{"space", "add", "c", "~/Source/Projects/a/sub/f"}, code: exitUsage, errHas: "not a folder",
			setup: func(f *fixture) {
				os.WriteFile(filepath.Join(f.home, "Source/Projects/a/sub/f"), nil, 0o600)
			}},
		{name: "glob root", args: []string{"space", "add", "c", "~/Source/*"}, code: exitConfig, errHas: "globs are not allowed"},
		{name: "~user root", args: []string{"space", "add", "c", "~root/x"}, code: exitUsage, errHas: "paths start with"},
		{name: "$HOME root", args: []string{"space", "add", "c", "$HOME/Source"}, code: exitUsage, errHas: "paths start with"},
		{name: "no name", args: []string{"space", "add"}, code: exitUsage},
		{name: "too many arguments", args: []string{"space", "add", "c", "~/Source/Projects/c", "x"}, code: exitUsage},
		{name: "existing file invalid", spaces: `{"deny_acess":[]}`, args: []string{"space", "add", "c", "~/Source/Projects/c"}, code: exitConfig, errHas: "fix it by hand"},
		{name: "existing file has a repeated key", spaces: `{"allow_launch":["~/Source"],"allow_launch":["~/Source"]}`,
			args: []string{"space", "add", "c", "~/Source/Projects/c"}, code: exitConfig, errHas: "appears twice"},
		{name: "existing file not an object", spaces: `[]`, args: []string{"space", "add", "c", "~/Source/Projects/c"}, code: exitConfig},
	})
}

func TestSpaceRm(t *testing.T) {
	noA := strings.Replace(baseSpaces, aBlock, "", 1)
	noB := edit(baseSpaces, "    },\n    \"b\": {\n      \"root\": \"~/Source/Projects/b\"\n    }\n", "    }\n")
	noSpaces := "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ],\n  \"deny_launch\": [\n    \"~/Source/private/*\"\n  ],\n  \"deny_access\": [\n    \"~/.ssh\"\n  ],\n  \"spaces\": {}\n}\n"
	runWriteCases(t, []writeCase{
		{name: "--yes without a TTY", args: []string{"space", "rm", "--yes", "a"}, wantSpaces: noA, out: "removed space a"},
		{name: "--yes after the name", args: []string{"space", "rm", "b", "--yes"}, wantSpaces: noB, out: "removed space b"},
		{name: "--yes with a TTY skips the question", args: []string{"space", "rm", "--yes", "a"}, tty: true, wantSpaces: noA},
		{name: "confirmed on a TTY", args: []string{"space", "rm", "a"}, tty: true, confirm: true, wantSpaces: noA,
			wantConfirm: "Remove space a (~/Source/Projects/a)?"},
		{name: "declined on a TTY", args: []string{"space", "rm", "a"}, tty: true, code: exitCancelled, wantConfirm: "Remove space a (~/Source/Projects/a)?"},
		{name: "no TTY and no --yes", args: []string{"space", "rm", "a"}, code: exitUsage, errHas: "--yes"},
		{name: "picker when omitted", args: []string{"space", "rm"}, tty: true, confirm: true, picks: []string{"b  ~/Source/Projects/b"},
			wantSpaces: noB, wantPicks: "space|a  ~/Source/Projects/a|b  ~/Source/Projects/b"},
		{name: "picker cancelled", args: []string{"space", "rm"}, tty: true, code: exitCancelled},
		{name: "picker does not guess from the cwd", args: []string{"space", "rm"}, tty: true, cwd: "Source/Projects/a", confirm: true,
			picks: []string{"b"}, wantSpaces: noB, wantPicks: "space|a  ~/Source/Projects/a|b  ~/Source/Projects/b"},
		{name: "omitted without a TTY", args: []string{"space", "rm", "--yes"}, code: exitUsage, errHas: "missing space"},
		{name: "dot is the space holding the cwd", args: []string{"space", "rm", "--yes", "."}, cwd: "Source/Projects/a/sub", wantSpaces: noA},
		{name: "unknown space", args: []string{"space", "rm", "--yes", "zz"}, code: exitUsage, errHas: "unknown space"},
		{name: "too many arguments", args: []string{"space", "rm", "--yes", "a", "b"}, code: exitUsage},
		{name: "last space leaves an empty object", spaces: `{"allow_launch":["~/Source"],"deny_launch":["~/Source/private/*"],"deny_access":["~/.ssh"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`,
			args: []string{"space", "rm", "--yes", "a"}, wantSpaces: noSpaces},
		{name: "missing spaces.json", spaces: missing, args: []string{"space", "rm", "--yes", "a"}, code: exitUsage, errHas: "unknown space"},
	})
}

func TestSpaceAttach(t *testing.T) {
	attachC := edit(baseSpaces, "\"~/Source/Projects/b\"\n      ],", "\"~/Source/Projects/b\",\n        \"~/Source/Projects/c\"\n      ],")
	bAttach := func(path string) string {
		return edit(baseSpaces, "\"root\": \"~/Source/Projects/b\"\n    }", "\"root\": \"~/Source/Projects/b\",\n      \"attach\": [\n        \""+path+"\"\n      ]\n    }")
	}
	dSpaces := `{"allow_launch":["~/Source"],"spaces":{"d":{"root":"~/Source/Projects/d","deny_access":["~/x"]},"e":{"root":"~/Source/Projects/b","attach":[]}}}`
	runWriteCases(t, []writeCase{
		{name: "path as written", args: []string{"space", "attach", "a", "~/Source/Projects/c"}, wantSpaces: attachC,
			out: "attached ~/Source/Projects/c to space a"},
		{name: "absolute path is written with a tilde", args: []string{"space", "attach", "a", "{HOME}/Source/Projects/c"}, wantSpaces: attachC},
		{name: "relative path", args: []string{"space", "attach", "a", "../c"}, cwd: "Source/Projects/a", wantSpaces: attachC},
		{name: "default path is the physical cwd", args: []string{"space", "attach", "a"}, cwd: "link-c", wantSpaces: attachC},
		{name: "one argument naming a space", args: []string{"space", "attach", "b"}, cwd: "Source/Projects/c", wantSpaces: bAttach("~/Source/Projects/c")},
		{name: "one argument that is a path, space picked", args: []string{"space", "attach", "~/Source/Projects/c"}, tty: true, picks: []string{"b  ~/Source/Projects/b"},
			wantSpaces: bAttach("~/Source/Projects/c"), wantPicks: "space|a  ~/Source/Projects/a|b  ~/Source/Projects/b"},
		{name: "one dot argument is a path", args: []string{"space", "attach", "."}, tty: true, cwd: "Source/Projects/c", picks: []string{"b"}, wantSpaces: bAttach("~/Source/Projects/c")},
		{name: "dot as the space", args: []string{"space", "attach", ".", "~/Source/Projects/c"}, cwd: "Source/Projects/a/sub", wantSpaces: attachC},
		{name: "space picker with the default path", args: []string{"space", "attach"}, tty: true, cwd: "Source/Projects/c", picks: []string{"b"}, wantSpaces: bAttach("~/Source/Projects/c")},
		{name: "space omitted without a TTY", args: []string{"space", "attach", "~/Source/Projects/c"}, code: exitUsage, errHas: "missing space"},
		{name: "picker cancelled", args: []string{"space", "attach", "~/Source/Projects/c"}, tty: true, code: exitCancelled},
		{name: "key inserted in order", spaces: dSpaces, args: []string{"space", "attach", "d", "~/Source/Projects/c"},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ],\n  \"spaces\": {\n    \"d\": {\n      \"root\": \"~/Source/Projects/d\",\n      \"attach\": [\n        \"~/Source/Projects/c\"\n      ],\n      \"deny_access\": [\n        \"~/x\"\n      ]\n    },\n    \"e\": {\n      \"root\": \"~/Source/Projects/b\",\n      \"attach\": []\n    }\n  }\n}\n"},
		{name: "empty attach list gains an entry", spaces: dSpaces, args: []string{"space", "attach", "e", "~/Source/Projects/c"},
			wantSpaces: "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ],\n  \"spaces\": {\n    \"d\": {\n      \"root\": \"~/Source/Projects/d\",\n      \"deny_access\": [\n        \"~/x\"\n      ]\n    },\n    \"e\": {\n      \"root\": \"~/Source/Projects/b\",\n      \"attach\": [\n        \"~/Source/Projects/c\"\n      ]\n    }\n  }\n}\n"},

		{name: "duplicate", args: []string{"space", "attach", "a", "~/Source/Projects/b"}, code: exitUsage, errHas: "already attached"},
		{name: "duplicate written another way", args: []string{"space", "attach", "a", "{HOME}/Source/Projects/b"}, code: exitUsage, errHas: "already attached"},
		{name: "duplicate by symlink", args: []string{"space", "attach", "a", "~/link-b"}, code: exitUsage, errHas: "already attached"},
		{name: "duplicate by cwd", args: []string{"space", "attach", "a"}, cwd: "Source/Projects/b", code: exitUsage, errHas: "already attached"},
		{name: "under global deny_access", args: []string{"space", "attach", "a", "~/.ssh"}, code: exitRefused, errHas: "attach_denied"},
		{name: "under global deny_access, deeper", args: []string{"space", "attach", "a", "~/.ssh/sub"}, code: exitRefused, errHas: "attach_denied"},
		{name: "under global deny_access through a symlink", args: []string{"space", "attach", "a", "~/link-ssh"}, code: exitRefused, errHas: "attach_denied"},
		{name: "logical path under deny_access, resolved path elsewhere", spaces: `{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/d"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`,
			args: []string{"space", "attach", "a", "~/Source/Projects/d/lnk"}, code: exitRefused, errHas: "attach_denied",
			setup: func(f *fixture) {
				os.Symlink(filepath.Join(f.home, "Source/Projects/c"), filepath.Join(f.home, "Source/Projects/d/lnk"))
			}},
		{name: "under the space's own deny_access", args: []string{"space", "attach", "a", "~/Source/Projects/a/secrets"}, code: exitRefused, errHas: "~/Source/Projects/a/secrets"},
		{name: "another space's deny_access does not apply", args: []string{"space", "attach", "b", "~/Source/Projects/a/secrets"},
			wantSpaces: bAttach("~/Source/Projects/a/secrets")},
		{name: "allow_launch and deny_launch do not govern attach", args: []string{"space", "attach", "b", "~/Source/private/drafts"},
			wantSpaces: bAttach("~/Source/private/drafts")},
		{name: "path does not exist", args: []string{"space", "attach", "a", "~/Source/Projects/zzz"}, code: exitUsage, errHas: "not a folder"},
		{name: "glob path", args: []string{"space", "attach", "a", "~/Source/*"}, code: exitConfig, errHas: "globs are not allowed"},
		{name: "unknown space", args: []string{"space", "attach", "zz", "~/Source/Projects/c"}, code: exitUsage, errHas: "unknown space"},
		{name: "too many arguments", args: []string{"space", "attach", "a", "~/Source/Projects/c", "x"}, code: exitUsage},
	})
}

func TestSpaceDetach(t *testing.T) {
	detached := edit(baseSpaces, "\"attach\": [\n        \"~/Source/Projects/b\"\n      ],", "\"attach\": [],")
	three := `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b","~/Source/Projects/c","/opt/x"]}}}`
	withAttach := func(items string) string {
		return "{\n  \"allow_launch\": [\n    \"~/Source\"\n  ],\n  \"spaces\": {\n    \"a\": {\n      \"root\": \"~/Source/Projects/a\",\n      \"attach\": " + items + "\n    }\n  }\n}\n"
	}
	runWriteCases(t, []writeCase{
		{name: "--yes", args: []string{"space", "detach", "--yes", "a", "~/Source/Projects/b"}, wantSpaces: detached, out: "detached ~/Source/Projects/b from space a"},
		{name: "every duplicate, in any written form",
			spaces: `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/Source/Projects/b","/opt/x","~/link-b","~/Source/Projects/b"]}}}`,
			args:   []string{"space", "detach", "--yes", "a", "~/Source/Projects/b"}, wantSpaces: withAttach("[\n        \"/opt/x\"\n      ]")},
		{name: "--yes after the arguments", args: []string{"space", "detach", "a", "~/Source/Projects/b", "--yes"}, wantSpaces: detached},
		{name: "absolute form matches the tilde form", args: []string{"space", "detach", "--yes", "a", "{HOME}/Source/Projects/b"}, wantSpaces: detached},
		{name: "symlink form matches", args: []string{"space", "detach", "--yes", "a", "~/link-b"}, wantSpaces: detached},
		{name: "relative form matches", args: []string{"space", "detach", "--yes", "a", "../b"}, cwd: "Source/Projects/a", wantSpaces: detached},
		{name: "one argument that is a path, space picked", args: []string{"space", "detach", "--yes", "~/Source/Projects/b"}, tty: true, picks: []string{"a"}, wantSpaces: detached,
			wantPicks: "space|a  ~/Source/Projects/a|b  ~/Source/Projects/b"},
		{name: "confirmed", args: []string{"space", "detach", "a", "~/Source/Projects/b"}, tty: true, confirm: true, wantSpaces: detached,
			wantConfirm: "Detach ~/Source/Projects/b from space a?"},
		{name: "declined", args: []string{"space", "detach", "a", "~/Source/Projects/b"}, tty: true, code: exitCancelled,
			wantConfirm: "Detach ~/Source/Projects/b from space a?"},
		{name: "no TTY and no --yes", args: []string{"space", "detach", "a", "~/Source/Projects/b"}, code: exitUsage, errHas: "--yes"},
		{name: "path picked from the attach list", args: []string{"space", "detach", "a"}, tty: true, confirm: true, picks: []string{"~/Source/Projects/b"},
			wantSpaces: detached, wantPicks: "attach|~/Source/Projects/b"},
		{name: "picked path with spaces survives", spaces: `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a","attach":["~/with space"]}}}`,
			args: []string{"space", "detach", "a"}, tty: true, confirm: true, picks: []string{"~/with space"}, wantSpaces: withAttach("[]")},
		{name: "path picker cancelled", args: []string{"space", "detach", "a"}, tty: true, code: exitCancelled},
		{name: "path omitted without a TTY", args: []string{"space", "detach", "--yes", "a"}, code: exitUsage, errHas: "missing path"},
		{name: "space has nothing attached", args: []string{"space", "detach", "b"}, tty: true, code: exitUsage, errHas: "no entries"},
		{name: "not attached", args: []string{"space", "detach", "--yes", "a", "~/Source/Projects/c"}, code: exitUsage, errHas: "not attached"},
		{name: "unknown space", args: []string{"space", "detach", "--yes", "zz", "~/Source/Projects/b"}, code: exitUsage},
		{name: "first of three", spaces: three, args: []string{"space", "detach", "--yes", "a", "~/Source/Projects/b"},
			wantSpaces: withAttach("[\n        \"~/Source/Projects/c\",\n        \"/opt/x\"\n      ]")},
		{name: "last of three, outside HOME", spaces: three, args: []string{"space", "detach", "--yes", "a", "/opt/x"},
			wantSpaces: withAttach("[\n        \"~/Source/Projects/b\",\n        \"~/Source/Projects/c\"\n      ]")},
		{name: "too many arguments", args: []string{"space", "detach", "--yes", "a", "~/Source/Projects/b", "x"}, code: exitUsage},
	})
}

func TestVerbPickersAndHelp(t *testing.T) {
	f := newFixture(t)
	f.pickReply = []string{"list"}
	f.runOK("harness")
	if got := strings.Join(f.picks[0], "|"); got != "harness|list|add|rm" {
		t.Errorf("harness verb picker = %s", got)
	}
	f.picks, f.pickReply = nil, []string{"list"}
	f.runOK("policy")
	if got := strings.Join(f.picks[0], "|"); got != "policy|list|add|rm" {
		t.Errorf("policy verb picker = %s", got)
	}

	f.tty = false
	for _, noun := range []string{"harness", "policy"} {
		if code := f.run(noun); code != exitUsage {
			t.Errorf("%s without a verb or TTY: exit %d", noun, code)
		}
		if code := f.run(noun, "bogus"); code != exitUsage {
			t.Errorf("%s bogus: exit %d", noun, code)
		}
	}
	for _, args := range [][]string{
		{"space", "-h"}, {"harness", "-h"}, {"policy", "-h"},
		{"space", "add", "-h"}, {"space", "rm", "-h"}, {"space", "attach", "-h"}, {"space", "detach", "-h"},
		{"harness", "add", "-h"}, {"harness", "rm", "-h"}, {"policy", "add", "-h"}, {"policy", "rm", "-h"},
	} {
		f.stderr.Reset()
		if code := f.run(args...); code != exitOK {
			t.Errorf("%q: exit %d", args, code)
		}
		for _, want := range []string{"Usage: workspaces " + args[0], "<file>.prev"} {
			if !strings.Contains(f.stderr.String(), want) {
				t.Errorf("%q: help lacks %q", args, want)
			}
		}
	}
	f.stderr.Reset()
	f.run("bogus")
	for _, line := range []string{"space add", "space rm", "space attach", "space detach", "harness list", "harness add", "harness rm", "policy list", "policy add", "policy rm"} {
		if !strings.Contains(f.stderr.String(), line) {
			t.Errorf("top-level usage lacks %q", line)
		}
	}
}

func TestConfirmErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		has  string
	}{
		{"gum missing", picker.ErrNoConfirm, exitUsage, "--yes"},
		{"gum failing", os.ErrPermission, exitInternal, "confirm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.confirmErr = tc.err
			path := filepath.Join(f.home, ".config/workspaces/spaces.json")
			before, _ := os.ReadFile(path)
			if code := f.run("space", "rm", "a"); code != tc.want {
				t.Fatalf("exit %d, want %d", code, tc.want)
			}
			if !strings.Contains(f.stderr.String(), tc.has) {
				t.Errorf("stderr = %q", f.stderr.String())
			}
			if after, _ := os.ReadFile(path); string(before) != string(after) {
				t.Error("file changed")
			}
		})
	}
}

// Concurrent edits each hold the config folder's lock from read to rename, so
// none of them is lost.
func TestConcurrentPolicyAdds(t *testing.T) {
	f := newFixture(t)
	f.writeConfig("spaces.json", baseSpaces)
	const n = 16
	codes := make(chan int, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			a := f.app()
			a.Stdout, a.Stderr = io.Discard, io.Discard
			codes <- a.Run([]string{"policy", "add", "deny_access", fmt.Sprintf("~/x%d", i)})
		}(i)
	}
	for i := 0; i < n; i++ {
		if code := <-codes; code != exitOK {
			t.Fatalf("policy add exit %d", code)
		}
	}
	got, err := os.ReadFile(filepath.Join(f.home, ".config/workspaces/spaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if !strings.Contains(string(got), fmt.Sprintf(`"~/x%d"`, i)) {
			t.Errorf("~/x%d was lost:\n%s", i, got)
		}
	}
}
