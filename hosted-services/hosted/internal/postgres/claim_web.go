package postgres

import (
	"bytes"
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
	"portico.local/hosted/internal/serverreturn"
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
	// An unresolved Local switch must bind its full signed owner start atomically.
	if q.Purpose == "switch" && q.ReturnRegistrationID == "" {
		return claimweb.Pending{}, claims.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return claimweb.Pending{}, e
	}
	defer tx.Rollback()
	// A captured setup cannot be replayed through the ordinary pending path.
	// Serialize with its registration before checking its immutable journal.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('claim-web:'||$1,0))`, q.ServerID); e != nil {
		return claimweb.Pending{}, e
	}
	var setup bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM setup_claim_starts WHERE operation_id=$1)`, q.OperationID).Scan(&setup); e != nil {
		return claimweb.Pending{}, e
	}
	if setup {
		return claimweb.Pending{}, claims.ErrInvalid
	}
	out, e := registerWebClaimTx(ctx, tx, q, key)
	if e != nil {
		return claimweb.Pending{}, e
	}
	return out, tx.Commit()
}

func registerWebClaimTx(ctx context.Context, tx *sql.Tx, q claimweb.Request, key []byte) (claimweb.Pending, error) {
	raw, _ := json.Marshal(q)
	mac := hmac.New(sha256.New, claimCodeKey(key))
	mac.Write([]byte("portico.claim-code.v1:" + q.OperationID))
	code := "claim_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	hash := security.Hash(string(raw))
	var e error
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
	var saved, previousRequest []byte
	var at time.Time
	var status string
	if e = tx.QueryRowContext(ctx, `SELECT request_hash,request,requested_at,status FROM claim_web_pending WHERE operation_id=$1`, q.OperationID).Scan(&saved, &previousRequest, &at, &status); e != nil {
		return claimweb.Pending{}, claims.ErrStale
	}
	if !equalSecret(saved, hash) {
		var frozen bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM owner_claim_starts WHERE operation_id=$1) OR EXISTS(SELECT 1 FROM setup_claim_starts WHERE operation_id=$1)`, q.OperationID).Scan(&frozen); e != nil {
			return claimweb.Pending{}, e
		}
		if frozen {
			return claimweb.Pending{}, claims.ErrStale
		}
		// Refresh may extend only time bounds. A stable approval code must never
		// change the server/name/address/account or the purpose/ownership intent
		// the authenticated approver has just reviewed.
		var previous claimweb.Request
		if json.Unmarshal(previousRequest, &previous) != nil || !q.IssuedAt.After(previous.IssuedAt) {
			return claimweb.Pending{}, claims.ErrStale
		}
		previous.IssuedAt = q.IssuedAt
		previous.ExpiresAt = q.ExpiresAt
		canonical, _ := json.Marshal(previous)
		if !bytes.Equal(canonical, raw) {
			return claimweb.Pending{}, claims.ErrStale
		}

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
	return claimweb.Pending{Purpose: q.Purpose, PreviousOwnerRole: q.PreviousOwnerRole, Code: code, ServerID: q.ServerID, Name: q.Name, Address: q.Address, RequestedAt: at, ExpiresAt: q.ExpiresAt, Status: status}, nil
}
func (s *Store) LookupWebClaim(ctx context.Context, a identity.CurrentFamilyExpectation, code string) (claimweb.LookupPending, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return claimweb.LookupPending{}, e
	}
	defer tx.Rollback()
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return claimweb.LookupPending{}, e
	}
	var raw []byte
	var p claimweb.LookupPending
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
	p.Purpose = q.Purpose
	p.PreviousOwnerRole = q.PreviousOwnerRole
	setup, e := readSetupStart(ctx, tx, q.OperationID, true, false)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return p, e
	}
	if e == nil {
		if q.Purpose != "setup" || !setup.liveAt(time.Now()) || (setup.a.AccountID != "" && !sameSetupFamily(setup.a, a)) {
			return p, claims.ErrStale
		}
		p.SetupTransactionID, p.ExpiresAt = setup.id, setup.expiry
		if setup.registration != "" {
			t, err := setupTupleTx(ctx, tx, setup)
			if err != nil || !sameReturnTuple(t, setup.t) {
				return p, claims.ErrStale
			}
			p.ReturnRegistration = &serverreturn.Registration{ID: setup.registration, Origin: setup.t.origin}
		}
		var live bool
		if e = tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() AND phase IN('prepared','approved','installed','completed') FROM setup_claim_starts WHERE transaction_id=$1`, setup.id).Scan(&live); e != nil {
			return p, e
		}
		if !live {
			return p, claims.ErrStale
		}
		if e = returnFresh(ctx, tx, facts); e != nil {
			return p, e
		}
		return p, tx.Commit()
	}
	owner, e := readOwnerStart(ctx, tx, q.OperationID, true, false)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return p, e
	}
	if e == nil {
		if !owner.liveAt(time.Now()) || (owner.a.AccountID != "" && !sameOwnerFamily(owner.a, a)) {
			return p, claims.ErrStale
		}
		p.OwnerTransactionID, p.ExpiresAt = owner.id, owner.expiry
		// Project only the mode frozen in this claim's own signed start.
		var startPayload []byte
		if e = tx.QueryRowContext(ctx, `SELECT start_payload FROM owner_claim_starts WHERE operation_id=$1`, q.OperationID).Scan(&startPayload); e != nil {
			return p, e
		}
		var start claimweb.OwnerStart
		if json.Unmarshal(startPayload, &start) != nil {
			return p, claims.ErrInvalid
		}
		p.TransitionMode = start.TransitionMode
		if owner.registration != "" {
			t, e := ownerTupleTx(ctx, tx, owner)
			if e != nil || !sameReturnTuple(t, owner.t) {
				return p, claims.ErrStale
			}
			p.ReturnRegistration = &serverreturn.Registration{ID: owner.registration, Origin: owner.t.origin}
		}
		var live bool
		if e = tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() AND phase IN('prepared','approved','installed','completed') FROM owner_claim_starts WHERE transaction_id=$1`, owner.id).Scan(&live); e != nil {
			return p, e
		}
		if !live {
			return p, claims.ErrStale
		}
		count, e := readOwnerSwitchCount(ctx, tx, owner, q.Audience)
		if e != nil {
			return p, e
		}
		if e = returnFresh(ctx, tx, facts); e != nil {
			return p, e
		}
		if e = tx.Commit(); e != nil {
			return p, e
		}
		// Family freshness and commit may wait. Optional display facts do not
		// outlive their signed deadline while those authority checks finish.
		if count != nil && count.expires.After(time.Now()) {
			p.OtherPeopleCount = &count.people
		}
		return p, nil
	}
	if q.CurrentOwnerAccountID != "" {
		var owner string
		if e = tx.QueryRowContext(ctx, `SELECT owner_account_id FROM servers WHERE id=$1 AND revoked_at IS NULL AND NOT custody_released`, q.ServerID).Scan(&owner); e != nil {
			return p, claims.ErrStale
		}
		if owner != q.CurrentOwnerAccountID {
			var installed bool
			e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM claim_operations o JOIN servers s ON s.id=o.server_id JOIN server_members m ON m.server_id=s.id AND m.account_id=o.account_id AND m.role='owner' WHERE o.id=$1 AND o.server_id=$2 AND o.account_id=$3 AND s.owner_account_id=o.account_id AND o.state='installed' AND o.public_key=$4 AND o.local_generation=$5 AND o.purpose=$6 AND o.current_owner_account_id=$7 AND o.previous_owner_role=$8 AND o.return_registration_id=$9 AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.committed_token_hash=s.token_hash AND s.revoked_at IS NULL AND NOT s.custody_released)`, q.OperationID, q.ServerID, a.AccountID, mustWebPublicKey(q.PublicKey), q.LocalGeneration, q.Purpose, q.CurrentOwnerAccountID, q.PreviousOwnerRole, q.ReturnRegistrationID).Scan(&installed)
			if e != nil || !installed || p.Status != "approved" {
				return p, claims.ErrStale
			}
		}
		if e = tx.QueryRowContext(ctx, `SELECT username FROM accounts WHERE id=$1`, q.CurrentOwnerAccountID).Scan(&p.CurrentOwnerUsername); e != nil {
			return p, claims.ErrStale
		}
	}
	// A signed address is not a registration. Resolve only this exact installed
	// operation and its current route generation, including owner membership.
	var claim, cred, gen int64
	e = tx.QueryRowContext(ctx, `SELECT s.claim_generation,s.credential_generation,n.generation FROM servers s JOIN networking_route_state n ON n.server_id=s.id JOIN networking_route_candidates c ON c.server_id=n.server_id AND c.operation_id=n.operation_id AND c.generation=n.generation AND c.origin=$2 AND c.state='verified' WHERE s.id=$1 AND s.owner_account_id=$3 AND s.revoked_at IS NULL AND n.operation_id=$4 AND n.claim_generation=s.claim_generation AND n.credential_generation=s.credential_generation AND (n.remote_enabled OR c.class='lan')`, q.ServerID, q.Address, a.AccountID, q.OperationID).Scan(&claim, &cred, &gen)
	if e == nil && serverreturn.Origin(q.Address) {
		registration := serverreturn.RegistrationID(q.ServerID, q.OperationID, claim, cred, gen, q.Address)
		tuple, e := returnTupleTx(ctx, tx, a.AccountID, q.ServerID, q.OperationID, q.Purpose, registration)
		if e != nil || tuple.current != q.CurrentOwnerAccountID || tuple.previous != q.PreviousOwnerRole {
			return p, claims.ErrStale
		}
		p.ReturnRegistration = &serverreturn.Registration{ID: registration, Origin: q.Address}
	}

	return p, tx.Commit()
}
func (s *Store) DecideWebClaim(ctx context.Context, a identity.CurrentFamilyExpectation, code, replaces string, approve bool, service *claims.Service) error {
	// Claim approval's existing account/family checks remain the positive authority.
	// Denial and approval serialize on the account lock, with a durable decision.
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Read the immutable server-signed request before taking any account lock,
	// then lock target/current accounts in the shared canonical order.
	var preliminary []byte
	if e = tx.QueryRowContext(ctx, `SELECT request FROM claim_web_pending WHERE code_hash=$1`, security.Hash(code)).Scan(&preliminary); e != nil {
		return claims.ErrStale
	}
	var pre claimweb.Request
	if json.Unmarshal(preliminary, &pre) != nil {
		return claims.ErrInvalid
	}
	if e = lockClaimAccounts(ctx, tx, a.AccountID, pre.ServerID); e != nil {
		return e
	}
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
	if approve {
		// Execute the same approval consumer in this transaction; no decision gap.
		input := claims.ApprovalInput{OperationID: q.OperationID, ServerID: q.ServerID, PublicKey: q.PublicKey, LocalGeneration: q.LocalGeneration, Name: q.Name, ReplacesServerID: replaces, Purpose: q.Purpose, CurrentOwnerAccountID: q.CurrentOwnerAccountID, PreviousOwnerRole: q.PreviousOwnerRole, ReturnRegistrationID: q.ReturnRegistrationID}
		approval, e := s.approveClaimTx(ctx, tx, a, input, q.Audience)
		if e != nil {
			return e
		}
		if e = bindOwnerApprovalTx(ctx, tx, a, q, &approval); e != nil {
			return e
		}
		if e = bindSetupApprovalTx(ctx, tx, a, q, &approval); e != nil {
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
	if !approve {
		if _, e = tx.ExecContext(ctx, `UPDATE setup_claim_starts SET phase='cancelled' WHERE operation_id=$1 AND phase<>'consumed'`, op); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE owner_claim_starts SET phase='cancelled' WHERE operation_id=$1 AND phase<>'consumed'`, op); e != nil {
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

func mustWebPublicKey(raw string) []byte {
	key, _ := base64.RawURLEncoding.DecodeString(raw)
	return key
}
