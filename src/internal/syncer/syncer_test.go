package syncer

import (
	"regexp"
	"testing"

	"foss-seeder/internal/feed"
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
