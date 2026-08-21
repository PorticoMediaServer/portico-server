package app

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const remoteCertificateManifestVersion = 1

type remoteCertificateManifest struct {
	Version           int    `json:"version"`
	CurrentGeneration string `json:"currentGeneration"`
	PreviousGood      string `json:"previousGood,omitempty"`
	PublishedAt       string `json:"publishedAt"`
}

func (s *Server) remoteCertificateRoot() string {
	return filepath.Join(s.cfg.AppDataDir, "remote-access")
}
func (s *Server) remoteCertificateManifestPath() string {
	return filepath.Join(s.remoteCertificateRoot(), "certificate-current.json")
}

func (s *Server) publishRemoteCertificatePair(keyPEM, chainPEM []byte) error {
	root := s.remoteCertificateRoot()
	generations := filepath.Join(root, "certificate-generations")
	if err := os.MkdirAll(generations, 0o700); err != nil {
		return err
	}
	generation := fmt.Sprintf("generation-%d", time.Now().UTC().UnixNano())
	dir := filepath.Join(generations, generation)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	keyPath := filepath.Join(dir, "certificate-key.pem")
	chainPath := filepath.Join(dir, "certificate-chain.pem")
	if err := writePrivateCertificateFile(keyPath, keyPEM, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	if err := writePrivateCertificateFile(chainPath, chainPEM, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	if _, err := tls.LoadX509KeyPair(chainPath, keyPath); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("certificate generation is not a matching pair: %w", err)
	}
	previous := ""
	if raw, err := os.ReadFile(s.remoteCertificateManifestPath()); err == nil {
		var old remoteCertificateManifest
		if json.Unmarshal(raw, &old) == nil && old.Version == remoteCertificateManifestVersion {
			previous = old.CurrentGeneration
		}
	} else if !os.IsNotExist(err) {
		_ = os.RemoveAll(dir)
		return err
	}
	manifest := remoteCertificateManifest{Version: remoteCertificateManifestVersion, CurrentGeneration: generation, PreviousGood: previous, PublishedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	raw, err := json.Marshal(manifest)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	if err := atomicPrivateReplace(s.remoteCertificateManifestPath(), raw, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return nil
}

func writePrivateCertificateFile(path string, bytes []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, bytes, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func atomicPrivateReplace(path string, bytes []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".certificate-manifest-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(bytes); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *Server) publishedCertificateGenerations() ([]string, error) {
	raw, err := os.ReadFile(s.remoteCertificateManifestPath())
	if err != nil {
		return nil, err
	}
	var manifest remoteCertificateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Version != remoteCertificateManifestVersion || manifest.CurrentGeneration == "" {
		return nil, errors.New("remote certificate manifest is invalid")
	}
	values := []string{manifest.CurrentGeneration}
	if manifest.PreviousGood != "" && manifest.PreviousGood != manifest.CurrentGeneration {
		values = append(values, manifest.PreviousGood)
	}
	for _, generation := range values {
		if generation == "." || generation == ".." || strings.ContainsAny(generation, `/\\`) {
			return nil, errors.New("remote certificate manifest contains an unsafe generation")
		}
	}
	return values, nil
}

func (s *Server) loadPublishedRemoteAccessCertificate() (*tls.Certificate, error) {
	generations, err := s.publishedCertificateGenerations()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, generation := range generations {
		dir := filepath.Join(s.remoteCertificateRoot(), "certificate-generations", generation)
		certificate, loadErr := tls.LoadX509KeyPair(filepath.Join(dir, "certificate-chain.pem"), filepath.Join(dir, "certificate-key.pem"))
		if loadErr != nil {
			lastErr = loadErr
			continue
		}
		if len(certificate.Certificate) == 0 {
			lastErr = errors.New("certificate chain is empty")
			continue
		}
		leaf, parseErr := x509.ParseCertificate(certificate.Certificate[0])
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		certificate.Leaf = leaf
		return &certificate, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no valid certificate generation is available")
	}
	return nil, lastErr
}
