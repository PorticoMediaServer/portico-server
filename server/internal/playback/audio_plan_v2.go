package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"portico.local/server/internal/audiofacts"
	"portico.local/server/internal/identity"
)

// AudioDecodeCap is one entry of a device's audioDecode (spec §3): what the
// client's own audio engine decodes.
type AudioDecodeCap struct {
	Codec         string
	Containers    []string
	MaxSampleRate int
	SampleRates   []int
	MaxChannels   int
	MaxBitDepth   int
}

// AudioPlan is the version 2 audio render plan (spec §18.1): the client decodes
// the original file (direct) or a FLAC/Opus conversion (converted) with the
// server's exact trim and gains. Pinned to one presentation.
type AudioPlan struct {
	Version        int              `json:"version"`
	Mode           string           `json:"mode"`
	ID             string           `json:"id"`
	Reason         string           `json:"reason,omitempty"`
	URL            string           `json:"url,omitempty"`
	Container      string           `json:"container,omitempty"`
	Codec          string           `json:"codec,omitempty"`
	DecoderConfig  string           `json:"decoderConfig,omitempty"`
	SampleRate     int              `json:"sampleRate,omitempty"`
	Channels       int              `json:"channels,omitempty"`
	BitDepth       int              `json:"bitDepth,omitempty"`
	Bytes          int64            `json:"bytes,omitempty"`
	PrefetchBytes  int64            `json:"prefetchBytes,omitempty"`
	DurationFrames int64            `json:"durationFrames,omitempty"`
	Downmixed      bool             `json:"downmixed,omitempty"`
	Trim           *AudioTrim       `json:"trim,omitempty"`
	Gain           *AudioGain       `json:"gain,omitempty"`
	Generation     int              `json:"generation"`
	Convert        *AudioConversion `json:"convert,omitempty"`
}

type AudioTrim struct {
	StartFrames int64  `json:"startFrames"`
	EndFrames   int64  `json:"endFrames"`
	Source      string `json:"source"`
}

// AudioGain is in dB against the ReplayGain 2.0 reference (−18 LUFS); nil is unknown.
type AudioGain struct {
	TrackDB   *float64 `json:"trackDb,omitempty"`
	AlbumDB   *float64 `json:"albumDb,omitempty"`
	TrackPeak *float64 `json:"trackPeak,omitempty"`
	AlbumPeak *float64 `json:"albumPeak,omitempty"`
	Source    string   `json:"source"`
}

// AudioConversion is how the converted route produces the plan's stream (never
// on the wire): the source's trimmed frames, resampled and encoded exactly.
type AudioConversion struct {
	Codec        string `json:"codec"`
	SourceRate   int    `json:"sourceRate"`
	SourceStart  int64  `json:"sourceStart"`
	SourceFrames int64  `json:"sourceFrames"`
	Rate         int    `json:"rate"`
	Channels     int    `json:"channels"`
	BitDepth     int    `json:"bitDepth"`
	BitrateBPS   int    `json:"bitrateBps"`
}

// OpusPreSkip is libopus's pre-skip at 48 kHz; Opus frames are 20 ms (960).
const OpusPreSkip, opusFrame = 312, 960

var errAudioPlan = errors.New("audio plan unavailable")

func capAllowsRate(c AudioDecodeCap, rate int) bool {
	if len(c.SampleRates) > 0 {
		return slices.Contains(c.SampleRates, rate)
	}
	return c.MaxSampleRate <= 0 || rate <= c.MaxSampleRate
}

// codecMatches maps FFmpeg's codec name to the capability vocabulary.
func codecMatches(declared, codec string) bool {
	declared = strings.ToLower(declared)
	return declared == codec || declared == "pcm" && strings.HasPrefix(codec, "pcm_")
}

func (c AudioDecodeCap) decodes(f audiofacts.Facts) bool {
	return codecMatches(c.Codec, f.Codec) && slices.Contains(c.Containers, f.Container) && capAllowsRate(c, f.SampleRate) &&
		(c.MaxChannels <= 0 || f.Channels <= c.MaxChannels) && (c.MaxBitDepth <= 0 || f.BitDepth <= c.MaxBitDepth)
}

