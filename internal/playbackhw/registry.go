package playbackhw

var registry = []Capability{
	{VideoToolbox, Darwin, Apple, []Codec{H264, HEVC}, []Codec{H264, HEVC}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale}, false},
	{QSV, Windows, Intel, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale, Deinterlace}, false},
	{QSV, Linux, Intel, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale, Deinterlace}, false},
	{VAAPI, Linux, Intel, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale, Deinterlace, ToneMap}, false},
	{VAAPI, Linux, AMD, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale, Deinterlace, ToneMap}, false},
	{NVIDIA, Windows, Nvidia, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale, Deinterlace}, false},
	{NVIDIA, Linux, Nvidia, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, []Operation{Scale, Deinterlace}, false},
	{AMF, Windows, AMD, nil, []Codec{H264, HEVC, AV1}, []int{8, 10}, []PixelFormat{NV12, P010LE}, nil, true},
}

// Registry returns a deep copy of the static potential-capability registry.
// Entries never imply that matching hardware, drivers, or FFmpeg components
// are installed on the current host.
func Registry() []Capability {
	out := make([]Capability, len(registry))
	for i, capability := range registry {
		out[i] = capability
		out[i].Decode = append([]Codec(nil), capability.Decode...)
		out[i].Encode = append([]Codec(nil), capability.Encode...)
		out[i].BitDepths = append([]int(nil), capability.BitDepths...)
		out[i].PixelFormats = append([]PixelFormat(nil), capability.PixelFormats...)
		out[i].HardwareFilters = append([]Operation(nil), capability.HardwareFilters...)
	}
	return out
}

func Find(backend Backend, os OS, vendor DeviceVendor) (Capability, bool) {
	for _, capability := range registry {
		if capability.Backend == backend && capability.OS == os && capability.Vendor == vendor {
			return capability, true
		}
	}
	return Capability{}, false
}
