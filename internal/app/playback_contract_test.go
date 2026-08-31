package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func doPlaybackContinuationJSON(t *testing.T, client *http.Client, method, endpoint, token string, origin string, payload any, out any) (int, string) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "PorticoPlayback "+token)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set(csrfHeaderName, "1")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			t.Fatalf("decode continuation response: %v\n%s", err, responseBody)
		}
	}
	return resp.StatusCode, string(responseBody)
}

func TestCanonicalPlaybackSessionRouteRejectsRetiredStartPath(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback/start", map[string]any{"mediaId": "movie_meridian"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("retired playback start status = %d, body: %s", status, body)
	}

	var playback PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK || playback.SessionID == "" || playback.NextEventSequence != 1 {
		t.Fatalf("canonical playback session status=%d response=%#v body=%s", status, playback, body)
	}
}

func TestPlaybackContinuationIsScopedRotatableAndDurablyAcknowledgesProgress(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK || playback.ContinuationCredential == nil || playback.ContinuationCredential.Token == "" {
		t.Fatalf("playback continuation was not issued: status=%d body=%s", status, body)
	}
	continuationURL := serverURL + "/api/playback-sessions/" + playback.SessionID + "/continuation"
	var acknowledgement PlaybackProgressAcknowledgement
	status, body = doPlaybackContinuationJSON(t, client, http.MethodPatch, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"eventSequence": 1, "recordedAt": time.Now().UTC().Format(time.RFC3339Nano), "positionSeconds": 2}, &acknowledgement)
	if status != http.StatusOK || !acknowledgement.Accepted || acknowledgement.HighestEventSequence != 1 {
		t.Fatalf("continuation progress was not accepted: status=%d acknowledgement=%#v body=%s", status, acknowledgement, body)
	}
	status, body = doPlaybackContinuationJSON(t, client, http.MethodPatch, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"eventSequence": 1, "recordedAt": time.Now().UTC().Format(time.RFC3339Nano), "positionSeconds": 2}, &acknowledgement)
	if status != http.StatusOK || !acknowledgement.Duplicate || acknowledgement.HighestEventSequence != 1 {
		t.Fatalf("continuation duplicate was not acknowledged: status=%d acknowledgement=%#v body=%s", status, acknowledgement, body)
	}
	status, body = doPlaybackContinuationJSON(t, client, http.MethodGet, continuationURL, playback.ContinuationCredential.Token, "https://wrong.example", nil, nil)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("origin mismatch status=%d body=%s", status, body)
	}
	var rotated PlaybackContinuationCredential
	status, body = doPlaybackContinuationJSON(t, client, http.MethodPost, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"requestId": "rotation-1"}, &rotated)
	if status != http.StatusOK || rotated.Token == "" || rotated.Token == playback.ContinuationCredential.Token {
		t.Fatalf("continuation rotation failed: status=%d rotated=%#v body=%s", status, rotated, body)
	}
	var recoveredRotation PlaybackContinuationCredential
	status, body = doPlaybackContinuationJSON(t, client, http.MethodPost, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"requestId": "rotation-1"}, &recoveredRotation)
	if status != http.StatusOK || recoveredRotation.Token != rotated.Token || recoveredRotation.ExpiresAt != rotated.ExpiresAt || recoveredRotation.Origin != rotated.Origin || recoveredRotation.Generation != rotated.Generation {
		t.Fatalf("lost-response rotation retry did not recover the exact credential: status=%d first=%#v recovered=%#v body=%s", status, rotated, recoveredRotation, body)
	}
	status, body = doPlaybackContinuationJSON(t, client, http.MethodDelete, continuationURL, rotated.Token, "", map[string]any{
		"requestId": "continuation-stop-1",
		"terminal": map[string]any{
			"disposition": "stopped", "generation": rotated.Generation, "eventSequence": 2,
			"recordedAt": time.Now().UTC().Format(time.RFC3339Nano), "positionSeconds": 2, "durationSeconds": 0,
		},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("continuation revoke status=%d body=%s", status, body)
	}
	status, body = doPlaybackContinuationJSON(t, client, http.MethodGet, continuationURL, rotated.Token, "", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked continuation status=%d body=%s", status, body)
	}
}

