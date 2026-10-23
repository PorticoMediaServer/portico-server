package disc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Persist publishes only a complete, validated helper response in the scan's
// existing source-fenced transaction. JSON is used for a deterministic hash,
// never as primary production persistence.
func Persist(ctx context.Context, tx *sql.Tx, asset string, size, modified int64, p *Probe) error {
	if p == nil {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	checked, err := DecodeProbe(raw)
	if err != nil {
		return err
	}
	var current bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_assets WHERE token=? AND size=? AND modified_ns=?)`, asset, size, modified).Scan(&current); err != nil {
		return err
	}
	if !current {
		return ErrProtocol
	}
	digest := sha256.Sum256(raw)
	_, err = tx.ExecContext(ctx, `INSERT INTO asset_discs(asset_id,source_size,source_modified_ns,revision,fingerprint,kind,packaging,encryption) VALUES(?,?,?,1,?,?,?,?) ON CONFLICT(asset_id) DO UPDATE SET source_size=excluded.source_size,source_modified_ns=excluded.source_modified_ns,revision=asset_discs.revision+CASE WHEN asset_discs.fingerprint<>excluded.fingerprint OR asset_discs.source_size<>excluded.source_size OR asset_discs.source_modified_ns<>excluded.source_modified_ns THEN 1 ELSE 0 END,fingerprint=excluded.fingerprint,kind=excluded.kind,packaging=excluded.packaging,encryption=excluded.encryption`, asset, size, modified, hex.EncodeToString(digest[:]), checked.Kind, checked.Packaging, checked.Encryption)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM disc_titles WHERE asset_id=?`, asset); err != nil {
		return err
	}
	for i, t := range checked.OrderedTitles() {
		key := t.Key(checked.Kind)
		var playlist any
		if t.Playlist != nil {
			playlist = *t.Playlist
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO disc_titles VALUES(?,?,?,?,?,?,?,?,?,?,?)`, asset, key, i+1, t.DiscOrder, playlist, t.PlaylistFile, t.Title, t.VTS, t.PGC, t.DurationMs, t.AngleCount); err != nil {
			return err
		}
		for _, c := range t.Clips {
			if _, err = tx.ExecContext(ctx, `INSERT INTO disc_clips VALUES(?,?,?,?,?,?)`, asset, key, c.Sequence, c.File, c.InTime90k, c.OutTime90k); err != nil {
				return err
			}
		}
		for j, s := range t.Streams {
			if _, err = tx.ExecContext(ctx, `INSERT INTO disc_streams VALUES(?,?,?,?,?,?,?,?,?,?)`, asset, key, j, s.ClipSequence, s.PID, s.Kind, s.Coding, s.Language, s.DolbyVisionEnhancementLayer, s.DolbyVisionEnhancementKnown); err != nil {
				return err
			}
		}
		for j, c := range t.Chapters {
			if _, err = tx.ExecContext(ctx, `INSERT INTO disc_chapters VALUES(?,?,?,?)`, asset, key, j, c.StartMs); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM disc_main_titles WHERE asset_id=? AND (source_size<>? OR source_modified_ns<>? OR NOT EXISTS(SELECT 1 FROM disc_titles t WHERE t.asset_id=disc_main_titles.asset_id AND t.title_key=disc_main_titles.title_key))`, asset, size, modified)
	return err
}

