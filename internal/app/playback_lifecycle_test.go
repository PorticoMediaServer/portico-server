package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type playbackLifecycleFixture struct {
	user      User
	sessionID string
	mediaID   string
	sourceID  string
	channelID string
}

func seedPlaybackLifecycleFixture(t *testing.T, server *Server, suffix string) playbackLifecycleFixture {
	t.Helper()
	user := dvrTestUser(t, server)
	accountID := accountIDForUser(user)
	if accountID == "" {
		accountID = user.ID
	}
	if err := server.db.QueryRow(`SELECT id, account_id FROM profiles WHERE account_id = ? AND is_primary = 1`, accountID).Scan(&user.ProfileID, &user.AccountID); err != nil {
		t.Fatalf("load lifecycle fixture profile: %v", err)
	}
	user.ID = user.AccountID
	user.ProfileIsPrimary = true
	fixture := playbackLifecycleFixture{
		user: user, sessionID: "play_lifecycle_" + suffix, mediaID: "movie_lifecycle_" + suffix,
		sourceID: "source_lifecycle_" + suffix, channelID: "channel_lifecycle_" + suffix,
	}
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	stamp := now.Format(time.RFC3339Nano)
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin playback lifecycle fixture: %v", err)
	}
	defer tx.Rollback()
	statements := []struct {
		query string
		args  []any
	}{
		{`
		INSERT INTO media_items (id, type, title, sort_title, duration_seconds, genres_json, added_at)
		VALUES (?, 'movie', ?, ?, 1000, '[]', ?)`, []any{fixture.mediaID, fixture.mediaID, fixture.mediaID, stamp}},
		{`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES (?, ?, 'm3u', 1, ?, ?)`, []any{fixture.sourceID, fixture.sourceID, stamp, stamp}},
		{`
		INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, 'https://media.example.test/live.m3u8', 1, ?, ?, ?)`, []any{fixture.channelID, fixture.sourceID, fixture.channelID, stamp, stamp, stamp}},
		{`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at,
			state, progress, position_seconds, progress_generation, last_event_sequence, is_live
		) VALUES (?, ?, ?, ?, 'movie', ?, ?, ?, 'playing', 96, 960, 3, 7, 0)`, []any{fixture.sessionID, accountIDForUser(user), viewerProfileID(user), fixture.mediaID, fixture.mediaID, stamp, stamp}},
		{`
		INSERT INTO playback_media_grants (
			id, token_hash, playback_session_id, principal_user_id, profile_id, resource_kind,
			resource_id, operation_classes_json, issued_at, expires_at
		) VALUES (?, ?, ?, ?, ?, 'media', ?, '["byte_range"]', ?, ?)`, []any{"grant_" + suffix, "grant_hash_" + suffix, fixture.sessionID, accountIDForUser(user), viewerProfileID(user), fixture.mediaID, stamp, now.Add(time.Hour).Format(time.RFC3339Nano)}},
		{`
		INSERT INTO playback_session_continuation_credentials (
			playback_session_id, token_hash, user_id, profile_id, client_instance_id, origin,
			issued_at, expires_at, absolute_expires_at
		) VALUES (?, ?, ?, ?, ?, 'https://app.example.test', ?, ?, ?)`, []any{fixture.sessionID, "continuation_hash_" + suffix, accountIDForUser(user), viewerProfileID(user), "client_" + suffix, stamp, now.Add(time.Hour).Format(time.RFC3339Nano), now.Add(24 * time.Hour).Format(time.RFC3339Nano)}},
		{`
		INSERT INTO playback_prepared_handoffs (
			id, user_id, profile_id, source_session_id, client_instance_id, media_id,
			current_entry_id, queue_entries_json, source_context_json, state, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, '[]', '{}', 'prepared', ?, ?)`, []any{"prepared_" + suffix, accountIDForUser(user), viewerProfileID(user), fixture.sessionID, "client_" + suffix, fixture.mediaID, "entry_" + suffix, stamp, now.Add(time.Hour).Format(time.RFC3339Nano)}},
		{`
		INSERT INTO live_tv_tuner_allocations (
			id, source_id, channel_id, allocation_kind, consumer_id, allocation_key,
			lease_token, acquired_at, heartbeat_at
		) VALUES (?, ?, ?, 'live_session', ?, ?, ?, ?, ?)`, []any{"tuner_" + suffix, fixture.sourceID, fixture.channelID, fixture.sessionID, "live_session:" + fixture.sessionID, "lease_" + suffix, stamp, stamp}},
	}
	for index, statement := range statements {
		if _, err := tx.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed playback lifecycle fixture statement %d: %v", index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit playback lifecycle fixture: %v", err)
	}
	return fixture
}

