// Package config defines the build-bouncer configuration model, parsing, validation,
// defaults, and path helpers.
//
// This package is intentionally small and dependency-light. It should be safe to call
// from CLI entrypoints and hooks without surprising side effects.
package config

import (
	"os"
	"path/filepath"
)

const (
	ConfigDirName     = ".buildbouncer"
	ConfigFileName    = "config.yaml"
	LegacyConfigName  = ".buildbouncer.yaml"
	DefaultAssetsDir  = "assets"
	DefaultInsultsRel = "assets/insults/default.json"
	DefaultBanterRel  = "assets/banter/default.json"
)

func ConfigDir(root string) string {
	return filepath.Join(root, ConfigDirName)
}

func DefaultConfigPath(root string) string {
	return filepath.Join(root, ConfigDirName, ConfigFileName)
}

func LegacyConfigPath(root string) string {
	return filepath.Join(root, LegacyConfigName)
}

func DefaultAssetsPath(root string) string {
	return filepath.Join(root, ConfigDirName, DefaultAssetsDir)
}

// FindConfigInRoot returns the config path to use and whether it already exists.
// Preference order:
//  1. .buildbouncer/config.yaml
//  2. .buildbouncer.yaml (legacy)
func FindConfigInRoot(root string) (string, bool) {
	defaultPath := DefaultConfigPath(root)
	if pathExists(defaultPath) {
		return defaultPath, true
	}

	legacyPath := LegacyConfigPath(root)
	if pathExists(legacyPath) {
		return legacyPath, true
	}

	return defaultPath, false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
