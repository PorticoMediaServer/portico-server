package decoder

import "sort"

// HardwareFactDevice is one adapter this host exposes: the name its driver gives
// it, the method proven on it by an actual probe, and whether conversions are
// going there now. Nothing here is inferred from a vendor string alone.
type HardwareFactDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Method is the stable name of the backend proven on this adapter, which the
	// owner's screen turns into the frame's own wording (Quick Sync, NVENC, …).
	// Empty means no backend has earned this adapter yet.
	Method string `json:"method,omitempty"`
	// InUse marks the adapter the effective backend's conversions run on.
	InUse bool `json:"inUse"`
}

// HardwareFactFormat is one row of the settings grid: whether this server will
// decode and encode that format in hardware right now. False is a statement
// about behaviour, not about the card: an unprobed cell is decoded by the
// processor because hardware decode is never assumed.
type HardwareFactFormat struct {
	Format string `json:"format"`
	Decode bool   `json:"decode"`
	Encode bool   `json:"encode"`
}

// HardwareFactFault is the one failure worth telling the owner about, in the
// probe's own words. It is published only when no hardware backend works, so an
// absent NVIDIA card on an Intel host is not reported as a fault.
type HardwareFactFault struct {
	Backend string `json:"backend,omitempty"`
	Device  string `json:"device,omitempty"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`
}

// HardwareFactReasonNoDevice stands for "no adapter answered at all", which is
// what a server with no graphics card passed in has to say.
const HardwareFactReasonNoDevice = "no_graphics_device"

// HardwareFacts is the measured half of Server › Streaming › Hardware. The
// install's remedy and the frame's format order come from hardwarefacts; every
// value here comes from a probe that ran on this host.
type HardwareFacts struct {
	Devices []HardwareFactDevice
	Formats []HardwareFactFormat
	// Fault is nil while at least one hardware backend works.
	Fault *HardwareFactFault
	// LastTest is when the newest probe finished, in Unix seconds; zero means
	// this process has not probed anything yet.
	LastTest int64
}

// hardwareFactMethods names each backend as the frame's device list does. A
// backend without a name keeps its own token, so a new one never goes blank.
var hardwareFactMethods = map[HardwareBackend]string{
	BackendQSV:          "quick_sync",
	BackendNVENC:        "nvenc",
	BackendVAAPI:        "vaapi",
	BackendAMF:          "amf",
	BackendVideoToolbox: "videotoolbox",
	"rkmpp":             "rkmpp",
}

// hardwareFactFormatOrder is the grid's one order, shared with hardwarefacts.
var hardwareFactFormatOrder = []string{"h264", "hevc8", "hevc10", "av1", "vp9", "vc1", "mpeg2"}

// hardwareFactDecodeKeys are the decode cells that count for one grid row.
var hardwareFactDecodeKeys = map[string][]string{
	"h264":   {"h264:8"},
	"hevc8":  {"hevc:8"},
	"hevc10": {"hevc:10", "hevc:12"},
	"av1":    {"av1:8", "av1:10"},
	"vp9":    {"vp9:8", "vp9:10"},
	// VC-1 needs a licensed fixture before it can be probed, so it stays false:
	// an unmeasured format is decoded by the processor, never assumed.
	"vc1":   {},
	"mpeg2": {"mpeg2:8"},
}

// hardwareFactEncodeKeys are the encode cells that count for one grid row. A
// format Portico never converts to has none, and says so by staying false.
var hardwareFactEncodeKeys = map[string][]string{
	"h264":   {"h264:8"},
	"hevc8":  {"hevc:8"},
	"hevc10": {"hevc:10"},
	"av1":    {"av1:8", "av1:10"},
}

// ComposeHardwareFacts answers the drawn Hardware rows from what this process
// measured. Devices come from discovery, the grid and the adapter's method from
// real probes, and effective is the backend the owner's setting resolved to.
func ComposeHardwareFacts(devices []HardwareDevice, probes []HardwareProbe, effective string) HardwareFacts {
	out := HardwareFacts{Devices: []HardwareFactDevice{}, Formats: []HardwareFactFormat{}}
	proven := map[string]HardwareBackend{}
	usable := false
	for _, probe := range probes {
		if !probe.Available || probe.Backend == BackendSoftware {
			continue
		}
		usable = true
		if probe.Device != "" {
			if _, seen := proven[probe.Device]; !seen {
				proven[probe.Device] = probe.Backend
			}
		}
	}
	for _, device := range devices {
		method, inUse := "", false
		if backend, ok := proven[device.Path]; ok {
			method = hardwareFactMethod(backend)
			inUse = string(backend) == effective
		} else if len(device.Backends) > 0 {
			// Found but not yet proven: name the method the adapter is expected
			// to offer, and say nothing about what it can do.
			method = hardwareFactMethod(device.Backends[0])
		}
		out.Devices = append(out.Devices, HardwareFactDevice{ID: device.ID, Name: hardwareFactDeviceName(device), Method: method, InUse: inUse})
	}
	for _, format := range hardwareFactFormatOrder {
		out.Formats = append(out.Formats, HardwareFactFormat{Format: format,
			Decode: anyProbeCell(probes, func(probe HardwareProbe) map[string]bool { return probe.DecodeCodecs }, hardwareFactDecodeKeys[format]),
			Encode: anyProbeCell(probes, func(probe HardwareProbe) map[string]bool { return probe.OutputQuality }, hardwareFactEncodeKeys[format]),
		})
	}
	if !usable {
		out.Fault = hardwareFactFault()
	}
	out.LastTest = newestProbeTime()
	return out
}

// hardwareFactMethod turns a backend into the stable method token the owner's
// screen names. An unmapped backend keeps its own string.
func hardwareFactMethod(backend HardwareBackend) string {
	if method, ok := hardwareFactMethods[backend]; ok {
		return method
	}
	return string(backend)
}

func hardwareFactDeviceName(device HardwareDevice) string {
	if device.Name != "" {
		return device.Name
	}
	return device.ID
}

func anyProbeCell(probes []HardwareProbe, cells func(HardwareProbe) map[string]bool, keys []string) bool {
	for _, probe := range probes {
		if !probe.Available {
			continue
		}
		measured := cells(probe)
		for _, key := range keys {
			if measured[key] {
				return true
			}
		}
	}
	return false
}

// hardwareFactFault names the most recent failing probe. A timer-out or a
// confinement refusal is already in the observation's reason and detail.
func hardwareFactFault() *HardwareFactFault {
	observations := ProbeObservations()
	sort.SliceStable(observations, func(i, j int) bool { return observations[i].At.After(observations[j].At) })
	for _, observation := range observations {
		if observation.Available {
			continue
		}
		return &HardwareFactFault{Backend: observation.Backend, Device: observation.Device, Reason: observation.Reason, Detail: observation.Detail}
	}
	return &HardwareFactFault{Reason: HardwareFactReasonNoDevice}
}

// newestProbeTime is when this process last finished a probe.
func newestProbeTime() int64 {
	var newest int64
	for _, observation := range ProbeObservations() {
		if at := observation.At.Unix(); at > newest {
			newest = at
		}
	}
	return newest
}
