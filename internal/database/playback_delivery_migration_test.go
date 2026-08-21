package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestReleaseBaselineEnforcesPlaybackReceiverProfileAuthority(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	db, err := Open(config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")})
	if err != nil {
		t.Fatalf("open release database: %v", err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, account := range []struct{ id, username, email string }{
		{id: "account_receiver_a", username: "receiver-a", email: "receiver-a@example.test"},
		{id: "account_receiver_b", username: "receiver-b", email: "receiver-b@example.test"},
	} {
		if _, err := db.Exec(`
			INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
			VALUES (?, ?, ?, 'Receiver authority', 'user', '{}', '{}', ?, ?)`, account.id, account.username, account.email, now, now); err != nil {
			t.Fatalf("insert account %s: %v", account.id, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO playback_receivers (id, user_id, profile_id, name, code, created_at, last_seen_at)
		VALUES ('receiver_valid', 'account_receiver_a', 'account_receiver_a', 'Valid receiver', 'valid-code', ?, ?)`, now, now); err != nil {
		t.Fatalf("baseline rejected truthful receiver profile: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO playback_receivers (id, user_id, profile_id, name, code, created_at, last_seen_at)
		VALUES ('receiver_cross_account', 'account_receiver_a', 'account_receiver_b', 'Cross account', 'cross-code', ?, ?)`, now, now); err == nil {
		t.Fatal("baseline accepted a cross-account receiver profile")
	}
}
