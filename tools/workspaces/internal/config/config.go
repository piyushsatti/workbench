package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var ErrMissing = errors.New("config file missing")

type Entry struct {
	Written string
	// Logical is Written with ~ expanded and the path cleaned, symlinks left
	// alone.
	Logical string
	// Path is expanded and cleaned, with the literal part before the first glob
	// segment resolved through symlinks when it exists.
	Path   string
	Glob   bool
	Exists bool
}

type Space struct {
	Name       string
	Root       Entry
	Attach     []Entry
	DenyAccess []Entry
	Todo       string
	Log        string
}

type Spaces struct {
	AllowLaunch []Entry
	DenyLaunch  []Entry
	DenyAccess  []Entry
	Spaces      map[string]*Space
}

type Harness struct {
	Name    string
	Adapter string
	Bin     string
	Env     map[string]string
}

type Harnesses struct {
	Default   string
	Harnesses map[string]Harness
}

type rawSpace struct {
	Root       string   `json:"root"`
	Attach     []string `json:"attach"`
	DenyAccess []string `json:"deny_access"`
	Todo       string   `json:"todo"`
	Log        string   `json:"log"`
}

type rawSpaces struct {
	AllowLaunch []string            `json:"allow_launch"`
	DenyLaunch  []string            `json:"deny_launch"`
	DenyAccess  []string            `json:"deny_access"`
	Spaces      map[string]rawSpace `json:"spaces"`
}

type rawHarness struct {
	Adapter string            `json:"adapter"`
	Bin     string            `json:"bin"`
	Env     map[string]string `json:"env"`
}

