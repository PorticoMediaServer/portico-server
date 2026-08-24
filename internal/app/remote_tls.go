package app

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// remoteTLSRuntime describes TLS availability on Portico's shared service
// port. The process-level listener is owned by porticod; the application owns
// certificate lifecycle, remote-access state, and status reporting.
type remoteTLSRuntime struct {
	address     string
	port        int
	err         string
	certificate *tls.Certificate
	revision    string
}

func (s *Server) StartRemoteAccessManager() {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		s.log.Warn("remote access settings unavailable", "error", err)
		return
	}
	s.configureRemoteTLS(settings)
}

func (s *Server) configureRemoteTLS(settings RemoteAccessSettings) {
	desired := s.desiredRemoteTLSAddress(settings)
	port := portFromAddress(desired)
	errText := ""
	s.remoteTLSMu.Lock()
	certificate := s.remoteTLS.certificate
	previousRevision := s.remoteTLS.revision
	s.remoteTLSMu.Unlock()
	revision := previousRevision
	if desired == "" {
		certificate = nil
		revision = ""
	}
	if desired != "" {
		var err error
		var replacement *tls.Certificate
		replacement, err = s.loadRemoteAccessCertificate(settings)
		if err != nil {
			errText = remoteAccessFailureCode(err)
		} else {
			certificate = replacement
			revision = remoteCertificateRevision(replacement)
		}
	}

	s.remoteTLSMu.Lock()
	changed := s.remoteTLS.address != desired || s.remoteTLS.port != port || s.remoteTLS.err != errText || s.remoteTLS.revision != revision
	s.remoteTLS.address = desired
	s.remoteTLS.port = port
	s.remoteTLS.err = errText
	s.remoteTLS.certificate = certificate
	s.remoteTLS.revision = revision
	s.remoteTLSMu.Unlock()

	if !changed {
		return
	}
	if desired == "" {
		s.log.Info("Portico TLS is inactive on the shared service port")
		return
	}
	if errText != "" {
		s.log.Warn("Portico TLS certificate unavailable on the shared service port", "addr", desired, "error", errText)
		return
	}
	certificateSource := "managed"
	if settings.CustomCertificateEnabled {
		certificateSource = "custom"
	}
	s.log.Info("Portico HTTP and HTTPS active on shared service port", "addr", desired, "certificateSource", certificateSource, "certificateRevision", revision)
}

func (s *Server) runRemoteTLSListenerRepair(ctx context.Context) {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		nextInterval := 15 * time.Second
		repaired, err := s.checkRemoteTLSListenerAndRepair(ctx)
		if err != nil {
			s.recordLog("warn", "Shared-port TLS repair failed", map[string]string{"error": err.Error()})
		} else if repaired {
			nextInterval = 5 * time.Minute
		}
		timer.Reset(nextInterval)
	}
}

func (s *Server) checkRemoteTLSListenerAndRepair(ctx context.Context) (bool, error) {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		return false, err
	}
	previousAddress, previousPort, previousError := s.remoteTLSStatus()
	previousRevision := s.remoteTLSCertificateRevision()
	if s.remoteTLSNeedsRepair(settings) {
		if updated, renewErr := s.ensureRemoteAccessCertificateFresh(ctx, settings); renewErr == nil {
			settings = updated
		}
	}
	s.configureRemoteTLS(settings)
	if s.remoteTLSNeedsRepair(settings) {
		return false, nil
	}
	currentRevision := s.remoteTLSCertificateRevision()
	if previousError == "" && previousRevision == currentRevision {
		return false, nil
	}
	s.recordLog("info", "Shared-port TLS certificate refreshed", map[string]string{
		"previousAddress": previousAddress,
		"previousPort":    strconv.Itoa(previousPort),
		"previousError":   previousError,
	})
	if settings.Enabled && settings.ClaimStatus == "claimed" && settings.ServerID != "" {
		if err := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{}); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (s *Server) remoteTLSCertificateRevision() string {
	s.remoteTLSMu.Lock()
	defer s.remoteTLSMu.Unlock()
	return s.remoteTLS.revision
}

func (s *Server) remoteTLSNeedsRepair(settings RemoteAccessSettings) bool {
	if s.desiredRemoteTLSAddress(settings) == "" {
		return false
	}
	s.remoteTLSMu.Lock()
	defer s.remoteTLSMu.Unlock()
	return s.remoteTLS.err != ""
}

func (s *Server) stopRemoteTLSLocked() {
	s.remoteTLS.address = ""
	s.remoteTLS.port = 0
	s.remoteTLS.err = ""
	s.remoteTLS.certificate = nil
	s.remoteTLS.revision = ""
}

func (s *Server) desiredRemoteTLSAddress(settings RemoteAccessSettings) string {
	if !settings.Enabled || settings.PublicPortMode == "disabled" {
		return ""
	}
	return s.cfg.Addr
}

func (s *Server) remoteTLSStatus() (string, int, string) {
	s.remoteTLSMu.Lock()
	defer s.remoteTLSMu.Unlock()
	return s.remoteTLS.address, s.remoteTLS.port, s.remoteTLS.err
}

// RemoteAccessCertificateLoader is used by porticod's shared HTTP/TLS
// listener. configureRemoteTLS atomically replaces immutable parsed material
// when configuration or certificate renewal changes; handshakes never read
// the filesystem or settings database.
func (s *Server) RemoteAccessCertificateLoader() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return s.remoteAccessCertificateLoader()
}

func (s *Server) remoteAccessCertificateLoader() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		s.remoteTLSMu.Lock()
		certificate := s.remoteTLS.certificate
		errText := s.remoteTLS.err
		s.remoteTLSMu.Unlock()
		if certificate == nil {
			if errText == "" {
				errText = "certificate material has not been loaded"
			}
			return nil, fmt.Errorf("remote access certificate unavailable: %s", errText)
		}
		if hello != nil && hello.ServerName != "" && certificate.Leaf != nil {
			if verifyErr := certificate.Leaf.VerifyHostname(hello.ServerName); verifyErr != nil {
				return nil, fmt.Errorf("remote access certificate does not cover %q: %w", hello.ServerName, verifyErr)
			}
		}
		return certificate, nil
	}
}

func (s *Server) loadRemoteAccessCertificate(settings RemoteAccessSettings) (*tls.Certificate, error) {
	var certificate *tls.Certificate
	var err error
	if settings.CustomCertificateEnabled {
		loaded, loadErr := tls.LoadX509KeyPair(settings.CustomCertificatePath, settings.CustomCertificateKeyPath)
		certificate = &loaded
		err = loadErr
	} else {
		certificate, err = s.loadPublishedRemoteAccessCertificate()
	}
	if err != nil {
		return nil, fmt.Errorf("remote access certificate unavailable: %w", err)
	}
	if len(certificate.Certificate) == 0 {
		return nil, fmt.Errorf("remote access certificate is invalid: certificate chain is empty")
	}
	if certificate.Leaf == nil {
		leaf, parseErr := x509.ParseCertificate(certificate.Certificate[0])
		if parseErr != nil {
			return nil, fmt.Errorf("remote access certificate is invalid: %w", parseErr)
		}
		certificate.Leaf = leaf
	}
	return certificate, nil
}

func remoteCertificateRevision(certificate *tls.Certificate) string {
	if certificate == nil {
		return ""
	}
	hash := sha256.New()
	for _, part := range certificate.Certificate {
		_, _ = hash.Write(part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
