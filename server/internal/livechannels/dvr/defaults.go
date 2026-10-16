package dvr

import (
	"context"
	"database/sql"
)

func DefaultOptionsTx(ctx context.Context, tx *sql.Tx) (Options, error) {
	var o Options
	err := tx.QueryRowContext(ctx, `SELECT before_seconds*1000,after_seconds*1000,retention_days,episode_limit FROM dvr_defaults WHERE singleton=1`).Scan(&o.BeforeMs, &o.AfterMs, &o.RetentionDays, &o.EpisodeLimit)
	return o, err
}
