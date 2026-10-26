package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/disc"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ownedcapture"
	"portico.local/server/internal/playbackdisc"
)

// This is a process-private measured value, not a recoverable proof in rows.
// The original preparation owns it until an admitted continuation completes its
// accepting writers or retires. Background measurement can never bind a session.
type discPlayMeasurement struct {
	ctx      context.Context
	input    *assets.PlaybackInput
	probe    *disc.CapturedProbe
	original func(context.Context, *sql.Tx) error
	mu       sync.Mutex
	snapshot playbackdisc.Snapshot
	bound    bool
	closed   atomic.Bool
}

// template contains only the original admitted actor/operation/physical source.
// Tools must observe the exact helper/probe/converter code used by this operation,
// before and after actual measurement. Original checks retain source/content,
// actor/family, claim, selected physical object and queue/custody intent.
// No field in this private construction is accepted from a public request as proof.
type capturedDiscMeasure struct {
	DB       *sql.DB
	Reader   *ownedcapture.Reader
	Tool     disc.Tool
	Probe    assets.Probe
	Template playbackdisc.Snapshot
	TitleKey string // resolved metadata hint; must match these actual measured bytes
	Current  func(context.Context) error
	AcceptTx func(context.Context, *sql.Tx) error
	Tools    func(context.Context) (playbackdisc.ToolPins, error)
}

// measureCapturedDiscPlay consumes Reader after the outside-SQL context check.
// Its caller must Close the returned value on EVERY outcome, after its original
// accepting COMMIT when accepted. An invalid outer context retains caller custody.
func measureCapturedDiscPlay(ctx context.Context, o capturedDiscMeasure) (_ *discPlayMeasurement, err error) {
	if ctx == nil || dbwork.Snapshot(ctx) != nil || o.Reader == nil {
		return nil, disc.ErrProtocol
	}
	var input *assets.PlaybackInput
	defer func() {
		if err != nil {
			if input != nil {
				input.Close()
			} else {
				o.Reader.Close()
			}
		}
	}()
	if o.DB == nil || o.Current == nil || o.AcceptTx == nil || o.Tools == nil || o.Template.ID != "" || o.Template.SessionID != "" || o.Template.Generation != 0 || o.Template.Input.Captured != nil || o.Template.Input.Conditional != nil {
		return nil, disc.ErrProtocol
	}
	if err = o.Current(ctx); err != nil {
		return nil, err
	}
	view, err := dbwork.BeginSnapshot(ctx, o.DB)
	if err != nil {
		return nil, err
	}
	err = o.AcceptTx(ctx, view.Tx())
	view.Rollback()
	if err != nil {
		return nil, err
	}
	pins, err := o.Tools(ctx)
	if err != nil {
		return nil, err
	}
	observation, err := o.Tool.ProbeCapturedISO(ctx, o.Reader)
	if err != nil {
		return nil, err
	}
	structure, err := observation.Structure()
	if err != nil {
		return nil, err
	}
	title, ok := structure.MainTitle()
	if o.TitleKey != "" {
		ok = false
		for _, candidate := range structure.Titles {
			if candidate.Key(structure.Kind) == o.TitleKey {
				if ok {
					return nil, disc.ErrProtocol
				}
				title, ok = candidate, true
			}
		}
	}
	if !ok {
		return nil, ErrStaleOffer
	}
	input, err = capturedDiscInput(ctx, o.Tool, o.Reader, structure, title, o.Current)
	if err != nil {
		return nil, err
	}
	facts, err := o.Probe.InspectDiscPlayback(ctx, input, structure, title)
	if err != nil {
		return nil, err
	}
	// Root65 supplies no disc-to-audio/publication adapter. A filename-derived
	// video item must have actual video evidence before planning this presentation.
	if facts.VideoCodec == "" || len(facts.Streams) == 0 {
		return nil, ErrIncompatible
	}
	after, err := o.Tools(ctx)
	if err != nil {
		return nil, err
	}
	if pins != after {
		return nil, ErrStaleOffer
	}
	if err = input.Validate(ctx); err != nil {
		return nil, err
	}
	snapshot := o.Template
	snapshot.Tools = pins
	snapshot.Input = playbackdisc.Input{Captured: &playbackdisc.Captured{Receipt: o.Reader.Receipt()}}
	snapshot.Media = playbackdisc.MediaFacts{Container: facts.Container, VideoCodec: facts.VideoCodec, AudioCodec: facts.AudioCodec,
		Width: facts.Width, Height: facts.Height, DurationMs: title.DurationMs, OriginUS: facts.OriginUS, TimingKnown: facts.TimingKnown,
		Streams: append([]assets.Stream(nil), facts.Streams...), Chapters: append([]assets.Chapter(nil), facts.Chapters...)}
	snapshot.Disc = playbackdisc.DiscFacts{Structure: structure, SelectedTitleKey: title.Key(structure.Kind), StructureRevision: observation.Fingerprint()}
	for _, part := range []struct {
		value any
		out   *string
	}{{snapshot.Media.Streams, &snapshot.Disc.StreamRevision}, {snapshot.Media.Chapters, &snapshot.Disc.ChapterRevision}, {snapshot.Media, &snapshot.Disc.FactsRevision}} {
		raw, e := json.Marshal(part.value)
		if e != nil {
			return nil, e
		}
		// Scope actual facts to this unique acquired input and selected title.
		*part.out = identity.Digest("disc-play-facts-v1\x00" + observation.Fingerprint() + "\x00" + snapshot.Disc.SelectedTitleKey + "\x00" + string(raw))
	}
	return &discPlayMeasurement{ctx: ctx, input: input, probe: observation, original: o.AcceptTx, snapshot: snapshot}, nil
}

