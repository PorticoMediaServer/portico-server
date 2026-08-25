package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app/apiroute"
)

type hostedDocumentFixture struct {
	PublicKeyB64   string          `json:"publicKeyB64"`
	PolicySnapshot json.RawMessage `json:"policySnapshot"`
}

func TestHostedPolicySnapshotVerifiesCrossServiceFixture(t *testing.T) {
	fixture := loadHostedDocumentFixture(t)
	var snapshot RemotePolicySnapshot
	if err := json.Unmarshal(fixture.PolicySnapshot, &snapshot); err != nil {
		t.Fatalf("decode policy snapshot: %v", err)
	}
	now := time.Date(2026, 7, 9, 12, 1, 0, 0, time.UTC)
	keys := map[string]string{snapshot.SignatureKeyID: fixture.PublicKeyB64}
	if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, snapshot, "srv_fixture", now, keys, remotePolicyState{}); err != nil {
		t.Fatalf("verify Hosted fixture: %v", err)
	}

	t.Run("unknown key fails closed", func(t *testing.T) {
		if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, snapshot, "srv_fixture", now, nil, remotePolicyState{}); err == nil {
			t.Fatal("snapshot signed by an unknown key was accepted")
		}
	})

	t.Run("wrong audience fails closed", func(t *testing.T) {
		tampered := snapshot
		tampered.Audience = "some-other-product"
		if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, tampered, "srv_fixture", now, keys, remotePolicyState{}); err == nil {
			t.Fatal("snapshot with the wrong audience was accepted")
		}
	})

	t.Run("expired document fails closed", func(t *testing.T) {
		late := time.Date(2026, 7, 9, 12, 12, 0, 0, time.UTC)
		if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, snapshot, "srv_fixture", late, keys, remotePolicyState{}); err == nil {
			t.Fatal("expired policy snapshot was accepted")
		}
	})

	t.Run("older revision fails closed", func(t *testing.T) {
		previous := remotePolicyState{SnapshotID: "newer", IssuedAt: "2026-07-09T12:00:01Z"}
		if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, snapshot, "srv_fixture", now, keys, previous); err == nil {
			t.Fatal("older policy snapshot was accepted")
		}
	})

	t.Run("policy revision rollback fails closed even with newer generation", func(t *testing.T) {
		rolledBack := snapshot
		rolledBack.Generation = snapshot.Generation + 1
		rolledBack.PolicyRevision = 6
		previous := remotePolicyState{SnapshotID: snapshot.SnapshotID, SnapshotDigest: snapshot.Digest, Generation: snapshot.Generation, PolicyRevision: 7, PolicyDigest: snapshot.PolicyDigest, IssuedAt: snapshot.IssuedAt}
		if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, rolledBack, "srv_fixture", now, keys, previous); err == nil {
			t.Fatal("policy revision rollback was accepted")
		}
	})

	t.Run("same revision cannot carry different authority", func(t *testing.T) {
		colliding := snapshot
		colliding.Generation = snapshot.Generation + 1
		colliding.PolicyRevision = 7
		colliding.PolicyDigest = strings.Repeat("d", 64)
		previous := remotePolicyState{SnapshotID: snapshot.SnapshotID, SnapshotDigest: snapshot.Digest, Generation: snapshot.Generation, PolicyRevision: 7, PolicyDigest: snapshot.PolicyDigest, IssuedAt: snapshot.IssuedAt}
		if err := verifyHostedPolicySnapshot(fixture.PolicySnapshot, colliding, "srv_fixture", now, keys, previous); err == nil {
			t.Fatal("same policy revision carried different authority")
		}
	})

	t.Run("same revision authority can renew with new signed identity", func(t *testing.T) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		renewal := snapshot
		renewal.SignatureKeyID = "renewal-key"
		renewal.PolicyRevision = 7
		renewal.PolicyRoot = strings.Repeat("a", 64)
		renewal.ContentRoot = strings.Repeat("b", 64)
		renewal.SnapshotID = "policy_fixture_renewed"
		renewal.ManifestID = "policy_fixture_manifest_renewed"
		renewal.IssuedAt = "2026-07-09T12:02:00Z"
		renewal.ExpiresAt = "2026-07-16T12:02:00Z"
		renewal.Digest = strings.Repeat("c", 64)
		rawRenewal, err := json.Marshal(renewal)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := canonicalHostedDocument("policy-snapshot", rawRenewal)
		if err != nil {
			t.Fatal(err)
		}
		renewal.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
		rawRenewal, err = json.Marshal(renewal)
		if err != nil {
			t.Fatal(err)
		}
		previous := remotePolicyState{
			SnapshotID: "policy_fixture", SnapshotDigest: snapshot.Digest, Generation: snapshot.Generation,
			PolicyRevision: 7, PolicyDigest: snapshot.PolicyDigest, PolicyRoot: renewal.PolicyRoot, ContentRoot: renewal.ContentRoot,
			IssuedAt: snapshot.IssuedAt,
		}
		keys := map[string]string{"renewal-key": base64.StdEncoding.EncodeToString(publicKey)}
		if err := verifyHostedPolicySnapshot(rawRenewal, renewal, "srv_fixture", now, keys, previous); err != nil {
			t.Fatalf("same-authority renewal was rejected: %v", err)
		}

		renewal.PolicyDigest = strings.Repeat("d", 64)
		rawRenewal, err = json.Marshal(renewal)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyHostedPolicySnapshot(rawRenewal, renewal, "srv_fixture", now, keys, previous); err == nil {
			t.Fatal("same revision renewal with different authority was accepted")
		}
	})
}

