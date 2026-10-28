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
	"net/url"
	"strings"

	"portico.local/server/internal/dbwork"
)

// Reuse the same provider/source variant from a retained selection. There is
// no new cache lifetime: its objects live only as long as the usual references.
func (s *Service) sharedArtwork(ctx context.Context, j artworkWork) (artworkInstalled, artworkInstalled, bool, error) {
	var a, b artworkInstalled
	// Existing retained objects do not record their encoding policy. Re-fetch
	// non-standard sizes rather than treating an old 1920 image as a 2560
	// original, or publishing an oversized object under the smaller policy.
	if policy, ok := ctx.Value(artworkEncodingPolicyKey{}).(artworkEncodingPolicy); ok && policy.edge != artworkLargeEdge {
		return a, b, false, nil
	}
	if j.sizeRefresh || j.provider == "local" || j.provider == "upload" || j.actor != "" {
		return a, b, false, nil
	}
	err := dbwork.QueryRow(ctx, s.db, `SELECT o.digest,o.mime,o.width,o.height,o.bytes,t.digest,t.mime,t.width,t.height,t.bytes FROM artwork_candidates c JOIN artwork_selections s ON s.candidate_id=c.id JOIN artwork_objects o ON o.digest=s.digest AND o.status='ready' JOIN artwork_objects t ON t.digest=s.thumbnail_digest AND t.status='ready' WHERE c.provider=? AND c.origin=? LIMIT 1`, j.provider, j.origin).Scan(&a.digest, &a.mime, &a.width, &a.height, &a.size, &b.digest, &b.mime, &b.width, &b.height, &b.size)
	if errors.Is(err, sql.ErrNoRows) {
		return a, b, false, nil
	}
	if err != nil {
		return a, b, false, err
	}
	edge, _ := artworkSlotLimits(ctx, artworkSlotLarge)
	if max(a.width, a.height) < edge && fullArtworkOrigin(j.origin, j.provider) != j.origin {
		// A retained provider thumbnail does not prove the original was small.
		return a, b, false, nil
	}
	for _, o := range []*artworkInstalled{&a, &b} {
		f, e := s.openArtworkObject(o.digest)
		if e != nil {
			return a, b, false, nil
		}
		raw, e := io.ReadAll(io.LimitReader(f, artworkMaxBytes+1))
		f.Close()
		if e != nil || len(raw) > artworkMaxBytes || len(raw) != o.size {
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
	origin := j.origin
	if !j.preview {
		origin = fullArtworkOrigin(origin, j.provider)
	}
	raw, e := s.fetchArtwork(ctx, origin, j.provider)
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

// Use the original of the same admitted TMDB image, never a larger host/origin
// permission. Candidate identity, source fence and stored origin are unchanged.
// Small discovery previews keep their bounded provider representation.
func fullArtworkOrigin(raw, provider string) string {
	if provider != "tmdb" || !artworkURLAllowed(raw, provider) {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/t/p/"), "/", 2)
	if len(parts) != 2 || parts[1] == "" || len(parts[0]) < 2 || len(parts[0]) > 6 || (parts[0][0] != 'w' && parts[0][0] != 'h') || parts[0][1] < '1' || parts[0][1] > '9' {
		return raw
	}
	for _, digit := range parts[0][1:] {
		if digit < '0' || digit > '9' {
			return raw
		}
	}
	u.Path = "/t/p/original/" + parts[1]
	u.RawPath = ""
	return u.String()
}
