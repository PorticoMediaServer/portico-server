package playback

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback/vod"
	"portico.local/server/internal/supervise"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrSegmentPreparing = errors.New("Media is still being prepared. Retry playback.")
var ErrFiniteUnsupported = errors.New("This source needs a playback path that is not available in the selected conversion policy.")

// FiniteHLS is the conversion-policy adapter. The session remains the authority;
// an internal generation owns only unpublished work. One actor per admitted HLS
// session serializes its bounded FIFO and shares the existing global admission.
// It never turns a late segment GET into a replacement playback intent.
type FiniteHLS struct {
	h       *HLS
	encoder *vod.Encoder
	mu      sync.Mutex
	actors  map[string]*finiteActor
}
type finiteActor struct {
	ctx     context.Context
	cancel  context.CancelFunc
	ready   chan struct{}
	wake    chan struct{}
	err     error // written before ready closes
	source  *vod.Source
	waiters int // under FiniteHLS.mu
}

func (h *HLS) EnableFinite(ffprobe string) error {
	e, err := vod.NewEncoder(h.binary, ffprobe, vod.Limits{})
	if err != nil {
		return err
	}
	h.finite = &FiniteHLS{h: h, encoder: e, actors: map[string]*finiteActor{}}
	// A process restart cannot adopt a worker generation that no longer exists.
	_, err = dbwork.ExecWrite(context.Background(), h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_vod_intervals SET status='queued',generation=0 WHERE status='working'`)
	return err
}
func (f *FiniteHLS) input(ctx context.Context, id string) (vod.Input, error) {
	pin, err := assets.ResolvePlaybackSource(ctx, f.h.db, id, "")
	if err != nil {
		return vod.Input{}, err
	}
	source, err := assets.OpenPlaybackInput(ctx, f.h.db, f.h.SourceStorage, id)
	if err != nil {
		return vod.Input{}, err
	}
	return vod.Input{File: source.File, Argument: source.Argument, Inherit: source.Inherit,
		Identity: vod.Identity{Size: pin.Size, ModifiedNS: pin.ModifiedNS},
		Validate: func(ctx context.Context, file *os.File, p vod.Identity) error {
			return f.h.SourceStorage.ValidatePlaybackDescriptor(ctx, file, p.Size, p.ModifiedNS)
		}}, nil
}
func (f *FiniteHLS) live(id string) bool {
	var live bool
	err := f.h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM playback_sessions p JOIN authorization_access s ON s.hash=p.session_hash LEFT JOIN accounts a ON a.id=s.account_id WHERE p.id=? AND p.state NOT IN ('stopped','ended','failed') AND p.expires_at>? AND s.expires_at>? AND s.revoked=0 AND (s.authority!='local' OR s.epoch=a.epoch))`, id, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)).Scan(&live)
	return err == nil && live
}
func finiteManifest(s *vod.Source) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:7\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-START:TIME-OFFSET=0,PRECISE=YES\n")
	for i := 0; i < vod.IntervalCount(s.Duration); i++ {
		// Video and AAC packet clocks share one source-relative timeline.
		start, end, _ := vod.IntervalBounds(s.Duration, i)
		fmt.Fprintf(&b, "#EXTINF:%.9f,\nsegment-%06d.ts\n", end-start, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}
func (f *FiniteHLS) ensure(ctx context.Context, id string) (*finiteActor, error) {
	if !f.live(id) {
		return nil, identity.ErrUnauthorized
	}
	f.mu.Lock()
	a := f.actors[id]
	if a == nil {
		f.h.mu.Lock()
		if f.h.cleaning[id] {
			f.h.mu.Unlock()
			f.mu.Unlock()
			return nil, identity.ErrUnauthorized
		}
		worker, cancel := context.WithCancel(f.h.ctx)
		a = &finiteActor{ctx: worker, cancel: cancel, ready: make(chan struct{}), wake: make(chan struct{}, 1)}
		f.actors[id] = a
		f.h.active[id] = cancel
		f.h.mu.Unlock()
		supervise.Go("playback.finite-hls.run", func() { f.run(id, a) })
	}
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.ready:
		if a.err != nil {
			return a, a.err
		}
		if a.ctx.Err() != nil {
			return nil, ErrSegmentPreparing
		}
		return a, nil
	}
}
func (f *FiniteHLS) prepare(id string, a *finiteActor) error {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	input, err := f.input(ctx, id)
	if err != nil {
		return err
	}
	defer input.File.Close()
	source, err := f.encoder.Analyze(ctx, input)
	if err != nil {
		if errors.Is(err, vod.ErrUnsupported) {
			return ErrFiniteUnsupported
		}
		return err
	}
	if !f.live(id) {
		return identity.ErrUnauthorized
	}
	manifest := finiteManifest(source)
	hash := fmt.Sprintf("%x", sha256.Sum256(manifest))
	dir := filepath.Join(f.h.root, id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// Uncommitted generations cannot be served. A prior process may have left
	// owned temporary directories; only this session's private work is removed.
	if err = os.RemoveAll(filepath.Join(dir, "work")); err != nil {
		return err
	}
	if err = os.Mkdir(filepath.Join(dir, "work"), 0700); err != nil {
		return err
	}
	var previous, previousPolicy string
	var size, modified int64
	var fps float64
	var vi, ai, width, height int
	err = f.h.db.QueryRow(`SELECT manifest_hash,policy,source_size,source_modified_ns,fps,video_index,audio_index,width,height FROM playback_vod_plans WHERE session_id=?`, id).Scan(&previous, &previousPolicy, &size, &modified, &fps, &vi, &ai, &width, &height)
	if err == nil && (previousPolicy != vod.Policy || size != source.Size || modified != source.ModifiedNS || fps != source.FPS || vi != source.VideoIndex || ai != source.AudioIndex || width != source.Width || height != source.Height) {
		return vod.ErrChanged
	}
	if err == nil && previous != hash {
		return vod.ErrChanged
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	gated, err := dbwork.Begin(ctx, f.h.db, dbwork.ClassPlaybackStart)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if _, err = tx.Exec(`INSERT INTO playback_vod_plans(session_id,policy,source_size,source_modified_ns,duration,fps,video_index,audio_index,width,height,interval_count,manifest_hash,status,analysis_wall,analysis_cpu,probe_output_bytes) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'ready',?,?,?) ON CONFLICT(session_id) DO NOTHING`, id, vod.Policy, source.Size, source.ModifiedNS, source.Duration, source.FPS, source.VideoIndex, source.AudioIndex, source.Width, source.Height, vod.IntervalCount(source.Duration), hash, source.AnalysisWork.WallSeconds, source.AnalysisWork.CPUSeconds, source.AnalysisWork.ProbeOutputBytes); err != nil {
		return err
	}
	for i := 0; i < vod.IntervalCount(source.Duration); i++ {
		start, end, _ := vod.IntervalBounds(source.Duration, i)
		if _, err = tx.Exec(`INSERT INTO playback_vod_intervals(session_id,ordinal,status,start,end) VALUES(?,?,'absent',?,?) ON CONFLICT(session_id,ordinal) DO NOTHING`, id, i, start, end); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`INSERT INTO playback_artifacts(session_id,directory,status) VALUES(?,?,'building') ON CONFLICT(session_id) DO UPDATE SET status='building',error=''`, id, dir); err != nil {
		return err
	}
	if err = gated.Commit(); err != nil {
		return err
	}
	// Atomic publication of an immutable index. A crash before this rename is
	// repaired from identical plan facts on reconnection, never speculative bytes.
	path := filepath.Join(dir, "master.m3u8")
	if b, e := os.ReadFile(path); e == nil {
		if string(b) != string(manifest) {
			return vod.ErrChanged
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	} else {
		if err = os.WriteFile(path+".tmp", manifest, 0600); err != nil {
			return err
		}
		if err = os.Rename(path+".tmp", path); err != nil {
			return err
		}
	}
	a.source = source
	return nil
}
func (f *FiniteHLS) run(id string, a *finiteActor) {
	// Keep authority monitoring independent of a potentially stalled subprocess.
	// Cancellation does not release the admission until the process actually exits.
	supervise.Go("playback.finite-hls.watch", func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-tick.C:
				if !f.live(id) {
					a.cancel()
					return
				}
			}
		}
	})
	defer func() {
		a.cancel()
		f.mu.Lock()
		f.h.mu.Lock()
		delete(f.actors, id)
		delete(f.h.active, id)
		f.h.mu.Unlock()
		f.mu.Unlock()
	}()
	// The producer's failures are fenced to the generation it started for.
	fail := a.ctx
	var generation int
	if f.h.db.QueryRowContext(a.ctx, `SELECT generation FROM playback_sessions WHERE id=?`, id).Scan(&generation) == nil {
		fail = withProducerGeneration(a.ctx, generation)
	}
	a.err = f.prepare(id, a)
	close(a.ready)
	if a.err != nil {
		f.h.failed(fail, id)
		return
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if !f.live(id) {
			return
		}
		var index int
		err := f.h.db.QueryRow(`SELECT ordinal FROM playback_vod_intervals WHERE session_id=? AND status='queued' ORDER BY requested_order,ordinal LIMIT 1`, id).Scan(&index)
		if err == nil {
			if err = f.produce(id, a, index); err != nil {
				if a.ctx.Err() != nil {
					return
				}
				var attempts int
				readErr := f.h.db.QueryRow(`SELECT attempts FROM playback_vod_intervals WHERE session_id=? AND ordinal=?`, id, index).Scan(&attempts)
				if readErr == nil && attempts < 3 && !errors.Is(err, vod.ErrUnsupported) && !errors.Is(err, vod.ErrChanged) && !errors.Is(err, vod.ErrBudget) && !errors.Is(err, identity.ErrUnauthorized) {
					_, updateErr := dbwork.ExecWrite(context.Background(), f.h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_vod_intervals SET status='absent',error_code='production_retryable' WHERE session_id=? AND ordinal=? AND status='working'`, id, index)
					if updateErr == nil {
						continue
					}
				}
				_, _ = dbwork.ExecWrite(context.Background(), f.h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_vod_intervals SET status='failed',error_code='production_failed' WHERE session_id=? AND ordinal=? AND status!='committed'`, id, index)
				f.h.failed(fail, id)
				return
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			f.h.failed(fail, id)
			return
		}
		select {
		case <-a.ctx.Done():
			return
		case <-a.wake:
		case <-tick.C:
		}
	}
}
func (f *FiniteHLS) produce(id string, a *finiteActor, index int) error {
	gated2, err := dbwork.Begin(a.ctx, f.h.db, dbwork.ClassPlaybackStart)
	if err != nil {
		return err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	var generation int64
	if err = tx.QueryRow(`UPDATE playback_vod_plans SET generation=generation+1 WHERE session_id=? RETURNING generation`, id).Scan(&generation); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE playback_vod_intervals SET status='working',generation=?,attempts=attempts+1 WHERE session_id=? AND ordinal=? AND status='queued'`, generation, id, index)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("work ownership changed")
	}
	var retained int64
	if err = tx.QueryRow(`SELECT COALESCE(sum(bytes),0) FROM playback_vod_intervals WHERE session_id=?`, id).Scan(&retained); err != nil {
		return err
	}
	if retained+(32<<20) > audioArtifactBytes {
		return vod.ErrBudget
	}
	if err = gated2.Commit(); err != nil {
		return err
	}
	input, err := f.input(a.ctx, id)
	if err != nil {
		return err
	}
	defer input.File.Close()
	dir := filepath.Join(f.h.root, id, "work", strconv.FormatInt(generation, 10))
	window, err := f.encoder.Encode(a.ctx, a.source, input, vod.Request{Index: index, GenerationDir: dir})
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if !f.live(id) {
		return identity.ErrUnauthorized
	}
	digest, err := finiteHash(window.Path)
	if err != nil {
		return err
	}
	path := filepath.Join(f.h.root, id, fmt.Sprintf("segment-%06d.ts", index))
	// Link is no-clobber, unlike Rename. A crashed pre-commit publication may be
	// adopted only if its bytes match this deterministic generation's output.
	if err = os.Link(window.Path, path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		old, e := finiteHash(path)
		if e != nil {
			return e
		}
		if old != digest {
			return vod.ErrChanged
		}
	}
	result, err = dbwork.ExecWrite(context.Background(), f.h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_vod_intervals SET status='committed',bytes=?,sha256=?,decode_from=?,encode_wall=?,encode_cpu=?,probe_wall=? WHERE session_id=? AND ordinal=? AND status='working' AND generation=? AND EXISTS(SELECT 1 FROM playback_sessions WHERE id=? AND state NOT IN ('stopped','ended','failed'))`, window.Bytes, digest, window.DecodeFrom, window.EncodeWork.WallSeconds, window.EncodeWork.CPUSeconds, window.ProbeWork.WallSeconds, id, index, generation, id)
	if err != nil {
		return err
	}
	n, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return identity.ErrUnauthorized
	}
	return nil
}
func (f *FiniteHLS) file(ctx context.Context, id, name string) (string, error) {
	a, err := f.ensure(ctx, id)
	if err != nil {
		return "", err
	}
	path := filepath.Join(f.h.root, id, name)
	if name == "master.m3u8" {
		return path, nil
	}
	if !artifactName.MatchString(name) {
		return "", errors.New("unknown finite resource")
	}
	var index int
	if _, err = fmt.Sscanf(name, "segment-%06d.ts", &index); err != nil {
		return "", err
	}
	if index < 0 || index >= vod.IntervalCount(a.source.Duration) {
		return "", errors.New("unknown finite interval")
	}
	f.mu.Lock()
	if a.waiters >= 32 {
		f.mu.Unlock()
		return "", ErrConversionCapacity
	}
	a.waiters++
	f.mu.Unlock()
	defer func() { f.mu.Lock(); a.waiters--; f.mu.Unlock() }()
	gated3, err := dbwork.Begin(ctx, f.h.db, dbwork.ClassPlaybackStart)
	if err != nil {
		return "", err
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	var state string
	if err = tx.QueryRow(`SELECT status FROM playback_vod_intervals WHERE session_id=? AND ordinal=?`, id, index).Scan(&state); err != nil {
		return "", err
	}
	if state == "absent" {
		var queued int
		if err = tx.QueryRow(`SELECT count(*) FROM playback_vod_intervals WHERE session_id=? AND status IN ('queued','working')`, id).Scan(&queued); err != nil {
			return "", err
		}
		if queued >= 8 {
			return "", ErrConversionCapacity
		}
		if _, err = tx.Exec(`UPDATE playback_vod_intervals SET status='queued',requested_order=? WHERE session_id=? AND ordinal=? AND status='absent'`, time.Now().UnixNano(), id, index); err != nil {
			return "", err
		}
	}
	if err = gated3.Commit(); err != nil {
		return "", err
	}
	select {
	case a.wake <- struct{}{}:
	default:
	}
	wait, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if !f.live(id) {
			return "", identity.ErrUnauthorized
		}
		var hash string
		if err = f.h.db.QueryRowContext(wait, `SELECT status,sha256 FROM playback_vod_intervals WHERE session_id=? AND ordinal=?`, id, index).Scan(&state, &hash); err != nil {
			return "", err
		}
		if state == "committed" {
			// Committed bytes never regenerate under an existing URI. Missing/corrupt
			// retained output fails this presentation rather than serving changed media.
			digest, e := finiteHash(path)
			if e != nil {
				f.h.failed(a.ctx, id)
				return "", e
			}
			if digest != hash {
				f.h.failed(a.ctx, id)
				return "", vod.ErrChanged
			}
			return path, nil
		}
		if state == "failed" {
			return "", ErrFiniteUnsupported
		}
		if state == "absent" {
			return "", ErrSegmentPreparing
		}
		select {
		case <-wait.Done():
			return "", ErrSegmentPreparing
		case <-tick.C:
		}
	}
}

func finiteHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.CopyBuffer(digest, file, make([]byte, 32<<10)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
