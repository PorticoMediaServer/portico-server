package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestOfflineDownloadAuthorizationCanonicalizationIsRFC8785AndSignatureIsExact(t *testing.T) {
	receipt := OfflineDownloadAuthorizationReceipt{
		Version: 1, Purpose: offlineDownloadAuthorizationPurpose, ReceiptID: "receipt-é-😀",
		ViewerScope: OfflineDownloadAuthorizationViewerScope{
			ScopeKind: "server-bound", Authority: "local", AccountID: "account", ProfileID: "profile",
			ServerID: "server", AuthorizationRevision: "revision",
		},
		Issuer: OfflineDownloadAuthorizationIssuer{ServerID: "server", SigningKeyFingerprint: "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		Preparation: OfflineDownloadAuthorizationPreparation{
			PreparationID: "preparation", MediaID: "media", MediaVersionID: "version", QualityID: "source",
		},
		Artifact: OfflineDownloadAuthorizationArtifact{
			SHA256: "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881", SizeBytes: 1,
		},
		LastVerifiedAt: "2026-08-30T12:00:00Z", VerifyBy: "2026-09-29T12:00:00Z", Signature: strings.Repeat("ignored", 10),
	}
	payload, err := canonicalOfflineDownloadAuthorizationPayload(receipt)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"artifact":{"sha256":"2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881","sizeBytes":1},"issuer":{"serverId":"server","signingKeyFingerprint":"sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},"lastVerifiedAt":"2026-08-30T12:00:00Z","preparation":{"mediaId":"media","mediaVersionId":"version","preparationId":"preparation","qualityId":"source"},"purpose":"offline-download-authorization","receiptId":"receipt-é-😀","verifyBy":"2026-09-29T12:00:00Z","version":1,"viewerScope":{"accountId":"account","authority":"local","authorizationRevision":"revision","profileId":"profile","scopeKind":"server-bound","serverId":"server"}}`
	if string(payload) != want {
		t.Fatalf("canonical payload\n got: %s\nwant: %s", payload, want)
	}
	seed := sha256.Sum256([]byte("offline-receipt-test-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	identity := serverIdentity{PrivateKey: privateKey, PublicKey: privateKey.Public().(ed25519.PublicKey)}
	identity.Fingerprint = publicKeyFingerprint(identity.PublicKey)
	record := downloadPreparationRecord{
		DownloadPreparation:   DownloadPreparation{ID: "preparation", MediaID: "media", QualityProfile: "source", SizeBytes: 1},
		AuthorizationRevision: "revision", MediaVersionID: "version", VersionFingerprint: "fence",
		ArtifactSHA256: receipt.Artifact.SHA256,
	}
	signed, err := signOfflineDownloadAuthorizationReceipt(record, receipt.ViewerScope, identity, time.Date(2026, 8, 30, 12, 0, 0, 123, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := decodeCanonicalBase64URL(signed.Signature, ed25519.SignatureSize)
	if err != nil {
		t.Fatal(err)
	}
	signedPayload, err := canonicalOfflineDownloadAuthorizationPayload(signed)
	if err != nil || !ed25519.Verify(identity.PublicKey, signedPayload, signature) {
		t.Fatalf("signed JCS payload did not verify: err=%v", err)
	}
	tampered := signed
	tampered.Artifact.SizeBytes++
	tamperedPayload, _ := canonicalOfflineDownloadAuthorizationPayload(tampered)
	if ed25519.Verify(identity.PublicKey, tamperedPayload, signature) {
		t.Fatal("tampered artifact verified")
	}
}

func TestOfflineReceiptViewerIdentityEnvelopeSeparatesMalformedFromForeign(t *testing.T) {
	scope := OfflineDownloadAuthorizationViewerScope{
		ScopeKind: "server-bound", Authority: "local", AccountID: "account", ProfileID: "profile",
		ServerID: "server", AuthorizationRevision: "revision",
	}
	matching := `{"viewerScope":{"scopeKind":"server-bound","authority":"local","accountId":"account","profileId":"profile","serverId":"server","authorizationRevision":"old-revision"},"issuer":{"serverId":"server"}}`
	foreign := `{"viewerScope":{"scopeKind":"server-bound","authority":"local","accountId":"account","profileId":"other-profile","serverId":"server","authorizationRevision":"old-revision"},"issuer":{"serverId":"server"}}`
	missingIssuer := `{"viewerScope":{"scopeKind":"server-bound","authority":"local","accountId":"account","profileId":"profile","serverId":"server","authorizationRevision":"old-revision"}}`
	tests := []struct {
		name string
		raw  string
		want offlineReceiptEnvelopeDisposition
	}{
		{name: "matching identity ignores stale revision", raw: matching, want: offlineReceiptEnvelopeMatching},
		{name: "complete foreign identity", raw: foreign, want: offlineReceiptEnvelopeForeign},
		{name: "empty object", raw: `{}`, want: offlineReceiptEnvelopeInvalid},
		{name: "missing issuer", raw: missingIssuer, want: offlineReceiptEnvelopeInvalid},
		{name: "malformed JSON", raw: `{"viewerScope":`, want: offlineReceiptEnvelopeInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := offlineReceiptViewerIdentityEnvelope(json.RawMessage(test.raw), scope); got != test.want {
				t.Fatalf("disposition=%d want=%d", got, test.want)
			}
		})
	}
}

func TestDownloadArtifactDigestWriterCoversOnlyCommittedProducerBytes(t *testing.T) {
	var output bytes.Buffer
	writer := newOptimizedArtifactDigestWriter(&output)
	if _, err := writer.Write([]byte("producer-")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("bytes")); err != nil {
		t.Fatal(err)
	}
	digest, size := writer.Digest()
	want := sha256.Sum256([]byte("producer-bytes"))
	if digest != hex.EncodeToString(want[:]) || size != int64(output.Len()) {
		t.Fatalf("producer digest=%q size=%d body=%q", digest, size, output.String())
	}
}

func TestOptimizeCompletionQueuesVerificationWithoutViewerPoll(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	seedDownloadGrantMedia(t, db)
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := server.createDownloadPreparationContext(context.Background(), user, DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := server.downloadPreparationRecordContext(context.Background(), `WHERE dp.id = ? LIMIT 1`, preparation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM jobs WHERE id = ?`, record.JobID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	job := Job{ID: "job_optimize_handoff", Type: "optimize_version", ResourceType: "media", ResourceID: record.MediaID}
	if _, err := db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json,
			leased_by, lease_expires_at, cancellation_requested_at, created_at, updated_at)
		VALUES (?, 'optimize_version', 'running', 92, 'Stored', 'media', ?, '{"profile":"universal-720p"}', ?, ?, '', ?, ?)`,
		job.ID, record.MediaID, server.jobLeaseOwner(job.ID), time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE download_preparations SET job_id = ?, state = 'running' WHERE id = ?`, job.ID, record.ID); err != nil {
		t.Fatal(err)
	}
	if err := server.completeOptimizeVersionAndQueueDownloadVerifications(job, "Optimized version complete."); err != nil {
		t.Fatal(err)
	}
	var preparationJobID, preparationState, producerStatus, nextType string
	if err := db.QueryRow(`SELECT job_id, state FROM download_preparations WHERE id = ?`, record.ID).Scan(&preparationJobID, &preparationState); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(&producerStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT type FROM jobs WHERE id = ?`, preparationJobID).Scan(&nextType); err != nil {
		t.Fatal(err)
	}
	if producerStatus != "complete" || preparationState != "queued" || nextType != downloadArtifactVerificationJobType {
		t.Fatalf("producer=%q preparation=%q next=%q", producerStatus, preparationState, nextType)
	}
}

func TestOptimizeFailureAtomicallyFailsLinkedPreparationsWithoutCrossingCancellation(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	seedDownloadGrantMedia(t, db)
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := server.createDownloadPreparationContext(context.Background(), user, DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := server.downloadPreparationRecordContext(context.Background(), `WHERE dp.id = ? LIMIT 1`, preparation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM jobs WHERE id = ?`, record.JobID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	job := Job{ID: "job_optimize_failure", Type: "optimize_version", ResourceType: "media", ResourceID: record.MediaID}
	if _, err := db.Exec(`INSERT INTO jobs
		(id,type,status,progress,message,resource_type,resource_id,leased_by,lease_expires_at,cancellation_requested_at,created_at,updated_at)
		VALUES (?, 'optimize_version', 'running', 50, 'running', 'media', ?, ?, ?, '', ?, ?)`,
		job.ID, record.MediaID, server.jobLeaseOwner(job.ID), time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE download_preparations SET job_id=?, state='running', artifact_sha256=?, size_bytes=1, size_kind='exact' WHERE id=?`,
		job.ID, strings.Repeat("a", 64), record.ID); err != nil {
		t.Fatal(err)
	}
	if err := server.failOptimizeVersionAndDownloadPreparations(job, "producer failed"); err != nil {
		t.Fatal(err)
	}
	var preparationState, preparationCode, digest, sizeKind, jobStatus, jobCode string
	var size int64
	if err := db.QueryRow(`SELECT state,error_code,artifact_sha256,size_bytes,size_kind FROM download_preparations WHERE id=?`, record.ID).
		Scan(&preparationState, &preparationCode, &digest, &size, &sizeKind); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status,error_code FROM jobs WHERE id=?`, job.ID).Scan(&jobStatus, &jobCode); err != nil {
		t.Fatal(err)
	}
	if preparationState != "failed" || preparationCode != "preparation_failed" || digest != "" || size != 0 || sizeKind != "unknown" || jobStatus != "failed" || jobCode != "optimize_failed" {
		t.Fatalf("preparation=%q/%q digest=%q size=%d/%q job=%q/%q", preparationState, preparationCode, digest, size, sizeKind, jobStatus, jobCode)
	}

	if _, err := db.Exec(`UPDATE jobs SET status='running', leased_by=?, cancellation_requested_at=?, error_code='' WHERE id=?`,
		server.jobLeaseOwner(job.ID), now, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE download_preparations SET state='running', error_code='' WHERE id=?`, record.ID); err != nil {
		t.Fatal(err)
	}
	if err := server.failOptimizeVersionAndDownloadPreparations(job, "must not win cancellation"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state FROM download_preparations WHERE id=?`, record.ID).Scan(&preparationState); err != nil || preparationState != "running" {
		t.Fatalf("cancelled producer overwrote preparation state=%q err=%v", preparationState, err)
	}
}

func TestDownloadPreparationProjectionCannotOverwriteConcurrentLifecycleState(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	seedDownloadGrantMedia(t, db)
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := server.createDownloadPreparationContext(context.Background(), user, DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := server.downloadPreparationRecordContext(context.Background(), `WHERE dp.id = ? LIMIT 1`, preparation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE download_preparations SET state = 'cancelled', cancelled_at = ?, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), stale.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.materializeDownloadPreparationContext(context.Background(), user, stale); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM download_preparations WHERE id = ?`, stale.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" {
		t.Fatalf("stale read overwrote authoritative lifecycle state with %q", state)
	}
}

func TestOfflineAuthorizationGrantAndRevalidationAreSecurityFenceAtomic(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	seedDownloadGrantMedia(t, db)
	var preparation downloadPreparationView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"}, &preparation)
	if status != http.StatusCreated {
		t.Fatalf("create preparation status=%d body=%s", status, body)
	}
	preparation = completeDownloadPreparationForTest(t, server, client, serverURL, preparation)
	userID := adminUserID(t, db)
	var originalPermissionsJSON string
	if err := db.QueryRow(`SELECT permissions_json FROM users WHERE id = ?`, userID).Scan(&originalPermissionsJSON); err != nil {
		t.Fatal(err)
	}
	revoke := func() {
		if _, err := db.Exec(`UPDATE users SET permissions_json = json_set(permissions_json, '$.downloadMedia', json('false')) WHERE id = ?`, userID); err != nil {
			panic(err)
		}
		server.invalidateAuthorizationCachesForMutation("UPDATE users SET permissions_json", []string{"users"})
	}
	restore := func() {
		if _, err := db.Exec(`UPDATE users SET permissions_json = ? WHERE id = ?`, originalPermissionsJSON, userID); err != nil {
			t.Fatal(err)
		}
		server.invalidateAuthorizationCachesForMutation("UPDATE users SET permissions_json", []string{"users"})
	}
	server.offlineAuthorizationBeforeCommitHook = revoke
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations/"+url.PathEscape(preparation.ID)+"/grant", downloadPreparationGrantRequest{Delivery: "native"}, nil)
	server.offlineAuthorizationBeforeCommitHook = nil
	if status != http.StatusConflict || !strings.Contains(body, "download_preparation_not_ready") {
		t.Fatalf("revoked-before-commit grant status=%d body=%s", status, body)
	}
	var grants int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_download_grants WHERE preparation_id = ?`, preparation.ID).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("stale authority published %d grants: %v", grants, err)
	}
	restore()
	var grant MediaDownloadGrantResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations/"+url.PathEscape(preparation.ID)+"/grant", downloadPreparationGrantRequest{Delivery: "native"}, &grant)
	if status != http.StatusCreated || grant.AuthorizationReceipt == nil {
		t.Fatalf("native grant status=%d body=%s grant=%#v", status, body, grant)
	}
	receipt := *grant.AuthorizationReceipt
	rawReceipt, _ := json.Marshal(receipt)
	server.offlineAuthorizationBeforeCommitHook = revoke
	var revoked OfflineDownloadAuthorizationRevalidationResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/offline-download-authorizations/revalidate",
		OfflineDownloadAuthorizationRevalidationRequest{Receipt: rawReceipt}, &revoked)
	server.offlineAuthorizationBeforeCommitHook = nil
	if status != http.StatusOK || revoked.Outcome != "revoked" || revoked.Receipt != nil {
		t.Fatalf("revocation race status=%d body=%s response=%#v", status, body, revoked)
	}
	restore()
	var replacement OfflineDownloadAuthorizationRevalidationResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/offline-download-authorizations/revalidate",
		OfflineDownloadAuthorizationRevalidationRequest{Receipt: rawReceipt}, &replacement)
	if status != http.StatusOK || replacement.Outcome != "valid-replacement" || replacement.Receipt == nil || replacement.Receipt.ReceiptID == receipt.ReceiptID {
		t.Fatalf("replacement status=%d body=%s response=%#v", status, body, replacement)
	}
	if !offlineReceiptMatchesPreparation(*replacement.Receipt, downloadPreparationRecord{
		DownloadPreparation: DownloadPreparation{ID: receipt.Preparation.PreparationID, MediaID: receipt.Preparation.MediaID, QualityProfile: receipt.Preparation.QualityID, SizeBytes: receipt.Artifact.SizeBytes},
		MediaVersionID:      receipt.Preparation.MediaVersionID, ArtifactSHA256: receipt.Artifact.SHA256,
	}) {
		t.Fatal("replacement changed immutable preparation or artifact binding")
	}
	tampered := receipt
	tampered.ViewerScope.ProfileID = "another-profile"
	tamperedRaw, _ := json.Marshal(tampered)
	var outOfScope OfflineDownloadAuthorizationRevalidationResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/offline-download-authorizations/revalidate",
		OfflineDownloadAuthorizationRevalidationRequest{Receipt: tamperedRaw}, &outOfScope)
	if status != http.StatusOK || outOfScope.Outcome != "out-of-scope" || strings.Contains(body, preparation.ID) {
		t.Fatalf("cross-profile receipt status=%d body=%s response=%#v", status, body, outOfScope)
	}
	tampered = receipt
	tampered.Artifact.SHA256 = strings.Repeat("0", 64)
	tamperedRaw, _ = json.Marshal(tampered)
	var invalid OfflineDownloadAuthorizationRevalidationResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/offline-download-authorizations/revalidate",
		OfflineDownloadAuthorizationRevalidationRequest{Receipt: tamperedRaw}, &invalid)
	if status != http.StatusOK || invalid.Outcome != "invalid" {
		t.Fatalf("tampered receipt status=%d body=%s response=%#v", status, body, invalid)
	}
}
