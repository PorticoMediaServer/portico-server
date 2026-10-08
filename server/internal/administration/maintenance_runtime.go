package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/workpolicy"
)

func maintenanceRuntime(ctx context.Context, tx *sql.Tx, settings *MaintenanceSettingsDocument) error {
	p, err := workpolicy.ReadTx(ctx, tx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(p.Windows)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &settings.Windows); err != nil {
		return err
	}
	settings.BackgroundTaskPriority = p.BackgroundPriority
	return nil
}

// BackupKeepCount reads the retained backup count for the backup scheduler.
// It answers the shipped default when nothing usable is stored. Internal use:
// the value is server policy, and the scheduler cannot present owner auth.
func (s *Service) BackupKeepCount(ctx context.Context) int {
	fallback := DefaultMaintenanceSettings().BackupKeepCount
	if s == nil || s.db == nil {
		return fallback
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return fallback
	}
	defer snapshot.Rollback()
	settings, _, err := readDocument(ctx, snapshot.Tx(), maintenanceScope, DefaultMaintenanceSettings())
	if err != nil {
		return fallback
	}
	normalizeMaintenance(&settings)
	clampInt(&settings.BackupKeepCount, 1, 365)
	return settings.BackupKeepCount
}
func (s *Service) loadMaintenanceRuntime(ctx context.Context, auth Authorize) (MaintenanceDocument, error) {
	var out MaintenanceDocument
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		settings, revision, err := readDocument(ctx, tx, maintenanceScope, DefaultMaintenanceSettings())
		if err != nil {
			return err
		}
		normalizeMaintenance(&settings)
		if err = maintenanceRuntime(ctx, tx, &settings); err != nil {
			return err
		}
		out = maintenanceDocument(Document[MaintenanceSettingsDocument]{Revision: revision, Digest: digestOf(settings), Settings: settings})
		return nil
	})
	return out, err
}
func (s *Service) saveMaintenanceRuntime(ctx context.Context, auth Authorize, change Change[MaintenanceSettingsDocument]) (MaintenanceDocument, error) {
	var out MaintenanceDocument
	if !validOperationID(change.OperationID) {
		return out, ErrInput
	}
	value := change.Settings
	priorityOmitted := value.BackgroundTaskPriority == ""
	normalizeMaintenance(&value)
	if err := validateMaintenance(&value); err != nil {
		return out, err
	}
	digest := digestOf(change)
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		saved, replayed, err := receipt[MaintenanceDocument](ctx, tx, maintenanceScope, change.OperationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = saved
			return nil
		}
		_, revision, err := readDocument(ctx, tx, maintenanceScope, DefaultMaintenanceSettings())
		if err != nil {
			return err
		}
		if revision != change.ExpectedRevision {
			return ErrConflict
		}
		if priorityOmitted {
			current, readErr := workpolicy.ReadTx(ctx, tx)
			if readErr != nil {
				return readErr
			}
			value.BackgroundTaskPriority = current.BackgroundPriority
		}
		raw, _ := json.Marshal(value.Windows)
		p := workpolicy.Policy{BackgroundPriority: value.BackgroundTaskPriority}
		if err = json.Unmarshal(raw, &p.Windows); err != nil {
			return err
		}
		if err = workpolicy.SaveTx(ctx, tx, p); err != nil {
			return err
		}
		storedValue := value
		storedValue.Windows = nil
		storedValue.BackgroundTaskPriority = ""
		if err = writeDocument(ctx, tx, maintenanceScope, revision+1, storedValue, s.milliseconds()); err != nil {
			return err
		}
		out = maintenanceDocument(Document[MaintenanceSettingsDocument]{Revision: revision + 1, Digest: digestOf(value), Settings: value})
		return saveReceipt(ctx, tx, maintenanceScope, change.OperationID, digest, out, s.milliseconds())
	})
	return out, err
}
