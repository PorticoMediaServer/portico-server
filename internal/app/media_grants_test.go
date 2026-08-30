package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func bindPlaybackSessionPlanForTest(t *testing.T, db *sql.DB, sessionID, mediaID string, isLive bool) {
	t.Helper()
	plan := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: "test:" + mediaID,
		SourceRevision: "test-revision:" + mediaID, CapabilityEvidenceID: "test-capability-v1",
		Mode: playbackplan.Remux, Protocol: "hls", Container: "mpegts",
		Streams:  []playbackplan.StreamAction{{Index: 0, Kind: "video", Action: playbackplan.Copy, InputCodec: "h264"}},
		Timeline: playbackplan.Timeline{Mode: map[bool]string{true: "live", false: "vod"}[isLive], Dynamic: isLive, Generation: 1},
		Subtitle: playbackplan.SubtitleDecision{Action: playbackplan.Drop},
	}
	plan.Digest, _ = plan.ComputeDigest()
	planJSON, _ := json.Marshal(plan)
	binding := playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: plan.SourceRevision, CapabilityEvidenceID: plan.CapabilityEvidenceID,
		Generation: 1, Mode: string(plan.Mode), Protocol: plan.Protocol, Container: plan.Container,
		Quality: "original", AudioMode: "auto", SubtitleMode: "off", DirectStream: true,
		X264Preset: "veryfast", Plan: planJSON,
	}
	if err := binding.seal(); err != nil {
		t.Fatalf("seal test playback plan: %v", err)
	}
	encoded, _ := json.Marshal(binding)
	if _, err := db.Exec(`UPDATE playback_sessions SET plan_schema_version=?, plan_digest=?, plan_json=?, source_revision=?, capability_evidence_id=?, playback_generation=? WHERE id=?`, binding.SchemaVersion, binding.Digest, string(encoded), binding.SourceRevision, binding.CapabilityEvidenceID, binding.Generation, sessionID); err != nil {
		t.Fatalf("bind test playback plan: %v", err)
	}
}

func playbackDecisionWithTestPlan(t *testing.T, decision PlaybackDecision, mediaID, subtitleMode, subtitleID string) PlaybackDecision {
	t.Helper()
	mode := playbackplan.VideoTranscode
	if decision.Mode == "direct_stream" {
		mode = playbackplan.Remux
	}
	plan := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: "test:" + mediaID,
		SourceRevision: "test-revision:" + mediaID, CapabilityEvidenceID: "test-capability-v1",
		Mode: mode, Protocol: "hls", Container: "mpegts",
		Streams:  []playbackplan.StreamAction{{Index: 0, Kind: "video", Action: playbackplan.Convert, InputCodec: "hevc", OutputCodec: "h264"}},
		Timeline: playbackplan.Timeline{Mode: "vod", Generation: 1},
		Subtitle: playbackplan.SubtitleDecision{Action: playbackplan.Drop},
	}
	if subtitleMode == "burn_in" {
		plan.Subtitle.Action = playbackplan.BurnIn
	} else if subtitleMode == "text" {
		plan.Subtitle.Action = playbackplan.ExternalText
	}
	plan.Digest, _ = plan.ComputeDigest()
	planJSON, _ := json.Marshal(plan)
	binding := playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: plan.SourceRevision, CapabilityEvidenceID: plan.CapabilityEvidenceID,
		Generation: 1, Mode: string(plan.Mode), Protocol: "hls", Container: "mpegts",
		Quality: "original", AudioMode: "auto", SubtitleMode: subtitleMode, SubtitleStreamID: subtitleID,
		X264Preset: "veryfast", Plan: planJSON,
	}
	if err := binding.seal(); err != nil {
		t.Fatalf("seal test decision: %v", err)
	}
	decision.Protocol, decision.Container, decision.DeliveryProfile = "hls", "mpegts", "original"
	decision.execution = &binding
	return decision
}

