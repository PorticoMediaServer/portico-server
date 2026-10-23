package decoder

import (
	"context"
	"os"
	"path/filepath"
	"portico.local/server/internal/mediaexec"
	"strconv"
	"strings"
	"time"
)

func deviceCircuitKey(backend HardwareBackend, device string) HardwareBackend {
	if device == "" {
		return backend
	}
	return HardwareBackend(string(backend) + "\x00" + device)
}
func RecordHardwareDeviceFailure(backend HardwareBackend, device string) {
	RecordHardwareFailure(deviceCircuitKey(backend, device))
}
func deviceCircuitOpen(backend HardwareBackend, device string) bool {
	return hardwareCircuitOpen(deviceCircuitKey(backend, device))
}
func deviceNeedsProbe(backend HardwareBackend, device string) bool {
	return hardwareNeedsProbe(deviceCircuitKey(backend, device))
}
func clearDeviceCircuit(backend HardwareBackend, device string) {
	clearHardwareCircuit(deviceCircuitKey(backend, device))
}

func hardwareDecodeArgs(backend HardwareBackend) []string {
	name := map[HardwareBackend]string{BackendNVENC: "cuda", BackendVAAPI: "vaapi", BackendQSV: "qsv", BackendVideoToolbox: "videotoolbox"}[backend]
	if name == "" {
		return nil
	}
	args := []string{"-hwaccel", name, "-hwaccel_output_format", name}
	if backend != BackendVideoToolbox {
		args = append(args, "-hwaccel_device", "hw")
	}
	return args
}
func nativeDeinterlace(backend HardwareBackend) string {
	return map[HardwareBackend]string{BackendNVENC: "bwdif_cuda=mode=send_frame:parity=auto:deint=interlaced", BackendVAAPI: "deinterlace_vaapi=mode=motion_adaptive:rate=frame:auto=1", BackendQSV: "vpp_qsv=deinterlace=advanced", BackendVideoToolbox: "bwdif_videotoolbox=mode=send_frame:parity=auto:deint=interlaced"}[backend]
}
func (d *HardwareDetector) probeDeinterlace(ctx context.Context, executable string, spec backendSpec, backend HardwareBackend, device string, timeout time.Duration) bool {
	filter := nativeDeinterlace(backend)
	if filter == "" {
		return false
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := probeArgs(spec, device, "")
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-vf" {
			args[i+1] += "," + filter
		}
	}
	return d.run(bounded, executable, args) == nil
}

// Decode cells require actual input decoding to a forced hardware surface.
// Software fixture generation is confined separately and uses at most one CPU
// worker. VC1 remains unprobed until a licensed fixture is supplied.
func (d *HardwareDetector) probeDecodeCodecs(ctx context.Context, executable string, backend HardwareBackend, device string, timeout time.Duration) map[string]bool {
	out := map[string]bool{"h264:8": false, "h264:10": false, "hevc:8": false, "hevc:10": false, "hevc:12": false, "av1:8": false, "vp9:8": false, "vc1:8": false, "mpeg2:8": false}
	if len(hardwareDecodeArgs(backend)) == 0 {
		return out
	}
	dir, err := os.MkdirTemp(os.TempDir(), "portico-hw-decode-")
	if err != nil {
		return out
	}
	defer os.RemoveAll(dir)
	cases := []struct{ key, encoder, format, mux string }{{"h264:8", "libx264", "yuv420p", "h264"}, {"h264:10", "libx264", "yuv420p10le", "h264"}, {"hevc:8", "libx265", "yuv420p", "hevc"}, {"hevc:10", "libx265", "yuv420p10le", "hevc"}, {"hevc:12", "libx265", "yuv420p12le", "hevc"}, {"av1:8", "libsvtav1", "yuv420p", "ivf"}, {"vp9:8", "libvpx-vp9", "yuv420p", "ivf"}, {"mpeg2:8", "mpeg2video", "yuv420p", "mpeg2video"}}
	for _, tc := range cases {
		if ctx.Err() != nil {
			break
		}
		file := filepath.Join(dir, strings.ReplaceAll(tc.key, ":", "-")+".bin")
		args := []string{"-v", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x128:rate=1", "-frames:v", "1", "-threads", "1", "-c:v", tc.encoder, "-pix_fmt", tc.format}
		if tc.encoder == "libx265" {
			args = append(args, "-x265-params", "pools=none:frame-threads=1:log-level=error")
		}
		if tc.encoder == "libsvtav1" {
			args = append(args, "-svtav1-params", "lp=1", "-preset", "12")
		}
		args = append(args, "-f", tc.mux, file)
		bounded, cancel := context.WithTimeout(ctx, timeout)
		cmd, e := mediaexec.CommandContext(bounded, mediaexec.Job{Executable: executable, Args: args, WriteDirs: []string{dir}, MaxFileBytes: 1 << 20, Background: true})
		if e == nil {
			cmd.WaitDelay = time.Second
			if d.Run != nil {
				e = d.Run(bounded, cmd)
			} else {
				buffer := &toolOutput{}
				cmd.Stderr = buffer
				e = cmd.Run()
			}
		}
		cancel()
		if e != nil {
			continue
		}
		decode := []string{"-v", "error", "-nostdin"}
		spec := backendSpecs[backend]
		if spec.device != nil {
			decode = append(decode, spec.device(device)...)
		}
		decode = append(decode, hardwareDecodeArgs(backend)...)
		decode = append(decode, "-f", tc.mux, "-i", file, "-frames:v", "1", "-vf", "hwdownload,format="+probeDownloadFormat(tc.key), "-an", "-sn", "-f", "null", "-")
		bounded, cancel = context.WithTimeout(ctx, timeout)
		out[tc.key] = d.runInput(bounded, executable, decode, []string{file}) == nil
		cancel()
	}
	return out
}
func (d *HardwareDetector) Reprobe(ctx context.Context, executable string) []HardwareProbe {
	d.mu.Lock()
	d.cache = nil
	d.discovered = nil
	d.mu.Unlock()
	for _, device := range d.candidates(ctx, "auto", "") {
		clearDeviceCircuit(device.Backend, device.Path)
	}
	return d.Report(ctx, executable, "")
}
func probeCodecKey(codec string, depth int) string {
	if depth == 0 {
		depth = 8
	}
	return codec + ":" + strconv.Itoa(depth)
}

func probeDownloadFormat(key string) string {
	if strings.HasSuffix(key, ":12") {
		return "p012le"
	}
	if strings.HasSuffix(key, ":10") {
		return "p010le"
	}
	return "nv12"
}
