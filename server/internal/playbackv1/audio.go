package playbackv1

import (
	"context"
	"errors"
	"time"

	"portico.local/server/internal/audiofacts"
	"portico.local/server/internal/playback"
)

// audioChoice prepares an audio presentation's version 2 plan (spec §18.1):
// the device's audioDecode, and the asset's facts, measured now (bounded) when
// the background backfill hasn't reached it yet. Other kinds are untouched.
func (s *Service) audioChoice(ctx context.Context, c Caller, item string, choice *playback.V1Choice) error {
	var kind string
	var err error
	if kind, err = readItemKind(ctx, s.DB, item); err != nil {
		return err
	}
	if kind != "song" && kind != "track" && kind != "audiobook_file" {
		return nil
	}
	choice.AudioPlanV2 = true
	if doc, err := s.Capabilities(ctx, c.DeviceID); err == nil {
		choice.AudioDecode = doc.Capabilities.AudioDecodeCaps()
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	if len(choice.AudioDecode) == 0 {
		// A client that sends its decode list with its playback profile (web, until it
		// publishes v1 capabilities).
		choice.AudioDecode = s.Playback.ClientProfileFor(ctx, c.Principal).DecodeCaps()
	}
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
