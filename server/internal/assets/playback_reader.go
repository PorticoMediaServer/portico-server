package assets

import (
	"context"
	"database/sql"
	"io"
	"os"
	"portico.local/server/internal/storage"
	"time"
)

type PlaybackSource struct {
	Path             string
	Size, ModifiedNS int64
}

func ResolvePlaybackSource(ctx context.Context, db *sql.DB, session, grantHash string) (PlaybackSource, error) {
	var v PlaybackSource
	e := db.QueryRowContext(ctx, `SELECT a.path,COALESCE(pin.size,a.size),COALESCE(pin.modified_ns,a.modified_ns) FROM playback_sessions ps JOIN catalog_assets a ON a.token=ps.asset_id LEFT JOIN playback_source_pins pin ON pin.session_id=ps.id WHERE (ps.id=? OR ps.grant_hash=?) AND a.available=1 AND (pin.session_id IS NULL OR (pin.asset_id=a.token AND pin.size=a.size AND pin.modified_ns=a.modified_ns))`, session, grantHash).Scan(&v.Path, &v.Size, &v.ModifiedNS)
	return v, e
}

type IsolatedPlaybackReader struct {
	io.ReadSeekCloser
	Modified time.Time
}

func OpenIsolatedPlayback(ctx context.Context, db *sql.DB, c *storage.Client, session, grantHash string) (*IsolatedPlaybackReader, error) {
	v, e := ResolvePlaybackSource(ctx, db, session, grantHash)
	if e != nil {
		return nil, e
	}
	// A local file is opened and version-checked once by the supervised helper,
	// which hands back the open descriptor and exits; the bytes are then read
	// in-process. The fully isolated reader keeps a helper process alive for
	// the whole response and moves every 32 KB chunk through a JSON command
	// and two pipes, which is the right price for a Portico-managed network
	// mount (a hung mount blocks the child, not the server) and far too high
	// for local disks: fifty direct-play streams were fifty-plus processes and
	// a process launch for every range request a player issues.
	if storage.DescriptorPassing && (c.MountedRoot == nil || c.MountedRoot(v.Path) == "") {
		file, err := c.OpenPlaybackDescriptor(ctx, v.Path, v.Size, v.ModifiedNS)
		if err != nil {
			return nil, err
		}
		return &IsolatedPlaybackReader{file, time.Unix(0, v.ModifiedNS)}, nil
	}
	f, e := c.OpenPlayback(ctx, v.Path, v.Size, v.ModifiedNS)
	if e != nil {
		return nil, e
	}
	return &IsolatedPlaybackReader{f, time.Unix(0, v.ModifiedNS)}, nil
}

// OpenIsolatedPlaybackDescriptor is the Unix-only form: it returns the
// transferred descriptor and nothing else. Converted playback uses
// OpenPlaybackInput instead, which also says what to hand ffmpeg — because on
// Windows that is a path, not a descriptor.
func OpenIsolatedPlaybackDescriptor(ctx context.Context, db *sql.DB, c *storage.Client, session string) (*os.File, error) {
	v, e := ResolvePlaybackSource(ctx, db, session, "")
	if e != nil {
		return nil, e
	}
	return c.OpenPlaybackDescriptor(ctx, v.Path, v.Size, v.ModifiedNS)
}
