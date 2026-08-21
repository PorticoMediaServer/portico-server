package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	hash, err := hashAccountPassword("Correct horse battery staple1")
	if err != nil {
		t.Fatalf("hash current password: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != currentPasswordBcryptCost {
		t.Fatalf("current password cost=%d err=%v", cost, err)
	}
	if valid, upgrade := verifyAccountPassword(hash, "Correct horse battery staple1"); !valid || upgrade {
		t.Fatalf("current hash valid=%t upgrade=%t", valid, upgrade)
	}
	legacyHash, err := bcrypt.GenerateFromPassword([]byte("Correct horse battery staple1"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash legacy password: %v", err)
	}
	if valid, upgrade := verifyAccountPassword(string(legacyHash), "Correct horse battery staple1"); !valid || !upgrade {
		t.Fatalf("legacy hash valid=%t upgrade=%t", valid, upgrade)
	}
	if valid, _ := verifyAccountPassword("", "wrong password"); valid {
		t.Fatalf("empty hash authenticated")
	}
	if valid, _ := verifyAccountPassword("malformed", "wrong password"); valid {
		t.Fatalf("malformed hash authenticated")
	}
}

func TestBrowserAndNativeLoginUpgradeLegacyPasswordCost(t *testing.T) {
	server := newScannerTestServer(t)
	db := server.db
	password := "Password1234"
	now := time.Now().UTC().Format(time.RFC3339)
	initialHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash initial password: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_kdf', 'admin', 'admin@example.test', 'Admin', ?, 'owner', 'local', '{}', '{}', ?, ?)`, string(initialHash), now, now); err != nil {
		t.Fatalf("insert KDF user: %v", err)
	}
	setLegacyHash := func() string {
		t.Helper()
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("hash legacy password: %v", err)
		}
		if _, err := db.Exec(`UPDATE users SET password_hash = ? WHERE username = 'admin'`, string(hash)); err != nil {
			t.Fatalf("seed legacy password hash: %v", err)
		}
		return string(hash)
	}
	storedCost := func() int {
		t.Helper()
		var hash string
		if err := db.QueryRow(`SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&hash); err != nil {
			t.Fatalf("load password hash: %v", err)
		}
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil {
			t.Fatalf("inspect password cost: %v", err)
		}
		return cost
	}

	legacyHash := setLegacyHash()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"login":"admin","password":"wrong-password"}`))
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	server.handleLogin(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password login status=%d body=%s", response.Code, response.Body.String())
	}
	var unchanged string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username = 'admin'`).Scan(&unchanged); err != nil || unchanged != legacyHash {
		t.Fatalf("failed login changed hash err=%v", err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"login":"admin","password":"Password1234"}`))
	request.RemoteAddr = "127.0.0.1:12346"
	response = httptest.NewRecorder()
	server.handleLogin(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("browser login status=%d body=%s", response.Code, response.Body.String())
	}
	if cost := storedCost(); cost != currentPasswordBcryptCost {
		t.Fatalf("browser login upgraded cost=%d", cost)
	}

	setLegacyHash()
	user, err := server.authenticateLocalNativeUser(context.Background(), "admin", password)
	if err != nil || user.ID != "usr_kdf" {
		t.Fatalf("native login user=%#v err=%v", user, err)
	}
	if cost := storedCost(); cost != currentPasswordBcryptCost {
		t.Fatalf("native login upgraded cost=%d", cost)
	}
}
