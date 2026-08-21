package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestP04TelemetryAggregationSkipsNonSuccessfulMetrics(t *testing.T) {
	start := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	warming := unavailableTelemetryMetric(telemetryStatusWarmingUp, "counter interval is not ready")
	points := []systemTelemetryPoint{
		{
			At:              start.Add(time.Second),
			ServerCPU:       80,
			SystemCPU:       60,
			ServerRAM:       40,
			SystemRAM:       70,
			GPUUsage:        50,
			GPUMemory:       30,
			GPUEncoder:      20,
			ServerCPUState:  availableTelemetryMetric(80),
			SystemCPUState:  availableTelemetryMetric(60),
			ServerRAMState:  availableTelemetryMetric(40),
			SystemRAMState:  availableTelemetryMetric(70),
			GPUUsageState:   availableTelemetryMetric(50),
			GPUMemoryState:  availableTelemetryMetric(30),
			GPUEncoderState: availableTelemetryMetric(20),
		},
		{
			At:              start.Add(2 * time.Second),
			ServerCPUState:  warming,
			SystemCPUState:  warming,
			ServerRAMState:  warming,
			SystemRAMState:  warming,
			GPUUsageState:   warming,
			GPUMemoryState:  warming,
			GPUEncoderState: warming,
		},
	}

	result := averageTelemetryBucket(points, start, start.Add(10*time.Second))
	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{name: "server CPU", got: result.ServerCPU, want: 80},
		{name: "system CPU", got: result.SystemCPU, want: 60},
		{name: "server RAM", got: result.ServerRAM, want: 40},
		{name: "system RAM", got: result.SystemRAM, want: 70},
		{name: "GPU usage", got: result.GPUUsage, want: 50},
		{name: "GPU memory", got: result.GPUMemory, want: 30},
		{name: "GPU encoder", got: result.GPUEncoder, want: 20},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s average = %v, want %v", check.name, check.got, check.want)
		}
	}
	for _, metric := range []struct {
		name   string
		status telemetryMetric
	}{
		{name: "server CPU", status: result.ServerCPUState},
		{name: "system CPU", status: result.SystemCPUState},
		{name: "server RAM", status: result.ServerRAMState},
		{name: "system RAM", status: result.SystemRAMState},
		{name: "GPU usage", status: result.GPUUsageState},
		{name: "GPU memory", status: result.GPUMemoryState},
		{name: "GPU encoder", status: result.GPUEncoderState},
	} {
		if metric.status.Status != telemetryStatusWarmingUp || metric.status.Reason != warming.Reason {
			t.Errorf("%s status = %#v, want warming-up reason preserved", metric.name, metric.status)
		}
	}
}

func TestP04TelemetryAggregationDoesNotInventSuccessWithoutSuccessfulSamples(t *testing.T) {
	start := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	failed := unavailableTelemetryMetric(telemetryStatusUnavailable, "collector failed")
	result := averageTelemetryBucket([]systemTelemetryPoint{{
		At:              start.Add(time.Second),
		ServerCPU:       91,
		SystemCPU:       82,
		ServerRAM:       73,
		SystemRAM:       64,
		GPUUsage:        55,
		GPUMemory:       46,
		GPUEncoder:      37,
		ServerCPUState:  failed,
		SystemCPUState:  failed,
		ServerRAMState:  failed,
		SystemRAMState:  failed,
		GPUUsageState:   failed,
		GPUMemoryState:  failed,
		GPUEncoderState: failed,
	}}, start, start.Add(10*time.Second))

	if result.ServerCPU != 0 || result.SystemCPU != 0 || result.ServerRAM != 0 || result.SystemRAM != 0 || result.GPUUsage != 0 || result.GPUMemory != 0 || result.GPUEncoder != 0 {
		t.Fatalf("unavailable-only telemetry produced successful numeric values: %#v", result)
	}
	if result.ServerCPUState.Status != telemetryStatusUnavailable || result.ServerCPUState.Reason != failed.Reason {
		t.Fatalf("unavailable status was not preserved: %#v", result.ServerCPUState)
	}
}

