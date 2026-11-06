package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var mutationMutex sync.Mutex

func WriteFile(path string, data []byte, defaultMode os.FileMode) error {
	mutationMutex.Lock()
	defer mutationMutex.Unlock()
	return writeExisting(path, data, defaultMode, true)
}

func WritePrivateState(path string, data []byte) error {
	mutationMutex.Lock()
	defer mutationMutex.Unlock()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("state directory %q must not be a symbolic link", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return writeExisting(path, data, 0o600, false)
}

func writeExisting(path string, data []byte, defaultMode os.FileMode, preserveMode bool) error {
	mode := defaultMode
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular file %q", path)
		}
		if preserveMode {
			mode = info.Mode().Perm()
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFile(path, data, mode, false)
}

func CreateFile(path string, data []byte, mode os.FileMode) error {
	mutationMutex.Lock()
	defer mutationMutex.Unlock()
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("file already exists: %s: %w", path, os.ErrExist)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFile(path, data, mode, true)
}

func writeFile(path string, data []byte, mode os.FileMode, createOnly bool) error {
	dir := filepath.Dir(path)
	out, err := os.CreateTemp(dir, ".build-bouncer-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	if err := out.Chmod(mode); err != nil {
		_ = out.Close()
		return err
	}
	if _, err := out.Write(data); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if createOnly {
		err = os.Link(out.Name(), path)
	} else {
		err = replaceFile(out.Name(), path)
	}
	if err != nil {
		return err
	}
	return syncDirectory(dir)
}

func ReplaceStagedFile(staged, path string) error {
	mutationMutex.Lock()
	defer mutationMutex.Unlock()
	if filepath.Dir(staged) != filepath.Dir(path) {
		return fmt.Errorf("staged file must be in destination directory")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular file %q", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := replaceFile(staged, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
