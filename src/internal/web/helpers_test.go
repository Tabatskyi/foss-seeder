package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"foss-seeder/internal/config"
	"foss-seeder/internal/feed"
	"foss-seeder/internal/logger"
	"foss-seeder/internal/qbit"
	"foss-seeder/internal/syncer"
)

func TestCleanDisplayNameAndSlug(t *testing.T) {
	tests := []struct {
		inputTitle   string
		expectedName string
		expectedSlug string
		testMatch    string
		nonMatches   []string
	}{
		{
			inputTitle:   "Alpine Linux 3.23.3 - Extended (x86) (x86)",
			expectedName: "Alpine Linux - Extended (x86)",
			expectedSlug: "alpine-linux-extended-x86",
			testMatch:    "Alpine Linux 3.24.0 - Extended (x86) (x86)",
			nonMatches: []string{
				"Alpine Linux 3.24.0 - Standard (x86) (x86)",
				"Alpine Linux 3.24.0 - Netboot (x86) (x86)",
			},
		},
		{
			inputTitle:   "Debian 13.6.0 - Netinst (amd64)",
			expectedName: "Debian - Netinst (amd64)",
			expectedSlug: "debian-netinst-amd64",
			testMatch:    "Debian 14.0.0 - Netinst (amd64)",
			nonMatches: []string{
				"Debian 13.6.0 - Edu - Netinst (amd64)",
				"Debian 13.6.0 - Mac - Netinst (amd64)",
				"Debian 14.0.0 - Edu - Netinst (amd64)",
				"Debian 14.0.0 - Live - Cinnamon (amd64)",
			},
		},
		{
			inputTitle:   "Debian 13.6.0 - Edu - Netinst (amd64)",
			expectedName: "Debian - Edu - Netinst (amd64)",
			expectedSlug: "debian-edu-netinst-amd64",
			testMatch:    "Debian 14.0.0 - Edu - Netinst (amd64)",
			nonMatches: []string{
				"Debian 13.6.0 - Netinst (amd64)",
				"Debian 13.6.0 - Mac - Netinst (amd64)",
				"Debian 14.0.0 - Netinst (amd64)",
			},
		},
		{
			inputTitle:   "Debian 13.6.0 - Mac - Netinst (amd64)",
			expectedName: "Debian - Mac - Netinst (amd64)",
			expectedSlug: "debian-mac-netinst-amd64",
			testMatch:    "Debian 14.0.0 - Mac - Netinst (amd64)",
			nonMatches: []string{
				"Debian 13.6.0 - Netinst (amd64)",
				"Debian 13.6.0 - Edu - Netinst (amd64)",
				"Debian 14.0.0 - Netinst (amd64)",
			},
		},
		{
			inputTitle:   "Cachy OS 260809 - Desktop (x86_64)",
			expectedName: "Cachy OS - Desktop (x86_64)",
			expectedSlug: "cachy-os-desktop-x86-64",
			testMatch:    "Cachy OS 260901 - Desktop (x86_64)",
			nonMatches: []string{
				"Cachy OS 260809 - Handheld (x86_64)",
				"Cachy OS 260901 - Server (x86_64)",
			},
		},
		{
			inputTitle:   "Kali Linux 2026.2 - Installer (amd64) (amd64)",
			expectedName: "Kali Linux - Installer (amd64)",
			expectedSlug: "kali-linux-installer-amd64",
			testMatch:    "Kali Linux 2026.3 - Installer (amd64) (amd64)",
			nonMatches: []string{
				"Kali Linux 2026.3 - Live (amd64) (amd64)",
			},
		},
		{
			inputTitle:   "CentOS 10-20260820.0 (x86_64) (x86_64)",
			expectedName: "CentOS (x86_64)",
			expectedSlug: "centos-x86-64",
			testMatch:    "CentOS 10-20260901.0 (x86_64) (x86_64)",
			nonMatches: []string{
				"CentOS Stream 9 (x86_64)",
			},
		},
		{
			inputTitle:   "Alpine Linux 3.23.3 - Mini Root Filesystem (x86)",
			expectedName: "Alpine Linux - Mini Root Filesystem (x86)",
			expectedSlug: "alpine-linux-mini-root-filesystem-x86",
			testMatch:    "Alpine Linux 3.24.0 - Mini Root Filesystem (x86)",
			nonMatches: []string{
				"Alpine Linux 3.23.3 - Mini Root Filesystem (x86_64)",
			},
		},
		{
			inputTitle:   "Alpine Linux 3.23.3 - Mini Root Filesystem (x86_64)",
			expectedName: "Alpine Linux - Mini Root Filesystem (x86_64)",
			expectedSlug: "alpine-linux-mini-root-filesystem-x86-64",
			testMatch:    "Alpine Linux 3.24.0 - Mini Root Filesystem (x86_64)",
			nonMatches: []string{
				"Alpine Linux 3.23.3 - Mini Root Filesystem (x86)",
			},
		},
		{
			inputTitle:   "Audacity 3.7.9 - Windows (Installer - 64bit) (64bit)",
			expectedName: "Audacity - Windows (Installer - 64bit)",
			expectedSlug: "audacity-windows-installer-64bit",
			testMatch:    "Audacity 3.8.0 - Windows (Installer - 64bit) (64bit)",
			nonMatches: []string{
				"Audacity 3.8.0 - Windows (Installer - 32bit) (32bit)",
				"Audacity 3.8.0 - Windows (Portable - 64bit) (64bit)",
			},
		},
		{
			inputTitle:   "Audacity 3.7.9 - Windows (Installer - 32bit) (32bit)",
			expectedName: "Audacity - Windows (Installer - 32bit)",
			expectedSlug: "audacity-windows-installer-32bit",
			testMatch:    "Audacity 3.8.0 - Windows (Installer - 32bit) (32bit)",
			nonMatches: []string{
				"Audacity 3.8.0 - Windows (Installer - 64bit) (64bit)",
				"Audacity 3.8.0 - Windows (Portable - 32bit) (32bit)",
			},
		},
	}

	for _, tt := range tests {
		gotName := cleanDisplayName(tt.inputTitle)
		if gotName != tt.expectedName {
			t.Errorf("cleanDisplayName(%q) = %q, want %q", tt.inputTitle, gotName, tt.expectedName)
		}

		gotSlug := createSlug(gotName)
		if gotSlug != tt.expectedSlug {
			t.Errorf("createSlug(%q) = %q, want %q", gotName, gotSlug, tt.expectedSlug)
		}

		regexStr := generateSmartRegex(tt.inputTitle)
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			t.Fatalf("failed to compile generated regex %q: %v", regexStr, err)
		}

		if !re.MatchString(tt.testMatch) {
			t.Errorf("regex %q did not match future version %q", regexStr, tt.testMatch)
		}

		for _, nonMatch := range tt.nonMatches {
			if re.MatchString(nonMatch) {
				t.Errorf("regex %q unexpectedly matched different variant %q", regexStr, nonMatch)
			}
		}
	}
}

