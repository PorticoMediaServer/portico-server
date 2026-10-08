package playbackv1

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/identity"
)

// Published window maximum (spec §8, invariant 8).
const WindowMax = 200

var shortEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// newShortID is a segment or snapshot id: lowercase letters and digits only, so
// "<snapshot>-<ordinal>" entry ids are unambiguous and URL-safe.
func newShortID(prefix string) string {
	var raw [10]byte
	_, _ = rand.Read(raw[:])
	return prefix + shortEncoding.EncodeToString(raw[:])
}

// QueueView is the queue header (spec §8, §17.9): constant size.
type QueueView struct {
	ID       string        `json:"id"`
	Revision string        `json:"revision"`
	Total    int64         `json:"total"`
	Current  *QueueCurrent `json:"current,omitempty"`
	Repeat   string        `json:"repeat"`
	Shuffle  *QueueShuffle `json:"shuffle,omitempty"`
	// Segments lists at most HeaderSegments of them, from the one holding the
	// current entry; SegmentCount is how many there are.
	Segments     []SegmentView `json:"segments"`
	SegmentCount int           `json:"segmentCount"`
	// Next and PostPlay (spec §18.5) are filled in by the server lane.
	Next     *QueueNext     `json:"next,omitempty"`
	PostPlay *QueuePostPlay `json:"postPlay,omitempty"`
}
type QueueCurrent struct {
	EntryID  string `json:"entryId"`
	Position int64  `json:"position"`
}
type QueueShuffle struct {
	Seed string `json:"seed"`
	Lap  int64  `json:"lap"`
}
type SegmentView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Count int64  `json:"count"`
	State string `json:"state"`
}

