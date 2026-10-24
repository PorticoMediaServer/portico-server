package remotesources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

// SourceReadValues is the existing normalized read-profile subset of the
// source's Reading settings. It does not stand in for scheduling or caching.
type SourceReadValues struct {
	ScanDepth         string `json:"scanDepth"`
	WholeFileReads    bool   `json:"wholeFileReads"`
	Connections       int    `json:"connections"`
	RequestsPerSecond int    `json:"requestsPerSecond"`
	BytesPerSecond    int64  `json:"bytesPerSecond"`
}

// A complete settings value is required, particularly for false and zero:
// omitted/null fields must not silently switch off a restriction.
func (v *SourceReadValues) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 5 {
		return storage.ErrRemoteConfig
	}
	for _, name := range []string{"scanDepth", "wholeFileReads", "connections", "requestsPerSecond", "bytesPerSecond"} {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return storage.ErrRemoteConfig
		}
	}
	type plain SourceReadValues
	var decoded plain
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return storage.ErrRemoteConfig
	}
	*v = SourceReadValues(decoded)
	return nil
}

type SourceReadSettings struct {
	SourceID   string           `json:"sourceId"`
	Generation string           `json:"generation"`
	Revision   int64            `json:"revision"`
	Saved      bool             `json:"saved"`
	Values     SourceReadValues `json:"values"`
}

type SourceReadSettingsChange struct {
	OperationID        string           `json:"operationId"`
	ExpectedGeneration string           `json:"expectedGeneration"`
	ExpectedRevision   int64            `json:"expectedRevision"`
	Values             SourceReadValues `json:"values"`
}

func (v *SourceReadSettingsChange) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return storage.ErrRemoteConfig
	}
	for _, name := range []string{"operationId", "expectedGeneration", "expectedRevision", "values"} {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return storage.ErrRemoteConfig
		}
	}
	type plain SourceReadSettingsChange
	var decoded plain
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return storage.ErrRemoteConfig
	}
	*v = SourceReadSettingsChange(decoded)
	return nil
}

func sourceReadValues(p SourceReadProfile) SourceReadValues {
	return SourceReadValues{ScanDepth: p.ScanDepth, WholeFileReads: p.WholeFileReads, Connections: p.Limits.Connections, RequestsPerSecond: p.Limits.RequestsPerSecond, BytesPerSecond: p.Limits.BytesPerSecond}
}

func (v SourceReadValues) profile(source string) SourceReadProfile {
	return SourceReadProfile{SourceID: source, ScanDepth: v.ScanDepth, WholeFileReads: v.WholeFileReads, Limits: ReadLimits{Connections: v.Connections, RequestsPerSecond: v.RequestsPerSecond, BytesPerSecond: v.BytesPerSecond}}
}

func readSettingsTx(ctx context.Context, tx *sql.Tx, source string) (Source, SourceReadSettings, error) {
	var row Source
	err := tx.QueryRowContext(ctx, `SELECT id,kind,name,root,generation,state,error_code,range_support,origin,insecure,credential_present,mount_id FROM remote_sources WHERE id=? AND removed=0`, source).Scan(&row.ID, &row.Kind, &row.Name, &row.RootPath, &row.Generation, &row.State, &row.ErrorCode, &row.RangeSupport, &row.Origin, &row.InsecureLocal, &row.CredentialPresent, &row.MountID)
	if err != nil {
		return row, SourceReadSettings{}, err
	}
	profile, err := SourceReadProfileTx(ctx, tx, source)
	return row, SourceReadSettings{SourceID: source, Generation: strconv.FormatInt(row.Generation, 10), Revision: profile.Revision, Saved: profile.Saved, Values: sourceReadValues(profile)}, err
}

func (s *Service) ReadSourceReadSettings(ctx context.Context, identities *identity.Service, owner identity.Principal, source string) (SourceReadSettings, error) {
	if s == nil || s.db == nil || source == "" {
		return SourceReadSettings{}, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return SourceReadSettings{}, err
	}
	defer snapshot.Rollback()
	if err = currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner); err != nil {
		return SourceReadSettings{}, err
	}
	_, out, err := readSettingsTx(ctx, snapshot.Tx(), source)
	return out, err
}

