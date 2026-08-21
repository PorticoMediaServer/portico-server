package app

import (
	"database/sql"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

func TestLibraryNavigationDefaultsPersistsAndRejectsUnavailableLibraries(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var initial LibraryNavigationPreferences
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/account/library-navigation", nil, &initial)
	if status != http.StatusOK || len(initial.PinnedLibraryIDs) < 3 {
		t.Fatalf("initial navigation status=%d body=%s preferences=%#v", status, body, initial)
	}

	wanted := []string{initial.PinnedLibraryIDs[2], initial.PinnedLibraryIDs[0]}
	var updated LibraryNavigationPreferences
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/account/library-navigation", LibraryNavigationPreferencesRequest{PinnedLibraryIDs: wanted}, &updated)
	if status != http.StatusOK || len(updated.PinnedLibraryIDs) != 2 || updated.PinnedLibraryIDs[0] != wanted[0] || updated.PinnedLibraryIDs[1] != wanted[1] {
		t.Fatalf("update navigation status=%d body=%s preferences=%#v", status, body, updated)
	}

	var persisted LibraryNavigationPreferences
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/account/library-navigation", nil, &persisted)
	if status != http.StatusOK || len(persisted.PinnedLibraryIDs) != 2 || persisted.PinnedLibraryIDs[0] != wanted[0] || persisted.PinnedLibraryIDs[1] != wanted[1] {
		t.Fatalf("persisted navigation status=%d body=%s preferences=%#v", status, body, persisted)
	}

	status, _ = doJSON(t, client, http.MethodPatch, serverURL+"/api/account/library-navigation", LibraryNavigationPreferencesRequest{PinnedLibraryIDs: []string{"lib_not_shared"}}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unavailable library update status=%d, expected %d", status, http.StatusBadRequest)
	}
}

func TestNewLibraryAndNewAccessDefaultToPinned(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var created Library
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/libraries", CreateLibraryRequest{Name: "Documentaries", Type: "movie", Paths: []string{t.TempDir()}}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create library status=%d body=%s", status, body)
	}
	var ownerNavigation LibraryNavigationPreferences
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/account/library-navigation", nil, &ownerNavigation)
	if status != http.StatusOK || !stringSliceContains(ownerNavigation.PinnedLibraryIDs, created.ID) {
		t.Fatalf("new owner library was not pinned: status=%d body=%s preferences=%#v", status, body, ownerNavigation)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	memberID := "usr_library_navigation_member"
	if _, err := db.Exec(`INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at) VALUES (?, 'navmember', 'navmember@example.test', 'Navigation Member', 'user', '{}', '{}', ?, ?)`, memberID, now, now); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if err := server.withUserTx(t.Context(), func(tx *sql.Tx) error { return replaceUserLibraries(tx, memberID, []string{created.ID}, now) }); err != nil {
		t.Fatalf("grant library access: %v", err)
	}
	var pinned int
	if err := db.QueryRow(`SELECT pinned FROM user_library_navigation WHERE user_id = ? AND library_id = ?`, memberID, created.ID).Scan(&pinned); err != nil || pinned != 1 {
		t.Fatalf("new access navigation row pinned=%d err=%v", pinned, err)
	}
}
