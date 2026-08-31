package app

import (
	"bytes"
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

func castReceiverKeyForTest(t *testing.T) *ecdh.PrivateKey {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func castBootstrapSecretForTest(t *testing.T, receiverKey *ecdh.PrivateKey, encoded string) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var envelope castBootstrapEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	serverPublicRaw, err := base64.RawURLEncoding.DecodeString(envelope.ServerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	var payload castBootstrapSecretPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Secret
}

func performCastBootstrapForTest(t *testing.T, server *Server, user User, request CastBootstrapRequest) (*httptest.ResponseRecorder, CastBootstrapResponse) {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/playback/cast/bootstrap", bytes.NewReader(encoded))
	recorder := httptest.NewRecorder()
	server.handleCastBootstrap(recorder, httpRequest, user)
	var response CastBootstrapResponse
	if recorder.Code == http.StatusCreated {
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	return recorder, response
}

func castReplacementBootstrapRequestForTest(t *testing.T, source PlaybackResponse, requestID string) CastBootstrapRequest {
	t.Helper()
	key := castReceiverKeyForTest(t)
	return CastBootstrapRequest{
		RequestID: requestID, ClientInstanceID: "terminal-authority-client",
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{Device: "Cast", Platform: "web", SupportsHLS: true}),
		SourceKind:    "media", SourceID: source.Media.ID,
		Replacement: &PlaybackReplacementRequest{
			SourceSessionID: source.SessionID, RequestID: requestID,
			PreviousTerminal: *playbackHandoffTerminalForTest(source, "stopped", 1),
		},
		ReceiverID: "receiver-" + requestID, ReceiverOrigin: "https://cast.getportico.tv",
		ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		ReceiverChallenge: "receiver-challenge-123456",
		Capabilities:      []string{"load", "progress", "renew", "stop", "advance"},
	}
}

func redeemCastBootstrapForTest(t *testing.T, server *Server, response CastBootstrapResponse, secret string) (*httptest.ResponseRecorder, CastReceiverSessionResponse) {
	t.Helper()
	encoded, _ := json.Marshal(CastRedeemRequest{
		BootstrapID: response.BootstrapID, BootstrapSecret: secret,
		ReceiverID: response.ReceiverID, ReceiverChallenge: "receiver-challenge-123456",
	})
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/playback/cast/redeem", bytes.NewReader(encoded))
	httpRequest.Header.Set("Origin", "https://cast.getportico.tv")
	recorder := httptest.NewRecorder()
	server.handleCastRedeem(recorder, httpRequest)
	var receiver CastReceiverSessionResponse
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &receiver); err != nil {
			t.Fatal(err)
		}
	}
	return recorder, receiver
}

func postCastReceiverActionForTest(t *testing.T, server *Server, receiver CastReceiverSessionResponse, action string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, _ := json.Marshal(body)
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/playback/cast/sessions/"+receiver.ReceiverSessionID+"/"+action, bytes.NewReader(encoded))
	httpRequest.Header.Set("Authorization", castReceiverAuthorization+receiver.ReceiverSessionToken)
	recorder := httptest.NewRecorder()
	server.handleCastSessionRoute(recorder, httpRequest)
	return recorder
}

func authenticateCastReceiverForTest(t *testing.T, server *Server, receiver CastReceiverSessionResponse) castSessionAuth {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/playback/cast/sessions/"+receiver.ReceiverSessionID+"/state", nil)
	request.Header.Set("Authorization", castReceiverAuthorization+receiver.ReceiverSessionToken)
	auth, err := server.authenticateCastSession(request, receiver.ReceiverSessionID)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func activeQueuedCastReceiverFixture(t *testing.T, requestID string) (*Server, User, CastReceiverSessionResponse) {
	t.Helper()
	server, user, _ := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	key := castReceiverKeyForTest(t)
	request := freshCastBootstrapRequestForTest(t, key, requestID)
	request.SourceID = "movie_terminal_source"
	request.QueueMediaIDs = []string{"movie_terminal_next"}
	bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
	if bootstrapRecorder.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	redeemRecorder, receiver := redeemCastBootstrapForTest(t, server, bootstrap, castBootstrapSecretForTest(t, key, bootstrap.BootstrapEnvelope))
	if redeemRecorder.Code != http.StatusOK {
		t.Fatalf("redeem status=%d body=%s", redeemRecorder.Code, redeemRecorder.Body.String())
	}
	position := 1.0
	progress := CastProgressRequest{Generation: receiver.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
		EventSequence: 1, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "playing", PositionSeconds: &position,
	}}
	if committed := postCastReceiverActionForTest(t, server, receiver, "progress", progress); committed.Code != http.StatusOK {
		t.Fatalf("first-playing status=%d body=%s", committed.Code, committed.Body.String())
	}
	return server, user, receiver
}

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

