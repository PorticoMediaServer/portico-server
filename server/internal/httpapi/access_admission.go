package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/playbackv1"
	"time"

	"portico.local/server/internal/access"
	"portico.local/server/internal/identity"
)

// controlItemID reads just the subject out of a v2 control envelope. The
// control service owns the full schema; admission needs only the one field,
// and an envelope it cannot read yields an empty subject, which applies the
// account-level limits and skips the per-title ones.
func controlItemID(raw []byte) string {
	var body struct {
		ItemID  string `json:"itemId"`
		Desired struct {
			Context struct {
				ItemID string `json:"itemId"`
			} `json:"context"`
		} `json:"desired"`
		Subject struct {
			ItemID string `json:"itemId"`
		} `json:"subject"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	if body.Desired.Context.ItemID != "" {
		return body.Desired.Context.ItemID
	}
	if body.ItemID != "" {
		return body.ItemID
	}
	return body.Subject.ItemID
}

// This file is the one place the per-member limits meet a running request.
//
//	access schedule       — admitSession, admitPlayback and admitChannel.
//	max streams           — admitPlayback, before a playback lease is created,
//	                        counting legacy sessions and live v1 sessions.
//	maximum content rating— admitPlayback, and catalog visibility through
//	                        access.Enforcer.VisibleItem / VisibilityClause.
//	denied labels         — admitPlayback, same visibility pair.
//	remote bitrate cap    — admitPlayback returns it as a delivery clamp.
//	channel policy        — admitChannel, on v1 channel session starts only
//	                        (canonical v1 ids), with the schedule.
//	device trust          — admitPlayback, when the server requires approval.

// admitSession applies the access schedule to a session that has just been
// issued. Checking after issuance keeps the identity service
// untouched; a denied session is revoked in the same request, so the caller
// never holds a usable token it was not entitled to.
func (d Dependencies) admitSession(r *http.Request, envelope identity.Envelope) error {
	if d.Access.Access == nil || d.DB == nil || envelope.AccessToken == "" {
		return nil
	}
	p := identity.Principal{Viewer: envelope.Viewer}
	gated, err := dbwork.Begin(r.Context(), d.DB, dbwork.ClassFrom(r.Context(), dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if err = d.enforcer(false).AdmitSession(r.Context(), tx, p); err == nil {
		return nil
	}
	denial := err
	if e := identity.RevokeFamiliesMatchingTx(r.Context(), tx, identity.RevokedAdminRevoke, `id IN (SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, identity.Digest(envelope.AccessToken)); e != nil {
		return e
	}
	if e := gated.Commit(); e != nil {
		return e
	}
	return denial
}

// ErrRemoteSignIn is the connectivity policy's refusal. It is separate from a
// bad password: the credentials were right, the origin was not.
var ErrRemoteSignIn = errors.New("This server does not accept sign-in from outside its own network.")

// admitRemoteSignIn applies the server-wide remote sign-in policy to a session
// that has just been issued. "owner-only" admits the owner tier and refuses the
// rest; "off" refuses every remote sign-in. Sessions already signed in are not
// disturbed, which is what the setting's description promises.
func (d Dependencies) admitRemoteSignIn(r *http.Request, envelope identity.Envelope) error {
	if d.Console == nil || envelope.AccessToken == "" {
		return nil
	}
	document, err := d.settingsDocument(r.Context())
	if err != nil {
		return err
	}
	policy := document.Effective.RemoteSignInPolicy
	if policy == "" || policy == "allow" || !d.requestIsRemote(r) {
		return nil
	}
	if policy == "owner-only" && identity.Grants(envelope.Viewer.Role, identity.TierOwner) {
		return nil
	}
	if d.DB != nil {
		if e := dbwork.WithWriteTx(r.Context(), d.DB, dbwork.ClassFrom(r.Context(), dbwork.ClassInteractive), func(tx *sql.Tx) error {
			return identity.RevokeFamiliesMatchingTx(r.Context(), tx, identity.RevokedAdminRevoke, `id IN (SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, identity.Digest(envelope.AccessToken))
		}); e != nil {
			return e
		}
	}
	return ErrRemoteSignIn
}

// admitPlayback applies the playback-side limits and returns the clamps a
// permitted request still has to respect.
func (d Dependencies) admitPlayback(r *http.Request, p identity.Principal, item string) (access.Decision, error) {
	if item != "" {
		if err := d.restrictedItem(r, p, item); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return access.Decision{}, identity.ErrContentRestricted
			}
			return access.Decision{}, err
		}
	}
	if d.Access.Access == nil || d.DB == nil {
		return access.Decision{}, nil
	}
	document, err := d.settingsDocument(r.Context())
	if err != nil {
		return access.Decision{}, err
	}
	// Locality may read settings on a cold cache, so it is decided before the
	// admission transaction opens (one SQLite connection: a read inside would
	// wait on the transaction holding it).
	remote := d.requestIsRemote(r)
	gated2, err := dbwork.Begin(r.Context(), d.DB, dbwork.ClassFrom(r.Context(), dbwork.ClassInteractive))
	if err != nil {
		return access.Decision{}, err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	device := r.Header.Get("X-Portico-Device-Id")
	decision, err := d.enforcer(document.Effective.DeviceApprovalRequired).AdmitPlayback(r.Context(), tx, p, access.Admission{ItemID: item, Remote: remote, DeviceID: device, ReplanSessionID: playbackv1.ReplanOf(r.Context()).Session, ReplanMediaID: playbackv1.ReplanOf(r.Context()).Media})
	if err != nil {
		return decision, err
	}
	budgeted, exhausted := false, false
	if remote && document.Effective.UploadCapacityKbps > 0 {
		ceiling, full, err := uploadBudgetCeiling(r.Context(), tx, document.Effective.UploadCapacityKbps, playbackv1.ReplanOf(r.Context()).Session, time.Now())
		if err != nil {
			return decision, err
		}
		decision.MaxVideoBitrateBPS = narrowBitrate(decision.MaxVideoBitrateBPS, ceiling)
		budgeted, exhausted = true, full
	}
	if err = d.Access.Access.Observe(r.Context(), tx, p, device, r.Header.Get("X-Portico-Device-Name"), r.Header.Get("X-Portico-Device-Platform")); err != nil {
		return decision, err
	}
	if err = gated2.Commit(); err != nil {
		return decision, err
	}
	if budgeted {
		d.alertUploadBudget(r.Context(), exhausted)
	}
	return decision, nil
}

// admitChannel applies the live channel allow/deny policy (canonical v1 ids
// only) with the schedule.
func (d Dependencies) admitChannel(r *http.Request, p identity.Principal, channel string) error {
	if err := d.featureAllowed(r.Context(), p, "live_tv"); err != nil {
		return err
	}
	if d.Access.Access == nil || d.DB == nil || channel == "" {
		return nil
	}
	gated3, err := dbwork.BeginSnapshot(r.Context(), d.DB)
	if err != nil {
		return err
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	return d.enforcer(false).AdmitChannel(r.Context(), tx, p, channel)
}
