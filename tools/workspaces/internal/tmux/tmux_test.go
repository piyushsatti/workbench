package tmux

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestPaneQuery(t *testing.T) {
	got := PaneQuery("%30")
	want := []string{
		"display-message", "-p", "-t", "%30", "#{socket_path}", ";",
		"display-message", "-p", "-t", "%30", "#{session_id}", ";",
		"display-message", "-p", "-t", "%30", "#{session_name}", ";",
		"display-message", "-p", "-t", "%30", "#{window_id}", ";",
		"display-message", "-p", "-t", "%30", "#{window_index}", ";",
		"display-message", "-p", "-t", "%30", "#{pane_id}", ";",
		"display-message", "-p", "-t", "%30", "#{pane_pid}", ";",
		"display-message", "-p", "-t", "%30", "#{window_name}",
	}
	if !slices.Equal(got, want) {
		t.Errorf("PaneQuery =\n  %q\nwant\n  %q", got, want)
	}
}

func TestParsePane(t *testing.T) {
	got, err := ParsePane("/tmp/tmux-1000/default\n$5\nmain\n@12\n3\n%30\n4242\nodd\nname\n")
	if err != nil {
		t.Fatal(err)
	}
	want := Pane{Socket: "/tmp/tmux-1000/default", SessionID: "$5", Session: "main", WindowID: "@12",
		WindowIndex: 3, PaneID: "%30", PanePID: 4242, Window: "odd\nname"}
	if got != want {
		t.Errorf("ParsePane = %+v, want %+v", got, want)
	}
	for _, bad := range []string{"", "a\nb\nc", "/s\n$1\nm\n@1\nx\n%1\n1\nw", "/s\n$1\nm\n@1\n1\n%1\nx\nw"} {
		if _, err := ParsePane(bad); err == nil {
			t.Errorf("ParsePane(%q) succeeded, want an error", bad)
		}
	}
}

func TestNames(t *testing.T) {
	if got := SessionName(""); got != "main" {
		t.Errorf("SessionName(none) = %s", got)
	}
	if got := SessionName("webapp"); got != "webapp" {
		t.Errorf("SessionName = %s", got)
	}
	if got := WindowName("", "claude", "/h/Source/proj"); got != "proj" {
		t.Errorf("WindowName(no space) = %s, want the launch dir basename", got)
	}
	if got := WindowName("s", "claude-alt", "/h/x"); got != "claude-alt" {
		t.Errorf("WindowName(space) = %s, want the harness", got)
	}
}

func TestBootstrapCommands(t *testing.T) {
	env := []string{"A=1", "B=two words"}
	cmd := []string{"/bin/workspaces", "run", "s", "claude", "fix it"}
	if got, want := NewSession("s", "claude", "/r", env, cmd), []string{
		"new-session", "-d", "-s", "s", "-n", "claude", "-c", "/r", "-e", "A=1", "-e", "B=two words",
		"--", "/bin/workspaces", "run", "s", "claude", "fix it",
	}; !slices.Equal(got, want) {
		t.Errorf("NewSession =\n  %q\nwant\n  %q", got, want)
	}
	if got, want := NewWindow("s", "claude", "/r", env, cmd), []string{
		"new-window", "-t", "=s:", "-n", "claude", "-c", "/r", "-e", "A=1", "-e", "B=two words",
		"--", "/bin/workspaces", "run", "s", "claude", "fix it",
	}; !slices.Equal(got, want) {
		t.Errorf("NewWindow =\n  %q\nwant\n  %q", got, want)
	}
	if got := HasSession("s"); !slices.Equal(got, []string{"has-session", "-t", "=s"}) {
		t.Errorf("HasSession = %q", got)
	}
	if got := AttachArgv("s"); !slices.Equal(got, []string{"tmux", "attach-session", "-t", "=s"}) {
		t.Errorf("AttachArgv = %q", got)
	}
}

func TestBootstrapEnv(t *testing.T) {
	in := []string{"HOME=/h", "TMUX=/tmp/x,1,0", "TMUX_PANE=%1", "TERM=xterm", "TERM_PROGRAM=x",
		"TERM_PROGRAM_VERSION=1", "SHLVL=2", "OLDPWD=/o", "_=/usr/bin/workspaces", "PWD=/physical", "CLAUDE_X=1", "noequals"}
	got := BootstrapEnv(in, "/logical")
	want := []string{"HOME=/h", "CLAUDE_X=1", "PWD=/logical"}
	if !slices.Equal(got, want) {
		t.Errorf("BootstrapEnv = %q, want %q", got, want)
	}
}

