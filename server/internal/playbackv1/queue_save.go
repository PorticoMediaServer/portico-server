package playbackv1

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// NEW-37: a queue longer than one synchronous playlist write is saved as a
// durable copy. The request creates the (empty) playlist under its
// Idempotency-Key and records a queue_v1_saves row; the queue builder appends
// the entries in play order, one window per short write, and a replay of the
// same key reports progress. A copy never holds the write gate for more than a
// window, reads the queue a few thousand entries per read snapshot, and never
// keeps the queue in memory. If the queue changes before the copy finishes, the
// copy stops ("queue_changed"): the entries copied so far stay, and the client
// is told, rather than the rest being read from a different order.

// saveStepEntries is how much of a queue one read snapshot copies before the
// builder looks at other work.
const saveStepEntries = 25 * WindowMax

// saveInterruptedAfter is how long a save row may wait for its playlist before
// it is taken as a request that died between recording it and creating the
// playlist.
const saveInterruptedAfter = 10 * time.Minute

// saveRetention keeps a finished save's status readable by a replay for the
// same 30 days as every other receipt.
const saveRetention = 30 * 24 * time.Hour

// Save states on the wire.
const (
	SaveStateSaved  = "saved"
	SaveStateSaving = "saving"
	SaveStateFailed = "failed"
)

type saveRow struct {
	queue, order, playlist, caller, state, code string
	total, next, saved                          int64
}

func (s *Service) loadSave(ctx context.Context, q querier, profile, key string) (saveRow, error) {
	var r saveRow
	err := q.QueryRowContext(ctx, `SELECT queue_id,queue_order,playlist_id,caller,total,next_ordinal,saved,state,error_code FROM queue_v1_saves WHERE profile_id=? AND key=?`, profile, key).Scan(&r.queue, &r.order, &r.playlist, &r.caller, &r.total, &r.next, &r.saved, &r.state, &r.code)
	return r, err
}

// saveView answers for a save row: the playlist, how many entries are in it so
// far, and the state.
func (s *Service) saveView(ctx context.Context, r saveRow) SavedPlaylist {
	out := SavedPlaylist{PlaylistID: r.playlist, Entries: int(r.saved), Total: r.total, State: SaveStateSaving}
	switch r.state {
	case "saved":
		out.State = SaveStateSaved
	case "failed":
		out.State, out.ErrorCode = SaveStateFailed, r.code
	}
	var revision int64
	if r.playlist != "" && s.DB.QueryRowContext(ctx, `SELECT revision FROM catalog_playlists WHERE token=?`, r.playlist).Scan(&revision) == nil {
		out.Revision = strconv.FormatInt(revision, 10)
	}
	return out
}

// startLargeSave records the copy, creates the playlist under the key (the
// catalog's own create and receipt), links them and wakes the builder.
func (s *Service) startLargeSave(ctx context.Context, p identity.Principal, queue, order string, total int64, key string, req SaveAsPlaylistRequest) (SavedPlaylist, error) {
	if s.CreatePlaylist == nil {
		return SavedPlaylist{}, ErrUnsupportedSelector
	}
	profile := identity.PersonalKey(p.Viewer)
	caller, _ := json.Marshal(buildCaller{Viewer: p.Viewer, Epoch: p.Epoch})
	now := s.now().UnixMilli()
	if _, err := dbwork.ExecWrite(ctx, s.DB, dbwork.ClassInteractive, `INSERT INTO queue_v1_saves(profile_id,key,queue_id,queue_order,caller,total,state,created_ms,updated_ms) VALUES(?,?,?,?,?,?,'building',?,?) ON CONFLICT(profile_id,key) DO NOTHING`, profile, key, queue, order, string(caller), total, now, now); err != nil {
		return SavedPlaylist{}, err
	}
	return s.linkSave(ctx, p, key, req)
}

