package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func playbackAuthorityFixture(t *testing.T) (*Server, User, PlaybackResponse) {
	t.Helper()
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_terminal_authority', 'Terminal Authority', 'movies', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES
			('movie_terminal_source', 'lib_terminal_authority', 'movie', 'Source', 'Source', ?, 'https://media.example.test/source.mp4', 120),
			('movie_terminal_next', 'lib_terminal_authority', 'movie', 'Next', 'Next', ?, 'https://media.example.test/next.mp4', 180)`, now, now); err != nil {
		t.Fatal(err)
	}
	seedExactPlaybackFactsForFixture(t, server, "movie_terminal_next")
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID: "movie_terminal_source", ClientInstanceID: "terminal-authority-client",
		QueueMediaIDs: []string{"movie_terminal_next"}, SkipPreroll: true,
	})
	return server, user, started
}

func prepareAuthorityNext(t *testing.T, server *Server, user User, source PlaybackResponse) PlaybackPreparedResponse {
	t.Helper()
	body, err := json.Marshal(PlaybackPrepareNextRequest{EntryID: source.Queue[0].EntryID})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+source.SessionID+"/prepare-next", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackPrepareNext(rec, req, user, source.SessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", rec.Code, rec.Body.String())
	}
	var prepared PlaybackPreparedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	return prepared
}

func performAuthorityHandoff(t *testing.T, server *Server, user User, sourceSessionID string, request PlaybackHandoffRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+sourceSessionID+"/handoff", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackHandoff(rec, req, user, sourceSessionID)
	return rec
}

func TestPreparedHandoffRejectsStaleTerminalWithoutSideEffects(t *testing.T) {
	for _, tc := range []struct {
		name       string
		generation int64
		sequence   int64
		wantCode   string
	}{
		{name: "generation", generation: 2, sequence: 3, wantCode: "playback_generation_stale"},
		{name: "sequence", generation: 1, sequence: 2, wantCode: "playback_event_sequence_stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, user, source := playbackAuthorityFixture(t)
			position := 44.0
			if _, err := server.touchPlaybackSession(user, source.SessionID, PlaybackProgressEvent{
				Generation: int64(source.Generation), EventSequence: 2,
				RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), PositionSeconds: &position,
			}); err != nil {
				t.Fatal(err)
			}
			prepared := prepareAuthorityNext(t, server, user, source)
			terminal := playbackHandoffTerminalForTest(source, "completed", tc.sequence)
			terminal.Generation = tc.generation
			rec := performAuthorityHandoff(t, server, user, source.SessionID, PlaybackHandoffRequest{
				RequestID: "stale-" + tc.name + "-request", PreparedSessionID: prepared.PreparedSessionID,
				EntryID: source.Queue[0].EntryID, PreviousTerminal: terminal,
			})
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var endedAt, state string
			if err := server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&endedAt, &state); err != nil {
				t.Fatal(err)
			}
			if endedAt != "" || state == "stopped" {
				t.Fatalf("source authority changed: endedAt=%q state=%q", endedAt, state)
			}
			for label, query := range map[string]string{
				"replacement sessions": `SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'movie_terminal_next'`,
				"next media state":     `SELECT COUNT(*) FROM user_media_state WHERE profile_id = '` + viewerProfileID(user) + `' AND media_id = 'movie_terminal_next'`,
				"handoff receipts":     `SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = '` + source.SessionID + `'`,
			} {
				var count int
				if err := server.db.QueryRow(query).Scan(&count); err != nil || count != 0 {
					t.Fatalf("%s residue=%d err=%v", label, count, err)
				}
			}
			var sourceCredentials int
			if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE playback_session_id = ? AND revoked_at = ''`, source.SessionID).Scan(&sourceCredentials); err != nil || sourceCredentials != 1 {
				t.Fatalf("source continuation authority=%d err=%v", sourceCredentials, err)
			}
		})
	}
}

