package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"sync"
	"time"
)

const ClaimChannel = "portico_claim_web"
const claimWebSchema = `CREATE TABLE claim_web_pending(operation_id text PRIMARY KEY,code_hash bytea NOT NULL UNIQUE,request jsonb NOT NULL,request_hash bytea NOT NULL,account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','approved','denied')),envelope jsonb,expires_at timestamptz NOT NULL,requested_at timestamptz NOT NULL DEFAULT clock_timestamp());CREATE INDEX claim_web_pending_expiry ON claim_web_pending(expires_at);`

type claimWatchers struct {
	mu      sync.Mutex
	next    uint64
	clients map[uint64]chan struct{}
}

func (s *Store) signalClaims() {
	s.claimWatch.mu.Lock()
	defer s.claimWatch.mu.Unlock()
	for _, ch := range s.claimWatch.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
func (s *Store) RunClaimNotifications(ctx context.Context, notify <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-notify:
			s.signalClaims()
		}
	}
}
func (s *Store) watchClaim() (<-chan struct{}, func()) {
	s.claimWatch.mu.Lock()
	defer s.claimWatch.mu.Unlock()
	if s.claimWatch.clients == nil {
		s.claimWatch.clients = map[uint64]chan struct{}{}
	}
	s.claimWatch.next++
	id := s.claimWatch.next
	ch := make(chan struct{}, 1)
	s.claimWatch.clients[id] = ch
	return ch, func() { s.claimWatch.mu.Lock(); delete(s.claimWatch.clients, id); s.claimWatch.mu.Unlock() }
}

// claimCodeKey derives the claim-code HMAC key from the sealing key, so the
// AES-GCM key is never used for anything else (A56).
func claimCodeKey(seal []byte) []byte {
	mac := hmac.New(sha256.New, seal)
	mac.Write([]byte("portico.claim-code-key.v1"))
	return mac.Sum(nil)
}

// liveWebClaimsPerServer bounds pending web claims per server key (A48). The
// cap is on the requesting server, never on an account someone else names.
const liveWebClaimsPerServer = 3

