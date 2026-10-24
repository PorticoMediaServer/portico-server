package hosted

import (
	"context"
	"database/sql"
	"encoding/json"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/serverreturn"
)

func (s *Service) setupCompletionCaller(ctx context.Context, bearer, origin string) (identity.PendingAccountSetup, string, string, error) {
	var none identity.PendingAccountSetup
	if s == nil || s.identity == nil || s.current == nil || !s.Configured() {
		return none, "", "", networking.ErrUnavailable
	}
	pending, proof, err := s.identity.SetupClaimAuthority(ctx, bearer)
	if err != nil {
		return none, "", "", err
	}
	g, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return none, "", "", err
	}
	defer g.Rollback()
	operation, err := s.identity.SetupClaimOperationTx(ctx, g.Tx(), pending, proof)
	if err != nil {
		return none, "", "", err
	}
	if err = identity.CheckSetupReturnOriginTx(ctx, g.Tx(), operation, origin, s.origin); err != nil {
		return none, "", "", err
	}
	return pending, proof, operation, nil
}

// PrepareSetupCompletion is an existing operation's post-install step. Its
// registration is resolved using the actual installed credential and serving
// HTTPS route. It never prepares an ordinary sign-in or invents an origin/ID.
func (s *Service) PrepareSetupCompletion(ctx context.Context, h *networking.ClaimHandler, bearer, sourceOrigin string) (serverreturn.Registration, error) {
	pending, proof, operation, err := s.setupCompletionCaller(ctx, bearer, sourceOrigin)
	if err != nil {
		return serverreturn.Registration{}, err
	}
	return s.prepareSetupCompletion(ctx, h, pending, proof, operation, sourceOrigin)
}

// PrepareCapturedSetupCompletion resumes the exact server-private capture after
// installation, including across restart. It binds a return registration only;
// no browser verifier, setup token or owner credential is recovered or issued.
func (s *Service) PrepareCapturedSetupCompletion(ctx context.Context, h *networking.ClaimHandler, pending identity.PendingAccountSetup, proof, operation string) (serverreturn.Registration, error) {
	if s == nil || s.identity == nil || s.current == nil || !s.Configured() {
		return serverreturn.Registration{}, networking.ErrUnavailable
	}
	g, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return serverreturn.Registration{}, err
	}
	sourceOrigin, err := s.identity.CapturedSetupReturnOriginTx(ctx, g.Tx(), pending, proof, operation, s.origin)
	g.Rollback()
	if err != nil {
		return serverreturn.Registration{}, err
	}
	return s.prepareSetupCompletion(ctx, h, pending, proof, operation, sourceOrigin)
}

func (s *Service) prepareSetupCompletion(ctx context.Context, h *networking.ClaimHandler, pending identity.PendingAccountSetup, proof, operation, sourceOrigin string) (serverreturn.Registration, error) {
	guard := func(ctx context.Context, tx *sql.Tx) error {
		if err := s.identity.CheckSetupContinuationTx(ctx, tx, pending, proof, operation, ""); err != nil {
			return err
		}
		return identity.CheckSetupReturnOriginTx(ctx, tx, operation, sourceOrigin, s.origin)
	}
	registration, _, err := h.BindSetupReturn(ctx, guard, func(ctx context.Context, tx *sql.Tx, r serverreturn.Registration, generation int64) (bool, error) {
		return s.identity.BindSetupReturnRegistrationTx(ctx, tx, pending, proof, operation, identity.SetupReturnRoute{Registration: r, Generation: generation})
	})
	return registration, err
}

// ExchangeSetupCompletion consumes exactly one captured operation attempt.
// The normal registered-return method above remains code-based and sends no
// Authorization. Only this separate private path uses the installed credential.
func (s *Service) ExchangeSetupCompletion(ctx context.Context, h *networking.ClaimHandler, bearer, sourceOrigin string, q serverreturn.SetupExchange) (identity.DirectSignIn, error) {
	var out identity.DirectSignIn
	pending, proof, operation, err := s.setupCompletionCaller(ctx, bearer, sourceOrigin)
	if err != nil {
		return out, err
	}
	var attempt, nonce string
	var route identity.SetupReturnRoute
	var who identity.PorticoIdentity
	err = h.RunSetupReturn(ctx, q, networking.SetupReturnActions{
		Guard: func(ctx context.Context, tx *sql.Tx) error {
			if err := s.identity.CheckSetupContinuationTx(ctx, tx, pending, proof, operation, ""); err != nil {
				return err
			}
			return identity.CheckSetupReturnOriginTx(ctx, tx, operation, sourceOrigin, s.origin)
		},
		BeginTx: func(ctx context.Context, tx *sql.Tx, r serverreturn.Registration, generation int64) error {
			route = identity.SetupReturnRoute{Registration: r, Generation: generation}
			actual, started, err := s.identity.BeginSetupReturnTx(ctx, tx, pending, proof, q, route)
			if err != nil {
				return err
			}
			if actual != operation {
				return identity.ErrUnauthorized
			}
			attempt = started
			return nil
		},
		CheckAttemptTx: func(ctx context.Context, tx *sql.Tx) error {
			if err := s.identity.CheckSetupReturnAttemptTx(ctx, tx, pending, proof, operation, attempt, q, route); err != nil {
				return err
			}
			return identity.CheckSetupReturnOriginTx(ctx, tx, operation, sourceOrigin, s.origin)
		},
		Verify: func(ctx context.Context, raw json.RawMessage) error {
			var err error
			who, nonce, err = s.VerifySetupReturn(ctx, raw, q)
			return err
		},
		CompleteTx: func(ctx context.Context, tx *sql.Tx) error {
			var err error
			out, err = s.identity.CompleteSetupReturnTx(ctx, tx, pending, proof, operation, attempt, q, route, who, nonce)
			return err
		},
		AbandonTx: func(ctx context.Context, tx *sql.Tx) error {
			return s.identity.AbandonSetupReturnTx(ctx, tx, operation, attempt)
		},
	})
	if err != nil {
		return identity.DirectSignIn{}, err
	}
	return out, nil
}
