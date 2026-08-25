package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	profileAdministrationProofTTL = 5 * time.Minute
	automaticProfileTrustTTL      = 90 * 24 * time.Hour
	profileAdministrationHeader   = "X-Portico-Profile-Admin-Proof"
)

var errInvalidProfileAdministrationProof = errors.New("profile administration proof is invalid or expired")

type managedProfile struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Avatar         *ProfileAvatar      `json:"avatar,omitempty"`
	IsPrimary      bool                `json:"isPrimary"`
	IsAccountAdmin bool                `json:"isAccountAdmin"`
	HasPIN         bool                `json:"hasPIN"`
	PINRevision    int64               `json:"pinRevision"`
	SortOrder      int                 `json:"sortOrder"`
	Policy         ProfileRestrictions `json:"policy"`
}

type managedProfileDirectory struct {
	Authority       string           `json:"authority"`
	AccountID       string           `json:"accountId"`
	ServerID        string           `json:"serverId"`
	ProfilesAllowed bool             `json:"profilesAllowed"`
	CanManage       bool             `json:"canManage"`
	Profiles        []managedProfile `json:"profiles"`
}

type createManagedProfileRequest struct {
	Name   string              `json:"name"`
	Avatar *ProfileAvatar      `json:"avatar,omitempty"`
	PIN    string              `json:"pin,omitempty"`
	Policy ProfileRestrictions `json:"policy"`
}

type updateManagedProfileRequest struct {
	Name   *string              `json:"name,omitempty"`
	Avatar *ProfileAvatar       `json:"avatar,omitempty"`
	Policy *ProfileRestrictions `json:"policy,omitempty"`
}

type localProfilePINMutationRequest struct {
	PIN      string `json:"pin,omitempty"`
	Password string `json:"password"`
}

type profileAdministrationProofRequest struct {
	PIN      string `json:"pin,omitempty"`
	Password string `json:"password,omitempty"`
}

type profileAdministrationProofResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

type automaticProfileTrustRequest struct {
	InstallationID string `json:"installationId,omitempty"`
}

type automaticProfileTrustRedemptionRequest struct {
	Token string `json:"token"`
}

type interactiveSessionBinding struct {
	SessionID      string
	DeviceID       string
	InstallationID string
}

type automaticProfileTrustResponse struct {
	Version        string `json:"version"`
	Purpose        string `json:"purpose"`
	Token          string `json:"token"`
	Authority      string `json:"authority"`
	AccountID      string `json:"accountId"`
	ServerID       string `json:"serverId"`
	InstallationID string `json:"installationId,omitempty"`
	ProfileID      string `json:"profileId"`
	PINRevision    int64  `json:"pinRevision"`
	ExpiresAt      string `json:"expiresAt"`
}

func (s *Server) handleAccountProfiles(w http.ResponseWriter, r *http.Request, user User) {
	if r.URL.Path != "/api/account/profiles" {
		s.handleAccountProfileRoute(w, r, user)
		return
	}
	switch r.Method {
	case http.MethodGet:
		directory, err := s.managedProfileDirectoryContext(r.Context(), user)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "profile_directory_failed", "Unable to load profiles.")
			return
		}
		writeJSON(w, http.StatusOK, directory)
	case http.MethodPost:
		if !s.requireLocalProfileManagement(w, r, user) {
			return
		}
		var request createManagedProfileRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		avatar, err := validateProfileAvatar(request.Avatar)
		if err != nil {
			writeProductError(w, http.StatusBadRequest, "invalid_profile", "Choose a valid profile image.")
			return
		}
		profile, err := s.createLocalProfileContext(r.Context(), accountIDForUser(user), CreateLocalProfileInput{
			DisplayName: request.Name, AvatarURL: profileAvatarReference(avatar), PIN: request.PIN, Restrictions: request.Policy,
		})
		if err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, managedProfileFromAccount(profile))
	default:
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
	}
}

