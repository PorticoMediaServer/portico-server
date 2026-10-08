package downloads

import (
	"database/sql"

	"portico.local/server/internal/identity"
)

// DownloadNotifier is the seam the notifications workstream fills in.
//
// internal/operations publishes Notify(tx, now, principal, code, target, dedupe)
// for its own inbox, but no documented download hook — there is no
// operations.NotifyDownload today. Rather than write into another workstream's
// table from here and guess at its dedupe and retention rules, downloads calls
// this interface and ships a no-op default.
//
// TODO(notifications workstream): implement operations.NotifyDownload and wire
// it as the default Notifier in cmd/server, keeping these two calls. The codes
// a client should expect are "download.ready" and "download.failed", with the
// preparation id as the notification target.
type DownloadNotifier interface {
	DownloadReady(tx *sql.Tx, now int64, viewer identity.Viewer, preparationID, itemID string) error
	DownloadFailed(tx *sql.Tx, now int64, viewer identity.Viewer, preparationID, itemID, reason string) error
}

type noopNotifier struct{}

func (noopNotifier) DownloadReady(*sql.Tx, int64, identity.Viewer, string, string) error { return nil }
func (noopNotifier) DownloadFailed(*sql.Tx, int64, identity.Viewer, string, string, string) error {
	return nil
}
