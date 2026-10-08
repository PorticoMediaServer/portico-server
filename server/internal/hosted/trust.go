package hosted

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/hostedtrust"
	"portico.local/server/internal/identity"
	"strings"
	"time"
)

func DefaultConfig(origin, pin, id string) (string, string, string, error) {
	if origin != "" || pin != "" || id != "" {
		if origin == "" || pin == "" || id == "" {
			return "", "", "", errors.New("explicit Hosted override requires origin, root public key and root ID")
		}
		key, e := base64.RawURLEncoding.Strict().DecodeString(pin)
		if e != nil || len(key) != 32 {
			return "", "", "", errors.New("invalid Hosted root override")
		}
		if id != trust.KeyID(key) {
			return "", "", "", errors.New("Hosted root override ID does not match its key")
		}
		u, e := url.Parse(origin)
		if e != nil {
			return "", "", "", e
		}
		if trust.IsDevelopmentRoot(key) && strings.EqualFold(u.Hostname(), "web.getportico.tv") {
			return "", "", "", errors.New("development root cannot use production Hosted origin")
		}
		return origin, pin, id, nil
	}
	root, err := trust.DefaultRoot()
	if err != nil {
		if !trust.Release {
			return "", "", "", nil
		}
		return "", "", "", err
	}
	if trust.IsDevelopmentRoot(root) {
		return "", "", "", nil
	}
	return trust.Origin, base64.RawURLEncoding.EncodeToString(root), trust.KeyID(root), nil
}
func (s *Service) TrustConfig() (string, string, string) {
	return s.origin, base64.RawURLEncoding.EncodeToString(s.key), s.keyID
}
func (s *Service) verifyEnvelope(ctx context.Context, e trust.Envelope) ([]byte, error) {
	return s.verifyEnvelopeAt(ctx, e, time.Now())
}

// verifyEnvelopeAt checks the signing certificate as of at: the moment the
// document was issued, for documents a client stores and presents later (A9:
// an offline profile proof stays valid after the key that signed it is
// renewed). Revocation is always checked as of now.
func (s *Service) verifyEnvelopeAt(ctx context.Context, e trust.Envelope, at time.Time) ([]byte, error) {
	raw, err := e.Verify(s.key, "documents", at)
	if err != nil {
		return nil, err
	}
	if s.db == nil {
		return raw, nil
	} // pure verifier fixtures have no persistence
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Root-signed revocations are permanent and monotonic even across restarts.
	// No transaction or writes for the common case with an unchanged key set.
	var revision uint64
	if err = s.db.QueryRowContext(ctx, `SELECT revision FROM hosted_trust_state WHERE singleton=1`).Scan(&revision); err != nil {
		return nil, err
	}
	if e.Certificate.Revision > revision {
		gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassInteractive)
		if err != nil {
			return nil, err
		}
		tx := gated.Tx()
		defer gated.Rollback()
		for _, id := range e.Certificate.Revoked {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO hosted_signing_revocations(key_id) VALUES(?)`, id); err != nil {
				return nil, err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE hosted_trust_state SET revision=max(revision,?) WHERE singleton=1`, e.Certificate.Revision); err != nil {
			return nil, err
		}
		// A newly learned root revocation also withdraws cached authorization.
		if err = identity.RevokeFamiliesMatchingTx(ctx, tx, identity.RevokedAdminRevoke, `authority='hosted' AND server_id IN (SELECT server_id FROM hosted_policy_signers p JOIN hosted_signing_revocations r ON r.key_id=p.key_id)`); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM policy WHERE server_id IN (SELECT server_id FROM hosted_policy_signers p JOIN hosted_signing_revocations r ON r.key_id=p.key_id)`); err != nil {
			return nil, err
		}
		if err = gated.Commit(); err != nil {
			return nil, err
		}
	}
	var revoked bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM hosted_signing_revocations WHERE key_id=?)`, e.KeyID).Scan(&revoked); err != nil {
		return nil, err
	}
	if revoked {
		return nil, trust.ErrTrust
	}
	return raw, nil
}
func (s *Service) ValidateTrust(ctx context.Context, e trust.Envelope) error {
	_, err := s.verifyEnvelope(ctx, e)
	return err
}