// SaveSourceReadSettings is the owner command boundary around the sole profile
// writer. The existing source-operation receipt and profile/library revisions
// commit together. Its post-commit notification never changes source identity.
func (s *Service) SaveSourceReadSettings(ctx context.Context, identities *identity.Service, owner identity.Principal, source string, in SourceReadSettingsChange) (Receipt, error) {
	if s == nil || s.db == nil || s.mounts == nil || source == "" || in.ExpectedGeneration == "" || in.ExpectedRevision < 0 || !validSourceReadProfile(in.Values.profile(source)) {
		return Receipt{}, storage.ErrRemoteConfig
	}
	s.commands.Lock()
	defer s.commands.Unlock()
	authorize := func(tx *sql.Tx) error { return currentLocalOwnerTx(ctx, tx, identities, owner) }
	if err := s.authorize(ctx, authorize); err != nil {
		return Receipt{}, err
	}
	actorBytes, _ := json.Marshal([]string{owner.Authority, owner.AccountID, owner.ProfileID})
	actor := string(actorBytes)
	facts, _ := json.Marshal(struct {
		Kind, Actor, Source string
		Change              SourceReadSettingsChange
	}{"read_settings", actor, source, in})
	fingerprint := s.mounts.PrivateDigest(facts)
	clear(facts)
	if receipt, err := s.receipt(ctx, actor, in.OperationID, fingerprint); err == nil {
		if receipt.ReadSettings == nil {
			return Receipt{}, storage.ErrRemoteConfig
		}
		return receipt, s.finishReadSettings(ctx, source, receipt.ReadSettings.Revision)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Receipt{}, err
	}
	if err := checkOperation(in.OperationID); err != nil {
		return Receipt{}, err
	}
	writer, err := dbwork.Begin(ctx, s.db, dbwork.ClassInteractive)
	if err != nil {
		return Receipt{}, err
	}
	defer writer.Rollback()
	_, err = s.ApplySourceReadProfileTx(ctx, writer.Tx(), identities, owner, SourceReadProfileChange{SourceID: source, Generation: in.ExpectedGeneration, ExpectedRevision: in.ExpectedRevision, Profile: in.Values.profile(source)})
	if err != nil {
		return Receipt{}, err
	}
	row, settings, err := readSettingsTx(ctx, writer.Tx(), source)
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{OperationID: in.OperationID, Accepted: true, Source: row, ReadSettings: &settings}
	response, err := json.Marshal(receipt)
	if err != nil {
		return Receipt{}, err
	}
	if _, err = writer.Tx().ExecContext(ctx, `INSERT INTO remote_source_operations(operation_id,actor,fingerprint,response,created_at) VALUES(?,?,?,?,?)`, in.OperationID, actor, fingerprint, string(response), time.Now().Unix()); err != nil {
		return Receipt{}, err
	}
	if err = writer.Commit(); err != nil {
		return Receipt{}, err
	}
	return receipt, s.finishReadSettings(ctx, source, settings.Revision)
}

// A committed command must retire obsolete readers even when its HTTP request
// has ended. This bounded cleanup does only DB reads and cancellation: it never
// opens a source or grants authority. Receipt replay retries it without a write.
// Read the current profile, so an old receipt never retires its replacement.
func (s *Service) finishReadSettings(ctx context.Context, source string, committedRevision int64) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	snapshot, err := dbwork.BeginSnapshot(cleanup, s.db)
	if err != nil {
		return err
	}
	profile, err := SourceReadProfileTx(cleanup, snapshot.Tx(), source)
	snapshot.Rollback()
	if err != nil {
		return err
	}
	if !profile.Saved || profile.Revision < committedRevision {
		return storage.ErrRemoteChanged
	}
	s.retireReadProfile(source, profile)
	return nil
}
