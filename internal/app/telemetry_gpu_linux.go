//go:build linux

package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var linuxDRMClassPath = "/sys/class/drm"

var linuxDRMCardName = regexp.MustCompile(`^card[0-9]+$`)

type linuxDRMGPU struct {
	card     string
	provider string
	device   string
}

func detectPlatformGPUInfo() DashboardGPUInfo {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		name := firstCSVField(commandString("nvidia-smi", "--query-gpu=name", "--format=csv,noheader,nounits"))
		if name != "" {
			return DashboardGPUInfo{Provider: "NVIDIA", Device: name, Available: true, Note: "Collected through the NVIDIA driver management interface."}
		}
	}
	gpus, err := discoverLinuxDRMGPUs(linuxDRMClassPath)
	if err != nil || len(gpus) == 0 {
		return DashboardGPUInfo{
			Provider: "Unknown", Device: "GPU telemetry unavailable",
			Note: linuxGPUExposureReason("Linux did not expose a supported DRM GPU device"),
		}
	}
	gpu := gpus[0]
	available := linuxDRMHasTelemetry(gpu.card)
	note := "Collected from the Linux DRM driver interface. Encoder utilization is not standardized by DRM."
	if !available {
		available, note = linuxDriverToolAvailable(gpu.provider)
		if !available {
			note = linuxGPUExposureReason(gpu.provider + " GPU detected, but its driver telemetry interface is not exposed")
		}
	}
	return DashboardGPUInfo{Provider: gpu.provider, Device: gpu.device, Available: available, Note: note}
}

func linuxDriverToolAvailable(provider string) (bool, string) {
	switch provider {
	case "AMD":
		if _, err := exec.LookPath("rocm-smi"); err == nil {
			return true, "Collected through the AMD driver management interface."
		}
		if _, err := exec.LookPath("amd-smi"); err == nil {
			return true, "Collected through the AMD driver management interface."
		}
	case "Intel":
		if _, err := exec.LookPath("intel_gpu_top"); err == nil {
			return true, "Collected through the Intel DRM driver management interface."
		}
	}
	return false, ""
}

func samplePlatformGPU(ctx context.Context, info DashboardGPUInfo) telemetryGPUSample {
	if err := ctx.Err(); err != nil {
		return unavailableGPUSample(telemetryStatusCanceled, err.Error())
	}
	gpus, err := discoverLinuxDRMGPUs(linuxDRMClassPath)
	if err != nil {
		return unavailableGPUSample(telemetryStatusUnavailable, err.Error())
	}
	var gpu linuxDRMGPU
	for _, candidate := range gpus {
		if candidate.provider == info.Provider {
			gpu = candidate
			break
		}
	}
	if gpu.card == "" {
		return unavailableGPUSample(telemetryStatusUnavailable, linuxGPUExposureReason(info.Provider+" GPU device is not exposed"))
	}
	sample := telemetryGPUSample{
		Usage:   unavailableTelemetryMetric(telemetryStatusUnsupported, "The DRM driver did not expose GPU utilization"),
		Memory:  unavailableTelemetryMetric(telemetryStatusUnsupported, "The DRM driver did not expose GPU memory usage"),
		Encoder: unavailableTelemetryMetric(telemetryStatusUnsupported, "The Linux DRM interface does not standardize encoder utilization"),
	}
	if value, ok := firstLinuxDRMNumber(gpu.card, "device/gpu_busy_percent", "device/gt_busy_percent"); ok {
		sample.Usage = availableTelemetryMetric(value)
	}
	used, usedOK := firstLinuxDRMNumber(gpu.card, "device/mem_info_vram_used")
	total, totalOK := firstLinuxDRMNumber(gpu.card, "device/mem_info_vram_total")
	if usedOK && totalOK && total > 0 {
		sample.Memory = availableTelemetryMetric(used / total * 100)
	}
	return sample
}

func discoverLinuxDRMGPUs(root string) ([]linuxDRMGPU, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	result := make([]linuxDRMGPU, 0, len(entries))
	for _, entry := range entries {
		if !linuxDRMCardName.MatchString(entry.Name()) {
			continue
		}
		card := filepath.Join(root, entry.Name())
		vendorRaw, err := readTelemetryFile(filepath.Join(card, "device/vendor"), 64)
		if err != nil {
			continue
		}
		provider := linuxGPUProvider(strings.TrimSpace(string(vendorRaw)))
		if provider == "" {
			continue
		}
		result = append(result, linuxDRMGPU{card: card, provider: provider, device: provider + " GPU (" + entry.Name() + ")"})
	}
	return result, nil
}

func linuxGPUProvider(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "0x10de":
		return "NVIDIA"
	case "0x1002":
		return "AMD"
	case "0x8086":
		return "Intel"
	default:
		return ""
	}
}

func linuxDRMHasTelemetry(card string) bool {
	for _, relative := range []string{"device/gpu_busy_percent", "device/gt_busy_percent", "device/mem_info_vram_used"} {
		if info, err := os.Stat(filepath.Join(card, relative)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func firstLinuxDRMNumber(card string, paths ...string) (float64, bool) {
	for _, relative := range paths {
		raw, err := readTelemetryFile(filepath.Join(card, relative), 128)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
			return value, true
		}
	}
	return 0, false
}

func linuxGPUExposureReason(reason string) string {
	return fmt.Sprintf("%s. In containers, the host GPU device and matching driver interfaces must be passed through; Portico cannot access hardware the container runtime does not expose", strings.TrimSuffix(reason, "."))
}
