package administration

import (
	"context"
	"database/sql"
	"portico.local/server/internal/apievents"
	"portico.local/server/internal/dbwork"
	"time"
)

// Only an invalidation hint reaches administrators. Release facts stay in the
// owner's reads; this never installs an update or performs another feed fetch.
func (s *Service) publishUpdateTransition(ctx context.Context, previous, next UpdateReport) error {
	if previous.State == next.State && previous.UpdateAvailable == next.UpdateAvailable && updateVersion(previous) == updateVersion(next) {
		return nil
	}
	if s.db == nil {
		return nil
	}
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		return apievents.Append(tx, apievents.AdminAudience, "server.update", "server", "updates", next.CheckedAt, nil)
	})
}
func updateVersion(r UpdateReport) string {
	if r.Latest != nil {
		return r.Latest.Version
	}
	return ""
}
func (s *Service) cachedUpdateReport(ctx context.Context, current UpdateBuild, channel, feed string, data []byte, err error, checked time.Time) UpdateReport {
	out := s.updatesFrom(ctx, current, channel, feed, func(context.Context, string) ([]byte, error) { return data, err })
	out.CheckedAt = checked.UTC().Format(time.RFC3339)
	return out
}
