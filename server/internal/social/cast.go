package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/identity"
)

// Google Cast bootstrap, redeem and reconnect.
//
// A Cast receiver page has no Portico credential and cannot be handed one over
// the Cast channel safely, so the viewer's phone asks the server for a short,
// one-use pairing code and shows it; the receiver page redeems that code
// directly with the server over HTTPS. Redemption returns two separate
// credentials:
//
//	deviceToken  long-lived, device-scoped, accepted ONLY by /v1/cast/reconnect
//	session      an ordinary short-lived viewer bearer the receiver plays with
//
// They are deliberately different credentials with different reach. The device
// token survives a receiver restart and is rotated on every reconnect; it is
// never a Bearer, so leaking it cannot be replayed against the media API. The
// session is the ordinary bearer every other client uses, so the receiver plays
// through the ordinary playback session API with no private surface at all.

// castCapabilities is the accepted capability vocabulary.
var castCapabilities = map[string]bool{"load": true, "control": true, "stop": true, "progress": true, "renew": true, "reconnect": true, "advance": true, "segment-skip": true}

type CastBootstrapRequest struct {
	ProtocolVersion string   `json:"protocolVersion"`
	DisplayName     string   `json:"displayName"`
	Capabilities    []string `json:"capabilities"`
}

type CastBootstrap struct {
	ID           string   `json:"id"`
	Code         string   `json:"code"`
	State        string   `json:"state"`
	DisplayName  string   `json:"displayName"`
	Capabilities []string `json:"capabilities"`
	ExpiresAt    string   `json:"expiresAt"`
	// ApplicationID is the published Google Cast application id, read from the
	// `cast.applicationId` setting. It is configuration, never a default: an
	// unset setting yields an empty value and the sender must not advertise Cast.
	ApplicationID string `json:"applicationId"`
	ReceiverURL   string `json:"receiverUrl"`
}

type CastBootstrapResponse struct {
	ProtocolVersion string        `json:"protocolVersion"`
	ServerTime      string        `json:"serverTime"`
	Bootstrap       CastBootstrap `json:"bootstrap"`
}

type CastRedeemRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	Code            string `json:"code"`
	DeviceID        string `json:"deviceId"`
	DisplayName     string `json:"displayName"`
	// Source is the caller's network, set by the route: failed attempts are
	// charged to it as well as to the server-wide budget.
	Source string `json:"-"`
}

type CastReconnectRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	DeviceToken     string `json:"deviceToken"`
	// DeviceID is the receiver's own id. It is required only to recover with the
	// token a lost reconnect replaced (see CastReconnect).
	DeviceID string `json:"deviceId,omitempty"`
	Source   string `json:"-"`
}

type CastScope struct {
	AccountID    string   `json:"accountId"`
	ProfileID    string   `json:"profileId"`
	Authority    string   `json:"authority"`
	Capabilities []string `json:"capabilities"`
}

type CastSession struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

type CastDevice struct {
	ID          string `json:"id"`
	DeviceID    string `json:"deviceId"`
	DisplayName string `json:"displayName"`
	ReceiverID  string `json:"receiverId"`
	State       string `json:"state"`
	Generation  string `json:"generation"`
	ExpiresAt   string `json:"expiresAt"`
}

// CastReceiverSession is the redeem and reconnect answer. `grantSemantics` is
// `initial` on first redemption and `rotation` on every reconnect, so a receiver
// can tell a fresh pairing from a renewed one.
type CastReceiverSession struct {
	ProtocolVersion string      `json:"protocolVersion"`
	ServerTime      string      `json:"serverTime"`
	GrantSemantics  string      `json:"grantSemantics"`
	DeviceToken     string      `json:"deviceToken"`
	Device          CastDevice  `json:"device"`
	Scope           CastScope   `json:"scope"`
	Session         CastSession `json:"session"`
	ApplicationID   string      `json:"applicationId"`
}

