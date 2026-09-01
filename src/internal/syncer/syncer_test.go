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
	_ = cfg.UpdateSettings("http://localhost:8080", "admin", "admin", "foss", "/tmp", []string{ts1.URL, ts2.URL}, 600, true, false)

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

	// First item should be Distro A from Feed 1 with priority 1
	if items[0].FeedPriority != 1 || items[0].SourceFeedURL != ts1.URL {
		t.Errorf("expected item 0 to have priority 1 and source feed 1, got priority %d url %s", items[0].FeedPriority, items[0].SourceFeedURL)
	}

	// Distro B should come from ts1.URL (priority 1) because ts1 was evaluated first
	var distroB feed.Item
	for _, it := range items {
		if it.Title == "Distro B v1.0" {
			distroB = it
			break
		}
	}
	if distroB.FeedPriority != 1 || distroB.SourceFeedURL != ts1.URL {
		t.Errorf("expected Distro B to be prioritized from Feed 1, got priority %d", distroB.FeedPriority)
	}

	// Check GetFeedInfos
	infos := syncer.GetFeedInfos(context.Background())
	if len(infos) != 2 {
		t.Fatalf("expected 2 feed infos, got %d", len(infos))
	}
	if infos[0].Priority != 1 || infos[0].Count != 2 {
		t.Errorf("expected feed 0 priority 1 count 2, got %v", infos[0])
	}
	if infos[1].Priority != 2 || infos[1].Count != 1 {
		t.Errorf("expected feed 1 priority 2 count 1, got %v", infos[1])
	}
}

func TestSizeResolutionAndCaching(t *testing.T) {
	// Server serving bencoded .torrent file
	torrentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bencoded := "d8:announce22:http://tracker.com/ann4:infod6:lengthi3221225472e4:name10:distro.isoee"
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write([]byte(bencoded))
	}))
	defer torrentServer.Close()

	// Feed server
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Torrent Feed</title>
    <item>
      <title>Distro ISO 2026</title>
      <guid>distro-2026</guid>
      <link>` + torrentServer.URL + `/distro.torrent</link>
      <enclosure url="` + torrentServer.URL + `/distro.torrent" type="application/x-bittorrent"/>
    </item>
  </channel>
</rss>`
		_, _ = w.Write([]byte(xml))
	}))
	defer feedServer.Close()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("CONFIG_PATH", configPath)

	cfg := config.LoadConfig()
	_ = cfg.UpdateSettings("http://localhost:8080", "admin", "admin", "foss", "/tmp", []string{feedServer.URL}, 600, true, false)

	log := logger.New(200)
	qClient, _ := qbit.NewClient("http://localhost:8080", "admin", "admin")
	s := New(cfg, feed.NewClient(), qClient, log)

	items, err := s.GetCachedFeed(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error fetching feed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// Size resolver runs in background; let's test synchronous resolve as well as cache check
	sz, err := s.sizeResolver.Resolve(context.Background(), torrentServer.URL+"/distro.torrent")
	if err != nil {
		t.Fatalf("unexpected error resolving size: %v", err)
	}
	if sz != 3221225472 {
		t.Errorf("expected size 3221225472, got %d", sz)
	}

	// Verify it is cached
	cachedSz, found := s.sizeResolver.GetCached(torrentServer.URL + "/distro.torrent")
	if !found || cachedSz != 3221225472 {
		t.Errorf("expected cached size 3221225472, got found=%v sz=%d", found, cachedSz)
	}
}
