package access

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"portico.local/server/internal/identity"
)

// KeyScopes are the published API key scopes, narrowest first.
//
//	read-only          — read catalog, browse, detail and personal state.
//	playback           — read-only plus opening and controlling playback.
//	library-management — playback plus library, scan and metadata administration.
//	full               — everything the key's account may do, administration
//	                     included. A full key never exceeds its account's tier.
var KeyScopes = []string{"read-only", "playback", "library-management", "full"}

func scopeRank(v string) int {
	for i, s := range KeyScopes {
		if s == v {
			return i + 1
		}
	}
	return 0
}

// ScopeAllows reports whether a key scope permits an operation class. Operation
// classes are the same names as the scopes: a read is "read-only", opening
// playback is "playback", a library or metadata write is "library-management",
// and anything administrative is "full".
func ScopeAllows(scope, needed string) bool {
	return scopeRank(scope) > 0 && scopeRank(scope) >= scopeRank(needed)
}

type APIKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	AccountID string `json:"accountId"`
	Hint      string `json:"hint"`
	CreatedAt string `json:"createdAt"`
	LastUsed  string `json:"lastUsedAt,omitempty"`
	Revoked   bool   `json:"revoked"`
	Revision  int64  `json:"revision"`
	// Secret is present only on the response that creates the key.
	Secret string `json:"secret,omitempty"`
}

type APIKeyPage struct {
	Page
	Items []APIKey `json:"items"`
}

// KeyPrefix marks a bearer token as an API key rather than a session token, so
// authentication can route it to this package without a database probe.
const KeyPrefix = "pk_"

func keyRow(row interface{ Scan(...any) error }) (APIKey, error) {
	var k APIKey
	var created int64
	var used sql.NullInt64
	if e := row.Scan(&k.ID, &k.Name, &k.Scope, &k.AccountID, &k.Hint, &created, &used, &k.Revoked, &k.Revision); e != nil {
		return k, e
	}
	k.CreatedAt = time.UnixMilli(created).UTC().Format(time.RFC3339)
	if used.Valid {
		k.LastUsed = time.UnixMilli(used.Int64).UTC().Format(time.RFC3339)
	}
	return k, nil
}

const keyColumns = `id,name,scope,account_id,hint,created_ms,last_used_ms,revoked,revision`

type APIKeyRequest struct {
	OperationID string `json:"operationId"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
}

// CreateAPIKey issues a key bound to the calling account. The secret is returned
// once; only its digest is stored, so the server cannot reveal it again.
func (s *Store) CreateAPIKey(ctx context.Context, auth Authorize, actor identity.Principal, q APIKeyRequest) (out APIKey, err error) {
	if !validOperationID(q.OperationID) {
		return out, invalid("operationId")
	}
	name := strings.TrimSpace(q.Name)
	if name == "" || !safeText(name, 80) {
		return out, invalid("name")
	}
	if scopeRank(q.Scope) == 0 {
		return out, invalid("scope")
	}
	secret := KeyPrefix + token()
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "api-key:" + actor.AccountID
		body, want, e := receipt(tx, scope, q.OperationID, q)
		if e != nil {
			return e
		}
		if body != "" {
			return json.Unmarshal([]byte(body), &out)
		}
		var count int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM access_api_keys WHERE revoked=0`).Scan(&count); e != nil {
			return e
		}
		if count >= 100 {
			return ErrCapacity
		}
		now, id := s.now(), token()[:22]
		// The hint is the tail of the secret. It identifies a key in a list
		// without being enough to use one.
		hint := secret[len(secret)-6:]
		if _, e = tx.ExecContext(ctx, `INSERT INTO access_api_keys(id,account_id,profile_id,name,scope,secret_hash,hint,created_ms,created_by) VALUES(?,?,?,?,?,?,?,?,?)`,
			id, actor.AccountID, actor.ProfileID, name, q.Scope, digest(secret), hint, now, actor.AccountID); e != nil {
			return e
		}
		if out, e = keyRow(tx.QueryRowContext(ctx, `SELECT `+keyColumns+` FROM access_api_keys WHERE id=?`, id)); e != nil {
			return e
		}
		if e = saveReceipt(tx, scope, q.OperationID, want, out, now); e != nil {
			return e
		}
		out.Secret = secret
		return nil
	})
	return
}

