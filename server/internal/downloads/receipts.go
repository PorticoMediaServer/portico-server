package downloads

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"strconv"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// ReceiptKind and ReceiptVersion identify the signed document. A verifier that
// does not recognize both refuses the receipt rather than guessing its meaning.
const (
	ReceiptKind    = "portico.download-receipt"
	ReceiptVersion = "1"
)

// ReceiptViewer is the exact viewer a receipt is good for. A receipt is not
// transferable: it names the authority, account and profile, and a client that
// signed in as somebody else must ask for its own.
type ReceiptViewer struct {
	Authority string `json:"authority"`
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId"`
	ServerID  string `json:"serverId"`
}

// ReceiptArtifact binds the receipt to specific bytes. A client verifies the
// file it holds against sha256 before playing it offline.
type ReceiptArtifact struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// ReceiptClaims is the signed document itself.
type ReceiptClaims struct {
	Kind          string          `json:"kind"`
	Version       string          `json:"version"`
	ReceiptID     string          `json:"receiptId"`
	KeyID         string          `json:"keyId"`
	Viewer        ReceiptViewer   `json:"viewer"`
	ItemID        string          `json:"itemId"`
	PreparationID string          `json:"preparationId"`
	Quality       string          `json:"quality"`
	QualityLabel  string          `json:"qualityLabel"`
	Artifact      ReceiptArtifact `json:"artifact"`
	IssuedAt      string          `json:"issuedAt"`
	ExpiresAt     string          `json:"expiresAt"`
}

// Receipt is what a client stores. payload carries the exact signed bytes, so a
// verifier never has to reproduce this server's JSON formatting to check the
// signature; claims is the same document decoded, for convenience.
type Receipt struct {
	ReceiptID string        `json:"receiptId"`
	Algorithm string        `json:"algorithm"`
	KeyID     string        `json:"keyId"`
	Payload   string        `json:"payload"`
	Signature string        `json:"signature"`
	Claims    ReceiptClaims `json:"claims"`
	Revision  int64         `json:"revision"`
}

// ReceiptKey is a published verification key. The private half never leaves the
// server; a client pins the key it saw and refuses a receipt signed by another.
type ReceiptKey struct {
	ID        string `json:"id"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"publicKey"`
	CreatedAt string `json:"createdAt"`
	RetiredAt string `json:"retiredAt,omitempty"`
}

// ReceiptOutcome is one entry's answer from issue or revalidate.
type ReceiptOutcome struct {
	PreparationID string   `json:"preparationId,omitempty"`
	ReceiptID     string   `json:"receiptId,omitempty"`
	Outcome       string   `json:"outcome"`
	Code          string   `json:"code,omitempty"`
	Receipt       *Receipt `json:"receipt,omitempty"`
}

// Outcomes. "issued" and "renewed" carry a receipt; everything else explains
// why offline playback must stop.
const (
	OutcomeIssued  = "issued"
	OutcomeRenewed = "renewed"
	OutcomeRefused = "refused"
)

// Revocation is one entry of the revocation list a client polls.
type Revocation struct {
	ReceiptID string `json:"receiptId"`
	ItemID    string `json:"itemId"`
	ProfileID string `json:"profileId"`
	Reason    string `json:"reason"`
	RevokedAt string `json:"revokedAt"`
	Sequence  int64  `json:"sequence"`
}

// RevocationPage is a cursor page of revocations plus the instant it describes.
type RevocationPage struct {
	Items      []Revocation `json:"items"`
	NextCursor string       `json:"nextCursor"`
	AsOf       string       `json:"asOf"`
}

// signingKey returns the active receipt key, creating one on first use. Key
// creation is part of the caller's transaction, so two concurrent first issues
// cannot install two keys.
func signingKey(tx *sql.Tx, now int64) (string, ed25519.PrivateKey, error) {
	var id string
	var private []byte
	e := tx.QueryRow(`SELECT id,private_key FROM download_receipt_keys WHERE retired_ms=0 ORDER BY created_ms DESC LIMIT 1`).Scan(&id, &private)
	if e == nil {
		if len(private) != ed25519.PrivateKeySize {
			return "", nil, ErrReceipt
		}
		return id, ed25519.PrivateKey(private), nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", nil, e
	}
	public, secret, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return "", nil, e
	}
	id = identity.Digest(identity.Token())[:32]
	if _, e = tx.Exec(`INSERT INTO download_receipt_keys(id,algorithm,private_key,public_key,created_ms) VALUES(?,'ed25519',?,?,?)`, id, []byte(secret), []byte(public), now); e != nil {
		return "", nil, e
	}
	return id, secret, nil
}

