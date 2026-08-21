package playbackhw

import "strings"

func PlanPipeline(req Request) (Plan, error) {
	if !knownBackend(req.Backend) {
		return Plan{}, unsupported(UnsupportedBackend, string(req.Backend))
	}
	capability, ok := Find(req.Backend, req.OS, req.Vendor)
	if !ok {
		return Plan{}, unsupported(UnsupportedOSDevice, string(req.Backend)+"/"+string(req.OS)+"/"+string(req.Vendor))
	}
	if req.Availability == AvailabilityUnavailable {
		return Plan{}, unsupported(BackendUnavailable, string(req.Backend))
	}
	if req.Availability != AvailabilityAvailable || req.Evidence == nil {
		return Plan{}, unsupported(ProbeEvidenceRequired, "successful executable probe evidence is required")
	}
	if !req.Evidence.Complete || !req.Evidence.Executable {
		return Plan{}, unsupported(ProbeEvidenceIncomplete, "probe set did not complete with executable evidence")
	}
	if (req.Backend == QSV || req.Backend == VAAPI || req.Backend == NVIDIA) && strings.TrimSpace(req.Device.DevicePath) == "" {
		return Plan{}, unsupported(InvalidDeviceContext, "backend requires an explicit device selector")
	}
	if req.Evidence.Backend != req.Backend || req.Evidence.OS != req.OS || req.Evidence.Vendor != req.Vendor ||
		strings.TrimSpace(req.Device.Identity) == "" || strings.TrimSpace(req.Device.BinaryFingerprint) == "" || strings.TrimSpace(req.Device.ProbeRevision) == "" ||
		req.Evidence.DeviceIdentity != req.Device.Identity || req.Evidence.DevicePath != req.Device.DevicePath ||
		req.Evidence.BinaryFingerprint != req.Device.BinaryFingerprint || req.Evidence.ProbeRevision != req.Device.ProbeRevision {
		return Plan{}, unsupported(ProbeEvidenceMismatch, "probe identity does not match the requested backend and device")
	}
	if req.Width < 0 || req.Height < 0 || (req.Width == 0) != (req.Height == 0) {
		return Plan{}, unsupported(InvalidDimensions, "width and height must both be positive or both zero")
	}
	if req.InputBitDepth == 0 {
		req.InputBitDepth = 8
	}
	if req.OutputBitDepth == 0 {
		req.OutputBitDepth = 8
	}
	if req.InputPixelFormat == "" {
		req.InputPixelFormat = formatForDepth(req.InputBitDepth, false)
	}
	if req.OutputPixelFormat == "" {
		req.OutputPixelFormat = formatForDepth(req.OutputBitDepth, false)
	}
	if !containsInt(capability.BitDepths, req.InputBitDepth) || !containsInt(capability.BitDepths, req.OutputBitDepth) {
		return Plan{}, unsupported(UnsupportedBitDepth, "only 8-bit and 10-bit pipelines are registered")
	}
	if !containsPixelFormat(capability.PixelFormats, formatForDepth(req.InputBitDepth, true)) || !containsPixelFormat(capability.PixelFormats, formatForDepth(req.OutputBitDepth, true)) {
		return Plan{}, unsupported(UnsupportedPixelFormat, "hardware pixel format is outside the backend maximum")
	}
	if !containsInt(req.Evidence.BitDepths, req.InputBitDepth) || !containsInt(req.Evidence.BitDepths, req.OutputBitDepth) {
		return Plan{}, unsupported(UnverifiedStage, "bit depth was not verified for this device")
	}
	if !validSoftwareFormat(req.InputPixelFormat) || !validSoftwareFormat(req.OutputPixelFormat) {
		return Plan{}, unsupported(UnsupportedPixelFormat, string(req.InputPixelFormat)+"->"+string(req.OutputPixelFormat))
	}
	if formatDepth(req.InputPixelFormat) != req.InputBitDepth || formatDepth(req.OutputPixelFormat) != req.OutputBitDepth {
		return Plan{}, unsupported(UnsupportedPixelFormat, "pixel format does not match declared bit depth")
	}
	if !containsPixelFormat(req.Evidence.PixelFormats, req.InputPixelFormat) || !containsPixelFormat(req.Evidence.PixelFormats, req.OutputPixelFormat) {
		return Plan{}, unsupported(UnverifiedStage, "pixel format was not verified for this device")
	}
	if req.HDRInput && !req.HDROutput && !req.ToneMap {
		return Plan{}, unsupported(InvalidHDRTransition, "HDR to SDR requires tone mapping")
	}
	if req.ToneMap && !req.HDRInput {
		return Plan{}, unsupported(InvalidHDRTransition, "tone mapping requires HDR input")
	}
	if !containsCodec(capability.Encode, req.OutputCodec) {
		return Plan{}, unsupported(UnsupportedEncode, string(req.OutputCodec))
	}
	if !containsCodec(req.Evidence.Encode, req.OutputCodec) {
		return Plan{}, unsupported(UnverifiedStage, "encoder was not verified for this device")
	}

	plan := Plan{Backend: req.Backend, RequiresRuntimeProbe: false}
	onHardware := false
	encodeOnly := capability.EncodeOnly || req.EncodeOnly
	if !encodeOnly {
		if !containsCodec(capability.Decode, req.InputCodec) {
			return Plan{}, unsupported(UnsupportedDecode, string(req.InputCodec))
		}
		if !containsCodec(req.Evidence.Decode, req.InputCodec) {
			return Plan{}, unsupported(UnverifiedStage, "decoder was not verified for this device")
		}
		args := decoderArgs(req)
		plan.InputArgs = append(plan.InputArgs, args...)
		plan.Stages = append(plan.Stages, Stage{Decode, Hardware, args})
		onHardware = true
	}

	filters := []string{}
	softwareDepth := req.InputBitDepth
	addHardwareFilter := func(op Operation, filter string) {
		filters = append(filters, filter)
		plan.Stages = append(plan.Stages, Stage{op, Hardware, []string{filter}})
	}
	addSoftwareFilter := func(op Operation, filter string) {
		if onHardware {
			download := "hwdownload,format=" + string(formatForDepth(softwareDepth, false))
			filters = append(filters, download)
			plan.Stages = append(plan.Stages, Stage{Download, Hardware, []string{download}})
			onHardware = false
		}
		filters = append(filters, filter)
		plan.Stages = append(plan.Stages, Stage{op, Software, []string{filter}})
	}

	if req.Deinterlace {
		if containsOperation(capability.HardwareFilters, Deinterlace) && containsOperation(req.Evidence.HardwareStages, Deinterlace) {
			addHardwareFilter(Deinterlace, hardwareFilter(req.Backend, Deinterlace, req))
		} else {
			addSoftwareFilter(Deinterlace, "bwdif")
		}
	}
	if req.ToneMap {
		if containsOperation(capability.HardwareFilters, ToneMap) && containsOperation(req.Evidence.HardwareStages, ToneMap) {
			addHardwareFilter(ToneMap, hardwareFilter(req.Backend, ToneMap, req))
			softwareDepth = req.OutputBitDepth
		} else {
			addSoftwareFilter(ToneMap, "zscale=transfer=linear,tonemap=mobius,zscale=transfer=bt709:primaries=bt709:matrix=bt709")
			softwareDepth = req.OutputBitDepth
		}
	}
	if req.Width > 0 {
		if onHardware && containsOperation(capability.HardwareFilters, Scale) && containsOperation(req.Evidence.HardwareStages, Scale) {
			addHardwareFilter(Scale, hardwareFilter(req.Backend, Scale, req))
		} else {
			addSoftwareFilter(Scale, dimensions("scale", req))
		}
	}
	if req.SubtitleFile != "" {
		addSoftwareFilter(SubtitleBurn, "subtitles="+escapeFilterValue(req.SubtitleFile))
	}

	requiresUpload := req.Backend == QSV || req.Backend == VAAPI || req.Backend == NVIDIA
	if (!encodeOnly || requiresUpload) && !onHardware {
		upload := uploadFilter(req.Backend, req.OutputBitDepth)
		filters = append(filters, upload)
		plan.Stages = append(plan.Stages, Stage{Upload, Hardware, []string{upload}})
		onHardware = true
	}
	encoder := encoderName(req.Backend, req.OutputCodec)
	plan.OutputArgs = []string{"-c:v", encoder, "-pix_fmt", string(req.OutputPixelFormat)}
	plan.Stages = append(plan.Stages, Stage{Encode, Hardware, append([]string(nil), plan.OutputArgs...)})
	plan.Filter = strings.Join(filters, ",")
	for _, stage := range plan.Stages {
		var verified bool
		switch {
		case stage.Operation == Upload || stage.Operation == Download:
			verified = containsOperation(req.Evidence.CrossoverStages, stage.Operation)
		case stage.Execution == Hardware:
			verified = containsOperation(req.Evidence.HardwareStages, stage.Operation)
		default:
			verified = containsOperation(req.Evidence.SoftwareStages, stage.Operation)
		}
		if !verified {
			return Plan{}, unsupported(UnverifiedStage, string(stage.Execution)+" "+string(stage.Operation)+" was not verified for this device")
		}
	}
	return plan, nil
}

