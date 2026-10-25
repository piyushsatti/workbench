// Package sessions reads Claude Code transcripts under <config dir>/projects.
package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"workspaces/internal/policy"
)

const transcriptExt = ".jsonl"

type Session struct {
	ID       string
	Path     string
	Modified time.Time
	// Cwd is the first cwd the transcript records.
	Cwd string
}

type candidate struct {
	path string
	id   string
	mod  time.Time
}

// Find returns the transcripts whose cwd is one of roots or below one, newest
// first, at most limit of them (0 for all). A project folder's name is never
// decoded, because Claude Code's folder-name encoding is not reliable; each
// transcript is read only up to its first record that carries a cwd, and
// those are read newest first so a limit stops the reading early. A file that
// cannot be read, or has no cwd, is skipped. A missing config dir has no
// sessions.
func Find(configDir string, roots []string, limit int) ([]Session, error) {
	projects, err := os.ReadDir(filepath.Join(configDir, "projects"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cands []candidate
	for _, p := range projects {
		// A symlinked project folder is never followed.
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(configDir, "projects", p.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if !f.Type().IsRegular() || !strings.HasSuffix(name, transcriptExt) {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			cands = append(cands, candidate{filepath.Join(dir, name), strings.TrimSuffix(name, transcriptExt), info.ModTime()})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if !cands[i].mod.Equal(cands[j].mod) {
			return cands[i].mod.After(cands[j].mod)
		}
		return cands[i].id < cands[j].id
	})

	var out []Session
	for _, c := range cands {
		cwd := firstCwd(c.path)
		if cwd == "" || !underAny(cwd, roots) {
			continue
		}
		out = append(out, Session{ID: c.id, Path: c.path, Modified: c.mod, Cwd: cwd})
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, nil
}

func underAny(p string, roots []string) bool {
	p = filepath.Clean(p)
	for _, r := range roots {
		if policy.Under(p, filepath.Clean(r)) {
			return true
		}
	}
	return false
}

// firstCwd returns the cwd of the first record that has one. Lines can be
// megabytes long, so it reads with a Reader rather than a Scanner.
func firstCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		// Cheap test first: most records before the first cwd are small, and
		// skipping the JSON decode of those that lack the key costs nothing.
		if bytes.Contains(line, []byte(`"cwd"`)) {
			var rec struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(line, &rec) == nil && rec.Cwd != "" {
				return rec.Cwd
			}
		}
		if err != nil {
			return ""
		}
	}
}

const (
	Untitled = "(untitled)"
	Empty    = "(empty)"
	// emptyLines is the length below which a transcript with no title record
	// counts as an abandoned, empty session.
	emptyLines      = 10
	promptTitleRune = 60
)

// Title returns the session's title: the last custom-title, else the first
// ai-title, else the last last-prompt cut to 60 characters. A transcript with
// none of them is "(empty)" when it is under ten lines and "(untitled)"
// otherwise. A transcript that cannot be read is "(untitled)".
func Title(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return Untitled
	}
	defer f.Close()
	var custom, ai, prompt string
	var haveCustom, haveAI, havePrompt bool
	lines := 0
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			lines++
		}
		if hasTitleKey(line) {
			var rec struct {
				Type        string `json:"type"`
				CustomTitle string `json:"customTitle"`
				AITitle     string `json:"aiTitle"`
				LastPrompt  string `json:"lastPrompt"`
			}
			if json.Unmarshal(line, &rec) == nil {
				switch rec.Type {
				case "custom-title":
					custom, haveCustom = rec.CustomTitle, true
				case "ai-title":
					if !haveAI {
						ai, haveAI = rec.AITitle, true
					}
				case "last-prompt":
					prompt, havePrompt = rec.LastPrompt, true
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return Untitled
			}
			break
		}
	}
	var title string
	switch {
	case haveCustom:
		title = stripCustom(custom)
	case haveAI:
		title = ai
	case havePrompt:
		title = cutRunes(prompt, promptTitleRune)
	}
	switch {
	case strings.TrimSpace(title) != "":
		return title
	case !haveCustom && !haveAI && !havePrompt && lines < emptyLines:
		return Empty
	}
	return Untitled
}

func hasTitleKey(line []byte) bool {
	return bytes.Contains(line, []byte(`"custom-title"`)) ||
		bytes.Contains(line, []byte(`"ai-title"`)) ||
		bytes.Contains(line, []byte(`"last-prompt"`))
}

// stripCustom drops a pasted local-command caveat,
// or the " (Branch" suffix Claude Code appends to a renamed fork, and
// everything after either, then leading space.
func stripCustom(s string) string {
	for _, marker := range []string{"<local-command-caveat>", " (Branch"} {
		if i := strings.Index(s, marker); i >= 0 {
			s = s[:i]
		}
	}
	return strings.TrimLeft(s, " \t\r\n")
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
