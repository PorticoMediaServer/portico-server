package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const identityResetJournalVersion = 1

type identityResetJournal struct {
	Version             int    `json:"version"`
	OperationID         string `json:"operationId"`
	PreviousServerID    string `json:"previousServerId"`
	PreviousFingerprint string `json:"previousFingerprint"`
	NewServerID         string `json:"newServerId"`
	NewFingerprint      string `json:"newFingerprint"`
	StagedKeyPath       string `json:"stagedKeyPath"`
	DatabaseCommitted   bool   `json:"databaseCommitted"`
	Publication         string `json:"publication"`
	CreatedAt           string `json:"createdAt"`
}

func (s *Server) identityResetJournalPath() string {
	return filepath.Join(s.cfg.AppDataDir, "remote-access", "identity-reset.json")
}

func (s *Server) writeIdentityResetJournal(journal identityResetJournal) error {
	if err := os.MkdirAll(filepath.Dir(s.identityResetJournalPath()), 0o700); err != nil {
		return err
	}
	bytes, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.identityResetJournalPath()), "identity-reset-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
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
	return os.Rename(tmpPath, s.identityResetJournalPath())
}

func (s *Server) readIdentityResetJournal() (identityResetJournal, error) {
	bytes, err := os.ReadFile(s.identityResetJournalPath())
	if err != nil {
		return identityResetJournal{}, err
	}
	var journal identityResetJournal
	if err := json.Unmarshal(bytes, &journal); err != nil {
		return identityResetJournal{}, fmt.Errorf("identity reset journal is invalid: %w", err)
	}
	if journal.Version != identityResetJournalVersion || journal.OperationID == "" || journal.NewServerID == "" || journal.NewFingerprint == "" || journal.StagedKeyPath == "" {
		return identityResetJournal{}, errors.New("identity reset journal has unsupported fields")
	}
	root := filepath.Clean(filepath.Join(s.cfg.AppDataDir, "remote-access"))
	stage, err := filepath.Abs(journal.StagedKeyPath)
	if err != nil || filepath.Dir(stage) != root {
		return identityResetJournal{}, errors.New("identity reset journal staged path is outside remote-access")
	}
	journal.StagedKeyPath = stage
	return journal, nil
}

func (s *Server) reconcileIdentityReset(ctx context.Context) error {
	journal, err := s.readIdentityResetJournal()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return err
	}
	identity, _ := settings["identity"].(map[string]any)
	serverID := settingString(identity, "serverId", "")
	canonical := s.serverIdentityKeyPath()
	if serverID == journal.NewServerID {
		if _, err := os.Stat(journal.StagedKeyPath); err == nil {
			if err := os.Rename(journal.StagedKeyPath, canonical); err != nil {
				return fmt.Errorf("publish staged server identity: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := os.Stat(canonical); err != nil {
			return fmt.Errorf("committed identity reset has no private key: %w", err)
		}
		return os.Remove(s.identityResetJournalPath())
	}
	if journal.PreviousServerID != "" && serverID != journal.PreviousServerID {
		return errors.New("identity reset journal does not match the persisted server identity")
	}
	if err := os.Remove(journal.StagedKeyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Remove(s.identityResetJournalPath())
}

func generateServerIdentityAt(path string) (serverIdentity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return serverIdentity{}, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return serverIdentity{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return serverIdentity{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "server-identity-*.tmp")
	if err != nil {
		return serverIdentity{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return serverIdentity{}, err
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return serverIdentity{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return serverIdentity{}, err
	}
	if err := tmp.Close(); err != nil {
		return serverIdentity{}, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return serverIdentity{}, err
	}
	return serverIdentity{PublicKey: publicKey, PrivateKey: privateKey, Fingerprint: publicKeyFingerprint(publicKey)}, nil
}