func (s *Server) handleAccountProfileRoute(w http.ResponseWriter, r *http.Request, user User) {
	path := strings.TrimPrefix(r.URL.Path, "/api/account/profiles/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
		writeProductError(w, http.StatusNotFound, "profile_not_found", "Profile not found.")
		return
	}
	profileID := parts[0]
	if len(parts) == 2 && parts[1] == "pin" {
		s.handleAccountProfilePIN(w, r, user, profileID)
		return
	}
	if len(parts) == 2 {
		writeProductError(w, http.StatusNotFound, "profile_not_found", "Profile not found.")
		return
	}
	if !s.requireLocalProfileManagement(w, r, user) {
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var request updateManagedProfileRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		profile, err := s.updateManagedProfileContext(r.Context(), accountIDForUser(user), profileID, request)
		if err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, managedProfileFromAccount(profile))
	case http.MethodDelete:
		operationID, err := s.deleteManagedProfileContext(r.Context(), accountIDForUser(user), profileID)
		if err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		s.recordAudit(r, user, "profile.erased", "profile", "", "warn", nil)
		writeJSON(w, http.StatusOK, map[string]any{"operationId": operationID, "erased": true})
	default:
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH or DELETE for this endpoint.")
	}
}

func (s *Server) handleAccountProfilePIN(w http.ResponseWriter, r *http.Request, user User, profileID string) {
	if !s.requireLocalProfileManagement(w, r, user) {
		return
	}
	switch r.Method {
	case http.MethodPut:
		var request localProfilePINMutationRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		expectedHash, sessionID, ok := s.requireLocalAccountPasswordAuthorization(w, r, user, request.Password)
		if !ok {
			return
		}
		if err := s.setLocalProfilePINAuthorizedContext(r.Context(), accountIDForUser(user), profileID, request.PIN, expectedHash, sessionID); err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		var request localProfilePINMutationRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		expectedHash, sessionID, ok := s.requireLocalAccountPasswordAuthorization(w, r, user, request.Password)
		if !ok {
			return
		}
		if err := s.clearLocalProfilePINAuthorizedContext(r.Context(), accountIDForUser(user), profileID, expectedHash, sessionID); err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PUT or DELETE for this endpoint.")
	}
}

func (s *Server) requireLocalAccountPasswordReauthentication(w http.ResponseWriter, r *http.Request, user User, password string) bool {
	_, _, ok := s.requireLocalAccountPasswordAuthorization(w, r, user, password)
	return ok
}

func (s *Server) requireLocalAccountPasswordAuthorization(w http.ResponseWriter, r *http.Request, user User, password string) (string, string, bool) {
	accountID := accountIDForUser(user)
	expectedHash, err := s.verifyLocalPasswordSnapshot(r.Context(), kdfProfileReauthCompare, accountID, password)
	if err != nil {
		if errors.Is(err, errInvalidCredentials) {
			writeProductError(w, http.StatusUnauthorized, "invalid_credentials", "Your current account password is incorrect.")
			return "", "", false
		}
		if writeKDFUnavailable(w, err) {
			return "", "", false
		}
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "profile_request_failed", "Unable to confirm the account password.")
		return "", "", false
	}
	sessionID, err := s.currentSessionIDContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusUnauthorized, "interactive_session_required", "Profile management requires a current interactive session.")
		return "", "", false
	}
	return expectedHash, sessionID, true
}

