package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/storage"
)

type setupFile struct {
	path    string
	mode    os.FileMode
	data    []byte
	existed bool
}

type setupSnapshot struct {
	files       []setupFile
	createdDirs []string
}

func snapshotSetup(root string) (setupSnapshot, error) {
	var snapshot setupSnapshot
	assets := config.DefaultAssetsPath(root)
	dirs := []string{
		config.ConfigDir(root),
		assets,
		filepath.Join(assets, "insults"),
		filepath.Join(assets, "banter"),
	}
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			snapshot.createdDirs = append(snapshot.createdDirs, dir)
			continue
		}
		if err != nil {
			return setupSnapshot{}, err
		}
		if !info.IsDir() {
			return setupSnapshot{}, fmt.Errorf("setup directory is not a directory: %s", dir)
		}
	}
	paths := []string{
		config.DefaultConfigPath(root),
		filepath.Join(assets, "insults", "default.json"),
		filepath.Join(assets, "banter", "default.json"),
	}
	for _, path := range paths {
		file := setupFile{path: path}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			snapshot.files = append(snapshot.files, file)
			continue
		}
		if err != nil {
			return setupSnapshot{}, err
		}
		if !info.Mode().IsRegular() {
			return setupSnapshot{}, fmt.Errorf("setup file is not a regular file: %s", path)
		}
		file.data, err = os.ReadFile(path)
		if err != nil {
			return setupSnapshot{}, err
		}
		file.mode = info.Mode().Perm()
		file.existed = true
		snapshot.files = append(snapshot.files, file)
	}
	return snapshot, nil
}

func (snapshot setupSnapshot) restore() error {
	var failures []error
	for i := len(snapshot.files) - 1; i >= 0; i-- {
		file := snapshot.files[i]
		if file.existed {
			if err := storage.WriteFile(file.path, file.data, file.mode); err != nil {
				failures = append(failures, fmt.Errorf("restore %s: %w", file.path, err))
				continue
			}
			if err := os.Chmod(file.path, file.mode); err != nil {
				failures = append(failures, fmt.Errorf("restore permissions %s: %w", file.path, err))
			}
			continue
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Errorf("remove created file %s: %w", file.path, err))
		}
	}
	for i := len(snapshot.createdDirs) - 1; i >= 0; i-- {
		dir := snapshot.createdDirs[i]
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("inspect created directory %s: %w", dir, err))
			continue
		}
		if len(entries) != 0 {
			continue
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Errorf("remove created directory %s: %w", dir, err))
		}
	}
	return errors.Join(failures...)
}
