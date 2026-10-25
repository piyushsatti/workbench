package policy

import (
	"testing"

	"workspaces/internal/config"
)

func TestMatches(t *testing.T) {
	tests := []struct {
		path  string
		entry config.Entry
		want  bool
	}{
		{"/h/Source", config.Entry{Path: "/h/Source"}, true},
		{"/h/Source/x/y", config.Entry{Path: "/h/Source"}, true},
		{"/h/SourceX", config.Entry{Path: "/h/Source"}, false},
		{"/h", config.Entry{Path: "/h/Source"}, false},
		{"/anything", config.Entry{Path: "/"}, true},
		{"/h/notes", config.Entry{Path: "/h/notes/*", Glob: true}, false},
		{"/h/notes/a", config.Entry{Path: "/h/notes/*", Glob: true}, true},
		{"/h/notes/a/b/c", config.Entry{Path: "/h/notes/*", Glob: true}, true},
		{"/h/notes/.dot", config.Entry{Path: "/h/notes/*", Glob: true}, true},
		{"/h/notesx/a", config.Entry{Path: "/h/notes/*", Glob: true}, false},
		{"/h/p1/x", config.Entry{Path: "/h/p?", Glob: true}, true},
		{"/h/pa", config.Entry{Path: "/h/p[0-9]", Glob: true}, false},
	}
	for _, tt := range tests {
		if got := Matches(tt.path, tt.entry); got != tt.want {
			t.Errorf("Matches(%q, %q) = %v, want %v", tt.path, tt.entry.Path, got, tt.want)
		}
	}
}
