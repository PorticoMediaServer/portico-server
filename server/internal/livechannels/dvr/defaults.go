package dvr

import (
	"context"
	"database/sql"
)

func DefaultOptionsTx(ctx context.Context, tx *sql.Tx) (Options, error) {
	var o Options
	err := tx.QueryRowContext(ctx, `SELECT before_seconds,after_seconds,retention_days,episode_limit FROM dvr_defaults WHERE singleton=1`).Scan(&o.BeforeSeconds, &o.AfterSeconds, &o.RetentionDays, &o.EpisodeLimit)
	return o, err
}