func (m *discPlayMeasurement) Close() error {
	if m == nil {
		return nil
	}
	m.closed.Store(true)
	return m.input.Close()
}

func (m *discPlayMeasurement) CheckTx(ctx context.Context, tx *sql.Tx, candidate playbackdisc.Snapshot) error {
	if m == nil || tx == nil || m.closed.Load() || m.ctx == nil {
		return disc.ErrProtocol
	}
	if err := m.ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	exact := m.bound && reflect.DeepEqual(candidate, m.snapshot)
	m.mu.Unlock()
	if !exact {
		return ErrStaleOffer
	}
	return m.CheckPreparedTx(ctx, tx)
}

// CheckPreparedTx checks actual sealed facts while no media session exists yet.
// A preparation row can retain this private value; it cannot recover the proof
// from stored facts, a receipt ID or matching bytes after a process restart.
func (m *discPlayMeasurement) CheckPreparedTx(ctx context.Context, tx *sql.Tx) error {
	if m == nil || ctx == nil || tx == nil || m.closed.Load() || m.ctx == nil {
		return disc.ErrProtocol
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.ctx.Err(); err != nil {
		return err
	}
	if err := m.original(ctx, tx); err != nil {
		return err
	}
	if err := m.probe.CheckTx(ctx, tx); err != nil {
		return err
	}
	if m.closed.Load() {
		return disc.ErrProtocol
	}
	return m.ctx.Err()
}

// BindPendingTx is called by the actual legacy-session writer, after inserting
// its real ID/generation and physical source pins. Even a failed attempt cannot
// retarget this process proof to a different session. Store activation belongs
// only to the later original v1 accepting continuation, never the background worker.
func (m *discPlayMeasurement) BindPendingTx(ctx context.Context, tx *sql.Tx, store *playbackdisc.Store, key playbackdisc.Key, continuation func(context.Context, *sql.Tx) error) (string, error) {
	if m == nil || store == nil || tx == nil || continuation == nil || m.closed.Load() || key.SessionID == "" || key.Generation < 1 {
		return "", disc.ErrProtocol
	}
	if err := continuation(ctx, tx); err != nil {
		return "", err
	}
	m.mu.Lock()
	if m.bound {
		m.mu.Unlock()
		return "", ErrStaleOffer
	}
	m.snapshot.ID, m.snapshot.SessionID, m.snapshot.Generation = identity.Token(), key.SessionID, key.Generation
	m.bound = true
	snapshot := m.snapshot
	m.mu.Unlock()
	if err := store.PublishPendingTx(ctx, tx, snapshot, m.acceptingCheck(continuation)); err != nil {
		return "", err
	}
	return snapshot.ID, nil
}

func (m *discPlayMeasurement) ActivateTx(ctx context.Context, tx *sql.Tx, store *playbackdisc.Store, key playbackdisc.Key, continuation func(context.Context, *sql.Tx) error) error {
	if m == nil || store == nil || continuation == nil {
		return disc.ErrProtocol
	}
	m.mu.Lock()
	snapshot := m.snapshot
	exact := m.bound && key.SessionID == snapshot.SessionID && key.Generation == snapshot.Generation
	m.mu.Unlock()
	if !exact {
		return ErrStaleOffer
	}
	_, err := store.ActivateTx(ctx, tx, key, snapshot.ID, m.acceptingCheck(continuation))
	return err
}

// Worker/source proof alone cannot accept. The real still-admitted original
// POST supplies this separate continuation check, including exact current
// request body/key/caller, prepared claim borrow, queue/custody and capacity.
// The caller also performs it before inserting a legacy row. No default applies.
func (m *discPlayMeasurement) acceptingCheck(continuation func(context.Context, *sql.Tx) error) playbackdisc.Check {
	return func(ctx context.Context, tx *sql.Tx, candidate playbackdisc.Snapshot) error {
		if continuation == nil {
			return disc.ErrProtocol
		}
		if err := continuation(ctx, tx); err != nil {
			return err
		}
		if err := m.CheckTx(ctx, tx, candidate); err != nil {
			return err
		}
		return continuation(ctx, tx)
	}
}
