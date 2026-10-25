//go:build linux || darwin

package picker

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIsTerminalRejectsNonTerminals(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	regular, err := os.Create(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	for name, f := range map[string]*os.File{
		"/dev/null": devNull, "regular file": regular, "pipe read end": r, "pipe write end": w,
	} {
		if isTerminal(f) {
			t.Errorf("isTerminal(%s) = true, want false", name)
		}
	}
}

// The pseudo-terminal master answers the termios ioctl, which proves the test
// above fails for the right reason rather than because the ioctl never works.
func TestIsTerminalAcceptsPseudoTerminal(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	defer ptmx.Close()
	if !isTerminal(ptmx) {
		t.Error("isTerminal(/dev/ptmx) = false, want true")
	}
}

func TestIsTTYNeedsBothEnds(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	defer ptmx.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	tests := []struct {
		name       string
		in, errOut *os.File
		want       bool
	}{
		{"pty, pty", ptmx, ptmx, true},
		{"pty, pipe", ptmx, w, false},
		{"pipe, pty", r, ptmx, false},
		{"pipe, pipe", r, w, false},
	}
	for _, tt := range tests {
		if got := isTTY(tt.in, tt.errOut); got != tt.want {
			t.Errorf("isTTY(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
