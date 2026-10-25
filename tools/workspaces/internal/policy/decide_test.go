package policy

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"workspaces/internal/config"
)

const baseSpaces = `{
 "allow_launch": ["~/Source"],
 "deny_launch": ["~/Source/private/*"],
 "deny_access": ["~/Source/Projects/_archive", "~/.ssh"],
 "spaces": {"a": {"root": "~/Source/Projects/a", "attach": ["~/Source/Projects/b"], "deny_access": ["~/Source/Projects/a/secrets"]}}
}`

// fixtureHome returns a resolved temporary HOME holding the directories and
// symlinks the policy rows launch from.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		"Source/private/drafts/archive", "Source/private/.hidden",
		"Source/Projects/a/sub", "Source/Projects/a/secrets", "Source/Projects/a/inner",
		"Source/Projects/b", "Source/Projects/_archive/old", "SourceX", "outside",
	} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		"drafts":                        "Source/private/drafts",
		"Source/Projects/out":           "outside",
		"src":                           "Source",
		"Source/private/drafts/link":    "Source/Projects/a",
		"Source/Projects/arch":          "Source/Projects/_archive/old",
		"Source/Projects/_archive/live": "Source/Projects/b",
	} {
		if err := os.Symlink(filepath.Join(home, target), filepath.Join(home, link)); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func loadSpaces(t *testing.T, home, body string) *config.Spaces {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spaces.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.LoadSpaces(path, home)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestDecidePolicyRows(t *testing.T) {
	home := fixtureHome(t)
	tests := []struct {
		row      int
		name     string
		spaces   string // empty means baseSpaces
		space    string // "." or a space name
		from     string // logical cwd relative to home; ignored for a named space
		allow    bool
		reason   string
		rule     string
		wantSp   string
		wantDirs []string
	}{
		{1, "source root", "", ".", "Source", true, ReasonAllowLaunch, "~/Source", "", nil},
		{2, "inside space a", "", ".", "Source/Projects/a/sub", true, ReasonAllowLaunch, "~/Source", "a", []string{"Source/Projects/b"}},
		{3, "private itself", "", ".", "Source/private", true, ReasonAllowLaunch, "~/Source", "", nil},
		{4, "private child", "", ".", "Source/private/drafts", false, ReasonDenyLaunch, "~/Source/private/*", "", nil},
		{5, "private grandchild", "", ".", "Source/private/drafts/archive", false, ReasonDenyLaunch, "~/Source/private/*", "", nil},
		{6, "star matches leading dot", "", ".", "Source/private/.hidden", false, ReasonDenyLaunch, "~/Source/private/*", "", nil},
		{7, "global deny_access", "", ".", "Source/Projects/_archive/old", false, ReasonDenyAccess, "~/Source/Projects/_archive", "", nil},
		{8, "space deny_access", "", ".", "Source/Projects/a/secrets", false, ReasonDenyAccess, "~/Source/Projects/a/secrets", "a", nil},
		{9, "home", "", ".", "", false, ReasonNotAllowed, "", "", nil},
		{10, "segment boundary", "", ".", "SourceX", false, ReasonNotAllowed, "", "", nil},
		{11, "deny through physical path", "", ".", "drafts", false, ReasonDenyLaunch, "~/Source/private/*", "", nil},
		{12, "physical path outside", "", ".", "Source/Projects/out", false, ReasonNotAllowed, "", "", nil},
		{13, "logical symlink into space", "", ".", "src/Projects/a", true, ReasonAllowLaunch, "~/Source", "a", nil},
		{14, "allow entry resolved through symlink",
			`{"allow_launch":["~/src"],"spaces":{}}`, ".", "Source/Projects/a", true, ReasonAllowLaunch, "~/src", "", nil},
		{15, "narrower allow does not reopen deny",
			`{"allow_launch":["~/Source","~/Source/private/drafts"],"deny_launch":["~/Source/private/*"]}`,
			".", "Source/private/drafts", false, ReasonDenyLaunch, "~/Source/private/*", "", nil},
		{16, "deny checks logical path", "", ".", "Source/private/drafts/link", false, ReasonDenyLaunch, "~/Source/private/*", "a", nil},
		{17, "named space from elsewhere", "", "a", "outside", true, ReasonAllowLaunch, "~/Source", "a", []string{"Source/Projects/b"}},
		{18, "attach under deny_access",
			`{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/_archive"],
			  "spaces":{"arch":{"root":"~/Source/Projects/b","attach":["~/Source/Projects/_archive/old"]}}}`,
			"arch", "outside", false, ReasonAttachDenied, "~/Source/Projects/_archive", "arch", nil},
		{19, "named space root under deny_launch",
			`{"allow_launch":["~/Source"],"deny_launch":["~/Source/private/*"],
			  "spaces":{"drafts":{"root":"~/Source/private/drafts"}}}`,
			"drafts", "outside", false, ReasonDenyLaunch, "~/Source/private/*", "drafts", nil},
		{20, "longest root wins",
			`{"allow_launch":["~/Source"],"spaces":{"a":{"root":"~/Source/Projects/a"},"inner":{"root":"~/Source/Projects/a/inner"}}}`,
			".", "Source/Projects/a/inner", true, ReasonAllowLaunch, "~/Source", "inner", nil},
		{21, "symlink in allowed tree into deny_access", "", ".", "Source/Projects/arch", false, ReasonDenyAccess, "~/Source/Projects/_archive", "", nil},
		{22, "deny_access logical path, allowed target", "", ".", "Source/Projects/_archive/live", false, ReasonDenyAccess, "~/Source/Projects/_archive", "", nil},
		{23, "named space root symlinked into deny_access",
			`{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/_archive"],"spaces":{"z":{"root":"~/Source/Projects/arch"}}}`,
			"z", "outside", false, ReasonDenyAccess, "~/Source/Projects/_archive", "z", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.spaces
			if body == "" {
				body = baseSpaces
			}
			cfg := loadSpaces(t, home, body)

			var sp *config.Space
			var cwd Cwd
			if tt.space == "." {
				var err error
				if cwd, err = ResolveCwd(filepath.Join(home, tt.from)); err != nil {
					t.Fatal(err)
				}
				sp = SelectSpace(cfg, cwd.Physical)
			} else {
				var ok bool
				if sp, ok = cfg.Space(tt.space); !ok {
					t.Fatalf("row %d: no space %q", tt.row, tt.space)
				}
				cwd = Cwd{Logical: sp.Root.Path, Physical: sp.Root.Path}
			}

			got := Decide(cwd, cfg, sp)
			want := Decision{Allow: tt.allow, Reason: tt.reason, Rule: tt.rule}
			if got != want {
				t.Errorf("row %d: Decide = %+v, want %+v", tt.row, got, want)
			}
			gotSp := ""
			if sp != nil {
				gotSp = sp.Name
			}
			if gotSp != tt.wantSp {
				t.Errorf("row %d: space = %q, want %q", tt.row, gotSp, tt.wantSp)
			}
			if tt.wantDirs != nil {
				var dirs []string
				for _, e := range sp.Attach {
					dirs = append(dirs, e.Path)
				}
				var want []string
				for _, d := range tt.wantDirs {
					want = append(want, filepath.Join(home, d))
				}
				if !slices.Equal(dirs, want) {
					t.Errorf("row %d: add_dirs = %v, want %v", tt.row, dirs, want)
				}
			}
		})
	}
}

func TestResolveCwdKeepsLogicalAndPhysical(t *testing.T) {
	home := fixtureHome(t)
	cwd, err := ResolveCwd(filepath.Join(home, "drafts") + "/")
	if err != nil {
		t.Fatal(err)
	}
	want := Cwd{Logical: filepath.Join(home, "drafts"), Physical: filepath.Join(home, "Source/private/drafts")}
	if cwd != want {
		t.Errorf("ResolveCwd = %+v, want %+v", cwd, want)
	}
}

func TestDenyAccessMatch(t *testing.T) {
	home := fixtureHome(t)
	cfg := loadSpaces(t, home, baseSpaces)
	a := cfg.Spaces["a"]
	tests := []struct {
		name  string
		space *config.Space
		path  string
		rule  string
	}{
		{"global entry", nil, "Source/Projects/_archive/old", "~/Source/Projects/_archive"},
		{"global entry with a space", a, "Source/Projects/_archive/old", "~/Source/Projects/_archive"},
		{"the space's own entry", a, "Source/Projects/a/secrets", "~/Source/Projects/a/secrets"},
		{"space entry without the space", nil, "Source/Projects/a/secrets", ""},
		{"deny_launch is not deny_access", a, "Source/private/drafts", ""},
		{"outside allow_launch", a, "outside", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := DenyAccessMatch(cfg, tt.space, filepath.Join(home, tt.path))
			if ok != (tt.rule != "") || e.Written != tt.rule {
				t.Errorf("DenyAccessMatch = %q, %v; want %q", e.Written, ok, tt.rule)
			}
		})
	}
}

