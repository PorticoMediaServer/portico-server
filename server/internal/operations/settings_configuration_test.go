package operations

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/persistence"
)

func TestSettingsConfigurationFileRuntimeAndDurableProjection(t *testing.T) {
	s, owner, authorize := consoleFixture(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "portico.toml")
	if err := os.WriteFile(path, []byte("[general]\nname='Library room'\n[streaming]\nupload_speed=25\n[updates]\nchannel='stable'\n[unrelated]\nkept=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeConfiguration(ctx, path); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Settings(ctx, authorize)
	if err != nil || doc.Requested.Name != "Library room" || doc.Requested.UploadCapacityKbps != 25000 {
		t.Fatal(doc, err)
	}
	config, err := s.Configuration(ctx, authorize)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, field := range config.Fields {
		if field.ID == "streaming.uploadSpeed" {
			found = true
			if field.Current != float64(25) || field.Default != nil || field.Source != "file" {
				t.Fatal(field)
			}
		}
	}
	if !found {
		t.Fatal("upload missing")
	}
	values := doc.Requested
	values.UploadCapacityKbps = 0
	values.OwnAddressEnabled = false
	if _, err = s.ApplySettings(ctx, owner, authorize, SettingsChange{doc.Revision, "write-empty-upload", values}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kept = true") {
		t.Fatal("unrelated configuration lost", string(raw))
	}
	var pending, graphs int
	if err = s.DB.QueryRow(`SELECT pending_revision FROM server_configuration_state WHERE singleton=1`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal(pending, err)
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM console_documents WHERE scope='server'`).Scan(&graphs); err != nil || graphs != 0 {
		t.Fatal("serialized server settings persisted", graphs, err)
	}
	// Simulate a process ending after its DB commit but before the file export.
	s.ConfigFile = nil
	current, _ := s.Settings(ctx, authorize)
	values = current.Requested
	values.Name = "After restart"
	if _, err = s.ApplySettings(ctx, owner, authorize, SettingsChange{current.Revision, "pending-projection", values}); err != nil {
		t.Fatal(err)
	}
	reloaded := New(s.DB)
	if err = reloaded.InitializeConfiguration(ctx, path); err != nil {
		t.Fatal(err)
	}
	current, err = reloaded.Settings(ctx, authorize)
	if err != nil || current.Requested.Name != "After restart" {
		t.Fatal(current, err)
	}
	// Invalid external values change neither the stored settings nor the revision.
	before := current.Revision
	if err = os.WriteFile(path, []byte("[remote]\nown_certificate=true\ncertificate_path='/cert.pem'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = reloaded.ConfigFile.Reload(); err != nil {
		t.Fatal(err)
	}
	if err = reloaded.ImportConfiguration(ctx); err == nil {
		t.Fatal("incomplete enabled certificate accepted")
	}
	current, err = reloaded.Settings(ctx, authorize)
	if err != nil || current.Revision != before || current.Requested.OwnAddressEnabled {
		t.Fatal("invalid config mutated runtime", current, err)
	}
	// The normal current-baseline opener accepts this normalized state.
	var dbPath string
	if err = s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := persistence.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
}
func TestOwnAddressStartsOffAndCanKeepIncompleteDraft(t *testing.T) {
	v := DefaultSettings()
	if v.OwnAddressEnabled {
		t.Fatal("own address must default off")
	}
	v.CustomCertificateDomain = "media.example.com"
	if err := validateSettings(&v); err != nil {
		t.Fatal("disabled draft refused", err)
	}
	v.OwnAddressEnabled = true
	if err := validateSettings(&v); err == nil {
		t.Fatal("enabled incomplete certificate accepted")
	}
	v.CustomCertificatePath = "/config/cert.pem"
	v.CustomCertificateKeyPath = "/config/key.pem"
	if err := validateSettings(&v); err != nil {
		t.Fatal(err)
	}
}