func TestCastRedeemLostResponseReplaysExactReceiverCredentials(t *testing.T) {
	server, user, _ := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	key := castReceiverKeyForTest(t)
	request := freshCastBootstrapRequestForTest(t, key, "cast-redeem-replay-bootstrap")
	bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
	if bootstrapRecorder.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	secret := castBootstrapSecretForTest(t, key, bootstrap.BootstrapEnvelope)
	firstRecorder, first := redeemCastBootstrapForTest(t, server, bootstrap, secret)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first redeem status=%d body=%s", firstRecorder.Code, firstRecorder.Body.String())
	}
	retryRecorder, retry := redeemCastBootstrapForTest(t, server, bootstrap, secret)
	if retryRecorder.Code != http.StatusOK || !bytes.Equal(retryRecorder.Body.Bytes(), firstRecorder.Body.Bytes()) || !reflect.DeepEqual(retry, first) {
		t.Fatalf("lost-response retry status=%d\nfirst=%#v\nretry=%#v\nbody=%s", retryRecorder.Code, first, retry, retryRecorder.Body.String())
	}
	if retry.ReceiverSessionToken == "" || retry.ReceiverSessionID == "" {
		t.Fatal("exact redeem replay omitted receiver credentials")
	}
	authenticateCastReceiverForTest(t, server, retry)
	var storedTokenHash, storedEnvelope string
	if err := server.db.QueryRow(`SELECT token_hash, redeem_response_envelope FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&storedTokenHash, &storedEnvelope); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedEnvelope, retry.ReceiverSessionToken) || storedTokenHash == retry.ReceiverSessionToken || storedEnvelope == "" {
		t.Fatal("redeem outcome was not stored as an opaque encrypted envelope")
	}

	for name, redeem := range map[string]CastRedeemRequest{
		"receiver challenge": {BootstrapID: bootstrap.BootstrapID, BootstrapSecret: secret, ReceiverID: bootstrap.ReceiverID, ReceiverChallenge: "modified-challenge-123456"},
		"receiver id":        {BootstrapID: bootstrap.BootstrapID, BootstrapSecret: secret, ReceiverID: "modified-receiver", ReceiverChallenge: request.ReceiverChallenge},
		"bootstrap proof":    {BootstrapID: bootstrap.BootstrapID, BootstrapSecret: "ptc_cb_modified-proof", ReceiverID: bootstrap.ReceiverID, ReceiverChallenge: request.ReceiverChallenge},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, _ := json.Marshal(redeem)
			httpRequest := httptest.NewRequest(http.MethodPost, "/api/playback/cast/redeem", bytes.NewReader(encoded))
			httpRequest.Header.Set("Origin", "https://cast.getportico.tv")
			recorder := httptest.NewRecorder()
			server.handleCastRedeem(recorder, httpRequest)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("modified binding status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
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

func TestCastReplacementClassifiesInactiveSourceBeforeReservation(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	if _, err := server.db.Exec(`UPDATE playback_sessions SET state = 'stopped', ended_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), source.SessionID); err != nil {
		t.Fatal(err)
	}
	request := castReplacementBootstrapRequestForTest(t, source, "cast-inactive-precheck")
	recorder, _ := performCastBootstrapForTest(t, server, user, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "replacement_source_inactive") || strings.Contains(recorder.Body.String(), "cast_source_session_invalid") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertCastReplacementHasNoReservedResidue(t, server, source.SessionID, request.RequestID)
}

func TestCastReplacementClassifiesSourceInactiveAfterReservationWithoutResidue(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		CREATE TEMP TRIGGER stop_cast_source_after_target_reservation
		AFTER INSERT ON playback_sessions
		WHEN NEW.state = 'handoff_pending' AND NEW.id <> '` + source.SessionID + `'
		BEGIN
			UPDATE playback_sessions SET state = 'stopped', ended_at = '` + endedAt + `'
			WHERE id = '` + source.SessionID + `';
		END`); err != nil {
		t.Fatal(err)
	}
	request := castReplacementBootstrapRequestForTest(t, source, "cast-inactive-after-reservation")
	recorder, _ := performCastBootstrapForTest(t, server, user, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "replacement_source_inactive") || strings.Contains(recorder.Body.String(), "replacement_required") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertCastReplacementHasNoReservedResidue(t, server, source.SessionID, request.RequestID)
}

func TestCastReplacementPreservesDifferentActiveAuthorityConflict(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		CREATE TEMP TRIGGER drift_cast_authority_after_target_reservation
		AFTER INSERT ON playback_sessions
		WHEN NEW.state = 'handoff_pending' AND NEW.id <> '` + source.SessionID + `'
		BEGIN
			UPDATE playback_sessions SET state = 'stopped', ended_at = '` + endedAt + `'
			WHERE id = '` + source.SessionID + `';
			UPDATE playback_sessions SET state = 'playing' WHERE id = NEW.id;
		END`); err != nil {
		t.Fatal(err)
	}
	request := castReplacementBootstrapRequestForTest(t, source, "cast-active-authority-drift")
	recorder, _ := performCastBootstrapForTest(t, server, user, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "replacement_required") || strings.Contains(recorder.Body.String(), "replacement_source_inactive") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertCastReplacementHasNoReservedResidue(t, server, source.SessionID, request.RequestID)
}

func TestCastTransferStatusClassifiesInactiveReplacementSource(t *testing.T) {
	for _, terminalStatus := range []string{"pending", "expired", "failed"} {
		t.Run(terminalStatus, func(t *testing.T) {
			server, user, source := playbackAuthorityFixture(t)
			server.cfg.PublicOrigin = "https://media.example.test"
			server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
			request := castReplacementBootstrapRequestForTest(t, source, "cast-status-inactive-"+terminalStatus)
			bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
			if bootstrapRecorder.Code != http.StatusCreated {
				t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
			}
			if terminalStatus != "pending" {
				var fingerprint, claimID string
				if err := server.db.QueryRow(`SELECT replacement_fingerprint, replacement_claim_id FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&fingerprint, &claimID); err != nil {
					t.Fatal(err)
				}
				if err := server.expireCastBootstrapTransfer(t.Context(), user, bootstrap.ReplacementSessionID, source.SessionID, request.RequestID, fingerprint, claimID); err != nil {
					t.Fatal(err)
				}
				if terminalStatus == "failed" {
					if _, err := server.db.Exec(`UPDATE cast_bootstraps SET transfer_state = 'failed' WHERE id = ?`, bootstrap.BootstrapID); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := server.db.Exec(`UPDATE playback_sessions SET state = 'stopped', ended_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), source.SessionID); err != nil {
				t.Fatal(err)
			}
			statusBody, _ := json.Marshal(CastTransferStatusRequest{
				ClientInstanceID: request.ClientInstanceID, SourceSessionID: source.SessionID, RequestID: request.RequestID,
			})
			statusRequest := httptest.NewRequest(http.MethodPost, "/api/playback/cast/transfer-status", bytes.NewReader(statusBody))
			statusRecorder := httptest.NewRecorder()
			server.handleCastTransferStatus(statusRecorder, statusRequest, user)
			if statusRecorder.Code != http.StatusConflict || !strings.Contains(statusRecorder.Body.String(), "replacement_source_inactive") {
				t.Fatalf("status=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
			}
			var successors, receipts int
			_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&successors)
			_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&receipts)
			if successors != 0 || receipts != 0 {
				t.Fatalf("inactive status retained successors=%d receipts=%d", successors, receipts)
			}
		})
	}
}

