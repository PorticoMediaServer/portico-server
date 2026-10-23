package httpapi

import (
	"context"
	"testing"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

func TestRegistryDeliverySettingsCanonicalMediaPolicy(t *testing.T) {
	f := administrationFixtureNew(t)
	ctx := context.Background()
	document, err := f.deps.Console.Settings(ctx, operations.AllowServerScope)
	if err != nil {
		t.Fatal(err)
	}
	values := document.Requested
	values.ConversionMode, values.OutputFormat, values.DownloadMaxQuality = "sound", "h264", "720p-high"
	owner := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "owner", ProfileID: "owner-profile", Role: "owner"}}
	if _, err := f.deps.Console.ApplySettings(ctx, owner, operations.AllowServerScope, operations.SettingsChange{ExpectedRevision: document.Revision, IdempotencyKey: "canonical-media-policy", Values: values}); err != nil {
		t.Fatal(err)
	}
	adapter := &RegistryDeliverySettings{Console: f.deps.Console}
	adapter.Refresh(ctx)
	delivery := adapter.Delivery()
	if delivery.ConversionMode != "sound" || delivery.OutputFormat != "h264" || delivery.DownloadMaxQuality != "720p-high" {
		t.Fatal("owner policy did not reach actual delivery adapter", delivery)
	}
}
