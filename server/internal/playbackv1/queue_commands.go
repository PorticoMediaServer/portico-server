package playbackv1

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

func init() {
	startTargets["queue"] = (*Service).startQueueEntry
}

// queueWrite runs one queue mutation in a gated transaction with the layout
// read inside it; mutate returns whether it changed the queue. The revision,
// the event and the updated time follow automatically.
func (s *Service) queueWrite(ctx context.Context, q queueRow, mutate func(ctx context.Context, tx *sql.Tx, l *layout) (bool, error)) (*layout, error) {
	var out *layout
	var canceled []string
	err := dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassInteractive, func(ctx context.Context, tx *sql.Tx) error {
		fresh, err := loadQueue(ctx, tx, q.id)
		if err != nil {
			return err
		}
		if fresh.revision != q.revision {
			return ErrRevision
		}
		l, err := s.readLayout(ctx, tx, fresh)
		if err != nil {
			return err
		}
		changed, err := mutate(ctx, tx, l)
		if err != nil {
			return err
		}
		if changed {
			// §18.2: any other queue command cancels the queue's prepared next.
			if canceled, err = cancelPreparedTx(ctx, tx, "queue_id", l.q.id, "queue_changed"); err != nil {
				return err
			}
			now := s.now()
			l.q.revision++
			l.q.updated = now.UnixMilli()
			if _, err = tx.ExecContext(ctx, `UPDATE queues_v1 SET revision=?,current_position=?,repeat=?,shuffle_on=?,shuffle_seed=?,shuffle_lap=?,shuffle_domain=?,shuffle_first=?,session_id=?,updated_ms=? WHERE id=?`,
				l.q.revision, l.q.current, l.q.repeat, l.q.shuffle, l.q.seed, l.q.lap, l.q.domain, l.q.first, l.q.session, l.q.updated, l.q.id); err != nil {
				return err
			}
			if err = compactQueue(ctx, tx, l.q.id); err != nil {
				return err
			}
			// The layout changed underneath: re-read it for the answer and the event.
			if l, err = s.readLayout(ctx, tx, l.q); err != nil {
				return err
			}
			if err = s.queueEventsTx(ctx, tx, l, now); err != nil {
				return err
			}
		}
		out = l
		return nil
	})
	if err == nil {
		s.hub.wake()
		s.stopMedia(ctx, canceled)
	}
	return out, err
}

// MaxQueueSegments and MaxQueueRemovals bound a queue's structure, so the
// layout every request reads stays bounded however it's edited (P15).
const MaxQueueSegments = 1000

// MaxQueueRemovals is the tombstone bound past which segments are compacted.
var MaxQueueRemovals = 10_000

// MaxQueueRemovalsForTest lowers the tombstone bound; the returned func restores it.
func MaxQueueRemovalsForTest(n int) func() {
	old := MaxQueueRemovals
	MaxQueueRemovals = n
	return func() { MaxQueueRemovals = old }
}

