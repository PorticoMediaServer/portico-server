package app

import (
	"bytes"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestLocalProfilePINMutationRequiresCurrentAccountPasswordInAdditionToAdminProof(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	cfg := config.Config{AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db")}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := &Server{cfg: cfg, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	user, err := server.createUser(UserRequest{
		Username: "profile-password", Email: "profile-password@example.test", DisplayName: "Profile Password",
		Password: "Password1234", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	user.ProfileIsPrimary = true
	accountID := user.ID

	now := time.Now().UTC()
	sessionToken := "profile-password-session"
	if _, err := db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ('sess_profile_password', ?, ?, '', ?, ?, ?, ?)`, accountID, accountID, hashToken(sessionToken),
		now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed interactive session: %v", err)
	}
	var pinRevision int64
	if err := db.QueryRow(`SELECT pin_revision FROM profiles WHERE id = ?`, accountID).Scan(&pinRevision); err != nil {
		t.Fatalf("read primary profile PIN revision: %v", err)
	}
	proofToken := "profile-password-proof"
	if _, err := db.Exec(`
		INSERT INTO local_profile_admin_proofs (id, token_hash, account_id, primary_profile_id, session_id, pin_revision, expires_at, created_at)
		VALUES ('proof_profile_password', ?, ?, ?, 'sess_profile_password', ?, ?, ?)`, hashToken(proofToken), accountID, accountID,
		pinRevision, now.Add(time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed profile administration proof: %v", err)
	}

	mutate := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "/api/account/profiles/"+accountID+"/pin", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(profileAdministrationHeader, proofToken)
		request.AddCookie(&http.Cookie{Name: server.sessionCookieNameContext(request.Context()), Value: sessionToken})
		recorder := httptest.NewRecorder()
		server.handleAccountProfilePIN(recorder, request, user, accountID)
		return recorder
	}
	credentialCount := func() int {
		t.Helper()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM local_profile_pin_credentials WHERE profile_id = ?`, accountID).Scan(&count); err != nil && err != sql.ErrNoRows {
			t.Fatalf("count profile PIN credentials: %v", err)
		}
		return count
	}

	if response := mutate(http.MethodPut, `{"pin":"2468","password":"wrong-password"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password set status = %d, body = %s", response.Code, response.Body.String())
	}
	if count := credentialCount(); count != 0 {
		t.Fatalf("wrong-password set created %d PIN credentials", count)
	}
	if response := mutate(http.MethodPut, `{"pin":"2468","password":"Password1234"}`); response.Code != http.StatusNoContent {
		t.Fatalf("password-confirmed set status = %d, body = %s", response.Code, response.Body.String())
	}
	if count := credentialCount(); count != 1 {
		t.Fatalf("password-confirmed set created %d PIN credentials", count)
	}
	// Changing the primary PIN intentionally revokes its administration proof.
	// Reconfirm the primary profile so clear still exercises password
	// reauthentication in addition to a fresh proof.
	if err := db.QueryRow(`SELECT pin_revision FROM profiles WHERE id = ?`, accountID).Scan(&pinRevision); err != nil {
		t.Fatalf("read updated primary profile PIN revision: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO local_profile_admin_proofs (id, token_hash, account_id, primary_profile_id, session_id, pin_revision, expires_at, created_at)
		VALUES ('proof_profile_password', ?, ?, ?, 'sess_profile_password', ?, ?, ?)`, hashToken(proofToken), accountID, accountID,
		pinRevision, now.Add(time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("refresh profile administration proof: %v", err)
	}
	if response := mutate(http.MethodDelete, `{"password":"wrong-password"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password clear status = %d, body = %s", response.Code, response.Body.String())
	}
	if count := credentialCount(); count != 1 {
		t.Fatalf("wrong-password clear changed PIN credential count to %d", count)
	}
	if response := mutate(http.MethodDelete, `{"password":"Password1234"}`); response.Code != http.StatusNoContent {
		t.Fatalf("password-confirmed clear status = %d, body = %s", response.Code, response.Body.String())
	}
	if count := credentialCount(); count != 0 {
		t.Fatalf("password-confirmed clear left %d PIN credentials", count)
	}
}
