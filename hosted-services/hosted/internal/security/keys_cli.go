package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"portico.local/hosted/trust"
	"strings"
	"time"
)

func writeNew(path string, raw []byte, mode os.FileMode) error {
	if path == "" {
		return errors.New("output path required")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}
func WriteJSONNew(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeNew(path, append(raw, '\n'), 0600)
}

// KeysCLI never runs during startup. "setup" and "renew" are the everyday
// commands (see deployment/hosted/KEYS.md); root, signing, certify, rotate and
// inspect-root are the individual steps they are built from.
func KeysCLI(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("keys requires setup, renew, root, signing, certify, rotate, or inspect-root")
	}
	mode := args[0]
	switch mode {
	case "adopt-legacy-seal":
		return keysAdoptLegacySeal(args[1:], out)
	case "setup":
		return keysSetup(args[1:], out)
	case "renew":
		return keysRenew(args[1:], out)
	}
	fs := flag.NewFlagSet("keys "+mode, flag.ContinueOnError)
	fs.SetOutput(out)
	target := fs.String("out", "", "new output file (never overwritten)")
	sealFrom := fs.String("seal-from", "", "existing sealing file required for rotation")
	publicOut := fs.String("public-out", "", "new public PEM output")
	rootPath := fs.String("root", "", "offline root private PEM for certification only")
	publicPath := fs.String("public", "", "public PEM to certify or inspect")
	days := fs.Int("days", int(trust.DefaultCertificateLifetime/(24*time.Hour)), "certificate lifetime in days")
	revision := fs.Uint64("revision", 1, "monotonic trust revision")
	revoked := fs.String("revoked", "", "comma-separated permanently revoked key IDs")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected key command argument")
	}
	var rotateSeal SealFile
	if mode == "rotate" {
		// A22: check the sealing file before writing anything, so a bad path leaves
		// no orphaned private key behind.
		if *sealFrom == "" {
			return errors.New("rotation requires --seal-from; never replace the database sealing key")
		}
		if err := readPrivate(*sealFrom, &rotateSeal); err != nil {
			return err
		}
	}
	switch mode {
	case "root", "signing", "rotate":
		if *target == "" || *publicOut == "" {
			return errors.New("--out and --public-out required")
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if mode == "root" {
			raw, err := x509.MarshalPKCS8PrivateKey(private)
			if err != nil {
				return err
			}
			if err = writeNew(*target, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0600); err != nil {
				return err
			}
		} else {
			if err = WriteJSONNew(*target, keyFile{base64.RawURLEncoding.EncodeToString(private)}); err != nil {
				return err
			}
			if mode == "rotate" {
				if err = WriteJSONNew(*target+".seal.json", rotateSeal); err != nil {
					return err
				}
			} else {
				seal := make([]byte, 32)
				if _, err = rand.Read(seal); err != nil {
					return err
				}
				if err = WriteJSONNew(*target+".seal.json", SealFile{Version: 1, Key: base64.RawURLEncoding.EncodeToString(seal)}); err != nil {
					return err
				}
			}

		}
		raw, err := x509.MarshalPKIXPublicKey(public)
		if err != nil {
			return err
		}
		if err = writeNew(*publicOut, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: raw}), 0644); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Created key", trust.KeyID(public))
		return err
	case "certify":
		raw, err := os.ReadFile(*publicPath)
		if err != nil {
			return err
		}
		public, err := trust.ParsePublicPEM(raw)
		if err != nil {
			return err
		}
		info, err := os.Stat(*rootPath)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0077 != 0 {
			return errors.New("root private file must be 0600")
		}
		raw, err = os.ReadFile(*rootPath)
		if err != nil {
			return err
		}
		block, rest := pem.Decode(raw)
		if block == nil || len(rest) != 0 || block.Type != "PRIVATE KEY" {
			return trust.ErrTrust
		}
		value, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		root, ok := value.(ed25519.PrivateKey)
		if !ok {
			return trust.ErrTrust
		}
		list := []string{}
		if *revoked != "" {
			list = strings.Split(*revoked, ",")
		}
		for _, id := range list {
			if !validKeyID(id) {
				return fmt.Errorf("invalid revoked key ID %q", id)
			}
		}
		if *days < 1 || time.Duration(*days)*24*time.Hour > trust.MaximumCertificateLifetime {
			return trust.ErrTrust
		}
		now := time.Now().UTC().Add(-time.Minute)
		cert, err := trust.Certify(root, public, trust.KeyID(public), "documents", now, now.Add(time.Duration(*days)*24*time.Hour), *revision, list)
		if err != nil {
			return err
		}
		return WriteJSONNew(*target, cert)
	case "inspect-root":
		raw, err := os.ReadFile(*publicPath)
		if err != nil {
			return err
		}
		key, err := trust.ParsePublicPEM(raw)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, base64.RawURLEncoding.EncodeToString(key))
		return err
	default:
		return errors.New("unknown keys command")
	}
}

