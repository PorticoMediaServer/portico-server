package appstore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"
)

// The notification intake has no other authentication, so every one of these
// paths is the difference between accepting a stranger's payload and refusing it.

type authority struct {
	rootKey    *ecdsa.PrivateKey
	rootCert   *x509.Certificate
	leafKey    *ecdsa.PrivateKey
	leafCert   *x509.Certificate
	rootPEM    []byte
	leafBase64 string
}

func newAuthority(t *testing.T, commonName string) authority {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName + " Root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: commonName + " Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	return authority{
		rootKey:    rootKey,
		rootCert:   rootCert,
		leafKey:    leafKey,
		leafCert:   leafCert,
		rootPEM:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}),
		leafBase64: base64.StdEncoding.EncodeToString(leafDER),
	}
}

// sign builds a compact JWS the way Apple does, with an overridable header so a
// test can express "the same payload, signed with a header a client chose".
func (a authority) sign(t *testing.T, header map[string]any, payload any) string {
	t.Helper()
	if header == nil {
		header = map[string]any{"alg": "ES256", "x5c": []string{a.leafBase64}}
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, a.leafKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func notification(uuid string) map[string]any {
	return map[string]any{
		"notificationUUID": uuid,
		"notificationType": "DID_RENEW",
		"version":          "2.0",
		"signedDate":       time.Now().UnixMilli(),
		"data":             map[string]any{"bundleId": "tv.getportico.app", "environment": "Production"},
	}
}

func keySet(t *testing.T, a authority) KeySet {
	t.Helper()
	set, err := LoadKeySet(a.rootPEM, "tv.getportico.app", "Production")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestLoadKeySetRefusesUnusableConfiguration(t *testing.T) {
	trusted := newAuthority(t, "Portico Test")
	for name, run := range map[string]func() error{
		"no certificate": func() error {
			_, e := LoadKeySet([]byte("not a certificate"), "tv.getportico.app", "Production")
			return e
		},
		"empty bundle":        func() error { _, e := LoadKeySet(trusted.rootPEM, "  ", "Production"); return e },
		"unknown environment": func() error { _, e := LoadKeySet(trusted.rootPEM, "tv.getportico.app", "Staging"); return e },
	} {
		if err := run(); err == nil {
			t.Errorf("%s: configuration was accepted; an intake that cannot verify must not exist", name)
		}
	}
}

func TestVerifyAcceptsAPinnedSignature(t *testing.T) {
	trusted := newAuthority(t, "Portico Test")
	set := keySet(t, trusted)
	out, err := set.Verify(trusted.sign(t, nil, notification("uuid-1")))
	if err != nil {
		t.Fatal(err)
	}
	if out.UUID != "uuid-1" || out.NotificationType != "DID_RENEW" {
		t.Fatalf("decoded %+v", out)
	}
	if out.SignedAt().IsZero() {
		t.Fatal("signedDate did not decode")
	}
}

func TestVerifyRefusesEveryUntrustedShape(t *testing.T) {
	trusted := newAuthority(t, "Portico Test")
	stranger := newAuthority(t, "Not Apple")
	set := keySet(t, trusted)
	valid := trusted.sign(t, nil, notification("uuid-1"))

	// A chain that does not reach the pinned root. This is the case that matters
	// most: the signature itself is perfectly valid, just not from Apple.
	if _, err := set.Verify(stranger.sign(t, nil, notification("uuid-2"))); !errors.Is(err, ErrChain) {
		t.Errorf("foreign chain: want ErrChain, got %v", err)
	}
	// "alg":"none" with the signature stripped: the classic JWS bypass.
	unsigned := trusted.sign(t, map[string]any{"alg": "none", "x5c": []string{trusted.leafBase64}}, notification("uuid-3"))
	parts := strings.Split(unsigned, ".")
	if _, err := set.Verify(parts[0] + "." + parts[1] + "."); !errors.Is(err, ErrAlgorithm) {
		t.Errorf("alg none: want ErrAlgorithm, got %v", err)
	}
	// An algorithm we do not implement must be compared and refused, not guessed at.
	if _, err := set.Verify(trusted.sign(t, map[string]any{"alg": "RS256", "x5c": []string{trusted.leafBase64}}, notification("uuid-4"))); !errors.Is(err, ErrAlgorithm) {
		t.Errorf("alg RS256: want ErrAlgorithm, got %v", err)
	}
	// No chain at all.
	if _, err := set.Verify(trusted.sign(t, map[string]any{"alg": "ES256"}, notification("uuid-5"))); !errors.Is(err, ErrChain) {
		t.Errorf("missing x5c: want ErrChain, got %v", err)
	}
	// A payload swapped under a real signature.
	swapped := trusted.sign(t, nil, notification("uuid-6"))
	other, _ := json.Marshal(notification("uuid-7"))
	swappedParts := strings.Split(swapped, ".")
	tampered := swappedParts[0] + "." + base64.RawURLEncoding.EncodeToString(other) + "." + swappedParts[2]
	if _, err := set.Verify(tampered); !errors.Is(err, ErrSignature) {
		t.Errorf("tampered payload: want ErrSignature, got %v", err)
	}
	// Framing.
	for name, payload := range map[string]string{
		"empty":      "",
		"two parts":  swappedParts[0] + "." + swappedParts[1],
		"not base64": "!!!.!!!.!!!",
		"oversize":   strings.Repeat("a", MaxPayloadBytes+1),
	} {
		if _, err := set.Verify(payload); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: want ErrMalformed, got %v", name, err)
		}
	}
	// An unconfigured key set must refuse a payload that would otherwise verify.
	if _, err := (KeySet{BundleID: "tv.getportico.app", Environment: "Production"}).Verify(valid); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("unconfigured: want ErrNotConfigured, got %v", err)
	}
}

func TestVerifyRefusesAnotherApplicationOrEnvironment(t *testing.T) {
	trusted := newAuthority(t, "Portico Test")
	set := keySet(t, trusted)
	other := notification("uuid-8")
	other["data"] = map[string]any{"bundleId": "com.example.other", "environment": "Production"}
	if _, err := set.Verify(trusted.sign(t, nil, other)); !errors.Is(err, ErrAudience) {
		t.Errorf("foreign bundle: want ErrAudience, got %v", err)
	}
	sandbox := notification("uuid-9")
	sandbox["data"] = map[string]any{"bundleId": "tv.getportico.app", "environment": "Sandbox"}
	if _, err := set.Verify(trusted.sign(t, nil, sandbox)); !errors.Is(err, ErrAudience) {
		t.Errorf("sandbox into production: want ErrAudience, got %v", err)
	}
}

func TestVerifyRefusesAnExpiredChain(t *testing.T) {
	trusted := newAuthority(t, "Portico Test")
	set := keySet(t, trusted)
	set.Now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	if _, err := set.Verify(trusted.sign(t, nil, notification("uuid-10"))); !errors.Is(err, ErrChain) {
		t.Errorf("expired chain: want ErrChain, got %v", err)
	}
}

func TestTransactionIsVerifiedAgainstTheSamePinnedChain(t *testing.T) {
	trusted := newAuthority(t, "Portico Test")
	stranger := newAuthority(t, "Not Apple")
	set := keySet(t, trusted)

	// Absent inner transaction: not an error, several notification types have none.
	out, err := set.Verify(trusted.sign(t, nil, notification("uuid-11")))
	if err != nil {
		t.Fatal(err)
	}
	if _, present, err := set.Transaction(out); err != nil || present {
		t.Fatalf("absent transaction: present=%v err=%v", present, err)
	}

	inner := trusted.sign(t, nil, map[string]any{"originalTransactionId": "2000000", "appAccountToken": "acct_1", "bundleId": "tv.getportico.app"})
	withInner := notification("uuid-12")
	withInner["data"] = map[string]any{"bundleId": "tv.getportico.app", "environment": "Production", "signedTransactionInfo": inner}
	decoded, err := set.Verify(trusted.sign(t, nil, withInner))
	if err != nil {
		t.Fatal(err)
	}
	claims, present, err := set.Transaction(decoded)
	if err != nil || !present || claims.OriginalTransactionID != "2000000" || claims.AppAccountToken != "acct_1" {
		t.Fatalf("claims=%+v present=%v err=%v", claims, present, err)
	}

	// An inner transaction signed by a stranger must not be read even when the
	// outer notification verified: the inner blob is where the money claims are.
	forged := notification("uuid-13")
	forged["data"] = map[string]any{
		"bundleId":              "tv.getportico.app",
		"environment":           "Production",
		"signedTransactionInfo": stranger.sign(t, nil, map[string]any{"originalTransactionId": "9", "appAccountToken": "victim", "bundleId": "tv.getportico.app"}),
	}
	outer, err := set.Verify(trusted.sign(t, nil, forged))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = set.Transaction(outer); !errors.Is(err, ErrChain) {
		t.Errorf("forged inner transaction: want ErrChain, got %v", err)
	}
}

func TestFailureReasonNeverEchoesThePayload(t *testing.T) {
	for _, err := range []error{ErrNotConfigured, ErrMalformed, ErrAlgorithm, ErrChain, ErrSignature, ErrAudience, errors.New("something else")} {
		reason := FailureReason(err)
		if reason == "" || strings.ContainsAny(reason, " .") {
			t.Errorf("%v: reason %q is not a short stable token", err, reason)
		}
	}
	if FailureReason(errors.New("boom")) != "unknown" {
		t.Error("an unmapped error must not leak its message as a reason")
	}
}
