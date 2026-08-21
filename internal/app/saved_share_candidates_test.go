package app

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestSavedShareCandidatesAreAvailableToOrdinaryMembersWithoutUserAdministrationData(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	requester := createSavedShareCandidateTestUser(t, server, "morgan", "Morgan Reed")
	alice := createSavedShareCandidateTestUser(t, server, "alice", "Alice Rivera")
	createSavedShareCandidateTestUser(t, server, "alina", "Alina Brooks")
	createSavedShareCandidateTestUser(t, server, "percent", "100% Viewer")
	createSavedShareCandidateTestUser(t, server, "plain", "100 Viewer")
	disabled := createSavedShareCandidateTestUser(t, server, "disabled", "Disabled Member")
	deleted := createSavedShareCandidateTestUser(t, server, "deleted", "Deleted Member")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now, disabled.ID); err != nil {
		t.Fatalf("disable candidate: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET auth_origin = 'portico_deleted' WHERE id = ?`, deleted.ID); err != nil {
		t.Fatalf("mark deleted candidate: %v", err)
	}

	unauthenticated := &http.Client{}
	status, _ := doJSON(t, unauthenticated, http.MethodGet, serverURL+"/api/saved/share-candidates", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated candidate list status=%d, want 401", status)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginSavedShareCandidateTestUser(t, client, serverURL, requester.Username)
	status, _ = doJSON(t, client, http.MethodGet, serverURL+"/api/users", nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("ordinary member listed administrative users status=%d, want 403", status)
	}

	var first SavedShareCandidatePage
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/saved/share-candidates?q=ALI&limit=1", nil, &first)
	if status != http.StatusOK || len(first.Items) != 1 || first.Items[0].DisplayName != "alice" || !first.HasMore {
		t.Fatalf("candidate search status=%d body=%s page=%#v", status, body, first)
	}
	for _, privateField := range []string{"@example.test", "username", "email", "permissions", "libraryIds", "authOrigin", "password", "restriction"} {
		if strings.Contains(body, privateField) {
			t.Fatalf("candidate response exposed private field %q: %s", privateField, body)
		}
	}

	var literal SavedShareCandidatePage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/saved/share-candidates?q=100%25&limit=50", nil, &literal)
	if status != http.StatusOK || len(literal.Items) != 1 || literal.Items[0].DisplayName != "percent" {
		t.Fatalf("literal wildcard search status=%d body=%s page=%#v", status, body, literal)
	}

	var all SavedShareCandidatePage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/saved/share-candidates?limit=50", nil, &all)
	if status != http.StatusOK {
		t.Fatalf("candidate list status=%d body=%s", status, body)
	}
	for _, candidate := range all.Items {
		if candidate.UserID == requester.ID || candidate.UserID == disabled.ID || candidate.UserID == deleted.ID {
			t.Fatalf("candidate list exposed ineligible account: %#v", candidate)
		}
	}

	var playlist SavedPlaylist
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{
		Title:  "Shared Queue",
		Shares: []SavedResourceShareRequest{{UserID: alice.ID, CanEdit: true}},
	}, &playlist)
	if status != http.StatusCreated || len(playlist.Shares) != 1 || playlist.Shares[0].UserID != alice.ID || !playlist.Shares[0].CanEdit {
		t.Fatalf("ordinary member share status=%d body=%s playlist=%#v", status, body, playlist)
	}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/collections", CollectionCreateRequest{
		Title:  "Invalid Share",
		Shares: []SavedResourceShareRequest{{UserID: disabled.ID}},
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("disabled share target status=%d, want 400", status)
	}

	for _, target := range []string{
		serverURL + "/api/saved/share-candidates?limit=0",
		serverURL + "/api/saved/share-candidates?limit=51",
		serverURL + "/api/saved/share-candidates?limit=not-a-number",
		serverURL + "/api/saved/share-candidates?q=" + strings.Repeat("a", savedShareCandidateMaximumQuery+1),
	} {
		status, _ = doJSON(t, client, http.MethodGet, target, nil, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid candidate query %q status=%d, want 400", target, status)
		}
	}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/saved/share-candidates", nil, nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("candidate POST status=%d, want 405", status)
	}
}

func TestSavedShareCandidatesRequireActivePorticoMembershipsInPorticoMode(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	active := createSavedShareCandidateTestUser(t, server, "portico-active", "Active Portico Member")
	inactive := createSavedShareCandidateTestUser(t, server, "portico-inactive", "Inactive Portico Member")
	disabled := createSavedShareCandidateTestUser(t, server, "portico-disabled", "Disabled Portico Member")
	now := time.Now().UTC().Format(time.RFC3339)
	for _, candidate := range []User{active, inactive, disabled} {
		membershipID := "membership-" + candidate.ID
		if _, err := db.Exec(`UPDATE users SET auth_origin = 'portico', portico_user_id = ?, portico_membership_id = ? WHERE id = ?`, "portico-"+candidate.ID, membershipID, candidate.ID); err != nil {
			t.Fatalf("mark Portico candidate %s: %v", candidate.ID, err)
		}
		status := "active"
		if candidate.ID == inactive.ID {
			status = "revoked"
		}
		if _, err := db.Exec(`
			INSERT INTO remote_access_members (portico_membership_id, portico_user_id, email, display_name, role, status, local_user_id, last_synced_at)
			VALUES (?, ?, ?, ?, 'user', ?, ?, ?)`, membershipID, "portico-"+candidate.ID, candidate.Email, candidate.DisplayName, status, candidate.ID, now); err != nil {
			t.Fatalf("insert Portico membership %s: %v", candidate.ID, err)
		}
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now, disabled.ID); err != nil {
		t.Fatalf("disable Portico candidate: %v", err)
	}

	page, err := server.listSavedShareCandidates(context.Background(), "unrelated-current-user", "Portico", 50, true)
	if err != nil {
		t.Fatalf("list Portico candidates: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].UserID != active.ID {
		t.Fatalf("Portico candidate eligibility=%#v, want only active member %s", page, active.ID)
	}

	if err := server.saveRemoteAccessSettings(RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           defaultHostedBaseURL,
		ClaimStatus:             "claimed",
		ServerID:                "server-share-candidates",
		PublicPortMode:          "manual",
		ManualPublicPort:        defaultRemotePublicPort,
		PreferredRemoteAuthMode: "portico",
	}); err != nil {
		t.Fatalf("enable Portico mode: %v", err)
	}
	validated, err := server.validateSavedShares(context.Background(), "owner", []SavedResourceShareRequest{{UserID: active.ID, CanEdit: true}})
	if err != nil || len(validated) != 1 || validated[0].UserID != active.ID {
		t.Fatalf("validate active Portico share=%#v err=%v", validated, err)
	}
	if _, err := server.validateSavedShares(context.Background(), "owner", []SavedResourceShareRequest{{UserID: inactive.ID}}); err == nil {
		t.Fatal("inactive Portico membership was accepted as a share target")
	}
}

func createSavedShareCandidateTestUser(t *testing.T, server *Server, username, displayName string) User {
	t.Helper()
	user, err := server.createUser(UserRequest{
		Username:    username,
		Email:       username + "@example.test",
		DisplayName: displayName,
		Password:    "Password1234",
		Role:        "user",
		Permissions: permissionsForRole("user"),
	})
	if err != nil {
		t.Fatalf("create share candidate %s: %v", username, err)
	}
	return user
}

func loginSavedShareCandidateTestUser(t *testing.T, client *http.Client, serverURL, username string) {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login": username, "password": "Password1234",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("login %s status=%d body=%s", username, status, body)
	}
}
