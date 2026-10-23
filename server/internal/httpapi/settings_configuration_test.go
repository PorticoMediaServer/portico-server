package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/operations"
)

func TestSettingsConfigurationOwnerReadAndCommittedRuntimeRefresh(t *testing.T) {
	f := administrationFixtureNew(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "portico.toml")
	if err := os.WriteFile(path, []byte("[streaming]\nmusic_away_quality='96'\nallow_conversion='sound'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.deps.Console.InitializeConfiguration(ctx, path); err != nil {
		t.Fatal(err)
	}
	adapter := &RegistryDeliverySettings{Console: f.deps.Console}
	adapter.Refresh(ctx)
	calls := 0
	f.deps.SettingsApplied = func(ctx context.Context) { calls++; adapter.Refresh(ctx) }
	f.handler = New(f.deps)
	for _, token := range []string{"", f.admin, f.member} {
		response := f.call("GET", "/v1/admin/settings/configuration", token, "")
		if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
			t.Fatalf("non-owner configuration read: %d %s", response.Code, response.Body)
		}
	}
	response := f.call("GET", "/v1/admin/settings/configuration", f.owner, "")
	if response.Code != http.StatusOK {
		t.Fatalf("configuration read: %d %s", response.Code, response.Body)
	}
	var envelope ConsoleReply[operations.SettingsConfiguration]
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, field := range envelope.Data.Fields {
		if field.ID == "streaming.musicAwayQuality" {
			found = field.Source == "file" && field.Current != nil && *field.Current == "96" && field.Default != nil && *field.Default == "original"
		}
	}
	if !found || envelope.Data.Path != path {
		t.Fatal("configuration omitted real source/default", envelope.Data)
	}
	if err := os.WriteFile(path, []byte("[streaming]\nmusic_away_quality='320'\nallow_conversion='never'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.deps.Console.ConfigFile.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := f.deps.Console.ImportConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if current := adapter.Delivery(); calls != 1 || current.MaxMusicQuality != "320" || current.ConversionMode != "never" {
		t.Fatal("file edit did not refresh current delivery policy", calls, current)
	}
	doc, err := f.deps.Console.Settings(ctx, operations.AllowServerScope)
	if err != nil {
		t.Fatal(err)
	}
	values := doc.Requested
	values.MusicAwayQuality, values.ConversionMode = "original", "all"
	body, err := json.Marshal(operations.SettingsChange{ExpectedRevision: doc.Revision, IdempotencyKey: "configuration-runtime-refresh", Values: values})
	if err != nil {
		t.Fatal(err)
	}
	response = f.call("PATCH", "/v1/admin/console/settings", f.owner, string(body))
	if response.Code != http.StatusOK {
		t.Fatalf("settings save: %d %s", response.Code, response.Body)
	}
	if current := adapter.Delivery(); calls != 2 || current.MaxMusicQuality != "original" || current.ConversionMode != "all" {
		t.Fatal("owner save did not refresh current delivery exactly once", calls, current)
	}
}
