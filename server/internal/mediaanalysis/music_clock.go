package mediaanalysis

import (
	"context"
	"database/sql"
	"math"

	"portico.local/server/internal/assets"
)

// Basic Musepack uses the validated header's integer count/rate, unlike an
// ffprobe six-decimal duration string. Preserve that sample clock and project
// it to the analysis store's microseconds; playback keeps its exact frames.
func musepackDurationUS(ctx context.Context, tx *sql.Tx, asset, codec string, seconds float64) (int64, error) {
	var observed, detail string
	if err := tx.QueryRowContext(ctx, `SELECT st.codec,st.detail_json FROM asset_streams st JOIN catalog_assets a ON a.id=st.asset_id WHERE a.token=? AND st.type='audio' ORDER BY st.stream_index LIMIT 1`, asset).Scan(&observed, &detail); err != nil {
		return 0, err
	}
	if observed != codec {
		return 0, ErrConflict
	}
	var stream assets.Stream
	stream.DecodeDetail(detail)
	rate := stream.Detail.SampleRate
	if rate != 32000 && rate != 37800 && rate != 44100 && rate != 48000 {
		return 0, ErrInput
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > float64((1<<53)-1)/1e6 {
		return 0, ErrInput
	}
	frames := seconds * float64(rate)
	if math.Abs(frames-math.Round(frames)) > .001 {
		return 0, ErrInput
	}
	return int64(math.Round(math.Round(frames) * 1e6 / float64(rate))), nil
}
