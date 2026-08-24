package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	remoteAccessSettingsKey        = "remoteAccess"
	remoteAccessClaimKey           = "remoteAccessClaim"
	remoteAccessClaimTokenKey      = "remoteAccessClaimToken"
	remoteAccessClaimReceiptKey    = "remoteAccessClaimReceipt"
	remoteAccessClaimOperationKey  = "remoteAccessClaimOperation"
	remoteAccessClaimActivationKey = "remoteAccessClaimActivation"
	remoteAccessCredentialKey      = "remoteAccessCredential"
	remoteAccessPolicyStateKey     = "remoteAccessPolicyState"
	defaultHostedBaseURL           = "https://api.getportico.tv"
	defaultRemotePublicPort        = 32500
)

var (
	errPorticoIdentityLinkRequired        = errors.New("portico_identity_link_required")
	errPorticoIdentityConflict            = errors.New("portico_identity_conflict")
	remoteAccessCertificateProvisioningMu sync.Mutex
	remoteAccessClaimPollStates           sync.Map
)

type remoteAccessClaimActivation struct {
	Claim            RemoteAccessClaim    `json:"claim"`
	Settings         RemoteAccessSettings `json:"settings"`
	ServerCredential string               `json:"serverCredential"`
}

type remoteAccessClaimPollState struct {
	mu          sync.Mutex
	lastErr     string
	lastClaimID string
	lastPollAt  time.Time
}

type RemoteAccessSettings struct {
	Enabled                         bool   `json:"enabled"`
	HostedBaseURL                   string `json:"hostedBaseUrl"`
	ClaimStatus                     string `json:"claimStatus"`
	ServerID                        string `json:"serverId"`
	AssignedHostname                string `json:"assignedHostname"`
	PublicPortMode                  string `json:"publicPortMode"`
	ManualPublicPort                int    `json:"manualPublicPort"`
	PreferredRemoteAuthMode         string `json:"preferredRemoteAuthMode"`
	AllowManualLocalAuthRemoteLogin bool   `json:"allowManualLocalAuthRemoteLogin"`
	LANDiscoveryEnabled             bool   `json:"lanDiscoveryEnabled"`
	RouterAutomationEnabled         bool   `json:"routerAutomationEnabled"`
	RemoteBitrateLimitMbps          int    `json:"remoteBitrateLimitMbps"`
	CertificateStatus               string `json:"certificateStatus"`
	CertificateExpiresAt            string `json:"certificateExpiresAt,omitempty"`
	LastCertificateRenewalAt        string `json:"lastCertificateRenewalAt,omitempty"`
	CertificateRenewalError         string `json:"certificateRenewalError,omitempty"`
	CustomCertificateEnabled        bool   `json:"customCertificateEnabled"`
	CustomCertificatePath           string `json:"customCertificatePath,omitempty"`
	CustomCertificateKeyPath        string `json:"customCertificateKeyPath,omitempty"`
	LastHeartbeatAt                 string `json:"lastHeartbeatAt,omitempty"`
	LastHeartbeatError              string `json:"lastHeartbeatError,omitempty"`
	LastHostedRemoteAccessState     string `json:"lastHostedRemoteAccessState,omitempty"`
	LastPublicIPAddress             string `json:"lastPublicIpAddress,omitempty"`
	LastPublicIPCheckAt             string `json:"lastPublicIpCheckAt,omitempty"`
	LastNetworkSignature            string `json:"lastNetworkSignature,omitempty"`
	LastNetworkChangeAt             string `json:"lastNetworkChangeAt,omitempty"`
	LastRouteRepairAt               string `json:"lastRouteRepairAt,omitempty"`
	LastRouteRepairReason           string `json:"lastRouteRepairReason,omitempty"`
	LastReachabilityCheckAt         string `json:"lastReachabilityCheckAt,omitempty"`
	LastReachabilityResult          string `json:"lastReachabilityResult,omitempty"`
	LastPublicRouteError            string `json:"lastPublicRouteError,omitempty"`
	RouterMappingStatus             string `json:"routerMappingStatus,omitempty"`
	LastRouterMappingAt             string `json:"lastRouterMappingAt,omitempty"`
	RouterMappingError              string `json:"routerMappingError,omitempty"`
}

type RemoteAccessStatus struct {
	Settings                   RemoteAccessSettings `json:"settings"`
	ServerPublicKey            string               `json:"serverPublicKey"`
	ServerPublicKeyFingerprint string               `json:"serverPublicKeyFingerprint"`
	LocalRoutes                []RemoteAccessRoute  `json:"localRoutes"`
	PublicEndpoint             RemoteAccessEndpoint `json:"publicEndpoint"`
	Connectivity               RemoteConnectivity   `json:"connectivity"`
	LocalTLSAddress            string               `json:"localTlsAddress,omitempty"`
	LocalTLSPort               int                  `json:"localTlsPort,omitempty"`
	LocalTLSPortMatchesPublic  bool                 `json:"localTlsPortMatchesPublic"`
	LocalTLSError              string               `json:"localTlsError,omitempty"`
	PorticoMembers             []RemoteAccessMember `json:"porticoMembers"`
	PolicySync                 RemotePolicySync     `json:"policySync"`
	PolicySyncPending          bool                 `json:"policySyncPending,omitempty"`
	Warning                    string               `json:"warning,omitempty"`
	Claim                      *RemoteAccessClaim   `json:"claim,omitempty"`
	PorticoConnected           bool                 `json:"porticoConnected"`
	GeneratedAt                string               `json:"generatedAt"`
}

type RemoteConnectivity struct {
	HostedServicesStatus  string `json:"hostedServicesStatus"`
	PublicRouteStatus     string `json:"publicRouteStatus"`
	PublicRouteError      string `json:"publicRouteError,omitempty"`
	LastCheckedAt         string `json:"lastCheckedAt,omitempty"`
	TroubleshootingStatus string `json:"troubleshootingStatus"`
	TroubleshootingHint   string `json:"troubleshootingHint,omitempty"`
}

type RemoteAccessRoute struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Source string `json:"source"`
}

type RemoteAccessEndpoint struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	URL    string `json:"url"`
}

type RemoteAccessClaim struct {
	ClaimID          string `json:"claimId"`
	ClaimCode        string `json:"claimCode,omitempty"`
	ClaimToken       string `json:"-"`
	ClaimURL         string `json:"claimUrl,omitempty"`
	Status           string `json:"status"`
	ExpiresAt        string `json:"expiresAt"`
	StartedAt        string `json:"startedAt"`
	HostedReady      bool   `json:"hostedReady"`
	LocalOwnerUserID string `json:"localOwnerUserId,omitempty"`
	ServerName       string `json:"serverName,omitempty"`
}

type RemoteAccessMember struct {
	ID                  string                   `json:"id,omitempty"`
	UserID              string                   `json:"userId,omitempty"`
	PorticoMembershipID string                   `json:"porticoMembershipId"`
	PorticoUserID       string                   `json:"porticoUserId"`
	Email               string                   `json:"email"`
	DisplayName         string                   `json:"displayName"`
	ProfileImageURL     string                   `json:"profileImageUrl,omitempty"`
	Role                string                   `json:"role"`
	Status              string                   `json:"status"`
	PermissionTemplate  RemotePermissionTemplate `json:"permissionTemplate,omitempty"`
	Preferences         UserPreferences          `json:"preferences,omitempty"`
	LocalUserID         string                   `json:"localUserId,omitempty"`
	LastSyncedAt        string                   `json:"lastSyncedAt"`
}

type RemotePolicySync struct {
	Status       string `json:"status"`
	LastSyncedAt string `json:"lastSyncedAt,omitempty"`
	MemberCount  int    `json:"memberCount"`
	Stale        bool   `json:"stale"`
	Note         string `json:"note"`
}

type RemotePolicySnapshot struct {
	Kind                     string                          `json:"kind"`
	Audience                 string                          `json:"audience"`
	SnapshotID               string                          `json:"snapshotId"`
	Generation               int64                           `json:"generation,omitempty"`
	Digest                   string                          `json:"digest"`
	PolicyDigest             string                          `json:"policyDigest"`
	Version                  int                             `json:"version"`
	ServerID                 string                          `json:"serverId"`
	Members                  []RemoteAccessMember            `json:"members"`
	DeletedAccountTombstones []RemoteDeletedAccountTombstone `json:"deletedAccountTombstones"`
	IssuedAt                 string                          `json:"issuedAt"`
	ExpiresAt                string                          `json:"expiresAt"`
	SignatureAlgorithm       string                          `json:"signatureAlgorithm"`
	SignatureKeyID           string                          `json:"signatureKeyId,omitempty"`
	Signature                string                          `json:"signature"`
}

type RemoteDeletedAccountTombstone struct {
	UserID    string `json:"userId"`
	DeletedAt string `json:"deletedAt"`
}

type RemotePermissionTemplate struct {
	Permissions      map[string]bool `json:"permissions,omitempty"`
	MaxContentRating string          `json:"maxContentRating,omitempty"`
}

type RemoteAccessSettingsPatch struct {
	Enabled                         *bool   `json:"enabled,omitempty"`
	HostedBaseURL                   *string `json:"hostedBaseUrl,omitempty"`
	PublicPortMode                  *string `json:"publicPortMode,omitempty"`
	ManualPublicPort                *int    `json:"manualPublicPort,omitempty"`
	PreferredRemoteAuthMode         *string `json:"preferredRemoteAuthMode,omitempty"`
	AllowManualLocalAuthRemoteLogin *bool   `json:"allowManualLocalAuthRemoteLogin,omitempty"`
	LANDiscoveryEnabled             *bool   `json:"lanDiscoveryEnabled,omitempty"`
	RouterAutomationEnabled         *bool   `json:"routerAutomationEnabled,omitempty"`
	RemoteBitrateLimitMbps          *int    `json:"remoteBitrateLimitMbps,omitempty"`
	CustomCertificateEnabled        *bool   `json:"customCertificateEnabled,omitempty"`
	CustomCertificatePath           *string `json:"customCertificatePath,omitempty"`
	CustomCertificateKeyPath        *string `json:"customCertificateKeyPath,omitempty"`
}

func (s *Server) handleRemoteAccessStatus(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	status, err := s.remoteAccessStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemoteAccessHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "identity_failed", "Unable to load server identity.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                     "ok",
		"serverId":                   settings.ServerID,
		"assignedHostname":           settings.AssignedHostname,
		"serverPublicKeyFingerprint": identity.Fingerprint,
		"remoteAccessEnabled":        settings.Enabled,
		"certificateStatus":          settings.CertificateStatus,
	})
}

func (s *Server) handleRemoteAccessSettings(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
		return
	}
	var patch RemoteAccessSettingsPatch
	if !decodeJSON(w, r, &patch) {
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return
	}
	previous := settings
	applyRemoteAccessSettingsPatch(&settings, patch)
	if err := s.validateRemoteAccessSettings(settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_remote_access_settings", err.Error())
		return
	}
	if settings.CustomCertificateEnabled {
		if err := s.refreshCustomCertificateStatus(&settings); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_custom_certificate", err.Error())
			return
		}
	}
	if shouldRemoveRouterMapping(previous, settings) {
		mapping := s.removeRouterMapping(r.Context(), previous)
		settings.RouterMappingStatus = "removed"
		settings.LastRouterMappingAt = time.Now().UTC().Format(time.RFC3339)
		settings.RouterMappingError = mapping.Error
		s.recordAudit(r, user, "remote_access.router_mapping_removed", "remote_access", previous.ServerID, "warn", routerMappingAuditMetadata(previous, mapping))
	}
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to save remote access settings.")
		return
	}
	s.configureRemoteTLS(settings)
	s.recordAudit(r, user, "remote_access.settings_updated", "remote_access", settings.ServerID, "warn", map[string]string{
		"enabled":                strconv.FormatBool(settings.Enabled),
		"authMode":               settings.PreferredRemoteAuthMode,
		"publicPort":             strconv.Itoa(settings.ManualPublicPort),
		"publicMode":             settings.PublicPortMode,
		"remoteBitrateLimitMbps": strconv.Itoa(settings.RemoteBitrateLimitMbps),
		"hostedBaseURL":          redactURL(settings.HostedBaseURL),
	})
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemoteAccessClaimStart(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return
	}
	settings.Enabled = true
	settings.ClaimStatus = "pending"
	if err := s.validateRemoteAccessSettings(settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_remote_access_settings", err.Error())
		return
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "identity_failed", "Unable to prepare server identity.")
		return
	}
	if claim := s.currentRemoteAccessClaim(); claim != nil {
		expiresAt, expiresErr := time.Parse(time.RFC3339Nano, claim.ExpiresAt)
		if claim.Status != "pending" || expiresErr != nil || !expiresAt.After(time.Now().UTC()) || s.secretSetting(remoteAccessClaimReceiptKey) == "" {
			if err := s.retireRemoteAccessClaim(); err != nil {
				writeError(w, http.StatusInternalServerError, "claim_failed", "Unable to retire the previous claim.")
				return
			}
		}
	}
	claim, claimReceipt, err := s.startHostedClaim(r.Context(), settings, identity)
	if err != nil {
		writeError(w, http.StatusBadGateway, "hosted_claim_failed", "Unable to start a claim with Portico.")
		return
	}
	claim.LocalOwnerUserID = user.ID
	if err := s.persistRemoteAccessClaimStart(claim, claimReceipt, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to save remote access settings.")
		return
	}
	s.recordAudit(r, user, "remote_access.claim_started", "remote_access", identity.Fingerprint, "warn", map[string]string{"claimId": claim.ClaimID})
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusCreated, status)
}

func (s *Server) startPorticoSetupClaim(ctx context.Context) (RemoteAccessStatus, error) {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		return RemoteAccessStatus{}, err
	}
	if claim := s.currentRemoteAccessClaim(); claim != nil {
		expiresAt, expiresErr := time.Parse(time.RFC3339Nano, claim.ExpiresAt)
		if claim.Status == "pending" && expiresErr == nil && expiresAt.After(time.Now().UTC()) && s.secretSetting(remoteAccessClaimReceiptKey) != "" {
			// Repair the final claim-start write if the process stopped after the
			// protected claim secrets and claim were committed but before settings
			// were published. Result polling is receipt-authenticated; the bearer is
			// retained only for an explicit pending-claim cancellation.
			settings.Enabled = true
			settings.ClaimStatus = "pending"
			if err := s.saveRemoteAccessSettings(settings); err != nil {
				return RemoteAccessStatus{}, err
			}
			status, statusErr := s.remoteAccessStatus()
			return status, statusErr
		}
		// A pending claim without its response receipt cannot ever be polled. An
		// expired or receipt-less claim must not retain the creation idempotency key, otherwise
		// Hosted Services can replay the expired operation indefinitely.
		if err := s.retireRemoteAccessClaim(); err != nil {
			return RemoteAccessStatus{}, err
		}
	}
	settings.Enabled = true
	settings.ClaimStatus = "pending"
	if err := s.validateRemoteAccessSettings(settings); err != nil {
		return RemoteAccessStatus{}, err
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return RemoteAccessStatus{}, err
	}
	claim, claimReceipt, err := s.startHostedClaim(ctx, settings, identity)
	if err != nil {
		return RemoteAccessStatus{}, err
	}
	if err := s.persistRemoteAccessClaimStart(claim, claimReceipt, settings); err != nil {
		return RemoteAccessStatus{}, err
	}
	s.recordLog("info", "Portico setup claim started", map[string]string{"claimId": claim.ClaimID})
	return s.remoteAccessStatus()
}

func porticoSetupClaimProblem(err error) (string, string) {
	if errors.Is(err, context.Canceled) {
		return "hosted_claim_cancelled", "The setup request was cancelled before Portico Hosted Services answered. Try again."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "hosted_claim_unreachable", "Portico Hosted Services did not answer in time. Check this server's internet connection and try again."
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return "hosted_claim_unreachable", "Portico Hosted Services could not be reached. Check this server's internet connection and try again."
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "claim returned") {
		return "hosted_claim_rejected", "Portico Hosted Services rejected the setup request. Try again shortly."
	}
	if strings.Contains(message, "missing claim") || strings.Contains(message, "invalid character") || strings.Contains(message, "claim url") {
		return "hosted_claim_invalid_response", "Portico Hosted Services returned an incomplete setup response. Try again shortly."
	}
	return "hosted_claim_failed", "This server could not finish preparing the Portico Account handoff. Check the server logs for more detail, then try again."
}

func porticoSetupClaimURL(rawClaimURL, localOrigin string) string {
	claimURL, err := url.Parse(strings.TrimSpace(rawClaimURL))
	if err != nil || claimURL.Host == "" ||
		(claimURL.Scheme != "https" && !(claimURL.Scheme == "http" && isLoopbackHost(claimURL.Hostname()))) {
		return rawClaimURL
	}
	local, err := url.Parse(strings.TrimSpace(localOrigin))
	if err != nil || local.User != nil || local.Path != "" || local.RawQuery != "" || local.Fragment != "" ||
		(local.Scheme != "http" && local.Scheme != "https") || !isLocalSetupHost(local.Hostname()) {
		return rawClaimURL
	}
	local.Path = "/"
	query := claimURL.Query()
	query.Set("returnUrl", local.String()+"?porticoSetup=continue")
	claimURL.RawQuery = query.Encode()
	return claimURL.String()
}

func isLocalSetupHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if isLoopbackHost(host) {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func hostedServicesHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 12 * time.Second,
		// Operation keys, bearers, and response receipts are all
		// credentials. Do not let net/http forward them to a redirect target.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("Hosted Services request redirected")
		},
	}
}

func (s *Server) startHostedClaim(ctx context.Context, settings RemoteAccessSettings, identity serverIdentity) (RemoteAccessClaim, string, error) {
	systemIdentity, err := s.systemIdentity()
	if err != nil {
		return RemoteAccessClaim{}, "", err
	}
	payload := map[string]string{
		"serverPublicKey":            base64.RawStdEncoding.EncodeToString(identity.PublicKey),
		"serverPublicKeyFingerprint": identity.Fingerprint,
		"serverName":                 systemIdentity.FriendlyName,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return RemoteAccessClaim{}, "", err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	operationKey := s.secretSetting(remoteAccessClaimOperationKey)
	if operationKey == "" {
		operationBytes := make([]byte, 18)
		if _, err := rand.Read(operationBytes); err != nil {
			return RemoteAccessClaim{}, "", fmt.Errorf("generate claim operation key: %w", err)
		}
		operationKey = "claim-" + base64.RawURLEncoding.EncodeToString(operationBytes)
		// Persist before the request. If Hosted commits but the response is lost,
		// the next attempt replays the same logical operation instead of creating
		// a second server claim.
		if err := s.saveSecretSetting(remoteAccessClaimOperationKey, operationKey); err != nil {
			return RemoteAccessClaim{}, "", fmt.Errorf("persist claim operation key: %w", err)
		}
	}
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/server-claims"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return RemoteAccessClaim{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", operationKey)
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return RemoteAccessClaim{}, "", err
	}
	defer resp.Body.Close()
	responseBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return RemoteAccessClaim{}, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RemoteAccessClaim{}, "", fmt.Errorf("Hosted Services claim returned %s", resp.Status)
	}
	var hostedClaim struct {
		ClaimID             string `json:"claimId"`
		ClaimCode           string `json:"claimCode"`
		ClaimToken          string `json:"claimToken"`
		ClaimURL            string `json:"claimUrl"`
		Status              string `json:"status"`
		ExpiresAt           string `json:"expiresAt"`
		LostResponseReceipt string `json:"lostResponseReceipt"`
	}
	if err := json.Unmarshal(responseBytes, &hostedClaim); err != nil {
		return RemoteAccessClaim{}, "", err
	}
	hostedClaim.LostResponseReceipt = strings.TrimSpace(hostedClaim.LostResponseReceipt)
	if hostedClaim.ClaimID == "" || hostedClaim.ClaimCode == "" || hostedClaim.ClaimToken == "" || hostedClaim.LostResponseReceipt == "" {
		return RemoteAccessClaim{}, "", errors.New("Hosted Services claim response missing claim credentials or response receipt")
	}
	if err := validateHostedClaimURL(settings.HostedBaseURL, hostedClaim.ClaimURL, hostedClaim.ClaimCode); err != nil {
		return RemoteAccessClaim{}, "", fmt.Errorf("Hosted Services claim URL is invalid: %w", err)
	}
	return RemoteAccessClaim{
		ClaimID:     hostedClaim.ClaimID,
		ClaimCode:   hostedClaim.ClaimCode,
		ClaimToken:  hostedClaim.ClaimToken,
		ClaimURL:    hostedClaim.ClaimURL,
		Status:      hostedClaim.Status,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:   hostedClaim.ExpiresAt,
		HostedReady: true,
		ServerName:  systemIdentity.FriendlyName,
	}, hostedClaim.LostResponseReceipt, nil
}