func TestDirectHandoffRejectsStaleTerminalWithoutReplacementResidue(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	position := 44.0
	if _, err := server.touchPlaybackSession(user, source.SessionID, PlaybackProgressEvent{
		Generation: int64(source.Generation), EventSequence: 2,
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), PositionSeconds: &position,
	}); err != nil {
		t.Fatal(err)
	}
	terminal := playbackHandoffTerminalForTest(source, "stopped", 2)
	terminal.PositionSeconds = position
	rec := performAuthorityHandoff(t, server, user, source.SessionID, PlaybackHandoffRequest{
		RequestID: "direct-stale-request", EntryID: source.Queue[0].EntryID, PreviousTerminal: terminal,
	})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "playback_event_sequence_stale") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var sourceActive, replacements, replacementGrants, nextState, receipts int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND ended_at = '' AND state <> 'stopped'`, source.SessionID).Scan(&sourceActive)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'movie_terminal_next'`).Scan(&replacements)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_media_grants WHERE playback_session_id <> ? AND revoked_at = ''`, source.SessionID).Scan(&replacementGrants)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM user_media_state WHERE profile_id = ? AND media_id = 'movie_terminal_next'`, viewerProfileID(user)).Scan(&nextState)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&receipts)
	if sourceActive != 1 || replacements != 0 || replacementGrants != 0 || nextState != 0 || receipts != 0 {
		t.Fatalf("source=%d replacements=%d grants=%d next_state=%d receipts=%d", sourceActive, replacements, replacementGrants, nextState, receipts)
	}
}

func TestPreparedHandoffCommitsTerminalAndRetrySurvivesPrivacyTeardown(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	if _, err := server.db.Exec(`UPDATE playback_sessions SET history_paused = 1 WHERE id = ?`, source.SessionID); err != nil {
		t.Fatal(err)
	}
	prepared := prepareAuthorityNext(t, server, user, source)
	request := PlaybackHandoffRequest{
		RequestID: "privacy-handoff-request", PreparedSessionID: prepared.PreparedSessionID,
		EntryID: source.Queue[0].EntryID, PreviousTerminal: playbackHandoffTerminalForTest(source, "completed", 1),
	}
	first := performAuthorityHandoff(t, server, user, source.SessionID, request)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var firstPlayback PlaybackResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstPlayback); err != nil {
		t.Fatal(err)
	}
	var sourceRows, preparedRows, receiptRows int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&sourceRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_prepared_handoffs WHERE id = ?`, prepared.PreparedSessionID).Scan(&preparedRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ? AND state = 'committed'`, source.SessionID).Scan(&receiptRows)
	if sourceRows != 0 || preparedRows != 0 || receiptRows != 1 {
		t.Fatalf("privacy teardown source=%d prepared=%d durable_receipt=%d", sourceRows, preparedRows, receiptRows)
	}
	retry := performAuthorityHandoff(t, server, user, source.SessionID, request)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	var retried PlaybackResponse
	if err := json.Unmarshal(retry.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	if retried.SessionID != firstPlayback.SessionID || retried.MediaGrant.Token != firstPlayback.MediaGrant.Token || retried.ContinuationCredential == nil || firstPlayback.ContinuationCredential == nil || retried.ContinuationCredential.Token != firstPlayback.ContinuationCredential.Token {
		t.Fatalf("retry did not recover exact handoff response: first=%#v retry=%#v", firstPlayback, retried)
	}
	conflict := request
	conflict.PreviousTerminal = playbackHandoffTerminalForTest(source, "stopped", 1)
	conflicted := performAuthorityHandoff(t, server, user, source.SessionID, conflict)
	if conflicted.Code != http.StatusConflict || !strings.Contains(conflicted.Body.String(), "handoff_request_conflict") {
		t.Fatalf("conflict status=%d body=%s", conflicted.Code, conflicted.Body.String())
	}
}

