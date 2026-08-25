package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	hostedServerWakeKind        = "server-wake"
	hostedServerWakeMaxLifetime = 5 * time.Minute
	hostedServerWakeMaxBody     = 16 << 10
)

type hostedServerWake struct {
	Kind                  string `json:"kind"`
	Version               int    `json:"version"`
	Audience              string `json:"audience"`
	ServerID              string `json:"serverId"`
	TargetPolicyRevision  int64  `json:"targetPolicyRevision"`
	AccountID             string `json:"accountId,omitempty"`
	TargetProfileRevision int64  `json:"targetProfileRevision,omitempty"`
	Reason                string `json:"reason"`
	WakeID                string `json:"wakeId"`
	IssuedAt              string `json:"issuedAt"`
	ExpiresAt             string `json:"expiresAt"`
	SignatureAlgorithm    string `json:"signatureAlgorithm"`
	SignatureKeyID        string `json:"signatureKeyId"`
	Signature             string `json:"signature"`
}

type hostedProfileWakeTarget struct {
	Revision int64
	WakeID   string
}

func validHostedServerWakeReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "policy_changed", "authority_changed", "profile_authority_changed", "repair_requested", "route_changed":
		return true
	default:
		return false
	}
}

func decodeHostedServerWake(raw []byte) (hostedServerWake, error) {
	var wake hostedServerWake
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wake); err != nil {
		return hostedServerWake{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return hostedServerWake{}, errors.New("wake document has trailing content")
	}
	return wake, nil
}

func (s *Server) verifyHostedServerWake(raw []byte, wake hostedServerWake, expectedServerID string, now time.Time) error {
	if wake.Kind != hostedServerWakeKind || wake.Version != 1 || wake.Audience != hostedDocumentAudience {
		return errors.New("wake document contract is invalid")
	}
	if strings.TrimSpace(wake.ServerID) == "" || wake.ServerID != expectedServerID {
		return errors.New("wake subject does not match this server")
	}
	if wake.TargetPolicyRevision < 1 || !validHostedServerWakeReason(wake.Reason) || len(strings.TrimSpace(wake.WakeID)) < 8 || len(wake.WakeID) > 128 {
		return errors.New("wake authority metadata is invalid")
	}
	wake.AccountID = strings.TrimSpace(wake.AccountID)
	if wake.Reason == "profile_authority_changed" {
		if wake.AccountID == "" || len(wake.AccountID) > 256 || wake.TargetProfileRevision < 1 {
			return errors.New("wake profile authority metadata is invalid")
		}
	} else if wake.AccountID != "" || wake.TargetProfileRevision != 0 {
		return errors.New("wake contains profile authority outside a profile change")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, wake.IssuedAt)
	if err != nil {
		return errors.New("wake issuedAt is invalid")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, wake.ExpiresAt)
	if err != nil || !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > hostedServerWakeMaxLifetime {
		return errors.New("wake expiry is invalid")
	}
	if issuedAt.After(now.Add(hostedDocumentClockSkew)) || now.After(expiresAt.Add(hostedDocumentClockSkew)) {
		return errors.New("wake is outside its validity window")
	}
	if wake.SignatureAlgorithm != hostedSignatureAlgorithm || !validHostedDocumentSigningKeyID(wake.SignatureKeyID) {
		return errors.New("wake signature metadata is invalid")
	}
	publicKey, err := decodeHostedDocumentPublicKey(s.trustedHostedDocumentKey(wake.SignatureKeyID))
	if err != nil {
		return errors.New("wake signing key is not trusted")
	}
	payload, err := canonicalHostedDocument(hostedServerWakeKind, raw)
	if err != nil {
		return errors.New("wake canonical form is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(wake.Signature))
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("wake signature is invalid")
	}
	return nil
}

func (s *Server) handleHostedServerWake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	allowed, retryAfter := s.hostedWakeLimiter.allow(clientIPFromRequest(r), quickConnectRatePolicy{
		limit: 120, window: time.Minute, code: "hosted_wake_rate_limited", detail: "Hosted wake requests are arriving too quickly.",
	}, time.Now().UTC())
	if !allowed {
		w.Header().Set("Retry-After", itoa(retryAfter))
		writeError(w, http.StatusTooManyRequests, "hosted_wake_rate_limited", "Hosted wake requests are arriving too quickly.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, hostedServerWakeMaxBody)
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_hosted_wake", "The Hosted wake could not be verified.")
		return
	}
	wake, err := decodeHostedServerWake(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_hosted_wake", "The Hosted wake could not be verified.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil || settings.ClaimStatus != "claimed" || strings.TrimSpace(settings.ServerID) == "" {
		writeError(w, http.StatusUnauthorized, "invalid_hosted_wake", "The Hosted wake could not be verified.")
		return
	}
	if err := s.verifyHostedServerWake(raw, wake, settings.ServerID, time.Now().UTC()); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_hosted_wake", "The Hosted wake could not be verified.")
		return
	}
	duplicate, err := s.consumeHostedServerWake(r.Context(), wake, settings.ServerID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "hosted_wake_unavailable", "The Hosted wake could not be durably recorded.")
		return
	}
	if duplicate {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "duplicate": true, "targetPolicyRevision": wake.TargetPolicyRevision})
		return
	}
	s.queueHostedServerWake(wake)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "targetPolicyRevision": wake.TargetPolicyRevision})
}

