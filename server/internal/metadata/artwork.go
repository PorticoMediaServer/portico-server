package metadata

import (
	"context"
	"database/sql"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/imagework"
)

var ErrArtworkVersion = errors.New("artwork version is no longer selected")

var ErrArtworkPending = errors.New("artwork is queued or unavailable")

func (s *Service) SetArtworkDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	s.cacheRoot = path
	return nil
}
func validArtworkRole(role string) bool {
	switch role {
	case "poster", "backdrop", "thumbnail", "still", "logo", "square", "portrait", "banner", "cover":
		return true
	}
	return false
}
func (s *Service) Artwork(ctx context.Context, item, role string) (*os.File, string, error) {
	return s.EntityArtwork(ctx, RepairTarget{"item", item}, role, "", false)
}

// ArtworkIdentity returns the selection digest for catalog metadata. HTTP serving
// must open the selected object before answering conditional requests: a missing
// cache object must schedule repair rather than extend a stale client cache.
func (s *Service) ArtworkIdentity(ctx context.Context, item, role string) (string, string, error) {
	return s.EntityArtworkIdentity(ctx, RepairTarget{"item", item}, role, "", false)
}

// EntityArtworkIdentity is ArtworkIdentity for any repair target.
func (s *Service) EntityArtworkIdentity(ctx context.Context, t RepairTarget, role, subject string, thumbnail bool) (string, string, error) {
	if !validArtworkRole(role) || len(subject) > 160 {
		return "", "", ErrRepairInput
	}
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return "", "", err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if err = validateArtworkTarget(ctx, tx, t); err != nil {
		return "", "", err
	}
	digest, mime, _, _, err := artworkSelection(ctx, tx, t, role, subject, thumbnail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if !artDigest(digest) {
		return "", "", nil
	}
	return digest, mime, gated.Commit()
}

// artworkSelection is the selection lookup the reader and the identity resolver
// share, so an ETag can never describe different bytes from the ones served.
func artworkSelection(ctx context.Context, tx *sql.Tx, t RepairTarget, role, subject string, thumbnail bool) (string, string, RepairTarget, string, error) {
	col := "digest"
	if thumbnail {
		col = "thumbnail_digest"
	}
	var digest, mime string
	selectedTarget, selectedRole := t, role
	entity, resolveErr := artworkEntity(ctx, tx, t.ID)
	if resolveErr != nil {
		return "", "", selectedTarget, selectedRole, resolveErr
	}
	err := tx.QueryRowContext(ctx, `SELECT o.digest,o.mime FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.`+col+` AND o.status='ready' WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject=?`, t.Kind, entity, role, subject).Scan(&digest, &mime)
	if errors.Is(err, sql.ErrNoRows) && t.Kind == "item" && (role == "poster" || role == "cover") {
		err = tx.QueryRowContext(ctx, `SELECT o.digest,o.mime,pid(al.public_id) FROM catalog_songs cs JOIN artwork_selections a ON a.kind='album' AND a.entity_id=cs.album_id AND a.role='cover' AND a.subject='' JOIN artwork_objects o ON o.digest=a.`+col+` AND o.status='ready' JOIN catalog_entities al ON al.id=cs.album_id WHERE cs.entity_id=?`, entity).Scan(&digest, &mime, &selectedTarget.ID)
		if err == nil {
			selectedTarget.Kind = "album"
			selectedRole = "cover"
		}
	}
	if errors.Is(err, sql.ErrNoRows) && t.Kind == "item" && (role == "poster" || role == "cover") {
		err = tx.QueryRowContext(ctx, `SELECT o.digest,o.mime,a.kind,pid(e.public_id),a.role FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.`+col+` AND o.status='ready' JOIN catalog_entities e ON e.id=a.entity_id WHERE a.subject='' AND ((a.kind='item' AND a.entity_id=? AND a.role='cover') OR (a.kind='book' AND a.entity_id IN(SELECT book_id FROM catalog_book_files WHERE entity_id=?) AND a.role='cover')) ORDER BY a.kind DESC LIMIT 1`, entity, entity).Scan(&digest, &mime, &selectedTarget.Kind, &selectedTarget.ID, &selectedRole)
	}
	if errors.Is(err, sql.ErrNoRows) && (t.Kind == "item" || t.Kind == "season") && (role == "poster" || role == "backdrop") {
		err = tx.QueryRowContext(ctx, `WITH parents(kind,id,priority) AS (
          SELECT 'season',season_id,0 FROM catalog_episodes WHERE entity_id=? AND season_id IS NOT NULL
          UNION ALL SELECT 'show',show_id,1 FROM catalog_episodes WHERE entity_id=?
          UNION ALL SELECT 'show',show_id,1 FROM catalog_seasons WHERE entity_id=?
        ) SELECT o.digest,o.mime,a.kind,pid(e.public_id),a.role FROM parents p JOIN artwork_selections a ON a.kind=p.kind AND a.entity_id=p.id AND a.role=? AND a.subject='' JOIN artwork_objects o ON o.digest=a.`+col+` AND o.status='ready' JOIN catalog_entities e ON e.id=a.entity_id ORDER BY p.priority LIMIT 1`, entity, entity, entity, role).Scan(&digest, &mime, &selectedTarget.Kind, &selectedTarget.ID, &selectedRole)
	}
	return digest, mime, selectedTarget, selectedRole, err
}

func (s *Service) EntityArtwork(ctx context.Context, t RepairTarget, role, subject string, thumbnail bool) (*os.File, string, error) {
	width := 1920
	if thumbnail {
		width = 400
	}
	return s.EntityArtworkVariant(ctx, t, role, subject, width, "")
}

// EntityArtworkVariant opens the selected immutable representation. Version
// identifies the selected full artwork, independent of the requested width.
// File existence is established before HTTP conditional response evaluation.
func (s *Service) EntityArtworkVariant(ctx context.Context, t RepairTarget, role, subject string, width int, version string) (*os.File, string, error) {
	return s.entityArtworkVariant(ctx, t, role, subject, width, version, true)
}

func (s *Service) entityArtworkVariant(ctx context.Context, t RepairTarget, role, subject string, width int, version string, build bool) (*os.File, string, error) {
	if width != ArtworkAmbientEdge && width != 400 && width != 800 && width != 1920 && width != 2560 {
		return nil, "", ErrRepairInput
	}

	if !validArtworkRole(role) || len(subject) > 160 {
		return nil, "", ErrRepairInput
	}
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return nil, "", err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if err = validateArtworkTarget(ctx, tx, t); err != nil {
		return nil, "", err
	}
	digest, mime, selectedTarget, selectedRole, err := artworkSelection(ctx, tx, t, role, subject, false)
	sourceDigest := digest
	if err == nil && version != "" && version != sourceDigest {
		return nil, "", ErrArtworkVersion
	}
	if err == nil && width == ArtworkAmbientEdge {
		// A cached ambient image never substitutes for opening the exact selected
		// source. In particular, missing/revoked selections cannot extend a 304.
		source, e := s.openArtworkObject(sourceDigest)
		if e != nil {
			s.queueMissingArtwork(selectedTarget, selectedRole, subject)
			return nil, "", ErrArtworkPending
		}
		defer source.Close()
		if f, e := s.openAmbientArtwork(ctx, sourceDigest); e == nil {
			if e = gated.Commit(); e != nil {
				f.Close()
				return nil, "", e
			}
			return f, "image/png", nil
		}
		if e := ctx.Err(); e != nil {
			return nil, "", e
		}
		if !build {
			return nil, "", ErrArtworkPending
		}
		if e := gated.Commit(); e != nil {
			return nil, "", e
		}
		if e := s.ensureAmbientArtwork(ctx, sourceDigest, source); e != nil {
			return nil, "", e
		}
		// Selection and physical source availability are checked again after
		// decoding, outside the snapshot, just like a missing foreground width.
		return s.entityArtworkVariant(ctx, t, role, subject, width, version, false)
	}
	if err == nil {
		var sourceWidth, sourceHeight int
		var mapped, variantMime sql.NullString
		var variantWidth, variantHeight sql.NullInt64
		if e := tx.QueryRowContext(ctx, `SELECT src.width,src.height,v.digest,dst.mime,dst.width,dst.height FROM artwork_objects src LEFT JOIN artwork_variants v ON v.source_digest=src.digest AND v.width=? LEFT JOIN artwork_objects dst ON dst.digest=v.digest AND dst.status='ready' WHERE src.digest=? AND src.status='ready'`, width, sourceDigest).Scan(&sourceWidth, &sourceHeight, &mapped, &variantMime, &variantWidth, &variantHeight); e != nil {
			return nil, "", e
		}
		// A request at or above the stored long edge opens the source itself,
		// even if an older bucket mapping names some other representation.
		if max(sourceWidth, sourceHeight) > width {
			if mapped.Valid && variantMime.Valid && variantWidth.Valid && variantHeight.Valid && max(variantWidth.Int64, variantHeight.Int64) <= int64(width) {
				digest, mime = mapped.String, variantMime.String
			} else {
				if !build {
					return nil, "", ErrArtworkPending
				}
				if e := gated.Commit(); e != nil {
					return nil, "", e
				}
				if e := s.ensureArtworkVariant(ctx, sourceDigest, width, mapped.String); e != nil {
					return nil, "", e
				}
				return s.entityArtworkVariant(ctx, t, role, subject, width, version, false)
			}
		}
	}

	if err == nil {
		if !artDigest(digest) {
			return nil, "", ErrRepairInput
		}
		f, e := s.openArtworkObject(digest)
		if e != nil {
			if build && digest != sourceDigest {
				if e := gated.Commit(); e != nil {
					return nil, "", e
				}
				if e := s.ensureArtworkVariant(ctx, sourceDigest, width, digest); e != nil {
					return nil, "", e
				}
				return s.entityArtworkVariant(ctx, t, role, subject, width, version, false)
			}
			s.queueMissingArtwork(selectedTarget, selectedRole, subject)
			return nil, "", ErrArtworkPending
		}
		if e = gated.Commit(); e != nil {
			f.Close()
			return nil, "", e
		}
		return f, mime, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	if width == ArtworkAmbientEdge {
		return nil, "", ErrArtworkPending
	}
	// Already installed local evidence remains available during initial migration.
	// Remote URLs are never followed, including when the worker is not configured.
	if t.Kind == "item" && (role == "poster" || role == "backdrop") && s.LocalArtwork != nil {
		column := "poster_url"
		if role == "backdrop" {
			column = "backdrop_url"
		}
		var value string
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(d.`+column+`, '') FROM catalog_entities e LEFT JOIN catalog_item_details d ON d.entity_id=e.id WHERE e.public_id=pid_blob(?)`, t.ID).Scan(&value); err != nil {
			return nil, "", err
		}
		if strings.HasPrefix(value, "local:") {
			if err = gated.Commit(); err != nil {
				return nil, "", err
			}
			return s.LocalArtwork(strings.TrimPrefix(value, "local:"))
		}
	}
	return nil, "", ErrArtworkPending
}

// Generate a missing width from the selected full object outside any read or
// write transaction. Requests that need the same width of the same source
// serialize on the decoder gate, which the encoder holds until its mapping is
// published: a waiter re-checks the exact published variant before decoding, so
// each source and width is encoded, installed and mapped once rather than once
// per concurrent request that missed the cache at the same moment.
func (s *Service) ensureArtworkVariant(ctx context.Context, source string, width int, replaceMissing string) error {
	f, err := s.openArtworkObject(source)
	if err != nil {
		return ErrArtworkPending
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(f)
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 24000000 {
		return ErrArtworkPending
	}
	if _, err = f.Seek(0, 0); err != nil {
		return ErrArtworkPending
	}
	release, err := imagework.Acquire(ctx)
	if err != nil {
		return err
	}
	// The gate is held until the mapping is committed below. Releasing it after
	// the encode would let the next waiter read the database before this
	// request's mapping exists and encode the same bytes again.
	defer release()
	// A peer may have published this exact variant while this request waited for
	// the gate. The check mirrors the serving selection and opens the installed
	// object, so a mapped-but-missing file still repairs instead of reporting a
	// variant that cannot be served.
	if published, checkErr := s.artworkVariantPublished(ctx, source, width); checkErr != nil {
		return checkErr
	} else if published {
		return nil
	}
	decoded, _, err := image.Decode(f)
	if err != nil {
		return ErrArtworkPending
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	slot, limit := artworkWidthLimits(width)
	raw, w, h, err := encodeArtworkWithin(ctx, decoded, slot, width, limit)
	if err != nil {
		return ErrArtworkPending
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	installed, err := s.installArtworkFile(raw, w, h)
	if err != nil {
		return ErrArtworkPending
	}
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		if err := recordArtworkObject(ctx, tx, installed, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO artwork_variants(source_digest,width,digest) VALUES(?,?,?) ON CONFLICT(source_digest,width) DO UPDATE SET digest=excluded.digest WHERE artwork_variants.digest=?`, source, width, installed.digest, replaceMissing)
		return err
	})
}

