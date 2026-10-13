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
	if q.InstallationID == "" {
		// Non-HTTP issuers (notably Cast pairing and older internal tests) still
		// get an individually revocable device record. First-party HTTP routes
		// supply a stable installation ID before they enter the service.
		q = DeviceRegistration{InstallationID: Token(), Name: "Portico device", Platform: "unknown", App: "portico"}
	}
	if !q.valid() {
		return Device{}, ErrDeviceInput
	}
	caller := deviceCaller{authority: authority, account: account}
	device, err := s.registerDeviceTx(ctx, tx, caller, q, claim.peer)
	if err != nil {
		return device, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE identity_devices SET last_seen=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), device.ID); err != nil {
		return Device{}, err
	}
	return device, nil
}
