package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)

	cfg := LoadConfig()
	if cfg.Port != "7474" && cfg.Port != "8080" {
		t.Logf("Port loaded: %s", cfg.Port)
	}

	if len(cfg.Rules) == 0 {
		t.Fatalf("expected default rules to be populated, got 0")
	}

	// Add a rule
	newRule := TargetRule{
		Key:        "test-distro",
		Name:       "Test Distro",
		TitleRegex: `Test .*`,
		Enabled:    true,
		AutoPurge:  true,
	}

	if err := cfg.SetRule(newRule); err != nil {
		t.Fatalf("failed to set rule: %v", err)
	}

	// Verify rule exists
	updated := cfg.Get()
	if r, ok := updated.Rules["test-distro"]; !ok || !r.Enabled {
		t.Fatalf("expected test-distro to be present and enabled")
	}

	// Toggle rule
	enabled, err := cfg.ToggleRule("test-distro")
	if err != nil || enabled {
		t.Fatalf("expected toggle to make rule disabled, got enabled=%v, err=%v", enabled, err)
	}

	// Check disk file existence
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("config file was not saved to disk")
	}
}
