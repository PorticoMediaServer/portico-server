package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	hostedProfileFreshnessLease     = 30 * time.Minute
	hostedProfileStaleIfError       = maximumPolicyLifetime
	hostedProfileRefreshRetryBase   = 30 * time.Second
	hostedProfileRefreshRetryJitter = 30 * time.Second
	hostedProfileRetryAfterSeconds  = int(hostedProfileRefreshRetryBase / time.Second)
	hostedProfileRequestTimeout     = 2 * time.Second
	hostedProfileRefreshAheadBase   = 5 * time.Minute
	hostedProfileRefreshAheadJitter = 2 * time.Minute
	hostedProfileDirectoryMaxSkew   = 30 * time.Second
	hostedProfileDirectoryMaxAgeSec = int(hostedProfileFreshnessLease / time.Second)
	hostedProfileRefreshConcurrency = 4
	hostedProfileRefreshBacklog     = 64
)

var (
	errHostedProfileDirectoryUnavailable = errors.New("Hosted profile directory is temporarily unavailable")
	errHostedProfileAccessRevoked        = errors.New("Hosted profile access was revoked")
)

type HostedProfileDirectorySnapshot struct {
	Version             string                  `json:"version"`
	SnapshotID          string                  `json:"snapshotId"`
	Audience            string                  `json:"audience"`
	ServerID            string                  `json:"serverId"`
	AccountID           string                  `json:"accountId"`
	Status              string                  `json:"status"`
	Revision            int64                   `json:"revision"`
	Profiles            []HostedProfileSnapshot `json:"profiles,omitempty"`
	CheckedAt           string                  `json:"checkedAt"`
	MaxAgeSeconds       int                     `json:"maxAgeSeconds"`
	StaleIfErrorSeconds int                     `json:"staleIfErrorSeconds"`
	SignatureAlgorithm  string                  `json:"signatureAlgorithm"`
	SignatureKeyID      string                  `json:"signatureKeyId"`
	Signature           string                  `json:"signature"`
}

type hostedProfileSnapshotState struct {
	Revision            int64
	CheckedAt           time.Time
	MaxAgeSeconds       int
	StaleIfErrorSeconds int
	RefreshRetryAt      time.Time
}

type hostedProfileRefreshCall struct {
	done chan struct{}
	err  error
}

func decodeHostedProfileDirectorySnapshot(raw json.RawMessage) (HostedProfileDirectorySnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot HostedProfileDirectorySnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: %v", errInvalidHostedProfileSnapshot, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: trailing content", errInvalidHostedProfileSnapshot)
	}
	return snapshot, nil
}

