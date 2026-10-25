package operations

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/identity"
)

// RecordingRootRuntime is captured once by startup, never rebound by a save.
// Admit must hold the recording-storage lifecycle exclusively until release,
// validate actual folder identity/authority/outside-data constraints, and refuse
// retained references, old or destination content, and root-bound work.
type RecordingRootRuntime struct {
	Effective string
	Identity string
	OperatorControlled bool
	Admit func(context.Context, *sql.Tx, string, string) (folderIdentity string, release func(), err error)
}

type RecordingRootRequest struct {
	Revision int64 `json:"revision"`
	Path string `json:"path"`
	// Private activation pin; never projected to a client.
	FolderIdentity string `json:"-"`
	RequestedBy string `json:"-"`
}

type RecordingRootStatus struct {
	Revision int64 `json:"revision"`
	Requested string `json:"requested"`
	Effective string `json:"effective"`
	PendingRestart bool `json:"pendingRestart"`
	OperatorControlled bool `json:"operatorControlled"`
}

type RecordingRootChange struct {
	ExpectedRevision int64 `json:"expectedRevision"`
	IdempotencyKey string `json:"idempotencyKey"`
	Path string `json:"path"`
}

func readRecordingRoot(tx *sql.Tx) (out RecordingRootRequest, err error) {
	err = tx.QueryRow(`SELECT revision,path,folder_identity,requested_by FROM recording_root_request WHERE singleton=1`).Scan(&out.Revision, &out.Path, &out.FolderIdentity, &out.RequestedBy)
	return
}

func (s *Store) recordingRootStatus(request RecordingRootRequest) RecordingRootStatus {
	out := RecordingRootStatus{Revision: request.Revision, Requested: request.Path}
	if s.RecordingRoot != nil {
		out.Effective = s.RecordingRoot.Effective
		out.OperatorControlled = s.RecordingRoot.OperatorControlled
	}
	out.PendingRestart = out.Requested != "" && out.Requested != out.Effective
	return out
}

func (s *Store) RecordingRootStatus(ctx context.Context, p identity.Principal, auth Authorize) (out RecordingRootStatus, err error) {
	if p.Authority != "local" || p.Role != "owner" {
		return out, identity.ErrUnauthorized
	}
	err = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		request, e := readRecordingRoot(tx)
		if e == nil { out = s.recordingRootStatus(request) }
		return e
	})
	return
}

// ApplyRecordingRoot only persists a requested destination. Startup must recheck
// the saved revision/identity/authority/content/work before accepting it. This
// method neither restarts nor modifies the current driver or filesystem.
func (s *Store) ApplyRecordingRoot(ctx context.Context, p identity.Principal, auth Authorize, change RecordingRootChange) (out RecordingRootStatus, err error) {
	if p.Authority != "local" || p.Role != "owner" {
		return out, identity.ErrUnauthorized
	}
	var release func()
	defer func() { if release != nil { release() } }()
	err = s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		scope := "recording-root:" + AccountKey(p)
		raw, digest, e := Receipt(tx, scope, change.IdempotencyKey, change, s.now())
		if e != nil { return e }
		if raw != "" {
			var saved RecordingRootRequest
			if e = decodeDocument(raw, &saved); e != nil { return e }
			out = s.recordingRootStatus(saved)
			return nil
		}
		if change.Path == "" || !validTemporaryDirectory(change.Path) { return invalidFields("path") }
		current, e := readRecordingRoot(tx)
		if e != nil { return e }
		if current.Revision != change.ExpectedRevision { return &ConflictError{current.Revision} }
		runtime := s.RecordingRoot
		if runtime == nil || runtime.Admit == nil { return errors.New("recording root admission unavailable") }
		if runtime.OperatorControlled && change.Path != runtime.Effective { return invalidFields("path") }
		folderIdentity := runtime.Identity
		if change.Path != runtime.Effective {
			folderIdentity, release, e = runtime.Admit(ctx, tx, runtime.Effective, change.Path)
			if e != nil { return e }
			if folderIdentity == "" || release == nil { return errors.New("recording root identity unavailable") }
		}
		if folderIdentity == "" { return errors.New("recording root identity unavailable") }
		saved := current
		if current.Path != change.Path || current.FolderIdentity != folderIdentity || current.RequestedBy != ViewerKey(p) {
			saved = RecordingRootRequest{Revision: current.Revision + 1, Path: change.Path, FolderIdentity: folderIdentity, RequestedBy: ViewerKey(p)}
			if _, e = tx.Exec(`UPDATE recording_root_request SET revision=?,path=?,folder_identity=?,requested_by=? WHERE singleton=1 AND revision=?`, saved.Revision, saved.Path, saved.FolderIdentity, saved.RequestedBy, current.Revision); e != nil { return e }
		}
		out = s.recordingRootStatus(saved)
		return SaveReceipt(tx, scope, change.IdempotencyKey, digest, saved, s.now())
	})
	return
}