// linkSave creates (or, on a replay, finds) the key's playlist and links it to
// the save row, then wakes the builder.
func (s *Service) linkSave(ctx context.Context, p identity.Principal, key string, req SaveAsPlaylistRequest) (SavedPlaylist, error) {
	profile := identity.PersonalKey(p.Viewer)
	id, _, err := s.CreatePlaylist(ctx, p, key, req.Name, req.Summary, nil)
	if err != nil {
		return SavedPlaylist{}, err
	}
	if _, err = dbwork.ExecWrite(ctx, s.DB, dbwork.ClassInteractive, `UPDATE queue_v1_saves SET playlist_id=?,updated_ms=? WHERE profile_id=? AND key=? AND playlist_id=''`, id, s.now().UnixMilli(), profile, key); err != nil {
		return SavedPlaylist{}, err
	}
	s.build.kick(s)
	r, err := s.loadSave(ctx, s.DB, profile, key)
	if err != nil {
		return SavedPlaylist{}, err
	}
	return s.saveView(ctx, r), nil
}

// nextSave is a save the builder can continue, or false.
func (s *Service) nextSave() (profile, key string, ok bool) {
	err := s.DB.QueryRow(`SELECT profile_id,key FROM queue_v1_saves WHERE state='building' AND playlist_id<>'' ORDER BY updated_ms LIMIT 1`).Scan(&profile, &key)
	return profile, key, err == nil
}

// endSave stops a copy with a reason; what it copied stays.
func (s *Service) endSave(ctx context.Context, profile, key, code string) error {
	_, err := dbwork.ExecWrite(ctx, s.DB, dbwork.ClassBackgroundMedia, `UPDATE queue_v1_saves SET state='failed',error_code=?,updated_ms=? WHERE profile_id=? AND key=? AND state='building'`, code, s.now().UnixMilli(), profile, key)
	return err
}

type saveWindow struct {
	from, next int64
	items      []string
}

// continueSave copies up to saveStepEntries more of one save. progressed is
// false when it could do nothing now (a snapshot still building).
func (s *Service) continueSave(ctx context.Context, profile, key string) (progressed bool, err error) {
	r, err := s.loadSave(ctx, s.DB, profile, key)
	if err != nil || r.state != "building" || r.playlist == "" {
		return false, err
	}
	var who buildCaller
	if json.Unmarshal([]byte(r.caller), &who) != nil {
		return true, s.endSave(ctx, profile, key, "queue_changed")
	}
	p := identity.Principal{Viewer: who.Viewer, Epoch: who.Epoch}
	windows, code, err := s.readSaveWindows(ctx, r, p)
	if err != nil {
		return false, err
	}
	if code != "" {
		return true, s.endSave(ctx, profile, key, code)
	}
	if windows == nil {
		return false, nil // a snapshot the queue needs is still building
	}
	if len(windows) == 0 {
		// Nothing left to read (a step that wrote the last window but not
		// the state): finish rather than pick the same row again.
		_, err = dbwork.ExecWrite(ctx, s.DB, dbwork.ClassBackgroundMedia, `UPDATE queue_v1_saves SET state='saved',updated_ms=? WHERE profile_id=? AND key=? AND state='building'`, s.now().UnixMilli(), profile, key)
		return true, err
	}
	actor := catalog.ResourceActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}
	for _, w := range windows {
		stop := false
		err = dbwork.WithWriteTx(ctx, s.DB, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			var state string
			var next int64
			if err := tx.QueryRowContext(ctx, `SELECT state,next_ordinal FROM queue_v1_saves WHERE profile_id=? AND key=?`, profile, key).Scan(&state, &next); err != nil {
				return err
			}
			if state != "building" || next != w.from {
				stop = true // another step got here first, or the copy ended
				return nil
			}
			_, appended, err := catalog.AppendPlaylistItemsTx(ctx, tx, actor, r.playlist, w.items)
			if errors.Is(err, sql.ErrNoRows) {
				stop = true
				_, err = tx.ExecContext(ctx, `UPDATE queue_v1_saves SET state='failed',error_code='playlist_deleted',updated_ms=? WHERE profile_id=? AND key=?`, s.now().UnixMilli(), profile, key)
				return err
			}
			if err != nil {
				return err
			}
			state = "building"
			if w.next >= r.total {
				state = "saved"
			}
			_, err = tx.ExecContext(ctx, `UPDATE queue_v1_saves SET next_ordinal=?,saved=saved+?,state=?,updated_ms=? WHERE profile_id=? AND key=?`, w.next, appended, state, s.now().UnixMilli(), profile, key)
			return err
		})
		if err != nil || stop {
			return true, err
		}
		if !dbwork.Yield(ctx) {
			return true, ctx.Err()
		}
	}
	return true, nil
}