func (s *Server) verifyHostedProfileDirectorySnapshot(raw json.RawMessage, expectedServerID, expectedAccountID string, now time.Time) (HostedProfileDirectorySnapshot, error) {
	snapshot, err := decodeHostedProfileDirectorySnapshot(raw)
	if err != nil {
		return HostedProfileDirectorySnapshot{}, err
	}
	snapshot.SnapshotID = strings.TrimSpace(snapshot.SnapshotID)
	snapshot.ServerID = strings.TrimSpace(snapshot.ServerID)
	snapshot.AccountID = strings.TrimSpace(snapshot.AccountID)
	if snapshot.Version != "v1" || snapshot.SnapshotID == "" || snapshot.Audience != hostedDocumentAudience ||
		snapshot.ServerID != strings.TrimSpace(expectedServerID) || snapshot.AccountID != strings.TrimSpace(expectedAccountID) ||
		snapshot.Revision <= 0 || (snapshot.Status != "unchanged" && snapshot.Status != "changed") ||
		snapshot.MaxAgeSeconds < 1 || snapshot.MaxAgeSeconds > hostedProfileDirectoryMaxAgeSec ||
		snapshot.StaleIfErrorSeconds < 0 || snapshot.StaleIfErrorSeconds > int(hostedProfileStaleIfError/time.Second) ||
		snapshot.SignatureAlgorithm != hostedSignatureAlgorithm || strings.TrimSpace(snapshot.Signature) == "" {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: profile directory snapshot metadata is invalid", errInvalidHostedProfileSnapshot)
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, snapshot.CheckedAt)
	if err != nil || checkedAt.After(now.Add(hostedProfileDirectoryMaxSkew)) || checkedAt.Before(now.Add(-hostedProfileFreshnessLease)) {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: checkedAt is invalid", errInvalidHostedProfileSnapshot)
	}
	if snapshot.Status == "unchanged" {
		if snapshot.Profiles != nil {
			return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: unchanged snapshot cannot contain profiles", errInvalidHostedProfileSnapshot)
		}
	} else if err := validateHostedProfileDirectoryProjection(snapshot.Profiles, snapshot.AccountID, checkedAt); err != nil {
		return HostedProfileDirectorySnapshot{}, err
	}
	encodedKey := s.trustedHostedDocumentKey(snapshot.SignatureKeyID)
	if encodedKey == "" {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: signing key is not trusted", errInvalidHostedProfileSnapshot)
	}
	publicKey, err := decodeHostedDocumentPublicKey(encodedKey)
	if err != nil {
		return HostedProfileDirectorySnapshot{}, err
	}
	payload, err := canonicalHostedDocument("profile-directory-snapshot", raw)
	if err != nil {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: canonicalization failed", errInvalidHostedProfileSnapshot)
	}
	signature, err := base64.RawURLEncoding.DecodeString(snapshot.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, payload, signature) {
		return HostedProfileDirectorySnapshot{}, fmt.Errorf("%w: signature is invalid", errInvalidHostedProfileSnapshot)
	}
	return snapshot, nil
}

func (s *Server) hostedProfileStateContext(ctx context.Context, accountID string) (hostedProfileSnapshotState, error) {
	var state hostedProfileSnapshotState
	var checkedAt, retryAt string
	err := s.queryUserRow(ctx, `
		SELECT revision, checked_at, max_age_seconds, stale_if_error_seconds, refresh_retry_at
		FROM hosted_profile_snapshot_state WHERE account_id = ?`, accountID).
		Scan(&state.Revision, &checkedAt, &state.MaxAgeSeconds, &state.StaleIfErrorSeconds, &retryAt)
	if err != nil {
		return hostedProfileSnapshotState{}, err
	}
	state.CheckedAt, err = time.Parse(time.RFC3339Nano, checkedAt)
	if err != nil || state.MaxAgeSeconds < 1 || state.MaxAgeSeconds > hostedProfileDirectoryMaxAgeSec ||
		state.StaleIfErrorSeconds < 0 || state.StaleIfErrorSeconds > int(hostedProfileStaleIfError/time.Second) {
		return hostedProfileSnapshotState{}, errInvalidHostedProfileSnapshot
	}
	if retryAt != "" {
		state.RefreshRetryAt, _ = time.Parse(time.RFC3339Nano, retryAt)
	}
	return state, nil
}

func (state hostedProfileSnapshotState) freshAt(now time.Time) bool {
	return state.CheckedAt.Add(time.Duration(state.MaxAgeSeconds) * time.Second).After(now)
}

func (state hostedProfileSnapshotState) staleErrorAllowedAt(now time.Time) bool {
	return state.CheckedAt.Add(time.Duration(state.MaxAgeSeconds+state.StaleIfErrorSeconds) * time.Second).After(now)
}

func hostedProfileRefreshAhead(accountID string) time.Duration {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(accountID))
	jitterSeconds := hasher.Sum32() % uint32(hostedProfileRefreshAheadJitter/time.Second+1)
	return hostedProfileRefreshAheadBase + time.Duration(jitterSeconds)*time.Second
}

func hostedProfileRefreshRetryDelay(accountID string) time.Duration {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte("retry:" + accountID))
	jitterSeconds := hasher.Sum32() % uint32(hostedProfileRefreshRetryJitter/time.Second+1)
	return hostedProfileRefreshRetryBase + time.Duration(jitterSeconds)*time.Second
}

