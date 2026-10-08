package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"portico.local/hosted/trust"
	"testing"
	"time"
)

func TestMissingKeysFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "keys.json")
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := LoadKeysWithRoot(path, pub); !os.IsNotExist(err) {
		t.Fatalf("expected missing-key error, got %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("LoadKeys created a directory: %v", err)
	}
}

func TestExistingKeysAndPermissions(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seal := make([]byte, 32)
	if _, err = rand.Read(seal); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(keyFile{PrivateKey: base64.RawURLEncoding.EncodeToString(private)})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "keys.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cert, err := trust.Certify(private, private.Public().(ed25519.PublicKey), "fixture", "documents", time.Now().Add(-time.Minute), time.Now().Add(time.Hour), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteJSONNew(path+".certificate.json", cert); err != nil {
		t.Fatal(err)
	}
	if err = WriteJSONNew(path+".seal.json", SealFile{Version: 1, Key: base64.RawURLEncoding.EncodeToString(seal)}); err != nil {
		t.Fatal(err)
	}
	first, err := LoadKeysWithRoot(path, private.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadKeysWithRoot(path, private.Public().(ed25519.PublicKey))
	if err != nil || first.KeyID != second.KeyID {
		t.Fatalf("key identity changed: %v", err)
	}
	ciphertext, err := first.Seal("fixture", "account")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := second.Open(ciphertext, "account"); err != nil || value != "fixture" {
		t.Fatalf("seal roundtrip: %v", err)
	}
	if _, err = second.Open(ciphertext, "other-account"); err == nil {
		t.Fatal("wrong AAD accepted")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadKeysWithRoot(path, private.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("publicly readable key accepted")
	}
}
