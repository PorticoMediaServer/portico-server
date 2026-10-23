package metadata

import (
	"context"
	"database/sql"
	"testing"
)

func TestGenericArtworkRepairPreservesGeneratedSelectionAndProvenance(t *testing.T) {
	s, c, target, _ := episodePublicationFixture(t)
	ctx := context.Background()
	actor := MBActor{Authority: "local", AccountID: "owner", ProfileID: "profile"}
	allow := func(*sql.Tx) error { return nil }
	picture := preparePublication(t, s, func(context.Context, *sql.Tx) error { return nil })
	state, err := s.publishEpisodeThumbnail(ctx, capturePublication(t, s, target), picture, &thumbnailPublicationAuthority{}, true, actor, allow)
	if err != nil {
		t.Fatal(err)
	}
	candidate := state.Snapshot.Artwork[0].CandidateID
	var fence, origin, digest string
	if err = c.DB.QueryRow(`SELECT c.source_fence,c.origin,a.digest FROM artwork_candidates c JOIN artwork_selections a ON a.candidate_id=c.id WHERE c.id=?`, candidate).Scan(&fence, &origin, &digest); err != nil {
		t.Fatal(err)
	}
	title := "Owner changed the title"
	state, err = s.Repair(ctx, target, RepairCommand{Action: "edit", ExpectedRevision: state.Revision, Fields: map[string]RepairFieldEdit{"title": {Value: &title}}}, actor, allow)
	if err != nil {
		t.Fatal(err)
	}
	c.Exec(`UPDATE library_sources SET generation=generation+1 WHERE id='episodes'`)
	state, err = s.RepairState(ctx, target, allow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Repair(ctx, target, RepairCommand{Action: "repair_assets", ExpectedRevision: state.Revision}, actor, allow); err != nil {
		t.Fatal(err)
	}
	var afterFence, afterOrigin, afterDigest string
	var locked bool
	if err = c.DB.QueryRow(`SELECT c.source_fence,c.origin,a.digest,a.locked FROM artwork_candidates c JOIN artwork_selections a ON a.candidate_id=c.id WHERE c.id=?`, candidate).Scan(&afterFence, &afterOrigin, &afterDigest, &locked); err != nil {
		t.Fatal(err)
	}
	var jobs int
	if err = c.DB.QueryRow(`SELECT count(*) FROM artwork_jobs WHERE candidate_id=?`, candidate).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || afterFence != fence || afterOrigin != origin || afterDigest != digest || !locked {
		t.Fatalf("generic repair tried to reacquire generated art: jobs=%d fence=%s/%s origin=%s/%s digest=%s/%s locked=%t", jobs, fence, afterFence, origin, afterOrigin, digest, afterDigest, locked)
	}
}

func TestMissingArtworkDoesNotRequeueALegacyGeneratedFetchJob(t *testing.T) {
	s, c, target, _ := episodePublicationFixture(t)
	ctx := context.Background()
	picture := preparePublication(t, s, func(context.Context, *sql.Tx) error { return nil })
	state, err := s.publishEpisodeThumbnail(ctx, capturePublication(t, s, target), picture, &thumbnailPublicationAuthority{}, true, MBActor{Authority: "local", AccountID: "owner", ProfileID: "profile"}, func(*sql.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	candidate := state.Snapshot.Artwork[0].CandidateID
	// A legacy malformed generic job is a negative fixture. The selected bytes
	// above were genuinely installed through the generated-art publisher.
	c.Exec(`INSERT INTO artwork_jobs(id,kind,entity_id,role,subject,candidate_id,origin,provider,source_fence,status,created_at) SELECT 'legacy-generated',kind,entity_id,role,subject,id,origin,provider,source_fence,'complete',observed_at FROM artwork_candidates WHERE id=?`, candidate)
	s.queueMissingArtwork(target, "still", "")
	if err = s.RepairMissingArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = c.DB.QueryRow(`SELECT status FROM artwork_jobs WHERE id='legacy-generated'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "complete" {
		t.Fatalf("a missing generated object was queued for generic fetching: %s", status)
	}
}
