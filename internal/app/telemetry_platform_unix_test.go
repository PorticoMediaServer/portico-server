//go:build !windows

package app

import (
	"context"
	"runtime"
	"testing"
)

func TestLinuxTelemetryParsersUseCounterDeltas(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc counter sampling is Linux-specific")
	}
	total, busy, err := parseLinuxCPUStat("cpu 100 20 30 40 10 0 0 0 0 0\n")
	if err != nil || total != 200 || busy != 150 {
		t.Fatalf("Linux CPU parse = total %d busy %d err %v", total, busy, err)
	}
	process, err := parseLinuxProcessCPUStat("123 (portico server) S 0 0 0 0 0 0 0 0 0 0 11 7")
	if err != nil || process != 18 {
		t.Fatalf("Linux process CPU parse = %d err %v", process, err)
	}
	state := &telemetrySamplerState{}
	first, system := sampleLinuxCPU(context.Background(), state)
	if first.Status != telemetryStatusWarmingUp || system.Status != telemetryStatusWarmingUp {
		t.Fatalf("first Linux CPU sample = %#v/%#v, want warming-up", first, system)
	}
}
