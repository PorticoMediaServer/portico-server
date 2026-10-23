package decoder

import (
	"errors"
	"sort"
	"sync"
)

var ErrHardwareBusy = errors.New("hardware encoder has no free session")
var hardwareSessions = struct {
	sync.Mutex
	active  map[string]int
	nvidia  int
	changed chan struct{}
}{active: map[string]int{}, changed: make(chan struct{})}

// TryHardwareSession limits actual simultaneous encoder jobs, not planning or
// playback sessions. Release follows process retirement, including cancelled
// jobs. Unknown NVIDIA licensing uses a conservative system-wide cap because
// the vendor's consumer limit applies across all non-qualified cards.
func TryHardwareSession(probe HardwareProbe) (func(), error) {
	if probe.Backend == "" || probe.Backend == BackendSoftware || !probe.Available {
		return func() {}, nil
	}
	limit := 4
	if probe.DeviceInfo != nil && probe.DeviceInfo.SessionLimit > 0 {
		limit = probe.DeviceInfo.SessionLimit
	}
	key := string(probe.Backend) + ":" + probe.Device
	hardwareSessions.Lock()
	defer hardwareSessions.Unlock()
	if hardwareSessions.active[key] >= limit || probe.Backend == BackendNVENC && hardwareSessions.nvidia >= 3 {
		return nil, ErrHardwareBusy
	}
	hardwareSessions.active[key]++
	if probe.Backend == BackendNVENC {
		hardwareSessions.nvidia++
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			hardwareSessions.Lock()
			hardwareSessions.active[key]--
			if probe.Backend == BackendNVENC {
				hardwareSessions.nvidia--
			}
			close(hardwareSessions.changed)
			hardwareSessions.changed = make(chan struct{})
			hardwareSessions.Unlock()
		})
	}, nil
}
func HardwareSessionsInUse(backend HardwareBackend, device string) int {
	hardwareSessions.Lock()
	defer hardwareSessions.Unlock()
	return hardwareSessions.active[string(backend)+":"+device]
}

var hardwareVerdicts = struct {
	sync.Mutex
	byIdentity map[string]map[string]HardwareProbe
}{byIdentity: map[string]map[string]HardwareProbe{}}

func publishHardwareProbe(identity string, probe HardwareProbe) {
	hardwareVerdicts.Lock()
	defer hardwareVerdicts.Unlock()
	if hardwareVerdicts.byIdentity[identity] == nil {
		hardwareVerdicts.byIdentity[identity] = map[string]HardwareProbe{}
	}
	hardwareVerdicts.byIdentity[identity][string(probe.Backend)+":"+probe.Device] = probe
}

// NextHardwareProbe tries another device in the same backend, then another
// qualified backend. It never probes on a playback path or changes codec/depth.
func NextHardwareProbe(executable string, failed []HardwareProbe, codec string, depth int) HardwareProbe {
	identity, err := ffmpegIdentity(executable)
	if err != nil {
		return HardwareProbe{Backend: BackendSoftware, Available: true}
	}
	tried := map[string]bool{}
	for _, p := range failed {
		tried[string(p.Backend)+":"+p.Device] = true
	}
	hardwareVerdicts.Lock()
	candidates := []HardwareProbe{}
	for key, p := range hardwareVerdicts.byIdentity[identity] {
		if tried[key] || !p.Available || deviceCircuitOpen(p.Backend, p.Device) || !p.OutputEncoders[probeCodecKey(codec, depth)] {
			continue
		}
		candidates = append(candidates, p)
	}
	hardwareVerdicts.Unlock()
	previous := HardwareBackend("")
	if len(failed) > 0 {
		previous = failed[len(failed)-1].Backend
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.Backend == previous) != (b.Backend == previous) {
			return a.Backend == previous
		}
		if a.DeviceInfo != nil && b.DeviceInfo != nil && a.DeviceInfo.Discrete != b.DeviceInfo.Discrete {
			return a.DeviceInfo.Discrete
		}
		if a.Backend != b.Backend {
			return a.Backend < b.Backend
		}
		return a.Device < b.Device
	})
	if len(candidates) > 0 {
		return candidates[0]
	}
	return HardwareProbe{Backend: BackendSoftware, Available: true}
}