func TestPreparedHandoffCommitFailureLeavesOnlySourceAuthority(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	prepared := prepareAuthorityNext(t, server, user, source)
	if _, err := server.db.Exec(`
		CREATE TEMP TRIGGER fail_terminal_handoff
		BEFORE UPDATE OF ended_at ON playback_sessions
		WHEN OLD.id = '` + source.SessionID + `' AND NEW.ended_at <> ''
		BEGIN SELECT RAISE(FAIL, 'forced terminal failure'); END`); err != nil {
		t.Fatal(err)
	}
	rec := performAuthorityHandoff(t, server, user, source.SessionID, PlaybackHandoffRequest{
		RequestID: "forced-failure-request", PreparedSessionID: prepared.PreparedSessionID,
		EntryID: source.Queue[0].EntryID, PreviousTerminal: playbackHandoffTerminalForTest(source, "completed", 1),
	})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "handoff_failed") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var sourceActive, replacementRows, replacementGrants, activeContinuations, nextState int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND ended_at = '' AND state <> 'stopped'`, source.SessionID).Scan(&sourceActive)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'movie_terminal_next'`).Scan(&replacementRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_media_grants WHERE playback_session_id <> ? AND revoked_at = ''`, source.SessionID).Scan(&replacementGrants)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE revoked_at = ''`).Scan(&activeContinuations)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM user_media_state WHERE profile_id = ? AND media_id = 'movie_terminal_next'`, viewerProfileID(user)).Scan(&nextState)
	if sourceActive != 1 || replacementRows != 0 || replacementGrants != 0 || activeContinuations != 1 || nextState != 0 {
		t.Fatalf("failed boundary source=%d replacements=%d replacement_grants=%d continuations=%d next_state=%d", sourceActive, replacementRows, replacementGrants, activeContinuations, nextState)
	}
}

func TestPreparedHandoffRecoversStaleCrashClaimAndPendingReplacement(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	prepared := prepareAuthorityNext(t, server, user, source)
	request := PlaybackHandoffRequest{
		RequestID: "restart-recovery-request", PreparedSessionID: prepared.PreparedSessionID,
		EntryID: source.Queue[0].EntryID, PreviousTerminal: playbackHandoffTerminalForTest(source, "completed", 1),
	}
	fingerprint := playbackHandoffFingerprint(request)
	committed, claim, err := server.consumeDirectPlaybackHandoff(t.Context(), user, source.SessionID, request.RequestID, fingerprint, nil, nil)
	if err != nil || committed != nil || claim.ID == "" || claim.ReplacementSessionID == "" {
		t.Fatalf("reserve committed=%#v claim=%#v err=%v", committed, claim, err)
	}
	if _, consumeErr := server.consumePreparedPlaybackHandoff(t.Context(), user, source.SessionID, "terminal-authority-client", prepared.PreparedSessionID, request.RequestID, fingerprint); consumeErr != nil {
		t.Fatalf("reserve prepared: %#v", consumeErr)
	}
	orphan, startErr := server.startPlaybackForRequest(
		httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil), user,
		PlaybackSessionCreateRequest{
			MediaID: "movie_terminal_next", ClientInstanceID: "terminal-authority-client", SkipPreroll: true,
			deferReplacement: true, reservedSessionID: claim.ReplacementSessionID,
		},
	)
	if startErr != nil {
		t.Fatalf("create orphan: %#v", startErr)
	}
	if orphan.SessionID != claim.ReplacementSessionID {
		t.Fatalf("replacement did not use claim-owned identity: got=%s want=%s", orphan.SessionID, claim.ReplacementSessionID)
	}
	if _, err := server.db.Exec(`UPDATE playback_handoff_receipts SET claim_expires_at = ? WHERE source_session_id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), source.SessionID); err != nil {
		t.Fatal(err)
	}
	recovered := performAuthorityHandoff(t, server, user, source.SessionID, request)
	if recovered.Code != http.StatusOK {
		t.Fatalf("recovered status=%d body=%s", recovered.Code, recovered.Body.String())
	}
	var replacement PlaybackResponse
	if err := json.Unmarshal(recovered.Body.Bytes(), &replacement); err != nil {
		t.Fatal(err)
	}
	if replacement.SessionID == orphan.SessionID {
		t.Fatalf("stale unexposed replacement was reused: %s", orphan.SessionID)
	}
	var orphanRows, pendingRows, activeRows int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, orphan.SessionID).Scan(&orphanRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE state = 'handoff_pending'`).Scan(&pendingRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE ended_at = '' AND state <> 'stopped'`).Scan(&activeRows)
	if orphanRows != 0 || pendingRows != 0 || activeRows != 1 {
		t.Fatalf("recovery orphan=%d pending=%d active=%d", orphanRows, pendingRows, activeRows)
	}
}

