package feed

import (
	"testing"

	"github.com/mmcdole/gofeed"
)

func TestExtractFilenameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{
			url:      "https://fosstorrents.com/direct-files/archlinux-2026.08.01-x86_64.iso.torrent",
			expected: "archlinux-2026.08.01-x86_64.iso",
		},
		{
			url:      "https://fosstorrents.com/direct-files/debian-12.5.0-amd64-netinst.iso.torrent?download=1",
			expected: "debian-12.5.0-amd64-netinst.iso",
		},
		{
			url:      "https://fosstorrents.com/direct-files/cachyos-desktop-linux-260824.iso",
			expected: "cachyos-desktop-linux-260824.iso",
		},
	}

	for _, tt := range tests {
		got := ExtractFilenameFromURL(tt.url)
		if got != tt.expected {
			t.Errorf("ExtractFilenameFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestIsTorrentMatching(t *testing.T) {
	tests := []struct {
		torrentName  string
		expectedName string
		shouldMatch  bool
	}{
		{
			torrentName:  "archlinux-2026.08.01-x86_64.iso",
			expectedName: "archlinux-2026.08.01-x86_64.iso",
			shouldMatch:  true,
		},
		{
			torrentName:  "archlinux-2026.08.01-x86_64",
			expectedName: "archlinux-2026.08.01-x86_64.iso",
			shouldMatch:  true,
		},
		{
			torrentName:  "archlinux-2026.07.01-x86_64.iso",
			expectedName: "archlinux-2026.08.01-x86_64.iso",
			shouldMatch:  false,
		},
		{
			torrentName:  "debian-12.5.0-amd64-netinst",
			expectedName: "debian-12.5.0-amd64-netinst.iso",
			shouldMatch:  true,
		},
		{
			torrentName:  "debian-12.4.0-amd64-netinst.iso",
			expectedName: "debian-12.5.0-amd64-netinst.iso",
			shouldMatch:  false,
		},
	}

	for _, tt := range tests {
		got := IsTorrentMatching(tt.torrentName, tt.expectedName)
		if got != tt.shouldMatch {
			t.Errorf("IsTorrentMatching(%q, %q) = %v, want %v", tt.torrentName, tt.expectedName, got, tt.shouldMatch)
		}
	}
}

func TestIsSameFamily(t *testing.T) {
	tests := []struct {
		torrentName  string
		expectedName string
		shouldMatch  bool
	}{
		{
			torrentName:  "archlinux-2026.07.01-x86_64.iso",
			expectedName: "archlinux-2026.08.01-x86_64.iso",
			shouldMatch:  true,
		},
		{
			torrentName:  "debian-12.4.0-amd64-netinst.iso",
			expectedName: "debian-12.5.0-amd64-netinst.iso",
			shouldMatch:  true,
		},
		{
			torrentName:  "debian-12.5.0-amd64-edu-netinst.iso",
			expectedName: "debian-12.5.0-amd64-netinst.iso",
			shouldMatch:  false,
		},
		{
			torrentName:  "cachyos-desktop-linux-260809.iso",
			expectedName: "cachyos-desktop-linux-260824.iso",
			shouldMatch:  true,
		},
		{
			torrentName:  "ubuntu-24.04-desktop-amd64.iso",
			expectedName: "debian-12.5.0-amd64-netinst.iso",
			shouldMatch:  false,
		},
	}

	for _, tt := range tests {
		got := IsSameFamily(tt.torrentName, tt.expectedName)
		if got != tt.shouldMatch {
			t.Errorf("IsSameFamily(%q, %q) = %v, want %v", tt.torrentName, tt.expectedName, got, tt.shouldMatch)
		}
	}
}

func TestExtractTorrentURL(t *testing.T) {
	// 1. Enclosure test
	item1 := &gofeed.Item{
		Enclosures: []*gofeed.Enclosure{
			{URL: "https://example.com/test.torrent", Type: "application/x-bittorrent"},
		},
	}
	if url := extractTorrentURL(item1); url != "https://example.com/test.torrent" {
		t.Errorf("expected enclosure URL, got %s", url)
	}

	// 2. AcademicTorrents details page link
	item2 := &gofeed.Item{
		Link: "https://academictorrents.com/details/dcb9178653b651c7ca4526e11fa8e22f74e2fd7a",
	}
	expectedAc := "https://academictorrents.com/download/dcb9178653b651c7ca4526e11fa8e22f74e2fd7a.torrent"
	if url := extractTorrentURL(item2); url != expectedAc {
		t.Errorf("expected %s, got %s", expectedAc, url)
	}

	// 3. HTML Description link (e.g. Wikimedia dumps)
	item3 := &gofeed.Item{
		Link:        "http://download.wikimedia.org/ukwiki/20260801",
		Description: `<a href="http://download.wikimedia.org/ukwiki/20260801/ukwiki-20260801-all-titles-in-ns0.gz">ukwiki-20260801-all-titles-in-ns0.gz</a>`,
	}
	expectedWk := "http://download.wikimedia.org/ukwiki/20260801/ukwiki-20260801-all-titles-in-ns0.gz"
	if url := extractTorrentURL(item3); url != expectedWk {
		t.Errorf("expected %s, got %s", expectedWk, url)
	}
}

func TestExtractSize(t *testing.T) {
	// Custom field <size>
	item1 := &gofeed.Item{
		Custom: map[string]string{
			"size": "71000122956",
		},
	}
	if sz := extractSize(item1); sz != 71000122956 {
		t.Errorf("expected 71000122956, got %d", sz)
	}

	// Enclosure length
	item2 := &gofeed.Item{
		Enclosures: []*gofeed.Enclosure{
			{Length: "4294967296"},
		},
	}
	if sz := extractSize(item2); sz != 4294967296 {
		t.Errorf("expected 4294967296, got %d", sz)
	}

	// Description parsing
	item3 := &gofeed.Item{
		Description: "Latest ISO release for x86_64. Size: 2.5 GB. Enjoy!",
	}
	if sz := extractSize(item3); sz != int64(2.5*1024*1024*1024) {
		t.Errorf("expected %d, got %d", int64(2.5*1024*1024*1024), sz)
	}
}

func TestParseSizeFromText(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"File Size: 1.5 GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"Download length: 700 MB", 700 * 1024 * 1024},
		{"Size: 50 GiB", 50 * 1024 * 1024 * 1024},
		{"No size here", 0},
	}

	for _, tt := range tests {
		got := parseSizeFromText(tt.input)
		if got != tt.expected {
			t.Errorf("parseSizeFromText(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