func (state hostedProfileSnapshotState) refreshAheadAt(accountID string, now time.Time) bool {
	expiresAt := state.CheckedAt.Add(time.Duration(state.MaxAgeSeconds) * time.Second)
	return expiresAt.After(now) && !expiresAt.After(now.Add(hostedProfileRefreshAhead(accountID)))
}

func (s *Server) ensureHostedProfileDirectoryFreshness(ctx context.Context, accountID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state, err := s.hostedProfileStateContext(ctx, accountID)
	if err == nil && state.freshAt(now) {
		if state.refreshAheadAt(accountID, now) && !state.RefreshRetryAt.After(now) {
			s.startHostedProfileDirectoryRefresh(accountID, state, nil, now)
		}
		return nil
	}
	if err == nil && state.RefreshRetryAt.After(now) && state.staleErrorAllowedAt(now) {
		return nil
	}

	call := s.startHostedProfileDirectoryRefresh(accountID, state, err, now)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-call.done:
		return call.err
	}
}

// startHostedProfileDirectoryRefresh coalesces both proactive and request-bound
// refreshes. The Hosted request has its own short deadline so a cancelled or
// slow caller cannot strand every other request waiting for the same account.
func (s *Server) startHostedProfileDirectoryRefresh(accountID string, state hostedProfileSnapshotState, stateErr error, now time.Time) *hostedProfileRefreshCall {
	s.hostedProfileRefreshMu.Lock()
	if s.hostedProfileRefreshes == nil {
		s.hostedProfileRefreshes = map[string]*hostedProfileRefreshCall{}
	}
	if existing := s.hostedProfileRefreshes[accountID]; existing != nil {
		s.hostedProfileRefreshMu.Unlock()
		return existing
	}
	call := &hostedProfileRefreshCall{done: make(chan struct{})}
	s.hostedProfileRefreshes[accountID] = call
	if s.hostedProfileRefreshQueue == nil {
		s.hostedProfileRefreshQueue = make(chan string, hostedProfileRefreshBacklog)
	}
	queue := s.hostedProfileRefreshQueue
	s.hostedProfileRefreshMu.Unlock()

	// One scheduler owns a fixed worker set. Requests are represented by an
	// account ID, so repeated wakes coalesce and a full queue can discard its
	// oldest pending account instead of creating an unbounded goroutine wave.
	s.hostedProfileRefreshSchedulerOnce.Do(func() {
		started := s.startOwnedAsync("hosted-profile-directory-refresh-scheduler", func(background context.Context) {
			s.hostedProfileRefreshScheduler(background)
		})
		if !started {
			s.hostedProfileRefreshMu.Lock()
			if s.hostedProfileRefreshes[accountID] == call {
				delete(s.hostedProfileRefreshes, accountID)
				call.err = context.Canceled
				close(call.done)
			}
			s.hostedProfileRefreshMu.Unlock()
		}
	})

	s.hostedProfileRefreshMu.Lock()
	if s.hostedProfileRefreshes[accountID] != call {
		s.hostedProfileRefreshMu.Unlock()
		return call
	}
	select {
	case queue <- accountID:
	default:
		// The queue stores only pending IDs. Dropping one ID drops the matching
		// call as well; a later request will enqueue a fresh latest-wins call.
		var dropped string
		select {
		case dropped = <-queue:
		default:
		}
		if dropped != "" {
			if droppedCall := s.hostedProfileRefreshes[dropped]; droppedCall != nil {
				delete(s.hostedProfileRefreshes, dropped)
				droppedCall.err = errHostedProfileDirectoryUnavailable
				close(droppedCall.done)
			}
		}
		select {
		case queue <- accountID:
		default:
			delete(s.hostedProfileRefreshes, accountID)
			call.err = errHostedProfileDirectoryUnavailable
			close(call.done)
		}
	}
	s.hostedProfileRefreshMu.Unlock()
	return call
}