func TestLiveHandoffClaimRejectsConcurrentDuplicateWithoutSecondReplacement(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	request := PlaybackHandoffRequest{
		RequestID: "concurrent-handoff-request", EntryID: source.CurrentQueueEntryID,
		PreviousTerminal: playbackHandoffTerminalForTest(source, "stopped", 1),
	}
	fingerprint := playbackHandoffFingerprint(request)
	_, claim, err := server.consumeDirectPlaybackHandoff(t.Context(), user, source.SessionID, request.RequestID, fingerprint, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, startErr := server.startPlaybackForRequest(
		httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil), user,
		PlaybackSessionCreateRequest{
			MediaID: "movie_terminal_source", ClientInstanceID: "terminal-authority-client", SkipPreroll: true,
			deferReplacement: true, reservedSessionID: claim.ReplacementSessionID,
		},
	)
	if startErr != nil {
		t.Fatalf("create pending: %#v", startErr)
	}
	if pending.SessionID != claim.ReplacementSessionID {
		t.Fatalf("replacement did not use claim-owned identity: got=%s want=%s", pending.SessionID, claim.ReplacementSessionID)
	}
	duplicate := performAuthorityHandoff(t, server, user, source.SessionID, request)
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "handoff_in_progress") {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	var pendingRows, sourceActive int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE state = 'handoff_pending'`).Scan(&pendingRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND ended_at = ''`, source.SessionID).Scan(&sourceActive)
	if pendingRows != 1 || sourceActive != 1 {
		t.Fatalf("concurrent duplicate pending=%d source=%d", pendingRows, sourceActive)
	}
}

func TestHandoffCommitsRejectSuccessorOutsideClaimAndRetainSource(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	claim := playbackHandoffClaim{
		ID: "claim-owned-successor", ReplacementSessionID: "expected-successor",
		AuthorizationRevision: "pinned-revision", ClientInstanceID: "terminal-authority-client",
	}
	terminal := *playbackHandoffTerminalForTest(source, "stopped", 1)
	wrong := PlaybackResponse{SessionID: "caller-selected-successor"}
	if err := server.commitDirectPlaybackHandoff(t.Context(), user, source.SessionID, terminal, "claim-mismatch-direct", "direct-fingerprint", claim, wrong); !errors.Is(err, errPreparedHandoffConflict) {
		t.Fatalf("direct mismatch error=%v", err)
	}
	prepared := preparedPlaybackHandoff{
		ID: "prepared-claim-mismatch", UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
		SessionID: source.SessionID, RequestID: "claim-mismatch-prepared", Fingerprint: "prepared-fingerprint",
	}
	if err := server.commitPreparedPlaybackHandoff(t.Context(), prepared, wrong, terminal, claim); !errors.Is(err, errPreparedHandoffConflict) {
		t.Fatalf("prepared mismatch error=%v", err)
	}
	var endedAt, state string
	if err := server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&endedAt, &state); err != nil {
		t.Fatal(err)
	}
	if endedAt != "" || state != "playing" {
		t.Fatalf("mismatched successor changed source: ended=%q state=%q", endedAt, state)
	}
}

func TestDirectReplayStartSecondsPresenceAndExactRetry(t *testing.T) {
	for _, tc := range []struct {
		name          string
		startSeconds  *int
		wantResumeSec int
	}{
		{name: "explicit zero", startSeconds: func() *int { value := 0; return &value }(), wantResumeSec: 0},
		{name: "terminal fallback", wantResumeSec: 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, user, source := playbackAuthorityFixture(t)
			position := 60.0
			if _, err := server.touchPlaybackSession(user, source.SessionID, PlaybackProgressEvent{
				Generation: int64(source.Generation), EventSequence: 1,
				RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), PositionSeconds: &position,
			}); err != nil {
				t.Fatal(err)
			}
			terminal := playbackHandoffTerminalForTest(source, "stopped", 2)
			terminal.PositionSeconds = position
			request := PlaybackHandoffRequest{
				RequestID: "direct-replay-" + strings.ReplaceAll(tc.name, " ", "-"), EntryID: source.CurrentQueueEntryID,
				PreviousTerminal: terminal, StartSeconds: tc.startSeconds,
			}
			first := performAuthorityHandoff(t, server, user, source.SessionID, request)
			if first.Code != http.StatusOK {
				t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
			}
			var replacement PlaybackResponse
			if err := json.Unmarshal(first.Body.Bytes(), &replacement); err != nil {
				t.Fatal(err)
			}
			if replacement.ResumePositionSeconds != tc.wantResumeSec {
				t.Fatalf("resume=%d want=%d", replacement.ResumePositionSeconds, tc.wantResumeSec)
			}
			retry := performAuthorityHandoff(t, server, user, source.SessionID, request)
			if retry.Code != http.StatusOK {
				t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
			}
			var retried PlaybackResponse
			_ = json.Unmarshal(retry.Body.Bytes(), &retried)
			if retried.SessionID != replacement.SessionID || retried.MediaGrant.Token != replacement.MediaGrant.Token {
				t.Fatalf("direct retry changed replacement: first=%#v retry=%#v", replacement, retried)
			}
			var active, pending int
			_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE ended_at = '' AND state <> 'stopped'`).Scan(&active)
			_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE state = 'handoff_pending'`).Scan(&pending)
			if active != 1 || pending != 0 {
				t.Fatalf("authority count active=%d pending=%d", active, pending)
			}
		})
	}
}