type Query interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Load(ctx context.Context, q Query, asset string, size, modified int64) (*Probe, error) {
	var p Probe
	p.Schema = Schema
	p.Titles = []Title{}
	err := q.QueryRowContext(ctx, `SELECT d.kind,d.packaging,d.encryption FROM asset_discs d JOIN catalog_assets a ON a.token=d.asset_id WHERE d.asset_id=? AND d.source_size=? AND d.source_modified_ns=? AND a.size=d.source_size AND a.modified_ns=d.source_modified_ns`, asset, size, modified).Scan(&p.Kind, &p.Packaging, &p.Encryption)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT disc_order,playlist,playlist_file,dvd_title,vts,pgc,duration_ms,angle_count FROM disc_titles WHERE asset_id=? ORDER BY disc_order LIMIT 4097`, asset)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t Title
		var playlist sql.NullInt64
		if err = rows.Scan(&t.DiscOrder, &playlist, &t.PlaylistFile, &t.Title, &t.VTS, &t.PGC, &t.DurationMs, &t.AngleCount); err != nil {
			rows.Close()
			return nil, err
		}
		if playlist.Valid {
			v := int(playlist.Int64)
			t.Playlist = &v
		}
		t.Clips = []Clip{}
		t.Streams = []Stream{}
		t.Chapters = []Chapter{}
		p.Titles = append(p.Titles, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(p.Titles) > MaxTitles {
		return nil, ErrProtocol
	}
	for i := range p.Titles {
		t := &p.Titles[i]
		key := t.Key(p.Kind)
		rows, err = q.QueryContext(ctx, `SELECT sequence,clip_file,in_time_90k,out_time_90k FROM disc_clips WHERE asset_id=? AND title_key=? ORDER BY sequence LIMIT 4097`, asset, key)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c Clip
			if err = rows.Scan(&c.Sequence, &c.File, &c.InTime90k, &c.OutTime90k); err != nil {
				rows.Close()
				return nil, err
			}
			t.Clips = append(t.Clips, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		rows, err = q.QueryContext(ctx, `SELECT clip_sequence,pid,kind,coding,language,dv_enhancement,dv_enhancement_known FROM disc_streams WHERE asset_id=? AND title_key=? ORDER BY ordinal LIMIT 65537`, asset, key)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var s Stream
			if err = rows.Scan(&s.ClipSequence, &s.PID, &s.Kind, &s.Coding, &s.Language, &s.DolbyVisionEnhancementLayer, &s.DolbyVisionEnhancementKnown); err != nil {
				rows.Close()
				return nil, err
			}
			t.Streams = append(t.Streams, s)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		rows, err = q.QueryContext(ctx, `SELECT start_ms FROM disc_chapters WHERE asset_id=? AND title_key=? ORDER BY chapter_index LIMIT 65536`, asset, key)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c Chapter
			if err = rows.Scan(&c.StartMs); err != nil {
				rows.Close()
				return nil, err
			}
			t.Chapters = append(t.Chapters, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if _, err = DecodeProbe(raw); err != nil {
		return nil, err
	}
	return &p, nil
}

// MainTitleKey reads the choice for the same item and unchanged disc. Empty is
// the default first sorted title; changing the choice never renames Title N.
func MainTitleKey(ctx context.Context, q Query, item int64, asset string, size, modified int64) (string, error) {
	var key string
	err := q.QueryRowContext(ctx, `SELECT m.title_key FROM disc_main_titles m JOIN asset_discs d ON d.asset_id=m.asset_id JOIN disc_titles t ON t.asset_id=m.asset_id AND t.title_key=m.title_key WHERE m.item_id=? AND m.asset_id=? AND m.source_size=? AND m.source_modified_ns=? AND d.source_size=m.source_size AND d.source_modified_ns=m.source_modified_ns`, item, asset, size, modified).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return key, err
}

// SetMainTitle belongs inside the existing owner-authorised title mutation.
// The returned duration must be applied through the catalogue's normal source
// fact writer in that same transaction; it is never a provider override.
func SetMainTitle(ctx context.Context, tx *sql.Tx, item int64, asset, key string, size, modified int64) (int64, error) {
	var duration int64
	err := tx.QueryRowContext(ctx, `SELECT t.duration_ms FROM disc_titles t JOIN asset_discs d ON d.asset_id=t.asset_id JOIN catalog_assets a ON a.token=d.asset_id JOIN catalog_asset_links l ON l.asset_id=a.id AND l.entity_id=? WHERE t.asset_id=? AND t.title_key=? AND d.source_size=? AND d.source_modified_ns=? AND a.size=d.source_size AND a.modified_ns=d.source_modified_ns`, item, asset, key, size, modified).Scan(&duration)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO disc_main_titles VALUES(?,?,?,?,?) ON CONFLICT(item_id,asset_id) DO UPDATE SET title_key=excluded.title_key,source_size=excluded.source_size,source_modified_ns=excluded.source_modified_ns`, item, asset, key, size, modified)
	return duration, err
}
