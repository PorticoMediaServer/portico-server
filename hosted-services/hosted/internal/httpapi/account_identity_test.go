package httpapi

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
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/appstore"
)

// The notification intake is the only unauthenticated write path in the service,
// so these tests are about what it refuses and what it answers, not about storage.

type signer struct {
	key        *ecdsa.PrivateKey
	rootPEM    []byte
	leafBase64 string
}

func newSigner(t *testing.T) signer {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, template, template, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Test Signer"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer{key: leafKey, rootPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), leafBase64: base64.StdEncoding.EncodeToString(leafDER)}
}

func (s signer) jws(t *testing.T, payload any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": []string{s.leafBase64}})
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(input))
	r, sig, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	sig.FillBytes(raw[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(raw)
}

func post(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/v1/appstore/notifications", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, r)
	return w
}

func receipt(t *testing.T, w *httptest.ResponseRecorder) (bool, string) {
	t.Helper()
	var out struct {
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return out.Accepted, out.Reason
}

func TestNotificationIntakeIsAbsentWithoutAKeySet(t *testing.T) {
	// An intake that cannot verify must not exist. A 404 here, rather than a route
	// that accepts and ignores, is what keeps an unconfigured deployment from
	// silently swallowing store events.
	w := post(t, &Handler{}, `{"signedPayload":"x"}`)
	if w.Code != 404 {
		t.Fatalf("status %d; an unconfigured intake must not be registered", w.Code)
	}
}

func TestNotificationIntakeRefusesWithoutRetryAndNeverEchoesThePayload(t *testing.T) {
	s := newSigner(t)
	set, err := appstore.LoadKeySet(s.rootPEM, "tv.getportico.app", "Production")
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{AppStore: &set}
	stranger := newSigner(t)

	for name, body := range map[string]string{
		"not json":           `nonsense`,
		"no signed payload":  `{"signedPayload":""}`,
		"not a jws":          `{"signedPayload":"aaa"}`,
		"foreign signature":  `{"signedPayload":"` + stranger.jws(t, map[string]any{"notificationUUID": "u", "notificationType": "DID_RENEW", "data": map[string]any{"bundleId": "tv.getportico.app", "environment": "Production"}}) + `"}`,
		"another bundle":     `{"signedPayload":"` + s.jws(t, map[string]any{"notificationUUID": "u", "notificationType": "DID_RENEW", "data": map[string]any{"bundleId": "com.example.other", "environment": "Production"}}) + `"}`,
		"sandbox in prod":    `{"signedPayload":"` + s.jws(t, map[string]any{"notificationUUID": "u", "notificationType": "DID_RENEW", "data": map[string]any{"bundleId": "tv.getportico.app", "environment": "Sandbox"}}) + `"}`,
		"no notification id": `{"signedPayload":"` + s.jws(t, map[string]any{"notificationType": "DID_RENEW"}) + `"}`,
	} {
		w := post(t, h, body)
		// Apple retries a non-2xx for days. A payload this deployment will never
		// accept is not made acceptable by retrying, so a decided refusal is 200.
		if w.Code != 200 {
			t.Errorf("%s: status %d, want 200 with accepted=false", name, w.Code)
			continue
		}
		accepted, reason := receipt(t, w)
		if accepted {
			t.Errorf("%s: accepted", name)
		}
		if reason == "" {
			t.Errorf("%s: no reason given", name)
		}
		// The reason is a short token. It must never carry any part of the payload.
		if strings.Contains(w.Body.String(), "signedPayload") || strings.Contains(w.Body.String(), "eyJ") {
			t.Errorf("%s: the response echoed the refused payload: %s", name, w.Body.String())
		}
	}
}

func TestNotificationIntakeAsksForARetryWhenItCannotStore(t *testing.T) {
	s := newSigner(t)
	set, err := appstore.LoadKeySet(s.rootPEM, "tv.getportico.app", "Production")
	if err != nil {
		t.Fatal(err)
	}
	// Verification succeeds but there is nowhere to record the event. That is the
	// one case Apple should retry: the deployment is wrong, not the payload.
	body := `{"signedPayload":"` + s.jws(t, map[string]any{"notificationUUID": "u1", "notificationType": "DID_RENEW", "signedDate": time.Now().UnixMilli(), "data": map[string]any{"bundleId": "tv.getportico.app", "environment": "Production"}}) + `"}`
	w := post(t, &Handler{AppStore: &set}, body)
	if w.Code != 503 {
		t.Fatalf("status %d, want 503 so Apple retries", w.Code)
	}
	var out struct {
		Error struct {
			Retryable bool `json:"retryable"`
		} `json:"error"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Error.Retryable {
		t.Fatal("a storage failure was not reported as retryable")
	}
}

func TestNotificationIntakeRejectsAnOversizeBody(t *testing.T) {
	s := newSigner(t)
	set, err := appstore.LoadKeySet(s.rootPEM, "tv.getportico.app", "Production")
	if err != nil {
		t.Fatal(err)
	}
	w := post(t, &Handler{AppStore: &set}, `{"signedPayload":"`+strings.Repeat("a", appstore.MaxPayloadBytes+16)+`"}`)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	if accepted, reason := receipt(t, w); accepted || reason != "malformed" {
		t.Fatalf("accepted=%v reason=%q", accepted, reason)
	}
}