func (s *Server) handleAccountProfileOrder(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPut {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PUT for this endpoint.")
		return
	}
	if !s.requireLocalProfileManagement(w, r, user) {
		return
	}
	var request struct {
		ProfileIDs []string `json:"profileIds"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := s.reorderManagedProfilesContext(r.Context(), accountIDForUser(user), request.ProfileIDs); err != nil {
		s.writeProfileManagementError(w, err)
		return
	}
	directory, err := s.managedProfileDirectoryContext(r.Context(), user)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "profile_directory_failed", "Unable to load profiles.")
		return
	}
	writeJSON(w, http.StatusOK, directory)
}

func (s *Server) handleProfileAdministrationProof(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !selectedProfileMayManageAccount(user) || user.AuthProvider == "api_key" {
		writeProductError(w, http.StatusForbidden, "primary_profile_required", "Switch to the primary profile to manage profiles.")
		return
	}
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		writeProductError(w, http.StatusConflict, "profiles_managed_by_portico_account", "Manage these profiles in Portico Account.")
		return
	}
	if allowed, err := s.accountProfilesAllowedContext(r.Context(), accountIDForUser(user)); err != nil || !allowed {
		writeProductError(w, http.StatusForbidden, "profile_not_available_on_server", "This server is currently limited to the primary profile.")
		return
	}
	var request profileAdministrationProofRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	accountID := accountIDForUser(user)
	profileID := viewerProfileID(user)
	var pinRequired int
	var pinRevision int64
	var passwordHash string
	if err := s.queryUserRow(r.Context(), `SELECT pin_required, pin_revision FROM profiles WHERE id = ? AND account_id = ? AND is_primary = 1 AND origin = 'local' AND disabled_at = ''`, profileID, accountID).Scan(&pinRequired, &pinRevision); err != nil {
		s.writeProfileManagementError(w, err)
		return
	}
	if pinRequired == 1 {
		valid, err := s.verifyLocalProfilePINContext(r.Context(), accountID, profileID, request.PIN, time.Now().UTC())
		if err != nil || !valid {
			s.writeProfileManagementError(w, errInvalidProfilePIN)
			return
		}
	} else {
		if err := s.queryUserRow(r.Context(), `SELECT COALESCE(password_hash, '') FROM users WHERE id = ?`, accountID).Scan(&passwordHash); err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		valid, _, verifiedHash, verifyErr := verifyAccountPasswordSnapshot(r.Context(), kdfProfileReauthCompare, passwordHash, request.Password)
		if verifyErr != nil {
			s.writeProfileManagementError(w, verifyErr)
			return
		}
		if !valid {
			writeProductError(w, http.StatusUnauthorized, "invalid_credentials", "Your current account password is incorrect.")
			return
		}
		passwordHash = verifiedHash
	}
	sessionID, err := s.currentSessionIDContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Profile management requires a signed-in app session.")
		return
	}
	rawToken, err := randomNativeCredentialToken(rand.Reader)
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "profile_proof_failed", "Unable to confirm profile management.")
		return
	}
	now := time.Now().UTC()
	err = s.withUserTxTagged(r.Context(), []string{"local_profile_admin_proofs", "users", "sessions", "devices", "profiles"}, func(tx *sql.Tx) error {
		if err := validatePasswordSessionTx(tx, accountID, profileID, sessionID, passwordHash, now); err != nil {
			return err
		}
		_, err := tx.Exec(`
			INSERT INTO local_profile_admin_proofs (id, token_hash, account_id, primary_profile_id, session_id, pin_revision, expires_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, randomID("pap"), hashToken(rawToken), accountID, profileID, sessionID, pinRevision,
			now.Add(profileAdministrationProofTTL).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		if errors.Is(err, errPasswordCredentialChanged) || errors.Is(err, errPrivilegedSessionChanged) {
			s.writeProfileManagementError(w, err)
			return
		}
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "profile_proof_failed", "Unable to confirm profile management.")
		return
	}
	writeJSON(w, http.StatusCreated, profileAdministrationProofResponse{Token: rawToken, ExpiresAt: now.Add(profileAdministrationProofTTL).Format(time.RFC3339Nano)})
}

func (s *Server) requireLocalProfileManagement(w http.ResponseWriter, r *http.Request, user User) bool {
	if !selectedProfileMayManageAccount(user) || user.AuthProvider == "api_key" {
		writeProductError(w, http.StatusForbidden, "primary_profile_required", "Switch to the primary profile to manage profiles.")
		return false
	}
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		writeProductError(w, http.StatusConflict, "profiles_managed_by_portico_account", "Manage these profiles in Portico Account.")
		return false
	}
	if allowed, err := s.accountProfilesAllowedContext(r.Context(), accountIDForUser(user)); err != nil || !allowed {
		writeProductError(w, http.StatusForbidden, "profile_not_available_on_server", "This server is currently limited to the primary profile.")
		return false
	}
	token := strings.TrimSpace(r.Header.Get(profileAdministrationHeader))
	if token == "" {
		writeProductError(w, http.StatusUnauthorized, "profile_admin_proof_required", "Confirm the primary profile PIN to continue.")
		return false
	}
	sessionID, err := s.currentSessionIDContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Profile management requires a signed-in app session.")
		return false
	}
	var count int
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.queryUserRow(r.Context(), `
		SELECT COUNT(*) FROM local_profile_admin_proofs proof
		JOIN profiles primary_profile ON primary_profile.id = proof.primary_profile_id
		WHERE proof.token_hash = ? AND proof.account_id = ? AND proof.session_id = ? AND proof.expires_at > ?
		  AND primary_profile.is_primary = 1 AND primary_profile.origin = 'local' AND primary_profile.disabled_at = ''
		  AND primary_profile.pin_revision = proof.pin_revision`, hashToken(token), accountIDForUser(user), sessionID, now).Scan(&count)
	if err != nil || count != 1 {
		writeProductError(w, http.StatusUnauthorized, "profile_admin_proof_expired", "Confirm the primary profile PIN again to continue.")
		return false
	}
	return true
}