func assertPlaybackLifecycleAuthorityCount(t *testing.T, db *sql.DB, fixture playbackLifecycleFixture, want int) {
	t.Helper()
	queries := map[string]string{
		"grants":        `SELECT COUNT(*) FROM playback_media_grants WHERE playback_session_id = ? AND revoked_at = ''`,
		"continuations": `SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE playback_session_id = ? AND revoked_at = ''`,
		"tuners":        `SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ?`,
		"preparations":  `SELECT COUNT(*) FROM playback_prepared_handoffs WHERE source_session_id = ? AND state = 'prepared'`,
	}
	for name, query := range queries {
		var count int
		if err := db.QueryRow(query, fixture.sessionID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != want {
			t.Fatalf("%s count=%d want=%d", name, count, want)
		}
	}
}

func TestPlaybackLifecycleTerminateAtomicallyFinalizesAndRevokesAllAuthority(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedPlaybackLifecycleFixture(t, server, "atomic")
	now := time.Date(2026, 8, 30, 12, 1, 0, 0, time.UTC)
	recordedAt := now.Add(-time.Second).Format(time.RFC3339Nano)

	result, err := server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
		SessionID: fixture.sessionID, UserID: accountIDForUser(fixture.user), ProfileID: viewerProfileID(fixture.user),
		Cause: playbackTerminationExplicit, RequireActive: true, Now: now,
		Event: &playbackTerminalEvent{
			Disposition: "completed", Generation: 3, EventSequence: 8, RecordedAt: recordedAt,
			PositionSeconds: 1000, DurationSeconds: 1000,
		},
	})
	if err != nil {
		t.Fatalf("terminate playback: %v", err)
	}
	if !result.Changed || !result.ProgressWritten || !result.AuthorityChanged || result.AlreadyTerminated {
		t.Fatalf("unexpected terminal result: %#v", result)
	}
	var endedAt, state, terminalRecordedAt string
	var position int
	var sequence int64
	if err := server.db.QueryRow(`SELECT ended_at, state, position_seconds, last_event_sequence, last_event_recorded_at FROM playback_sessions WHERE id = ?`, fixture.sessionID).
		Scan(&endedAt, &state, &position, &sequence, &terminalRecordedAt); err != nil {
		t.Fatalf("load terminal session: %v", err)
	}
	if endedAt == "" || state != "stopped" || position != 1000 || sequence != 8 || terminalRecordedAt != recordedAt {
		t.Fatalf("terminal session ended=%q state=%q position=%d sequence=%d recorded=%q", endedAt, state, position, sequence, terminalRecordedAt)
	}
	var watched, progress int
	if err := server.db.QueryRow(`SELECT watched, progress_seconds FROM user_media_state WHERE profile_id = ? AND media_id = ?`, viewerProfileID(fixture.user), fixture.mediaID).Scan(&watched, &progress); err != nil {
		t.Fatalf("load finalized progress: %v", err)
	}
	if watched != 1 || progress != 0 {
		t.Fatalf("completed progress watched=%d progress=%d", watched, progress)
	}
	assertPlaybackLifecycleAuthorityCount(t, server.db, fixture, 0)

	// Simulate residue from an interrupted historical release. Idempotent cleanup
	// must fence it without rewriting the already accepted terminal event.
	if _, err := server.db.Exec(`UPDATE playback_media_grants SET revoked_at = '' WHERE playback_session_id = ?`, fixture.sessionID); err != nil {
		t.Fatalf("restore grant residue: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE playback_session_continuation_credentials SET revoked_at = '' WHERE playback_session_id = ?`, fixture.sessionID); err != nil {
		t.Fatalf("restore continuation residue: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO playback_prepared_handoffs (
			id, user_id, profile_id, source_session_id, client_instance_id, media_id,
			current_entry_id, queue_entries_json, source_context_json, state, created_at, expires_at
		) VALUES (?, ?, ?, ?, 'client_residue', ?, 'entry_residue', '[]', '{}', 'prepared', ?, ?)`,
		"prepared_residue", accountIDForUser(fixture.user), viewerProfileID(fixture.user), fixture.sessionID, fixture.mediaID, now.Format(time.RFC3339Nano), now.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed prepared residue: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_tuner_allocations (
			id, source_id, channel_id, allocation_kind, consumer_id, allocation_key,
			lease_token, acquired_at, heartbeat_at
		) VALUES (?, ?, ?, 'live_session', ?, ?, 'lease_residue', ?, ?)`,
		"tuner_residue", fixture.sourceID, fixture.channelID, fixture.sessionID, "live_session:"+fixture.sessionID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed tuner residue: %v", err)
	}
	result, err = server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
		SessionID: fixture.sessionID, Cause: playbackTerminationStale, Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("idempotent residue cleanup: %v", err)
	}
	if !result.AlreadyTerminated || result.Changed || !result.AuthorityChanged {
		t.Fatalf("unexpected idempotent result: %#v", result)
	}
	assertPlaybackLifecycleAuthorityCount(t, server.db, fixture, 0)
}