func normalizeCastCapabilities(in []string) ([]string, bool) {
	if len(in) == 0 {
		in = []string{"load", "control", "stop", "progress", "renew", "reconnect"}
	}
	seen := map[string]bool{}
	for _, c := range in {
		if !castCapabilities[c] {
			return nil, false
		}
		seen[c] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, true
}

// applicationID reads the published Cast application id. tx may be nil when the
// caller holds no transaction.
func (s *Store) applicationID(ctx context.Context, tx *sql.Tx) string {
	if s.CastApplicationID == nil {
		return ""
	}
	return s.CastApplicationID(ctx, tx)
}

// CastBootstrapStart issues a one-use pairing code for the calling viewer.
func (s *Store) CastBootstrapStart(ctx context.Context, p identity.Principal, req CastBootstrapRequest) (CastBootstrapResponse, error) {
	var out CastBootstrapResponse
	if req.ProtocolVersion != Protocol {
		return out, errInvalid
	}
	if req.DisplayName == "" {
		req.DisplayName = "Cast receiver"
	}
	if !text(req.DisplayName, 64) {
		return out, errInvalid
	}
	capabilities, ok := normalizeCastCapabilities(req.Capabilities)
	if !ok {
		return out, errInvalid
	}
	raw, e := json.Marshal(capabilities)
	if e != nil {
		return out, e
	}
	gated, e := s.begin(ctx)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = s.limit(ctx, tx, "cast-bootstrap:"+viewerKey(p.Viewer), CastBootstrapPerHour, time.Hour); e != nil {
		return out, e
	}
	// A Hosted viewer the current Hosted policy wouldn't issue a session to is
	// refused now, before a code is shown that could never be redeemed.
	if _, e = s.castHorizonTx(ctx, tx, castIdentity{p.Authority, p.AccountID, p.ProfileID, p.Role, p.Epoch}); e != nil {
		return out, e
	}
	// A viewer holds one pending code at a time: showing two codes on two TVs
	// invites the wrong one being typed.
	if _, e = tx.ExecContext(ctx, `UPDATE social_cast_bootstraps SET state='cancelled' WHERE authority=? AND account_id=? AND profile_id=? AND state='pending'`, p.Authority, p.AccountID, p.ProfileID); e != nil {
		return out, e
	}
	code, e := newCode(6)
	if e != nil {
		return out, e
	}
	now := s.ms()
	expires := now + CastCodeTTLSeconds*1000
	id := token("cbs_")
	if _, e = tx.ExecContext(ctx, `INSERT INTO social_cast_bootstraps(id,code_digest,authority,account_id,profile_id,role,account_epoch,display_name,capabilities,state,created_at_ms,expires_at_ms) VALUES(?,?,?,?,?,?,?,?,?,'pending',?,?)`,
		id, digest(code), p.Authority, p.AccountID, p.ProfileID, p.Role, p.Epoch, req.DisplayName, string(raw), now, expires); e != nil {
		return out, e
	}
	out = CastBootstrapResponse{ProtocolVersion: Protocol, ServerTime: stamp(now), Bootstrap: CastBootstrap{
		ID: id, Code: code, State: "pending", DisplayName: req.DisplayName, Capabilities: capabilities,
		ExpiresAt: stamp(expires), ApplicationID: s.applicationID(ctx, tx), ReceiverURL: "/receiver/cast/",
	}}
	return out, gated.Commit()
}

// CastBootstrapStatus lets the sender watch its own code without leaking it: the
// code is not echoed back.
func (s *Store) CastBootstrapStatus(ctx context.Context, p identity.Principal, id string) (CastBootstrapResponse, error) {
	var out CastBootstrapResponse
	gated2, e := s.begin(ctx)
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	var state, name, capabilities string
	var expires int64
	e = tx.QueryRowContext(ctx, `SELECT state,display_name,capabilities,expires_at_ms FROM social_cast_bootstraps WHERE id=? AND authority=? AND account_id=? AND profile_id=?`, id, p.Authority, p.AccountID, p.ProfileID).Scan(&state, &name, &capabilities, &expires)
	if errors.Is(e, sql.ErrNoRows) {
		return out, errNotFound
	}
	if e != nil {
		return out, e
	}
	if state == "pending" && s.ms() >= expires {
		state = "expired"
	}
	bootstrap := CastBootstrap{ID: id, State: state, DisplayName: name, ExpiresAt: stamp(expires), ApplicationID: s.applicationID(ctx, tx), ReceiverURL: "/receiver/cast/"}
	_ = json.Unmarshal([]byte(capabilities), &bootstrap.Capabilities)
	return CastBootstrapResponse{ProtocolVersion: Protocol, ServerTime: stamp(s.ms()), Bootstrap: bootstrap}, nil
}

// CastRedeem exchanges a pairing code for a device token and a viewer session.
// It is the one unauthenticated route in this group — the receiver has no
// credential yet — so failed attempts are budgeted (see castAttempt) and the
// code is single use. The code is consumed, the device token stored and the
// session issued in one transaction: a code is never spent on a session that
// could not be issued.
func (s *Store) CastRedeem(ctx context.Context, req CastRedeemRequest) (CastReceiverSession, error) {
	var out CastReceiverSession
	if req.ProtocolVersion != Protocol || !ValidID(req.DeviceID) {
		return out, errInvalid
	}
	code, ok := normalizeCode(req.Code)
	if !ok {
		return out, errInvalid
	}
	if req.DisplayName != "" && !text(req.DisplayName, 64) {
		return out, errInvalid
	}
	if s.Identity == nil {
		return out, &Fault{Code: "cast_unavailable", Status: 503, Message: "Cast pairing is not available.", Retryable: true}
	}
	return s.castAttempt(ctx, req.Source, func(tx *sql.Tx) (CastReceiverSession, error) {
		var bootstrapID, authority, account, profile, role, name, capabilities, state string
		var epoch int
		var expires int64
		e := tx.QueryRowContext(ctx, `SELECT id,authority,account_id,profile_id,role,account_epoch,display_name,capabilities,state,expires_at_ms FROM social_cast_bootstraps WHERE code_digest=?`, digest(code)).Scan(&bootstrapID, &authority, &account, &profile, &role, &epoch, &name, &capabilities, &state, &expires)
		if errors.Is(e, sql.ErrNoRows) {
			return out, credentialFault("cast_code_not_found", 404, "That pairing code is not valid.")
		}
		if e != nil {
			return out, e
		}
		now := s.ms()
		switch {
		case state == "redeemed":
			return out, credentialFault("cast_code_consumed", 410, "That pairing code has already been used.")
		case state == "cancelled":
			return out, credentialFault("cast_code_revoked", 410, "That pairing code was cancelled.")
		case state != "pending" || now >= expires:
			return out, credentialFault("cast_code_expired", 410, "That pairing code has expired.")
		}
		if _, e = tx.ExecContext(ctx, `UPDATE social_cast_bootstraps SET state='redeemed',device_id=?,redeemed_at_ms=?,attempts=attempts+1 WHERE id=?`, req.DeviceID, now, bootstrapID); e != nil {
			return out, e
		}
		if req.DisplayName != "" {
			name = req.DisplayName
		}
		// The Cast device also lands in the receiver directory, so the viewer's
		// phone lists the TV alongside first-party Portico receivers.
		receiverID := token("rcv_")
		if _, e = tx.ExecContext(ctx, `INSERT INTO social_receivers(id,authority,account_id,profile_id,kind,device_id,display_name,platform,key_fingerprint,supported_commands,grant_policy,authorization_revision,state,created_at_ms,last_seen_ms) VALUES(?,?,?,?,'cast',?,?,'google-cast','',?,'open',1,'active',?,?) ON CONFLICT(authority,account_id,profile_id,device_id) DO UPDATE SET display_name=excluded.display_name,state='active',last_seen_ms=excluded.last_seen_ms`,
			receiverID, authority, account, profile, req.DeviceID, name, `["load","play","pause","seek","stop"]`, now, now); e != nil {
			return out, e
		}
		if e = tx.QueryRowContext(ctx, `SELECT id FROM social_receivers WHERE authority=? AND account_id=? AND profile_id=? AND device_id=?`, authority, account, profile, req.DeviceID).Scan(&receiverID); e != nil {
			return out, e
		}
		id := castIdentity{authority, account, profile, role, epoch}
		rotation := castRotation{bootstrapID: bootstrapID, receiverID: receiverID, name: name, capabilities: capabilities}
		return s.issueCast(ctx, tx, id, req.DeviceID, rotation, "initial")
	})
}

type castIdentity struct {
	authority, account, profile, role string
	epoch                             int
}

// castRotation is what a device row is (re)issued with. previous is the digest
// of the token this issue replaces, usable until previousUntil (ms); both are
// empty on a pairing, and a recovery keeps the deadline of the rotation it
// recovers so retries never extend it.
type castRotation struct {
	bootstrapID, receiverID, name, capabilities string
	previous                                    string
	previousUntil                               int64
}

// CastRecoveryWindow is how long the token a reconnect replaced still reconnects
// the same device, for a receiver that never received the answer. Using the new
// token ends it at once.
const CastRecoveryWindow = 2 * time.Minute

// issueCast creates or rotates the device row, issues the viewer session and
// retires the session the device held before, all in tx. The token is stored
// only as a digest; the plaintext is returned once.
func (s *Store) issueCast(ctx context.Context, tx *sql.Tx, id castIdentity, deviceID string, r castRotation, semantics string) (CastReceiverSession, error) {
	var out CastReceiverSession
	now := s.ms()
	expires := now + CastSessionTTLSeconds*1000
	value := "ptc_cr_" + identity.Token()
	var rowID, priorFamily string
	var generation int64
	e := tx.QueryRowContext(ctx, `SELECT id,generation,session_family_id FROM social_cast_devices WHERE authority=? AND account_id=? AND profile_id=? AND device_id=?`, id.authority, id.account, id.profile, deviceID).Scan(&rowID, &generation, &priorFamily)
	switch {
	case errors.Is(e, sql.ErrNoRows):
		rowID, generation = token("cdv_"), 1
		if _, e = tx.ExecContext(ctx, `INSERT INTO social_cast_devices(id,token_digest,bootstrap_id,receiver_id,authority,account_id,profile_id,role,account_epoch,device_id,display_name,capabilities,generation,state,created_at_ms,expires_at_ms,last_seen_ms) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,'active',?,?,?)`,
			rowID, digest(value), r.bootstrapID, r.receiverID, id.authority, id.account, id.profile, id.role, id.epoch, deviceID, r.name, r.capabilities, now, expires, now); e != nil {
			return out, e
		}
	case e != nil:
		return out, e
	default:
		generation++
		if _, e = tx.ExecContext(ctx, `UPDATE social_cast_devices SET token_digest=?,previous_token_digest=?,previous_valid_until_ms=?,bootstrap_id=?,receiver_id=?,display_name=?,capabilities=?,generation=?,state='active',expires_at_ms=?,last_seen_ms=? WHERE id=?`,
			digest(value), r.previous, r.previousUntil, r.bootstrapID, r.receiverID, r.name, r.capabilities, generation, expires, now, rowID); e != nil {
			return out, e
		}
	}
	device := CastDevice{ID: rowID, DeviceID: deviceID, DisplayName: r.name, ReceiverID: r.receiverID, State: "active", Generation: counter(generation), ExpiresAt: stamp(expires)}
	session, family, e := s.castSessionTx(ctx, tx, id, device)
	if e != nil {
		return out, e
	}
	if priorFamily != "" && priorFamily != family {
		if e = s.Identity.RetireFamilyTx(ctx, tx, id.authority, id.account, id.profile, priorFamily); e != nil {
			return out, e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE social_cast_devices SET session_family_id=? WHERE id=?`, family, rowID); e != nil {
		return out, e
	}
	var list []string
	_ = json.Unmarshal([]byte(r.capabilities), &list)
	return CastReceiverSession{
		ProtocolVersion: Protocol, ServerTime: stamp(s.ms()), GrantSemantics: semantics,
		DeviceToken: value, Device: device,
		Scope:         CastScope{AccountID: id.account, ProfileID: id.profile, Authority: id.authority, Capabilities: list},
		Session:       session,
		ApplicationID: s.applicationID(ctx, tx),
	}, nil
}

// castSessionTx mints the ordinary viewer bearer the receiver plays with, inside
// the pairing transaction. The bearer binds the receiver's own identity-device
// record — one stable installation per pairing, reused across reconnects — so it
// is an ordinary renewable device-bound session. A Hosted viewer's session is
// issued under the server's current verified Hosted policy, exactly like any
// other Hosted session; a viewer that policy no longer admits gets none.
func (s *Store) castSessionTx(ctx context.Context, tx *sql.Tx, id castIdentity, device CastDevice) (CastSession, string, error) {
	claimCtx, e := identity.WithIssuingDevice(ctx, identity.DeviceRegistration{
		InstallationID: "cast-" + strings.TrimPrefix(device.ID, "cdv_"),
		Name:           device.DisplayName,
		Platform:       "google-cast",
		App:            "cast",
	}, "")
	if e != nil {
		return CastSession{}, "", e
	}
	horizon, e := s.castHorizonTx(ctx, tx, id)
	if e != nil {
		return CastSession{}, "", e
	}
	envelope, e := s.Identity.IssueTx(claimCtx, tx, id.account, id.profile, id.authority, id.role, id.epoch, horizon)
	if errors.Is(e, identity.ErrUnauthorized) {
		return CastSession{}, "", errCastAccessEnded
	}
	if e != nil {
		return CastSession{}, "", e
	}
	return CastSession{AccessToken: envelope.AccessToken, ExpiresAt: envelope.ExpiresAt}, envelope.SessionFamilyID, nil
}

var errCastAccessEnded = &Fault{Code: "cast_access_ended", Status: 403, Message: "This account can no longer play on this server. Pair again from a signed-in device."}

// castHorizonTx is the verified Hosted policy horizon for a Hosted viewer, and
// zero for a local one.
func (s *Store) castHorizonTx(ctx context.Context, tx *sql.Tx, id castIdentity) (time.Time, error) {
	switch id.authority {
	case "local":
		return time.Time{}, nil
	case "hosted":
		if s.HostedHorizon == nil {
			return time.Time{}, errCastHostedUnavailable
		}
		principal := identity.Principal{Viewer: identity.Viewer{AccountID: id.account, ProfileID: id.profile, ServerID: s.Identity.ID(), Authority: id.authority, Role: id.role}, Epoch: id.epoch}
		horizon, e := s.HostedHorizon(ctx, tx, principal)
		if errors.Is(e, identity.ErrUnauthorized) {
			return time.Time{}, errCastAccessEnded
		}
		return horizon, e
	default:
		return time.Time{}, errCastAccessEnded
	}
}

var errCastHostedUnavailable = &Fault{Code: "cast_unavailable", Status: 503, Message: "Cast isn't available for Portico Accounts on this server right now.", Retryable: true}

// CastReconnect renews a paired receiver after a restart. The device token is
// rotated on every call, so a captured token is useful for one renewal and the
// legitimate receiver detects the theft at its next reconnect.
//
// A reconnect whose answer was lost leaves the receiver holding the token it
// replaced. That token reconnects the same device (its deviceId must match)
// until CastRecoveryWindow after the rotation or until the new token is used,
// whichever is first; the session the lost answer carried is retired.
func (s *Store) CastReconnect(ctx context.Context, req CastReconnectRequest) (CastReceiverSession, error) {
	var out CastReceiverSession
	if req.ProtocolVersion != Protocol || len(req.DeviceToken) < 43 || len(req.DeviceToken) > 256 || req.DeviceID != "" && !ValidID(req.DeviceID) {
		return out, errInvalid
	}
	if s.Identity == nil {
		return out, &Fault{Code: "cast_unavailable", Status: 503, Message: "Cast pairing is not available.", Retryable: true}
	}
	var expiredDevice string
	out, e := s.castAttempt(ctx, req.Source, func(tx *sql.Tx) (CastReceiverSession, error) {
		const columns = `id,authority,account_id,profile_id,role,account_epoch,device_id,display_name,capabilities,state,receiver_id,bootstrap_id,expires_at_ms,previous_token_digest,previous_valid_until_ms`
		var rowID, authority, account, profile, role, deviceID, name, capabilities, state, receiverID, bootstrapID, previous string
		var epoch int
		var expires, previousUntil int64
		scan := func(row *sql.Row) error {
			return row.Scan(&rowID, &authority, &account, &profile, &role, &epoch, &deviceID, &name, &capabilities, &state, &receiverID, &bootstrapID, &expires, &previous, &previousUntil)
		}
		now := s.ms()
		presented := digest(req.DeviceToken)
		recovering := false
		e := scan(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM social_cast_devices WHERE token_digest=?`, presented))
		if errors.Is(e, sql.ErrNoRows) && req.DeviceID != "" {
			recovering = true
			e = scan(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM social_cast_devices WHERE previous_token_digest=? AND previous_valid_until_ms>? AND device_id=?`, presented, now, req.DeviceID))
		}
		if errors.Is(e, sql.ErrNoRows) {
			return out, credentialFault("cast_device_not_found", 404, "That receiver is not paired.")
		}
		if e != nil {
			return out, e
		}
		switch {
		case state == "revoked" || state == "stopped":
			return out, credentialFault("cast_device_revoked", 403, "That receiver pairing was revoked.")
		case state != "active" || now >= expires:
			expiredDevice = rowID
			return out, credentialFault("cast_device_expired", 410, "That receiver pairing has expired; pair again.")
		}
		rotation := castRotation{bootstrapID: bootstrapID, receiverID: receiverID, name: name, capabilities: capabilities, previous: presented, previousUntil: now + CastRecoveryWindow.Milliseconds()}
		if recovering {
			rotation.previousUntil = previousUntil
		}
		result, e := s.issueCast(ctx, tx, castIdentity{authority, account, profile, role, epoch}, deviceID, rotation, "rotation")
		if e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE social_receivers SET last_seen_ms=? WHERE id=?`, now, receiverID); e != nil {
			return out, e
		}
		return result, nil
	})
	if expiredDevice != "" {
		// Recorded after the refusal so the next attempt reads the same answer.
		_, _ = dbwork.ExecWrite(context.WithoutCancel(ctx), s.DB, dbwork.ClassInteractive, `UPDATE social_cast_devices SET state='expired' WHERE id=? AND state='active'`, expiredDevice)
	}
	return out, e
}