func (s *Server) currentSessionIDContext(ctx context.Context, r *http.Request, user User) (string, error) {
	for _, cookie := range s.requestSessionCookies(r) {
		var sessionID string
		if err := s.queryUserRow(ctx, `SELECT id FROM sessions WHERE token_hash = ? AND user_id = ? AND COALESCE(NULLIF(profile_id, ''), user_id) = ? AND expires_at > ?`,
			hashToken(cookie.Value), accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339Nano)).Scan(&sessionID); err == nil {
			return sessionID, nil
		}
	}
	if token, ok := bearerTokenFromRequest(r); ok && token != "" && !strings.HasPrefix(token, "ptc_api_") && !strings.HasPrefix(token, "ptc_clt_") {
		var sessionID string
		if err := s.queryUserRow(ctx, `SELECT id FROM sessions WHERE token_hash = ? AND user_id = ? AND COALESCE(NULLIF(profile_id, ''), user_id) = ? AND expires_at > ?`,
			hashToken(token), accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339Nano)).Scan(&sessionID); err == nil {
			return sessionID, nil
		}
	}
	return "", errInvalidProfileAdministrationProof
}

func (s *Server) managedProfileDirectoryContext(ctx context.Context, user User) (managedProfileDirectory, error) {
	directory, err := s.profileDirectoryContext(ctx, accountIDForUser(user))
	if err != nil {
		return managedProfileDirectory{}, err
	}
	profiles := make([]managedProfile, 0, len(directory.Profiles))
	for _, profile := range directory.Profiles {
		profiles = append(profiles, managedProfile{
			ID: profile.ID, Name: profile.Name, Avatar: profile.Avatar, IsPrimary: profile.IsPrimary,
			IsAccountAdmin: profile.IsAccountAdmin, HasPIN: profile.HasPIN, PINRevision: profile.PINRevision,
			SortOrder: profile.SortOrder, Policy: profile.Policy,
		})
	}
	return managedProfileDirectory{
		Authority: directory.Authority, AccountID: directory.AccountID, ServerID: directory.ServerID,
		ProfilesAllowed: directory.ProfilesAllowed,
		CanManage:       directory.Authority == "local" && user.AuthProvider != "api_key" && directory.ProfilesAllowed && selectedProfileMayManageAccount(user),
		Profiles:        profiles,
	}, nil
}

