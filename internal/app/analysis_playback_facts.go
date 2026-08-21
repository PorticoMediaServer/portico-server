package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/streamfactsadapter"
)

type analysisFileIdentity struct {
	ID, Fingerprint, ModTime string
	SizeBytes                int64
}

func canonicalAnalysisFileIdentity(id, fingerprint string, size int64, modTime string) analysisFileIdentity {
	id, fingerprint, modTime = strings.TrimSpace(id), strings.TrimSpace(fingerprint), strings.TrimSpace(modTime)
	if fingerprint == "" {
		fingerprint = analysisIdentityHash("fingerprint", id, strconv.FormatInt(size, 10), modTime)
	}
	return analysisFileIdentity{ID: id, Fingerprint: fingerprint, SizeBytes: size, ModTime: modTime}
}

func (f analysisFileIdentity) revision() string {
	return analysisIdentityHash("revision", f.ID, f.Fingerprint, strconv.FormatInt(f.SizeBytes, 10), f.ModTime)
}

func analysisIdentityHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func persistPlaybackFacts(tx *sql.Tx, mediaID string, file analysisFileIdentity, payload ffprobePayload, analyzedAt string) error {
	facts, err := playbackFactsFromFFprobe(file, payload)
	if err != nil {
		return fmt.Errorf("canonical stream facts: %w", err)
	}
	if err := applyPersistedKeyframeEvidence(tx, mediaID, file, &facts); err != nil {
		return fmt.Errorf("canonical stream facts: %w", err)
	}
	body, err := facts.StableJSON()
	if err != nil {
		return fmt.Errorf("canonical stream facts: %w", err)
	}
	digest, err := facts.Digest()
	if err != nil {
		return fmt.Errorf("canonical stream facts: %w", err)
	}
	_, err = tx.Exec(`INSERT INTO media_analysis_facts
		(media_id, media_file_id, schema_version, source_revision, source_fingerprint, facts_digest, facts_json, analyzed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, media_file_id) DO UPDATE SET
			schema_version=excluded.schema_version, source_revision=excluded.source_revision,
			source_fingerprint=excluded.source_fingerprint, facts_digest=excluded.facts_digest,
			facts_json=excluded.facts_json, analyzed_at=excluded.analyzed_at`,
		mediaID, file.ID, mediafacts.SchemaVersion, file.revision(), file.Fingerprint, digest, string(body), analyzedAt)
	return err
}