// ReceiptKeys publishes every verification key this server has used, so a
// client can still verify a receipt signed before a rotation while refusing to
// accept a new one from a retired key.
func (s *Service) ReceiptKeys(ctx context.Context) ([]ReceiptKey, error) {
	out := []ReceiptKey{}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if _, _, e = signingKey(tx, s.millis()); e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,algorithm,public_key,created_ms,retired_ms FROM download_receipt_keys ORDER BY created_ms DESC LIMIT 32`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var key ReceiptKey
		var public []byte
		var created, retired int64
		if e = rows.Scan(&key.ID, &key.Algorithm, &public, &created, &retired); e != nil {
			rows.Close()
			return out, e
		}
		key.PublicKey, key.CreatedAt, key.RetiredAt = base64.RawURLEncoding.EncodeToString(public), stamp(created), stamp(retired)
		out = append(out, key)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, gated.Commit()
}

// sign renders and signs one receipt.
func sign(keyID string, secret ed25519.PrivateKey, claims ReceiptClaims, revision int64) (Receipt, error) {
	raw, e := json.Marshal(claims)
	if e != nil {
		return Receipt{}, e
	}
	return Receipt{
		ReceiptID: claims.ReceiptID,
		Algorithm: "ed25519",
		KeyID:     keyID,
		Payload:   base64.RawURLEncoding.EncodeToString(raw),
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(secret, raw)),
		Claims:    claims,
		Revision:  revision,
	}, nil
}

// VerifyReceipt checks an envelope against this server's published keys. It is
// the same check a client performs offline, exposed so the server's own tests
// and a third-party client can agree on what "valid" means.
func VerifyReceipt(keys []ReceiptKey, envelope Receipt, now time.Time) (ReceiptClaims, error) {
	var claims ReceiptClaims
	raw, e := base64.RawURLEncoding.Strict().DecodeString(envelope.Payload)
	if e != nil || len(raw) > 8<<10 {
		return claims, ErrReceipt
	}
	signature, e := base64.RawURLEncoding.Strict().DecodeString(envelope.Signature)
	if e != nil || len(signature) != ed25519.SignatureSize {
		return claims, ErrReceipt
	}
	if e = json.Unmarshal(raw, &claims); e != nil {
		return claims, ErrReceipt
	}
	if claims.Kind != ReceiptKind || claims.Version != ReceiptVersion || claims.KeyID != envelope.KeyID {
		return claims, ErrReceipt
	}
	for _, key := range keys {
		if key.ID != envelope.KeyID || key.Algorithm != "ed25519" {
			continue
		}
		public, err := base64.RawURLEncoding.Strict().DecodeString(key.PublicKey)
		if err != nil || len(public) != ed25519.PublicKeySize {
			return claims, ErrReceipt
		}
		if !ed25519.Verify(ed25519.PublicKey(public), raw, signature) {
			return claims, ErrReceipt
		}
		expires, err := time.Parse(time.RFC3339, claims.ExpiresAt)
		if err != nil || !now.Before(expires) {
			return claims, ErrReceipt
		}
		return claims, nil
	}
	return claims, ErrReceipt
}

// IssueReceipts hands out offline authorization for ready preparations. Each
// entry answers on its own, because one deleted item in a season must not cost
// a client the receipts for the rest of it.
func (s *Service) IssueReceipts(ctx context.Context, p identity.Principal, serverID, operationID string, preparations []string) ([]ReceiptOutcome, error) {
	out := []ReceiptOutcome{}
	if !validID.MatchString(operationID) || len(preparations) == 0 || len(preparations) > MaxBatchTargets {
		return out, ErrInput
	}
	for _, id := range preparations {
		if !validID.MatchString(id) {
			return out, ErrInput
		}
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	now := s.millis()
	key := operations.ViewerKey(p)
	scope := "downloads-receipts:" + key
	raw, digest, e := operations.Receipt(tx, scope, operationID, []any{preparations}, now)
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
		return out, json.Unmarshal([]byte(raw), &out)
	}
	keyID, secret, e := signingKey(tx, now)
	if e != nil {
		return out, e
	}
	for _, id := range preparations {
		var state, item, quality, artifactDigest string
		var size int64
		err := tx.QueryRowContext(ctx, `SELECT state,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_preparations.item_id),''),quality,artifact_digest,bytes_total FROM download_preparations WHERE id=? AND profile_key=?`, id, key).Scan(&state, &item, &quality, &artifactDigest, &size)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, ReceiptOutcome{PreparationID: id, Outcome: OutcomeRefused, Code: ReasonUnknownReceipt})
			continue
		}
		if err != nil {
			return out, err
		}
		if state != StateReady || !validDigest.MatchString(artifactDigest) || size <= 0 {
			out = append(out, ReceiptOutcome{PreparationID: id, Outcome: OutcomeRefused, Code: ReasonFailed})
			continue
		}
		if err = contentaccess.VisibleKnownItemTx(ctx, tx, p, item); err != nil {
			// A catalogue change still publishing is a retry, not a refusal.
			if !errors.Is(err, identity.ErrContentRestricted) {
				return out, err
			}
			out = append(out, ReceiptOutcome{PreparationID: id, Outcome: OutcomeRefused, Code: ReasonItemDeleted})
			continue
		}
		receiptID, receipt, err := s.storeReceipt(ctx, tx, p, serverID, keyID, secret, id, item, quality, artifactDigest, size, now)
		if err != nil {
			return out, err
		}
		out = append(out, ReceiptOutcome{PreparationID: id, ReceiptID: receiptID, Outcome: OutcomeIssued, Receipt: receipt})
	}
	if e = operations.SaveReceipt(tx, scope, operationID, digest, out, now); e != nil {
		return out, e
	}
	return out, gated2.Commit()
}

func (s *Service) storeReceipt(ctx context.Context, tx *sql.Tx, p identity.Principal, serverID, keyID string, secret ed25519.PrivateKey, preparation, item, quality, artifactDigest string, size, now int64) (string, *Receipt, error) {
	id := identity.Digest(identity.Token())
	expires := now + int64(ReceiptTTL/time.Millisecond)
	var sequence int64
	if _, e := tx.ExecContext(ctx, `UPDATE download_receipt_sequence SET value=value+1 WHERE singleton=1`); e != nil {
		return "", nil, e
	}
	if e := tx.QueryRowContext(ctx, `SELECT value FROM download_receipt_sequence WHERE singleton=1`).Scan(&sequence); e != nil {
		return "", nil, e
	}
	entity, e := entityid.Resolve(ctx, tx, item)
	if e != nil {
		return "", nil, e
	}
	if _, e := tx.ExecContext(ctx, `INSERT INTO download_receipts(id,profile_key,authority,account_id,profile_id,item_id,preparation_id,quality,artifact_digest,artifact_bytes,key_id,issued_ms,expires_ms,sequence) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, operations.ViewerKey(p), p.Authority, p.AccountID, p.ProfileID, entity, preparation, quality, artifactDigest, size, keyID, now, expires, sequence); e != nil {
		return "", nil, e
	}
	claims := ReceiptClaims{
		Kind: ReceiptKind, Version: ReceiptVersion, ReceiptID: id, KeyID: keyID,
		Viewer:        ReceiptViewer{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID, ServerID: serverID},
		ItemID:        item,
		PreparationID: preparation,
		Quality:       quality,
		QualityLabel:  qualityLabel(quality),
		Artifact:      ReceiptArtifact{SHA256: artifactDigest, Bytes: size},
		IssuedAt:      stamp(now), ExpiresAt: stamp(expires),
	}
	receipt, e := sign(keyID, secret, claims, 1)
	if e != nil {
		return "", nil, e
	}
	return id, &receipt, nil
}

