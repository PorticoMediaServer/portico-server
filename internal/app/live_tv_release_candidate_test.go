package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestConsumerLiveTVGuideNeverSerializesProviderConfiguration(t *testing.T) {
	guide := LiveTVGuideResponse{Source: LiveTVSource{
		ID: "source-private", Name: "Living Room Tuner", Type: "xtream", Enabled: true,
		M3UURL: "https://provider.example/playlist?token=secret", HasM3UText: true,
		EPGURL: "https://provider.example/guide?token=secret", HasEPGText: true,
		XtreamBaseURL: "https://provider.example", XtreamUsername: "private-user", HasXtreamPassword: true,
		HDHomeRunBaseURL: "http://192.168.1.50",
	}}
	encoded, err := json.Marshal(guide)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	for _, forbidden := range []string{"provider.example", "private-user", "192.168.1.50", "m3uUrl", "hasM3uText", "xtreamUsername", "hasXtreamPassword"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("consumer guide exposed %q: %s", forbidden, value)
		}
	}
	for _, required := range []string{`"id":"source-private"`, `"name":"Living Room Tuner"`, `"type":"xtream"`} {
		if !strings.Contains(value, required) {
			t.Fatalf("consumer guide omitted safe descriptor %s: %s", required, value)
		}
	}
}

