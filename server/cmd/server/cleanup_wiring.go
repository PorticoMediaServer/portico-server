package main

import (
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

// Retention work follows its inputs, not unrelated telemetry or control writes.
// Unknown write scopes still wake this subscription, and worker.Run retains its
// safety tick. The cleanup functions and their deadlines are unchanged.
func minuteCleanupWake() (*worker.Signal, func()) {
	wake := worker.NewSignal()
	stop := dbwork.WakeOnTables(wake,
		"playback_sessions", "personal_*", "playlist_receipts", "saved_resource_receipts",
		"lyric_candidates", "artwork_*", "metadata_owner_fields",
		"catalog_*", "books", "book_files", "inventory_*",
		"libraries", "library_*", "server_settings",
	)
	return wake, stop
}
