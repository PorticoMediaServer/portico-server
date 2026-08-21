package app

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const remoteAccessPendingCertificateOrderFile = "pending-certificate-order.json"

type remoteAccessPendingCertificateOrder struct {
	OrderID       string `json:"orderId,omitempty"`
	Hostname      string `json:"hostname"`
	PrivateKeyPEM string `json:"privateKeyPem"`
	CSRPEM        string `json:"csrPem"`
}

func (s *Server) pendingCertificateOrderPath() string {
	return filepath.Join(s.cfg.AppDataDir, "remote-access", remoteAccessPendingCertificateOrderFile)
}

func (s *Server) savePendingCertificateOrder(order remoteAccessPendingCertificateOrder) error {
	if strings.TrimSpace(order.Hostname) == "" || strings.TrimSpace(order.PrivateKeyPEM) == "" || strings.TrimSpace(order.CSRPEM) == "" {
		return errors.New("pending certificate order is incomplete")
	}
	encoded, err := json.Marshal(order)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.pendingCertificateOrderPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".pending-certificate-order-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(encoded); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, s.pendingCertificateOrderPath())
}

func (s *Server) loadPendingCertificateOrder() (remoteAccessPendingCertificateOrder, error) {
	path := s.pendingCertificateOrderPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return remoteAccessPendingCertificateOrder{}, os.ErrNotExist
	}
	if err != nil {
		return remoteAccessPendingCertificateOrder{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return remoteAccessPendingCertificateOrder{}, errors.New("pending certificate order path is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return remoteAccessPendingCertificateOrder{}, errors.New("pending certificate order is readable by group or others")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return remoteAccessPendingCertificateOrder{}, err
	}
	var order remoteAccessPendingCertificateOrder
	if err := json.Unmarshal(contents, &order); err != nil {
		return remoteAccessPendingCertificateOrder{}, fmt.Errorf("decode pending certificate order: %w", err)
	}
	if strings.TrimSpace(order.Hostname) == "" || strings.TrimSpace(order.PrivateKeyPEM) == "" || strings.TrimSpace(order.CSRPEM) == "" {
		return remoteAccessPendingCertificateOrder{}, errors.New("pending certificate order is incomplete")
	}
	return order, nil
}

func (s *Server) clearPendingCertificateOrder() error {
	err := os.Remove(s.pendingCertificateOrderPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func privateKeyFromPendingCertificateOrder(order remoteAccessPendingCertificateOrder) (*ecdsa.PrivateKey, []byte, error) {
	keyPEM := []byte(order.PrivateKeyPEM)
	block, rest := pem.Decode(keyPEM)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 || block.Type != "EC PRIVATE KEY" {
		return nil, nil, errors.New("pending certificate private key is invalid")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse pending certificate private key: %w", err)
	}
	return key, keyPEM, nil
}
