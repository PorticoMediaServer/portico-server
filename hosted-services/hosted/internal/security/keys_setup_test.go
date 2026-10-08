package security

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/trust"
)

// The yearly-renewal flow in KEYS.md, end to end in temporary folders.
func TestKeysSetupThenRenew(t *testing.T) {
	dir := t.TempDir()
	rootPrivate := filepath.Join(dir, "root-private.pem")
	rootPublic := filepath.Join(dir, "root-public-key.pem")
	first := filepath.Join(dir, "first")
	var out bytes.Buffer
	if err := KeysCLI([]string{"setup", "--private-out", rootPrivate, "--public-out", rootPublic, "--staging", first}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "password manager") {
		t.Fatalf("setup gave no safekeeping reminder:\n%s", out.String())
	}
	if info, err := os.Stat(rootPrivate); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("root private file not 0600", err)
	}
	raw, err := os.ReadFile(rootPublic)
	if err != nil {
		t.Fatal(err)
	}
	root, err := trust.ParsePublicPEM(raw)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := LoadKeysWithRoot(filepath.Join(first, "signing.json"), root)
	if err != nil {
		t.Fatalf("Hosted can't load the installed set: %v", err)
	}
	if life := keys.Certificate.NotAfter.Sub(keys.Certificate.NotBefore); life < 364*24*time.Hour || life > 366*24*time.Hour {
		t.Fatalf("certificate lasts %v, not a year", life)
	}
	// Setup never overwrites.
	if err = KeysCLI([]string{"setup", "--private-out", rootPrivate, "--public-out", filepath.Join(dir, "other.pem"), "--staging", filepath.Join(dir, "again")}, &out); err == nil {
		t.Fatal("setup overwrote the root private key")
	}
	second := filepath.Join(dir, "second")
	out.Reset()
	if err = KeysCLI([]string{"renew", "--root", rootPrivate, "--current-certificate", filepath.Join(first, "signing.json.certificate.json"), "--staging", second}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(second, "signing.json.seal.json")); err == nil {
		t.Fatal("renewal produced a sealing key; the existing one stays on Hosted")
	}
	// Installing means replacing two files next to the kept sealing file.
	seal, err := os.ReadFile(filepath.Join(first, "signing.json.seal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(second, "signing.json.seal.json"), seal, 0600); err != nil {
		t.Fatal(err)
	}
	renewed, err := LoadKeysWithRoot(filepath.Join(second, "signing.json"), root)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.KeyID == keys.KeyID || renewed.Certificate.Revision != keys.Certificate.Revision+1 || !bytes.Equal(renewed.SealKey, keys.SealKey) {
		t.Fatalf("renewal: key %s->%s revision %d->%d same seal %v", keys.KeyID, renewed.KeyID, keys.Certificate.Revision, renewed.Certificate.Revision, bytes.Equal(renewed.SealKey, keys.SealKey))
	}
	// The old set keeps working during the overlap.
	if _, err = LoadKeysWithRoot(filepath.Join(first, "signing.json"), root); err != nil {
		t.Fatal("the current key stopped working at renewal", err)
	}
	// A different root can't renew this installation.
	otherPrivate := filepath.Join(dir, "other-private.pem")
	if err = KeysCLI([]string{"setup", "--private-out", otherPrivate, "--public-out", filepath.Join(dir, "other-public.pem"), "--staging", filepath.Join(dir, "other")}, &out); err != nil {
		t.Fatal(err)
	}
	if err = KeysCLI([]string{"renew", "--root", otherPrivate, "--current-certificate", filepath.Join(first, "signing.json.certificate.json"), "--staging", filepath.Join(dir, "wrong")}, &out); err == nil {
		t.Fatal("a foreign root renewed the signing key")
	}
}

// A22: a rotation with a bad sealing file writes nothing.
func TestRotateChecksTheSealingFileFirst(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "next.json")
	var out bytes.Buffer
	if err := KeysCLI([]string{"rotate", "--out", target, "--public-out", filepath.Join(dir, "next.pem"), "--seal-from", filepath.Join(dir, "missing.seal.json")}, &out); err == nil {
		t.Fatal("rotation accepted a missing sealing file")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("rotation left an orphaned private key")
	}
}
