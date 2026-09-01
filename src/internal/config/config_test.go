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
	err := cfg.UpdateSettings("http://192.168.1.100:8080", "customuser", "secretpass", "custom-cat", "/custom/storage", []string{"https://example.com/feed.xml", "https://example.com/feed2.xml"}, 3600, false, true)
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
	if len(restartedCfg.FeedURLs) != 2 || restartedCfg.FeedURLs[0] != "https://example.com/feed.xml" || restartedCfg.FeedURLs[1] != "https://example.com/feed2.xml" {
		t.Errorf("expected persisted FeedURLs [https://example.com/feed.xml, https://example.com/feed2.xml], got %v", restartedCfg.FeedURLs)
	}
	if restartedCfg.SeparateFeedTabs != true {
		t.Errorf("expected persisted SeparateFeedTabs true, got %v", restartedCfg.SeparateFeedTabs)
	}
	if restartedCfg.CheckIntervalSeconds != 3600 {
		t.Errorf("expected persisted CheckIntervalSeconds 3600, got %d", restartedCfg.CheckIntervalSeconds)
	}
	if restartedCfg.SequentialDownload != false {
		t.Errorf("expected persisted SequentialDownload false, got %v", restartedCfg.SequentialDownload)
	}
}

func TestMultipleFeedURLsConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "legacy_config.json")
	t.Setenv("CONFIG_PATH", configPath)

	// Write a legacy config file with a single feed_url
	legacyContent := []byte(`{
		"port": "7474",
		"feed_url": "https://legacy.example.com/rss.xml",
		"qbit_host": "http://localhost:8080"
	}`)
	if err := os.WriteFile(configPath, legacyContent, 0644); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}

	cfg := LoadConfig()
	if len(cfg.FeedURLs) != 1 || cfg.FeedURLs[0] != "https://legacy.example.com/rss.xml" {
		t.Fatalf("expected legacy feed_url to populate FeedURLs, got %v", cfg.FeedURLs)
	}
	if cfg.FeedURL != "https://legacy.example.com/rss.xml" {
		t.Fatalf("expected legacy feed_url, got %s", cfg.FeedURL)
	}

	// Update to multiple feeds
	newFeeds := []string{"https://feed1.com/rss", "https://feed2.com/rss"}
	if err := cfg.UpdateSettings(cfg.QbitHost, cfg.QbitUser, cfg.QbitPass, cfg.QbitCategory, cfg.SavePath, newFeeds, 600, true, false); err != nil {
		t.Fatalf("failed to update multi feeds: %v", err)
	}

	// Test adding a rule tied to feed
	feedRule := TargetRule{
		Key:        "arch-feed-1",
		Name:       "Arch Feed 1",
		TitleRegex: "^Arch.*",
		Enabled:    true,
		FeedURL:    "https://feed1.com/rss",
	}
	if err := cfg.SetRule(feedRule); err != nil {
		t.Fatalf("failed to set rule tied to feed: %v", err)
	}

	reloaded := LoadConfig()
	if len(reloaded.FeedURLs) != 2 || reloaded.FeedURLs[0] != "https://feed1.com/rss" || reloaded.FeedURLs[1] != "https://feed2.com/rss" {
		t.Fatalf("expected reloaded FeedURLs to have 2 entries, got %v", reloaded.FeedURLs)
	}
	if reloaded.FeedURL != "https://feed1.com/rss" {
		t.Fatalf("expected FeedURL fallback to be first feed, got %s", reloaded.FeedURL)
	}
	if r, ok := reloaded.Rules["arch-feed-1"]; !ok || r.FeedURL != "https://feed1.com/rss" {
		t.Fatalf("expected rule tied to feed1, got %v", r)
	}

	// Test toggle separate tabs
	newState, err := cfg.ToggleSeparateFeedTabs()
	if err != nil || !newState {
		t.Fatalf("expected ToggleSeparateFeedTabs to return true, got %v, err=%v", newState, err)
	}
}