func TestPlaybackContinuationRotationReceiptFailureRollsBackCredentialAndRetryIsIdempotent(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK || playback.ContinuationCredential == nil || playback.ContinuationCredential.Token == "" {
		t.Fatalf("playback continuation was not issued: status=%d body=%s", status, body)
	}
	continuationURL := serverURL + "/api/playback-sessions/" + playback.SessionID + "/continuation"
	oldHash := hashToken(playback.ContinuationCredential.Token)
	const triggerName = "reject_playback_continuation_rotation_receipt"
	if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + ` BEFORE UPDATE OF last_rotation_receipt ON playback_session_continuation_credentials BEGIN SELECT RAISE(ABORT, 'test rotation receipt persistence failure'); END`); err != nil {
		t.Fatalf("create rotation receipt failure trigger: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP TRIGGER ` + triggerName) })

	status, body = doPlaybackContinuationJSON(t, client, http.MethodPost, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"requestId": "rotation-atomic-1"}, nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("failed atomic rotation status=%d body=%s", status, body)
	}
	var tokenHash, lastRequestID, receipt string
	if err := db.QueryRow(`SELECT token_hash, last_rotation_request_id, last_rotation_receipt FROM playback_session_continuation_credentials WHERE playback_session_id = ?`, playback.SessionID).Scan(&tokenHash, &lastRequestID, &receipt); err != nil {
		t.Fatalf("read rolled-back continuation rotation: %v", err)
	}
	if tokenHash != oldHash || lastRequestID != "" || receipt != "" {
		t.Fatalf("failed rotation partially committed token=%q request=%q receipt=%q", tokenHash, lastRequestID, receipt)
	}
	if _, err := db.Exec(`DROP TRIGGER ` + triggerName); err != nil {
		t.Fatalf("drop rotation receipt failure trigger: %v", err)
	}

	var rotated, retried PlaybackContinuationCredential
	status, body = doPlaybackContinuationJSON(t, client, http.MethodPost, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"requestId": "rotation-atomic-1"}, &rotated)
	if status != http.StatusOK || rotated.Token == "" || rotated.Token == playback.ContinuationCredential.Token {
		t.Fatalf("rotation retry failed: status=%d rotated=%#v body=%s", status, rotated, body)
	}
	status, body = doPlaybackContinuationJSON(t, client, http.MethodPost, continuationURL, playback.ContinuationCredential.Token, "", map[string]any{"requestId": "rotation-atomic-1"}, &retried)
	if status != http.StatusOK || retried.Token != rotated.Token || retried.ExpiresAt != rotated.ExpiresAt || retried.Origin != rotated.Origin || retried.Generation != rotated.Generation {
		t.Fatalf("idempotent rotation retry changed durable receipt: status=%d first=%#v retry=%#v body=%s", status, rotated, retried, body)
	}
}

