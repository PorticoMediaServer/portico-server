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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/entityid"
	"regexp"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diskspace"
	"portico.local/server/internal/identity"
)

const artworkBytes = 8 << 20

var archiveItemPath = regexp.MustCompile(`^/[0-9]+/items/mbid-`)
var archiveHost = regexp.MustCompile(`^ia[0-9]+\.(us|eu)\.archive\.org$`)

func artworkURLAllowed(raw, provider string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawQuery != "" || strings.Contains(u.Path, "..") || strings.ContainsAny(u.Path, "\\\x00") || u.RawPath != "" {
		return false
	}
	switch provider {
	case "tmdb":
		return u.Host == "image.tmdb.org" && strings.HasPrefix(u.Path, "/t/p/")
	case "tvdb":
		return u.Host == "artworks.thetvdb.com" && strings.HasPrefix(u.Path, "/banners/")
	case "coverartarchive":
		return (u.Host == "coverartarchive.org" && strings.HasPrefix(u.Path, "/release/")) || ((u.Host == "archive.org" || archiveHost.MatchString(u.Host)) && (strings.HasPrefix(u.Path, "/download/mbid-") || archiveItemPath.MatchString(u.Path)))
	case "commons":
		return u.Host == "upload.wikimedia.org" && strings.HasPrefix(u.Path, "/wikipedia/commons/")
	}
	return false
}

// DNS is resolved at the dial boundary, so an allowed hostname cannot rebind to
// a private address between validation and connect. No proxy or credentials.
func newArtworkClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.MaxConnsPerHost = 2
	tr.MaxResponseHeaderBytes = 64 << 10
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Second
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, ErrArtworkPending
		}
		for _, ip := range ips {
			v := ip.IP
			if !v.IsGlobalUnicast() || v.IsPrivate() || v.IsLoopback() || v.IsLinkLocalUnicast() || v.To4() != nil && (v.To4()[0] == 100 && v.To4()[1] >= 64 && v.To4()[1] <= 127 || v.To4()[0] == 198 && (v.To4()[1] == 18 || v.To4()[1] == 19)) {
				return nil, errors.New("non-public artwork origin")
			}
		}
		dial := net.Dialer{Timeout: 10 * time.Second}
		var last error
		for _, ip := range ips {
			conn, e := dial.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	return &http.Client{Transport: tr, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (s *Service) fetchArtwork(ctx context.Context, origin, provider string) ([]byte, error) {
	if provider == "local" {
		if !strings.HasPrefix(origin, "local:") || s.LocalArtwork == nil {
			return nil, ErrArtworkPending
		}
		f, _, err := s.LocalArtwork(strings.TrimPrefix(origin, "local:"))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return boundedArtworkRead(f)
	}
	return s.fetchArtworkRemote(ctx, origin, provider, artworkBytes, "image/*")
}
func (s *Service) fetchArtworkRemote(ctx context.Context, origin, provider string, limit int64, accept string) ([]byte, error) {
	client := s.artClient
	if client == nil {
		client = newArtworkClient()
	}
	// Redirects are explicitly revalidated; caller/provider headers are never forwarded.
	for n := 0; n <= 4; n++ {
		if !artworkURLAllowed(origin, provider) {
			return nil, errors.New("origin_rejected")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", origin, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Portico/0.1 (https://getportico.tv)")
		req.Header.Set("Accept", accept)
		response, err := client.Do(req)
		if err != nil {
			return nil, errors.New("offline")
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			next, e := response.Location()
			response.Body.Close()
			if e != nil || n == 4 {
				return nil, errors.New("redirect_rejected")
			}
			origin = next.String()
			continue
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			if response.StatusCode == 429 {
				return nil, &artworkRetry{code: "rate_limited", after: artworkRetryAfter(response.Header.Get("Retry-After"), s.publicationTime())}
			}
			if response.StatusCode == 404 {
				return nil, errors.New("not_found")
			}
			return nil, errors.New("provider_unavailable")
		}
		raw, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if int64(len(raw)) > limit {
			e = errors.New("size_limit")
			raw = nil
		}
		response.Body.Close()
		return raw, e
	}
	return nil, ErrArtworkPending
}

type artworkRetry struct {
	code  string
	after time.Time
}

func (e *artworkRetry) Error() string { return e.code }
func artworkRetryAfter(value string, now time.Time) time.Time {
	if n, e := strconv.Atoi(value); e == nil && n >= 0 {
		return now.Add(time.Duration(min(n, 86400)) * time.Second)
	}
	if v, e := http.ParseTime(value); e == nil && v.After(now) {
		if v.After(now.Add(24 * time.Hour)) {
			return now.Add(24 * time.Hour)
		}
		return v
	}
	return now.Add(time.Minute)
}
func boundedArtworkRead(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, artworkBytes+1))
	if e != nil {
		return nil, e
	}
	if len(b) > artworkBytes {
		return nil, errors.New("size_limit")
	}
	return b, nil
}