func TestRemotePolicyAuthorityBoundsAndTombstoneFences(t *testing.T) {
	owner := RemoteAccessMember{PorticoMembershipID: "mem_owner", PorticoUserID: "usr_owner", Role: "owner", Status: "active"}
	if err := validateRemotePolicyMembershipAuthority([]RemoteAccessMember{owner}); err != nil {
		t.Fatalf("valid owner policy rejected: %v", err)
	}
	if err := validateRemotePolicyMembershipAuthority(make([]RemoteAccessMember, maxRemotePolicyMembers+1)); err == nil {
		t.Fatal("oversized member policy was accepted")
	}
	if err := validateRemotePolicyTombstones([]RemoteDeletedAccountTombstone{{UserID: "usr_deleted", DeletedAt: "2026-08-25T12:00:00Z"}}); err != nil {
		t.Fatalf("valid tombstone rejected: %v", err)
	}
	if err := validateRemotePolicyTombstones([]RemoteDeletedAccountTombstone{{UserID: "usr_deleted", DeletedAt: "not-a-time"}}); err == nil {
		t.Fatal("malformed tombstone timestamp was accepted")
	}
}

func TestRemotePolicyContinuityUsesContractWindows(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	state := remotePolicyState{
		IssuedAt:         now.Add(-time.Hour).Format(time.RFC3339Nano),
		ExpiresAt:        now.Add(time.Hour).Format(time.RFC3339Nano),
		TrustedTimeFloor: now.Add(-time.Hour).Format(time.RFC3339Nano),
	}
	if got := remotePolicyContinuity(state, now); got != "valid" {
		t.Fatalf("continuity before expiry = %q", got)
	}
	if remotePolicyRenewalDue(state, now) {
		t.Fatal("policy was renewed before one full day from issuance")
	}
	state.IssuedAt = now.Add(-23 * time.Hour).Format(time.RFC3339Nano)
	state.ExpiresAt = now.Add(6 * 24 * time.Hour).Format(time.RFC3339Nano)
	if remotePolicyRenewalDue(state, now) {
		t.Fatal("policy younger than the daily renewal interval was due")
	}
	state.IssuedAt = now.Add(-24 * time.Hour).Format(time.RFC3339Nano)
	if !remotePolicyRenewalDue(state, now) {
		t.Fatal("policy at the daily renewal interval was not due")
	}
	state.ExpiresAt = now.Format(time.RFC3339Nano)
	if got := remotePolicyContinuity(state, now); got != "hard-expired" {
		t.Fatalf("continuity at exact expiry = %q", got)
	}
	if got := remotePolicyContinuity(state, now.Add(24*time.Hour)); got != "hard-expired" {
		t.Fatalf("continuity after expiry grace-shaped interval = %q", got)
	}
	state.ExpiresAt = now.Add(time.Hour).Format(time.RFC3339Nano)
	state.TrustedTimeFloor = now.Add(2 * time.Hour).Format(time.RFC3339Nano)
	if got := remotePolicyContinuity(state, now); got != "clock-invalid" {
		t.Fatalf("continuity after clock rollback = %q", got)
	}
}

