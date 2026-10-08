package subtitles

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/identity"
	"time"
)

type scanTextAuthorityKey struct{}
type scanTextAuthority struct {
	item, source string
	check        func(context.Context, *sql.Tx) error
}

// ImportScanText is called only by the durable inventory analysis worker after
// its exact source revision has been published. The caller's inventory fence is
// checked inside every read/write transaction; no viewer credential is forged or
// borrowed. The internal actor can import shared resources for this one source.
func (s *Service) ImportScanText(ctx context.Context, item, source string, check func(context.Context, *sql.Tx) error) error {
	if check == nil {
		return ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	select {
	case s.scanTextSlot <- struct{}{}:
		defer func() { <-s.scanTextSlot }()
	case <-ctx.Done():
		return ctx.Err()
	}
	ctx = context.WithValue(ctx, scanTextAuthorityKey{}, scanTextAuthority{item, source, check})
	principal := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "inventory-analysis", ProfileID: "inventory-analysis", Role: "owner"}}
	gate, p, e := s.tx(ctx, principal, item)
	if e != nil {
		return e
	}
	catalog, e := s.ListTx(ctx, gate.Tx(), p, item, source)
	gate.Rollback()
	if e != nil {
		return e
	}
	imported := 0
	for _, track := range catalog.Discovered {
		if track.Origin != "embedded" || !track.Enabled || !textFormat(track.Format) {
			continue
		}
		if imported >= 8 {
			break
		}
		imported++
		_, e = s.Import(ctx, principal, item, ImportRequest{OperationID: identity.Digest(fmt.Sprintf("scan-text:%s:%s:%d", source, track.ID, track.Revision)), SourceID: source, DiscoveryID: track.ID, ExpectedRevision: track.Revision, Scope: "shared", Render: "text"})
		if e != nil {
			return e
		}
	}
	return nil
}
func textFormat(format string) bool {
	switch format {
	case "srt", "vtt", "ass", "ssa":
		return true
	}
	return false
}
