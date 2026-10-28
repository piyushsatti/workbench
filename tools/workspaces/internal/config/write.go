package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockDir takes an exclusive flock on dir, creating it with mode 0700 when
// missing, and returns the function that releases it. A config edit holds it
// from its read to its rename, so two edits at once cannot lose either one.
func LockDir(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		d.Close()
		return nil, fmt.Errorf("locking %s: %w", dir, err)
	}
	return func() {
		syscall.Flock(int(d.Fd()), syscall.LOCK_UN)
		d.Close()
	}, nil
}

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