func playbackContinuationExtensionTestSetup(t *testing.T) (*Server, *sql.DB, PlaybackResponse) {
	t.Helper()
	serverURL, db, server := newAuthTestServerWithInstance(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK || playback.ContinuationCredential == nil || playback.ContinuationCredential.Token == "" {
		t.Fatalf("start playback status=%d response=%#v body=%s", status, playback, body)
	}

	dueSoon := time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE playback_session_continuation_credentials SET expires_at = ? WHERE playback_session_id = ?`, dueSoon, playback.SessionID); err != nil {
		t.Fatalf("make continuation due for extension: %v", err)
	}
	return server, db, playback
}

func playbackContinuationExtensionRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "/api/playback-sessions/test/continuation", nil)
	req.Header.Set("Authorization", "PorticoPlayback "+token)
	return req
}

func playbackContinuationExpiry(t *testing.T, db *sql.DB, sessionID string) string {
	t.Helper()
	var expiry string
	if err := db.QueryRow(`SELECT expires_at FROM playback_session_continuation_credentials WHERE playback_session_id = ?`, sessionID).Scan(&expiry); err != nil {
		t.Fatalf("read continuation expiry: %v", err)
	}
	return expiry
}

func TestPlaybackContinuationExtensionDoesNotReportExpiryAfterWriteFailure(t *testing.T) {
	server, db, playback := playbackContinuationExtensionTestSetup(t)
	before := playbackContinuationExpiry(t, db, playback.SessionID)
	const triggerName = "reject_playback_continuation_extension"
	if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + ` BEFORE UPDATE OF expires_at ON playback_session_continuation_credentials BEGIN SELECT RAISE(ABORT, 'test continuation persistence failure'); END`); err != nil {
		t.Fatalf("create continuation failure trigger: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP TRIGGER ` + triggerName) })

	if got := server.extendPlaybackContinuation(playbackContinuationExtensionRequest(playback.ContinuationCredential.Token), playback.SessionID); got != "" {
		t.Fatalf("failed continuation extension returned uncommitted expiry %q", got)
	}
	if after := playbackContinuationExpiry(t, db, playback.SessionID); after != before {
		t.Fatalf("failed continuation extension changed expiry from %q to %q", before, after)
	}
}

func TestPlaybackContinuationExtensionRequiresOneAffectedRow(t *testing.T) {
	server, db, playback := playbackContinuationExtensionTestSetup(t)
	before := playbackContinuationExpiry(t, db, playback.SessionID)
	const triggerName = "ignore_playback_continuation_extension"
	if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + ` BEFORE UPDATE OF expires_at ON playback_session_continuation_credentials BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatalf("create continuation zero-row trigger: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP TRIGGER ` + triggerName) })

	if got := server.extendPlaybackContinuation(playbackContinuationExtensionRequest(playback.ContinuationCredential.Token), playback.SessionID); got != "" {
		t.Fatalf("zero-row continuation extension returned expiry %q", got)
	}
	if after := playbackContinuationExpiry(t, db, playback.SessionID); after != before {
		t.Fatalf("zero-row continuation extension changed expiry from %q to %q", before, after)
	}
}

func TestPlaybackContinuationExtensionRetriesAfterFailureAndIsIdempotent(t *testing.T) {
	server, db, playback := playbackContinuationExtensionTestSetup(t)
	const triggerName = "retry_playback_continuation_extension"
	if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + ` BEFORE UPDATE OF expires_at ON playback_session_continuation_credentials BEGIN SELECT RAISE(ABORT, 'test continuation retry failure'); END`); err != nil {
		t.Fatalf("create continuation retry trigger: %v", err)
	}
	if got := server.extendPlaybackContinuation(playbackContinuationExtensionRequest(playback.ContinuationCredential.Token), playback.SessionID); got != "" {
		t.Fatalf("failed first continuation extension returned expiry %q", got)
	}
	if _, err := db.Exec(`DROP TRIGGER ` + triggerName); err != nil {
		t.Fatalf("drop continuation retry trigger: %v", err)
	}

	first := server.extendPlaybackContinuation(playbackContinuationExtensionRequest(playback.ContinuationCredential.Token), playback.SessionID)
	if first == "" {
		t.Fatal("continuation retry did not return the committed expiry")
	}
	if persisted := playbackContinuationExpiry(t, db, playback.SessionID); persisted != first {
		t.Fatalf("continuation retry returned %q but persisted %q", first, persisted)
	}
	second := server.extendPlaybackContinuation(playbackContinuationExtensionRequest(playback.ContinuationCredential.Token), playback.SessionID)
	if second != first {
		t.Fatalf("idempotent continuation retry returned %q after %q", second, first)
	}
}

