package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"io"

	"portico.local/server/internal/dbwork"
)

// Reuse the same provider/source variant from a retained selection. There is
// no new cache lifetime: its objects live only as long as the usual references.
func (s *Service) sharedArtwork(ctx context.Context, j artworkWork) (artworkInstalled, artworkInstalled, bool, error) {
	var a, b artworkInstalled
	if j.provider == "local" || j.provider == "upload" || j.actor != "" {
		return a, b, false, nil
	}
	err := dbwork.QueryRow(ctx, s.db, `SELECT o.digest,o.mime,o.width,o.height,o.bytes,t.digest,t.mime,t.width,t.height,t.bytes FROM artwork_candidates c JOIN artwork_selections s ON s.candidate_id=c.id JOIN artwork_objects o ON o.digest=s.digest AND o.status='ready' JOIN artwork_objects t ON t.digest=s.thumbnail_digest AND t.status='ready' WHERE c.provider=? AND c.origin=? LIMIT 1`, j.provider, j.origin).Scan(&a.digest, &a.mime, &a.width, &a.height, &a.size, &b.digest, &b.mime, &b.width, &b.height, &b.size)
	if errors.Is(err, sql.ErrNoRows) {
		return a, b, false, nil
	}
	if err != nil {
		return a, b, false, err
	}
	for _, o := range []*artworkInstalled{&a, &b} {
		f, e := s.openArtworkObject(o.digest)
		if e != nil {
			return a, b, false, nil
		}
		raw, e := io.ReadAll(io.LimitReader(f, artworkDisplayBytes+1))
		f.Close()
		if e != nil || len(raw) > artworkDisplayBytes || len(raw) != o.size {
			return a, b, false, nil
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != o.digest {
			return a, b, false, nil
		}
		o.raw = raw
	}
	return a, b, true, nil
}
func (s *Service) acquireArtworkObjects(ctx context.Context, j artworkWork) (artworkInstalled, artworkInstalled, error) {
	a, b, held, e := s.sharedArtwork(ctx, j)
	if e != nil || held {
		return a, b, e
	}
	raw, e := s.fetchArtwork(ctx, j.origin, j.provider)
	if e != nil {
		return a, b, e
	}
	original, thumb, w, h, e := normalizeArtworkFormatsContext(ctx, raw, map[string]bool{"jpeg": true, "png": true})
	if e != nil {
		return a, b, e
	}
	a, e = s.installArtworkContext(ctx, original, w, h)
	if e != nil {
		return a, b, errors.New("storage_unavailable")
	}
	tc, _, e := image.DecodeConfig(bytes.NewReader(thumb))
	if e != nil {
		return a, b, e
	}
	b, e = s.installArtworkContext(ctx, thumb, tc.Width, tc.Height)
	if e != nil {
		return a, b, errors.New("storage_unavailable")
	}
	return a, b, nil
}
