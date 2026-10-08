package decoder

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"portico.local/server/internal/storage"
)

// PreparedRecipe is intentionally not an arbitrary ffmpeg argument surface.
// Profile identity/versioning is owned by preparedmedia, not by a client.
type PreparedRecipe struct {
	AudioOnly, DropChapters, BoundedFragments bool
	Width, Height, VideoKbps, AudioKbps       int
}

func preparedArgs(input string, r PreparedRecipe, validate bool) ([]string, error) {
	args := []string{"-v", "error", "-xerror", "-nostdin", "-threads", "2", "-protocol_whitelist", "http,tcp", "-format_whitelist", "mov,matroska,mp3,flac,ogg,wav,aac,mpegts,avi", "-probesize", "8388608", "-analyzeduration", "10000000", "-i", input}
	if validate {
		return append(args, "-map", "0:v?", "-map", "0:a?", "-sn", "-dn", "-f", "null", "-"), nil
	}
	if r.AudioKbps != 192 {
		return nil, ErrInvalidConfiguration
	}
	if r.AudioOnly {
		args = append(args, "-map", "0:a", "-vn")
	} else {
		if !((r.Width == 1280 && r.Height == 720 && r.VideoKbps == 2500) || (r.Width == 1920 && r.Height == 1080 && r.VideoKbps == 5000)) {
			return nil, ErrInvalidConfiguration
		}
		scale := fmt.Sprintf("scale=w='max(2,trunc(min(min(iw*sar,%d),%d*dar)/2)*2)':h='max(2,trunc(min(min(ih,%d),%d/dar)/2)*2)',setsar=1", r.Width, r.Height, r.Height, r.Width)
		args = append(args, "-map", "0:v:0", "-map", "0:a?", "-vf", scale, "-c:v", "libx264", "-preset", "medium", "-pix_fmt", "yuv420p", "-profile:v", "high", "-level:v", "4.0", "-r", "30", "-crf", "21", "-maxrate", strconv.Itoa(r.VideoKbps)+"k", "-bufsize", strconv.Itoa(r.VideoKbps*2)+"k", "-threads", "2")
	}
	if r.DropChapters {
		// V2 recipes omit unremapped chapter/data tracks explicitly. V1 byte
		// recipes stay immutable for already-admitted work and provenance.
		args = append(args, "-map_chapters", "-1")
	}
	if r.BoundedFragments {
		// A pipe cannot seek back to repair an unbounded MP4 fragment. Flush
		// at one-second intervals, including audio with no video keyframes.
		args = append(args, "-frag_duration", "1000000")
	}
	return append(args, "-sn", "-dn", "-map_metadata", "-1", "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-movflags", "+frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1"), nil
}
func preparedCommand(executable, input string, res *EndpointReservation, recipe PreparedRecipe, validate bool, libraries ...string) (*exec.Cmd, string, error) {
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || len(libraries) > 256 {
		return nil, "", ErrInvalidConfiguration
	}
	for _, p := range libraries {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return nil, "", ErrInvalidConfiguration
		}
	}
	endpoint, e := bridgeEndpoint(input)
	if e != nil {
		return nil, "", e
	}
	if res == nil || !res.matches(endpoint) {
		return nil, "", ErrInvalidConfiguration
	}
	args, e := preparedArgs(input, recipe, validate)
	if e != nil {
		return nil, "", e
	}
	cmd, e := decoderJob{executable: executable, args: args, endpoint: endpoint, libraries: libraries, background: true}.command()
	return cmd, endpoint, e
}

// runPrepared holds the caller's inherited custody descriptor until the actual
// child retires, including cancellation and parent-death recovery. Background
// work does not consume the supervisor's reserved interactive playback slot.
func runPrepared(ctx context.Context, s *storage.Supervisor, key string, cmd *exec.Cmd, endpoint string, res *EndpointReservation, custody *os.File, consume func(io.Reader) error) error {
	if s == nil || key == "" || custody == nil || cmd == nil {
		return ErrInvalidConfiguration
	}
	cmd.ExtraFiles = append(cmd.ExtraFiles, custody)
	if !res.claim(endpoint, cmd) {
		return ErrReservationInUse
	}
	retired := make(chan struct{})
	e := s.RunOwnedCompletion(ctx, "optimization:"+key, cmd, consume, nil, func() { res.retire(); close(retired) })
	<-retired
	return e
}
func RunPreparedProbe(ctx context.Context, s *storage.Supervisor, key, executable, input string, res *EndpointReservation, custody *os.File, libraries ...string) ([]byte, error) {
	cmd, e := ProbeCommand(executable, input, res, libraries...)
	if e != nil {
		return nil, e
	}
	endpoint, e := bridgeEndpoint(input)
	if e != nil {
		return nil, e
	}
	var b []byte
	e = runPrepared(ctx, s, key, cmd, endpoint, res, custody, func(r io.Reader) error {
		var err error
		b, err = io.ReadAll(io.LimitReader(r, MaxProbeOutputBytes+1))
		if err == nil && len(b) > MaxProbeOutputBytes {
			err = ErrProbeOutput
		}
		return err
	})
	return b, e
}
func RunPreparedEncode(ctx context.Context, s *storage.Supervisor, key, executable, input string, res *EndpointReservation, custody *os.File, recipe PreparedRecipe, out io.Writer, libraries ...string) error {
	cmd, endpoint, e := preparedCommand(executable, input, res, recipe, false, libraries...)
	if e != nil {
		return e
	}
	return runPrepared(ctx, s, key, cmd, endpoint, res, custody, func(r io.Reader) error { _, e := io.Copy(out, r); return e })
}
func RunPreparedValidate(ctx context.Context, s *storage.Supervisor, key, executable, input string, res *EndpointReservation, custody *os.File, libraries ...string) error {
	cmd, endpoint, e := preparedCommand(executable, input, res, PreparedRecipe{}, true, libraries...)
	if e != nil {
		return e
	}
	return runPrepared(ctx, s, key, cmd, endpoint, res, custody, func(r io.Reader) error { _, e := io.Copy(io.Discard, r); return e })
}
