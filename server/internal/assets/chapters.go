package assets

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
)

func PersistChapters(tx *sql.Tx, asset string, size, modified int64, f Facts) error {
	if f.ChapterStatus == "" {
		return nil
	}
	if f.ChapterStatus != "known" && f.ChapterStatus != "invalid" {
		return errors.New("invalid chapter status")
	}
	chapters := f.Chapters
	if f.ChapterStatus == "invalid" {
		chapters = nil
	}
	if len(chapters) > 4096 {
		return errors.New("chapter count exceeds capacity")
	}
	last := 0.0
	for i, c := range chapters {
		if math.IsNaN(c.Start) || math.IsNaN(c.End) || math.IsInf(c.Start, 0) || math.IsInf(c.End, 0) || c.Start < 0 || c.End <= c.Start || c.End > f.Duration || i > 0 && c.Start < last {
			return errors.New("invalid chapter timeline")
		}
		last = c.End
	}
	raw, _ := json.Marshal([]any{f.ChapterStatus, chapters})
	hash := sha256.Sum256(raw)
	_, e := tx.Exec(`INSERT INTO asset_chapter_facts VALUES(?,1,?,?,?,?) ON CONFLICT(asset_id) DO UPDATE SET revision=asset_chapter_facts.revision+CASE WHEN asset_chapter_facts.fingerprint!=excluded.fingerprint OR asset_chapter_facts.size!=excluded.size OR asset_chapter_facts.modified_ns!=excluded.modified_ns THEN 1 ELSE 0 END,size=excluded.size,modified_ns=excluded.modified_ns,status=excluded.status,fingerprint=excluded.fingerprint`, asset, size, modified, f.ChapterStatus, hex.EncodeToString(hash[:]))
	if e != nil {
		return e
	}
	if _, e = tx.Exec(`DELETE FROM asset_chapters WHERE asset_id=?`, asset); e != nil {
		return e
	}
	for i, c := range chapters {
		if _, e = tx.Exec(`INSERT INTO asset_chapters VALUES(?,?,?,?,?)`, asset, i+1, streamText(c.Title, 500), c.Start, c.End); e != nil {
			return e
		}
	}
	return nil
}
