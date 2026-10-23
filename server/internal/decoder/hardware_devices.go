package decoder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HardwareDevice is discovery evidence, not a capability claim. Only a real
// codec probe may set HardwareProbe.Available or an encode/decode cell.
type HardwareDevice struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Path          string          `json:"path"`
	Backend       HardwareBackend `json:"backend"`
	Vendor        string          `json:"vendor,omitempty"`
	Driver        string          `json:"driver,omitempty"`
	DriverVersion string          `json:"driverVersion,omitempty"`
	Discrete      bool            `json:"discrete"`
	SessionLimit  int             `json:"sessionLimit"`
	LimitSource   string          `json:"limitSource"`
}

func readDeviceFile(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

func discoverDRM(root, devices string) []HardwareDevice {
	nodes, _ := filepath.Glob(filepath.Join(devices, "renderD*"))
	out := []HardwareDevice{}
	for _, node := range nodes {
		sys := filepath.Join(root, "class/drm", filepath.Base(node), "device")
		vendor := readDeviceFile(filepath.Join(sys, "vendor"))
		pci := readDeviceFile(filepath.Join(sys, "device"))
		driver, _ := filepath.EvalSymlinks(filepath.Join(sys, "driver"))
		name := map[string]string{"0x8086": "Intel", "0x1002": "AMD", "0x10de": "NVIDIA"}[vendor]
		if name == "" {
			name = "Graphics device"
		}
		// PCI ids remain explicit when the system has no human-readable product
		// database. Never invent a GPU generation from an unrecognised device id.
		product := readDeviceFile(filepath.Join(sys, "label"))
		if product == "" {
			product = name + " " + pci
		}
		discrete := vendor == "0x1002" || vendor == "0x10de"
		if memory := readDeviceFile(filepath.Join(sys, "mem_info_vram_total")); memory != "" {
			v, _ := strconv.ParseUint(memory, 10, 64)
			discrete = v > 0
		}
		for _, backend := range []HardwareBackend{BackendVAAPI, BackendQSV} {
			if backend == BackendQSV && vendor != "0x8086" {
				continue
			}
			out = append(out, HardwareDevice{ID: string(backend) + ":" + node, Name: product, Path: node, Backend: backend, Vendor: vendor, Driver: filepath.Base(driver), Discrete: discrete, SessionLimit: 4, LimitSource: "conservative_default"})
		}
	}
	return out
}

func boundedDeviceOutput(ctx context.Context, executable string, args ...string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, executable, args...)
	cmd.WaitDelay = time.Second
	out := &toolOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	return out.Bytes(), err
}

func discoverCUDA(ctx context.Context) []HardwareDevice {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}
	raw, err := boundedDeviceOutput(ctx, path, "--query-gpu=index,name,driver_version", "--format=csv,noheader,nounits")
	if err != nil {
		return nil
	}
	out := []HardwareDevice{}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Split(line, ",")
		if len(parts) != 3 {
			continue
		}
		index := strings.TrimSpace(parts[0])
		if _, err := strconv.Atoi(index); err != nil {
			continue
		}
		// A driver version does not reliably identify qualified cards or virtual
		// GPU licensing. Three is the conservative unknown-driver aggregate cap;
		// a bounded concurrent-session qualification may raise it explicitly.
		out = append(out, HardwareDevice{ID: "nvenc:" + index, Name: strings.TrimSpace(parts[1]), Path: index, Backend: BackendNVENC, Vendor: "NVIDIA", Driver: "nvidia", DriverVersion: strings.TrimSpace(parts[2]), Discrete: true, SessionLimit: 3, LimitSource: "unqualified_driver_conservative_system_cap"})
	}
	return out
}

func discoverWindows(ctx context.Context) []HardwareDevice {
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		return nil
	}
	raw, err := boundedDeviceOutput(ctx, path, "-NoProfile", "-NonInteractive", "-Command", "[Console]::OutputEncoding=[System.Text.UTF8Encoding]::new(); @(Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID,DriverVersion) | ConvertTo-Json -Compress")
	if err != nil {
		return nil
	}
	var adapters []struct{ Name, PNPDeviceID, DriverVersion string }
	if json.Unmarshal(raw, &adapters) != nil {
		return nil
	}
	out := []HardwareDevice{}
	for i, a := range adapters {
		backends := []HardwareBackend{}
		id := strings.ToUpper(a.PNPDeviceID)
		if strings.Contains(id, "VEN_8086") {
			backends = append(backends, BackendQSV)
		}
		if strings.Contains(id, "VEN_1002") {
			backends = append(backends, BackendAMF)
		}
		for _, b := range backends {
			out = append(out, HardwareDevice{ID: fmt.Sprintf("%s:%d", b, i), Name: a.Name, Path: strconv.Itoa(i), Backend: b, DriverVersion: a.DriverVersion, Discrete: b == BackendAMF, SessionLimit: 4, LimitSource: "conservative_default"})
		}
	}
	return out
}

