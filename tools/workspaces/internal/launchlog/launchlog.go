package launchlog

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

type Tmux struct {
	Socket      string `json:"socket"`
	SessionID   string `json:"session_id"`
	Session     string `json:"session"`
	WindowID    string `json:"window_id"`
	WindowIndex *int   `json:"window_index"`
	Window      string `json:"window"`
	PaneID      string `json:"pane_id"`
	// Entry is "pane" for a launch typed in an existing pane, "bootstrap" when
	// workspaces created the window.
	Entry string `json:"entry"`
}

type Record struct {
	TS           string   `json:"ts"`
	ID           string   `json:"id"`
	Agent        string   `json:"agent"`
	Decision     string   `json:"decision"`
	Reason       string   `json:"reason"`
	Rule         string   `json:"rule"`
	Cwd          string   `json:"cwd"`
	CwdLogical   string   `json:"cwd_logical"`
	LaunchDir    string   `json:"launch_dir"`
	Space        *string  `json:"space"`
	Harness      string   `json:"harness"`
	Adapter      string   `json:"adapter"`
	Mode         string   `json:"mode"`
	Tmux         *Tmux    `json:"tmux"`
	Bin          string   `json:"bin"`
	BinTarget    string   `json:"bin_target"`
	Args         []string `json:"args"`
	AddDirs      []string `json:"add_dirs"`
	ConfigSHA256 string   `json:"config_sha256"`
	TTY          bool     `json:"tty"`
	PID          int      `json:"pid"`
	PPID         int      `json:"ppid"`
	// UnenforcedDeny counts deny_access paths the harness cannot enforce; it
	// is left out when zero, so rows from before it existed read the same.
	UnenforcedDeny int `json:"unenforced_deny,omitempty"`
}

// SessionRecord joins a launch to the session it started, written by the
// SessionStart hook.
type SessionRecord struct {
	TS        string `json:"ts"`
	LaunchID  string `json:"launch_id"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Source    string `json:"source"`
}

type Line struct {
	Raw    string
	Record Record
}

func FormatTime(t time.Time) string {
	return t.Format(timeLayout)
}

func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Append writes r as one line with a single write on an O_APPEND descriptor,
// so concurrent writers never interleave within a record.
func Append(path string, r Record) error {
	if r.Args == nil {
		r.Args = []string{}
	}
	if r.AddDirs == nil {
		r.AddDirs = []string{}
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return appendLine(path, data)
}

// AppendSession writes r to sessions.jsonl the way Append writes a launch.
func AppendSession(path string, r SessionRecord) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return appendLine(path, data)
}

func appendLine(path string, data []byte) error {
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// A missing file is an empty log. A FIFO, device or directory is an error
// rather than a read that could block.
func Read(path string) ([]Line, int, error) {
	var lines []Line
	malformed, err := readJSONL(path, func(raw []byte) error {
		var r Record
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		lines = append(lines, Line{Raw: string(raw), Record: r})
		return nil
	})
	return lines, malformed, err
}

// ReadSessions reads sessions.jsonl, oldest first, with the same rules as Read.
func ReadSessions(path string) ([]SessionRecord, int, error) {
	var out []SessionRecord
	malformed, err := readJSONL(path, func(raw []byte) error {
		var r SessionRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	return out, malformed, err
}

// readJSONL calls decode for each non-blank line and counts the lines it
// rejects.
func readJSONL(path string, decode func(raw []byte) error) (int, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	malformed := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		if err := decode(raw); err != nil {
			malformed++
		}
	}
	return malformed, sc.Err()
}
