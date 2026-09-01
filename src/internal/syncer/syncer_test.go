package syncer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"

	"foss-seeder/internal/config"
	"foss-seeder/internal/feed"
	"foss-seeder/internal/logger"
	"foss-seeder/internal/qbit"
)

func TestRegexFamilyMatching(t *testing.T) {
	tests := []struct {
		ruleRegex string
		feedTitle string
		matched   bool
	}{
		{
			ruleRegex: `ArchLinux .*`,
			feedTitle: "ArchLinux 2026.08.01",
			matched:   true,
		},
		{
			ruleRegex: `Cachy OS .* - Desktop \(x86_64\)`,
			feedTitle: "Cachy OS 260824 - Desktop (x86_64)",
			matched:   true,
		},
		{
			ruleRegex: `Debian .* - Netinst \(amd64\)`,
			feedTitle: "Debian 12.5.0 - Netinst (amd64)",
			matched:   true,
		},
		{
			ruleRegex: `Kali Linux .* - Installer \(amd64\) \(amd64\)`,
			feedTitle: "Kali Linux 2026.2 - Installer (amd64) (amd64)",
			matched:   true,
		},
		{
			ruleRegex: `CentOS 10-.* \(x86_64\) \(x86_64\)`,
			feedTitle: "CentOS 10-20260820.0 (x86_64) (x86_64)",
			matched:   true,
		},
		{
			ruleRegex: `Ubuntu .* Desktop \(amd64\)`,
			feedTitle: "Ubuntu 24.04 LTS Desktop (amd64)",
			matched:   true,
		},
		{
			ruleRegex: `ArchLinux .*`,
			feedTitle: "Ubuntu 24.04 LTS Desktop (amd64)",
			matched:   false,
		},
	}

	for _, tt := range tests {
		re, err := regexp.Compile("(?i)" + tt.ruleRegex)
		if err != nil {
			t.Fatalf("failed to compile regex %q: %v", tt.ruleRegex, err)
		}

		got := re.MatchString(tt.feedTitle)
		if got != tt.matched {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.ruleRegex, tt.feedTitle, got, tt.matched)
		}
	}
}

func TestObsoleteDetection(t *testing.T) {
	latestURL := "https://fosstorrents.com/direct-files/archlinux-2026.08.01-x86_64.iso.torrent"
	expectedName := feed.ExtractFilenameFromURL(latestURL)

	oldTorrentName := "archlinux-2026.07.01-x86_64.iso"
	currentTorrentName := "archlinux-2026.08.01-x86_64.iso"

	if feed.IsTorrentMatching(currentTorrentName, expectedName) != true {
		t.Errorf("expected current torrent to match latest")
	}

	if feed.IsTorrentMatching(oldTorrentName, expectedName) != false {
		t.Errorf("expected old torrent NOT to match latest, indicating it should be purged")
	}
}

func TestMultiFeedFetchAndDeduplication(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed 1</title>
    <item>
      <title>Distro A v1.0</title>
      <guid>guid-a-1</guid>
      <enclosure url="https://example.com/distro-a.torrent" type="application/x-bittorrent"/>
    </item>
    <item>
      <title>Distro B v1.0</title>
      <guid>guid-b-1</guid>
      <enclosure url="https://example.com/distro-b.torrent" type="application/x-bittorrent"/>
    </item>
  </channel>
</rss>`))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed 2</title>
    <item>
      <title>Distro B v1.0</title>
      <guid>guid-b-1</guid>
      <enclosure url="https://example.com/distro-b.torrent" type="application/x-bittorrent"/>
    </item>
    <item>
      <title>Distro C v1.0</title>
      <guid>guid-c-1</guid>
      <enclosure url="https://example.com/distro-c.torrent" type="application/x-bittorrent"/>
    </item>
  </channel>
</rss>`))
	}))
	defer ts2.Close()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)

	cfg := config.LoadConfig()
	_ = cfg.UpdateSettings("http://localhost:8080", "admin", "admin", "foss", "/tmp", []string{ts1.URL, ts2.URL}, 600, true)

	log := logger.New(200)
	qClient, _ := qbit.NewClient("http://localhost:8080", "admin", "admin")
	syncer := New(cfg, feed.NewClient(), qClient, log)

	items, err := syncer.GetCachedFeed(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error fetching feeds: %v", err)
	}

	// Should aggregate Distro A, Distro B, Distro C (total 3 unique items)
	if len(items) != 3 {
		t.Fatalf("expected 3 aggregated items, got %d", len(items))
	}
}
