package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIdentityReconciliationReviewHandlersRequireAdminAndResolveKeepSeparate(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Review Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	insertIdentityMedia(t, server, "med_review_subject", library.ID, "movie", "", now)
	insertIdentityMedia(t, server, "med_review_candidate", library.ID, "movie", "", now)
	insertIdentityReview(t, server, "idrev_keep", "media", library.ID, "med_review_subject", []string{"med_review_candidate"}, now)

	request := httptest.NewRequest(http.MethodGet, "/api/identity-reconciliation/reviews", nil)
	response := httptest.NewRecorder()
	server.handleIdentityReconciliationReviews(response, request, User{Permissions: map[string]bool{}})
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin list status=%d body=%s", response.Code, response.Body.String())
	}

	admin := User{ID: "usr_identity_admin", AccountID: "usr_identity_admin", ProfileID: "usr_identity_admin", ProfileIsPrimary: true, Email: "admin@example.test", Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()}
	request = httptest.NewRequest(http.MethodGet, "/api/identity-reconciliation/reviews?domain=media&status=open", nil)
	response = httptest.NewRecorder()
	server.handleIdentityReconciliationReviews(response, request, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("admin list status=%d body=%s", response.Code, response.Body.String())
	}
	var page IdentityReconciliationReviewPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode review page: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "idrev_keep" || page.Items[0].SubjectID != "med_review_subject" {
		t.Fatalf("review page=%#v", page)
	}

	body := bytes.NewBufferString(`{"resolution":"keep_separate","note":"Distinct releases"}`)
	request = httptest.NewRequest(http.MethodPost, "/api/identity-reconciliation/reviews/idrev_keep/resolve", body)
	response = httptest.NewRecorder()
	server.handleIdentityReconciliationReviewRoute(response, request, admin)
	if response.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", response.Code, response.Body.String())
	}
	var resolved IdentityReconciliationReview
	if err := json.Unmarshal(response.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("decode resolved review: %v", err)
	}
	if resolved.Status != "resolved" || resolved.Resolution != identityResolutionKeepSeparate || resolved.ResolutionNote != "Distinct releases" || resolved.ResolvedByUserID != admin.ID {
		t.Fatalf("resolved review=%#v", resolved)
	}

	body = bytes.NewBufferString(`{"resolution":"keep_separate"}`)
	request = httptest.NewRequest(http.MethodPost, "/api/identity-reconciliation/reviews/idrev_keep/resolve", body)
	response = httptest.NewRecorder()
	server.handleIdentityReconciliationReviewRoute(response, request, admin)
	if response.Code != http.StatusConflict {
		t.Fatalf("second resolution status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestResolveMediaParentIdentityReparentsChildrenAndAliases(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Shows", Type: "show", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	insertIdentityMedia(t, server, "med_show_subject", library.ID, "show", "", now)
	insertIdentityMedia(t, server, "med_show_candidate", library.ID, "show", "", now)
	insertIdentityMedia(t, server, "med_subject_season", library.ID, "season", "med_show_subject", now)
	if _, err := server.db.Exec(`
		INSERT INTO media_scanner_identity_aliases (library_id, scanner_key, media_id, first_seen_at, last_seen_at)
		VALUES (?, 'parent-scanner:show:subject', 'med_show_subject', ?, ?)`, library.ID, now, now); err != nil {
		t.Fatalf("insert subject alias: %v", err)
	}
	insertIdentityReview(t, server, "idrev_parent_merge", "media_parent", library.ID, "med_show_subject", []string{"med_show_candidate"}, now)

	resolved, err := server.resolveIdentityReconciliationReview(t.Context(), "idrev_parent_merge", "usr_admin", IdentityReconciliationResolveRequest{Resolution: identityResolutionMerge, SelectedCandidateID: "med_show_candidate"})
	if err != nil {
		t.Fatalf("resolve parent merge: %v", err)
	}
	if resolved.Status != "resolved" || resolved.SelectedCandidateID != "med_show_candidate" {
		t.Fatalf("resolved parent review=%#v", resolved)
	}
	var parentID, aliasID string
	if err := server.db.QueryRow(`SELECT parent_id FROM media_items WHERE id = 'med_subject_season'`).Scan(&parentID); err != nil {
		t.Fatalf("load reparented season: %v", err)
	}
	if err := server.db.QueryRow(`SELECT media_id FROM media_scanner_identity_aliases WHERE scanner_key = 'parent-scanner:show:subject'`).Scan(&aliasID); err != nil {
		t.Fatalf("load moved alias: %v", err)
	}
	if parentID != "med_show_candidate" || aliasID != "med_show_candidate" {
		t.Fatalf("merge parent=%q alias=%q", parentID, aliasID)
	}
	var subjectCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id = 'med_show_subject'`).Scan(&subjectCount); err != nil || subjectCount != 0 {
		t.Fatalf("subject count=%d err=%v", subjectCount, err)
	}
}

func TestResolveMediaIdentityPreservesDurableUserState(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	insertIdentityMedia(t, server, "med_state_subject", library.ID, "movie", "", now)
	insertIdentityMedia(t, server, "med_state_candidate", library.ID, "movie", "", now)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, email, display_name, role, permissions_json, created_at, updated_at)
		VALUES ('usr_state', 'state@example.test', 'State', 'user', '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert state user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, favorite, updated_at)
		VALUES ('usr_state', 'usr_state', 'med_state_subject', 1, ?)`, now); err != nil {
		t.Fatalf("insert durable state: %v", err)
	}
	insertIdentityReview(t, server, "idrev_state", "media", library.ID, "med_state_subject", []string{"med_state_candidate"}, now)
	_, err = server.resolveIdentityReconciliationReview(t.Context(), "idrev_state", "usr_admin", IdentityReconciliationResolveRequest{Resolution: identityResolutionMerge, SelectedCandidateID: "med_state_candidate"})
	if err != nil {
		t.Fatalf("merge with durable state: %v", err)
	}
	var favorite int
	if err := server.db.QueryRow(`SELECT favorite FROM user_media_state WHERE profile_id='usr_state' AND media_id='med_state_candidate'`).Scan(&favorite); err != nil || favorite != 1 {
		t.Fatalf("merged favorite=%d err=%v", favorite, err)
	}
}