func validateHostedClaimURL(hostedBaseURL, rawClaimURL, claimCode string) error {
	expected, err := url.Parse(porticoHostedWebBaseURL(hostedBaseURL))
	if err != nil || expected.Scheme == "" || expected.Host == "" {
		return errors.New("expected Web origin is unavailable")
	}
	claimURL, err := url.Parse(strings.TrimSpace(rawClaimURL))
	if err != nil || claimURL.Scheme == "" || claimURL.Host == "" {
		return errors.New("claim URL is not absolute")
	}
	if claimURL.User != nil || claimURL.Fragment != "" || !sameHTTPOrigin(claimURL, expected) {
		return errors.New("claim URL origin is not trusted")
	}
	if claimURL.Path != "/claim" {
		return errors.New("claim URL path is not trusted")
	}
	query := claimURL.Query()
	codes, ok := query["code"]
	if !ok || len(codes) != 1 || strings.TrimSpace(codes[0]) != strings.TrimSpace(claimCode) {
		return errors.New("claim URL code does not match the response")
	}
	for key, values := range query {
		if key != "code" && key != "serverName" {
			return errors.New("claim URL contains an unexpected parameter")
		}
		if len(values) != 1 {
			return errors.New("claim URL contains duplicate parameters")
		}
	}
	return nil
}

func (s *Server) persistRemoteAccessClaimStart(claim RemoteAccessClaim, claimReceipt string, settings RemoteAccessSettings) error {
	claimReceipt = strings.TrimSpace(claimReceipt)
	if claimReceipt == "" {
		return errors.New("claim response receipt is missing")
	}
	if err := s.saveSecretSetting(remoteAccessClaimReceiptKey, claimReceipt); err != nil {
		return fmt.Errorf("store claim response receipt: %w", err)
	}
	if err := s.saveSecretSetting(remoteAccessClaimTokenKey, claim.ClaimToken); err != nil {
		_ = s.deleteSetting(remoteAccessClaimReceiptKey)
		return fmt.Errorf("store claim token: %w", err)
	}
	if err := s.saveRemoteAccessClaim(claim); err != nil {
		_ = s.deleteSetting(remoteAccessClaimTokenKey)
		_ = s.deleteSetting(remoteAccessClaimReceiptKey)
		return fmt.Errorf("store claim: %w", err)
	}
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		return fmt.Errorf("store claim settings: %w", err)
	}
	return nil
}