func TestTrustedTimeFloorAdvancesOnlyForExactAcknowledgedPolicy(t *testing.T) {
	srv := newPorticoIdentitySyncTestServer(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	state := remotePolicyState{
		SnapshotID: "policy_exact", Generation: 7, PolicyDigest: strings.Repeat("a", 64),
		IssuedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano),
		TrustedTimeFloor: now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
	}
	if err := srv.saveRemotePolicyState(state); err != nil {
		t.Fatalf("save trusted-time fixture: %v", err)
	}
	mismatch := state
	mismatch.Generation++
	if err := srv.advanceRemotePolicyTrustedTimeFloor(mismatch, now); err != nil {
		t.Fatalf("ignore mismatched heartbeat policy: %v", err)
	}
	if got := srv.loadRemotePolicyState().TrustedTimeFloor; got != state.TrustedTimeFloor {
		t.Fatalf("mismatched policy advanced trusted time to %q", got)
	}
	if err := srv.advanceRemotePolicyTrustedTimeFloor(state, now); err != nil {
		t.Fatalf("advance exact acknowledged policy: %v", err)
	}
	if got := srv.loadRemotePolicyState().TrustedTimeFloor; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("exact acknowledged policy trusted time = %q", got)
	}
	ackPending := srv.loadRemotePolicyState()
	ackPending.AckPending = true
	ackPending.TrustedTimeFloor = now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if err := srv.saveRemotePolicyState(ackPending); err != nil {
		t.Fatalf("save pending-ack policy: %v", err)
	}
	if err := srv.advanceRemotePolicyTrustedTimeFloor(ackPending, now); err != nil {
		t.Fatalf("ignore pending-ack policy: %v", err)
	}
	if got := srv.loadRemotePolicyState().TrustedTimeFloor; got != ackPending.TrustedTimeFloor {
		t.Fatalf("pending-ack policy advanced trusted time to %q", got)
	}
}

func TestHostedPolicyContinuityFailsClosedAtSignedExpiry(t *testing.T) {
	srv := newPorticoIdentitySyncTestServer(t)
	now := time.Now().UTC()
	state := remotePolicyState{
		SnapshotID: "policy_expired", Generation: 1, PolicyDigest: strings.Repeat("c", 64),
		IssuedAt: now.Add(-maximumPolicyLifetime).Format(time.RFC3339Nano), ExpiresAt: now.Add(-time.Second).Format(time.RFC3339Nano),
		TrustedTimeFloor: now.Add(-time.Hour).Format(time.RFC3339Nano),
	}
	if err := srv.saveRemotePolicyState(state); err != nil {
		t.Fatalf("save expired policy: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/auth/me", nil)
	if srv.enforceHostedPolicyContinuity(recorder, request, apiroute.Route{}, false) {
		t.Fatal("expired signed authority was accepted")
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode expired authority problem: %v", err)
	}
	if recorder.Code != 503 || problem["code"] != "hosted_authority_stale" || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("expired authority response = status %d headers=%v problem=%#v", recorder.Code, recorder.Header(), problem)
	}
	logoutRecorder := httptest.NewRecorder()
	if !srv.enforceHostedPolicyContinuity(logoutRecorder, request, apiroute.Route{OperationID: "postAuthLogout"}, true) {
		t.Fatalf("expired authority blocked caller-owned logout: status=%d body=%s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
}

func TestHostedPolicyContinuityAllowsOnlyExplicitDenyOnlyCleanup(t *testing.T) {
	for _, operationID := range []string{"postAuthLogout", "deleteAccountSessionsId", "revokeAutomaticProfileTrusts"} {
		if !hostedContinuityDenyOnlyOperation(operationID) {
			t.Fatalf("deny-only operation %q was blocked", operationID)
		}
	}
	for _, operationID := range []string{"patchDevicesId", "deleteDevicesId", "deleteAuthApiKeysId", "deleteUsersId", "deleteMediaId", "deletePlaybackSessionsSessionId", "refreshNativeSession", ""} {
		if hostedContinuityDenyOnlyOperation(operationID) {
			t.Fatalf("authority-changing/destructive operation %q was treated as deny-only cleanup", operationID)
		}
	}
}

func loadHostedDocumentFixture(t *testing.T) hostedDocumentFixture {
	t.Helper()
	path := filepath.Join("testdata", "document-signing-fixture.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Hosted signing fixture: %v", err)
	}
	var fixture hostedDocumentFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("decode Hosted signing fixture: %v", err)
	}
	return fixture
}
