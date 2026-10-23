package playback

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

// ObservedPlayback contains playing-device observations, not capabilities,
// requested settings or server encoder targets. Absence means unreported.
// The V1 timeline admits it only with the playing device's fresh generation
// and sequence. A new generation cannot inherit an old output observation.
type ObservedPlayback struct {
	Screen               *ObservedScreen      `json:"screen,omitempty"`
	Audio                *ObservedAudioOutput `json:"audio,omitempty"`
	Subtitles            *ObservedSubtitles   `json:"subtitles,omitempty"`
	DeliveredBitrateKbps int                  `json:"deliveredBitrateKbps,omitempty"`
}
type ObservedScreen struct {
	DynamicRange  string  `json:"dynamicRange,omitempty"`
	RefreshRateHz float64 `json:"refreshRateHz,omitempty"`
}
type ObservedAudioOutput struct {
	Handling    string `json:"handling"`
	Codec       string `json:"codec,omitempty"`
	ObjectAudio string `json:"objectAudio,omitempty"`
	Channels    int    `json:"channels,omitempty"`
	Layout      string `json:"layout,omitempty"`
	SampleRate  int    `json:"sampleRate,omitempty"`
	BitDepth    int    `json:"bitDepth,omitempty"`
}
type ObservedSubtitles struct {
	Rendering string `json:"rendering"`
}

func (o *ObservedPlayback) Valid() bool {
	if o == nil {
		return true
	}
	if o.DeliveredBitrateKbps < 0 || o.DeliveredBitrateKbps > 1000000 {
		return false
	}
	if s := o.Screen; s != nil {
		if s.DynamicRange != "" && !containsString([]string{"sdr", "hdr10", "hdr10plus", "hlg", "dolby_vision"}, s.DynamicRange) {
			return false
		}
		if math.IsNaN(s.RefreshRateHz) || math.IsInf(s.RefreshRateHz, 0) || s.RefreshRateHz < 0 || s.RefreshRateHz > 1000 {
			return false
		}
	}
	if a := o.Audio; a != nil {
		if !containsString([]string{"passthrough", "decoded", "pcm", "mixed"}, a.Handling) {
			return false
		}
		if a.Codec != "" && !containsString([]string{"aac", "ac3", "eac3", "truehd", "dts", "dts_hd", "flac", "alac", "pcm", "opus", "mp3", "mp2", "vorbis", "ac4"}, a.Codec) {
			return false
		}
		if a.ObjectAudio != "" && a.ObjectAudio != "atmos" && a.ObjectAudio != "dtsx" {
			return false
		}
		if a.Channels < 0 || a.Channels > 64 || a.SampleRate < 0 || a.SampleRate > 1536000 {
			return false
		}
		if a.Layout != "" && !containsString([]string{"mono", "stereo", "2.0", "5.1", "5.1(side)", "7.1", "7.1(wide)", "7.1(wide-side)"}, a.Layout) {
			return false
		}
		if a.BitDepth != 0 && a.BitDepth != 16 && a.BitDepth != 20 && a.BitDepth != 24 && a.BitDepth != 32 {
			return false
		}
	}
	if s := o.Subtitles; s != nil && !containsString([]string{"drawnByDevice", "externalText", "burnedIn", "off"}, s.Rendering) {
		return false
	}
	return true
}

// StoreObservedPlaybackTx is called only after the same transaction accepted
// a fresh V1 timeline update. The INSERT also checks device/generation/sequence
// authority, so a racing presentation change cannot attach obsolete facts.
func StoreObservedPlaybackTx(ctx context.Context, tx *sql.Tx, session, device string, generation int, seq, now int64, o *ObservedPlayback) error {
	if o == nil {
		return nil
	}
	if !o.Valid() {
		return errors.New("invalid playback observation")
	}
	screen := ObservedScreen{}
	audio := ObservedAudioOutput{}
	sub := ObservedSubtitles{}
	if o.Screen != nil {
		screen = *o.Screen
	}
	if o.Audio != nil {
		audio = *o.Audio
	}
	if o.Subtitles != nil {
		sub = *o.Subtitles
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO playback_observed_output(session_id,generation,seq,observed_ms,screen_known,dynamic_range,refresh_hz,audio_known,audio_handling,audio_codec,object_audio,channels,channel_layout,sample_rate,bit_depth,subtitles_known,subtitle_rendering,delivered_kbps)
 SELECT id,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,? FROM playback_v1_sessions WHERE id=? AND device_id=? AND generation=? AND last_seq=? AND ended_ms=0
 ON CONFLICT(session_id) DO UPDATE SET generation=excluded.generation,seq=excluded.seq,observed_ms=excluded.observed_ms,screen_known=excluded.screen_known,dynamic_range=excluded.dynamic_range,refresh_hz=excluded.refresh_hz,audio_known=excluded.audio_known,audio_handling=excluded.audio_handling,audio_codec=excluded.audio_codec,object_audio=excluded.object_audio,channels=excluded.channels,channel_layout=excluded.channel_layout,sample_rate=excluded.sample_rate,bit_depth=excluded.bit_depth,subtitles_known=excluded.subtitles_known,subtitle_rendering=excluded.subtitle_rendering,delivered_kbps=excluded.delivered_kbps
 WHERE playback_observed_output.generation<>excluded.generation OR playback_observed_output.seq<excluded.seq`, generation, seq, now, o.Screen != nil, screen.DynamicRange, screen.RefreshRateHz, o.Audio != nil, audio.Handling, audio.Codec, audio.ObjectAudio, audio.Channels, audio.Layout, audio.SampleRate, audio.BitDepth, o.Subtitles != nil, sub.Rendering, o.DeliveredBitrateKbps, session, device, generation, seq)
	return err
}
func ReadObservedPlayback(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, media string, generation ...int) (*ObservedPlayback, error) {
	var o ObservedPlayback
	expected := 0
	if len(generation) == 1 {
		expected = generation[0]
	}
	var screen ObservedScreen
	var audio ObservedAudioOutput
	var sub ObservedSubtitles
	var hasScreen, hasAudio, hasSub bool
	err := q.QueryRowContext(ctx, `SELECT o.screen_known,o.dynamic_range,o.refresh_hz,o.audio_known,o.audio_handling,o.audio_codec,o.object_audio,o.channels,o.channel_layout,o.sample_rate,o.bit_depth,o.subtitles_known,o.subtitle_rendering,o.delivered_kbps FROM playback_observed_output o JOIN playback_v1_sessions s ON s.id=o.session_id AND s.generation=o.generation WHERE s.media_session_id=? AND s.ended_ms=0 AND (?=0 OR s.generation=?)`, media, expected, expected).Scan(&hasScreen, &screen.DynamicRange, &screen.RefreshRateHz, &hasAudio, &audio.Handling, &audio.Codec, &audio.ObjectAudio, &audio.Channels, &audio.Layout, &audio.SampleRate, &audio.BitDepth, &hasSub, &sub.Rendering, &o.DeliveredBitrateKbps)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if hasScreen {
		o.Screen = &screen
	}
	if hasAudio {
		o.Audio = &audio
	}
	if hasSub {
		o.Subtitles = &sub
	}
	if !o.Valid() {
		return nil, errors.New("invalid stored playback observation")
	}
	return &o, nil
}
