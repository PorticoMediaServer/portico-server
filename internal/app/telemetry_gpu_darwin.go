//go:build darwin

package app

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const appleGPUUnavailableReason = "macOS did not expose GPU performance statistics for this hardware"

func detectPlatformGPUInfo() DashboardGPUInfo {
	raw, err := runTelemetryCommand(context.Background(), "ioreg", "-r", "-d", "1", "-w", "0", "-c", "IOAccelerator")
	if err != nil {
		return DashboardGPUInfo{Provider: "Unknown", Device: "GPU telemetry unavailable", Note: appleGPUUnavailableReason}
	}
	model := ioregStringValue(raw, "model")
	if model == "" {
		model = "Integrated macOS GPU"
	}
	provider := "Intel"
	if strings.Contains(strings.ToLower(model), "apple") {
		provider = "Apple Silicon"
	}
	if _, err := parseIOAcceleratorSample(raw); err != nil {
		return DashboardGPUInfo{Provider: provider, Device: model, Note: appleGPUUnavailableReason}
	}
	return DashboardGPUInfo{
		Provider:  provider,
		Device:    model,
		Available: true,
		Note:      "Collected from macOS IOAccelerator without elevated privileges. Encoder utilization is not exposed by this interface.",
	}
}

func samplePlatformGPU(ctx context.Context, info DashboardGPUInfo) telemetryGPUSample {
	if info.Provider != "Apple Silicon" && info.Provider != "Intel" {
		return unavailableGPUSample(telemetryStatusUnsupported, "This GPU is not exposed through macOS IOAccelerator")
	}
	raw, err := runTelemetryCommand(ctx, "ioreg", "-r", "-d", "1", "-w", "0", "-c", "IOAccelerator")
	if err != nil {
		return unavailableGPUSample(telemetryCommandMetricStatus(err), err.Error())
	}
	sample, err := parseIOAcceleratorSample(raw)
	if err != nil {
		return unavailableGPUSample(telemetryStatusParseError, err.Error())
	}
	return sample
}

func parseIOAcceleratorSample(raw string) (telemetryGPUSample, error) {
	usage, usageOK := ioregNumberValue(raw, "Device Utilization %")
	if !usageOK {
		renderer, rendererOK := ioregNumberValue(raw, "Renderer Utilization %")
		tiler, tilerOK := ioregNumberValue(raw, "Tiler Utilization %")
		if rendererOK || tilerOK {
			usage = math.Max(renderer, tiler)
			usageOK = true
		}
	}
	allocated, allocatedOK := ioregNumberValue(raw, "Alloc system memory")
	inUse, inUseOK := ioregNumberValue(raw, "In use system memory")
	if !usageOK && !(allocatedOK && inUseOK && allocated > 0) {
		return telemetryGPUSample{}, errors.New(appleGPUUnavailableReason)
	}
	sample := telemetryGPUSample{
		Usage:   unavailableTelemetryMetric(telemetryStatusUnsupported, "macOS did not report GPU utilization"),
		Memory:  unavailableTelemetryMetric(telemetryStatusUnsupported, "macOS did not report GPU memory usage"),
		Encoder: unavailableTelemetryMetric(telemetryStatusUnsupported, "macOS IOAccelerator does not expose encoder utilization"),
	}
	if usageOK {
		sample.Usage = availableTelemetryMetric(usage)
	}
	if allocatedOK && inUseOK && allocated > 0 {
		sample.Memory = availableTelemetryMetric(inUse / allocated * 100)
	}
	return sample, nil
}

func ioregStringValue(raw, key string) string {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*=\s*"([^"]{1,256})"`)
	match := pattern.FindStringSubmatch(raw)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func ioregNumberValue(raw, key string) (float64, bool) {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*=\s*([0-9]+(?:\.[0-9]+)?)`)
	match := pattern.FindStringSubmatch(raw)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}
