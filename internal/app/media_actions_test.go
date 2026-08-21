package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestMediaActionsArePermissionAvailabilityAndStateFiltered(t *testing.T) {
	item := MediaItem{
		ID: "movie_action_contract", Type: "movie", FileCount: 1,
		State: MediaState{Watchlisted: true},
	}
	viewer := User{ID: "usr_viewer", Permissions: map[string]bool{
		"playMedia": true, "downloadMedia": true,
	}}
	actions := stringSet(mediaActionsForItem(item, viewer))
	for _, required := range []string{
		mediaActionPlay, mediaActionDownload, mediaActionQueueAdd,
		mediaActionWatchlistRemove, mediaActionFavoriteAdd, mediaActionWatchedMark,
		mediaActionCollectionAdd, mediaActionPlaylistAdd,
	} {
		if !actions[required] {
			t.Errorf("available viewer action projection omitted %q: %#v", required, actions)
		}
	}
	for _, forbidden := range []string{mediaActionWatchlistAdd, mediaActionMetadataEdit, mediaActionMediaDelete} {
		if actions[forbidden] {
			t.Errorf("viewer action projection exposed %q: %#v", forbidden, actions)
		}
	}

	watchTogetherViewer := viewer
	watchTogetherViewer.Preferences.Privacy.IncludeInWatchWithFriends = true
	if stringSet(mediaActionsForItem(item, watchTogetherViewer))[mediaActionWatchTogether] {
		t.Fatal("watch-with-friends action was projected from preference without current permission")
	}
	watchTogetherViewer.Permissions["watchWithFriends"] = true
	if !stringSet(mediaActionsForItem(item, watchTogetherViewer))[mediaActionWatchTogether] {
		t.Fatal("eligible watch-with-friends action was not projected")
	}

	item.MissingFileCount = 1
	actions = stringSet(mediaActionsForItem(item, viewer))
	for _, forbidden := range []string{mediaActionPlay, mediaActionDownload, mediaActionQueueAdd} {
		if actions[forbidden] {
			t.Errorf("unavailable media exposed %q: %#v", forbidden, actions)
		}
	}

	apiReader := viewer
	apiReader.AuthProvider = "api_key"
	apiReader.APIKeyScopes = []string{"read"}
	actions = stringSet(mediaActionsForItem(MediaItem{ID: item.ID, Type: "movie", FileCount: 1}, apiReader))
	for _, forbidden := range []string{mediaActionWatchlistAdd, mediaActionFavoriteAdd, mediaActionCollectionAdd, mediaActionPlaylistAdd} {
		if actions[forbidden] {
			t.Errorf("read-only API key exposed mutating action %q: %#v", forbidden, actions)
		}
	}
}

func TestFullAndLeanMediaHydrationShareCanonicalActions(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	var mediaID, libraryID string
	if err := db.QueryRow(`SELECT id, library_id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 1`).Scan(&mediaID, &libraryID); err != nil {
		t.Fatalf("load movie fixture: %v", err)
	}
	user := User{ID: "usr_action_projection", Role: "user", Permissions: map[string]bool{
		"playMedia": true, "editMetadata": true,
	}}
	permissionsJSON, _ := json.Marshal(user.Permissions)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES (?, 'action-projection', 'action-projection@example.test', 'Action Projection', 'user', ?, '{}', ?, ?)`,
		user.ID, string(permissionsJSON), now, now); err != nil {
		t.Fatalf("insert action projection user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, ?, ?)`, user.ID, libraryID, now); err != nil {
		t.Fatalf("grant action projection library: %v", err)
	}
	ctx := contextWithMediaActionUser(context.Background(), user)
	lean, err := server.queryMediaListItemsContext(ctx, user.ID, `WHERE m.id = ? LIMIT 1`, []any{mediaID})
	if err != nil || len(lean) != 1 {
		t.Fatalf("lean media hydration failed: items=%#v err=%v", lean, err)
	}
	full, err := server.queryMediaContext(ctx, user.ID, `WHERE m.id = ? LIMIT 1`, []any{mediaID})
	if err != nil || len(full) != 1 {
		t.Fatalf("full media hydration failed: items=%#v err=%v", full, err)
	}
	for label, item := range map[string]MediaItem{"lean": lean[0], "full": full[0]} {
		actions := stringSet(item.Actions)
		if !actions[mediaActionPlay] || !actions[mediaActionMetadataEdit] || actions[mediaActionMediaDelete] {
			t.Errorf("%s action projection = %#v", label, item.Actions)
		}
	}
	card := mediaCardForBrowse(lean[0], user, nil)
	if got, want := card.Actions, lean[0].Actions; !sameStringSet(got, want) {
		t.Fatalf("browse and full-media action vocabularies diverged: card=%#v media=%#v", got, want)
	}
}

