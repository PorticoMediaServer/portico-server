package remotesources

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"regexp"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

type DAVInput struct {
	OperationID        string `json:"operationId"`
	ExpectedGeneration int64  `json:"expectedGeneration,omitempty"`
	Name               string `json:"name"`
	Root               string `json:"root"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	KeepConnection     bool   `json:"keepConnection,omitempty"`
	KeepPassword       bool   `json:"keepPassword,omitempty"`
	InsecureLocal      bool   `json:"insecureLocal"`
}
type Receipt struct {
	OperationID string `json:"operationId"`
	Accepted    bool   `json:"accepted"`
	Removed     bool   `json:"removed"`
	Source      Source `json:"source"`
}

var opPattern = regexp.MustCompile(`^([0-9]{13})-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func checkOperation(op string) error {
	m := opPattern.FindStringSubmatch(op)
	if m == nil {
		return mounts.ErrInvalid
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	t := time.UnixMilli(n)
	if time.Since(t) > 10*time.Minute || time.Until(t) > time.Minute {
		return mounts.ErrCommandExpired
	}
	return nil
}
func (s *Service) receipt(ctx context.Context, actor, op, fingerprint string) (Receipt, error) {
	var out Receipt
	var a, h, raw string
	err := s.db.QueryRowContext(ctx, `SELECT actor,fingerprint,response FROM remote_source_operations WHERE operation_id=?`, op).Scan(&a, &h, &raw)
	if err != nil {
		return out, err
	}
	if a != actor || fingerprint != "" && !hmac.Equal([]byte(h), []byte(fingerprint)) {
		return out, mounts.ErrCommandConflict
	}
	err = json.Unmarshal([]byte(raw), &out)
	return out, err
}
func (s *Service) Receipt(ctx context.Context, actor, op string) (Receipt, error) {
	return s.receipt(ctx, actor, op, "")
}
func (s *Service) authorize(ctx context.Context, authorize func(*sql.Tx) error) error {
	if authorize == nil {
		return identity.ErrUnauthorized
	}
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	return authorize(tx)
}
func (s *Service) ConfigureDAV(ctx context.Context, actor, id string, in DAVInput, authorize func(*sql.Tx) error) (Receipt, error) {
	s.commands.Lock()
	defer s.commands.Unlock()
	var zero Receipt
	if err := s.authorize(ctx, authorize); err != nil {
		return zero, err
	}
	raw, _ := json.Marshal(struct {
		Actor, ID string
		Input     DAVInput
	}{actor, id, in})
	fingerprint := s.mounts.PrivateDigest(raw)
	clear(raw)
	if r, err := s.receipt(ctx, actor, in.OperationID, fingerprint); err == nil {
		return r, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return zero, err
	}
	if err := checkOperation(in.OperationID); err != nil {
		return zero, err
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 || len(in.Root) > 8192 || len(in.Username) > 1024 || len(in.Password) > 8192 {
		return zero, storage.ErrRemoteConfig
	}
	if id == "" && in.ExpectedGeneration != 0 || id != "" && in.ExpectedGeneration < 1 {
		return zero, mounts.ErrCommandConflict
	}
	source := Source{ID: id, Kind: "webdav", Name: strings.TrimSpace(in.Name), Generation: 1, State: "ready", RangeSupport: "unknown", InsecureLocal: in.InsecureLocal}
	if id != "" {
		old, err := s.get(ctx, id)
		if err != nil {
			return zero, err
		}
		if old.Kind != "webdav" || old.Generation != in.ExpectedGeneration {
			return zero, mounts.ErrCommandConflict
		}
		source.RootPath = old.RootPath
		source.Generation = old.Generation + 1
		if in.KeepPassword || in.KeepConnection {
			if in.KeepPassword && in.Password != "" {
				return zero, storage.ErrRemoteConfig
			}
			var sealed []byte
			if s.db.QueryRowContext(ctx, `SELECT config FROM remote_sources WHERE id=?`, id).Scan(&sealed) != nil {
				return zero, storage.ErrRemoteConfig
			}
			raw, err := s.mounts.Open(sealed)
			if err != nil {
				return zero, err
			}
			var oldConfig remotemedia.DAVConfig
			err = json.Unmarshal(raw, &oldConfig)
			clear(raw)
			if err != nil {
				return zero, storage.ErrRemoteConfig
			}
			if in.KeepConnection {
				if in.Root != "" || in.Username != "" {
					return zero, storage.ErrRemoteConfig
				}
				in.Root, in.Username = oldConfig.Root, oldConfig.Username
			}
			// Retained credentials cannot silently migrate to a different origin/user.
			if in.KeepPassword {
				if safeOrigin(oldConfig.Root) != safeOrigin(in.Root) || oldConfig.Username != in.Username {
					return zero, storage.ErrRemoteConfig
				}
				in.Password = oldConfig.Password
			}
		}
	} else {
		if in.KeepPassword || in.KeepConnection {
			return zero, storage.ErrRemoteConfig
		}
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM remote_sources WHERE kind='webdav' AND removed=0`).Scan(&count); err != nil {
			return zero, err
		}
		if count >= 8 {
			return zero, mounts.ErrCommandCapacity
		}
		source.ID = identity.Token()
		source.RootPath = filepath.Join(s.private, source.ID)
	}
	// Candidate network validation happens without a database write transaction.
	config, err := remotemedia.ValidateDAVConfig(ctx, remotemedia.DAVConfig{Root: in.Root, Username: in.Username, Password: in.Password, InsecureLocal: in.InsecureLocal}, s.resolver)
	if err != nil {
		return zero, mapError(err)
	}
	d, err := remotemedia.NewDAV(config, s.resolver)
	if err != nil {
		return zero, mapError(err)
	}
	rootEntry, err := d.Stat(ctx, "")
	if err != nil {
		return zero, mapError(err)
	}
	if !rootEntry.Directory {
		return zero, storage.ErrRemoteConfig
	}
	page, err := d.Sync(ctx, "", "")
	if errors.Is(err, remotemedia.ErrDAVUnsupported) || errors.Is(err, remotemedia.ErrDAVToken) {
		page, err = d.List(ctx, "")
	}
	if err != nil {
		return zero, mapError(err)
	}
	for _, e := range page.Entries {
		if !e.Deleted && !e.Directory && e.Size > 0 && source.RangeSupport == "unknown" {
			err = d.CheckRange(ctx, e.Relative)
			switch {
			case err == nil:
				source.RangeSupport = "conditional"
			case errors.Is(err, mediasource.ErrIdentityRequired), errors.Is(err, remotemedia.ErrRepresentationRange):
				source.RangeSupport = "unsupported"
			default:
				return zero, mapError(err)
			}
		}
	}
	source.Origin = safeOrigin(config.Root)
	source.CredentialPresent = config.Username != "" || config.Password != ""
	raw, _ = json.Marshal(config)
	sealed, err := s.mounts.Seal(raw)
	clear(raw)
	if err != nil {
		return zero, err
	}
	if id == "" {
		if err = os.Mkdir(source.RootPath, 0700); err != nil {
			return zero, err
		}
	}
	committed := false
	defer func() {
		if !committed && id == "" {
			_ = os.Remove(source.RootPath)
		}
	}()
	gated2, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return zero, err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if err = authorize(tx); err != nil {
		return zero, err
	}
	if id == "" {
		_, err = tx.Exec(`INSERT INTO remote_sources(id,kind,name,root,generation,config,state,range_support,origin,insecure,credential_present) VALUES(?,'webdav',?,?,1,?,'ready',?,?,?,?)`, source.ID, source.Name, source.RootPath, sealed, source.RangeSupport, source.Origin, source.InsecureLocal, source.CredentialPresent)
	} else {
		var result sql.Result
		result, err = tx.Exec(`UPDATE remote_sources SET name=?,generation=generation+1,config=?,state='ready',error_code='',range_support=?,origin=?,insecure=?,credential_present=? WHERE id=? AND generation=? AND removed=0`, source.Name, sealed, source.RangeSupport, source.Origin, source.InsecureLocal, source.CredentialPresent, id, in.ExpectedGeneration)
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				err = mounts.ErrCommandConflict
			}
		}
	}
	if err != nil {
		return zero, err
	}
	receipt := Receipt{OperationID: in.OperationID, Accepted: true, Source: source}
	response, _ := json.Marshal(receipt)
	if _, err = tx.Exec(`DELETE FROM remote_source_operations WHERE created_at<?`, time.Now().Add(-30*24*time.Hour).Unix()); err != nil {
		return zero, err
	}
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM remote_source_operations`).Scan(&count); err != nil {
		return zero, err
	}
	if count >= 4096 {
		return zero, mounts.ErrCommandCapacity
	}
	if _, err = tx.Exec(`INSERT INTO remote_source_operations VALUES(?,?,?,?,?)`, in.OperationID, actor, fingerprint, string(response), time.Now().Unix()); err != nil {
		return zero, err
	}
	if err = gated2.Commit(); err != nil {
		return zero, err
	}
	committed = true
	s.invalidate(source.ID)
	_ = s.refreshBindings(ctx)
	return receipt, nil
}
func (s *Service) RemoveDAV(ctx context.Context, actor, id, op string, generation int64, authorize func(*sql.Tx) error) (Receipt, error) {
	s.commands.Lock()
	defer s.commands.Unlock()
	var zero Receipt
	if err := s.authorize(ctx, authorize); err != nil {
		return zero, err
	}
	raw, _ := json.Marshal([]any{actor, id, op, generation, "remove"})
	hash := s.mounts.PrivateDigest(raw)
	if r, e := s.receipt(ctx, actor, op, hash); e == nil {
		return r, nil
	} else if !errors.Is(e, sql.ErrNoRows) {
		return zero, e
	}
	if err := checkOperation(op); err != nil {
		return zero, err
	}
	source, err := s.get(ctx, id)
	if err != nil {
		return zero, err
	}
	if source.Kind != "webdav" || source.Generation != generation {
		return zero, mounts.ErrCommandConflict
	}
	if s.PlaybackActive(id) {
		return zero, storage.ErrBusy
	}
	gated3, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return zero, err
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	if err = authorize(tx); err != nil {
		return zero, err
	}
	result, err := tx.Exec(`UPDATE remote_sources SET removed=1,state='removed',config=NULL,generation=generation+1 WHERE id=? AND generation=? AND removed=0`, id, generation)
	if err != nil {
		return zero, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return zero, mounts.ErrCommandConflict
	}
	// Root tombstones remain so an empty virtual folder can never become proof of
	// deletion. Explicit source removal marks only its associated bytes unavailable.
	// A prefix range on the path index (substr() read every asset): the paths
	// under root/ sort between root/ and root followed by the byte after /.
	rows, err := tx.Query(`SELECT id FROM catalog_assets WHERE path=? OR path>=? AND path<?`, source.RootPath, source.RootPath+string(filepath.Separator), source.RootPath+string(rune(filepath.Separator)+1))
	if err != nil {
		return zero, err
	}
	unavailable := []int64{}
	for rows.Next() {
		var asset int64
		if err = rows.Scan(&asset); err != nil {
			rows.Close()
			return zero, err
		}
		unavailable = append(unavailable, asset)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return zero, err
	}
	rows.Close()
	for _, asset := range unavailable {
		if err = compactcatalog.SetAssetAvailableTx(ctx, tx, asset, false); err != nil {
			return zero, err
		}
	}
	source.State = "removed"
	source.Generation++
	receipt := Receipt{OperationID: op, Accepted: true, Removed: true, Source: source}
	response, _ := json.Marshal(receipt)
	if _, err = tx.Exec(`DELETE FROM remote_source_operations WHERE created_at<?`, time.Now().Add(-30*24*time.Hour).Unix()); err != nil {
		return zero, err
	}
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM remote_source_operations`).Scan(&count); err != nil {
		return zero, err
	}
	if count >= 4096 {
		return zero, mounts.ErrCommandCapacity
	}
	if _, err = tx.Exec(`INSERT INTO remote_source_operations VALUES(?,?,?,?,?)`, op, actor, hash, string(response), time.Now().Unix()); err != nil {
		return zero, err
	}
	if err = gated3.Commit(); err != nil {
		return zero, err
	}
	s.invalidate(id)
	_ = s.refreshBindings(ctx)
	return receipt, nil
}
