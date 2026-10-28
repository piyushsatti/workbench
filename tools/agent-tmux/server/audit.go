package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Audit appends one line per tool call. Only validated IDs and fixed codes are written raw; free text is len+sha256.
type Audit struct {
	path string
	pid  int
	mu   sync.Mutex
}

func NewAudit(path string) *Audit {
	if path == "" {
		return nil
	}
	return &Audit{path: path, pid: os.Getpid()}
}

type auditRec struct {
	// call numbers the server's tool calls from 1, so an intent line pairs with its outcome when calls overlap.
	// pid plus call is unique across servers sharing the log. The client's JSON-RPC id is never logged: it is
	// client-chosen text.
	call                       uint64
	home, tool, target, result string
	created                    string // "@W,%P" of a window or pane this call made
	bytes                      int
	args                       []string // already hashed, "name=len:N,sha256:HEX"
}

func hashArg(name, v string) string {
	return fmt.Sprintf("%s=len:%d,sha256:%x", name, len(v), sha256.Sum256([]byte(v)))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// privateDir makes dir with mode 0700, and tightens it to 0700 when it already exists with a looser mode, which
// MkdirAll alone leaves as it is.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if fi.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

func (a *Audit) Write(r auditRec) error {
	if a == nil {
		return errors.New("audit log path unknown")
	}
	line := fmt.Sprintf("%s pid=%d call=%d home=%s tool=%s target=%s result=%s bytes=%d new=%s args=%s\n",
		time.Now().UTC().Format(time.RFC3339), a.pid, r.call, orDash(r.home), orDash(r.tool), orDash(r.target),
		orDash(r.result), r.bytes, orDash(r.created), orDash(strings.Join(r.args, ";")))
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := privateDir(filepath.Dir(a.path)); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
