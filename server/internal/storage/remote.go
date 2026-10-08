package storage

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediasource"
)

// RemoteSources is the adapter-specific seam used by the existing path-based
// catalog, durable inventory and acquisition. Paths are assigned virtual roots, never URLs.
// Local and externally mounted paths continue through the physical helper.
type RemoteSources interface {
	Handles(string) bool
	InspectRoot(context.Context, string) (Snapshot, error)
	Stat(context.Context, string) (Snapshot, error)
	ListPage(context.Context, string, string, string) (RemotePage, error)
	CommitPage(context.Context, *sql.Tx, RemotePage) error
	CheckScan(context.Context, string, string) error
	ValidateScan(context.Context, *sql.Tx, string, string) error
	RecordReference(context.Context, *sql.Tx, string, string) error
	Acquire(context.Context, string, string) (RemoteObject, error)
}

// RemoteInventoryVerifier is additive: peer acquisition consumers continue to
// use RemoteSources. An inventory consumer requires fresh complete-child proof.
type RemoteInventoryVerifier interface {
	VerifyInventoryDirectory(context.Context, string, string, string) error
}

func RemoteDirectoryIdentity(source, path string) string {
	return "remote-dir:" + source + ":" + identity.Digest(path)
}

type RemotePage struct {
	ID, SourceID, Directory, Generation, Cursor, Next string
	Entries                                           []Snapshot
	Complete, Pending                                 bool
}

// RemoteObject owns a captured configuration and either a conditional HTTP
// representation or one retained sequential acquisition. A zero Version is
// deliberately not promoted to an immutable version from size/mtime alone.
type RemoteObject interface {
	Snapshot() Snapshot
	Version() mediasource.Version
	Access() string
	Observe(context.Context) (Snapshot, error)
	OpenRange(context.Context, int64, int64) (io.ReadCloser, error)
	Close() error
}

var (
	ErrRemoteAnalysisDisabled = errors.New("STRM target analysis is not enabled")
	ErrRemoteConfig           = errors.New("remote source configuration is invalid")
	ErrRemoteChanged          = errors.New("remote source configuration or object changed")
	ErrRemoteOffline          = errors.New("remote source is unavailable")
	ErrRemoteCredentials      = errors.New("remote source credentials require attention")
	ErrRemoteRange            = errors.New("remote source does not support safe byte ranges")
	ErrRemoteCursor           = errors.New("remote inventory continuation expired; start a new scan")
	ErrRemoteLimit            = errors.New("remote source operation exceeded its safety budget")
	ErrRemoteBinary           = errors.New("approved rclone executable changed or is unavailable")
)

func (c *Client) IsRemote(path string) bool {
	return c != nil && c.Remote != nil && c.Remote.Handles(path)
}
