package app

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func signedHostedWakeForTest(t *testing.T, wake hostedServerWake) []byte {
	t.Helper()
	wake.Signature = ""
	raw, err := json.Marshal(wake)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalHostedDocument(hostedServerWakeKind, raw)
	if err != nil {
		t.Fatal(err)
	}
	wake.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(testHostedDocumentPrivateKey(), payload))
	raw, err = json.Marshal(wake)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestHostedProfileWakeAckUsesExactServerBoundReceipt(t *testing.T) {
	var authorization string
	var received map[string]any
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/api/servers/srv_wake/profile-wake-ack" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode acknowledgement: %v", err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	t.Cleanup(hosted.Close)

	server := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{Enabled: true, ClaimStatus: "claimed", ServerID: "srv_wake", HostedBaseURL: hosted.URL, PublicPortMode: "disabled", PreferredRemoteAuthMode: "portico"}
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := server.ackHostedProfileWake(context.Background(), settings, "server-credential", "wake_profile_9", "acct_one", 9); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer server-credential" || received["wakeId"] != "wake_profile_9" || received["accountId"] != "acct_one" || received["targetProfileRevision"] != float64(9) {
		t.Fatalf("unexpected acknowledgement auth=%q body=%v", authorization, received)
	}
}

func validHostedWakeForTest(now time.Time) hostedServerWake {
	return hostedServerWake{
		Kind: hostedServerWakeKind, Version: 1, Audience: hostedDocumentAudience,
		ServerID: "srv_wake", TargetPolicyRevision: 42, Reason: "policy_changed", WakeID: "wake_revision_42",
		IssuedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: now.Add(2 * time.Minute).UTC().Format(time.RFC3339Nano),
		SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: testHostedDocumentKeyID,
	}
}

func TestHostedServerWakeVerificationRejectsTamperingAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	server := &Server{}
	server.cfg.HostedDocumentPublicKeys = testHostedDocumentPublicKeys()
	wake := validHostedWakeForTest(now)
	raw := signedHostedWakeForTest(t, wake)
	decoded, err := decodeHostedServerWake(raw)
	if err != nil || server.verifyHostedServerWake(raw, decoded, wake.ServerID, now) != nil {
		t.Fatalf("valid signed wake was rejected: decode=%v verify=%v", err, server.verifyHostedServerWake(raw, decoded, wake.ServerID, now))
	}

	var tampered map[string]any
	if err := json.Unmarshal(raw, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered["targetPolicyRevision"] = float64(43)
	tamperedRaw, _ := json.Marshal(tampered)
	tamperedWake, _ := decodeHostedServerWake(tamperedRaw)
	if err := server.verifyHostedServerWake(tamperedRaw, tamperedWake, wake.ServerID, now); err == nil {
		t.Fatal("tampered wake revision was accepted")
	}

	expired := validHostedWakeForTest(now.Add(-10 * time.Minute))
	expiredRaw := signedHostedWakeForTest(t, expired)
	expiredWake, _ := decodeHostedServerWake(expiredRaw)
	if err := server.verifyHostedServerWake(expiredRaw, expiredWake, wake.ServerID, now); err == nil {
		t.Fatal("expired wake was accepted")
	}
}

func TestHostedServerWakeHandlerAcceptsOnlyClaimedSignedSubject(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	server.cfg.HostedDocumentPublicKeys = testHostedDocumentPublicKeys()
	if err := server.saveRemoteAccessSettings(RemoteAccessSettings{
		Enabled: true, ClaimStatus: "claimed", ServerID: "srv_wake", HostedBaseURL: "https://hosted.example.test",
		PublicPortMode: "disabled", PreferredRemoteAuthMode: "portico",
	}); err != nil {
		t.Fatal(err)
	}
	// Keep this focused handler test from starting outbound synchronization.
	server.ownedAsyncClosing = true
	now := time.Now().UTC()
	raw := signedHostedWakeForTest(t, validHostedWakeForTest(now))

	request := httptest.NewRequest(http.MethodPost, "/api/remote-access/hosted-wake", strings.NewReader(string(raw)))
	request.RemoteAddr = "192.0.2.10:443"
	response := httptest.NewRecorder()
	server.handleHostedServerWake(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("signed wake status=%d body=%s", response.Code, response.Body.String())
	}

	var wrongSubject hostedServerWake
	if err := json.Unmarshal(raw, &wrongSubject); err != nil {
		t.Fatal(err)
	}
	wrongSubject.ServerID = "srv_other"
	wrongRaw := signedHostedWakeForTest(t, wrongSubject)
	request = httptest.NewRequest(http.MethodPost, "/api/remote-access/hosted-wake", strings.NewReader(string(wrongRaw)))
	request.RemoteAddr = "192.0.2.11:443"
	response = httptest.NewRecorder()
	server.handleHostedServerWake(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-subject wake status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHostedServerWakeCoalescesHighestRevisionAndRepair(t *testing.T) {
	server := &Server{hostedWakeRunning: true}
	server.queueHostedServerWake(hostedServerWake{TargetPolicyRevision: 10, Reason: "policy_changed"})
	server.queueHostedServerWake(hostedServerWake{TargetPolicyRevision: 8, Reason: "repair_requested"})
	server.queueHostedServerWake(hostedServerWake{TargetPolicyRevision: 8, Reason: "profile_authority_changed", AccountID: "acct_one", TargetProfileRevision: 7, WakeID: "wake_profile_7"})
	server.queueHostedServerWake(hostedServerWake{TargetPolicyRevision: 9, Reason: "profile_authority_changed", AccountID: "acct_one", TargetProfileRevision: 9, WakeID: "wake_profile_9"})
	server.queueHostedServerWake(hostedServerWake{TargetPolicyRevision: 12, Reason: "authority_changed"})
	revision, repair, profiles, ok := server.nextHostedServerWake()
	if !ok || revision != 12 || !repair || profiles["acct_one"].Revision != 9 || profiles["acct_one"].WakeID != "wake_profile_9" {
		t.Fatalf("coalesced wake revision=%d repair=%t profiles=%v ok=%t", revision, repair, profiles, ok)
	}
}
