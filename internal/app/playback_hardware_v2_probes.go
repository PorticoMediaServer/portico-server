package app

import (
	"fmt"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegprobe"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
)

// playbackHardwareProbes creates only bounded, executable assertions. No
// -encoders/-filters/-hwaccels listing command appears here because listings do
// not prove that the selected device and driver can execute a graph.
func playbackHardwareProbes(backend playbackhw.Backend, pipeline playbackhw.Request, inputs map[playbackhw.Codec]string) ([]ffmpegprobe.Probe, playbackHardwareProbeSpec, error) {
	if pipeline.OutputCodec == "" {
		return nil, playbackHardwareProbeSpec{}, hardwareProbeFailure("output codec")
	}
	if pipeline.InputBitDepth == 0 {
		pipeline.InputBitDepth = 8
	}
	if pipeline.OutputBitDepth == 0 {
		pipeline.OutputBitDepth = 8
	}
	if pipeline.InputPixelFormat == "" {
		if pipeline.InputBitDepth == 10 {
			pipeline.InputPixelFormat = playbackhw.YUV420P10LE
		} else {
			pipeline.InputPixelFormat = playbackhw.YUV420P
		}
	}
	if pipeline.OutputPixelFormat == "" {
		if pipeline.OutputBitDepth == 10 {
			pipeline.OutputPixelFormat = playbackhw.YUV420P10LE
		} else {
			pipeline.OutputPixelFormat = playbackhw.YUV420P
		}
	}
	encodeOnly := pipeline.EncodeOnly || backend == playbackhw.AMF
	if !encodeOnly && strings.TrimSpace(inputs[pipeline.InputCodec]) == "" {
		return nil, playbackHardwareProbeSpec{}, hardwareProbeFailure(string(pipeline.InputCodec) + " decode vector")
	}

	spec := playbackHardwareProbeSpec{
		decode: pipeline.InputCodec, encode: pipeline.OutputCodec,
		bitDepths:      uniqueHardwareInts([]int{pipeline.InputBitDepth, pipeline.OutputBitDepth}),
		pixelFormats:   uniqueHardwarePixelFormats([]playbackhw.PixelFormat{pipeline.InputPixelFormat, pipeline.OutputPixelFormat}),
		hardwareStages: []playbackhw.Operation{playbackhw.Encode},
	}
	if encodeOnly && (backend == playbackhw.QSV || backend == playbackhw.VAAPI || backend == playbackhw.NVIDIA) {
		spec.crossovers = append(spec.crossovers, playbackhw.Upload)
	}
	probes := []ffmpegprobe.Probe{}
	if !encodeOnly {
		spec.hardwareStages = append(spec.hardwareStages, playbackhw.Decode)
		if backend == playbackhw.VideoToolbox {
			spec.crossovers = append(spec.crossovers, playbackhw.Download)
		}
		probes = append(probes, executableHardwareProbe("decode-"+string(pipeline.InputCodec), ffmpegprobe.Decode,
			append(append([]string{"-hide_banner", "-nostdin", "-v", "error"}, playbackHardwareDecoderArgs(backend, pipeline.Device.DevicePath)...),
				"-i", inputs[pipeline.InputCodec], "-frames:v", "1", "-f", "null", "-")...))
	}
	encodeArgs := playbackHardwareSyntheticEncodeArgs(backend, pipeline.OutputCodec, pipeline.OutputBitDepth, pipeline.Device.DevicePath, nil)
	probes = append(probes,
		executableHardwareProbe("encode-"+string(pipeline.OutputCodec), ffmpegprobe.Encode, encodeArgs...),
		executableHardwareProbe(fmt.Sprintf("bit-depth-%d", pipeline.OutputBitDepth), map[bool]ffmpegprobe.Kind{true: ffmpegprobe.BitDepth10, false: ffmpegprobe.BitDepth8}[pipeline.OutputBitDepth == 10], encodeArgs...),
	)

	filterArgs := []string{}
	if pipeline.Deinterlace {
		filter := playbackHardwareFilterName(backend, playbackhw.Deinterlace, pipeline)
		if filter != "" {
			spec.hardwareStages = append(spec.hardwareStages, playbackhw.Deinterlace)
			filterArgs = append(filterArgs, filter)
		} else {
			spec.softwareStages = append(spec.softwareStages, playbackhw.Deinterlace)
			if !encodeOnly {
				spec.crossovers = append(spec.crossovers, playbackHardwareSoftwareFrameCrossovers(backend)...)
			}
		}
	}
	if pipeline.ToneMap {
		filter := playbackHardwareFilterName(backend, playbackhw.ToneMap, pipeline)
		if filter != "" {
			spec.hardwareStages = append(spec.hardwareStages, playbackhw.ToneMap)
			filterArgs = append(filterArgs, filter)
		} else {
			spec.softwareStages = append(spec.softwareStages, playbackhw.ToneMap)
			if !encodeOnly {
				spec.crossovers = append(spec.crossovers, playbackHardwareSoftwareFrameCrossovers(backend)...)
			}
		}
	}
	if pipeline.Width > 0 {
		filter := playbackHardwareFilterName(backend, playbackhw.Scale, pipeline)
		if filter != "" {
			spec.hardwareStages = append(spec.hardwareStages, playbackhw.Scale)
			filterArgs = append(filterArgs, filter)
		} else {
			spec.softwareStages = append(spec.softwareStages, playbackhw.Scale)
			if !encodeOnly {
				spec.crossovers = append(spec.crossovers, playbackHardwareSoftwareFrameCrossovers(backend)...)
			}
		}
	}
	if pipeline.SubtitleFile != "" {
		spec.softwareStages = append(spec.softwareStages, playbackhw.SubtitleBurn)
		if !encodeOnly {
			spec.crossovers = append(spec.crossovers, playbackHardwareSoftwareFrameCrossovers(backend)...)
		}
	}
	if backend == playbackhw.VideoToolbox && !encodeOnly &&
		(pipeline.InputPixelFormat != pipeline.OutputPixelFormat || pipeline.InputBitDepth != pipeline.OutputBitDepth) {
		// Prove the exact software-frame format crossover used between
		// VideoToolbox decode and encode.
		spec.softwareStages = append(spec.softwareStages, playbackhw.Scale)
		spec.crossovers = append(spec.crossovers, playbackhw.Download)
	}
	if len(filterArgs) > 0 {
		kind := ffmpegprobe.ScaleDeinterlace
		if pipeline.ToneMap {
			kind = ffmpegprobe.HDRToneMap
		}
		probes = append(probes, executableHardwareProbe("hardware-filter-graph", kind,
			playbackHardwareSyntheticEncodeArgs(backend, pipeline.OutputCodec, pipeline.OutputBitDepth, pipeline.Device.DevicePath, filterArgs)...))
	}
	// Software/crossover operations are verified as a single real decode ->
	// download -> filter -> upload -> encode graph against the corpus vector.
	if len(spec.softwareStages) > 0 || len(spec.crossovers) > 0 {
		if strings.TrimSpace(inputs[pipeline.InputCodec]) == "" {
			return nil, playbackHardwareProbeSpec{}, hardwareProbeFailure("crossover vector")
		}
		args := playbackHardwareCrossoverArgs(backend, pipeline, inputs[pipeline.InputCodec])
		probes = append(probes, executableHardwareProbe("download-filter-reupload", ffmpegprobe.DownloadReupload, args...))
	}
	spec.hardwareStages = uniqueHardwareOperations(spec.hardwareStages)
	spec.softwareStages = uniqueHardwareOperations(spec.softwareStages)
	spec.crossovers = uniqueHardwareOperations(spec.crossovers)
	return probes, spec, nil
}

