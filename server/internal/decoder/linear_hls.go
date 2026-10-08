package decoder

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"portico.local/server/internal/storage"
)

type LinearEncoding struct {
	Deinterlace                          bool
	MaxFrameRate                         float64
	Profile                              string
	Level                                int
	Toolchain                            *ToolchainFacts
	HLSInput, Finite, Video, Audio       bool
	ConvertVideo, ConvertAudio, HDRToSDR bool
	SeekUS, ClipEndUS                    int64
	MaxHeight, VideoBitrate              int
}

var outputPath = regexp.MustCompile(`^/output/[a-f0-9]{64}/$`)

func linearOutputEndpoint(input, output string) (string, error) {
	endpoint, e := bridgeEndpoint(input)
	if e != nil {
		return "", e
	}
	u, e := url.Parse(output)
	if e != nil || u.Scheme != "http" || u.Host != endpoint || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || !outputPath.MatchString(u.Path) {
		return "", ErrInvalidConfiguration
	}
	return endpoint, nil
}

// LinearProbe is a bounded stream probe. Every HLS descendant has already been
// rewritten by the private gateway; protocol/file restrictions are not relaxed.
func LinearProbe(ctx context.Context, supervisor *storage.Supervisor, key, executable, input string, hls bool, reservation *EndpointReservation, physicalLock *os.File, libraries ...string) ([]byte, error) {
	defer reservation.Close()
	endpoint, e := bridgeEndpoint(input)
	if e != nil || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || !reservation.matches(endpoint) {
		return nil, ErrInvalidConfiguration
	}
	protocols, formats := "http,tcp", "mov,matroska,mp3,flac,ogg,wav,aac,mpegts,avi"
	if hls {
		protocols, formats = "http,tcp,crypto", "hls,mpegts,mov,aac,mp3"
	}
	args := []string{"-v", "error", "-threads", "1", "-protocol_whitelist", protocols, "-format_whitelist", formats, "-rw_timeout", "15000000", "-probesize", "8388608", "-analyzeduration", "5000000", "-show_format", "-show_streams", "-of", "json"}
	if hls {
		args = append(args, "-allowed_extensions", "ALL")
	}
	args = append(args, input)
	for _, file := range libraries {
		if !filepath.IsAbs(file) || filepath.Clean(file) != file {
			return nil, ErrInvalidConfiguration
		}
	}
	cmd, e := confinedCommand(executable, args, endpoint, libraries...)
	if e != nil {
		return nil, e
	}
	if physicalLock != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, physicalLock)
	}
	return runOwned(ctx, supervisor, key, cmd, endpoint, reservation)
}
func linearHLSArgs(input, output string, p LinearEncoding) ([]string, error) {
	if !p.Video && !p.Audio || p.SeekUS < 0 || p.ClipEndUS < 0 || p.Finite && p.ClipEndUS <= p.SeekUS || p.MaxHeight < 0 || p.MaxHeight > 4320 || p.VideoBitrate < 0 || p.VideoBitrate > 200000000 || p.HDRToSDR && !p.ConvertVideo {
		return nil, ErrInvalidConfiguration
	}
	protocols, formats := "http,tcp", "mpegts"
	if p.HLSInput {
		protocols, formats = "http,tcp,crypto", "hls,mpegts,mov,aac,mp3"
	} else if p.Finite {
		formats = "mov,matroska,mp3,flac,ogg,wav,aac,mpegts,avi"
	}
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-threads", "1", "-protocol_whitelist", protocols, "-format_whitelist", formats, "-rw_timeout", "15000000", "-probesize", "8388608", "-analyzeduration", "5000000", "-copyts", "-start_at_zero"}
	if p.HLSInput {
		args = append(args, "-allowed_extensions", "ALL")
	}
	seconds := func(n int64) string { return strconv.FormatFloat(float64(n)/1e6, 'f', 6, 64) }
	if p.Finite {
		args = append(args, "-readrate", "1", "-readrate_initial_burst", "12", "-ss", seconds(p.SeekUS))
	}
	var graph ConversionGraph
	if p.ConvertVideo {
		var err error
		graph, err = BuildConversion(ConversionRequest{ConvertVideo: true, ToneMap: p.HDRToSDR, Deinterlace: p.Deinterlace, MaxHeight: p.MaxHeight, MaxFrameRate: p.MaxFrameRate, VideoBitrateBPS: p.VideoBitrate, KeyframeSeconds: 4, Toolchain: p.Toolchain})
		if err != nil {
			return nil, err
		}
		args = append(args, graph.Input...)
	}
	args = append(args, "-i", input)
	// With copyts/start_at_zero, the stop coordinate is on the preserved source
	// clock, not the duration since this worker's mid-programme join.
	if p.Finite {
		args = append(args, "-t", seconds(p.ClipEndUS))
	}
	if p.Video {
		args = append(args, "-map", "0:v:0", "-c:v", "copy")
	}
	if p.Audio {
		args = append(args, "-map", "0:a:0", "-c:a", "copy")
	}
	if p.ConvertVideo {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-threads:v", "2", "-pix_fmt", "yuv420p", "-force_key_frames", "expr:if(isnan(prev_forced_t),1,gte(t,prev_forced_t+4))", "-sc_threshold", "0")
		args = append(args, graph.Video...)
		if p.Profile != "" {
			args = append(args, "-profile:v", p.Profile)
		}
		if p.Level > 0 {
			args = append(args, "-level:v", strconv.FormatFloat(float64(p.Level)/10, 'f', 1, 64))
		}

	}
	if p.ConvertAudio {
		args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-ar", "48000")
	}
	args = append(args, "-sn", "-dn", "-avoid_negative_ts", "disabled", "-muxdelay", "0", "-f", "hls", "-hls_time", "4", "-hls_list_size", "12", "-hls_flags", "independent_segments+program_date_time", "-hls_segment_options", "mpegts_copyts=1", "-method", "PUT", "-rw_timeout", "15000000", "-hls_segment_filename", output+"segment-%09d.ts", output+"index.m3u8")
	return args, nil
}

// RunLinearHLS retains the process, private endpoints and optional physical tuner
// lock until actual child exit. PUT output contains only complete muxed media.
func RunLinearHLS(ctx context.Context, supervisor *storage.Supervisor, key, executable, input, output string, p LinearEncoding, reservation *EndpointReservation, physicalLock *os.File, libraries ...string) error {
	defer reservation.Close()
	endpoint, e := linearOutputEndpoint(input, output)
	if e != nil || supervisor == nil || key == "" || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || !reservation.matches(endpoint) {
		return ErrInvalidConfiguration
	}
	for _, file := range libraries {
		if !filepath.IsAbs(file) || filepath.Clean(file) != file {
			return ErrInvalidConfiguration
		}
	}
	p.Toolchain = CurrentToolchain()
	args, e := linearHLSArgs(input, output, p)
	if e != nil {
		return e
	}
	// Only this job's reserved output path may be written through the sandbox.
	written, e := url.Parse(output)
	if e != nil {
		return ErrInvalidConfiguration
	}
	cmd, e := confinedOutputCommand(executable, args, endpoint, written.Path, libraries...)
	if e != nil {
		return e
	}
	if physicalLock != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, physicalLock)
	}
	if !reservation.claim(endpoint, cmd) {
		return ErrReservationInUse
	}
	retired := make(chan struct{})
	e = supervisor.RunOwnedLinear(ctx, "playback:linear:"+key, cmd, func(r io.Reader) error { _, e := io.Copy(io.Discard, r); return e }, nil, func() { reservation.retire(); close(retired) })
	<-retired
	return e
}