// compactQueue keeps a queue's structure small without changing any entry id
// or position: segments with nothing left are dropped with their tombstones
// (and a snapshot nothing references any more), and neighbouring segments
// that are contiguous ranges of one snapshot are merged back into one. Then
// the structural bounds are checked; past them the command is refused.
func compactQueue(ctx context.Context, tx *sql.Tx, queue string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM queue_v1_segments WHERE queue_id=? AND count=removed`, queue); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,snapshot_id,first_ordinal,count,removed FROM queue_v1_segments WHERE queue_id=? ORDER BY sort_key`, queue)
	if err != nil {
		return err
	}
	type run struct {
		id, snapshot          string
		first, count, removed int64
	}
	var runs []run
	for rows.Next() {
		var r run
		if err = rows.Scan(&r.id, &r.snapshot, &r.first, &r.count, &r.removed); err != nil {
			rows.Close()
			return err
		}
		runs = append(runs, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for i := 0; i+1 < len(runs); {
		a, b := &runs[i], runs[i+1]
		if a.snapshot != b.snapshot || a.first+a.count != b.first {
			i++
			continue
		}
		a.count, a.removed = a.count+b.count, a.removed+b.removed
		if _, err = tx.ExecContext(ctx, `UPDATE queue_v1_segments SET count=?,removed=? WHERE id=?`, a.count, a.removed, a.id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM queue_v1_segments WHERE id=?`, b.id); err != nil {
			return err
		}
		runs = append(runs[:i+1], runs[i+2:]...)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM queue_v1_removals WHERE snapshot_id IN (SELECT id FROM queue_v1_snapshots WHERE queue_id=?) AND NOT EXISTS(SELECT 1 FROM queue_v1_segments g WHERE g.snapshot_id=queue_v1_removals.snapshot_id AND queue_v1_removals.ordinal>=g.first_ordinal AND queue_v1_removals.ordinal<g.first_ordinal+g.count)`, queue); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM queue_v1_snapshots WHERE queue_id=? AND NOT EXISTS(SELECT 1 FROM queue_v1_segments g WHERE g.snapshot_id=queue_v1_snapshots.id)`, queue); err != nil {
		return err
	}
	removals, err := countRemovals(ctx, tx, queue)
	if err != nil {
		return err
	}
	// Past the tombstone bound, the most-edited segments are rewritten into
	// fresh snapshots of just their live keys (P28): positions and order stay,
	// the rewritten entries get new ids, and the edit goes ahead.
	for removals > MaxQueueRemovals {
		var id string
		err = tx.QueryRowContext(ctx, `SELECT g.id FROM queue_v1_segments g JOIN queue_v1_snapshots s ON s.id=g.snapshot_id WHERE g.queue_id=? AND g.removed>0 AND s.state='ready' AND g.count-g.removed<=? ORDER BY g.removed DESC LIMIT 1`, queue, rewriteLimit).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		if err = rewriteSegment(ctx, tx, id); err != nil {
			return err
		}
		if removals, err = countRemovals(ctx, tx, queue); err != nil {
			return err
		}
	}
	if len(runs) > MaxQueueSegments || removals > MaxQueueRemovals {
		return ErrQueueTooLarge
	}
	return nil
}

// rewriteLimit bounds the live keys one tombstone compaction copies.
const rewriteLimit = 250_000

func countRemovals(ctx context.Context, tx *sql.Tx, queue string) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM queue_v1_removals r JOIN queue_v1_snapshots s ON s.id=r.snapshot_id WHERE s.queue_id=?`, queue).Scan(&n)
	return n, err
}

// rewriteSegment copies a segment's live keys, in order, into a new snapshot
// and points the segment at it, dropping the tombstones in its range and the
// old snapshot when nothing else uses it.
func rewriteSegment(ctx context.Context, tx *sql.Tx, segment string) error {
	var queue, old, kind, selector, label string
	var first, count int64
	if err := tx.QueryRowContext(ctx, `SELECT g.queue_id,g.snapshot_id,g.first_ordinal,g.count,s.kind,s.selector,s.label FROM queue_v1_segments g JOIN queue_v1_snapshots s ON s.id=g.snapshot_id WHERE g.id=?`, segment).Scan(&queue, &old, &first, &count, &kind, &selector, &label); err != nil {
		return err
	}
	removed := map[int64]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT ordinal FROM queue_v1_removals WHERE snapshot_id=? AND ordinal>=? AND ordinal<?`, old, first, first+count)
	if err != nil {
		return err
	}
	for rows.Next() {
		var o int64
		if err = rows.Scan(&o); err != nil {
			rows.Close()
			return err
		}
		removed[o] = true
	}
	rows.Close()
	chunks, err := tx.QueryContext(ctx, `SELECT chunk_no,keys FROM queue_v1_chunks WHERE snapshot_id=? AND chunk_no>=? AND chunk_no<=? ORDER BY chunk_no`, old, first/chunkKeys, (first+count-1)/chunkKeys)
	if err != nil {
		return err
	}
	fresh := newShortID("s")
	out := make([]string, 0, chunkKeys)
	var n, no int64
	flush := func() error {
		if len(out) == 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO queue_v1_chunks(snapshot_id,chunk_no,keys) VALUES(?,?,?)`, fresh, no, encodeChunk(out))
		no++
		out = out[:0]
		return err
	}
	type chunkBlob struct {
		no   int64
		keys []byte
	}
	blobs := []chunkBlob{}
	for chunks.Next() {
		var b chunkBlob
		if err = chunks.Scan(&b.no, &b.keys); err != nil {
			chunks.Close()
			return err
		}
		blobs = append(blobs, b)
	}
	chunks.Close()
	if _, err = tx.ExecContext(ctx, `INSERT INTO queue_v1_snapshots(id,queue_id,kind,selector,label,count,width,built) VALUES(?,?,?,?,?,0,0,0)`, fresh, queue, kind, selector, label); err != nil {
		return err
	}
	for _, b := range blobs {
		for i := 0; i < chunkKeys; i++ {
			ordinal := b.no*chunkKeys + int64(i)
			if ordinal < first || ordinal >= first+count || removed[ordinal] {
				continue
			}
			key, ok := chunkKey(b.keys, i)
			if !ok {
				break
			}
			out = append(out, key)
			n++
			if len(out) == chunkKeys {
				if err = flush(); err != nil {
					return err
				}
			}
		}
	}
	if err = flush(); err != nil {
		return err
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE queue_v1_snapshots SET count=?,built=? WHERE id=?`, []any{n, n, fresh}},
		{`UPDATE queue_v1_segments SET snapshot_id=?,first_ordinal=0,count=?,removed=0 WHERE id=?`, []any{fresh, n, segment}},
		{`DELETE FROM queue_v1_removals WHERE snapshot_id=? AND ordinal>=? AND ordinal<?`, []any{old, first, first + count}},
		{`DELETE FROM queue_v1_snapshots WHERE id=? AND NOT EXISTS(SELECT 1 FROM queue_v1_segments WHERE snapshot_id=?)`, []any{old, old}},
	} {
		if _, err = tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return err
		}
	}
	return nil
}

// queueFor loads a queue the caller may use and checks If-Match (428/412 with
// the current header, spec §17.3).
func (s *Service) queueFor(ctx context.Context, c Caller, id, ifMatch string, needMatch bool) (queueRow, error) {
	s.resumeOnce()
	q, err := loadQueue(ctx, s.DB, id)
	if err != nil {
		return q, err
	}
	if !ownsQueue(c, q) {
		return q, ErrNotFound
	}
	if !needMatch {
		return q, nil
	}
	if ifMatch == "" {
		return q, ErrRevisionRequired
	}
	if want, ok := ParseRevision(ifMatch); !ok || want != q.revision {
		return q, s.staleQueue(ctx, q)
	}
	return q, nil
}

func (s *Service) staleQueue(ctx context.Context, q queueRow) error {
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return err
	}
	defer done()
	l, err := s.readLayout(ctx, tx, q)
	if err != nil {
		return err
	}
	return &RevisionError{Current: s.header(l), Revision: q.revision}
}

// CreateQueue is POST /v1/queues (spec §8): Play replaces this device's
// current queue; with startPlayback the first session starts in the same
// request (Play/Shuffle is one request).
func (s *Service) CreateQueue(ctx context.Context, c Caller, key string, req CreateQueueRequest) (QueueReply, error) {
	if key == "" {
		return QueueReply{}, ErrKeyRequired
	}
	if !keyPattern.MatchString(key) {
		return QueueReply{}, &FieldError{Path: "Idempotency-Key"}
	}
	if req.Owner != "" && req.Owner != "device" {
		return QueueReply{}, &FieldError{Path: "owner"}
	}
	segments := req.Segments
	if req.Selector != nil {
		if len(segments) > 0 {
			return QueueReply{}, &FieldError{Path: "selector"}
		}
		in := SegmentInput{Source: *req.Selector}
		if req.Shuffle != nil {
			in.Order = &Order{Mode: "shuffle", Seed: req.Shuffle.Seed}
		}
		segments = []SegmentInput{in}
	}
	if len(segments) == 0 || len(segments) > 16 {
		return QueueReply{}, &FieldError{Path: "segments"}
	}
	switch req.Repeat {
	case "", "off", "one", "all":
	default:
		return QueueReply{}, &FieldError{Path: "repeat"}
	}
	if sp := req.StartPlayback; sp != nil {
		if sp.State != "" && sp.State != "playing" && sp.State != "paused" {
			return QueueReply{}, &FieldError{Path: "startPlayback.state"}
		}
		if sp.Quality != nil {
			if err := sp.Quality.Validate("startPlayback.quality"); err != nil {
				return QueueReply{}, err
			}
		}
	}
	want := digest(req)
	// A replay is answered by its receipt (review P18).
	if reply, ok, err := s.createReplay(ctx, c, key, want, req); ok || err != nil {
		return reply, err
	}
	now := s.now()
	q := queueRow{id: newID("q_"), ownerKind: "device", ownerID: c.DeviceID, authority: c.Principal.Authority, account: c.Principal.AccountID, profile: c.Principal.ProfileID,
		repeat: "off", revision: 1, created: now.UnixMilli(), updated: now.UnixMilli()}
	if req.Repeat != "" {
		q.repeat = req.Repeat
	}
	// Snapshots are scanned before the queue's write, in read snapshots (P16).
	// A large one writes its batches as it goes, and its first batch writes
	// this queue's row under a staging owner that nothing looks up, so the
	// device's current queue stays as it is until this command commits.
	s.resumeOnce()
	s.sweepQueues()
	staging := "~" + c.DeviceID
	insertQueue := `INSERT INTO queues_v1(id,owner_kind,owner_id,authority,account_id,profile_id,revision,repeat,created_ms,updated_ms) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET owner_id=excluded.owner_id`
	stage := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, insertQueue, q.id, q.ownerKind, staging, q.authority, q.account, q.profile, q.revision, q.repeat, q.created, q.updated)
		return err
	}
	snaps := make([]snapshot, 0, len(segments))
	staged := false
	discard := func() {
		if staged {
			_, _ = dbwork.ExecWrite(context.WithoutCancel(ctx), s.DB, dbwork.ClassBackgroundMedia, `DELETE FROM queues_v1 WHERE id=? AND owner_id=?`, q.id, staging)
		}
	}
	var planned int64
	// The request scans a bounded amount (P16); the rest is built in the background.
	budget := s.requestBudget()
	for i, in := range segments {
		if in.Order != nil && in.Order.Mode != "source" && in.Order.Mode != "shuffle" {
			return QueueReply{}, &FieldError{Path: "segments[" + strconv.Itoa(i) + "].order.mode"}
		}
		snap, err := s.buildSnapshot(ctx, c.Principal, q.id, in, "segments["+strconv.Itoa(i)+"]", s.queueKeyLimit()-planned, stage, budget)
		staged = staged || snap.stored
		if err != nil {
			discard()
			return QueueReply{}, err
		}
		planned += snap.count
		snaps = append(snaps, snap)
	}
	if err := s.planBuilding(ctx, c, segments, snaps); err != nil {
		discard()
		return QueueReply{}, err
	}
	var l *layout
	err := dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassInteractive, func(ctx context.Context, tx *sql.Tx) error {
		// The key is this command's: a concurrent retry that committed first
		// wins, and this one answers as its replay (review P18).
		claimed, err := tx.ExecContext(ctx, `INSERT INTO queue_v1_create_receipts(device_id,create_key,digest,queue_id,created_ms) VALUES(?,?,?,?,?) ON CONFLICT(device_id,create_key) DO NOTHING`, c.DeviceID, key, want, q.id, q.created)
		if err != nil {
			return err
		}
		if n, err := claimed.RowsAffected(); err != nil || n == 0 {
			return errCreateRaced
		}
		// Play replaces the device's current queue (ARCH-MEDIA-06). The old one
		// is retired, not deleted here: its keys (up to millions) are removed
		// in batches by the sweep, off this write (review P19).
		if _, err := tx.ExecContext(ctx, `UPDATE queues_v1 SET owner_id='!'||owner_id WHERE owner_kind='device' AND owner_id=?`, c.DeviceID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, insertQueue, q.id, q.ownerKind, q.ownerID, q.authority, q.account, q.profile, q.revision, q.repeat, q.created, q.updated); err != nil {
			return err
		}
		sortKey := ""
		var base int64
		anchored := false
		var order *Order
		for i, in := range segments {
			snap := snaps[i]
			if snap.count == 0 {
				continue
			}
			if err := storeSnapshot(ctx, tx, q.id, &snap); err != nil {
				return err
			}
			sortKey = sortKeyBetween(sortKey, "")
			if _, err = tx.ExecContext(ctx, `INSERT INTO queue_v1_segments(id,queue_id,sort_key,snapshot_id,first_ordinal,count) VALUES(?,?,?,?,0,?)`, newShortID("g"), q.id, sortKey, snap.id, snap.size()); err != nil {
				return err
			}
			if !anchored && snap.anchor >= 0 {
				q.current, anchored = base+snap.anchor, true
			}
			// A shuffle waiting for its snapshot's size is set up when it's built.
			if i == 0 && in.Order != nil && in.Order.Mode == "shuffle" && snap.start == "" {
				order = in.Order
			}
			base += snap.size()
		}
		if base == 0 {
			return ErrNotFound // Nothing to play: an empty or inaccessible selection.
		}
		if order != nil {
			seed, err := seedFrom(order.Seed)
			if err != nil {
				return err
			}
			q.shuffle, q.seed, q.domain = true, seed, base
			if anchored {
				q.first = q.current
			} else if sh, err := NewShuffle(uint64(base), uint32(seed), 1); err == nil {
				// Shuffle-play from the start picks position 0 from the seed (ARCH-MEDIA-04).
				first, _ := sh.At(0)
				q.first = int64(first)
			}
			q.current = 0
		}
		if _, err := tx.ExecContext(ctx, `UPDATE queues_v1 SET current_position=?,shuffle_on=?,shuffle_seed=?,shuffle_domain=?,shuffle_first=? WHERE id=?`, q.current, q.shuffle, q.seed, q.domain, q.first, q.id); err != nil {
			return err
		}
		if l, err = s.readLayout(ctx, tx, q); err != nil {
			return err
		}
		return s.queueEventsTx(ctx, tx, l, now)
	})
	if errors.Is(err, errCreateRaced) {
		discard()
		reply, _, err := s.createReplay(ctx, c, key, want, req)
		return reply, err
	}
	if err != nil {
		discard()
		return QueueReply{}, err
	}
	s.hub.wake()
	s.sweepQueues() // the queue this one replaced
	for _, snap := range snaps {
		if snap.partial {
			s.build.kick(s)
			break
		}
	}
	return s.finishCreate(ctx, c, key, req, q)
}

// errCreateRaced: another request with this create's key committed first.
var errCreateRaced = errors.New("queue create raced")

// createReplay answers a create whose key this device has used (review P18):
// with its kept response; a different body is a mismatch; a create that
// committed but hasn't kept its response yet (in flight, or cut off after its
// commit) is finished here, the same way, which is idempotent. ok is false
// when the key is new.
func (s *Service) createReplay(ctx context.Context, c Caller, key, want string, req CreateQueueRequest) (QueueReply, bool, error) {
	var digest, queue, response string
	err := s.DB.QueryRowContext(ctx, `SELECT digest,queue_id,response FROM queue_v1_create_receipts WHERE device_id=? AND create_key=?`, c.DeviceID, key).Scan(&digest, &queue, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueReply{}, false, nil
	}
	if err != nil {
		return QueueReply{}, true, err
	}
	if digest != want {
		return QueueReply{}, true, ErrIdempotencyMismatch
	}
	var reply QueueReply
	if response != "" && json.Unmarshal([]byte(response), &reply) == nil {
		return reply, true, nil
	}
	q, err := loadQueue(ctx, s.DB, queue)
	if err != nil || !ownsQueue(c, q) {
		return QueueReply{}, true, ErrNotFound // replaced before its answer was kept
	}
	reply, err = s.finishCreate(ctx, c, key, req, q)
	return reply, true, err
}

// finishCreate is everything after a create's commit, and safe to repeat:
// the first playback (started under a key derived from the create's, so a
// repeat is the same session), the header and window, and the kept response.
func (s *Service) finishCreate(ctx context.Context, c Caller, key string, req CreateQueueRequest, q queueRow) (QueueReply, error) {
	var reply QueueReply
	if err := s.withLayout(ctx, q, func(l *layout) error { reply.Queue = s.header(l); return nil }); err != nil {
		return QueueReply{}, err
	}
	if sp := req.StartPlayback; sp != nil && reply.Queue.Current != nil {
		start := StartRequest{Queue: &QueueRef{QueueID: q.id, EntryID: reply.Queue.Current.EntryID}, State: sp.State, Quality: sp.Quality}
		session, _, err := s.Start(ctx, c, playKey(key), start)
		if err != nil {
			return QueueReply{}, err
		}
		reply.Session = &session
		if fresh, err := loadQueue(ctx, s.DB, q.id); err == nil {
			q = fresh
		}
	}
	if err := s.withLayout(ctx, q, func(l *layout) error {
		reply.Queue = s.header(l)
		from := max(0, l.q.current-20)
		var err error
		reply.Window, err = s.window(ctx, l, c.Principal, from, l.q.current-from+51)
		return err
	}); err != nil {
		return QueueReply{}, err
	}
	s.withNext(ctx, c, &reply.Queue, q)
	raw, _ := json.Marshal(reply)
	_, _ = dbwork.ExecWrite(ctx, s.DB, dbwork.ClassInteractive, `UPDATE queue_v1_create_receipts SET response=? WHERE device_id=? AND create_key=? AND response=''`, string(raw), c.DeviceID, key)
	return reply, nil
}

// playKey is the key a create's first playback starts under: derived from
// the create's key, always a valid key whatever that key's length (review
// P18: key+"-play" overflowed a 124-128 character key).
func playKey(key string) string {
	sum := sha256.Sum256([]byte("queue-play:" + key))
	return "qp" + hex.EncodeToString(sum[:20])
}

// withLayout reads a queue's layout in a read snapshot.
func (s *Service) withLayout(ctx context.Context, q queueRow, fn func(*layout) error) error {
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return err
	}
	defer done()
	l, err := s.readLayout(ctx, tx, q)
	if err != nil {
		return err
	}
	return fn(l)
}

// Queue is GET /v1/queues/{id}: the header only (constant size).
func (s *Service) Queue(ctx context.Context, c Caller, id string) (QueueView, error) {
	q, err := s.queueFor(ctx, c, id, "", false)
	if err != nil {
		return QueueView{}, err
	}
	var out QueueView
	err = s.withLayout(ctx, q, func(l *layout) error { out = s.header(l); return nil })
	if err == nil {
		s.withNext(ctx, c, &out, q)
	}
	return out, err
}

// Window is GET /v1/queues/{id}/window: entries around the current one.
func (s *Service) Window(ctx context.Context, c Caller, id string, before, after int64) (EntryPage, error) {
	if before < 0 || after < 0 {
		return EntryPage{}, &FieldError{Path: "before"}
	}
	before = min(before, WindowMax)
	after = min(after, WindowMax-before)
	q, err := s.queueFor(ctx, c, id, "", false)
	if err != nil {
		return EntryPage{}, err
	}
	var out EntryPage
	err = s.withLayout(ctx, q, func(l *layout) error {
		from := max(0, l.q.current-before)
		items, err := s.window(ctx, l, c.Principal, from, l.q.current-from+after+1)
		out = EntryPage{Items: items, Page: Page{Limit: int(l.q.current - from + after + 1), Total: l.total(), Start: from, Revision: strconv.FormatInt(l.q.revision, 10)}}
		return err
	})
	return out, err
}

// Entries is GET /v1/queues/{id}/entries: a page from a position (≤ 200).
func (s *Service) Entries(ctx context.Context, c Caller, id string, from int64, limit int) (EntryPage, error) {
	if from < 0 {
		return EntryPage{}, &FieldError{Path: "from"}
	}
	if limit < 1 || limit > WindowMax {
		return EntryPage{}, &FieldError{Path: "limit"}
	}
	q, err := s.queueFor(ctx, c, id, "", false)
	if err != nil {
		return EntryPage{}, err
	}
	var out EntryPage
	err = s.withLayout(ctx, q, func(l *layout) error {
		items, err := s.window(ctx, l, c.Principal, from, int64(limit))
		total := l.total()
		out = EntryPage{Items: items, Page: Page{Limit: limit, Total: total, Start: from, Revision: strconv.FormatInt(l.q.revision, 10)}}
		if next := from + int64(len(items)); next < total {
			out.Page.NextCursor = "from:" + strconv.FormatInt(next, 10)
		}
		return err
	})
	return out, err
}

// reshuffle keeps the current entry first under a new domain after the queue's
// structure changed (an insert inside the shuffled body, a remove or a move).
func (l *layout) reshuffle(srcCurrent int64) {
	if !l.q.shuffle {
		return
	}
	l.q.domain, l.q.first, l.q.current = l.total(), srcCurrent, 0
}

// splitAfter splits the segment holding an entry right after it and returns
// the sort key a new segment takes there.
func (s *Service) splitAfter(ctx context.Context, tx *sql.Tx, l *layout, segIndex int, ordinal int64) (string, error) {
	g := l.segments[segIndex]
	next := ""
	if segIndex+1 < len(l.segments) {
		next = l.segments[segIndex+1].sortKey
	}
	cut := ordinal + 1 - g.first
	if cut >= g.count {
		return sortKeyBetween(g.sortKey, next), nil
	}
	// The tail keeps its entries (same snapshot, same ordinals): only ranges change.
	tailKey := sortKeyBetween(g.sortKey, next)
	var removedHead, removedTail int64
	for _, r := range l.removals[g.snapshot] {
		switch {
		case r >= g.first && r < g.first+cut:
			removedHead++
		case r >= g.first+cut && r < g.first+g.count:
			removedTail++
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_v1_segments SET count=?,removed=? WHERE id=?`, cut, removedHead, g.id); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO queue_v1_segments(id,queue_id,sort_key,snapshot_id,first_ordinal,count,removed) VALUES(?,?,?,?,?,?,?)`, newShortID("g"), l.q.id, tailKey, g.snapshot, g.first+cut, g.count-cut, removedTail); err != nil {
		return "", err
	}
	return sortKeyBetween(g.sortKey, tailKey), nil
}

// currentSource is the source position of the current entry (for keeping it
// current across structural changes).
func (l *layout) currentEntry() (string, bool) {
	if l.waiting() {
		return "", false
	}
	entry, _, err := l.entryAtErr(l.q.current)
	return entry, err == nil || errors.Is(err, ErrQueueBuilding)
}

func (l *layout) restoreCurrent(entry string) {
	if entry == "" {
		return
	}
	if src, _, ok := l.sourceIndexOf(entry); ok {
		if l.q.shuffle {
			l.reshuffle(src)
			return
		}
		l.q.current = src
	}
}

// AddSegment is POST /v1/queues/{id}/segments: Play next, Add to queue, or
// insert after an entry.
func (s *Service) AddSegment(ctx context.Context, c Caller, id, ifMatch string, req AddSegmentRequest) (QueueReply, error) {
	q, err := s.queueFor(ctx, c, id, ifMatch, true)
	if err != nil {
		return QueueReply{}, err
	}
	// Scanned before the queue's write, in a read snapshot (P16), within what
	// the queue has room for.
	room := s.queueKeyLimit()
	if err = s.withLayout(ctx, q, func(l *layout) error { room -= l.total(); return nil }); err != nil {
		return QueueReply{}, err
	}
	if room <= 0 {
		return QueueReply{}, ErrQueueTooLarge
	}
	snap, err := s.buildSnapshot(ctx, c.Principal, q.id, SegmentInput{Source: req.Source, Order: req.Order, Anchor: req.Anchor}, "source", room, nil, s.requestBudget())
	if err != nil {
		s.discardSnapshot(ctx, snap)
		return QueueReply{}, err
	}
	if snap.partial {
		// The rest is built in the background; an added segment grows in place.
		snap.anchorItem = ""
		raw, _ := json.Marshal(buildCaller{Viewer: c.Principal.Viewer, Epoch: c.Principal.Epoch})
		snap.caller = string(raw)
	}
	if snap.count == 0 {
		return QueueReply{}, ErrNotFound
	}
	l, err := s.queueWrite(ctx, q, func(ctx context.Context, tx *sql.Tx, l *layout) (bool, error) {
		current, _ := l.currentEntry()
		if l.total()+snap.count > s.queueKeyLimit() {
			return false, ErrQueueTooLarge
		}
		if err := storeSnapshot(ctx, tx, l.q.id, &snap); err != nil {
			return false, err
		}
		var key string
		where := req.Placement.Where
		if l.q.shuffle {
			// While shuffled, added entries form the unshuffled tail after the
			// shuffled body (spec §17.10).
			where = "end"
		}
		switch where {
		case "end", "":
			last := ""
			if n := len(l.segments); n > 0 {
				last = l.segments[n-1].sortKey
			}
			key = sortKeyBetween(last, "")
		default:
			anchor := current
			if req.Placement.Where == "after" {
				anchor = req.Placement.After
			}
			_, segIndex, ok := l.sourceIndexOf(anchor)
			if !ok {
				return false, &FieldError{Path: "placement.after"}
			}
			_, ordinalText, _ := cutEntry(anchor)
			if key, err = s.splitAfter(ctx, tx, l, segIndex, ordinalText); err != nil {
				return false, err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO queue_v1_segments(id,queue_id,sort_key,snapshot_id,first_ordinal,count) VALUES(?,?,?,?,0,?)`, newShortID("g"), l.q.id, key, snap.id, snap.count); err != nil {
			return false, err
		}
		fresh, err := s.readLayout(ctx, tx, l.q)
		if err != nil {
			return false, err
		}
		if src, _, ok := fresh.sourceIndexOf(current); ok && !l.q.shuffle {
			l.q.current = src
		}
		return true, nil
	})
	if err != nil {
		s.discardSnapshot(ctx, snap)
	} else if snap.partial {
		s.build.kick(s)
	}
	return s.queueReply(l, err, c)
}

