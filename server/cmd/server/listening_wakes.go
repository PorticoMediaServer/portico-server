package main

import (
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

func subscribeListeningGroups(wake, changed *worker.Signal) func() {
	stopWake := dbwork.WakeOnTables(wake, "books", "book_files")
	stopChanged := dbwork.WakeOnTables(changed, "books", "book_files")
	return func() { stopWake(); stopChanged() }
}
