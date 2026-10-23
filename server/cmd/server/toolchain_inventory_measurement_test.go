package main

import (
	"context"
	"os"
	"testing"
	"time"

	"portico.local/server/internal/decoder"
)

// An explicit installed tool is required: this does not substitute a system
// FFmpeg for the retained target whose startup cost is being measured.
func TestSelectedToolchainInventoryMeasurement(t *testing.T) {
	selected := os.Getenv("PORTICO_TEST_TOOLCHAIN_INVENTORY")
	if selected == "" {
		t.Skip("set PORTICO_TEST_TOOLCHAIN_INVENTORY to the exact selected executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	identity := ""
	for _, pass := range []string{"cold", "repeat"} {
		started := time.Now()
		facts, err := decoder.ProbeToolchain(ctx, selected)
		elapsed := time.Since(started)
		if err != nil || facts.Identity == "" || len(facts.MissingRequired()) != 0 {
			t.Fatal("actual selected inventory unavailable", err, facts.MissingRequired())
		}
		if pass == "repeat" && facts.Identity != identity {
			t.Fatal("selected executable changed between measurements")
		}
		if facts.VulkanAvailable || facts.OpenCLAvailable || facts.DolbyVisionProfile7To81 || facts.DolbyVisionDualLayerJoin {
			t.Fatal("inventory granted an unperformed optional qualification")
		}
		identity = facts.Identity
		t.Logf("selected inventory %s elapsed=%s encoders=%d decoders=%d filters=%d muxers=%d", pass, elapsed, len(facts.Encoders), len(facts.Decoders), len(facts.Filters), len(facts.Muxers))
	}
}