func cutEntry(entry string) (string, int64, bool) {
	snap, ordinalText, ok := cutString(entry, '-')
	n, err := strconv.ParseInt(ordinalText, 10, 64)
	return snap, n, ok && err == nil
}

func cutString(s string, sep byte) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func (s *Service) queueReply(l *layout, err error, c Caller) (QueueReply, error) {
	if errors.Is(err, ErrRevision) && l == nil {
		return QueueReply{}, err
	}
	if err != nil {
		return QueueReply{}, err
	}
	return QueueReply{Queue: s.header(l)}, nil
}

// RemoveEntry is DELETE /v1/queues/{id}/entries/{entryId}: a tombstone.
func (s *Service) RemoveEntry(ctx context.Context, c Caller, id, ifMatch, entry string) (QueueReply, error) {
	q, err := s.queueFor(ctx, c, id, ifMatch, true)
	if err != nil {
		return QueueReply{}, err
	}
	l, err := s.queueWrite(ctx, q, func(ctx context.Context, tx *sql.Tx, l *layout) (bool, error) {
		src, segIndex, ok := l.sourceIndexOf(entry)
		if !ok {
			return false, ErrNotFound
		}
		current, _ := l.currentEntry()
		if err := s.tombstone(ctx, tx, l, segIndex, entry); err != nil {
			return false, err
		}
		fresh, err := s.readLayout(ctx, tx, l.q)
		if err != nil {
			return false, err
		}
		if current == entry {
			// The next entry slides into the removed one's place.
			l.q.current = min(src, max(0, fresh.total()-1))
			if l.q.shuffle {
				l.q.domain, l.q.first, l.q.current = fresh.total(), l.q.current, 0
			}
			return true, nil
		}
		if newSrc, _, ok := fresh.sourceIndexOf(current); ok {
			if l.q.shuffle {
				l.q.domain, l.q.first, l.q.current = fresh.total(), newSrc, 0
			} else {
				l.q.current = newSrc
			}
		}
		return true, nil
	})
	return s.queueReply(l, err, c)
}