func validKeyID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func readRootPrivate(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("root private file must be readable by you only (chmod 600)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(raw)
	if block == nil || len(rest) != 0 || block.Type != "PRIVATE KEY" {
		return nil, trust.ErrTrust
	}
	value, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	root, ok := value.(ed25519.PrivateKey)
	if !ok {
		return nil, trust.ErrTrust
	}
	return root, nil
}

// writeSigningSet writes a new signing key, its sealing file and its
// certificate into dir as the three files Hosted loads from --key-file.
func writeSigningSet(dir string, root ed25519.PrivateKey, seal *SealFile, days int, revision uint64, revoked []string) (*trust.Certificate, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Add(-time.Minute)
	cert, err := trust.Certify(root, public, trust.KeyID(public), "documents", now, now.Add(time.Duration(days)*24*time.Hour), revision, revoked)
	if err != nil {
		return nil, err
	}
	path := dir + "/signing.json"
	if err = WriteJSONNew(path, keyFile{base64.RawURLEncoding.EncodeToString(private)}); err != nil {
		return nil, err
	}
	if seal != nil {
		if err = WriteJSONNew(path+".seal.json", seal); err != nil {
			return nil, err
		}
	}
	if err = WriteJSONNew(path+".certificate.json", cert); err != nil {
		return nil, err
	}
	return cert, nil
}

func newStagingDir(dir string) error {
	if dir == "" {
		return errors.New("--staging required")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("staging folder must not exist yet: %w", err)
	}
	return nil
}

// keysSetup is the one-time trust setup (KEYS.md): a new root, its public PEM
// for release builds, and a signing key certified by it, ready to install.
func keysSetup(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keys setup", flag.ContinueOnError)
	fs.SetOutput(out)
	privateOut := fs.String("private-out", "", "where to write the ROOT PRIVATE key (a path you choose; never overwritten)")
	publicOut := fs.String("public-out", "", "where to write the root public PEM (deployment/hosted/root-public-key.pem)")
	staging := fs.String("staging", "", "new folder for the files to install on Hosted")
	days := fs.Int("days", int(trust.DefaultCertificateLifetime/(24*time.Hour)), "signing certificate lifetime in days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *privateOut == "" || *publicOut == "" {
		return errors.New("setup needs --private-out, --public-out and --staging")
	}
	if *days < 30 || time.Duration(*days)*24*time.Hour > trust.MaximumCertificateLifetime {
		return fmt.Errorf("--days must be between 30 and %d", int(trust.MaximumCertificateLifetime/(24*time.Hour)))
	}
	for _, path := range []string{*privateOut, *publicOut} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists; setup never overwrites keys", path)
		}
	}
	if err := newStagingDir(*staging); err != nil {
		return err
	}
	rootPublic, rootPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	raw, err := x509.MarshalPKCS8PrivateKey(rootPrivate)
	if err != nil {
		return err
	}
	if err = writeNew(*privateOut, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0600); err != nil {
		return err
	}
	raw, err = x509.MarshalPKIXPublicKey(rootPublic)
	if err != nil {
		return err
	}
	if err = writeNew(*publicOut, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: raw}), 0644); err != nil {
		return err
	}
	sealKey := make([]byte, 32)
	if _, err = rand.Read(sealKey); err != nil {
		return err
	}
	cert, err := writeSigningSet(*staging, rootPrivate, &SealFile{Version: 1, Key: base64.RawURLEncoding.EncodeToString(sealKey)}, *days, 1, nil)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, `Root key ID:     %s
Signing key ID:  %s (valid until %s)

Root private key: %s
  Put it in your password manager (as a file attachment) or an encrypted volume,
  then delete this copy and any other stray copies. It is needed only once a
  year, to renew the signing key. Never put it on Hosted, in Git or in CI.
Root public key:  %s (commit it; release builds embed it)
Install on Hosted, owned by the Hosted service user (0600):
  %s/signing.json
  %s/signing.json.certificate.json
  %s/signing.json.seal.json
`, trust.KeyID(rootPublic), cert.KeyID, cert.NotAfter.Format("2 Jan 2006"), *privateOut, *publicOut, *staging, *staging, *staging)
	return err
}

