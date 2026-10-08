package subtitlevideo

import (
	"context"
	"errors"
	"io"

	"portico.local/server/internal/audiofacts"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// MeasureAudio measures one audio asset for client-side decoding (spec §18.7):
// the same root/configuration-fenced input and confined ffprobe as every other
// media read, with one packet-and-frame pass. The input is revalidated after the
// pass, so a file that changed underneath is not measured.
func (r *Runtime) MeasureAudio(ctx context.Context, item, asset string) (audiofacts.Facts, error) {
	f, e := r.measureAudio(ctx, item, asset)
	// The shared input reports a file that no longer matches its scanned
	// version as a subtitle revision conflict; for audio facts it's a changed
	// source (NEW-34), not an unreadable file.
	if errors.Is(e, subtitles.ErrConflict) {
		e = audiofacts.ErrSourceChanged
	}
	return f, e
}

func (r *Runtime) measureAudio(ctx context.Context, item, asset string) (audiofacts.Facts, error) {
	var f audiofacts.Facts
	if r.ffprobe == "" {
		return f, subtitles.ErrRendererConfiguration
	}
	input, e := r.OpenSubtitleInput(ctx, item, asset, "")
	if e != nil {
		return f, e
	}
	defer input.Close()
	b, e := openBridge(ctx, input, &extentLedger{})
	if e != nil {
		return f, e
	}
	defer b.Close()
	var parsed error
	e = decoder.RunStreamingProbe(ctx, r.storage.Supervisor, identity.Token(), r.ffprobe, b.url, audiofacts.Args(b.url), b.reservation, func(out io.Reader) error {
		f, parsed = audiofacts.Parse(out)
		_, _ = io.Copy(io.Discard, out)
		return nil
	}, r.libraries...)
	if e != nil {
		return f, e
	}
	if parsed != nil {
		return f, parsed
	}
	return f, input.Validate(ctx)
}
