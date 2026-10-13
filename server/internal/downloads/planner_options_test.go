package downloads

import (
	"context"
	"encoding/json"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/testauth"
	"testing"
)

func TestDownloadMenuUsesDeviceDocumentAndConverterInventory(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	testauth.InsertSession(t, h.db, h.viewer.Hash, h.viewer.AccountID, h.viewer.ProfileID, h.viewer.Authority, h.viewer.Role, "2099-01-01T00:00:00Z")
	player := playback.New(h.db)
	doc := playback.BaselineCapabilityDocument()
	doc.Engines.Native.Evidence = playback.EvidenceDeclared
	doc.Engines.Native.Video[0].MaxHeight = 480
	raw, _ := json.Marshal(doc)
	if _, err := player.PublishCapabilities(ctx, h.viewer, raw); err != nil {
		t.Fatal(err)
	}
	view, err := h.service.Options(ctx, h.viewer, h.item, true)
	if err != nil {
		t.Fatal(err)
	}
	saw480 := false
	for _, option := range view.Options {
		if option.Quality == "480p" {
			saw480 = true
		}
		if option.Quality != "original" && option.TargetHeight > 480 {
			t.Fatal("offered a file this decoder cannot play", option)
		}
	}
	if !saw480 {
		t.Fatal(view.Options)
	}
	old := decoder.CurrentToolchain()
	decoder.ConfigureToolchain(decoder.ToolchainFacts{Encoders: map[string]bool{"libx265": true, "aac": true}})
	defer decoder.RestoreToolchain(old)
	view, err = h.service.Options(ctx, h.viewer, h.item, true)
	if err != nil || len(view.Options) != 1 || view.Options[0].Quality != "original" {
		t.Fatal(view, err)
	}
}