func TestHandoffRequestIDRejectsShortAndInvalidCharacters(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	for _, requestID := range []string{"short", "invalid request", strings.Repeat("x", 129)} {
		rec := performAuthorityHandoff(t, server, user, source.SessionID, PlaybackHandoffRequest{
			RequestID: requestID, EntryID: source.CurrentQueueEntryID,
			PreviousTerminal: playbackHandoffTerminalForTest(source, "stopped", 1),
		})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "handoff_request_id_invalid") {
			t.Fatalf("requestId=%q status=%d body=%s", requestID, rec.Code, rec.Body.String())
		}
	}
}

func replacementStartRequestForTest(source PlaybackResponse) PlaybackSessionCreateRequest {
	return PlaybackSessionCreateRequest{
		MediaID: "movie_terminal_next", ClientInstanceID: "terminal-authority-client", SkipPreroll: true,
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{}),
		Intent:        PlaybackIntent{Quality: PlaybackQualitySelection{Mode: playbackQualityModeAutomatic}},
		Replacement: &PlaybackReplacementRequest{
			SourceSessionID: source.SessionID, RequestID: "general-replacement-request",
			PreviousTerminal: *playbackHandoffTerminalForTest(source, "stopped", 1),
		},
	}
}

func performReplacementStart(t *testing.T, server *Server, user User, request PlaybackSessionCreateRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	return rec
}

func TestGeneralTargetReplacementCommitsExactlyOnceAndLeavesOneAuthority(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	request := replacementStartRequestForTest(source)
	first := performReplacementStart(t, server, user, request)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var replacement PlaybackResponse
	if err := json.Unmarshal(first.Body.Bytes(), &replacement); err != nil {
		t.Fatal(err)
	}
	retry := performReplacementStart(t, server, user, request)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	var retried PlaybackResponse
	_ = json.Unmarshal(retry.Body.Bytes(), &retried)
	if retried.SessionID != replacement.SessionID || retried.MediaGrant.Token != replacement.MediaGrant.Token {
		t.Fatalf("retry changed committed successor: first=%s retry=%s", replacement.SessionID, retried.SessionID)
	}
	var sourceActive, successorActive, pending, activeForClient int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND ended_at = '' AND state <> 'stopped'`, source.SessionID).Scan(&sourceActive)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND ended_at = '' AND state = 'playing'`, replacement.SessionID).Scan(&successorActive)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE state = 'handoff_pending'`).Scan(&pending)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE profile_id = ? AND client_instance_id = ? AND ended_at = '' AND state NOT IN ('stopped','handoff_pending')`, viewerProfileID(user), "terminal-authority-client").Scan(&activeForClient)
	if sourceActive != 0 || successorActive != 1 || pending != 0 || activeForClient != 1 {
		t.Fatalf("source=%d successor=%d pending=%d activeClient=%d", sourceActive, successorActive, pending, activeForClient)
	}
}

func TestGeneralTargetReplacementClassifiesInactiveSourceBeforeReservation(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	if _, err := server.db.Exec(`UPDATE playback_sessions SET state = 'stopped', ended_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), source.SessionID); err != nil {
		t.Fatal(err)
	}
	rec := performReplacementStart(t, server, user, replacementStartRequestForTest(source))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "replacement_source_inactive") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var successors, claims int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'movie_terminal_next'`).Scan(&successors)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&claims)
	if successors != 0 || claims != 0 {
		t.Fatalf("inactive reservation left successors=%d claims=%d", successors, claims)
	}
}