func TestCheckArgvSize(t *testing.T) {
	small := []string{"new-session", "-e", "A=1"}
	if err := CheckArgvSize(small, []string{"A=1"}); err != nil {
		t.Fatalf("small argv: %v", err)
	}
	env := []string{"SMALL=1", "BIG1=" + strings.Repeat("x", 6000), "BIG2=" + strings.Repeat("y", 5000), "BIG3=" + strings.Repeat("z", 4000)}
	args := []string{"new-session"}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	err := CheckArgvSize(args, env)
	var tooLong *ArgvTooLongError
	if !errors.As(err, &tooLong) {
		t.Fatalf("err = %v, want ArgvTooLongError", err)
	}
	if len(tooLong.Largest) != 3 || !strings.HasPrefix(tooLong.Largest[0], "BIG1 ") ||
		!strings.HasPrefix(tooLong.Largest[1], "BIG2 ") || !strings.HasPrefix(tooLong.Largest[2], "BIG3 ") {
		t.Errorf("Largest = %q, want BIG1, BIG2, BIG3", tooLong.Largest)
	}
}

func TestIsDuplicateSession(t *testing.T) {
	if !IsDuplicateSession(errors.New("tmux new-session: duplicate session: s")) {
		t.Error("duplicate session error not recognised")
	}
	if IsDuplicateSession(nil) || IsDuplicateSession(errors.New("no server running")) {
		t.Error("false positive")
	}
}

func TestBootstrapCommandsEscape(t *testing.T) {
	env := []string{"A=x;", "B=#{pane_id}"}
	cmd := []string{"/bin/workspaces", "run", ".", "claude", "fix it;", ";", `ends\;`, "#{no}"}
	tail := []string{"-e", `A=x\;`, "-e", "B=#{pane_id}", "--",
		"/bin/workspaces", "run", ".", "claude", `fix it\;`, `\;`, `ends\\;`, "#{no}"}
	wantNew := append([]string{"new-session", "-d", "-s", "s", "-n", `w##{x}\;`, "-c", `/r/##(id)\;`}, tail...)
	if got := NewSession("s", "w#{x};", "/r/#(id);", env, cmd); !slices.Equal(got, wantNew) {
		t.Errorf("NewSession =\n  %q\nwant\n  %q", got, wantNew)
	}
	wantWin := append([]string{"new-window", "-t", "=s:", "-n", `w##{x}\;`, "-c", `/r/##(id)\;`}, tail...)
	if got := NewWindow("s", "w#{x};", "/r/#(id);", env, cmd); !slices.Equal(got, wantWin) {
		t.Errorf("NewWindow =\n  %q\nwant\n  %q", got, wantWin)
	}
	if got := PaneQuery("%1;"); got[3] != `%1\;` || got[5] != ";" {
		t.Errorf("PaneQuery escaped %q and %q, want the pane escaped and the separator kept", got[3], got[5])
	}
}

func TestEscapeArgRoundTrip(t *testing.T) {
	// unescape mirrors tmux's argument splitter for an argument that ends a command.
	unescape := func(s string) (string, bool) {
		if !strings.HasSuffix(s, ";") {
			return s, true
		}
		s = s[:len(s)-1]
		if strings.HasSuffix(s, `\`) {
			return s[:len(s)-1] + ";", true
		}
		return s, false
	}
	for _, in := range []string{"", "plain", ";", "a;", `a\;`, `a\\;`, ";;", "a;b"} {
		got, literal := unescape(escapeArg(in))
		if !literal || got != in {
			t.Errorf("tmux reads escapeArg(%q) as (%q, literal %v), want (%q, true)", in, got, literal, in)
		}
	}
}

func TestCheckArgvSizeBoundary(t *testing.T) {
	// The size counts "tmux" and every element, each plus one separator byte.
	at := []string{strings.Repeat("x", MaxArgvBytes-len("tmux ")-1)}
	if err := CheckArgvSize(at, nil); err != nil {
		t.Errorf("argv of exactly %d bytes: %v", MaxArgvBytes, err)
	}
	over := []string{strings.Repeat("x", MaxArgvBytes-len("tmux "))}
	var tooLong *ArgvTooLongError
	if err := CheckArgvSize(over, nil); !errors.As(err, &tooLong) || tooLong.Bytes != MaxArgvBytes+1 {
		t.Errorf("argv of %d bytes: err = %v, want ArgvTooLongError with Bytes %d", MaxArgvBytes+1, err, MaxArgvBytes+1)
	}
}

func TestListSessionsArgv(t *testing.T) {
	if got := strings.Join(ListSessions(), " "); got != "list-sessions -F #{session_name}" {
		t.Errorf("argv = %q", got)
	}
}

func TestIsNoServer(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("tmux list-sessions: no server running on /tmp/tmux-1000/default"), true},
		{errors.New("tmux list-sessions: error connecting to /tmp/tmux-1000/default (No such file or directory)"), true},
		{errors.New("tmux list-sessions: error connecting to /tmp/tmux-1000/default (Permission denied)"), false},
		{errors.New("tmux list-sessions: exit status 1"), false},
	} {
		if got := IsNoServer(tt.err); got != tt.want {
			t.Errorf("IsNoServer(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
