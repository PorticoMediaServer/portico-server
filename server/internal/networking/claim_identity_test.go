package networking

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestClaimIdentitySharedVector(t *testing.T) {
	// RFC8032 section7.1 public key (test1); expected digest is independently
	// calculated with Python hashlib, not from ServerIdentity itself.
	raw, e := hex.DecodeString("d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
	if e != nil {
		t.Fatal(e)
	}
	id, e := ServerIdentity(ed25519.PublicKey(raw))
	if e != nil {
		t.Fatal(e)
	}
	if id != "srv_7BFIeYyXBpRWRLzgrvXRa8Ml2H7kI5jz7XcCg1ymsaE" {
		t.Fatalf("wrong domain-separated identity: %s", id)
	}
	raw[0] ^= 1
	other, e := ServerIdentity(raw)
	if e != nil || other == id {
		t.Fatal("changed key retained identity")
	}
	for _, bad := range [][]byte{nil, make([]byte, 31), make([]byte, 33)} {
		if _, e = ServerIdentity(bad); e == nil {
			t.Fatal("invalid public key length accepted")
		}
	}
}
