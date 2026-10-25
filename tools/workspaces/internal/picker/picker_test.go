package picker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeGum puts a gum on PATH that exits with code.
func fakeGum(t *testing.T, code string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nexit " + code + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gum"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestConfirm(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want bool
	}{
		{"yes", "0", true},
		{"no", "1", false},
		{"interrupted", "130", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeGum(t, tc.code)
			got, err := Confirm("Remove?")
			if err != nil || got != tc.want {
				t.Errorf("Confirm = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestConfirmWithoutGum(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Confirm("Remove?"); !errors.Is(err, ErrNoConfirm) {
		t.Errorf("err = %v, want ErrNoConfirm", err)
	}
}

func TestConfirmGumNotRunnable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gum"), []byte("#!/nonexistent/interpreter\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := Confirm("Remove?")
	if err == nil || got {
		t.Errorf("Confirm = %v, %v; want false and an error", got, err)
	}
}

// fakePicker puts an fzf on PATH that records its arguments to args and prints
// output, then exits with code.
func fakePicker(t *testing.T, name, output, code string) (argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	script := "#!/bin/sh\necho \"$*\" > " + argsFile + "\ncat > /dev/null\nprintf '" + output + "'\nexit " + code + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return argsFile
}

func TestPickMultiFzf(t *testing.T) {
	args := fakePicker(t, "fzf", "one\\ntwo\\n", "0")
	got, err := PickMulti("sessions", []string{"one", "two", "three"})
	if err != nil || len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("PickMulti = %q, %v", got, err)
	}
	b, _ := os.ReadFile(args)
	if want := "--height 40% --reverse --multi --prompt sessions> \n"; string(b) != want {
		t.Errorf("fzf args = %q, want %q", b, want)
	}
}

func TestPickSingleStaysSingle(t *testing.T) {
	args := fakePicker(t, "fzf", "one\\n", "0")
	got, err := Pick("space", []string{"one", "two"})
	if err != nil || got != "one" {
		t.Fatalf("Pick = %q, %v", got, err)
	}
	b, _ := os.ReadFile(args)
	if want := "--height 40% --reverse --no-multi --prompt space> \n"; string(b) != want {
		t.Errorf("fzf args = %q, want %q", b, want)
	}
}

func TestPickMultiGumUsesNoLimit(t *testing.T) {
	args := fakePicker(t, "gum", "a\\n", "0")
	if _, err := PickMulti("sessions", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(args)
	if want := "filter --placeholder sessions --no-limit\n"; string(b) != want {
		t.Errorf("gum args = %q, want %q", b, want)
	}
}

func TestPickMultiCancelledAndEmpty(t *testing.T) {
	for _, tt := range []struct{ name, output, code string }{
		{"esc", "", "130"},
		{"nothing chosen", "", "0"},
		{"blank lines only", "\\n\\n", "0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fakePicker(t, "fzf", tt.output, tt.code)
			if got, err := PickMulti("sessions", []string{"a"}); !errors.Is(err, ErrCancelled) || got != nil {
				t.Errorf("PickMulti = %q, %v; want ErrCancelled", got, err)
			}
		})
	}
}

func TestPickMultiNoPicker(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := PickMulti("sessions", []string{"a"}); !errors.Is(err, ErrNoPicker) {
		t.Errorf("err = %v, want ErrNoPicker", err)
	}
}