// Revalidate renews receipts a client presents when it regains contact. A
// renewal is refused — and the receipt revoked — when the profile lost
// allowDownloads, the item is gone, or the account was disabled. The server
// answers from its own rows rather than from the envelope the client sent,
// because the envelope is exactly what a stale client would be holding.
func (s *Service) Revalidate(ctx context.Context, p identity.Principal, serverID, operationID string, receiptIDs []string) ([]ReceiptOutcome, error) {
	out := []ReceiptOutcome{}
	if !validID.MatchString(operationID) || len(receiptIDs) == 0 || len(receiptIDs) > MaxBatchTargets {
		return out, ErrInput
	}
	for _, id := range receiptIDs {
		if !validID.MatchString(id) {
			return out, ErrInput
		}
	}
	gated3, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	now := s.millis()
	key := operations.ViewerKey(p)
	scope := "downloads-revalidate:" + key
	raw, digest, e := operations.Receipt(tx, scope, operationID, []any{receiptIDs}, now)
	if e != nil {
		return out, e
	}
	allowed, e := ProfileAllowsDownloads(tx, p.Viewer)
	if e != nil {
		return out, e
	}
	if raw != "" {
		if !allowed {
			return out, ErrPolicy
		}
		return out, json.Unmarshal([]byte(raw), &out)
	}
	disabled, e := AccountDisabled(tx, p.Viewer)
	if e != nil {
		return out, e
	}
	keyID, secret, e := signingKey(tx, now)
	if e != nil {
		return out, e
	}
	for _, id := range receiptIDs {
		var item, preparation, quality, artifactDigest string
		var size, revoked, revision int64
		err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_receipts.item_id),''),preparation_id,quality,artifact_digest,artifact_bytes,revoked_ms,revision FROM download_receipts WHERE id=? AND profile_key=?`, id, key).Scan(&item, &preparation, &quality, &artifactDigest, &size, &revoked, &revision)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, ReceiptOutcome{ReceiptID: id, Outcome: OutcomeRefused, Code: ReasonUnknownReceipt})
			continue
		}
		if err != nil {
			return out, err
		}
		if revoked > 0 {
			out = append(out, ReceiptOutcome{ReceiptID: id, Outcome: OutcomeRefused, Code: ReasonRevoked})
			continue
		}
		refuse := ""
		switch {
		case disabled:
			refuse = ReasonAccountDisabled
		case !allowed:
			refuse = ReasonNotAllowed
		}
		if refuse == "" {
			var exists int
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities WHERE public_id=pid_blob(?))`, item).Scan(&exists); err != nil {
				return out, err
			}
			if exists == 0 {
				refuse = ReasonItemDeleted
			}
		}
		if refuse == "" {
			// Renewal never revokes a copy because a catalogue edit is still
			// publishing: that answers retry. Only a published refusal revokes.
			if err = contentaccess.VisibleKnownItemTx(ctx, tx, p, item); errors.Is(err, identity.ErrContentRestricted) {
				refuse = ReasonItemDeleted
			} else if err != nil {
				return out, err
			}
		}
		if refuse == "" {
			var state string
			if err = tx.QueryRowContext(ctx, `SELECT state FROM download_preparations WHERE id=?`, preparation).Scan(&state); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return out, err
			} else if errors.Is(err, sql.ErrNoRows) || state != StateReady {
				refuse = ReasonRetention
			}
		}
		if refuse != "" {
			if err = revokeReceipt(tx, id, refuse, now); err != nil {
				return out, err
			}
			out = append(out, ReceiptOutcome{ReceiptID: id, Outcome: OutcomeRefused, Code: refuse})
			continue
		}
		expires := now + int64(ReceiptTTL/time.Millisecond)
		if _, err = tx.ExecContext(ctx, `UPDATE download_receipts SET issued_ms=?,expires_ms=?,key_id=?,revision=revision+1 WHERE id=?`, now, expires, keyID, id); err != nil {
			return out, err
		}
		claims := ReceiptClaims{
			Kind: ReceiptKind, Version: ReceiptVersion, ReceiptID: id, KeyID: keyID,
			Viewer:        ReceiptViewer{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID, ServerID: serverID},
			ItemID:        item,
			PreparationID: preparation,
			Quality:       quality,
			QualityLabel:  qualityLabel(quality),
			Artifact:      ReceiptArtifact{SHA256: artifactDigest, Bytes: size},
			IssuedAt:      stamp(now), ExpiresAt: stamp(expires),
		}
		renewed, err := sign(keyID, secret, claims, revision+1)
		if err != nil {
			return out, err
		}
		out = append(out, ReceiptOutcome{ReceiptID: id, PreparationID: preparation, Outcome: OutcomeRenewed, Receipt: &renewed})
	}
	if e = operations.SaveReceipt(tx, scope, operationID, digest, out, now); e != nil {
		return out, e
	}
	return out, gated3.Commit()
}

