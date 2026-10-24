package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"strconv"
	"time"

	"portico.local/server/internal/claimweb"
	"portico.local/server/internal/serverreturn"
)

// SetupReturnRoute is a private installed resolver fact, never URL input.
type SetupReturnRoute struct {
	Registration serverreturn.Registration
	Generation   int64
}
type setupCompletion struct {
	operation, phase, transaction, installation, challenge, registration, origin, attempt string
	generation, expires                                                                   int64
}

func readSetupCompletionTx(ctx context.Context, tx *sql.Tx, operation string) (c setupCompletion, err error) {
	c.operation = operation
	err = tx.QueryRowContext(ctx, `SELECT phase,transaction_id,installation_id,server_challenge_hash,registration_id,registration_origin,route_generation,expires_ms,attempt_id FROM identity_setup_completions WHERE operation_id=?`, operation).Scan(&c.phase, &c.transaction, &c.installation, &c.challenge, &c.registration, &c.origin, &c.generation, &c.expires, &c.attempt)
	return
}
func (s *Service) SetupClaimOperationTx(ctx context.Context, tx *sql.Tx, pending PendingAccountSetup, proof string) (string, error) {
	var operation string
	if err := tx.QueryRowContext(ctx, `SELECT operation_id FROM identity_setup_completions WHERE setup_request_id=? AND setup_revision=? AND setup_proof=? AND phase IN('prepared','installed')`, pending.RequestID, pending.Revision, proof).Scan(&operation); err != nil {
		return "", err
	}
	return operation, s.CheckSetupContinuationTx(ctx, tx, pending, proof, operation, "")
}
func CheckSetupReturnOriginTx(ctx context.Context, tx *sql.Tx, operation, origin, audience string) error {
	var cp, cs, sp, ss []byte
	if err := tx.QueryRowContext(ctx, `SELECT claim_payload,claim_signature,start_payload,start_signature FROM identity_setup_completions WHERE operation_id=?`, operation).Scan(&cp, &cs, &sp, &ss); err != nil {
		return err
	}
	claim, _, err := claimweb.VerifySetupStart(claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(cp), Signature: base64.RawURLEncoding.EncodeToString(cs)}, claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(sp), Signature: base64.RawURLEncoding.EncodeToString(ss)}, audience, time.Now())
	if err != nil || origin == "" || claim.Address != origin {
		return ErrUnauthorized
	}
	return ctx.Err()
}
func setupReturnRouteTx(ctx context.Context, tx *sql.Tx, operation string, route SetupReturnRoute) error {
	if !serverreturn.Origin(route.Registration.Origin) || route.Generation < 1 {
		return ErrUnauthorized
	}
	var server, claim, credential string
	var generation int64
	var enabled bool
	err := tx.QueryRowContext(ctx, `SELECT i.server_id,i.claim_generation,i.credential_generation,r.generation,m.enabled FROM networking_claim_intents i JOIN networking_remote_state r ON r.singleton=1 JOIN identity_managed_route m ON m.singleton=1 JOIN networking_claim_identity k ON k.singleton=1 AND k.active_operation_id=i.operation_id AND k.installed_operation_id=i.operation_id AND k.server_id=i.server_id AND k.public_key=i.public_key AND k.reset_generation=i.local_generation WHERE i.operation_id=? AND i.purpose='setup' AND i.stage='installed' AND i.installation_acknowledged=1`, operation).Scan(&server, &claim, &credential, &generation, &enabled)
	if err != nil {
		return err
	}
	cg, ce := strconv.ParseInt(claim, 10, 64)
	kg, ke := strconv.ParseInt(credential, 10, 64)
	if ce != nil || ke != nil || cg < 1 || kg < 1 || !enabled || generation != route.Generation || route.Registration.ID != serverreturn.RegistrationID(server, operation, cg, kg, generation, route.Registration.Origin) {
		return ErrUnauthorized
	}
	return ctx.Err()
}
func (s *Service) BindSetupReturnRegistrationTx(ctx context.Context, tx *sql.Tx, pending PendingAccountSetup, proof, operation string, route SetupReturnRoute) (bool, error) {
	if err := s.CheckSetupContinuationTx(ctx, tx, pending, proof, operation, ""); err != nil {
		return false, err
	}
	c, err := readSetupCompletionTx(ctx, tx, operation)
	if err != nil {
		return false, err
	}
	if c.transaction == "" {
		return false, ErrUnauthorized
	}
	if err = setupReturnRouteTx(ctx, tx, operation, route); err != nil {
		return false, err
	}
	if c.registration != "" {
		if c.registration == route.Registration.ID && c.origin == route.Registration.Origin && c.generation == route.Generation {
			return true, nil
		}
		return false, s.endSetupReturnTx(ctx, tx, c, "superseded")
	}
	result, err := tx.ExecContext(ctx, `UPDATE identity_setup_completions SET phase='installed',installed_ms=?,registration_id=?,registration_origin=?,route_generation=? WHERE operation_id=? AND phase='prepared' AND registration_id=''`, time.Now().UnixMilli(), route.Registration.ID, route.Registration.Origin, route.Generation, operation)
	if err != nil {
		return false, err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return false, ErrConflict
	}
	return true, s.CheckSetupContinuationTx(ctx, tx, pending, proof, operation, "")
}
func setupReturnBindingTx(ctx context.Context, tx *sql.Tx, c setupCompletion, q serverreturn.SetupExchange, route SetupReturnRoute) error {
	if !q.Valid() || !ownerReturnSecret32(q.State) || !ownerReturnSecret32(q.Verifier) || !validInstallationID(q.InstallationID) || c.transaction != q.TransactionID || c.installation != q.InstallationID || c.challenge != Digest(q.ServerChallenge) || c.registration == "" || c.registration != q.RegistrationID || c.origin != route.Registration.Origin || c.registration != route.Registration.ID || c.generation != route.Generation {
		return ErrUnauthorized
	}
	if err := setupReturnRouteTx(ctx, tx, c.operation, route); err != nil {
		return err
	}
	var state, challenge string
	if err := tx.QueryRowContext(ctx, `SELECT state_hash,code_challenge FROM identity_setup_completions WHERE operation_id=?`, c.operation).Scan(&state, &challenge); err != nil {
		return err
	}
	pkce := sha256.Sum256([]byte(q.Verifier))
	if subtle.ConstantTimeCompare([]byte(state), []byte(Digest(q.State))) != 1 || subtle.ConstantTimeCompare([]byte(challenge), []byte(base64.RawURLEncoding.EncodeToString(pkce[:]))) != 1 {
		return ErrUnauthorized
	}
	return ctx.Err()
}
func (s *Service) BeginSetupReturnTx(ctx context.Context, tx *sql.Tx, pending PendingAccountSetup, proof string, q serverreturn.SetupExchange, route SetupReturnRoute) (operation, attempt string, err error) {
	operation, err = s.SetupClaimOperationTx(ctx, tx, pending, proof)
	if err != nil {
		return "", "", err
	}
	c, err := readSetupCompletionTx(ctx, tx, operation)
	if err != nil {
		return "", "", err
	}
	if c.phase != "installed" {
		return "", "", ErrUnauthorized
	}
	if err = setupReturnBindingTx(ctx, tx, c, q, route); err != nil {
		return "", "", err
	}
	attempt = Token()
	result, err := tx.ExecContext(ctx, `UPDATE identity_setup_completions SET phase='consumed',consumed_ms=?,attempt_id=? WHERE operation_id=? AND phase='installed' AND attempt_id='' AND consumed_ms=0`, time.Now().UnixMilli(), attempt, operation)
	if err != nil {
		return "", "", err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return "", "", ErrConflict
	}
	return operation, attempt, s.CheckSetupReturnAttemptTx(ctx, tx, pending, proof, operation, attempt, q, route)
}
func (s *Service) CheckSetupReturnAttemptTx(ctx context.Context, tx *sql.Tx, pending PendingAccountSetup, proof, operation, attempt string, q serverreturn.SetupExchange, route SetupReturnRoute) error {
	if attempt == "" {
		return ErrUnauthorized
	}
	if err := s.CheckSetupContinuationTx(ctx, tx, pending, proof, operation, attempt); err != nil {
		return err
	}
	c, err := readSetupCompletionTx(ctx, tx, operation)
	if err != nil {
		return err
	}
	if c.phase != "consumed" || c.attempt != attempt {
		return ErrUnauthorized
	}
	return setupReturnBindingTx(ctx, tx, c, q, route)
}

