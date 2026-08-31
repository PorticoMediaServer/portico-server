package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestPlaybackQueueOccurrenceIdentityDuplicatesMovesReceiptsAndRestart(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_f091_queue', 'F091 Queue', 'music', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES
			('track_f091_a', 'lib_f091_queue', 'track', 'A', 'A', ?, 'https://media.example.test/a.mp3', 120),
			('track_f091_b', 'lib_f091_queue', 'track', 'B', 'B', ?, 'https://media.example.test/b.mp3', 120)`, now, now); err != nil {
		t.Fatal(err)
	}
	seedExactPlaybackFactsForFixture(t, server, "track_f091_a")
	seedExactPlaybackFactsForFixture(t, server, "track_f091_b")
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID:       "track_f091_a",
		QueueMediaIDs: []string{"track_f091_b", "track_f091_b", "track_f091_b"},
		SkipPreroll:   true,
	})
	if len(started.Queue) != 3 {
		t.Fatalf("SD-027 duplicate queue length=%d queue=%#v", len(started.Queue), started.Queue)
	}
	seen := map[string]bool{started.CurrentQueueEntryID: true}
	for _, entry := range started.Queue {
		if entry.Media.ID != "track_f091_b" || entry.EntryID == "" || seen[entry.EntryID] {
			t.Fatalf("SD-027 queue occurrence is not distinct: %#v", started)
		}
		seen[entry.EntryID] = true
	}

	middle := started.Queue[1].EntryID
	remove := PlaybackSessionQueueRequest{Action: "remove", EntryID: middle, IdempotencyKey: "f091-remove-middle"}
	remove.ExpectedRevision = int64Ptr(0)
	if replayed, err := server.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, 0, remove, playbackQueueRequestFingerprint("mutate", remove)); err != nil || replayed {
		t.Fatalf("SD-028 remove middle replayed=%t err=%v", replayed, err)
	}
	snapshot, err := server.playbackSessionQueueSnapshot(context.Background(), user, started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Queue) != 2 || snapshot.Queue[0].EntryID != started.Queue[0].EntryID || snapshot.Queue[1].EntryID != started.Queue[2].EntryID {
		t.Fatalf("SD-028 removed more than the addressed occurrence: %#v", snapshot.Queue)
	}

	move := PlaybackSessionQueueRequest{
		Action: "reorder", EntryID: snapshot.Queue[1].EntryID, DestinationEntryID: snapshot.Queue[0].EntryID,
		Placement: "before", IdempotencyKey: "f091-move-occurrence", ExpectedRevision: int64Ptr(snapshot.Revision),
	}
	moveFingerprint := playbackQueueRequestFingerprint("mutate", move)
	if replayed, err := server.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, snapshot.Revision, move, moveFingerprint); err != nil || replayed {
		t.Fatalf("SD-029 move replayed=%t err=%v", replayed, err)
	}
	moved, _ := server.playbackSessionQueueSnapshot(context.Background(), user, started.SessionID)
	if moved.Queue[0].EntryID != started.Queue[2].EntryID || moved.Queue[1].EntryID != started.Queue[0].EntryID {
		t.Fatalf("SD-029 move changed occurrence identities: %#v", moved.Queue)
	}

	repeat := PlaybackSessionQueueRequest{Action: "set_repeat", RepeatMode: "all", IdempotencyKey: "f091-intervening", ExpectedRevision: int64Ptr(moved.Revision)}
	if _, err := server.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, moved.Revision, repeat, playbackQueueRequestFingerprint("mutate", repeat)); err != nil {
		t.Fatal(err)
	}
	afterIntervening, _ := server.playbackSessionQueueSnapshot(context.Background(), user, started.SessionID)
	if replayed, err := server.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, *move.ExpectedRevision, move, moveFingerprint); err != nil || !replayed {
		t.Fatalf("SD-030 lost-response replay replayed=%t err=%v", replayed, err)
	}
	afterReplay, _ := server.playbackSessionQueueSnapshot(context.Background(), user, started.SessionID)
	if afterReplay.Revision != afterIntervening.Revision || afterReplay.RepeatMode != "all" {
		t.Fatalf("SD-030 replay did not return authoritative current state: before=%#v after=%#v", afterIntervening, afterReplay)
	}
	conflict := move
	conflict.Placement = "after"
	if _, err := server.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, *conflict.ExpectedRevision, conflict, playbackQueueRequestFingerprint("mutate", conflict)); !errors.Is(err, errPlaybackQueueIdempotencyConflict) {
		t.Fatalf("same key with different fingerprint err=%v", err)
	}

	revision := afterReplay.Revision
	var last PlaybackSessionQueueRequest
	for index := 0; index < 140; index++ {
		mode := "off"
		if index%2 == 0 {
			mode = "all"
		}
		last = PlaybackSessionQueueRequest{Action: "set_repeat", RepeatMode: mode, IdempotencyKey: fmt.Sprintf("f091-retained-%03d", index), ExpectedRevision: int64Ptr(revision)}
		if _, err := server.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, revision, last, playbackQueueRequestFingerprint("mutate", last)); err != nil {
			t.Fatalf("receipt mutation %d: %v", index, err)
		}
		revision++
	}
	var receiptCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_queue_receipts WHERE session_id = ?`, started.SessionID).Scan(&receiptCount); err != nil || receiptCount != 128 {
		t.Fatalf("bounded receipt count=%d err=%v", receiptCount, err)
	}

	reopenedDB, err := database.Open(server.cfg)
	if err != nil {
		t.Fatalf("reopen queue database: %v", err)
	}
	defer reopenedDB.Close()
	reopened := NewInertServer(server.cfg, reopenedDB, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if replayed, err := reopened.mutatePlaybackSessionQueueState(context.Background(), user, started.SessionID, *last.ExpectedRevision, last, playbackQueueRequestFingerprint("mutate", last)); err != nil || !replayed {
		t.Fatalf("durable receipt replay after reopen replayed=%t err=%v", replayed, err)
	}
}