func findCap(caps []AudioDecodeCap, codec, container string) (AudioDecodeCap, bool) {
	for _, c := range caps {
		if codecMatches(c.Codec, codec) && slices.Contains(c.Containers, container) {
			return c, true
		}
	}
	return AudioDecodeCap{}, false
}

var lossless = map[string]bool{"flac": true, "alac": true, "wavpack": true, "ape": true, "tta": true, "mlp": true, "truehd": true}

func isLossless(codec string) bool { return lossless[codec] || strings.HasPrefix(codec, "pcm_") }

// convertedRate is the source rate when the FLAC capability allows it, else the
// highest standard rate it allows at or under the source, else its lowest.
func convertedRate(c AudioDecodeCap, source int) int {
	if capAllowsRate(c, source) {
		return source
	}
	standard := []int{384000, 352800, 192000, 176400, 96000, 88200, 48000, 44100, 32000, 22050}
	for _, r := range standard {
		if r <= source && capAllowsRate(c, r) {
			return r
		}
	}
	for i := len(standard) - 1; i >= 0; i-- {
		if capAllowsRate(c, standard[i]) {
			return standard[i]
		}
	}
	return 0
}

// pinAudioPlan builds and pins the version 2 plan for a v1 audio presentation.
// Facts were ensured before the transaction (audiofacts.Ensure); without them
// the plan is unavailable, never guessed.
func (s *Service) pinAudioPlan(ctx context.Context, tx *sql.Tx, session Session, item, asset, grant string, v1 *V1Choice, ownerTranscoding bool) (*AudioPlan, error) {
	p := &AudioPlan{Version: 2, Mode: "unavailable", ID: session.ID + "." + itoa(session.Generation), Generation: session.Generation}
	f, ok, err := audiofacts.Read(ctx, tx, asset)
	if err != nil {
		return nil, err
	}
	var size int64
	if err = tx.QueryRowContext(ctx, `SELECT size FROM catalog_assets WHERE token=?`, asset).Scan(&size); err != nil {
		return nil, err
	}
	if !ok {
		p.Reason = "Portico hasn't measured this track yet, so it plays without gapless or crossfade."
		return p, s.storeAudioPlan(tx, session.ID, p)
	}
	gain, err := audioGains(ctx, tx, item, asset)
	if err != nil {
		return nil, err
	}
	seconds := float64(f.DurationFrames) / float64(f.SampleRate)
	sourceBPS := 0
	if seconds > 0 {
		sourceBPS = int(float64(size) * 8 / seconds)
	}
	limited := v1.Limit && v1.MaxAudioBitrateBPS > 0 && sourceBPS > v1.MaxAudioBitrateBPS
	direct := false
	if !limited {
		for _, c := range v1.AudioDecode {
			if c.decodes(f) {
				direct = true
				break
			}
		}
	}
	if direct {
		p.Mode, p.URL = "direct", "/v1/media/"+grant+"/audio"
		p.Container, p.Codec, p.DecoderConfig = f.Container, f.Codec, f.DecoderConfig
		p.SampleRate, p.Channels, p.BitDepth = f.SampleRate, f.Channels, f.BitDepth
		p.Bytes, p.DurationFrames = size, f.DurationFrames
		p.Trim = &AudioTrim{StartFrames: f.StartFrames, EndFrames: f.EndFrames, Source: f.TrimSource}
		p.Gain = gain
		// The start plus a trailing index (an MP4 moov at the end): 30 s at the
		// file's own rate, never under 4 MiB, never over 16 MiB or the file.
		p.PrefetchBytes = min(size, max(4<<20, min(16<<20, int64(sourceBPS/8)*30+(1<<20))))
		return p, s.storeAudioPlan(tx, session.ID, p)
	}
	if !ownerTranscoding {
		p.Reason = "This device can't decode this track's format, and the server owner doesn't allow converting audio. It plays without gapless or crossfade."
		return p, s.storeAudioPlan(tx, session.ID, p)
	}
	flacCap, canFLAC := findCap(v1.AudioDecode, "flac", "flac")
	opusCap, canOpus := findCap(v1.AudioDecode, "opus", "ogg")
	useOpus := canOpus && capAllowsRate(opusCap, 48000) && (limited || !canFLAC)
	conv := &AudioConversion{SourceRate: f.SampleRate, SourceStart: f.StartFrames, SourceFrames: f.DurationFrames}
	switch {
	case useOpus:
		conv.Codec, conv.Rate, conv.Channels = "opus", 48000, min(f.Channels, 2)
		if opusCap.MaxChannels > 0 {
			conv.Channels = min(conv.Channels, opusCap.MaxChannels)
		}
		conv.BitrateBPS = 128000 * conv.Channels / 2
		if v1.Limit && v1.MaxAudioBitrateBPS > 0 {
			conv.BitrateBPS = max(24000, min(conv.BitrateBPS, v1.MaxAudioBitrateBPS))
		}
		p.Container, p.Codec = "ogg", "opus"
		p.Reason = "Portico converts this track to Opus for this device."
	case canFLAC && !limited:
		conv.Codec, conv.Rate, conv.Channels = "flac", convertedRate(flacCap, f.SampleRate), f.Channels
		if flacCap.MaxChannels > 0 {
			conv.Channels = min(conv.Channels, flacCap.MaxChannels)
		}
		conv.BitDepth = 16
		if isLossless(f.Codec) && f.BitDepth > 16 {
			conv.BitDepth = 24
		}
		if flacCap.MaxBitDepth > 0 && flacCap.MaxBitDepth < conv.BitDepth {
			conv.BitDepth = 16
		}
		if conv.Rate == 0 {
			conv = nil
		}
		p.Container, p.Codec = "flac", "flac"
		p.Reason = "This device can't decode " + strings.ToUpper(f.Codec) + ", so Portico converts it to FLAC."
	default:
		conv = nil
	}
	if conv == nil {
		p.Container, p.Codec = "", ""
		p.Reason = "This device can't decode this track's format or the converted formats (FLAC, Opus). It plays without gapless or crossfade."
		return p, s.storeAudioPlan(tx, session.ID, p)
	}
	frames := int64(math.Round(float64(f.DurationFrames) * float64(conv.Rate) / float64(f.SampleRate)))
	p.Mode, p.URL, p.Convert = "converted", "/v1/media/"+grant+"/audio-converted", conv
	p.SampleRate, p.Channels, p.BitDepth, p.DurationFrames = conv.Rate, conv.Channels, conv.BitDepth, frames
	p.Downmixed = conv.Channels < f.Channels
	p.Trim = &AudioTrim{Source: "flac"}
	if conv.Codec == "opus" {
		p.BitDepth = 0
		total := (OpusPreSkip + frames + opusFrame - 1) / opusFrame * opusFrame
		p.Trim = &AudioTrim{StartFrames: OpusPreSkip, EndFrames: total - OpusPreSkip - frames, Source: "opus-preskip"}
	}
	p.Gain = gain
	p.PrefetchBytes = 4 << 20
	if conv.Codec == "flac" {
		// About 30 s of FLAC at this rate, depth and channel count (~60% of PCM).
		p.PrefetchBytes = max(4<<20, int64(conv.Rate)*int64(conv.Channels)*int64(conv.BitDepth/8)*30*6/10)
	}
	return p, s.storeAudioPlan(tx, session.ID, p)
}

