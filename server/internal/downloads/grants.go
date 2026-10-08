package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// Grant is a short-lived handle on one ready preparation's bytes. The URL is a
// path on this server; the token is the whole authority, so it is returned once
// and stored only as a digest.
type Grant struct {
	PreparationID       string   `json:"preparationId"`
	ItemID              string   `json:"itemId"`
	URL                 string   `json:"url"`
	Token               string   `json:"token"`
	IssuedAt            string   `json:"issuedAt"`
	ExpiresAt           string   `json:"expiresAt"`
	ReplayWindowSeconds int      `json:"replayWindowSeconds"`
	Artifact            Artifact `json:"artifact"`
}

// IssueGrant mints a transfer handle for a ready preparation. It is idempotent
// on operationId, which matters: a client that retried the request must not be
// handed a second token it will never use while the first one burns its window.
func (s *Service) IssueGrant(ctx context.Context, p identity.Principal, id, operationID string) (Grant, error) {
	var out Grant
	if !validID.MatchString(id) || !validID.MatchString(operationID) {
		return out, ErrInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	now := s.millis()
	key := operations.ViewerKey(p)
	scope := "downloads-grant:" + key
	raw, digest, e := operations.Receipt(tx, scope, operationID, []any{id}, now)
	if e != nil {
		return out, e
	}
	allowed, e := ProfileAllowsDownloads(tx, p.Viewer)
	if e != nil {
		return out, e
	}
	if !allowed {
		return out, ErrPolicy
	}
	if raw != "" {
		current, err := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=? AND profile_key=?`, id, key))
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrNotFound
		}
		if err != nil {
			return out, err
		}
		if err = contentaccess.VisibleKnownItemTx(ctx, tx, p, current.ItemID); err != nil {
			if errors.Is(err, identity.ErrContentRestricted) {
				return out, ErrNotFound
			}
			return out, err
		}
		return out, json.Unmarshal([]byte(raw), &out)
	}
	current, e := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=? AND profile_key=?`, id, key))
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if current.State != StateReady {
		return out, ErrNotReady
	}
	if e = contentaccess.VisibleKnownItemTx(ctx, tx, p, current.ItemID); e != nil {
		if errors.Is(e, identity.ErrContentRestricted) {
			return out, ErrNotFound
		}
		return out, e
	}
	token := identity.Token()
	expires := now + int64(GrantTTL/time.Millisecond)
	entity, e := entityid.Resolve(ctx, tx, current.ItemID)
	if errors.Is(e, entityid.ErrNotFound) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO download_grants(hash,preparation_id,profile_key,item_id,issued_ms,expires_ms) VALUES(?,?,?,?,?,?)`, identity.Digest(token), id, key, entity, now, expires); e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET used_ms=?,updated_ms=? WHERE id=?`, now, now, id); e != nil {
		return out, e
	}
	out = Grant{PreparationID: id, ItemID: current.ItemID, URL: s.grantPath + token, Token: token, IssuedAt: stamp(now), ExpiresAt: stamp(expires), ReplayWindowSeconds: int(GrantReplayWindow / time.Second), Artifact: current.Artifact}
	if e = operations.SaveReceipt(tx, scope, operationID, digest, out, now); e != nil {
		return out, e
	}
	return out, gated.Commit()
}

// Transfer is an open artifact plus everything the HTTP layer needs to serve it
// with Range, ETag and resumption. The caller closes Reader.
type Transfer struct {
	Reader    io.ReadSeekCloser
	Artifact  Artifact
	Viewer    identity.Viewer
	ItemID    string
	LibraryID string
	Modified  time.Time
}

