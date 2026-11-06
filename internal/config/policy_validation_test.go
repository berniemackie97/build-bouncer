package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyValuesAndNormalization(t *testing.T) {
	cases := []struct {
		level, insult, wantLevel, wantInsult, wantError string
	}{
		{"", "", "moderate", "snarky", ""},
		{" STRICT ", " POLITE ", "strict", "polite", ""},
		{"lax", "nuclear", "lax", "nuclear", ""},
		{"strcit", "", "", "", "protection.level"},
		{"moderate", "unknown", "", "", "insults.mode"},
	}
	for _, tc := range cases {
		cfg := &Config{
			Checks:     []Check{{Name: "test", Run: "echo ok"}},
			Protection: Protection{Level: tc.level},
			Insults:    Insults{Mode: tc.insult},
		}
		err := validateAndDefault(cfg)
		if tc.wantError != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("level=%q insults=%q: expected %q error, got %v", tc.level, tc.insult, tc.wantError, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Protection.ProtectionLevel() != tc.wantLevel || cfg.Insults.Mode != tc.wantInsult {
			t.Fatalf("wrong normalized policy: %+v %+v", cfg.Protection, cfg.Insults)
		}
	}
	enabled := true
	if (Protection{Level: "strict", Interactive: &enabled}).IsInteractive() {
		t.Fatal("strict mode must never offer an override")
	}
}

func TestCheckDirectoryValidationIsPortable(t *testing.T) {
	cases := []struct {
		cwd, normalized string
		reject          bool
	}{
		{cwd: ".", normalized: "."},
		{cwd: "./src/../tests", normalized: "tests"},
		{cwd: `src\app`, normalized: "src/app"},
		{cwd: " ", normalized: ""},
		{cwd: "../outside", reject: true},
		{cwd: "src/../../outside", reject: true},
		{cwd: "/tmp/work", reject: true},
		{cwd: `C:\work`, reject: true},
		{cwd: "C:relative", reject: true},
		{cwd: `\\server\share`, reject: true},
		{cwd: "src\nother", reject: true},
	}
	for _, tc := range cases {
		cfgBytes := fmt.Sprintf("version: 1\nchecks:\n  - name: test\n    run: echo ok\n    cwd: %q\n", tc.cwd)
		cfg, err := Parse([]byte(cfgBytes))
		if tc.reject {
			if err == nil || !strings.Contains(err.Error(), "cwd") {
				t.Fatalf("cwd %q should fail, got %v", tc.cwd, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("cwd %q rejected: %v", tc.cwd, err)
		}
		if cfg.Checks[0].Cwd != tc.normalized {
			t.Fatalf("cwd %q normalized to %q, want %q", tc.cwd, cfg.Checks[0].Cwd, tc.normalized)
		}
	}
}

func TestInvalidEnvOverridesRejected(t *testing.T) {
	for _, env := range []map[string]string{
		{"": "value"},
		{" KEY": "value"},
		{"BAD KEY": "value"},
		{"KEY=VAL": "value"},
		{"KEY\nVAL": "value"},
		{"KEY": "before\x00after"},
	} {
		cfg := &Config{Checks: []Check{{Name: "test", Run: "echo ok", Env: env}}}
		if err := validateAndDefault(cfg); err == nil || !strings.Contains(err.Error(), "env") {
			t.Fatalf("invalid env %q accepted: %v", env, err)
		}
	}
	cfg := &Config{Checks: []Check{{Name: "test", Run: "echo ok", Env: map[string]string{"DOTNET_ENVIRONMENT": "Development", "EMPTY": ""}}}}
	if err := validateAndDefault(cfg); err != nil {
		t.Fatalf("valid env rejected: %v", err)
	}
}

func TestInvalidConfigurationNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("user config")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []*Config{
		{Checks: []Check{{Name: "test", Run: "echo ok"}}, Protection: Protection{Level: "strcit"}},
		{Checks: []Check{{Name: "test", Run: "echo ok", Cwd: "../outside"}}},
		{Checks: []Check{{Name: "test", Run: "echo ok", Env: map[string]string{"BAD KEY": "value"}}}},
	} {
		if err := Save(path, cfg); err == nil {
			t.Fatal("invalid config saved successfully")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(original) {
			t.Fatalf("invalid save modified original: %q %v", got, err)
		}
	}
}