type rawHarnesses struct {
	Default   string                `json:"default"`
	Harnesses map[string]rawHarness `json:"harnesses"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

const maxNameLen = 32

// Space names become tmux session names, and tmux rewrites "." and ":" in
// those, so neither is allowed.
func ValidName(s string) bool {
	return len(s) <= maxNameLen && namePattern.MatchString(s)
}

func ConfigDir(getenv func(string) string) (string, error) {
	return xdgDir(getenv, "XDG_CONFIG_HOME", ".config")
}

func StateDir(getenv func(string) string) (string, error) {
	return xdgDir(getenv, "XDG_STATE_HOME", filepath.Join(".local", "state"))
}

func xdgDir(getenv func(string) string, key, fallback string) (string, error) {
	if dir := getenv(key); filepath.IsAbs(dir) {
		return filepath.Join(dir, "workspaces"), nil
	}
	home := getenv("HOME")
	if !filepath.IsAbs(home) {
		return "", errors.New("HOME is not set to an absolute path")
	}
	return filepath.Join(home, fallback, "workspaces"), nil
}

// The returned hash covers the file's bytes and is set whenever the file could
// be read, even when its content is invalid.
func LoadSpaces(path, home string) (*Spaces, string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("%s: %w", path, ErrMissing)
	}
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	cfg, err := ParseSpaces(data, home)
	if err != nil {
		return nil, hash, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, hash, nil
}

// ParseSpaces validates the bytes of a spaces.json, so a change can be checked
// before it is written.
func ParseSpaces(data []byte, home string) (*Spaces, error) {
	var raw rawSpaces
	if err := decodeStrict(data, &raw); err != nil {
		return nil, err
	}
	return buildSpaces(raw, home)
}

// LoadHarnesses reads harness.json, or returns the built-in default when the
// file is missing.
func LoadHarnesses(path, home string, knownAdapter func(string) bool) (*Harnesses, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultHarnesses(), nil
	}
	if err != nil {
		return nil, err
	}
	h, err := ParseHarnesses(data, home, knownAdapter)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return h, nil
}

// ParseHarnesses validates the bytes of a harness.json.
func ParseHarnesses(data []byte, home string, knownAdapter func(string) bool) (*Harnesses, error) {
	var raw rawHarnesses
	if err := decodeStrict(data, &raw); err != nil {
		return nil, err
	}
	return buildHarnesses(raw, home, knownAdapter)
}

// DefaultHarnessesJSON is the file DefaultHarnesses stands for.
const DefaultHarnessesJSON = `{"default":"claude","harnesses":{"claude":{"adapter":"claude"}}}`

func DefaultHarnesses() *Harnesses {
	return &Harnesses{
		Default:   "claude",
		Harnesses: map[string]Harness{"claude": {Name: "claude", Adapter: "claude"}},
	}
}

func (s *Spaces) Space(name string) (*Space, bool) {
	sp, ok := s.Spaces[name]
	return sp, ok
}

func (s *Spaces) SpaceNames() []string {
	return sortedKeys(s.Spaces)
}

func (s *Spaces) Missing() []string {
	var out []string
	add := func(where string, es ...Entry) {
		for _, e := range es {
			if !e.Exists {
				out = append(out, fmt.Sprintf("%s %s does not exist", where, e.Written))
			}
		}
	}
	add("allow_launch", s.AllowLaunch...)
	add("deny_launch", s.DenyLaunch...)
	add("deny_access", s.DenyAccess...)
	for _, name := range s.SpaceNames() {
		sp := s.Spaces[name]
		add("spaces."+name+".root", sp.Root)
		add("spaces."+name+".attach", sp.Attach...)
		add("spaces."+name+".deny_access", sp.DenyAccess...)
	}
	return out
}

// Names lists harness names with the default first and the rest sorted.
func (h *Harnesses) Names() []string {
	names := []string{h.Default}
	for _, n := range sortedKeys(h.Harnesses) {
		if n != h.Default {
			names = append(names, n)
		}
	}
	return names
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after the top-level object")
	}
	return nil
}

func buildSpaces(raw rawSpaces, home string) (*Spaces, error) {
	cfg := &Spaces{Spaces: map[string]*Space{}}
	var err error
	if cfg.AllowLaunch, err = parseEntries("allow_launch", raw.AllowLaunch, home, true); err != nil {
		return nil, err
	}
	if cfg.DenyLaunch, err = parseEntries("deny_launch", raw.DenyLaunch, home, true); err != nil {
		return nil, err
	}
	if cfg.DenyAccess, err = parseEntries("deny_access", raw.DenyAccess, home, false); err != nil {
		return nil, err
	}
	for name, rs := range raw.Spaces {
		if !ValidName(name) {
			return nil, fmt.Errorf("space name %q: must match [a-z0-9][a-z0-9_-]* and be at most %d characters", name, maxNameLen)
		}
		key := "spaces." + name
		if rs.Root == "" {
			return nil, fmt.Errorf("%s.root is required", key)
		}
		sp := &Space{Name: name}
		if sp.Root, err = ParseEntry(rs.Root, home, false); err != nil {
			return nil, fmt.Errorf("%s.root: %w", key, err)
		}
		if sp.Attach, err = parseEntries(key+".attach", rs.Attach, home, false); err != nil {
			return nil, err
		}
		if sp.DenyAccess, err = parseEntries(key+".deny_access", rs.DenyAccess, home, false); err != nil {
			return nil, err
		}
		sp.Todo = spaceFile(rs.Todo, "TODO.jsonl", sp.Root.Path, home)
		sp.Log = spaceFile(rs.Log, "LOGS.jsonl", sp.Root.Path, home)
		cfg.Spaces[name] = sp
	}
	return cfg, nil
}

func spaceFile(written, def, root, home string) string {
	switch {
	case written == "":
		return filepath.Join(root, def)
	case written == "~" || strings.HasPrefix(written, "~/"):
		return filepath.Join(home, written[1:])
	case filepath.IsAbs(written):
		return filepath.Clean(written)
	default:
		return filepath.Join(root, written)
	}
}

func buildHarnesses(raw rawHarnesses, home string, knownAdapter func(string) bool) (*Harnesses, error) {
	if len(raw.Harnesses) == 0 {
		return nil, errors.New("harnesses is empty")
	}
	h := &Harnesses{Default: raw.Default, Harnesses: map[string]Harness{}}
	for name, rh := range raw.Harnesses {
		if !ValidName(name) {
			return nil, fmt.Errorf("harness name %q: must match [a-z0-9][a-z0-9_-]* and be at most %d characters", name, maxNameLen)
		}
		if rh.Adapter == "" {
			return nil, fmt.Errorf("harnesses.%s.adapter is required", name)
		}
		if !knownAdapter(rh.Adapter) {
			return nil, fmt.Errorf("harnesses.%s.adapter %q is not a compiled-in adapter", name, rh.Adapter)
		}
		env := make(map[string]string, len(rh.Env))
		for k, v := range rh.Env {
			env[k] = expandTilde(v, home)
		}
		bin := expandTilde(rh.Bin, home)
		// A relative path would resolve against whatever folder workspaces runs in.
		if strings.Contains(bin, "/") && !filepath.IsAbs(bin) {
			return nil, fmt.Errorf("harnesses.%s.bin %q: must be absolute, start with ~/, or be a bare name looked up on PATH", name, rh.Bin)
		}
		h.Harnesses[name] = Harness{Name: name, Adapter: rh.Adapter, Bin: bin, Env: env}
	}
	if h.Default == "" {
		return nil, errors.New("default is required")
	}
	if _, ok := h.Harnesses[h.Default]; !ok {
		return nil, fmt.Errorf("default %q names no harness", h.Default)
	}
	return h, nil
}

func expandTilde(v, home string) string {
	if v == "~" || strings.HasPrefix(v, "~/") {
		return home + v[1:]
	}
	return v
}

func parseEntries(key string, written []string, home string, globOK bool) ([]Entry, error) {
	out := make([]Entry, 0, len(written))
	for _, w := range written {
		e, err := ParseEntry(w, home, globOK)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func ParseEntry(written, home string, globOK bool) (Entry, error) {
	var abs string
	switch {
	case written == "~" || strings.HasPrefix(written, "~/"):
		abs = home + written[1:]
	case strings.HasPrefix(written, "/"):
		abs = written
	default:
		return Entry{}, fmt.Errorf("%q: paths must start with ~, ~/ or /", written)
	}
	clean := filepath.Clean(abs)
	e := Entry{Written: written, Logical: clean, Path: clean, Glob: strings.ContainsAny(clean, "*?[")}
	if e.Glob {
		if !globOK {
			return Entry{}, fmt.Errorf("%q: globs are not allowed here", written)
		}
		if strings.Contains(clean, "**") {
			return Entry{}, fmt.Errorf("%q: ** is not supported", written)
		}
		if _, err := filepath.Match(clean, ""); err != nil {
			return Entry{}, fmt.Errorf("%q: %w", written, err)
		}
	}

	segs := strings.Split(clean, "/")
	lit := len(segs)
	for i, s := range segs {
		if strings.ContainsAny(s, "*?[") {
			lit = i
			break
		}
	}
	literal := "/" + filepath.Join(segs[:lit]...)
	resolved, err := filepath.EvalSymlinks(literal)
	if err != nil {
		return e, nil
	}
	e.Exists = true
	e.Path = filepath.Join(append([]string{resolved}, segs[lit:]...)...)
	return e, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