type artworkInstalled struct {
	digest, mime  string
	width, height int
	size          int
	medium        *artworkInstalled
	// raw keeps the bytes until the referencing transaction has committed, so
	// a concurrently retired file can be reinstalled (artwork_lifecycle.go).
	raw []byte
}

// thumbnailBounds contains a thumbnail inside 400x400 without cropping.
func thumbnailBounds(w, h int) (int, int) {
	if w <= 400 && h <= 400 {
		return w, h
	}
	if w >= h {
		return 400, max(1, h*400/w)
	}
	return max(1, w*400/h), 400
}

func normalizeArtwork(raw []byte) ([]byte, []byte, int, int, error) {
	return normalizeArtworkFormats(raw, map[string]bool{"jpeg": true, "png": true})
}

func normalizeArtworkFormats(raw []byte, allowed map[string]bool) ([]byte, []byte, int, int, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || !allowed[format] {
		return nil, nil, 0, 0, errors.New("unsupported_image")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 24000000 {
		return nil, nil, 0, 0, errors.New("dimension_limit")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, 0, 0, errors.New("malformed_image")
	}
	// Keep bounded display representations, never the provider original.
	original, w, h, err := encodeDisplayArtwork(decoded, artworkLargeEdge, format == "jpeg")
	if err != nil {
		return nil, nil, 0, 0, err
	}
	thumb, _, _, err := encodeDisplayArtwork(decoded, artworkSmallEdge, format == "jpeg")
	return original, thumb, w, h, err
}

// ErrArtworkSpace is a refusal, not a failure: the image is fine, the volume is
// not. The work stays queued and runs once there is room, because artwork that
// fills the disk holding server.sqlite costs far more than artwork that arrives
// late.
var ErrArtworkSpace = errors.New("not enough free space to store artwork")

func (s *Service) installArtwork(raw []byte, w, h int) (artworkInstalled, error) {
	a, err := s.installArtworkFile(raw, w, h)
	if err != nil || max(w, h) <= 400 {
		return a, err
	}
	decoded, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return a, err
	}
	medium, mw, mh, err := encodeDisplayArtwork(decoded, 800, format == "jpeg")
	if err != nil {
		return a, err
	}
	m, err := s.installArtworkFile(medium, mw, mh)
	if err != nil {
		return a, err
	}
	a.medium = &m
	return a, nil
}

func recordArtworkObject(ctx context.Context, tx *sql.Tx, o artworkInstalled, stamp string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO artwork_objects VALUES(?,?,?,?,?,?,'ready') ON CONFLICT(digest) DO UPDATE SET status='ready'`, o.digest, o.mime, o.width, o.height, o.size, stamp); err != nil {
		return err
	}
	if o.medium != nil {
		if err := recordArtworkObject(ctx, tx, *o.medium, stamp); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO artwork_variants(source_digest,width,digest) VALUES(?,800,?) ON CONFLICT(source_digest,width) DO UPDATE SET digest=excluded.digest`, o.digest, o.medium.digest)
		return err
	}
	return nil
}