func TestWatchWithFriendsQueueOccurrenceIdentityPreservesDuplicates(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var group WatchWithFriendsGroup
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_saffron"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("create group status=%d body=%s", status, body)
	}
	for index := 0; index < 2; index++ {
		status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueRequest{
			MediaID: "movie_saffron", ExpectedRevision: int64Ptr(group.Revision), IdempotencyKey: fmt.Sprintf("f091-wwf-add-%d", index),
		}, &group)
		if status != http.StatusOK {
			t.Fatalf("add duplicate %d status=%d body=%s", index, status, body)
		}
	}
	if len(group.Queue) != 3 {
		t.Fatalf("SD-027 WWF duplicate count=%d queue=%#v", len(group.Queue), group.Queue)
	}
	seen := map[string]bool{}
	for _, item := range group.Queue {
		if item.MediaID != "movie_saffron" || item.EntryID == "" || seen[item.EntryID] {
			t.Fatalf("WWF occurrence identity collapsed: %#v", group.Queue)
		}
		seen[item.EntryID] = true
	}

	middleEntryID := group.Queue[1].EntryID
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue/"+url.PathEscape(middleEntryID)+fmt.Sprintf("?expectedRevision=%d&idempotencyKey=f091-wwf-remove", group.Revision), nil, &group)
	if status != http.StatusOK || len(group.Queue) != 2 || group.Queue[0].EntryID == middleEntryID || group.Queue[1].EntryID == middleEntryID {
		t.Fatalf("SD-028 WWF exact remove status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueOrderRequest{
		EntryID: group.Queue[1].EntryID, DestinationEntryID: group.Queue[0].EntryID, Placement: "before",
		ExpectedRevision: int64Ptr(group.Revision), IdempotencyKey: "f091-wwf-move",
	}, &group)
	if status != http.StatusOK || group.Queue[0].EntryID == group.CurrentEntryID {
		t.Fatalf("SD-029 WWF exact move status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	loadEntryID := group.Queue[0].EntryID
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{
		Action: "load", EntryID: loadEntryID, ExpectedRevision: int64Ptr(group.Revision), IdempotencyKey: "f091-wwf-load",
	}, &group)
	if status != http.StatusOK || group.CurrentEntryID != loadEntryID {
		t.Fatalf("WWF load did not preserve occurrence identity status=%d body=%s group=%#v", status, body, group)
	}
}