func TestPlaybackMediaGrantIsHashedScopedExpiringAndRevocable(t *testing.T) {
	serverURL, db, server := newEmptyAuthTestServerWithInstance(t)
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/setup", map[string]any{
		"serverName":            "Grant Test Server",
		"username":              "grant-owner",
		"email":                 "grant-owner@example.test",
		"displayName":           "Grant Owner",
		"password":              "Correct horse battery staple1",
		"setupMode":             "local_only",
		"localOnlyAcknowledged": true,
	}, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("setup owner status=%d body=%s", status, body)
	}
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'grant-owner'`).Scan(&userID); err != nil {
		t.Fatalf("query grant owner: %v", err)
	}
	var profileID string
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND is_primary = 1`, userID).Scan(&profileID); err != nil {
		t.Fatalf("query grant owner profile: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO media_items (id, type, title, sort_title, genres_json, tags_json, labels_json, added_at)
		VALUES ('media_grant_test', 'movie', 'Grant test', 'Grant test', '[]', '[]', '[]', ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert grant media: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state)
		VALUES ('play_grant_test', ?, ?, 'media_grant_test', 'movie', 'Grant test', ?, ?, 'playing')`,
		userID, profileID, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert playback session: %v", err)
	}
	bindPlaybackSessionPlanForTest(t, db, "play_grant_test", "media_grant_test", false)
	user := User{ID: userID, AccountID: userID, ProfileID: profileID, ProfileIsPrimary: true, Permissions: map[string]bool{"playMedia": true}}
	grant, err := server.issueMediaGrant(context.Background(), user, "play_grant_test", "media", "media_grant_test")
	if err != nil {
		t.Fatalf("issue media grant: %v", err)
	}
	if !strings.HasPrefix(grant.Token, "ptc_mg_") || grant.ExpiresAt == "" {
		t.Fatalf("unexpected grant: %#v", grant)
	}
	var storedHash string
	if err := db.QueryRow(`SELECT token_hash FROM playback_media_grants WHERE playback_session_id = 'play_grant_test' AND revoked_at = ''`).Scan(&storedHash); err != nil {
		t.Fatalf("query stored grant: %v", err)
	}
	if storedHash == grant.Token || storedHash != hashToken(grant.Token) {
		t.Fatalf("grant was not stored as a one-way hash")
	}

	request := mediaGrantRequest(http.MethodGet, "/api/media/media_grant_test/hls/segment?name=segment_00000.ts", grant.Token)
	resolved, err := server.userForMediaGrant(request)
	if err != nil || resolved.ID != userID {
		t.Fatalf("resolve valid grant user=%#v err=%v", resolved, err)
	}
	wrongMedia := mediaGrantRequest(http.MethodGet, "/api/media/other_media/stream", grant.Token)
	if _, err := server.userForMediaGrant(wrongMedia); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("cross-media grant unexpectedly authorized: %v", err)
	}
	adminRoute := mediaGrantRequest(http.MethodGet, "/api/media/media_grant_test", grant.Token)
	if _, err := server.userForMediaGrant(adminRoute); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("non-resource route unexpectedly authorized: %v", err)
	}
	soon := now.Add(time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE playback_media_grants SET expires_at = ? WHERE token_hash = ?`, soon, hashToken(grant.Token)); err != nil {
		t.Fatalf("shorten grant: %v", err)
	}
	if err := server.renewMediaGrantsForSession(context.Background(), user, "play_grant_test"); err != nil {
		t.Fatalf("renew media grant: %v", err)
	}
	var renewedExpiry string
	if err := db.QueryRow(`SELECT expires_at FROM playback_media_grants WHERE token_hash = ?`, hashToken(grant.Token)).Scan(&renewedExpiry); err != nil {
		t.Fatalf("query renewed expiry: %v", err)
	}
	if renewedExpiry <= soon {
		t.Fatalf("grant expiry was not extended: before=%s after=%s", soon, renewedExpiry)
	}
	if _, err := db.Exec(`UPDATE playback_media_grants SET expires_at = ? WHERE token_hash = ?`, now.Add(-time.Minute).Format(time.RFC3339), hashToken(grant.Token)); err != nil {
		t.Fatalf("expire grant: %v", err)
	}
	if _, err := server.userForMediaGrant(request); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("expired grant unexpectedly authorized: %v", err)
	}

	rotated, err := server.issueMediaGrant(context.Background(), user, "play_grant_test", "media", "media_grant_test")
	if err != nil {
		t.Fatalf("rotate grant: %v", err)
	}
	server.revokeMediaGrantsForSession(context.Background(), "play_grant_test")
	revokedRequest := mediaGrantRequest(http.MethodGet, "/api/media/media_grant_test/stream", rotated.Token)
	if _, err := server.userForMediaGrant(revokedRequest); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("revoked grant unexpectedly authorized: %v", err)
	}

	revisionBound, err := server.issueMediaGrantWithOptions(context.Background(), user, "play_grant_test", "media", "media_grant_test", true, false)
	if err != nil {
		t.Fatalf("issue authorization-revision media grant: %v", err)
	}
	if _, err := db.Exec(`UPDATE profiles SET policy_updated_at = ? WHERE id = ?`, now.Add(time.Minute).Format(time.RFC3339Nano), userID); err != nil {
		t.Fatalf("advance profile authorization revision: %v", err)
	}
	revisionRequest := httptest.NewRequest(http.MethodGet, "/api/media/media_grant_test/stream", nil)
	revisionRequest.Header.Set("Authorization", "PorticoMedia "+revisionBound.Token)
	if _, err := server.userForMediaGrant(revisionRequest); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("authorization-revision change did not revoke grant: %v", err)
	}
}