func itoa(n int) string { return strconv.Itoa(n) }

func (s *Service) storeAudioPlan(tx *sql.Tx, session string, p *AudioPlan) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO playback_audio_plans(session_id,plan_json) VALUES(?,?) ON CONFLICT(session_id) DO UPDATE SET plan_json=excluded.plan_json`, session, string(raw))
	return err
}

// readAudioPlan is a presentation's pinned plan for its current generation.
func readAudioPlan(tx interface {
	QueryRow(string, ...any) *sql.Row
}, id string, generation int) (*AudioPlan, error) {
	var raw string
	err := tx.QueryRow(`SELECT plan_json FROM playback_audio_plans WHERE session_id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p AudioPlan
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	if p.Generation != generation {
		return nil, nil
	}
	return &p, nil
}

// AudioPlanByGrant is the plan and media session behind a grant, for the audio routes.
func (s *Service) AudioPlanByGrant(ctx context.Context, grant string) (*AudioPlan, string, error) {
	var id string
	var generation int
	if err := s.db.QueryRowContext(ctx, `SELECT id,generation FROM playback_sessions WHERE grant_hash=?`, identity.Digest(grant)).Scan(&id, &generation); err != nil {
		return nil, "", err
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT plan_json FROM playback_audio_plans WHERE session_id=?`, id).Scan(&raw); err != nil {
		return nil, id, err
	}
	var p AudioPlan
	if err := json.Unmarshal([]byte(raw), &p); err != nil || p.Generation != generation {
		return nil, id, errAudioPlan
	}
	return &p, id, nil
}

// audioGains: tags first (ReplayGain, or R128 converted to the −18 LUFS
// reference), then the server's loudness analysis; album values from tags, or
// from analysis once every track of the album (or book) is measured.
func audioGains(ctx context.Context, tx *sql.Tx, item, asset string) (*AudioGain, error) {
	// audio_tag_evidence is a fact table written synchronously; no readiness
	// wait.
	tags := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT field,value FROM audio_tag_evidence WHERE asset_id=? AND library_id=(SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?))`, asset, item)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k, v string
		if err = rows.Scan(&k, &v); err != nil {
			rows.Close()
			return nil, err
		}
		tags[k] = v
	}
	rows.Close()
	g := &AudioGain{Source: "tags"}
	g.TrackDB, g.AlbumDB = replayGain(tags, "track"), replayGain(tags, "album")
	g.TrackPeak = audioTagNumber(tags["replaygain_track_peak"], 0.000001, 64)
	g.AlbumPeak = audioTagNumber(tags["replaygain_album_peak"], 0.000001, 64)
	if g.TrackDB != nil {
		return g, nil
	}
	lufs, peak, ok, err := assetLoudness(ctx, tx, asset)
	if err != nil || !ok {
		if err == nil && g.AlbumDB == nil {
			return nil, nil
		}
		return g, err
	}
	v := -18 - lufs
	g.TrackDB, g.Source = &v, "analysis"
	if peak > 0 {
		g.TrackPeak = &peak
	}
	if g.AlbumDB == nil {
		if a, ap, ok, err := albumLoudness(ctx, tx, item); err != nil {
			return nil, err
		} else if ok {
			av := -18 - a
			g.AlbumDB = &av
			if ap > 0 {
				g.AlbumPeak = &ap
			}
		}
	}
	return g, nil
}