// QueueEntry is one window entry (ARCH-MEDIA-03 delivery).
type QueueEntry struct {
	EntryID    string `json:"entryId"`
	Position   int64  `json:"position"`
	ItemID     string `json:"itemId"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Available  bool   `json:"available"`
	// AlbumID, Disc and Track place a song on its album (a book file in its
	// book), so a client joins consecutive tracks of one album gaplessly rather
	// than crossfading them (Plexamp's behavior, Plan §9.6). Absent when unknown.
	AlbumID string `json:"albumId,omitempty"`
	Disc    int    `json:"disc,omitempty"`
	Track   int    `json:"track,omitempty"`
}

// Page is the style guide's collection envelope for queue entries (§17.9).
type Page struct {
	Limit      int    `json:"limit"`
	Total      int64  `json:"total"`
	Start      int64  `json:"start"`
	Revision   string `json:"revision"`
	NextCursor string `json:"nextCursor,omitempty"`
}
type EntryPage struct {
	Items []QueueEntry `json:"items"`
	Page  Page         `json:"page"`
}

// CreateQueueRequest is POST /v1/queues. The segment form is ARCH-MEDIA-03's;
// selector/shuffle/repeat is spec §17.10's single-selector shorthand.
type CreateQueueRequest struct {
	Owner         string         `json:"owner,omitempty"`
	Segments      []SegmentInput `json:"segments,omitempty"`
	Selector      *Selector      `json:"selector,omitempty"`
	Shuffle       *QueueSeed     `json:"shuffle,omitempty"`
	Repeat        string         `json:"repeat,omitempty"`
	StartPlayback *StartPlayback `json:"startPlayback,omitempty"`
}
type QueueSeed struct {
	Seed string `json:"seed,omitempty"`
}
type StartPlayback struct {
	State   string   `json:"state,omitempty"`
	Quality *Quality `json:"quality,omitempty"`
}

// QueueReply is a command's answer: the new header and, when a session
// started (Play, advance), that session (§17.8).
type QueueReply struct {
	Queue   QueueView    `json:"queue"`
	Window  []QueueEntry `json:"window,omitempty"`
	Session *SessionView `json:"session,omitempty"`
}

// AddSegmentRequest is POST /v1/queues/{id}/segments.
type AddSegmentRequest struct {
	Placement Placement `json:"placement"`
	Source    Selector  `json:"source"`
	Order     *Order    `json:"order,omitempty"`
	Anchor    *Anchor   `json:"anchor,omitempty"`
}

// Placement is "next", "end" or {after: entryId}.
type Placement struct {
	Where string
	After string
}

func (p *Placement) UnmarshalJSON(b []byte) error {
	var word string
	if json.Unmarshal(b, &word) == nil {
		if word != "next" && word != "end" {
			return errors.New("unknown placement")
		}
		p.Where = word
		return nil
	}
	var v struct {
		After string `json:"after"`
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil || v.After == "" {
		return errors.New("placement needs after")
	}
	p.Where, p.After = "after", v.After
	return nil
}
func (Placement) JSONSchema() map[string]any { return map[string]any{} }

// MoveRequest is POST /v1/queues/{id}/entries/{entryId}:move.
type MoveRequest struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// QueueChange is PATCH /v1/queues/{id}.
type QueueChange struct {
	Repeat  string         `json:"repeat,omitempty"`
	Shuffle *ShuffleSwitch `json:"shuffle,omitempty"`
}
type ShuffleSwitch struct {
	On   bool   `json:"on"`
	Seed string `json:"seed,omitempty"`
}

// AdvanceRequest is POST /v1/queues/{id}:advance.
type AdvanceRequest struct {
	Reason  string `json:"reason"`
	EntryID string `json:"entryId,omitempty"`
}

// queueRow is one queues_v1 row.
type queueRow struct {
	id, ownerKind, ownerID, authority, account, profile, session  string
	repeat                                                        string
	revision, current, seed, lap, domain, first, created, updated int64
	shuffle                                                       bool
}

const queueColumns = `id,owner_kind,owner_id,authority,account_id,profile_id,session_id,repeat,revision,current_position,shuffle_seed,shuffle_lap,shuffle_domain,shuffle_first,created_ms,updated_ms,shuffle_on`

func scanQueue(r scanner) (queueRow, error) {
	var q queueRow
	err := r.Scan(&q.id, &q.ownerKind, &q.ownerID, &q.authority, &q.account, &q.profile, &q.session, &q.repeat, &q.revision, &q.current, &q.seed, &q.lap, &q.domain, &q.first, &q.created, &q.updated, &q.shuffle)
	return q, err
}

// segment is one queue_v1_segments row with its snapshot's kind and label.
type segment struct {
	id, snapshot, kind, label, sortKey string
	first, count, removed              int64
	// A snapshot still building (P16) holds keys [0, built); the rest are
	// pending, except one resolved ahead (hint). waiting: a start waits for it.
	state   string
	built   int64
	waiting bool
	hint    int64
	hintKey string
}

func (g segment) live() int64 { return g.count - g.removed }

// layout is a queue's segments in order plus the removals inside them; it is
// read once per request (a queue has few segments) and answers position ↔
// entry in O(segments + removals), independent of queue length.
type layout struct {
	q        queueRow
	segments []segment
	removals map[string][]int64 // snapshot → sorted ordinals
	chunks   map[string][]byte  // snapshot/chunk → keys (per request)
	building []segment          // segments of snapshots still building
	tx       *sql.Tx
	ctx      context.Context
}

func (s *Service) readLayout(ctx context.Context, tx *sql.Tx, q queueRow) (*layout, error) {
	l := &layout{q: q, removals: map[string][]int64{}, chunks: map[string][]byte{}, tx: tx, ctx: ctx}
	rows, err := tx.QueryContext(ctx, `SELECT g.id,g.snapshot_id,s.kind,s.label,g.sort_key,g.first_ordinal,g.count,g.removed,s.state,s.built,s.start<>'',s.hint_ordinal,s.hint_key FROM queue_v1_segments g JOIN queue_v1_snapshots s ON s.id=g.snapshot_id WHERE g.queue_id=? ORDER BY g.sort_key`, q.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var g segment
		if err = rows.Scan(&g.id, &g.snapshot, &g.kind, &g.label, &g.sortKey, &g.first, &g.count, &g.removed, &g.state, &g.built, &g.waiting, &g.hint, &g.hintKey); err != nil {
			return nil, err
		}
		g.waiting = g.waiting && g.state == "building"
		l.segments = append(l.segments, g)
		if g.state == "building" {
			l.building = append(l.building, g)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	removals, err := tx.QueryContext(ctx, `SELECT r.snapshot_id,r.ordinal FROM queue_v1_removals r JOIN queue_v1_snapshots s ON s.id=r.snapshot_id WHERE s.queue_id=? ORDER BY r.snapshot_id,r.ordinal`, q.id)
	if err != nil {
		return nil, err
	}
	defer removals.Close()
	for removals.Next() {
		var snap string
		var ordinal int64
		if err = removals.Scan(&snap, &ordinal); err != nil {
			return nil, err
		}
		l.removals[snap] = append(l.removals[snap], ordinal)
	}
	return l, removals.Err()
}

func (l *layout) total() int64 {
	var n int64
	for _, g := range l.segments {
		n += g.live()
	}
	return n
}

// sourceAt maps a source position to (segment index, absolute ordinal).
func (l *layout) sourceAt(p int64) (int, int64, bool) {
	if p < 0 {
		return 0, 0, false
	}
	for i, g := range l.segments {
		if p < g.live() {
			ordinal := g.first + p
			for _, r := range l.removals[g.snapshot] {
				if r < g.first || r >= g.first+g.count {
					continue
				}
				if r <= ordinal {
					ordinal++
				}
			}
			return i, ordinal, true
		}
		p -= g.live()
	}
	return 0, 0, false
}

// sourceIndexOf maps an entry id to its source position.
func (l *layout) sourceIndexOf(entry string) (int64, int, bool) {
	snap, ordinalText, ok := strings.Cut(entry, "-")
	ordinal, err := strconv.ParseInt(ordinalText, 10, 64)
	if !ok || err != nil {
		return 0, 0, false
	}
	var base int64
	for i, g := range l.segments {
		if g.snapshot == snap && ordinal >= g.first && ordinal < g.first+g.count {
			before := int64(0)
			for _, r := range l.removals[snap] {
				if r == ordinal {
					return 0, 0, false
				}
				if r >= g.first && r < ordinal {
					before++
				}
			}
			return base + ordinal - g.first - before, i, true
		}
		base += g.live()
	}
	return 0, 0, false
}

func (l *layout) shuffle() *Shuffle {
	if !l.q.shuffle || l.q.domain < 1 {
		return nil
	}
	s, err := NewShuffle(uint64(l.q.domain), uint32(l.q.seed), uint32(l.q.lap))
	if err != nil {
		return nil
	}
	return s
}

// orderToSource: play order → source position. With shuffle on, the current
// entry at the time is position 0, the rest follow the permutation, and
// entries added later form an unshuffled tail (spec §17.10).
func (l *layout) orderToSource(p int64) int64 {
	sh := l.shuffle()
	if sh == nil || p >= l.q.domain {
		return p
	}
	if p == 0 {
		return l.q.first
	}
	c, _ := sh.IndexOf(uint64(l.q.first))
	k := uint64(p)
	if uint64(p-1) < c {
		k = uint64(p - 1)
	}
	at, _ := sh.At(k)
	return int64(at)
}

func (l *layout) sourceToOrder(src int64) int64 {
	sh := l.shuffle()
	if sh == nil || src >= l.q.domain {
		return src
	}
	if src == l.q.first {
		return 0
	}
	c, _ := sh.IndexOf(uint64(l.q.first))
	k, _ := sh.IndexOf(uint64(src))
	if k < c {
		return int64(k) + 1
	}
	return int64(k)
}

// key reads an entry's item key from its snapshot chunk (cached per request).
// pendingKey says whether a snapshot's ordinal isn't built yet (and the one
// key resolved ahead of the build, if it's that one).
func (l *layout) pendingKey(snap string, ordinal int64) (string, bool) {
	for _, g := range l.building {
		if g.snapshot == snap && ordinal >= g.built {
			if ordinal == g.hint && g.hintKey != "" {
				return g.hintKey, true
			}
			return "", true
		}
	}
	return "", false
}

// waiting says whether a start waits for a building snapshot (no current yet).
func (l *layout) waiting() bool {
	for _, g := range l.building {
		if g.waiting {
			return true
		}
	}
	return false
}

func (l *layout) key(snap string, ordinal int64) (string, error) {
	if key, pending := l.pendingKey(snap, ordinal); pending {
		if key != "" {
			return key, nil
		}
		return "", ErrQueueBuilding
	}
	no := ordinal / chunkKeys
	cacheKey := snap + "/" + strconv.FormatInt(no, 10)
	chunk, ok := l.chunks[cacheKey]
	if !ok {
		if err := l.tx.QueryRowContext(l.ctx, `SELECT keys FROM queue_v1_chunks WHERE snapshot_id=? AND chunk_no=?`, snap, no).Scan(&chunk); err != nil {
			return "", err
		}
		l.chunks[cacheKey] = chunk
	}
	k, ok := chunkKey(chunk, int(ordinal%chunkKeys))
	if !ok {
		return "", ErrNotFound
	}
	return k, nil
}

// prefetch reads, in one statement, every chunk that positions [from, from+n)
// need and the request hasn't read yet: a shuffled window touches up to n
// different chunks, which were n statements (P14).
func (l *layout) prefetch(from, n int64) error {
	type ref struct {
		Snapshot string `json:"s"`
		Chunk    int64  `json:"c"`
	}
	want := []ref{}
	seen := map[string]bool{}
	for p := from; p < from+n; p++ {
		i, ordinal, ok := l.sourceAt(l.orderToSource(p))
		if !ok {
			continue
		}
		snap, no := l.segments[i].snapshot, ordinal/chunkKeys
		if _, pending := l.pendingKey(snap, ordinal); pending {
			continue
		}
		cacheKey := snap + "/" + strconv.FormatInt(no, 10)
		if _, cached := l.chunks[cacheKey]; cached || seen[cacheKey] {
			continue
		}
		seen[cacheKey] = true
		want = append(want, ref{snap, no})
	}
	if len(want) == 0 {
		return nil
	}
	raw, _ := json.Marshal(want)
	rows, err := l.tx.QueryContext(l.ctx, `SELECT c.snapshot_id,c.chunk_no,c.keys FROM json_each(?) w JOIN queue_v1_chunks c ON c.snapshot_id=json_extract(w.value,'$.s') AND c.chunk_no=json_extract(w.value,'$.c')`, string(raw))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var snap string
		var no int64
		var keys []byte
		if err = rows.Scan(&snap, &no, &keys); err != nil {
			return err
		}
		l.chunks[snap+"/"+strconv.FormatInt(no, 10)] = keys
	}
	return rows.Err()
}

// entryAt resolves a play-order position to its entry id and item key.
func (l *layout) entryAt(p int64) (string, string, bool) {
	entry, key, err := l.entryAtErr(p)
	return entry, key, err == nil
}

// entryAtErr is entryAt that tells a pending entry (ErrQueueBuilding: it has an
// id and a place, but its key isn't built yet) from one that doesn't exist.
func (l *layout) entryAtErr(p int64) (string, string, error) {
	i, ordinal, ok := l.sourceAt(l.orderToSource(p))
	if !ok {
		return "", "", ErrNotFound
	}
	snap := l.segments[i].snapshot
	entry := snap + "-" + strconv.FormatInt(ordinal, 10)
	key, err := l.key(snap, ordinal)
	if errors.Is(err, ErrQueueBuilding) {
		return entry, "", err
	}
	if err != nil {
		return "", "", ErrNotFound
	}
	return entry, key, nil
}

func (l *layout) positionOf(entry string) (int64, bool) {
	src, _, ok := l.sourceIndexOf(entry)
	if !ok {
		return 0, false
	}
	return l.sourceToOrder(src), true
}

func (s *Service) header(l *layout) QueueView {
	v := QueueView{ID: l.q.id, Revision: strconv.FormatInt(l.q.revision, 10), Total: l.total(), Repeat: l.q.repeat, Segments: []SegmentView{}}
	// While a start waits for its snapshot (P16) there's no current entry; a
	// current entry that's still pending has its id and position already.
	if entry, _, err := l.entryAtErr(l.q.current); !l.waiting() && (err == nil || errors.Is(err, ErrQueueBuilding)) {
		v.Current = &QueueCurrent{EntryID: entry, Position: l.q.current}
	}
	if l.q.shuffle {
		v.Shuffle = &QueueShuffle{Seed: strconv.FormatInt(l.q.seed, 10), Lap: l.q.lap}
	}
	// A constant-size header (P15): the segments from the current one on.
	v.SegmentCount = len(l.segments)
	start := 0
	if i, _, ok := l.sourceAt(l.orderToSource(l.q.current)); ok {
		start = i
	}
	start = max(0, min(start, len(l.segments)-HeaderSegments))
	for _, g := range l.segments[start:min(len(l.segments), start+HeaderSegments)] {
		kind := g.kind
		if kind != "items" {
			kind = "selector"
		}
		v.Segments = append(v.Segments, SegmentView{ID: g.id, Kind: kind, Label: g.label, Count: g.live(), State: g.state})
	}
	return v
}

// HeaderSegments is the most segments a queue header lists.
const HeaderSegments = 64

// Visibility is the item fence as one SQL predicate over an item id expression:
// it admits only what the caller may play (library access, profile
// restrictions, member limits). HTTP installs contentaccess.VisibleItemsSQL.
type Visibility func(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) (string, []any, error)

// visibleFilter keeps, in order, the ids the caller may play: one statement per
// call, for a window or a chunk of a snapshot. Inaccessible and absent ids are
// the same: dropped.
func (s *Service) visibleFilter(ctx context.Context, tx *sql.Tx, p identity.Principal) (func([]string) ([]string, error), error) {
	// The filter reads catalogue facts; the visibility clause brings its own
	// reads. No derived-domain wait here.
	clause, args := "1", []any(nil)
	if s.Visibility != nil {
		var err error
		if clause, args, err = s.Visibility(ctx, tx, p, "i.id"); err != nil {
			return nil, err
		}
	}
	query := `SELECT v.value FROM json_each(?) v JOIN catalog_entities i ON i.public_id=pid_blob(v.value) JOIN catalog_kinds k ON k.id=i.kind AND k.playable=1 WHERE ` + clause + ` ORDER BY v.key`
	return func(ids []string) ([]string, error) {
		if len(ids) == 0 {
			return nil, nil
		}
		rows, err := tx.QueryContext(ctx, query, append([]any{jsonList(ids)}, args...)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]string, 0, len(ids))
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	}, nil
}

// window resolves positions [from, from+n) with one batched item read and one
// set-based authorization; unavailable entries are placeholders (ARCH-MEDIA-03).
func (s *Service) window(ctx context.Context, l *layout, p identity.Principal, from, n int64) ([]QueueEntry, error) {
	total := l.total()
	if from < 0 {
		from = 0
	}
	n = max(0, min(n, WindowMax, total-from))
	if err := l.prefetch(from, n); err != nil {
		return nil, err
	}
	out := make([]QueueEntry, 0, n)
	keys := make([]string, 0, n)
	for i := int64(0); i < n; i++ {
		entry, key, err := l.entryAtErr(from + i)
		if errors.Is(err, ErrQueueBuilding) {
			// Still being snapshotted (P16): a placeholder that fills in shortly.
			out = append(out, QueueEntry{EntryID: entry, Position: from + i, Kind: "pending"})
			continue
		}
		if err != nil {
			continue
		}
		out = append(out, QueueEntry{EntryID: entry, Position: from + i, ItemID: key})
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return out, nil
	}
	// Facts only (entities, songs, book files, links, assets): no wait.
	type info struct {
		kind, title, album string
		duration           float64
		disc, track        int
	}
	found := map[string]info{}
	rows, err := l.tx.QueryContext(ctx, `SELECT pid(i.public_id),k.name,i.title,
 COALESCE((SELECT a.duration FROM catalog_asset_links x JOIN catalog_assets a ON a.id=x.asset_id WHERE x.entity_id=i.id ORDER BY x.part_index,a.id LIMIT 1),0),
 COALESCE(pid(album.public_id),pid(book.public_id),''),COALESCE(s.disc_number,b.disc_number,0),COALESCE(s.track_number,b.part_number,0)
	FROM catalog_entities i JOIN catalog_kinds k ON k.id=i.kind AND k.playable=1
	LEFT JOIN catalog_songs s ON s.entity_id=i.id LEFT JOIN catalog_entities album ON album.id=s.album_id
	LEFT JOIN catalog_book_files b ON b.entity_id=i.id LEFT JOIN catalog_entities book ON book.id=b.book_id
	WHERE i.public_id IN (SELECT pid_blob(value) FROM json_each(?))`, jsonList(keys))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var v info
		if err = rows.Scan(&id, &v.kind, &v.title, &v.duration, &v.album, &v.disc, &v.track); err != nil {
			rows.Close()
			return nil, err
		}
		found[id] = v
	}
	rows.Close()
	filter, err := s.visibleFilter(ctx, l.tx, p)
	if err != nil {
		return nil, err
	}
	allowed, err := filter(keys)
	if err != nil {
		return nil, err
	}
	visible := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		visible[id] = true
	}
	for i := range out {
		if out[i].Kind == "pending" {
			continue
		}
		v, ok := found[out[i].ItemID]
		if ok && visible[out[i].ItemID] {
			out[i].Kind, out[i].Title, out[i].DurationMs, out[i].Available = v1Kind(v.kind), v.title, int64(v.duration*1000), true
			out[i].AlbumID, out[i].Disc, out[i].Track = v.album, max(v.disc, 0), max(v.track, 0)
		} else {
			// A title that's gone or became restricted after the snapshot is a
			// placeholder: no id, no title, not playable.
			out[i].ItemID, out[i].Kind, out[i].Available = "", "unavailable", false
		}
	}
	return out, nil
}

const sortDigits = "0123456789abcdefghijklmnopqrstuvwxyz"

// sortKeyBetween is a fractional key strictly between a and b ("" as b is the
// open end; a < b), so inserting a segment between two others is one row
// write. Generated keys never end in '0', which keeps a gap below every key.
func sortKeyBetween(a, b string) string {
	digit := func(s string, i int) int {
		if i < len(s) {
			return strings.IndexByte(sortDigits, s[i])
		}
		return 0
	}
	if b == "" {
		// Appending is the common case: step the first digit that can still
		// grow, so keys lengthen by one character per ~35 appends.
		for i := 0; ; i++ {
			if d := digit(a, i); d < len(sortDigits)-1 {
				return a[:min(i, len(a))] + strings.Repeat("0", max(0, i-len(a))) + string(sortDigits[d+1])
			}
		}
	}
	var out []byte
	for i := 0; ; i++ {
		lo, hi := digit(a, i), len(sortDigits)
		if b != "" {
			hi = digit(b, i)
		}
		if b != "" && hi == lo {
			out = append(out, sortDigits[lo])
			continue
		}
		if hi-lo > 1 {
			return string(append(out, sortDigits[(lo+hi)/2]))
		}
		// Adjacent digits: keep a's digit and find room above the rest of a.
		out = append(out, sortDigits[lo])
		b = ""
	}
}

func seedFrom(text string) (int64, error) {
	if text == "" {
		var raw [4]byte
		_, _ = rand.Read(raw[:])
		return int64(binary.BigEndian.Uint32(raw[:])), nil
	}
	n, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, &FieldError{Path: "shuffle.seed"}
	}
	return int64(uint32(n)), nil
}

func loadQueue(ctx context.Context, q querier, id string) (queueRow, error) {
	row, err := scanQueue(q.QueryRowContext(ctx, `SELECT `+queueColumns+` FROM queues_v1 WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// ownsQueue: the queue's profile, from its own device or another of that
// profile's devices. The device alone is never enough: another profile signed
// in on the same TV can't read or control it (P17).
// ownsQueue: the caller's profile's queue, and a live one: a queue being
// staged ("~" owner) or retired by a replacement ("!" owner) is no one's.
func ownsQueue(c Caller, q queueRow) bool {
	return q.authority == c.Principal.Authority && q.account == c.Principal.AccountID && q.profile == c.Principal.ProfileID &&
		!strings.HasPrefix(q.ownerID, "~") && !strings.HasPrefix(q.ownerID, "!")
}

func (s *Service) queueEventsTx(ctx context.Context, tx *sql.Tx, l *layout, now time.Time) error {
	v := s.header(l)
	data := map[string]any{"total": v.Total}
	if v.Current != nil {
		data["current"] = v.Current
	}
	resource := &EventResource{Kind: "queue", ID: l.q.id}
	rev := strconv.FormatInt(l.q.revision, 10)
	if l.q.ownerKind == "group" {
		return publishTx(ctx, tx, now, GroupAudience(l.q.ownerID), "queue.updated", resource, rev, data)
	}
	if err := publishTx(ctx, tx, now, DeviceAudience(l.q.ownerID), "queue.updated", resource, rev, data); err != nil {
		return err
	}
	return publishTx(ctx, tx, now, ProfileAudience(l.q.authority, l.q.account, l.q.profile), "queue.updated", resource, rev, data)
}
