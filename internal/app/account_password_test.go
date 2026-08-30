package app

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestAccountPasswordChangeRequiresCurrentPasswordAndUpdatesLocalCredential(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var originalHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&originalHash); err != nil {
		t.Fatalf("read original password hash: %v", err)
	}

	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/account/password", AccountPasswordChangeRequest{
		CurrentPassword: "wrong-password",
		NewPassword:     "Different-password-5678",
	}, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, "invalid_credentials") {
		t.Fatalf("wrong current password status=%d body=%s", status, body)
	}
	var unchangedHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&unchangedHash); err != nil || unchangedHash != originalHash {
		t.Fatalf("wrong current password changed stored hash: hashChanged=%v err=%v", unchangedHash != originalHash, err)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/account/password", AccountPasswordChangeRequest{
		CurrentPassword: "Password1234",
		NewPassword:     "too-short",
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "invalid_password") {
		t.Fatalf("weak replacement password status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/account/password", AccountPasswordChangeRequest{
		CurrentPassword: "Password1234",
		NewPassword:     "Password1234",
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "password_unchanged") {
		t.Fatalf("unchanged password status=%d body=%s", status, body)
	}

	const replacement = "Different-password-5678"
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/account/password", AccountPasswordChangeRequest{
		CurrentPassword: "Password1234",
		NewPassword:     replacement,
	}, nil)
	if status != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("change password status=%d body=%s", status, body)
	}

	var userHash string
	var credentialHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&userHash); err != nil {
		t.Fatalf("read changed user password hash: %v", err)
	}
	if err := db.QueryRow(`SELECT password_hash FROM local_credentials WHERE id = 'cred_local_' || (SELECT id FROM users WHERE username = 'admin') AND revoked_at = ''`).Scan(&credentialHash); err != nil {
		t.Fatalf("read changed local credential hash: %v", err)
	}
	if userHash == originalHash || credentialHash != userHash {
		t.Fatalf("password change did not atomically update compatibility and identity stores")
	}
	if valid, err := verifyAccountPassword(t.Context(), kdfPasswordChangeCompare, userHash, replacement); !valid || err != nil {
		t.Fatalf("replacement password does not verify")
	}
	if valid, err := verifyAccountPassword(t.Context(), kdfPasswordChangeCompare, userHash, "Password1234"); valid || err != nil {
		t.Fatalf("previous password still verifies")
	}

	assertAuthenticated(t, client, serverURL, true)
	oldPasswordClient := &http.Client{}
	status, _ = doJSON(t, oldPasswordClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "admin", "password": "Password1234"}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("previous password login status=%d, expected unauthorized", status)
	}
	status, body = doJSON(t, oldPasswordClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "admin", "password": replacement}, nil)
	if status != http.StatusOK {
		t.Fatalf("replacement password login status=%d body=%s", status, body)
	}

	assertPasswordAuditContainsNoSecrets(t, db, "Password1234", replacement)
}

func TestAccountPasswordChangeRejectsAPIKeys(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var created APIKeyCreateResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/api-keys", APIKeyCreateRequest{Name: "Credential Test", Scopes: []string{"read"}}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create API key status=%d body=%s", status, body)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/account/password", strings.NewReader(`{"currentPassword":"Password1234","newPassword":"Different-password-5678"}`))
	if err != nil {
		t.Fatalf("create password request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+created.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send password request: %v", err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden || (!strings.Contains(string(responseBody), "interactive_session_required") && !strings.Contains(string(responseBody), "api_key_scope_denied")) {
		t.Fatalf("API key password change status=%d body=%s", resp.StatusCode, responseBody)
	}
}

func assertPasswordAuditContainsNoSecrets(t *testing.T, db *sql.DB, secrets ...string) {
	t.Helper()
	rows, err := db.Query(`SELECT action, metadata_json FROM audit_events WHERE action IN ('account.password_change_failed', 'account.password_changed') ORDER BY created_at`)
	if err != nil {
		t.Fatalf("query password audit events: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var action string
		var metadata string
		if err := rows.Scan(&action, &metadata); err != nil {
			t.Fatalf("scan password audit event: %v", err)
		}
		seen[action] = true
		for _, secret := range secrets {
			if strings.Contains(metadata, secret) {
				t.Fatalf("audit metadata for %s contains a password", action)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read password audit events: %v", err)
	}
	if !seen["account.password_change_failed"] || !seen["account.password_changed"] {
		t.Fatalf("missing password audit events: %#v", seen)
	}
}
