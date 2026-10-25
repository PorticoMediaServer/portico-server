package operations

import (
	"context"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/segmentmarkers"
	"testing"
)

func TestSegmentConfidenceSavedPolicyAndOlderDocumentPreservation(t *testing.T) {
	s, owner, auth := consoleFixture(t)
	ctx := context.Background()
	doc, err := s.Settings(ctx, auth)
	if err != nil {
		t.Fatal(err)
	}
	values := doc.Requested
	values.SegmentViewerConfidencePercent = 70
	values.SegmentAutomaticConfidencePercent = 95
	saved, err := s.ApplySettings(ctx, owner, auth, SettingsChange{doc.Revision, "segment-confidence", values})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := segmentmarkers.ReadConfidencePolicy(ctx, snapshot.Tx())
	snapshot.Rollback()
	if err != nil || policy.ViewerPercent != 70 || policy.AutomaticPercent != 95 || policy.AutomaticSafe(false, .9, "season_audio_match:intro:v1") {
		t.Fatal(policy, err)
	}
	// A client from before these optional fields existed cannot reset their values.
	values = saved.Requested
	values.SegmentViewerConfidencePercent = 0
	values.SegmentAutomaticConfidencePercent = 0
	saved, err = s.ApplySettings(ctx, owner, auth, SettingsChange{saved.Revision, "older-settings-client", values})
	if err != nil || saved.Requested.SegmentViewerConfidencePercent != 70 || saved.Requested.SegmentAutomaticConfidencePercent != 95 {
		t.Fatal(saved, err)
	}
	for _, invalid := range []int{1, 49, 71, 100} {
		values = saved.Requested
		values.SegmentViewerConfidencePercent = invalid
		if _, err = s.ApplySettings(ctx, owner, auth, SettingsChange{saved.Revision, "invalid-confidence", values}); err == nil {
			t.Fatal("invalid confidence accepted", invalid)
		}
	}
}