func TestPlaybackQueueUnavailableProjectionPreservesOccurrenceAndMediaIdentity(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	entries, err := server.playbackQueueEntriesForOccurrences(t.Context(), viewerProfileID(user), []playbackQueueOccurrence{{EntryID: "entry-unavailable", MediaID: "media-unavailable"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].EntryID != "entry-unavailable" || entries[0].Media.ID != "media-unavailable" || !entries[0].Media.Missing {
		t.Fatalf("unavailable occurrence projection lost stable identity: %#v", entries)
	}
}

func TestPlaybackReplayHandoffPreservesCurrentOccurrenceAndQueue(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_f091_replay', 'F091 Replay', 'music', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds) VALUES
		('track_f091_replay_a', 'lib_f091_replay', 'track', 'A', 'A', ?, 'https://media.example.test/a.mp3', 120),
		('track_f091_replay_b', 'lib_f091_replay', 'track', 'B', 'B', ?, 'https://media.example.test/b.mp3', 120)`, now, now); err != nil {
		t.Fatal(err)
	}
	seedExactPlaybackFactsForFixture(t, server, "track_f091_replay_a")
	seedExactPlaybackFactsForFixture(t, server, "track_f091_replay_b")
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID: "track_f091_replay_a", QueueMediaIDs: []string{"track_f091_replay_b", "track_f091_replay_b"}, SkipPreroll: true,
	})
	body, err := json.Marshal(PlaybackHandoffRequest{RequestID: "handoff-replay-current", EntryID: started.CurrentQueueEntryID, PreviousTerminal: playbackHandoffTerminalForTest(started, "stopped", 1)})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+started.SessionID+"/handoff", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackHandoff(rec, req, user, started.SessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay handoff status=%d body=%s", rec.Code, rec.Body.String())
	}
	var replayed PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &replayed); err != nil {
		t.Fatal(err)
	}
	if replayed.CurrentQueueEntryID != started.CurrentQueueEntryID || len(replayed.Queue) != len(started.Queue) {
		t.Fatalf("replay changed occurrence identity: started=%#v replayed=%#v", started, replayed)
	}
	for index := range started.Queue {
		if replayed.Queue[index].EntryID != started.Queue[index].EntryID {
			t.Fatalf("replay queue entry %d changed identity: started=%#v replayed=%#v", index, started.Queue, replayed.Queue)
		}
	}
	if snapshot, err := server.playbackSessionQueueSnapshot(t.Context(), user, replayed.SessionID); err != nil || len(snapshot.History) != 0 {
		t.Fatalf("replay invented history event: history=%#v err=%v", snapshot.History, err)
	}
}

func TestPlaybackFailedStartCleanupCoversContinuityAndHistoryPersistence(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_f091_failure', 'F091 Failure', 'music', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds) VALUES
		('track_f091_failure_a', 'lib_f091_failure', 'track', 'A', 'A', ?, 'https://media.example.test/a.mp3', 120),
		('track_f091_failure_b', 'lib_f091_failure', 'track', 'B', 'B', ?, 'https://media.example.test/b.mp3', 120)`, now, now); err != nil {
		t.Fatal(err)
	}
	seedExactPlaybackFactsForFixture(t, server, "track_f091_failure_a")
	seedExactPlaybackFactsForFixture(t, server, "track_f091_failure_b")

	if _, err := server.db.Exec(`CREATE TEMP TRIGGER fail_f091_continuity BEFORE INSERT ON playback_session_continuation_credentials BEGIN SELECT RAISE(FAIL, 'forced continuity failure'); END`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil)
	if _, startErr := server.startPlaybackForRequest(request, user, PlaybackSessionCreateRequest{MediaID: "track_f091_failure_a", SkipPreroll: true}); startErr == nil || startErr.code != "playback_continuity_failed" {
		t.Fatalf("continuity failure result=%#v", startErr)
	}
	if _, err := server.db.Exec(`DROP TRIGGER fail_f091_continuity`); err != nil {
		t.Fatal(err)
	}
	assertNoPlaybackStartResidueForMedia(t, server, "track_f091_failure_a")

	source := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID: "track_f091_failure_a", QueueMediaIDs: []string{"track_f091_failure_b"},
		ClientInstanceID: "f091-failure-client", SkipPreroll: true,
	})
	if _, err := server.db.Exec(`CREATE TEMP TRIGGER fail_f091_history BEFORE INSERT ON playback_session_history BEGIN SELECT RAISE(FAIL, 'forced history failure'); END`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.playbackSessionQueueSnapshot(t.Context(), user, source.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil)
	_, startErr := server.startPlaybackForRequest(request, user, PlaybackSessionCreateRequest{
		MediaID: snapshot.Queue[0].MediaID, ClientInstanceID: "f091-failure-client", SkipPreroll: true,
		currentEntryID: snapshot.Queue[0].EntryID, queueOwned: true,
		historyOccurrences: []playbackQueueOccurrence{snapshot.Current},
		deferReplacement:   true, reservedSessionID: randomID("failed-history-successor"),
	})
	if startErr == nil || startErr.code != "playback_history_failed" {
		t.Fatalf("history failure result=%#v", startErr)
	}
	var active, replacementResidue int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id=? AND ended_at='' AND state<>'stopped'`, source.SessionID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE profile_id=? AND id<>?`, viewerProfileID(user), source.SessionID).Scan(&replacementResidue); err != nil {
		t.Fatal(err)
	}
	if active != 1 || replacementResidue != 0 {
		t.Fatalf("failed history handoff changed authority active=%d residue=%d", active, replacementResidue)
	}
}

func assertNoPlaybackStartResidueForMedia(t *testing.T, server *Server, mediaID string) {
	t.Helper()
	for table, query := range map[string]string{
		"sessions":      `SELECT COUNT(*) FROM playback_sessions WHERE media_id=?`,
		"grants":        `SELECT COUNT(*) FROM playback_media_grants WHERE resource_id=?`,
		"queue":         `SELECT COUNT(*) FROM playback_session_queue WHERE media_id=?`,
		"continuations": `SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE playback_session_id IN (SELECT id FROM playback_sessions WHERE media_id=?)`,
	} {
		var count int
		if err := server.db.QueryRow(query, mediaID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed start %s residue=%d err=%v", table, count, err)
		}
	}
}

func int64Ptr(value int64) *int64 { return &value }
