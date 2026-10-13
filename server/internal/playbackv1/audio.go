package playbackv1

import (
	"context"
	"time"

	"portico.local/server/internal/audiofacts"
	"portico.local/server/internal/playback"
)

// audioChoice prepares an audio presentation's version 2 plan (spec §18.1):
// the device's audioDecode, and the asset's facts, measured now (bounded) when
// the background backfill hasn't reached it yet. Other kinds are untouched.
func (s *Service) audioChoice(ctx context.Context, c Caller, item, kind string, choice *playback.V1Choice) error {
	if kind != "song" && kind != "track" && kind != "audiobook_file" {
		return nil
	}
	choice.AudioPlanV2 = true
	choice.AudioDecode = s.deviceDocument(ctx, c.DeviceID).DecodeCaps()
	asset, err := s.Playback.SelectedAsset(ctx, item, choice)
	if err != nil {
		return nil // the start itself reports a missing version
	}
	budget := s.AudioMeasureBudget
	if budget <= 0 {
		budget = 5 * time.Second
	}
	_, _, err = audiofacts.Ensure(ctx, s.DB, s.MeasureAudio, item, asset, budget)
	return err
}
