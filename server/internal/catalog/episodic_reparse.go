package catalog

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"

	"portico.local/server/internal/dbwork"
)

// EpisodeParserVersion names how ParseEpisode reads a file name. Bump it with
// every change to that reading: sources the old reading left with a naming
// issue are read again once (ReparseEpisodicIssues), because a scan never
// re-reads a file whose directory hasn't changed.
//
//	1: the original reader.
//	2: NEW-26, only a trailing stack suffix ("- pt1", ".part2", "cd1") is a
//	   stacked part; "Jupiter Jazz (Part 1)" is a title.
const EpisodeParserVersion = 2

const episodeParserVersionKey = "catalog.episode_parser_version"

// reparseBatch is how many issue rows one read looks at and one write
// re-commits.
const reparseBatch = 100

// ReparseEpisodicIssues reads every automatically parsed episodic source that
// still carries a naming issue again with the current ParseEpisode, once per
// EpisodeParserVersion, and commits the new reading when it differs: a source
// the new reader understands becomes its episodes, exactly as a scan would
// have made them. Manual assignments are never touched. It walks the issue rows
// in keyset batches, reads outside the write, re-checks each row inside the
// short write, yields between batches, and records the version only when the
// walk finishes, so a restart resumes from the start of the (small) issue set.
func (s *Service) ReparseEpisodicIssues(ctx context.Context) error {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key=?`, episodeParserVersionKey).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if v, _ := strconv.Atoi(raw); v >= EpisodeParserVersion {
		return nil
	}
	type candidate struct {
		library, asset, issue, name string
		plan                        EpisodeNaming
	}
	afterLibrary, afterAsset := "", ""
	for {
		if !dbwork.Yield(ctx) {
			return ctx.Err()
		}
		rows, err := s.db.QueryContext(ctx, `SELECT e.library_id,e.asset_id,e.issue,l.kind,l.root,a.path,
 COALESCE((SELECT o.relative_path FROM inventory_objects o JOIN library_sources ls ON ls.id=o.source_id WHERE o.asset_id=e.asset_id AND ls.library_id=e.library_id AND o.retired=0 LIMIT 1),'')
 FROM episodic_sources e JOIN libraries l ON l.id=e.library_id JOIN catalog_assets a ON a.token=e.asset_id
 WHERE (e.library_id,e.asset_id)>(?,?) AND e.issue<>'' AND e.manual=0 ORDER BY e.library_id,e.asset_id LIMIT ?`, afterLibrary, afterAsset, reparseBatch)
		if err != nil {
			return err
		}
		batch, changed := 0, []candidate{}
		for rows.Next() {
			var c candidate
			var kind, root, path, relative string
			if err = rows.Scan(&c.library, &c.asset, &c.issue, &kind, &root, &path, &relative); err != nil {
				rows.Close()
				return err
			}
			batch++
			afterLibrary, afterAsset = c.library, c.asset
			if kind != "tv" && kind != "anime" {
				continue
			}
			if relative == "" {
				if relative, err = filepath.Rel(root, path); err != nil {
					continue
				}
			}
			c.name, c.plan = filepath.Base(path), ParseEpisode(relative, kind)
			if c.plan.Issue != c.issue {
				changed = append(changed, c)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(changed) > 0 {
			err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
				for _, c := range changed {
					var issue string
					var manual int
					err := tx.QueryRowContext(ctx, `SELECT issue,manual FROM episodic_sources WHERE library_id=? AND asset_id=?`, c.library, c.asset).Scan(&issue, &manual)
					if errors.Is(err, sql.ErrNoRows) {
						continue
					}
					if err != nil {
						return err
					}
					if issue != c.issue || manual != 0 {
						continue // assigned or re-read since the batch was read
					}
					if err = s.commitEpisodes(tx, c.library, c.asset, c.name, c.plan, false); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		if batch < reparseBatch {
			break
		}
	}
	_, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassMaintenance, `INSERT INTO configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, episodeParserVersionKey, strconv.Itoa(EpisodeParserVersion))
	return err
}
