package app

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestCastBootstrapEnvelopeBindsReceiverKeyAndScope(t *testing.T) {
	receiverKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := makeCastBootstrapEnvelope("bootstrap-1", "cast-app-1", "https://cast.getportico.tv", "https://server.example", "challenge-123456789", receiverKey.PublicKey(), "ptc_cb_secret", "2999-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var envelope castBootstrapEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	serverPublicRaw, _ := base64.RawURLEncoding.DecodeString(envelope.ServerPublicKey)
	serverPublic, err := ecdh.P256().NewPublicKey(serverPublicRaw)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := receiverKey.ECDH(serverPublic)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := castEnvelopeAEAD(shared)
	if err != nil {
		t.Fatal(err)
	}
	nonce, _ := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	ciphertext, _ := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	plain, err := aead.Open(nil, nonce, ciphertext, castEnvelopeAAD(envelope.BootstrapID, envelope.ReceiverID, envelope.ReceiverOrigin, envelope.ServerOrigin, envelope.ReceiverChallenge))
	if err != nil {
		t.Fatal("receiver key could not decrypt envelope:", err)
	}
	var payload castBootstrapSecretPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Secret != "ptc_cb_secret" || payload.BootstrapID != envelope.BootstrapID {
		t.Fatalf("unexpected envelope payload: %#v", payload)
	}
	if _, err := aead.Open(nil, nonce, ciphertext, castEnvelopeAAD(envelope.BootstrapID, envelope.ReceiverID, envelope.ReceiverOrigin, "https://attacker.example", envelope.ReceiverChallenge)); err == nil {
		t.Fatal("envelope decrypted with an off-origin AAD")
	}
}

func TestCastCORSIsExactAndPurposeSpecific(t *testing.T) {
	server := &Server{cfg: config.Config{CastReceiverOrigins: []string{"https://cast.getportico.tv"}}}
	request := httptest.NewRequest(http.MethodOptions, "https://server.example/api/media/movie-1/hls/master.m3u8", nil)
	request.Header.Set("Origin", "https://cast.getportico.tv")
	request.Header.Set("Access-Control-Request-Method", "GET")
	request.Header.Set("Access-Control-Request-Headers", "Authorization, Range")
	response := httptest.NewRecorder()
	if !server.applyCastCORS(response, request, request.Header.Get("Origin")) {
		t.Fatal("expected configured Cast origin")
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "https://cast.getportico.tv" || !strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), "Range") || response.Header().Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("unexpected Cast CORS response: %v", response.Header())
	}
	bad := httptest.NewRequest(http.MethodOptions, "https://server.example/api/media/movie-1/hls/master.m3u8", nil)
	bad.Header.Set("Origin", "https://cast.getportico.tv")
	bad.Header.Set("Access-Control-Request-Method", "POST")
	if server.applyCastCORS(httptest.NewRecorder(), bad, bad.Header.Get("Origin")) {
		t.Fatal("media CORS accepted an unapproved method")
	}
	if castCORSPath("/api/media/movie-1") || castCORSPath("/api/libraries/lib-1") {
		t.Fatal("Cast CORS widened to ordinary account/catalog routes")
	}
}

