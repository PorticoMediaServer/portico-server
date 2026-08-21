package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountRuntimeFenceCancelsAllProfilesAndBlocksOnlyTargetAccount(t *testing.T) {
	var fence profileRuntimeFence
	primaryContext, releasePrimary, err := fence.acquire(context.Background(), "account-a", "primary-a")
	if err != nil {
		t.Fatalf("acquire primary lease: %v", err)
	}
	childContext, releaseChild, err := fence.acquire(context.Background(), "account-a", "child-a")
	if err != nil {
		t.Fatalf("acquire child lease: %v", err)
	}
	handle := fence.beginAccountErasure("account-a")
	for label, ctx := range map[string]context.Context{"primary": primaryContext, "child": childContext} {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatalf("%s lease was not cancelled", label)
		}
	}
	if _, _, err := fence.acquire(context.Background(), "account-a", "new-child"); !errors.Is(err, errProfileErasureInProgress) {
		t.Fatalf("target account admitted new profile work: %v", err)
	}
	unrelatedContext, releaseUnrelated, err := fence.acquire(context.Background(), "account-b", "primary-b")
	if err != nil {
		t.Fatalf("unrelated account was fenced: %v", err)
	}
	select {
	case <-unrelatedContext.Done():
		t.Fatal("unrelated account lease was cancelled")
	default:
	}
	releasePrimary()
	releaseChild()
	if !handle.wait(context.Background(), time.Second) {
		t.Fatal("account fence did not drain after lease acknowledgement")
	}
	handle.finish()
	reopenedContext, releaseReopened, err := fence.acquire(context.Background(), "account-a", "primary-a")
	if err != nil {
		t.Fatalf("account admission did not reopen after fence completion: %v", err)
	}
	releaseReopened()
	releaseUnrelated()
	if reopenedContext.Err() == nil || unrelatedContext.Err() == nil {
		t.Fatal("released leases should have cancelled contexts")
	}
	fence.mu.Lock()
	defer fence.mu.Unlock()
	if len(fence.entries) != 0 || len(fence.accountEntries) != 0 {
		t.Fatalf("runtime fence retained identifiers after completion: profiles=%d accounts=%d", len(fence.entries), len(fence.accountEntries))
	}
}

