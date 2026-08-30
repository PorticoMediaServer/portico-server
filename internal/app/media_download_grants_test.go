package app

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestMediaDownloadGrantIsHashedScopedAndRangeSafeUntilExpiry(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	mediaRoot, sourcePath := seedDownloadGrantMedia(t, db)
	_ = mediaRoot
	credentialOnlyResponse, err := client.Get(serverURL + "/api/media/grant_media/download")
	if err != nil {
		t.Fatalf("send account-only download: %v", err)
	}
	_, _ = io.Copy(io.Discard, credentialOnlyResponse.Body)
	_ = credentialOnlyResponse.Body.Close()
	if credentialOnlyResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("account credential bypassed download capability: status=%d", credentialOnlyResponse.StatusCode)
	}
	credentialOnlyHead, err := client.Head(serverURL + "/api/media/grant_media/download")
	if err != nil {
		t.Fatalf("send account-only download HEAD: %v", err)
	}
	_ = credentialOnlyHead.Body.Close()
	if credentialOnlyHead.StatusCode != http.StatusUnauthorized {
		t.Fatalf("account credential bypassed download capability HEAD: status=%d", credentialOnlyHead.StatusCode)
	}

	grant := createDownloadGrantForTest(t, client, serverURL, "grant_media", "source")
	token := downloadGrantTokenForTest(t, grant)
	var storedHash, serverID, principalUserID, mediaID, versionKind, versionID, fingerprint string
	if err := db.QueryRow(`
		SELECT token_hash, server_id, principal_user_id, media_id, version_kind, version_id, version_fingerprint
		FROM media_download_grants WHERE token_hash = ?`, hashToken(token)).Scan(
		&storedHash, &serverID, &principalUserID, &mediaID, &versionKind, &versionID, &fingerprint,
	); err != nil {
		t.Fatalf("query stored download grant: %v", err)
	}
	if storedHash == token || storedHash != hashToken(token) {
		t.Fatalf("download grant was not stored as a one-way hash")
	}
	if serverID == "" || principalUserID == "" || mediaID != "grant_media" || versionKind != "source" || versionID != "mf_grant" || fingerprint == "" {
		t.Fatalf("download grant binding is incomplete: server=%q user=%q media=%q kind=%q version=%q fingerprint=%q", serverID, principalUserID, mediaID, versionKind, versionID, fingerprint)
	}

	wrongMediaURL := serverURL + strings.Replace(grant.DownloadURL, "/grant_media/", "/grant_other/", 1)
	wrongRequest, _ := http.NewRequest(http.MethodGet, wrongMediaURL, nil)
	wrongRequest.Header.Set("Authorization", "PorticoDownload "+token)
	wrongResponse, err := http.DefaultClient.Do(wrongRequest)
	if err != nil {
		t.Fatalf("send cross-media download: %v", err)
	}
	wrongBody, _ := io.ReadAll(wrongResponse.Body)
	_ = wrongResponse.Body.Close()
	if wrongResponse.StatusCode != http.StatusUnauthorized || !strings.Contains(string(wrongBody), "download_grant_denied") {
		t.Fatalf("cross-media download status=%d body=%s", wrongResponse.StatusCode, wrongBody)
	}
	var consumedAt string
	if err := db.QueryRow(`SELECT consumed_at FROM media_download_grants WHERE token_hash = ?`, hashToken(token)).Scan(&consumedAt); err != nil || consumedAt != "" {
		t.Fatalf("cross-media attempt consumed grant: consumed=%q err=%v", consumedAt, err)
	}
	// The short-lived exact-artifact grant is the browser transfer capability;
	// account credentials are deliberately not required on the byte request.
	capabilityResponse, err := client.Get(serverURL + grant.DownloadURL)
	if err != nil {
		t.Fatalf("send capability download: %v", err)
	}
	capabilityBody, _ := io.ReadAll(capabilityResponse.Body)
	_ = capabilityResponse.Body.Close()
	if capabilityResponse.StatusCode != http.StatusOK || string(capabilityBody) != "grant-download-body" {
		t.Fatalf("capability download status=%d body=%s", capabilityResponse.StatusCode, capabilityBody)
	}
	headRequest, err := http.NewRequest(http.MethodHead, serverURL+grant.DownloadURL, nil)
	if err != nil {
		t.Fatalf("build capability HEAD: %v", err)
	}
	headResponse, err := client.Do(headRequest)
	if err != nil {
		t.Fatalf("send capability HEAD: %v", err)
	}
	_ = headResponse.Body.Close()
	if headResponse.StatusCode != http.StatusOK {
		t.Fatalf("capability HEAD status=%d", headResponse.StatusCode)
	}

	for attempt := 0; attempt < 2; attempt++ {
		request, err := http.NewRequest(http.MethodGet, serverURL+grant.DownloadURL, nil)
		if err != nil {
			t.Fatalf("build range-safe request: %v", err)
		}
		request.Header.Set("Range", "bytes=0-4")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("send range-safe request %d: %v", attempt, err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusPartialContent {
			t.Fatalf("range-safe attempt %d status=%d, expected 206", attempt, response.StatusCode)
		}
	}
	if _, err := db.Exec(`UPDATE media_download_grants SET consumed_at = ? WHERE token_hash = ?`, time.Now().UTC().Add(-downloadGrantReplayWindow-time.Second).Format(time.RFC3339), hashToken(token)); err != nil {
		t.Fatalf("age consumed download grant: %v", err)
	}
	assertDownloadGrantDenied(t, client, serverURL+grant.DownloadURL)

	var auditMetadata []string
	rows, err := db.Query(`SELECT metadata_json FROM audit_events WHERE action IN ('media.download_grant_issued', 'media.download_grant_authorized')`)
	if err != nil {
		t.Fatalf("query download grant audit: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var metadata string
		if err := rows.Scan(&metadata); err != nil {
			t.Fatalf("scan download grant audit: %v", err)
		}
		auditMetadata = append(auditMetadata, metadata)
		if strings.Contains(metadata, token) || strings.Contains(metadata, storedHash) || strings.Contains(metadata, sourcePath) {
			t.Fatalf("download grant audit leaked a token, hash, or source path: %s", metadata)
		}
	}
	if len(auditMetadata) < 2 {
		t.Fatalf("expected issue and consume audits, got %v", auditMetadata)
	}
}

func TestMediaDownloadGrantRevalidatesExpiryVersionAndPermission(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, sourcePath := seedDownloadGrantMedia(t, db)

	expired := createDownloadGrantForTest(t, client, serverURL, "grant_media", "source")
	expiredToken := downloadGrantTokenForTest(t, expired)
	if _, err := db.Exec(`UPDATE media_download_grants SET expires_at = ? WHERE token_hash = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), hashToken(expiredToken)); err != nil {
		t.Fatalf("expire download grant: %v", err)
	}
	assertDownloadGrantDenied(t, client, serverURL+expired.DownloadURL)

	changed := createDownloadGrantForTest(t, client, serverURL, "grant_media", "source")
	if err := os.WriteFile(sourcePath, []byte("a changed source version"), 0o600); err != nil {
		t.Fatalf("change source version: %v", err)
	}
	assertDownloadGrantDenied(t, client, serverURL+changed.DownloadURL)

	permission := createDownloadGrantForTest(t, client, serverURL, "grant_media", "source")
	userID := adminUserID(t, db)
	if _, err := db.Exec(`UPDATE users SET permissions_json = json_set(permissions_json, '$.downloadMedia', false) WHERE id = ?`, userID); err != nil {
		t.Fatalf("revoke download permission: %v", err)
	}
	assertDownloadGrantDenied(t, client, serverURL+permission.DownloadURL)
}

func TestDownloadStateCleanupExpiresCapabilitiesAndOnlyOldTerminalPreparations(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, _ = seedDownloadGrantMedia(t, db)
	now := time.Now().UTC()

	expired := createDownloadGrantForTest(t, client, serverURL, "grant_media", "source")
	if _, err := db.Exec(`UPDATE media_download_grants SET expires_at = ? WHERE token_hash = ?`, now.Add(-time.Minute).Format(time.RFC3339), hashToken(downloadGrantTokenForTest(t, expired))); err != nil {
		t.Fatalf("expire cleanup grant: %v", err)
	}
	active := createDownloadGrantForTest(t, client, serverURL, "grant_media", "source")

	var preparation downloadPreparationView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"}, &preparation)
	if status != http.StatusCreated {
		t.Fatalf("create cleanup preparation status=%d body=%s", status, body)
	}
	oldTerminal := now.Add(-downloadPreparationTerminalRetention - time.Hour).Format(time.RFC3339)
	oldTombstone := now.Add(-downloadPreparationTombstoneRetention - time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO download_preparations (
			id, server_id, account_id, profile_id, authorization_revision, media_id, quality_profile,
			state, job_id, size_bytes, size_kind, artifact_expires_at, progress, error_code,
			created_at, updated_at, cancelled_at, removed_at
		)
		SELECT 'cleanup-terminal', server_id, account_id, profile_id, authorization_revision, media_id, 'cleanup-terminal',
			'failed', '', size_bytes, size_kind, '', 0, 'preparation_failed', ?, ?, '', ''
		FROM download_preparations WHERE id = ?`, oldTerminal, oldTerminal, preparation.ID); err != nil {
		t.Fatalf("insert old terminal preparation: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO download_preparations (
			id, server_id, account_id, profile_id, authorization_revision, media_id, quality_profile,
			state, job_id, size_bytes, size_kind, artifact_expires_at, progress, error_code,
			created_at, updated_at, cancelled_at, removed_at
		)
		SELECT 'cleanup-removed', server_id, account_id, profile_id, authorization_revision, media_id, 'cleanup-removed',
			'ready', '', size_bytes, size_kind, '', 100, '', ?, ?, '', ?
		FROM download_preparations WHERE id = ?`, oldTombstone, oldTombstone, oldTombstone, preparation.ID); err != nil {
		t.Fatalf("insert old removed preparation: %v", err)
	}

	if err := server.pruneDownloadStateContext(context.Background(), now); err != nil {
		t.Fatalf("prune download state: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM media_download_grants WHERE token_hash = '`+hashToken(downloadGrantTokenForTest(t, expired))+`'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM media_download_grants WHERE token_hash = '`+hashToken(downloadGrantTokenForTest(t, active))+`'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM download_preparations WHERE id IN ('cleanup-terminal', 'cleanup-removed')`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM download_preparations WHERE id = '`+preparation.ID+`'`, 1)
}

func TestSourceDownloadPreparationIsDurableRangeSafeAndRemovable(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, _ = seedDownloadGrantMedia(t, db)

	var preparation downloadPreparationView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"}, &preparation)
	if status != http.StatusCreated || preparation.State != "ready" || preparation.Progress != 100 || preparation.ID == "" || preparation.SizeKind != "exact" {
		t.Fatalf("create source preparation status=%d body=%s preparation=%#v", status, body, preparation)
	}
	var listed ListResponse[downloadPreparationView]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/download-preparations", nil, &listed)
	if status != http.StatusOK || listed.Total != 1 || listed.Items[0].ID != preparation.ID {
		t.Fatalf("list source preparation status=%d body=%s list=%#v", status, body, listed)
	}
	var grant MediaDownloadGrantResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations/"+url.PathEscape(preparation.ID)+"/grant", downloadPreparationGrantRequest{Delivery: "native"}, &grant)
	if status != http.StatusCreated || grant.DownloadURL == "" {
		t.Fatalf("create preparation grant status=%d body=%s grant=%#v", status, body, grant)
	}
	if !strings.Contains(body, "downloadUrl") {
		t.Fatalf("preparation grant response was not a transfer capability: %s", body)
	}
	downloadAuthorization := "PorticoDownload " + downloadGrantTokenForTest(t, grant)
	for attempt := 0; attempt < 2; attempt++ {
		request, _ := http.NewRequest(http.MethodGet, serverURL+grant.DownloadURL, nil)
		request.Header.Set("Range", "bytes=0-4")
		request.Header.Set("Authorization", downloadAuthorization)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("download prepared range %d: %v", attempt, err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusPartialContent {
			t.Fatalf("prepared range %d status=%d", attempt, response.StatusCode)
		}
		if response.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("prepared range %d referrer policy=%q", attempt, response.Header.Get("Referrer-Policy"))
		}
	}
	restarted := NewServer(server.cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	restartedHTTP := httptest.NewServer(restarted.Handler())
	t.Cleanup(func() {
		restartedHTTP.Close()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = restarted.Shutdown(shutdownContext)
	})
	restartRequest, _ := http.NewRequest(http.MethodGet, restartedHTTP.URL+grant.DownloadURL, nil)
	restartRequest.Header.Set("Range", "bytes=5-9")
	restartRequest.Header.Set("Authorization", downloadAuthorization)
	restartResponse, err := client.Do(restartRequest)
	if err != nil {
		t.Fatalf("download prepared range after restart: %v", err)
	}
	_, _ = io.Copy(io.Discard, restartResponse.Body)
	_ = restartResponse.Body.Close()
	if restartResponse.StatusCode != http.StatusPartialContent {
		t.Fatalf("prepared range after restart status=%d", restartResponse.StatusCode)
	}
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/download-preparations/"+url.PathEscape(preparation.ID), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("remove source preparation status=%d body=%s", status, body)
	}
	assertDownloadGrantDenied(t, client, serverURL+grant.DownloadURL)
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/download-preparations/"+url.PathEscape(preparation.ID), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("removed preparation status=%d, expected 404", status)
	}
}

func TestDownloadPreparationConcurrentCreateIsUniqueAndServerScoped(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, _ = seedDownloadGrantMedia(t, db)
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load preparation owner: %v", err)
	}
	// Give the contenders distinct write mutexes while retaining one SQLite
	// database. This exercises the database invariant and transaction retry path
	// that protect two Portico processes, rather than merely proving that one
	// Server instance serializes requests in memory.
	contender := &Server{cfg: server.cfg, db: db, startedAt: server.startedAt}

	const callers = 12
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for index := range callers {
		wg.Add(1)
		go func(owner *Server) {
			defer wg.Done()
			preparation, createErr := owner.createDownloadPreparationContext(context.Background(), user, DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"})
			if createErr != nil {
				errs <- createErr
				return
			}
			ids <- preparation.ID
		}([]*Server{server, contender}[index%2])
	}
	wg.Wait()
	close(ids)
	close(errs)
	for failure := range errs {
		t.Fatalf("concurrent preparation failed: %v", failure)
	}
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	if len(unique) != 1 {
		t.Fatalf("concurrent create returned %d preparations: %v", len(unique), unique)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM download_preparations WHERE media_id = 'grant_media' AND removed_at = ''`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("active preparation count=%d err=%v", count, err)
	}
	var preparationID string
	for id := range unique {
		preparationID = id
	}
	if _, err := db.Exec(`UPDATE download_preparations SET server_id = 'different-server' WHERE id = ?`, preparationID); err != nil {
		t.Fatalf("move preparation to another server scope: %v", err)
	}
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/download-preparations/"+url.PathEscape(preparationID), nil, nil)
	if status != http.StatusNotFound || strings.Contains(strings.ToLower(body), "sqlite") {
		t.Fatalf("cross-server preparation status=%d body=%s", status, body)
	}
	var listed ListResponse[downloadPreparationView]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/download-preparations", nil, &listed)
	if status != http.StatusOK || listed.Total != 0 {
		t.Fatalf("cross-server preparation leaked into list status=%d body=%s listed=%#v", status, body, listed)
	}
}

func TestDownloadPreparationSharedJobCancellationIsInterestSafe(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, _ = seedDownloadGrantMedia(t, db)

	var first, second downloadPreparationView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "source"}, &first)
	if status != http.StatusCreated {
		t.Fatalf("create first preparation status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: "grant_other", QualityProfile: "source"}, &second)
	if status != http.StatusCreated {
		t.Fatalf("create second preparation status=%d body=%s", status, body)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES ('job_shared_download', 'optimize_version', 'queued', 0, 'Shared download', 'media', 'grant_media', '{"profile":"universal-720p"}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert shared download job: %v", err)
	}
	if _, err := db.Exec(`UPDATE download_preparations SET state = 'queued', quality_profile = 'universal-720p', job_id = 'job_shared_download', progress = 0, size_kind = 'estimated' WHERE id IN (?, ?)`, first.ID, second.ID); err != nil {
		t.Fatalf("attach shared download interests: %v", err)
	}
	var paused downloadPreparationView
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/download-preparations/"+url.PathEscape(first.ID), DownloadPreparationUpdateRequest{Action: "pause"}, &paused)
	if status != http.StatusOK || paused.State != "paused" {
		t.Fatalf("pause first preparation status=%d body=%s paused=%#v", status, body, paused)
	}
	job, err := server.getJob("job_shared_download")
	if err != nil || job.Status != "queued" {
		t.Fatalf("shared job cancelled while an interest remained: job=%#v err=%v", job, err)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/download-preparations/"+url.PathEscape(second.ID), DownloadPreparationUpdateRequest{Action: "cancel"}, nil)
	if status != http.StatusOK {
		t.Fatalf("cancel final preparation status=%d body=%s", status, body)
	}
	job, err = server.getJob("job_shared_download")
	if err != nil || job.Status != "cancelled" {
		t.Fatalf("orphaned shared job not cancelled atomically: job=%#v err=%v", job, err)
	}
}

func TestDownloadPreparationBatchAndNextEpisodeContracts(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, sourcePath := seedDownloadGrantMedia(t, db)
	seedDownloadGrantEpisodes(t, db, sourcePath)

	var batch downloadPreparationBatchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", downloadPreparationCreateEnvelope{ContainerID: "grant_season", QualityProfile: "source"}, &batch)
	if status != http.StatusCreated || len(batch.Items) != 2 || len(batch.Rejected) != 0 || batch.Total != 2 {
		t.Fatalf("season batch status=%d body=%s batch=%#v", status, body, batch)
	}
	var next downloadPreparationView
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", downloadPreparationCreateEnvelope{NextAfterMediaID: "grant_episode_1", QualityProfile: "source"}, &next)
	if status != http.StatusCreated || next.MediaID != "grant_episode_2" || next.ID == "" {
		t.Fatalf("next episode status=%d body=%s preparation=%#v", status, body, next)
	}
}

func TestDownloadPreparationPublicFailureCodeIsSanitized(t *testing.T) {
	record := downloadPreparationRecord{DownloadPreparation: DownloadPreparation{State: "failed", ErrorCode: "SQLITE_CONSTRAINT: secret table path"}, SizeKind: "not-valid"}
	view := downloadPreparationPublic(record)
	if view.ErrorCode != "preparation_failed" || view.SizeKind != "unknown" {
		t.Fatalf("unsafe preparation details escaped: %#v", view)
	}

	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, _ = seedDownloadGrantMedia(t, db)
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/download-preparations/missing", DownloadPreparationUpdateRequest{Action: "pause"}, nil)
	if status != http.StatusNotFound || strings.Contains(strings.ToLower(body), "sqlite") {
		t.Fatalf("raw database error escaped status=%d body=%s", status, body)
	}
}

func TestDownloadPreparationDistinguishesEstimatedAndExactArtifactSize(t *testing.T) {
	if state := normalizedDownloadPreparationJobState("complete"); state != "ready" {
		t.Fatalf("canonical completed optimize job materialized as %q", state)
	}
	videoEstimate := estimatedPreparedDownloadSize(MediaItem{Type: "movie", DurationSeconds: 3600}, "universal-720p")
	audioEstimate := estimatedPreparedDownloadSize(MediaItem{Type: "audiobook", DurationSeconds: 3600}, "universal-720p")
	if videoEstimate <= 0 || audioEstimate <= 0 || audioEstimate >= videoEstimate {
		t.Fatalf("media-type estimates are not meaningful: video=%d audio=%d", videoEstimate, audioEstimate)
	}

	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	client := authenticatedDownloadGrantClient(t, serverURL)
	_, _ = seedDownloadGrantMedia(t, db)
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","autoDelete":true,"retentionDays":7,"maxPerItem":3,"maxStorageMB":0}`)
	if _, err := server.normalizeMediaDownloadGrantProfile("universal-720p"); err != nil {
		t.Fatalf("normalize canonical optimized profile: %v", err)
	}
	var preparation downloadPreparationView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: "grant_media", QualityProfile: "universal-720p"}, &preparation)
	if status != http.StatusCreated || preparation.SizeKind != "estimated" || preparation.SizeBytes <= 0 || preparation.ArtifactExpiresAt != "" {
		t.Fatalf("queued optimized preparation status=%d body=%s preparation=%#v", status, body, preparation)
	}
}

func authenticatedDownloadGrantClient(t *testing.T, serverURL string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	return client
}

func seedDownloadGrantMedia(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	mediaRoot := t.TempDir()
	sourcePath := filepath.Join(mediaRoot, "Grant Download.mp4")
	if err := os.WriteFile(sourcePath, []byte("grant-download-body"), 0o600); err != nil {
		t.Fatalf("write grant source: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_grant', 'Grant downloads', 'movie', 101, ?, '{}', ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert grant library: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('lp_grant', 'lib_grant', ?, 0, ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert grant library path: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, 'lib_grant', ?)`, adminUserID(t, db), now); err != nil {
		t.Fatalf("grant library access: %v", err)
	}
	for _, media := range []struct{ id, title string }{{"grant_media", "Grant Download"}, {"grant_other", "Other Download"}} {
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url)
			VALUES (?, 'lib_grant', 'movie', ?, ?, ?, ?)`, media.id, media.title, media.title, now, sourcePath); err != nil {
			t.Fatalf("insert %s: %v", media.id, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO media_files (
			id, media_id, library_id, path, container, source_type, size_bytes, mod_time,
			available, first_seen_at, last_seen_at
		) VALUES ('mf_grant', 'grant_media', 'lib_grant', ?, 'mp4', 'local', ?, ?, 1, ?, ?)`,
		sourcePath, len("grant-download-body"), now, now, now); err != nil {
		t.Fatalf("insert grant media file: %v", err)
	}
	return mediaRoot, sourcePath
}

func seedDownloadGrantEpisodes(t *testing.T, db *sql.DB, sourcePath string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	stat, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat episode source: %v", err)
	}
	statSize := stat.Size()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES ('grant_show', 'lib_grant', 'show', 'Grant Show', 'Grant Show', ?)`, []any{now}},
		{`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, season_number, index_number, added_at) VALUES ('grant_season', 'lib_grant', 'grant_show', 'season', 'Season 1', 'Season 1', 1, 1, ?)`, []any{now}},
		{`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, season_number, episode_number, index_number, duration_seconds, source_url, added_at) VALUES ('grant_episode_1', 'lib_grant', 'grant_season', 'episode', 'Episode 1', 'Episode 1', 1, 1, 1, 1800, ?, ?)`, []any{sourcePath, now}},
		{`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, season_number, episode_number, index_number, duration_seconds, source_url, added_at) VALUES ('grant_episode_2', 'lib_grant', 'grant_season', 'episode', 'Episode 2', 'Episode 2', 1, 2, 2, 1800, ?, ?)`, []any{sourcePath, now}},
		{`INSERT INTO media_files (id, media_id, library_id, path, container, source_type, size_bytes, mod_time, available, first_seen_at, last_seen_at) VALUES ('mf_grant_ep1', 'grant_episode_1', 'lib_grant', ?, 'mp4', 'local', ?, ?, 1, ?, ?)`, []any{sourcePath, statSize, now, now, now}},
		{`INSERT INTO media_files (id, media_id, library_id, path, container, source_type, size_bytes, mod_time, available, first_seen_at, last_seen_at) VALUES ('mf_grant_ep2', 'grant_episode_2', 'lib_grant', ?, 'mp4', 'local', ?, ?, 1, ?, ?)`, []any{sourcePath, statSize, now, now, now}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed download episode contract: %v", err)
		}
	}
}

func createDownloadGrantForTest(t *testing.T, client *http.Client, serverURL, mediaID, profile string) MediaDownloadGrantResponse {
	t.Helper()
	var preparation downloadPreparationView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations", DownloadPreparationCreateRequest{MediaID: mediaID, QualityProfile: profile}, &preparation)
	if status != http.StatusCreated {
		t.Fatalf("create download preparation status=%d body=%s", status, body)
	}
	var grant MediaDownloadGrantResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/download-preparations/"+url.PathEscape(preparation.ID)+"/grant", downloadPreparationGrantRequest{Delivery: "browser"}, &grant)
	if status != http.StatusCreated {
		t.Fatalf("create download grant status=%d body=%s", status, body)
	}
	if grant.DownloadURL == "" || grant.ExpiresAt == "" || grant.Profile == "" {
		t.Fatalf("incomplete download grant response: %#v", grant)
	}
	if grant.GrantToken != "" || strings.Contains(body, "grantToken") {
		t.Fatalf("browser grant response exposed its credential: %s", body)
	}
	requestURL, err := url.Parse(serverURL + grant.DownloadURL)
	if err != nil {
		t.Fatalf("parse browser grant URL: %v", err)
	}
	for _, cookie := range client.Jar.Cookies(requestURL) {
		if cookie.Name == downloadGrantCookieName {
			grant.GrantToken = cookie.Value
			break
		}
	}
	return grant
}

func downloadGrantTokenForTest(t *testing.T, grant MediaDownloadGrantResponse) string {
	t.Helper()
	token := grant.GrantToken
	if !strings.HasPrefix(token, "ptc_dg_") {
		t.Fatalf("download grant response contains no opaque grant: %#v", grant)
	}
	if strings.Contains(grant.DownloadURL, "grant=") || strings.Contains(grant.DownloadURL, token) {
		t.Fatalf("download URL leaked its credential: %s", grant.DownloadURL)
	}
	return token
}

func assertDownloadGrantDenied(t *testing.T, client *http.Client, endpoint string) {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("send denied download grant: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), "download_grant_denied") {
		t.Fatalf("download grant status=%d body=%s", response.StatusCode, body)
	}
}

func TestReusableDownloadCredentialsAreRedactedFromLogs(t *testing.T) {
	raw := "GET /api/media/movie/download?profile=source&download_grant=ptc_dg_secret&media_grant=ptc_mg_secret"
	redacted := redactLogValue(raw)
	if strings.Contains(redacted, "ptc_dg_secret") || strings.Contains(redacted, "ptc_mg_secret") {
		t.Fatalf("server log value retained a reusable media credential: %q", redacted)
	}
	clientRedacted := redactClientLogText(raw)
	if strings.Contains(clientRedacted, "ptc_dg_secret") || strings.Contains(clientRedacted, "ptc_mg_secret") {
		t.Fatalf("client log upload retained a reusable media credential: %q", clientRedacted)
	}
}
