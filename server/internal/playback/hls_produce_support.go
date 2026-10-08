package playback

import (
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/identity"
	"runtime"
	"strings"
	"sync"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/decoder"
)

// Conversion failure codes. The viewer sees the class; the owner also sees the
// converter's own words (with paths removed) on the diagnostics page.
const (
	FailureConverter        = "converter_failed"
	FailureDecoderMissing   = "source_codec_not_decodable"
	FailureEncoderMissing   = "encoder_unavailable"
	FailureFilterMissing    = "filter_unavailable"
	FailureSourceUnreadable = "source_unreadable"
	FailureOutOfSpace       = "conversion_storage_full"
	FailureCopyTimeline     = "copy_timeline_unavailable"

	// ReasonHardwareFailedSoftware is added to a plan whose hardware encoder
	// failed on the real source and was replaced by software mid-session.
	ReasonHardwareFailedSoftware = "hardware_encode_failed_software_retry"
	// ReasonCopyTimelineUnavailable is added to a plan that wanted to copy the
	// picture but whose source keyframes could not be indexed.
	ReasonCopyTimelineUnavailable = "copy_timeline_unavailable"
)

// tailWriter keeps the end of a stream, because a converter's last words are the
// ones that say why it stopped.
type tailWriter struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	if len(w.data) > w.limit {
		w.data = append([]byte{}, w.data[len(w.data)-w.limit:]...)
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.data)
}

// sanitizeConverterOutput removes what a diagnostic must never carry: the source
// path (on Windows the converter is given the file's own path) and the server's
// generated-media root. Control characters are dropped and the text is bounded.
func sanitizeConverterOutput(text string, secrets ...string) string {
	for _, secret := range secrets {
		if len(secret) < 4 {
			continue
		}
		text = strings.ReplaceAll(text, secret, "[path]")
		text = strings.ReplaceAll(text, filepath.ToSlash(secret), "[path]")
	}
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
	text = strings.TrimSpace(text)
	if len(text) > 2000 {
		text = text[len(text)-2000:]
	}
	return text
}

// classifyConverterFailure reads the converter's diagnostics for the causes an
// owner can act on. Anything else is the general class.
func classifyConverterFailure(diagnostic string) string {
	d := strings.ToLower(diagnostic)
	switch {
	case strings.Contains(d, "no space left"), strings.Contains(d, "disk full"), strings.Contains(d, "not enough space"):
		return FailureOutOfSpace
	case strings.Contains(d, "unknown encoder"), strings.Contains(d, "encoder not found"):
		return FailureEncoderMissing
	case strings.Contains(d, "no such filter"), strings.Contains(d, "filter not found"):
		return FailureFilterMissing
	case strings.Contains(d, "decoder not found"), strings.Contains(d, "unknown decoder"), strings.Contains(d, "decoding for stream") && strings.Contains(d, "failed"), strings.Contains(d, "could not find codec parameters"):
		return FailureDecoderMissing
	case strings.Contains(d, "invalid data found when processing input"), strings.Contains(d, "no such file"), strings.Contains(d, "input/output error"), strings.Contains(d, "permission denied"):
		return FailureSourceUnreadable
	}
	return FailureConverter
}

