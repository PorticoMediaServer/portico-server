package subtitles

import (
	"context"
	"database/sql"
	"portico.local/server/internal/dbwork"
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
	// Discovery creates selectable resources. Extraction is a cache fill at
	// first delivery, so basic scans never demux the whole media file.
	snapshot, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return e
	}
	defer snapshot.Rollback()
	return check(ctx, snapshot.Tx())
}

func textFormat(format string) bool {
	switch format {
	case "srt", "vtt", "ass", "ssa", "smi", "sami", "ttml", "dfxp":
		return true
	}
	return false
}