func (s *Server) linkPorticoOwnerProfile(localOwnerUserID string) error {
	if strings.TrimSpace(localOwnerUserID) == "" {
		return nil
	}
	members := s.listRemoteAccessMembers()
	for _, member := range members {
		if member.Status != "active" || member.Role != "owner" {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := s.execUserWrite(context.Background(), `
				UPDATE users
				SET portico_user_id = ?, portico_membership_id = ?, auth_origin = 'portico', updated_at = ?
				WHERE id = ? AND role = 'owner'`,
			member.PorticoUserID, member.PorticoMembershipID, now, localOwnerUserID); err != nil {
			return err
		}
		return s.mapRemoteAccessMember(member.PorticoMembershipID, localOwnerUserID)
	}
	return nil
}

func (s *Server) handleRemoteAccessClaimCancel(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	settings, settingsErr := s.remoteAccessSettings()
	if claim := s.currentRemoteAccessClaim(); claim != nil && claim.HostedReady && claim.Status == "pending" {
		claim.ClaimToken = s.secretSetting(remoteAccessClaimTokenKey)
		if settingsErr == nil {
			if err := s.cancelHostedClaim(r.Context(), settings, *claim); err != nil {
				s.recordLog("warn", "Remote access Hosted Services claim cancel failed", map[string]string{"error": err.Error(), "claimId": claim.ClaimID})
			}
		}
	}
	_ = s.deleteSetting(remoteAccessClaimKey)
	_ = s.deleteSetting(remoteAccessClaimTokenKey)
	_ = s.deleteSetting(remoteAccessClaimOperationKey)
	_ = s.deleteSetting(remoteAccessClaimReceiptKey)
	settings, err := s.remoteAccessSettings()
	if err == nil {
		settings.ClaimStatus = "not_claimed"
		_ = s.saveRemoteAccessSettings(settings)
	}
	s.recordAudit(r, user, "remote_access.claim_cancelled", "remote_access", "", "warn", nil)
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) cancelHostedClaim(ctx context.Context, settings RemoteAccessSettings, claim RemoteAccessClaim) error {
	if strings.TrimSpace(settings.HostedBaseURL) == "" || strings.TrimSpace(claim.ClaimID) == "" || strings.TrimSpace(claim.ClaimToken) == "" {
		return nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/server-claims/" + url.PathEscape(claim.ClaimID) + "/cancel"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+claim.ClaimToken)
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Hosted Services claim cancel returned %s", resp.Status)
	}
	return nil
}

func (s *Server) handleRemoteAccessUnclaim(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return
	}
	previousServerID := settings.ServerID
	// Hosted ownership is account authority and must be removed by an
	// account-authenticated client before it asks this server to forget its
	// local claim. A server credential can never delete its own Hosted record.
	settings.ClaimStatus = "not_claimed"
	settings.ServerID = ""
	settings.AssignedHostname = ""
	settings.CertificateStatus = "not_requested"
	settings.LastHeartbeatAt = ""
	_ = s.deleteSetting(remoteAccessClaimKey)
	_ = s.deleteSetting(remoteAccessClaimTokenKey)
	_ = s.deleteSetting(remoteAccessClaimOperationKey)
	_ = s.deleteSetting(remoteAccessClaimReceiptKey)
	_ = s.deleteSetting(remoteAccessCredentialKey)
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to save remote access settings.")
		return
	}
	s.recordAudit(r, user, "remote_access.unclaimed", "remote_access", previousServerID, "critical", nil)
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemoteAccessTestDirect(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return
	}
	result := "not_tested"
	if settings.PublicPortMode == "disabled" || !settings.Enabled {
		result = "disabled"
	} else if settings.ManualPublicPort <= 0 {
		result = "missing_public_port"
	} else if settings.PublicPortMode == "automatic" && settings.RouterAutomationEnabled {
		s.recordAudit(r, user, "remote_access.router_mapping_attempted", "remote_access", settings.ServerID, "warn", routerMappingAuditMetadata(settings, RouterMappingResult{Status: "attempted"}))
		mapping := s.applyRouterMapping(r.Context(), settings)
		settings.RouterMappingStatus = mapping.Status
		settings.LastRouterMappingAt = time.Now().UTC().Format(time.RFC3339)
		settings.RouterMappingError = mapping.Error
		s.recordAudit(r, user, "remote_access.router_mapping_completed", "remote_access", settings.ServerID, "warn", routerMappingAuditMetadata(settings, mapping))
		result = mapping.Status
		if mapping.Status == "mapped" {
			result = "router_mapping_active"
		}
	} else {
		result = "local_endpoint_ready"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	settings.LastReachabilityCheckAt = now
	settings.LastReachabilityResult = result
	_ = s.saveRemoteAccessSettings(settings)
	s.recordAudit(r, user, "remote_access.direct_test", "remote_access", settings.ServerID, "info", map[string]string{"result": result})
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemoteAccessCertificateRenew(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return
	}
	if settings.ServerID == "" || settings.AssignedHostname == "" || s.secretSetting(remoteAccessCredentialKey) == "" {
		writeError(w, http.StatusConflict, "server_not_claimed", "Remote access must be claimed before requesting certificates.")
		return
	}
	if _, err := s.ensureRemoteAccessCertificateFreshWithOptions(r.Context(), settings, remoteAccessCertificateOptions{Force: true}); err != nil {
		writeError(w, http.StatusBadGateway, "certificate_renew_failed", "Unable to renew certificate through Portico.")
		return
	}
	s.recordAudit(r, user, "remote_access.certificate_renewed", "remote_access", settings.ServerID, "warn", nil)
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemoteAccessLocalRoutes(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	writeJSON(w, http.StatusOK, ListResponse[RemoteAccessRoute]{Items: s.localRemoteAccessRoutes(), Total: len(s.localRemoteAccessRoutes())})
}

func (s *Server) handleRemoteAccessMemberRoute(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	memberID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/remote-access/members/"), "/")
	if memberID == "" || r.Method != http.MethodPatch {
		writeError(w, http.StatusNotFound, "not_found", "Remote access member route was not found.")
		return
	}
	var req struct {
		LocalUserID *string `json:"localUserId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.LocalUserID == nil {
		writeError(w, http.StatusBadRequest, "local_user_id_required", "A local user mapping is required.")
		return
	}
	localUserID := strings.TrimSpace(*req.LocalUserID)
	if localUserID != "" {
		target, err := s.getUser(localUserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "local_user_not_found", "Mapped local user was not found.")
			return
		}
		if target.Role == "owner" {
			memberRole := ""
			for _, member := range s.listRemoteAccessMembers() {
				if member.PorticoMembershipID == memberID || member.ID == memberID {
					memberRole = strings.TrimSpace(member.Role)
					break
				}
			}
			if memberRole != "owner" {
				writeError(w, http.StatusForbidden, "owner_mapping_forbidden", "Only the Hosted owner membership can map to the server owner.")
				return
			}
		}
	}
	if err := s.mapRemoteAccessMember(memberID, localUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "member_not_found", "Portico account member was not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "member_mapping_failed", "Unable to update Portico member mapping.")
		return
	}
	s.recordAudit(r, user, "remote_access.member_mapped", "remote_access", memberID, "warn", map[string]string{"localUserId": localUserID})
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemoteAccessPolicySync(w http.ResponseWriter, r *http.Request, user User) {
	if !s.requireManageServer(w, user) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	settings, credential, ok := s.remoteAccessPorticoCredential(w)
	if !ok {
		return
	}
	if err := s.syncRemoteAccessMembers(r.Context(), settings, credential); err != nil {
		writeError(w, http.StatusBadGateway, "portico_policy_sync_failed", "Unable to refresh Hosted membership policy.")
		return
	}
	s.recordAudit(r, user, "remote_access.policy_synced", "remote_access", settings.ServerID, "info", nil)
	status, _ := s.remoteAccessStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) remoteAccessPorticoCredential(w http.ResponseWriter) (RemoteAccessSettings, string, bool) {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load remote access settings.")
		return RemoteAccessSettings{}, "", false
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if !settings.Enabled || settings.ClaimStatus != "claimed" || strings.TrimSpace(settings.ServerID) == "" || strings.TrimSpace(credential) == "" {
		writeError(w, http.StatusConflict, "portico_not_connected", "This server is not connected to Portico account login.")
		return RemoteAccessSettings{}, "", false
	}
	return settings, credential, true
}

func (s *Server) currentPorticoUser(r *http.Request) (User, bool) {
	// Hosted bootstrap credentials are accepted only by the explicit attach
	// route together with their bound profile-selection envelope.
	return User{}, false
}

func porticoAccessTokenFromRequest(r *http.Request) (string, bool) {
	return bearerTokenFromRequest(r)
}

func bearerTokenFromRequest(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), true
	}
	return "", false
}

func (s *Server) porticoAttachmentForAccessToken(ctx context.Context, settings RemoteAccessSettings, accessToken string, selectionEnvelope HostedProfileSelectionEnvelope) (User, string, error) {
	if !strings.HasPrefix(accessToken, "ptc_clt_") {
		return User{}, "", errNativeSessionRevoked
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if credential == "" {
		return User{}, "", errors.New("server credential is missing")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/portico-sessions/introspect"
	payload, _ := json.Marshal(map[string]any{"accessToken": accessToken, "selectionEnvelope": selectionEnvelope})
	var response struct {
		Active            bool                           `json:"active"`
		Member            RemoteAccessMember             `json:"member"`
		DeviceID          string                         `json:"deviceId"`
		SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
	}
	if err := s.hostedJSON(requestCtx, http.MethodPost, endpoint, credential, payload, &response); err != nil {
		s.recordLog("warn", "Portico account session introspection failed", map[string]string{"error": err.Error()})
		return User{}, "", err
	}
	if !response.Active || strings.TrimSpace(response.DeviceID) == "" || response.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID ||
		response.SelectionEnvelope.ProfileID != selectionEnvelope.ProfileID {
		s.recordLog("warn", "Portico account session introspection returned inactive", nil)
		s.syncRemoteAccessMembersAfterPorticoAuthFailure(settings, credential)
		return User{}, "", errNativeSessionRevoked
	}
	memberID := response.Member.PorticoMembershipID
	if memberID == "" {
		memberID = response.Member.ID
	}
	if memberID == "" {
		s.recordLog("warn", "Portico account session introspection returned a member without an ID", nil)
		return User{}, "", errNativeMembershipInactive
	}
	if response.Member.PorticoMembershipID == "" {
		response.Member.PorticoMembershipID = response.Member.ID
	}
	if response.Member.PorticoUserID == "" {
		response.Member.PorticoUserID = response.Member.UserID
	}
	if response.Member.Status == "" {
		response.Member.Status = "active"
	}
	if response.Member.Status != "active" {
		s.syncRemoteAccessMembersAfterPorticoAuthFailure(settings, credential)
		return User{}, "", errNativeMembershipInactive
	}
	response.Member = normalizeRemoteAccessMemberProfileURL(settings.HostedBaseURL, response.Member)
	user, err := s.userForPorticoMembership(response.Member)
	if err != nil {
		s.recordLog("warn", "Portico account profile provisioning failed", map[string]string{"member": memberID, "error": err.Error()})
		return User{}, "", err
	}
	s.enrichUserAuthContext(&user, "portico")
	return user, strings.TrimSpace(response.DeviceID), nil
}

func (s *Server) syncRemoteAccessMembersAfterPorticoAuthFailure(settings RemoteAccessSettings, credential string) {
	if settings.ServerID == "" || credential == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := s.syncRemoteAccessMembers(ctx, settings, credential); err != nil {
		s.recordLog("warn", "Portico policy sync after Portico auth failure failed", map[string]string{"error": err.Error()})
	}
}

func (s *Server) remoteAccessStatus() (RemoteAccessStatus, error) {
	if err := s.reconcileRemoteAccessClaimActivation(); err != nil {
		return RemoteAccessStatus{}, err
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		return RemoteAccessStatus{}, err
	}
	if settings.CustomCertificateEnabled {
		previousStatus := settings.CertificateStatus
		previousExpiry := settings.CertificateExpiresAt
		previousError := settings.CertificateRenewalError
		if err := s.refreshCustomCertificateStatus(&settings); err != nil {
			settings.CertificateStatus = "custom_error"
			settings.CertificateRenewalError = err.Error()
		}
		if settings.CertificateStatus != previousStatus || settings.CertificateExpiresAt != previousExpiry || settings.CertificateRenewalError != previousError {
			_ = s.saveRemoteAccessSettings(settings)
		}
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return RemoteAccessStatus{}, err
	}
	if settings.ServerID != "" && settings.ClaimStatus == "claimed" {
		// Keep the claim record while its receipt is present. It is the durable
		// local handle needed to retry acknowledgement after a restart or a lost
		// acknowledgement response. The claim bearer is no longer useful after
		// activation and must not be used for result recovery.
		if s.secretSetting(remoteAccessClaimReceiptKey) == "" {
			_ = s.deleteSetting(remoteAccessClaimKey)
		}
		_ = s.deleteSetting(remoteAccessClaimTokenKey)
		_ = s.deleteSetting(remoteAccessClaimOperationKey)
	}
	claim := s.currentRemoteAccessClaim()
	pollState := s.remoteAccessClaimPollState()
	claimPollAttempted := false
	if claim != nil && claim.Status == "pending" {
		if expires, err := time.Parse(time.RFC3339, claim.ExpiresAt); err == nil && time.Now().UTC().After(expires) {
			claim.Status = "expired"
			settings.ClaimStatus = "expired"
			_ = s.saveRemoteAccessClaim(*claim)
			_ = s.saveRemoteAccessSettings(settings)
			_ = s.deleteSetting(remoteAccessClaimOperationKey)
		} else if claim.HostedReady {
			pollState.mu.Lock()
			// Another status request may have completed the claim while this one
			// waited. Re-read before contacting Hosted so polling is singleflight.
			current := s.currentRemoteAccessClaim()
			updatedClaim, updatedSettings, err := *claim, settings, error(nil)
			if current != nil && current.Status == "pending" &&
				(current.ClaimID != pollState.lastClaimID || time.Since(pollState.lastPollAt) >= time.Second) {
				claimPollAttempted = true
				updatedClaim, updatedSettings, err = s.pollHostedClaim(context.Background(), *current, settings)
				pollState.lastClaimID = current.ClaimID
				pollState.lastPollAt = time.Now()
			}
			if err == nil {
				pollState.lastErr = ""
			} else {
				// An expired or invalid receipt cannot be replayed. Preserve the
				// receipt for explicit cleanup, but retire the claim operation so a
				// subsequent start can create a fresh Hosted claim.
				if strings.Contains(strings.ToLower(err.Error()), "claim result receipt is invalid or expired") {
					updatedClaim.Status = "expired"
					updatedSettings.ClaimStatus = "expired"
					_ = s.saveRemoteAccessClaim(updatedClaim)
					_ = s.saveRemoteAccessSettings(updatedSettings)
					_ = s.deleteSetting(remoteAccessClaimOperationKey)
				}
				pollState.lastErr = remoteAccessClaimPollProblem(err)
			}
			// A successful local activation is reflected immediately even when the
			// Hosted acknowledgement still needs a retry.
			claim = &updatedClaim
			settings = updatedSettings
			pollState.mu.Unlock()
		}
	}
	if !claimPollAttempted && claim != nil && claim.Status == "claimed" && settings.ServerID != "" && s.secretSetting(remoteAccessClaimReceiptKey) != "" {
		pollState.mu.Lock()
		current := s.currentRemoteAccessClaim()
		if current != nil && current.Status == "claimed" {
			claim = current
			if err := s.acknowledgeHostedClaimResult(context.Background(), *current, settings); err != nil {
				pollState.lastErr = remoteAccessClaimPollProblem(err)
			} else {
				pollState.lastErr = ""
				s.finishRemoteAccessClaimActivation(context.Background(), *current, settings)
			}
		}
		pollState.mu.Unlock()
	}
	localTLSAddress, localTLSPort, localTLSError := s.remoteTLSStatus()
	members := s.listRemoteAccessMembers()
	return RemoteAccessStatus{
		Settings:                   settings,
		ServerPublicKey:            base64.RawStdEncoding.EncodeToString(identity.PublicKey),
		ServerPublicKeyFingerprint: identity.Fingerprint,
		LocalRoutes:                s.localRemoteAccessRoutes(),
		PublicEndpoint:             s.remotePublicEndpoint(settings),
		Connectivity:               remoteConnectivityStatus(settings),
		LocalTLSAddress:            localTLSAddress,
		LocalTLSPort:               localTLSPort,
		LocalTLSPortMatchesPublic:  localTLSPort > 0 && localTLSPort == settings.ManualPublicPort,
		LocalTLSError:              localTLSError,
		PorticoMembers:             members,
		PolicySync:                 remotePolicySyncStatus(settings, members, s.loadRemotePolicyState(), time.Now().UTC()),
		Claim:                      claim,
		PorticoConnected:           settings.ServerID != "" && settings.ClaimStatus == "claimed",
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339),
		Warning:                    s.remoteAccessClaimPollWarning(),
	}, nil
}

func (s *Server) remoteAccessClaimPollWarning() string {
	state := s.remoteAccessClaimPollState()
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.lastErr
}

func (s *Server) remoteAccessClaimPollState() *remoteAccessClaimPollState {
	state, _ := remoteAccessClaimPollStates.LoadOrStore(s, &remoteAccessClaimPollState{})
	return state.(*remoteAccessClaimPollState)
}

func remoteAccessClaimPollProblem(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "Portico Hosted Services did not answer the claim status check in time. The claim is still safe to retry."
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return "Portico Hosted Services is temporarily unreachable. The claim will be checked again."
	}
	lowerError := strings.ToLower(err.Error())
	if strings.Contains(lowerError, "claim receipt is missing") || strings.Contains(lowerError, "response receipt is missing") {
		return "This claim cannot be resumed because its local response receipt is missing. Start a new claim."
	}
	if strings.Contains(lowerError, "receipt is invalid or expired") {
		return "This claim cannot be resumed because its response receipt is invalid or expired. Start a new claim."
	}
	return "Portico could not refresh the claim status. The claim will be checked again."
}

func remotePolicySyncStatus(settings RemoteAccessSettings, members []RemoteAccessMember, policy remotePolicyState, now time.Time) RemotePolicySync {
	if settings.ServerID == "" || settings.ClaimStatus != "claimed" {
		return RemotePolicySync{Status: "local_only", Note: "This server is not connected to Portico, so Portico membership policy is not synced."}
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(policy.IssuedAt))
	if issuedErr != nil || strings.TrimSpace(policy.SnapshotID) == "" {
		return RemotePolicySync{Status: "missing", MemberCount: len(members), Stale: true, Note: "No Portico Account policy snapshot has been applied yet."}
	}
	continuity := remotePolicyContinuity(policy, now)
	stale := continuity != "valid"
	status := continuity
	note := "Portico Account membership policy has been applied to this server."
	switch continuity {
	case "grace":
		note = "Portico Account services are unavailable. Existing sessions have bounded access while policy reconciliation continues."
	case "hard-expired-draining":
		note = "Portico Account policy has expired. Only already-established playback may finish while reconciliation continues."
	case "hard-expired":
		note = "Portico Account policy has expired. Hosted-member access requires policy reconciliation."
	case "unknown":
		status = "missing"
		note = "Portico Account policy timing is unavailable and must be reconciled."
	}
	return RemotePolicySync{
		Status:       status,
		LastSyncedAt: issuedAt.UTC().Format(time.RFC3339),
		MemberCount:  len(members),
		Stale:        stale,
		Note:         note,
	}
}

func remoteConnectivityStatus(settings RemoteAccessSettings) RemoteConnectivity {
	hostedStatus := "not_connected"
	if settings.ServerID != "" && settings.ClaimStatus == "claimed" {
		hostedStatus = "unknown"
	}
	if settings.LastHeartbeatAt != "" && settings.LastHeartbeatError == "" {
		hostedStatus = "reachable"
	}
	if settings.LastHeartbeatError != "" {
		hostedStatus = "unreachable"
	}
	publicStatus := strings.TrimSpace(settings.LastReachabilityResult)
	if publicStatus == "" {
		publicStatus = "not_tested"
	}
	result := RemoteConnectivity{
		HostedServicesStatus:  hostedStatus,
		PublicRouteStatus:     publicStatus,
		LastCheckedAt:         settings.LastReachabilityCheckAt,
		TroubleshootingStatus: "unknown",
	}
	if settings.LastHeartbeatError != "" {
		result.PublicRouteError = settings.LastHeartbeatError
		result.TroubleshootingStatus = "hosted_unreachable"
		result.TroubleshootingHint = "This server cannot currently reach Portico Hosted Services. Check internet connectivity, DNS, firewall rules, or hosted service availability."
		return result
	}
	result.PublicRouteError = settings.LastPublicRouteError
	switch publicStatus {
	case "public_reachable", "reachable", "hosted_enabled":
		result.TroubleshootingStatus = "ok"
		result.TroubleshootingHint = "Hosted Services can reach this server's public route."
	case "public_unreachable", "public_http_failed", "public_tls_failed", "public_failed":
		result.TroubleshootingStatus = "port_closed"
		result.TroubleshootingHint = "This server can reach Hosted Services, but Hosted Services cannot reach the server's public route. Check port forwarding, router mapping, firewall rules, and ISP carrier-grade NAT."
	case "public_checking", "checking", "dns_synced":
		result.TroubleshootingStatus = "checking"
		result.TroubleshootingHint = "Hosted Services has the server heartbeat and is checking the public route."
	case "public_missing":
		result.TroubleshootingStatus = "public_route_missing"
		result.TroubleshootingHint = "Hosted Services has not received a usable public route for this server yet."
	default:
		if strings.HasPrefix(publicStatus, "repair_") {
			result.TroubleshootingStatus = "checking"
			result.TroubleshootingHint = "Portico is confirming the hosted route with Hosted Services."
		}
	}
	return result
}

func (s *Server) pollHostedClaim(ctx context.Context, claim RemoteAccessClaim, settings RemoteAccessSettings) (RemoteAccessClaim, RemoteAccessSettings, error) {
	claimReceipt := strings.TrimSpace(s.secretSetting(remoteAccessClaimReceiptKey))
	if claimReceipt == "" {
		return claim, settings, errors.New("claim receipt is missing")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/server-claims/" + url.PathEscape(claim.ClaimID) + "/result"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return claim, settings, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Portico-Claim-Receipt", claimReceipt)
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return claim, settings, err
	}
	defer resp.Body.Close()
	responseBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return claim, settings, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone {
			return claim, settings, fmt.Errorf("Hosted Services claim result receipt is invalid or expired (%s)", resp.Status)
		}
		return claim, settings, fmt.Errorf("Hosted Services claim poll returned %s", resp.Status)
	}
	var result struct {
		Status           string `json:"status"`
		ServerCredential string `json:"serverCredential"`
		Server           struct {
			ID                string `json:"id"`
			AssignedHostname  string `json:"assignedHostname"`
			CertificateStatus string `json:"certificateStatus"`
			PreferredAuthMode string `json:"preferredAuthMode"`
		} `json:"server"`
	}
	if err := json.Unmarshal(responseBytes, &result); err != nil {
		return claim, settings, err
	}
	if result.Status == "" {
		return claim, settings, errors.New("Hosted Services claim poll missing status")
	}
	claim.Status = result.Status
	settings.ClaimStatus = result.Status
	if result.Status == "claimed" {
		if result.Server.ID == "" || result.ServerCredential == "" {
			return claim, settings, errors.New("claimed server response missing server credential")
		}
		settings.ServerID = result.Server.ID
		settings.AssignedHostname = result.Server.AssignedHostname
		if result.Server.CertificateStatus != "" {
			settings.CertificateStatus = result.Server.CertificateStatus
		}
		if result.Server.PreferredAuthMode != "" {
			settings.PreferredRemoteAuthMode = result.Server.PreferredAuthMode
		}
		activation := remoteAccessClaimActivation{Claim: claim, Settings: settings, ServerCredential: result.ServerCredential}
		if err := s.saveRemoteAccessClaimActivation(activation); err != nil {
			return claim, settings, err
		}
		if err := s.applyRemoteAccessClaimActivation(activation); err != nil {
			return claim, settings, err
		}
		if err := s.acknowledgeHostedClaimResult(ctx, claim, settings); err != nil {
			return claim, settings, fmt.Errorf("acknowledge Hosted Services claim result: %w", err)
		}
		s.finishRemoteAccessClaimActivation(ctx, claim, settings)
		return claim, settings, nil
	}
	if err := s.saveRemoteAccessClaim(claim); err != nil {
		return claim, settings, err
	}
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		return claim, settings, err
	}
	return claim, settings, nil
}

func (s *Server) finishRemoteAccessClaimActivation(ctx context.Context, claim RemoteAccessClaim, settings RemoteAccessSettings) {
	if ctx == nil {
		ctx = context.Background()
	}
	if credential := s.secretSetting(remoteAccessCredentialKey); credential != "" {
		syncCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		if err := s.syncRemoteAccessMembers(syncCtx, settings, credential); err != nil {
			s.recordLog("warn", "Portico account member sync after claim failed", map[string]string{"error": err.Error()})
		}
		cancel()
	}
	_ = s.linkClaimingOwnerProfile(claim)
	s.startRemoteAccessPostClaimProvisioning(settings)
}

func (s *Server) acknowledgeHostedClaimResult(ctx context.Context, claim RemoteAccessClaim, settings RemoteAccessSettings) error {
	claimReceipt := strings.TrimSpace(s.secretSetting(remoteAccessClaimReceiptKey))
	if claimReceipt == "" {
		return errors.New("claim receipt is missing")
	}
	if strings.TrimSpace(claim.ClaimID) == "" {
		return errors.New("claim id is missing")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/server-claims/" + url.PathEscape(claim.ClaimID) + "/result/ack"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Portico-Claim-Receipt", claimReceipt)
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := s.deleteSetting(remoteAccessClaimReceiptKey); err != nil {
			return fmt.Errorf("remove acknowledged claim response receipt: %w", err)
		}
		return nil
	}

	// The acknowledgement may have committed while its response was lost. A
	// 410 result with acknowledged=true is durable Hosted evidence that it is
	// safe to remove the local receipt; every other failure keeps the receipt
	// for a later retry.
	acknowledged, probeErr := s.hostedClaimResultAcknowledged(ctx, claim, settings, claimReceipt)
	if probeErr == nil && acknowledged {
		if err := s.deleteSetting(remoteAccessClaimReceiptKey); err != nil {
			return fmt.Errorf("remove acknowledged claim response receipt: %w", err)
		}
		return nil
	}
	if probeErr != nil && len(responseBytes) == 0 {
		return fmt.Errorf("Hosted Services claim acknowledgement returned %s: %w", resp.Status, probeErr)
	}
	return fmt.Errorf("Hosted Services claim acknowledgement returned %s", resp.Status)
}

func (s *Server) hostedClaimResultAcknowledged(ctx context.Context, claim RemoteAccessClaim, settings RemoteAccessSettings, claimReceipt string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/server-claims/" + url.PathEscape(claim.ClaimID) + "/result"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Portico-Claim-Receipt", claimReceipt)
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	responseBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusGone {
		return false, fmt.Errorf("Hosted Services claim result probe returned %s", resp.Status)
	}
	var result struct {
		Acknowledged bool `json:"acknowledged"`
	}
	if err := json.Unmarshal(responseBytes, &result); err != nil {
		return false, err
	}
	return result.Acknowledged, nil
}

func (s *Server) saveRemoteAccessClaimActivation(activation remoteAccessClaimActivation) error {
	bytes, err := json.Marshal(activation)
	if err != nil {
		return err
	}
	return s.saveSecretSetting(remoteAccessClaimActivationKey, string(bytes))
}

func (s *Server) reconcileRemoteAccessClaimActivation() error {
	all, err := s.loadSettings()
	if err != nil {
		return err
	}
	if _, ok := all[remoteAccessClaimActivationKey]; !ok {
		return nil
	}
	plaintext, err := s.secretSettingWithError(remoteAccessClaimActivationKey)
	if err != nil {
		// Preserve the journal when its envelope cannot be opened. Removing it
		// would turn a recoverable key/provider outage into credential loss.
		return err
	}
	var activation remoteAccessClaimActivation
	if err := json.Unmarshal([]byte(plaintext), &activation); err != nil {
		return err
	}
	if activation.Claim.Status != "claimed" || activation.Settings.ServerID == "" || activation.ServerCredential == "" {
		return errors.New("remote access claim activation journal is invalid")
	}
	return s.applyRemoteAccessClaimActivation(activation)
}

func (s *Server) applyRemoteAccessClaimActivation(activation remoteAccessClaimActivation) error {
	// The journal is written first and removed last. Every operation here is
	// idempotent, so a restart at any boundary safely finishes activation.
	if err := s.saveSecretSetting(remoteAccessCredentialKey, activation.ServerCredential); err != nil {
		return err
	}
	if err := s.saveRemoteAccessClaim(activation.Claim); err != nil {
		return err
	}
	if err := s.saveRemoteAccessSettings(activation.Settings); err != nil {
		return err
	}
	if err := s.deleteSetting(remoteAccessClaimTokenKey); err != nil {
		return err
	}
	if err := s.deleteSetting(remoteAccessClaimOperationKey); err != nil {
		return err
	}
	return s.deleteSetting(remoteAccessClaimActivationKey)
}

func (s *Server) retireRemoteAccessClaim() error {
	if claim := s.currentRemoteAccessClaim(); claim != nil && claim.Status == "claimed" && s.secretSetting(remoteAccessClaimReceiptKey) != "" {
		return errors.New("claim result acknowledgement is still pending")
	}
	for _, key := range []string{remoteAccessClaimKey, remoteAccessClaimTokenKey, remoteAccessClaimOperationKey, remoteAccessClaimReceiptKey} {
		if err := s.deleteSetting(key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) startRemoteAccessPostClaimProvisioning(settings RemoteAccessSettings) {
	if s.backgroundCtx == nil || !settings.Enabled || settings.ServerID == "" {
		return
	}
	s.startOwnedAsync("remote-access-post-claim", func(ctx context.Context) {
		if err := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{}); err != nil {
			s.recordLog("warn", "Remote access heartbeat after claim failed", map[string]string{"error": err.Error()})
		} else if refreshed, err := s.remoteAccessSettings(); err == nil {
			settings = refreshed
		}
		updated, err := s.ensureRemoteAccessCertificateFresh(ctx, settings)
		if err != nil {
			s.recordLog("warn", "Remote access certificate provisioning after claim failed", map[string]string{"error": err.Error()})
			return
		}
		settings = updated
		s.configureRemoteTLS(settings)
		if err := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{}); err != nil {
			s.recordLog("warn", "Remote access certificate publication after claim failed", map[string]string{"error": err.Error()})
		}
	})
}

func (s *Server) finishPorticoSetupActivation(ctx context.Context, status RemoteAccessStatus) error {
	if !status.PorticoConnected || status.Settings.ServerID == "" {
		return errors.New("Portico server claim is not complete")
	}
	if err := s.ensurePorticoSetupOwnerProfile(); err == nil {
		return nil
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if credential == "" {
		return errors.New("Portico server credential is unavailable")
	}
	syncCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := s.syncRemoteAccessMembers(syncCtx, status.Settings, credential); err != nil {
		return fmt.Errorf("sync Portico owner membership: %w", err)
	}
	if err := s.ensurePorticoSetupOwnerProfile(); err != nil {
		return fmt.Errorf("create Portico owner profile: %w", err)
	}
	return nil
}

func (s *Server) linkClaimingOwnerProfile(claim RemoteAccessClaim) error {
	if strings.TrimSpace(claim.LocalOwnerUserID) == "" {
		return nil
	}
	members := s.listRemoteAccessMembers()
	for _, member := range members {
		if member.Status != "active" || member.Role != "owner" {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.execUserWrite(context.Background(), `
				UPDATE users
				SET portico_user_id = ?, portico_membership_id = ?, auth_origin = 'portico', updated_at = ?
				WHERE id = ? AND role = 'owner'`,
			member.PorticoUserID, member.PorticoMembershipID, now, claim.LocalOwnerUserID)
		if err != nil {
			return err
		}
		return s.mapRemoteAccessMember(member.PorticoMembershipID, claim.LocalOwnerUserID)
	}
	return nil
}

func (s *Server) ensurePorticoSetupOwnerProfile() error {
	var ownerCount int
	if err := s.queryUserRow(context.Background(), `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&ownerCount); err != nil {
		return err
	}
	if ownerCount > 0 {
		return nil
	}
	members := s.listRemoteAccessMembers()
	for _, member := range members {
		if member.Status != "active" || member.Role != "owner" || member.PorticoUserID == "" || member.PorticoMembershipID == "" {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		permissionsJSON, _ := json.Marshal(ownerPermissions())
		preferencesJSON := remoteMemberPreferencesJSON(member)
		var existingID string
		err := s.queryUserRow(context.Background(), `
				SELECT id
				FROM users
			WHERE (portico_membership_id = ? OR portico_user_id = ?) AND auth_origin = 'portico'
			ORDER BY CASE WHEN portico_membership_id = ? THEN 0 ELSE 1 END
			LIMIT 1`,
			member.PorticoMembershipID, member.PorticoUserID, member.PorticoMembershipID).Scan(&existingID)
		if err == nil {
			if _, err := s.execUserWrite(context.Background(), `
						UPDATE users
						SET email = ?, display_name = ?, profile_image_url = ?, role = 'owner', permissions_json = ?, preferences_json = ?, max_content_rating = '', updated_at = ?
						WHERE id = ?`,
				strings.ToLower(strings.TrimSpace(member.Email)), porticoDisplayName(member), strings.TrimSpace(member.ProfileImageURL), string(permissionsJSON), preferencesJSON, now, existingID); err != nil {
				return err
			}
			if _, err := s.execUserWrite(context.Background(), `UPDATE remote_access_members SET local_user_id = ? WHERE portico_membership_id = ?`, existingID, member.PorticoMembershipID); err != nil {
				return err
			}
			return s.seedUserMediaState(existingID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		userID := randomID("usr")
		email := strings.ToLower(strings.TrimSpace(member.Email))
		if email == "" {
			email = member.PorticoUserID + "@portico-account.invalid"
		}
		err = s.withUserTxTagged(context.Background(), []string{"settings"}, func(tx *sql.Tx) error {
			if err := releaseDisposablePorticoEmailCollisionTx(tx, email, "", member, now); err != nil {
				return err
			}
			username := uniquePorticoAccountUsername(tx, member)
			displayName := porticoDisplayName(member)
			if _, err := tx.Exec(`
					INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, profile_image_url, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
					VALUES (?, ?, ?, ?, NULL, 'owner', 'portico', ?, ?, ?, ?, ?, '', ?, ?)`,
				userID, username, email, displayName, member.PorticoUserID, member.PorticoMembershipID, strings.TrimSpace(member.ProfileImageURL), string(permissionsJSON), preferencesJSON, now, now); err != nil {
				return err
			}
			_, err := tx.Exec(`UPDATE remote_access_members SET local_user_id = ? WHERE portico_membership_id = ?`, userID, member.PorticoMembershipID)
			return err
		})
		if err != nil {
			return err
		}
		return s.seedUserMediaState(userID)
	}
	return errors.New("active Portico owner membership is not available")
}

func (s *Server) sendRemoteAccessHeartbeat(ctx context.Context, settings RemoteAccessSettings) error {
	return s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{SyncPolicy: true})
}

type remoteAccessHeartbeatOptions struct {
	SyncPolicy     bool
	SuppressRepair bool
}

type remoteAccessCertificateOptions struct {
	Force bool
}

type remoteAccessRepairSignal struct {
	RepairRequested         bool   `json:"repairRequested"`
	Reason                  string `json:"reason"`
	Status                  string `json:"status"`
	RouteType               string `json:"routeType"`
	Host                    string `json:"host"`
	LastRequestedAt         string `json:"lastRequestedAt"`
	HostedServicesReachable bool   `json:"hostedServicesReachable"`
	PublicRouteStatus       string `json:"publicRouteStatus"`
	PublicRouteError        string `json:"publicRouteError"`
	PublicRouteCheckedAt    string `json:"publicRouteCheckedAt"`
	PublicRouteHost         string `json:"publicRouteHost"`
}

func (s *Server) sendRemoteAccessHeartbeatWithOptions(ctx context.Context, settings RemoteAccessSettings, options remoteAccessHeartbeatOptions) error {
	if settings.ServerID == "" {
		return errors.New("server is not claimed")
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if credential == "" {
		return errors.New("server credential is missing")
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return err
	}
	serverName := ""
	if systemIdentity, identityErr := s.systemIdentityContext(ctx); identityErr == nil {
		serverName = systemIdentity.FriendlyName
	}
	compatibility := s.compatibilityEnvelope()
	policyState := s.loadRemotePolicyState()
	policyDigest := ""
	if digest, digestErr := normalizedSHA256Digest(policyState.PolicyDigest); digestErr == nil {
		policyDigest = digest
	}
	payload := map[string]any{
		"serverId":                      settings.ServerID,
		"serverName":                    serverName,
		"serverPublicKeyFingerprint":    identity.Fingerprint,
		"softwareVersion":               compatibility.Build.Version,
		"buildNumber":                   compatibility.Build.Number,
		"buildChannel":                  compatibility.Build.Channel,
		"buildCommit":                   compatibility.Build.Commit,
		"buildTimestamp":                compatibility.Build.Timestamp,
		"compatibilityEnvelopeRevision": compatibility.EnvelopeRevision,
		"protocolMinimum":               compatibility.SupportedClientProtocol.Minimum,
		"protocolMaximum":               compatibility.SupportedClientProtocol.Maximum,
		"apiContractDigestAlgorithm":    compatibility.APIContract.DigestAlgorithm,
		"apiContractIdentity":           compatibility.APIContract.Identity,
		"apiContractDigest":             compatibility.APIContract.Digest,
		"semanticRevisions":             compatibility.SemanticRevisions,
		"requiredSemantics":             compatibility.RequiredSemantics,
		"forwardCompatibility":          compatibility.ForwardCompatibility,
		"capabilities":                  compatibility.Capabilities,
		"publicPort":                    settings.ManualPublicPort,
		"publicIpCandidate":             settings.LastPublicIPAddress,
		"certificateStatus":             settings.CertificateStatus,
		"certificateExpiresAt":          settings.CertificateExpiresAt,
		"remoteAccessEnabled":           settings.Enabled,
		"preferredAuthMode":             settings.PreferredRemoteAuthMode,
		"lastReachabilityTestResult":    settings.LastReachabilityResult,
		"lanEndpointCandidates":         s.lanEndpointCandidates(settings),
		"policyDigest":                  policyDigest,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/heartbeat"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &hostedHTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			RetryAfter: parseHostedRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}
	var response struct {
		AssignedHostname    string                    `json:"assignedHostname"`
		RemoteAccessEnabled *bool                     `json:"remoteAccessEnabled,omitempty"`
		PublicIP            string                    `json:"publicIp,omitempty"`
		LeaseSeconds        int64                     `json:"leaseSeconds,omitempty"`
		RepairPollSeconds   int64                     `json:"repairPollSeconds,omitempty"`
		StateChanged        bool                      `json:"stateChanged,omitempty"`
		TopologyChanged     bool                      `json:"topologyChanged,omitempty"`
		Repair              *remoteAccessRepairSignal `json:"repair,omitempty"`
		PolicyDigest        string                    `json:"policyDigest,omitempty"`
		PolicyChanged       bool                      `json:"policyChanged,omitempty"`
	}
	if len(bytes.TrimSpace(responseBytes)) > 0 {
		if err := json.Unmarshal(responseBytes, &response); err != nil {
			return err
		}
	}
	if response.AssignedHostname != "" && response.AssignedHostname != settings.AssignedHostname {
		settings.AssignedHostname = response.AssignedHostname
	}
	if response.LeaseSeconds >= 60 && response.LeaseSeconds <= 3600 {
		s.remoteAccessLeaseSeconds.Store(response.LeaseSeconds)
	}
	if response.RepairPollSeconds >= 60 && response.RepairPollSeconds <= 1800 {
		s.remoteAccessRepairPollSeconds.Store(response.RepairPollSeconds)
	}
	if response.StateChanged || response.TopologyChanged {
		s.recordLog("info", "Hosted remote access state changed", map[string]string{
			"stateChanged":    strconv.FormatBool(response.StateChanged),
			"topologyChanged": strconv.FormatBool(response.TopologyChanged),
		})
	}
	if response.TopologyChanged && settings.Enabled {
		// Hosted reachability is asynchronous. Do not keep presenting the
		// result for the superseded route while the new topology is checked.
		settings.LastReachabilityResult = "public_checking"
		settings.LastReachabilityCheckAt = time.Now().UTC().Format(time.RFC3339)
		settings.LastPublicRouteError = ""
	}
	if publicIP := validPublicIPString(response.PublicIP); publicIP != "" {
		now := time.Now().UTC().Format(time.RFC3339)
		if settings.LastPublicIPAddress != "" && settings.LastPublicIPAddress != publicIP {
			settings.LastRouteRepairAt = now
			settings.LastReachabilityCheckAt = now
			// Preserve the more actionable local topology-change cause when this
			// heartbeat was sent by the local network monitor. Otherwise record the
			// public-address change first observed by Hosted Services.
			if settings.LastRouteRepairReason != "network_changed" {
				settings.LastRouteRepairReason = "hosted_public_ip_changed"
				settings.LastReachabilityResult = "repair_hosted_public_ip_changed"
			}
		}
		settings.LastPublicIPAddress = publicIP
		settings.LastPublicIPCheckAt = now
	}
	if response.RemoteAccessEnabled != nil {
		if *response.RemoteAccessEnabled {
			settings.LastHostedRemoteAccessState = "enabled"
		} else {
			settings.LastHostedRemoteAccessState = "disabled"
		}
		if *response.RemoteAccessEnabled != settings.Enabled {
			settings.LastReachabilityCheckAt = time.Now().UTC().Format(time.RFC3339)
			if *response.RemoteAccessEnabled {
				settings.LastReachabilityResult = "hosted_enabled"
			} else {
				settings.LastReachabilityResult = "hosted_disabled"
			}
		}
	}
	if response.Repair != nil {
		updateRemoteAccessPublicRouteDiagnostics(&settings, response.Repair.PublicRouteStatus, response.Repair.PublicRouteError, response.Repair.PublicRouteCheckedAt)
	}
	settings.LastHeartbeatAt = time.Now().UTC().Format(time.RFC3339)
	settings.LastHeartbeatError = ""
	if settings.LastHostedRemoteAccessState == "" {
		settings.LastHostedRemoteAccessState = "unknown"
	}
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		return err
	}
	if response.Repair != nil && response.Repair.RepairRequested && !options.SuppressRepair {
		signal := *response.Repair
		s.startOwnedAsync("remote-access-heartbeat-repair", func(ctx context.Context) {
			current, loadErr := s.remoteAccessSettings()
			if loadErr != nil {
				s.recordLog("warn", "Remote access heartbeat repair state reload failed", map[string]string{"error": loadErr.Error()})
				return
			}
			if _, repairErr := s.handleRemoteAccessRepairSignal(ctx, current, signal, false); repairErr != nil {
				s.recordLog("warn", "Remote access heartbeat repair failed", map[string]string{"error": repairErr.Error()})
			}
		})
	}
	now := time.Now().UTC()
	responsePolicyDigest, responseDigestErr := normalizedSHA256Digest(response.PolicyDigest)
	hasAuthoritativePolicyDigest := responseDigestErr == nil
	localPolicyAbsent := strings.TrimSpace(policyState.SnapshotID) == "" || policyDigest == ""
	policyRenewalDue := remotePolicyRenewalDue(policyState, now)
	knownDigestSync := hasAuthoritativePolicyDigest && (response.PolicyChanged || !strings.EqualFold(responsePolicyDigest, policyDigest) || policyRenewalDue)
	unknownDigestSync := !hasAuthoritativePolicyDigest && (localPolicyAbsent || policyRenewalDue) && s.claimUnknownPolicySyncAttempt(now)
	shouldSyncPolicy := options.SyncPolicy && (knownDigestSync || unknownDigestSync)
	if shouldSyncPolicy {
		s.startOwnedAsync("remote-policy-heartbeat-sync", func(ctx context.Context) {
			syncCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			if err := s.syncRemoteAccessMembers(syncCtx, settings, credential); err != nil {
				s.recordLog("warn", "Portico account member sync after heartbeat failed", map[string]string{"error": err.Error()})
			}
		})
	} else if options.SyncPolicy && policyState.AckPending {
		s.startOwnedAsync("remote-policy-heartbeat-ack", func(ctx context.Context) {
			ackCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			if err := s.retryRemotePolicyAck(ackCtx, settings, credential); err != nil {
				s.recordLog("warn", "Portico policy snapshot ack retry failed", map[string]string{"snapshotId": policyState.SnapshotID, "error": err.Error()})
			}
		})
	}
	return nil
}

func (s *Server) claimUnknownPolicySyncAttempt(now time.Time) bool {
	const cooldown = 5 * time.Minute
	for {
		next := s.remotePolicyNextUnknownSyncUnix.Load()
		if next > now.Unix() {
			return false
		}
		if s.remotePolicyNextUnknownSyncUnix.CompareAndSwap(next, now.Add(cooldown).Unix()) {
			return true
		}
	}
}

func (s *Server) syncRemoteAccessMembers(ctx context.Context, settings RemoteAccessSettings, credential string) error {
	if settings.ServerID == "" || credential == "" {
		return nil
	}
	return s.syncRemoteAccessPolicySnapshot(ctx, settings, credential)
}

func (s *Server) syncRemoteAccessPolicySnapshot(ctx context.Context, settings RemoteAccessSettings, credential string) error {
	s.remotePolicySyncMu.Lock()
	defer s.remotePolicySyncMu.Unlock()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/policy-snapshot"
	var raw json.RawMessage
	if err := s.hostedJSON(ctx, http.MethodGet, endpoint, credential, nil, &raw); err != nil {
		return err
	}
	var snapshot RemotePolicySnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	if err := s.ensureHostedDocumentKey(ctx, settings.HostedBaseURL, snapshot.SignatureKeyID); err != nil {
		return fmt.Errorf("Hosted Services policy signing key was unavailable: %w", err)
	}
	if err := s.verifyHostedPolicySnapshot(raw, snapshot, settings.ServerID, time.Now().UTC()); err != nil {
		return fmt.Errorf("Hosted Services policy snapshot was rejected: %w", err)
	}
	if err := validateRemotePolicyMembershipAuthority(snapshot.Members); err != nil {
		return fmt.Errorf("Hosted Services policy snapshot authority was rejected: %w", err)
	}
	snapshotDigest, err := normalizedSHA256Digest(snapshot.Digest)
	if err != nil {
		return fmt.Errorf("Hosted Services policy snapshot digest was rejected: %w", err)
	}
	policyDigest, err := normalizedSHA256Digest(snapshot.PolicyDigest)
	if err != nil {
		return fmt.Errorf("Hosted Services policy snapshot revision was rejected: %w", err)
	}
	if err := s.replaceRemoteAccessMembers(normalizeRemoteAccessMemberProfileURLs(settings.HostedBaseURL, snapshot.Members)); err != nil {
		return err
	}
	if err := s.applyRemoteDeletedAccountTombstones(snapshot.DeletedAccountTombstones); err != nil {
		return err
	}
	policyState := remotePolicyState{
		SnapshotID: snapshot.SnapshotID, SnapshotDigest: snapshotDigest, Generation: snapshot.Generation,
		IssuedAt: snapshot.IssuedAt, ExpiresAt: snapshot.ExpiresAt, PolicyDigest: policyDigest, AckPending: true,
	}
	if err := s.saveRemotePolicyState(policyState); err != nil {
		return err
	}
	if err := s.ackRemotePolicyState(ctx, settings, credential, policyState); err != nil {
		s.recordLog("warn", "Portico policy snapshot ack failed", map[string]string{"snapshotId": snapshot.SnapshotID, "error": err.Error()})
		return nil
	}
	policyState.AckPending = false
	return s.saveRemotePolicyState(policyState)
}

func (s *Server) retryRemotePolicyAck(ctx context.Context, settings RemoteAccessSettings, credential string) error {
	s.remotePolicySyncMu.Lock()
	defer s.remotePolicySyncMu.Unlock()
	state := s.loadRemotePolicyState()
	if !state.AckPending {
		return nil
	}
	if err := s.ackRemotePolicyState(ctx, settings, credential, state); err != nil {
		return err
	}
	state.AckPending = false
	return s.saveRemotePolicyState(state)
}

func (s *Server) ackRemotePolicyState(ctx context.Context, settings RemoteAccessSettings, credential string, state remotePolicyState) error {
	if strings.TrimSpace(state.SnapshotID) == "" {
		return errors.New("policy snapshot ID is missing")
	}
	snapshotDigest, err := normalizedSHA256Digest(state.SnapshotDigest)
	if err != nil {
		return err
	}
	ackBody, _ := json.Marshal(map[string]string{
		"snapshotId": state.SnapshotID,
		"digest":     snapshotDigest,
		"status":     "applied",
	})
	ackEndpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/policy-sync-ack"
	return s.hostedJSON(ctx, http.MethodPost, ackEndpoint, credential, ackBody, nil)
}

func validateRemotePolicyMembershipAuthority(members []RemoteAccessMember) error {
	activeOwners := 0
	membershipIDs := map[string]bool{}
	activeUserIDs := map[string]bool{}
	for _, member := range members {
		role := strings.TrimSpace(member.Role)
		if role != "owner" && role != "user" {
			return fmt.Errorf("membership %q has unsupported role %q", member.PorticoMembershipID, role)
		}
		if err := validateUserGrantablePermissions(member.PermissionTemplate.Permissions); err != nil {
			return fmt.Errorf("membership %q has invalid permissions: %w", member.PorticoMembershipID, err)
		}
		membershipID := strings.TrimSpace(firstNonEmpty(member.PorticoMembershipID, member.ID))
		userID := strings.TrimSpace(firstNonEmpty(member.PorticoUserID, member.UserID))
		if membershipID == "" || userID == "" || membershipIDs[membershipID] {
			return errors.New("membership identities must be present and unique")
		}
		membershipIDs[membershipID] = true
		if member.Status != "active" {
			if role == "owner" {
				return errors.New("the owner membership must remain active")
			}
			continue
		}
		if activeUserIDs[userID] {
			return errors.New("an active account may have only one membership")
		}
		activeUserIDs[userID] = true
		if role == "owner" {
			activeOwners++
		}
	}
	if activeOwners != 1 {
		return fmt.Errorf("policy snapshot must contain exactly one active owner membership, found %d", activeOwners)
	}
	return nil
}

func (s *Server) applyRemoteDeletedAccountTombstones(tombstones []RemoteDeletedAccountTombstone) error {
	if len(tombstones) == 0 {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		accountIDs, err := s.remoteDeletedAccountIDsContext(context.Background(), tombstones)
		if err != nil {
			return err
		}
		handles, err := s.beginAccountRuntimeErasuresContext(context.Background(), accountIDs)
		if err != nil {
			return err
		}
		fenced := map[string]bool{}
		for _, accountID := range accountIDs {
			fenced[accountID] = true
		}
		err = s.applyRemoteDeletedAccountTombstonesFenced(tombstones, fenced)
		finishProfileErasureFences(handles)
		if errors.Is(err, errRemoteAuthorityFenceRetry) {
			continue
		}
		return err
	}
	return errors.New("remote account tombstones changed repeatedly while applying authority revocation")
}

var errRemoteAuthorityFenceRetry = errors.New("remote authority revocation requires a runtime fence retry")

func (s *Server) remoteDeletedAccountIDsContext(ctx context.Context, tombstones []RemoteDeletedAccountTombstone) ([]string, error) {
	ids := []string{}
	for _, tombstone := range tombstones {
		porticoUserID := strings.TrimSpace(tombstone.UserID)
		if porticoUserID == "" {
			continue
		}
		rows, err := s.queryUserRead(ctx, `SELECT id FROM users WHERE portico_user_id = ? ORDER BY id`, porticoUserID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *Server) applyRemoteDeletedAccountTombstonesFenced(tombstones []RemoteDeletedAccountTombstone, fenced map[string]bool) error {
	return s.withBackgroundTxTagged(context.Background(), []string{"remote-access", "portico-members", "users", "profiles", "account"}, func(tx *sql.Tx) error {
		for _, tombstone := range tombstones {
			porticoUserID := strings.TrimSpace(tombstone.UserID)
			if porticoUserID == "" {
				continue
			}
			rows, err := tx.Query(`SELECT id FROM users WHERE portico_user_id = ?`, porticoUserID)
			if err != nil {
				return err
			}
			var localUserIDs []string
			for rows.Next() {
				var localUserID string
				if err := rows.Scan(&localUserID); err != nil {
					rows.Close()
					return err
				}
				localUserIDs = append(localUserIDs, localUserID)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			for _, localUserID := range localUserIDs {
				if !fenced[localUserID] {
					return errRemoteAuthorityFenceRetry
				}
				now := time.Now().UTC().Format(time.RFC3339Nano)
				if err := s.revokeAccountAuthorityTx(context.Background(), tx, localUserID, now); err != nil {
					return err
				}
				deletedEmail, err := uniqueDeletedPorticoPrincipalEmailTx(tx, porticoUserID, localUserID)
				if err != nil {
					return err
				}
				deletedUsername, err := uniqueDeletedPorticoPrincipalUsernameTx(tx, porticoUserID, localUserID)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(`
					UPDATE users
					SET auth_origin = 'portico_deleted',
						portico_user_id = '',
						portico_membership_id = '',
						email = ?,
						username = ?,
						password_hash = NULL,
						disabled_at = CASE WHEN disabled_at = '' THEN ? ELSE disabled_at END,
						updated_at = ?
					WHERE id = ?`, deletedEmail, deletedUsername, now, now, localUserID); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`UPDATE remote_access_members SET status = 'deleted', local_user_id = '', last_synced_at = ? WHERE portico_user_id = ?`,
				time.Now().UTC().Format(time.RFC3339), porticoUserID); err != nil {
				return err
			}
		}
		return nil
	})
}

func deletedPorticoPrincipalEmail(porticoUserID, localUserID string) string {
	parts := []string{}
	if sanitized := sanitizePorticoCacheIdentifier(porticoUserID); sanitized != "" {
		parts = append(parts, sanitized)
	}
	if sanitized := sanitizePorticoCacheIdentifier(localUserID); sanitized != "" {
		parts = append(parts, sanitized)
	}
	id := strings.Join(parts, "-")
	if id == "" {
		id = randomID("deleted")
	}
	return id + "@deleted.portico-account.invalid"
}

func deletedPorticoPrincipalUsername(porticoUserID, localUserID string) string {
	parts := []string{}
	if sanitized := sanitizePorticoCacheIdentifier(porticoUserID); sanitized != "" {
		parts = append(parts, sanitized)
	}
	if sanitized := sanitizePorticoCacheIdentifier(localUserID); sanitized != "" {
		parts = append(parts, sanitized)
	}
	id := strings.Join(parts, "-")
	if id == "" {
		id = randomID("deleted")
	}
	return "deleted-portico-" + id
}

func uniqueDeletedPorticoPrincipalEmailTx(tx *sql.Tx, porticoUserID, localUserID string) (string, error) {
	base := deletedPorticoPrincipalEmail(porticoUserID, localUserID)
	const domain = "@deleted.portico-account.invalid"
	prefix := strings.TrimSuffix(base, domain)
	for attempt := 1; attempt <= 100; attempt++ {
		candidate := base
		if attempt > 1 {
			candidate = fmt.Sprintf("%s-%d%s", prefix, attempt, domain)
		}
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower(?) AND id <> ?)`, candidate, localUserID).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("allocate deleted Portico email for local user %q: exhausted deterministic candidates", localUserID)
}

func uniqueDeletedPorticoPrincipalUsernameTx(tx *sql.Tx, porticoUserID, localUserID string) (string, error) {
	base := deletedPorticoPrincipalUsername(porticoUserID, localUserID)
	for attempt := 1; attempt <= 100; attempt++ {
		candidate := base
		if attempt > 1 {
			candidate = fmt.Sprintf("%s-%d", base, attempt)
		}
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE lower(username) = lower(?) AND id <> ?)`, candidate, localUserID).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("allocate deleted Portico username for local user %q: exhausted deterministic candidates", localUserID)
}

func (s *Server) syncRemoteAccessMembersIfClaimed(ctx context.Context, settings RemoteAccessSettings) {
	if !settings.Enabled || settings.ClaimStatus != "claimed" || settings.ServerID == "" {
		return
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if credential == "" {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := s.syncRemoteAccessMembers(syncCtx, settings, credential); err != nil {
		s.recordLog("warn", "Portico account member sync failed", map[string]string{"error": err.Error()})
	}
}

func (s *Server) requestRemoteAccessCertificate(ctx context.Context, settings RemoteAccessSettings) error {
	certificateHostname := remoteAccessCertificateHostname(settings.AssignedHostname)
	if certificateHostname == "" {
		return errors.New("assigned hostname is not a Portico direct-access hostname")
	}
	pending, err := s.loadPendingCertificateOrder()
	if errors.Is(err, os.ErrNotExist) {
		privateKey, csrPEM, generateErr := s.generateCertificateCSR(certificateHostname)
		if generateErr != nil {
			return generateErr
		}
		keyBytes, marshalErr := x509.MarshalECPrivateKey(privateKey)
		if marshalErr != nil {
			return marshalErr
		}
		pending = remoteAccessPendingCertificateOrder{
			Hostname:      certificateHostname,
			PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})),
			CSRPEM:        string(csrPEM),
		}
		if saveErr := s.savePendingCertificateOrder(pending); saveErr != nil {
			return saveErr
		}
	} else if err != nil {
		return err
	} else if !strings.EqualFold(strings.TrimSpace(pending.Hostname), certificateHostname) {
		return errors.New("pending certificate order targets a different assigned hostname")
	}
	privateKey, keyPEM, err := privateKeyFromPendingCertificateOrder(pending)
	if err != nil {
		return err
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if pending.OrderID == "" {
		createPayload, _ := json.Marshal(map[string]string{"csrPem": pending.CSRPEM})
		createEndpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/certificate-orders"
		var created struct {
			ID string `json:"id"`
		}
		idempotencyDigest := sha256.Sum256([]byte(settings.ServerID + "\n" + certificateHostname + "\n" + pending.CSRPEM))
		idempotencyKey := "certificate-" + hex.EncodeToString(idempotencyDigest[:16])
		if err := s.hostedJSONWithIdempotency(ctx, http.MethodPost, createEndpoint, credential, createPayload, &created, idempotencyKey); err != nil {
			return err
		}
		if strings.TrimSpace(created.ID) == "" {
			return errors.New("certificate order response missing ID")
		}
		pending.OrderID = created.ID
		if err := s.savePendingCertificateOrder(pending); err != nil {
			return err
		}
	}
	return s.finalizeRemoteAccessCertificate(ctx, settings, credential, pending.OrderID, privateKey, keyPEM, certificateHostname)
}

func (s *Server) finalizeRemoteAccessCertificate(ctx context.Context, settings RemoteAccessSettings, credential string, orderID string, privateKey *ecdsa.PrivateKey, keyPEM []byte, certificateHostname string) error {
	if orderID == "" {
		return errors.New("certificate order response missing ID")
	}
	finalizeEndpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/certificate-orders/" + url.PathEscape(orderID) + "/finalize"
	type certificateOrderResult struct {
		Status              string `json:"status"`
		CertificateChainPEM string `json:"certificateChainPem"`
		ExpiresAt           string `json:"expiresAt"`
	}
	var finalized certificateOrderResult
	finalizeDigest := sha256.Sum256([]byte(settings.ServerID + "\n" + orderID + "\nfinalize"))
	finalizeKey := "certificate-finalize-" + hex.EncodeToString(finalizeDigest[:16])
	metadata, err := s.hostedJSONWithIdempotencyMetadata(ctx, http.MethodPost, finalizeEndpoint, credential, nil, &finalized, finalizeKey)
	if err != nil {
		return err
	}
	pollEndpoint := strings.TrimSuffix(finalizeEndpoint, "/finalize")
	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	pollAttempt := 0
	for finalized.Status != "valid" || strings.TrimSpace(finalized.CertificateChainPEM) == "" {
		switch finalized.Status {
		case "pending", "queued", "leased", "finalizing", "":
		case "failed", "cancelled", "revoked", "reconcile_required":
			return fmt.Errorf("certificate order ended with status %s", finalized.Status)
		default:
			return fmt.Errorf("certificate order returned unsupported status %s", finalized.Status)
		}
		delay := remoteAccessCertificatePollInterval(orderID, pollAttempt, metadata.RetryAfter)
		timer := time.NewTimer(delay)
		select {
		case <-pollCtx.Done():
			timer.Stop()
			return fmt.Errorf("wait for certificate order: %w", pollCtx.Err())
		case <-timer.C:
		}
		metadata, err = s.hostedJSONWithMetadata(pollCtx, http.MethodGet, pollEndpoint, credential, nil, &finalized)
		if err != nil {
			var hostedErr *hostedHTTPError
			if errors.As(err, &hostedErr) && (hostedErr.StatusCode == http.StatusRequestTimeout || hostedErr.StatusCode == http.StatusTooManyRequests || hostedErr.StatusCode >= 500) {
				pollAttempt++
				continue
			}
			return err
		}
		pollAttempt++
	}
	if err := validateCertificateChainForPrivateKeyAndHostname([]byte(finalized.CertificateChainPEM), privateKey, certificateHostname); err != nil {
		return err
	}
	if err := s.writeRemoteAccessCertificateFiles(keyPEM, []byte(finalized.CertificateChainPEM)); err != nil {
		return err
	}
	settings.CertificateStatus = finalized.Status
	settings.CertificateExpiresAt = finalized.ExpiresAt
	settings.LastCertificateRenewalAt = time.Now().UTC().Format(time.RFC3339)
	settings.CertificateRenewalError = ""
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		return err
	}
	if err := s.clearPendingCertificateOrder(); err != nil {
		return fmt.Errorf("clear completed certificate order: %w", err)
	}
	s.configureRemoteTLS(settings)
	return nil
}

func remoteAccessCertificatePollInterval(orderID string, attempt int, retryAfter time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 5 {
		attempt = 5
	}
	base := 5 * time.Second * time.Duration(1<<attempt)
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	digest := sha256.Sum256([]byte(orderID + "\n" + strconv.Itoa(attempt)))
	offsetPercent := int(digest[0]%21) - 10
	delay := base + (base * time.Duration(offsetPercent) / 100)
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay < 5*time.Second {
		return 5 * time.Second
	}
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}

func remoteAccessCertificateHostname(assignedHostname string) string {
	assignedHostname = strings.ToLower(strings.Trim(strings.TrimSpace(assignedHostname), "."))
	const zone = ".direct.getportico.tv"
	if strings.HasSuffix(assignedHostname, zone) {
		token := strings.TrimSuffix(assignedHostname, zone)
		if validDirectAccessNamespace(token) {
			return "*." + token + zone
		}
	}
	return ""
}

func validDirectAccessNamespace(value string) bool {
	if len(value) != 24 || !strings.HasPrefix(value, "ptc-") {
		return false
	}
	for _, char := range value[len("ptc-"):] {
		if !((char >= 'a' && char <= 'z') || (char >= '2' && char <= '7')) {
			return false
		}
	}
	return true
}

func validateCertificateChainForPrivateKeyAndHostname(chainPEM []byte, privateKey *ecdsa.PrivateKey, hostname string) error {
	rest := chainPEM
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}
		publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("certificate public key is not ECDSA")
		}
		if publicKey.Curve != privateKey.Curve || publicKey.X.Cmp(privateKey.X) != 0 || publicKey.Y.Cmp(privateKey.Y) != 0 {
			return errors.New("certificate public key does not match private key")
		}
		if err := certificateCoversHostname(cert, hostname); err != nil {
			return fmt.Errorf("certificate does not cover assigned hostname: %w", err)
		}
		return nil
	}
	return errors.New("certificate chain did not contain a certificate")
}

func certificateCoversHostname(cert *x509.Certificate, hostname string) error {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if strings.HasPrefix(hostname, "*.") {
		for _, name := range cert.DNSNames {
			if strings.EqualFold(strings.TrimSpace(name), hostname) {
				return nil
			}
		}
		if len(cert.DNSNames) == 0 && strings.EqualFold(strings.TrimSpace(cert.Subject.CommonName), hostname) {
			return nil
		}
		return fmt.Errorf("certificate does not include %s", hostname)
	}
	return cert.VerifyHostname(hostname)
}

func (s *Server) writeRemoteAccessCertificateFiles(keyPEM, chainPEM []byte) error {
	return s.publishRemoteCertificatePair(keyPEM, chainPEM)
}

func (s *Server) ensureRemoteAccessCertificateFresh(ctx context.Context, settings RemoteAccessSettings) (RemoteAccessSettings, error) {
	return s.ensureRemoteAccessCertificateFreshWithOptions(ctx, settings, remoteAccessCertificateOptions{})
}

func (s *Server) ensureRemoteAccessCertificateFreshWithOptions(ctx context.Context, settings RemoteAccessSettings, options remoteAccessCertificateOptions) (RemoteAccessSettings, error) {
	if settings.ServerID == "" || settings.AssignedHostname == "" || s.secretSetting(remoteAccessCredentialKey) == "" {
		return settings, nil
	}
	if settings.CustomCertificateEnabled {
		if err := s.refreshCustomCertificateStatus(&settings); err != nil {
			_ = s.saveRemoteAccessSettings(settings)
			return settings, err
		}
		_ = s.saveRemoteAccessSettings(settings)
		return settings, nil
	}
	remoteAccessCertificateProvisioningMu.Lock()
	defer remoteAccessCertificateProvisioningMu.Unlock()
	// Status polling, listener repair, startup repair, and the heartbeat manager
	// can all notice the same missing certificate. Re-read state after entering
	// the singleflight boundary so queued workers observe the completed install.
	if current, loadErr := s.remoteAccessSettings(); loadErr == nil {
		settings = current
	}
	due, err := s.remoteAccessCertificateRenewalDue(settings, time.Now().UTC())
	if err != nil {
		settings.CertificateRenewalError = err.Error()
		_ = s.saveRemoteAccessSettings(settings)
		return settings, err
	}
	if !due && !options.Force {
		// A crash after atomically installing the certificate but before removing
		// the staging record is safe to finish here: renewalDue already proved the
		// installed chain is current and covers the assigned namespace.
		_ = s.clearPendingCertificateOrder()
		return settings, nil
	}
	if err := s.requestRemoteAccessCertificate(ctx, settings); err != nil {
		settings.CertificateRenewalError = err.Error()
		_ = s.saveRemoteAccessSettings(settings)
		return settings, err
	}
	return s.remoteAccessSettings()
}

func (s *Server) remoteAccessCertificateRenewalDue(settings RemoteAccessSettings, now time.Time) (bool, error) {
	if settings.CustomCertificateEnabled {
		return false, nil
	}
	if settings.CertificateStatus != "valid" {
		return true, nil
	}
	certificateHostname := remoteAccessCertificateHostname(settings.AssignedHostname)
	if certificateHostname == "" {
		return true, errors.New("assigned hostname is not a Portico direct-access hostname")
	}
	if err := s.certificateKeyPairCoversHostname(certificateHostname); err != nil {
		// A certificate issued for the legacy unencoded hostname remains unexpired,
		// but it cannot authenticate the stateless IP-encoded route. Replace it
		// immediately instead of waiting for the ordinary expiry window.
		return true, nil
	}
	expiresAt := strings.TrimSpace(settings.CertificateExpiresAt)
	if expiresAt == "" {
		fileExpiresAt, err := s.certificateChainExpiresAt()
		if err != nil {
			return true, nil
		}
		expiresAt = fileExpiresAt.Format(time.RFC3339)
		settings.CertificateExpiresAt = expiresAt
		_ = s.saveRemoteAccessSettings(settings)
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true, nil
	}
	return parsed.Before(now.Add(14 * 24 * time.Hour)), nil
}

func (s *Server) certificateKeyPairCoversHostname(hostname string) error {
	pair, err := s.loadPublishedRemoteAccessCertificate()
	if err != nil {
		return err
	}
	return certificateCoversHostname(pair.Leaf, hostname)
}

func (s *Server) certificateChainCoversHostname(hostname string) error {
	pair, err := s.loadPublishedRemoteAccessCertificate()
	if err != nil {
		return err
	}
	return certificateCoversHostname(pair.Leaf, hostname)
}

func (s *Server) certificateChainExpiresAt() (time.Time, error) {
	pair, err := s.loadPublishedRemoteAccessCertificate()
	if err != nil {
		return time.Time{}, err
	}
	return pair.Leaf.NotAfter.UTC(), nil
}

func (s *Server) refreshCustomCertificateStatus(settings *RemoteAccessSettings) error {
	if settings == nil || !settings.CustomCertificateEnabled {
		return nil
	}
	expiresAt, err := validateCustomCertificateFiles(settings.CustomCertificatePath, settings.CustomCertificateKeyPath)
	if err != nil {
		settings.CertificateStatus = "custom_error"
		settings.CertificateRenewalError = err.Error()
		return err
	}
	settings.CertificateStatus = "custom_valid"
	settings.CertificateExpiresAt = expiresAt.Format(time.RFC3339)
	settings.CertificateRenewalError = ""
	return nil
}

func validateCustomCertificateFiles(chainPath string, keyPath string) (time.Time, error) {
	chainPath = strings.TrimSpace(chainPath)
	keyPath = strings.TrimSpace(keyPath)
	if chainPath == "" || keyPath == "" {
		return time.Time{}, errors.New("custom certificate and private key paths are required")
	}
	if !filepath.IsAbs(chainPath) || !filepath.IsAbs(keyPath) {
		return time.Time{}, errors.New("custom certificate paths must be absolute")
	}
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("private key unavailable: %w", err)
	}
	if keyInfo.IsDir() {
		return time.Time{}, errors.New("private key path points to a directory")
	}
	if keyInfo.Mode().Perm()&0o077 != 0 {
		return time.Time{}, errors.New("private key file must not be readable by group or others")
	}
	chainInfo, err := os.Stat(chainPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("certificate chain unavailable: %w", err)
	}
	if chainInfo.IsDir() {
		return time.Time{}, errors.New("certificate chain path points to a directory")
	}
	cert, err := tls.LoadX509KeyPair(chainPath, keyPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("certificate and private key do not form a valid pair: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return time.Time{}, errors.New("certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("certificate leaf is invalid: %w", err)
	}
	now := time.Now().UTC()
	if now.Before(leaf.NotBefore) {
		return time.Time{}, errors.New("certificate is not valid yet")
	}
	if !now.Before(leaf.NotAfter) {
		return time.Time{}, errors.New("certificate is expired")
	}
	return leaf.NotAfter.UTC(), nil
}

func (s *Server) hostedJSON(ctx context.Context, method, endpoint, bearer string, body []byte, out any) error {
	return s.hostedJSONWithTimeout(ctx, method, endpoint, bearer, body, out, 15*time.Second)
}

func (s *Server) hostedJSONWithIdempotency(ctx context.Context, method, endpoint, bearer string, body []byte, out any, idempotencyKey string) error {
	_, err := s.hostedJSONRequest(ctx, method, endpoint, bearer, body, out, 15*time.Second, idempotencyKey)
	return err
}

func (s *Server) hostedJSONWithMetadata(ctx context.Context, method, endpoint, bearer string, body []byte, out any) (hostedResponseMetadata, error) {
	return s.hostedJSONRequest(ctx, method, endpoint, bearer, body, out, 15*time.Second, "")
}

func (s *Server) hostedJSONWithIdempotencyMetadata(ctx context.Context, method, endpoint, bearer string, body []byte, out any, idempotencyKey string) (hostedResponseMetadata, error) {
	return s.hostedJSONRequest(ctx, method, endpoint, bearer, body, out, 15*time.Second, idempotencyKey)
}

type hostedHTTPError struct {
	StatusCode int
	Status     string
	Code       string
	Detail     string
	MessageID  string
	RetryAfter time.Duration
}

func (e *hostedHTTPError) Error() string {
	return "Hosted Services returned " + e.Status
}

func (s *Server) hostedJSONWithTimeout(ctx context.Context, method, endpoint, bearer string, body []byte, out any, timeout time.Duration) error {
	_, err := s.hostedJSONRequest(ctx, method, endpoint, bearer, body, out, timeout, "")
	return err
}

type hostedResponseMetadata struct {
	RetryAfter time.Duration
}

func (s *Server) hostedJSONRequest(ctx context.Context, method, endpoint, bearer string, body []byte, out any, timeout time.Duration, idempotencyKey string) (hostedResponseMetadata, error) {
	metadata := hostedResponseMetadata{}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, reader)
	if err != nil {
		return metadata, err
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.Scheme == "" || endpointURL.Host == "" {
		return metadata, errors.New("Hosted request endpoint is not a valid origin")
	}
	settings, settingsErr := s.remoteAccessSettings()
	if settingsErr != nil {
		return metadata, fmt.Errorf("load Hosted authority: %w", settingsErr)
	}
	configuredURL, parseErr := url.Parse(strings.TrimSpace(settings.HostedBaseURL))
	if parseErr != nil || !sameHTTPOrigin(endpointURL, configuredURL) {
		return metadata, errors.New("Hosted request endpoint is outside the configured authority")
	}
	if err := s.validateRemoteAccessSettings(settings); err != nil {
		return metadata, fmt.Errorf("Hosted request authority is not approved: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	client := &http.Client{Timeout: timeout}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		// Hosted endpoints are canonical API routes. Following even a same-origin
		// redirect makes one-time operations and credential delivery ambiguous.
		return errors.New("Hosted Services request redirected")
	}
	resp, err := client.Do(req)
	if err != nil {
		return metadata, err
	}
	defer resp.Body.Close()
	metadata.RetryAfter = parseHostedRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	responseBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return metadata, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		hostedErr := &hostedHTTPError{StatusCode: resp.StatusCode, Status: resp.Status, RetryAfter: metadata.RetryAfter}
		var problem struct {
			Code      string `json:"code"`
			Detail    string `json:"detail"`
			MessageID string `json:"messageId"`
		}
		if json.Unmarshal(responseBytes, &problem) == nil {
			hostedErr.Code = strings.TrimSpace(problem.Code)
			hostedErr.Detail = strings.TrimSpace(problem.Detail)
			hostedErr.MessageID = strings.TrimSpace(problem.MessageID)
		}
		return metadata, hostedErr
	}
	if out != nil {
		return metadata, json.Unmarshal(responseBytes, out)
	}
	return metadata, nil
}

func parseHostedRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 {
		delay := time.Duration(seconds * float64(time.Second))
		if delay > time.Hour {
			return time.Hour
		}
		return delay
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		delay := retryAt.Sub(now)
		if delay < 0 {
			return 0
		}
		if delay > time.Hour {
			return time.Hour
		}
		return delay
	}
	return 0
}

func hostedRetryAfter(err error) time.Duration {
	var hostedErr *hostedHTTPError
	if errors.As(err, &hostedErr) {
		return hostedErr.RetryAfter
	}
	return 0
}

func sameHTTPOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func (s *Server) generateCertificateCSR(hostname string) (*ecdsa.PrivateKey, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	template := x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: hostname},
		DNSNames: []string{hostname},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		return nil, nil, err
	}
	return privateKey, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), nil
}