// Completion spends the challenge and nonce with the sole existing account
// setup admission boundary; approval or a return alone never grants a session.
func (s *Service) CompleteSetupReturnTx(ctx context.Context, tx *sql.Tx, pending PendingAccountSetup, proof, operation, attempt string, q serverreturn.SetupExchange, route SetupReturnRoute, who PorticoIdentity, nonce string) (DirectSignIn, error) {
	var none DirectSignIn
	if err := s.CheckSetupReturnAttemptTx(ctx, tx, pending, proof, operation, attempt, q, route); err != nil {
		return none, err
	}
	c, err := readSetupCompletionTx(ctx, tx, operation)
	if err != nil {
		return none, err
	}
	r := who.Return
	if r == nil || r.Purpose != "setup" || r.CurrentOwnerAccountID != "" || r.PreviousOwnerRole != "" || r.OperationID != operation || r.InstallationID != q.InstallationID || r.RegistrationID != q.RegistrationID || len(nonce) < 22 || len(nonce) > 128 {
		return none, ErrUnauthorized
	}
	if err = CheckPorticoReturnTx(ctx, tx, who); err != nil {
		return none, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM hosted_identity_challenges WHERE challenge_hash=? AND installation_id=? AND expires_ms>?`, c.challenge, c.installation, time.Now().UnixMilli())
	if err != nil {
		return none, err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return none, ErrUnauthorized
	}
	result, err = tx.ExecContext(ctx, `INSERT INTO hosted_identity_nonces(nonce,expires_ms) VALUES(?,?) ON CONFLICT(nonce) DO NOTHING`, nonce, r.ExpiresAt.UnixMilli())
	if err != nil {
		return none, err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return none, ErrUnauthorized
	}
	out, err := s.completeAccountSetupTx(ctx, tx, pending, proof, who)
	if err != nil {
		return none, err
	}
	result, err = tx.ExecContext(ctx, `UPDATE identity_setup_completions SET phase='completed',completed_ms=? WHERE operation_id=? AND phase='consumed' AND attempt_id=? AND expires_ms>?`, time.Now().UnixMilli(), operation, attempt, time.Now().UnixMilli())
	if err != nil {
		return none, err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return none, ErrUnauthorized
	}
	if err = CheckPorticoReturnTx(ctx, tx, who); err != nil {
		return none, err
	}
	if time.Now().UnixMilli() >= c.expires {
		return none, ErrUnauthorized
	}
	return out, ctx.Err()
}
func (s *Service) endSetupReturnTx(ctx context.Context, tx *sql.Tx, c setupCompletion, phase string) error {
	if phase != "cancelled" && phase != "superseded" && phase != "expired" {
		return ErrDirectInput
	}
	if _, err := tx.ExecContext(ctx, `UPDATE identity_setup_completions SET phase=? WHERE operation_id=? AND phase IN('prepared','installed','consumed')`, phase, c.operation); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM hosted_identity_challenges WHERE challenge_hash=? AND installation_id=?`, c.challenge, c.installation)
	return err
}
func (s *Service) AbandonSetupReturnTx(ctx context.Context, tx *sql.Tx, operation, attempt string) error {
	c, err := readSetupCompletionTx(ctx, tx, operation)
	if err != nil {
		return err
	}
	if c.phase != "consumed" || c.attempt != attempt || attempt == "" {
		return ErrUnauthorized
	}
	return s.endSetupReturnTx(ctx, tx, c, "cancelled")
}
