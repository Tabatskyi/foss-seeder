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

	if len(cfg.Rules) != 0 {
		t.Fatalf("expected 0 initial rules, got %d", len(cfg.Rules))
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

func TestSettingsPersistenceWithEnvVars(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)
	t.Setenv("QBIT_HOST", "http://127.0.0.1:8080")
	t.Setenv("SAVE_PATH", "/downloads/default")

	// 1. Initial load from env vars
	cfg := LoadConfig()
	if cfg.QbitHost != "http://127.0.0.1:8080" {
		t.Fatalf("expected initial QbitHost from env, got %s", cfg.QbitHost)
	}

	// 2. User updates settings in UI and saves
	err := cfg.UpdateSettings("http://192.168.1.100:8080", "customuser", "secretpass", "custom-cat", "/custom/storage", "https://example.com/feed.xml", 3600, false)
	if err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	// 3. Simulate container restart with environment variables still set
	restartedCfg := LoadConfig()

	if restartedCfg.QbitHost != "http://192.168.1.100:8080" {
		t.Errorf("expected persisted QbitHost http://192.168.1.100:8080, got %s", restartedCfg.QbitHost)
	}
	if restartedCfg.SavePath != "/custom/storage" {
		t.Errorf("expected persisted SavePath /custom/storage, got %s", restartedCfg.SavePath)
	}
	if restartedCfg.QbitUser != "customuser" {
		t.Errorf("expected persisted QbitUser customuser, got %s", restartedCfg.QbitUser)
	}
	if restartedCfg.QbitPass != "secretpass" {
		t.Errorf("expected persisted QbitPass secretpass, got %s", restartedCfg.QbitPass)
	}
	if restartedCfg.QbitCategory != "custom-cat" {
		t.Errorf("expected persisted QbitCategory custom-cat, got %s", restartedCfg.QbitCategory)
	}
	if restartedCfg.CheckIntervalSeconds != 3600 {
		t.Errorf("expected persisted CheckIntervalSeconds 3600, got %d", restartedCfg.CheckIntervalSeconds)
	}
	if restartedCfg.SequentialDownload != false {
		t.Errorf("expected persisted SequentialDownload false, got %v", restartedCfg.SequentialDownload)
	}
}
