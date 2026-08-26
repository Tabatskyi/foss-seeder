package web

import (
	"regexp"
	"testing"
)

func TestCleanDisplayNameAndSlug(t *testing.T) {
	tests := []struct {
		inputTitle   string
		expectedName string
		expectedSlug string
		testMatch    string
	}{
		{
			inputTitle:   "Alpine Linux 3.23.3 - Extended (x86) (x86)",
			expectedName: "Alpine Linux - Extended (x86)",
			expectedSlug: "alpine-linux-extended-x86",
			testMatch:    "Alpine Linux 3.24.0 - Extended (x86) (x86)",
		},
		{
			inputTitle:   "Debian 13.6.0 - Netinst (amd64)",
			expectedName: "Debian - Netinst (amd64)",
			expectedSlug: "debian-netinst-amd64",
			testMatch:    "Debian 14.0.0 - Netinst (amd64)",
		},
		{
			inputTitle:   "Cachy OS 260809 - Desktop (x86_64)",
			expectedName: "Cachy OS - Desktop (x86_64)",
			expectedSlug: "cachy-os-desktop-x86-64",
			testMatch:    "Cachy OS 260901 - Desktop (x86_64)",
		},
		{
			inputTitle:   "Kali Linux 2026.2 - Installer (amd64) (amd64)",
			expectedName: "Kali Linux - Installer (amd64)",
			expectedSlug: "kali-linux-installer-amd64",
			testMatch:    "Kali Linux 2026.3 - Installer (amd64) (amd64)",
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
	}
}