func TestGeneralTargetReplacementClassifiesSourceInactiveAfterClaimWithoutResidue(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		CREATE TEMP TRIGGER stop_source_after_replacement_claim
		AFTER INSERT ON playback_sessions
		WHEN NEW.state = 'handoff_pending' AND NEW.media_id = 'movie_terminal_next'
		BEGIN
			UPDATE playback_sessions SET state = 'stopped', ended_at = '` + endedAt + `'
			WHERE id = '` + source.SessionID + `';
		END`); err != nil {
		t.Fatal(err)
	}
	rec := performReplacementStart(t, server, user, replacementStartRequestForTest(source))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "replacement_source_inactive") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var successors, claims int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'movie_terminal_next'`).Scan(&successors)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&claims)
	if successors != 0 || claims != 0 {
		t.Fatalf("inactive commit left successors=%d claims=%d", successors, claims)
	}
}

func TestGeneralTargetReplacementPreservesClientOwnershipConflict(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	request := replacementStartRequestForTest(source)
	request.ClientInstanceID = "different-client-instance"
	rec := performReplacementStart(t, server, user, request)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "replacement_source_conflict") || strings.Contains(rec.Body.String(), "replacement_source_inactive") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var claims int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&claims)
	if claims != 0 {
		t.Fatalf("ownership conflict retained claims=%d", claims)
	}
}

func TestGeneralTargetReplacementPreservesActiveRevisionConflict(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	if _, err := server.db.Exec(`
		CREATE TEMP TRIGGER drift_source_after_replacement_claim
		AFTER INSERT ON playback_sessions
		WHEN NEW.state = 'handoff_pending' AND NEW.media_id = 'movie_terminal_next'
		BEGIN
			UPDATE playback_sessions SET queue_revision = queue_revision + 1
			WHERE id = '` + source.SessionID + `';
		END`); err != nil {
		t.Fatal(err)
	}
	rec := performReplacementStart(t, server, user, replacementStartRequestForTest(source))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "handoff_source_revision_conflict") || strings.Contains(rec.Body.String(), "replacement_source_inactive") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var sourceActive, successors, claims int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ? AND ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')`, source.SessionID).Scan(&sourceActive)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'movie_terminal_next'`).Scan(&successors)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ?`, source.SessionID).Scan(&claims)
	if sourceActive != 1 || successors != 0 || claims != 0 {
		t.Fatalf("revision conflict source=%d successors=%d claims=%d", sourceActive, successors, claims)
	}
}

