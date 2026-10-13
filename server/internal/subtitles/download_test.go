package subtitles

import (
	"context"
	"errors"
	"strings"
	"testing"

	"portico.local/server/internal/identity"
)

// The download fixtures reuse the ordinary subtitle harness: one movie with a
// video file and, through Mutate, one imported text track. DownloadTracks lists
// what a listing would, so the two must name the same resource and revision.
func TestDownloadTracksListsTheImportedTextTrack(t *testing.T) {
	f := newSubtitleFixture(t)
	ctx := context.Background()
	receipt, e := f.s.Mutate(ctx, f.p, f.item.Public, uploadMutation("download-list", plainSubtitle, f.item.Token))
	if e != nil {
		t.Fatal(e)
	}
	tracks, e := f.s.DownloadTracks(ctx, f.p, f.item.Public, f.item.Token)
	if e != nil {
		t.Fatal(e)
	}
	if len(tracks) != 1 {
		t.Fatalf("downloads carry %d tracks, want 1: %+v", len(tracks), tracks)
	}
	if len(tracks[0].Formats) != 1 || tracks[0].Formats[0] != "vtt" {
		t.Fatalf("formats %v, want [vtt]", tracks[0].Formats)
	}
	catalog, e := f.s.List(ctx, f.p, f.item.Public)
	if e != nil {
		t.Fatal(e)
	}
	var listed *Resource
	for i := range catalog.Resources {
		if catalog.Resources[i].ID == tracks[0].ID {
			listed = &catalog.Resources[i]
			break
		}
	}
	if listed == nil {
		t.Fatalf("download track %q is not in the catalogue: %+v", tracks[0].ID, catalog.Resources)
	}
	if tracks[0].ID != receipt.ResourceID || tracks[0].Revision != listed.Revision {
		t.Fatalf("download track id=%q revision=%d, list revision=%d, receipt %+v", tracks[0].ID, tracks[0].Revision, listed.Revision, receipt)
	}
}

// A downloaded track is one whole WebVTT document for a file kept beside its
// video: it begins with WEBVTT, carries the imported cue text and drops the
// streaming timestamp map that a whole file has no use for.
func TestDownloadTrackVTTRendersTheWholeDocument(t *testing.T) {
	f := newSubtitleFixture(t)
	ctx := context.Background()
	receipt, e := f.s.Mutate(ctx, f.p, f.item.Public, uploadMutation("download-vtt", plainSubtitle, f.item.Token))
	if e != nil {
		t.Fatal(e)
	}
	document, e := f.s.DownloadTrackVTT(ctx, f.p, f.item.Public, receipt.ResourceID, receipt.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(string(document), "WEBVTT") {
		t.Fatalf("document does not begin with WEBVTT: %q", string(document))
	}
	if !strings.Contains(string(document), "Hello") {
		t.Fatalf("document does not carry the imported cue: %q", string(document))
	}
	if strings.Contains(string(document), "X-TIMESTAMP-MAP") {
		t.Fatalf("document carries a streaming timestamp map: %q", string(document))
	}
}

// An unknown track and a stale revision are the same answer: the track is not
// visible, and no bytes are handed over with the refusal.
func TestDownloadTrackVTTRefusesUnknownTracksAndStaleRevisions(t *testing.T) {
	f := newSubtitleFixture(t)
	ctx := context.Background()
	receipt, e := f.s.Mutate(ctx, f.p, f.item.Public, uploadMutation("download-stale", plainSubtitle, f.item.Token))
	if e != nil {
		t.Fatal(e)
	}
	for name, tc := range map[string]struct {
		id       string
		revision int64
	}{
		"unknown id":     {"no-such-track", 1},
		"stale revision": {receipt.ResourceID, receipt.Revision + 1},
	} {
		document, e := f.s.DownloadTrackVTT(ctx, f.p, f.item.Public, tc.id, tc.revision)
		if !errors.Is(e, identity.ErrNotVisible) {
			t.Fatalf("%s: error %v, want ErrNotVisible", name, e)
		}
		if document != nil {
			t.Fatalf("%s: returned %d bytes with the refusal", name, len(document))
		}
	}
}

// A personal track belongs to the viewer who imported it: it is neither listed
// for another viewer nor readable by them, even though both can see the file.
func TestDownloadTracksAndVTTKeepPersonalTracksPrivate(t *testing.T) {
	f := newSubtitleFixture(t)
	ctx := context.Background()
	receipt, e := f.s.Mutate(ctx, f.p, f.item.Public, uploadMutation("download-private", plainSubtitle, f.item.Token))
	if e != nil {
		t.Fatal(e)
	}
	other := f.p
	other.ProfileID = "other"
	other.Role = "viewer"
	tracks, e := f.s.DownloadTracks(ctx, other, f.item.Public, f.item.Token)
	if e != nil {
		t.Fatal(e)
	}
	if len(tracks) != 0 {
		t.Fatalf("another viewer's download lists %d personal tracks: %+v", len(tracks), tracks)
	}
	document, e := f.s.DownloadTrackVTT(ctx, other, f.item.Public, receipt.ResourceID, receipt.Revision)
	if !errors.Is(e, identity.ErrNotVisible) {
		t.Fatalf("another viewer read a personal track: %v", e)
	}
	if document != nil {
		t.Fatalf("another viewer received %d bytes with the refusal", len(document))
	}
}
