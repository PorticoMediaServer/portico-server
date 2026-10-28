package assets

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/storage"
	"time"
)

type PlaybackSource struct {
	Path             string
	Size, ModifiedNS int64
	Binding          *storage.PlaybackBinding
}

func ResolvePlaybackSource(ctx context.Context, db *sql.DB, session, grantHash string) (PlaybackSource, error) {
	var v PlaybackSource
	var asset, source, incarnation string
	var generation int64
	e := db.QueryRowContext(ctx, `SELECT a.path,COALESCE(pin.size,a.size),COALESCE(pin.modified_ns,a.modified_ns),a.token,COALESCE(physical.source_id,''),COALESCE(physical.incarnation,''),COALESCE(physical.configuration_generation,0) FROM playback_sessions ps JOIN catalog_assets a ON a.token=ps.asset_id LEFT JOIN playback_source_pins pin ON pin.session_id=ps.id LEFT JOIN playback_physical_source_pins physical ON physical.session_id=ps.id WHERE (ps.id=? OR ps.grant_hash=?) AND a.available=1 AND (pin.session_id IS NULL OR (pin.asset_id=a.token AND pin.size=a.size AND pin.modified_ns=a.modified_ns))`, session, grantHash).Scan(&v.Path, &v.Size, &v.ModifiedNS, &asset, &source, &incarnation, &generation)
	if e != nil {
		return PlaybackSource{}, e
	}
	v.Binding, e = resolvePlaybackBinding(ctx, db, v, asset, source, incarnation, generation)
	return v, e
}

// ResolveCatalogPlaybackSource retains the same scan mapping and acquisition
// requirements for reads without a session, including DLNA's authorized item.
func ResolveCatalogPlaybackSource(ctx context.Context, db *sql.DB, asset string) (PlaybackSource, error) {
	var v PlaybackSource
	err := db.QueryRowContext(ctx, `SELECT path,size,modified_ns FROM catalog_assets WHERE token=? AND available=1`, asset).Scan(&v.Path, &v.Size, &v.ModifiedNS)
	if err != nil {
		return PlaybackSource{}, err
	}
	v.Binding, err = resolvePlaybackBinding(ctx, db, v, asset, "", "", 0)
	return v, err
}

func resolvePlaybackBinding(ctx context.Context, db *sql.DB, v PlaybackSource, asset, source, incarnation string, generation int64) (*storage.PlaybackBinding, error) {
	var binding storage.PlaybackBinding
	var valid bool
	var kind string
	err := db.QueryRowContext(ctx, `SELECT s.root,s.root_identity,s.follow_symlinks,o.relative_path,o.object_identity,s.kind,
 o.size=? AND o.modified_ns=? AND o.retired=0 AND o.state='available' AND s.kind IN('local','remote') AND s.enabled=1 AND s.identity_confirmed=1 AND s.health NOT IN('offline','root_changed','removing') AND o.root_incarnation=s.incarnation AND (?='' OR (s.incarnation=? AND s.generation=?))
 FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id
 WHERE o.asset_id=? AND (?='' OR s.id=?)
 ORDER BY (rtrim(s.root,?)||?||o.relative_path=?) DESC,(o.retired=0 AND o.state='available' AND o.root_incarnation=s.incarnation) DESC,s.id,o.id LIMIT 1`, v.Size, v.ModifiedNS, source, incarnation, generation, asset, source, source, string(filepath.Separator), string(filepath.Separator), filepath.Clean(v.Path)).Scan(&binding.Inventory.Root, &binding.Inventory.RootIdentity, &binding.Inventory.FollowSymlinks, &binding.Inventory.RelativePath, &binding.ObjectIdentity, &kind, &valid)
	if errors.Is(err, sql.ErrNoRows) && source == "" {
		// Source-less publications can have no inventory row. This preserves
		// their existing no-follow acquisition; it is not root/object custody.
		// Persistent acceptance evidence for this case remains a schema task.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !valid || binding.ObjectIdentity == "" || binding.Inventory.RootIdentity == "" || !filepath.IsLocal(binding.Inventory.RelativePath) || filepath.Join(binding.Inventory.Root, binding.Inventory.RelativePath) != filepath.Clean(v.Path) {
		return nil, storage.ErrPlaybackSource
	}
	// Native remote inventory IDs describe provider objects, not the inodes
	// exposed by rclone/FUSE. This mapping only permits the existing supervised
	// mounted-file path after the client confirms its managed mount registry.
	// It does not establish immutable accepted-object custody for that path.
	binding.Mounted = kind == "remote"
	return &binding, nil
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
	return OpenIsolatedSource(ctx, c, v)
}

// OpenIsolatedSource opens a catalogued file through the storage helper. Local
// scan evidence binds its filesystem identity; remote inventory requires the
// registered physical mount and retains the existing size/mtime check. Missing
// or stale evidence never silently downgrades a mapped source.
func OpenIsolatedSource(ctx context.Context, c *storage.Client, v PlaybackSource) (*IsolatedPlaybackReader, error) {
	// A local file is opened and version-checked once by the supervised helper,
	// which hands back the open descriptor and exits; the bytes are then read
	// in-process. The fully isolated reader keeps a helper process alive for
	// the whole response and moves every 32 KB chunk through a JSON command
	// and two pipes, which is the right price for a Portico-managed network
	// mount (a hung mount blocks the child, not the server) and far too high
	// for local disks: fifty direct-play streams were fifty-plus processes and
	// a process launch for every range request a player issues.
	if storage.DescriptorPassing && (c.MountedRoot == nil || c.MountedRoot(v.Path) == "") {
		file, err := c.OpenPlaybackDescriptor(ctx, v.Path, v.Size, v.ModifiedNS, v.Binding)
		if err != nil {
			return nil, err
		}
		return &IsolatedPlaybackReader{file, time.Unix(0, v.ModifiedNS)}, nil
	}
	f, e := c.OpenPlayback(ctx, v.Path, v.Size, v.ModifiedNS, v.Binding)
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
	return c.OpenPlaybackDescriptor(ctx, v.Path, v.Size, v.ModifiedNS, v.Binding)
}
