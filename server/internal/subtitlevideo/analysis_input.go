package subtitlevideo

import (
	"context"
	"io"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/subtitles"
)

// AnalysisDecoder exposes the already-retained, root/configuration-fenced input
// bridge to P11A without granting analysis any subtitle or playback authority.
// A fresh runner is created for each analysis stage; its ledger survives every
// chapter-image subprocess. The caller retains input until all calls return.
func (r *Runtime) AnalysisDecoder(input subtitles.RenderInput) func(context.Context, string, decoder.AnalysisSpec, func(io.Reader) error) error {
	ledger := &extentLedger{}
	return func(ctx context.Context, key string, spec decoder.AnalysisSpec, consume func(io.Reader) error) error {
		b, err := openBridge(ctx, input, ledger)
		if err != nil {
			return err
		}
		defer b.Close()
		if err = decoder.RunAnalysis(ctx, r.storage.Supervisor, key, r.ffmpeg, b.url, b.reservation, spec, consume, r.libraries...); err != nil {
			return err
		}
		return input.Validate(ctx)
	}
}
