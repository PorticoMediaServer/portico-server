package postgres_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"portico.local/hosted/claimweb"
)

func signedSwitchCount(t *testing.T, f ownerReturnFixture, people int) claimweb.Signed {
	t.Helper()
	return ownerSigned(t, f.key, claimweb.OwnerSwitchCount{
		Kind: claimweb.OwnerSwitchCountKind, ServerID: f.request.ServerID, PublicKey: f.request.PublicKey,
		OperationID: f.request.OperationID, Purpose: "switch", PreviewID: "captured-preview", PreviewRevision: 1,
		LocalGeneration: f.request.LocalGeneration, ClaimHash: f.start.ClaimHash, OtherPeopleCount: people,
		IssuedAt: f.start.IssuedAt, ExpiresAt: f.start.ExpiresAt,
	})
}

func registerSwitchCount(t *testing.T, f *ownerReturnFixture, count *claimweb.Signed) {
	t.Helper()
	raw := returnCall(t, f.handler, "/v1/server-claims/pending", "", claimweb.PendingRequest{
		Payload: f.original.Payload, Signature: f.original.Signature, OwnerStart: &f.signed, OwnerSwitchCount: count,
	}, 200)
	if json.Unmarshal(raw, &f.pending) != nil || f.pending.OwnerTransactionID == "" {
		t.Fatal("count registration did not retain genuine owner start")
	}
}

func lookupSwitchCount(t *testing.T, f ownerReturnFixture) *int {
	t.Helper()
	raw := returnCall(t, f.handler, "/v1/server-claims/lookup", "claim-owner-access-token-fixture", map[string]string{"code": f.pending.Code}, 200)
	var p claimweb.LookupPending
	if json.Unmarshal(raw, &p) != nil || p.OwnerTransactionID != f.pending.OwnerTransactionID {
		t.Fatal("count lookup changed owner transaction")
	}
	return p.OtherPeopleCount
}

func TestOwnerSwitchCountFreezesFirstVerifiedProofWithParents(t *testing.T) {
	for _, people := range []int{0, 10000} {
		f := newOwnerReturn(t, "https://owner.example")
		count := signedSwitchCount(t, f, people)
		registerSwitchCount(t, &f, &count)
		originalID := f.pending.OwnerTransactionID
		if got := lookupSwitchCount(t, f); got == nil || *got != people {
			t.Fatal("verified captured count missing")
		}
		replacement := signedSwitchCount(t, f, 7)
		registerSwitchCount(t, &f, &replacement)
		if f.pending.OwnerTransactionID != originalID {
			t.Fatal("retry replaced parent transaction")
		}
		if got := lookupSwitchCount(t, f); got == nil || *got != people {
			t.Fatal("retry replaced frozen count")
		}
		var payload, signature []byte
		if e := f.store.DB.QueryRowContext(context.Background(), `SELECT switch_count_payload,switch_count_signature FROM owner_claim_starts WHERE transaction_id=$1`, originalID).Scan(&payload, &signature); e != nil {
			t.Fatal(e)
		}
		if base64.RawURLEncoding.EncodeToString(payload) != count.Payload || base64.RawURLEncoding.EncodeToString(signature) != count.Signature {
			t.Fatal("stored count bytes differ from original capture")
		}
	}
}

func TestOwnerSwitchCountAbsentOrInvalidNeverBlocksOrAppends(t *testing.T) {
	for _, mode := range []string{"absent", "overflow", "signature", "foreign-parent"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerReturn(t, "https://owner.example")
			var count *claimweb.Signed
			if mode != "absent" {
				value := signedSwitchCount(t, f, 0)
				switch mode {
				case "overflow":
					value = signedSwitchCount(t, f, 10001)
				case "signature":
					value.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
				case "foreign-parent":
					other := f
					other.request.OperationID += "-other"
					value = signedSwitchCount(t, other, 1)
				}
				count = &value
			}
			registerSwitchCount(t, &f, count)
			if lookupSwitchCount(t, f) != nil {
				t.Fatal("unverified numerical fact disclosed")
			}
			valid := signedSwitchCount(t, f, 3)
			registerSwitchCount(t, &f, &valid)
			if lookupSwitchCount(t, f) != nil {
				t.Fatal("retry appended count to frozen absence")
			}
			f.approve(t)
		})
	}
}

func TestOwnerSwitchCountLookupReverifiesStoredProofWithoutChangingApproval(t *testing.T) {
	f := newOwnerReturn(t, "https://owner.example")
	count := signedSwitchCount(t, f, 12)
	registerSwitchCount(t, &f, &count)
	if got := lookupSwitchCount(t, f); got == nil || *got != 12 {
		t.Fatal("genuine count missing before damage")
	}
	if _, e := f.store.DB.ExecContext(context.Background(), `UPDATE owner_claim_starts SET switch_count_signature=$2 WHERE transaction_id=$1`, f.pending.OwnerTransactionID, make([]byte, 64)); e != nil {
		t.Fatal(e)
	}
	if lookupSwitchCount(t, f) != nil {
		t.Fatal("damaged stored count became a display fact")
	}
	f.approve(t)
}