func TestCastBootstrapResponseDoesNotPublishBearerSecret(t *testing.T) {
	encoded, err := json.Marshal(CastBootstrapResponse{Version: castProtocolVersion, BootstrapEnvelope: "ciphertext", BootstrapID: "bootstrap-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "bootstrapToken") || strings.Contains(string(encoded), "ptc_cb_") {
		t.Fatalf("bootstrap response leaked a bearer secret: %s", encoded)
	}
}

func TestCastBootstrapRequestPublishesExplicitSubtitleSelection(t *testing.T) {
	encoded, err := json.Marshal(CastBootstrapRequest{
		SourceKind:       "media",
		SourceID:         "movie-1",
		SubtitleStreamID: "subtitle-french",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"subtitleStreamId":"subtitle-french"`) {
		t.Fatalf("Cast bootstrap omitted explicit text subtitle selection: %s", encoded)
	}
}

func TestCastReceiverHandoffRetiresSourceAndClearsPointerAtomically(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	user := User{ID: userID, AccountID: userID, ProfileID: userID, ProfileIsPrimary: true, Role: "owner", Permissions: ownerPermissions()}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, session := range []struct {
		id, mediaID string
	}{
		{id: "cast-source-session", mediaID: "movie_meridian"},
		{id: "cast-receiver-playback", mediaID: "movie_meridian"},
	} {
		if _, err := db.Exec(`
			INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state)
			VALUES (?, ?, ?, ?, 'movie', 'Meridian', ?, ?, 'playing')`,
			session.id, userID, userID, session.mediaID, now, now); err != nil {
			t.Fatalf("insert %s: %v", session.id, err)
		}
	}
	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO cast_receiver_sessions (
			id, token_hash, user_id, profile_id, receiver_id, receiver_origin, server_origin,
			playback_session_id, source_playback_session_id, client_instance_id, generation,
			capabilities_json, automation_json, status, expires_at, last_seen_at, created_at
		) VALUES ('cast-receiver-session', 'token-hash', ?, ?, 'receiver', 'https://cast.getportico.tv',
			'https://server.example', 'cast-receiver-playback', 'cast-source-session', 'installation-1',
			1, '[]', '{}', 'active', ?, ?, ?)`, userID, userID, expiresAt, now, now); err != nil {
		t.Fatalf("insert Cast receiver session: %v", err)
	}
	auth := castSessionAuth{
		user: user,
		record: castReceiverRecord{
			ID: "cast-receiver-session", UserID: userID, ProfileID: userID,
			PlaybackSessionID: "cast-receiver-playback", SourcePlaybackSessionID: "cast-source-session",
			Generation: 1, Status: "active",
		},
	}
	if err := server.commitCastReceiverHandoff(context.Background(), auth); err != nil {
		t.Fatalf("commit Cast receiver handoff: %v", err)
	}
	var sourceEndedAt, sourceState, sourcePointer string
	if err := db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = 'cast-source-session'`).Scan(&sourceEndedAt, &sourceState); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT source_playback_session_id FROM cast_receiver_sessions WHERE id = 'cast-receiver-session'`).Scan(&sourcePointer); err != nil {
		t.Fatal(err)
	}
	if sourceEndedAt == "" || sourceState != "stopped" || sourcePointer != "" {
		t.Fatalf("handoff authority split: endedAt=%q state=%q sourcePointer=%q", sourceEndedAt, sourceState, sourcePointer)
	}
}

