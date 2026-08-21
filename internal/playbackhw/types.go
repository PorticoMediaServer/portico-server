// Package playbackhw describes conservative FFmpeg hardware pipelines. It does
// not probe FFmpeg, drivers, or devices; callers must supply probe results.
package playbackhw

import "fmt"

type Backend string

const (
	VideoToolbox Backend = "videotoolbox"
	QSV          Backend = "qsv"
	VAAPI        Backend = "vaapi"
	NVIDIA       Backend = "nvidia"
	AMF          Backend = "amf"
)

type OS string

const (
	Darwin  OS = "darwin"
	Linux   OS = "linux"
	Windows OS = "windows"
)

type DeviceVendor string

const (
	Apple  DeviceVendor = "apple"
	Intel  DeviceVendor = "intel"
	AMD    DeviceVendor = "amd"
	Nvidia DeviceVendor = "nvidia"
)

type Codec string

const (
	H264 Codec = "h264"
	HEVC Codec = "hevc"
	AV1  Codec = "av1"
)

type PixelFormat string

const (
	YUV420P     PixelFormat = "yuv420p"
	NV12        PixelFormat = "nv12"
	YUV420P10LE PixelFormat = "yuv420p10le"
	P010LE      PixelFormat = "p010le"
)

type Availability string

const (
	AvailabilityUnknown     Availability = "unknown"
	AvailabilityAvailable   Availability = "available"
	AvailabilityUnavailable Availability = "unavailable"
)

type Operation string

const (
	Decode       Operation = "decode"
	Upload       Operation = "upload"
	Download     Operation = "download"
	Scale        Operation = "scale"
	Deinterlace  Operation = "deinterlace"
	ToneMap      Operation = "tone_map"
	SubtitleBurn Operation = "subtitle_burn"
	Encode       Operation = "encode"
)

type Execution string

const (
	Hardware Execution = "hardware"
	Software Execution = "software"
)

type Capability struct {
	Backend         Backend
	OS              OS
	Vendor          DeviceVendor
	Decode          []Codec
	Encode          []Codec
	BitDepths       []int
	PixelFormats    []PixelFormat
	HardwareFilters []Operation
	EncodeOnly      bool
}

// DeviceContext is execution-only adapter context. DevicePath is permitted in
// FFmpeg argument fragments but is deliberately omitted from PlanSummary.
type DeviceContext struct {
	Identity          string
	DevicePath        string
	BinaryFingerprint string
	ProbeRevision     string
}

// VerifiedEvidence represents successful executable probes for one exact
// FFmpeg build, driver, and device identity. Backend listings are not evidence.
// Complete must only be set after the probe set itself completed successfully.
type VerifiedEvidence struct {
	Complete          bool
	Executable        bool
	Backend           Backend
	OS                OS
	Vendor            DeviceVendor
	DeviceIdentity    string
	DevicePath        string
	BinaryFingerprint string
	ProbeRevision     string
	Decode            []Codec
	Encode            []Codec
	BitDepths         []int
	PixelFormats      []PixelFormat
	HardwareStages    []Operation
	SoftwareStages    []Operation
	CrossoverStages   []Operation
}

type Request struct {
	Backend           Backend
	OS                OS
	Vendor            DeviceVendor
	Availability      Availability
	Device            DeviceContext
	Evidence          *VerifiedEvidence
	InputCodec        Codec
	OutputCodec       Codec
	InputBitDepth     int
	OutputBitDepth    int
	InputPixelFormat  PixelFormat
	OutputPixelFormat PixelFormat
	HDRInput          bool
	HDROutput         bool
	Width             int
	Height            int
	Deinterlace       bool
	ToneMap           bool
	SubtitleFile      string
	// EncodeOnly keeps decoding and filtering in software while using the
	// verified hardware encoder. It is set from the independent encode setting.
	EncodeOnly bool
}

type Stage struct {
	Operation Operation
	Execution Execution
	Args      []string
}

// RuntimeIdentity is private execution authority. It is retained only inside
// the server-sealed binding; PlanSummary deliberately omits every field.
type RuntimeIdentity struct {
	ExecutablePath    string
	BinaryFingerprint string
	DeviceIdentity    string
	DevicePath        string
	DriverIdentity    string
	DriverVersion     string
}

type Plan struct {
	Backend              Backend
	RuntimeIdentity      RuntimeIdentity
	Stages               []Stage
	InputArgs            []string
	Filter               string
	OutputArgs           []string
	RequiresRuntimeProbe bool
}

// PlanSummary contains no command arguments, diagnostics, device paths, or
// probe identity data and is safe for public status surfaces.
type PlanSummary struct {
	Backend Backend
	Stages  []struct {
		Operation Operation
		Execution Execution
	}
}

func (p Plan) Summary() PlanSummary {
	s := PlanSummary{Backend: p.Backend, Stages: make([]struct {
		Operation Operation
		Execution Execution
	}, len(p.Stages))}
	for i, stage := range p.Stages {
		s.Stages[i].Operation, s.Stages[i].Execution = stage.Operation, stage.Execution
	}
	return s
}

type UnsupportedCode string

const (
	UnsupportedBackend      UnsupportedCode = "unsupported_backend"
	UnsupportedOSDevice     UnsupportedCode = "unsupported_os_device"
	BackendUnavailable      UnsupportedCode = "backend_unavailable"
	ProbeEvidenceRequired   UnsupportedCode = "probe_evidence_required"
	ProbeEvidenceMismatch   UnsupportedCode = "probe_evidence_mismatch"
	ProbeEvidenceIncomplete UnsupportedCode = "probe_evidence_incomplete"
	UnverifiedStage         UnsupportedCode = "unverified_stage"
	UnsupportedDecode       UnsupportedCode = "unsupported_decode"
	UnsupportedEncode       UnsupportedCode = "unsupported_encode"
	UnsupportedBitDepth     UnsupportedCode = "unsupported_bit_depth"
	UnsupportedPixelFormat  UnsupportedCode = "unsupported_pixel_format"
	InvalidHDRTransition    UnsupportedCode = "invalid_hdr_transition"
	InvalidDimensions       UnsupportedCode = "invalid_dimensions"
	InvalidDeviceContext    UnsupportedCode = "invalid_device_context"
)

type UnsupportedError struct {
	Code   UnsupportedCode
	Detail string
}

func (e *UnsupportedError) Error() string {
	if e.Detail == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}
