package postgres

import (
	"portico.local/hosted/internal/security"
	"testing"
	"time"
)

func TestNativeCompletionProof(t *testing.T) {
	now := time.Now()
	code := security.RandomID("")
	expected := security.Hash(code)
	for _, tc := range []struct {
		name, code     string
		hash           []byte
		expiry         time.Time
		consumed, want bool
	}{
		{"callback_device", code, expected, now.Add(time.Minute), false, true},
		{"initiator_has_only_binding", "", expected, now.Add(time.Minute), false, false},
		{"wrong_device", security.RandomID(""), expected, now.Add(time.Minute), false, false},
		{"different_transaction", code, security.Hash(security.RandomID("")), now.Add(time.Minute), false, false},
		{"replay", code, expected, now.Add(time.Minute), true, false},
		{"expired", code, expected, now, false, false},
		{"missing_hash", code, nil, now.Add(time.Minute), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validNativeCompletion(tc.code, tc.hash, tc.expiry, tc.consumed, now); got != tc.want {
				t.Fatalf("accepted=%v want=%v", got, tc.want)
			}
		})
	}
}
