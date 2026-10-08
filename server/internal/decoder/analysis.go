package decoder

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"portico.local/server/internal/storage"
)

// AnalysisSpec is a closed recipe. No user filter, filename, protocol or encoder
// argument enters the decoder. All output is streamed through supervised stdout.
type AnalysisSpec struct {
	PhysicalLock  *os.File
	Kind          string
	StartUS       int64
	IntervalUS    int64
	MaxFrames     int
	MaxDurationUS int64
	// Width/Height select the trickplay frame geometry. Zero keeps the historic
	// 320x180 preview frame. Both must be even, bounded, and chosen by the
	// caller from validated policy, never from media or a client request.
	Width, Height int
	// Audio conversion (spec §18.1 converted mode): raw decoded frames
	// [AudioStart, AudioStart+AudioFrames) of the source, resampled to AudioRate
	// and AudioChannels, padded or cut to exactly AudioOut frames, encoded as FLAC
	// (AudioBitDepth 16 or 24) or Opus (AudioBitrate bits per second).
	AudioStart, AudioFrames, AudioOut int64
	AudioRate, AudioChannels          int
	AudioBitDepth, AudioBitrate       int
}

func AnalysisArgs(input string, s AnalysisSpec) ([]string, error) {
	if s.StartUS < 0 || s.MaxDurationUS <= 0 || s.StartUS >= s.MaxDurationUS || s.MaxDurationUS > (1<<53)-1 {
		return nil, ErrInvalidConfiguration
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-xerror", "-threads", "1", "-filter_threads", "1"}
	if s.Kind == "image" {
		args = append(args, "-ss", fmt.Sprintf("%d.%06d", s.StartUS/1000000, s.StartUS%1000000))
	}
	if s.Kind == "audio_flac" || s.Kind == "audio_opus" {
		// Every decoded frame, priming included: the facts' trim is relative to it.
		args = append(args, "-flags2", "+skip_manual")
	}
	args = append(args, "-protocol_whitelist", "http,tcp", "-format_whitelist", "mov,matroska,mp3,flac,ogg,wav,aac,mpegts,avi,aiff,asf,ape,wv,tta,amr,mpeg,mpegvideo", "-probesize", "8388608", "-analyzeduration", "10000000", "-i", input, "-map_metadata", "-1", "-map_chapters", "-1", "-threads", "1")
	switch s.Kind {
	case "pcm":
		args = append(args, "-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-af", "aresample=11025:async=1:first_pts=0", "-ar", "11025", "-c:a", "pcm_s16le", "-f", "s16le", "pipe:1")
	case "loudness":
		// ametadata's file=- writes metadata to stdout; null emits no media bytes.
		args = append(args, "-map", "0:a:0", "-vn", "-sn", "-dn", "-af", "ebur128=metadata=1:peak=true,ametadata=mode=print:file=-", "-f", "null", "pipe:1")
	case "audio_flac", "audio_opus":
		if s.AudioStart < 0 || s.AudioFrames <= 0 || s.AudioOut <= 0 || s.AudioRate < 8000 || s.AudioRate > 768000 || s.AudioChannels < 1 || s.AudioChannels > 8 {
			return nil, ErrInvalidConfiguration
		}
		// asetpts counts samples from the first decoded frame, so the trim is in raw
		// frames whatever timestamps the container gave the priming; again after
		// resampling, whose first output timestamp needn't be 0, so the final cut is
		// exact.
		filter := fmt.Sprintf("asetpts=N/SR/TB,atrim=start_sample=%d:end_sample=%d,asetpts=N/SR/TB,aresample=%d,asetpts=N/SR/TB,apad=whole_len=%d,atrim=end_sample=%d", s.AudioStart, s.AudioStart+s.AudioFrames, s.AudioRate, s.AudioOut, s.AudioOut)
		args = append(args, "-map", "0:a:0", "-vn", "-sn", "-dn", "-af", filter, "-ac", fmt.Sprint(s.AudioChannels), "-ar", fmt.Sprint(s.AudioRate))
		if s.Kind == "audio_flac" {
			format, depth := "s16", "16"
			if s.AudioBitDepth == 24 {
				format, depth = "s32", "24"
			} else if s.AudioBitDepth != 16 {
				return nil, ErrInvalidConfiguration
			}
			args = append(args, "-c:a", "flac", "-sample_fmt", format, "-bits_per_raw_sample", depth, "-f", "flac", "pipe:1")
		} else {
			if s.AudioRate != 48000 || s.AudioBitrate < 6000 || s.AudioBitrate > 512000 {
				return nil, ErrInvalidConfiguration
			}
			args = append(args, "-c:a", "libopus", "-b:a", fmt.Sprint(s.AudioBitrate), "-frame_duration", "20", "-application", "audio", "-f", "ogg", "pipe:1")
		}
	case "image":
		args = append(args, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", "scale=320:180:force_original_aspect_ratio=decrease,pad=320:180:(ow-iw)/2:(oh-ih)/2,setsar=1", "-frames:v", "1", "-c:v", "mjpeg", "-q:v", "3", "-f", "image2pipe", "pipe:1")
	case "trickplay", "scenes":
		if s.IntervalUS < 100000 || s.MaxFrames < 1 || s.MaxFrames > 65536 {
			return nil, ErrInvalidConfiguration
		}
		width, height := s.Width, s.Height
		if width == 0 && height == 0 {
			width, height = 320, 180
		}
		if width < 16 || height < 16 || width > 1920 || height > 1080 || width%2 != 0 || height%2 != 0 {
			return nil, ErrInvalidConfiguration
		}
		scale, pixel := fmt.Sprintf("%d:%d", width, height), "rgb24"
		if s.Kind == "scenes" {
			if s.Width != 0 || s.Height != 0 {
				return nil, ErrInvalidConfiguration
			}
			scale, pixel = "32:18", "gray"
		}
		filter := fmt.Sprintf("fps=1000000/%d:start_time=0,scale=%s:force_original_aspect_ratio=decrease,pad=%s:(ow-iw)/2:(oh-ih)/2,setsar=1,format=%s", s.IntervalUS, scale, scale, pixel)
		args = append(args, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", filter, "-pix_fmt", pixel, "-f", "rawvideo", "pipe:1")
	default:
		return nil, ErrInvalidConfiguration
	}
	return args, nil
}

// RunAnalysis waits for actual process retirement even after cancellation. This
// deliberately uses a background key so playback retains its reserved slot.
func RunAnalysis(ctx context.Context, sup *storage.Supervisor, key, executable, input string, res *EndpointReservation, spec AnalysisSpec, consume func(io.Reader) error, libraries ...string) error {
	endpoint, err := bridgeEndpoint(input)
	if err != nil || sup == nil || consume == nil || key == "" || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || !res.matches(endpoint) || len(libraries) > 256 {
		if res != nil {
			res.Close()
		}
		return ErrInvalidConfiguration
	}
	for _, path := range libraries {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			res.Close()
			return ErrInvalidConfiguration
		}
	}
	args, err := AnalysisArgs(input, spec)
	if err != nil {
		res.Close()
		return err
	}
	cmd, err := decoderJob{executable: executable, args: args, endpoint: endpoint, libraries: libraries, background: true}.command()
	if err != nil {
		res.Close()
		return err
	}
	if spec.PhysicalLock != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, spec.PhysicalLock)
	}
	if !res.claim(endpoint, cmd) {
		return ErrReservationInUse
	}
	retired := make(chan struct{})
	err = sup.RunOwnedCompletion(ctx, "analysis:"+key, cmd, consume, nil, func() { res.retire(); close(retired) })
	<-retired
	return err
}