func playbackHardwareSoftwareFrameCrossovers(backend playbackhw.Backend) []playbackhw.Operation {
	if backend == playbackhw.VideoToolbox {
		return []playbackhw.Operation{playbackhw.Download}
	}
	return []playbackhw.Operation{playbackhw.Download, playbackhw.Upload}
}

func uniqueHardwareInts(values []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value > 0 && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func uniqueHardwarePixelFormats(values []playbackhw.PixelFormat) []playbackhw.PixelFormat {
	seen := map[playbackhw.PixelFormat]bool{}
	out := make([]playbackhw.PixelFormat, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func executableHardwareProbe(name string, kind ffmpegprobe.Kind, args ...string) ffmpegprobe.Probe {
	return ffmpegprobe.Probe{
		Name: name, Kind: kind, Args: args,
		UnsupportedExitCodes:         []int{1, 22, 38},
		UnsupportedDiagnosticMarkers: []string{"operation not supported", "not supported", "unsupported pixel format", "no capable devices found"},
		UnavailableDiagnosticMarkers: []string{"no device available", "cannot load libcuda", "failed to initialise vaapi connection", "device creation failed", "mfx session failed"},
	}
}

func playbackHardwareDecoderArgs(backend playbackhw.Backend, path string) []string {
	switch backend {
	case playbackhw.VideoToolbox:
		return []string{"-hwaccel", "videotoolbox"}
	case playbackhw.QSV:
		return []string{"-init_hw_device", "vaapi=portico_vaapi:" + path, "-init_hw_device", "qsv=portico_probe@portico_vaapi", "-filter_hw_device", "portico_probe", "-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
	case playbackhw.VAAPI:
		return []string{"-init_hw_device", "vaapi=portico_probe:" + path, "-filter_hw_device", "portico_probe", "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}
	case playbackhw.NVIDIA:
		return []string{"-init_hw_device", "cuda=portico_probe:" + path, "-filter_hw_device", "portico_probe", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
	default:
		return nil
	}
}

func playbackHardwareSyntheticEncodeArgs(backend playbackhw.Backend, codec playbackhw.Codec, depth int, path string, filters []string) []string {
	pix := "yuv420p"
	if depth == 10 {
		pix = "yuv420p10le"
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error"}
	if backend == playbackhw.QSV {
		args = append(args, playbackHardwareDecoderArgs(backend, path)[:6]...)
	} else if backend == playbackhw.VAAPI || backend == playbackhw.NVIDIA {
		args = append(args, playbackHardwareDecoderArgs(backend, path)[:4]...)
	}
	args = append(args, "-f", "lavfi", "-i", "color=size=64x64:rate=1:duration=1,format="+pix)
	graph := append([]string{}, filters...)
	// VideoToolbox encoders accept software NV12 frames directly and create
	// their own device context. A generic hwupload filter has no device to bind
	// on macOS and makes an otherwise executable encoder probe fail closed.
	if backend == playbackhw.VideoToolbox {
		format := "nv12"
		if depth == 10 {
			format = "p010le"
		}
		graph = append([]string{"format=" + format}, graph...)
	} else if backend != playbackhw.AMF {
		graph = append([]string{playbackHardwareUploadFilter(backend, depth)}, graph...)
	}
	if len(graph) > 0 {
		args = append(args, "-vf", strings.Join(graph, ","))
	}
	return append(args, "-frames:v", "1", "-c:v", playbackHardwareEncoderName(backend, codec), "-f", "null", "-")
}

func playbackHardwareCrossoverArgs(backend playbackhw.Backend, pipeline playbackhw.Request, input string) []string {
	args := []string{"-hide_banner", "-nostdin", "-v", "error"}
	if !pipeline.EncodeOnly {
		args = append(args, playbackHardwareDecoderArgs(backend, pipeline.Device.DevicePath)...)
	}
	args = append(args, "-i", input)
	softwareFormat := "yuv420p"
	if pipeline.OutputBitDepth == 10 {
		softwareFormat = "yuv420p10le"
	}
	filters := []string{}
	if !pipeline.EncodeOnly {
		if backend == playbackhw.VideoToolbox {
			filters = append(filters, "format="+softwareFormat)
		} else {
			filters = append(filters, "hwdownload", "format="+softwareFormat)
		}
	}
	if pipeline.Deinterlace {
		filters = append(filters, "bwdif")
	}
	if pipeline.ToneMap {
		filters = append(filters, "zscale=transfer=linear", "tonemap=mobius", "zscale=transfer=bt709:primaries=bt709:matrix=bt709")
	}
	if pipeline.Width > 0 {
		filters = append(filters, fmt.Sprintf("scale=%d:%d", pipeline.Width, pipeline.Height))
	}
	if pipeline.SubtitleFile != "" {
		filters = append(filters, "subtitles='"+strings.ReplaceAll(pipeline.SubtitleFile, "'", "\\'")+"'")
	}
	if !pipeline.EncodeOnly && backend != playbackhw.VideoToolbox {
		filters = append(filters, playbackHardwareUploadFilter(backend, pipeline.OutputBitDepth))
	}
	return append(args, "-vf", strings.Join(filters, ","), "-frames:v", "1", "-c:v", playbackHardwareEncoderName(backend, pipeline.OutputCodec), "-f", "null", "-")
}

func playbackHardwareUploadFilter(backend playbackhw.Backend, depth int) string {
	format := "nv12"
	if depth == 10 {
		format = "p010le"
	}
	switch backend {
	case playbackhw.NVIDIA:
		return "format=" + format + ",hwupload_cuda"
	case playbackhw.QSV:
		return "format=" + format + ",hwupload=extra_hw_frames=64"
	default:
		return "format=" + format + ",hwupload"
	}
}

func playbackHardwareEncoderName(backend playbackhw.Backend, codec playbackhw.Codec) string {
	suffix := map[playbackhw.Backend]string{playbackhw.VideoToolbox: "videotoolbox", playbackhw.QSV: "qsv", playbackhw.VAAPI: "vaapi", playbackhw.NVIDIA: "nvenc", playbackhw.AMF: "amf"}[backend]
	return string(codec) + "_" + suffix
}

func playbackHardwareFilterName(backend playbackhw.Backend, operation playbackhw.Operation, pipeline playbackhw.Request) string {
	switch operation {
	case playbackhw.Scale:
		name := map[playbackhw.Backend]string{playbackhw.VideoToolbox: "scale_vt", playbackhw.QSV: "scale_qsv", playbackhw.VAAPI: "scale_vaapi", playbackhw.NVIDIA: "scale_cuda"}[backend]
		if name != "" {
			return fmt.Sprintf("%s=%d:%d", name, pipeline.Width, pipeline.Height)
		}
	case playbackhw.Deinterlace:
		return map[playbackhw.Backend]string{playbackhw.QSV: "deinterlace_qsv", playbackhw.VAAPI: "deinterlace_vaapi", playbackhw.NVIDIA: "yadif_cuda"}[backend]
	case playbackhw.ToneMap:
		if backend == playbackhw.VAAPI {
			return "tonemap_vaapi=format=" + map[bool]string{true: "p010le", false: "nv12"}[pipeline.OutputBitDepth == 10]
		}
	}
	return ""
}