func TestMediaFeedbackActionsFollowEffectiveProfilePolicy(t *testing.T) {
	item := MediaItem{ID: "movie_feedback_actions", Type: "movie", FileCount: 1}
	allowed := User{ID: "viewer", AllowFeedback: true, Permissions: map[string]bool{"playMedia": true}}
	actions := stringSet(mediaActionsForItem(item, allowed))
	if !actions[mediaActionReportProblem] || !actions[mediaActionRequestQuality] {
		t.Fatalf("feedback-enabled profile actions = %#v", actions)
	}
	allowed.AllowFeedback = false
	actions = stringSet(mediaActionsForItem(item, allowed))
	if actions[mediaActionReportProblem] || actions[mediaActionRequestQuality] {
		t.Fatalf("feedback-disabled profile actions = %#v", actions)
	}
	allowed.AllowFeedback = true
	allowed.AuthProvider = "api_key"
	actions = stringSet(mediaActionsForItem(item, allowed))
	if actions[mediaActionReportProblem] || actions[mediaActionRequestQuality] {
		t.Fatalf("API key exposed interactive feedback actions = %#v", actions)
	}
}

func TestMediaActionProjectionPreservesDistinctAccountAndProfileIdentity(t *testing.T) {
	server := newScannerTestServer(t)
	account, childProfile := createProfileProtocolAccount(t, server)
	child := explicitPrimaryUser(account)
	child.ProfileID = childProfile.ID
	child.ProfileIsPrimary = false
	child.AllowFeedback = true
	child.Permissions = map[string]bool{"playMedia": true}
	item := MediaItem{ID: "profile-action-movie", Type: "movie", FileCount: 1}

	projected, err := server.applyMediaActionProjectionContext(contextWithMediaActionUser(context.Background(), child), childProfile.ID, []MediaItem{item})
	if err != nil || len(projected) != 1 {
		t.Fatalf("context projection items=%#v err=%v", projected, err)
	}
	actions := stringSet(projected[0].Actions)
	if !actions[mediaActionPlay] || !actions[mediaActionReportProblem] {
		t.Fatalf("subordinate profile lost actions: %#v", projected[0].Actions)
	}

	fallback, err := server.applyMediaActionProjectionContext(context.Background(), childProfile.ID, []MediaItem{item})
	if err != nil || len(fallback) != 1 || !stringSet(fallback[0].Actions)[mediaActionPlay] {
		t.Fatalf("profile fallback projection items=%#v err=%v", fallback, err)
	}
}

func TestMediaActionUserContextResolvesSecondaryProfilePolicy(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	account, err := server.createUser(UserRequest{
		Username: "action-profile-account", Email: "action-profile@example.test", DisplayName: "Action Profile Account",
		Password: "Action-profile-password", Role: "user",
		Permissions: map[string]bool{"playMedia": true, "downloadMedia": true, "editMetadata": true},
	})
	if err != nil {
		t.Fatalf("create action profile account: %v", err)
	}
	restrictions := defaultProfileRestrictions()
	restrictions.AllowDownloads = false
	encodedRestrictions, err := encodeProfileRestrictions(restrictions)
	if err != nil {
		t.Fatalf("encode profile restrictions: %v", err)
	}
	profileID := "profile_action_projection_child"
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO profiles (
			id, account_id, origin, external_profile_id, is_primary, sort_order,
			display_name, role, permissions_json, preferences_json, restrictions_json,
			created_at, updated_at
		) VALUES (?, ?, 'hosted', 'action-profile-child', 0, 1, 'Action Child', 'user', '{}', '{}', ?, ?, ?)`,
		profileID, account.ID, encodedRestrictions, now, now); err != nil {
		t.Fatalf("insert action child profile: %v", err)
	}

	resolved, err := server.mediaActionUserContext(context.Background(), profileID)
	if err != nil {
		t.Fatalf("resolve secondary profile: %v", err)
	}
	if resolved.ID != account.ID || resolved.AccountID != account.ID || resolved.ProfileID != profileID || resolved.ProfileIsPrimary {
		t.Fatalf("secondary profile identity was not preserved: %#v", resolved)
	}
	if !resolved.Permissions["playMedia"] || resolved.Permissions["downloadMedia"] || !resolved.Permissions["editMetadata"] {
		t.Fatalf("secondary profile permission envelope was not applied: %#v", resolved.Permissions)
	}
	actions := stringSet(mediaActionsForItem(MediaItem{ID: "movie_profile_action", Type: "movie", FileCount: 1}, resolved))
	if !actions[mediaActionPlay] || actions[mediaActionDownload] || !actions[mediaActionMetadataEdit] || actions[mediaActionMediaOptimize] {
		t.Fatalf("secondary profile actions bypassed profile policy: %#v", actions)
	}

	contextUser := resolved
	contextUser.Permissions = map[string]bool{"playMedia": true}
	fromContext, err := server.mediaActionUserContext(contextWithMediaActionUser(context.Background(), contextUser), profileID)
	if err != nil || fromContext.ProfileID != profileID || fromContext.Permissions["downloadMedia"] {
		t.Fatalf("profile-scoped action context was not reused: user=%#v err=%v", fromContext, err)
	}
}
