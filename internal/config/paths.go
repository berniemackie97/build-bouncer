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

func FindConfigInRoot(root string) (string, bool) {
	if pathExists(DefaultConfigPath(root)) {
		return DefaultConfigPath(root), true
	}
	if pathExists(LegacyConfigPath(root)) {
		return LegacyConfigPath(root), true
	}
	return DefaultConfigPath(root), false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
