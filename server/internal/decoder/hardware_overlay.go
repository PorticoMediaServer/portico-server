package decoder

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func hardwareOverlay(backend HardwareBackend) string {
	return map[HardwareBackend]string{BackendNVENC: "overlay_cuda", BackendVAAPI: "overlay_vaapi", BackendQSV: "overlay_qsv", BackendVideoToolbox: "overlay_videotoolbox"}[backend]
}
func subtitleUpload(backend HardwareBackend, dynamicRange string) string {
	pixel := "bgra"
	if backend == BackendNVENC {
		pixel = "yuva420p"
	}
	return "format=" + pixel + "," + map[HardwareBackend]string{BackendNVENC: "hwupload_cuda", BackendVAAPI: "hwupload", BackendQSV: "hwupload=extra_hw_frames=32", BackendVideoToolbox: "hwupload"}[backend]
}

func hardwareBurnFilter(b BurnIn, facts *ToolchainFacts, backend HardwareBackend) (string, []string, error) {
	// Reuse the validated cue origin, fonts and canvas contract.
	sw, extra, err := burnFilter(b, facts)
	if err != nil {
		return "", nil, err
	}
	overlay := hardwareOverlay(backend)
	if overlay == "" {
		return "", nil, ErrInvalidConfiguration
	}
	if b.Format == "ass" || b.Format == "ssa" {
		if b.Width < 1 || b.Height < 1 {
			return "", nil, ErrInvalidConfiguration
		}
		shift := seconds(b.PositionUS - b.OffsetUS)
		draw := "setpts=PTS+(" + shift + ")/TB,ass=filename=" + escapeFilterPath(b.File) + ":alpha=1"
		if b.FontsDirectory != "" {
			draw += ":fontsdir=" + escapeFilterPath(b.FontsDirectory)
		}
		draw += fmt.Sprintf(":original_size=%dx%d,setpts=PTS-(%s)/TB", b.Width, b.Height, shift)
		cue := fmt.Sprintf("color=c=black@0:s=%dx%d:r=60,format=rgba,%s", b.Width, b.Height, draw)
		if b.DynamicRange == "hdr10" || b.DynamicRange == "hlg" {
			cue += "," + hdrSubtitleFilter(b.DynamicRange)
		}
		return "null[burn_picture];" + cue + "," + subtitleUpload(backend, b.DynamicRange) + "[burn_cues];[burn_picture][burn_cues]" + overlay + "=eof_action=pass:shortest=1", nil, nil
	}
	// A bitmap decoder renders only the subtitle plane in system memory. The
	// movie remains in GPU frames; the actual composition runs in the overlay.
	sw = strings.Replace(sw, "[burn_cues];[burn_picture][burn_cues]overlay=format=yuv420p10:eof_action=pass:shortest=0", ","+subtitleUpload(backend, b.DynamicRange)+"[burn_cues];[burn_picture][burn_cues]"+overlay+"=eof_action=pass:shortest=0", 1)
	sw = strings.Replace(sw, "[burn_cues];[burn_picture][burn_cues]overlay=eof_action=pass:shortest=0", ","+subtitleUpload(backend, b.DynamicRange)+"[burn_cues];[burn_picture][burn_cues]"+overlay+"=eof_action=pass:shortest=0", 1)
	return sw, extra, nil
}
func (d *HardwareDetector) probeOverlay(ctx context.Context, executable string, spec backendSpec, backend HardwareBackend, device string, timeout time.Duration) bool {
	overlay := hardwareOverlay(backend)
	if overlay == "" || spec.upload == "" {
		return false
	}
	args := probeArgs(spec, device, "")
	filtered := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "-vf" {
			i++
			continue
		}
		filtered = append(filtered, args[i])
	}
	graph := "[0:v]" + spec.upload + "[picture];color=c=black@0:s=160x120:r=10,format=rgba," + subtitleUpload(backend, "sdr") + "[cue];[picture][cue]" + overlay + "=shortest=1[out]"
	filtered = append(filtered[:len(filtered)-4], append([]string{"-filter_complex", graph, "-map", "[out]"}, filtered[len(filtered)-4:]...)...)
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return d.run(bounded, executable, filtered) == nil
}

func nativeToneFilter(backend HardwareBackend, algorithm string) string {
	switch backend {
	case BackendVAAPI:
		return "hwmap=derive_device=opencl,tonemap_opencl=tonemap=" + algorithm + ":desat=0:format=nv12,hwmap=derive_device=vaapi:reverse=1"
	case BackendQSV:
		return "hwmap=derive_device=vaapi,hwmap=derive_device=opencl,tonemap_opencl=tonemap=" + algorithm + ":desat=0:format=nv12,hwmap=derive_device=vaapi:reverse=1,hwmap=derive_device=qsv"
	case BackendVideoToolbox:
		return "tonemap_videotoolbox=tonemap=" + algorithm + ":desat=0:format=nv12"
	case BackendNVENC:
		return "tonemap_cuda=tonemap=" + algorithm + ":desat=0:format=yuv420p"
	}
	return ""
}
func nativeScale(backend HardwareBackend, h int) string { return "scale_vt=w=-2:h=" + strconv.Itoa(h) }

func nativeHDRReshape(backend HardwareBackend) string {
	color := ":tonemap=none:apply_dovi=1:format=p010:p=bt2020:t=smpte2084:m=bt2020nc"
	switch backend {
	case BackendNVENC:
		return "tonemap_cuda" + color
	case BackendVideoToolbox:
		return "tonemap_videotoolbox" + color
	case BackendVAAPI:
		return "hwmap=derive_device=opencl,tonemap_opencl" + color + ",hwmap=derive_device=vaapi:reverse=1"
	case BackendQSV:
		return "hwmap=derive_device=vaapi,hwmap=derive_device=opencl,tonemap_opencl" + color + ",hwmap=derive_device=vaapi:reverse=1,hwmap=derive_device=qsv"
	}
	return ""
}
func vulkanColorFilter(algorithm, dynamicRange string) string {
	if dynamicRange == "hdr10" {
		return "libplacebo=apply_dolbyvision=1:tonemapping=clip:colorspace=bt2020nc:color_primaries=bt2020:color_trc=smpte2084:format=yuv420p10le"
	}
	return "libplacebo=apply_dolbyvision=1:tonemapping=" + algorithm + ":colorspace=bt709:color_primaries=bt709:color_trc=bt709:format=yuv420p"
}
