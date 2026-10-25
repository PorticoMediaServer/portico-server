package decoder

import "sync"

// HardwareSessionPermits is the one place that decides whether another hardware
// conversion may start on one adapter, for every launch path that starts one. It
// counts reservations, not sessions observed after the fact: a caller reserves
// before it launches and releases only once the child has been waited, reaped or
// cancelled, so a slot is never given away while a converter is still running.
//
// A ceiling is only ever a measured, configuration-bound driver maximum. Until
// one exists for the exact runtime and adapter, reservations are counted and
// nothing is refused: this type never turns "N sessions started" or a generic
// resource refusal into a limit.
type HardwareSessionPermits struct {
	mu       sync.Mutex
	ceilings map[string]int
	inflight map[string]int
}

// permitKey binds a reservation to the exact runtime, backend and adapter. A
// different executable, library set or adapter is a different key, so a stale
// ceiling can never apply to a runtime that was never measured.
func permitKey(identity string, backend HardwareBackend, device string) string {
	return identity + "\x00" + string(backend) + "\x00" + device
}

var hardwareSessionPermits = &HardwareSessionPermits{ceilings: map[string]int{}, inflight: map[string]int{}}

// ConversionPermitIdentity names the media runtime a reservation belongs to, so
// a replaced executable or library set never inherits another runtime's ceiling.
func ConversionPermitIdentity() string {
	if facts := CurrentToolchain(); facts != nil {
		return facts.Identity
	}
	return ""
}

// HardwarePermits is the process-wide registry every encoder launch path shares.
func HardwarePermits() *HardwareSessionPermits { return hardwareSessionPermits }

// RecordCeiling stores a measured ceiling for one exact runtime, backend and
// adapter. It is only called with a maximum the driver actually enforced at that
// configuration; nothing in this package derives a ceiling from session counts.
func (p *HardwareSessionPermits) RecordCeiling(identity string, backend HardwareBackend, device string, sessions int) {
	if sessions < 2 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ceilings[permitKey(identity, backend, device)] = sessions
}

// Reserve asks for one slot. It never waits: a caller that cannot reserve falls
// back exactly as it already does for a failed backend (another adapter, then
// software). The release is safe to call once, and only after the child has been
// waited or cancelled and reaped.
func (p *HardwareSessionPermits) Reserve(identity string, backend HardwareBackend, device string) (release func(), ok bool) {
	key := permitKey(identity, backend, device)
	p.mu.Lock()
	ceiling := p.ceilings[key]
	if ceiling > 0 && p.inflight[key] >= ceiling {
		p.mu.Unlock()
		return func() {}, false
	}
	p.inflight[key]++
	p.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.inflight[key] > 0 {
				p.inflight[key]--
			}
			if p.inflight[key] == 0 {
				delete(p.inflight, key)
			}
		})
	}, true
}

// Retire drops every ceiling measured for a runtime, or for one adapter of it.
// A re-test or a replaced runtime or adapter must not leave an old maximum in
// force. Reservations already made are untouched: they belong to live children.
func (p *HardwareSessionPermits) Retire(identity string, backend HardwareBackend, device string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if identity == "" && backend == "" && device == "" {
		clear(p.ceilings)
		return
	}
	for key := range p.ceilings {
		if identity != "" && !hasKeyPrefix(key, identity) {
			continue
		}
		if backend != "" && !hasKeyPart(key, string(backend)) {
			continue
		}
		if device != "" && !hasKeySuffix(key, device) {
			continue
		}
		delete(p.ceilings, key)
	}
}

// Inflight answers how many reservations are held for one exact cell. Diagnostics
// and tests read it; admission decisions use Reserve.
func (p *HardwareSessionPermits) Inflight(identity string, backend HardwareBackend, device string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inflight[permitKey(identity, backend, device)]
}

func hasKeyPrefix(key, prefix string) bool { return len(key) >= len(prefix) && key[:len(prefix)+1] == prefix+"\x00" }
func hasKeySuffix(key, suffix string) bool { return len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix }
func hasKeyPart(key, part string) bool {
	for i := 0; i+len(part) <= len(key); i++ {
		if key[i:i+len(part)] == part && (i == 0 || key[i-1] == 0) && (i+len(part) == len(key) || key[i+len(part)] == 0) {
			return true
		}
	}
	return false
}