func (s *Server) runRemoteAccessHeartbeat(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	consecutiveFailures := 0
	var lastHeartbeat time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		nextInterval := s.remoteAccessLeaseInterval()
		settings, err := s.remoteAccessSettings()
		if err == nil && settings.Enabled && settings.ClaimStatus == "claimed" && settings.ServerID != "" {
			now := time.Now().UTC()
			if lastHeartbeat.IsZero() && settings.LastHeartbeatAt != "" {
				lastHeartbeat, _ = time.Parse(time.RFC3339, settings.LastHeartbeatAt)
			}
			repairReason := ""
			signature := s.remoteAccessNetworkSignature()
			if signature != "" {
				if settings.LastNetworkSignature == "" {
					settings.LastNetworkSignature = signature
				} else if signature != settings.LastNetworkSignature {
					settings.LastNetworkSignature = signature
					settings.LastNetworkChangeAt = now.Format(time.RFC3339)
					repairReason = "network_changed"
				}
			}
			if repairReason != "" {
				settings.LastRouteRepairAt = now.Format(time.RFC3339)
				settings.LastRouteRepairReason = repairReason
				settings.LastReachabilityCheckAt = now.Format(time.RFC3339)
				settings.LastReachabilityResult = "repair_" + repairReason
			}
			heartbeatDue := lastHeartbeat.IsZero() || now.Sub(lastHeartbeat) >= s.remoteAccessLeaseInterval() || repairReason != "" || consecutiveFailures > 0
			if !heartbeatDue {
				// The Hosted lease is still current. Any pending repair directive
				// arrives in the next heartbeat response without a separate poll.
			} else if err := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{SyncPolicy: true}); err != nil {
				consecutiveFailures++
				settings.LastReachabilityCheckAt = now.Format(time.RFC3339)
				settings.LastReachabilityResult = "heartbeat_failed"
				settings.LastHeartbeatError = remoteAccessFailureCode(err)
				_ = s.saveRemoteAccessSettings(settings)
				s.recordLog("warn", "Remote access heartbeat failed", map[string]string{"error": err.Error()})
				nextInterval = remoteAccessFailureRetryInterval(consecutiveFailures)
				if retryAfter := hostedRetryAfter(err); retryAfter > nextInterval {
					nextInterval = retryAfter
				}
			} else {
				consecutiveFailures = 0
				lastHeartbeat = now
				if refreshed, loadErr := s.remoteAccessSettings(); loadErr == nil {
					settings = refreshed
				}
				previousCertificateStatus, previousCertificateExpiry := settings.CertificateStatus, settings.CertificateExpiresAt
				if updated, renewErr := s.ensureRemoteAccessCertificateFresh(ctx, settings); renewErr != nil {
					s.recordLog("warn", "Remote access certificate renewal failed", map[string]string{"error": renewErr.Error()})
				} else {
					settings = updated
					s.configureRemoteTLS(settings)
					if settings.CertificateStatus != previousCertificateStatus || settings.CertificateExpiresAt != previousCertificateExpiry {
						if publishErr := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{SyncPolicy: false}); publishErr != nil {
							s.recordLog("warn", "Remote access certificate publication heartbeat failed", map[string]string{"error": publishErr.Error()})
						}
					}
				}
			}
		}
		timer.Reset(jitterRemoteAccessInterval(nextInterval))
	}
}