func TestPlaybackContinuationRevalidatesProfileAndGeneration(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	start := func() PlaybackResponse {
		var playback PlaybackResponse
		status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
		if status != http.StatusOK || playback.ContinuationCredential == nil {
			t.Fatalf("start playback status=%d body=%s", status, body)
		}
		return playback
	}

	playback := start()
	if _, err := db.Exec(`UPDATE playback_sessions SET progress_generation = progress_generation + 1 WHERE id = ?`, playback.SessionID); err != nil {
		t.Fatal(err)
	}
	status, body := doPlaybackContinuationJSON(t, client, http.MethodGet, serverURL+"/api/playback-sessions/"+playback.SessionID+"/continuation", playback.ContinuationCredential.Token, "", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("stale continuation generation status=%d body=%s", status, body)
	}

	playback = start()
	var profileID string
	if err := db.QueryRow(`SELECT profile_id FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE profiles SET disabled_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), profileID); err != nil {
		t.Fatal(err)
	}
	status, body = doPlaybackContinuationJSON(t, client, http.MethodGet, serverURL+"/api/playback-sessions/"+playback.SessionID+"/continuation", playback.ContinuationCredential.Token, "", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("disabled-profile continuation status=%d body=%s", status, body)
	}
}

func TestPlaybackProgressAcknowledgementRollsBackWhenMediaStateCannotPersist(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", status, body)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_progress_state BEFORE INSERT ON user_media_state WHEN NEW.media_id = 'movie_meridian' BEGIN SELECT RAISE(ABORT, 'test progress persistence failure'); END`); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"eventSequence": 1, "generation": playback.Generation, "recordedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"positionSeconds": 120, "durationSeconds": playbackProgressTestDurationSeconds,
	}
	status, _ = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, payload, nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("progress persistence failure status=%d, want 500", status)
	}
	var sequence int64
	if err := db.QueryRow(`SELECT last_event_sequence FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if sequence != 0 {
		t.Fatalf("failed durable progress advanced sequence to %d", sequence)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_progress_state`); err != nil {
		t.Fatal(err)
	}
	var acknowledgement PlaybackProgressAcknowledgement
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, payload, &acknowledgement)
	if status != http.StatusOK || !acknowledgement.Accepted || acknowledgement.HighestEventSequence != 1 {
		t.Fatalf("durable progress retry status=%d acknowledgement=%#v body=%s", status, acknowledgement, body)
	}
}