func (s *Store) RegisterWebClaim(ctx context.Context, q claimweb.Request, key []byte) (claimweb.Pending, error) {
	raw, _ := json.Marshal(q)
	mac := hmac.New(sha256.New, claimCodeKey(key))
	mac.Write([]byte("portico.claim-code.v1:" + q.OperationID))
	code := "claim_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	hash := security.Hash(string(raw))
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return claimweb.Pending{}, e
	}
	defer tx.Rollback()
	// Serialize count-then-insert per server key.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('claim-web:'||$1,0))`, q.ServerID); e != nil {
		return claimweb.Pending{}, e
	}
	var live int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM claim_web_pending WHERE server_id=$1 AND expires_at>clock_timestamp() AND operation_id<>$2`, q.ServerID, q.OperationID).Scan(&live); e != nil {
		return claimweb.Pending{}, e
	}
	if live >= liveWebClaimsPerServer {
		return claimweb.Pending{}, claims.ErrCapacity
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO claim_web_pending(operation_id,code_hash,request,request_hash,account_id,server_id,expires_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7) ON CONFLICT(operation_id) DO NOTHING`, q.OperationID, security.Hash(code), raw, hash, q.AccountID, q.ServerID, q.ExpiresAt); e != nil {
		return claimweb.Pending{}, e
	}
	var saved []byte
	var at time.Time
	var status string
	if e = tx.QueryRowContext(ctx, `SELECT request_hash,requested_at,status FROM claim_web_pending WHERE operation_id=$1`, q.OperationID).Scan(&saved, &at, &status); e != nil {
		return claimweb.Pending{}, claims.ErrStale
	}
	if !equalSecret(saved, hash) {
		// The server re-sent this operation with a newer request (a retried Connect, or one
		// after the earlier request expired). The request is server-signed, so while nobody
		// has approved or denied it, the newest one replaces the old: otherwise a server
		// whose earlier attempt went unanswered could never connect again (demo, 24 Sep).
		if status != "pending" {
			return claimweb.Pending{}, claims.ErrStale
		}
		if e = tx.QueryRowContext(ctx, `UPDATE claim_web_pending SET request=$2,request_hash=$3,account_id=NULLIF($4,''),expires_at=$5,requested_at=clock_timestamp() WHERE operation_id=$1 AND status='pending' RETURNING requested_at`, q.OperationID, raw, hash, q.AccountID, q.ExpiresAt).Scan(&at); e != nil {
			return claimweb.Pending{}, claims.ErrStale
		}
	}
	if e = tx.Commit(); e != nil {
		return claimweb.Pending{}, e
	}
	return claimweb.Pending{Code: code, ServerID: q.ServerID, Name: q.Name, Address: q.Address, RequestedAt: at, ExpiresAt: q.ExpiresAt, Status: status}, nil
}
func (s *Store) LookupWebClaim(ctx context.Context, a identity.CurrentFamilyExpectation, code string) (claimweb.Pending, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return claimweb.Pending{}, e
	}
	defer tx.Rollback()
	if _, e = AuthorizeCurrentFamilyTx(ctx, tx, a); e != nil {
		return claimweb.Pending{}, e
	}
	var raw []byte
	var p claimweb.Pending
	e = tx.QueryRowContext(ctx, `SELECT request,requested_at,expires_at,status FROM claim_web_pending WHERE code_hash=$1 AND (account_id IS NULL OR account_id=$2) AND expires_at>clock_timestamp()`, security.Hash(code), a.AccountID).Scan(&raw, &p.RequestedAt, &p.ExpiresAt, &p.Status)
	if e != nil {
		return p, claims.ErrStale
	}
	var q claimweb.Request
	if json.Unmarshal(raw, &q) != nil {
		return p, claims.ErrInvalid
	}
	p.Code = code
	p.ServerID = q.ServerID
	p.Name = q.Name
	p.Address = q.Address
	return p, tx.Commit()
}
func (s *Store) DecideWebClaim(ctx context.Context, a identity.CurrentFamilyExpectation, code, profile, replaces string, approve bool, service *claims.Service) error {
	// Claim approval's existing account/family checks remain the positive authority.
	// Denial and approval serialize on the account lock, with a durable decision.
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = AuthorizeCurrentFamilyTx(ctx, tx, a); e != nil {
		return e
	}
	var raw []byte
	var status, op string
	e = tx.QueryRowContext(ctx, `SELECT request,status,operation_id FROM claim_web_pending WHERE code_hash=$1 AND (account_id IS NULL OR account_id=$2) AND expires_at>clock_timestamp() FOR UPDATE`, security.Hash(code), a.AccountID).Scan(&raw, &status, &op)
	if e != nil {
		return claims.ErrStale
	}
	want := "denied"
	if approve {
		want = "approved"
	}
	if status != "pending" {
		if status == want {
			return nil
		}
		return claims.ErrStale
	}
	var q claimweb.Request
	if json.Unmarshal(raw, &q) != nil {
		return claims.ErrInvalid
	}
	var envelope []byte
	if approve && profile == "" {
		// The web page names an owner profile only when the account has several
		// (and Hosted keeps no server profiles since server-owned membership):
		// otherwise it is the account's primary profile, or its first.
		if e = tx.QueryRowContext(ctx, `SELECT id FROM profiles WHERE account_id=$1 AND deleted_at IS NULL ORDER BY is_primary DESC,position,created_at,id LIMIT 1`, a.AccountID).Scan(&profile); e != nil {
			return currentFamilyError(ctx, e)
		}
	}
	if approve {
		// Execute the same approval consumer in this transaction; no decision gap.
		input := claims.ApprovalInput{OperationID: q.OperationID, ServerID: q.ServerID, PublicKey: q.PublicKey, LocalGeneration: q.LocalGeneration, Name: q.Name, OwnerProfileID: profile, ReplacesServerID: replaces}
		approval, e := s.approveClaimTx(ctx, tx, a, input, q.Audience)
		if e != nil {
			return e
		}
		signed, e := service.SignWebApproval(approval)
		if e != nil {
			return e
		}
		envelope, e = json.Marshal(signed)
		if e != nil {
			return e
		}
	}
	// The person holding the code binds their account when approving (Plex-style).
	if _, e = tx.ExecContext(ctx, `UPDATE claim_web_pending SET status=$2,envelope=$3,account_id=CASE WHEN $2='approved' THEN COALESCE(account_id,$4) ELSE account_id END WHERE operation_id=$1`, op, want, envelope, a.AccountID); e != nil {
		return e
	}
	if e = notifyTx(ctx, tx, ClaimChannel); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	s.signalClaims()
	return nil
}
func (s *Store) WaitWebClaim(ctx context.Context, q claimweb.Request) (json.RawMessage, error) {
	ch, stop := s.watchClaim()
	defer stop()
	raw, _ := json.Marshal(q)
	hash := security.Hash(string(raw))
	for {
		var status string
		var envelope []byte
		var saved []byte
		e := s.DB.QueryRowContext(ctx, `SELECT status,envelope,request_hash FROM claim_web_pending WHERE operation_id=$1 AND expires_at>clock_timestamp()`, q.OperationID).Scan(&status, &envelope, &saved)
		if errors.Is(e, sql.ErrNoRows) || !equalSecret(hash, saved) {
			return nil, claims.ErrStale
		}
		if e != nil {
			return nil, e
		}
		if status == "denied" {
			return nil, claims.ErrCancelled
		}
		if status == "approved" {
			// The envelope is kept as jsonb, which reorders keys and adds
			// spaces. The server accepts only the canonical encoding (its
			// decoder compares the bytes with its own re-encoding), so send it
			// re-encoded; the signature covers the payload, not this wrapper.
			var signed claims.Envelope
			if e = json.Unmarshal(envelope, &signed); e != nil {
				return nil, claims.ErrInvalid
			}
			return json.Marshal(signed)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}