func (s *Server) remoteAccessLeaseInterval() time.Duration {
	seconds := s.remoteAccessLeaseSeconds.Load()
	if seconds < 60 || seconds > 3600 {
		return 10 * time.Minute
	}
	// Hosted returns the lease lifetime, not the instant at which it should be
	// renewed. Renew at two-thirds of that lifetime so jitter and a transient
	// retry cannot make an online server appear offline between heartbeats.
	interval := time.Duration(seconds) * time.Second * 2 / 3
	if interval < time.Minute {
		return time.Minute
	}
	return interval
}

func (s *Server) remoteAccessRepairPollInterval() time.Duration {
	seconds := s.remoteAccessRepairPollSeconds.Load()
	if seconds < 60 || seconds > 1800 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func jitterRemoteAccessInterval(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	spread := base / 10
	if spread < time.Second {
		spread = time.Second
	}
	// Wall-clock entropy is sufficient here: this is only fleet de-synchrony,
	// never a security decision. The result remains bounded to +/-10%.
	offset := time.Duration(time.Now().UnixNano()%int64(2*spread+1)) - spread
	interval := base + offset
	if interval < time.Second {
		return time.Second
	}
	return interval
}

func (s *Server) runRemoteAccessNetworkMonitor(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		nextInterval := 10 * time.Second
		changed, err := s.checkRemoteAccessNetworkAndRefresh(ctx)
		if err != nil {
			s.recordLog("warn", "Remote access network monitor failed", map[string]string{"error": err.Error()})
			nextInterval = 20 * time.Second
		} else if changed {
			// Re-sample promptly once after a transition so a multi-step interface
			// reconfiguration settles quickly without polling Hosted Services.
			nextInterval = 2 * time.Second
		}
		timer.Reset(jitterRemoteAccessInterval(nextInterval))
	}
}

