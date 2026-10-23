package administration

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/livechannels"
)

// logoImportCandidateTx fences a candidate against current policy and the
// source's current publication. A changed URL or an owner upload made while a
// provider was responding cannot be replaced using an older snapshot.
func logoImportCandidateTx(ctx context.Context, tx *sql.Tx, source, channel, locator string) (bool, error) {
	mode, enabled, err := livechannels.ReadSourceLogoImportTx(ctx, tx, source)
	if err != nil || !enabled {
		return false, err
	}
	var current string
	var stored bool
	err = tx.QueryRowContext(ctx, `SELECT l.url,EXISTS(SELECT 1 FROM admin_channel_logos a WHERE a.channel_id='live:'||s.id||':'||l.channel_id) FROM live_sources s JOIN live_channel_logos l ON l.generation_id=s.active_generation WHERE s.id=? AND l.channel_id=?`, source, channel).Scan(&current, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && current == locator && (mode.Mode == "replace" || !stored), err
}

func logoImportAuthority(auth Authorize, source, channel, locator string) Authorize {
	return func(ctx context.Context, tx *sql.Tx) error {
		if auth != nil {
			if err := auth(ctx, tx); err != nil {
				return err
			}
		}
		allowed, err := logoImportCandidateTx(ctx, tx, source, channel, locator)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrConflict
		}
		return nil
	}
}

func guideImageImportCandidateTx(ctx context.Context, tx *sql.Tx, source, generation, locator string) (bool, error) {
	_, enabled, err := livechannels.ReadSourceLogoImportTx(ctx, tx, source)
	if err != nil || !enabled {
		return false, err
	}
	var current bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_sources s JOIN live_programme_icons i ON i.generation_id=s.active_generation WHERE s.id=? AND s.active_generation=? AND i.url=? AND NOT EXISTS(SELECT 1 FROM live_programme_images m WHERE m.url=i.url))`, source, generation, locator).Scan(&current)
	return current, err
}