// failedWith fails a session and records why, for the people who can read it.
func (h *HLS) failedWith(ctx context.Context, id, code, diagnostic string) {
	if !h.failed(ctx, id) {
		return
	}
	write, cancel := persist(ctx)
	defer cancel()
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_artifacts SET failure_code=?,diagnostic=? WHERE session_id=?`, code, diagnostic, id)
}

// recordSoftwareFallback rewrites the stored plan after a hardware encoder failed
// on a real source, so every later window of this session starts in software and
// the technical details say what happened.
func (h *HLS) recordSoftwareFallback(ctx context.Context, id, backend, diagnostic string) {
	write, cancel := persist(ctx)
	defer cancel()
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_delivery_plans SET hardware_backend='software',hardware_native=0,hardware_device='',reason_codes=CASE WHEN instr(reason_codes,?)>0 THEN reason_codes ELSE reason_codes||','||? END WHERE session_id=?`, ReasonHardwareFailedSoftware, ReasonHardwareFailedSoftware, id)
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_artifacts SET failure_code=?,diagnostic=? WHERE session_id=?`, "hardware_encoder_failed:"+backend, diagnostic, id)
	h.hardwareFailures.Add(1)
	decoder.RecordHardwareFailure(decoder.HardwareBackend(backend))
}

// downgradeCopyPlan turns a plan that copies the picture into one that converts
// it, for a source whose keyframes could not be indexed. It is refused when the
// owner has turned conversion off: a wrong timeline is not something to fall
// back to.
func (h *HLS) downgradeCopyPlan(ctx context.Context, id string, plan *DeliveryPlan, replacement ...bool) (*DeliveryPlan, error) {
	gate, err := dbwork.Begin(ctx, h.db, dbwork.ClassEstablishedPlayback)
	if err != nil {
		return nil, err
	}
	defer gate.Rollback()
	tx := gate.Tx()
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1`).Scan(&enabled); err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrTranscodingDisabled
	}
	source, err := loadDeliverySource(ctx, tx, plan.SourceID)
	if err != nil {
		return nil, err
	}
	var p identity.Principal
	if err = tx.QueryRowContext(ctx, `SELECT session_hash,account_id,profile_id FROM playback_sessions WHERE id=? AND state NOT IN('stopped','ended','failed')`, id).Scan(&p.Hash, &p.AccountID, &p.ProfileID); err != nil {
		return nil, err
	}
	cfg := h.configuration()
	policy := ResolveDeliveryPolicy(nil, NetworkUnknown, LocalityUnknown, "unknown", cfg)
	if plan.Policy != nil {
		policy = *plan.Policy
	}
	height := 0
	if source.Video != nil {
		height = source.Video.Height
	}
	rung, ok := SelectedRung(QualityOffers(height, true, true, policy), plan.QualityID)
	if !ok {
		return nil, ErrQualityUnavailable
	}
	excluded, err := routeFailuresTx(ctx, tx, p, plan.SourceID)
	if err != nil {
		return nil, err
	}
	if excluded == nil {
		excluded = map[DeliveryStrategy]RouteRejection{}
	}
	for _, route := range []DeliveryStrategy{DeliveryOriginal, DeliveryCopyRemux, DeliveryAudioConversion} {
		excluded[route] = RouteRejection{Code: ReasonCopyTimelineUnavailable}
	}
	next, err := planDelivery(DeliveryInput{SourceID: source.ID, Source: &source, Client: clientProfileFor(ctx, tx, p), Server: decoder.CurrentToolchain(), Policy: policy, Target: rung.Target(policy, height), Config: cfg, TranscodingEnabled: true, AudioStream: plan.AudioStream, HasAudioChoice: plan.AudioStream >= 0, Excluded: excluded, BurnIn: plan.BurnIn != nil, Remote: source.Container == "strm"})
	if err != nil {
		return nil, err
	}
	if next.VideoAction != "convert" {
		return nil, ErrIncompatible
	}
	next.SourceSize, next.SourceModifiedNS, next.FactsRevision = plan.SourceSize, plan.SourceModifiedNS, plan.FactsRevision
	next.BurnIn = plan.BurnIn
	next.QualityID = plan.QualityID
	next.ReasonCodes = append(next.ReasonCodes, ReasonCopyTimelineUnavailable)
	if plan.Trace != nil && plan.Trace.Audio != nil && next.Trace != nil && next.Trace.Audio != nil {
		next.Trace.Audio.ChosenBy = plan.Trace.Audio.ChosenBy
	}
	if err = reserveHLS(tx, id, id, &next, cfg); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM playback_delivery_plans WHERE session_id=?`, id); err != nil {
		return nil, err
	}
	if err = persistDeliveryPlan(tx, id, &next); err != nil {
		return nil, err
	}
	if len(replacement) > 0 && replacement[0] {
		grant := identity.Token()
		if _, err = tx.ExecContext(ctx, `UPDATE playback_sessions SET generation=generation+1,grant_token=?,grant_hash=? WHERE id=?`, grant, identity.Digest(grant), id); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE playback_subtitle_state SET generation=generation+1 WHERE session_id=?`, id); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE playback_subtitle_presentations SET generation=generation+1 WHERE session_id=?`, id); err != nil {
			return nil, err
		}
	}
	if err = gate.Commit(); err != nil {
		return nil, err
	}
	return &next, nil
}

// copyTimelineFor returns the cut list for a session's source: from memory, from
// the per-file cache, or by indexing the container now.
func (h *HLS) copyTimelineFor(ctx context.Context, id, asset string, input *assets.PlaybackInput, duration float64) (*copyTimeline, error) {
	h.mu.Lock()
	if t := h.timelines[id]; t != nil {
		h.mu.Unlock()
		return t, nil
	}
	h.mu.Unlock()
	var size, modified int64
	if err := dbwork.QueryRow(ctx, h.db, `SELECT size,modified_ns FROM playback_source_pins WHERE session_id=? AND asset_id=?`, id, asset).Scan(&size, &modified); err != nil {
		return nil, err
	}
	if t, err := loadCopyTimeline(ctx, h.db, asset, size, modified); err == nil && t != nil {
		return t, nil
	}
	if h.ffprobe == "" {
		return nil, errCopyTimeline
	}
	t, err := probeCopyTimeline(ctx, h.ffprobe, input, duration)
	if err != nil {
		return nil, err
	}
	storeCopyTimeline(ctx, h.db, asset, size, modified, t)
	return t, nil
}

// sessionTimeline is the cut list a request for this session's segments is
// checked against. A session not in memory (the server restarted under it) reads
// the per-file cache; a session that is not a copy session has none.
func (h *HLS) sessionTimeline(ctx context.Context, id string) *copyTimeline {
	h.mu.Lock()
	t := h.timelines[id]
	h.mu.Unlock()
	if t != nil {
		return t
	}
	var action, asset string
	var size, modified int64
	err := dbwork.QueryRow(ctx, h.db, `SELECT d.video_action,pin.asset_id,pin.size,pin.modified_ns FROM playback_delivery_plans d JOIN playback_source_pins pin ON pin.session_id=d.session_id WHERE d.session_id=?`, id).Scan(&action, &asset, &size, &modified)
	if err != nil || action != "copy" {
		return nil
	}
	t, err = loadCopyTimeline(ctx, h.db, asset, size, modified)
	if err != nil || t == nil {
		return nil
	}
	h.mu.Lock()
	h.timelines[id] = t
	h.mu.Unlock()
	return t
}

func (h *HLS) timelineCount(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.timelines[id].count()
}

// siblingProbe finds ffprobe next to the ffmpeg this producer runs, which is
// where every packaged build puts it, and falls back to the search path.
func siblingProbe(ffmpeg string) string {
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name = "ffprobe.exe"
	}
	if ffmpeg != "" {
		candidate := filepath.Join(filepath.Dir(ffmpeg), name)
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved
	}
	return ""
}

// ConfigureProbe names the ffprobe used to index copied sources.
func (h *HLS) ConfigureProbe(path string) {
	if resolved, err := exec.LookPath(path); err == nil {
		h.ffprobe = resolved
	}
}

// conversionFailure is what the diagnostics reader publishes about a failed or
// degraded conversion.
type conversionFailure struct {
	Code       string `json:"code"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

func readConversionFailure(ctx context.Context, tx *sql.Tx, session string) (*conversionFailure, error) {
	var out conversionFailure
	err := tx.QueryRowContext(ctx, `SELECT failure_code,diagnostic FROM playback_artifacts WHERE session_id=?`, session).Scan(&out.Code, &out.Diagnostic)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && out.Code == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}
