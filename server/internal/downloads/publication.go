package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/reading"
)

func readingKind(kind string) bool { return kind == "ebook" || kind == "comic" }

// Installed once with the router's actual Reading authority/source service.
func (s *Service) SetReading(service *reading.Service) {
	s.accessMu.Lock()
	s.reading = service
	s.accessMu.Unlock()
}
func (s *Service) readingService() *reading.Service {
	s.accessMu.RLock()
	defer s.accessMu.RUnlock()
	return s.reading
}
func (s *Service) publicationForRequest(ctx context.Context, p identity.Principal, r Request) (*reading.OfflineDocument, error) {
	if r.MediaID == "" {
		return nil, nil
	}
	snap, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return nil, err
	}
	tx := snap.Tx()
	found, err := resolveVersionSource(ctx, tx, r.MediaID, r.VersionID)
	if err != nil {
		snap.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if !readingKind(found.Kind) {
		snap.Rollback()
		return nil, nil
	}
	if r.VersionID == "" || len(r.MediaIDs) > 0 || r.NextAfterMediaID != "" || (r.Quality != "" && string(r.Quality) != QualityOriginal) {
		snap.Rollback()
		return nil, ErrInput
	}
	if err = s.visibleItem(ctx, tx, p, r.MediaID); err != nil {
		snap.Rollback()
		return nil, err
	}
	allowed, err := ProfileAllowsDownloads(tx, p.Viewer)
	snap.Rollback()
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, identity.ErrForbidden
	}
	reader := s.readingService()
	if reader == nil {
		return nil, ErrInput
	}
	doc, err := reader.OfflineDocument(ctx, p, r.MediaID, r.VersionID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(doc)
	if err != nil || len(raw) > 2<<20 {
		return nil, ErrInput
	}
	return &doc, nil
}
func (s *Service) checkPublicationTx(ctx context.Context, tx *sql.Tx, p identity.Principal, found source, doc *reading.OfflineDocument) error {
	if !readingKind(found.Kind) {
		return nil
	}
	reader := s.readingService()
	if reader == nil || doc == nil || doc.ItemID != found.Item || doc.SourceID != found.Asset || doc.Size != found.Size {
		return ErrInput
	}
	selected, err := reader.SelectOfflineTx(ctx, tx, p, found.Item, found.Asset)
	if err != nil {
		return err
	}
	if selected.Revision != doc.Revision || selected.Format != doc.Format || selected.ModifiedNS != found.ModifiedNS {
		return reading.ErrChanged
	}
	return nil
}

// ValidatePublicationReceipt requires both the cryptographic envelope and the
// current authoritative receipt row, current profile entitlement and content access.
func (s *Service) ValidatePublicationReceipt(ctx context.Context, p identity.Principal, serverID string, envelope Receipt) (ReceiptClaims, error) {
	keys, err := s.ReceiptKeys(ctx)
	if err != nil {
		return ReceiptClaims{}, err
	}
	claims, err := VerifyReceipt(keys, envelope, s.now())
	if err != nil {
		return claims, err
	}
	if claims.Publication == nil || claims.Viewer.Authority != p.Authority || claims.Viewer.AccountID != p.AccountID || claims.Viewer.ProfileID != p.ProfileID || claims.Viewer.ServerID != serverID {
		return claims, identity.ErrForbidden
	}
	snap, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return claims, err
	}
	defer snap.Rollback()
	tx := snap.Tx()
	var preparation, digest string
	var bytes, expires, revoked int64
	err = tx.QueryRowContext(ctx, `SELECT preparation_id,artifact_digest,artifact_bytes,expires_ms,revoked_ms FROM download_receipts WHERE id=? AND profile_key=?`, claims.ReceiptID, operations.ViewerKey(p)).Scan(&preparation, &digest, &bytes, &expires, &revoked)
	if err != nil {
		return claims, identity.ErrForbidden
	}
	if revoked != 0 || expires < s.millis() || preparation != claims.PreparationID || digest != claims.Artifact.SHA256 || bytes != claims.Artifact.Bytes {
		return claims, identity.ErrForbidden
	}
	allowed, err := ProfileAllowsDownloads(tx, p.Viewer)
	if err != nil {
		return claims, err
	}
	if !allowed {
		return claims, identity.ErrForbidden
	}
	if err = s.visibleItem(ctx, tx, p, claims.ItemID); err != nil {
		return claims, err
	}
	doc, err := publicationTx(ctx, tx, preparation)
	if err != nil {
		return claims, err
	}
	if doc == nil || doc.ItemID != claims.ItemID || doc.SourceID != claims.Publication.SourceID || doc.Revision != claims.Publication.Revision || doc.Format != claims.Publication.Format || doc.Digest() != claims.Publication.DocumentSHA256 {
		return claims, identity.ErrForbidden
	}
	return claims, nil
}
func publicationJSON(doc *reading.OfflineDocument) string {
	if doc == nil {
		return ""
	}
	raw, _ := json.Marshal(doc)
	return string(raw)
}
func publicationTx(ctx context.Context, tx *sql.Tx, id string) (*reading.OfflineDocument, error) {
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT publication_json FROM download_preparations WHERE id=?`, id).Scan(&raw); err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var doc reading.OfflineDocument
	if len(raw) > 2<<20 || json.Unmarshal([]byte(raw), &doc) != nil {
		return nil, ErrInput
	}
	return &doc, nil
}