func (s *Service) tombstone(ctx context.Context, tx *sql.Tx, l *layout, segIndex int, entry string) error {
	snap, ordinal, ok := cutEntry(entry)
	if !ok {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO queue_v1_removals(snapshot_id,ordinal) VALUES(?,?)`, snap, ordinal); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE queue_v1_segments SET removed=removed+1 WHERE id=?`, l.segments[segIndex].id)
	return err
}

// MoveEntry is POST /v1/queues/{id}/entries/{entryId}:move: a tombstone plus a
// one-key segment beside the target (the moved entry gets a new id).
func (s *Service) MoveEntry(ctx context.Context, c Caller, id, ifMatch, entry string, req MoveRequest) (QueueReply, error) {
	if (req.Before == "") == (req.After == "") {
		return QueueReply{}, &FieldError{Path: "before"}
	}
	q, err := s.queueFor(ctx, c, id, ifMatch, true)
	if err != nil {
		return QueueReply{}, err
	}
	l, err := s.queueWrite(ctx, q, func(ctx context.Context, tx *sql.Tx, l *layout) (bool, error) {
		_, segIndex, ok := l.sourceIndexOf(entry)
		target := req.After + req.Before
		_, targetIndex, targetOK := l.sourceIndexOf(target)
		if !ok || !targetOK || target == entry {
			return false, ErrNotFound
		}
		snapID, ordinal, _ := cutEntry(entry)
		item, err := l.key(snapID, ordinal)
		if err != nil {
			return false, err
		}
		current, _ := l.currentEntry()
		if err = s.tombstone(ctx, tx, l, segIndex, entry); err != nil {
			return false, err
		}
		// The split below must see the tombstone.
		if l, err = s.readLayout(ctx, tx, l.q); err != nil {
			return false, err
		}
		if _, targetIndex, targetOK = l.sourceIndexOf(target); !targetOK {
			return false, ErrNotFound
		}
		// Place the key: after the target, or after the entry before it.
		var key string
		_, targetOrdinal, _ := cutEntry(target)
		if req.After != "" {
			if key, err = s.splitAfter(ctx, tx, l, targetIndex, targetOrdinal); err != nil {
				return false, err
			}
		} else {
			g := l.segments[targetIndex]
			if targetOrdinal > g.first {
				if key, err = s.splitAfter(ctx, tx, l, targetIndex, targetOrdinal-1); err != nil {
					return false, err
				}
			} else {
				prev := ""
				if targetIndex > 0 {
					prev = l.segments[targetIndex-1].sortKey
				}
				key = sortKeyBetween(prev, g.sortKey)
			}
		}
		snap := newShortID("s")
		if _, err = tx.ExecContext(ctx, `INSERT INTO queue_v1_snapshots(id,queue_id,kind,selector,label,count,width) VALUES(?,?,'items',?,'1 item',1,0)`, snap, l.q.id, jsonList([]string{item})); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO queue_v1_chunks(snapshot_id,chunk_no,keys) VALUES(?,0,?)`, snap, encodeChunk([]string{item})); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO queue_v1_segments(id,queue_id,sort_key,snapshot_id,first_ordinal,count) VALUES(?,?,?,?,0,1)`, newShortID("g"), l.q.id, key, snap); err != nil {
			return false, err
		}
		fresh, err := s.readLayout(ctx, tx, l.q)
		if err != nil {
			return false, err
		}
		if current == entry {
			current = snap + "-0"
		}
		if src, _, ok := fresh.sourceIndexOf(current); ok {
			if l.q.shuffle {
				l.q.domain, l.q.first, l.q.current = fresh.total(), src, 0
			} else {
				l.q.current = src
			}
		}
		return true, nil
	})
	return s.queueReply(l, err, c)
}

