package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile replaces path with data without ever leaving a half-written file:
// the old bytes go to path+".prev", the new ones to a temp file in the same
// directory, which is synced and renamed over path, and the directory is synced
// last. The directory is created with mode 0700 when missing. A symlinked
// path is written through to its target, so a config file linked from a
// dotfiles repo stays linked.
func WriteFile(path string, data []byte) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	old, err := os.ReadFile(path)
	switch {
	case err == nil:
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := replace(path+".prev", old, mode); err != nil {
			return fmt.Errorf("keeping the previous version: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	return replace(path, data, mode)
}

func replace(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
