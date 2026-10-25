package decoder

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SessionLimitObservation is one measured ceiling for concurrent hardware
// conversions on one adapter. It exists only when the driver refused a session
// while the lower levels were open: that refusal is the maximum, measured. An
// active-session count, a utilisation figure or a driver-version table is not a
// maximum and is never published as one.
type SessionLimitObservation struct {
	Backend    HardwareBackend
	Device     string
	Sessions   int
	Codec      string
	Width      int
	Height     int
	MeasuredAt time.Time
}

// sessionLimitLevels are the concurrency levels the probe opens, lowest first.
// A level that runs completely leaves the ceiling unproven above it; the first
// level the driver refuses gives the number.
var sessionLimitLevels = []int{2, 4, 8}

// sessionLimitProbeSeconds is how long each opened session encodes, so the
// sessions genuinely overlap before any of them finishes.
const sessionLimitProbeSeconds = 3

// sessionLimitKept bounds the remembered observations, one per adapter.
const sessionLimitKept = 8

var hardwareSessionLimits = struct {
	sync.Mutex
	last map[string]SessionLimitObservation
}{last: map[string]SessionLimitObservation{}}

func sessionLimitKey(backend HardwareBackend, device string) string {
	return string(backend) + "\x00" + device
}

// LastSessionLimit answers the measured ceiling for one adapter, and false until
// a probe has actually seen the driver refuse a session on it.
func LastSessionLimit(backend HardwareBackend, device string) (SessionLimitObservation, bool) {
	hardwareSessionLimits.Lock()
	defer hardwareSessionLimits.Unlock()
	observation, ok := hardwareSessionLimits.last[sessionLimitKey(backend, device)]
	return observation, ok
}

func noteSessionLimit(observation SessionLimitObservation) {
	hardwareSessionLimits.Lock()
	defer hardwareSessionLimits.Unlock()
	if len(hardwareSessionLimits.last) >= sessionLimitKept {
		for key := range hardwareSessionLimits.last {
			delete(hardwareSessionLimits.last, key)
			break
		}
	}
	hardwareSessionLimits.last[sessionLimitKey(observation.Backend, observation.Device)] = observation
}

// forgetSessionLimits drops measured ceilings. Tests use it; production keeps
// what this process measured until it exits.
func forgetSessionLimits() {
	hardwareSessionLimits.Lock()
	defer hardwareSessionLimits.Unlock()
	hardwareSessionLimits.last = map[string]SessionLimitObservation{}
}

// ProbeSessionLimit opens this many concurrent conversions until the driver
// refuses one, and remembers the last level that still worked. It is a deliberate
// owner action (Test hardware), never a page read: each level starts real
// encoder processes. A server whose card never refuses within the probed levels
// keeps no ceiling, because an unrefused probe proves only "at least".
func (d *HardwareDetector) ProbeSessionLimit(ctx context.Context, executable string, backend HardwareBackend, device string) (SessionLimitObservation, bool) {
	spec, ok := backendSpecs[backend]
	if !ok || backend == BackendSoftware || spec.encoder == "" {
		return SessionLimitObservation{}, false
	}
	// Only NVENC documents a per-session ceiling; the other backends are limited
	// by memory and bandwidth rather than by a fixed count, so probing them would
	// measure this machine's mood, not a limit.
	if backend != BackendNVENC {
		return SessionLimitObservation{}, false
	}
	codec, width, height := "h264", 1920, 1080
	for _, level := range sessionLimitLevels {
		if ctx.Err() != nil {
			break
		}
		worked, refused := d.runConcurrentEncodes(ctx, executable, spec, backend, device, level, codec, width, height)
		if !refused {
			continue
		}
		if worked < 2 {
			// One session is not a ceiling worth publishing.
			return SessionLimitObservation{}, false
		}
		observation := SessionLimitObservation{Backend: backend, Device: device, Sessions: worked,
			Codec: codec, Width: width, Height: height, MeasuredAt: time.Now().UTC()}
		noteSessionLimit(observation)
		return observation, true
	}
	return SessionLimitObservation{}, false
}

// runConcurrentEncodes starts level sessions at once and reports how many
// finished and whether the driver refused one. A refusal is recognised from the
// tool's own words, so a machine that fails for an unrelated reason is never
// recorded as a session ceiling.
func (d *HardwareDetector) runConcurrentEncodes(ctx context.Context, executable string, spec backendSpec, backend HardwareBackend, device string, level int, codec string, width, height int) (int, bool) {
	// A real encode at the profile the ceiling is measured for: the sessions have
	// to overlap, so each one runs for the probe's whole duration.
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	if spec.device != nil {
		args = append(args, spec.device(device)...)
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+strconv.Itoa(width)+"x"+strconv.Itoa(height)+":rate=10:duration="+strconv.Itoa(sessionLimitProbeSeconds),
		"-c:v", spec.encoder, "-b:v", "200k", "-an", "-sn", "-dn", "-f", "null", "-")
	var mu sync.Mutex
	finished, refused := 0, false
	var wait sync.WaitGroup
	for range level {
		wait.Add(1)
		go func() {
			defer wait.Done()
			bounded, cancel := context.WithTimeout(ctx, time.Duration(sessionLimitProbeSeconds+5)*time.Second)
			defer cancel()
			detail, err := d.runObserved(bounded, executable, args, nil)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				finished++
				return
			}
			if isSessionRefusal(detail) {
				refused = true
			}
		}()
	}
	wait.Wait()
	return finished, refused
}

// isSessionRefusal recognises the driver declining another concurrent session,
// as opposed to a codec, device or sandbox failure.
func isSessionRefusal(detail string) bool {
	for _, phrase := range []string{"out of memory", "OpenEncodeSessionEx", "no capable devices", "session limit", "too many concurrent sessions"} {
		if strings.Contains(strings.ToLower(detail), strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}
