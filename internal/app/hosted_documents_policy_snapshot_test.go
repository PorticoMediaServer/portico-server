package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRemotePolicyChunkDigestBindsPayload(t *testing.T) {
	first := json.RawMessage(`{"members":[{"porticoMembershipId":"m1"}],"deletedAccountTombstones":[]}`)
	second := json.RawMessage(`{"members":[{"porticoMembershipId":"m2"}],"deletedAccountTombstones":[]}`)
	a, err := remotePolicyChunkDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := remotePolicyChunkDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) != 64 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("chunk digest did not bind payload: %q %q", a, b)
	}
}

func TestRemotePolicyContentRootBindsOrderedChunksAndCount(t *testing.T) {
	first, err := remotePolicyContentRoot([]string{"a", "b"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if same, err := remotePolicyContentRoot([]string{"b", "a"}, 2); err != nil || same == first {
		t.Fatalf("content root did not bind chunk order: %q %q", first, same)
	}
	if same, err := remotePolicyContentRoot([]string{"a", "b"}, 3); err != nil || same == first {
		t.Fatalf("content root did not bind item count: %q %q", first, same)
	}
}