func TestPlaybackRenegotiationValidatesBeforeAdvancingRevision(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{
		MediaID: "movie_meridian", SkipPreroll: true,
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{Device: "iPhone", Platform: "ios", SupportsHLS: true, MaxWidth: 1920, MaxHeight: 1080}),
		Intent:        PlaybackIntent{TransportClass: "wifi", Quality: PlaybackQualitySelection{Mode: playbackQualityModeAutomatic}},
	}, &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", status, body)
	}
	invalidAudio := "audio_does_not_exist"
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+playback.SessionID+"/renegotiate", PlaybackRenegotiationRequest{
		RequestID: "invalid-audio", ExpectedRevision: 0, AudioStreamID: &invalidAudio,
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid renegotiation status=%d body=%s", status, body)
	}
	var revision int64
	var profileJSON, intentJSON string
	if err := db.QueryRow(`SELECT renegotiation_revision, client_profile_json, playback_intent_json FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&revision, &profileJSON, &intentJSON); err != nil {
		t.Fatal(err)
	}
	if revision != 0 {
		t.Fatalf("invalid renegotiation advanced revision to %d", revision)
	}
	if !strings.Contains(profileJSON, `"platform":"ios"`) || !strings.Contains(intentJSON, `"transportClass":"wifi"`) || !strings.Contains(intentJSON, `"mode":"automatic"`) {
		t.Fatalf("canonical playback inputs were not persisted: profile=%s intent=%s", profileJSON, intentJSON)
	}
}

func TestSilentVideoResponseTracksDoNotInventAudio(t *testing.T) {
	audio, subtitles := playbackResponseTracks(MediaItem{Streams: []Stream{{ID: "v1", Kind: "video", Codec: "h264"}}})
	if len(audio) != 0 {
		t.Fatalf("silent-video response invented audio tracks: %#v", audio)
	}
	if len(subtitles) != 1 || subtitles[0].ID != "sub_none" {
		t.Fatalf("subtitle-off selection missing: %#v", subtitles)
	}
}

func TestPlaybackRenegotiationPreservesOmittedSelectionsAndReplaysIdempotently(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var started PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{
		MediaID: "movie_meridian", SkipPreroll: true,
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{Device: "iPhone", Platform: "ios", SupportsHLS: true, MaxWidth: 1920, MaxHeight: 1080}),
		Intent:        PlaybackIntent{TransportClass: "wifi", Quality: PlaybackQualitySelection{Mode: playbackQualityModeAutomatic}},
	}, &started)
	if status != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", status, body)
	}
	var beforeAudio, beforeSubtitle, beforeMode, beforeVersion string
	if err := db.QueryRow(`
		SELECT selected_audio_stream_id, selected_subtitle_stream_id, selected_subtitle_mode, selected_version_id
		FROM playback_sessions WHERE id = ?`, started.SessionID,
	).Scan(&beforeAudio, &beforeSubtitle, &beforeMode, &beforeVersion); err != nil {
		t.Fatal(err)
	}

	req := PlaybackRenegotiationRequest{RequestID: "preserve-and-replay", ExpectedRevision: 0}
	var first PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+started.SessionID+"/renegotiate", req, &first)
	if status != http.StatusOK || first.PlaybackRevision != 1 || first.ContinuationCredential == nil || first.ContinuationCredential.Token == "" {
		t.Fatalf("first renegotiation status=%d revision=%d body=%s", status, first.PlaybackRevision, body)
	}
	var afterAudio, afterSubtitle, afterMode, afterVersion, profileJSON, intentJSON string
	if err := db.QueryRow(`
		SELECT selected_audio_stream_id, selected_subtitle_stream_id, selected_subtitle_mode,
			selected_version_id, client_profile_json, playback_intent_json
		FROM playback_sessions WHERE id = ?`, started.SessionID,
	).Scan(&afterAudio, &afterSubtitle, &afterMode, &afterVersion, &profileJSON, &intentJSON); err != nil {
		t.Fatal(err)
	}
	if beforeAudio != afterAudio || beforeSubtitle != afterSubtitle || beforeMode != afterMode || beforeVersion != afterVersion {
		t.Fatalf("omitted renegotiation fields changed selection: before=%q/%q/%q/%q after=%q/%q/%q/%q",
			beforeAudio, beforeSubtitle, beforeMode, beforeVersion,
			afterAudio, afterSubtitle, afterMode, afterVersion)
	}
	if !strings.Contains(profileJSON, `"platform":"ios"`) || !strings.Contains(intentJSON, `"transportClass":"wifi"`) {
		t.Fatalf("omitted canonical inputs were not preserved: profile=%s intent=%s", profileJSON, intentJSON)
	}

	var replay PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+started.SessionID+"/renegotiate", req, &replay)
	if status != http.StatusOK || replay.PlaybackRevision != 1 || replay.SessionID != started.SessionID || replay.ContinuationCredential == nil || replay.ContinuationCredential.Token == "" {
		t.Fatalf("idempotent replay status=%d response=%#v body=%s", status, replay, body)
	}
	explicitOff := "off"
	req.SubtitleMode = &explicitOff
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+started.SessionID+"/renegotiate", req, nil)
	if status != http.StatusConflict || !strings.Contains(body, `"code":"renegotiation_request_conflict"`) {
		t.Fatalf("request ID fingerprint reuse status=%d body=%s", status, body)
	}
}

func TestRetiredMediaProgressRouteIsUnavailable(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/progress", map[string]any{
		"progressSeconds": 120,
		"watched":         false,
	}, nil)
	if status != http.StatusNotFound || !strings.Contains(body, `"code":"not_found"`) {
		t.Fatalf("retired media progress status=%d body=%s", status, body)
	}
}

func TestPlaybackProgressContractRequiresOrderingMetadata(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("create playback session status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"state":           "playing",
		"positionSeconds": 120,
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, `"code":"invalid_event_sequence"`) {
		t.Fatalf("missing ordering metadata status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      "not-a-time",
		"state":           "playing",
		"positionSeconds": 120,
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, `"code":"invalid_recorded_at"`) {
		t.Fatalf("invalid recordedAt status=%d body=%s", status, body)
	}
}

func TestAtomicCompletedPlaybackStopPersistsTerminalStateAndCannotBeRevived(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("create playback session status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"requestId": "completed-stop-1",
		"terminal": map[string]any{
			"disposition":     "completed",
			"generation":      playback.Generation,
			"eventSequence":   playback.NextEventSequence,
			"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
			"positionSeconds": playbackProgressTestDurationSeconds,
			"durationSeconds": playbackProgressTestDurationSeconds,
		},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("atomic completed stop status=%d body=%s", status, body)
	}
	var position, progress int
	var state, endedAt string
	if err := db.QueryRow(`SELECT position_seconds, progress, state, ended_at FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&position, &progress, &state, &endedAt); err != nil {
		t.Fatal(err)
	}
	if position != playbackProgressTestDurationSeconds || progress != 100 || state != "stopped" || endedAt == "" {
		t.Fatalf("terminal state position=%d progress=%d state=%q endedAt=%q", position, progress, state, endedAt)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   2,
		"recordedAt":      time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
		"state":           "playing",
		"positionSeconds": 30,
	}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("revive stopped playback status=%d body=%s", status, body)
	}

	var item MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK || !item.State.Watched || item.State.Resume != nil {
		t.Fatalf("completed media state status=%d state=%#v body=%s", status, item.State, body)
	}
}