// UpdateQueue is PATCH /v1/queues/{id}: repeat and shuffle (ARCH-MEDIA-04:
// shuffle on makes the current entry position 0; off continues in source order
// from it).
func (s *Service) UpdateQueue(ctx context.Context, c Caller, id, ifMatch string, change QueueChange) (QueueReply, error) {
	switch change.Repeat {
	case "", "off", "one", "all":
	default:
		return QueueReply{}, &FieldError{Path: "repeat"}
	}
	q, err := s.queueFor(ctx, c, id, ifMatch, true)
	if err != nil {
		return QueueReply{}, err
	}
	l, err := s.queueWrite(ctx, q, func(ctx context.Context, tx *sql.Tx, l *layout) (bool, error) {
		changed := false
		if change.Repeat != "" && change.Repeat != l.q.repeat {
			l.q.repeat, changed = change.Repeat, true
		}
		if sh := change.Shuffle; sh != nil {
			// A shuffle's domain is the queue's size: not while a segment is growing.
			if len(l.building) > 0 {
				return false, ErrQueueBuilding
			}
			src := l.orderToSource(l.q.current)
			switch {
			case sh.On:
				seed, err := seedFrom(sh.Seed)
				if err != nil {
					return false, err
				}
				l.q.shuffle, l.q.seed, l.q.lap, l.q.domain, l.q.first, l.q.current = true, seed, 0, l.total(), src, 0
				changed = true
			case l.q.shuffle:
				l.q.shuffle, l.q.current = false, src
				changed = true
			}
		}
		return changed, nil
	})
	return s.queueReply(l, err, c)
}

