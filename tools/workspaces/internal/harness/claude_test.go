package harness

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClaudeMode(t *testing.T) {
	tests := []struct {
		args []string
		want Mode
	}{
		{nil, ModeSession},
		{[]string{"fix it"}, ModeSession},
		{[]string{"--resume", "x"}, ModeSession},
		{[]string{"-c"}, ModeSession},
		{[]string{"attach", "abc"}, ModeSession},
		{[]string{"-p", "x"}, ModePrint},
		{[]string{"--print"}, ModePrint},
		{[]string{"--bg"}, ModeBackground},
		{[]string{"--background"}, ModeBackground},
		{[]string{"-v"}, ModeUtility},
		{[]string{"respawn"}, ModeBackground},
		{[]string{"--version"}, ModeUtility},
		{[]string{"-h"}, ModeUtility},
		{[]string{"auth", "status"}, ModeUtility},
		{[]string{"mcp", "list"}, ModeUtility},
		{[]string{"update"}, ModeUtility},
		{[]string{"--bg", "--version"}, ModeUtility},
		{[]string{"respawn", "-h"}, ModeUtility},
		{[]string{"-p", "x", "--help"}, ModeUtility},
		{[]string{"--bg", "-p", "x"}, ModeBackground},
		{[]string{"--", "-p"}, ModeSession},
		{[]string{"some-future-subcommand"}, ModeSession},
	}
	for _, tt := range tests {
		if got := (Claude{}).Mode(tt.args); got != tt.want {
			t.Errorf("Mode(%q) = %s, want %s", tt.args, got, tt.want)
		}
	}
}

func TestClaudeCommand(t *testing.T) {
	base := []string{"HOME=/h", "PWD=/old", "CLAUDE_CONFIG_DIR=/parent", "KEEP=1"}
	tests := []struct {
		name     string
		launch   Launch
		wantArgv []string
	}{
		{
			name:     "no space",
			launch:   Launch{Dir: "/h/Source", UserArgs: []string{"--resume", "abc"}},
			wantArgv: []string{"claude", "--resume", "abc"},
		},
		{
			name:   "attach paths become --settings",
			launch: Launch{Dir: "/h/a", AddDirs: []string{"/h/b", "/h/c d"}, UserArgs: []string{"--resume", "abc"}},
			wantArgv: []string{"claude", "--settings",
				`{"permissions":{"additionalDirectories":["/h/b","/h/c d"]}}`, "--resume", "abc"},
		},
		{
			name:     "user args kept exactly, -- included",
			launch:   Launch{Dir: "/h", UserArgs: []string{"-p", "it's \"quoted\"", "--", "--settings", "  spaced  "}},
			wantArgv: []string{"claude", "-p", "it's \"quoted\"", "--", "--settings", "  spaced  "},
		},
		{
			name:     "--settings after -- is a prompt, not a collision",
			launch:   Launch{Dir: "/h/a", AddDirs: []string{"/h/b"}, UserArgs: []string{"--", "--settings"}},
			wantArgv: []string{"claude", "--settings", `{"permissions":{"additionalDirectories":["/h/b"]}}`, "--", "--settings"},
		},
		{
			name:   "deny_access only",
			launch: Launch{Dir: "/h", DenyAccess: []string{"/nonexistent-agent-test/a"}, UserArgs: []string{"--resume", "abc"}},
			wantArgv: []string{"claude", "--settings", `{"permissions":{"deny":["Read(//nonexistent-agent-test/a)","Read(//nonexistent-agent-test/a/**)",` +
				`"Edit(//nonexistent-agent-test/a)","Edit(//nonexistent-agent-test/a/**)"]},` +
				`"sandbox":{"enabled":true,"filesystem":{"denyRead":["/nonexistent-agent-test/a"],"denyWrite":["/nonexistent-agent-test/a"]}}}`,
				"--resume", "abc"},
		},
		{
			name: "attach and deny_access",
			launch: Launch{Dir: "/h/a", AddDirs: []string{"/h/b"},
				DenyAccess: []string{"/nonexistent-agent-test/a", "/nonexistent-agent-test/b c"}},
			wantArgv: []string{"claude", "--settings", `{"permissions":{"additionalDirectories":["/h/b"],` +
				`"deny":["Read(//nonexistent-agent-test/a)","Read(//nonexistent-agent-test/a/**)",` +
				`"Edit(//nonexistent-agent-test/a)","Edit(//nonexistent-agent-test/a/**)",` +
				`"Read(//nonexistent-agent-test/b c)","Read(//nonexistent-agent-test/b c/**)",` +
				`"Edit(//nonexistent-agent-test/b c)","Edit(//nonexistent-agent-test/b c/**)"]},` +
				`"sandbox":{"enabled":true,"filesystem":` +
				`{"denyRead":["/nonexistent-agent-test/a","/nonexistent-agent-test/b c"],` +
				`"denyWrite":["/nonexistent-agent-test/a","/nonexistent-agent-test/b c"]}}}`},
		},
		{
			name:     "no --settings when nothing to pass",
			launch:   Launch{Dir: "/h", UserArgs: []string{"--settings", "mine.json"}},
			wantArgv: []string{"claude", "--settings", "mine.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.launch.BaseEnv = base
			argv, _, err := Claude{}.Command(tt.launch)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(argv, tt.wantArgv) {
				t.Errorf("argv =\n  %q\nwant\n  %q", argv, tt.wantArgv)
			}
		})
	}
}

