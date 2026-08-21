package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestSecretSettingWithErrorDoesNotTurnProviderFailureIntoAuthority(t *testing.T) {
	appDataDir := t.TempDir()
	cfg := config.Config{AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db")}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	server := &Server{cfg: cfg, db: db}
	keyPath := filepath.Join(appDataDir, "keys", "hosted-authority.key")
	server.secretProvider = NewLocalSecretKeyProvider(keyPath)
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "authority-value"); err != nil {
		t.Fatalf("save secret setting: %v", err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatalf("remove provider key: %v", err)
	}

	if got := server.secretSetting(remoteAccessCredentialKey); got != "" {
		t.Fatalf("secretSetting returned authority after provider failure: %q", got)
	}
	got, err := server.secretSettingWithError(remoteAccessCredentialKey)
	if got != "" || !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("secretSettingWithError value=%q error=%v", got, err)
	}
	var providerErr *SecretProviderError
	if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateMissing {
		t.Fatalf("secretSettingWithError provider error = %#v", providerErr)
	}
}