// artworkVariantPublished reports whether the variant this request would
// generate is already mapped to an installed object. It mirrors the serving
// selection in entityArtworkVariant rather than trusting the mapping row alone,
// so a digest whose file cannot be opened is reported as missing and rebuilt.
// A source at or below the requested edge is served directly and needs no
// variant, so that case never counts as published.
func (s *Service) artworkVariantPublished(ctx context.Context, source string, width int) (bool, error) {
	var sourceWidth, sourceHeight int
	var digest, mime sql.NullString
	var variantWidth, variantHeight sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT src.width,src.height,v.digest,dst.mime,dst.width,dst.height FROM artwork_objects src LEFT JOIN artwork_variants v ON v.source_digest=src.digest AND v.width=? LEFT JOIN artwork_objects dst ON dst.digest=v.digest AND dst.status='ready' WHERE src.digest=? AND src.status='ready'`, width, source).Scan(&sourceWidth, &sourceHeight, &digest, &mime, &variantWidth, &variantHeight); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// Nothing publishable is visible: the ordinary generate path stays
		// authoritative, including its own error reporting.
		return false, nil
	}
	if max(sourceWidth, sourceHeight) <= width || !digest.Valid || !mime.Valid || !variantWidth.Valid || !variantHeight.Valid || max(variantWidth.Int64, variantHeight.Int64) > int64(width) || !artDigest(digest.String) {
		return false, nil
	}
	f, err := s.openArtworkObject(digest.String)
	if err != nil {
		return false, nil
	}
	f.Close()
	return true, nil
}
func (s *Service) verifyRestorableArtwork(choices []ArtworkChoice) error {
	for _, choice := range choices {
		for _, digest := range []string{choice.Digest, choice.Thumbnail} {
			if !artDigest(digest) {
				return ErrRepairInput
			}
			f, err := s.openArtworkObject(digest)
			if err != nil {
				return errors.New("previous artwork bytes are missing; repair the retained selection before restoring")
			}
			f.Close()
		}
	}
	return nil
}

// Open a content-addressed, regular cache object without serving a symlink or a
// file replaced between inspection and open. The descriptor remains valid if a
// later install/cleanup renames or unlinks the pathname.
func (s *Service) openArtworkObject(digest string) (*os.File, error) {
	if s.cacheRoot == "" || !artDigest(digest) {
		return nil, ErrArtworkPending
	}
	path := filepath.Join(s.cacheRoot, digest+".img")
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() == 0 {
		return nil, ErrArtworkPending
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		f.Close()
		return nil, ErrArtworkPending
	}
	return f, nil
}
