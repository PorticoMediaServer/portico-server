package app

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHostedDocumentKeySetLifecycleFencesRollbackAndRevocation(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	oldPrivate := ed25519.NewKeyFromSeed(bytesForKeyLifecycleTest(0x21))
	newPrivate := ed25519.NewKeyFromSeed(bytesForKeyLifecycleTest(0x22))
	oldPublic := base64.StdEncoding.EncodeToString(oldPrivate.Public().(ed25519.PublicKey))
	newPublic := base64.StdEncoding.EncodeToString(newPrivate.Public().(ed25519.PublicKey))
	keySet := hostedDocumentSigningKeySet{
		SchemaVersion: 1, Generation: 4, IssuedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano), ActiveKeyID: "key-new",
		Keys: []hostedDocumentSigningPublicKey{
			{KeyID: "key-old", Algorithm: hostedSignatureAlgorithm, PublicKeyB64: oldPublic, State: "verification", ValidFrom: now.Add(-time.Hour).Format(time.RFC3339Nano), ValidUntil: now.Add(24 * time.Hour).Format(time.RFC3339Nano)},
			{KeyID: "key-new", Algorithm: hostedSignatureAlgorithm, PublicKeyB64: newPublic, State: "active", ValidFrom: now.Add(-time.Minute).Format(time.RFC3339Nano), ValidUntil: now.Add(24 * time.Hour).Format(time.RFC3339Nano)},
		},
	}
	fingerprint, err := hostedDocumentKeySetFingerprint(keySet)
	if err != nil {
		t.Fatal(err)
	}
	keySet.Fingerprint = fingerprint
	validated, state, err := validateHostedDocumentKeySet(keySet, "key-old", now, hostedDocumentSigningKeySetState{})
	if err != nil || validated["key-old"] == "" || state.Generation != 4 {
		t.Fatalf("valid staged keyset rejected: validated=%v state=%+v err=%v", validated, state, err)
	}

	rollback := keySet
	rollback.Generation = 3
	rollback.Fingerprint, _ = hostedDocumentKeySetFingerprint(rollback)
	if _, _, err := validateHostedDocumentKeySet(rollback, "key-old", now, state); err == nil {
		t.Fatal("keyset generation rollback was accepted")
	}

	equivocation := keySet
	equivocation.Keys = append([]hostedDocumentSigningPublicKey(nil), keySet.Keys...)
	equivocation.Keys[1].PublicKeyB64 = oldPublic
	equivocation.Fingerprint, _ = hostedDocumentKeySetFingerprint(equivocation)
	if _, _, err := validateHostedDocumentKeySet(equivocation, "key-old", now, state); err == nil {
		t.Fatal("same-generation keyset equivocation was accepted")
	}

	revoked := keySet
	revoked.Generation = 5
	revoked.Keys = append([]hostedDocumentSigningPublicKey(nil), keySet.Keys...)
	revoked.Keys[0].State = "revoked"
	revoked.Keys[0].RevokedAt = now.Add(-time.Second).Format(time.RFC3339Nano)
	revoked.Fingerprint, _ = hostedDocumentKeySetFingerprint(revoked)
	validated, _, err = validateHostedDocumentKeySet(revoked, "key-old", now, state)
	if err != nil || validated["key-old"] != "" {
		t.Fatalf("revoked key remained trusted: validated=%v err=%v", validated, err)
	}
}

func bytesForKeyLifecycleTest(value byte) []byte {
	seed := sha256.Sum256([]byte{value})
	return seed[:]
}

func TestHostedDocumentKeyBootstrapFetchesLifecycleAndFencesRevocationAcrossRestartOutage(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Now().UTC().Truncate(time.Second)
	oldPrivate := ed25519.NewKeyFromSeed(bytesForKeyLifecycleTest(0x31))
	newPrivate := ed25519.NewKeyFromSeed(bytesForKeyLifecycleTest(0x32))
	oldPublic := base64.StdEncoding.EncodeToString(oldPrivate.Public().(ed25519.PublicKey))
	newPublic := base64.StdEncoding.EncodeToString(newPrivate.Public().(ed25519.PublicKey))
	keySet := func(generation int64, oldState, oldRevokedAt string) hostedDocumentSigningKeySet {
		result := hostedDocumentSigningKeySet{
			SchemaVersion: 1,
			Generation:    generation,
			IssuedAt:      now.Add(-25 * time.Hour).Format(time.RFC3339Nano),
			ExpiresAt:     now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			ActiveKeyID:   "key-new",
			Keys: []hostedDocumentSigningPublicKey{
				{KeyID: "key-old", Algorithm: hostedSignatureAlgorithm, PublicKeyB64: oldPublic, State: oldState, RevokedAt: oldRevokedAt},
				{KeyID: "key-new", Algorithm: hostedSignatureAlgorithm, PublicKeyB64: newPublic, State: "active"},
			},
		}
		result.Fingerprint, _ = hostedDocumentKeySetFingerprint(result)
		return result
	}
	current := keySet(1, "verification", "")
	outage := false
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/signing-keys" {
			http.NotFound(w, r)
			return
		}
		if outage {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(current)
	}))
	t.Cleanup(hosted.Close)
	server.cfg.HostedDocumentPublicKeys = map[string]string{"key-old": oldPublic}
	if err := server.saveRemoteAccessSettings(RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "key-lifecycle", PublicPortMode: "disabled", PreferredRemoteAuthMode: "portico"}); err != nil {
		t.Fatalf("save Hosted authority settings: %v", err)
	}
	if err := server.ensureHostedDocumentKey(context.Background(), hosted.URL, "key-old"); err != nil {
		t.Fatalf("initial bootstrap lifecycle fetch: %v", err)
	}
	if state := server.loadHostedDocumentKeySetState(); state.Generation != 1 {
		t.Fatalf("initial lifecycle fetch did not persist generation: %+v", state)
	}

	current = keySet(2, "revoked", now.Add(-time.Hour).Format(time.RFC3339Nano))
	if err := server.ensureHostedDocumentKey(context.Background(), hosted.URL, "key-old"); err == nil {
		t.Fatal("higher-generation revocation was accepted for the old key")
	}
	if trusted := server.trustedHostedDocumentKey("key-old"); trusted != "" {
		t.Fatal("revoked key remained trusted after lifecycle refresh")
	}

	restarted := NewInertServer(server.cfg, db, server.log)
	outage = true
	if err := restarted.ensureHostedDocumentKey(context.Background(), hosted.URL, "key-old"); err == nil {
		t.Fatal("restart/outage accepted a revoked configured bootstrap key")
	}
	if trusted := restarted.trustedHostedDocumentKey("key-old"); trusted != "" {
		t.Fatal("restart reintroduced the revoked configured key")
	}
}