func unsupported(code UnsupportedCode, detail string) error {
	return &UnsupportedError{Code: code, Detail: detail}
}
func knownBackend(v Backend) bool {
	for _, b := range []Backend{VideoToolbox, QSV, VAAPI, NVIDIA, AMF} {
		if v == b {
			return true
		}
	}
	return false
}
func containsCodec(v []Codec, x Codec) bool {
	for _, e := range v {
		if e == x {
			return true
		}
	}
	return false
}
func containsInt(v []int, x int) bool {
	for _, e := range v {
		if e == x {
			return true
		}
	}
	return false
}
func containsOperation(v []Operation, x Operation) bool {
	for _, e := range v {
		if e == x {
			return true
		}
	}
	return false
}
func containsPixelFormat(v []PixelFormat, x PixelFormat) bool {
	for _, e := range v {
		if e == x {
			return true
		}
	}
	return false
}
func validSoftwareFormat(v PixelFormat) bool {
	return v == YUV420P || v == NV12 || v == YUV420P10LE || v == P010LE
}
func formatDepth(v PixelFormat) int {
	if v == YUV420P10LE || v == P010LE {
		return 10
	}
	return 8
}
func formatForDepth(depth int, hardware bool) PixelFormat {
	if depth == 10 {
		if hardware {
			return P010LE
		}
		return YUV420P10LE
	}
	if hardware {
		return NV12
	}
	return YUV420P
}
func dimensions(prefix string, r Request) string {
	return prefix + "=" + itoa(r.Width) + ":" + itoa(r.Height)
}
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
func escapeFilterValue(v string) string {
	r := strings.NewReplacer("\\", "\\\\", ":", "\\:", "'", "\\'")
	return "'" + r.Replace(v) + "'"
}

