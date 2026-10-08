package networking

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
)

// ProtectedKeys is the current immutable key registry. It never imports a key
// from configuration, replaces a missing key, or deletes predecessor keys.
type ProtectedKeys struct {
	db   *sql.DB
	root *os.Root
}

func (k *ProtectedKeys) Close() error { return k.root.Close() }
func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func privateDirectory(path string) error {
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(path)
	if e != nil || !info.IsDir() {
		return ErrUnavailable
	}
	return nil
}
func (k *ProtectedKeys) read(incarnation string) (ed25519.PrivateKey, error) {
	if !validID(incarnation) {
		return nil, ErrInvalid
	}
	// Every refusal below is ErrUnavailable to callers. The stage and the
	// system error travel beside it so a handler can log which one it was; no
	// path, name or content does.
	name := incarnation + ".ed25519"
	info, e := k.root.Lstat(name)
	switch {
	case e != nil:
		return nil, keyFileFailure("lstat", e)
	case !info.Mode().IsRegular():
		return nil, keyFileFailure("type", nil)
	case info.Size() != ed25519.PrivateKeySize:
		return nil, keyFileFailure("size", nil)
	}
	f, e := k.root.Open(name)
	if e != nil {
		return nil, keyFileFailure("open", e)
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil {
		return nil, keyFileFailure("stat", e)
	}
	if !os.SameFile(info, actual) {
		return nil, keyFileFailure("changed", nil)
	}
	raw, e := io.ReadAll(io.LimitReader(f, 65))
	if e != nil || len(raw) != 64 {
		clear(raw)
		return nil, keyFileFailure("read", e)
	}
	key := ed25519.PrivateKey(raw)
	derived := ed25519.NewKeyFromSeed(key.Seed())
	defer clear(derived)
	if !bytes.Equal(derived, key) {
		clear(raw)
		return nil, keyFileFailure("content", nil)
	}
	return key, nil
}

// OpenCurrentKeys initializes one canonical application/server identity before
// identity.New or Setup. Existing unmarked state is refused, never adopted.
func OpenCurrentKeys(ctx context.Context, db *sql.DB, state string, runner *AuthorityRunner) (*ProtectedKeys, error) {
	if db == nil || runner == nil {
		return nil, ErrInvalid
	}
	directory := filepath.Join(state, "networking-keys")
	if e := privateDirectory(directory); e != nil {
		return nil, e
	}
	root, e := os.OpenRoot(directory)
	if e != nil {
		return nil, e
	}
	keys := &ProtectedKeys{db, root}
	e = runner.Do(ctx, func(ctx context.Context) error {
		gated, e := dbwork.Begin(ctx, db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
		if e != nil {
			return e
		}
		tx := gated.Tx()
		defer gated.Rollback()
		var applicationID string
		e = tx.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key='id'`).Scan(&applicationID)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if applicationID != "" {
			snapshot, e := CurrentIdentityTx(ctx, tx)
			if e != nil || snapshot.ServerID != applicationID {
				return ErrStale
			}
			key, e := keys.read(snapshot.KeyIncarnation)
			if e != nil {
				return e
			}
			defer clear(key)
			if !bytes.Equal(key.Public().(ed25519.PublicKey), snapshot.PublicKey) {
				return ErrStale
			}
			return nil
		}
		var count int
		if e = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM accounts)+(SELECT count(*) FROM networking_server_identities)+(SELECT count(*) FROM configuration WHERE key IN('hosted_token','server_private_key'))`).Scan(&count); e != nil || count != 0 {
			return ErrStale
		}
		entries, e := os.ReadDir(directory)
		if e != nil || len(entries) != 0 {
			return ErrUnavailable
		}
		public, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		defer clear(key)
		random := make([]byte, 32)
		if _, e = rand.Read(random); e != nil {
			return e
		}
		incarnation := hex.EncodeToString(random)
		f, e := root.OpenFile(incarnation+".ed25519", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(key)
		if e == nil {
			e = f.Sync()
		}
		closed := f.Close()
		if e != nil {
			return e
		}
		if closed != nil {
			return closed
		}
		if e = syncDirectory(directory); e != nil {
			return e
		}
		if e = syncDirectory(state); e != nil {
			return e
		}
		reopened, e := keys.read(incarnation)
		if e != nil {
			return e
		}
		defer clear(reopened)
		if !bytes.Equal(reopened, key) {
			return ErrUnavailable
		}
		server, e := ServerIdentity(public)
		if e != nil {
			return e
		}
		if _, e = StageIdentityTx(ctx, tx, nil, DurableIdentity{server, public, incarnation}); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES('id',?)`, server); e != nil {
			return e
		}
		return commitClaimTx(ctx, tx.Commit)
	})
	if e != nil {
		root.Close()
		return nil, e
	}
	return keys, nil
}
func (k *ProtectedKeys) Sign(ctx context.Context, b Binding, payload []byte) ([]byte, error) {
	if _, e := claimAuthority(ctx); e != nil {
		return nil, e
	}
	if !validBinding(b) || len(payload) < 1 || len(payload) > 16<<10 {
		return nil, ErrInvalid
	}
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(payload, &kind) != nil {
		return nil, ErrInvalid
	}
	switch kind.Kind {
	case "portico.claim.nonce-request", "portico.claim.proof", "portico.claim.cancel":
	case "portico.claim.web.v1":
		var q struct {
			OperationID     string `json:"operationId"`
			ServerID        string `json:"serverId"`
			AccountID       string `json:"accountId"`
			PublicKey       string `json:"publicKey"`
			LocalGeneration int64  `json:"localGeneration,string"`
		}
		// A web claim prepared without an account carries accountId "" (the
		// approver becomes the owner); its binding holds UnboundAccount.
		account := b.AccountID
		if account == UnboundAccount {
			account = ""
		}
		if json.Unmarshal(payload, &q) != nil || q.OperationID != b.OperationID || q.ServerID != b.ServerID || q.AccountID != account || q.PublicKey != base64.RawURLEncoding.EncodeToString(b.PublicKey) || q.LocalGeneration != b.LocalGeneration {
			return nil, ErrInvalid
		}
	case "portico.server.endpoint.v1":
		if !validateEndpointSigningPayload(payload, b) {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	gated2, e := dbwork.BeginSnapshot(ctx, k.db)
	if e != nil {
		return nil, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if e = guardClaimRequestTx(ctx, tx); e != nil {
		return nil, e
	}
	id, active, _, e := readCurrentIdentityTx(ctx, tx)
	if e != nil {
		return nil, e
	}
	if id.ServerID != b.ServerID || !bytes.Equal(id.PublicKey, b.PublicKey) || id.ResetGeneration != b.LocalGeneration || !active.Valid || active.String != b.OperationID {
		return nil, ErrStale
	}
	if kind.Kind == "portico.server.endpoint.v1" {
		var proof endpointChallenge
		if json.Unmarshal(payload, &proof) != nil {
			return nil, ErrInvalid
		}
		current, e := loadIntentTx(ctx, tx, b.OperationID)
		if e != nil {
			return nil, e
		}
		if !current.InstallationAcknowledged || proof.ClaimGeneration != current.ClaimGeneration || proof.CredentialGeneration != current.CredentialGeneration {
			return nil, ErrStale
		}
		if e = GuardInstalledClaim(ctx, tx, current.Intent); e != nil {
			return nil, e
		}
	}
	key, e := k.read(id.KeyIncarnation)
	if e != nil {
		return nil, e
	}
	defer clear(key)
	if !bytes.Equal(key.Public().(ed25519.PublicKey), b.PublicKey) {
		return nil, ErrStale
	}
	if _, e = claimAuthority(ctx); e != nil {
		return nil, e
	}
	if e = guardClaimRequestTx(ctx, tx); e != nil {
		return nil, e
	}
	return ed25519.Sign(key, payload), nil
}