// consumeHostedServerWake is the restart-safe replay fence. A verified wake is
// only queued after its WakeID has been inserted in the durable receipt table;
// the unique key makes concurrent duplicate deliveries converge to one queue
// action. Expiry and the hard row cap keep this acceleration-only cache bounded.
func (s *Server) consumeHostedServerWake(ctx context.Context, wake hostedServerWake, expectedServerID string, now time.Time) (bool, error) {
	wakeID := strings.TrimSpace(wake.WakeID)
	serverID := strings.TrimSpace(expectedServerID)
	if wakeID == "" || serverID == "" {
		return false, errors.New("wake replay identity is incomplete")
	}
	nowText := now.UTC().Format(time.RFC3339Nano)
	expiresAt, err := time.Parse(time.RFC3339Nano, wake.ExpiresAt)
	if err != nil {
		return false, err
	}
	var duplicate bool
	err = s.withUserTxTagged(ctx, []string{"hosted_wake_replays"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM hosted_wake_replays WHERE expires_at <= ?`, nowText); err != nil {
			return err
		}
		result, err := tx.Exec(`INSERT INTO hosted_wake_replays (wake_id, server_id, received_at, expires_at) VALUES (?, ?, ?, ?) ON CONFLICT(wake_id) DO NOTHING`, wakeID, serverID, nowText, expiresAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		duplicate = changed == 0
		if _, err := tx.Exec(`DELETE FROM hosted_wake_replays WHERE wake_id NOT IN (SELECT wake_id FROM hosted_wake_replays ORDER BY received_at DESC LIMIT 4096)`); err != nil {
			return err
		}
		return nil
	})
	return duplicate, err
}

func (s *Server) queueHostedServerWake(wake hostedServerWake) {
	s.hostedWakeMu.Lock()
	if wake.TargetPolicyRevision > s.hostedWakePendingRevision {
		s.hostedWakePendingRevision = wake.TargetPolicyRevision
	}
	if wake.Reason == "repair_requested" || wake.Reason == "route_changed" {
		s.hostedWakePendingRepair = true
	}
	if wake.Reason == "profile_authority_changed" {
		if s.hostedWakePendingProfiles == nil {
			s.hostedWakePendingProfiles = map[string]hostedProfileWakeTarget{}
		}
		current := s.hostedWakePendingProfiles[wake.AccountID]
		if wake.TargetProfileRevision >= current.Revision {
			s.hostedWakePendingProfiles[wake.AccountID] = hostedProfileWakeTarget{Revision: wake.TargetProfileRevision, WakeID: wake.WakeID}
		}
	}
	if s.hostedWakeRunning {
		s.hostedWakeMu.Unlock()
		return
	}
	s.hostedWakeRunning = true
	s.hostedWakeMu.Unlock()
	if !s.startOwnedAsync("hosted-server-wake", s.runHostedServerWakes) {
		s.hostedWakeMu.Lock()
		s.hostedWakeRunning = false
		s.hostedWakeMu.Unlock()
	}
}

func (s *Server) nextHostedServerWake() (int64, bool, map[string]hostedProfileWakeTarget, bool) {
	s.hostedWakeMu.Lock()
	defer s.hostedWakeMu.Unlock()
	revision, repair := s.hostedWakePendingRevision, s.hostedWakePendingRepair
	profiles := s.hostedWakePendingProfiles
	if revision == 0 && !repair && len(profiles) == 0 {
		s.hostedWakeRunning = false
		return 0, false, nil, false
	}
	s.hostedWakePendingRevision = 0
	s.hostedWakePendingRepair = false
	s.hostedWakePendingProfiles = nil
	return revision, repair, profiles, true
}

func (s *Server) runHostedServerWakes(ctx context.Context) {
	for {
		targetRevision, repair, profileRevisions, ok := s.nextHostedServerWake()
		if !ok {
			return
		}
		settings, err := s.remoteAccessSettings()
		credential := s.secretSetting(remoteAccessCredentialKey)
		if err != nil || settings.ClaimStatus != "claimed" || settings.ServerID == "" || credential == "" {
			s.recordLog("warn", "Hosted server wake could not load claimed-server state", map[string]string{"error": firstNonEmpty(errorString(err), "server credential unavailable")})
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		policyState := s.loadRemotePolicyState()
		if targetRevision > policyState.PolicyRevision {
			err = s.syncRemoteAccessMembers(attemptCtx, settings, credential)
		} else if policyState.AckPending {
			err = s.retryRemotePolicyAck(attemptCtx, settings, credential)
		}
		heartbeatSent := false
		if err == nil && repair {
			heartbeatSent, err = s.checkRemoteAccessRepairSignalAndRepair(attemptCtx)
		}
		if err == nil {
			for hostedAccountID, target := range profileRevisions {
				if profileErr := s.refreshHostedProfileAuthorityFromWake(attemptCtx, hostedAccountID, target.Revision); profileErr != nil {
					err = profileErr
					break
				}
				if profileErr := s.ackHostedProfileWake(attemptCtx, settings, credential, target.WakeID, hostedAccountID, target.Revision); profileErr != nil {
					err = profileErr
					break
				}
			}
		}
		if err == nil && !heartbeatSent {
			// Confirm the applied revision even when the original policy ACK was
			// lost. Hosted keeps its durable wake pending until this revision fence
			// is observed, so an accepted callback is never mistaken for application.
			err = s.sendRemoteAccessHeartbeatWithOptions(attemptCtx, settings, remoteAccessHeartbeatOptions{SyncPolicy: false, SuppressRepair: repair})
		}
		cancel()
		if err != nil {
			s.recordLog("warn", "Hosted server wake refresh failed", map[string]string{"error": err.Error(), "targetPolicyRevision": strconv.FormatInt(targetRevision, 10)})
		}
	}
}

func (s *Server) ackHostedProfileWake(ctx context.Context, settings RemoteAccessSettings, credential, wakeID, hostedAccountID string, targetRevision int64) error {
	wakeID = strings.TrimSpace(wakeID)
	hostedAccountID = strings.TrimSpace(hostedAccountID)
	if wakeID == "" || hostedAccountID == "" || targetRevision < 1 {
		return errors.New("Hosted profile wake acknowledgement is incomplete")
	}
	body, err := json.Marshal(map[string]any{
		"wakeId":                wakeID,
		"accountId":             hostedAccountID,
		"targetProfileRevision": targetRevision,
	})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/profile-wake-ack"
	return s.hostedJSON(ctx, http.MethodPost, endpoint, credential, body, nil)
}

func (s *Server) refreshHostedProfileAuthorityFromWake(ctx context.Context, hostedAccountID string, targetRevision int64) error {
	var localAccountID string
	err := s.queryUserRow(ctx, `SELECT id FROM users WHERE auth_origin = 'portico' AND portico_user_id = ? AND COALESCE(disabled_at, '') = ''`, strings.TrimSpace(hostedAccountID)).Scan(&localAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		// The membership may have been removed by the same policy revision. There
		// is then no local profile authority left to refresh.
		return nil
	}
	if err != nil {
		return err
	}
	previous, previousErr := s.hostedProfileStateContext(ctx, localAccountID)
	if previousErr == nil && previous.Revision >= targetRevision {
		return nil
	}
	if err := s.refreshHostedProfileDirectoryContext(ctx, localAccountID, previous, previousErr, time.Now().UTC()); err != nil {
		return err
	}
	applied, err := s.hostedProfileStateContext(ctx, localAccountID)
	if err != nil || applied.Revision < targetRevision {
		return errors.New("Hosted profile authority revision did not converge")
	}
	return nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
