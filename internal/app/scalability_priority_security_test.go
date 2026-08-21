package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestSettingsReadsObserveAuthoritativeOutOfBandMutation(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at) VALUES ('cache_isolation_probe', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, `{"revision":1}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	first, err := server.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(first["cache_isolation_probe"]); string(encoded) != `{"revision":1}` {
		t.Fatalf("first authoritative settings read = %s", encoded)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ?, updated_at = ? WHERE key = 'cache_isolation_probe'`, `{"revision":2}`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	second, err := server.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(second["cache_isolation_probe"]); string(encoded) != `{"revision":2}` {
		t.Fatalf("settings read leaked stale process state after authoritative mutation: %s", encoded)
	}
}

func TestVerifiedMediaGrantContinuesOnlyAcrossBoundedTransientSQLiteFailure(t *testing.T) {
	serverURL, db, server := newEmptyAuthTestServerWithInstance(t)
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/setup", map[string]any{
		"serverName": "Continuity Test Server",
		"username":   "continuity-owner", "email": "continuity@example.test", "displayName": "Continuity Owner",
		"password": "Correct horse battery staple1", "setupMode": "local_only", "localOnlyAcknowledged": true,
	}, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("setup owner status=%d body=%s", status, body)
	}
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'continuity-owner'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO media_items (id,type,title,sort_title,genres_json,tags_json,labels_json,added_at) VALUES ('continuity_media','movie','Continuity','Continuity','[]','[]','[]',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,media_id,media_type,title,started_at,last_seen_at,state) VALUES ('continuity_play',?,?,'continuity_media','movie','Continuity',?,?,'playing')`, userID, userID, now, now); err != nil {
		t.Fatal(err)
	}
	bindPlaybackSessionPlanForTest(t, db, "continuity_play", "continuity_media", false)
	user := User{ID: userID, AccountID: userID, ProfileID: userID, Permissions: map[string]bool{"playMedia": true}}
	grant, err := server.issueMediaGrant(context.Background(), user, "continuity_play", "media", "continuity_media")
	if err != nil {
		t.Fatal(err)
	}
	request := mediaGrantRequest(http.MethodGet, "/api/media/continuity_media/hls/segment?name=segment_00000.ts", grant.Token)
	if _, err := server.userForMediaGrant(request); err != nil {
		t.Fatalf("establish grant: %v", err)
	}

	probes := 0
	server.mediaGrantTerminalProbe = func(context.Context, string, mediaGrantScope) (mediaGrantTerminalState, error) {
		probes++
		return mediaGrantTerminalState{}, errors.New("SQLITE_BUSY: database is locked")
	}
	if _, err := server.userForMediaGrant(request); err != nil {
		t.Fatalf("hot cached segment failed: %v", err)
	}
	if probes != 0 {
		t.Fatalf("hot cached segment performed %d terminal probes, expected zero", probes)
	}
	server.mediaGrantCacheMu.Lock()
	entry := server.mediaGrantCache[hashToken(grant.Token)]
	entry.verifiedAt = time.Now().UTC().Add(-mediaGrantVerifyInterval - time.Millisecond)
	server.mediaGrantCache[hashToken(grant.Token)] = entry
	server.mediaGrantCacheMu.Unlock()
	if resolved, err := server.userForMediaGrant(request); err != nil || resolved.ID != userID {
		t.Fatalf("verified playback did not survive transient busy: user=%#v err=%v", resolved, err)
	}
	metrics := server.mediaGrantCacheMetricsSnapshot()
	if metrics.BusyFallbacks != 1 || metrics.Entries != 1 || metrics.Entries > metrics.Capacity {
		t.Fatalf("unexpected bounded grant metrics: %#v", metrics)
	}

	server.mediaGrantCacheMu.Lock()
	entry = server.mediaGrantCache[hashToken(grant.Token)]
	entry.verifiedAt = time.Now().UTC().Add(-mediaGrantBusyFallbackTTL - time.Second)
	server.mediaGrantCache[hashToken(grant.Token)] = entry
	server.mediaGrantCacheMu.Unlock()
	if _, err := server.userForMediaGrant(request); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("stale busy fallback unexpectedly authorized: %v", err)
	}
}

func TestVerifiedMediaGrantDeniesImmediatelyAfterObservedRevocation(t *testing.T) {
	serverURL, db, server := newEmptyAuthTestServerWithInstance(t)
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/setup", map[string]any{
		"serverName": "Revocation Test Server",
		"username":   "revoke-owner", "email": "revoke@example.test", "displayName": "Revoke Owner",
		"password": "Correct horse battery staple1", "setupMode": "local_only", "localOnlyAcknowledged": true,
	}, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("setup owner status=%d body=%s", status, body)
	}
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'revoke-owner'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO media_items (id,type,title,sort_title,genres_json,tags_json,labels_json,added_at) VALUES ('revoke_media','movie','Revoke','Revoke','[]','[]','[]',?)`, now)
	_, _ = db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,media_id,media_type,title,started_at,last_seen_at,state) VALUES ('revoke_play',?,?,'revoke_media','movie','Revoke',?,?,'playing')`, userID, userID, now, now)
	bindPlaybackSessionPlanForTest(t, db, "revoke_play", "revoke_media", false)
	grant, err := server.issueMediaGrant(context.Background(), User{ID: userID, AccountID: userID, ProfileID: userID, Permissions: map[string]bool{"playMedia": true}}, "revoke_play", "media", "revoke_media")
	if err != nil {
		t.Fatal(err)
	}
	request := mediaGrantRequest(http.MethodGet, "/api/media/revoke_media/stream", grant.Token)
	if _, err := server.userForMediaGrant(request); err != nil {
		t.Fatalf("establish grant: %v", err)
	}
	server.mediaGrantCacheMu.Lock()
	entry := server.mediaGrantCache[hashToken(grant.Token)]
	entry.verifiedAt = time.Now().UTC().Add(-mediaGrantVerifyInterval - time.Millisecond)
	server.mediaGrantCache[hashToken(grant.Token)] = entry
	server.mediaGrantCacheMu.Unlock()
	if _, err := db.Exec(`UPDATE playback_media_grants SET revoked_at = ? WHERE token_hash = ?`, time.Now().UTC().Format(time.RFC3339), hashToken(grant.Token)); err != nil {
		t.Fatal(err)
	}
	if _, err := server.userForMediaGrant(request); !errorsIsMediaGrantDenied(err) {
		t.Fatalf("observed revocation did not deny cached grant: %v", err)
	}
	if metrics := server.mediaGrantCacheMetricsSnapshot(); metrics.TerminalDenials != 1 || metrics.Entries != 0 {
		t.Fatalf("revoked snapshot was not terminally evicted: %#v", metrics)
	}
}

func TestSQLiteWriteSchedulerStrictPriority(t *testing.T) {
	var scheduler sqliteWriteScheduler
	releaseActive, err := scheduler.acquire(context.Background(), sqliteWriteBackground)
	if err != nil {
		t.Fatal(err)
	}
	order := make(chan sqliteWritePriority, 3)
	start := func(priority sqliteWritePriority) {
		go func() {
			release, acquireErr := scheduler.acquire(context.Background(), priority)
			if acquireErr != nil {
				return
			}
			order <- priority
			release()
		}()
	}
	start(sqliteWriteBackground)
	start(sqliteWriteInteractive)
	start(sqliteWritePlayback)
	deadline := time.Now().Add(time.Second)
	for {
		playback, interactive := scheduler.pressure()
		scheduler.mu.Lock()
		background := len(scheduler.waitQueue[sqliteWriteBackground])
		scheduler.mu.Unlock()
		if playback == 1 && interactive == 1 && background == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writers did not enter scheduler queue")
		}
		time.Sleep(time.Millisecond)
	}
	releaseActive()
	for _, expected := range []sqliteWritePriority{sqliteWritePlayback, sqliteWriteInteractive, sqliteWriteBackground} {
		select {
		case actual := <-order:
			if actual != expected {
				t.Fatalf("write order=%v, expected %v", actual, expected)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for prioritized writer")
		}
	}
}

func TestSQLiteWriteAdmissionReleasedBeforeInvalidationCallbacks(t *testing.T) {
	server := newScannerTestServer(t)
	done := make(chan error, 1)
	go func() {
		_, err := server.execUserWriteTagged(context.Background(), []string{"settings", "home"}, `
			INSERT INTO settings (key, value_json, updated_at) VALUES ('scheduler_deadlock_probe', '{}', ?)
			ON CONFLICT(key) DO UPDATE SET updated_at = excluded.updated_at`, time.Now().UTC().Format(time.RFC3339))
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prioritized write failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prioritized write retained admission across invalidation callback")
	}
}
