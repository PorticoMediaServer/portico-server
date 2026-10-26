package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/decoder"
)

// StoreConversionTarget is called after persisting the plan and generation in
// the planner's transaction, before publishing its presentation. The selected target is immutable for a session
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
	var generation int
	if err := tx.QueryRowContext(ctx, `SELECT generation FROM playback_sessions WHERE id=?`, session).Scan(&generation); err != nil {
		return err
	}
	backend, device, native := "software", "", false
	probeJSON := ""
	if len(hardware) == 1 {
		encoded, err := json.Marshal(hardware[0])
		if err != nil || len(encoded) > 8192 {
			return decoder.ErrInvalidConfiguration
		}
		probeJSON = string(encoded)
		if hardware[0].Available && decoder.ValidBackend(string(hardware[0].Backend)) {
			backend, device, native = string(hardware[0].Backend), hardware[0].Device, hardware[0].NativeFilters || !target.ToneMap && hardware[0].ScaleFilters
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO playback_conversion_targets(session_id,generation,target_json,hardware_json) VALUES(?,?,?,?) ON CONFLICT(session_id) DO UPDATE SET generation=excluded.generation,target_json=excluded.target_json,hardware_json=excluded.hardware_json`, session, generation, string(raw), probeJSON)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE playback_delivery_plans SET output_container=?,output_video=CASE WHEN video_action='none' THEN '' ELSE ? END,output_audio=CASE WHEN audio_action='none' THEN '' ELSE ? END,audio_channels=?,target_width=?,target_height=?,max_frame_rate=?,video_bitrate=?,audio_bitrate=?,tone_map=?,hardware_backend=?,hardware_device=?,hardware_native=? WHERE session_id=?`, target.Packaging, target.VideoCodec, target.AudioCodec, target.AudioChannels, target.Width, target.Height, target.FrameRate, target.VideoBitrateBPS, target.AudioBitrateBPS, target.ToneMap, backend, device, native, session)
	return err
}

func loadConversionTarget(ctx context.Context, q planRowQuery, session string, p *DeliveryPlan) error {
	var raw, probeJSON string
	err := q.QueryRowContext(ctx, `SELECT t.target_json,t.hardware_json FROM playback_conversion_targets t JOIN playback_sessions ps ON ps.id=t.session_id WHERE t.session_id=? AND t.generation=ps.generation`, session).Scan(&raw, &probeJSON)
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
	p.OutputContainer = t.Packaging
	if p.VideoAction != "none" {
		p.OutputVideoCodec = t.VideoCodec
	}
	if p.AudioAction != "none" {
		p.OutputAudioCodec = t.AudioCodec
	}
	p.TargetWidth, p.TargetHeight, p.MaxFrameRate = t.Width, t.Height, t.FrameRate
	p.VideoBitrateBPS, p.AudioBitrateBPS, p.AudioChannels, p.AudioSampleRate = t.VideoBitrateBPS, t.AudioBitrateBPS, t.AudioChannels, t.AudioSampleRate
	p.ToneMap = t.ToneMap
	return nil
}
