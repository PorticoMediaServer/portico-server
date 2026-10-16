package identity

import (
	"context"
	"database/sql"
	"time"
)

type issuingDeviceKey struct{}

// vouchedDeviceKey marks an issuance for a device that a signed-in, approved
// device has vouched for (a Cast target it paired). Such a device does not
// wait for the owner's approval; what it is issued is narrow by construction.
type vouchedDeviceKey struct{}

// WithVouchedDevice marks ctx so the device registered by the next issuance
// is approved on the word of the session that paired it.
func WithVouchedDevice(ctx context.Context) context.Context {
	return context.WithValue(ctx, vouchedDeviceKey{}, true)
}

type issuingDevice struct {
	registration DeviceRegistration
	peer         string
	deviceID     string
}

// WithIssuingDevice carries a client's installation claim to the one
// transaction that checks approval and creates its authorization family.
// The installation ID is a random client secret, never an IP fingerprint.
func WithIssuingDevice(ctx context.Context, registration DeviceRegistration, peer string) (context.Context, error) {
	if !registration.valid() {
		return ctx, ErrDeviceInput
	}
	return context.WithValue(ctx, issuingDeviceKey{}, issuingDevice{registration: registration, peer: peer}), nil
}

// withExistingDevice carries a previously proved account session's device into
// profile selection. The request cannot substitute another installation ID.
func withExistingDevice(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, issuingDeviceKey{}, issuingDevice{deviceID: id})
}

func (s *Service) issuingDeviceTx(ctx context.Context, tx *sql.Tx, authority, account string) (Device, error) {
	claim, _ := ctx.Value(issuingDeviceKey{}).(issuingDevice)
	if claim.deviceID != "" {
		device, err := s.deviceTx(ctx, tx, authority, account, "", claim.deviceID)
		if err != nil {
			return Device{}, err
		}
		if device.ApprovalState != "approved" {
			return Device{}, ErrDevicePending
		}
		return device, nil
	}
	q := claim.registration
	caller := deviceCaller{authority: authority, account: account}
	if q.InstallationID == "" {
		// A sign-in that names no installation (another client, a script; Cast
		// pairing and internal callers) still gets an individually revocable
		// device record, under an installation id the server makes and
		// returns. Each such sign-in makes a new one, so the ones left waiting
		// for approval are bounded per account: only the newest few are kept.
		q = DeviceRegistration{InstallationID: unnamedInstallationPrefix + Token(), Name: "Portico device", Platform: "unknown", App: "portico"}
		if err := pruneUnnamedWaitingTx(ctx, tx, caller); err != nil {
			return Device{}, err
		}
	}
	if !q.valid() {
		return Device{}, ErrDeviceInput
	}
	device, err := s.registerDeviceTx(ctx, tx, caller, q, claim.peer)
	if err != nil {
		return device, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE identity_devices SET last_seen=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), device.ID); err != nil {
		return Device{}, err
	}
	return device, nil
}

// unnamedInstallationPrefix marks an installation id the server made for a
// sign-in that did not name one.
const unnamedInstallationPrefix = "unnamed-"

// unnamedWaitingKept is how many waiting records of server-named
// installations an account keeps; older ones go before a new one is made.
const unnamedWaitingKept = 4

// pruneUnnamedWaitingTx removes an account's oldest waiting records of
// server-named installations beyond the newest few. A waiting device has no
// sign-in yet (no session family or refresh credential points at it), so
// removing one ends nothing; it only stops the list growing.
func pruneUnnamedWaitingTx(ctx context.Context, tx *sql.Tx, c deviceCaller) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM identity_devices WHERE id IN (SELECT d.id FROM identity_devices d WHERE d.authority=? AND d.account_id=? AND d.approval_state='pending' AND substr(d.installation_id,1,?)=?
 AND NOT EXISTS(SELECT 1 FROM identity_device_families f WHERE f.device_id=d.id) AND NOT EXISTS(SELECT 1 FROM identity_refresh_credentials r WHERE r.device_id=d.id)
 ORDER BY d.first_seen DESC,d.id DESC LIMIT -1 OFFSET ?)`, c.authority, c.account, len(unnamedInstallationPrefix), unnamedInstallationPrefix, unnamedWaitingKept-1)
	return err
}
