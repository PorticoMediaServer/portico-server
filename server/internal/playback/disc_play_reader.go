package playback

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/disc"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ownedcapture"
	"portico.local/server/internal/playbackdisc"
)

// This is ordinary active-presentation authority, separate from preparation's
// token/deadline and worker/continuation checks. The consumer supplies the exact
// accepted legacy key AND snapshot ID. No asset/digest/latest lookup is offered.
type discPlayReaderOptions struct {
	DB       *sql.DB
	Store    *playbackdisc.Store
	Captures *ownedcapture.Service
	Key      playbackdisc.Key
	Snapshot string
	Tool     disc.Tool
	Tools    func(context.Context) (playbackdisc.ToolPins, error)
	Current  playbackdisc.Check
}

// openCapturedDiscPresentation opens a NEW independently checked reader for an
// ACTIVE immutable snapshot. It never borrows a preparation's descriptor or
// 120s context. The normalized presentation pin commits before descriptor/hash
// work or helper launch. Retain the returned whole input until its child exits.
func openCapturedDiscPresentation(ctx context.Context, o discPlayReaderOptions) (_ *assets.PlaybackInput, err error) {
	if ctx == nil || ctx.Done() == nil || dbwork.Snapshot(ctx) != nil || o.DB == nil || o.Store == nil || o.Captures == nil || o.Current == nil || o.Tools == nil || o.Key.SessionID == "" || o.Key.Generation < 1 || o.Snapshot == "" {
		return nil, disc.ErrProtocol
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	tools, err := o.Tools(ctx)
	if err != nil {
		return nil, err
	}
	var original playbackdisc.Snapshot
	var pin playbackdisc.Pin
	err = dbwork.WithWriteTxContext(ctx, o.DB, dbwork.ClassEstablishedPlayback, func(check context.Context, tx *sql.Tx) error {
		var e error
		original, e = o.Store.ResolveTx(check, tx, o.Key, o.Snapshot, o.Current)
		if e != nil {
			return e
		}
		if original.Input.Captured == nil || original.Input.Conditional != nil || original.Tools != tools {
			return disc.ErrRemoteUnqualified
		}
		var until int64
		if e = tx.QueryRowContext(check, `SELECT expires_ms FROM playback_disc_lifecycle WHERE snapshot_id=? AND state='active' AND sealed=1`, original.ID).Scan(&until); e != nil {
			return e
		}
		if deadline, finite := ctx.Deadline(); finite && deadline.UnixMilli() < until {
			until = deadline.UnixMilli()
		}
		pin, e = o.Store.PinTx(check, tx, o.Key, o.Snapshot, identity.Token(), time.UnixMilli(until), o.Current)
		return e
	})
	if err != nil {
		return nil, err
	}
	life, cancel := context.WithDeadline(ctx, pin.ExpiresAt)
	var releaseOnce sync.Once
	var releaseErr error
	releasePin := func() error {
		releaseOnce.Do(func() {
			cancel()
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer stop()
			releaseErr = dbwork.WithWriteTxContext(cleanup, o.DB, dbwork.ClassMaintenance, func(check context.Context, tx *sql.Tx) error {
				return o.Store.ReleasePinTx(check, tx, pin)
			})
		})
		return releaseErr
	}
	good := false
	defer func() {
		if !good {
			err = errors.Join(err, releasePin())
		}
	}()
	var retired atomic.Bool
	current := func(check context.Context) (checkErr error) {
		defer func() {
			if checkErr != nil {
				retired.Store(true)
				cancel()
			}
		}()
		if retired.Load() || life.Err() != nil {
			return playbackdisc.ErrCurrent
		}
		validate := func(check context.Context, tx *sql.Tx) error {
			actual, e := o.Store.ResolvePinTx(check, tx, pin, o.Current)
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(actual, original) {
				return playbackdisc.ErrCurrent
			}
			return life.Err()
		}
		if tx := dbwork.Snapshot(check); tx != nil {
			return validate(check, tx)
		}
		view, e := dbwork.BeginSnapshot(check, o.DB)
		if e != nil {
			return e
		}
		defer view.Rollback()
		return validate(view.Context(), view.Tx())
	}
	if err = current(life); err != nil {
		return nil, err
	}
	reader, err := o.Captures.Open(life, original.Input.Captured.Receipt)
	if err != nil {
		return nil, err
	}
	readerOwned := true
	defer func() {
		if readerOwned {
			reader.Close()
		}
	}()
	title, err := measuredDiscTitle(original.Disc.Structure, original.Disc.SelectedTitleKey)
	if err != nil {
		return nil, err
	}
	input, err := capturedDiscInput(life, o.Tool, reader, original.Disc.Structure, title, current)
	if err != nil {
		return nil, err
	}
	readerOwned = false
	inputOwned := true
	defer func() {
		if inputOwned {
			input.Close()
		}
	}()
	after, err := o.Tools(life)
	if err != nil {
		return nil, err
	}
	if after != tools {
		return nil, ErrStaleOffer
	}
	if err = input.Validate(life); err != nil {
		return nil, err
	}
	// Revalidate the original tool closure around each new helper/seek selection,
	// outside SQL. Authority polling itself remains DB/source checking rather
	// than repeatedly hashing the server binaries. A changed closure retires this
	// reader; restoring old code cannot revive it.
	selectInput := input.InputAt
	input.InputAt = func(check context.Context, ms int64) ([]string, string, int64, error) {
		if dbwork.Snapshot(check) != nil {
			return nil, "", 0, disc.ErrProtocol
		}
		witness := func() error {
			actual, e := o.Tools(check)
			if e == nil && actual != tools {
				e = ErrStaleOffer
			}
			if e != nil {
				retired.Store(true)
				cancel()
			}
			return e
		}
		if e := witness(); e != nil {
			return nil, "", 0, e
		}
		options, address, offset, e := selectInput(check, ms)
		if e != nil {
			return nil, "", 0, e
		}
		if e = witness(); e != nil {
			return nil, "", 0, e
		}
		return options, address, offset, nil
	}
	// The original wrapper owns File, source lease and capture pin together.
	// Release ONLY this additional presentation reference after closing it.
	closeSource := input.ReleaseSource
	input.ReleaseSource = func() error {
		err := closeSource()
		return errors.Join(err, releasePin())
	}
	inputOwned, good = false, true
	return input, nil
}
