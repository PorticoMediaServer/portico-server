package playback

import "portico.local/server/internal/decoder"

// DeliveryConfiguration is the owner-administered half of delivery planning.
// Playback never reads the administration package: the owner settings registry
// is wired to this struct through DeliverySettings, so the two concerns can be
// built independently and neither owns the other's file.
type DeliveryConfiguration struct {
	// HardwareBackend is "auto", "software" or a decoder backend name. A named
	// backend that fails its probe falls back to software; it never fails a session.
	HardwareBackend string `json:"hardwareBackend"`
	HardwareDevice  string `json:"hardwareDevice"`

	ToneMapping      bool   `json:"toneMapping"`
	ToneMapAlgorithm string `json:"toneMapAlgorithm"`
	SoftwarePreset   string `json:"softwarePreset"`

	// PlanningPolicy biases the plan when more than one route is admissible.
	PlanningPolicy string `json:"planningPolicy"`

	ThrottleBufferSeconds  int  `json:"throttleBufferSeconds"`
	PlayedRetentionSeconds int  `json:"playedRetentionSeconds"`
	RemuxEnabled           bool `json:"remuxEnabled"`

	// Server clamps narrow every viewer preference. Zero means "no clamp".
	MaxVideoBitrateBPS int  `json:"maxVideoBitrateBps"`
	MaxAudioBitrateBPS int  `json:"maxAudioBitrateBps"`
	MaxVideoHeight     int  `json:"maxVideoHeight"`
	AllowHDR           bool `json:"allowHDR"`

	// Owner ceilings on simultaneous conversions. Zero means unlimited; the
	// host-capacity bound on running converters still applies underneath. A
	// session that only repackages (both streams copied) is not a conversion.
	MaxConversions         int `json:"maxConversions"`
	MaxHardwareConversions int `json:"maxHardwareConversions"`
	MaxSoftwareConversions int `json:"maxSoftwareConversions"`
}

// Planning policies. maximum_fidelity keeps the most of the source;
// maximum_compatibility prefers the most widely playable output; and
// minimize_server_work prefers the cheapest route the policy still allows.
const (
	PlanningMaximumFidelity     = "maximum_fidelity"
	PlanningMaximumCompatible   = "maximum_compatibility"
	PlanningMinimizeServerWork  = "minimize_server_work"
	DefaultThrottleBufferSecond = 60
	DefaultPlayedRetention      = 300
)

// PlanningPolicies is the published domain for the owner setting.
var PlanningPolicies = []string{PlanningMaximumFidelity, PlanningMaximumCompatible, PlanningMinimizeServerWork}

func validPlanningPolicy(p string) bool {
	for _, v := range PlanningPolicies {
		if v == p {
			return true
		}
	}
	return false
}

// DefaultDeliveryConfiguration is what a server without owner administration
// does. It is deliberately the same shape the registry will fill in.
func DefaultDeliveryConfiguration() DeliveryConfiguration {
	return DeliveryConfiguration{
		HardwareBackend:        "auto",
		ToneMapping:            true,
		ToneMapAlgorithm:       decoder.DefaultToneMapAlgorithm,
		SoftwarePreset:         decoder.DefaultSoftwarePreset,
		PlanningPolicy:         PlanningMaximumFidelity,
		ThrottleBufferSeconds:  DefaultThrottleBufferSecond,
		PlayedRetentionSeconds: DefaultPlayedRetention,
		RemuxEnabled:           true,
		AllowHDR:               true,
	}
}

// Normalized repairs an owner configuration without refusing playback. An
// invalid stored value must never take delivery down; it falls back to the
// default for that field alone.
func (c DeliveryConfiguration) Normalized() DeliveryConfiguration {
	d := DefaultDeliveryConfiguration()
	if c.HardwareBackend != "auto" && !decoder.ValidBackend(c.HardwareBackend) {
		c.HardwareBackend = d.HardwareBackend
	}
	if !decoder.ValidToneMapAlgorithm(c.ToneMapAlgorithm) {
		c.ToneMapAlgorithm = d.ToneMapAlgorithm
	}
	if !decoder.ValidSoftwarePreset(c.SoftwarePreset) {
		c.SoftwarePreset = d.SoftwarePreset
	}
	if !validPlanningPolicy(c.PlanningPolicy) {
		c.PlanningPolicy = d.PlanningPolicy
	}
	if c.ThrottleBufferSeconds < 10 || c.ThrottleBufferSeconds > 3600 {
		c.ThrottleBufferSeconds = d.ThrottleBufferSeconds
	}
	if c.PlayedRetentionSeconds < 0 || c.PlayedRetentionSeconds > 86400 {
		c.PlayedRetentionSeconds = d.PlayedRetentionSeconds
	}
	if c.MaxVideoBitrateBPS < 0 || c.MaxVideoBitrateBPS > 200_000_000 {
		c.MaxVideoBitrateBPS = 0
	}
	if c.MaxAudioBitrateBPS < 0 || c.MaxAudioBitrateBPS > 4_096_000 {
		c.MaxAudioBitrateBPS = 0
	}
	if c.MaxVideoHeight < 0 || c.MaxVideoHeight > 4320 {
		c.MaxVideoHeight = 0
	}
	for _, limit := range []*int{&c.MaxConversions, &c.MaxHardwareConversions, &c.MaxSoftwareConversions} {
		if *limit < 0 || *limit > 1000 {
			*limit = 0
		}
	}
	return c
}

// DeliverySettings is the single seam between owner administration and delivery.
// One method, so the administration agent can satisfy it from the settings
// registry without playback knowing that registry exists.
type DeliverySettings interface {
	Delivery() DeliveryConfiguration
}

type defaultDeliverySettings struct{}

func (defaultDeliverySettings) Delivery() DeliveryConfiguration {
	return DefaultDeliveryConfiguration()
}

// DefaultDeliverySettings is the implementation a server uses until the owner
// settings registry is wired in.
func DefaultDeliverySettings() DeliverySettings { return defaultDeliverySettings{} }

// deliveryConfiguration reads the seam defensively: a nil settings source, or
// one that returns nonsense, still yields a usable configuration.
func deliveryConfiguration(s DeliverySettings) DeliveryConfiguration {
	if s == nil {
		return DefaultDeliveryConfiguration()
	}
	return s.Delivery().Normalized()
}
