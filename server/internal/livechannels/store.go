package livechannels

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
)

const MaxSources = 64
const StageBatch = 128
const GuideCandidateBatch = 512

type Store struct {
	db *sql.DB
	// cursorKey signs continuation cursors. It is stable per state folder
	// (configuration live_cursor_key), never a file key.
	cursorKey []byte
}

// cursorKeyName keeps the cursor HMAC key in the configuration table. Hashing
// that is not encryption stays; there is no key file and no lost-key refusal.
const cursorKeyName = "live_cursor_key"

func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, ErrUnavailable
	}
	cursor, err := loadOrCreateCursorKey(db)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, cursorKey: cursor}, nil
}

// loadOrCreateCursorKey reads the stable cursor key, generating and storing
// it on first use.
func loadOrCreateCursorKey(db *sql.DB) ([]byte, error) {
	var raw string
	if err := db.QueryRow(`SELECT value FROM configuration WHERE key=?`, cursorKeyName).Scan(&raw); err == nil {
		if key, derr := hex.DecodeString(raw); derr == nil && len(key) == 32 {
			return key, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, cursorKeyName, hex.EncodeToString(key)); err != nil {
		return nil, err
	}
	return key, nil
}
func id(parts ...string) string {
	h := sha256.New()
	for _, v := range parts {
		fmt.Fprintf(h, "%d:%s", len(v), v)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func opaque(v string) bool {
	if len(v) != 48 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil && strings.ToLower(v) == v
}
func nonceID() (string, error) {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Store) seal(v, _ string) ([]byte, error) {
	// Plaintext: source URLs and credentials are plain columns. Folder
	// permissions are the protection, as in Plex.
	return []byte(v), nil
}
func authorize(ctx context.Context, tx *sql.Tx, a Authority, owner bool) (string, func(string, string) bool, error) {
	if a == nil {
		return "", nil, ErrDenied
	}
	f, allowed, e := a(ctx, tx, owner)
	if e != nil {
		if errors.Is(e, ErrDenied) {
			return "", nil, ErrDenied
		}
		return "", nil, ErrUnavailable
	}
	if f == "" {
		return "", nil, ErrDenied
	}
	if allowed == nil {
		allowed = func(string, string) bool { return false }
	}
	return f, allowed, nil
}
func (s *Store) transaction(ctx context.Context, a Authority, owner bool, work func(*sql.Tx) error) error {
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return ErrUnavailable
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if _, _, e = authorize(ctx, tx, a, owner); e != nil {
		return e
	}
	if e = work(tx); e != nil {
		return e
	}
	if _, _, e = authorize(ctx, tx, a, owner); e != nil {
		return e
	}
	if e = gated.Commit(); e != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Store) snapshot(ctx context.Context, a Authority, owner bool, work func(*sql.Tx) error) error {
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return ErrUnavailable
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if _, _, e = authorize(ctx, tx, a, owner); e != nil {
		return e
	}
	if e = work(tx); e != nil {
		return e
	}
	if _, _, e = authorize(ctx, tx, a, owner); e != nil {
		return e
	}
	if e = gated.Commit(); e != nil {
		return ErrUnavailable
	}
	return nil
}
func sourceProjection(v Source) Source {
	v.DeliveryValidation = "not-validated"
	v.CapacityKnown = v.TunerCount > 0
	v.PlanningEstimate = v.TunerCount
	v.TunerCountMode = "configured"
	if !v.CapacityKnown {
		v.PlanningEstimate = 1
		v.TunerCountMode = "defaulted"
	}
	return v
}
func operationSource(ctx context.Context, tx *sql.Tx, requestID string) (Source, error) {
	var v Source
	e := tx.QueryRowContext(ctx, `SELECT o.source_id,g.name,o.result_revision,o.result_state,g.tuner_count,o.generation_id,o.published_at,g.channel_count,g.programme_count FROM live_operations o JOIN live_generations g ON g.id=o.generation_id WHERE o.request_id=? AND o.status='published'`, requestID).Scan(&v.ID, &v.Name, &v.Revision, &v.State, &v.TunerCount, &v.Generation, &v.PublishedAt, &v.Channels, &v.Programmes)
	if e != nil {
		return v, ErrUnavailable
	}
	return sourceProjection(v), nil
}

// Save resumes a durable upload operation. Each transaction stages at most128 rows;
// only the final CAS publishes the configuration and complete generation together.
// Cancellation leaves staging resumable by the same request ID and input digest.
func (s *Store) Save(ctx context.Context, a Authority, in SourceInput) (Source, error) {
	// Uploaded replacement explicitly changes this source back to manual mode.
	// Remote request IDs cannot be replayed through this less-specific endpoint.
	err := s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var exists bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_remote_save_bindings WHERE request_id=?)`, in.RequestID).Scan(&exists); e != nil {
			return ErrUnavailable
		}
		if exists {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return Source{}, err
	}
	return s.save(ctx, a, in, false, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `DELETE FROM live_remote_configs WHERE source_id=?`, in.ID); e != nil {
			return ErrUnavailable
		}
		_, e := tx.ExecContext(ctx, `DELETE FROM live_source_settings WHERE source_id=?`, in.ID)
		if e != nil {
			return ErrUnavailable
		}
		return nil
	})
}

func (s *Store) save(ctx context.Context, a Authority, in SourceInput, refresh bool, publish func(*sql.Tx) error) (Source, error) {
	if !opaque(in.ID) || !opaque(in.RequestID) {
		return Source{}, ErrInvalid
	}
	data, e := parse(in)
	if e != nil {
		return Source{}, e
	}
	hash := id(sourceHash(in), fmt.Sprint(refresh))
	generation := ""
	status := ""
	var result Source
	e = s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var removed bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_source_removals WHERE source_id=?)`, in.ID).Scan(&removed); e != nil {
			return ErrUnavailable
		}
		if removed {
			return ErrConflict
		}
		var old string
		e := tx.QueryRowContext(ctx, `SELECT input_hash,generation_id,status FROM live_operations WHERE request_id=?`, in.RequestID).Scan(&old, &generation, &status)
		if e == nil {
			if old != hash {
				return ErrConflict
			}
			if status == "superseded" {
				return ErrConflict
			}
			if status == "published" {
				var e error
				result, e = operationSource(ctx, tx, in.RequestID)
				return e
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return ErrUnavailable
		}
		var rev int64
		e = tx.QueryRowContext(ctx, `SELECT revision FROM live_sources WHERE id=?`, in.ID).Scan(&rev)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return ErrUnavailable
		}
		if rev != in.ExpectedRevision {
			return ErrConflict
		}
		generation, e = nonceID()
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO live_source_identities VALUES(?) ON CONFLICT DO NOTHING`, in.ID); e != nil {
			return ErrUnavailable
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO live_generations VALUES(?,?,?,?,?,?,?)`, generation, in.ID, in.Name, in.TunerCount, len(data.channels), len(data.programmes), time.Now().UTC().Format(time.RFC3339)); e != nil {
			return ErrUnavailable
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO live_operations(request_id,input_hash,source_id,generation_id,expected_revision,status) VALUES(?,?,?,?,?,'staging')`, in.RequestID, hash, in.ID, generation, in.ExpectedRevision); e != nil {
			return ErrUnavailable
		}
		return nil
	})
	if e != nil || status == "published" {
		return result, e
	}
	ranges := map[string][2]string{}
	for _, p := range data.programmes {
		v := ranges[p.channel]
		start, end := p.start.Format(time.RFC3339), p.end.Format(time.RFC3339)
		if v[0] == "" || start < v[0] {
			v[0] = start
		}
		if end > v[1] {
			v[1] = end
		}
		ranges[p.channel] = v
	}
	for begin := 0; begin < len(data.channels); begin += StageBatch {
		end := min(begin+StageBatch, len(data.channels))
		sealed := make([][]byte, end-begin)
		for i := begin; i < end; i++ {
			sealed[i-begin], e = s.seal(data.channels[i].locator, id(in.ID, data.channels[i].key)+":"+generation)
			if e != nil {
				return Source{}, e
			}
		}
		e = s.transaction(ctx, a, true, func(tx *sql.Tx) error {
			if e := staging(ctx, tx, in.RequestID); e != nil {
				return e
			}
			for i := begin; i < end; i++ {
				c := data.channels[i]
				r := ranges[c.key]
				if _, e := tx.ExecContext(ctx, `INSERT INTO live_channel_versions VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(generation_id,channel_id) DO NOTHING`, generation, id(in.ID, c.key), c.key, c.name, c.number, c.group, i, sealed[i-begin], r[0], r[1]); e != nil {
					return ErrUnavailable
				}
				if c.logo != "" {
					if _, e := tx.ExecContext(ctx, `INSERT INTO live_channel_logos(generation_id,channel_id,url) VALUES(?,?,?) ON CONFLICT DO NOTHING`, generation, id(in.ID, c.key), c.logo); e != nil {
						return ErrUnavailable
					}
				}
			}
			return nil
		})
		if e != nil {
			return Source{}, e
		}
	}
	for begin := 0; begin < len(data.programmes); begin += StageBatch {
		end := min(begin+StageBatch, len(data.programmes))
		e = s.transaction(ctx, a, true, func(tx *sql.Tx) error {
			if e := staging(ctx, tx, in.RequestID); e != nil {
				return e
			}
			for _, p := range data.programmes[begin:end] {
				pid, err := programmeIdentityTx(ctx, tx, in.ID, generation, p)
				if err != nil {
					return err
				}
				if _, e := tx.ExecContext(ctx, `INSERT INTO live_programmes VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(generation_id,id) DO NOTHING`, generation, pid, id(in.ID, p.channel), p.key, p.title, p.start.Format(time.RFC3339), p.end.Format(time.RFC3339), p.lineage); e != nil {
					return ErrUnavailable
				}
				if _, e := tx.ExecContext(ctx, `INSERT INTO live_programme_metadata(generation_id,id,series_id,episode_id,new_evidence,description,facts) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, generation, pid, p.series, p.episode, p.newEvidence, p.description, factsOrEmpty(p.facts)); e != nil {
					return ErrUnavailable
				}
				if p.icon != "" {
					if _, e := tx.ExecContext(ctx, `INSERT INTO live_programme_icons(generation_id,programme_id,url) VALUES(?,?,?) ON CONFLICT DO NOTHING`, generation, pid, p.icon); e != nil {
						return ErrUnavailable
					}
				}
			}
			return nil
		})
		if e != nil {
			return Source{}, e
		}
	}
	conflict := false
	e = s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var status string
		if e := tx.QueryRowContext(ctx, `SELECT status FROM live_operations WHERE request_id=?`, in.RequestID).Scan(&status); e != nil {
			return ErrUnavailable
		}
		if status == "published" {
			var e error
			result, e = operationSource(ctx, tx, in.RequestID)
			return e
		}
		if status != "staging" {
			return ErrConflict
		}
		var channels, programmes int
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM live_channel_versions WHERE generation_id=?),(SELECT count(*) FROM live_programmes WHERE generation_id=?)`, generation, generation).Scan(&channels, &programmes); e != nil || channels != len(data.channels) || programmes != len(data.programmes) {
			return ErrUnavailable
		}
		var rev int64
		state := "active"
		e := tx.QueryRowContext(ctx, `SELECT revision,state FROM live_sources WHERE id=?`, in.ID).Scan(&rev, &state)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return ErrUnavailable
		}
		if rev != in.ExpectedRevision {
			conflict = true
			_, e = tx.ExecContext(ctx, `UPDATE live_operations SET status='superseded' WHERE request_id=?`, in.RequestID)
			if e != nil {
				return ErrUnavailable
			}
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339)
		resultRevision := rev + 1
		if refresh {
			if rev == 0 {
				return ErrConflict
			}
			resultRevision = rev
		}
		if publish != nil {
			if e = publish(tx); e != nil {
				return e
			}
		}
		if rev == 0 {
			var n int
			if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM live_sources s WHERE NOT EXISTS(SELECT 1 FROM live_source_removals r WHERE r.source_id=s.id)`).Scan(&n); e != nil {
				return ErrUnavailable
			}
			if n >= MaxSources {
				return ErrInvalid
			}
			_, e = tx.ExecContext(ctx, `INSERT INTO live_sources VALUES(?,?,?,?,?,?,?)`, in.ID, in.Name, 1, state, in.TunerCount, generation, now)
		} else {
			_, e = tx.ExecContext(ctx, `UPDATE live_sources SET name=?,revision=?,tuner_count=?,active_generation=?,published_at=? WHERE id=? AND revision=?`, in.Name, resultRevision, in.TunerCount, generation, now, in.ID, rev)
		}
		if e != nil {
			return ErrUnavailable
		}
		if _, e = tx.ExecContext(ctx, `UPDATE live_operations SET status='published',result_revision=?,result_state=?,published_at=? WHERE request_id=?`, resultRevision, state, now, in.RequestID); e != nil {
			return ErrUnavailable
		}
		result, e = operationSource(ctx, tx, in.RequestID)
		return e
	})
	if conflict {
		return Source{}, ErrConflict
	}
	return result, e
}
func staging(ctx context.Context, tx *sql.Tx, requestID string) error {
	var status string
	if e := tx.QueryRowContext(ctx, `SELECT status FROM live_operations WHERE request_id=?`, requestID).Scan(&status); e != nil {
		return ErrUnavailable
	}
	if status == "superseded" {
		return ErrConflict
	}
	return nil
}
func (s *Store) Sources(ctx context.Context, a Authority) ([]Source, error) {
	out := []Source{}
	e := s.snapshot(ctx, a, true, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT s.id,s.name,s.revision,s.state,s.tuner_count,s.active_generation,s.published_at,g.channel_count,g.programme_count,COALESCE(x.physical_count,0),COALESCE(x.owner_limit,0) FROM live_sources s JOIN live_generations g ON g.id=s.active_generation LEFT JOIN live_source_settings x ON x.source_id=s.id WHERE NOT EXISTS(SELECT 1 FROM live_source_removals r WHERE r.source_id=s.id) ORDER BY s.name,s.id`)
		if e != nil {
			return ErrUnavailable
		}
		defer rows.Close()
		for rows.Next() {
			var v Source
			var physical, limit int
			if e = rows.Scan(&v.ID, &v.Name, &v.Revision, &v.State, &v.TunerCount, &v.Generation, &v.PublishedAt, &v.Channels, &v.Programmes, &physical, &limit); e != nil {
				return ErrUnavailable
			}
			v = sourceProjection(v)
			if physical > 0 && limit == 0 {
				v.TunerCountMode = "discovered"
			}
			out = append(out, v)
			if len(out) > MaxSources {
				return ErrUnavailable
			}
		}
		if rows.Err() != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func (s *Store) SetEnabled(ctx context.Context, a Authority, sourceID string, revision int64, enabled bool, requestID string) error {
	if !opaque(sourceID) || !opaque(requestID) || revision < 1 {
		return ErrInvalid
	}
	hash := id(sourceID, fmt.Sprint(revision), fmt.Sprint(enabled))
	return s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var old string
		e := tx.QueryRowContext(ctx, `SELECT input_hash FROM live_enable_receipts WHERE request_id=?`, requestID).Scan(&old)
		if e == nil {
			if old != hash {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return ErrUnavailable
		}
		state := "disabled"
		if enabled {
			state = "active"
		}
		res, e := tx.ExecContext(ctx, `UPDATE live_sources SET state=?,revision=revision+1 WHERE id=? AND revision=? AND NOT EXISTS(SELECT 1 FROM live_source_removals r WHERE r.source_id=live_sources.id)`, state, sourceID, revision)
		if e != nil {
			return ErrUnavailable
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrConflict
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO live_enable_receipts VALUES(?,?,?,?)`, requestID, hash, sourceID, revision+1); e != nil {
			return ErrUnavailable
		}
		return nil
	})
}