func (s *Server) accountProfilesAllowedContext(ctx context.Context, accountID string) (bool, error) {
	var allowed int
	if err := s.queryUserRow(ctx, `SELECT COALESCE(allow_account_profiles, 1) FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, strings.TrimSpace(accountID)).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func managedProfileFromAccount(profile AccountProfile) managedProfile {
	var avatar *ProfileAvatar
	if profile.AvatarURL != "" {
		avatar = &ProfileAvatar{Kind: "custom", Reference: profile.AvatarURL}
	}
	return managedProfile{
		ID: profile.ID, Name: profile.DisplayName, Avatar: avatar, IsPrimary: profile.IsPrimary,
		IsAccountAdmin: profile.IsPrimary, HasPIN: profile.HasPIN || profile.PINRequired, PINRevision: profile.PINRevision,
		SortOrder: profile.SortOrder, Policy: profile.Restrictions,
	}
}

func (s *Server) updateManagedProfileContext(ctx context.Context, accountID, profileID string, request updateManagedProfileRequest) (AccountProfile, error) {
	profiles, err := s.listAccountProfilesContext(ctx, accountID)
	if err != nil {
		return AccountProfile{}, err
	}
	var current *AccountProfile
	for index := range profiles {
		if profiles[index].ID == profileID {
			current = &profiles[index]
			break
		}
	}
	if current == nil {
		return AccountProfile{}, errProfileNotFound
	}
	if current.Origin != "local" {
		return AccountProfile{}, errHostedProfileLocalPIN
	}
	name := current.DisplayName
	if request.Name != nil {
		var ok bool
		name, ok = normalizeProfileDisplayName(*request.Name)
		if !ok {
			return AccountProfile{}, errors.New("profile display name is required and must be at most 80 characters")
		}
	}
	avatarURL := current.AvatarURL
	if request.Avatar != nil {
		avatar, err := validateProfileAvatar(request.Avatar)
		if err != nil {
			return AccountProfile{}, err
		}
		avatarURL = profileAvatarReference(avatar)
	}
	policy := current.Restrictions
	if request.Policy != nil {
		policy = *request.Policy
	}
	policyJSON, err := encodeProfileRestrictions(policy)
	if err != nil {
		return AccountProfile{}, err
	}
	policy, err = decodeProfileRestrictions(policyJSON)
	if err != nil {
		return AccountProfile{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{"profiles"}, func(tx *sql.Tx) error {
		result, err := tx.Exec(`UPDATE profiles SET display_name = ?, avatar_url = ?, restrictions_json = ?, policy_updated_at = ?, updated_at = ? WHERE id = ? AND account_id = ? AND origin = 'local' AND disabled_at = ''`,
			name, avatarURL, policyJSON, now, now, profileID, accountID)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return errProfileNotFound
		}
		return nil
	})
	if err != nil {
		return AccountProfile{}, err
	}
	current.DisplayName, current.AvatarURL, current.Restrictions, current.PolicyUpdatedAt = name, avatarURL, policy, now
	return *current, nil
}

func (s *Server) deleteManagedProfileContext(ctx context.Context, accountID, profileID string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	fence := s.profileRuntime.beginErasure(accountID, profileID)
	if !fence.owner {
		_ = fence.wait(ctx, profileErasureDrainTimeout)
		return "", errProfileErasureInProgress
	}
	if !fence.wait(ctx, profileErasureDrainTimeout) {
		fence.finish()
		return "", errProfileErasureDrainTimeout
	}
	defer fence.finish()
	playbackSessionIDs := s.profilePlaybackSessionIDsContext(ctx, accountID, profileID)
	watchGroupIDs := s.profileWatchGroupIDsContext(ctx, accountID, profileID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var operationID string
	err := s.withUserTxTagged(ctx, []string{"profiles", "sessions", "native_refresh_tokens", "playback", "saved", "dvr", "notifications", "preferences"}, func(tx *sql.Tx) error {
		var err error
		operationID, err = s.eraseSecondaryProfileTx(ctx, tx, accountID, profileID, "local", now)
		return err
	})
	if err == nil {
		for _, sessionID := range playbackSessionIDs {
			s.notifyPlaybackCommand(sessionID)
		}
		for _, groupID := range watchGroupIDs {
			s.notifyWatchWithFriendsGroup(groupID)
		}
	}
	return operationID, err
}

func (s *Server) profilePlaybackSessionIDsContext(ctx context.Context, accountID, profileID string) []string {
	rows, err := s.queryUserRead(ctx, `SELECT id FROM playback_sessions WHERE user_id = ? AND profile_id = ?`, strings.TrimSpace(accountID), strings.TrimSpace(profileID))
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Server) profileWatchGroupIDsContext(ctx context.Context, accountID, profileID string) []string {
	rows, err := s.queryUserRead(ctx, `
		SELECT id FROM watch_with_friends_groups WHERE owner_user_id = ? AND owner_profile_id = ?
		UNION
		SELECT group_id FROM watch_with_friends_members WHERE user_id = ? AND profile_id = ?
		UNION
		SELECT group_id FROM watch_with_friends_queue WHERE added_by_user_id = ? AND added_by_profile_id = ?`,
		strings.TrimSpace(accountID), strings.TrimSpace(profileID), strings.TrimSpace(accountID), strings.TrimSpace(profileID), strings.TrimSpace(accountID), strings.TrimSpace(profileID))
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Server) reorderManagedProfilesContext(ctx context.Context, accountID string, rawIDs []string) error {
	profiles, err := s.listAccountProfilesContext(ctx, accountID)
	if err != nil {
		return err
	}
	if len(rawIDs) != len(profiles) {
		return errors.New("profile order must include every active profile exactly once")
	}
	expected := map[string]bool{}
	for _, profile := range profiles {
		expected[profile.ID] = true
	}
	ids := make([]string, 0, len(rawIDs))
	seen := map[string]bool{}
	for _, raw := range rawIDs {
		id := strings.TrimSpace(raw)
		if !expected[id] || seen[id] {
			return errors.New("profile order must include every active profile exactly once")
		}
		seen[id], ids = true, append(ids, id)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withUserTxTagged(ctx, []string{"profiles"}, func(tx *sql.Tx) error {
		for index, id := range ids {
			if _, err := tx.Exec(`UPDATE profiles SET sort_order = ?, updated_at = ? WHERE id = ? AND account_id = ? AND origin = 'local' AND disabled_at = ''`, index, now, id, accountID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Server) handleAutomaticProfileTrusts(w http.ResponseWriter, r *http.Request, user User) {
	if r.URL.Path == "/api/account/profile-trusts/redeem" {
		s.handleAutomaticProfileTrustRedemption(w, r, user)
		return
	}
	var request automaticProfileTrustRequest
	if (r.Method == http.MethodPost || r.Method == http.MethodDelete) && !decodeJSON(w, r, &request) {
		return
	}
	binding, err := s.currentInteractiveSessionBindingContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Remembering a profile requires a signed-in app session.")
		return
	}
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "profile_trust_failed", "Unable to remember this profile.")
		return
	}
	authority := "local"
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		authority = "hosted"
	}
	switch r.Method {
	case http.MethodPost:
		// A Hosted trust is issued only after a Cloud assertion has already
		// established this exact server-local profile session. Hosted Web may use
		// it to reopen the matching remembered server session, but the Local Auth
		// selection endpoint below rejects it by authority.
		var pinRevision int64
		if err := s.queryUserRow(r.Context(), `SELECT pin_revision FROM profiles WHERE id = ? AND account_id = ? AND disabled_at = ''`, viewerProfileID(user), accountIDForUser(user)).Scan(&pinRevision); err != nil {
			s.writeProfileManagementError(w, err)
			return
		}
		rawToken, err := randomNativeCredentialToken(rand.Reader)
		if err != nil {
			writeProductError(w, http.StatusInternalServerError, "profile_trust_failed", "Unable to remember this profile.")
			return
		}
		now, expires := time.Now().UTC(), time.Now().UTC().Add(automaticProfileTrustTTL)
		_, err = s.execUserWrite(r.Context(), `
			INSERT INTO automatic_profile_selection_trusts (
				id, token_hash, authority, account_id, server_id, device_id, installation_id, profile_id, pin_revision,
				expires_at, revoked_at, last_used_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?)
			ON CONFLICT(authority, account_id, server_id, device_id, installation_id, profile_id) DO UPDATE SET
				token_hash = excluded.token_hash, pin_revision = excluded.pin_revision, expires_at = excluded.expires_at,
				revoked_at = '', last_used_at = '', updated_at = excluded.updated_at`,
			randomID("ptrust"), hashToken(rawToken), authority, accountIDForUser(user), serverID, binding.DeviceID, binding.InstallationID,
			viewerProfileID(user), pinRevision, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "profile_trust_failed", "Unable to remember this profile.")
			return
		}
		writeJSON(w, http.StatusCreated, automaticProfileTrustResponse{
			Version: "v1", Purpose: "automatic-profile-selection", Token: rawToken, Authority: authority, AccountID: accountIDForUser(user), ServerID: serverID,
			InstallationID: binding.InstallationID, ProfileID: viewerProfileID(user), PINRevision: pinRevision, ExpiresAt: expires.Format(time.RFC3339Nano),
		})
	case http.MethodDelete:
		_, err := s.execUserWrite(r.Context(), `UPDATE automatic_profile_selection_trusts SET revoked_at = ?, updated_at = ? WHERE authority = ? AND account_id = ? AND server_id = ? AND device_id = ? AND revoked_at = ''`,
			time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), authority, accountIDForUser(user), serverID, binding.DeviceID)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "profile_trust_failed", "Unable to forget this profile.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST or DELETE for this endpoint.")
	}
}

func (s *Server) handleAutomaticProfileTrustRedemption(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request automaticProfileTrustRedemptionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	binding, err := s.currentInteractiveSessionBindingContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Opening a remembered profile requires a signed-in app session.")
		return
	}
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "profile_trust_failed", "Unable to confirm this remembered profile.")
		return
	}
	authority := "local"
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		authority = "hosted"
	}
	err = s.withUserTxTagged(r.Context(), []string{"automatic_profile_selection_trusts"}, func(tx *sql.Tx) error {
		return automaticProfileTrustBindingTx(tx, request.Token, authority, accountIDForUser(user), viewerProfileID(user), serverID, binding.DeviceID, time.Now().UTC())
	})
	if err != nil {
		writeProductError(w, http.StatusUnauthorized, "automatic_profile_trust_required", "Choose this profile and enter its PIN again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) currentInteractiveSessionBindingContext(ctx context.Context, r *http.Request, user User) (interactiveSessionBinding, error) {
	if user.APIKeyID != "" || user.AuthProvider == "api_key" {
		return interactiveSessionBinding{}, errInvalidProfileAdministrationProof
	}
	tokens := make([]string, 0, len(s.requestSessionCookies(r))+1)
	for _, cookie := range s.requestSessionCookies(r) {
		if strings.TrimSpace(cookie.Value) != "" {
			tokens = append(tokens, cookie.Value)
		}
	}
	if token, ok := bearerTokenFromRequest(r); ok && token != "" && !strings.HasPrefix(token, "ptc_api_") && !strings.HasPrefix(token, "ptc_clt_") {
		tokens = append(tokens, token)
	}
	for _, token := range tokens {
		var binding interactiveSessionBinding
		var revokedAt string
		err := s.queryUserRow(ctx, `
			SELECT session.id, session.device_id, device.installation_id, COALESCE(device.revoked_at, '')
			FROM sessions session
			JOIN devices device ON device.id = session.device_id AND device.user_id = session.user_id
			WHERE session.token_hash = ? AND session.user_id = ?
			  AND COALESCE(NULLIF(session.profile_id, ''), session.user_id) = ? AND session.expires_at > ?`,
			hashToken(token), accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339Nano)).Scan(
			&binding.SessionID, &binding.DeviceID, &binding.InstallationID, &revokedAt)
		if err == nil && revokedAt == "" && binding.DeviceID != "" && (user.DeviceID == "" || user.DeviceID == binding.DeviceID) {
			return binding, nil
		}
	}
	return interactiveSessionBinding{}, errInvalidProfileAdministrationProof
}

func automaticProfileTrustTx(tx *sql.Tx, rawToken string, account profileAccountAuthenticationRecord, profileID, serverID string, now time.Time) error {
	return automaticProfileTrustBindingTx(tx, rawToken, account.AuthProvider, account.AccountID, profileID, serverID, account.DeviceID, now)
}

func automaticProfileTrustAuthority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hosted", "portico":
		return "hosted"
	default:
		return "local"
	}
}

func automaticProfileTrustBindingTx(tx *sql.Tx, rawToken, authority, accountID, profileID, serverID, deviceID string, now time.Time) error {
	var storedAuthority, storedAccount, storedProfile, storedServer, storedDevice, storedInstallation, revokedAt, expiresAt string
	var pinRevision, currentPINRevision int64
	err := tx.QueryRow(`
		SELECT trust.authority, trust.account_id, trust.profile_id, trust.server_id, trust.device_id, trust.installation_id, trust.pin_revision,
		       trust.expires_at, trust.revoked_at, profile.pin_revision
		FROM automatic_profile_selection_trusts trust
		JOIN profiles profile ON profile.id = trust.profile_id AND profile.account_id = trust.account_id AND profile.disabled_at = ''
		WHERE trust.token_hash = ?`, hashToken(strings.TrimSpace(rawToken))).Scan(
		&storedAuthority, &storedAccount, &storedProfile, &storedServer, &storedDevice, &storedInstallation, &pinRevision, &expiresAt, &revokedAt, &currentPINRevision)
	if err != nil || storedAuthority != automaticProfileTrustAuthority(authority) || storedAccount != strings.TrimSpace(accountID) || storedProfile != strings.TrimSpace(profileID) || storedServer != strings.TrimSpace(serverID) ||
		storedDevice != strings.TrimSpace(deviceID) || pinRevision != currentPINRevision || revokedAt != "" {
		return errInvalidProfileSelectionGrant
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !expires.After(now) {
		return errInvalidProfileSelectionGrant
	}
	_, err = tx.Exec(`UPDATE automatic_profile_selection_trusts SET last_used_at = ?, updated_at = ? WHERE token_hash = ?`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), hashToken(strings.TrimSpace(rawToken)))
	return err
}

func (s *Server) writeProfileManagementError(w http.ResponseWriter, err error) {
	if writeKDFUnavailable(w, err) {
		return
	}
	switch {
	case errors.Is(err, errPasswordCredentialChanged), errors.Is(err, errPrivilegedSessionChanged):
		writeProductError(w, http.StatusUnauthorized, "credentials_changed", "The account authorization changed while Portico was saving this request. Sign in again and retry.")
	case errors.Is(err, errProfilePINConcurrentChange):
		w.Header().Set("Retry-After", "1")
		writeProductError(w, http.StatusServiceUnavailable, "profile_request_busy", "This profile changed while Portico was verifying it. Try again shortly.")
	case errors.Is(err, errProfileNotFound), errors.Is(err, sql.ErrNoRows):
		writeProductError(w, http.StatusNotFound, "profile_not_found", "Profile not found.")
	case errors.Is(err, errProfileLimit):
		writeProductError(w, http.StatusConflict, "profile_limit_reached", "This account already has the maximum number of profiles.")
	case errors.Is(err, errProfileNotAllowed):
		writeProductError(w, http.StatusForbidden, "profile_not_available_on_server", "This server is currently limited to the primary profile.")
	case errors.Is(err, errInvalidProfilePIN), errors.Is(err, errProfilePINNotSet):
		writeProductError(w, http.StatusUnauthorized, "profile_pin_invalid", "Enter the four-digit primary profile PIN.")
	case errors.Is(err, errProfilePINLocked), errors.Is(err, errProfilePINBackoff):
		writeProductError(w, http.StatusTooManyRequests, "profile_pin_locked", "Too many PIN attempts. Wait a moment and try again.")
	case errors.Is(err, errPrimaryProfilePINRequired):
		writeProductError(w, http.StatusConflict, "primary_profile_pin_required", "Set a PIN on the primary profile before adding another profile.")
	case errors.Is(err, errPrimaryProfilePINInUse):
		writeProductError(w, http.StatusConflict, "primary_profile_pin_in_use", "Keep the primary profile PIN while other profiles exist.")
	case errors.Is(err, errHostedProfileLocalPIN):
		writeProductError(w, http.StatusConflict, "profiles_managed_by_portico_account", "Manage these profiles in Portico Account.")
	case errors.Is(err, errProfileErasureInProgress):
		writeProductError(w, http.StatusConflict, "profile_erasure_in_progress", "This profile is already being permanently erased.")
	case errors.Is(err, errProfileErasureDrainTimeout):
		w.Header().Set("Retry-After", "2")
		writeProductError(w, http.StatusServiceUnavailable, "profile_erasure_drain_timeout", "Active profile work did not stop safely. Try the erasure again shortly.")
	case errors.Is(err, errInvalidProfileRestriction):
		writeProductError(w, http.StatusBadRequest, "invalid_profile_policy", "Review the profile restrictions and try again.")
	default:
		writeProductError(w, http.StatusInternalServerError, "profile_request_failed", "Unable to complete the profile request.")
	}
}

func sortedProfileIDs(profiles []AccountProfile) []string {
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].SortOrder < profiles[j].SortOrder })
	result := make([]string, len(profiles))
	for index := range profiles {
		result[index] = profiles[index].ID
	}
	return result
}
