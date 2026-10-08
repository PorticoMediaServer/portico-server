package administration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	// WebP is accepted exactly as the channel logo path accepts it. Nothing is
	// re-encoded: the stored object is the provider's bytes, proved decodable.
	_ "golang.org/x/image/webp"
	"portico.local/server/internal/atomicfile"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

// RunGuideImageImports drains source-publication jobs for guide programme
// images. Saving a source publishes its programmes' icon URLs and enqueues the
// job in the same database transaction. The worker never fetches on a request
// path or polls when there is no work.
//
// A provider URL never appears in a log line, an error message or any
// response: fetch and validation failures are recorded against the URL in the
// database and counted, never returned.
func (s *Service) RunGuideImageImports(ctx context.Context) {
	if s == nil || s.db == nil || s.LogoFetch == nil {
		return
	}
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	wake := worker.NewSignal()
	unregister := dbwork.WakeOnTables(wake, "live_icon_jobs")
	defer unregister()
	for ctx.Err() == nil {
		var source, generation, last string
		err := dbwork.ReadHandle(ctx, s.db).QueryRowContext(ctx,
			`SELECT source_id,generation_id,last_url FROM live_icon_jobs INDEXED BY live_icon_jobs_due WHERE state='pending' AND next_attempt_ms<=? ORDER BY next_attempt_ms,source_id LIMIT 1`, s.milliseconds()).Scan(&source, &generation, &last)
		if err == sql.ErrNoRows {
			var next sql.NullInt64
			if err = dbwork.ReadHandle(ctx, s.db).QueryRowContext(ctx,
				`SELECT min(next_attempt_ms) FROM live_icon_jobs WHERE state='pending'`).Scan(&next); err != nil {
				log.Printf("guide image schedule: %v", err)
				if !wake.Wait(ctx) {
					return
				}
				continue
			}
			if !next.Valid {
				if !wake.Wait(ctx) {
					return
				}
				continue
			}
			wait := time.Until(time.UnixMilli(next.Int64))
			if wait > 0 {
				waitCtx, cancel := context.WithTimeout(ctx, wait)
				_ = wake.Wait(waitCtx)
				cancel()
			}
			continue
		}
		if err != nil {
			log.Printf("guide image queue: %v", err)
			if !wake.Wait(ctx) {
				return
			}
			continue
		}
		batchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		result, nextURL, done, importErr := s.importGuideImagesBatch(batchCtx, source, generation, last, 16)
		batchErr := batchCtx.Err()
		cancel()
		state, nextAttempt := "pending", int64(0)
		if done {
			state = "complete"
		}
		if importErr != nil {
			if ctx.Err() == nil && (batchErr == context.DeadlineExceeded || errors.Is(importErr, context.DeadlineExceeded)) {
				nextAttempt = s.milliseconds() + 60_000
			} else {
				state = "failed"
			}
			if ctx.Err() == nil {
				log.Printf("guide image import: %v", importErr)
			}
		}
		if ctx.Err() != nil {
			return
		}
		if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia,
			`UPDATE live_icon_jobs SET state=?,imported=imported+?,skipped=skipped+?,last_url=?,next_attempt_ms=? WHERE source_id=? AND generation_id=? AND state='pending'`,
			state, result.Imported, result.Skipped, nextURL, nextAttempt, source, generation); err != nil {
			log.Printf("guide image receipt: %v", err)
			if !wake.Wait(ctx) {
				return
			}
			continue
		}
		if state == "complete" {
			s.cleanupGuideImages(ctx)
		}
	}
}

