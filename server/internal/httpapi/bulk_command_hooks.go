package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"portico.local/server/internal/administration"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
)

func metadataJobEdit(a catalog.JobArguments) metadata.BulkEdit {
	raw, _ := json.Marshal(a)
	var edit metadata.BulkEdit
	json.Unmarshal(raw, &edit)
	return edit
}
func (d Dependencies) bulkCommandHooks() catalog.BulkCommandHooks {
	return catalog.BulkCommandHooks{
		Trash: func(ctx context.Context, p identity.Principal, operation, item string, auth func(context.Context, *sql.Tx) error, complete func(*sql.Tx) error) error {
			if d.Administration == nil {
				return errors.New("trash service unavailable")
			}
			e := d.Administration.TrashJobItem(ctx, auth, p.Authority+":"+p.AccountID, operation, item, complete)
			switch {
			case errors.Is(e, administration.ErrTrashRecovery):
				return e
			case errors.Is(e, administration.ErrDenied):
				return &catalog.JobItemError{Code: "trash_not_allowed"}
			case errors.Is(e, administration.ErrConflict):
				return &catalog.JobItemError{Code: "trash_conflict"}
			case errors.Is(e, administration.ErrNotFound):
				return &catalog.JobItemError{Code: "not_found"}
			}
			return e
		},
		Validate: func(command string, a catalog.JobArguments) error {
			switch command {
			case "trash":
				if d.Administration == nil {
					return catalog.ErrJobRequest
				}
				return nil
			case "refresh":
				if d.Metadata == nil {
					return catalog.ErrJobRequest
				}
				return nil
			case "metadata-edit":
				if d.Metadata == nil {
					return catalog.ErrJobRequest
				}
				if e := metadata.ValidateJobEdit(metadataJobEdit(a)); e != nil {
					return catalog.ErrJobRequest
				}
				return nil
			default:
				return catalog.ErrJobRequest
			}
		},
		Authorize: func(ctx context.Context, tx *sql.Tx, p identity.Principal, command string) error {
			return d.ownerAuthorityTx(ctx, tx, p)
		},
		Capture: func(ctx context.Context, tx *sql.Tx, p identity.Principal, command, item string, a catalog.JobArguments) (string, error) {
			if command != "metadata-edit" {
				return "", nil
			}
			if d.Metadata == nil {
				return "", errors.New("metadata service unavailable")
			}
			return d.Metadata.JobEditRevision(ctx, tx, item)
		},
		Apply: func(ctx context.Context, tx *sql.Tx, p identity.Principal, command, item, expected string, a catalog.JobArguments) error {
			if d.Metadata == nil {
				return errors.New("metadata service unavailable")
			}
			var e error
			switch command {
			case "refresh":
				e = d.Metadata.QueueRefreshTx(ctx, tx, item)
			case "metadata-edit":
				_, e = d.Metadata.ApplyBulkTargetTx(ctx, tx, metadata.RepairTarget{Kind: "item", ID: item}, expected, metadataJobEdit(a), metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, func(tx *sql.Tx) error { return d.ownerAuthorityTx(ctx, tx, p) })
			default:
				return errors.New("command worker unavailable")
			}
			switch {
			case errors.Is(e, metadata.ErrRepairConflict):
				return &catalog.JobItemError{Code: "metadata_conflict"}
			case errors.Is(e, metadata.ErrRepairInput):
				return &catalog.JobItemError{Code: "invalid_metadata_repair"}
			case errors.Is(e, metadata.ErrRefreshKind):
				return &catalog.JobItemError{Code: "unsupported_item_kind"}
			case errors.Is(e, sql.ErrNoRows):
				return &catalog.JobItemError{Code: "not_found"}
			}
			return e
		},
	}
}