type guideCursor struct {
	Binding string `json:"b"`
	Offset  int    `json:"o"`
}

func (s *Store) cursor(c guideCursor) string {
	b, _ := json.Marshal(c)
	m := hmac.New(sha256.New, s.cursorKey)
	m.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (s *Store) readCursor(v string) (guideCursor, error) {
	var c guideCursor
	p := strings.Split(v, ".")
	if len(p) != 2 || len(v) > 2048 {
		return c, ErrCursor
	}
	b, e := base64.RawURLEncoding.DecodeString(p[0])
	if e != nil {
		return c, ErrCursor
	}
	sig, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil {
		return c, ErrCursor
	}
	m := hmac.New(sha256.New, s.cursorKey)
	m.Write(b)
	if !hmac.Equal(sig, m.Sum(nil)) || json.Unmarshal(b, &c) != nil || c.Offset < 0 {
		return c, ErrCursor
	}
	return c, nil
}
func (s *Store) Guide(ctx context.Context, a Authority, q GuideQuery) (Guide, error) {
	out := Guide{Channels: []Channel{}, Sources: []GuideSource{}, Timezone: q.Timezone, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if q.Start.IsZero() || q.End.IsZero() || !q.End.After(q.Start) || q.End.Sub(q.Start) > 24*time.Hour || q.Start.Nanosecond() != 0 || q.End.Nanosecond() != 0 || q.Limit < 1 || q.Limit > 50 || len(q.Search) > 120 || len(q.SourceID) > 256 || q.Kind != LiveSource {
		return out, ErrInvalid
	}
	if _, e := time.LoadLocation(q.Timezone); e != nil {
		return out, ErrInvalid
	}
	q.Start = q.Start.UTC()
	q.End = q.End.UTC()
	out.Start = q.Start.Format(time.RFC3339)
	out.End = q.End.Format(time.RFC3339)
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return out, ErrUnavailable
	}
	defer tx.Rollback()
	f, allowed, e := authorize(ctx, tx, a, false)
	if e != nil {
		return out, e
	}
	out.ViewerFence = f
	rows, e := tx.QueryContext(ctx, `SELECT id,active_generation FROM live_sources WHERE state='active' ORDER BY id`)
	if e != nil {
		return out, ErrUnavailable
	}
	keys := []string{}
	for rows.Next() {
		var sid, g string
		if e = rows.Scan(&sid, &g); e != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		keys = append(keys, sid+":"+g)
		if len(keys) > MaxSources {
			rows.Close()
			return out, ErrUnavailable
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, ErrUnavailable
	}
	sort.Strings(keys)
	var preferenceRevision int64
	if q.Viewer.Valid() {
		if tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(revision),0) FROM live_channel_preferences WHERE authority=? AND account_id=? AND profile_id=?`, q.Viewer.Authority, q.Viewer.AccountID, q.Viewer.ProfileID).Scan(&preferenceRevision) != nil {
			return out, ErrUnavailable
		}
	}
	if len(q.ChannelIDs) > 50 {
		return out, ErrInvalid
	}
	channelFilter := ""
	if len(q.ChannelIDs) > 0 {
		raw, e := json.Marshal(q.ChannelIDs)
		if e != nil {
			return out, ErrInvalid
		}
		channelFilter = string(raw)
	}
	binding := id(f, q.Viewer.Key(), fmt.Sprint(preferenceRevision), fmt.Sprint(q.FavoritesOnly), fmt.Sprint(q.IncludeHidden), q.Group, out.Start, out.End, q.Timezone, q.Search, q.SourceID, string(q.Kind), fmt.Sprint(q.Limit), strings.Join(keys, ","), channelFilter, fmt.Sprint(q.NoProgrammes))
	offset := 0
	if q.Cursor != "" {
		c, e := s.readCursor(q.Cursor)
		if e != nil || c.Binding != binding {
			return out, ErrCursor
		}
		offset = c.Offset
	}
	rows, e = tx.QueryContext(ctx, `SELECT c.channel_id,s.id,c.name,c.number,c.group_name,s.active_generation,s.name,s.published_at,c.available_start,c.available_end,COALESCE(p.favorite,0),COALESCE(p.hidden,0),COALESCE(p.revision,0),COALESCE(r.state,'manual') FROM live_sources s JOIN live_channel_versions c ON c.generation_id=s.active_generation LEFT JOIN live_channel_preferences p ON p.authority=? AND p.account_id=? AND p.profile_id=? AND p.source_id=s.id AND p.channel_id=c.channel_id LEFT JOIN live_remote_configs r ON r.source_id=s.id WHERE s.state='active' AND (?='' OR s.id=?) AND (?='' OR instr(lower(c.name||' '||c.group_name),lower(?))>0) AND (?='' OR c.group_name=?) AND (?=0 OR COALESCE(p.favorite,0)=1) AND (?=1 OR COALESCE(p.hidden,0)=0) AND (?='' OR c.channel_id IN (SELECT value FROM json_each(?))) ORDER BY s.name,s.id,c.position,c.channel_id LIMIT ? OFFSET ?`, q.Viewer.Authority, q.Viewer.AccountID, q.Viewer.ProfileID, q.SourceID, q.SourceID, q.Search, q.Search, q.Group, q.Group, q.FavoritesOnly, q.IncludeHidden, channelFilter, channelFilter, GuideCandidateBatch+1, offset)
	if e != nil {
		return out, ErrUnavailable
	}
	sources := map[string]GuideSource{}
	scanned := 0
	more := false
	for rows.Next() {
		if scanned == GuideCandidateBatch || len(out.Channels) == q.Limit {
			more = true
			break
		}
		scanned++
		var c Channel
		var src GuideSource
		if e = rows.Scan(&c.ID, &c.SourceID, &c.Name, &c.Number, &c.Group, &c.Generation, &src.Name, &src.PublishedAt, &src.AvailableStart, &src.AvailableEnd, &c.Favorite, &c.Hidden, &c.PreferenceRevision, &src.RefreshState); e != nil {
			rows.Close()
			return out, ErrUnavailable
		}
		if !allowed(c.SourceID, c.ID) {
			continue
		}
		src.ID = c.SourceID
		src.Generation = c.Generation
		src.Provenance = LiveSource
		c.TuneUnavailableReason = "delivery-unavailable"
		c.RecordUnavailableReason = "recording-unavailable"
		if old, ok := sources[src.ID]; ok {
			if old.AvailableStart != "" && (src.AvailableStart == "" || old.AvailableStart < src.AvailableStart) {
				src.AvailableStart = old.AvailableStart
			}
			if old.AvailableEnd > src.AvailableEnd {
				src.AvailableEnd = old.AvailableEnd
			}
		}
		sources[src.ID] = src
		c.Provenance = LiveSource
		c.TuneUnavailableReason = "delivery-unavailable"
		c.RecordUnavailableReason = "recording-unavailable"
		c.Programmes = []Programme{}
		out.Channels = append(out.Channels, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, ErrUnavailable
	}
	if more {
		out.NextCursor = s.cursor(guideCursor{binding, offset + scanned})
	}
	for _, src := range sources {
		out.Sources = append(out.Sources, src)
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })
	// PERF-13: one programme query per guide page, not one per channel.
	// A generation belongs to exactly one channel version, so filtering by
	// the page's generations returns the same rows as N per-channel window
	// queries; rows are grouped back by channel below.
	count := 0
	if len(out.Channels) > 0 && !q.NoProgrammes {
		gens := make([]string, 0, len(out.Channels))
		byChannel := make(map[string]*Channel, len(out.Channels))
		for i := range out.Channels {
			c := &out.Channels[i]
			gens = append(gens, c.Generation)
			byChannel[c.Generation+"\x00"+c.ID] = c
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", len(gens)), ",")
		args := make([]any, 0, len(gens)+2)
		for _, g := range gens {
			args = append(args, g)
		}
		args = append(args, out.End, out.Start)
		rows, e = tx.QueryContext(ctx, `SELECT p.generation_id,p.channel_id,p.id,p.title,p.start_utc,p.end_utc,p.lineage,COALESCE(m.series_id,''),COALESCE(m.episode_id,''),COALESCE(m.new_evidence,'unknown'),COALESCE(m.description,''),COALESCE(m.facts,'{}') FROM live_programmes p LEFT JOIN live_programme_metadata m ON m.generation_id=p.generation_id AND m.id=p.id WHERE p.generation_id IN (`+marks+`) AND p.start_utc<? AND p.end_utc>? ORDER BY p.generation_id,p.start_utc,p.id`, args...)
		if e != nil {
			return out, ErrUnavailable
		}
		for rows.Next() {
			var gen, channelID string
			p := Programme{}
			var facts string
			if e = rows.Scan(&gen, &channelID, &p.ID, &p.Title, &p.Start, &p.End, &p.Lineage, &p.SeriesID, &p.EpisodeID, &p.NewEvidence, &p.Description, &facts); e != nil {
				rows.Close()
				return out, ErrUnavailable
			}
			p.ChannelID = channelID
			p.ApplyFacts(facts)
			if c := byChannel[gen+"\x00"+channelID]; c != nil {
				c.Programmes = append(c.Programmes, p)
				count++
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, ErrUnavailable
		}
		// Programme images (BEAPI-1), one query for the page like the
		// programmes above, keyed by generation and programme.
		type programmeKey struct{ generation, id string }
		digests := map[programmeKey]string{}
		ids := []string{}
		for i := range out.Channels {
			for _, p := range out.Channels[i].Programmes {
				ids = append(ids, p.ID)
			}
		}
		if len(ids) > 0 {
			encoded, e := json.Marshal(ids)
			if e != nil {
				return out, ErrUnavailable
			}
			imgArgs := append(append(make([]any, 0, len(gens)+1), args[:len(gens)]...), string(encoded))
			imgRows, e := tx.QueryContext(ctx, `SELECT i.generation_id,i.programme_id,img.digest FROM live_programme_icons i JOIN live_programme_images img ON img.url=i.url WHERE i.generation_id IN (`+marks+`) AND i.programme_id IN(SELECT value FROM json_each(?))`, imgArgs...)
			if e != nil {
				return out, ErrUnavailable
			}
			for imgRows.Next() {
				var gen, pid, digest string
				if e = imgRows.Scan(&gen, &pid, &digest); e != nil {
					imgRows.Close()
					return out, ErrUnavailable
				}
				digests[programmeKey{gen, pid}] = digest
			}
			e = imgRows.Err()
			imgRows.Close()
			if e != nil {
				return out, ErrUnavailable
			}
		}
		// The image is set before Admit: a refused programme is reduced to its
		// slot, which never shows it.
		for i := range out.Channels {
			c := &out.Channels[i]
			for j := range c.Programmes {
				if digest, ok := digests[programmeKey{c.Generation, c.Programmes[j].ID}]; ok {
					c.Programmes[j].Image = "/v1/guide/images/" + digest
				}
				c.Programmes[j].Admit(q.ProgrammeAllowed)
			}
		}
	}
	out.State = "ready"
	if len(out.Channels) == 0 {
		out.State = "filter-empty"
		if more {
			out.State = "continuation-required"
		} else if len(keys) == 0 {
			out.State = "no-sources"
		}
	} else if count == 0 {
		out.State = "no-guide-data"
	} else {
		for _, source := range out.Sources {
			if source.RefreshState == "degraded" || source.RefreshState == "credentials-required" {
				out.State = "guide-stale"
				break
			}
		}
	}
	out.Days = GuideDays(out.Sources, time.Now())
	return out, nil
}

// ContinuationKey derives a domain-separated server-only signing key. Never put
// it in a consumer DTO. This does not export the protected source encryption key.
func (s *Store) ContinuationKey(domain string) []byte {
	h := hmac.New(sha256.New, s.cursorKey)
	h.Write([]byte("portico/continuation/" + domain))
	return h.Sum(nil)
}