func TestCastExactBootstrapRetryClassifiesInactiveReplacementSource(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	request := castReplacementBootstrapRequestForTest(t, source, "cast-retry-inactive-source")
	bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
	if bootstrapRecorder.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	if _, err := server.db.Exec(`UPDATE playback_sessions SET state = 'stopped', ended_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), source.SessionID); err != nil {
		t.Fatal(err)
	}
	retryRecorder, _ := performCastBootstrapForTest(t, server, user, request)
	if retryRecorder.Code != http.StatusConflict || !strings.Contains(retryRecorder.Body.String(), "replacement_source_inactive") || strings.Contains(retryRecorder.Body.String(), "cast_bootstrap_retry_conflict") {
		t.Fatalf("retry status=%d body=%s", retryRecorder.Code, retryRecorder.Body.String())
	}
	var successors, receipts int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&successors)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&receipts)
	if successors != 0 || receipts != 0 {
		t.Fatalf("inactive retry retained successors=%d receipts=%d", successors, receipts)
	}
}

func assertCastReplacementHasNoReservedResidue(t *testing.T, server *Server, sourceSessionID, requestID string) {
	t.Helper()
	var successors, receipts, bootstraps int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id <> ? AND client_instance_id = 'terminal-authority-client'`, sourceSessionID).Scan(&successors)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, sourceSessionID).Scan(&receipts)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM cast_bootstraps WHERE replacement_request_id = ?`, requestID).Scan(&bootstraps)
	if successors != 0 || receipts != 0 || bootstraps != 0 {
		t.Fatalf("Cast replacement residue successors=%d receipts=%d bootstraps=%d", successors, receipts, bootstraps)
	}
}

func TestCastFirstPlayingCommitsSourceReceiptTargetAndReceiverAtomically(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	server.cfg.PublicOrigin = "https://media.example.test"
	server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
	receiverKey := castReceiverKeyForTest(t)
	request := CastBootstrapRequest{
		RequestID: "cast-atomic-transfer-request", ClientInstanceID: "terminal-authority-client",
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{Device: "Cast", Platform: "web", SupportsHLS: true}),
		SourceKind:    "media", SourceID: source.Media.ID,
		Replacement: &PlaybackReplacementRequest{
			SourceSessionID: source.SessionID, RequestID: "cast-atomic-transfer-request",
			PreviousTerminal: *playbackHandoffTerminalForTest(source, "stopped", 1),
		},
		ReceiverID: "receiver-atomic", ReceiverOrigin: "https://cast.getportico.tv",
		ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(receiverKey.PublicKey().Bytes()),
		ReceiverChallenge: "receiver-challenge-123456",
		Capabilities:      []string{"load", "progress", "renew", "stop", "advance"},
	}

	bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
	if bootstrapRecorder.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	retryRecorder, retried := performCastBootstrapForTest(t, server, user, request)
	if retryRecorder.Code != http.StatusCreated || retried.BootstrapID != bootstrap.BootstrapID || retried.ReplacementSessionID != bootstrap.ReplacementSessionID {
		t.Fatalf("exact bootstrap retry status=%d first=%#v retry=%#v body=%s", retryRecorder.Code, bootstrap, retried, retryRecorder.Body.String())
	}
	var pendingTargets, bootstrapRows int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND state = 'handoff_pending' AND ended_at = ''`, bootstrap.ReplacementSessionID).Scan(&pendingTargets)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM cast_bootstraps WHERE replacement_request_id = ? AND transfer_state = 'pending'`, request.RequestID).Scan(&bootstrapRows)
	if pendingTargets != 1 || bootstrapRows != 1 {
		t.Fatalf("exact bootstrap did not reuse one pending target: targets=%d bootstraps=%d", pendingTargets, bootstrapRows)
	}

	secret := castBootstrapSecretForTest(t, receiverKey, bootstrap.BootstrapEnvelope)
	redeemRecorder, receiver := redeemCastBootstrapForTest(t, server, bootstrap, secret)
	if redeemRecorder.Code != http.StatusOK {
		t.Fatalf("redeem status=%d body=%s", redeemRecorder.Code, redeemRecorder.Body.String())
	}
	if receiver.Playback.ContinuationCredential != nil {
		t.Fatal("pending Cast receiver received a parallel continuation credential")
	}
	if _, err := server.castReceiverRecordForUser(t.Context(), user, receiver.ReceiverSessionID, ""); err == nil {
		t.Fatal("account reconnect published a pending receiver as active")
	}

	// Break the bootstrap half of the final CAS. The entire common terminal,
	// receipt, target, receiver, and bootstrap transaction must roll back.
	if _, err := server.db.Exec(`UPDATE cast_bootstraps SET replacement_claim_id = 'wrong-claim' WHERE id = ?`, bootstrap.BootstrapID); err != nil {
		t.Fatal(err)
	}
	position := 1.0
	playing := CastProgressRequest{Generation: receiver.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
		EventSequence: 1, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "playing", PositionSeconds: &position,
	}}
	failed := postCastReceiverActionForTest(t, server, receiver, "progress", playing)
	if failed.Code != http.StatusConflict || !strings.Contains(failed.Body.String(), "cast_handoff_commit_pending") {
		t.Fatalf("broken final CAS status=%d body=%s", failed.Code, failed.Body.String())
	}
	var sourceEnded, sourceState, targetState, receiptState, receiverState, bootstrapState string
	_ = server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&sourceEnded, &sourceState)
	_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&targetState)
	_ = server.db.QueryRow(`SELECT state FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&receiptState)
	_ = server.db.QueryRow(`SELECT status || ':' || transfer_state FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverState)
	_ = server.db.QueryRow(`SELECT transfer_state FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&bootstrapState)
	if sourceEnded != "" || sourceState != "playing" || targetState != "handoff_pending" || receiptState != "committing" || receiverState != "pending:pending" || bootstrapState != "pending" {
		t.Fatalf("failed final CAS leaked partial authority: source=%q/%q target=%q receipt=%q receiver=%q bootstrap=%q", sourceEnded, sourceState, targetState, receiptState, receiverState, bootstrapState)
	}

	if _, err := server.db.Exec(`UPDATE cast_bootstraps SET replacement_claim_id = ? WHERE id = ?`, func() string {
		var claim string
		_ = server.db.QueryRow(`SELECT replacement_claim_id FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&claim)
		return claim
	}(), bootstrap.BootstrapID); err != nil {
		t.Fatal(err)
	}
	committed := postCastReceiverActionForTest(t, server, receiver, "progress", playing)
	if committed.Code != http.StatusOK {
		t.Fatalf("first-playing retry status=%d body=%s", committed.Code, committed.Body.String())
	}
	_ = server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&sourceEnded, &sourceState)
	_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&targetState)
	_ = server.db.QueryRow(`SELECT state FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&receiptState)
	_ = server.db.QueryRow(`SELECT status || ':' || transfer_state FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverState)
	_ = server.db.QueryRow(`SELECT transfer_state FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&bootstrapState)
	if sourceEnded == "" || sourceState != "stopped" || targetState != "playing" || receiptState != "committed" || receiverState != "active:committed" || bootstrapState != "committed" {
		t.Fatalf("first-playing did not publish one atomic outcome: source=%q/%q target=%q receipt=%q receiver=%q bootstrap=%q", sourceEnded, sourceState, targetState, receiptState, receiverState, bootstrapState)
	}
}

func freshCastBootstrapRequestForTest(t *testing.T, receiverKey *ecdh.PrivateKey, requestID string) CastBootstrapRequest {
	t.Helper()
	return CastBootstrapRequest{
		RequestID: requestID, ClientInstanceID: "fresh-cast-client-" + requestID,
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{Device: "Cast", Platform: "web", SupportsHLS: true}),
		SourceKind:    "media", SourceID: "movie_terminal_next",
		ReceiverID: "receiver-" + requestID, ReceiverOrigin: "https://cast.getportico.tv",
		ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(receiverKey.PublicKey().Bytes()),
		ReceiverChallenge: "receiver-challenge-123456",
		Capabilities:      []string{"load", "progress", "renew", "stop", "advance"},
	}
}

