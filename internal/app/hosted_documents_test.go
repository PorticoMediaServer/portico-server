package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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
