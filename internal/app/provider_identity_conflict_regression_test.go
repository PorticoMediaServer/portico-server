package app

import (
	"testing"
	"time"
)

func TestProviderIdentityConflictPreservesHistoryAndManualAcceptanceSupersedes(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Identity History", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertIdentityMedia(t, server, "media_identity_history", library.ID, "movie", "", now)

	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin identity transaction: %v", err)
	}
	defer tx.Rollback()
	if err := upsertMediaProviderIdentityTx(tx, "media_identity_history", "tmdb", "101", "movie", .95, "provider-search", false, "", now); err != nil {
		t.Fatalf("accept first automated identity: %v", err)
	}
	if err := upsertMediaProviderIdentityTx(tx, "media_identity_history", "tmdb", "202", "movie", .99, "provider-search", false, "", now); err != nil {
		t.Fatalf("record conflicting automated identity: %v", err)
	}

	var firstStatus, secondStatus string
	if err := tx.QueryRow(`SELECT status FROM media_provider_ids WHERE media_id=? AND provider='tmdb' AND external_id='101'`, "media_identity_history").Scan(&firstStatus); err != nil {
		t.Fatalf("load accepted identity: %v", err)
	}
	if err := tx.QueryRow(`SELECT status FROM media_provider_ids WHERE media_id=? AND provider='tmdb' AND external_id='202'`, "media_identity_history").Scan(&secondStatus); err != nil {
		t.Fatalf("load conflicting identity: %v", err)
	}
	if firstStatus != "accepted" || secondStatus != "candidate" {
		t.Fatalf("automated conflict statuses = first:%q second:%q", firstStatus, secondStatus)
	}

	manualAt := time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)
	if err := upsertMediaProviderIdentityTx(tx, "media_identity_history", "tmdb", "202", "movie", 1, "manual-match", true, "user_reviewer", manualAt); err != nil {
		t.Fatalf("manually accept conflicting identity: %v", err)
	}
	var acceptedBy string
	if err := tx.QueryRow(`SELECT status FROM media_provider_ids WHERE media_id=? AND provider='tmdb' AND external_id='101'`, "media_identity_history").Scan(&firstStatus); err != nil {
		t.Fatalf("reload prior identity: %v", err)
	}
	if err := tx.QueryRow(`SELECT status, accepted_by_user_id FROM media_provider_ids WHERE media_id=? AND provider='tmdb' AND external_id='202'`, "media_identity_history").Scan(&secondStatus, &acceptedBy); err != nil {
		t.Fatalf("reload manually accepted identity: %v", err)
	}
	if firstStatus != "superseded" || secondStatus != "accepted" || acceptedBy != "user_reviewer" {
		t.Fatalf("manual acceptance statuses = first:%q second:%q acceptedBy:%q", firstStatus, secondStatus, acceptedBy)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit identity transaction: %v", err)
	}

	var historicalRows, acceptedRows int
	if err := server.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN status='accepted' THEN 1 ELSE 0 END) FROM media_provider_ids WHERE media_id=? AND provider='tmdb'`, "media_identity_history").Scan(&historicalRows, &acceptedRows); err != nil {
		t.Fatalf("load committed identity history: %v", err)
	}
	if historicalRows != 2 || acceptedRows != 1 {
		t.Fatalf("committed identity history rows=%d accepted=%d", historicalRows, acceptedRows)
	}
}

func TestProviderIdentityRequiresTypedSyntaxAndAutomaticThreshold(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Typed Identity", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	insertIdentityMedia(t, server, "media_typed_identity", library.ID, "movie", "", now)
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin identity transaction: %v", err)
	}
	defer tx.Rollback()
	if err := upsertMediaProviderIdentityTx(tx, "media_typed_identity", "tmdb", "tt123", "movie", 1, "nfo", false, "", now); err != nil {
		t.Fatalf("reject malformed TMDB identity: %v", err)
	}
	if err := upsertMediaProviderIdentityTx(tx, "media_typed_identity", "tmdb", "303", "movie", .84, "provider-search", false, "", now); err != nil {
		t.Fatalf("record sub-threshold identity: %v", err)
	}
	var malformed int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM media_provider_ids WHERE media_id=? AND external_id='tt123'`, "media_typed_identity").Scan(&malformed); err != nil {
		t.Fatal(err)
	}
	if malformed != 0 {
		t.Fatalf("malformed TMDB identity persisted: %d rows", malformed)
	}
	var status string
	if err := tx.QueryRow(`SELECT status FROM media_provider_ids WHERE media_id=? AND external_id='303'`, "media_typed_identity").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(metadataIdentityCandidate) {
		t.Fatalf("sub-threshold identity status = %q, expected candidate", status)
	}
}