// replayGain reads ReplayGain, then R128 (Q7.8 dB against −23 LUFS, so +5 dB to
// the ReplayGain reference).
func replayGain(tags map[string]string, kind string) *float64 {
	if v := audioTagNumber(tags["replaygain_"+kind+"_gain"], -60, 30); v != nil {
		return v
	}
	if v := audioTagNumber(tags["r128_"+kind+"_gain"], -32768, 32767); v != nil && *v == math.Trunc(*v) {
		g := *v/256 + 5
		if g >= -60 && g <= 30 {
			return &g
		}
	}
	return nil
}

func audioTagNumber(raw string, min, max float64) *float64 {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "db"))
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < min || v > max {
		return nil
	}
	return &v
}

func assetLoudness(ctx context.Context, tx *sql.Tx, asset string) (lufs, peak float64, ok bool, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT summary_json FROM analysis_results WHERE asset_id=? AND stage='loudness' AND retired_ms=0 ORDER BY created_ms DESC LIMIT 1`, asset).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	var s struct {
		LUFS *float64 `json:"integratedLUFS"`
		Peak *float64 `json:"truePeakLinear"`
	}
	if json.Unmarshal([]byte(raw), &s) != nil || s.LUFS == nil || math.IsNaN(*s.LUFS) || *s.LUFS < -70 || *s.LUFS > 10 {
		return 0, 0, false, nil
	}
	if s.Peak != nil && *s.Peak > 0 && *s.Peak < 64 {
		peak = *s.Peak
	}
	return *s.LUFS, peak, true, nil
}

// albumLoudness is the duration-weighted energy mean of the album's (or book's)
// tracks, once every one is measured; bounded to 2000 tracks.
func albumLoudness(ctx context.Context, tx *sql.Tx, item string) (float64, float64, bool, error) {
	// Siblings, links, assets and loudness facts are all synchronous facts;
	// the old projection wait is gone.
	rows, err := tx.QueryContext(ctx, `WITH song_siblings(item_id) AS (
   SELECT s2.entity_id FROM catalog_entities source JOIN catalog_songs s1 ON s1.entity_id=source.id JOIN catalog_songs s2 ON s2.album_id=s1.album_id WHERE source.public_id=pid_blob(?) ORDER BY s2.entity_id LIMIT 2001),
 book_siblings(item_id) AS (
   SELECT b2.entity_id FROM catalog_entities source JOIN catalog_book_files b1 ON b1.entity_id=source.id JOIN catalog_book_files b2 ON b2.book_id=b1.book_id WHERE source.public_id=pid_blob(?) ORDER BY b2.entity_id LIMIT 2001),
 siblings(item_id) AS (SELECT item_id FROM song_siblings UNION SELECT item_id FROM book_siblings)
 SELECT MIN(a.token),MAX(a.duration) FROM siblings x JOIN catalog_asset_links ia ON ia.entity_id=x.item_id JOIN catalog_assets a ON a.id=ia.asset_id GROUP BY x.item_id LIMIT 2001`, item, item)
	if err != nil {
		return 0, 0, false, err
	}
	type track struct {
		asset    string
		duration float64
	}
	var tracks []track
	for rows.Next() {
		var t track
		if err = rows.Scan(&t.asset, &t.duration); err != nil {
			rows.Close()
			return 0, 0, false, err
		}
		tracks = append(tracks, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, false, err
	}
	if len(tracks) == 0 || len(tracks) > 2000 {
		return 0, 0, false, nil
	}
	var energy, total, peak float64
	for _, t := range tracks {
		l, p, ok, err := assetLoudness(ctx, tx, t.asset)
		if err != nil || !ok || t.duration <= 0 {
			return 0, 0, false, err
		}
		energy += t.duration * math.Pow(10, l/10)
		total += t.duration
		peak = max(peak, p)
	}
	return 10 * math.Log10(energy/total), peak, true, nil
}

// ResolveDecodeGrant resolves a grant for the version 2 audio routes: like ResolveGrant,
// and a prepared (private) presentation's grant too (spec §18.2), whose bytes
// PrefetchCharge bounds.
func (s *Service) ResolveDecodeGrant(grant string) (string, identity.Principal, string, error) {
	return s.ResolveDecodeGrantContext(context.Background(), grant)
}

// ResolveDecodeGrantContext applies the caller's cancellation to initial audio
// grant checks, including a prepared presentation's private grant.
func (s *Service) ResolveDecodeGrantContext(ctx context.Context, grant string) (string, identity.Principal, string, error) {
	return s.resolveGrant(ctx, grant, true)
}

// PrivatePresentation reports whether a media session is a prepared, not yet
// committed next track (spec §18.2).
func (s *Service) PrivatePresentation(ctx context.Context, session string) (bool, error) {
	var private bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_private_presentations WHERE session_id=?)`, session).Scan(&private)
	return private, err
}

// PrefetchCharge counts bytes served to a private presentation against its
// plan's prefetchBytes; false when n more would exceed it (403 prepared_limit).
// A caller that charges before serving gives back what it didn't serve with
// PrefetchRefund, so a failed request never spends the budget (NEW-33).
func (s *Service) PrefetchCharge(session string, n, limit int64) bool {
	s.prefetchMu.Lock()
	defer s.prefetchMu.Unlock()
	if s.prefetched == nil {
		s.prefetched = map[string]int64{}
	}
	if len(s.prefetched) > 4096 {
		clear(s.prefetched) // bounded: a stale entry only re-grants a few MiB
	}
	if s.prefetched[session]+n > limit {
		return false
	}
	s.prefetched[session] += n
	return true
}

// PrefetchRefund returns n charged bytes that weren't served.
func (s *Service) PrefetchRefund(session string, n int64) {
	if n <= 0 {
		return
	}
	s.prefetchMu.Lock()
	defer s.prefetchMu.Unlock()
	if s.prefetched[session] <= n {
		delete(s.prefetched, session)
		return
	}
	s.prefetched[session] -= n
}
