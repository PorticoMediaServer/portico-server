package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPrivacyRetentionPreservesPlaybackByDefaultAndOwnerCanConfigureCleanup(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	expired := now.Add(-25 * time.Hour).Format(time.RFC3339)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("load user: %v", err)
	}

	for _, row := range []struct{ id, endedAt, address string }{
		{"old-ended", old, "203.0.113.10"},
		{"recent-ended", recent, "203.0.113.11"},
		{"active", "", "203.0.113.12"},
	} {
		if _, err := db.Exec(`INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, client_ip) VALUES (?, ?, ?, 'movie-test', 'movie', 'Test', ?, ?, ?, ?)`, row.id, userID, userID, old, recent, row.endedAt, row.address); err != nil {
			t.Fatalf("insert playback %s: %v", row.id, err)
		}
	}

	for _, table := range []string{"quick_connect_requests", "tv_setup_sessions", "portico_login_requests"} {
		insertTransientPrivacyRows(t, db, table, userID, expired, recent)
	}

	if _, err := db.Exec(`INSERT INTO devices (id, user_id, name, client_ip, created_at, last_seen_at) VALUES ('stale-device', ?, 'Stale', '203.0.113.20', ?, ?)`, userID, old, old); err != nil {
		t.Fatalf("insert stale device: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO devices (id, user_id, name, client_ip, created_at, last_seen_at) VALUES ('active-device', ?, 'Active', '203.0.113.21', ?, ?)`, userID, old, old); err != nil {
		t.Fatalf("insert active device: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at) VALUES ('active-session', ?, ?, 'active-device', 'privacy-token', ?, ?, ?)`, userID, userID, now.Add(time.Hour).Format(time.RFC3339), recent, recent); err != nil {
		t.Fatalf("insert active auth session: %v", err)
	}

	if err := server.prunePrivacySensitiveOperationalData(context.Background(), now); err != nil {
		t.Fatalf("default privacy retention: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM playback_sessions WHERE id = 'old-ended'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM dashboard_playback_rollups WHERE session_id = 'old-ended'`, 0)
	assertStringValue(t, db, `SELECT client_ip FROM devices WHERE id = 'stale-device'`, "")
	assertStringValue(t, db, `SELECT client_ip FROM playback_sessions WHERE id = 'active'`, "203.0.113.12")
	for _, table := range []string{"quick_connect_requests", "tv_setup_sessions", "portico_login_requests"} {
		assertCount(t, db, `SELECT COUNT(*) FROM `+table+` WHERE id LIKE 'old-%'`, 1)
	}

	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('retention', '{"playbackDetailDays":30,"playbackHistoryDays":365,"auditHistoryDays":365,"diagnosticHistoryDays":30,"authRequestDays":1,"deviceIPDays":1}', ?)`, recent); err != nil {
		t.Fatalf("configure retention: %v", err)
	}
	if err := server.prunePrivacySensitiveOperationalData(context.Background(), now); err != nil {
		t.Fatalf("configured privacy retention: %v", err)
	}

	assertStringValue(t, db, `SELECT client_ip FROM playback_sessions WHERE id = 'active'`, "203.0.113.12")
	assertStringValue(t, db, `SELECT client_ip FROM playback_sessions WHERE id = 'recent-ended'`, "203.0.113.11")
	assertCount(t, db, `SELECT COUNT(*) FROM playback_sessions WHERE id = 'old-ended'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM dashboard_playback_rollups WHERE session_id = 'old-ended'`, 1)
	assertStringValue(t, db, `SELECT client_ip FROM devices WHERE id = 'stale-device'`, "")
	assertStringValue(t, db, `SELECT client_ip FROM devices WHERE id = 'active-device'`, "203.0.113.21")
	for _, table := range []string{"quick_connect_requests", "tv_setup_sessions", "portico_login_requests"} {
		assertCount(t, db, `SELECT COUNT(*) FROM `+table+` WHERE id LIKE 'old-%'`, 0)
		assertCount(t, db, `SELECT COUNT(*) FROM `+table+` WHERE id LIKE 'recent-%'`, 1)
	}
}

func TestNetworkSignatureFingerprintIsKeyedAndDoesNotExposeNetworkData(t *testing.T) {
	parts := []string{"en0|aa:bb:cc:dd:ee:ff|192.168.1.10/24"}
	first := networkSignatureFingerprint([]byte("server-secret-a"), parts)
	second := networkSignatureFingerprint([]byte("server-secret-a"), parts)
	otherKey := networkSignatureFingerprint([]byte("server-secret-b"), parts)
	if first != second || first == otherKey || !strings.HasPrefix(first, "hmac-sha256:") {
		t.Fatalf("unexpected keyed fingerprint behavior: %q %q %q", first, second, otherKey)
	}
	for _, raw := range []string{"en0", "aa:bb:cc:dd:ee:ff", "192.168.1.10"} {
		if strings.Contains(first, raw) {
			t.Fatalf("fingerprint leaked %q: %s", raw, first)
		}
	}
}

func TestRetentionSettingsExposeBoundedOperationalDefaultsAndAcceptIndependentPeriods(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	group, ok := server.clientSettings(map[string]any{})["retention"].(map[string]any)
	if !ok {
		t.Fatal("retention defaults were not exposed")
	}
	expected := map[string]any{
		"playbackDetailDays":          0,
		"playbackHistoryDays":         0,
		"auditHistoryDays":            90,
		"diagnosticHistoryDays":       30,
		"clientDiagnosticHistoryDays": 30,
		"jobHistoryDays":              30,
		"authRequestDays":             14,
		"deviceIPDays":                30,
	}
	for field, want := range expected {
		if got, ok := group[field].(float64); !ok || got != float64(want.(int)) {
			t.Fatalf("default %s = %#v, expected %#v", field, group[field], want)
		}
	}
	raw := json.RawMessage(`{"playbackDetailDays":30,"playbackHistoryDays":365,"auditHistoryDays":730,"diagnosticHistoryDays":14,"authRequestDays":2,"deviceIPDays":7}`)
	if _, err := normalizeWritableSettingGroup("retention", raw); err != nil {
		t.Fatalf("normalize retention settings: %v", err)
	}
}

func TestOperationalRetentionUsesInjectedClockForAllR3Lanes(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	server.retentionClock = func() time.Time { return now }
	oldAudit := now.AddDate(0, 0, -91).Format(time.RFC3339Nano)
	recentAudit := now.AddDate(0, 0, -89).Format(time.RFC3339Nano)
	oldDiagnostic := now.AddDate(0, 0, -31).Format(time.RFC3339Nano)
	recentDiagnostic := now.AddDate(0, 0, -29).Format(time.RFC3339Nano)
	oldClient := now.AddDate(0, 0, -31).Format(time.RFC3339Nano)
	recentClient := now.AddDate(0, 0, -29).Format(time.RFC3339Nano)
	oldJob := now.AddDate(0, 0, -31).Format(time.RFC3339Nano)
	recentJob := now.AddDate(0, 0, -29).Format(time.RFC3339Nano)

	if _, err := db.Exec(`
		INSERT INTO audit_events (id, action, resource_type, resource_id, severity, metadata_json, created_at)
		VALUES ('r3-old-audit', 'test', 'test', 'old', 'info', '{}', ?),
		       ('r3-recent-audit', 'test', 'test', 'recent', 'info', '{}', ?)`, oldAudit, recentAudit); err != nil {
		t.Fatalf("insert audit retention fixtures: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO security_audit_events (id, previous_hash, event_hash, action, metadata_json, byte_size, created_at)
		VALUES ('r3-old-security', '', 'old-hash', 'test', '{}', 1, ?),
		       ('r3-recent-security', 'old-hash', 'recent-hash', 'test', '{}', 1, ?)`, oldAudit, recentAudit); err != nil {
		t.Fatalf("insert security retention fixtures: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO server_diagnostic_events (id, level, message, fields_json, byte_size, created_at)
		VALUES ('r3-old-server', 'info', 'old', '{}', 1, ?),
		       ('r3-recent-server', 'info', 'recent', '{}', 1, ?)`, oldDiagnostic, recentDiagnostic); err != nil {
		t.Fatalf("insert server diagnostic retention fixtures: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO client_diagnostic_events (id, level, message, fields_json, byte_size, created_at)
		VALUES ('r3-old-client', 'info', 'old', '{}', 1, ?),
		       ('r3-recent-client', 'info', 'recent', '{}', 1, ?)`, oldClient, recentClient); err != nil {
		t.Fatalf("insert client diagnostic retention fixtures: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES ('r3-old-job', 'database_backup', 'failed', 100, 'old', 'database', 'old', ?, ?),
		       ('r3-recent-job', 'database_backup', 'failed', 100, 'recent', 'database', 'recent', ?, ?)`, oldJob, oldJob, recentJob, recentJob); err != nil {
		t.Fatalf("insert job retention fixtures: %v", err)
	}

	if err := server.pruneOperationalTables(context.Background()); err != nil {
		t.Fatalf("prune injected retention fixtures: %v", err)
	}
	for _, fixture := range []struct {
		table string
		id    string
	}{
		{"audit_events", "r3-old-audit"},
		{"security_audit_events", "r3-old-security"},
		{"server_diagnostic_events", "r3-old-server"},
		{"client_diagnostic_events", "r3-old-client"},
		{"jobs", "r3-old-job"},
	} {
		assertCount(t, db, `SELECT COUNT(*) FROM `+fixture.table+` WHERE id = '`+fixture.id+`'`, 0)
	}
	for _, fixture := range []struct {
		table string
		id    string
	}{
		{"audit_events", "r3-recent-audit"},
		{"security_audit_events", "r3-recent-security"},
		{"server_diagnostic_events", "r3-recent-server"},
		{"client_diagnostic_events", "r3-recent-client"},
		{"jobs", "r3-recent-job"},
	} {
		assertCount(t, db, `SELECT COUNT(*) FROM `+fixture.table+` WHERE id = '`+fixture.id+`'`, 1)
	}
}

func TestOperationalAuditAndDiagnosticHistoryBoundedByDefault(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	old := time.Now().UTC().AddDate(-5, 0, 0).Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO audit_events (id, action, resource_type, resource_id, severity, metadata_json, created_at)
		VALUES ('owner-audit-history', 'owner.test', 'test', 'history', 'info', '{}', ?)`, old); err != nil {
		t.Fatalf("insert audit history: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES ('owner-diagnostic-history', 'owner_test', 'complete', 100, 'History', 'test', 'history', ?, ?)`, old, old); err != nil {
		t.Fatalf("insert diagnostic history: %v", err)
	}
	if err := server.pruneOperationalTables(context.Background()); err != nil {
		t.Fatalf("default operational retention: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM audit_events WHERE id = 'owner-audit-history'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM jobs WHERE id = 'owner-diagnostic-history'`, 0)
}

func TestPrivacyRetentionExplicitForeverPreservesOwnerData(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	server.retentionClock = func() time.Time { return now }
	var userID string
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("load user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('retention', ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, `{"playbackDetailDays":0,"playbackHistoryDays":0,"auditHistoryDays":0,"diagnosticHistoryDays":0,"clientDiagnosticHistoryDays":0,"jobHistoryDays":0,"authRequestDays":0,"deviceIPDays":0}`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("configure forever retention: %v", err)
	}
	old := now.AddDate(0, 0, -365).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, client_ip) VALUES ('forever-playback', ?, ?, 'movie-test', 'movie', 'Test', ?, ?, ?, '203.0.113.30')`, userID, userID, old, old, old); err != nil {
		t.Fatalf("insert forever playback: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO audit_events (id, action, resource_type, resource_id, severity, metadata_json, created_at) VALUES ('forever-audit', 'test', 'test', 'forever', 'info', '{}', ?)`, old); err != nil {
		t.Fatalf("insert forever audit: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO server_diagnostic_events (id, level, message, fields_json, byte_size, created_at) VALUES ('forever-server-diagnostic', 'info', 'forever', '{}', 1, ?)`, old); err != nil {
		t.Fatalf("insert forever server diagnostic: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at) VALUES ('forever-job', 'forever_test', 'complete', 100, 'Forever', 'test', 'forever', ?, ?)`, old, old); err != nil {
		t.Fatalf("insert forever job: %v", err)
	}
	insertTransientPrivacyRows(t, db, "quick_connect_requests", userID, old, old)
	if _, err := db.Exec(`INSERT INTO devices (id, user_id, name, client_ip, created_at, last_seen_at) VALUES ('forever-device', ?, 'Forever', '203.0.113.31', ?, ?)`, userID, old, old); err != nil {
		t.Fatalf("insert forever device: %v", err)
	}

	if err := server.pruneOperationalTables(context.Background()); err != nil {
		t.Fatalf("explicit forever retention: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM playback_sessions WHERE id = 'forever-playback'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM audit_events WHERE id = 'forever-audit'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM server_diagnostic_events WHERE id = 'forever-server-diagnostic'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM jobs WHERE id = 'forever-job'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM quick_connect_requests WHERE id = 'old-quick'`, 1)
	assertStringValue(t, db, `SELECT client_ip FROM playback_sessions WHERE id = 'forever-playback'`, "203.0.113.30")
	assertStringValue(t, db, `SELECT client_ip FROM devices WHERE id = 'forever-device'`, "203.0.113.31")
}

func TestPlaybackHistoryRetentionPreventsOldDetailFromRepopulatingRollups(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("load user: %v", err)
	}
	now := time.Now().UTC()
	old := now.AddDate(-1, 0, 0).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at) VALUES ('retained-detail-only', ?, ?, 'movie-test', 'movie', 'Test', ?, ?, ?)`, userID, userID, old, old, old); err != nil {
		t.Fatalf("insert old detail: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('retention', '{"playbackDetailDays":0,"playbackHistoryDays":30}', ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("configure playback history retention: %v", err)
	}
	if _, err := server.refreshDashboardPlaybackRollupsContext(context.Background(), time.Time{}, 100); err != nil {
		t.Fatalf("refresh rollups: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM playback_sessions WHERE id = 'retained-detail-only'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM dashboard_playback_rollups WHERE session_id = 'retained-detail-only'`, 0)
}

func TestEndingPlaybackSessionRetainsClientAddressByDefault(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("load user: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, client_ip) VALUES ('end-address-test', ?, ?, 'movie-test', 'movie', 'Test', ?, ?, '203.0.113.40')`, userID, userID, now, now); err != nil {
		t.Fatalf("insert playback: %v", err)
	}
	if err := server.endPlaybackSession(User{ID: userID}, "end-address-test"); err != nil {
		t.Fatalf("end playback: %v", err)
	}
	assertStringValue(t, db, `SELECT client_ip FROM playback_sessions WHERE id = 'end-address-test'`, "203.0.113.40")
}

// Kept explicit because the three handshake tables intentionally have
// different payload schemas even though they share one retention policy.
func insertTransientPrivacyRows(t *testing.T, db *sql.DB, table, userID, old, recent string) {
	t.Helper()
	_ = userID
	var err error
	switch table {
	case "quick_connect_requests":
		_, err = db.Exec(`
			INSERT INTO quick_connect_requests (id, code, secret_hash, status, expires_at, created_at, updated_at)
			VALUES ('old-quick', '100001', 'old-quick-secret', 'consumed', ?, ?, ?),
			       ('recent-quick', '100002', 'recent-quick-secret', 'consumed', ?, ?, ?)`, old, old, old, recent, recent, recent)
	case "tv_setup_sessions":
		_, err = db.Exec(`
			INSERT INTO tv_setup_sessions (id, code, status, device_public_key, expires_at, created_at, updated_at)
			VALUES ('old-tv', '200001', 'redeemed', 'old-key', ?, ?, ?),
			       ('recent-tv', '200002', 'redeemed', 'recent-key', ?, ?, ?)`, old, old, old, recent, recent, recent)
	case "portico_login_requests":
		_, err = db.Exec(`
			INSERT INTO portico_login_requests (id, state_hash, status, return_url, expires_at, created_at, updated_at)
			VALUES ('old-login', 'old-state', 'consumed', '/', ?, ?, ?),
			       ('recent-login', 'recent-state', 'consumed', '/', ?, ?, ?)`, old, old, old, recent, recent, recent)
	default:
		t.Fatalf("unsupported transient table %q", table)
	}
	if err != nil {
		t.Fatalf("insert %s privacy rows: %v", table, err)
	}
}

func assertStringValue(t *testing.T, db *sql.DB, query, expected string) {
	t.Helper()
	var actual string
	if err := db.QueryRow(query).Scan(&actual); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if actual != expected {
		t.Fatalf("query %q = %q, expected %q", query, actual, expected)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, expected int) {
	t.Helper()
	var actual int
	if err := db.QueryRow(query).Scan(&actual); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if actual != expected {
		t.Fatalf("query %q = %d, expected %d", query, actual, expected)
	}
}