func (s *Server) hostedProfileRefreshScheduler(background context.Context) {
	workers := make(chan struct{}, hostedProfileRefreshConcurrency)
	var workerWG sync.WaitGroup
	for {
		select {
		case <-background.Done():
			workerWG.Wait()
			s.hostedProfileRefreshMu.Lock()
			for accountID, call := range s.hostedProfileRefreshes {
				delete(s.hostedProfileRefreshes, accountID)
				call.err = context.Canceled
				close(call.done)
			}
			s.hostedProfileRefreshMu.Unlock()
			return
		case accountID := <-s.hostedProfileRefreshQueue:
			s.hostedProfileRefreshMu.Lock()
			call := s.hostedProfileRefreshes[accountID]
			s.hostedProfileRefreshMu.Unlock()
			if call == nil {
				continue
			}
			select {
			case workers <- struct{}{}:
			case <-background.Done():
				continue
			}
			workerWG.Add(1)
			go func(accountID string, call *hostedProfileRefreshCall) {
				defer workerWG.Done()
				defer func() { <-workers }()
				// The state is read at dequeue time. This keeps retries and wake
				// hints latest-wins without retaining per-account goroutines.
				state, stateErr := s.hostedProfileStateContext(background, accountID)
				refreshCtx, cancel := context.WithTimeout(background, hostedProfileRequestTimeout)
				err := s.refreshHostedProfileDirectoryContext(refreshCtx, accountID, state, stateErr, time.Now().UTC())
				cancel()
				s.hostedProfileRefreshMu.Lock()
				if s.hostedProfileRefreshes[accountID] == call {
					delete(s.hostedProfileRefreshes, accountID)
					call.err = err
					close(call.done)
				}
				s.hostedProfileRefreshMu.Unlock()
			}(accountID, call)
		}
	}
}