func linkArtworkBuckets(ctx context.Context, tx *sql.Tx, full, thumbnail artworkInstalled) error {
	for _, bucket := range []struct {
		width  int
		digest string
	}{{400, thumbnail.digest}, {1920, full.digest}} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO artwork_variants(source_digest,width,digest) VALUES(?,?,?) ON CONFLICT(source_digest,width) DO NOTHING`, full.digest, bucket.width, bucket.digest); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) installArtworkFile(raw []byte, w, h int) (artworkInstalled, error) {
	sum := sha256.Sum256(raw)
	a := artworkInstalled{digest: hex.EncodeToString(sum[:]), mime: http.DetectContentType(raw), width: w, height: h, size: len(raw), raw: raw}
	if !diskspace.Room(s.cacheRoot, diskspace.ProducerFloor) {
		return a, ErrArtworkSpace
	}
	target := filepath.Join(s.cacheRoot, a.digest+".img")
	f, err := os.CreateTemp(s.cacheRoot, "stage-")
	if err != nil {
		return a, err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return a, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return a, err
	}
	if err = f.Close(); err != nil {
		return a, err
	}
	if existing, e := s.openArtworkObject(a.digest); e == nil {
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(existing, int64(len(raw))+1))
		existing.Close()
		if e == nil && n == int64(len(raw)) && hex.EncodeToString(h.Sum(nil)) == a.digest {
			return a, nil
		}
	}
	if err = os.Rename(temp, target); err != nil {
		return a, err
	}
	dir, err := os.Open(s.cacheRoot)
	if err != nil {
		return a, err
	}
	err = dir.Sync()
	dir.Close()
	return a, err
}

type artworkWork struct {
	id, lease                                                string
	target                                                   RepairTarget
	role, subject, candidate, origin, provider, fence, actor string
	revision                                                 int64
	preview                                                  bool
	locked                                                   bool
	attempts                                                 int
}

func (s *Service) ArtworkStep(ctx context.Context) error {
	if s.cacheRoot == "" || !diskspace.Room(s.cacheRoot, diskspace.ProducerFloor) {
		// Wait for capacity before acquiring a lease or downloading any bytes.
		return nil
	}
	if err := s.seedArtwork(ctx); err != nil {
		return err
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var j artworkWork
	var entity int64
	now := s.publicationTime()
	stamp := now.Format(time.RFC3339Nano)
	err = tx.QueryRowContext(ctx, `SELECT id,kind,entity_id,role,subject,candidate_id,origin,provider,source_fence,selection_revision,actor,locked,attempts,preview FROM artwork_jobs WHERE ((status IN('pending','retry') AND next_attempt<=?) OR (status='running' AND lease_until<=?) OR (status='failed' AND error='storage_unavailable' AND next_attempt<=?)) ORDER BY CASE WHEN role IN('poster','cover','backdrop') THEN 0 ELSE 1 END,created_at,id LIMIT 1`, stamp, stamp, stamp).Scan(&j.id, &j.target.Kind, &entity, &j.role, &j.subject, &j.candidate, &j.origin, &j.provider, &j.fence, &j.revision, &j.actor, &j.locked, &j.attempts, &j.preview)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if j.target.ID, err = entityid.Public(ctx, tx, entity); errors.Is(err, entityid.ErrNotFound) {
		// The entity is gone; retire its job instead of wedging the step.
		if _, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='stale',error='entity_missing',lease='',lease_until='' WHERE id=?`, j.id); err != nil {
			return err
		}
		return gated.Commit()
	} else if err != nil {
		return err
	}
	enabled, err := s.artworkProviderEnabled(ctx, tx, j.target, j.provider)
	if err != nil {
		return err
	}
	if !enabled {
		_, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='disabled',error='provider_disabled',lease='',lease_until='' WHERE id=?`, j.id)
		if err != nil {
			return err
		}
		return gated.Commit()
	}
	fence, err := artworkFence(ctx, tx, j.target)
	if err != nil {
		return err
	}
	if fence != j.fence {
		_, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='stale',error='source_or_match_changed' WHERE id=?`, j.id)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO artwork_dirty VALUES(?,?)`, j.target.Kind, entity); err != nil {
			return err
		}
		return gated.Commit()
	}
	j.lease = identity.Token()
	_, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='running',lease=?,lease_until=?,attempts=attempts+1 WHERE id=?`, j.lease, now.Add(2*time.Minute).Format(time.RFC3339Nano), j.id)
	if err != nil {
		return err
	}
	if err = gated.Commit(); err != nil {
		return err
	}
	workCtx, cancel := s.artworkRequestContext(ctx, j.target, j.provider, j.fence)
	defer cancel()
	raw, err := s.fetchArtwork(workCtx, j.origin, j.provider)
	if err != nil {
		return s.failArtwork(ctx, j, err)
	}
	original, thumb, w, h, err := normalizeArtwork(raw)
	if err != nil {
		return s.failArtwork(ctx, j, err)
	}
	// No lock: files are installed first, the publication commits, and any file
	// a concurrent retirement moved aside is reinstalled (artwork_lifecycle.go).
	a, err := s.installArtwork(original, w, h)
	if err != nil {
		return s.failArtwork(ctx, j, errors.New("storage_unavailable"))
	}
	tc, _, _ := image.DecodeConfig(bytes.NewReader(thumb))
	b, err := s.installArtwork(thumb, tc.Width, tc.Height)
	if err != nil {
		return s.failArtwork(ctx, j, errors.New("storage_unavailable"))
	}
	var gated2 *dbwork.Write
	gated2, err = dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx = gated2.Tx()
	defer gated2.Rollback()
	commit := func() error {
		if err := gated2.Commit(); err != nil {
			return err
		}
		return s.ensureArtworkFiles(a, b)
	}
	var live bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_jobs WHERE id=? AND status='running' AND lease=? AND lease_until>?)`, j.id, j.lease, s.publicationTime().Format(time.RFC3339Nano)).Scan(&live)
	if err != nil {
		return err
	}
	if !live {
		return nil
	}
	fence, err = artworkFence(ctx, tx, j.target)
	if err != nil {
		return err
	}
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM artwork_selections WHERE kind=? AND entity_id=? AND role=? AND subject=?),0)`, j.target.Kind, entity, j.role, j.subject).Scan(&revision); err != nil {
		return err
	}
	enabled, err = s.artworkProviderEnabled(ctx, tx, j.target, j.provider)
	if err != nil {
		return err
	}
	if fence != j.fence || (!j.preview && revision != j.revision) || !enabled {
		_, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='stale',error='source_match_or_selection_changed',lease='',lease_until='' WHERE id=? AND lease=?`, j.id, j.lease)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO artwork_dirty VALUES(?,?)`, j.target.Kind, entity); err != nil {
			return err
		}
		return commit()
	}
	before, ent, err := readRepairSnapshot(ctx, tx, j.target)
	if err != nil {
		return err
	}
	base, err := repairRevision(ctx, tx, j.target, before, ent)
	if err != nil {
		return err
	}
	stamp = s.publicationTime().Format(time.RFC3339Nano)
	for _, o := range []artworkInstalled{a, b} {
		if err = recordArtworkObject(ctx, tx, o, stamp); err != nil {
			return err
		}
	}
	if j.preview {
		if _, err = tx.ExecContext(ctx, `INSERT INTO artwork_previews VALUES(?,?,?,?) ON CONFLICT(candidate_id) DO UPDATE SET digest=excluded.digest,source_fence=excluded.source_fence,created_at=excluded.created_at`, j.candidate, b.digest, j.fence, stamp); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='complete',lease='',lease_until='',error='' WHERE id=? AND lease=?`, j.id, j.lease); err != nil {
			return err
		}
		return commit()
	}
	if err = linkArtworkBuckets(ctx, tx, a, b); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artwork_selections VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(kind,entity_id,role,subject) DO UPDATE SET candidate_id=excluded.candidate_id,digest=excluded.digest,thumbnail_digest=excluded.thumbnail_digest,locked=excluded.locked,revision=artwork_selections.revision+1,actor=excluded.actor,observed_at=excluded.observed_at`, j.target.Kind, entity, j.role, j.subject, j.candidate, a.digest, b.digest, j.locked, 1, j.actor, stamp); err != nil {
		return err
	}
	if err = projectSelectedArtwork(ctx, tx, j.target, j.role); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='complete',lease='',lease_until='',error='',selection_revision=? WHERE id=? AND lease=?`, revision+1, j.id, j.lease); err != nil {
		return err
	}
	if j.actor != "" {
		if err = recordRepair(ctx, tx, j.target, before, base, "artwork_published", j.actor); err != nil {
			return err
		}
	}
	return commit()
}
func (s *Service) failArtwork(ctx context.Context, j artworkWork, problem error) error {
	code := problem.Error()
	allowed := map[string]bool{"offline": true, "rate_limited": true, "not_found": true, "origin_rejected": true, "redirect_rejected": true, "size_limit": true, "unsupported_image": true, "dimension_limit": true, "malformed_image": true, "normalized_size_limit": true, "storage_unavailable": true, "provider_unavailable": true}
	if !allowed[code] {
		code = "acquisition_failed"
	}
	status := "retry"
	if (j.attempts >= 5 && code != "storage_unavailable") || code == "origin_rejected" || code == "unsupported_image" || code == "dimension_limit" {
		status = "failed"
	}
	delay := time.Duration(30*(1<<min(j.attempts, 8))) * time.Second
	if code == "storage_unavailable" {
		delay = 5 * time.Minute
	}
	next := s.publicationTime().Add(delay)
	var retry *artworkRetry
	if errors.As(problem, &retry) && retry.after.After(next) {
		next = retry.after
	}
	_, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE artwork_jobs SET status=?,error=?,next_attempt=?,lease='',lease_until='' WHERE id=? AND lease=? AND status='running'`, status, code, next.Format(time.RFC3339Nano), j.id, j.lease)
	return err
}
func projectSelectedArtwork(ctx context.Context, tx *sql.Tx, t RepairTarget, role string) error {
	entity, err := artworkEntity(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	// The projection writes the details column directly with the same guard the
	// old items UPDATE had; it does not go through the write API, so an owner
	// lock on poster_url/backdrop_url is not consulted here (same as before).
	// The direct write queues its derived work explicitly below.
	if t.Kind == "item" && (role == "poster" || role == "backdrop" || role == "cover") {
		col := "poster_url"
		if role == "backdrop" {
			col = "backdrop_url"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_item_details(entity_id,`+col+`) VALUES(?,?) ON CONFLICT(entity_id) DO UPDATE SET `+col+`=excluded.`+col+` WHERE catalog_item_details.`+col+`=''`, entity, "artwork:"+role); err != nil {
			return err
		}
		return compactcatalog.TouchTx(ctx, tx, compactcatalog.DomainBrowseRows, entity)
	}
	if t.Kind == "book" && role == "cover" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_item_details(entity_id,poster_url) SELECT entity_id,? FROM catalog_book_files WHERE book_id=? ON CONFLICT(entity_id) DO UPDATE SET poster_url=excluded.poster_url WHERE catalog_item_details.poster_url='' OR catalog_item_details.poster_url LIKE 'artwork:book:%'`, "artwork:book:"+t.ID, entity); err != nil {
			return err
		}
		members, err := artworkMemberEntities(ctx, tx, `SELECT entity_id FROM catalog_book_files WHERE book_id=?`, entity)
		if err != nil {
			return err
		}
		return compactcatalog.TouchTx(ctx, tx, compactcatalog.DomainBrowseRows, members...)
	}
	if t.Kind == "album" && role == "cover" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_item_details(entity_id,poster_url) SELECT entity_id,? FROM catalog_songs WHERE album_id=? ON CONFLICT(entity_id) DO UPDATE SET poster_url=excluded.poster_url WHERE catalog_item_details.poster_url='' OR catalog_item_details.poster_url LIKE 'artwork:album:%'`, "artwork:album:"+t.ID, entity); err != nil {
			return err
		}
		members, err := artworkMemberEntities(ctx, tx, `SELECT entity_id FROM catalog_songs WHERE album_id=?`, entity)
		if err != nil {
			return err
		}
		return compactcatalog.TouchTx(ctx, tx, compactcatalog.DomainBrowseRows, members...)
	}
	return nil
}

// artworkMemberEntities lists the member entities of a container for the
// projection's derived-work touch.
func artworkMemberEntities(ctx context.Context, tx *sql.Tx, query string, container int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, container)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GC is reference-aware, not an LRU over selected artwork. Restorable snapshots
// and active publications pin objects. Open readers own their descriptors; on
// platforms forbidding unlink of an open file, deletion is retried later.
func (s *Service) CleanupArtwork(ctx context.Context) error {
	if s.cacheRoot == "" {
		return nil
	}
	if _, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM artwork_previews WHERE created_at<?`, s.publicationTime().Add(-7*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	var running bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_jobs WHERE status='running')`).Scan(&running); err != nil || running {
		return err
	}
	// Candidates are found with indexed anti-joins only (one
	// NOT EXISTS per referencing column, never an OR across two). The owner
	// history is small and JSON, so it is checked here rather than by instr()
	// per object. Each candidate is re-checked by the database while its file is
	// set aside, with no lock (artwork_lifecycle.go).
	cut := s.publicationTime().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `SELECT digest FROM artwork_objects o WHERE created_at<? AND `+artUnreferenced+` ORDER BY created_at LIMIT 64`, cut)
	if err != nil {
		return err
	}
	candidates := []string{}
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	digests := make([]string, 0, len(candidates))
	if len(candidates) > 0 {
		history, err := s.ownerHistoryText(ctx)
		if err != nil {
			return err
		}
		for _, d := range candidates {
			if !strings.Contains(history, d) {
				digests = append(digests, d)
			}
		}
	}
	for _, d := range digests {
		if !artDigest(d) {
			continue
		}
		marked, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE artwork_objects SET status='deleting' WHERE digest=? AND `+artUnreferencedBy("?"), artUnreferencedArgs(d)...)
		if err != nil {
			return err
		}
		if n, _ := marked.RowsAffected(); n == 0 {
			continue // referenced since the candidate scan
		}
		gone, err := s.retireArtworkFile(ctx, d, func(ctx context.Context) (bool, error) {
			// Still unreferenced now that the file is set aside? A publication
			// that referenced it meanwhile keeps it (and reset it to ready).
			deleted, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM artwork_objects WHERE digest=? AND status='deleting' AND `+artUnreferencedBy("?"), artUnreferencedArgs(d)...)
			if err != nil {
				return false, err
			}
			n, _ := deleted.RowsAffected()
			return n == 1, nil
		})
		if err != nil {
			return err
		}
		if !gone {
			if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE artwork_objects SET status='ready' WHERE digest=? AND status='deleting'`, d); err != nil {
				return err
			}
		}
	}
	// Crash-before-database-publication leaves only unreferenced content-addressed
	// files or private stages. Finite directory batches retain all younger writers.
	entries, err := s.nextArtworkDirectoryBatch()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Mode().IsRegular() || entry.ModTime().After(s.publicationTime().Add(-24*time.Hour)) {
			continue
		}
		name := entry.Name()
		digest := strings.TrimSuffix(name, ".img")
		if strings.HasPrefix(name, "stage-") {
			_ = os.Remove(filepath.Join(s.cacheRoot, name))
			continue
		}
		if strings.Contains(name, artworkTombMarker) {
			if err = s.settleArtworkTomb(ctx, name); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if !artDigest(digest) || digest+".img" != name {
			continue
		}
		if _, err = s.retireArtworkFile(ctx, digest, func(ctx context.Context) (bool, error) {
			var used bool
			err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_objects WHERE digest=?)`, digest).Scan(&used)
			return !used, err
		}); err != nil {
			return err
		}
	}
	return nil
}

// ArtCleanupInterval is how often the background loop should call
// CleanupArtwork: objects become collectable only a day after creation, so a
// pass per commit wake buys nothing. The throttle lives in the caller so tests
// and explicit maintenance can still sweep on demand.
const ArtCleanupInterval = 10 * time.Minute

// artUnreferenced is the indexed "nothing points at o.digest" predicate.
//
// Lane B's 800 px variants are referenced objects too: a variant digest stays
// while its source's artwork_variants row exists (the row cascades with the
// source), via the artwork_variants_digest index.
const artUnreferenced = `NOT EXISTS(SELECT 1 FROM artwork_variants v WHERE v.digest=o.digest AND v.source_digest<>o.digest) AND NOT EXISTS(SELECT 1 FROM artwork_previews p WHERE p.digest=o.digest) AND NOT EXISTS(SELECT 1 FROM artwork_selections a WHERE a.digest=o.digest) AND NOT EXISTS(SELECT 1 FROM artwork_selections a WHERE a.thumbnail_digest=o.digest) AND NOT EXISTS(SELECT 1 FROM artwork_uploads u WHERE u.digest=o.digest) AND NOT EXISTS(SELECT 1 FROM artwork_uploads u WHERE u.thumbnail_digest=o.digest)`

// artUnreferencedBy is the same predicate against a bound digest parameter,
// for the re-check while the file is set aside.
func artUnreferencedBy(p string) string {
	return strings.ReplaceAll(artUnreferenced, "o.digest", p)
}

// artUnreferencedArgs binds the digest for "digest=?" plus every placeholder
// artUnreferencedBy("?") introduces.
func artUnreferencedArgs(d string) []any {
	args := make([]any, 1+strings.Count(artUnreferenced, "o.digest"))
	for i := range args {
		args[i] = d
	}
	return args
}

func (s *Service) ownerHistoryText(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT before_json||' '||after_json FROM metadata_owner_history`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return b.String(), rows.Err()
}

// nextArtworkDirectoryBatch returns the next 128 entries of the cache directory
// walk. Only the directory handle is guarded; no database work happens under
// the guard.
func (s *Service) nextArtworkDirectoryBatch() ([]os.FileInfo, error) {
	s.artDirMu.Lock()
	defer s.artDirMu.Unlock()
	if s.artDirectory == nil {
		directory, err := os.Open(s.cacheRoot)
		if err != nil {
			return nil, err
		}
		s.artDirectory = directory
	}
	entries, err := s.artDirectory.Readdir(128)
	if err == io.EOF {
		s.artDirectory.Close()
		s.artDirectory = nil
		return entries, nil
	}
	return entries, err
}
