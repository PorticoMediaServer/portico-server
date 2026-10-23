package decoder

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// HardwareEnvironment describes this server, not the client's Settings row.os.
// A detected vendor is PCI discovery evidence, never a successful GPU probe.
type HardwareEnvironment struct {
	HostOS   string   `json:"hostOS"`
	Platform string   `json:"platform"`
	Vendors  []string `json:"vendors"`
}

func DetectHardwareEnvironment() HardwareEnvironment {
	return hardwareEnvironment(runtime.GOOS, os.Getenv("PORTICO_PLATFORM"), os.Getenv("PORTICO_CONTAINER") == "1", readDeviceFile, func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

func hardwareEnvironment(goos, installation string, container bool, read func(string) string, exists func(string) bool) HardwareEnvironment {
	hostOS := goos
	if goos == "darwin" {
		hostOS = "macos"
	}
	out := HardwareEnvironment{HostOS: hostOS, Platform: hostOS, Vendors: []string{}}
	if goos != "linux" {
		return out
	}
	// Install templates may identify their host explicitly. Unknown values do
	// not become platform names or user-visible remedy text.
	switch installation {
	case "unraid", "truenas", "synology", "proxmox":
		out.Platform = installation
	default:
		switch {
		case exists("/etc/unraid-version"):
			out.Platform = "unraid"
		case strings.Contains(strings.ToLower(read("/etc/version")), "truenas"):
			out.Platform = "truenas"
		case strings.Contains(read("/etc.defaults/VERSION"), "productversion="):
			out.Platform = "synology"
		case strings.TrimSpace(read("/run/systemd/container")) == "lxc":
			out.Platform = "lxc"
		case container || exists("/.dockerenv") || exists("/run/.containerenv"):
			out.Platform = "docker"
		case strings.Contains(read("/proc/device-tree/model"), "Raspberry Pi 5"):
			out.Platform = "raspberrypi5"
		case strings.Contains(strings.ToLower(read("/proc/device-tree/compatible")), "rockchip"):
			out.Platform = "rockchip"
		}
	}
	seen := map[string]bool{}
	devices, _ := filepath.Glob("/sys/bus/pci/devices/*")
	// PCI enumeration is bounded and reads only display-controller attributes.
	for i, device := range devices {
		if i >= 512 {
			break
		}
		if !strings.HasPrefix(strings.ToLower(read(filepath.Join(device, "class"))), "0x03") {
			continue
		}
		vendor := map[string]string{"0x8086": "Intel", "0x1002": "AMD", "0x10de": "NVIDIA"}[strings.ToLower(read(filepath.Join(device, "vendor")))]
		if vendor != "" && !seen[vendor] {
			seen[vendor] = true
			out.Vendors = append(out.Vendors, vendor)
		}
	}
	sort.Strings(out.Vendors)
	return out
}

func (e HardwareEnvironment) Remedy(backend HardwareBackend) string {
	if e.Platform == "docker" {
		nvidia, other := false, false
		for _, vendor := range e.Vendors {
			nvidia = nvidia || vendor == "NVIDIA"
			other = other || vendor == "Intel" || vendor == "AMD"
		}
		// A requested backend is not evidence that the host has that vendor.
		if nvidia && (backend == BackendNVENC || !other) {
			return HardwareRemedy("docker", BackendNVENC)
		}
		return HardwareRemedy("docker", BackendVAAPI)
	}
	return HardwareRemedy(e.Platform, backend)
}