func TestMediaResourceAuthRejectsLongLivedAccountTokenInQuery(t *testing.T) {
	_, _, server := newEmptyAuthTestServerWithInstance(t)
	handler := server.withMediaResourceAuth(func(w http.ResponseWriter, _ *http.Request, _ User) {
		w.WriteHeader(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/media/example/stream?access_token=long-lived-device-token", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("long-lived query token status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if payload["code"] != "media_grant_denied" {
		t.Fatalf("unexpected rejection payload: %#v", payload)
	}
}

func TestDirectPlaybackSourceRevisionFence(t *testing.T) {
	binding := playbackExecutionBinding{SourceRevision: "revision-a"}
	if !playbackSourceRevisionMatches(binding, "revision-a") {
		t.Fatal("current direct-play source revision was rejected")
	}
	for _, revision := range []string{"", "revision-b"} {
		if playbackSourceRevisionMatches(binding, revision) {
			t.Fatalf("stale direct-play plan accepted source revision %q", revision)
		}
	}
}

func TestMediaGrantTransportRejectsQueryAndAcceptsHeaderOrCookie(t *testing.T) {
	query := httptest.NewRequest(http.MethodGet, "/api/live-tv/hls/channel/playlist.m3u8?media_grant=ptc_mg_query", nil)
	if token := mediaGrantFromRequest(query); token != "" {
		t.Fatalf("query media grant was accepted: %q", token)
	}
	header := mediaGrantRequest(http.MethodGet, "/api/live-tv/hls/channel/playlist.m3u8", "ptc_mg_header")
	if token := mediaGrantFromRequest(header); token != "ptc_mg_header" {
		t.Fatalf("Authorization media grant=%q", token)
	}
	cookie := httptest.NewRequest(http.MethodGet, "/api/live-tv/hls/channel/playlist.m3u8", nil)
	cookie.AddCookie(&http.Cookie{Name: mediaGrantCookieName, Value: "ptc_mg_cookie"})
	if token := mediaGrantFromRequest(cookie); token != "ptc_mg_cookie" {
		t.Fatalf("cookie media grant=%q", token)
	}
}

func TestPlaybackMediaGrantCookieIsHttpOnlySecureAndPathScoped(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "https://portico.example/api/library-channels/channel-cookie/tune", nil)
	setPlaybackMediaGrantCookie(recorder, request, PlaybackResponse{
		MediaGrant: MediaGrant{Token: "ptc_mg_cookie", ExpiresAt: time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)},
		Media:      MediaItem{ID: "channel-cookie", Type: "library_channel"},
	})
	cookie := recorder.Header().Get("Set-Cookie")
	for _, required := range []string{"portico_media_grant=ptc_mg_cookie", "Path=/api/library-channels/channel-cookie/hls", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(cookie, required) {
			t.Fatalf("media grant cookie omitted %q: %s", required, cookie)
		}
	}
}

func TestCapabilityCookieTLSUsesTrustedTransportContext(t *testing.T) {
	spoofed := httptest.NewRequest(http.MethodPost, "http://portico.example/api/playback", nil)
	spoofed.Header.Set("X-Forwarded-Proto", "https")
	if requestUsesTLS(spoofed) {
		t.Fatal("untrusted forwarded proto marked capability cookie secure")
	}

	trusted := httptest.NewRequest(http.MethodPost, "http://portico.example/api/playback", nil)
	trusted = trusted.WithContext(context.WithValue(trusted.Context(), requestTransportSecureKey{}, true))
	if !requestUsesTLS(trusted) {
		t.Fatal("trusted transport context did not mark capability cookie secure")
	}

	playbackRecorder := httptest.NewRecorder()
	setPlaybackMediaGrantCookie(playbackRecorder, trusted, PlaybackResponse{
		MediaGrant: MediaGrant{Token: "ptc_mg_trusted", ExpiresAt: time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)},
		Media:      MediaItem{ID: "trusted-cookie", Type: "movie"},
	})
	if !strings.Contains(playbackRecorder.Header().Get("Set-Cookie"), "Secure") {
		t.Fatal("playback capability cookie was not Secure under trusted TLS context")
	}

	downloadRecorder := httptest.NewRecorder()
	setMediaDownloadGrantCookie(downloadRecorder, trusted, "trusted-cookie", MediaDownloadGrantResponse{
		GrantToken: "ptc_dg_trusted",
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
	})
	if !strings.Contains(downloadRecorder.Header().Get("Set-Cookie"), "Secure") {
		t.Fatal("download capability cookie was not Secure under trusted TLS context")
	}
}