func (s *Server) checkRemoteAccessNetworkAndRefresh(ctx context.Context) (bool, error) {
	return s.checkRemoteAccessNetworkSignatureAndRefresh(ctx, s.remoteAccessNetworkSignature())
}

func (s *Server) checkRemoteAccessNetworkSignatureAndRefresh(ctx context.Context, signature string) (bool, error) {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		return false, err
	}
	if !settings.Enabled || settings.ClaimStatus != "claimed" || settings.ServerID == "" {
		return false, nil
	}
	if s.secretSetting(remoteAccessCredentialKey) == "" {
		return false, nil
	}
	signature = strings.TrimSpace(signature)
	if signature == "" || signature == settings.LastNetworkSignature {
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if settings.LastNetworkSignature == "" {
		// Establish a local baseline without creating a Hosted request at startup.
		// The heartbeat manager independently sends the initial lease heartbeat.
		settings.LastNetworkSignature = signature
		return false, s.saveRemoteAccessSettings(settings)
	}
	settings.LastNetworkSignature = signature
	settings.LastNetworkChangeAt = now
	settings.LastRouteRepairAt = now
	settings.LastRouteRepairReason = "network_changed"
	settings.LastReachabilityResult = "repair_network_changed"
	settings.LastReachabilityCheckAt = now
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		return false, err
	}
	if err := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{SyncPolicy: false}); err != nil {
		settings.LastHeartbeatError = remoteAccessFailureCode(err)
		_ = s.saveRemoteAccessSettings(settings)
		return true, err
	}
	s.recordLog("info", "Remote access network change pushed to Hosted Services", map[string]string{"reason": settings.LastRouteRepairReason})
	return true, nil
}

func (s *Server) checkRemoteAccessRepairSignalAndRepair(ctx context.Context) (bool, error) {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		return false, err
	}
	if !settings.Enabled || settings.ClaimStatus != "claimed" || settings.ServerID == "" {
		return false, nil
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if credential == "" {
		return false, nil
	}
	var signal remoteAccessRepairSignal
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/repair-signal"
	unchanged, err := s.fetchRemoteAccessRepairSignal(ctx, endpoint, credential, &signal)
	if err != nil {
		return false, err
	}
	if unchanged {
		return false, nil
	}
	if updateRemoteAccessPublicRouteDiagnostics(&settings, signal.PublicRouteStatus, signal.PublicRouteError, signal.PublicRouteCheckedAt) {
		_ = s.saveRemoteAccessSettings(settings)
	}
	if !signal.RepairRequested {
		return false, nil
	}
	return s.handleRemoteAccessRepairSignal(ctx, settings, signal, true)
}

func (s *Server) handleRemoteAccessRepairSignal(ctx context.Context, settings RemoteAccessSettings, signal remoteAccessRepairSignal, refreshPublicIP bool) (bool, error) {
	if updateRemoteAccessPublicRouteDiagnostics(&settings, signal.PublicRouteStatus, signal.PublicRouteError, signal.PublicRouteCheckedAt) {
		_ = s.saveRemoteAccessSettings(settings)
	}
	if !signal.RepairRequested {
		return false, nil
	}
	if refreshPublicIP {
		if publicIP, ipErr := s.queryHostedObservedPublicIP(ctx, settings); ipErr != nil {
			s.recordLog("warn", "Remote access repair signal public IP refresh failed", map[string]string{"error": ipErr.Error()})
		} else if publicIP != "" {
			settings.LastPublicIPAddress = publicIP
			settings.LastPublicIPCheckAt = time.Now().UTC().Format(time.RFC3339)
			// Certificate provisioning intentionally reloads authoritative settings
			// after entering its process-wide singleflight. Persist the newly observed
			// network state first so that reload cannot restore a stale public address.
			if err := s.saveRemoteAccessSettings(settings); err != nil {
				return false, err
			}
		}
	}
	if remoteAccessRepairSignalNeedsCertificateRenewal(signal.Status, signal.Reason) {
		updated, certErr := s.ensureRemoteAccessCertificateFreshWithOptions(ctx, settings, remoteAccessCertificateOptions{Force: true})
		if certErr != nil {
			s.recordLog("warn", "Remote access repair signal certificate renewal failed", map[string]string{"error": certErr.Error()})
			if refreshed, loadErr := s.remoteAccessSettings(); loadErr == nil {
				settings = refreshed
			}
		} else {
			settings = updated
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	settings.LastRouteRepairAt = now
	settings.LastRouteRepairReason = "hosted_route_failure"
	if strings.TrimSpace(settings.LastReachabilityResult) == "" || strings.HasPrefix(settings.LastReachabilityResult, "repair_") {
		settings.LastReachabilityResult = "public_checking"
	}
	if strings.TrimSpace(settings.LastReachabilityCheckAt) == "" {
		settings.LastReachabilityCheckAt = now
	}
	if err := s.saveRemoteAccessSettings(settings); err != nil {
		return false, err
	}
	if err := s.sendRemoteAccessHeartbeatWithOptions(ctx, settings, remoteAccessHeartbeatOptions{SyncPolicy: false, SuppressRepair: true}); err != nil {
		settings.LastHeartbeatError = remoteAccessFailureCode(err)
		_ = s.saveRemoteAccessSettings(settings)
		return true, err
	}
	fields := map[string]string{
		"status":    signal.Status,
		"routeType": signal.RouteType,
		"host":      signal.Host,
	}
	if reason := strings.TrimSpace(signal.Reason); reason != "" {
		fields["reason"] = reason
	}
	s.recordLog("info", "Remote access hosted repair signal handled", fields)
	return true, nil
}

func (s *Server) fetchRemoteAccessRepairSignal(ctx context.Context, endpoint, credential string, target any) (bool, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	s.remoteAccessSignalMu.Lock()
	etag := ""
	if s.remoteAccessRepairEndpoint == endpoint {
		etag = s.remoteAccessRepairETag
	}
	s.remoteAccessSignalMu.Unlock()
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("Hosted Services repair signal returned %s", resp.Status)
	}
	if responseETag := strings.TrimSpace(resp.Header.Get("ETag")); responseETag != "" {
		s.remoteAccessSignalMu.Lock()
		s.remoteAccessRepairEndpoint = endpoint
		s.remoteAccessRepairETag = responseETag
		s.remoteAccessSignalMu.Unlock()
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return false, err
	}
	return false, nil
}

func remoteAccessRepairSignalNeedsCertificateRenewal(status string, reason string) bool {
	combined := strings.ToLower(strings.TrimSpace(status) + " " + strings.TrimSpace(reason))
	return strings.Contains(combined, "tls") || strings.Contains(combined, "certificate") || strings.Contains(combined, "x509")
}

func updateRemoteAccessPublicRouteDiagnostics(settings *RemoteAccessSettings, status string, errorMessage string, checkedAt string) bool {
	status = strings.TrimSpace(status)
	errorMessage = remoteAccessDiagnosticCode(errorMessage)
	checkedAt = strings.TrimSpace(checkedAt)
	if status == "" {
		return false
	}
	result := "public_" + status
	switch status {
	case "reachable":
		result = "public_reachable"
	case "failed", "http_failed", "tls_failed", "identity_mismatch":
		result = "public_unreachable"
	case "missing":
		result = "public_missing"
	case "checking", "dns_synced":
		result = "public_checking"
	}
	if checkedAt == "" || strings.HasPrefix(checkedAt, "0001-01-01") {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	changed := false
	if settings.LastReachabilityResult != result {
		settings.LastReachabilityResult = result
		changed = true
	}
	if settings.LastReachabilityCheckAt != checkedAt {
		settings.LastReachabilityCheckAt = checkedAt
		changed = true
	}
	if settings.LastPublicRouteError != errorMessage {
		settings.LastPublicRouteError = errorMessage
		changed = true
	}
	return changed
}

func remoteAccessFailureRetryInterval(failures int) time.Duration {
	if failures <= 1 {
		return 30 * time.Second
	}
	if failures == 2 {
		return time.Minute
	}
	if failures == 3 {
		return 2 * time.Minute
	}
	return 5 * time.Minute
}

// Remote-access diagnostics are persisted into settings and returned by the
// settings/status APIs. Keep them as stable product codes so upstream URLs,
// addresses, certificate subjects, and transport implementation details do
// not become account-visible data.
func remoteAccessFailureCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return "hosted_timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "hosted_timeout"
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var certificateInvalid x509.CertificateInvalidError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameError) || errors.As(err, &certificateInvalid) {
		return "tls_verification_failed"
	}
	var responseError *hostedHTTPError
	if errors.As(err, &responseError) {
		switch responseError.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "hosted_authorization_failed"
		case http.StatusTooManyRequests:
			return "hosted_rate_limited"
		default:
			if responseError.StatusCode >= 500 {
				return "hosted_unavailable"
			}
			return "hosted_request_rejected"
		}
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "redirect"):
		return "hosted_redirect_rejected"
	case strings.Contains(message, "connection refused"):
		return "hosted_connection_refused"
	case strings.Contains(message, "no such host"), strings.Contains(message, "network is unreachable"):
		return "hosted_network_unreachable"
	default:
		return "hosted_request_failed"
	}
}

func remoteAccessDiagnosticCode(value string) string {
	switch strings.TrimSpace(value) {
	case "tls_verification_failed", "network_timeout", "connection_refused", "network_unreachable", "unexpected_http_status", "request_failed", "client_route_verification_failed":
		return strings.TrimSpace(value)
	case "":
		return ""
	default:
		return "route_verification_failed"
	}
}

func remoteAccessPublicIPCheckDue(settings RemoteAccessSettings, now time.Time, interval time.Duration) bool {
	if settings.LastPublicIPAddress == "" || settings.LastPublicIPCheckAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, settings.LastPublicIPCheckAt)
	if err != nil {
		return true
	}
	return now.Sub(last) >= interval
}