func TestCastReceiverHandoffCASDoesNotRetireSourceForStaleGeneration(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	user := User{ID: userID, AccountID: userID, ProfileID: userID, ProfileIsPrimary: true, Role: "owner", Permissions: ownerPermissions()}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, sessionID := range []string{"cast-stale-source", "cast-current-playback"} {
		if _, err := db.Exec(`
			INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state)
			VALUES (?, ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, 'playing')`, sessionID, userID, userID, now, now); err != nil {
			t.Fatalf("insert %s: %v", sessionID, err)
		}
	}
	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO cast_receiver_sessions (
			id, token_hash, user_id, profile_id, receiver_id, receiver_origin, server_origin,
			playback_session_id, source_playback_session_id, client_instance_id, generation,
			capabilities_json, automation_json, status, expires_at, last_seen_at, created_at
		) VALUES ('cast-current-receiver', 'stale-token-hash', ?, ?, 'receiver', 'https://cast.getportico.tv',
			'https://server.example', 'cast-current-playback', 'cast-stale-source', 'installation-1',
			2, '[]', '{}', 'active', ?, ?, ?)`, userID, userID, expiresAt, now, now); err != nil {
		t.Fatalf("insert Cast receiver session: %v", err)
	}
	stale := castSessionAuth{
		user: user,
		record: castReceiverRecord{
			ID: "cast-current-receiver", UserID: userID, ProfileID: userID,
			PlaybackSessionID: "cast-current-playback", SourcePlaybackSessionID: "cast-stale-source",
			Generation: 1, Status: "active",
		},
	}
	if err := server.commitCastReceiverHandoff(context.Background(), stale); err == nil {
		t.Fatal("stale Cast generation committed receiver handoff")
	}
	var endedAt, sourcePointer string
	if err := db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = 'cast-stale-source'`).Scan(&endedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT source_playback_session_id FROM cast_receiver_sessions WHERE id = 'cast-current-receiver'`).Scan(&sourcePointer); err != nil {
		t.Fatal(err)
	}
	if endedAt != "" || sourcePointer != "cast-stale-source" {
		t.Fatalf("stale CAS changed handoff state: endedAt=%q sourcePointer=%q", endedAt, sourcePointer)
	}
}

func TestCastReceiverSupersessionPreservesReplacementSource(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	user := User{ID: userID, AccountID: userID, ProfileID: userID, ProfileIsPrimary: true, Role: "owner", Permissions: ownerPermissions()}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state)
		VALUES ('cast-protected-source', ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, 'playing')`, userID, userID, now, now); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO cast_receiver_sessions (
			id, token_hash, user_id, profile_id, receiver_id, receiver_origin, server_origin,
			playback_session_id, source_playback_session_id, client_instance_id, generation,
			capabilities_json, automation_json, status, expires_at, last_seen_at, created_at
		) VALUES ('cast-lost-receiver', 'lost-token-hash', ?, ?, 'receiver', 'https://cast.getportico.tv',
			'https://server.example', 'cast-protected-source', '', 'installation-1',
			4, '[]', '{}', 'active', ?, ?, ?)`, userID, userID, expiresAt, now, now); err != nil {
		t.Fatal(err)
	}
	if err := server.supersedeCastReceiverSession(context.Background(), user, "cast-lost-receiver", 4, "installation-1", "cast-protected-source"); err != nil {
		t.Fatalf("supersede lost Cast receiver: %v", err)
	}
	var receiverStatus, playbackEndedAt, playbackState string
	if err := db.QueryRow(`SELECT status FROM cast_receiver_sessions WHERE id = 'cast-lost-receiver'`).Scan(&receiverStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = 'cast-protected-source'`).Scan(&playbackEndedAt, &playbackState); err != nil {
		t.Fatal(err)
	}
	if receiverStatus != "stopped" || playbackEndedAt != "" || playbackState != "playing" {
		t.Fatalf("supersession damaged replacement source: receiver=%q endedAt=%q playback=%q", receiverStatus, playbackEndedAt, playbackState)
	}
}

func TestCastOriginsAndChallengesFailClosed(t *testing.T) {
	if canonicalCastServerOrigin("http://server.example") != "" || canonicalCastServerOrigin("https://server.example:8443") != "" {
		t.Fatal("Cast server origin accepted non-production origin")
	}
	if validCastChallenge("short") || !validCastChallenge("challenge-123456789") {
		t.Fatal("Cast challenge validation is not bounded")
	}
}

func TestCastPublicServerOriginPrefersConfiguredHTTPSOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32500/api/playback/cast/bootstrap", nil)
	server := &Server{cfg: config.Config{PublicOrigin: "https://media.example.test"}}
	if got := server.castPublicServerOrigin(request); got != "https://media.example.test" {
		t.Fatalf("configured Cast public origin = %q", got)
	}
	server.cfg.PublicOrigin = "http://media.example.test"
	if got := server.castPublicServerOrigin(request); got != "" {
		t.Fatalf("insecure configured Cast public origin = %q", got)
	}
	server.cfg.PublicOrigin = ""
	if got := server.castPublicServerOrigin(request); got != "" {
		t.Fatalf("Cast public origin fell back to sender request origin = %q", got)
	}
}