type APIKeyQuery struct {
	Cursor string
	Limit  string
}

func (s *Store) APIKeys(ctx context.Context, auth Authorize, q APIKeyQuery) (out APIKeyPage, err error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	created, after, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	out.Limit, out.Items = limit, []APIKey{}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT `+keyColumns+` FROM access_api_keys WHERE (?=0 OR (created_ms,id)<(?,?)) ORDER BY created_ms DESC,id DESC LIMIT ?`, created, created, after, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			k, e := keyRow(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, k)
		}
		return rows.Err()
	})
	if err == nil && len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		t, _ := time.Parse(time.RFC3339, last.CreatedAt)
		out.NextCursor = encodeCursor(t.UnixMilli(), last.ID)
	}
	return
}

type APIKeyRevoke struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}

func (s *Store) RevokeAPIKey(ctx context.Context, auth Authorize, id string, c APIKeyRevoke) (out APIKey, err error) {
	if !validID(id) {
		return out, invalid("id")
	}
	if !validOperationID(c.OperationID) {
		return out, invalid("operationId")
	}
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "api-key-revoke:" + id
		body, want, e := receipt(tx, scope, c.OperationID, c)
		if e != nil {
			return e
		}
		if body != "" {
			return json.Unmarshal([]byte(body), &out)
		}
		var revision int64
		if e = tx.QueryRowContext(ctx, `SELECT revision FROM access_api_keys WHERE id=?`, id).Scan(&revision); e != nil {
			return e
		}
		if revision != c.ExpectedRevision {
			return &ConflictError{revision}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE access_api_keys SET revoked=1,revision=revision+1 WHERE id=?`, id); e != nil {
			return e
		}
		if out, e = keyRow(tx.QueryRowContext(ctx, `SELECT `+keyColumns+` FROM access_api_keys WHERE id=?`, id)); e != nil {
			return e
		}
		return saveReceipt(tx, scope, c.OperationID, want, out, s.now())
	})
	return
}

// KeyPrincipal is an authenticated API key. Authority is "api-key" rather than
// "local": a key is a distinct principal type, so every authorization helper
// that requires a live session family rejects it unless it opted in.
type KeyPrincipal struct {
	identity.Principal
	KeyID string
	Scope string
}

// AuthenticateKey resolves a bearer secret that carries KeyPrefix. It returns
// identity.ErrUnauthorized for an unknown, revoked or disabled key, and records
// the sighting as the key's last use.
func (s *Store) AuthenticateKey(ctx context.Context, secret string) (KeyPrincipal, error) {
	var out KeyPrincipal
	if !strings.HasPrefix(secret, KeyPrefix) || len(secret) < 20 || len(secret) > 256 {
		return out, identity.ErrUnauthorized
	}
	err := s.transaction(ctx, nil, func(tx *sql.Tx) error {
		var id, account, profile, scope string
		var revoked bool
		e := tx.QueryRowContext(ctx, `SELECT k.id,k.account_id,k.profile_id,k.scope,k.revoked FROM access_api_keys k WHERE k.secret_hash=?`, digest(secret)).Scan(&id, &account, &profile, &scope, &revoked)
		if errors.Is(e, sql.ErrNoRows) {
			return identity.ErrUnauthorized
		}
		if e != nil {
			return e
		}
		if revoked {
			return identity.ErrUnauthorized
		}
		role, e := Role(tx, account)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE access_api_keys SET last_used_ms=? WHERE id=?`, s.now(), id); e != nil {
			return e
		}
		out = KeyPrincipal{Principal: identity.Principal{Viewer: identity.Viewer{AccountID: account, ProfileID: profile, Authority: "api-key", Role: role}}, KeyID: id, Scope: scope}
		return nil
	})
	return out, err
}
