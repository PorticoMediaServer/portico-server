package main

import (
	"context"
	"database/sql"
	"log"
	"path/filepath"
	"time"

	"portico.local/server/internal/access"
	"portico.local/server/internal/connectivity"
	"portico.local/server/internal/httpapi"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/servicelog"
)

// initializeAdministration wires workstream G: the access store, the message log
// recorder, client log uploads and LAN discovery.
//
// The recorder becomes the standard library log package's output, so every
// existing log.Printf call in the server lands in the message log and the live
// tail without a single call site changing. Output still reaches stderr too,
// because a container's operator reads stderr and must not lose it.
func initializeAdministration(ctx context.Context, db *sql.DB, console *operations.Store, ident *identity.Service, state string, port int) (httpapi.AccessArea, func()) {
	recorder := servicelog.New(servicelog.Options{Directory: filepath.Join(state, "logs")})
	log.SetOutput(recorder.Tee(log.Writer()))
	advertiser := connectivity.NewAdvertiser(connectivity.AdvertiserOptions{Instance: ident.Name(), Host: ident.Name(), Port: port,
		TXT: []string{"serverId=" + ident.ID(), "product=portico"}})
	out := httpapi.AccessArea{Access: access.New(db), Logs: recorder, ClientLogs: servicelog.NewClientLogStore(db), Advertiser: advertiser}
	if console != nil {
		if document, err := console.Settings(ctx, operations.AllowServerScope); err == nil {
			recorder.SetLevel(document.Effective.LogLevel)
			recorder.SetRetention(document.Effective.LogRetentionDays())
			advertiser.Apply(document.Effective.LANDiscoveryEnabled)
		}
	}
	// A debug window that was open when the process stopped stays open: an owner
	// who raised the level to catch an intermittent failure should not lose it to
	// the restart that failure caused.
	var expires int64
	if err := db.QueryRowContext(ctx, `SELECT expires_ms FROM diagnostics_debug_window WHERE singleton=1`).Scan(&expires); err == nil {
		recorder.OpenDebugWindow(time.UnixMilli(expires).UTC())
	}
	return out, func() {
		advertiser.Close()
		_ = recorder.Close()
	}
}