func TestDenyAccessMatchPrefersGlobalEntry(t *testing.T) {
	home := fixtureHome(t)
	cfg := loadSpaces(t, home, `{"allow_launch":["~/Source"],"deny_access":["~/Source/Projects/a"],
	 "spaces":{"a":{"root":"~/Source/Projects/a","deny_access":["~/Source/Projects/a/secrets"]}}}`)
	e, ok := DenyAccessMatch(cfg, cfg.Spaces["a"], filepath.Join(home, "Source/Projects/a/secrets"))
	if !ok || e.Written != "~/Source/Projects/a" {
		t.Errorf("DenyAccessMatch = %q, %v; want the global entry", e.Written, ok)
	}
}

// A global list with spare capacity must not take the space's entries in
// place, or the next call for another space would see them.
func TestDenyAccessDoesNotWriteIntoGlobalList(t *testing.T) {
	home := fixtureHome(t)
	cfg := loadSpaces(t, home, baseSpaces)
	global := make([]config.Entry, len(cfg.DenyAccess), len(cfg.DenyAccess)+4)
	copy(global, cfg.DenyAccess)
	cfg.DenyAccess = global
	a := cfg.Spaces["a"]
	DenyAccessMatch(cfg, a, "/nowhere")
	Decide(Cwd{Logical: a.Root.Path, Physical: a.Root.Path}, cfg, a)
	if spare := global[len(global):cap(global)]; spare[0] != (config.Entry{}) {
		t.Errorf("space entries written into the global list's spare capacity: %+v", spare[0])
	}
}
