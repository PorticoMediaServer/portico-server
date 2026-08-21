package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPlaybackSubtitleFileIsBounded(t *testing.T) {
	server := newScannerTestServer(t)
	path := filepath.Join(t.TempDir(), "captions.vtt")
	want := []byte("WEBVTT\n\n00:00.000 --> 00:01.000\nHello\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := server.readPlaybackSubtitleFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("subtitle = %q", got)
	}

	large := filepath.Join(t.TempDir(), "oversized.vtt")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(playbackSubtitleFileLimit + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := server.readPlaybackSubtitleFile(context.Background(), large); err == nil || errors.Is(err, errPlaybackStorageTransient) {
		t.Fatalf("oversized subtitle returned %v", err)
	}
}
