package main

import (
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

func subscribeListeningGroups(wake, changed *worker.Signal) func() {
	stopWake := dbwork.WakeOnTables(wake, "catalog_books", "catalog_book_context")
	stopChanged := dbwork.WakeOnTables(changed, "catalog_books", "catalog_book_context")
	return func() { stopWake(); stopChanged() }
}