func TestClaudeCommandEnv(t *testing.T) {
	base := []string{"HOME=/h", "PWD=/old", "CLAUDE_CONFIG_DIR=/parent", "KEEP=1"}
	_, env, err := Claude{}.Command(Launch{
		Dir:     "/h/a",
		BaseEnv: base,
		Env:     map[string]string{"CLAUDE_CONFIG_DIR": "/h/.claude-alt", "AGENT_SPACE": "a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"HOME=/h", "PWD=/h/a", "CLAUDE_CONFIG_DIR=/h/.claude-alt", "KEEP=1", "AGENT_SPACE=a"}
	if !slices.Equal(env, want) {
		t.Errorf("env = %q, want %q", env, want)
	}
}

func TestClaudeSettingsCollision(t *testing.T) {
	launches := map[string]Launch{
		"attach only": {AddDirs: []string{"/h/b"}},
		"deny only":   {DenyAccess: []string{"/h/.ssh"}},
		"both":        {AddDirs: []string{"/h/b"}, DenyAccess: []string{"/h/.ssh"}},
	}
	for name, l := range launches {
		for _, args := range [][]string{{"--settings", "x.json"}, {"--settings={}"}, {"-p", "x", "--settings", "x.json"}} {
			l.Dir, l.UserArgs = "/h/a", args
			_, _, err := Claude{}.Command(l)
			if !errors.Is(err, ErrSettingsCollision) {
				t.Errorf("%s, args %q: err = %v, want ErrSettingsCollision", name, args, err)
			}
		}
	}
}

func TestRegistry(t *testing.T) {
	a, ok := Lookup("claude")
	if !ok || a.Binary() != "claude" {
		t.Fatalf("Lookup(claude) = %v, %v", a, ok)
	}
	if !Known("codex") || Known("gemini") {
		t.Errorf("Known(codex) = %v, Known(gemini) = %v, want true and false", Known("codex"), Known("gemini"))
	}
}

func TestSettingsJSONDoesNotEscapeHTML(t *testing.T) {
	s, err := claudeSettings([]string{"/h/a&b<c>"}, []string{"/h/d&e"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/h/a&b<c>", "Read(//h/d&e)"} {
		if !strings.Contains(s, want) {
			t.Errorf("settings = %s, want %q verbatim", s, want)
		}
	}
}

func TestClaudeSettingsSymlinkedDenyPath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	other := filepath.Join(root, "other")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	rules := func(p string) string {
		return `"Read(/` + p + `)","Read(/` + p + `/**)","Edit(/` + p + `)","Edit(/` + p + `/**)"`
	}
	tests := []struct {
		name string
		deny []string
		want string
	}{
		{
			name: "link then its target",
			deny: []string{link},
			want: `{"permissions":{"deny":[` + rules(link) + `,` + rules(real) + `]},` +
				`"sandbox":{"enabled":true,"filesystem":{"denyRead":["` + link + `","` + real + `"],"denyWrite":["` + link + `","` + real + `"]}}}`,
		},
		{
			name: "target already listed is not repeated",
			deny: []string{real, link},
			want: `{"permissions":{"deny":[` + rules(real) + `,` + rules(link) + `]},` +
				`"sandbox":{"enabled":true,"filesystem":{"denyRead":["` + real + `","` + link + `"],"denyWrite":["` + real + `","` + link + `"]}}}`,
		},
		{
			name: "link first keeps order across entries",
			deny: []string{link, other},
			want: `{"permissions":{"deny":[` + rules(link) + `,` + rules(real) + `,` + rules(other) + `]},` +
				`"sandbox":{"enabled":true,"filesystem":{"denyRead":["` + link + `","` + real + `","` + other + `"],"denyWrite":["` + link + `","` + real + `","` + other + `"]}}}`,
		},
		{
			name: "a missing path is passed as written",
			deny: []string{other},
			want: `{"permissions":{"deny":[` + rules(other) + `]},` +
				`"sandbox":{"enabled":true,"filesystem":{"denyRead":["` + other + `"],"denyWrite":["` + other + `"]}}}`,
		},
		{
			name: "a dangling link is passed as written",
			deny: []string{filepath.Join(root, "dangling")},
			want: `{"permissions":{"deny":[` + rules(filepath.Join(root, "dangling")) + `]},` +
				`"sandbox":{"enabled":true,"filesystem":{"denyRead":["` + filepath.Join(root, "dangling") + `"],"denyWrite":["` + filepath.Join(root, "dangling") + `"]}}}`,
		},
	}
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := claudeSettings(nil, tt.deny)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("settings =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

func TestClaudeSettingsNoSandboxWithoutDeny(t *testing.T) {
	got, err := claudeSettings([]string{"/h/b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"permissions":{"additionalDirectories":["/h/b"]}}`; got != want {
		t.Errorf("settings = %s, want %s", got, want)
	}
}