// readSaveWindows reads the next saveStepEntries of the queue, in play order,
// through the saver's own item fence, in one read snapshot. A queue that has
// changed since the save started (or is gone) answers code "queue_changed";
// one whose saver can no longer read it, "authority_revoked". nil windows and
// no code: a snapshot is still building.
func (s *Service) readSaveWindows(ctx context.Context, r saveRow, p identity.Principal) (windows []saveWindow, code string, err error) {
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return nil, "", err
	}
	defer done()
	q, err := loadQueue(ctx, tx, r.queue)
	if errors.Is(err, ErrNotFound) {
		return nil, "queue_changed", nil
	}
	if err != nil {
		return nil, "", err
	}
	l, err := s.readLayout(ctx, tx, q)
	if err != nil {
		return nil, "", err
	}
	// Playing the queue moves its current position and revision; only a
	// change to its order stops the copy.
	if l.orderDigest() != r.order || l.total() != r.total {
		return nil, "queue_changed", nil
	}
	windows = []saveWindow{}
	for from := r.next; from < r.total && from < r.next+saveStepEntries; from += WindowMax {
		n := min(int64(WindowMax), r.total-from)
		entries, err := s.window(ctx, l, p, from, n)
		if errors.Is(err, identity.ErrUnauthorized) || errors.Is(err, identity.ErrNotVisible) {
			return nil, "authority_revoked", nil
		}
		if err != nil {
			return nil, "", err
		}
		w := saveWindow{from: from, next: from + n, items: make([]string, 0, len(entries))}
		for _, e := range entries {
			if e.Kind == "pending" {
				return nil, "", nil
			}
			// An entry the saver can no longer play is left out (SEC-02),
			// as in the synchronous save.
			if e.Available && e.ItemID != "" {
				w.items = append(w.items, e.ItemID)
			}
		}
		windows = append(windows, w)
	}
	return windows, "", nil
}

// sweepSaves ends saves whose request died before the playlist existed and
// forgets finished saves after saveRetention.
func (s *Service) sweepSaves(ctx context.Context) {
	now := s.now()
	if _, err := dbwork.ExecWrite(ctx, s.DB, dbwork.ClassBackgroundMedia, `UPDATE queue_v1_saves SET state='failed',error_code='interrupted',updated_ms=? WHERE rowid IN(SELECT rowid FROM queue_v1_saves WHERE state='building' AND playlist_id='' AND updated_ms<? LIMIT 64)`, now.UnixMilli(), now.Add(-saveInterruptedAfter).UnixMilli()); err != nil && ctx.Err() == nil {
		log.Printf("Queue saves could not be swept: %v", err)
		return
	}
	for _, state := range []string{"saved", "failed"} {
		if _, err := dbwork.ExecWrite(ctx, s.DB, dbwork.ClassBackgroundMedia, `DELETE FROM queue_v1_saves WHERE rowid IN(SELECT rowid FROM queue_v1_saves WHERE state=? AND updated_ms<? LIMIT 256)`, state, now.Add(-saveRetention).UnixMilli()); err != nil {
			return
		}
	}
}

// orderDigest names the queue's play order: its segments, the removals inside
// them and its shuffle, but not its current position or repeat mode, which
// playing and settings change without reordering anything.
func (l *layout) orderDigest() string {
	h := sha256.New()
	for _, g := range l.segments {
		fmt.Fprintf(h, "g%s|%s|%s|%d|%d|%d\n", g.id, g.snapshot, g.sortKey, g.first, g.count, g.removed)
		for _, ordinal := range l.removals[g.snapshot] {
			fmt.Fprintf(h, "r%d\n", ordinal)
		}
	}
	fmt.Fprintf(h, "s%t|%d|%d|%d|%d\n", l.q.shuffle, l.q.seed, l.q.lap, l.q.domain, l.q.first)
	return hex.EncodeToString(h.Sum(nil))
}
