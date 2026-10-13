package security

import (
	"bytes"
	"os"
	"path/filepath"
	"portico.local/hosted/trust"
	"testing"
)

func TestOfflineCeremonyAndRotation(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root.pem")
	rootPub := filepath.Join(dir, "root-public.pem")
	leaf := filepath.Join(dir, "leaf.json")
	leafPub := filepath.Join(dir, "leaf-public.pem")
	run := func(args ...string) {
		t.Helper()
		if err := KeysCLI(args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	run("root", "--out", root, "--public-out", rootPub)
	run("signing", "--out", leaf, "--public-out", leafPub)
	run("certify", "--root", root, "--public", leafPub, "--out", leaf+".certificate.json")
	raw, _ := os.ReadFile(rootPub)
	public, err := trust.ParsePublicPEM(raw)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := LoadKeysWithRoot(leaf, public)
	if err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(dir, "next.json")
	nextPub := filepath.Join(dir, "next-public.pem")
	run("rotate", "--out", next, "--public-out", nextPub, "--seal-from", leaf+".seal.json")
	run("certify", "--root", root, "--public", nextPub, "--out", next+".certificate.json", "--revision", "2")
	newKeys, err := LoadKeysWithRoot(next, public)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(keys.PrivateKey, newKeys.PrivateKey) || !bytes.Equal(keys.SealKey, newKeys.SealKey) {
		t.Fatal("rotation must replace signing key and preserve sealing key")
	}
	if err = KeysCLI([]string{"root", "--out", root, "--public-out", rootPub}, &bytes.Buffer{}); err == nil {
		t.Fatal("root overwritten")
	}
}

func TestCertificationRefusesRevisionBeyondTheLocalServerRange(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root.pem")
	public := filepath.Join(dir, "root-public.pem")
	leaf := filepath.Join(dir, "leaf.json")
	leafPublic := filepath.Join(dir, "leaf-public.pem")
	for _, args := range [][]string{{"root", "--out", root, "--public-out", public}, {"signing", "--out", leaf, "--public-out", leafPublic}} {
		if err := KeysCLI(args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	target := leaf + ".certificate.json"
	if err := KeysCLI([]string{"certify", "--root", root, "--public", leafPublic, "--out", target, "--revision", "9007199254740992"}, &bytes.Buffer{}); err == nil {
		t.Fatal("certified an unsupported revision")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("refused certification wrote an artifact", err)
	}
}
