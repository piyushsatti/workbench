package harness

import (
	"slices"
	"testing"
)

func TestCodexMode(t *testing.T) {
	tests := []struct {
		args []string
		want Mode
	}{
		{nil, ModeSession},
		{[]string{"fix it"}, ModeSession},
		{[]string{"resume", "--last"}, ModeSession},
		{[]string{"fork"}, ModeSession},
		{[]string{"agents"}, ModeSession},
		{[]string{"cloud"}, ModeSession},
		{[]string{"some-future-subcommand"}, ModeSession},
		{[]string{"-m", "o3", "fix"}, ModeSession},
		{[]string{"exec", "fix"}, ModePrint},
		{[]string{"e", "fix"}, ModePrint},
		{[]string{"review"}, ModePrint},
		{[]string{"exec", "resume", "x"}, ModePrint},
		{[]string{"login"}, ModeUtility},
		{[]string{"logout"}, ModeUtility},
		{[]string{"mcp", "list"}, ModeUtility},
		{[]string{"plugin"}, ModeUtility},
		{[]string{"completion", "bash"}, ModeUtility},
		{[]string{"update"}, ModeUtility},
		{[]string{"doctor"}, ModeUtility},
		{[]string{"sandbox", "ls"}, ModeUtility},
		{[]string{"debug"}, ModeUtility},
		{[]string{"apply"}, ModeUtility},
		{[]string{"a"}, ModeUtility},
		{[]string{"features"}, ModeUtility},
		{[]string{"help"}, ModeUtility},
		{[]string{"archive", "x"}, ModeUtility},
		{[]string{"delete", "x"}, ModeUtility},
		{[]string{"unarchive", "x"}, ModeUtility},
		{[]string{"queue", "x"}, ModeUtility},
		{[]string{"migrate-rollouts"}, ModeUtility},
		{[]string{"app-server"}, ModeUtility},
		{[]string{"remote-control"}, ModeUtility},
		{[]string{"mcp-server"}, ModeUtility},
		{[]string{"exec-server"}, ModeUtility},
		{[]string{"-V"}, ModeUtility},
		{[]string{"--version"}, ModeUtility},
		{[]string{"-h"}, ModeUtility},
		{[]string{"--help"}, ModeUtility},
		{[]string{"exec", "--help"}, ModeUtility},
		{[]string{"resume", "-h"}, ModeUtility},
		{[]string{"--", "-h"}, ModeSession},
		{[]string{"--", "login"}, ModeSession},
		{[]string{"exec", "--", "-V"}, ModePrint},
		// Claude's words and flags mean nothing to Codex.
		{[]string{"-p", "x"}, ModeSession},
		{[]string{"--bg"}, ModeSession},
		{[]string{"auth"}, ModeSession},
	}
	for _, tt := range tests {
		if got := (Codex{}).Mode(tt.args); got != tt.want {
			t.Errorf("Mode(%q) = %s, want %s", tt.args, got, tt.want)
		}
	}
}

func TestCodexCommand(t *testing.T) {
	base := []string{"HOME=/h", "CODEX_HOME=/old", "PWD=/elsewhere"}
	tests := []struct {
		name string
		l    Launch
		argv []string
	}{
		{"no attach", Launch{UserArgs: []string{"fix it"}}, []string{"codex", "fix it"}},
		{"one attach", Launch{AddDirs: []string{"/h/b"}, UserArgs: []string{"fix it"}}, []string{"codex", "--add-dir", "/h/b", "fix it"}},
		{"one flag per path, before user args", Launch{AddDirs: []string{"/h/b", "/h/c d"}, UserArgs: []string{"-m", "o3", "x"}},
			[]string{"codex", "--add-dir", "/h/b", "--add-dir", "/h/c d", "-m", "o3", "x"}},
		{"attach with no user args", Launch{AddDirs: []string{"/h/b"}}, []string{"codex", "--add-dir", "/h/b"}},
		{"print mode keeps them", Launch{AddDirs: []string{"/h/b"}, UserArgs: []string{"exec", "x"}}, []string{"codex", "--add-dir", "/h/b", "exec", "x"}},
		{"utility gets none", Launch{AddDirs: []string{"/h/b"}, UserArgs: []string{"login"}}, []string{"codex", "login"}},
		{"help gets none", Launch{AddDirs: []string{"/h/b"}, UserArgs: []string{"--help"}}, []string{"codex", "--help"}},
		{"deny paths add nothing", Launch{DenyAccess: []string{"/h/.ssh"}, UserArgs: []string{"x"}}, []string{"codex", "x"}},
		{"double dash passes through", Launch{AddDirs: []string{"/h/b"}, UserArgs: []string{"--", "--add-dir"}}, []string{"codex", "--add-dir", "/h/b", "--", "--add-dir"}},
		{"user's own --add-dir is kept, no collision", Launch{AddDirs: []string{"/h/b"}, UserArgs: []string{"--add-dir", "/x"}},
			[]string{"codex", "--add-dir", "/h/b", "--add-dir", "/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.l.Dir = "/h/a"
			tt.l.BaseEnv = base
			tt.l.Env = map[string]string{"CODEX_HOME": "/h/.codex-alt", "AGENT_SPACE": "a"}
			argv, env, err := Codex{}.Command(tt.l)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(argv, tt.argv) {
				t.Errorf("argv = %q, want %q", argv, tt.argv)
			}
			want := []string{"HOME=/h", "CODEX_HOME=/h/.codex-alt", "PWD=/h/a", "AGENT_SPACE=a"}
			if !slices.Equal(env, want) {
				t.Errorf("env = %q, want %q", env, want)
			}
		})
	}
}

func TestCodexUnenforcedDeny(t *testing.T) {
	var d DenyLimited = Codex{}
	for _, tt := range []struct {
		paths []string
		want  int
	}{{nil, 0}, {[]string{"/a"}, 1}, {[]string{"/a", "/b", "/c"}, 3}} {
		if got := d.UnenforcedDeny(tt.paths); got != tt.want {
			t.Errorf("UnenforcedDeny(%q) = %d, want %d", tt.paths, got, tt.want)
		}
	}
	if _, ok := Adapter(Claude{}).(DenyLimited); ok {
		t.Error("Claude enforces deny_access and must not report unenforced paths")
	}
}

func TestCodexBinaryAndRegistry(t *testing.T) {
	a, ok := Lookup("codex")
	if !ok || a.Binary() != "codex" {
		t.Fatalf("Lookup(codex) = %v, %v", a, ok)
	}
}