func TestReplacementCommittedTombstoneNeverReplaysExpiredBearers(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	request := replacementStartRequestForTest(source)
	first := performReplacementStart(t, server, user, request)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var replacement PlaybackResponse
	_ = json.Unmarshal(first.Body.Bytes(), &replacement)
	if _, err := server.db.Exec(`UPDATE playback_handoff_receipts SET payload_expires_at = ? WHERE source_session_id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), source.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := server.prunePlaybackReplacementPayloads(t.Context(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	retry := performReplacementStart(t, server, user, request)
	if retry.Code != http.StatusConflict || !strings.Contains(retry.Body.String(), "playback_replacement_committed_restore_required") || !strings.Contains(retry.Body.String(), replacement.SessionID) {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	var receiptRows, payloadRows int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ? AND state = 'committed'`, source.SessionID).Scan(&receiptRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_handoff_receipts WHERE source_session_id = ? AND committed_response <> ''`, source.SessionID).Scan(&payloadRows)
	if receiptRows != 1 || payloadRows != 0 {
		t.Fatalf("tombstone=%d payload=%d", receiptRows, payloadRows)
	}
}

func TestReplacementAndUserTerminalReceiptsDoNotReplayAcrossAuthorizationRevision(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		server, user, source := playbackAuthorityFixture(t)
		request := replacementStartRequestForTest(source)
		if rec := performReplacementStart(t, server, user, request); rec.Code != http.StatusOK {
			t.Fatalf("commit status=%d body=%s", rec.Code, rec.Body.String())
		}
		if _, err := server.db.Exec(`UPDATE profiles SET policy_updated_at = ? WHERE id = ?`, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano), viewerProfileID(user)); err != nil {
			t.Fatal(err)
		}
		retry := performReplacementStart(t, server, user, request)
		if retry.Code != http.StatusConflict || !strings.Contains(retry.Body.String(), "playback_replacement_scope_changed") {
			t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
		}
	})
	t.Run("standalone terminal", func(t *testing.T) {
		server, user, source := playbackAuthorityFixture(t)
		body := PlaybackSessionStopRequest{RequestID: "scope-terminal-request", Terminal: *playbackHandoffTerminalForTest(source, "stopped", 1)}
		perform := func() *httptest.ResponseRecorder {
			encoded, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodDelete, "/api/playback-sessions/"+source.SessionID, bytes.NewReader(encoded))
			rec := httptest.NewRecorder()
			server.handlePlaybackSessionRoute(rec, req, user)
			return rec
		}
		if first := perform(); first.Code != http.StatusOK {
			t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
		}
		if _, err := server.db.Exec(`UPDATE profiles SET policy_updated_at = ? WHERE id = ?`, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano), viewerProfileID(user)); err != nil {
			t.Fatal(err)
		}
		retry := perform()
		if retry.Code != http.StatusConflict || !strings.Contains(retry.Body.String(), "playback_terminal_scope_changed") {
			t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
		}
	})
}

func TestCommittingReplacementFencesQueueCommandsAndClaimTheft(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	requestID := "mutation-fence-request"
	fingerprint := hashToken("mutation-fence-target")
	var originalCommandJSON string
	if err := server.db.QueryRow(`SELECT command_json FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&originalCommandJSON); err != nil {
		t.Fatal(err)
	}
	_, claim, err := server.consumeDirectPlaybackHandoff(t.Context(), user, source.SessionID, requestID, fingerprint, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claim.ID == "" || playbackHandoffClaimTTL < 5*time.Minute {
		t.Fatalf("claim=%#v ttl=%s", claim, playbackHandoffClaimTTL)
	}
	if _, err := server.replacePlaybackSessionQueueState(t.Context(), user, source.SessionID, source.QueueRevision, []MediaItem{}, "off", "fenced-queue-request", hashToken("queue")); !errors.Is(err, errPlaybackHandoffInProgress) {
		t.Fatalf("queue mutation error=%v", err)
	}
	if _, err := server.issuePlaybackCommand(user, source.SessionID, PlaybackCommandRequest{Action: "pause"}); !errors.Is(err, errPlaybackHandoffInProgress) {
		t.Fatalf("command error=%v", err)
	}
	if _, _, err := server.consumeDirectPlaybackHandoff(t.Context(), user, source.SessionID, requestID, fingerprint, nil, nil); !errors.Is(err, errPlaybackHandoffInProgress) {
		t.Fatalf("unexpired claim was stolen: %v", err)
	}
	var queueRevision int64
	var commandJSON string
	if err := server.db.QueryRow(`SELECT queue_revision, command_json FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&queueRevision, &commandJSON); err != nil {
		t.Fatal(err)
	}
	if queueRevision != source.QueueRevision || commandJSON != originalCommandJSON {
		t.Fatalf("fenced source mutated: queue=%d command=%q", queueRevision, commandJSON)
	}
}

func TestStandaloneTerminalReceiptsAreExactAndSurvivePrivacyTeardown(t *testing.T) {
	server, user, source := playbackAuthorityFixture(t)
	if _, err := server.db.Exec(`UPDATE playback_sessions SET history_paused = 1 WHERE id = ?`, source.SessionID); err != nil {
		t.Fatal(err)
	}
	reqBody := PlaybackSessionStopRequest{
		RequestID: "terminal-receipt-request",
		Terminal:  *playbackHandoffTerminalForTest(source, "completed", 1),
	}
	perform := func(body PlaybackSessionStopRequest) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodDelete, "/api/playback-sessions/"+source.SessionID, bytes.NewReader(encoded))
		rec := httptest.NewRecorder()
		server.handlePlaybackSessionRoute(rec, req, user)
		return rec
	}
	first := perform(reqBody)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var firstAck PlaybackSessionTerminalAcknowledgement
	_ = json.Unmarshal(first.Body.Bytes(), &firstAck)
	if !firstAck.Accepted || firstAck.Duplicate {
		t.Fatalf("first ack=%#v", firstAck)
	}
	var sourceRows, receiptRows int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&sourceRows)
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_terminal_receipts WHERE playback_session_id = ?`, source.SessionID).Scan(&receiptRows)
	if sourceRows != 0 || receiptRows != 1 {
		t.Fatalf("privacy teardown source=%d receipt=%d", sourceRows, receiptRows)
	}
	retry := perform(reqBody)
	var retryAck PlaybackSessionTerminalAcknowledgement
	_ = json.Unmarshal(retry.Body.Bytes(), &retryAck)
	if retry.Code != http.StatusOK || !retryAck.Accepted || !retryAck.Duplicate || retryAck.Terminal != firstAck.Terminal {
		t.Fatalf("retry status=%d ack=%#v body=%s", retry.Code, retryAck, retry.Body.String())
	}
	conflict := reqBody
	conflict.Terminal.Disposition = "stopped"
	conflict.Terminal.PositionSeconds--
	conflicted := perform(conflict)
	if conflicted.Code != http.StatusConflict || !strings.Contains(conflicted.Body.String(), "playback_terminal_request_conflict") {
		t.Fatalf("conflict status=%d body=%s", conflicted.Code, conflicted.Body.String())
	}
	changedCompleted := reqBody
	changedCompleted.Terminal.PositionSeconds--
	changedCompletedRec := perform(changedCompleted)
	if changedCompletedRec.Code != http.StatusBadRequest || !strings.Contains(changedCompletedRec.Body.String(), "invalid_playback_terminal_position") {
		t.Fatalf("changed completed body status=%d body=%s", changedCompletedRec.Code, changedCompletedRec.Body.String())
	}
	differentID := reqBody
	differentID.RequestID = "different-terminal-request"
	differentIDRec := perform(differentID)
	if differentIDRec.Code != http.StatusConflict || !strings.Contains(differentIDRec.Body.String(), "playback_terminal_request_conflict") {
		t.Fatalf("different requestId status=%d body=%s", differentIDRec.Code, differentIDRec.Body.String())
	}
	for _, invalidID := range []string{"short", "invalid request id", strings.Repeat("x", 129)} {
		invalid := reqBody
		invalid.RequestID = invalidID
		invalidRec := perform(invalid)
		if invalidRec.Code != http.StatusBadRequest || !strings.Contains(invalidRec.Body.String(), "playback_terminal_request_id_invalid") {
			t.Fatalf("invalid id %q status=%d body=%s", invalidID, invalidRec.Code, invalidRec.Body.String())
		}
	}
}

func TestContinuationTerminalLostResponseRetryUsesOnlyExactRevokedCredential(t *testing.T) {
	server, _, source := playbackAuthorityFixture(t)
	if source.ContinuationCredential == nil {
		t.Fatal("missing continuation credential")
	}
	reqBody := PlaybackSessionStopRequest{
		RequestID: "native-terminal-request",
		Terminal:  *playbackHandoffTerminalForTest(source, "stopped", 1),
	}
	perform := func(method, token string, body any) *httptest.ResponseRecorder {
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		req := httptest.NewRequest(method, "/api/playback-sessions/"+source.SessionID+"/continuation", reader)
		req.Header.Set("Authorization", "PorticoPlayback "+token)
		rec := httptest.NewRecorder()
		server.handlePlaybackContinuationRoute(rec, req)
		return rec
	}
	token := source.ContinuationCredential.Token
	first := perform(http.MethodDelete, token, reqBody)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	retry := perform(http.MethodDelete, token, reqBody)
	var retryAck PlaybackSessionTerminalAcknowledgement
	_ = json.Unmarshal(retry.Body.Bytes(), &retryAck)
	if retry.Code != http.StatusOK || !retryAck.Accepted || !retryAck.Duplicate {
		t.Fatalf("retry status=%d ack=%#v body=%s", retry.Code, retryAck, retry.Body.String())
	}
	conflict := reqBody
	conflict.Terminal.PositionSeconds = 1
	conflicted := perform(http.MethodDelete, token, conflict)
	if conflicted.Code != http.StatusConflict || !strings.Contains(conflicted.Body.String(), "playback_terminal_request_conflict") {
		t.Fatalf("conflict status=%d body=%s", conflicted.Code, conflicted.Body.String())
	}
	if get := perform(http.MethodGet, token, nil); get.Code != http.StatusUnauthorized {
		t.Fatalf("revoked credential reopened GET: status=%d body=%s", get.Code, get.Body.String())
	}
	if wrong := perform(http.MethodDelete, token+"wrong", reqBody); wrong.Code != http.StatusUnauthorized {
		t.Fatalf("different credential replay status=%d body=%s", wrong.Code, wrong.Body.String())
	}
}