func TestCastPlaybackURLsUseConfiguredPublicOriginForLANSender(t *testing.T) {
	playback := PlaybackResponse{
		SourceURL:       "/api/media/movie-1/hls/master.m3u8",
		Media:           MediaItem{SourceURL: "/api/media/movie-1/hls/master.m3u8", Streams: []Stream{{Kind: "subtitle", SourceURL: "/api/media/movie-1/subtitles/en.vtt"}}},
		Resources:       []PlaybackResource{{SourceURL: "/api/media/movie-1/hls/master.m3u8"}},
		SubtitleStreams: []Stream{{Kind: "subtitle", SourceURL: "/api/media/movie-1/subtitles/en.vtt"}},
	}
	if err := absolutizeCastPlaybackURLs(&playback, "https://media.example.test"); err != nil {
		t.Fatalf("absolutize Cast playback URLs: %v", err)
	}
	for _, value := range []string{playback.SourceURL, playback.Media.SourceURL, playback.Media.Streams[0].SourceURL, playback.Resources[0].SourceURL, playback.SubtitleStreams[0].SourceURL} {
		if value != "https://media.example.test/api/media/movie-1/hls/master.m3u8" && value != "https://media.example.test/api/media/movie-1/subtitles/en.vtt" {
			t.Fatalf("Cast playback URL was not absolutized to public origin: %q", value)
		}
	}
	if _, err := absolutizeCastResourceURL("https://127.0.0.1:32500/api/media/movie-1/hls/master.m3u8", "https://media.example.test"); err == nil {
		t.Fatal("Cast playback accepted an off-origin LAN source")
	}
	if _, err := absolutizeCastResourceURL("/api/media/movie-1/hls/master.m3u8?media_grant=secret", "https://media.example.test"); err == nil {
		t.Fatal("Cast playback accepted a credential query")
	}
}

func TestCastQueueNextHonorsRepeatAndServerQueue(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  playbackSessionQueueSnapshot
		next      string
		remaining []string
		ok        bool
	}{
		{name: "queue", snapshot: playbackSessionQueueSnapshot{MediaID: "one", RepeatMode: "off", QueueIDs: []string{"two", "three"}}, next: "two", remaining: []string{"three"}, ok: true},
		{name: "repeat one", snapshot: playbackSessionQueueSnapshot{MediaID: "one", RepeatMode: "one", QueueIDs: []string{"two"}}, next: "one", remaining: []string{"two"}, ok: true},
		{name: "repeat all", snapshot: playbackSessionQueueSnapshot{MediaID: "two", RepeatMode: "all", SourceContext: PlaybackSourceContext{MediaIDs: []string{"one", "two", "three"}}}, next: "three", remaining: []string{"one", "two"}, ok: true},
		{name: "exhausted", snapshot: playbackSessionQueueSnapshot{MediaID: "one", RepeatMode: "off"}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, remaining, ok := castQueueNext(tt.snapshot)
			if next != tt.next || ok != tt.ok || !reflect.DeepEqual(remaining, tt.remaining) {
				t.Fatalf("castQueueNext() = (%q, %v, %v), want (%q, %v, %v)", next, remaining, ok, tt.next, tt.remaining, tt.ok)
			}
		})
	}
}

func TestCastAutomationProjectionClampsAndOmitsPolicy(t *testing.T) {
	if got := normalizeCastAutomation(CastPlaybackAutomation{UpNextCountdownSeconds: 7, PassoutAfterEpisodes: 0, IntroSkip: "unsafe", CreditsSkip: "automatic"}); got.UpNextCountdownSeconds != 10 || got.PassoutAfterEpisodes != 1 || got.IntroSkip != "ask" || got.CreditsSkip != "automatic" {
		t.Fatalf("unexpected normalized Cast automation: %#v", got)
	}
	encoded, err := json.Marshal(normalizeCastAutomation(CastPlaybackAutomation{AutoplayNext: true, UpNextCountdownSeconds: 5, PassoutProtection: true, PassoutAfterEpisodes: 3, IntroSkip: "automatic", CreditsSkip: "off"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "credentials") || strings.Contains(string(encoded), "rawPolicy") {
		t.Fatalf("Cast automation projection contains viewer policy or credentials: %s", encoded)
	}
}
