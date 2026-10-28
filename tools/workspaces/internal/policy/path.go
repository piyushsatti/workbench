package policy

import (
	"path/filepath"
	"runtime"
	"strings"

	"workspaces/internal/config"
)

type Cwd struct {
	Logical  string
	Physical string
}

// ResolveCwd takes the logical cwd as os.Getwd returns it, which on Unix is
// $PWD when $PWD names the current directory.
func ResolveCwd(logical string) (Cwd, error) {
	l := filepath.Clean(logical)
	p, err := filepath.EvalSymlinks(l)
	if err != nil {
		return Cwd{}, err
	}
	return Cwd{Logical: l, Physical: p}, nil
}

func Matches(p string, e config.Entry) bool {
	if e.Glob {
		return globMatch(p, e.Path)
	}
	return Under(p, e.Path)
}

// Under is the plain-path rule: p is e or sits below it, at a segment boundary.
func Under(p, e string) bool {
	p, e = fold(p), fold(e)
	return e == "/" || p == e || strings.HasPrefix(p, e+"/")
}

// foldCase: macOS file systems ignore letter case, so ~/Source/SECRET is the
// folder a deny on ~/Source/secret names; Linux file systems do not.
var foldCase = runtime.GOOS == "darwin"

func fold(p string) string {
	if foldCase {
		return strings.ToLower(p)
	}
	return p
}

// globMatch tries p and each of its ancestors, so a pattern ending in "/*"
// covers whole subtrees but never the parent directory itself.
func globMatch(p, pattern string) bool {
	p, pattern = fold(p), fold(pattern)
	for a := p; ; a = filepath.Dir(a) {
		if ok, _ := filepath.Match(pattern, a); ok {
			return true
		}
		if a == "/" || a == "." {
			return false
		}
	}
}
