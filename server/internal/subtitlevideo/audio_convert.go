package subtitlevideo

import (
	"context"
	"io"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// ConvertAudio streams an audio asset converted exactly (spec §18.1 converted
// mode): the same fenced input and confined FFmpeg as analysis, with a closed
// recipe (decoder.AnalysisSpec kinds audio_flac and audio_opus).
func (r *Runtime) ConvertAudio(ctx context.Context, item, asset string, spec decoder.AnalysisSpec, out io.Writer) error {
	if spec.Kind != "audio_flac" && spec.Kind != "audio_opus" {
		return subtitles.ErrRendererConfiguration
	}
	input, e := r.OpenSubtitleInput(ctx, item, asset, "")
	if e != nil {
		return e
	}
	defer input.Close()
	return r.AnalysisDecoder(input)(ctx, identity.Token(), spec, func(stream io.Reader) error {
		_, err := io.Copy(out, stream)
		return err
	})
}
