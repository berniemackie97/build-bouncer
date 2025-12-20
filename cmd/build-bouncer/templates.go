package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func ensureDefaultPack(targetRoot string, destPath string, templateName string, force bool) error {
	if !force {
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}
	}

	templateBytes, err := loadTemplateBytes(targetRoot, templateName)
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, templateBytes, 0o644)
}

func loadTemplateBytes(targetRoot string, templateName string) ([]byte, error) {
	candidates := []string{
		filepath.Join(targetRoot, "assets", "templates", templateName),
	}

	if dir := strings.TrimSpace(os.Getenv("BUILDBOUNCER_TEMPLATES_DIR")); dir != "" {
		candidates = append(candidates,
			filepath.Join(dir, templateName),
			filepath.Join(dir, "templates", templateName),
		)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "templates", templateName),
			filepath.Join(exeDir, "assets", "templates", templateName),
			filepath.Join(exeDir, "..", "share", "build-bouncer", "templates", templateName),
			filepath.Join(exeDir, "..", "libexec", "build-bouncer", "templates", templateName),
		)
	}

	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return b, nil
		}
	}

	return nil, errors.New("template not found: " + templateName + " (expected assets/templates or set BUILDBOUNCER_TEMPLATES_DIR)")
}