func TestOrdinaryViewerListsSafeLiveTVSourceSummaries(t *testing.T) {
	server := newScannerTestServer(t)
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,enabled,m3u_url,epg_url,user_agent,tuner_count,last_error,created_at,updated_at) VALUES ('source_viewer_safe','Viewer Tuner','m3u',1,'https://provider.test/list?secret=1','https://provider.test/epg?secret=2','private-agent',4,'provider raw failure',?,?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	viewer := User{ID: "viewer_safe", AccountID: "viewer_safe", ProfileID: "viewer_safe", Role: "user", Permissions: map[string]bool{"viewLiveTV": true}}
	recorder := httptest.NewRecorder()
	server.handleLiveTV(recorder, httptest.NewRequest(http.MethodGet, "/api/live-tv", nil), viewer)
	if recorder.Code != http.StatusOK {
		t.Fatalf("viewer source list=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"provider.test", "private-agent", "provider raw failure", "m3uUrl", "epgUrl", "tunerCount", "lastError"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("viewer source summary exposed %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{`"id":"source_viewer_safe"`, `"name":"Viewer Tuner"`, `"type":"m3u"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("viewer source summary omitted %s: %s", required, body)
		}
	}
	adminRecorder := httptest.NewRecorder()
	server.handleLiveTVSourceRoute(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/live-tv/sources", nil), viewer, nil)
	if adminRecorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary viewer admin source list=%d body=%s", adminRecorder.Code, adminRecorder.Body.String())
	}
	dvrManager := viewer
	dvrManager.Permissions = map[string]bool{"viewLiveTV": true, "manageDVR": true}
	dvrAdminRecorder := httptest.NewRecorder()
	server.handleLiveTVSourceRoute(dvrAdminRecorder, httptest.NewRequest(http.MethodGet, "/api/live-tv/sources", nil), dvrManager, nil)
	if dvrAdminRecorder.Code != http.StatusForbidden {
		t.Fatalf("manageDVR disclosed source administration: status=%d body=%s", dvrAdminRecorder.Code, dvrAdminRecorder.Body.String())
	}
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/live-tv/sources", nil)
	if apiKeyAllowsRequest(User{AuthProvider: "api_key", APIKeyScopes: []string{"manageDVR"}}, apiRequest) {
		t.Fatal("manageDVR API key authorized source administration")
	}
	for _, scopes := range [][]string{{"read"}, {"all"}, {"manageServer"}} {
		apiUser := User{Role: "owner", AuthProvider: "api_key", APIKeyID: "key_source_admin", APIKeyScopes: scopes, Permissions: map[string]bool{"manageServer": true}}
		if canManageLiveTVSources(apiUser) {
			t.Fatalf("API key scopes %v authorized interactive source administration", scopes)
		}
		recorder := httptest.NewRecorder()
		server.handleLiveTVSourceRoute(recorder, apiRequest.Clone(apiRequest.Context()), apiUser, nil)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("API key scopes %v source list=%d body=%s", scopes, recorder.Code, recorder.Body.String())
		}
	}
}

func TestLiveTVSourceAdministrationRequiresInteractiveOwnerAndManageServer(t *testing.T) {
	server := newScannerTestServer(t)
	cases := []struct {
		name string
		user User
	}{
		{name: "non owner manager", user: User{Role: "user", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true}}},
		{name: "owner read api key", user: User{Role: "owner", AuthProvider: "api_key", APIKeyID: "key_read", APIKeyScopes: []string{"read"}, Permissions: map[string]bool{"manageServer": true}}},
		{name: "owner api key without hydrated id", user: User{Role: "owner", AuthProvider: "api_key", APIKeyScopes: []string{"all"}, Permissions: map[string]bool{"manageServer": true}}},
		{name: "owner all api key", user: User{Role: "owner", AuthProvider: "api_key", APIKeyID: "key_all", APIKeyScopes: []string{"all"}, Permissions: map[string]bool{"manageServer": true}}},
		{name: "owner manage server api key", user: User{Role: "owner", AuthProvider: "api_key", APIKeyID: "key_manage", APIKeyScopes: []string{"manageServer"}, Permissions: map[string]bool{"manageServer": true}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for _, request := range []struct {
				method string
				path   string
				parts  []string
			}{
				{http.MethodGet, "/api/live-tv/sources", nil},
				{http.MethodPost, "/api/live-tv/sources", nil},
				{http.MethodPost, "/api/live-tv/sources/test-add", []string{"test-add"}},
				{http.MethodPost, "/api/live-tv/sources/hdhomerun/discover", []string{"hdhomerun", "discover"}},
				{http.MethodPatch, "/api/live-tv/sources/source-private", []string{"source-private"}},
				{http.MethodDelete, "/api/live-tv/sources/source-private", []string{"source-private"}},
				{http.MethodPost, "/api/live-tv/sources/source-private/refresh", []string{"source-private", "refresh"}},
			} {
				recorder := httptest.NewRecorder()
				server.handleLiveTVSourceRoute(recorder, httptest.NewRequest(request.method, request.path, strings.NewReader(`{}`)), test.user, request.parts)
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s %s=%d body=%s", request.method, request.path, recorder.Code, recorder.Body.String())
				}
			}
			dvrRecorder := httptest.NewRecorder()
			server.handleAdminDVROperationalStatus(dvrRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/dvr/status", nil), test.user)
			if dvrRecorder.Code != http.StatusForbidden {
				t.Fatalf("admin DVR operational status=%d body=%s", dvrRecorder.Code, dvrRecorder.Body.String())
			}
			libraryRecorder := httptest.NewRecorder()
			server.handleAdminLibraryChannels(libraryRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/library-channels", nil), test.user)
			if libraryRecorder.Code != http.StatusForbidden {
				t.Fatalf("admin Library Channels status=%d body=%s", libraryRecorder.Code, libraryRecorder.Body.String())
			}
		})
	}

	owner := User{ID: "owner", AccountID: "owner", ProfileID: "owner", ProfileIsPrimary: true, Role: "owner", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true}}
	if !canManageLiveTVSources(owner) {
		t.Fatal("interactive owner with manageServer could not manage Live TV sources")
	}
	owner.Permissions = map[string]bool{"viewLiveTV": true}
	if !canManageLiveTVSources(owner) {
		t.Fatal("interactive owner authority depended on a mutable permission bit")
	}
}

func TestDVRRecordingJSONUsesStableFailureWithoutPathOrRawError(t *testing.T) {
	recording := DVRRecording{
		ID: "recording-private", ProfileID: "profile-private", SourceID: "source-private", Title: "Evening News",
		Status: "failed", Priority: 75, FailureCode: "source_unavailable", FailureMessageID: "dvr.recording-failed",
		Path: "/private/dvr/Evening News.ts", Error: "provider password secret failed", Revision: 4,
		StartsAt: time.Now().UTC().Format(time.RFC3339), EndsAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
	encoded, err := json.Marshal(recording)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	for _, forbidden := range []string{"/private/dvr", "provider password", `"path"`, `"error"`} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("DVR response leaked %q: %s", forbidden, value)
		}
	}
	for _, required := range []string{`"failureCode":"source_unavailable"`, `"failureMessageId":"dvr.recording-failed"`, `"priority":75`, `"revision":4`} {
		if !strings.Contains(value, required) {
			t.Fatalf("DVR response omitted %s: %s", required, value)
		}
	}
}

func TestOrdinaryProfilesManageOnlyTheirOwnDVRRulesAndReceiveRedactedStatus(t *testing.T) {
	server := newScannerTestServer(t)
	insertReleaseCandidateLiveSource(t, server, "source_own_rules", "channel_own_rules", 2)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, identity := range []struct{ id, profile string }{{"user_rule_a", "user_rule_a"}, {"user_rule_b", "user_rule_b"}} {
		if _, err := server.db.Exec(`INSERT INTO users (id,username,email,display_name,password_hash,role,permissions_json,preferences_json,created_at,updated_at) VALUES (?,?,?,?,'hash','user','{}','{}',?,?)`, identity.id, identity.id, identity.id+"@example.test", identity.id, now, now); err != nil {
			t.Fatalf("insert %s: %v", identity.id, err)
		}
		if _, err := server.db.Exec(`INSERT INTO profiles (id,account_id,display_name,role,permissions_json,preferences_json,is_primary,restrictions_json,created_at,updated_at) VALUES (?,?,?,'user','{}','{}',1,'{}',?,?) ON CONFLICT(id) DO NOTHING`, identity.profile, identity.id, identity.id, now, now); err != nil {
			t.Fatalf("insert profile %s: %v", identity.profile, err)
		}
	}
	permissions := map[string]bool{"viewLiveTV": true, "playLiveTV": true, "viewDVR": true, "scheduleDVR": true, "playMedia": true}
	userA := User{ID: "user_rule_a", AccountID: "user_rule_a", ProfileID: "user_rule_a", Role: "user", Permissions: permissions}
	userB := User{ID: "user_rule_b", AccountID: "user_rule_b", ProfileID: "user_rule_b", Role: "user", Permissions: permissions}
	rule, err := server.createDVRRule(userA, DVRRecordingRuleRequest{SourceID: "source_own_rules", ChannelID: "channel_own_rules", Title: "Own series", MatchType: "series", Enabled: true})
	if err != nil {
		t.Fatalf("ordinary profile create own rule: %v", err)
	}
	if !userCanModifyDVRRule(userA, rule) || userCanModifyDVRRule(userB, rule) {
		t.Fatalf("own-rule authorization A=%v B=%v", userCanModifyDVRRule(userA, rule), userCanModifyDVRRule(userB, rule))
	}
	if _, err := server.getDVRRuleForUser(userB, rule.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("other profile read error=%v", err)
	}
	rules, err := server.listDVRRulesForUser(userB)
	if err != nil || len(rules) != 0 {
		t.Fatalf("other profile rules=%#v err=%v", rules, err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/dvr/status", nil)
	recorder := httptest.NewRecorder()
	server.handleDVROperationalStatus(recorder, request, userA, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("consumer DVR status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var status DVRConsumerStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Capabilities.CanCreateOwnRules || !status.Capabilities.CanEditOwnRules || !status.Capabilities.CanDeleteOwnRules || status.Capabilities.CanManageRecordingRules || status.Capabilities.CanManageAllRules {
		t.Fatalf("ordinary DVR capabilities=%#v", status.Capabilities)
	}
	if status.Storage.UsedBytes < 0 || status.Storage.AvailableBytes < 0 || strings.TrimSpace(status.Storage.State) == "" {
		t.Fatalf("consumer-safe DVR storage=%#v", status.Storage)
	}
	consumerJSON := recorder.Body.String()
	for _, forbidden := range []string{"freeBytes", "path", "sources", "lastError", "guideStatus", "activeTuners"} {
		if strings.Contains(consumerJSON, forbidden) {
			t.Fatalf("consumer DVR status exposed %q: %s", forbidden, consumerJSON)
		}
	}
	adminRecorder := httptest.NewRecorder()
	server.handleAdminDVROperationalStatus(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/dvr/status", nil), userA)
	if adminRecorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary viewer admin DVR status=%d body=%s", adminRecorder.Code, adminRecorder.Body.String())
	}
}

func TestDVRSchedulingRejectsBlockedAndMismatchedLiveTVReferences(t *testing.T) {
	server := newScannerTestServer(t)
	user := releaseCandidateOrdinaryDVRUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	for _, sourceID := range []string{"src_policy_a", "src_policy_b"} {
		if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,enabled,created_at,updated_at) VALUES (?,?,'m3u',1,?,?)`, sourceID, sourceID, stamp, stamp); err != nil {
			t.Fatalf("insert source %s: %v", sourceID, err)
		}
	}
	for _, channel := range []struct{ id, source string }{{"channel_allowed", "src_policy_a"}, {"channel_blocked", "src_policy_a"}, {"channel_other_source", "src_policy_b"}} {
		if _, err := server.db.Exec(`INSERT INTO live_tv_channels (id,source_id,name,stream_url,enabled,last_seen_at,created_at,updated_at) VALUES (?,?,?,'https://media.example.test/live.m3u8',1,?,?,?)`, channel.id, channel.source, channel.id, stamp, stamp, stamp); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_programs (id,source_id,channel_id,title,start_at,end_at,created_at) VALUES ('program_allowed','src_policy_a','channel_allowed','Allowed',?,?,?)`, now.Add(time.Hour).Format(time.RFC3339), now.Add(2*time.Hour).Format(time.RFC3339), stamp); err != nil {
		t.Fatalf("insert program: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE users SET preferences_json='{"channelPolicy":{"blockedChannelIds":["channel_blocked"]}}' WHERE id=?`, user.ID); err != nil {
		t.Fatalf("set channel policy: %v", err)
	}

	base := DVRRecordingRequest{Title: "Policy test", StartsAt: now.Add(time.Hour).Format(time.RFC3339), EndsAt: now.Add(2 * time.Hour).Format(time.RFC3339)}
	blocked := base
	blocked.SourceID, blocked.ChannelID = "src_policy_a", "channel_blocked"
	if _, err := server.createDVRRecording(user, blocked); !errors.Is(err, errDVRLiveTVReferenceDenied) {
		t.Fatalf("blocked channel scheduling error = %v", err)
	}
	mismatched := base
	mismatched.SourceID, mismatched.ChannelID, mismatched.ProgramID = "src_policy_b", "channel_other_source", "program_allowed"
	if _, err := server.createDVRRecording(user, mismatched); !errors.Is(err, errDVRLiveTVReferenceDenied) {
		t.Fatalf("mismatched source/program scheduling error = %v", err)
	}
	if _, err := server.createDVRRule(user, DVRRecordingRuleRequest{SourceID: "src_policy_a", ChannelID: "channel_blocked", Title: "Blocked series", MatchType: "series", Enabled: true}); !errors.Is(err, errDVRLiveTVReferenceDenied) {
		t.Fatalf("blocked channel rule error = %v", err)
	}
	allowed := base
	allowed.SourceID, allowed.ChannelID, allowed.ProgramID = "src_policy_a", "channel_allowed", "program_allowed"
	recording, err := server.createDVRRecording(user, allowed)
	if err != nil {
		t.Fatalf("allowed canonical recording: %v", err)
	}
	if recording.SourceID != "src_policy_a" || recording.ChannelID != "channel_allowed" || recording.ProgramID != "program_allowed" {
		t.Fatalf("recording reference was not canonical: %#v", recording)
	}
	profileRestricted := user
	profileRestricted.ChannelPolicy = UserChannelPolicy{BlockedChannelIDs: []string{"channel_allowed"}}
	if _, err := server.resolveAuthorizedDVRLiveTVReference(context.Background(), profileRestricted, "src_policy_a", "channel_allowed", "program_allowed", false, true); !errors.Is(err, errDVRLiveTVReferenceDenied) {
		t.Fatalf("profile channel-policy restriction error=%v", err)
	}
	if _, err := server.db.Exec(`UPDATE users SET preferences_json='{"channelPolicy":{"blockedChannelIds":["channel_allowed"]}}' WHERE id=?`, user.ID); err != nil {
		t.Fatalf("revoke channel: %v", err)
	}
	if server.dvrRecordingChannelAllowed(user, recording) {
		t.Fatal("recorded playback remained authorized after channel-policy revocation")
	}
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "policy-recording.ts")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordingPath, []byte("recorded"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_recordings SET status='complete',path=?,size_bytes=8 WHERE id=?`, recordingPath, recording.ID); err != nil {
		t.Fatal(err)
	}
	playback := performDVRRouteRequest(server, user, http.MethodPost, "/api/dvr/recordings/"+recording.ID+"/playback", `{"intent":{"quality":{"mode":"automatic"}}}`)
	if playback.Code != http.StatusNotFound {
		t.Fatalf("revoked DVR playback=%d body=%s", playback.Code, playback.Body.String())
	}
}

func TestDVRRuleProgramMaterializationIsDatabaseIdempotent(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_idempotent", "channel_idempotent", 2)
	start := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_programs (id,source_id,channel_id,title,start_at,end_at,created_at)
		VALUES ('program_idempotent','source_idempotent','channel_idempotent','Repeat-safe episode',?,?,?)`,
		start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339), stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recording_rules (id,user_id,profile_id,source_id,channel_id,program_id,title,match_type,enabled,created_at,updated_at)
		VALUES ('rule_idempotent',?,?, 'source_idempotent','channel_idempotent','','Repeat-safe episode','series',1,?,?)`,
		accountIDForUser(user), viewerProfileID(user), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	req := DVRRecordingRequest{
		RuleID: "rule_idempotent", SourceID: "source_idempotent", ChannelID: "channel_idempotent", ProgramID: "program_idempotent",
		Title: "Repeat-safe episode", StartsAt: start.Format(time.RFC3339), EndsAt: start.Add(time.Hour).Format(time.RFC3339),
	}
	first, err := server.createDVRRecording(user, req)
	if err != nil {
		t.Fatalf("first materialization: %v", err)
	}
	second, err := server.createDVRRecording(user, req)
	if err != nil {
		t.Fatalf("repeated materialization: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("repeated materialization returned %q then %q", first.ID, second.ID)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE rule_id='rule_idempotent' AND program_id='program_idempotent'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rule/program materialization count=%d err=%v", count, err)
	}
	if _, err := server.db.Exec(`INSERT INTO users (id,username,email,display_name,password_hash,role,permissions_json,preferences_json,created_at,updated_at) VALUES ('foreign_rule_user','foreign-rule-user','foreign-rule@example.test','Foreign Rule','hash','user','{}','{}',?,?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_recording_rules (id,user_id,profile_id,source_id,channel_id,program_id,title,match_type,enabled,created_at,updated_at) VALUES ('foreign_rule','foreign_rule_user','foreign_rule_user','source_idempotent','channel_idempotent','','Foreign','series',1,?,?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	foreign := req
	foreign.RuleID = "foreign_rule"
	if _, err := server.createDVRRecording(user, foreign); !errors.Is(err, errDVRLiveTVReferenceDenied) {
		t.Fatalf("cross-profile rule binding error=%v", err)
	}
}

func TestGuideRefreshReconcilesSeriesRulesAndRevokedChannelPolicy(t *testing.T) {
	server := newScannerTestServer(t)
	user := releaseCandidateOrdinaryDVRUser(t, server)
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,enabled,created_at,updated_at) VALUES ('source_refresh_rules','Refresh Rules','m3u',1,?,?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	source, err := server.getLiveTVSourceRecord("source_refresh_rules")
	if err != nil {
		t.Fatal(err)
	}
	channel := liveTVChannelImport{ID: "channel_refresh_rules", ProviderKey: "guide-refresh", Number: "10", Name: "Refresh Channel", StreamURL: "https://media.example.test/refresh.m3u8", GroupTitle: "Drama"}
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	program := func(id string, offset time.Duration) liveTVProgramImport {
		return liveTVProgramImport{ID: id, ChannelID: channel.ID, ChannelRef: channel.ProviderKey, Title: "Refresh Series", StartAt: start.Add(offset).Format(time.RFC3339), EndAt: start.Add(offset + time.Hour).Format(time.RFC3339), IsNew: true}
	}
	first := program("program_refresh_one", 0)
	if err := server.storeLiveTVImport(source, []liveTVChannelImport{channel}, []liveTVProgramImport{first}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT channel_id FROM live_tv_channel_locators WHERE source_id=? AND provider_key=?`, source.ID, channel.ProviderKey).Scan(&channel.ID); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT id FROM live_tv_programs WHERE source_id=? AND title=? ORDER BY start_at LIMIT 1`, source.ID, first.Title).Scan(&first.ID); err != nil {
		t.Fatal(err)
	}
	first.ChannelID = channel.ID
	rule, err := server.createDVRRule(user, DVRRecordingRuleRequest{SourceID: source.ID, ChannelID: channel.ID, ProgramID: first.ID, Title: first.Title, MatchType: "series", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	second := program("program_refresh_two", 2*time.Hour)
	second.ChannelID = channel.ID
	if err := server.storeLiveTVImport(source, []liveTVChannelImport{channel}, []liveTVProgramImport{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT id FROM live_tv_programs WHERE source_id=? AND title=? ORDER BY start_at DESC LIMIT 1`, source.ID, second.Title).Scan(&second.ID); err != nil {
		t.Fatal(err)
	}
	var materialized int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE rule_id=? AND program_id=?`, rule.ID, second.ID).Scan(&materialized); err != nil || materialized != 1 {
		t.Fatalf("refreshed episode materialized=%d err=%v", materialized, err)
	}
	if _, err := server.db.Exec(`UPDATE users SET preferences_json=? WHERE id=?`, fmt.Sprintf(`{"channelPolicy":{"blockedChannelIds":[%q]}}`, channel.ID), user.ID); err != nil {
		t.Fatal(err)
	}
	if err := server.storeLiveTVImport(source, []liveTVChannelImport{channel}, []liveTVProgramImport{first, second}); err != nil {
		t.Fatal(err)
	}
	var enabled, future int
	if err := server.db.QueryRow(`SELECT enabled FROM live_tv_recording_rules WHERE id=?`, rule.ID).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE rule_id=? AND status='scheduled'`, rule.ID).Scan(&future); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || future != 0 {
		t.Fatalf("revoked rule enabled=%d future=%d", enabled, future)
	}
}

func TestDVRLeaseTokenFencesStaleWorkerFailureAndArtifacts(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_fenced", "channel_fenced", 1)
	now := time.Now().UTC()
	if _, err := server.db.Exec(`INSERT INTO live_tv_recordings (id,user_id,profile_id,source_id,channel_id,title,status,starts_at,ends_at,path,priority,revision,created_at,updated_at) VALUES ('recording_fenced',?,?, 'source_fenced','channel_fenced','Fenced','running',?,?, '',50,1,?,?)`, user.ID, viewerProfileID(user), now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	lease, err := server.reserveLiveTVTunerAllocation(context.Background(), "source_fenced", "channel_fenced", "dvr_recording", "recording_fenced")
	if err != nil {
		t.Fatal(err)
	}
	replacementPath := filepath.Join(server.cfg.AppDataDir, "recordings", "replacement.partial.ts")
	if err := os.MkdirAll(filepath.Dir(replacementPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacementPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	server.failDVRRecordingLease("recording_fenced", "stale-token", replacementPath, errors.New("stale worker"))
	var status, token string
	if err := server.db.QueryRow(`SELECT r.status,a.lease_token FROM live_tv_recordings r JOIN live_tv_tuner_allocations a ON a.consumer_id=r.id WHERE r.id='recording_fenced'`).Scan(&status, &token); err != nil {
		t.Fatal(err)
	}
	if status != "running" || token != lease.Token {
		t.Fatalf("stale worker changed replacement status=%s token=%s", status, token)
	}
	if _, err := os.Stat(replacementPath); err != nil {
		t.Fatalf("stale worker removed replacement artifact: %v", err)
	}
}

func TestDVRConflictSweepAllowsDisjointHalfWindowsOnTwoTuners(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_sweep", "channel_sweep", 2)
	now := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	stamp := time.Now().UTC().Format(time.RFC3339)
	insert := func(id string, start, end time.Time) {
		if _, err := server.db.Exec(`INSERT INTO live_tv_recordings (id,user_id,profile_id,source_id,channel_id,title,status,starts_at,ends_at,priority,revision,created_at,updated_at) VALUES (?,?,?,'source_sweep','channel_sweep',?,'scheduled',?,?,50,1,?,?)`, id, user.ID, viewerProfileID(user), id, start.Format(time.RFC3339), end.Format(time.RFC3339), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	insert("half_a", now, now.Add(30*time.Minute))
	insert("half_b", now.Add(30*time.Minute), now.Add(time.Hour))
	conflict, err := server.findDVRRecordingConflictWithPriority("source_sweep", now, now.Add(time.Hour), "", 50)
	if err != nil || conflict.ID != "" {
		t.Fatalf("legal disjoint-half sweep conflict=%#v err=%v", conflict, err)
	}
	insert("middle_c", now.Add(15*time.Minute), now.Add(45*time.Minute))
	conflict, err = server.findDVRRecordingConflictWithPriority("source_sweep", now, now.Add(time.Hour), "", 50)
	if err != nil || conflict.ID == "" || conflict.AllocationCapacity != 2 || conflict.AllocationDemand != 3 {
		t.Fatalf("over-capacity subinterval conflict=%#v err=%v", conflict, err)
	}
}

func TestPersistentTunerAllocationCoordinatesLiveAndDVRAcrossProcesses(t *testing.T) {
	primary := newScannerTestServer(t)
	secondary := secondProcessTestServer(t, primary)
	insertReleaseCandidateLiveSource(t, primary, "source_capacity", "channel_capacity", 2)
	start := make(chan struct{})
	type result struct {
		kind  string
		lease liveTVTunerLease
		err   error
	}
	results := make(chan result, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	reserve := func(server *Server, kind, consumer string) {
		ready.Done()
		<-start
		lease, err := server.reserveLiveTVTunerAllocation(context.Background(), "source_capacity", "channel_capacity", kind, consumer)
		results <- result{kind: kind, lease: lease, err: err}
	}
	go reserve(primary, "live_session", "live_cross_process")
	go reserve(secondary, "dvr_recording", "dvr_cross_process")
	ready.Wait()
	close(start)
	for range 2 {
		result := <-results
		if result.err != nil || !result.lease.Created || result.lease.Token == "" {
			t.Fatalf("%s reservation = %#v, err=%v", result.kind, result.lease, result.err)
		}
	}
	if _, err := secondary.reserveLiveTVTunerAllocation(context.Background(), "source_capacity", "channel_capacity", "live_session", "live_over_capacity"); !errors.Is(err, errLiveTVTunerCapacity) {
		t.Fatalf("third cross-process allocation error = %v", err)
	}
	var total, live, dvr int
	if err := primary.db.QueryRow(`SELECT COUNT(*), SUM(allocation_kind='live_session'), SUM(allocation_kind='dvr_recording') FROM live_tv_tuner_allocations WHERE source_id='source_capacity'`).Scan(&total, &live, &dvr); err != nil {
		t.Fatal(err)
	}
	if total != 2 || live != 1 || dvr != 1 {
		t.Fatalf("persistent allocations total=%d live=%d dvr=%d", total, live, dvr)
	}
}

func TestConcurrentScheduleCheckAndInsertIsSerializedAcrossProcesses(t *testing.T) {
	primary := newScannerTestServer(t)
	secondary := secondProcessTestServer(t, primary)
	user := dvrTestUser(t, primary)
	insertReleaseCandidateLiveSource(t, primary, "source_schedule", "channel_schedule", 1)
	startAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	request := DVRRecordingRequest{SourceID: "source_schedule", ChannelID: "channel_schedule", Title: "Atomic schedule", StartsAt: startAt.Format(time.RFC3339), EndsAt: startAt.Add(time.Hour).Format(time.RFC3339)}
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, server := range []*Server{primary, secondary} {
		go func(server *Server) {
			ready.Done()
			<-start
			_, err := server.createDVRRecording(user, request)
			results <- err
		}(server)
	}
	ready.Wait()
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		var conflict *dvrScheduleConflictError
		if errors.As(err, &conflict) {
			conflicts++
			if conflict.Capacity != 1 || conflict.Demand != 2 {
				t.Fatalf("conflict capacity=%d demand=%d", conflict.Capacity, conflict.Demand)
			}
			continue
		}
		t.Fatalf("unexpected concurrent schedule error: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent schedule successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestLiveTVReservationRollbackAndImmediateStalePrune(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_rollback", "channel_rollback", 1)
	request := httptest.NewRequest(http.MethodPost, "/api/playback/sessions", nil)
	if _, err := server.liveTVPlaybackResponseForSession(request, user, "missing_playback_session", "channel_rollback", PlaybackClientProfile{SupportsHLS: true}, PlaybackIntent{}); err == nil {
		t.Fatal("response construction unexpectedly succeeded without a playback session")
	}
	var allocations int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE source_id='source_rollback'`).Scan(&allocations); err != nil {
		t.Fatal(err)
	}
	if allocations != 0 {
		t.Fatalf("failed response leaked %d tuner allocations", allocations)
	}
	stale := time.Now().UTC().Add(-liveTVAllocationStaleAfter - time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_tuner_allocations (id,source_id,channel_id,allocation_kind,consumer_id,allocation_key,lease_token,acquired_at,heartbeat_at,expires_at) VALUES ('stale_live','source_rollback','channel_rollback','live_session','gone_session','live_session:gone_session','old_lease',?,?, '')`, stale, stale); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	conflict, err := server.findDVRRecordingConflictWithPriority("source_rollback", now, now.Add(time.Hour), "", 50)
	if err != nil || conflict.ID != "" {
		t.Fatalf("stale allocation conflict=%#v err=%v", conflict, err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE id='stale_live'`).Scan(&allocations); err != nil || allocations != 0 {
		t.Fatalf("stale allocation count=%d err=%v", allocations, err)
	}
}

func TestLiveTVGrantHeartbeatKeepsActiveTunerLeaseFresh(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_heartbeat", "channel_heartbeat", 1)
	profileID := viewerProfileID(user)
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,media_id,media_type,title,started_at,last_seen_at,state,is_live) VALUES ('session_heartbeat',?,?, 'channel_heartbeat','live_channel','Heartbeat',?,?,'playing',1)`, user.ID, profileID, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	bindPlaybackSessionPlanForTest(t, server.db, "session_heartbeat", "channel_heartbeat", true)
	lease, err := server.reserveLiveTVTunerAllocation(context.Background(), "source_heartbeat", "channel_heartbeat", "live_session", "session_heartbeat")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := server.issueMediaGrant(context.Background(), user, "session_heartbeat", "live_channel", "channel_heartbeat")
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-30 * time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`UPDATE live_tv_tuner_allocations SET heartbeat_at=? WHERE consumer_id='session_heartbeat'`, old); err != nil {
		t.Fatal(err)
	}
	if !server.heartbeatLiveTVTunerAllocationForGrant(context.Background(), grant.Token) {
		t.Fatal("active manifest/segment heartbeat did not refresh the tuner allocation")
	}
	var heartbeat, token string
	if err := server.db.QueryRow(`SELECT heartbeat_at,lease_token FROM live_tv_tuner_allocations WHERE consumer_id='session_heartbeat'`).Scan(&heartbeat, &token); err != nil {
		t.Fatal(err)
	}
	refreshed, _ := time.Parse(time.RFC3339, heartbeat)
	if !refreshed.After(now.Add(-5*time.Second)) || token != lease.Token {
		t.Fatalf("heartbeat=%s token=%s lease=%s", heartbeat, token, lease.Token)
	}
}

func TestLiveTVMediaGrantFailsClosedAfterProfileOrChannelPolicyRevocation(t *testing.T) {
	server := newScannerTestServer(t)
	user := releaseCandidateOrdinaryDVRUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_grant_revoke", "channel_grant_revoke", 1)
	issue := func(sessionID string) MediaGrant {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := server.db.Exec(`INSERT INTO playback_sessions (id,user_id,profile_id,media_id,media_type,title,started_at,last_seen_at,state,is_live) VALUES (?,?,?,'channel_grant_revoke','live_channel','Revocation',?,?,'playing',1)`, sessionID, user.ID, viewerProfileID(user), now, now); err != nil {
			t.Fatal(err)
		}
		bindPlaybackSessionPlanForTest(t, server.db, sessionID, "channel_grant_revoke", true)
		if _, err := server.reserveLiveTVTunerAllocation(context.Background(), "source_grant_revoke", "channel_grant_revoke", "live_session", sessionID); err != nil {
			t.Fatal(err)
		}
		grant, err := server.issueMediaGrant(context.Background(), user, sessionID, "live_channel", "channel_grant_revoke")
		if err != nil {
			t.Fatal(err)
		}
		return grant
	}
	requestFor := func(grant MediaGrant) *http.Request {
		return mediaGrantRequest(http.MethodGet, "/api/live-tv/hls/channel_grant_revoke/playlist.m3u8", grant.Token)
	}
	profileGrant := issue("session_profile_revoked")
	if _, err := server.userForMediaGrant(requestFor(profileGrant)); err != nil {
		t.Fatalf("valid grant: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE profiles SET disabled_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), viewerProfileID(user)); err != nil {
		t.Fatal(err)
	}
	if _, err := server.userForMediaGrant(requestFor(profileGrant)); !errors.Is(err, errMediaGrantDenied) {
		t.Fatalf("disabled profile grant error=%v", err)
	}
	var allocations int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations`).Scan(&allocations); err != nil || allocations != 0 {
		t.Fatalf("disabled profile allocation=%d err=%v", allocations, err)
	}

	if _, err := server.db.Exec(`UPDATE profiles SET disabled_at='' WHERE id=?`, viewerProfileID(user)); err != nil {
		t.Fatal(err)
	}
	policyGrant := issue("session_policy_revoked")
	if _, err := server.db.Exec(`UPDATE users SET preferences_json='{"channelPolicy":{"blockedChannelIds":["channel_grant_revoke"]}}' WHERE id=?`, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.userForMediaGrant(requestFor(policyGrant)); !errors.Is(err, errMediaGrantDenied) {
		t.Fatalf("blocked channel grant error=%v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations`).Scan(&allocations); err != nil || allocations != 0 {
		t.Fatalf("blocked channel allocation=%d err=%v", allocations, err)
	}
}

func releaseCandidateOrdinaryDVRUser(t *testing.T, server *Server) User {
	t.Helper()
	user, err := server.createUser(UserRequest{
		Username: "release-dvr-user", Email: "release-dvr-user@example.test", DisplayName: "Release DVR User",
		Password: "Release-dvr-user-password1", Role: "user", Permissions: ownerPermissions(),
	})
	if err != nil {
		t.Fatalf("create ordinary DVR policy principal: %v", err)
	}
	return user
}

func TestDVRRestartReconciliationRecoversStaleLeaseAndExpiresMissedWindow(t *testing.T) {
	primary := newScannerTestServer(t)
	restarted := secondProcessTestServer(t, primary)
	user := dvrTestUser(t, primary)
	insertReleaseCandidateLiveSource(t, primary, "source_restart", "channel_restart", 1)
	now := time.Now().UTC().Truncate(time.Second)
	recordingRoot := filepath.Join(primary.cfg.AppDataDir, "recordings")
	if err := os.MkdirAll(recordingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(recordingRoot, "partial.ts")
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := now.Format(time.RFC3339)
	for _, row := range []struct{ id, status, starts, ends, path string }{
		{"restart_running", "running", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), partial},
		{"restart_missed", "scheduled", now.Add(-2 * time.Hour).Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339), ""},
	} {
		if _, err := primary.db.Exec(`INSERT INTO live_tv_recordings (id,user_id,profile_id,source_id,channel_id,program_id,title,status,starts_at,ends_at,path,priority,revision,created_at,updated_at) VALUES (?,?,?,?,?,'',?,?,?, ?,?,50,1,?,?)`, row.id, user.ID, viewerProfileID(user), "source_restart", "channel_restart", row.id, row.status, row.starts, row.ends, row.path, stamp, stamp); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	stale := now.Add(-liveTVAllocationStaleAfter - time.Second).Format(time.RFC3339)
	if _, err := primary.db.Exec(`INSERT INTO live_tv_tuner_allocations (id,source_id,channel_id,allocation_kind,consumer_id,allocation_key,lease_token,acquired_at,heartbeat_at,expires_at) VALUES ('restart_lease','source_restart','channel_restart','dvr_recording','restart_running','dvr_recording:restart_running','stale_restart',?,?, '')`, stale, stale); err != nil {
		t.Fatal(err)
	}
	if err := restarted.reconcileDVRStateAfterRestart(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var runningStatus, runningPath, missedStatus, missedCode string
	if err := primary.db.QueryRow(`SELECT status,path FROM live_tv_recordings WHERE id='restart_running'`).Scan(&runningStatus, &runningPath); err != nil {
		t.Fatal(err)
	}
	if err := primary.db.QueryRow(`SELECT status,failure_code FROM live_tv_recordings WHERE id='restart_missed'`).Scan(&missedStatus, &missedCode); err != nil {
		t.Fatal(err)
	}
	if runningStatus != "scheduled" || runningPath != "" || missedStatus != "failed" || missedCode != "missed_window" {
		t.Fatalf("running=%s path=%q missed=%s code=%s", runningStatus, runningPath, missedStatus, missedCode)
	}
	if _, err := os.Stat(partial); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial recording still exists: %v", err)
	}
	var leaseCount int
	if err := primary.db.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE consumer_id='restart_running'`).Scan(&leaseCount); err != nil || leaseCount != 0 {
		t.Fatalf("restart lease count=%d err=%v", leaseCount, err)
	}
}

func insertReleaseCandidateLiveSource(t *testing.T, server *Server, sourceID, channelID string, tunerCount int) {
	t.Helper()
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,enabled,tuner_count,created_at,updated_at) VALUES (?,?,'m3u',1,?,?,?)`, sourceID, sourceID, tunerCount, stamp, stamp); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_channels (id,source_id,name,stream_url,enabled,last_seen_at,created_at,updated_at) VALUES (?,?,?,'https://media.example.test/live.m3u8',1,?,?,?)`, channelID, sourceID, channelID, stamp, stamp, stamp); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
}

func secondProcessTestServer(t *testing.T, primary *Server) *Server {
	t.Helper()
	db, err := database.OpenRuntimeHandle(primary.cfg)
	if err != nil {
		t.Fatalf("open second process database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{
		cfg: primary.cfg, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		logSubscribers: map[chan LogEvent]bool{}, scannerWatch: map[string]string{}, transcodes: map[string]*transcodeSession{},
	}
}