func TestPlaybackLifecycleRejectsNonFinalCompletedEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedPlaybackLifecycleFixture(t, server, "nonfinal_completed")

	_, err := server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
		SessionID: fixture.sessionID, UserID: accountIDForUser(fixture.user), ProfileID: viewerProfileID(fixture.user),
		Cause: playbackTerminationExplicit, RequireActive: true,
		Event: &playbackTerminalEvent{
			Disposition: "completed", Generation: 3, EventSequence: 8,
			RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), PositionSeconds: 999, DurationSeconds: 1000,
		},
	})
	if !errors.Is(err, errPlaybackTerminalEvidenceInvalid) {
		t.Fatalf("non-final completed evidence error=%v", err)
	}
	assertPlaybackLifecycleAuthorityCount(t, server.db, fixture, 1)
	var endedAt, state string
	if err := server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, fixture.sessionID).Scan(&endedAt, &state); err != nil {
		t.Fatal(err)
	}
	if endedAt != "" || state != "playing" {
		t.Fatalf("invalid terminal mutated source ended=%q state=%q", endedAt, state)
	}
}

func TestPlaybackLifecycleTerminalFenceRollsBackAllMutations(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedPlaybackLifecycleFixture(t, server, "rollback")

	_, err := server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
		SessionID: fixture.sessionID, UserID: accountIDForUser(fixture.user), ProfileID: viewerProfileID(fixture.user),
		Cause: playbackTerminationExplicit, RequireActive: true,
		Event: &playbackTerminalEvent{
			Disposition: "completed", Generation: 2, EventSequence: 8,
			RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), PositionSeconds: 1000, DurationSeconds: 1000,
		},
	})
	if !errors.Is(err, errPlaybackGenerationStale) {
		t.Fatalf("wrong generation error=%v", err)
	}
	var endedAt, state string
	var sequence int64
	if err := server.db.QueryRow(`SELECT ended_at, state, last_event_sequence FROM playback_sessions WHERE id = ?`, fixture.sessionID).Scan(&endedAt, &state, &sequence); err != nil {
		t.Fatal(err)
	}
	if endedAt != "" || state != "playing" || sequence != 7 {
		t.Fatalf("stale terminal mutated session ended=%q state=%q sequence=%d", endedAt, state, sequence)
	}
	assertPlaybackLifecycleAuthorityCount(t, server.db, fixture, 1)
	var mediaStates int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM user_media_state WHERE profile_id = ? AND media_id = ?`, viewerProfileID(fixture.user), fixture.mediaID).Scan(&mediaStates); err != nil || mediaStates != 0 {
		t.Fatalf("stale terminal wrote progress count=%d err=%v", mediaStates, err)
	}
}

func TestPlaybackLifecycleConcurrentTerminationHasOneStateOwner(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedPlaybackLifecycleFixture(t, server, "race")
	const contenders = 8
	results := make(chan playbackTerminationResult, contenders)
	errs := make(chan error, contenders)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < contenders; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			result, err := server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
				SessionID: fixture.sessionID, Cause: PlaybackTerminationCause(fmt.Sprintf("race_%d", index)),
			})
			results <- result
			errs <- err
		}(index)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent terminate: %v", err)
		}
	}
	changed, already := 0, 0
	for result := range results {
		if result.Changed {
			changed++
		}
		if result.AlreadyTerminated {
			already++
		}
	}
	if changed != 1 || already != contenders-1 {
		t.Fatalf("changed=%d already=%d want 1/%d", changed, already, contenders-1)
	}
	assertPlaybackLifecycleAuthorityCount(t, server.db, fixture, 0)
}

func TestPlaybackLifecycleFailedStartRemovesSessionAndAuthority(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedPlaybackLifecycleFixture(t, server, "failed_start")
	result, err := server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
		SessionID: fixture.sessionID, UserID: accountIDForUser(fixture.user), ProfileID: viewerProfileID(fixture.user),
		Cause: playbackTerminationFailedStart, RemoveSession: true,
	})
	if err != nil {
		t.Fatalf("failed-start cleanup: %v", err)
	}
	if !result.Changed || !result.Removed {
		t.Fatalf("unexpected failed-start result: %#v", result)
	}
	var sessions int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, fixture.sessionID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("failed-start session count=%d err=%v", sessions, err)
	}
	assertPlaybackLifecycleAuthorityCount(t, server.db, fixture, 0)
}

func TestPlaybackLifecycleImplicitCleanupPreservesNewerHandoffProgress(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedPlaybackLifecycleFixture(t, server, "handoff_progress")
	observedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	newerAt := observedAt.Add(30 * time.Second)
	if _, err := server.db.Exec(`UPDATE playback_sessions SET last_event_recorded_at = ? WHERE id = ?`, observedAt.Format(time.RFC3339Nano), fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO user_media_state (
			profile_id, user_id, media_id, watched, progress_seconds, updated_at,
			progress_session_id, progress_recorded_at
		) VALUES (?, ?, ?, 0, 990, ?, ?, ?)`,
		viewerProfileID(fixture.user), accountIDForUser(fixture.user), fixture.mediaID,
		newerAt.Format(time.RFC3339), fixture.sessionID, newerAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed newer handoff progress: %v", err)
	}
	result, err := server.playbackLifecycle().Terminate(context.Background(), playbackTerminationRequest{
		SessionID: fixture.sessionID, UserID: accountIDForUser(fixture.user), ProfileID: viewerProfileID(fixture.user),
		Cause: playbackTerminationHandoff, Now: observedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("terminate handoff source: %v", err)
	}
	if result.ProgressWritten {
		t.Fatalf("implicit terminal cleanup overwrote a newer observation: %#v", result)
	}
	var progress int
	var recordedAt string
	if err := server.db.QueryRow(`SELECT progress_seconds, progress_recorded_at FROM user_media_state WHERE profile_id = ? AND media_id = ?`, viewerProfileID(fixture.user), fixture.mediaID).Scan(&progress, &recordedAt); err != nil {
		t.Fatal(err)
	}
	if progress != 990 || recordedAt != newerAt.Format(time.RFC3339Nano) {
		t.Fatalf("newer handoff progress changed progress=%d recordedAt=%q", progress, recordedAt)
	}
}