// keysRenew certifies a new signing key with the same root. It needs only the
// current certificate (public) for the revision and revocation list; the
// sealing key never leaves Hosted, which keeps its signing.json.seal.json. The
// current certificate stays valid until it expires, so both work while the new
// files are deployed.
func keysRenew(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keys renew", flag.ContinueOnError)
	fs.SetOutput(out)
	rootPath := fs.String("root", "", "the root private key file")
	current := fs.String("current-certificate", "", "a copy of Hosted's current signing.json.certificate.json")
	staging := fs.String("staging", "", "new folder for the renewed files")
	days := fs.Int("days", int(trust.DefaultCertificateLifetime/(24*time.Hour)), "signing certificate lifetime in days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *rootPath == "" || *current == "" {
		return errors.New("renew needs --root, --current-certificate and --staging")
	}
	if *days < 30 || time.Duration(*days)*24*time.Hour > trust.MaximumCertificateLifetime {
		return fmt.Errorf("--days must be between 30 and %d", int(trust.MaximumCertificateLifetime/(24*time.Hour)))
	}
	root, err := readRootPrivate(*rootPath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*current)
	if err != nil {
		return err
	}
	var old trust.Certificate
	if err = json.Unmarshal(raw, &old); err != nil {
		return err
	}
	// The current certificate must come from this root. It may already have
	// expired; renewal must still work then.
	if _, err = old.Verify(root.Public().(ed25519.PublicKey), "documents", old.NotBefore.Add(time.Second)); err != nil {
		return errors.New("the current certificate was not issued by this root")
	}
	if err = newStagingDir(*staging); err != nil {
		return err
	}
	cert, err := writeSigningSet(*staging, root, nil, *days, old.Revision+1, old.Revoked)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, `New signing key ID: %s (revision %d, valid until %s)
The current key %s stays valid until %s, so deploy these while both work.
Replace these two files on Hosted and keep its signing.json.seal.json:
  %s/signing.json
  %s/signing.json.certificate.json
`, cert.KeyID, cert.Revision, cert.NotAfter.Format("2 Jan 2006"), old.KeyID, old.NotAfter.Format("2 Jan 2006"), *staging, *staging)
	return err
}

// keysAdoptLegacySeal turns the sealing key in a pre-versioned keys.json (the
// 22 Sep production format, {privateKey,sealKey}) into the sealing file the
// current build loads, keeping the same key so every sealed row stays
// readable. It runs on the Hosted host during the first deploy of the root
// trust model; the legacy signing key is not carried over.
func keysAdoptLegacySeal(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keys adopt-legacy-seal", flag.ContinueOnError)
	fs.SetOutput(out)
	legacy := fs.String("legacy", "", "the existing keys.json")
	target := fs.String("out", "", "the new signing.json.seal.json (never overwritten)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *legacy == "" || *target == "" {
		return errors.New("adopt-legacy-seal needs --legacy and --out")
	}
	var old struct {
		SealKey string `json:"sealKey"`
	}
	if err := readPrivate(*legacy, &old); err != nil {
		return err
	}
	key, err := base64.RawURLEncoding.DecodeString(old.SealKey)
	if err != nil || len(key) != 32 {
		return errors.New("the legacy file has no valid sealKey")
	}
	if err = WriteJSONNew(*target, SealFile{Version: 1, Key: old.SealKey, LegacyUnversioned: true}); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "Wrote", *target, "with the existing sealing key; rows sealed by the previous release stay readable.")
	return err
}