func DiscoverHardwareDevices(ctx context.Context) []HardwareDevice {
	out := []HardwareDevice{}
	switch runtime.GOOS {
	case "linux":
		out = append(out, discoverDRM("/sys", "/dev/dri")...)
	case "windows":
		out = append(out, discoverWindows(ctx)...)
	case "darwin":
		out = append(out, HardwareDevice{ID: "videotoolbox:default", Name: "Apple VideoToolbox", Backend: BackendVideoToolbox, SessionLimit: 4, LimitSource: "conservative_default"})
	}
	out = append(out, discoverCUDA(ctx)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Discrete != out[j].Discrete {
			return out[i].Discrete
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (d *HardwareDetector) candidates(ctx context.Context, configured, device string) []HardwareDevice {
	if configured == "" {
		configured = "auto"
	}
	var devices []HardwareDevice
	if d.Devices != nil {
		devices = d.Devices(ctx)
	} else {
		d.mu.Lock()
		devices = append(devices, d.discovered...)
		d.mu.Unlock()
		if devices == nil {
			devices = DiscoverHardwareDevices(ctx)
			d.mu.Lock()
			d.discovered = append([]HardwareDevice{}, devices...)
			d.mu.Unlock()
		}
	}
	out := []HardwareDevice{}
	for _, found := range devices {
		if configured != "auto" && string(found.Backend) != configured {
			continue
		}
		if device != "" && found.Path != device && found.ID != device {
			continue
		}
		out = append(out, found)
	}
	// If an owner named a device, or discovery is unavailable, attempt actual
	// backend probes rather than inventing discovery as a capability gate.
	if len(out) == 0 {
		for _, backend := range HardwareBackends {
			if configured != "auto" && configured != string(backend) {
				continue
			}
			out = append(out, HardwareDevice{ID: string(backend) + ":" + device, Path: device, Backend: backend, SessionLimit: 4, LimitSource: "conservative_default"})
		}
	}
	return out
}

// HardwareRemedy is the canonical plain platform instruction. An empty macOS
// remedy is intentional; failures there retain their actual diagnostic output.
func HardwareRemedy(platform string, backend HardwareBackend) string {
	switch platform {
	case "unraid":
		return "Install the Intel GPU TOP or Nvidia Driver plugin, then turn on the GPU in Portico's template."
	case "truenas":
		return "Turn on NVIDIA drivers in Apps settings, then choose the GPU in Portico's app settings."
	case "synology":
		return "Map /dev/dri in Container Manager. Only Intel models have a usable GPU."
	case "proxmox":
		return "Pass the render device through to this container."
	case "windows":
		return "Install the latest graphics driver from Intel, NVIDIA or AMD."
	case "raspberrypi5":
		return "This board has no video encoder, so the processor converts video."
	case "rockchip":
		return "Needs a Rockchip kernel. See the guide."
	case "darwin":
		return ""
	case "docker":
		if backend == BackendNVENC {
			return "Install the NVIDIA Container Toolkit, then start this container with the GPU (--gpus all)."
		}
		return "Give Portico your graphics card: add the device /dev/dri to this container."
	default:
		return "Install the graphics driver and allow Portico to use the render device."
	}
}

func (d *HardwareDetector) cachedCandidates(configured, device string) []HardwareDevice {
	if d.Devices != nil {
		return d.candidates(context.Background(), configured, device)
	}
	d.mu.Lock()
	devices := append([]HardwareDevice{}, d.discovered...)
	d.mu.Unlock()
	out := []HardwareDevice{}
	for _, found := range devices {
		if configured != "auto" && configured != "" && string(found.Backend) != configured {
			continue
		}
		if device != "" && found.Path != device && found.ID != device {
			continue
		}
		out = append(out, found)
	}
	if len(out) == 0 {
		for _, backend := range HardwareBackends {
			if configured != "auto" && configured != "" && configured != string(backend) {
				continue
			}
			out = append(out, HardwareDevice{Backend: backend, Path: device})
		}
	}
	return out
}