func TestP04FailedGPUProbeIsCachedPerProviderAndCadence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("GPU collector command fixtures use Unix executable scripts")
	}
	directory := t.TempDir()
	callsPath := filepath.Join(directory, "nvidia-calls")
	nvidia := filepath.Join(directory, "nvidia-smi")
	if err := os.WriteFile(nvidia, []byte("#!/bin/sh\nprintf x >> \"$PORTICO_TEST_GPU_CALLS\"\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("write NVIDIA fixture: %v", err)
	}
	amd := filepath.Join(directory, "rocm-smi")
	if err := os.WriteFile(amd, []byte("#!/bin/sh\nprintf '%s\\n' '25% 35%'\n"), 0o700); err != nil {
		t.Fatalf("write AMD fixture: %v", err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PORTICO_TEST_GPU_CALLS", callsPath)

	state := &telemetrySamplerState{}
	nvidiaInfo := DashboardGPUInfo{Provider: "NVIDIA", Device: "fixture", Available: true}
	first := sampleGPUContext(context.Background(), state, nvidiaInfo)
	second := sampleGPUContext(context.Background(), state, nvidiaInfo)
	if first.Usage.Status != telemetryStatusUnavailable || second.Usage.Status != telemetryStatusUnavailable {
		t.Fatalf("failed NVIDIA probe status = %#v/%#v", first.Usage, second.Usage)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read NVIDIA call count: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("failed NVIDIA probe was not cached for the cadence: %d calls, want 1", len(calls))
	}

	state.gpu.At = time.Now().Add(-telemetryGPUCacheInterval - time.Millisecond)
	third := sampleGPUContext(context.Background(), state, nvidiaInfo)
	if third.Usage.Status != telemetryStatusUnavailable {
		t.Fatalf("expired failed NVIDIA probe status = %#v", third.Usage)
	}
	calls, err = os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read expired NVIDIA call count: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expired failed NVIDIA probe calls = %d, want 2", len(calls))
	}

	changed := sampleGPUContext(context.Background(), state, DashboardGPUInfo{Provider: "AMD", Device: "fixture", Available: true})
	if changed.Usage.Status != telemetryStatusOK || changed.Usage.Value != 25 {
		t.Fatalf("provider change reused the NVIDIA failure: %#v", changed.Usage)
	}
	if state.gpu.Provider != "AMD" {
		t.Fatalf("GPU cache provider = %q, want AMD", state.gpu.Provider)
	}
}

func TestP04TelemetryCollectionBudgetIncludesInjectedIOPressure(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	deadlineObserved := make(chan bool, 1)
	state := &telemetrySamplerState{
		ioPressureSampler: func(ctx context.Context) ioPressureSample {
			_, hasDeadline := ctx.Deadline()
			deadlineObserved <- hasDeadline
			select {
			case <-ctx.Done():
				return ioPressureSample{}
			case <-time.After(100 * time.Millisecond):
				return ioPressureSample{Status: ioPressureStatusUnavailable, Error: "injected IO sampler ignored its context"}
			}
		},
	}

	started := time.Now()
	point := sampleSystemTelemetryContext(parent, state, DashboardGPUInfo{Provider: "Unknown", Note: "test collector unavailable"})
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("collection exceeded bounded test budget: %s", elapsed)
	}
	select {
	case hasDeadline := <-deadlineObserved:
		if !hasDeadline {
			t.Fatal("IO sampler did not receive the collection deadline")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("injected IO sampler was not called")
	}
	if point.CollectionStatus != telemetryStatusCollectionTimeout {
		t.Fatalf("collection status = %q, want %q", point.CollectionStatus, telemetryStatusCollectionTimeout)
	}
	if point.IOPressure.Status != ioPressureStatusUnavailable || !strings.Contains(point.IOPressure.Error, "deadline") {
		t.Fatalf("IO pressure timeout was not explicit: %#v", point.IOPressure)
	}
}
