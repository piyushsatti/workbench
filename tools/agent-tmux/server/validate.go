package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxNameLen = 40
	maxNoteLen = 200
	maxDirLen  = 1024
)

// patText mirrors checkText for the JSON schemas: printable ASCII without '#' or ';', first character not '-'.
const patText = `^[\x20-\x22\x24-\x2c\x2e-\x3a\x3c-\x7e][\x20-\x22\x24-\x3a\x3c-\x7e]*$`

const patDir = `^/[A-Za-z0-9._/-]*$`

var reDirChars = regexp.MustCompile(patDir)

func badInput(msg string) error { return &toolErr{"bad_input", msg} }

// checkText enforces the rules every agent-supplied string obeys before it reaches tmux.
// '#' is rejected because tmux format-expands names, titles and -c paths, and can run #() there;
// ';' because an argv element ending in ';' splits into a second tmux command, even after '--'.
func checkText(field, v string, max int) error {
	if v == "" {
		return badInput(field + " must not be empty")
	}
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c < 0x20 || c > 0x7e:
			return badInput(field + " may contain only printable ASCII (0x20-0x7E)")
		case c == '#':
			return badInput(field + " must not contain '#' (tmux expands it as a format)")
		case c == ';':
			return badInput(field + " must not contain ';' (tmux can read it as a command separator)")
		}
	}
	if v[0] == '-' {
		return badInput(field + " must not start with '-'")
	}
	if len(v) > max {
		return badInput(fmt.Sprintf("%s is longer than %d characters", field, max))
	}
	return nil
}

func under(p, root string) bool { return p == root || strings.HasPrefix(p, root+"/") }

// checkDir returns the canonical form of dir, which must be an existing directory under userHome
// spelled only with [A-Za-z0-9._/-], before and after symlinks are resolved.
func checkDir(dir, userHome string) (string, error) {
	if userHome == "" {
		return "", &toolErr{"refused", "user home directory unknown, so no start directory can be checked"}
	}
	if len(dir) > maxDirLen || !reDirChars.MatchString(dir) {
		return "", badInput("dir must be an absolute path of at most 1024 characters from A-Z a-z 0-9 . _ / -")
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == ".." {
			return "", badInput("dir must not contain a '..' segment")
		}
	}
	root, err := filepath.EvalSymlinks(userHome)
	if err != nil {
		return "", &toolErr{"refused", "user home directory cannot be resolved"}
	}
	clean := filepath.Clean(dir)
	if !under(clean, userHome) && !under(clean, root) {
		return "", &toolErr{"out_of_scope", "dir must be inside the user's home directory"}
	}
	fi, err := os.Stat(clean)
	if err != nil {
		return "", badInput("dir does not exist or cannot be read")
	}
	if !fi.IsDir() {
		return "", badInput("dir is not a directory")
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", badInput("dir cannot be resolved")
	}
	if !reDirChars.MatchString(real) {
		return "", badInput("dir resolves to a path with characters outside A-Z a-z 0-9 . _ / -")
	}
	if !under(real, root) {
		return "", &toolErr{"out_of_scope", "dir resolves outside the user's home directory"}
	}
	return real, nil
}
