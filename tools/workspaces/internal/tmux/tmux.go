package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Runner func(args []string) (string, error)

// MaxArgvBytes stays under tmux's 16 KB cap on one client command, so workspaces
// can name the variables at fault instead of tmux failing vaguely.
const MaxArgvBytes = 12 * 1024

// Run talks to the server named by $TMUX when set, otherwise to the default
// socket.
func Run(args []string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return string(out), fmt.Errorf("tmux %s: %w", args[0], err)
		}
		return string(out), fmt.Errorf("tmux %s: %s", args[0], msg)
	}
	return string(out), nil
}

type Pane struct {
	Socket      string
	SessionID   string
	Session     string
	WindowID    string
	WindowIndex int
	Window      string
	PaneID      string
	PanePID     int
}

// The window name goes last because it is the only field that may contain a
// newline; ParsePane keeps everything after the seventh line as the name.
var paneFields = []string{
	"#{socket_path}", "#{session_id}", "#{session_name}", "#{window_id}",
	"#{window_index}", "#{pane_id}", "#{pane_pid}", "#{window_name}",
}

// PaneQuery chains one display-message per field, each printing on its own
// line, so no separator character is needed.
func PaneQuery(pane string) []string {
	var args []string
	for i, f := range paneFields {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, "display-message", "-p", "-t", escapeArg(pane), f)
	}
	return args
}

func SocketQuery() []string {
	return []string{"display-message", "-p", "#{socket_path}"}
}

func ParsePane(out string) (Pane, error) {
	parts := strings.SplitN(strings.TrimSuffix(out, "\n"), "\n", len(paneFields))
	if len(parts) != len(paneFields) {
		return Pane{}, fmt.Errorf("tmux returned %d of %d pane fields", len(parts), len(paneFields))
	}
	idx, err := strconv.Atoi(parts[4])
	if err != nil {
		return Pane{}, fmt.Errorf("window_index %q: %w", parts[4], err)
	}
	pid, err := strconv.Atoi(parts[6])
	if err != nil {
		return Pane{}, fmt.Errorf("pane_pid %q: %w", parts[6], err)
	}
	return Pane{
		Socket: parts[0], SessionID: parts[1], Session: parts[2], WindowID: parts[3],
		WindowIndex: idx, PaneID: parts[5], PanePID: pid, Window: parts[7],
	}, nil
}

func SessionName(space string) string {
	if space == "" {
		return "main"
	}
	return space
}

func WindowName(space, harness, launchDir string) string {
	if space == "" {
		return filepath.Base(launchDir)
	}
	return harness
}

// The "=" prefix forces an exact session match; without it tmux matches
// prefixes.
func HasSession(session string) []string {
	return []string{"has-session", "-t", "=" + session}
}

func NewSession(session, window, dir string, env, command []string) []string {
	args := []string{"new-session", "-d", "-s", session, "-n", escapeFormat(window), "-c", escapeFormat(dir)}
	return appendCommand(args, env, command)
}

func NewWindow(session, window, dir string, env, command []string) []string {
	args := []string{"new-window", "-t", "=" + session + ":", "-n", escapeFormat(window), "-c", escapeFormat(dir)}
	return appendCommand(args, env, command)
}

func appendCommand(args, env, command []string) []string {
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	args = append(args, "--")
	args = append(args, command...)
	for i := 1; i < len(args); i++ {
		args[i] = escapeArg(args[i])
	}
	return args
}

// tmux expands formats such as #{...} and #(...) in the -n and -c values;
// "##" is a literal "#".
func escapeFormat(s string) string {
	return strings.ReplaceAll(s, "#", "##")
}

// tmux reads an argument ending in ";" as a command separator, wherever it
// sits, and turns a final "\;" into a literal ";". Adding one backslash
// before the final ";" therefore passes any argument through unchanged,
// including one that already ends in "\;".
func escapeArg(s string) string {
	if strings.HasSuffix(s, ";") {
		return s[:len(s)-1] + `\;`
	}
	return s
}

func AttachArgv(session string) []string {
	return []string{"tmux", "attach-session", "-t", "=" + session}
}

func IsDuplicateSession(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate session")
}

var bootstrapDropped = map[string]bool{
	"TMUX": true, "TMUX_PANE": true, "TERM": true, "TERM_PROGRAM": true,
	"TERM_PROGRAM_VERSION": true, "SHLVL": true, "OLDPWD": true, "_": true,
}

// BootstrapEnv is the environment handed to the new pane with -e. Without it
// the harness would get the environment the tmux server captured at start.
func BootstrapEnv(environ []string, logicalCwd string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || bootstrapDropped[k] || k == "PWD" {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PWD="+logicalCwd)
}

type ArgvTooLongError struct {
	Bytes   int
	Largest []string
}

func (e *ArgvTooLongError) Error() string {
	return fmt.Sprintf("bootstrap command is %d bytes, over the %d byte limit; largest variables: %s",
		e.Bytes, MaxArgvBytes, strings.Join(e.Largest, ", "))
}

func CheckArgvSize(args, env []string) error {
	size := len("tmux") + 1
	for _, a := range args {
		size += len(a) + 1
	}
	if size <= MaxArgvBytes {
		return nil
	}
	sorted := append([]string{}, env...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	var largest []string
	for _, kv := range sorted[:min(3, len(sorted))] {
		k, _, _ := strings.Cut(kv, "=")
		largest = append(largest, fmt.Sprintf("%s (%d bytes)", k, len(kv)))
	}
	return &ArgvTooLongError{Bytes: size, Largest: largest}
}

// ListSessions asks the server for its session names.
func ListSessions() []string {
	return []string{"list-sessions", "-F", "#{session_name}"}
}

// IsNoServer reports an error that means no tmux server is running on the
// socket, which tmux words as "no server running" or, when the socket file is
// gone, "error connecting to <socket> (No such file or directory)".
func IsNoServer(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no server running") ||
		(strings.Contains(msg, "error connecting to") && strings.Contains(msg, "No such file or directory"))
}
