package remotesources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// Tokens are provider-private encrypted state, not catalog identifiers or public
// cursors. A token never advances without the exact candidate changes it covers.
func (s *Service) buildDAVStep(ctx context.Context, b binding, relative, id, generation string) error {
	d, gen, err := s.dav(ctx, b.ID)
	if err != nil {
		return err
	}
	if gen != generation {
		return storage.ErrRemoteChanged
	}
	var sealed []byte
	mode, token := "sync", ""
	var steps int
	var restarted bool
	err = s.db.QueryRowContext(ctx, `SELECT token,mode,steps,restarted FROM remote_listing_progress WHERE listing_id=?`, id).Scan(&sealed, &mode, &steps, &restarted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if len(sealed) > 0 {
		raw, e := s.mounts.Open(sealed)
		if e != nil {
			return storage.ErrRemoteCursor
		}
		token = string(raw)
		clear(raw)
	}
	if steps >= 10000 {
		return storage.ErrRemoteLimit
	}
	var page remotemedia.DAVPage
	if mode == "sync" {
		page, err = d.Sync(ctx, relative, token)
	} else {
		page, err = d.List(ctx, relative)
	}
	reset := false
	if errors.Is(err, remotemedia.ErrDAVUnsupported) && token == "" || errors.Is(err, remotemedia.ErrDAVToken) && !restarted {
		// Reset only unpublished state; existing catalog availability is untouched.
		mode = "propfind"
		token = ""
		restarted = true
		reset = true
		err = nil
	}
	if err != nil {
		return err
	}
	if !reset && mode == "sync" && (page.Token == "" || page.More && page.Token == token) {
		return storage.ErrRemoteCursor
	}
	if err = s.check(ctx, b, generation); err != nil {
		return err
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var current string
	var removed bool
	if err = tx.QueryRowContext(ctx, `SELECT CAST(generation AS TEXT),removed FROM remote_sources WHERE id=?`, b.ID).Scan(&current, &removed); err != nil {
		return err
	}
	if removed || current != generation {
		return storage.ErrRemoteChanged
	}
	if reset {
		_, err = tx.ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE listing_id=?`, id)
		if err != nil {
			return err
		}
	} else {
		for _, entry := range page.Entries {
			if entry.Relative == relative {
				continue
			}
			path := filepath.Join(b.Root, filepath.FromSlash(entry.Relative))
			if filepath.Dir(path) != filepath.Join(b.Root, filepath.FromSlash(relative)) {
				return storage.ErrRemoteConfig
			}
			if entry.Deleted {
				_, err = tx.ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE listing_id=? AND name=?`, id, path)
			} else {
				raw, e := json.Marshal(davSnapshot(entry, b.Root))
				if e != nil {
					return e
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO remote_listing_entries VALUES(?,?,?) ON CONFLICT(listing_id,name) DO UPDATE SET snapshot=excluded.snapshot`, id, path, string(raw))
			}
			if err != nil {
				return err
			}
		}
		token = page.Token
	}
	sealed = nil
	if token != "" {
		sealed, err = s.mounts.Seal([]byte(token))
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO remote_listing_progress VALUES(?,?,?,?,?) ON CONFLICT(listing_id) DO UPDATE SET token=excluded.token,mode=excluded.mode,steps=excluded.steps,restarted=excluded.restarted`, id, sealed, mode, steps+1, restarted)
	if err != nil {
		return err
	}
	if !reset && !page.More {
		_, err = tx.ExecContext(ctx, `UPDATE remote_listing_sessions SET ready=1 WHERE id=? AND generation=?`, id, generation)
		if err != nil {
			return err
		}
	}
	return gated.Commit()
}
