package reading

import (
	"context"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// Root20: only recognized explicit fields classify a publication. Generic serial,
// cadence, dates, folder/title words, MIME and subjects are not type evidence.
func explicitPublicationType(values []string) string {
	accepted := ""
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if !compactcatalog.ValidReadingPublicationType(value) {
			continue
		}
		if accepted != "" && accepted != value {
			return ""
		}
		accepted = value
	}
	return accepted
}

// File facts are independent of owner rows. Adoption checks the original current
// source and principal again in the write transaction; unknown never becomes Book.
func (s *Service) adoptPublicationType(ctx context.Context, p identity.Principal, selected Selection, value string) error {
	if selected.Format != "epub" && selected.Format != "cbz" {
		return nil
	}
	if value != "" && !compactcatalog.ValidReadingPublicationType(value) {
		return ErrInvalid
	}
	gate, err := dbwork.Begin(ctx, s.DB, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gate.Rollback()
	current, err := s.selectTx(ctx, gate.Tx(), p, selected.ItemID, selected.SourceID)
	if err != nil {
		return err
	}
	if current.Revision != selected.Revision || current.object != selected.object || current.inventory != selected.inventory {
		return ErrChanged
	}
	if value == "" {
		_, err = gate.Tx().ExecContext(ctx, `DELETE FROM reading_publication_types WHERE item_id=? AND asset_id=?`, current.Entity, current.Asset)
	} else {
		_, err = gate.Tx().ExecContext(ctx, `INSERT INTO reading_publication_types(item_id,asset_id,publication_type,inventory_object_id,source_revision,classifier_version)
          VALUES(?,?,?,?,?,1) ON CONFLICT(item_id,asset_id) DO UPDATE SET publication_type=excluded.publication_type,inventory_object_id=excluded.inventory_object_id,source_revision=excluded.source_revision,classifier_version=excluded.classifier_version
          WHERE reading_publication_types.publication_type<>excluded.publication_type OR reading_publication_types.inventory_object_id<>excluded.inventory_object_id OR reading_publication_types.source_revision<>excluded.source_revision OR reading_publication_types.classifier_version<>excluded.classifier_version`, current.Entity, current.Asset, value, current.object, current.inventory)
	}
	if err != nil {
		return err
	}
	return gate.Commit()
}
