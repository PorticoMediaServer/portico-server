package recordingroot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/storagepaths"
)

// Activate runs once before the recording driver, storageIO and workers become
// available. bind must construct all stores against the one chosen root and
// return a rollback-only closer. It must not publish or start workers. Nothing
// is moved/deleted; an unsafe pending request fails startup visibly.
func Activate(ctx context.Context, db *sql.DB, admin *administration.Service, ident *identity.Service, data, operator string, bind func(string) (rollback func(), verify func() error, err error)) (err error) {
	if db == nil || admin == nil || ident == nil || bind == nil || !validRootPath(operator) { return errors.New("recording root activation unavailable") }
	gated, err := dbwork.Begin(ctx, db, dbwork.ClassInteractive)
	if err != nil { return err }
	defer gated.Rollback()
	tx := gated.Tx()
	request, err := operations.RecordingRootRequestTx(tx)
	if err != nil { return err }
	chosen := operator
	if request.Path != "" {
		if operator != "" && operator != request.Path { return errors.New("recording folder is operator controlled") }
		chosen = request.Path
	}
	pending := request.Path != "" && (request.Path != request.AppliedPath || request.FolderIdentity != request.AppliedIdentity)
	expectedIdentity := request.AppliedIdentity
	if chosen != "" {
		if err = storagepaths.At(data).ValidateRecordings(chosen); err != nil { return err }
	}
	if pending {
		var actor []string
		if json.Unmarshal([]byte(request.RequestedBy), &actor) != nil || len(actor) != 3 { return identity.ErrForbidden }
		if err = ident.AuthorizeRecordingRootOwnerTx(ctx, tx, actor[0], actor[1], actor[2], request.RequestedEpoch); err != nil { return err }
		if err = dvr.RetainedRootReferences(ctx, tx); err != nil { return err }
		if request.AppliedPath != "" {
			old, e := storagepaths.EmptyRecordingRootIdentity(request.AppliedPath)
			if e != nil || old != request.AppliedIdentity { return errors.New("previous recording folder contains content or changed") }
		}
		expectedIdentity, err = admin.RecordingDestination(chosen)
		if err != nil { return err }
		if expectedIdentity != request.FolderIdentity { return errors.New("requested recording folder changed") }
	} else if chosen != "" {
		// Preserve existing operator bootstrap behavior. A previously accepted
		// root must exist with the same identity; it is never silently recreated.
		current, e := storagepaths.RecordingRootIdentity(chosen)
		if e != nil {
			if request.AppliedPath != "" || !errors.Is(e, os.ErrNotExist) { return e }
		} else {
			if request.AppliedPath != "" && (chosen != request.AppliedPath || current != request.AppliedIdentity) { return errors.New("effective recording folder changed") }
			expectedIdentity = current
		}
	}
	rollback, verify, err := bind(chosen)
	if err != nil { if rollback != nil { rollback() }; return err }
	if rollback == nil || verify == nil { if rollback != nil { rollback() }; return errors.New("recording root rollback unavailable") }
	defer func() { if err != nil { rollback() } }()
	actualIdentity := ""
	if chosen != "" {
		if pending { actualIdentity, err = admin.RecordingRootIdentity(chosen) } else { actualIdentity, err = storagepaths.RecordingRootIdentity(chosen) }
		if err != nil { return err }
		if expectedIdentity != "" && actualIdentity != expectedIdentity { return errors.New("recording folder changed while binding") }
	}
	if err = verify(); err != nil { return err }
	result, err := tx.ExecContext(ctx, `UPDATE recording_root_request SET applied_path=?,applied_identity=? WHERE singleton=1 AND revision=? AND path=? AND folder_identity=?`, chosen, actualIdentity, request.Revision, request.Path, request.FolderIdentity)
	if err != nil { return err }
	changed, err := result.RowsAffected()
	if err != nil { return err }
	if changed != 1 { return operations.ErrConflict }
	return gated.Commit()
}
