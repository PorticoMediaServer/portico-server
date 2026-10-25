package decoder

import (
	"context"
	"errors"
	"time"
)

// errHardwareBurnUnsupported means the device cannot do this burn-in, so the
// processor path is the right one and nothing is lost but a copy.
var errHardwareBurnUnsupported = errors.New("hardware_subtitle_burn_in_unsupported")

// hardwareBurnIn is one backend's own subtitle composite: the picture and the
// bitmap subtitle surface live on the same device, and the device blends them.
// The upload moves a system frame in the subtitle's pixel format onto that
// device; the overlay keeps the picture in the encoder's own surface format.
type hardwareBurnIn struct {
	upload  string
	overlay string
}

// hardwareBurnInFor names the composite for a backend that has one. Apple's
// VideoToolbox and AMD's AMF have no such filter, so they keep the processor
// burn-in; nothing is claimed for them here.
func hardwareBurnInFor(backend HardwareBackend) (hardwareBurnIn, bool) {
	switch backend {
	case BackendVAAPI:
		return hardwareBurnIn{upload: "hwupload", overlay: "overlay_vaapi=eof_action=pass:shortest=0"}, true
	case BackendNVENC:
		return hardwareBurnIn{upload: "hwupload_cuda", overlay: "overlay_cuda=eof_action=pass:shortest=0"}, true
	case BackendQSV:
		// The same frames pool the backend's own upload asks for.
		return hardwareBurnIn{upload: "hwupload=extra_hw_frames=32", overlay: "overlay_qsv=eof_action=pass:shortest=0"}, true
	}
	return hardwareBurnIn{}, false
}

// hardwareBurnInSupported answers whether this burn-in can run on the device at
// all: a bitmap subtitle, a standard-range picture, and a backend whose own
// composite filter exists. Text subtitles are rendered by libass on system
// frames, and an HDR subtitle surface carries both 10-bit colour and alpha the
// device upload cannot hold at once, so both stay on the processor.
func hardwareBurnInSupported(backend HardwareBackend, b BurnIn, target ConversionTarget) bool {
	if _, ok := hardwareBurnInFor(backend); !ok {
		return false
	}
	// The probe proves the 8-bit standard-range surface a session would use; a
	// 10-bit or HDR picture needs a surface the probe did not test.
	if target.DynamicRange != "sdr" || target.BitDepth != 8 || b.DynamicRange != "" && b.DynamicRange != "sdr" {
		return false
	}
	switch b.Format {
	case "pgs", "vobsub", "mks", "dvb":
		return b.Width >= 1 && b.Height >= 1 && b.Width <= 8192 && b.Height <= 4320
	}
	return false
}

// hardwareSubtitleChain builds the subtitle half of the composite: the same
// clock shift and the same fit the processor path uses, then the upload onto the
// device. It answers the extra input arguments the graph needs.
func hardwareSubtitleChain(backend HardwareBackend, b BurnIn) (string, []string, error) {
	composite, ok := hardwareBurnInFor(backend)
	if !ok {
		return "", nil, errHardwareBurnUnsupported
	}
	inputs, err := bitmapSubtitleInput(b)
	if err != nil {
		return "", nil, err
	}
	chain := "[1:s:0]settb=AVTB,setpts=PTS+(" + seconds(b.OffsetUS-b.PositionUS) + ")/TB,format=rgba," + subtitleFit(b) + "," + composite.upload + "[burn_cues]"
	return chain, inputs, nil
}

// hardwareBurnInGraph is the whole composite: the accumulated picture chain
// stays on the device and the device blends the subtitle surface into it. Both
// the production conversion and the probe build this same graph, so a probe that
// passes is a probe of the filter set a session will run.
func hardwareBurnInGraph(backend HardwareBackend, b BurnIn, picture string) (string, []string, error) {
	composite, ok := hardwareBurnInFor(backend)
	if !ok {
		return "", nil, errHardwareBurnUnsupported
	}
	cues, inputs, err := hardwareSubtitleChain(backend, b)
	if err != nil {
		return "", nil, err
	}
	return "[0:v:0]" + picture + "[burn_picture];" + cues + ";[burn_picture][burn_cues]" + composite.overlay + "[burn_output]", inputs, nil
}

// probeBurnIn proves the composite on this device with the graph a session
// would run: the picture uploaded and scaled to the encoder surface, a synthetic
// subtitle with real alpha moved onto the same device, the blend, and the result
// read back. A device or driver that cannot hold an alpha surface fails here, so
// the session keeps the processor burn-in instead of discovering it mid-stream.
func (d *HardwareDetector) probeBurnIn(ctx context.Context, executable string, spec backendSpec, backend HardwareBackend, device string, timeout time.Duration) bool {
	composite, ok := hardwareBurnInFor(backend)
	if !ok || spec.upload == "" {
		return false
	}
	picture := spec.upload
	if spec.scale != nil {
		picture += "," + spec.scale(120)
	}
	graph := "[0:v]" + picture + "[burn_picture];[1:v]format=rgba,scale=160:120," + composite.upload + "[burn_cues];[burn_picture][burn_cues]" + composite.overlay + ",hwdownload,format=nv12[burn_out]"
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	if spec.device != nil {
		args = append(args, spec.device(device)...)
	}
	args = append(args,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10:duration=0.5",
		"-f", "lavfi", "-i", "color=c=black@0.0:s=160x120:rate=10:duration=0.5",
		"-filter_complex", graph, "-map", "[burn_out]", "-frames:v", "3", "-an", "-sn", "-dn", "-f", "null", "-")
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return d.run(bounded, executable, args) == nil
}
