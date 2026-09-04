package web

import (
	"regexp"
	"testing"

	"foss-seeder/internal/config"
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