// Advance is POST /v1/queues/{id}:advance (spec §17.8): completion, next,
// previous or a jump to an entry. With a session playing from this queue, the
// next session starts in the same request, replacing it (slot transfer).
func (s *Service) Advance(ctx context.Context, c Caller, id, ifMatch string, req AdvanceRequest) (QueueReply, error) {
	switch req.Reason {
	case "completion", "next", "previous", "entry":
	default:
		return QueueReply{}, &FieldError{Path: "reason"}
	}
	q, err := s.queueFor(ctx, c, id, ifMatch, true)
	if err != nil {
		return QueueReply{}, err
	}
	l, err := s.queueWrite(ctx, q, func(ctx context.Context, tx *sql.Tx, l *layout) (bool, error) {
		total := l.total()
		switch {
		case req.Reason == "entry":
			p, ok := l.positionOf(req.EntryID)
			if !ok {
				return false, ErrNotFound
			}
			l.q.current = p
		case req.Reason == "previous":
			l.q.current = max(0, l.q.current-1)
		case req.Reason == "completion" && l.q.repeat == "one":
			// The same entry again.
		case l.q.current+1 < total:
			l.q.current++
		case l.q.repeat == "all":
			l.q.current = 0
			if l.q.shuffle {
				l.q.lap++
			}
		default:
			return false, ErrQueueEnded
		}
		// The entry must be one this caller can play before the position moves
		// (review P21): next, previous and completion step past entries the
		// fence withholds (a bounded walk, wrapping with repeat all); a named
		// entry, or repeat one, that it withholds is not found and nothing moves.
		visible, err := s.visibleFilter(ctx, tx, c.Principal)
		if err != nil {
			return false, err
		}
		step := int64(1)
		if req.Reason == "previous" {
			step = -1
		}
		fixed := req.Reason == "entry" || req.Reason == "completion" && l.q.repeat == "one"
		for walked := 0; ; walked++ {
			// An entry still being snapshotted can't play yet: nothing moves, and
			// the client retries shortly (P16).
			_, key, err := l.entryAtErr(l.q.current)
			if l.waiting() || errors.Is(err, ErrQueueBuilding) {
				return false, ErrQueueBuilding
			}
			if err != nil {
				return false, err
			}
			allowed, err := visible([]string{key})
			if err != nil {
				return false, err
			}
			if len(allowed) == 1 {
				return true, nil
			}
			if fixed || walked >= advanceSkipMax {
				return false, ErrNotFound
			}
			next := l.q.current + step
			switch {
			case next >= 0 && next < total:
			case next >= total && l.q.repeat == "all":
				next = 0
				if l.q.shuffle {
					l.q.lap++
				}
			case next >= total:
				return false, ErrQueueEnded
			default:
				return false, ErrNotFound
			}
			l.q.current = next
		}
	})
	if err != nil {
		return QueueReply{}, err
	}
	reply := QueueReply{Queue: s.header(l)}
	if l.q.session != "" && reply.Queue.Current != nil {
		old, err := loadRow(ctx, s.DB, l.q.session)
		// §18.5: a completion still starts the next entry after the track reported
		// "ended" on this device, and the successor inherits the play state and
		// the quality request.
		if err == nil && (old.ended == 0 || req.Reason == "completion" && old.device == c.DeviceID) {
			start := StartRequest{Queue: &QueueRef{QueueID: l.q.id, EntryID: reply.Queue.Current.EntryID}}
			var prior struct {
				storedRequest
				State string `json:"state"`
			}
			if json.Unmarshal([]byte(old.request), &prior) == nil && prior.Quality.Mode != "" {
				quality := prior.Quality
				start.Quality = &quality
			}
			if old.state == "paused" {
				start.State = "paused"
			}
			if old.device == c.DeviceID {
				start.ReplacesSessionID = old.id
			}
			session, _, err := s.Start(ctx, c, "advance-"+l.q.id[2:]+"-"+strconv.FormatInt(l.q.revision, 10), start)
			if err != nil {
				return QueueReply{}, err
			}
			if start.ReplacesSessionID == "" && old.ended == 0 {
				_ = s.End(ctx, old.id, "replaced", "")
			}
			reply.Session = &session
			if q, err := loadQueue(ctx, s.DB, l.q.id); err == nil {
				_ = s.withLayout(ctx, q, func(l *layout) error { reply.Queue = s.header(l); return nil })
			}
		}
	}
	if q, err := loadQueue(ctx, s.DB, l.q.id); err == nil {
		s.withNext(ctx, c, &reply.Queue, q)
	}
	return reply, nil
}

