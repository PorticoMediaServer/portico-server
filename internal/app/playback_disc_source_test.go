package app

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestLogicalDiscPlaybackSourcesReturnStableTruthfulReason(t *testing.T) {
	for _, kind := range []string{"dvd-structure", "bluray-structure", "disc-image"} {
		item := MediaItem{MediaFiles: []MediaFileVersion{{ID: "disc", SourceType: kind, Selected: true, Available: true}}}
		err := resolveLogicalDiscPlaybackSource(item)
		if !errors.Is(err, errLogicalDiscPlaybackUnsupported) {
			t.Fatalf("%s error = %v", kind, err)
		}
		startErr := playbackSourceStartError("playback_source_unavailable", err)
		if startErr.status != http.StatusUnprocessableEntity || startErr.code != "logical_disc_playback_unsupported" {
			t.Fatalf("%s response = %#v", kind, startErr)
		}
		for _, claim := range []string{"libdvdread/libbluray", "unencrypted", "menus", "encrypted discs", "auto-mounting"} {
			if !strings.Contains(startErr.message, claim) {
				t.Fatalf("%s reason %q does not contain %q", kind, startErr.message, claim)
			}
		}
	}
}

func TestUncommonFilesRemainNormalPlannerInputs(t *testing.T) {
	for _, path := range []string{"feature.vob", "feature.flv", "feature.f4v"} {
		item := MediaItem{SourceURL: path, MediaFiles: []MediaFileVersion{{ID: "file", Path: path, SourceType: "file", Selected: true, Available: true}}}
		if kind := logicalDiscPlaybackSourceType(item); kind != "" {
			t.Fatalf("%s classified as %s", path, kind)
		}
		if err := resolveLogicalDiscPlaybackSource(item); err != nil {
			t.Fatalf("%s was removed from normal planning: %v", path, err)
		}
	}
}

func TestLogicalDiscResolutionUsesOnlySelectedVersion(t *testing.T) {
	item := MediaItem{MediaFiles: []MediaFileVersion{
		{ID: "disc", SourceType: "disc-image", Available: true},
		{ID: "file", SourceType: "file", Selected: true, Available: true},
	}}
	if kind := logicalDiscPlaybackSourceType(item); kind != "" {
		t.Fatalf("unselected logical disc affected selected file: %s", kind)
	}
}
