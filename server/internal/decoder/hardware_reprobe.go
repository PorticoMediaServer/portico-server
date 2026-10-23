package decoder

import (
	"context"
	"portico.local/server/internal/supervise"
	"time"
)

type HardwareReprobe struct {
	Status      string `json:"status"`
	StartedAt   int64  `json:"startedAt"`
	CompletedAt int64  `json:"completedAt"`
}

// StartReprobe deduplicates an owner's explicit test. A POST returns immediately
// while all media work remains in bounded confined background processes.
func (d *HardwareDetector) StartReprobe(executable string) HardwareReprobe {
	if _, err := ffmpegIdentity(executable); err != nil {
		return HardwareReprobe{Status: "unavailable"}
	}
	d.mu.Lock()
	if d.reprobe.Status == "running" {
		result := d.reprobe
		d.mu.Unlock()
		return result
	}
	d.reprobe = HardwareReprobe{Status: "running", StartedAt: time.Now().UnixMilli()}
	d.revision++
	result := d.reprobe
	d.mu.Unlock()
	supervise.Go("decoder.hardware.reprobe", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		d.Reprobe(ctx, executable)
		d.mu.Lock()
		d.reprobe.Status = "complete"
		if ctx.Err() != nil {
			d.reprobe.Status = "timed_out"
		}
		d.reprobe.CompletedAt = time.Now().UnixMilli()
		d.revision++
		d.mu.Unlock()
	})
	return result
}

// ReprobeState is the owner's current test lifecycle, not a success verdict.
func (d *HardwareDetector) ReprobeState() HardwareReprobe {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reprobe
}

// ProbeRevision changes when a cached observation or explicit test changes.
// Capacity polling reads this counter without launching a media process.
func (d *HardwareDetector) ProbeRevision() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.revision
}

func (d *HardwareDetector) CachedReport(executable, device string) []HardwareProbe {
	identity, err := ffmpegIdentity(executable)
	if err != nil {
		return []HardwareProbe{{Backend: BackendSoftware, Available: true, Reason: ProbeAvailable, Encoder: "libx264"}}
	}
	out := []HardwareProbe{{Backend: BackendSoftware, Available: true, Reason: ProbeAvailable, Encoder: "libx264"}}
	cold := false
	for _, info := range d.cachedCandidates("auto", device) {
		if !backendSpecs[info.Backend].supported() {
			continue
		}
		d.mu.Lock()
		probe, ok := d.cache[identity+"/"+string(info.Backend)+"/"+info.Path]
		d.mu.Unlock()
		if !ok {
			probe = HardwareProbe{Backend: info.Backend, Device: info.Path, Reason: ProbeWarming, Identity: identity}
			cold = true
		}
		found := info
		probe.DeviceInfo = &found
		probe.ActiveSessions = HardwareSessionsInUse(probe.Backend, probe.Device)
		out = append(out, probe)
	}
	if cold && d.ReprobeState().Status != "running" {
		d.warm(executable, "auto", device)
	}
	return out
}
