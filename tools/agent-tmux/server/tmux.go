package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	tmuxBin     = "/usr/bin/tmux"
	tmuxTimeout = 5 * time.Second
	maxOutput   = 4 << 20
	// sep separates -F fields. It never occurs in IDs or numbers; in free text it breaks the row (see query).
	sep = "\x1f"
)

// socketPath is fixed from the uid, never taken from $TMUX or $TMUX_TMPDIR.
var socketPath = fmt.Sprintf("/tmp/tmux-%d/default", os.Getuid())

// Runner runs one tmux command (argv after the socket flags) and returns stdout.
type Runner interface {
	Run(args []string) (string, error)
}

// ExecRunner is the real Runner: absolute binary, fixed socket, rebuilt env, no shell, cwd "/".
type ExecRunner struct{ Home string }

func (e ExecRunner) Run(args []string) (string, error) {
	if err := checkArgv(args); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tmuxBin, append([]string{"-S", socketPath}, args...)...)
	cmd.Env = []string{"PATH=" + clientPath(args), "HOME=" + e.Home, "LANG=C.UTF-8"}
	cmd.Dir = "/"
	var out, errb capBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.b.String())
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return "", fmt.Errorf("tmux %s failed: %v: %s", args[0], err, msg)
	}
	if out.over {
		return "", errors.New("tmux output too large")
	}
	return out.b.String(), nil
}

// clientPath is the PATH of the tmux client process. tmux gives a pane made by a detached client that client's
// PATH, applied after -e, so a new-window or split-window carrying -e PATH=... (pathEnv) runs with that PATH
// itself. Any other command gets /usr/bin:/bin. tmuxBin is absolute, so the client never searches PATH.
func clientPath(args []string) string {
	if args[0] == "new-window" || args[0] == "split-window" {
		for i := 1; i+1 < len(args); i++ {
			if v, ok := strings.CutPrefix(args[i+1], "PATH="); ok && args[i] == "-e" {
				return v
			}
		}
	}
	return "/usr/bin:/bin"
}

// capBuffer keeps at most maxOutput bytes and records overflow.
type capBuffer struct {
	b    bytes.Buffer
	over bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if c.b.Len()+len(p) > maxOutput {
		c.over = true
		return len(p), nil
	}
	return c.b.Write(p)
}

// checkArgv rejects any element tmux would treat as a command separator.
func checkArgv(args []string) error {
	if len(args) == 0 {
		return errors.New("empty tmux argv")
	}
	for _, a := range args {
		if strings.HasSuffix(a, ";") {
			return fmt.Errorf("refused: tmux argv element ends with ';'")
		}
	}
	return nil
}

// tmux wraps a Runner so every caller, fake or real, goes through checkArgv.
type tmux struct{ r Runner }

func (t tmux) run(args ...string) (string, error) {
	if err := checkArgv(args); err != nil {
		return "", err
	}
	return t.r.Run(args)
}

// query runs a command one of whose arguments is formatOf(fields), and parses its rows strictly. Each row is
// framed by a token drawn for this call. Free text such as a pane's path can hold a newline and sep, and so could
// end its own row early. To forge a whole row it would also need this call's token, which exists only in this
// call's tmux argv.
func (t tmux) query(fields []string, args ...string) ([]map[string]string, error) {
	tok := rand.Text()
	framed := slices.Clone(args)
	i := slices.Index(framed, formatOf(fields))
	if i < 0 {
		return nil, errors.New("tmux query without its format argument")
	}
	framed[i] = tok + sep + framed[i] + sep + tok
	out, err := t.run(framed...)
	if err != nil {
		return nil, err
	}
	return parseRows(out, fields, tok)
}

func formatOf(fields []string) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = "#{" + f + "}"
	}
	return strings.Join(parts, sep)
}

// parseRows accepts only lines that are one whole row: tok, the fields, tok.
func parseRows(out string, fields []string, tok string) ([]map[string]string, error) {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil, nil
	}
	var rows []map[string]string
	for n, line := range strings.Split(out, "\n") {
		cols := strings.Split(line, sep)
		if len(cols) != len(fields)+2 || cols[0] != tok || cols[len(cols)-1] != tok {
			return nil, fmt.Errorf("tmux output line %d is not one whole row of %d fields", n+1, len(fields))
		}
		cols = cols[1 : len(cols)-1]
		m := make(map[string]string, len(fields))
		for i, f := range fields {
			m[f] = cols[i]
		}
		rows = append(rows, m)
	}
	return rows, nil
}

var (
	rePaneID    = regexp.MustCompile(`^%[0-9]{1,9}$`)
	reWindowID  = regexp.MustCompile(`^@[0-9]{1,9}$`)
	reSessionID = regexp.MustCompile(`^\$[0-9]{1,9}$`)
)

func parseFlag(s string) (bool, error) {
	switch s {
	case "0":
		return false, nil
	case "1":
		return true, nil
	}
	return false, fmt.Errorf("bad flag %q from tmux", s)
}

func parseUint(s string) (int, error) {
	if s == "" || len(s) > 9 || strings.TrimLeft(s, "0123456789") != "" {
		return 0, fmt.Errorf("bad number %q from tmux", s)
	}
	return strconv.Atoi(s)
}

func checkID(re *regexp.Regexp, s string) error {
	if !re.MatchString(s) {
		return fmt.Errorf("bad id %q from tmux", s)
	}
	return nil
}
