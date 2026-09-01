//go:build !darwin && !linux && !windows

package app

import "context"

func detectPlatformGPUInfo() DashboardGPUInfo {
	return DashboardGPUInfo{Provider: "Unknown", Device: "GPU telemetry unavailable", Note: "This operating system does not expose a supported native GPU telemetry interface."}
}

func samplePlatformGPU(_ context.Context, info DashboardGPUInfo) telemetryGPUSample {
	return unavailableGPUSample(telemetryStatusUnsupported, info.Note)
}