func TestAtomicPlaybackStopRejectsStaleAuthorityAndLegacyCompletedPatch(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("create playback session status=%d body=%s", status, body)
	}
	terminalEvent := map[string]any{
		"disposition": "completed", "generation": playback.Generation,
		"eventSequence": playback.NextEventSequence, "recordedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"positionSeconds": playbackProgressTestDurationSeconds, "durationSeconds": playbackProgressTestDurationSeconds,
	}
	terminal := map[string]any{"requestId": "stale-terminal-1", "terminal": terminalEvent}
	terminalEvent["generation"] = playback.Generation + 1
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, terminal, nil)
	if status != http.StatusConflict || !strings.Contains(body, `"code":"playback_generation_stale"`) {
		t.Fatalf("stale generation status=%d body=%s", status, body)
	}
	terminalEvent["generation"] = playback.Generation
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence": playback.NextEventSequence, "generation": playback.Generation,
		"recordedAt": time.Now().UTC().Format(time.RFC3339Nano), "positionSeconds": 10,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("seed progress status=%d body=%s", status, body)
	}
	terminalEvent["eventSequence"] = playback.NextEventSequence
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, terminal, nil)
	if status != http.StatusConflict || !strings.Contains(body, `"code":"playback_event_sequence_stale"`) {
		t.Fatalf("stale sequence status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence": playback.NextEventSequence, "recordedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"positionSeconds": playbackProgressTestDurationSeconds, "durationSeconds": playbackProgressTestDurationSeconds, "completed": true,
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, `"code":"bad_json"`) {
		t.Fatalf("legacy completed PATCH status=%d body=%s", status, body)
	}
}
