package security

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The first root-trust deploy keeps the production sealing key: rows the
// 22 Sep release sealed (unversioned) stay readable, new rows are versioned.
func TestAdoptedLegacySealOpensBothFormats(t *testing.T) {
	dir := t.TempDir()
	seal := make([]byte, 32)
	if _, err := rand.Read(seal); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(dir, "keys.json")
	raw, _ := json.Marshal(map[string]string{"privateKey": "unused", "sealKey": base64.RawURLEncoding.EncodeToString(seal)})
	if err := os.WriteFile(legacyFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "signing.json.seal.json")
	var out bytes.Buffer
	if err := KeysCLI([]string{"adopt-legacy-seal", "--legacy", legacyFile, "--out", target}, &out); err != nil {
		t.Fatal(err)
	}
	var file SealFile
	if err := readPrivate(target, &file); err != nil || !file.LegacyUnversioned || file.Version != 1 {
		t.Fatal(file, err)
	}
	// A value sealed the way the 22 Sep release did it.
	block, _ := aes.NewCipher(seal)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce)
	old := gcm.Seal(nonce, nonce, []byte("server token"), []byte("server:srv_1"))
	k := &Keys{SealKey: seal, SealVersion: 1, SealLegacy: true}
	if got, err := k.Open(old, "server:srv_1"); err != nil || got != "server token" {
		t.Fatal("legacy row unreadable", got, err)
	}
	fresh, err := k.Seal("new value", "ctx")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := k.Open(fresh, "ctx"); err != nil || got != "new value" {
		t.Fatal(got, err)
	}
	strict := &Keys{SealKey: seal, SealVersion: 1}
	if _, err = strict.Open(old, "server:srv_1"); err == nil {
		t.Fatal("a key without the legacy flag read an unversioned row")
	}
}
