//go:build linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxDRMSampleUsesKernelTelemetry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	card := filepath.Join(root, "card0", "device")
	if err := os.MkdirAll(card, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"vendor": "0x1002\n", "gpu_busy_percent": "47\n", "mem_info_vram_used": "25\n", "mem_info_vram_total": "100\n",
	} {
		if err := os.WriteFile(filepath.Join(card, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	previous := linuxDRMClassPath
	linuxDRMClassPath = root
	t.Cleanup(func() { linuxDRMClassPath = previous })

	info := detectPlatformGPUInfo()
	if info.Provider != "AMD" || !info.Available {
		t.Fatalf("GPU info = %#v", info)
	}
	sample := samplePlatformGPU(context.Background(), info)
	if sample.Usage.Value != 47 || sample.Memory.Value != 25 {
		t.Fatalf("DRM sample = %#v", sample)
	}
	if sample.Encoder.Status != telemetryStatusUnsupported {
		t.Fatalf("encoder status = %#v", sample.Encoder)
	}
}

func TestLinuxDRMDetectionExplainsContainerPassthrough(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	previous := linuxDRMClassPath
	linuxDRMClassPath = root
	t.Cleanup(func() { linuxDRMClassPath = previous })
	info := detectPlatformGPUInfo()
	if info.Available || info.Provider != "Unknown" {
		t.Fatalf("GPU info = %#v", info)
	}
	if want := "container runtime does not expose"; !strings.Contains(info.Note, want) {
		t.Fatalf("note = %q, want %q", info.Note, want)
	}
}