// OpenGrant redeems a transfer token. The grant is single-use in the sense that
// matters for a download: the first request opens a replay window, and only the
// same grant may keep issuing ranged requests inside it. After the window, or
// after the TTL, the client asks for a new grant instead of resuming forever.
func (s *Service) OpenGrant(ctx context.Context, token string) (*Transfer, error) {
	if token == "" || len(token) > 200 {
		return nil, ErrGrant
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return nil, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	now := s.millis()
	var preparation, key string
	var expires, firstUse int64
	var revoked int
	e = tx.QueryRowContext(ctx, `SELECT preparation_id,profile_key,expires_ms,first_use_ms,revoked FROM download_grants WHERE hash=?`, identity.Digest(token)).Scan(&preparation, &key, &expires, &firstUse, &revoked)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrGrant
	}
	if e != nil {
		return nil, e
	}
	if revoked != 0 || expires <= now {
		return nil, ErrGrant
	}
	if firstUse > 0 && now > firstUse+int64(GrantReplayWindow/time.Millisecond) {
		return nil, ErrGrant
	}
	if firstUse == 0 {
		firstUse = now
	}
	if _, e = tx.ExecContext(ctx, `UPDATE download_grants SET first_use_ms=?,uses=uses+1 WHERE hash=?`, firstUse, identity.Digest(token)); e != nil {
		return nil, e
	}
	var state, item, library, kind, ref, artifactDigest, container, authority, account, profile, snapshotJSON string
	var size int64
	e = tx.QueryRowContext(ctx, `SELECT state,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_preparations.item_id),''),library_id,artifact_kind,artifact_ref,artifact_digest,artifact_container,bytes_total,authority,account_id,profile_id,source_version_json FROM download_preparations WHERE id=? AND profile_key=?`, preparation, key).Scan(&state, &item, &library, &kind, &ref, &artifactDigest, &container, &size, &authority, &account, &profile, &snapshotJSON)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrGrant
	}
	if e != nil {
		return nil, e
	}
	if state != StateReady {
		return nil, ErrNotReady
	}
	viewer := identity.Viewer{AccountID: account, ProfileID: profile, Authority: authority}
	if e = contentaccess.VisibleItemTx(ctx, tx, identity.Principal{Viewer: viewer}, item); e != nil {
		return nil, ErrGrant
	}
	allowed, e := ProfileAllowsDownloads(tx, viewer)
	if e != nil {
		return nil, e
	}
	if !allowed {
		return nil, ErrPolicy
	}
	_, err := resolveSource(ctx, tx, item)
	if err != nil && kind == "source" {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, ErrGrant
	}
	if kind == "source" {
		version, err := sourceSnapshotVersion(snapshotJSON, preparation, size)
		if err != nil || version.Evidence().Revision != artifactDigest {
			if err := terminal(ctx, gated2, preparation, StateUnavailable, ReasonSourceChanged, now); err != nil {
				return nil, err
			}
			return nil, ErrGrant
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET used_ms=? WHERE id=?`, now, preparation); e != nil {
		return nil, e
	}
	if e = gated2.Commit(); e != nil {
		return nil, e
	}
	contentType, fileName := artifactDelivery(container, preparation)
	out := &Transfer{
		Artifact:  Artifact{Kind: kind, Ref: ref, SHA256: artifactDigest, Bytes: size, Container: container, ContentType: contentType, FileName: fileName},
		Viewer:    viewer,
		ItemID:    item,
		LibraryID: library,
		// Prepared bytes are immutable and carry no meaningful modification
		// time; a zero time makes ServeContent omit Last-Modified rather than
		// claim 1970. Original snapshots are immutable too.
		Modified: time.Time{},
	}
	switch kind {
	case "source":
		version, err := sourceSnapshotVersion(snapshotJSON, preparation, size)
		if err != nil || version.Evidence().Revision != artifactDigest {
			// Legacy ready rows have no immutable source pin. Retire the old
			// grant/receipt association rather than advertising its digest.
			return nil, ErrGrant
		}
		reader, err := s.openSnapshot(version)
		if err != nil {
			return nil, err
		}
		out.Reader = reader
	case "prepared":
		if s.artifacts == nil {
			return nil, ErrNotReady
		}
		reader, err := s.artifacts.OpenDownloadArtifact(ctx, ref, artifactDigest, size)
		if err != nil {
			return nil, err
		}
		out.Reader = reader
	default:
		return nil, ErrNotReady
	}
	return out, nil
}