func (s *Server) queryHostedObservedPublicIP(ctx context.Context, settings RemoteAccessSettings) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/network/public-ip"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if credential := s.secretSetting(remoteAccessCredentialKey); credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	resp, err := hostedServicesHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("public IP check returned %s", resp.Status)
	}
	var payload struct {
		PublicIP string `json:"publicIp"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	ip := validPublicIPString(payload.PublicIP)
	if ip == "" {
		return "", errors.New("Hosted Services returned an invalid public IP")
	}
	return ip, nil
}

func validPublicIPString(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return ""
	}
	return ip.String()
}

func (s *Server) remoteAccessNetworkSignature() string {
	key := []byte(s.secretSetting(remoteAccessCredentialKey))
	if len(key) == 0 {
		return ""
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var parts []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var addrParts []string
		for _, addr := range addrs {
			addrParts = append(addrParts, addr.String())
		}
		if len(addrParts) == 0 {
			continue
		}
		sort.Strings(addrParts)
		parts = append(parts, iface.Name+"|"+iface.HardwareAddr.String()+"|"+strings.Join(addrParts, ","))
	}
	sort.Strings(parts)
	return networkSignatureFingerprint(key, parts)
}

func networkSignatureFingerprint(key []byte, parts []string) string {
	if len(key) == 0 || len(parts) == 0 {
		return ""
	}
	canonical := append([]string(nil), parts...)
	sort.Strings(canonical)
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write([]byte(strings.Join(canonical, "\n")))
	return "hmac-sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func (s *Server) lanEndpointCandidates(settings RemoteAccessSettings) []map[string]any {
	if !settings.LANDiscoveryEnabled {
		return nil
	}
	candidates := []map[string]any{}
	seen := map[string]bool{}
	appendCandidate := func(endpointType string, host string, port int, scheme string) {
		host = strings.TrimSpace(host)
		scheme = strings.TrimSpace(scheme)
		if host == "" || port <= 0 {
			return
		}
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && (ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast()) {
			return
		}
		if scheme == "" {
			scheme = "https"
		}
		if endpointType == "" {
			endpointType = "lan"
		}
		key := endpointType + "\n" + strings.ToLower(host) + "\n" + strconv.Itoa(port) + "\n" + strings.ToLower(scheme)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, map[string]any{
			"type":   endpointType,
			"host":   host,
			"port":   port,
			"scheme": scheme,
		})
	}
	for _, route := range s.localRemoteAccessRoutes() {
		parsed, err := url.Parse(route.URL)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		port := settings.ManualPublicPort
		if parsed.Port() != "" {
			if parsedPort, err := strconv.Atoi(parsed.Port()); err == nil {
				port = parsedPort
			}
		}
		appendCandidate("lan", parsed.Hostname(), port, parsed.Scheme)
	}
	if localPort := portFromAddress(s.cfg.Addr); localPort > 0 {
		for _, rawURL := range localDiscoveryHTTPURLs(localPort) {
			parsed, err := url.Parse(rawURL)
			if err == nil {
				port := localPort
				if parsed.Port() != "" {
					port, _ = strconv.Atoi(parsed.Port())
				}
				appendCandidate("lan", parsed.Hostname(), port, parsed.Scheme)
			}
		}
	}
	// The unified listener selects TLS from the IP-encoded direct hostname's
	// SNI. Publish current private interface addresses as HTTPS candidates on
	// the configured public port so Hosted clients can use a certificate-valid
	// LAN route without sending credentials over mixed-content HTTP.
	if settings.ManualPublicPort > 0 {
		for _, host := range localPrivateInterfaceHosts() {
			appendCandidate("lan", host, settings.ManualPublicPort, "https")
		}
	}
	return candidates
}

func localPrivateInterfaceHosts() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	hosts := map[string]bool{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				ip = net.ParseIP(addr.String())
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || !ip.IsPrivate() {
				continue
			}
			hosts[ip.String()] = true
		}
	}
	result := make([]string, 0, len(hosts))
	for host := range hosts {
		result = append(result, host)
	}
	sort.Strings(result)
	return result
}

func (s *Server) remoteAccessSettings() (RemoteAccessSettings, error) {
	defaults := RemoteAccessSettings{
		Enabled:                         false,
		HostedBaseURL:                   s.defaultHostedBaseURL(),
		ClaimStatus:                     "not_claimed",
		PublicPortMode:                  "manual",
		ManualPublicPort:                defaultRemotePublicPort,
		PreferredRemoteAuthMode:         "portico",
		AllowManualLocalAuthRemoteLogin: false,
		LANDiscoveryEnabled:             true,
		RouterAutomationEnabled:         false,
		CertificateStatus:               "not_requested",
	}
	settings, err := s.loadSettings()
	if err != nil {
		return defaults, err
	}
	group, _ := settings[remoteAccessSettingsKey].(map[string]any)
	defaults.Enabled = settingBool(group, "enabled", defaults.Enabled)
	defaults.HostedBaseURL = settingString(group, "hostedBaseUrl", defaults.HostedBaseURL)
	defaults.ClaimStatus = settingString(group, "claimStatus", defaults.ClaimStatus)
	defaults.ServerID = settingString(group, "serverId", "")
	defaults.AssignedHostname = settingString(group, "assignedHostname", "")
	defaults.PublicPortMode = settingString(group, "publicPortMode", defaults.PublicPortMode)
	defaults.ManualPublicPort = settingInt(group, "manualPublicPort", defaults.ManualPublicPort)
	defaults.PreferredRemoteAuthMode = settingString(group, "preferredRemoteAuthMode", defaults.PreferredRemoteAuthMode)
	defaults.AllowManualLocalAuthRemoteLogin = settingBool(group, "allowManualLocalAuthRemoteLogin", defaults.AllowManualLocalAuthRemoteLogin)
	defaults.LANDiscoveryEnabled = settingBool(group, "lanDiscoveryEnabled", defaults.LANDiscoveryEnabled)
	defaults.RouterAutomationEnabled = settingBool(group, "routerAutomationEnabled", defaults.RouterAutomationEnabled)
	defaults.RemoteBitrateLimitMbps = normalizeRemoteBitrateLimitMbps(settingInt(group, "remoteBitrateLimitMbps", defaults.RemoteBitrateLimitMbps))
	defaults.CertificateStatus = settingString(group, "certificateStatus", defaults.CertificateStatus)
	defaults.CertificateExpiresAt = settingString(group, "certificateExpiresAt", "")
	defaults.LastCertificateRenewalAt = settingString(group, "lastCertificateRenewalAt", "")
	defaults.CertificateRenewalError = settingString(group, "certificateRenewalError", "")
	defaults.CustomCertificateEnabled = settingBool(group, "customCertificateEnabled", false)
	defaults.CustomCertificatePath = settingString(group, "customCertificatePath", "")
	defaults.CustomCertificateKeyPath = settingString(group, "customCertificateKeyPath", "")
	defaults.LastHeartbeatAt = settingString(group, "lastHeartbeatAt", "")
	defaults.LastHeartbeatError = settingString(group, "lastHeartbeatError", "")
	defaults.LastHostedRemoteAccessState = settingString(group, "lastHostedRemoteAccessState", "")
	defaults.LastPublicIPAddress = settingString(group, "lastPublicIpAddress", "")
	defaults.LastPublicIPCheckAt = settingString(group, "lastPublicIpCheckAt", "")
	defaults.LastNetworkSignature = settingString(group, "lastNetworkSignature", "")
	legacyNetworkSignature := defaults.LastNetworkSignature != "" && !strings.HasPrefix(defaults.LastNetworkSignature, "hmac-sha256:")
	if legacyNetworkSignature {
		defaults.LastNetworkSignature = ""
	}
	defaults.LastNetworkChangeAt = settingString(group, "lastNetworkChangeAt", "")
	defaults.LastRouteRepairAt = settingString(group, "lastRouteRepairAt", "")
	defaults.LastRouteRepairReason = settingString(group, "lastRouteRepairReason", "")
	defaults.LastReachabilityCheckAt = settingString(group, "lastReachabilityCheckAt", "")
	defaults.LastReachabilityResult = settingString(group, "lastReachabilityResult", "")
	defaults.LastPublicRouteError = settingString(group, "lastPublicRouteError", "")
	defaults.RouterMappingStatus = settingString(group, "routerMappingStatus", "")
	defaults.LastRouterMappingAt = settingString(group, "lastRouterMappingAt", "")
	defaults.RouterMappingError = settingString(group, "routerMappingError", "")
	if legacyNetworkSignature {
		_ = s.saveRemoteAccessSettings(defaults)
	}
	if defaults.ServerID != "" && s.secretSetting(remoteAccessCredentialKey) != "" {
		if s.secretSetting(remoteAccessClaimReceiptKey) == "" {
			_ = s.deleteSetting(remoteAccessClaimKey)
		}
		_ = s.deleteSetting(remoteAccessClaimTokenKey)
		if defaults.ClaimStatus != "claimed" {
			defaults.ClaimStatus = "claimed"
			_ = s.saveRemoteAccessSettings(defaults)
		}
	}
	return defaults, s.validateRemoteAccessSettings(defaults)
}

func (s *Server) saveRemoteAccessSettings(settings RemoteAccessSettings) error {
	bytes, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.execUserWrite(context.Background(), `INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, remoteAccessSettingsKey, string(bytes), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Server) saveRemoteAccessClaim(claim RemoteAccessClaim) error {
	bytes, err := json.Marshal(claim)
	if err != nil {
		return err
	}
	_, err = s.execUserWrite(context.Background(), `INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, remoteAccessClaimKey, string(bytes), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Server) currentRemoteAccessClaim() *RemoteAccessClaim {
	settings, err := s.loadSettings()
	if err != nil {
		return nil
	}
	group, ok := settings[remoteAccessClaimKey].(map[string]any)
	if !ok {
		return nil
	}
	bytes, err := json.Marshal(group)
	if err != nil {
		return nil
	}
	var claim RemoteAccessClaim
	if err := json.Unmarshal(bytes, &claim); err != nil || claim.ClaimID == "" {
		return nil
	}
	return &claim
}

func (s *Server) deleteSetting(key string) error {
	_, err := s.execUserWrite(context.Background(), `DELETE FROM settings WHERE key = ?`, key)
	return err
}

func (s *Server) saveSecretSetting(key, value string) error {
	bytes, err := s.encryptRemoteSecret(value)
	if err != nil {
		return err
	}
	_, err = s.execUserWrite(context.Background(), `INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, key, string(bytes), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Server) secretSetting(key string) string {
	value, _ := s.secretSettingWithError(key)
	return value
}

func (s *Server) secretSettingWithError(key string) (string, error) {
	settings, err := s.loadSettings()
	if err != nil {
		return "", err
	}
	group, ok := settings[key].(map[string]any)
	if !ok {
		return "", fmt.Errorf("secret setting %q is unavailable", key)
	}
	bytes, err := json.Marshal(group)
	if err != nil {
		return "", err
	}
	value, err := s.decryptRemoteSecret(bytes)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Server) replaceRemoteAccessMembers(members []RemoteAccessMember) error {
	for attempt := 0; attempt < 3; attempt++ {
		accountIDs, err := s.remoteAccountsRevokedByMemberSnapshotContext(context.Background(), members)
		if err != nil {
			return err
		}
		handles, err := s.beginAccountRuntimeErasuresContext(context.Background(), accountIDs)
		if err != nil {
			return err
		}
		fenced := map[string]bool{}
		for _, accountID := range accountIDs {
			fenced[accountID] = true
		}
		err = s.replaceRemoteAccessMembersFenced(members, fenced)
		finishProfileErasureFences(handles)
		if errors.Is(err, errRemoteAuthorityFenceRetry) {
			continue
		}
		return err
	}
	return errors.New("remote membership policy changed repeatedly while applying authority revocation")
}

func (s *Server) remoteAccountsRevokedByMemberSnapshotContext(ctx context.Context, members []RemoteAccessMember) ([]string, error) {
	activeMemberships := map[string]bool{}
	for _, member := range members {
		membershipID := strings.TrimSpace(member.PorticoMembershipID)
		if membershipID == "" {
			membershipID = strings.TrimSpace(member.ID)
		}
		status := strings.TrimSpace(member.Status)
		if status == "" {
			status = "active"
		}
		if membershipID != "" && status == "active" {
			activeMemberships[membershipID] = true
		}
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT DISTINCT u.id, ram.portico_membership_id
		FROM users u
		JOIN remote_access_members ram ON ram.local_user_id = u.id
		WHERE u.auth_origin = 'portico' AND ram.status = 'active'
		ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accountIDs := []string{}
	for rows.Next() {
		var accountID, membershipID string
		if err := rows.Scan(&accountID, &membershipID); err != nil {
			return nil, err
		}
		if !activeMemberships[membershipID] {
			accountIDs = append(accountIDs, accountID)
		}
	}
	return accountIDs, rows.Err()
}

func (s *Server) replaceRemoteAccessMembersFenced(members []RemoteAccessMember, fenced map[string]bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.withBackgroundTxTagged(context.Background(), []string{"remote-access", "portico-members", "users", "profiles", "account"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE remote_access_members SET status = 'revoked', last_synced_at = ?`, now); err != nil {
			return err
		}
		for _, member := range members {
			if member.PorticoMembershipID == "" {
				member.PorticoMembershipID = member.ID
			}
			if member.PorticoUserID == "" {
				member.PorticoUserID = member.UserID
			}
			if member.PorticoMembershipID == "" || member.PorticoUserID == "" {
				continue
			}
			status := member.Status
			if status == "" {
				status = "active"
			}
			localUserID, provisionErr := s.linkSingleLocalOwnerForPorticoMemberTx(tx, member, status, now)
			if provisionErr != nil {
				return provisionErr
			}
			if localUserID == "" {
				localUserID, provisionErr = s.provisionPorticoProfileTx(tx, member, status, now)
			}
			if provisionErr != nil {
				return provisionErr
			}
			permissionTemplateJSON, err := json.Marshal(member.PermissionTemplate)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`
				INSERT INTO remote_access_members (portico_membership_id, portico_user_id, email, display_name, role, status, permission_template_json, local_user_id, last_synced_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), NULLIF((SELECT local_user_id FROM remote_access_members WHERE portico_membership_id = ?), ''), ''), ?)
				ON CONFLICT(portico_membership_id) DO UPDATE SET
					portico_user_id = excluded.portico_user_id,
					email = excluded.email,
					display_name = excluded.display_name,
					role = excluded.role,
					status = excluded.status,
					permission_template_json = excluded.permission_template_json,
					local_user_id = CASE WHEN excluded.local_user_id <> '' THEN excluded.local_user_id ELSE remote_access_members.local_user_id END,
					last_synced_at = excluded.last_synced_at`,
				member.PorticoMembershipID, member.PorticoUserID, member.Email, member.DisplayName, member.Role, status, string(permissionTemplateJSON), localUserID, member.PorticoMembershipID, now); err != nil {
				return err
			}
		}
		// The policy snapshot is authoritative. A revoked Cloud membership must
		// invalidate local sessions minted by an earlier Portico login, otherwise
		// the removed member remains signed in until the independent local expiry.
		if _, err := tx.Exec(`
			DELETE FROM sessions
			WHERE user_id IN (
				SELECT u.id
				FROM users u
				JOIN remote_access_members ram ON ram.local_user_id = u.id
				WHERE ram.status <> 'active'
					AND u.auth_origin = 'portico'
					AND u.role <> 'owner'
			)`); err != nil {
			return err
		}
		rows, err := tx.Query(`
			SELECT DISTINCT u.id
			FROM users u
			JOIN remote_access_members ram ON ram.local_user_id = u.id
			WHERE ram.status <> 'active' AND u.auth_origin = 'portico'`)
		if err != nil {
			return err
		}
		inactiveUserIDs := []string{}
		for rows.Next() {
			var userID string
			if err := rows.Scan(&userID); err != nil {
				rows.Close()
				return err
			}
			inactiveUserIDs = append(inactiveUserIDs, userID)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, userID := range inactiveUserIDs {
			if !fenced[userID] {
				return errRemoteAuthorityFenceRetry
			}
			if err := s.revokeAccountAuthorityTx(context.Background(), tx, userID, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func normalizeRemoteAccessMemberProfileURLs(hostedBaseURL string, members []RemoteAccessMember) []RemoteAccessMember {
	normalized := make([]RemoteAccessMember, 0, len(members))
	for _, member := range members {
		normalized = append(normalized, normalizeRemoteAccessMemberProfileURL(hostedBaseURL, member))
	}
	return normalized
}

func normalizeRemoteAccessMemberProfileURL(hostedBaseURL string, member RemoteAccessMember) RemoteAccessMember {
	member.ProfileImageURL = absoluteHostedProfileURL(hostedBaseURL, member.ProfileImageURL)
	return member
}

func absoluteHostedProfileURL(hostedBaseURL, profileImageURL string) string {
	profileImageURL = strings.TrimSpace(profileImageURL)
	if profileImageURL == "" {
		return ""
	}
	parsed, err := url.Parse(profileImageURL)
	if err == nil && parsed.IsAbs() {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return profileImageURL
		}
		return ""
	}
	if !strings.HasPrefix(profileImageURL, "/") {
		return profileImageURL
	}
	base := strings.TrimRight(strings.TrimSpace(hostedBaseURL), "/")
	if base == "" {
		base = defaultHostedBaseURL
	}
	return base + profileImageURL
}

func (s *Server) linkSingleLocalOwnerForPorticoMemberTx(tx *sql.Tx, member RemoteAccessMember, status string, now string) (string, error) {
	if status != "active" || !strings.EqualFold(strings.TrimSpace(member.Role), "owner") || member.PorticoMembershipID == "" || member.PorticoUserID == "" {
		return "", nil
	}
	var existingID string
	err := tx.QueryRow(`
		SELECT id
		FROM users
		WHERE portico_membership_id = ? OR portico_user_id = ?
		ORDER BY CASE WHEN portico_membership_id = ? THEN 0 ELSE 1 END
		LIMIT 1`, member.PorticoMembershipID, member.PorticoUserID, member.PorticoMembershipID).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	rows, err := tx.Query(`SELECT id FROM users WHERE role = 'owner' ORDER BY created_at ASC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	owners := []string{}
	for rows.Next() {
		var ownerID string
		if err := rows.Scan(&ownerID); err != nil {
			return "", err
		}
		owners = append(owners, ownerID)
		if len(owners) > 1 {
			return "", nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(owners) != 1 {
		return "", nil
	}
	_, err = tx.Exec(`
		UPDATE users
		SET portico_user_id = ?, portico_membership_id = ?, auth_origin = 'portico', updated_at = ?
		WHERE id = ? AND role = 'owner'`,
		member.PorticoUserID, member.PorticoMembershipID, now, owners[0])
	if err != nil {
		return "", err
	}
	return owners[0], nil
}

func (s *Server) userForPorticoMembership(member RemoteAccessMember) (User, error) {
	if member.PorticoMembershipID == "" {
		member.PorticoMembershipID = member.ID
	}
	if member.PorticoUserID == "" {
		member.PorticoUserID = member.UserID
	}
	if member.PorticoMembershipID == "" || member.PorticoUserID == "" || member.Status != "active" {
		return User{}, sql.ErrNoRows
	}
	var mappedUserID string
	err := s.queryUserRow(context.Background(), `SELECT local_user_id FROM remote_access_members WHERE portico_membership_id = ? AND status = 'active'`, member.PorticoMembershipID).Scan(&mappedUserID)
	if err == nil && strings.TrimSpace(mappedUserID) != "" {
		return s.getUser(mappedUserID)
	}
	var userID string
	err = s.queryUserRow(context.Background(), `
		SELECT id
		FROM users
		WHERE portico_membership_id = ? OR portico_user_id = ?
		ORDER BY CASE WHEN portico_membership_id = ? THEN 0 ELSE 1 END
		LIMIT 1`, member.PorticoMembershipID, member.PorticoUserID, member.PorticoMembershipID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC().Format(time.RFC3339)
		err = s.withUserTxTagged(context.Background(), []string{"settings"}, func(tx *sql.Tx) error {
			var provisionErr error
			userID, provisionErr = s.provisionPorticoProfileTx(tx, member, member.Status, now)
			if provisionErr != nil {
				return provisionErr
			}
			permissionTemplateJSON, marshalErr := json.Marshal(member.PermissionTemplate)
			if marshalErr != nil {
				return marshalErr
			}
			_, provisionErr = tx.Exec(`
				INSERT INTO remote_access_members (portico_membership_id, portico_user_id, email, display_name, role, status, permission_template_json, local_user_id, last_synced_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(portico_membership_id) DO UPDATE SET
					portico_user_id = excluded.portico_user_id,
					email = excluded.email,
					display_name = excluded.display_name,
					role = excluded.role,
					status = excluded.status,
					permission_template_json = excluded.permission_template_json,
					local_user_id = excluded.local_user_id,
					last_synced_at = excluded.last_synced_at`,
				member.PorticoMembershipID, member.PorticoUserID, member.Email, member.DisplayName, member.Role, member.Status, string(permissionTemplateJSON), userID, now)
			return provisionErr
		})
		if err != nil {
			return User{}, err
		}
	} else if err != nil {
		return User{}, err
	}
	return s.getUser(userID)
}

func (s *Server) provisionPorticoProfileTx(tx *sql.Tx, member RemoteAccessMember, status string, now string) (string, error) {
	if member.PorticoMembershipID == "" {
		member.PorticoMembershipID = member.ID
	}
	if member.PorticoUserID == "" {
		member.PorticoUserID = member.UserID
	}
	if member.PorticoMembershipID == "" || member.PorticoUserID == "" || status != "active" {
		return "", nil
	}
	var existingID, existingAuthOrigin, existingRole string
	err := tx.QueryRow(`
		SELECT id, COALESCE(auth_origin, 'local'), role
		FROM users
		WHERE portico_membership_id = ? OR portico_user_id = ?
		ORDER BY CASE WHEN portico_membership_id = ? THEN 0 ELSE 1 END
		LIMIT 1`, member.PorticoMembershipID, member.PorticoUserID, member.PorticoMembershipID).Scan(&existingID, &existingAuthOrigin, &existingRole)
	if err == nil {
		role := normalizeUserRole(member.Role)
		if role == "" {
			role = "user"
		}
		if role == "owner" {
			role = "user"
		}
		permissions, _, maxContentRating, policyErr := s.permissionsFromRemoteTemplateTx(tx, role, member.PermissionTemplate)
		if policyErr != nil {
			return "", policyErr
		}
		permissionsJSON, _ := json.Marshal(permissions)
		if err := releaseDisposablePorticoEmailCollisionTx(tx, member.Email, existingID, member, now); err != nil {
			return existingID, err
		}
		if err := releaseDisposablePorticoSubjectCollisionTx(tx, member.PorticoUserID, existingID); err != nil {
			return existingID, err
		}
		_, err = tx.Exec(`
			UPDATE users
			SET portico_user_id = ?, portico_membership_id = ?, auth_origin = 'portico',
				email = CASE WHEN auth_origin = 'portico' THEN ? WHEN email = '' THEN ? ELSE email END,
				display_name = CASE WHEN auth_origin = 'portico' THEN ? WHEN display_name = '' THEN ? ELSE display_name END,
				profile_image_url = CASE WHEN auth_origin = 'portico' THEN ? ELSE profile_image_url END,
				preferences_json = CASE WHEN auth_origin = 'portico' THEN ? ELSE preferences_json END,
				role = CASE WHEN role = 'owner' OR auth_origin <> 'portico' THEN role ELSE ? END,
				permissions_json = CASE WHEN role = 'owner' OR auth_origin <> 'portico' THEN permissions_json ELSE ? END,
				max_content_rating = CASE WHEN role = 'owner' OR auth_origin <> 'portico' THEN max_content_rating ELSE ? END,
				updated_at = ?
			WHERE id = ?`,
			member.PorticoUserID, member.PorticoMembershipID,
			strings.ToLower(strings.TrimSpace(member.Email)), strings.ToLower(strings.TrimSpace(member.Email)),
			porticoDisplayName(member), porticoDisplayName(member), strings.TrimSpace(member.ProfileImageURL), remoteMemberPreferencesJSON(member),
			role, string(permissionsJSON), maxContentRating, now, existingID)
		if err != nil {
			return existingID, err
		}
		// Hosted owns generic grants and limits only. Library access is a
		// server-local assignment, so every policy refresh preserves mappings
		// already held by an existing local profile.
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	role := normalizeUserRole(member.Role)
	if role == "" || role == "owner" {
		role = "user"
	}
	permissions, libraryIDs, maxContentRating, err := s.permissionsFromRemoteTemplateTx(tx, role, member.PermissionTemplate)
	if err != nil {
		return "", err
	}
	permissionsJSON, _ := json.Marshal(permissions)
	preferencesJSON := remoteMemberPreferencesJSON(member)
	email := strings.ToLower(strings.TrimSpace(member.Email))
	if email == "" {
		email = member.PorticoUserID + "@portico-account.invalid"
	}
	displayName := porticoDisplayName(member)
	if err := releaseDisposablePorticoEmailCollisionTx(tx, email, "", member, now); err != nil {
		return "", err
	}
	if err := releaseDisposablePorticoSubjectCollisionTx(tx, member.PorticoUserID, ""); err != nil {
		return "", err
	}
	userID := randomID("usr")
	username := uniquePorticoAccountUsername(tx, member)
	if _, err := tx.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, profile_image_url, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, 'portico', ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, username, email, displayName, role, member.PorticoUserID, member.PorticoMembershipID, strings.TrimSpace(member.ProfileImageURL), string(permissionsJSON), preferencesJSON, maxContentRating, now, now); err != nil {
		return "", err
	}
	if err = replaceUserLibraries(tx, userID, libraryIDs, now); err != nil {
		return "", err
	}
	return userID, nil
}

func releaseDisposablePorticoEmailCollisionTx(tx *sql.Tx, email, targetUserID string, member RemoteAccessMember, now string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || strings.HasSuffix(email, ".invalid") {
		return nil
	}
	rows, err := tx.Query(`
		SELECT id, COALESCE(auth_origin, 'local'), role, COALESCE(password_hash, ''),
			COALESCE(portico_user_id, ''), COALESCE(portico_membership_id, ''),
			(SELECT COUNT(*) FROM sessions WHERE user_id = users.id),
			(SELECT COUNT(*) FROM native_refresh_tokens WHERE user_id = users.id),
			(SELECT COUNT(*) FROM devices WHERE user_id = users.id),
			(SELECT COUNT(*) FROM api_keys WHERE user_id = users.id),
			(SELECT COUNT(*) FROM browser_account_entries WHERE user_id = users.id),
			(SELECT COUNT(*) FROM local_credentials lc JOIN profile_identities pi ON pi.id = lc.profile_identity_id WHERE pi.profile_id = users.id),
			(SELECT COUNT(*) FROM remote_access_members ram WHERE ram.local_user_id = users.id AND ram.status = 'active')
		FROM users
		WHERE lower(email) = lower(?)
			AND id <> ?`, email, targetUserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type collision struct {
		userID              string
		authOrigin          string
		role                string
		passwordHash        string
		porticoUserID       string
		porticoMembershipID string
		sessions            int
		refreshCredentials  int
		devices             int
		apiKeys             int
		browserEntries      int
		localCredentials    int
		activeRemoteLinks   int
	}
	var collisions []collision
	for rows.Next() {
		var current collision
		if err := rows.Scan(
			&current.userID, &current.authOrigin, &current.role, &current.passwordHash,
			&current.porticoUserID, &current.porticoMembershipID,
			&current.sessions, &current.refreshCredentials, &current.devices,
			&current.apiKeys, &current.browserEntries, &current.localCredentials, &current.activeRemoteLinks,
		); err != nil {
			return err
		}
		collisions = append(collisions, current)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, current := range collisions {
		authOrigin := strings.ToLower(strings.TrimSpace(current.authOrigin))
		disposableOrigin := authOrigin == "local"
		if strings.TrimSpace(current.porticoUserID) != "" || strings.TrimSpace(current.porticoMembershipID) != "" {
			return fmt.Errorf("%w: email belongs to a different Portico subject", errPorticoIdentityConflict)
		}
		if !disposableOrigin || strings.EqualFold(strings.TrimSpace(current.role), "owner") || strings.TrimSpace(current.passwordHash) != "" || current.sessions > 0 || current.refreshCredentials > 0 || current.devices > 0 || current.apiKeys > 0 || current.browserEntries > 0 || current.localCredentials > 0 || current.activeRemoteLinks > 0 {
			return fmt.Errorf("%w: authenticate the existing local profile before linking this Portico account", errPorticoIdentityLinkRequired)
		}
	}
	for _, current := range collisions {
		deletedEmail, err := uniqueDeletedPorticoPrincipalEmailTx(tx, member.PorticoUserID, current.userID)
		if err != nil {
			return err
		}
		deletedUsername, err := uniqueDeletedPorticoPrincipalUsernameTx(tx, member.PorticoUserID, current.userID)
		if err != nil {
			return err
		}
		// Remove the abandoned identity projection before quarantining the empty
		// local placeholder so the authoritative provider/subject key stays unique.
		if _, err := tx.Exec(`DELETE FROM profile_identities WHERE profile_id = ? AND provider = 'portico'`, current.userID); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			UPDATE users
			SET auth_origin = 'portico_deleted',
				portico_user_id = '',
				portico_membership_id = '',
				email = ?,
				username = ?,
				password_hash = NULL,
				updated_at = ?
			WHERE id = ?`,
			deletedEmail,
			deletedUsername,
			now,
			current.userID); err != nil {
			return err
		}
	}
	return nil
}

func releaseDisposablePorticoSubjectCollisionTx(tx *sql.Tx, porticoUserID, targetUserID string) error {
	porticoUserID = strings.TrimSpace(porticoUserID)
	if porticoUserID == "" {
		return nil
	}
	rows, err := tx.Query(`
		SELECT pi.id, pi.profile_id, COALESCE(u.auth_origin, 'local'), u.role,
			COALESCE(u.password_hash, ''), COALESCE(u.portico_user_id, ''), COALESCE(u.portico_membership_id, ''),
			(SELECT COUNT(*) FROM sessions s WHERE s.user_id = u.id OR s.profile_id = u.id OR s.profile_identity_id = pi.id),
			(SELECT COUNT(*) FROM native_refresh_tokens nrt WHERE nrt.user_id = u.id),
			(SELECT COUNT(*) FROM devices d WHERE d.user_id = u.id),
			(SELECT COUNT(*) FROM api_keys ak WHERE ak.user_id = u.id),
			(SELECT COUNT(*) FROM browser_account_entries bae WHERE bae.user_id = u.id OR bae.profile_identity_id = pi.id),
			(SELECT COUNT(*) FROM local_credentials lc JOIN profile_identities linked ON linked.id = lc.profile_identity_id WHERE linked.profile_id = u.id),
			(SELECT COUNT(*) FROM remote_access_members ram WHERE ram.local_user_id = u.id AND ram.status = 'active')
		FROM profile_identities pi
		JOIN users u ON u.id = pi.profile_id
		WHERE pi.provider = 'portico' AND pi.subject = ? AND pi.profile_id <> ?`, porticoUserID, targetUserID)
	if err != nil {
		return err
	}
	type collision struct {
		identityID          string
		profileID           string
		authOrigin          string
		role                string
		passwordHash        string
		linkedPorticoUserID string
		membershipID        string
		sessions            int
		refreshCredentials  int
		devices             int
		apiKeys             int
		browserEntries      int
		localCredentials    int
		activeRemoteLinks   int
	}
	var collisions []collision
	for rows.Next() {
		var current collision
		if err := rows.Scan(
			&current.identityID, &current.profileID, &current.authOrigin, &current.role,
			&current.passwordHash, &current.linkedPorticoUserID, &current.membershipID,
			&current.sessions, &current.refreshCredentials, &current.devices, &current.apiKeys,
			&current.browserEntries, &current.localCredentials, &current.activeRemoteLinks,
		); err != nil {
			rows.Close()
			return err
		}
		collisions = append(collisions, current)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, current := range collisions {
		authOrigin := strings.ToLower(strings.TrimSpace(current.authOrigin))
		disposableOrigin := authOrigin == "cloud" || authOrigin == "local" || authOrigin == "cloud_deleted" || authOrigin == "portico_deleted"
		if !disposableOrigin || strings.EqualFold(strings.TrimSpace(current.role), "owner") ||
			strings.TrimSpace(current.passwordHash) != "" || strings.TrimSpace(current.linkedPorticoUserID) != "" || strings.TrimSpace(current.membershipID) != "" ||
			current.sessions > 0 || current.refreshCredentials > 0 || current.devices > 0 || current.apiKeys > 0 ||
			current.browserEntries > 0 || current.localCredentials > 0 || current.activeRemoteLinks > 0 {
			return fmt.Errorf("%w: Portico subject is linked to an existing protected profile", errPorticoIdentityConflict)
		}
	}
	for _, current := range collisions {
		if _, err := tx.Exec(`DELETE FROM profile_identities WHERE id = ?`, current.identityID); err != nil {
			return err
		}
	}
	return nil
}

func sanitizePorticoCacheIdentifier(value string) string {
	base := strings.ToLower(strings.TrimSpace(value))
	base = strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z':
			return char
		case char >= '0' && char <= '9':
			return char
		default:
			return '-'
		}
	}, base)
	return strings.Trim(base, "-")
}

func (s *Server) permissionsFromRemoteTemplateTx(tx *sql.Tx, role string, template RemotePermissionTemplate) (map[string]bool, []string, string, error) {
	if !remotePermissionTemplatePresent(template) {
		return restrictedPorticoPermissions(), nil, "", nil
	}
	permissions := permissionsForRole(role)
	if len(template.Permissions) > 0 {
		permissions = sanitizePermissions(template.Permissions, role)
	}
	maxContentRating := ""
	if role != "owner" {
		maxContentRating = normalizeMaxContentRating(template.MaxContentRating)
	}
	return permissions, []string{}, maxContentRating, nil
}

func remotePermissionTemplatePresent(template RemotePermissionTemplate) bool {
	return len(template.Permissions) > 0 || strings.TrimSpace(template.MaxContentRating) != ""
}

func cleanLibraryIDsTx(tx *sql.Tx, input []string) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM libraries ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	valid := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		valid[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, raw := range input {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		if !valid[id] {
			return nil, fmt.Errorf("library %s does not exist", id)
		}
		seen[id] = true
		clean = append(clean, id)
	}
	return clean, nil
}

func allLibraryIDsTx(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM libraries ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	libraryIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		libraryIDs = append(libraryIDs, id)
	}
	return libraryIDs, rows.Err()
}

func restrictedPorticoPermissions() map[string]bool {
	permissions := permissionsForRole("user")
	for key := range permissions {
		permissions[key] = false
	}
	permissions["playMedia"] = true
	return permissions
}

func porticoDisplayName(member RemoteAccessMember) string {
	if value := strings.TrimSpace(member.DisplayName); value != "" {
		return value
	}
	if email := strings.TrimSpace(member.Email); email != "" {
		return strings.Split(email, "@")[0]
	}
	return "Portico Member"
}

func remoteMemberPreferencesJSON(member RemoteAccessMember) string {
	preferences := member.Preferences
	if strings.TrimSpace(preferences.Locale) == "" {
		preferences = defaultUserPreferences()
	}
	bytes, err := marshalUserPreferencesWithPolicies(preferences, UserAccessSchedule{}, UserTagPolicy{}, UserDevicePolicy{Mode: "any"}, UserChannelPolicy{})
	if err != nil {
		bytes, _ = json.Marshal(defaultUserPreferences())
	}
	return string(bytes)
}

type usernameQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func uniquePorticoAccountUsername(tx *sql.Tx, member RemoteAccessMember) string {
	return uniquePorticoAccountUsernameFrom(tx, member)
}

func uniquePorticoAccountUsernameTx(db *sql.DB, member RemoteAccessMember) string {
	return uniquePorticoAccountUsernameFrom(db, member)
}

func uniquePorticoAccountUsernameFrom(querier usernameQuerier, member RemoteAccessMember) string {
	base := normalizeUsername(strings.Split(strings.TrimSpace(member.Email), "@")[0])
	if base == "" {
		base = normalizeUsername(porticoDisplayName(member))
	}
	if base == "" {
		base = "portico-user"
	}
	candidate := base
	for index := 0; index < 50; index++ {
		var count int
		_ = querier.QueryRow(`SELECT COUNT(*) FROM users WHERE lower(username) = lower(?)`, candidate).Scan(&count)
		if count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, index+2)
	}
	return base + "-" + randomID("cld")
}

func (s *Server) listRemoteAccessMembers() []RemoteAccessMember {
	rows, err := s.queryUserRead(context.Background(), `SELECT portico_membership_id, portico_user_id, email, display_name, role, status, COALESCE(permission_template_json, '{}'), local_user_id, last_synced_at FROM remote_access_members ORDER BY email ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var members []RemoteAccessMember
	for rows.Next() {
		var member RemoteAccessMember
		var permissionTemplateJSON string
		if err := rows.Scan(&member.PorticoMembershipID, &member.PorticoUserID, &member.Email, &member.DisplayName, &member.Role, &member.Status, &permissionTemplateJSON, &member.LocalUserID, &member.LastSyncedAt); err == nil {
			if err := json.Unmarshal([]byte(permissionTemplateJSON), &member.PermissionTemplate); err != nil {
				continue
			}
			members = append(members, member)
		}
	}
	return members
}

func (s *Server) mapRemoteAccessMember(memberID, localUserID string) error {
	result, err := s.execUserWrite(context.Background(), `UPDATE remote_access_members SET local_user_id = ? WHERE portico_membership_id = ?`, localUserID, memberID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func applyRemoteAccessSettingsPatch(settings *RemoteAccessSettings, patch RemoteAccessSettingsPatch) {
	if patch.Enabled != nil {
		settings.Enabled = *patch.Enabled
	}
	if patch.HostedBaseURL != nil {
		settings.HostedBaseURL = strings.TrimSpace(*patch.HostedBaseURL)
	}
	if patch.PublicPortMode != nil {
		settings.PublicPortMode = strings.TrimSpace(*patch.PublicPortMode)
	}
	if patch.ManualPublicPort != nil {
		settings.ManualPublicPort = *patch.ManualPublicPort
	}
	if patch.PreferredRemoteAuthMode != nil {
		settings.PreferredRemoteAuthMode = strings.TrimSpace(*patch.PreferredRemoteAuthMode)
	}
	if patch.AllowManualLocalAuthRemoteLogin != nil {
		settings.AllowManualLocalAuthRemoteLogin = *patch.AllowManualLocalAuthRemoteLogin
	}
	if patch.LANDiscoveryEnabled != nil {
		settings.LANDiscoveryEnabled = *patch.LANDiscoveryEnabled
	}
	if patch.RouterAutomationEnabled != nil {
		settings.RouterAutomationEnabled = *patch.RouterAutomationEnabled
	}
	if patch.RemoteBitrateLimitMbps != nil {
		settings.RemoteBitrateLimitMbps = normalizeRemoteBitrateLimitMbps(*patch.RemoteBitrateLimitMbps)
	}
	if patch.CustomCertificateEnabled != nil {
		settings.CustomCertificateEnabled = *patch.CustomCertificateEnabled
	}
	if patch.CustomCertificatePath != nil {
		settings.CustomCertificatePath = strings.TrimSpace(*patch.CustomCertificatePath)
	}
	if patch.CustomCertificateKeyPath != nil {
		settings.CustomCertificateKeyPath = strings.TrimSpace(*patch.CustomCertificateKeyPath)
	}
}

func (s *Server) defaultHostedBaseURL() string {
	// Tests and embedded callers that construct a zero Config retain the
	// historical fixture authority. Real runtime configuration always carries
	// the explicit Foundation environment and generated/default authority.
	if strings.TrimSpace(s.cfg.Environment) == "" {
		return defaultHostedBaseURL
	}
	if authority := strings.TrimSpace(s.cfg.HostedAPIAuthority); authority != "" && authority != "REQUIRED_EXTERNAL_CONFIGURATION" {
		return strings.TrimRight(authority, "/")
	}
	return defaultHostedBaseURL
}

func (s *Server) validateRemoteAccessSettings(settings RemoteAccessSettings) error {
	if err := validateRemoteAccessSettings(settings); err != nil {
		return err
	}
	environment := strings.ToLower(strings.TrimSpace(s.cfg.Environment))
	configuredAuthority := strings.TrimSpace(s.cfg.HostedAPIAuthority)
	// A zero Config is used by in-process unit fixtures. Production/runtime
	// configuration is loaded through config.Load and always has an explicit
	// environment, so this branch cannot weaken protected runtime authority.
	if environment == "" && configuredAuthority == "" {
		return nil
	}
	switch environment {
	case "development", "test", "staging", "production":
	default:
		return errors.New("PORTICO_ENVIRONMENT must be exactly development, test, staging, or production")
	}
	if configuredAuthority == "" || configuredAuthority == "REQUIRED_EXTERNAL_CONFIGURATION" {
		if environment == "staging" || environment == "production" {
			return errors.New("protected Hosted authority is unavailable; configure PORTICO_HOSTED_API_AUTHORITY")
		}
		if parsed, err := url.Parse(strings.TrimSpace(settings.HostedBaseURL)); err == nil && parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return errors.New("development/test custom Hosted authority requires explicit PORTICO_HOSTED_API_AUTHORITY")
	}
	approved, err := url.Parse(strings.TrimRight(configuredAuthority, "/"))
	if err != nil || approved.Scheme == "" || approved.Host == "" || approved.User != nil || approved.RawQuery != "" || approved.Fragment != "" || (approved.Path != "" && approved.Path != "/") {
		return errors.New("PORTICO_HOSTED_API_AUTHORITY must be an exact origin without credentials, path, query, or fragment")
	}
	configured, err := url.Parse(strings.TrimSpace(settings.HostedBaseURL))
	if err != nil || !sameHTTPOrigin(configured, approved) {
		return errors.New("hostedBaseUrl does not match the approved Hosted authority")
	}
	return nil
}

func validateRemoteAccessSettings(settings RemoteAccessSettings) error {
	parsed, err := url.Parse(settings.HostedBaseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" {
		return errors.New("hostedBaseUrl must be an origin without credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return errors.New("hostedBaseUrl must be HTTPS outside loopback local development")
	}
	switch settings.PublicPortMode {
	case "manual", "automatic", "disabled":
	default:
		return errors.New("publicPortMode must be manual, automatic, or disabled")
	}
	if settings.PublicPortMode != "disabled" && (settings.ManualPublicPort < 1 || settings.ManualPublicPort > 65535) {
		return errors.New("manualPublicPort must be between 1 and 65535")
	}
	switch settings.PreferredRemoteAuthMode {
	case "portico", "local":
	default:
		return errors.New("preferredRemoteAuthMode must be portico or local")
	}
	if settings.RouterAutomationEnabled && settings.PublicPortMode != "automatic" {
		return errors.New("routerAutomationEnabled requires publicPortMode automatic")
	}
	if settings.RemoteBitrateLimitMbps != normalizeRemoteBitrateLimitMbps(settings.RemoteBitrateLimitMbps) {
		return errors.New("remoteBitrateLimitMbps must be between 0 and 1000")
	}
	if settings.CustomCertificateEnabled {
		if !filepath.IsAbs(strings.TrimSpace(settings.CustomCertificatePath)) {
			return errors.New("customCertificatePath must be an absolute certificate chain path")
		}
		if !filepath.IsAbs(strings.TrimSpace(settings.CustomCertificateKeyPath)) {
			return errors.New("customCertificateKeyPath must be an absolute private key path")
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type serverIdentity struct {
	PublicKey   ed25519.PublicKey
	PrivateKey  ed25519.PrivateKey
	Fingerprint string
}

func (s *Server) loadOrCreateServerIdentity() (serverIdentity, error) {
	if err := s.reconcileIdentityReset(context.Background()); err != nil {
		return serverIdentity{}, err
	}
	path := s.serverIdentityKeyPath()
	bytes, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(bytes)
		if block == nil || block.Type != "PRIVATE KEY" {
			return serverIdentity{}, errors.New("invalid server identity key")
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return serverIdentity{}, err
		}
		privateKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return serverIdentity{}, errors.New("server identity key is not ed25519")
		}
		publicKey, ok := privateKey.Public().(ed25519.PublicKey)
		if !ok {
			return serverIdentity{}, errors.New("server identity public key unavailable")
		}
		return serverIdentity{PublicKey: publicKey, PrivateKey: privateKey, Fingerprint: publicKeyFingerprint(publicKey)}, nil
	}
	if !os.IsNotExist(err) {
		return serverIdentity{}, err
	}
	return s.writeNewServerIdentityKey()
}

func (s *Server) writeNewServerIdentityKey() (serverIdentity, error) {
	return generateServerIdentityAt(s.serverIdentityKeyPath())
}

func (s *Server) serverIdentityKeyPath() string {
	return filepath.Join(s.cfg.AppDataDir, "remote-access", "server-identity-ed25519.pem")
}

func (s *Server) resetServerIdentity(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	newServerID := randomID("srv")
	identityValue, err := json.Marshal(map[string]any{
		"serverId":  newServerID,
		"createdAt": now,
	})
	if err != nil {
		return err
	}
	remoteSettings, err := s.remoteAccessSettings()
	if err != nil {
		return err
	}
	remoteSettings.Enabled = false
	remoteSettings.ClaimStatus = "not_claimed"
	remoteSettings.ServerID = ""
	remoteSettings.AssignedHostname = ""
	remoteSettings.CertificateStatus = "not_requested"
	remoteSettings.CertificateExpiresAt = ""
	remoteSettings.LastCertificateRenewalAt = ""
	remoteSettings.CertificateRenewalError = ""
	remoteSettings.LastHeartbeatAt = ""
	remoteSettings.LastHeartbeatError = ""
	remoteSettings.LastHostedRemoteAccessState = ""
	remoteSettings.LastPublicRouteError = ""
	remoteSettings.RouterMappingStatus = ""
	remoteSettings.RouterMappingError = ""
	remoteSettings.LastRouterMappingAt = ""
	allSettings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return err
	}
	identitySetting, _ := allSettings["identity"].(map[string]any)
	remoteBytes, err := json.Marshal(remoteSettings)
	if err != nil {
		return err
	}
	previous, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return err
	}
	stagePath := filepath.Join(filepath.Dir(s.serverIdentityKeyPath()), "server-identity-reset-"+randomID("stage")+".pem")
	next, err := generateServerIdentityAt(stagePath)
	if err != nil {
		return err
	}
	journal := identityResetJournal{
		Version:             identityResetJournalVersion,
		OperationID:         randomID("identity-reset"),
		PreviousServerID:    settingString(identitySetting, "serverId", ""),
		PreviousFingerprint: previous.Fingerprint,
		NewServerID:         newServerID,
		NewFingerprint:      next.Fingerprint,
		StagedKeyPath:       stagePath,
		Publication:         "staged",
		CreatedAt:           now,
	}
	if err := s.writeIdentityResetJournal(journal); err != nil {
		_ = os.Remove(stagePath)
		return err
	}
	if err := s.withUserTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('identity', ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, string(identityValue), now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, remoteAccessSettingsKey, string(remoteBytes), now); err != nil {
			return err
		}
		for _, key := range []string{remoteAccessClaimKey, remoteAccessClaimTokenKey, remoteAccessClaimOperationKey, remoteAccessClaimReceiptKey, remoteAccessCredentialKey} {
			if _, err := tx.Exec(`DELETE FROM settings WHERE key = ?`, key); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	journal.DatabaseCommitted = true
	journal.Publication = "database_committed"
	if err := s.writeIdentityResetJournal(journal); err != nil {
		return err
	}
	if err := os.Rename(stagePath, s.serverIdentityKeyPath()); err != nil {
		return fmt.Errorf("publish reset server identity: %w", err)
	}
	return os.Remove(s.identityResetJournalPath())
}

func publicKeyFingerprint(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return "sha256:" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func humanClaimCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	var builder strings.Builder
	for _, value := range bytes {
		builder.WriteByte(alphabet[int(value)%len(alphabet)])
	}
	return builder.String()
}

func (s *Server) localRemoteAccessRoutes() []RemoteAccessRoute {
	routes := []RemoteAccessRoute{{
		Type:   "local",
		URL:    "http://" + s.cfg.Addr,
		Source: "listener",
	}}
	for _, accessURL := range s.networkSettings().CustomAccessURLs {
		routes = append(routes, RemoteAccessRoute{Type: "custom", URL: accessURL, Source: "network_settings"})
	}
	return routes
}

func (s *Server) remotePublicEndpoint(settings RemoteAccessSettings) RemoteAccessEndpoint {
	host := remoteAccessPublicHostname(settings.AssignedHostname, settings.LastPublicIPAddress)
	if host == "" {
		return RemoteAccessEndpoint{}
	}
	port := settings.ManualPublicPort
	if port <= 0 {
		port = defaultRemotePublicPort
	}
	return RemoteAccessEndpoint{
		Scheme: "https",
		Host:   host,
		Port:   port,
		URL:    fmt.Sprintf("https://%s:%d", host, port),
	}
}

func remoteAccessPublicHostname(assignedHostname, publicIPAddress string) string {
	assignedHostname = strings.ToLower(strings.Trim(strings.TrimSpace(assignedHostname), "."))
	if remoteAccessCertificateHostname(assignedHostname) == "" {
		return ""
	}
	publicIPAddress = validPublicIPString(publicIPAddress)
	if publicIPAddress == "" {
		return ""
	}
	addressLabel := strings.NewReplacer(".", "-", ":", "-").Replace(publicIPAddress)
	return addressLabel + "." + assignedHostname
}

func portFromAddress(address string) int {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0
	}
	if port, err := strconv.Atoi(strings.TrimPrefix(address, ":")); err == nil {
		return port
	}
	if _, portText, err := net.SplitHostPort(address); err == nil {
		port, _ := strconv.Atoi(portText)
		return port
	}
	if parsed, err := url.Parse(address); err == nil {
		port, _ := strconv.Atoi(parsed.Port())
		return port
	}
	return 0
}

func (s *Server) requireManageServer(w http.ResponseWriter, user User) bool {
	if canInteractivelyManageServer(user) {
		return true
	}
	writeError(w, http.StatusForbidden, "owner_required", "Only the server owner can manage remote access.")
	return false
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "invalid"
	}
	return parsed.Scheme + "://" + parsed.Host
}

func routerMappingAuditMetadata(settings RemoteAccessSettings, result RouterMappingResult) map[string]string {
	metadata := map[string]string{
		"status":     result.Status,
		"publicPort": strconv.Itoa(settings.ManualPublicPort),
		"publicMode": settings.PublicPortMode,
	}
	if result.Protocol != "" {
		metadata["protocol"] = result.Protocol
	}
	if result.Error != "" {
		metadata["error"] = result.Error
	}
	return metadata
}

func clientHostFromRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.Host)
	if err == nil {
		return host
	}
	return r.Host
}
