package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const codexHarnesses = `{"default":"claude","harnesses":{
 "claude":{"adapter":"claude"},
 "codex":{"adapter":"codex"},
 "codex-alt":{"adapter":"codex","env":{"CODEX_HOME":"~/.codex-alt"}}}}`

func codexFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.writeConfig("harness.json", codexHarnesses)
	f.lookPath["codex"] = "/usr/bin/codex"
	f.inTmux("999")
	return f
}

func TestCodexRun(t *testing.T) {
	tests := []struct {
		name         string
		spaces       string
		harness      string
		args         []string
		wantArgv     func(home string) []string
		wantWarn     string
		wantField    int
		wantCodexEnv string
	}{
		{"attach path and three deny paths", "", "codex", []string{"fix it"},
			func(h string) []string { return []string{"codex", "--add-dir", h + "/Source/Projects/b", "fix it"} },
			"workspaces: warning: the codex adapter cannot enforce deny_access: 3 paths are not denied inside this session", 3, ""},
		{"profile sets CODEX_HOME", "", "codex-alt", nil,
			func(h string) []string { return []string{"codex", "--add-dir", h + "/Source/Projects/b"} },
			"3 paths are not denied", 3, "~/.codex-alt"},
		{"one deny path reads singular", `{"allow_launch":["~/Source"],"deny_access":["~/.ssh"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`, "codex", nil,
			func(string) []string { return []string{"codex"} },
			"1 path is not denied", 1, ""},
		{"no deny paths means no warning", `{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a"}}}`, "codex", nil,
			func(string) []string { return []string{"codex"} }, "", 0, ""},
		{"utility command is not warned or given add-dir", "", "codex", []string{"login"},
			func(string) []string { return []string{"codex", "login"} }, "", 0, ""},
		{"version flag is utility", "", "codex", []string{"--version"},
			func(string) []string { return []string{"codex", "--version"} }, "", 0, ""},
		{"exec is print mode and still warned", "", "codex", []string{"exec", "hi"},
			func(h string) []string {
				return []string{"codex", "--add-dir", h + "/Source/Projects/b", "exec", "hi"}
			}, "3 paths are not denied", 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := codexFixture(t)
			if tt.spaces != "" {
				f.writeConfig("spaces.json", tt.spaces)
			}
			args := append([]string{"run", "a", tt.harness}, tt.args...)
			if code := f.run(args...); code != exitOK {
				t.Fatalf("exit %d; stderr %s", code, f.stderr.String())
			}
			if len(f.execs) != 1 {
				t.Fatalf("execs = %d", len(f.execs))
			}
			if got, want := f.execs[0].argv, tt.wantArgv(f.home); !slices.Equal(got, want) {
				t.Errorf("argv = %q, want %q", got, want)
			}
			stderr := f.stderr.String()
			if tt.wantWarn == "" && strings.Contains(stderr, "warning") {
				t.Errorf("unexpected warning: %s", stderr)
			}
			if tt.wantWarn != "" && !strings.Contains(stderr, tt.wantWarn) {
				t.Errorf("stderr %q lacks %q", stderr, tt.wantWarn)
			}
			rec := f.onlyRecord()
			if rec.UnenforcedDeny != tt.wantField {
				t.Errorf("unenforced_deny = %d, want %d", rec.UnenforcedDeny, tt.wantField)
			}
			raw := readFile(t, filepath.Join(f.home, ".local/state/workspaces/launches.jsonl"))
			if has := strings.Contains(raw, "unenforced_deny"); has != (tt.wantField > 0) {
				t.Errorf("unenforced_deny in log = %v, want %v: %s", has, tt.wantField > 0, raw)
			}
			home, set := envValue(f.execs[0].env, "CODEX_HOME")
			if tt.wantCodexEnv == "" && set {
				t.Errorf("CODEX_HOME = %q, want unset", home)
			}
			if tt.wantCodexEnv != "" && home != strings.Replace(tt.wantCodexEnv, "~", f.home, 1) {
				t.Errorf("CODEX_HOME = %q", home)
			}
		})
	}
}

func TestCodexWarningPrintsBeforeExec(t *testing.T) {
	f := codexFixture(t)
	f.run("run", "a", "codex")
	if len(f.chdirs) != 1 || !strings.Contains(f.stderr.String(), "warning") {
		t.Fatalf("chdirs %v, stderr %q", f.chdirs, f.stderr.String())
	}
}

func TestClaudeHarnessHasNoUnenforcedField(t *testing.T) {
	f := codexFixture(t)
	f.run("run", "a", "claude")
	if f.onlyRecord().UnenforcedDeny != 0 || strings.Contains(f.stderr.String(), "warning") {
		t.Errorf("claude run reported unenforced deny: %s", f.stderr.String())
	}
}

func TestCodexCheck(t *testing.T) {
	f := codexFixture(t)
	if code := f.run("check", "a", "codex"); code != exitOK {
		t.Fatalf("exit %d; %s", code, f.stderr.String())
	}
	out := f.stdout.String()
	if !strings.Contains(out, "0 paths in session (3 not enforced)") {
		t.Errorf("deny row missing:\n%s", out)
	}
	if !strings.Contains(out, "warning       the codex adapter cannot enforce deny_access: 3 paths are not denied") {
		t.Errorf("no warning row:\n%s", out)
	}
	if f.logExists() || len(f.execs) != 0 {
		t.Error("check wrote the log or launched")
	}

	f = codexFixture(t)
	f.run("check", "--json", "a", "codex")
	var rep map[string]any
	if err := json.Unmarshal(f.stdout.Bytes(), &rep); err != nil {
		t.Fatalf("%v\n%s", err, f.stdout.String())
	}
	if rep["unenforced_deny"] != float64(3) {
		t.Errorf("unenforced_deny = %v", rep["unenforced_deny"])
	}

	f = codexFixture(t)
	f.run("check", "--json", "a", "claude")
	if strings.Contains(f.stdout.String(), "unenforced_deny") {
		t.Error("claude check carries unenforced_deny")
	}
}
