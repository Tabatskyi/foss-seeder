package feed

import (
	"testing"
)

func TestParseTorrentSize_SingleFile(t *testing.T) {
	// bencoded dict: d8:announce22:http://tracker.com/ann4:infod6:lengthi4294967296e4:name10:ubuntu.isoee
	bencoded := "d8:announce22:http://tracker.com/ann4:infod6:lengthi4294967296e4:name10:ubuntu.isoee"
	size, err := ParseTorrentSizeFromBytes([]byte(bencoded))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := int64(4294967296) // 4 GB
	if size != expected {
		t.Errorf("got size %d, want %d", size, expected)
	}
}

func TestParseTorrentSize_MultiFile(t *testing.T) {
	// bencoded dict with info.files: [ {length: 1000, path: [file1]}, {length: 2500, path: [file2]}, {length: 500, path: [file3]} ] => total 4000
	bencoded := "d4:infod5:filesld6:lengthi1000e4:pathl5:file1eed6:lengthi2500e4:pathl5:file2eed6:lengthi500e4:pathl5:file3eee4:name9:test-distee"
	size, err := ParseTorrentSizeFromBytes([]byte(bencoded))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := int64(4000)
	if size != expected {
		t.Errorf("got size %d, want %d", size, expected)
	}
}

func TestParseTorrentSize_InvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte("")},
		{"corrupt", []byte("not bencoded at all")},
		{"no info dict", []byte("d4:name4:teste")},
		{"empty dict", []byte("de")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTorrentSizeFromBytes(tt.data)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}
