package playbackhw

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func fullEvidence(b Backend, os OS, vendor DeviceVendor) *VerifiedEvidence {
	return &VerifiedEvidence{Complete: true, Executable: true, Backend: b, OS: os, Vendor: vendor, DeviceIdentity: "device-identity", BinaryFingerprint: "ffmpeg-sha256", ProbeRevision: "w5-v1", Decode: []Codec{H264, HEVC, AV1}, Encode: []Codec{H264, HEVC, AV1}, BitDepths: []int{8, 10}, PixelFormats: []PixelFormat{YUV420P, NV12, YUV420P10LE, P010LE}, HardwareStages: []Operation{Decode, Scale, Deinterlace, ToneMap, Encode}, SoftwareStages: []Operation{Scale, Deinterlace, ToneMap, SubtitleBurn}, CrossoverStages: []Operation{Upload, Download}}
}

func TestVideoToolboxPlanSealsTenToEightBitSoftwareFormatConversion(t *testing.T) {
	r := verifiedRequest(VideoToolbox, Darwin, Apple)
	r.InputCodec = HEVC
	r.InputBitDepth = 10
	r.InputPixelFormat = YUV420P10LE
	r.OutputCodec = H264
	r.OutputBitDepth = 8
	r.OutputPixelFormat = YUV420P
	p, err := PlanPipeline(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Filter != "format=yuv420p" {
		t.Fatalf("filter = %q, want explicit 8-bit software conversion", p.Filter)
	}
	found := false
	for _, stage := range p.Stages {
		if stage.Operation == Scale && stage.Execution == Software && len(stage.Args) == 1 && stage.Args[0] == "format=yuv420p" {
			found = true
		}
	}
	if !found {
		t.Fatalf("software format stage missing from %#v", p.Stages)
	}
}
func verifiedRequest(b Backend, os OS, vendor DeviceVendor) Request {
	path := ""
	if b == VAAPI {
		path = "/dev/dri/renderD128"
	}
	if b == QSV {
		path = "/dev/dri/renderD129"
	}
	if b == NVIDIA {
		path = "0"
	}
	evidence := fullEvidence(b, os, vendor)
	evidence.DevicePath = path
	return Request{Backend: b, OS: os, Vendor: vendor, Availability: AvailabilityAvailable, Device: DeviceContext{Identity: "device-identity", DevicePath: path, BinaryFingerprint: "ffmpeg-sha256", ProbeRevision: "w5-v1"}, Evidence: evidence, InputCodec: H264, OutputCodec: H264}
}

func TestRegistryMatrix(t *testing.T) {
	want := []struct {
		b          Backend
		o          OS
		v          DeviceVendor
		d, e       []Codec
		encodeOnly bool
	}{
		{VideoToolbox, Darwin, Apple, []Codec{H264, HEVC}, []Codec{H264, HEVC}, false}, {QSV, Windows, Intel, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, false}, {QSV, Linux, Intel, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, false}, {VAAPI, Linux, Intel, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, false}, {VAAPI, Linux, AMD, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, false}, {NVIDIA, Windows, Nvidia, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, false}, {NVIDIA, Linux, Nvidia, []Codec{H264, HEVC, AV1}, []Codec{H264, HEVC, AV1}, false}, {AMF, Windows, AMD, nil, []Codec{H264, HEVC, AV1}, true},
	}
	got := Registry()
	if len(got) != len(want) {
		t.Fatalf("length=%d", len(got))
	}
	for i, w := range want {
		g := got[i]
		if g.Backend != w.b || g.OS != w.o || g.Vendor != w.v || g.EncodeOnly != w.encodeOnly || !reflect.DeepEqual(g.Decode, w.d) || !reflect.DeepEqual(g.Encode, w.e) {
			t.Errorf("entry %d=%#v", i, g)
		}
	}
	got[0].Encode[0] = AV1
	if Registry()[0].Encode[0] != H264 {
		t.Fatal("Registry did not deep copy")
	}
}

func TestPlanPipelineGolden(t *testing.T) {
	vt := verifiedRequest(VideoToolbox, Darwin, Apple)
	vt.InputCodec = HEVC
	vt.InputBitDepth = 10
	vt.OutputBitDepth = 8
	vt.HDRInput = true
	vt.ToneMap = true
	vt.Width = 1920
	vt.Height = 1080
	qsv := verifiedRequest(QSV, Linux, Intel)
	qsv.InputCodec = AV1
	qsv.OutputCodec = HEVC
	qsv.InputBitDepth = 10
	qsv.OutputBitDepth = 10
	qsv.Width = 3840
	qsv.Height = 2160
	va := verifiedRequest(VAAPI, Linux, AMD)
	va.InputCodec = HEVC
	va.InputBitDepth = 10
	va.HDRInput = true
	va.ToneMap = true
	va.SubtitleFile = "/media/a:b's.srt"
	nv := verifiedRequest(NVIDIA, Windows, Nvidia)
	nv.OutputCodec = AV1
	nv.Deinterlace = true
	amf := verifiedRequest(AMF, Windows, AMD)
	amf.OutputCodec = HEVC
	amf.OutputBitDepth = 10
	tests := []struct {
		name   string
		req    Request
		input  []string
		filter string
		output []string
		stages []Operation
	}{
		{"videotoolbox", vt, []string{"-hwaccel", "videotoolbox"}, "zscale=transfer=linear,tonemap=mobius,zscale=transfer=bt709:primaries=bt709:matrix=bt709,scale=1920:1080,format=yuv420p", []string{"-c:v", "h264_videotoolbox", "-pix_fmt", "yuv420p"}, []Operation{Decode, Download, ToneMap, Scale, Scale, Encode}},
		{"qsv", qsv, []string{"-init_hw_device", "vaapi=portico_vaapi:/dev/dri/renderD129", "-init_hw_device", "qsv=portico_hw@portico_vaapi", "-filter_hw_device", "portico_hw", "-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}, "scale_qsv=3840:2160", []string{"-c:v", "hevc_qsv", "-pix_fmt", "yuv420p10le"}, []Operation{Decode, Scale, Encode}},
		{"vaapi", va, []string{"-init_hw_device", "vaapi=portico_hw:/dev/dri/renderD128", "-filter_hw_device", "portico_hw", "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}, "tonemap_vaapi=format=nv12,hwdownload,format=yuv420p,subtitles='/media/a\\:b\\'s.srt',format=yuv420p,format=nv12,hwupload", []string{"-c:v", "h264_vaapi", "-pix_fmt", "yuv420p"}, []Operation{Decode, ToneMap, Download, SubtitleBurn, Scale, Upload, Encode}},
		{"nvidia", nv, []string{"-init_hw_device", "cuda=portico_hw:0", "-filter_hw_device", "portico_hw", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}, "yadif_cuda", []string{"-c:v", "av1_nvenc", "-pix_fmt", "yuv420p"}, []Operation{Decode, Deinterlace, Encode}},
		{"amf", amf, nil, "format=yuv420p10le", []string{"-c:v", "hevc_amf", "-pix_fmt", "yuv420p10le"}, []Operation{Scale, Encode}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PlanPipeline(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.InputArgs, tt.input) || got.Filter != tt.filter || !reflect.DeepEqual(got.OutputArgs, tt.output) || got.RequiresRuntimeProbe {
				t.Fatalf("plan=%#v", got)
			}
			ops := []Operation{}
			for _, s := range got.Stages {
				ops = append(ops, s.Operation)
			}
			if !reflect.DeepEqual(ops, tt.stages) {
				t.Errorf("stages=%v", ops)
			}
		})
	}
}

func TestAvailabilityAndListingsNeverUnlockPlanning(t *testing.T) {
	r := verifiedRequest(QSV, Linux, Intel)
	r.Evidence = nil
	assertCode(t, r, ProbeEvidenceRequired)
	r.Evidence = &VerifiedEvidence{Complete: true, Backend: QSV, OS: Linux, Vendor: Intel, DeviceIdentity: "device-identity", DevicePath: r.Device.DevicePath, BinaryFingerprint: "ffmpeg-sha256", ProbeRevision: "w5-v1"} // listing-like, never executed
	assertCode(t, r, ProbeEvidenceIncomplete)
	r.Evidence.Executable = true
	r.Evidence.Complete = false
	assertCode(t, r, ProbeEvidenceIncomplete)
}

func TestOlderAvailableDeviceCannotInheritStaticMaximum(t *testing.T) {
	r := verifiedRequest(VAAPI, Linux, Intel)
	r.Evidence.Decode = []Codec{H264, HEVC}
	r.Evidence.Encode = []Codec{H264, HEVC}
	r.Evidence.BitDepths = []int{8}
	r.Evidence.PixelFormats = []PixelFormat{YUV420P, NV12}
	r.Evidence.HardwareStages = []Operation{Decode, Scale, Encode}
	r.InputCodec = AV1
	assertCode(t, r, UnverifiedStage)
	r.InputCodec = H264
	r.OutputCodec = AV1
	assertCode(t, r, UnverifiedStage)
	r.OutputCodec = H264
	r.InputBitDepth = 10
	assertCode(t, r, UnverifiedStage)
	r.InputBitDepth = 8
	r.HDRInput = true
	r.ToneMap = true
	r.Evidence.SoftwareStages = nil
	r.Evidence.CrossoverStages = nil
	assertCode(t, r, UnverifiedStage)
}

func TestExactStageAndCrossoverEvidenceRequired(t *testing.T) {
	r := verifiedRequest(VideoToolbox, Darwin, Apple)
	r.SubtitleFile = "captions.srt"
	r.Evidence.CrossoverStages = nil
	assertCode(t, r, UnverifiedStage)
	r.Evidence.CrossoverStages = []Operation{Download, Upload}
	r.Evidence.SoftwareStages = nil
	assertCode(t, r, UnverifiedStage)
	r.Evidence.SoftwareStages = []Operation{SubtitleBurn}
	if _, err := PlanPipeline(r); err != nil {
		t.Fatalf("exact evidence should succeed: %v", err)
	}
}

func TestPlanSummaryDoesNotExposeExecutionContext(t *testing.T) {
	r := verifiedRequest(VAAPI, Linux, Intel)
	p, err := PlanPipeline(r)
	if err != nil {
		t.Fatal(err)
	}
	s := p.Summary()
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{string(s.Backend)}, " ")))
	if strings.Contains(text, "renderd") || strings.Contains(text, "device-identity") {
		t.Fatal("summary exposed device context")
	}
}

func TestStableUnsupportedReasons(t *testing.T) {
	base := verifiedRequest(QSV, Linux, Intel)
	tests := []struct {
		name   string
		mutate func(*Request)
		code   UnsupportedCode
	}{
		{"d3d", func(r *Request) { r.Backend = "d3d11va" }, UnsupportedBackend}, {"wrong os", func(r *Request) { r.OS = Darwin }, UnsupportedOSDevice}, {"unavailable", func(r *Request) { r.Availability = AvailabilityUnavailable }, BackendUnavailable}, {"identity", func(r *Request) { r.Device.Identity = "other" }, ProbeEvidenceMismatch}, {"device", func(r *Request) { r.Device.DevicePath = "" }, InvalidDeviceContext}, {"decode", func(r *Request) { r.InputCodec = "vp9" }, UnsupportedDecode}, {"encode", func(r *Request) { r.OutputCodec = "vp9" }, UnsupportedEncode}, {"depth", func(r *Request) { r.InputBitDepth = 12 }, UnsupportedBitDepth}, {"format", func(r *Request) { r.InputPixelFormat = "rgb24" }, UnsupportedPixelFormat}, {"hdr", func(r *Request) { r.HDRInput = true }, InvalidHDRTransition}, {"dimensions", func(r *Request) { r.Width = 1 }, InvalidDimensions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { r := base; tt.mutate(&r); assertCode(t, r, tt.code) })
	}
}
func assertCode(t *testing.T, r Request, want UnsupportedCode) {
	t.Helper()
	_, err := PlanPipeline(r)
	var u *UnsupportedError
	if !errors.As(err, &u) {
		t.Fatalf("error=%v", err)
	}
	if u.Code != want {
		t.Fatalf("code=%s want %s (%v)", u.Code, want, err)
	}
}