func TestConcurrentDisjointUserPatchesPreserveBothMutations(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	user, err := server.createUser(UserRequest{
		Username: "concurrent-user", Email: "concurrent-user@example.test", DisplayName: "Concurrent User",
		Password: "Concurrent-user-password1", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	displayName := "Updated Concurrent User"
	email := "updated-concurrent-user@example.test"
	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var workers sync.WaitGroup
	for _, request := range []UserPatchRequest{{DisplayName: &displayName}, {Email: &email}} {
		request := request
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, updateErr := server.updateUserContext(context.Background(), user.ID, request)
			errorsCh <- updateErr
		}()
	}
	close(start)
	workers.Wait()
	close(errorsCh)
	for updateErr := range errorsCh {
		if updateErr != nil {
			t.Fatalf("concurrent user patch: %v", updateErr)
		}
	}
	updated, err := server.getUser(user.ID)
	if err != nil {
		t.Fatalf("load updated user: %v", err)
	}
	if updated.DisplayName != displayName || updated.Email != email {
		t.Fatalf("concurrent patches lost a mutation: displayName=%q email=%q", updated.DisplayName, updated.Email)
	}
}

func TestUserPatchServiceRejectsOwnerTargetAndControlPlanePermissionKeys(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var ownerID string
	if err := db.QueryRow(`SELECT id FROM users WHERE role = 'owner'`).Scan(&ownerID); err != nil {
		t.Fatalf("load owner: %v", err)
	}
	name := "Taken Over"
	if _, err := server.updateUserContext(context.Background(), ownerID, UserPatchRequest{DisplayName: &name}); err == nil {
		t.Fatal("general user service accepted the owner as a target")
	}
	user, err := server.createUser(UserRequest{
		Username: "permission-user", Email: "permission-user@example.test", DisplayName: "Permission User",
		Password: "Permission-user-password1", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	permissions := map[string]bool{"playMedia": true, "manageServer": false}
	if _, err := server.updateUserContext(context.Background(), user.ID, UserPatchRequest{Permissions: &permissions}); err == nil {
		t.Fatal("user patch accepted a control-plane permission key")
	}
}

func TestAuthorityChangingUserPatchAdvancesRevisionAndRevokesCredentials(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	user, err := server.createUser(UserRequest{
		Username: "authority-user", Email: "authority-user@example.test", DisplayName: "Authority User",
		Password: "Authority-user-password1", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	before := server.authorizationRevisionForUserContext(context.Background(), user)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, profile_id, token_hash, expires_at, created_at, last_seen_at) VALUES ('authority-session', ?, ?, 'authority-session-token', ?, ?, ?)`, user.ID, user.ID, time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), now, now); err != nil {
		t.Fatalf("insert active session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO api_keys (id, user_id, name, token_hash, created_at) VALUES ('authority-key', ?, 'Authority key', 'authority-key-token', ?)`, user.ID, now); err != nil {
		t.Fatalf("insert active API key: %v", err)
	}
	permissions := map[string]bool{"playMedia": true, "downloadMedia": true}
	updated, err := server.updateUserContext(context.Background(), user.ID, UserPatchRequest{Permissions: &permissions})
	if err != nil {
		t.Fatalf("patch authority: %v", err)
	}
	if after := server.authorizationRevisionForUserContext(context.Background(), updated); after == before {
		t.Fatal("authority-changing patch did not advance the authorization revision")
	}
	var sessions, activeKeys int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE user_id = ? AND revoked_at = ''`, user.ID).Scan(&activeKeys); err != nil {
		t.Fatalf("count API keys: %v", err)
	}
	if sessions != 0 || activeKeys != 0 {
		t.Fatalf("authority-changing patch left credentials active: sessions=%d apiKeys=%d", sessions, activeKeys)
	}
}

func TestUserPatchOmissionExplicitEmptyZeroAndFieldErrors(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	user, err := server.createUser(UserRequest{
		Username: "patch-matrix", Email: "patch-matrix@example.test", DisplayName: "Patch Matrix",
		Password: "Patch-matrix-password1", Permissions: map[string]bool{"playMedia": true, "downloadMedia": true},
		AccessSchedule: UserAccessSchedule{Enabled: true, Days: []int{1, 3}, StartMinute: 60, EndMinute: 120},
		TagPolicy:      UserTagPolicy{AllowedTags: []string{"family"}}, DevicePolicy: UserDevicePolicy{Mode: "trusted"},
		ChannelPolicy: UserChannelPolicy{AllowedChannelIDs: []string{"channel-one"}}, MaxContentRating: "PG-13",
		MaxActiveSessions: 4, MaxActiveStreams: 3, RemoteBitrateLimitMbps: 25,
	})
	if err != nil {
		t.Fatalf("create patch matrix user: %v", err)
	}
	baseline := user
	name := "Patch Matrix Updated"
	updated, err := server.updateUserContext(context.Background(), user.ID, UserPatchRequest{DisplayName: &name})
	if err != nil {
		t.Fatalf("omission patch: %v", err)
	}
	if updated.DisplayName != name || updated.Email != baseline.Email || updated.Username != baseline.Username ||
		updated.MaxContentRating != baseline.MaxContentRating || updated.MaxActiveSessions != baseline.MaxActiveSessions ||
		updated.MaxActiveStreams != baseline.MaxActiveStreams || updated.RemoteBitrateLimitMbps != baseline.RemoteBitrateLimitMbps ||
		!reflect.DeepEqual(updated.Permissions, baseline.Permissions) || !reflect.DeepEqual(updated.AccessSchedule, baseline.AccessSchedule) ||
		!reflect.DeepEqual(updated.TagPolicy, baseline.TagPolicy) || !reflect.DeepEqual(updated.DevicePolicy, baseline.DevicePolicy) ||
		!reflect.DeepEqual(updated.ChannelPolicy, baseline.ChannelPolicy) {
		t.Fatalf("omitted patch field changed unexpectedly: baseline=%#v updated=%#v", baseline, updated)
	}

	zero := 0
	emptyRating := ""
	emptyPermissions := map[string]bool{}
	emptyLibraries := []string{}
	emptySchedule := UserAccessSchedule{}
	emptyTags := UserTagPolicy{}
	emptyDevices := UserDevicePolicy{Mode: "any"}
	emptyChannels := UserChannelPolicy{}
	updated, err = server.updateUserContext(context.Background(), user.ID, UserPatchRequest{
		Permissions: &emptyPermissions, LibraryIDs: &emptyLibraries, AccessSchedule: &emptySchedule,
		TagPolicy: &emptyTags, DevicePolicy: &emptyDevices, ChannelPolicy: &emptyChannels,
		MaxContentRating: &emptyRating, MaxActiveSessions: &zero, MaxActiveStreams: &zero, RemoteBitrateLimitMbps: &zero,
	})
	if err != nil {
		t.Fatalf("explicit empty/zero patch: %v", err)
	}
	permissionEnabled := false
	for _, enabled := range updated.Permissions {
		permissionEnabled = permissionEnabled || enabled
	}
	if updated.MaxContentRating != "" || updated.MaxActiveSessions != 0 || updated.MaxActiveStreams != 0 || updated.RemoteBitrateLimitMbps != 0 ||
		permissionEnabled || len(updated.LibraryIDs) != 0 || updated.AccessSchedule.Enabled || updated.DevicePolicy.Mode != "any" ||
		len(updated.TagPolicy.AllowedTags) != 0 || len(updated.ChannelPolicy.AllowedChannelIDs) != 0 {
		t.Fatalf("explicit empty/zero fields did not clear: %#v", updated)
	}

	for _, field := range []string{"username", "email", "displayName", "password", "permissions", "libraryIds", "accessSchedule", "tagPolicy", "devicePolicy", "channelPolicy", "maxContentRating", "maxActiveSessions", "maxActiveStreams", "remoteBitrateLimitMbps"} {
		var request UserPatchRequest
		err := json.Unmarshal([]byte(`{"`+field+`":null}`), &request)
		var fieldErr *requestFieldValidationError
		if !errors.As(err, &fieldErr) || fieldErr.FieldPath != field {
			t.Fatalf("null %s error=%v fieldError=%#v", field, err, fieldErr)
		}
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/users/"+user.ID, map[string]any{"role": "owner"}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "invalid_user_field") || !strings.Contains(body, `"fieldPath":"role"`) {
		t.Fatalf("unknown user patch field status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/users/"+user.ID, map[string]any{"maxActiveSessions": 101}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, `"fieldPath":"maxActiveSessions"`) {
		t.Fatalf("invalid user patch range status=%d body=%s", status, body)
	}
}
