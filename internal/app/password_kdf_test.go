package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestAccountPasswordPolicyAndKDF(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "valid exact minimum", password: "Portico1", valid: true},
		{name: "special satisfies final class", password: "Portico!", valid: true},
		{name: "missing uppercase", password: "portico1", valid: false},
		{name: "missing lowercase", password: "PORTICO1", valid: false},
		{name: "missing number or special", password: "PorticoPass", valid: false},
		{name: "too short", password: "Port1!", valid: false},
		{name: "maximum bcrypt input", password: "A1" + strings.Repeat("x", accountPasswordMaxBytes-2), valid: true},
		{name: "over bcrypt input", password: strings.Repeat("x", accountPasswordMaxBytes+1), valid: false},
		{name: "unicode byte bound", password: strings.Repeat("é", 37), valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validAccountPassword(test.password); got != test.valid {
				t.Fatalf("validAccountPassword()=%t want=%t", got, test.valid)
			}
		})
	}

	hash, err := hashAccountPassword(t.Context(), kdfAccountSetupHash, "Correct horse battery staple1")
	if err != nil {
		t.Fatalf("hash current password: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != currentPasswordBcryptCost {
		t.Fatalf("current password cost=%d err=%v", cost, err)
	}
	if valid, err := verifyAccountPassword(t.Context(), kdfBrowserLoginCompare, hash, "Correct horse battery staple1"); !valid || err != nil {
		t.Fatalf("current hash valid=%t err=%v", valid, err)
	}
	noncanonicalHash, err := bcrypt.GenerateFromPassword([]byte("Correct horse battery staple1"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash noncanonical password: %v", err)
	}
	if valid, err := verifyAccountPassword(t.Context(), kdfBrowserLoginCompare, string(noncanonicalHash), "Correct horse battery staple1"); valid || err != nil {
		t.Fatalf("noncanonical hash authenticated valid=%t err=%v", valid, err)
	}
	if valid, err := verifyAccountPassword(t.Context(), kdfBrowserLoginCompare, "", "wrong password"); valid || err != nil {
		t.Fatalf("empty hash authenticated")
	}
	if valid, err := verifyAccountPassword(t.Context(), kdfBrowserLoginCompare, "malformed", "wrong password"); valid || err != nil {
		t.Fatalf("malformed hash authenticated")
	}
}

func TestPasswordVerificationSnapshotFencesSessionIssuance(t *testing.T) {
	server := newScannerTestServer(t)
	account, err := server.createUser(UserRequest{
		Username: "password-cas", Email: "password-cas@example.test", DisplayName: "Password CAS",
		Password: "Password-cas-original1", Role: "user",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	verified, err := server.authenticateLocalNativeUser(t.Context(), account.Username, "Password-cas-original1")
	if err != nil {
		t.Fatalf("verify original password: %v", err)
	}
	if verified.verifiedPasswordHash == "" {
		t.Fatal("authentication did not carry an exact credential snapshot")
	}
	replacement, err := hashAccountPassword(t.Context(), kdfPasswordChangeHash, "Password-cas-replacement2")
	if err != nil {
		t.Fatalf("hash replacement: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, replacement, account.ID); err != nil {
		t.Fatalf("replace credential: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/auth/native", nil)
	request.Header.Set("User-Agent", "Portico password CAS test")
	_, err = server.issueNativeSessionCredentials(request, verified, "local", nativeDeviceDescriptor{
		InstallationID: "password-cas-native", Name: "CAS", App: "test", Platform: "test",
	})
	if !errors.Is(err, errPasswordCredentialChanged) {
		t.Fatalf("native credential crossed password replacement: %v", err)
	}

	browserRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	browserRequest.Header.Set("User-Agent", "Portico password CAS browser test")
	_, err = server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), browserRequest, account.ID, "local", sessionCreateOptions{
		ExpectedLocalPasswordHash: verified.verifiedPasswordHash,
	})
	if !errors.Is(err, errPasswordCredentialChanged) {
		t.Fatalf("browser session crossed password replacement: %v", err)
	}
	var sessions, refreshTokens int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, account.ID).Scan(&sessions); err != nil {
		t.Fatalf("count browser sessions: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM native_refresh_tokens WHERE user_id = ?`, account.ID).Scan(&refreshTokens); err != nil {
		t.Fatalf("count native tokens: %v", err)
	}
	if sessions != 0 || refreshTokens != 0 {
		t.Fatalf("stale verification minted sessions=%d refreshTokens=%d", sessions, refreshTokens)
	}
}