// advanceSkipMax bounds how many withheld entries one advance steps past.
const advanceSkipMax = 200

// DeleteQueue is DELETE /v1/queues/{id}: idempotent. A queue that doesn't
// exist and one that isn't the caller's get the same answer (review P20: no
// existence oracle); only the caller's own is deleted.
func (s *Service) DeleteQueue(ctx context.Context, c Caller, id string) error {
	q, err := loadQueue(ctx, s.DB, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !ownsQueue(c, q) {
		return nil
	}
	// Retired now; its keys are removed in batches by the sweep (review P19).
	if _, err = dbwork.ExecWrite(ctx, s.DB, dbwork.ClassInteractive, `UPDATE queues_v1 SET owner_id='!'||owner_id WHERE id=? AND substr(owner_id,1,1) NOT IN('~','!')`, id); err != nil {
		return err
	}
	s.sweepQueues()
	return nil
}

// startQueueEntry starts a session on a queue entry ({queue: {queueId,
// entryId}}), links it to the queue, and makes it the queue's session.
func (s *Service) startQueueEntry(ctx context.Context, c Caller, key, startDigest string, req StartRequest) (SessionView, bool, error) {
	q, err := loadQueue(ctx, s.DB, req.Queue.QueueID)
	if err != nil || !ownsQueue(c, q) {
		return SessionView{}, false, ErrNotFound
	}
	var item string
	err = s.withLayout(ctx, q, func(l *layout) error {
		p, ok := l.positionOf(req.Queue.EntryID)
		if !ok {
			return ErrNotFound
		}
		_, key, err := l.entryAtErr(p)
		if err != nil {
			return err // ErrQueueBuilding: this entry's key is still being snapshotted
		}
		item = key
		return nil
	})
	if err != nil {
		return SessionView{}, false, err
	}
	kind, err := readItemKind(ctx, s.DB, item)
	if errors.Is(err, compactcatalog.ErrBuilding) {
		return SessionView{}, false, err
	}
	if err != nil {
		return SessionView{}, false, ErrNotFound
	}
	sessionKind := "vod"
	if kind == "song" || kind == "audiobook_file" || kind == "track" {
		sessionKind = "audio"
	}
	stored := storedRequest{VersionID: req.VersionID, Quality: Quality{Mode: "original"}, Audio: req.Audio, Subtitles: req.Subtitles, Network: req.Network}
	if req.Quality != nil {
		stored.Quality = *req.Quality
	}
	view, preparing, err := s.startPresentation(ctx, c, key, startDigest, req, row{kind: sessionKind, role: "local", item: item, version: req.VersionID, queueID: q.id, entryID: req.Queue.EntryID}, stored)
	if err != nil {
		return view, preparing, err
	}
	_, _ = dbwork.ExecWrite(ctx, s.DB, dbwork.ClassInteractive, `UPDATE queues_v1 SET session_id=? WHERE id=?`, view.ID, q.id)
	return view, preparing, nil
}

var _ = identity.ErrUnauthorized

// planBuilding settles, before a create's write, how each snapshot the request
// couldn't finish starts (P16):
//   - ordered: at its anchor or first entry, at once, as the segment grows; an
//     item anchor the scan hadn't reached waits for the build;
//   - shuffled: one fenced count gives the full size, and the first entry's key
//     is resolved ahead, so playback starts at once over the final domain; if
//     that can't be done in time, the start waits for the build.
func (s *Service) planBuilding(ctx context.Context, c Caller, segments []SegmentInput, snaps []snapshot) error {
	caller, _ := json.Marshal(buildCaller{Viewer: c.Principal.Viewer, Epoch: c.Principal.Epoch})
	for i := range snaps {
		snap := &snaps[i]
		if !snap.partial {
			continue
		}
		snap.caller = string(caller)
		in := segments[i]
		if !(i == 0 && in.Order != nil && in.Order.Mode == "shuffle") {
			if snap.anchorItem != "" {
				raw, _ := json.Marshal(queueStart{Mode: "ordered", AnchorItem: snap.anchorItem})
				snap.start = string(raw)
			}
			continue
		}
		seed, err := seedFrom(in.Order.Seed)
		if err != nil {
			return err
		}
		// Only a shuffle of this one segment starts at once: its pick is uniform
		// over the queue only when nothing else is in it.
		ok := len(snaps) == 1
		if ok {
			var n, ordinal int64
			var key string
			n, ordinal, key, ok = s.countAndPick(ctx, c.Principal, in, seed, snap.anchorItem)
			if ok {
				// An anchor already found keeps its place; otherwise the pass's
				// anchor item or seeded pick is the first entry.
				snap.declared, snap.anchorItem = n, ""
				if snap.anchor < 0 {
					snap.anchor = ordinal
					if ordinal >= snap.count {
						snap.hint, snap.hintKey = ordinal, key
					}
				}
			}
		}
		if !ok {
			raw, _ := json.Marshal(queueStart{Mode: "shuffle", Seed: seed})
			snap.start = string(raw)
		}
	}
	return nil
}
