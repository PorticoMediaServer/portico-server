package subtitlevideo

import (
	"context"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

type playbackWindow struct {
	runtime *Runtime
	input   subtitles.RenderInput
	bridge  *MediaBridge
	session string
}

func (r *Runtime) OpenPlaybackWindow(ctx context.Context, session string) (subtitles.WindowInput, error) {
	var item, source string
	if e := r.db.QueryRowContext(ctx, `SELECT COALESCE(pid(e.public_id),''),ps.asset_id FROM playback_sessions ps LEFT JOIN catalog_entities e ON e.id=ps.item_id WHERE ps.id=?`, session).Scan(&item, &source); e != nil {
		return nil, e
	}
	input, e := r.OpenSubtitleInput(ctx, item, source, session)
	if e != nil {
		return nil, e
	}
	var expected string
	if e = r.db.QueryRowContext(ctx, `SELECT evidence FROM subtitle_remote_sessions WHERE session_id=?`, session).Scan(&expected); e != nil || expected != persistentEvidence(input) {
		input.Close()
		return nil, subtitles.ErrConflict
	}
	return &playbackWindow{r, input, NewMediaBridge(input), session}, nil
}
func (w *playbackWindow) Close() error { return w.input.Close() }
func (w *playbackWindow) Run(ctx context.Context, dir string, build func(string) ([]string, error)) error {
	return w.bridge.With(ctx, func(url string, res *decoder.EndpointReservation) error {
		if e := w.input.Validate(ctx); e != nil {
			return e
		}
		args, e := build(url)
		if e != nil {
			return e
		}
		e = decoder.RunHLSWindow(ctx, w.runtime.storage.Supervisor, identity.Token(), w.runtime.ffmpeg, url, dir, res, args, w.runtime.libraries...)
		if e == nil {
			e = w.input.Validate(ctx)
		}
		return e
	})
}
