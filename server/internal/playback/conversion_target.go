package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/decoder"
)

// StoreConversionTarget is called in the planner's session transaction, before
// publishing its presentation. The selected target is immutable for a session
// generation; changing quality creates/replans the presentation as usual.
func StoreConversionTarget(ctx context.Context, tx *sql.Tx, session string, target decoder.ConversionTarget, hardware ...decoder.HardwareProbe) error {
	target, err := target.Normalized()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(target)
	if err != nil {
		return err
	}
	if len(hardware) > 1 {
		return decoder.ErrInvalidConfiguration
	}
	probeJSON := ""
	if len(hardware) == 1 {
		encoded, err := json.Marshal(hardware[0])
		if err != nil || len(encoded) > 8192 {
			return decoder.ErrInvalidConfiguration
		}
		probeJSON = string(encoded)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO playback_conversion_targets(session_id,target_json,hardware_json) VALUES(?,?,?) ON CONFLICT(session_id) DO UPDATE SET target_json=excluded.target_json,hardware_json=excluded.hardware_json`, session, string(raw), probeJSON)
	return err
}

func loadConversionTarget(q audioQuery, session string, p *DeliveryPlan) error {
	var raw, probeJSON string
	err := q.QueryRow(`SELECT target_json,hardware_json FROM playback_conversion_targets WHERE session_id=?`, session).Scan(&raw, &probeJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var t decoder.ConversionTarget
	if len(raw) > 8192 || json.Unmarshal([]byte(raw), &t) != nil {
		return decoder.ErrInvalidConfiguration
	}
	t, err = t.Normalized()
	if err != nil {
		return err
	}
	p.ConversionTarget = &t
	if probeJSON != "" {
		var probe decoder.HardwareProbe
		if len(probeJSON) > 8192 || json.Unmarshal([]byte(probeJSON), &probe) != nil {
			return decoder.ErrInvalidConfiguration
		}
		p.ConversionProbe = &probe
	}
	p.OutputVideoCodec, p.OutputAudioCodec, p.OutputContainer = t.VideoCodec, t.AudioCodec, t.Packaging
	p.TargetWidth, p.TargetHeight, p.MaxFrameRate = t.Width, t.Height, t.FrameRate
	p.VideoBitrateBPS, p.AudioBitrateBPS, p.AudioChannels, p.AudioSampleRate = t.VideoBitrateBPS, t.AudioBitrateBPS, t.AudioChannels, t.AudioSampleRate
	p.ToneMap = t.ToneMap
	return nil
}