// Revoke withdraws offline authorization. A viewer may revoke their own
// receipts — the action behind "remove this download from my other device" —
// and an owner may revoke any.
func (s *Service) Revoke(ctx context.Context, p identity.Principal, operationID, reason string, receiptIDs []string, anyViewer bool) ([]ReceiptOutcome, error) {
	out := []ReceiptOutcome{}
	if !validID.MatchString(operationID) || len(receiptIDs) == 0 || len(receiptIDs) > MaxBatchTargets {
		return out, ErrInput
	}
	if reason == "" {
		reason = ReasonRevoked
	}
	if !validID.MatchString(reason) {
		return out, ErrInput
	}
	gated4, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	now := s.millis()
	scope := "downloads-revoke:" + operations.ViewerKey(p)
	raw, digest, e := operations.Receipt(tx, scope, operationID, []any{receiptIDs, reason}, now)
	if e != nil {
		return out, e
	}
	if raw != "" {
		return out, json.Unmarshal([]byte(raw), &out)
	}
	for _, id := range receiptIDs {
		if !validID.MatchString(id) {
			return out, ErrInput
		}
		query := `SELECT id FROM download_receipts WHERE id=? AND profile_key=?`
		args := []any{id, operations.ViewerKey(p)}
		if anyViewer {
			query, args = `SELECT id FROM download_receipts WHERE id=?`, []any{id}
		}
		var found string
		err := tx.QueryRowContext(ctx, query, args...).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, ReceiptOutcome{ReceiptID: id, Outcome: OutcomeRefused, Code: ReasonUnknownReceipt})
			continue
		}
		if err != nil {
			return out, err
		}
		if err = revokeReceipt(tx, id, reason, now); err != nil {
			return out, err
		}
		out = append(out, ReceiptOutcome{ReceiptID: id, Outcome: OutcomeRefused, Code: ReasonRevoked})
	}
	if e = operations.Audit(tx, now, operations.AccountKey(p), "downloads.revoke", reason, int64(len(out))); e != nil {
		return out, e
	}
	if e = operations.SaveReceipt(tx, scope, operationID, digest, out, now); e != nil {
		return out, e
	}
	return out, gated4.Commit()
}