func TestCastFreshPendingAuthorizationAndDurableOutcomeTombstones(t *testing.T) {
	t.Run("authorization revision invalidates pending receiver", func(t *testing.T) {
		server, user, _ := playbackAuthorityFixture(t)
		server.cfg.PublicOrigin = "https://media.example.test"
		server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
		key := castReceiverKeyForTest(t)
		request := freshCastBootstrapRequestForTest(t, key, "fresh-auth-revision-request")
		bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
		if bootstrapRecorder.Code != http.StatusCreated {
			t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
		}
		redeemRecorder, receiver := redeemCastBootstrapForTest(t, server, bootstrap, castBootstrapSecretForTest(t, key, bootstrap.BootstrapEnvelope))
		if redeemRecorder.Code != http.StatusOK {
			t.Fatalf("redeem status=%d body=%s", redeemRecorder.Code, redeemRecorder.Body.String())
		}
		if _, err := server.db.Exec(`UPDATE profiles SET policy_updated_at = ? WHERE id = ?`, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano), viewerProfileID(user)); err != nil {
			t.Fatal(err)
		}
		position := 1.0
		progress := CastProgressRequest{Generation: receiver.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
			EventSequence: 1, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "playing", PositionSeconds: &position,
		}}
		denied := postCastReceiverActionForTest(t, server, receiver, "progress", progress)
		if denied.Code != http.StatusUnauthorized || !strings.Contains(denied.Body.String(), "cast_session_invalid") {
			t.Fatalf("changed authorization status=%d body=%s", denied.Code, denied.Body.String())
		}
		var targetState, receiverState, bootstrapState string
		_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&targetState)
		_ = server.db.QueryRow(`SELECT status || ':' || transfer_state FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverState)
		_ = server.db.QueryRow(`SELECT transfer_state FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&bootstrapState)
		if targetState != "handoff_pending" || receiverState != "pending:pending" || bootstrapState != "pending" {
			t.Fatalf("changed authorization published authority: target=%q receiver=%q bootstrap=%q", targetState, receiverState, bootstrapState)
		}
	})

	t.Run("pending expiry is definitive", func(t *testing.T) {
		server, user, _ := playbackAuthorityFixture(t)
		server.cfg.PublicOrigin = "https://media.example.test"
		server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
		key := castReceiverKeyForTest(t)
		request := freshCastBootstrapRequestForTest(t, key, "fresh-expiry-tombstone-request")
		bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
		if bootstrapRecorder.Code != http.StatusCreated {
			t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
		}
		if _, err := server.db.Exec(`UPDATE cast_bootstraps SET expires_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), bootstrap.BootstrapID); err != nil {
			t.Fatal(err)
		}
		if err := server.pruneCastPlaybackTransfers(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		var state, responseJSON, envelope string
		if err := server.db.QueryRow(`SELECT transfer_state, bootstrap_response_json, playback_envelope FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&state, &responseJSON, &envelope); err != nil {
			t.Fatal(err)
		}
		var targetRows int
		_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&targetRows)
		if state != "expired" || responseJSON != "" || envelope != "" || targetRows != 0 {
			t.Fatalf("expired tombstone state=%q response=%q envelope=%q targetRows=%d", state, responseJSON, envelope, targetRows)
		}
		retry, _ := performCastBootstrapForTest(t, server, user, request)
		if retry.Code != http.StatusConflict || !strings.Contains(retry.Body.String(), "cast_transfer_expired") || strings.Contains(retry.Body.String(), "bootstrapEnvelope") {
			t.Fatalf("expired exact retry status=%d body=%s", retry.Code, retry.Body.String())
		}
	})

	t.Run("replacement expiry retains source and releases claim", func(t *testing.T) {
		server, user, source := playbackAuthorityFixture(t)
		server.cfg.PublicOrigin = "https://media.example.test"
		server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
		key := castReceiverKeyForTest(t)
		request := CastBootstrapRequest{
			RequestID: "replacement-expiry-tombstone-request", ClientInstanceID: "terminal-authority-client",
			ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{Device: "Cast", Platform: "web", SupportsHLS: true}),
			SourceKind:    "media", SourceID: source.Media.ID,
			Replacement: &PlaybackReplacementRequest{
				SourceSessionID: source.SessionID, RequestID: "replacement-expiry-tombstone-request",
				PreviousTerminal: *playbackHandoffTerminalForTest(source, "stopped", 1),
			},
			ReceiverID: "receiver-replacement-expiry", ReceiverOrigin: "https://cast.getportico.tv",
			ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			ReceiverChallenge: "receiver-challenge-123456",
			Capabilities:      []string{"load", "progress", "renew", "stop"},
		}
		bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
		if bootstrapRecorder.Code != http.StatusCreated {
			t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
		}
		if _, err := server.db.Exec(`UPDATE cast_bootstraps SET expires_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), bootstrap.BootstrapID); err != nil {
			t.Fatal(err)
		}
		if err := server.pruneCastPlaybackTransfers(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		var sourceEnded, sourceState, bootstrapState string
		var targetRows, claimRows int
		_ = server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&sourceEnded, &sourceState)
		_ = server.db.QueryRow(`SELECT transfer_state FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&bootstrapState)
		_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID).Scan(&targetRows)
		_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&claimRows)
		if sourceEnded != "" || sourceState != "playing" || bootstrapState != "expired" || targetRows != 0 || claimRows != 0 {
			t.Fatalf("replacement expiry source=%q/%q bootstrap=%q targets=%d claims=%d", sourceEnded, sourceState, bootstrapState, targetRows, claimRows)
		}
	})

	t.Run("committed status survives playback deletion", func(t *testing.T) {
		server, user, _ := playbackAuthorityFixture(t)
		server.cfg.PublicOrigin = "https://media.example.test"
		server.cfg.CastReceiverOrigins = []string{"https://cast.getportico.tv"}
		key := castReceiverKeyForTest(t)
		request := freshCastBootstrapRequestForTest(t, key, "fresh-committed-tombstone-request")
		bootstrapRecorder, bootstrap := performCastBootstrapForTest(t, server, user, request)
		if bootstrapRecorder.Code != http.StatusCreated {
			t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
		}
		redeemRecorder, receiver := redeemCastBootstrapForTest(t, server, bootstrap, castBootstrapSecretForTest(t, key, bootstrap.BootstrapEnvelope))
		if redeemRecorder.Code != http.StatusOK {
			t.Fatalf("redeem status=%d body=%s", redeemRecorder.Code, redeemRecorder.Body.String())
		}
		position := 1.0
		progress := CastProgressRequest{Generation: receiver.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
			EventSequence: 1, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "playing", PositionSeconds: &position,
		}}
		if committed := postCastReceiverActionForTest(t, server, receiver, "progress", progress); committed.Code != http.StatusOK {
			t.Fatalf("first-playing status=%d body=%s", committed.Code, committed.Body.String())
		}
		if _, err := server.db.Exec(`UPDATE cast_bootstraps SET payload_expires_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), bootstrap.BootstrapID); err != nil {
			t.Fatal(err)
		}
		if err := server.prunePlaybackReplacementPayloads(t.Context(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		var responseJSON, playbackEnvelope, receiverPublicKey string
		if err := server.db.QueryRow(`SELECT bootstrap_response_json, playback_envelope, receiver_public_key FROM cast_bootstraps WHERE id = ?`, bootstrap.BootstrapID).Scan(&responseJSON, &playbackEnvelope, &receiverPublicKey); err != nil {
			t.Fatal(err)
		}
		if responseJSON != "" || playbackEnvelope != "" || receiverPublicKey != "" {
			t.Fatalf("expired credential payload survived purge: response=%q envelope=%q receiverKey=%q", responseJSON, playbackEnvelope, receiverPublicKey)
		}
		if _, err := server.db.Exec(`DELETE FROM playback_sessions WHERE id = ?`, bootstrap.ReplacementSessionID); err != nil {
			t.Fatal(err)
		}
		statusBody, _ := json.Marshal(CastTransferStatusRequest{ClientInstanceID: request.ClientInstanceID, RequestID: request.RequestID})
		statusRequest := httptest.NewRequest(http.MethodPost, "/api/playback/cast/transfer-status", bytes.NewReader(statusBody))
		statusRecorder := httptest.NewRecorder()
		server.handleCastTransferStatus(statusRecorder, statusRequest, user)
		if statusRecorder.Code != http.StatusOK {
			t.Fatalf("status after playback deletion=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
		}
		var status CastTransferStatusResponse
		_ = json.Unmarshal(statusRecorder.Body.Bytes(), &status)
		if status.Status != "committed" || status.ReplacementSessionID != bootstrap.ReplacementSessionID || status.RequestFingerprint == "" || status.SourceSessionID != "" {
			t.Fatalf("committed tombstone lost exact evidence: %#v", status)
		}
	})
}

func TestCastAdvanceRetainsOldActorThroughLostResponseCancelAndFirstPlaying(t *testing.T) {
	t.Run("exhausted terminal outcome is exact after lost response", func(t *testing.T) {
		server, _, receiver := activeQueuedCastReceiverFixture(t, "cast-advance-exhausted-bootstrap")
		auth := authenticateCastReceiverForTest(t, server, receiver)
		if _, err := server.db.Exec(`DELETE FROM playback_session_queue WHERE session_id = ?`, auth.record.PlaybackSessionID); err != nil {
			t.Fatal(err)
		}
		advance := CastAdvanceRequest{
			Generation: receiver.Generation, AdvanceID: "advance-exhausted-1",
			RequestID:        "cast-advance-exhausted-request",
			PreviousTerminal: *playbackHandoffTerminalForTest(receiver.Playback, "stopped", 2),
		}
		first := postCastReceiverActionForTest(t, server, receiver, "advance", advance)
		var outcome CastAdvanceResponse
		_ = json.Unmarshal(first.Body.Bytes(), &outcome)
		if first.Code != http.StatusOK || outcome.Status != "exhausted" || outcome.RequestFingerprint == "" || outcome.PreviousTerminal != advance.PreviousTerminal {
			t.Fatalf("exhausted status=%d outcome=%#v body=%s", first.Code, outcome, first.Body.String())
		}
		retry := postCastReceiverActionForTest(t, server, receiver, "advance", advance)
		var retried CastAdvanceResponse
		_ = json.Unmarshal(retry.Body.Bytes(), &retried)
		if retry.Code != http.StatusOK || retried.Status != "exhausted" || retried.RequestFingerprint != outcome.RequestFingerprint {
			t.Fatalf("exhausted lost-response retry status=%d outcome=%#v body=%s", retry.Code, retried, retry.Body.String())
		}
		var receiverStatus, endedAt string
		var receiptRows int
		_ = server.db.QueryRow(`SELECT status FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverStatus)
		_ = server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = ?`, auth.record.PlaybackSessionID).Scan(&endedAt)
		_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_terminal_receipts WHERE playback_session_id = ?`, auth.record.PlaybackSessionID).Scan(&receiptRows)
		if receiverStatus != "stopped" || endedAt == "" || receiptRows != 1 {
			t.Fatalf("exhausted terminal transaction receiver=%q ended=%q receipts=%d", receiverStatus, endedAt, receiptRows)
		}
	})

	t.Run("lost response and cancel retain old actor", func(t *testing.T) {
		server, _, receiver := activeQueuedCastReceiverFixture(t, "cast-advance-cancel-bootstrap")
		auth := authenticateCastReceiverForTest(t, server, receiver)
		terminal := *playbackHandoffTerminalForTest(receiver.Playback, "stopped", 2)
		advance := CastAdvanceRequest{
			Generation: receiver.Generation, AdvanceID: "advance-lost-response-1",
			RequestID: "cast-advance-lost-response-request", PreviousTerminal: terminal,
		}
		first := postCastReceiverActionForTest(t, server, receiver, "advance", advance)
		if first.Code != http.StatusOK {
			t.Fatalf("advance status=%d body=%s", first.Code, first.Body.String())
		}
		var prepared CastAdvanceResponse
		_ = json.Unmarshal(first.Body.Bytes(), &prepared)
		if prepared.Status != "prepared" || prepared.Playback == nil || prepared.Playback.ContinuationCredential != nil {
			t.Fatalf("unexpected prepared advance: %#v", prepared)
		}
		retry := postCastReceiverActionForTest(t, server, receiver, "advance", advance)
		var retried CastAdvanceResponse
		_ = json.Unmarshal(retry.Body.Bytes(), &retried)
		if retry.Code != http.StatusOK || retried.ReplacementSessionID != prepared.ReplacementSessionID || retried.Generation != prepared.Generation {
			t.Fatalf("lost-response retry status=%d first=%#v retry=%#v body=%s", retry.Code, prepared, retried, retry.Body.String())
		}
		for name, mutate := range map[string]func(*CastAdvanceRequest){
			"terminal":   func(candidate *CastAdvanceRequest) { candidate.PreviousTerminal.EventSequence++ },
			"automatic":  func(candidate *CastAdvanceRequest) { candidate.Automatic = !candidate.Automatic },
			"generation": func(candidate *CastAdvanceRequest) { candidate.Generation++ },
		} {
			t.Run("modified "+name+" conflicts", func(t *testing.T) {
				candidate := advance
				mutate(&candidate)
				conflict := postCastReceiverActionForTest(t, server, receiver, "advance", candidate)
				if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "cast_advance_retry_conflict") {
					t.Fatalf("modified retry status=%d body=%s", conflict.Code, conflict.Body.String())
				}
			})
		}
		var activePlayback, pendingPlayback, receiverPointer string
		var receiverGeneration int64
		_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, auth.record.PlaybackSessionID).Scan(&activePlayback)
		_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, prepared.ReplacementSessionID).Scan(&pendingPlayback)
		_ = server.db.QueryRow(`SELECT playback_session_id, generation FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverPointer, &receiverGeneration)
		if activePlayback != "playing" || pendingPlayback != "handoff_pending" || receiverPointer != auth.record.PlaybackSessionID || receiverGeneration != receiver.Generation {
			t.Fatalf("advance preparation replaced old actor: source=%q target=%q pointer=%q generation=%d", activePlayback, pendingPlayback, receiverPointer, receiverGeneration)
		}
		cancel := postCastReceiverActionForTest(t, server, receiver, "advance-cancel", CastAdvanceCancelRequest{Generation: receiver.Generation, RequestID: advance.RequestID})
		if cancel.Code != http.StatusOK {
			t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
		}
		var targetRows int
		_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, prepared.ReplacementSessionID).Scan(&targetRows)
		position := 2.0
		oldActorProgress := CastProgressRequest{Generation: receiver.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
			EventSequence: 2, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "playing", PositionSeconds: &position,
		}}
		resumed := postCastReceiverActionForTest(t, server, receiver, "progress", oldActorProgress)
		if resumed.Code != http.StatusOK || targetRows != 0 {
			t.Fatalf("cancel did not restore old actor: progress=%d body=%s targetRows=%d", resumed.Code, resumed.Body.String(), targetRows)
		}
		definitiveRetry := postCastReceiverActionForTest(t, server, receiver, "advance", advance)
		var failedOutcome CastAdvanceResponse
		_ = json.Unmarshal(definitiveRetry.Body.Bytes(), &failedOutcome)
		if definitiveRetry.Code != http.StatusOK || failedOutcome.Status != "failed" || failedOutcome.ReplacementSessionID != prepared.ReplacementSessionID {
			t.Fatalf("cancel tombstone was not exact: status=%d response=%#v body=%s", definitiveRetry.Code, failedOutcome, definitiveRetry.Body.String())
		}
	})

	t.Run("successor first-playing atomically advances authority", func(t *testing.T) {
		server, _, receiver := activeQueuedCastReceiverFixture(t, "cast-advance-commit-bootstrap")
		auth := authenticateCastReceiverForTest(t, server, receiver)
		advance := CastAdvanceRequest{
			Generation: receiver.Generation, AdvanceID: "advance-commit-1",
			RequestID:        "cast-advance-commit-request",
			PreviousTerminal: *playbackHandoffTerminalForTest(receiver.Playback, "stopped", 2),
		}
		preparedRecorder := postCastReceiverActionForTest(t, server, receiver, "advance", advance)
		if preparedRecorder.Code != http.StatusOK {
			t.Fatalf("prepare status=%d body=%s", preparedRecorder.Code, preparedRecorder.Body.String())
		}
		var prepared CastAdvanceResponse
		_ = json.Unmarshal(preparedRecorder.Body.Bytes(), &prepared)
		position := 1.0
		firstPlaying := CastProgressRequest{Generation: prepared.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
			EventSequence: 1, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "playing", PositionSeconds: &position,
		}}
		committed := postCastReceiverActionForTest(t, server, receiver, "progress", firstPlaying)
		if committed.Code != http.StatusOK {
			t.Fatalf("successor first-playing status=%d body=%s", committed.Code, committed.Body.String())
		}
		var sourceEnded, sourceState, targetState, receiptState, receiverPointer, pendingPointer string
		var generation int64
		_ = server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, auth.record.PlaybackSessionID).Scan(&sourceEnded, &sourceState)
		_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, prepared.ReplacementSessionID).Scan(&targetState)
		_ = server.db.QueryRow(`SELECT state FROM playback_handoff_receipts WHERE source_session_id = ? AND request_id = ?`, auth.record.PlaybackSessionID, advance.RequestID).Scan(&receiptState)
		_ = server.db.QueryRow(`SELECT playback_session_id, generation, pending_playback_session_id FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverPointer, &generation, &pendingPointer)
		if sourceEnded == "" || sourceState != "stopped" || targetState != "playing" || receiptState != "committed" || receiverPointer != prepared.ReplacementSessionID || generation != prepared.Generation || pendingPointer != "" {
			t.Fatalf("advance commit source=%q/%q target=%q receipt=%q receiver=%q/%d pending=%q", sourceEnded, sourceState, targetState, receiptState, receiverPointer, generation, pendingPointer)
		}
	})
}