func decoderArgs(r Request) []string {
	switch r.Backend {
	case VideoToolbox:
		return []string{"-hwaccel", "videotoolbox", "-hwaccel_output_format", "videotoolbox_vld"}
	case QSV:
		if r.OS == Linux {
			return []string{"-init_hw_device", "vaapi=portico_vaapi:" + r.Device.DevicePath, "-init_hw_device", "qsv=portico_hw@portico_vaapi", "-filter_hw_device", "portico_hw", "-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
		}
		return nil
	case VAAPI:
		return []string{"-init_hw_device", "vaapi=portico_hw:" + r.Device.DevicePath, "-filter_hw_device", "portico_hw", "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}
	case NVIDIA:
		return []string{"-init_hw_device", "cuda=portico_hw:" + r.Device.DevicePath, "-filter_hw_device", "portico_hw", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
	}
	return nil
}
func encoderName(b Backend, c Codec) string {
	suffix := map[Backend]string{VideoToolbox: "videotoolbox", QSV: "qsv", VAAPI: "vaapi", NVIDIA: "nvenc", AMF: "amf"}[b]
	return string(c) + "_" + suffix
}
func hardwareFilter(b Backend, op Operation, r Request) string {
	if op == Scale {
		return dimensions(map[Backend]string{VideoToolbox: "scale_vt", QSV: "scale_qsv", VAAPI: "scale_vaapi", NVIDIA: "scale_cuda"}[b], r)
	}
	if op == Deinterlace {
		return map[Backend]string{VideoToolbox: "yadif_videotoolbox", QSV: "deinterlace_qsv", VAAPI: "deinterlace_vaapi", NVIDIA: "yadif_cuda"}[b]
	}
	if op == ToneMap {
		return "tonemap_vaapi=format=" + string(formatForDepth(r.OutputBitDepth, true))
	}
	return ""
}
func uploadFilter(b Backend, depth int) string {
	f := string(formatForDepth(depth, true))
	switch b {
	case QSV:
		return "format=" + f + ",hwupload=extra_hw_frames=64"
	case VAAPI:
		return "format=" + f + ",hwupload"
	case NVIDIA:
		return "format=" + f + ",hwupload_cuda"
	case VideoToolbox:
		return "format=" + f + ",hwupload"
	}
	return ""
}