func revokeReceipt(tx *sql.Tx, id, reason string, now int64) error {
	var sequence int64
	if _, e := tx.Exec(`UPDATE download_receipt_sequence SET value=value+1 WHERE singleton=1`); e != nil {
		return e
	}
	if e := tx.QueryRow(`SELECT value FROM download_receipt_sequence WHERE singleton=1`).Scan(&sequence); e != nil {
		return e
	}
	_, e := tx.Exec(`UPDATE download_receipts SET revoked_ms=?,revoked_reason=?,sequence=?,revision=revision+1 WHERE id=? AND revoked_ms=0`, now, reason, sequence, id)
	return e
}

// revokeForPreparation withdraws every live receipt that vouched for one
// preparation. It is called wherever a claim stops being true.
func revokeForPreparation(tx *sql.Tx, preparation, reason string, now int64) error {
	if _, err := tx.Exec(`UPDATE download_grants SET revoked=1 WHERE preparation_id=?`, preparation); err != nil {
		return err
	}
	rows, e := tx.Query(`SELECT id FROM download_receipts WHERE preparation_id=? AND revoked_ms=0 LIMIT 256`, preparation)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = revokeReceipt(tx, id, reason, now); e != nil {
			return e
		}
	}
	return nil
}

// Revocations pages the revocation list. A client that was offline polls from
// the sequence it last saw; the list is server-wide for an owner and scoped to
// the viewer otherwise, because a revocation names a viewer's own receipt.
func (s *Service) Revocations(ctx context.Context, p identity.Principal, cursor string, limit int, anyViewer bool) (RevocationPage, error) {
	out := RevocationPage{Items: []Revocation{}, AsOf: stamp(s.millis())}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	after := int64(0)
	if cursor != "" {
		n, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil || n < 0 {
			return out, ErrInput
		}
		after = n
	}
	gated5, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	query := `SELECT id,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_receipts.item_id),''),profile_id,revoked_reason,revoked_ms,sequence FROM download_receipts WHERE revoked_ms>0 AND sequence>?`
	args := []any{after}
	if !anyViewer {
		query += ` AND profile_key=?`
		args = append(args, operations.ViewerKey(p))
	}
	query += ` ORDER BY sequence LIMIT ?`
	args = append(args, limit+1)
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var entry Revocation
		var revoked int64
		if e = rows.Scan(&entry.ReceiptID, &entry.ItemID, &entry.ProfileID, &entry.Reason, &revoked, &entry.Sequence); e != nil {
			rows.Close()
			return out, e
		}
		entry.RevokedAt = stamp(revoked)
		out.Items = append(out.Items, entry)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = strconv.FormatInt(out.Items[limit-1].Sequence, 10)
	}
	return out, gated5.Commit()
}