func TestCastStopReceiptAndReceiverStateShareOneTransaction(t *testing.T) {
	server, _, receiver := activeQueuedCastReceiverFixture(t, "cast-stop-atomic-bootstrap")
	auth := authenticateCastReceiverForTest(t, server, receiver)
	request := PlaybackSessionStopRequest{
		RequestID: "cast-stop-atomic-request",
		Terminal:  *playbackHandoffTerminalForTest(receiver.Playback, "stopped", 2),
	}
	stale := auth
	stale.record.Generation++
	if _, err := server.stopCastReceiverSession(t.Context(), stale, request); err == nil {
		t.Fatal("stale receiver generation unexpectedly committed stop")
	}
	var endedAt, receiverStatus string
	var receiptRows int
	_ = server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = ?`, auth.record.PlaybackSessionID).Scan(&endedAt)
	_ = server.db.QueryRow(`SELECT status FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverStatus)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_terminal_receipts WHERE playback_session_id = ?`, auth.record.PlaybackSessionID).Scan(&receiptRows)
	if endedAt != "" || receiverStatus != "active" || receiptRows != 0 {
		t.Fatalf("failed receiver CAS leaked terminal state: ended=%q receiver=%q receipts=%d", endedAt, receiverStatus, receiptRows)
	}
	position := float64(receiver.Playback.Timeline.DurationSeconds)
	completedProgress := CastProgressRequest{Generation: receiver.Generation, PlaybackProgressEvent: PlaybackProgressEvent{
		EventSequence: 2, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), State: "completed", PositionSeconds: &position,
	}}
	if rejected := postCastReceiverActionForTest(t, server, receiver, "progress", completedProgress); rejected.Code != http.StatusConflict || !strings.Contains(rejected.Body.String(), "cast_terminal_required") {
		t.Fatalf("completed progress bypass status=%d body=%s", rejected.Code, rejected.Body.String())
	}
	first := postCastReceiverActionForTest(t, server, receiver, "stop", CastStopRequest{Generation: receiver.Generation, RequestID: request.RequestID, Terminal: request.Terminal})
	if first.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", first.Code, first.Body.String())
	}
	_ = server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = ?`, auth.record.PlaybackSessionID).Scan(&endedAt)
	_ = server.db.QueryRow(`SELECT status FROM cast_receiver_sessions WHERE id = ?`, receiver.ReceiverSessionID).Scan(&receiverStatus)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_terminal_receipts WHERE playback_session_id = ?`, auth.record.PlaybackSessionID).Scan(&receiptRows)
	if endedAt == "" || receiverStatus != "stopped" || receiptRows != 1 {
		t.Fatalf("stop transaction ended=%q receiver=%q receipts=%d", endedAt, receiverStatus, receiptRows)
	}
	retry := postCastReceiverActionForTest(t, server, receiver, "stop", CastStopRequest{Generation: receiver.Generation, RequestID: request.RequestID, Terminal: request.Terminal})
	if retry.Code != http.StatusOK || !strings.Contains(retry.Body.String(), `"duplicate":true`) {
		t.Fatalf("exact stop retry status=%d body=%s", retry.Code, retry.Body.String())
	}
}

func TestCastReceiverHandoffRetiresSourceAndClearsPointerAtomically(t *testing.T) {
	_, db, _ := newAuthTestServerWithInstance(t)
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
	_ = auth
	var sourceEndedAt, sourceState, sourcePointer string
	if err := db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = 'cast-source-session'`).Scan(&sourceEndedAt, &sourceState); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT source_playback_session_id FROM cast_receiver_sessions WHERE id = 'cast-receiver-session'`).Scan(&sourcePointer); err != nil {
		t.Fatal(err)
	}
	if sourceEndedAt != "" || sourceState != "playing" || sourcePointer != "cast-source-session" {
		t.Fatalf("uncommitted receiver changed source authority: endedAt=%q state=%q sourcePointer=%q", sourceEndedAt, sourceState, sourcePointer)
	}
}

func TestCastReceiverHandoffCASDoesNotRetireSourceForStaleGeneration(t *testing.T) {
	_, db, _ := newAuthTestServerWithInstance(t)
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
	_ = stale
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
	_, db, _ := newAuthTestServerWithInstance(t)
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
	_ = user
	var receiverStatus, playbackEndedAt, playbackState string
	if err := db.QueryRow(`SELECT status FROM cast_receiver_sessions WHERE id = 'cast-lost-receiver'`).Scan(&receiverStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = 'cast-protected-source'`).Scan(&playbackEndedAt, &playbackState); err != nil {
		t.Fatal(err)
	}
	if receiverStatus != "active" || playbackEndedAt != "" || playbackState != "playing" {
		t.Fatalf("receiver changed without actor terminal: receiver=%q endedAt=%q playback=%q", receiverStatus, playbackEndedAt, playbackState)
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
		next      playbackQueueOccurrence
		remaining []playbackQueueOccurrence
		ok        bool
	}{
		{name: "queue", snapshot: playbackSessionQueueSnapshot{Current: playbackQueueOccurrence{EntryID: "e1", MediaID: "one"}, RepeatMode: "off", Queue: []playbackQueueOccurrence{{EntryID: "e2", MediaID: "two"}, {EntryID: "e3", MediaID: "three"}}}, next: playbackQueueOccurrence{EntryID: "e2", MediaID: "two"}, remaining: []playbackQueueOccurrence{{EntryID: "e3", MediaID: "three"}}, ok: true},
		{name: "repeat one", snapshot: playbackSessionQueueSnapshot{Current: playbackQueueOccurrence{EntryID: "e1", MediaID: "one"}, RepeatMode: "one", Queue: []playbackQueueOccurrence{{EntryID: "e2", MediaID: "two"}}}, next: playbackQueueOccurrence{EntryID: "e1", MediaID: "one"}, remaining: []playbackQueueOccurrence{{EntryID: "e2", MediaID: "two"}}, ok: true},
		{name: "repeat all", snapshot: playbackSessionQueueSnapshot{Current: playbackQueueOccurrence{EntryID: "e3", MediaID: "three"}, RepeatMode: "all", History: []playbackQueueOccurrence{{HistoryID: "h2", EntryID: "e2", MediaID: "two"}, {HistoryID: "h1", EntryID: "e1", MediaID: "one"}}}, next: playbackQueueOccurrence{EntryID: "e1", MediaID: "one"}, remaining: []playbackQueueOccurrence{{EntryID: "e2", MediaID: "two"}, {EntryID: "e3", MediaID: "three"}}, ok: true},
		{name: "exhausted", snapshot: playbackSessionQueueSnapshot{Current: playbackQueueOccurrence{EntryID: "e1", MediaID: "one"}, RepeatMode: "off"}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, remaining, ok := castQueueNext(tt.snapshot)
			if next != tt.next || ok != tt.ok || !reflect.DeepEqual(remaining, tt.remaining) {
				t.Fatalf("castQueueNext() = (%v, %v, %v), want (%v, %v, %v)", next, remaining, ok, tt.next, tt.remaining, tt.ok)
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