// Failed Cast credential attempts (unknown, used, cancelled or expired codes and
// device tokens) are budgeted per source network and server-wide. The charge is
// committed in its own transaction, so the refusal's rollback cannot undo it and
// a restart does not reset it. Successful attempts are not charged: a household
// of receivers reconnecting after every restart never exhausts its own budget.
// The server-wide ceiling is wide enough that one network cannot lock every TV
// out, and narrow enough that guessing a live six-character code (31^6) within
// its five-minute life stays out of reach.
const (
	CastFailuresPerSource = 30
	CastFailuresPerServer = 1000
)

// castAttempt runs one redeem or reconnect in a transaction after checking both
// failure budgets, and charges them when attempt refuses the credential.
func (s *Store) castAttempt(ctx context.Context, source string, attempt func(*sql.Tx) (CastReceiverSession, error)) (CastReceiverSession, error) {
	var out CastReceiverSession
	buckets := []rateBucket{{"cast-fail:" + digest(source), CastFailuresPerSource}, {"cast-fail", CastFailuresPerServer}}
	gated, e := s.begin(ctx)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	for _, b := range buckets {
		if e = s.rateFull(ctx, tx, b.name, b.max, time.Hour); e != nil {
			return out, e
		}
	}
	out, e = attempt(tx)
	var f *Fault
	if errors.As(e, &f) && f.credential {
		gated.Rollback()
		if charge := s.chargeFailure(ctx, buckets); charge != nil {
			return out, charge
		}
		return out, e
	}
	if e != nil {
		return out, e
	}
	return out, gated.Commit()
}

// RevokeCastDevice unpairs a receiver and ends the session it plays with. The
// next reconnect fails closed.
func (s *Store) RevokeCastDevice(ctx context.Context, p identity.Principal, id string) error {
	gated, e := s.begin(ctx)
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var family string
	e = tx.QueryRowContext(ctx, `SELECT session_family_id FROM social_cast_devices WHERE id=? AND authority=? AND account_id=? AND profile_id=?`, id, p.Authority, p.AccountID, p.ProfileID).Scan(&family)
	if errors.Is(e, sql.ErrNoRows) {
		return errNotFound
	}
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE social_cast_devices SET state='revoked',previous_token_digest='',previous_valid_until_ms=0,session_family_id='' WHERE id=?`, id); e != nil {
		return e
	}
	if family != "" && s.Identity != nil {
		if e = s.Identity.RetireFamilyTx(ctx, tx, p.Authority, p.AccountID, p.ProfileID, family); e != nil {
			return e
		}
	}
	return gated.Commit()
}
