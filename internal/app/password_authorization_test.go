package app

import (
	"testing"
	"time"
)

func TestValidatePasswordSessionTxRequiresOwnedActiveDevice(t *testing.T) {
	server := newScannerTestServer(t)
	db := server.db
	now := time.Now().UTC()
	owner, err := server.createUser(UserRequest{
		Username: "password-owner", Email: "password-owner@example.test", DisplayName: "Password Owner",
		Password: "Password1234", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := server.createUser(UserRequest{
		Username: "password-other", Email: "password-other@example.test", DisplayName: "Password Other",
		Password: "Password1234", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create other account: %v", err)
	}
	for _, device := range []struct {
		id, userID, revokedAt string
	}{
		{id: "owned-active", userID: owner.ID},
		{id: "owned-revoked", userID: owner.ID, revokedAt: now.Format(time.RFC3339Nano)},
		{id: "foreign-active", userID: other.ID},
	} {
		if _, err := db.Exec(`
			INSERT INTO devices (id, user_id, name, app, platform, revoked_at, created_at, last_seen_at)
			VALUES (?, ?, 'test', 'test', 'test', ?, ?, ?)`, device.id, device.userID, device.revokedAt,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			t.Fatalf("seed device %s: %v", device.id, err)
		}
	}
	var passwordHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, owner.ID).Scan(&passwordHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}

	tests := []struct {
		name, deviceID string
		want           error
	}{
		{name: "unbound browser session", deviceID: ""},
		{name: "owned active device", deviceID: "owned-active"},
		{name: "missing device", deviceID: "missing-device", want: errPasswordCredentialChanged},
		{name: "foreign device", deviceID: "foreign-active", want: errPasswordCredentialChanged},
		{name: "revoked device", deviceID: "owned-revoked", want: errPasswordCredentialChanged},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sessionID := "password-session-" + string(rune('a'+index))
			if _, err := db.Exec(`
				INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, sessionID, owner.ID, owner.ID, test.deviceID, "token-"+sessionID,
				now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				t.Fatalf("seed session: %v", err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			got := validatePasswordSessionTx(tx, owner.ID, owner.ID, sessionID, passwordHash, now)
			_ = tx.Rollback()
			if test.want == nil && got != nil {
				t.Fatalf("validate active session: %v", got)
			}
			if test.want != nil && got != test.want {
				t.Fatalf("validation error=%v want=%v", got, test.want)
			}
		})
	}
}

func TestValidatePasswordSessionTxObservesDeviceRevocationBeforePrivilegeCommit(t *testing.T) {
	server := newScannerTestServer(t)
	db := server.db
	now := time.Now().UTC()
	user, err := server.createUser(UserRequest{
		Username: "password-revoke", Email: "password-revoke@example.test", DisplayName: "Password Revoke",
		Password: "Password1234", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO devices (id, user_id, name, app, platform, created_at, last_seen_at) VALUES ('revoke-device', ?, 'test', 'test', 'test', ?, ?)`,
		user.ID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at) VALUES ('revoke-session', ?, ?, 'revoke-device', 'revoke-token', ?, ?, ?)`,
		user.ID, user.ID, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, user.ID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE devices SET revoked_at = ? WHERE id = 'revoke-device'`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	privilegedCommit := func(key, expectedHash string) error {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := validatePasswordSessionTx(tx, user.ID, user.ID, "revoke-session", expectedHash, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES (?, '{}', ?)`, key, now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := privilegedCommit("forbidden-password-write", passwordHash); err != errPasswordCredentialChanged {
		t.Fatalf("password-authorized commit crossed device revocation: %v", err)
	}
	if err := privilegedCommit("forbidden-pin-write", ""); err != errPrivilegedSessionChanged {
		t.Fatalf("PIN-authorized commit crossed device revocation: %v", err)
	}
	var writes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key LIKE 'forbidden-%-write'`).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("revoked authority committed %d privileged writes", writes)
	}
}