func (s *Server) refreshHostedProfileDirectoryContext(ctx context.Context, accountID string, previous hostedProfileSnapshotState, previousErr error, now time.Time) error {
	var hostedAccountID string
	if err := s.queryUserRow(ctx, `SELECT COALESCE(portico_user_id, '') FROM users WHERE id = ? AND auth_origin = 'portico' AND COALESCE(disabled_at, '') = ''`, accountID).Scan(&hostedAccountID); err != nil {
		return errHostedProfileAccessRevoked
	}
	settings, err := s.remoteAccessSettings()
	if err != nil || settings.ClaimStatus != "claimed" || strings.TrimSpace(settings.ServerID) == "" {
		return errHostedProfileDirectoryUnavailable
	}
	credential := strings.TrimSpace(s.secretSetting(remoteAccessCredentialKey))
	if credential == "" {
		return errHostedProfileDirectoryUnavailable
	}
	knownRevision := int64(0)
	if previousErr == nil {
		knownRevision = previous.Revision
	}
	raw, exchangeErr := s.requestHostedProfileDirectorySnapshot(ctx, settings, credential, hostedAccountID, knownRevision)
	if hostedErr := new(hostedHTTPError); errors.As(exchangeErr, &hostedErr) && hostedErr.StatusCode == http.StatusConflict && knownRevision > 0 {
		raw, exchangeErr = s.requestHostedProfileDirectorySnapshot(ctx, settings, credential, hostedAccountID, 0)
	}
	if exchangeErr != nil {
		var hostedErr *hostedHTTPError
		if errors.As(exchangeErr, &hostedErr) && (hostedErr.StatusCode == http.StatusUnauthorized || hostedErr.StatusCode == http.StatusForbidden || hostedErr.StatusCode == http.StatusNotFound) {
			_ = s.revokeHostedProfileAccountContext(ctx, accountID, now)
			return errHostedProfileAccessRevoked
		}
		if previousErr == nil && previous.staleErrorAllowedAt(now) {
			_, _ = s.execUserWrite(ctx, `UPDATE hosted_profile_snapshot_state SET refresh_retry_at = ? WHERE account_id = ?`, now.Add(hostedProfileRefreshRetryDelay(accountID)).Format(time.RFC3339Nano), accountID)
			return nil
		}
		return fmt.Errorf("%w: %v", errHostedProfileDirectoryUnavailable, exchangeErr)
	}
	var signingKeyRef struct {
		SignatureKeyID string `json:"signatureKeyId"`
	}
	if err := json.Unmarshal(raw, &signingKeyRef); err != nil {
		return errHostedProfileDirectoryUnavailable
	}
	if err := s.ensureHostedDocumentKey(ctx, settings.HostedBaseURL, signingKeyRef.SignatureKeyID); err != nil {
		if previousErr == nil && previous.staleErrorAllowedAt(now) {
			_, _ = s.execUserWrite(ctx, `UPDATE hosted_profile_snapshot_state SET refresh_retry_at = ? WHERE account_id = ?`, now.Add(hostedProfileRefreshRetryDelay(accountID)).Format(time.RFC3339Nano), accountID)
			return nil
		}
		return errHostedProfileDirectoryUnavailable
	}
	snapshot, err := s.verifyHostedProfileDirectorySnapshot(raw, settings.ServerID, hostedAccountID, now)
	if err != nil {
		return err
	}
	checkedAt, _ := time.Parse(time.RFC3339Nano, snapshot.CheckedAt)
	if snapshot.Status == "unchanged" {
		if previousErr != nil || snapshot.Revision != previous.Revision {
			return errInvalidHostedProfileSnapshot
		}
		_, err = s.execUserWrite(ctx, `
			UPDATE hosted_profile_snapshot_state
			SET checked_at = ?, max_age_seconds = ?, stale_if_error_seconds = ?, refresh_retry_at = ''
			WHERE account_id = ? AND revision = ?`, snapshot.CheckedAt, snapshot.MaxAgeSeconds, snapshot.StaleIfErrorSeconds, accountID, snapshot.Revision)
		return err
	}
	synthetic := HostedProfileSelectionEnvelope{
		AssertionID: snapshot.SnapshotID, AccountID: snapshot.AccountID, AccountRevision: snapshot.Revision,
		Profiles: snapshot.Profiles, IssuedAt: snapshot.CheckedAt,
		ExpiresAt: checkedAt.Add(time.Duration(snapshot.MaxAgeSeconds) * time.Second).Format(time.RFC3339Nano),
	}
	if err := s.reconcileHostedProfileSelectionEnvelopeContext(ctx, accountID, synthetic, now); err != nil {
		return err
	}
	_, err = s.execUserWrite(ctx, `
		UPDATE hosted_profile_snapshot_state
		SET checked_at = ?, max_age_seconds = ?, stale_if_error_seconds = ?, refresh_retry_at = ''
		WHERE account_id = ? AND revision = ?`, snapshot.CheckedAt, snapshot.MaxAgeSeconds, snapshot.StaleIfErrorSeconds, accountID, snapshot.Revision)
	return err
}

func (s *Server) requestHostedProfileDirectorySnapshot(ctx context.Context, settings RemoteAccessSettings, credential, accountID string, knownRevision int64) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"accountId": accountID, "knownRevision": knownRevision})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/profile-directory-snapshots"
	var raw json.RawMessage
	if err := s.hostedJSONWithTimeout(ctx, http.MethodPost, endpoint, credential, body, &raw, hostedProfileRequestTimeout); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *Server) revokeHostedProfileAccountContext(ctx context.Context, accountID string, now time.Time) error {
	fences, err := s.beginAccountRuntimeErasureContext(ctx, accountID)
	if err != nil {
		return err
	}
	defer finishProfileErasureFences(fences)
	timestamp := now.UTC().Format(time.RFC3339Nano)
	return s.withUserTxTagged(ctx, []string{"users", "profiles", "sessions", "native_refresh_tokens", "profile_selection_grants", "profile_account_authentications"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET disabled_at = ?, updated_at = ? WHERE id = ? AND auth_origin = 'portico'`, timestamp, timestamp, accountID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE profiles SET disabled_at = ?, updated_at = ? WHERE account_id = ?`, timestamp, timestamp, accountID); err != nil {
			return err
		}
		return s.revokeAccountAuthorityTx(ctx, tx, accountID, timestamp)
	})
}
