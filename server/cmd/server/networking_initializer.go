package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/authoritygate"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/httpapi"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/operations"
	"strconv"
)

func syncCurrentDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func openCurrentAuthority(state string) (*networking.AuthorityRunner, error) {
	directory := filepath.Join(state, "supervisor")
	if e := os.MkdirAll(directory, 0700); e != nil {
		return nil, e
	}
	fail := func(e error) (*networking.AuthorityRunner, error) { return nil, e }
	path := filepath.Join(directory, "authority-incarnation.v1")
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		if _, dbErr := os.Lstat(filepath.Join(state, "server.sqlite")); !errors.Is(dbErr, os.ErrNotExist) {
			return fail(errors.New("existing server state has no current authority incarnation"))
		}
		random := make([]byte, 32)
		if _, e = rand.Read(random); e != nil {
			return fail(e)
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return fail(e)
		}
		_, e = f.WriteString(hex.EncodeToString(random))
		if e == nil {
			e = f.Sync()
		}
		closed := f.Close()
		if e != nil {
			return fail(e)
		}
		if closed != nil {
			return fail(closed)
		}
		if e = syncCurrentDirectory(directory); e != nil {
			return fail(e)
		}
		if e = syncCurrentDirectory(state); e != nil {
			return fail(e)
		}
		info, e = os.Lstat(path)
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() != 64 {
		return fail(errors.New("current authority incarnation is unavailable"))
	}
	f, e := os.Open(path)
	if e != nil {
		return fail(e)
	}
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) {
		f.Close()
		return fail(errors.New("authority incarnation changed"))
	}
	raw, e := io.ReadAll(io.LimitReader(f, 65))
	f.Close()
	if e != nil {
		return fail(e)
	}
	gate, e := authoritygate.NewAuthorityGate(string(raw), false)
	if e != nil {
		return fail(e)
	}
	runner, e := networking.NewAuthorityRunner(func(ctx context.Context) (networking.LifecycleLease, error) { return gate.Acquire(ctx) })
	if e != nil {
		return fail(e)
	}
	return runner, nil
}
func initializeCurrentClaimControl(db *sql.DB, state string, id *identity.Service, runner *networking.AuthorityRunner, keys *networking.ProtectedKeys, control *hosted.Service, origins []string) (*networking.ClaimHandler, error) {
	if !control.Configured() {
		return nil, nil
	}
	origin, root, rootID := control.TrustConfig()
	pin, e := base64.RawURLEncoding.Strict().DecodeString(root)
	if e != nil {
		return nil, e
	}
	verifier, e := networking.NewApprovalVerifier(origin, rootID, pin, control.ValidateTrust)
	if e != nil {
		return nil, e
	}
	var cipher *networking.ClaimCipher
	e = runner.Do(context.Background(), func(ctx context.Context) error {
		var e error
		cipher, e = networking.OpenClaimCipher(ctx, db, state)
		return e
	})
	if e != nil {
		return nil, e
	}
	store, e := networking.NewSQLiteStore(db, cipher, networking.GuardLocalOwner, networking.GuardInstalledClaim, verifier, origin)
	if e != nil {
		return nil, e
	}
	if e = store.SetTerminalRevoker(control.RevokeClaimTx); e != nil {
		return nil, e
	}
	transport, e := networking.NewHTTPTransport(origin, store)
	if e != nil {
		return nil, e
	}
	if e = control.UseCurrentClaims(store, runner, transport); e != nil {
		return nil, e
	}
	if e = control.ConfigureClaimCleanup(keys); e != nil {
		return nil, e
	}
	handler, e := networking.NewClaimHandler(store, runner, transport, keys, httpapi.NetworkingClaimAuthorizer(id, origins), id.Name)
	if e != nil {
		return nil, e
	}
	if e = handler.ConfigureCertificates(context.Background(), state, networking.CertificateOptions{
		Environment:      os.Getenv("PORTICO_CERTIFICATE_ENVIRONMENT"),
		StagingRootsFile: os.Getenv("PORTICO_CERTIFICATE_STAGING_ROOTS"),
	}); e != nil {
		return nil, e
	}
	return handler, nil
}

// accessURLsForPublication projects the console settings onto the published
// manual routes: the access URL list plus, when a custom certificate domain is
// configured, its HTTPS origin on the remote public port (no :port at 443).
// The result is always non-nil, so an empty list clears the published routes
// while a settings read error (nil from accessURLsProvider) keeps them.
func accessURLsForPublication(v operations.Settings, publicPort int) []string {
	out := make([]string, 0, len(v.AccessURLs)+1)
	out = append(out, v.AccessURLs...)
	if v.CustomCertificateDomain != "" {
		u := "https://" + v.CustomCertificateDomain
		if publicPort != 443 {
			u += ":" + strconv.Itoa(publicPort)
		}
		out = append(out, u)
	}
	return out
}

// accessURLsProvider reads the console settings for the remote manager's
// access-URL seam. A settings read error returns nil so the manager keeps its
// last good list instead of clearing the published routes.
func accessURLsProvider(console *operations.Store, manager *networking.RemoteManager) func(context.Context) []string {
	return func(ctx context.Context) []string {
		document, err := console.Settings(ctx, operations.AllowServerScope)
		if err != nil {
			return nil
		}
		return accessURLsForPublication(document.Effective, manager.PublicPort(ctx))
	}
}

// Identity metadata is published by the same transaction as the native family.
// This is a read of root's durable identity, never a second signing authority.
func wireNativeServerIdentity(id *identity.Service) {
	id.SetupClaimReadyTx = networking.SetupClaimInstalledTx
	id.NativeIdentityTx = func(ctx context.Context, tx *sql.Tx) (identity.NativeServerIdentity, error) {
		current, err := networking.CurrentIdentityTx(ctx, tx)
		if err != nil {
			return identity.NativeServerIdentity{}, err
		}
		if current.ServerID != id.ID() {
			return identity.NativeServerIdentity{}, networking.ErrStale
		}
		digest := sha256.Sum256(current.PublicKey)
		return identity.NativeServerIdentity{PublicKey: base64.RawURLEncoding.EncodeToString(current.PublicKey), Fingerprint: base64.RawURLEncoding.EncodeToString(digest[:])}, nil
	}
}