func TestLibraryChannelHLSRequiresScopedGrantEvenWithAccountSession(t *testing.T) {
	serverURL, db, server := newEmptyAuthTestServerWithInstance(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/setup", map[string]any{
		"serverName": "Library Channel Test Server",
		"username":   "library-channel-owner", "email": "library-channel-owner@example.test", "displayName": "Library Channel Owner",
		"password": "Correct horse battery staple1", "setupMode": "local_only", "localOnlyAcknowledged": true,
	}, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("setup owner status=%d body=%s", status, body)
	}
	var userID, profileID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username='library-channel-owner'`).Scan(&userID); err != nil {
		t.Fatalf("query owner: %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id=? ORDER BY is_primary DESC LIMIT 1`, userID).Scan(&profileID); err != nil {
		t.Fatalf("query profile: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,media_id,media_type,title,started_at,last_seen_at,state,is_live) VALUES ('play_library_channel_grant',?,?, 'channel_grant_test','library_channel','Grant Channel',?,?,'playing',1)`, userID, profileID, now, now); err != nil {
		t.Fatalf("insert live playback session: %v", err)
	}
	bindPlaybackSessionPlanForTest(t, db, "play_library_channel_grant", "channel_grant_test", true)
	unixNow := time.Now().UTC().Unix()
	if _, err := db.Exec(`
		INSERT INTO library_channels (id,name,timezone,seed,created_at,updated_at)
		VALUES ('channel_grant_test','Grant Channel','UTC','grant-channel-seed',?,?)`, unixNow, unixNow); err != nil {
		t.Fatalf("insert Library Channel: %v", err)
	}
	user := User{ID: userID, AccountID: userID, ProfileID: profileID, Role: "owner", Permissions: map[string]bool{"playMedia": true, "playLiveTV": true, "viewLiveTV": true, "manageServer": true}}
	policy := ResolvedPlaybackPolicy{
		QualityProfile: "720p-medium",
		LiveHLS:        &LiveHLSPlaybackPolicy{AuthorizationTransport: "header_or_secure_http_only_cookie", PlaylistScope: "playback_session", SegmentScope: "playback_session", CredentialQueryAllowed: false},
		LiveDelivery: &PlaybackDeliveryPolicy{
			DeliveryMode: "server_hls", GrantRequired: true,
			AllowedOperationClasses: []string{"manifest", "segment"}, QualityProfile: "720p-medium", ResourceRevision: 1,
		},
	}
	if err := server.bindLibraryChannelDeliveryPolicy(context.Background(), "play_library_channel_grant", "channel_grant_test", user, policy); err != nil {
		t.Fatalf("bind Library Channel delivery policy: %v", err)
	}
	grant, err := server.issueMediaGrant(context.Background(), user, "play_library_channel_grant", "live_channel", "channel_grant_test")
	if err != nil {
		t.Fatalf("issue Library Channel grant: %v", err)
	}

	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/library-channels/channel_grant_test/hls/playlist.m3u8", nil, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":"media_grant_denied"`) {
		t.Fatalf("account session bypassed manifest grant: status=%d body=%s", status, body)
	}
	status, body = doMediaGrantGET(t, client, serverURL+"/api/library-channels/channel_grant_test/hls/playlist.m3u8", grant.Token)
	if status == http.StatusUnauthorized {
		t.Fatalf("valid scoped manifest grant was rejected: status=%d body=%s", status, body)
	}
	status, body = doMediaGrantGET(t, client, serverURL+"/api/library-channels/other-channel/hls/playlist.m3u8", grant.Token)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":"media_grant_denied"`) {
		t.Fatalf("cross-channel grant authorized: status=%d body=%s", status, body)
	}
	if _, err := db.Exec(`DELETE FROM playback_sessions WHERE id = 'play_library_channel_grant'`); err != nil {
		t.Fatalf("delete playback session: %v", err)
	}
	var policies int
	if err := db.QueryRow(`SELECT COUNT(*) FROM library_channel_playback_policies WHERE playback_session_id = 'play_library_channel_grant'`).Scan(&policies); err != nil || policies != 0 {
		t.Fatalf("Library Channel policy did not cascade with session: count=%d err=%v", policies, err)
	}
	if _, err := db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,media_id,media_type,title,started_at,last_seen_at,state,is_live) VALUES ('play_library_channel_expired',?,?, 'channel_grant_test','library_channel','Expired Channel',?,?,'playing',1)`, userID, profileID, now, now); err != nil {
		t.Fatalf("insert expiring playback session: %v", err)
	}
	bindPlaybackSessionPlanForTest(t, db, "play_library_channel_expired", "channel_grant_test", true)
	if err := server.bindLibraryChannelDeliveryPolicy(context.Background(), "play_library_channel_expired", "channel_grant_test", user, policy); err != nil {
		t.Fatalf("bind expiring Library Channel policy: %v", err)
	}
	if _, err := db.Exec(`UPDATE library_channel_playback_policies SET expires_at=? WHERE playback_session_id='play_library_channel_expired'`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatalf("expire Library Channel policy: %v", err)
	}
	if err := server.expireStalePlaybackSessions(time.Now().UTC()); err != nil {
		t.Fatalf("prune expired Library Channel policy: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM library_channel_playback_policies WHERE playback_session_id = 'play_library_channel_expired'`).Scan(&policies); err != nil || policies != 0 {
		t.Fatalf("expired Library Channel policy was retained: count=%d err=%v", policies, err)
	}
}

func TestLibraryChannelResolvedPlaybackPolicyIsOperationScoped(t *testing.T) {
	response := PlaybackResponse{Policy: ResolvedPlaybackPolicy{DeliveryProfile: "library-channel-hls", LiveDelivery: &PlaybackDeliveryPolicy{
		DeliveryMode: "server_hls", GrantRequired: true, AllowedOperationClasses: []string{"manifest", "segment"},
		AuthorizationRecheckSeconds: 60, QualityProfile: "720p-medium", OverlayTranscode: true,
	}}}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"deliveryMode":"server_hls"`, `"grantRequired":true`, `"allowedOperationClasses":["manifest","segment"]`, `"qualityProfile":"720p-medium"`, `"overlayTranscode":true`} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("Library Channel playback policy omitted %s: %s", expected, encoded)
		}
	}
}

func mediaGrantRequest(method, target, token string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Authorization", "PorticoMedia "+token)
	return request
}

func doMediaGrantGET(t *testing.T, client *http.Client, endpoint, token string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "PorticoMedia "+token)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(body)
}

func errorsIsMediaGrantDenied(err error) bool {
	return errors.Is(err, errMediaGrantDenied)
}