func TestMediaMergePolicyInventoriesEverySchemaReference(t *testing.T) {
	server := newScannerTestServer(t)
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if err := validateMediaMergePolicyTx(tx); err != nil {
		t.Fatal(err)
	}
}

func TestMediaMergePolicyRejectsUnknownSchemaReference(t *testing.T) {
	server := newScannerTestServer(t)
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE unexpected_media_reference (id TEXT PRIMARY KEY, media_id TEXT NOT NULL REFERENCES media_items(id))`); err != nil {
		t.Fatalf("create unknown reference: %v", err)
	}
	err = validateMediaMergePolicyTx(tx)
	if err == nil || !strings.Contains(err.Error(), "unexpected_media_reference") {
		t.Fatalf("expected unknown-reference failure, got %v", err)
	}
}

func TestResolveMediaIdentityCreatesCoherentMergeRevisionAndRebuildsProjections(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Merge Evidence", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertIdentityMedia(t, server, "med_merge_evidence_subject", library.ID, "movie", "", now)
	insertIdentityMedia(t, server, "med_merge_evidence_target", library.ID, "movie", "", now)
	for _, mediaID := range []string{"med_merge_evidence_subject", "med_merge_evidence_target"} {
		if _, err := server.db.Exec(`INSERT INTO media_metadata_revisions
			(id,media_id,revision,base_revision,state,trigger_kind,started_at,completed_at)
			VALUES (?,?,1,0,'applied','scanner',?,?)`, "mrev_"+mediaID, mediaID, now, now); err != nil {
			t.Fatalf("seed revision for %s: %v", mediaID, err)
		}
	}
	if _, err := server.db.Exec(`UPDATE media_items SET metadata_revision=1, genres_json='["Drama"]' WHERE id IN ('med_merge_evidence_subject','med_merge_evidence_target')`); err != nil {
		t.Fatalf("seed canonical metadata: %v", err)
	}
	for _, row := range []struct {
		mediaID, externalID string
		confidence          float64
	}{
		{"med_merge_evidence_subject", "subject-id", .99},
		{"med_merge_evidence_target", "target-id", .50},
	} {
		if _, err := server.db.Exec(`INSERT INTO media_provider_ids
			(media_id,provider,external_id,external_type,confidence,source,status,evidence_revision,updated_at)
			VALUES (?, 'tmdb', ?, 'movie', ?, 'test', 'accepted', 1, ?)`, row.mediaID, row.externalID, row.confidence, now); err != nil {
			t.Fatalf("seed provider identity: %v", err)
		}
	}
	if _, err := server.db.Exec(`INSERT INTO media_search (media_id,title) VALUES
		('med_merge_evidence_subject','stale subject'),('med_merge_evidence_target','stale target')`); err != nil {
		t.Fatalf("seed search projections: %v", err)
	}
	accountID := "usr_merge_fixture"
	if _, err := server.db.Exec(`INSERT INTO users (id,email,display_name,role,permissions_json,created_at,updated_at) VALUES (?, 'merge-fixture@example.test', 'Merge Fixture', 'owner', '{}', ?, ?)`, accountID, now, now); err != nil {
		t.Fatalf("seed fixture account: %v", err)
	}
	var profileID string
	if err := server.db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND is_primary = 1`, accountID).Scan(&profileID); err != nil {
		t.Fatalf("load fixture primary profile: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,current_entry_id,media_id,started_at,last_seen_at) VALUES ('play_merge_prepared',?,?,'entry-current','med_merge_evidence_target',?,?)`, accountID, profileID, now, now); err != nil {
		t.Fatalf("seed prepared source session: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO playback_prepared_handoffs
		(id,user_id,profile_id,source_session_id,media_id,current_entry_id,queue_entries_json,created_at,expires_at)
		VALUES ('prep_merge_entries',?,?,'play_merge_prepared','med_merge_evidence_target','entry-current',?, ?, ?)`, accountID, profileID,
		`[{"entryId":"entry-subject-1","mediaId":"med_merge_evidence_subject"},{"entryId":"entry-target","mediaId":"med_merge_evidence_target"},{"entryId":"entry-subject-2","mediaId":"med_merge_evidence_subject"}]`, now, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed prepared occurrence queue: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO download_preparations
		(id,server_id,account_id,profile_id,authorization_revision,media_id,quality_profile,state,created_at,updated_at)
		VALUES
		('download_merge_subject','server-merge',?,?, 'revision', 'med_merge_evidence_subject','source','ready',?,?),
		('download_merge_target','server-merge',?,?, 'revision', 'med_merge_evidence_target','source','ready',?,?)`,
		accountID, profileID, now, now, accountID, profileID, now, now); err != nil {
		t.Fatalf("seed colliding download preparations: %v", err)
	}
	insertIdentityReview(t, server, "idrev_merge_evidence", "media", library.ID, "med_merge_evidence_subject", []string{"med_merge_evidence_target"}, now)
	if _, err := server.resolveIdentityReconciliationReview(t.Context(), "idrev_merge_evidence", "usr_admin", IdentityReconciliationResolveRequest{Resolution: identityResolutionMerge, SelectedCandidateID: "med_merge_evidence_target"}); err != nil {
		t.Fatalf("resolve identity merge: %v", err)
	}
	var revisions, distinctRevisions, mergeRevisions, canonicalRevision int
	if err := server.db.QueryRow(`SELECT COUNT(*),COUNT(DISTINCT revision),SUM(CASE WHEN trigger_kind='system' THEN 1 ELSE 0 END) FROM media_metadata_revisions WHERE media_id='med_merge_evidence_target'`).Scan(&revisions, &distinctRevisions, &mergeRevisions); err != nil {
		t.Fatalf("load merged revisions: %v", err)
	}
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_items WHERE id='med_merge_evidence_target'`).Scan(&canonicalRevision); err != nil {
		t.Fatalf("load canonical revision: %v", err)
	}
	if revisions != 3 || distinctRevisions != 3 || mergeRevisions != 1 || canonicalRevision != 3 {
		t.Fatalf("revision ledger rows=%d distinct=%d merges=%d canonical=%d", revisions, distinctRevisions, mergeRevisions, canonicalRevision)
	}
	var activeDownloads, invalidatedDownloads int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM download_preparations
		WHERE media_id='med_merge_evidence_target' AND quality_profile='source' AND removed_at=''`).Scan(&activeDownloads); err != nil {
		t.Fatalf("count active merged download preparations: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM download_preparations
		WHERE id='download_merge_subject' AND media_id='med_merge_evidence_target' AND state='unavailable'
			AND error_code='media_identity_changed' AND removed_at<>'' AND job_id='' AND artifact_sha256='' AND size_bytes=0`).Scan(&invalidatedDownloads); err != nil {
		t.Fatalf("count invalidated merged download preparations: %v", err)
	}
	if activeDownloads != 1 || invalidatedDownloads != 1 {
		t.Fatalf("merged download authority active=%d invalidated=%d", activeDownloads, invalidatedDownloads)
	}
	var acceptedID string
	if err := server.db.QueryRow(`SELECT external_id FROM media_provider_ids WHERE media_id='med_merge_evidence_target' AND status='accepted'`).Scan(&acceptedID); err != nil || acceptedID != "target-id" {
		t.Fatalf("accepted provider identity=%q err=%v", acceptedID, err)
	}
	var searchRows, facetRows, staleSearchRows int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_search WHERE media_id='med_merge_evidence_target'`).Scan(&searchRows); err != nil {
		t.Fatalf("count target search rows: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_search WHERE media_id='med_merge_evidence_subject' OR title IN ('stale subject','stale target')`).Scan(&staleSearchRows); err != nil {
		t.Fatalf("count stale search rows: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE media_id='med_merge_evidence_target' AND metadata_revision=3 AND facet_type='genre' AND value='Drama'`).Scan(&facetRows); err != nil {
		t.Fatalf("count rebuilt facets: %v", err)
	}
	if searchRows != 1 || staleSearchRows != 0 || facetRows != 1 {
		t.Fatalf("projection search=%d stale=%d facets=%d", searchRows, staleSearchRows, facetRows)
	}
	var preparedQueueJSON string
	if err := server.db.QueryRow(`SELECT queue_entries_json FROM playback_prepared_handoffs WHERE id='prep_merge_entries'`).Scan(&preparedQueueJSON); err != nil {
		t.Fatalf("load merged prepared queue: %v", err)
	}
	var preparedQueue []playbackQueueOccurrence
	if err := json.Unmarshal([]byte(preparedQueueJSON), &preparedQueue); err != nil {
		t.Fatalf("decode merged prepared queue: %v", err)
	}
	if len(preparedQueue) != 3 || preparedQueue[0].EntryID != "entry-subject-1" || preparedQueue[1].EntryID != "entry-target" || preparedQueue[2].EntryID != "entry-subject-2" {
		t.Fatalf("prepared occurrence identities changed: %+v", preparedQueue)
	}
	for _, occurrence := range preparedQueue {
		if occurrence.MediaID != "med_merge_evidence_target" {
			t.Fatalf("prepared occurrence retained merged media id: %+v", occurrence)
		}
	}
}

func TestMediaMergeRejectsConflictingAcceptedIdentityWhenTargetWasManuallyAccepted(t *testing.T) {
	testMediaMergeProtectedIdentityConflict(t, "", "usr_target_reviewer")
}

func TestMediaMergeRejectsConflictingAcceptedIdentityWhenSourceWasManuallyAccepted(t *testing.T) {
	testMediaMergeProtectedIdentityConflict(t, "usr_source_reviewer", "")
}

func TestMediaMergeRejectsTwoDistinctManuallyAcceptedIdentities(t *testing.T) {
	testMediaMergeProtectedIdentityConflict(t, "usr_source_reviewer", "usr_target_reviewer")
}

func testMediaMergeProtectedIdentityConflict(t *testing.T, sourceAcceptedBy, targetAcceptedBy string) {
	t.Helper()
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Protected Identity Merge", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertIdentityMedia(t, server, "med_protected_source", library.ID, "movie", "", now)
	insertIdentityMedia(t, server, "med_protected_target", library.ID, "movie", "", now)
	for _, row := range []struct {
		mediaID, externalID, acceptedBy string
	}{
		{"med_protected_source", "source-id", sourceAcceptedBy},
		{"med_protected_target", "target-id", targetAcceptedBy},
	} {
		if _, err := server.db.Exec(`INSERT INTO media_provider_ids
			(media_id,provider,external_id,external_type,confidence,source,status,accepted_at,accepted_by_user_id,updated_at)
			VALUES (?, 'tmdb', ?, 'movie', .99, 'test', 'accepted', ?, ?, ?)`, row.mediaID, row.externalID, now, row.acceptedBy, now); err != nil {
			t.Fatalf("seed provider identity: %v", err)
		}
	}
	insertIdentityReview(t, server, "idrev_protected_identity", "media", library.ID, "med_protected_source", []string{"med_protected_target"}, now)

	_, err = server.resolveIdentityReconciliationReview(t.Context(), "idrev_protected_identity", "usr_admin", IdentityReconciliationResolveRequest{
		Resolution:          identityResolutionMerge,
		SelectedCandidateID: "med_protected_target",
	})
	if err != errIdentityReviewUnsafeMerge {
		t.Fatalf("merge error=%v, want %v", err, errIdentityReviewUnsafeMerge)
	}

	var sourceCount, targetCount, acceptedCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id='med_protected_source'`).Scan(&sourceCount); err != nil {
		t.Fatalf("count source: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id='med_protected_target'`).Scan(&targetCount); err != nil {
		t.Fatalf("count target: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_provider_ids
		WHERE media_id IN ('med_protected_source','med_protected_target') AND status='accepted'`).Scan(&acceptedCount); err != nil {
		t.Fatalf("count accepted identities: %v", err)
	}
	if sourceCount != 1 || targetCount != 1 || acceptedCount != 2 {
		t.Fatalf("rejected merge changed state: source=%d target=%d accepted=%d", sourceCount, targetCount, acceptedCount)
	}
}

func TestResolveLiveTVChannelIdentityPreservesReferencesAndState(t *testing.T) {
	server := newScannerTestServer(t)
	viewer := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_merge', 'Merge TV', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for _, row := range []struct{ id string }{
		{id: "ch_merge_subject"},
		{id: "ch_merge_candidate"},
	} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_merge', '7', ?, ?, 1, ?, ?, ?)`, row.id, row.id, "https://tv.example.test/"+row.id, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", row.id, err)
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channel_profile_state (
			profile_id, user_id, channel_id, favorite, hidden, created_at, updated_at
		) VALUES (?, ?, 'ch_merge_subject', 1, 1, ?, ?)`, viewerProfileID(viewer), accountIDForUser(viewer), now, now); err != nil {
		t.Fatalf("insert subject viewer state: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channel_locators (id, channel_id, source_id, provider_kind, provider_key, stream_url, active, first_seen_at, last_seen_at)
		VALUES ('loc_merge_subject', 'ch_merge_subject', 'src_merge', 'm3u', 'provider-subject', 'https://tv.example.test/subject?token=secret', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert subject locator: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_programs (id, source_id, channel_id, title, start_at, end_at, created_at, import_generation)
		VALUES ('program_merge', 'src_merge', 'ch_merge_subject', 'News', ?, ?, ?, 'generation')`, now, now, now); err != nil {
		t.Fatalf("insert program: %v", err)
	}
	insertIdentityReview(t, server, "idrev_channel_merge", "live_tv_channel", "src_merge", "ch_merge_subject", []string{"ch_merge_candidate"}, now)

	resolved, err := server.resolveIdentityReconciliationReview(t.Context(), "idrev_channel_merge", "usr_admin", IdentityReconciliationResolveRequest{Resolution: identityResolutionMerge, SelectedCandidateID: "ch_merge_candidate"})
	if err != nil {
		t.Fatalf("resolve channel merge: %v", err)
	}
	if resolved.Status != "resolved" {
		t.Fatalf("resolved channel review=%#v", resolved)
	}
	var programChannel, locatorChannel string
	var favorite, hidden int
	if err := server.db.QueryRow(`SELECT channel_id FROM live_tv_programs WHERE id = 'program_merge'`).Scan(&programChannel); err != nil {
		t.Fatalf("load moved program: %v", err)
	}
	if err := server.db.QueryRow(`SELECT channel_id FROM live_tv_channel_locators WHERE id = 'loc_merge_subject'`).Scan(&locatorChannel); err != nil {
		t.Fatalf("load moved locator: %v", err)
	}
	if err := server.db.QueryRow(`SELECT favorite, hidden FROM live_tv_channel_profile_state WHERE profile_id = ? AND channel_id = 'ch_merge_candidate'`, viewerProfileID(viewer)).Scan(&favorite, &hidden); err != nil {
		t.Fatalf("load merged channel state: %v", err)
	}
	if programChannel != "ch_merge_candidate" || locatorChannel != "ch_merge_candidate" || favorite != 1 || hidden != 1 {
		t.Fatalf("program=%q locator=%q favorite=%d hidden=%d", programChannel, locatorChannel, favorite, hidden)
	}
	var globalFavorite, globalHidden int
	if err := server.db.QueryRow(`SELECT favorite, hidden FROM live_tv_channels WHERE id = 'ch_merge_candidate'`).Scan(&globalFavorite, &globalHidden); err != nil {
		t.Fatalf("load merged channel catalog flags: %v", err)
	}
	if globalFavorite != 0 || globalHidden != 0 {
		t.Fatalf("identity merge leaked viewer state into catalog: favorite=%d hidden=%d", globalFavorite, globalHidden)
	}
}

func TestLiveTVAmbiguityReusesOpenReviewSubject(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_ambiguous', 'Ambiguous TV', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	source, err := server.getLiveTVSourceRecord("src_ambiguous")
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	for index, id := range []string{"ch_existing_a", "ch_existing_b"} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_ambiguous', ?, ?, 1, ?, ?, ?)`, id, id, "https://tv.example.test/existing/"+id, now, now, now); err != nil {
			t.Fatalf("insert existing channel: %v", err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channel_locators (id, channel_id, source_id, provider_kind, provider_key, stream_url, active, first_seen_at, last_seen_at)
			VALUES (?, ?, 'src_ambiguous', 'm3u', 'shared-key', ?, 1, ?, ?)`, "loc_existing_"+string(rune('a'+index)), id, "https://tv.example.test/existing/"+id, now, now); err != nil {
			t.Fatalf("insert existing locator: %v", err)
		}
	}
	imported := []liveTVChannelImport{{ID: "provider_provisional", ProviderKey: "shared-key", Number: "7", Name: "Shared", StreamURL: "https://tv.example.test/new?token=secret"}}
	if err := server.storeLiveTVImport(source, imported, nil); err != nil {
		t.Fatalf("first ambiguous import: %v", err)
	}
	var reviewID, subjectID string
	if err := server.db.QueryRow(`
		SELECT id, subject_id FROM identity_reconciliation_reviews
		WHERE domain = 'live_tv_channel' AND status = 'open'`).Scan(&reviewID, &subjectID); err != nil {
		t.Fatalf("load open channel review: %v", err)
	}
	if subjectID == "" || !strings.HasPrefix(subjectID, "ch_") {
		t.Fatalf("review subject ID=%q", subjectID)
	}
	if err := server.storeLiveTVImport(source, imported, nil); err != nil {
		t.Fatalf("repeat ambiguous import: %v", err)
	}
	var reviewCount, subjectCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM identity_reconciliation_reviews WHERE domain = 'live_tv_channel' AND status = 'open'`).Scan(&reviewCount); err != nil {
		t.Fatalf("count reviews: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_channels WHERE id = ?`, subjectID).Scan(&subjectCount); err != nil {
		t.Fatalf("count subject channel: %v", err)
	}
	if reviewCount != 1 || subjectCount != 1 {
		t.Fatalf("repeat ambiguity reviewCount=%d subjectCount=%d reviewID=%s", reviewCount, subjectCount, reviewID)
	}
	review, err := server.getIdentityReconciliationReview(t.Context(), reviewID)
	if err != nil {
		t.Fatalf("load redacted review: %v", err)
	}
	if strings.Contains(review.CandidateLocator, "token=") || review.CandidateLocator != "https://tv.example.test" {
		t.Fatalf("live locator was not redacted: %q", review.CandidateLocator)
	}
}

func insertIdentityMedia(t *testing.T, server *Server, id, libraryID, mediaType, parentID, now string) {
	t.Helper()
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at, random_key)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`, id, libraryID, parentID, mediaType, id, id, now, id); err != nil {
		t.Fatalf("insert media %s: %v", id, err)
	}
}

func insertIdentityReview(t *testing.T, server *Server, id, domain, libraryOrSourceID, subjectID string, candidateIDs []string, now string) {
	t.Helper()
	encoded, err := json.Marshal(candidateIDs)
	if err != nil {
		t.Fatalf("encode candidate IDs: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO identity_reconciliation_reviews (
			id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
			evidence_value, candidate_ids_json, status, created_at
		) VALUES (?, ?, ?, ?, '/candidate', 'test_ambiguity', 'test', ?, 'open', ?)`,
		id, domain, libraryOrSourceID, subjectID, string(encoded), now); err != nil {
		t.Fatalf("insert review %s: %v", id, err)
	}
}