func TestGenerateUniqueSlug(t *testing.T) {
	rules := map[string]config.TargetRule{
		"alpine-linux-standard-x86-64": {
			Key:     "alpine-linux-standard-x86-64",
			FeedURL: "https://fosstorrents.com/feed/torrents.xml",
		},
	}

	// Same name and same feed -> reuses slug
	slug1 := generateUniqueSlug(rules, "Alpine Linux - Standard (x86_64)", "https://fosstorrents.com/feed/torrents.xml")
	if slug1 != "alpine-linux-standard-x86-64" {
		t.Errorf("expected reuse of slug, got %q", slug1)
	}

	// Same name from different feed -> disambiguates
	slug2 := generateUniqueSlug(rules, "Alpine Linux - Standard (x86_64)", "https://distrowatch.com/news/torrents.xml")
	if slug2 == "alpine-linux-standard-x86-64" {
		t.Errorf("expected distinct slug for different feed, got %q", slug2)
	}
}

func TestHandleSaveSettingsSyncMode(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)

	cfg := config.LoadConfig()
	log := logger.New(100)
	qClient, _ := qbit.NewClient("http://localhost:8080", "admin", "admin")
	sEngine := syncer.New(cfg, feed.NewClient(), qClient, log)

	srv, err := NewServer(cfg, sEngine, qClient, log)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Post settings with sync_mode=scheduled
	formData := url.Values{}
	formData.Set("qbit_host", "http://localhost:8080")
	formData.Set("qbit_user", "admin")
	formData.Set("qbit_category", "foss")
	formData.Set("save_path", "/downloads")
	formData.Set("feed_urls", "https://example.com/rss.xml")
	formData.Set("check_interval", "3600")
	formData.Set("sync_mode", "scheduled")

	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if cfg.Get().SyncMode != "scheduled" {
		t.Errorf("expected SyncMode 'scheduled', got %q", cfg.Get().SyncMode)
	}

	// 2. Post settings with sync_mode=immediate
	formData.Set("sync_mode", "immediate")
	req2 := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(formData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()

	srv.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w2.Code)
	}

	if cfg.Get().SyncMode != "immediate" {
		t.Errorf("expected SyncMode 'immediate', got %q", cfg.Get().SyncMode)
	}
}

