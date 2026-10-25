package picker

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

var (
	ErrCancelled = errors.New("picker cancelled")
	ErrNoPicker  = errors.New("neither fzf nor gum is installed")
	ErrNoConfirm = errors.New("gum is not installed")
)

// Any non-zero exit of fzf or gum, including fzf's 130 on Esc or Ctrl-C, is a
// cancel.
func Pick(prompt string, items []string) (string, error) {
	out, err := pick(prompt, items, false)
	if err != nil {
		return "", err
	}
	return out[0], nil
}

// PickMulti lets the user mark several items (Tab in fzf) and returns the
// chosen lines in the order shown. Cancelling, or choosing nothing, is
// ErrCancelled.
func PickMulti(prompt string, items []string) ([]string, error) {
	return pick(prompt, items, true)
}

func pick(prompt string, items []string, multi bool) ([]string, error) {
	var cmd *exec.Cmd
	if path, err := exec.LookPath("fzf"); err == nil {
		mode := "--no-multi"
		if multi {
			mode = "--multi"
		}
		cmd = exec.Command(path, "--height", "40%", "--reverse", mode, "--prompt", prompt+"> ")
	} else if path, err := exec.LookPath("gum"); err == nil {
		args := []string{"filter", "--placeholder", prompt}
		if multi {
			args = append(args, "--no-limit")
		}
		cmd = exec.Command(path, args...)
	} else {
		return nil, ErrNoPicker
	}
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n") + "\n")
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out

	// Ctrl-C reaches the whole foreground process group; ignoring it here lets
	// workspaces exit 130 cleanly once the picker returns instead of dying with it.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	var choices []string
	for _, line := range strings.Split(out.String(), "\n") {
		if line != "" {
			choices = append(choices, line)
		}
	}
	if len(choices) == 0 {
		return nil, ErrCancelled
	}
	return choices, nil
}

// Confirm asks a yes/no question with gum. Any answer but yes, including
// Ctrl-C, is false.
func Confirm(prompt string) (bool, error) {
	path, err := exec.LookPath("gum")
	if err != nil {
		return false, ErrNoConfirm
	}
	cmd := exec.Command(path, "confirm", prompt)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