// applyPersistedKeyframeEvidence joins the authoritative keyframe-grid result
// written by persistFFprobeAnalysis back into the canonical facts document.
// SQLite stores unknown and false as the same integer, so the evidence
// timestamp is the presence marker: an empty timestamp remains unknown, while
// a timestamp plus 0 is an explicit negative result.
func applyPersistedKeyframeEvidence(tx *sql.Tx, mediaID string, file analysisFileIdentity, facts *mediafacts.Facts) error {
	rows, err := tx.Query(`SELECT stream_index, exact_seek_safe, keyframe_evidence_at
		FROM media_streams
		WHERE media_id = ? AND file_id = ? AND kind = 'video' AND source_kind = 'ffprobe'`, mediaID, file.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type evidence struct {
		safe bool
		at   string
	}
	byIndex := make(map[int]evidence)
	for rows.Next() {
		var index, safe int
		var at string
		if err := rows.Scan(&index, &safe, &at); err != nil {
			return err
		}
		at = strings.TrimSpace(at)
		if at == "" {
			continue
		}
		byIndex[index] = evidence{safe: safe != 0, at: at}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range facts.Video {
		observed, ok := byIndex[facts.Video[i].Index]
		if !ok {
			continue
		}
		facts.Video[i].ExactSeekSafe = exactSeekBoolPointer(observed.safe)
		facts.Video[i].KeyframeEvidenceAt = observed.at
		facts.Video[i].KeyframeEvidenceRevision = file.revision()
	}
	canonical, err := facts.Canonical()
	if err != nil {
		return err
	}
	*facts = canonical
	return nil
}

func exactSeekBoolPointer(value bool) *bool { return &value }

func playbackFactsFromFFprobe(file analysisFileIdentity, payload ffprobePayload) (mediafacts.Facts, error) {
	durationUS := decimalSecondsUS(payload.Format.Duration)
	source := streamfactsadapter.SourceRecord{
		Fingerprint: file.Fingerprint, Revision: file.revision(), SizeBytes: file.SizeBytes,
		Container: containerFromFFprobe(payload, "unknown.bin"), DurationUS: durationUS,
		DurationConfidence: confidence(durationUS > 0), StartTime: decimalRational(payload.Format.StartTime),
		VariableFrameRateConfidence: mediafacts.ConfidenceUnknown,
	}
	for _, raw := range payload.Streams {
		record, ok := playbackStreamRecord(raw)
		if ok {
			source.Streams = append(source.Streams, record)
			if source.TimeBase == nil && record.TimeBase != nil {
				source.TimeBase = record.TimeBase
			}
			if record.Kind == "video" && source.VariableFrameRate == nil && record.VariableFrameRate != nil {
				source.VariableFrameRate = record.VariableFrameRate
				source.VariableFrameRateConfidence = record.VariableFrameRateConfidence
			}
		}
	}
	facts, err := streamfactsadapter.Adapt(source)
	if err != nil {
		return mediafacts.Facts{}, err
	}
	for i, chapter := range payload.Chapters {
		facts.Chapters = append(facts.Chapters, mediafacts.Chapter{Index: i, StartUS: decimalSecondsUS(chapter.StartTime), EndUS: decimalSecondsUS(chapter.EndTime), Title: firstNonEmpty(chapter.Tags["title"], chapter.Tags["TITLE"])})
	}
	return facts.Canonical()
}

func playbackStreamRecord(raw ffprobeStream) (streamfactsadapter.StreamRecord, bool) {
	kind := ffprobeKind(raw.CodecType)
	if raw.CodecType == "attachment" || ffprobeStreamIsAttachedPicture(raw) {
		kind = "attachment"
	}
	if kind == "" && raw.CodecType != "attachment" {
		return streamfactsadapter.StreamRecord{}, false
	}
	falseValue := false
	disposition := func(key string) *bool { v := raw.Disposition[key] != 0; return &v }
	r := streamfactsadapter.StreamRecord{
		Index: raw.Index, Kind: kind, Codec: raw.CodecName, Profile: raw.Profile,
		Level: levelString(raw.Level), CodecTag: raw.CodecTagString,
		Duration: decimalRational(raw.Duration), DurationConfidence: confidence(decimalSecondsUS(raw.Duration) > 0),
		StartTime: decimalRational(raw.StartTime), TimeBase: parseRational(raw.TimeBase),
		Disposition: streamfactsadapter.DispositionRecord{Default: disposition("default"), Forced: disposition("forced"), HearingImpaired: disposition("hearing_impaired"), VisualImpaired: disposition("visual_impaired"), Original: disposition("original"), Commentary: disposition("comment")},
		Language:    firstNonEmpty(raw.Tags["language"], raw.Tags["LANGUAGE"]), Title: firstNonEmpty(raw.Tags["title"], raw.Tags["TITLE"]),
	}
	if bitrate, err := strconv.ParseInt(strings.TrimSpace(raw.BitRate), 10, 64); err == nil && bitrate > 0 {
		r.Bitrate = bitrate
	}
	switch kind {
	case "video":
		r.CodedWidth, r.CodedHeight, r.PixelFormat = raw.Width, raw.Height, raw.PixelFormat
		r.SampleAspectRatio = rationalOr(raw.SampleAspectRatio, mediafacts.Rational{Num: 1, Den: 1})
		r.DisplayAspectRatio = rationalOr(raw.AspectRatio, mediafacts.Rational{Num: int64(max(raw.Width, 1)), Den: int64(max(raw.Height, 1))})
		r.BitDepth, r.ChromaSubsampling = ffprobeBitDepth(raw), chromaFromPixelFormat(raw.PixelFormat)
		r.ColorRange, r.ColorPrimaries, r.ColorTransfer, r.ColorMatrix = raw.ColorRange, raw.ColorPrimaries, raw.ColorTransfer, raw.ColorSpace
		r.FieldOrder = raw.FieldOrder
		avg, nominal := parseRational(raw.AverageFrameRate), parseRational(raw.FrameRate)
		if avg != nil {
			r.AverageFrameRate, r.FrameRate = avg, *avg
		} else if nominal != nil {
			r.FrameRate = *nominal
		} else {
			r.FrameRate = mediafacts.Rational{Num: 0, Den: 1}
		}
		r.NominalFrameRate = nominal
		if avg != nil && nominal != nil {
			v := !rationalNear(*avg, *nominal)
			r.VariableFrameRate = &v
			r.VariableFrameRateConfidence = mediafacts.ConfidenceEstimated
		}
		r.HDR10Plus = &falseValue
		for _, side := range raw.SideDataList {
			applyVideoSideData(&r, side, raw)
		}
	case "audio":
		r.Layout, r.Channels, r.SampleRate, r.SampleFormat = raw.ChannelLayout, raw.Channels, intFromString(raw.SampleRate), raw.SampleFormat
		r.Service = firstNonEmpty(raw.Tags["service_name"], raw.Tags["SERVICE_NAME"])
		r.BitDepth = audioBitDepth(raw)
		r.EncoderDelaySamples, r.EncoderPaddingSamples = nonNegative64(raw.InitialPadding), nonNegative64(raw.TrailingPadding)
		if r.EncoderDelaySamples > 0 || r.EncoderPaddingSamples > 0 {
			r.GaplessConfidence = mediafacts.ConfidenceEstimated
			r.GaplessEvidence = "ffprobe stream initial_padding/trailing_padding"
		}
		profile := strings.ToLower(raw.Profile + " " + raw.Tags["title"])
		switch {
		case strings.Contains(profile, "atmos"):
			r.ObjectAudio, r.ObjectAudioEvidence = "dolby_atmos", "ffprobe profile/title explicitly identifies Atmos"
		case strings.Contains(profile, "dts:x"):
			r.ObjectAudio, r.ObjectAudioEvidence = "dts_x", "ffprobe profile/title explicitly identifies DTS:X"
		}
	case "subtitle":
		r.SubtitleKind = subtitleKind(raw.CodecName)
		r.ClosedCaption, r.SDH, r.Signs = disposition("captions"), disposition("hearing_impaired"), disposition("forced")
	case "attachment":
		r.Filename, r.MIMEType = firstNonEmpty(raw.Tags["filename"], raw.Tags["FILENAME"]), firstNonEmpty(raw.Tags["mimetype"], raw.Tags["MIMETYPE"])
	}
	return r, true
}

func applyVideoSideData(r *streamfactsadapter.StreamRecord, side ffprobeSideData, raw ffprobeStream) {
	t := strings.ToLower(side.SideDataType)
	if strings.Contains(t, "display matrix") {
		r.Rotation = int(math.Round(side.Rotation/90)) * 90
		for _, field := range strings.Fields(side.DisplayMatrix) {
			if v, err := strconv.ParseInt(field, 10, 64); err == nil {
				r.DisplayMatrix = append(r.DisplayMatrix, v)
			}
		}
		if len(r.DisplayMatrix) != 9 {
			r.DisplayMatrix = nil
		}
	}
	if strings.Contains(t, "content light") {
		r.MaxCLL, r.MaxFALL = side.MaxContent, side.MaxAverage
	}
	if strings.Contains(t, "mastering display") {
		r.MasteringDisplay = masteringDisplay(side)
	}
	if strings.Contains(t, "smpte2094-40") || strings.Contains(t, "hdr10+") {
		yes := true
		r.HDR10Plus = &yes
	}
	if side.DVProfile > 0 && (strings.Contains(t, "dovi") || strings.Contains(t, "dolby vision")) {
		r.DolbyVision = &streamfactsadapter.DolbyVisionRecord{Profile: side.DVProfile, Level: side.DVLevel, RPU: intBool(side.RPUPresent), EnhancementLayer: intBool(side.ELPresent), BaseLayerPresent: intBool(side.BLPresent), BaseLayerCodec: raw.CodecName, Fallback: dvFallback(side.BLSignalCompatibilityID), Evidence: "ffprobe Dolby Vision configuration record"}
	}
}

func masteringDisplay(s ffprobeSideData) *mediafacts.MasteringDisplay {
	values := []string{s.RedX, s.RedY, s.GreenX, s.GreenY, s.BlueX, s.BlueY, s.WhitePointX, s.WhitePointY, s.MinLuminance, s.MaxLuminance}
	parsed := make([]mediafacts.Rational, len(values))
	for i, v := range values {
		p := parseRational(v)
		if p == nil {
			return nil
		}
		parsed[i] = *p
	}
	return &mediafacts.MasteringDisplay{Red: mediafacts.Chromaticity{X: parsed[0], Y: parsed[1]}, Green: mediafacts.Chromaticity{X: parsed[2], Y: parsed[3]}, Blue: mediafacts.Chromaticity{X: parsed[4], Y: parsed[5]}, WhitePoint: mediafacts.Chromaticity{X: parsed[6], Y: parsed[7]}, MinLuminance: parsed[8], MaxLuminance: parsed[9]}
}

func parseRational(v string) *mediafacts.Rational {
	v = strings.TrimSpace(v)
	if v == "" || v == "N/A" || v == "0/0" {
		return nil
	}
	sep := "/"
	if strings.Contains(v, ":") {
		sep = ":"
	}
	p := strings.Split(v, sep)
	if len(p) != 2 {
		return nil
	}
	n, e1 := strconv.ParseInt(strings.TrimSpace(p[0]), 10, 64)
	d, e2 := strconv.ParseInt(strings.TrimSpace(p[1]), 10, 64)
	if e1 != nil || e2 != nil || d == 0 {
		return nil
	}
	return &mediafacts.Rational{Num: n, Den: d}
}
func decimalRational(v string) *mediafacts.Rational {
	f, e := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if e != nil {
		return nil
	}
	return &mediafacts.Rational{Num: int64(math.Round(f * 1_000_000)), Den: 1_000_000}
}
func decimalSecondsUS(v string) int64 {
	r := decimalRational(v)
	if r == nil || r.Num < 0 {
		return 0
	}
	return r.Num
}
func rationalOr(v string, fallback mediafacts.Rational) mediafacts.Rational {
	if r := parseRational(v); r != nil {
		return *r
	}
	return fallback
}
func rationalNear(a, b mediafacts.Rational) bool {
	return math.Abs(float64(a.Num)/float64(a.Den)-float64(b.Num)/float64(b.Den)) < .001
}
func confidence(ok bool) mediafacts.Confidence {
	if ok {
		return mediafacts.ConfidenceExact
	}
	return mediafacts.ConfidenceUnknown
}
func levelString(v int) string {
	if v <= 0 {
		return ""
	}
	return strconv.Itoa(v)
}
func intBool(v *int) *bool {
	if v == nil {
		return nil
	}
	b := *v != 0
	return &b
}
func dvFallback(id int) string {
	switch id {
	case 1:
		return "sdr"
	case 2:
		return "hdr10"
	case 4:
		return "hlg"
	}
	return ""
}
func chromaFromPixelFormat(v string) string {
	v = strings.ToLower(v)
	switch {
	case strings.Contains(v, "420"):
		return "4:2:0"
	case strings.Contains(v, "422"):
		return "4:2:2"
	case strings.Contains(v, "444"):
		return "4:4:4"
	}
	return ""
}
func audioBitDepth(s ffprobeStream) int {
	if v := intFromString(s.BitsPerRawSample); v > 0 {
		return v
	}
	f := strings.ToLower(s.SampleFormat)
	switch {
	case strings.Contains(f, "s32"), strings.Contains(f, "flt"):
		return 32
	case strings.Contains(f, "s24"):
		return 24
	case strings.Contains(f, "s16"):
		return 16
	case strings.Contains(f, "u8"):
		return 8
	}
	return 0
}
func subtitleKind(codec string) string {
	switch strings.ToLower(codec) {
	case "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "xsub":
		return "bitmap"
	}
	return "text"
}

func nonNegative64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