func TestExtractVersion(t *testing.T) {
	cases := map[string]string{
		"Alpine Linux 3.23.3 - Extended (x86)":                 "3.23.3",
		"alpine-extended-3.23.3-x86.iso":                       "3.23.3",
		"v1.2.3":                                               "1.2.3",
		"Kali Linux 2026.2 - Installer":                        "2026.2",
		"Cachy OS 260809 - Desktop":                            "260809",
		"CentOS 10-20260820.0 (x86_64)":                        "10-20260820.0",
		"Ubuntu 24.04.1 LTS":                                   "24.04.1",
		"Debian 13.6.0 - Netinst (amd64)":                      "13.6.0",
		"audacity-win-3.7.9-32bit.exe":                         "3.7.9",
		"audacity-win-3.7.9-64bit.exe":                         "3.7.9",
		"Audacity 3.7.9 - Windows (Installer - 64bit) (64bit)": "3.7.9",
		"NonVersionedTitle":                                    "",
	}

	for in, want := range cases {
		got := extractVersion(in)
		if got != want {
			t.Errorf("extractVersion(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestHandleToggleAutoPurge(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)

	cfg := config.LoadConfig()
	_ = cfg.SetRule(config.TargetRule{
		Key:        "test-toggle-purge",
		Name:       "Test Toggle Purge",
		TitleRegex: ".*",
		AutoPurge:  true,
		Enabled:    true,
	})

	log := logger.New(100)
	qClient, _ := qbit.NewClient("http://localhost:8080", "admin", "admin")
	sEngine := syncer.New(cfg, feed.NewClient(), qClient, log)

	srv, err := NewServer(cfg, sEngine, qClient, log)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Toggle from true to false
	req := httptest.NewRequest(http.MethodPost, "/api/rules/toggle-autopurge?key=test-toggle-purge", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if cfg.Get().Rules["test-toggle-purge"].AutoPurge != false {
		t.Errorf("expected AutoPurge to be false after toggle")
	}

	body := w.Body.String()
	if !strings.Contains(body, "Keep all") {
		t.Errorf("expected rendered HTML to contain 'Keep all'")
	}

	// 2. Toggle back from false to true
	req2 := httptest.NewRequest(http.MethodPost, "/api/rules/toggle-autopurge?key=test-toggle-purge", nil)
	w2 := httptest.NewRecorder()
	srv.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w2.Code)
	}

	if cfg.Get().Rules["test-toggle-purge"].AutoPurge != true {
		t.Errorf("expected AutoPurge to be true after second toggle")
	}

	body2 := w2.Body.String()
	if !strings.Contains(body2, "Enabled") {
		t.Errorf("expected rendered HTML to contain 'Enabled'")
	}
}

func TestBuildRulesData(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)

	cfg := config.LoadConfig()
	_ = cfg.SetRule(config.TargetRule{
		Key:        "alpine-std",
		Name:       "Alpine Linux - Standard (x86_64)",
		TitleRegex: `(?i)Alpine Linux .* Standard \(x86_64\)`,
		AutoPurge:  true,
		Enabled:    true,
	})

	log := logger.New(100)
	qClient, _ := qbit.NewClient("http://localhost:8080", "admin", "admin")
	sEngine := syncer.New(cfg, feed.NewClient(), qClient, log)

	srv, err := NewServer(cfg, sEngine, qClient, log)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	data := srv.buildRulesData(context.Background(), cfg.Get(), nil)
	view, ok := data.Rules["alpine-std"]
	if !ok {
		t.Fatalf("expected alpine-std in rules data")
	}
	if view.Key != "alpine-std" {
		t.Errorf("expected Key alpine-std, got %s", view.Key)
	}
	if !view.AutoPurge {
		t.Errorf("expected AutoPurge true")
	}
}