// importGuideImagesBatch snapshots up to limit distinct icon URLs of one job's
// generation that have no stored image yet, then fetches and stores them after
// the snapshot closes, so a slow provider never holds a SQLite connection or
// the write gate. URLs whose failure backoff has not expired are skipped. The
// cursor advances past every URL it visits, so a batch always makes progress.
func (s *Service) importGuideImagesBatch(ctx context.Context, source, generation, after string, limit int) (LogoImportResult, string, bool, error) {
	out := LogoImportResult{SourceID: source}
	if source == "" || generation == "" || limit < 0 {
		return out, after, false, ErrInput
	}
	if s.LogoFetch == nil {
		out.Message = "This server is not configured to fetch remote images."
		return out, after, true, nil
	}
	candidates := []string{}
	skipped := map[string]bool{}
	err := s.snapshot(ctx, nil, func(tx *sql.Tx) error {
		document, err := s.liveSourceDocument(ctx, tx, source)
		if err != nil {
			return err
		}
		if !document.Effective.LogoImport {
			out.Message = "Logo import is switched off for this source."
			return nil
		}
		if !tableExists(ctx, tx, "live_programme_icons") {
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT i.url FROM live_programme_icons i WHERE i.generation_id=? AND i.url>? AND NOT EXISTS(SELECT 1 FROM live_programme_images m WHERE m.url=i.url) ORDER BY i.url LIMIT ?`, generation, after, limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var url string
			if err = rows.Scan(&url); err != nil {
				break
			}
			candidates = append(candidates, url)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		rows, err = tx.QueryContext(ctx, `SELECT DISTINCT i.url FROM live_programme_icons i JOIN live_logo_failures f ON f.url=i.url WHERE i.generation_id=? AND f.retry_after_ms>?`, generation, s.milliseconds())
		if err != nil {
			return err
		}
		for rows.Next() {
			var url string
			if err = rows.Scan(&url); err != nil {
				break
			}
			skipped[url] = true
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		return err
	})
	if err != nil {
		return out, after, false, err
	}
	for _, url := range candidates {
		if err = ctx.Err(); err != nil {
			return out, after, false, err
		}
		if skipped[url] {
			after = url
			out.Skipped++
			continue
		}
		out.Requested++
		fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		raw, fetchErr := s.LogoFetch(fetchCtx, url)
		cancel()
		if fetchErr != nil {
			if ctx.Err() != nil {
				return out, after, false, ctx.Err()
			}
			if err = s.rememberLogoFailure(ctx, url); err != nil {
				return out, after, false, err
			}
			after = url
			out.Skipped++
			continue
		}
		if !validGuideImage(raw) {
			if err = s.rememberLogoFailure(ctx, url); err != nil {
				return out, after, false, err
			}
			after = url
			out.Skipped++
			continue
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || config.Width < minLogoPixels || config.Height < minLogoPixels || config.Width > maxLogoPixels || config.Height > maxLogoPixels {
			if rememberErr := s.rememberLogoFailure(ctx, url); rememberErr != nil {
				return out, after, false, rememberErr
			}
			after = url
			out.Skipped++
			continue
		}
		mime := sniffLogo(raw)
		sum := sha256.Sum256(raw)
		digest := hex.EncodeToString(sum[:])
		root, err := s.guideImageRoot()
		if err != nil {
			return out, after, false, err
		}
		path := filepath.Join(root, digest+logoExtension(mime))
		s.files.Lock()
		if _, err = os.Stat(path); err != nil {
			// The image is addressed by the digest of its own bytes, so a
			// reader that finds this name expects exactly these bytes. A
			// partial write would be a file whose name is a promise it does
			// not keep.
			if err = atomicfile.Write(path, raw, 0600); err != nil {
				s.files.Unlock()
				return out, after, false, ErrUnavailable
			}
		}
		s.files.Unlock()
		if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `INSERT INTO live_programme_images(url,digest,media_type,width,height,stored_ms) VALUES(?,?,?,?,?,?) ON CONFLICT(url) DO NOTHING`, url, digest, mime, config.Width, config.Height, s.milliseconds()); err != nil {
			return out, after, false, err
		}
		if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM live_logo_failures WHERE url=?`, url); err != nil {
			return out, after, false, err
		}
		after = url
		out.Imported++
	}
	return out, after, len(candidates) < limit, nil
}

// validGuideImage proves the bytes are an image the server accepts, exactly as
// the channel logo upload does. Decoding happens before any transaction opens,
// so a hostile image never holds a write lock.
func validGuideImage(raw []byte) bool {
	if len(raw) == 0 || len(raw) > LogoUploadBytes {
		return false
	}
	return sniffLogo(raw) != ""
}

func (s *Service) guideImageRoot() (string, error) {
	if s.StateDirectory() == "" {
		return "", ErrUnavailable
	}
	root := filepath.Join(s.StateDirectory(), "guide-images")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", ErrUnavailable
	}
	return root, nil
}

// GuideImageBytes reads a stored programme image for delivery. Images are
// addressed by the digest of their own bytes, so a programme the profile may
// not see is never published with its image path; this read itself carries no
// programme identity.
func (s *Service) GuideImageBytes(ctx context.Context, digest string) ([]byte, string, error) {
	if s == nil || s.db == nil {
		return nil, "", ErrUnavailable
	}
	var mime string
	err := s.db.QueryRowContext(ctx, `SELECT media_type FROM live_programme_images WHERE digest=? LIMIT 1`, digest).Scan(&mime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	root, err := s.guideImageRoot()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(filepath.Join(root, digest+logoExtension(mime)))
	if err != nil {
		return nil, "", ErrNotFound
	}
	return raw, mime, nil
}

// cleanupGuideImages drops stored images no programme icon references any
// more, then removes the files whose digest no row references. Only names
// shaped like <64 hex><ext> are ever removed; anything else in the directory
// is left alone.
func (s *Service) cleanupGuideImages(ctx context.Context) {
	if _, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia,
		`DELETE FROM live_programme_images WHERE NOT EXISTS(SELECT 1 FROM live_programme_icons i WHERE i.url=live_programme_images.url)`); err != nil {
		log.Printf("guide image cleanup: %v", err)
		return
	}
	root, err := s.guideImageRoot()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if entry.IsDir() {
			continue
		}
		digest, ok := guideImageName(entry.Name())
		if !ok {
			continue
		}
		var referenced bool
		if err := dbwork.ReadHandle(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_programme_images WHERE digest=?)`, digest).Scan(&referenced); err != nil {
			log.Printf("guide image cleanup: %v", err)
			return
		}
		if !referenced {
			_ = os.Remove(filepath.Join(root, entry.Name()))
		}
	}
}

// guideImageName splits a stored image file name into its content digest. Only
// the names this worker writes are recognised.
func guideImageName(name string) (string, bool) {
	var ext string
	switch {
	case strings.HasSuffix(name, ".png"):
		ext = ".png"
	case strings.HasSuffix(name, ".jpg"):
		ext = ".jpg"
	case strings.HasSuffix(name, ".webp"):
		ext = ".webp"
	default:
		return "", false
	}
	digest := strings.TrimSuffix(name, ext)
	if len(digest) != 64 {
		return "", false
	}
	for _, r := range digest {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", false
		}
	}
	return digest, true
}
