package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"
)

const (
	maxProfilesPerAccount                          = 8
	localProfilePINLength                          = 4
	localProfilePINFailureLimit                    = 5
	localProfilePINLockDuration                    = 15 * time.Minute
	localProfilePINBackoffBase                     = time.Second
	localProfilePINBackoffMaximum                  = 8 * time.Second
	localProfilePINBcryptCost                      = 8
	profileRestrictionsVersion                     = "v1"
	hostedProfileSelectionAssertionVersion         = "v1"
	profileSelectionGrantTTL                       = 2 * time.Minute
	maximumHostedProfileSelectionAssertionLifetime = 5 * time.Minute
	maximumBlockedProfileLabels                    = 64
	maximumBlockedProfileLabelRunes                = 128
)

var (
	errProfileNotFound                         = errors.New("profile not found")
	errProfileNotAllowed                       = errors.New("account profiles are not allowed for this membership")
	errProfileAccountMismatch                  = errors.New("profile does not belong to account")
	errProfileLimit                            = errors.New("account profile limit reached")
	errInvalidProfilePIN                       = errors.New("profile pin must contain exactly four digits")
	errProfilePINNotSet                        = errors.New("profile pin is not set")
	errProfilePINLocked                        = errors.New("profile pin is temporarily locked")
	errHostedProfileLocalPIN                   = errors.New("hosted profile pins are verified by hosted services")
	errInvalidProfileRestriction               = errors.New("invalid profile restrictions")
	errPrimaryProfilePINRequired               = errors.New("primary profile pin is required before adding profiles")
	errPrimaryProfilePINInUse                  = errors.New("primary profile pin cannot be cleared while child profiles exist")
	errInvalidProfileSelectionGrant            = errors.New("profile selection grant is invalid or expired")
	errProfileSelectionGrantConsumed           = errors.New("profile selection grant has already been consumed")
	errInvalidHostedProfileSnapshot            = errors.New("invalid hosted profile snapshot")
	errStaleHostedProfileSnapshot              = errors.New("hosted profile snapshot is stale")
	errInvalidHostedProfileSelectionAssertion  = errors.New("invalid hosted profile selection assertion")
	errHostedProfileSelectionAssertionReplayed = errors.New("hosted profile selection assertion has already been used")
	errProfilePINBackoff                       = errors.New("profile pin verification must wait before retrying")
)

type profilePINRetryAfterError struct {
	kind       error
	retryAfter time.Duration
}

func (e *profilePINRetryAfterError) Error() string { return e.kind.Error() }
func (e *profilePINRetryAfterError) Unwrap() error { return e.kind }

func profilePINRetryError(kind error, until time.Time, now time.Time) error {
	retryAfter := until.Sub(now)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return &profilePINRetryAfterError{kind: kind, retryAfter: retryAfter}
}

type AuthenticationAuthority string

const (
	AuthenticationAuthorityLocal  AuthenticationAuthority = "local"
	AuthenticationAuthorityHosted AuthenticationAuthority = "hosted"
)

func normalizeAuthenticationAuthority(authOrigin string) AuthenticationAuthority {
	if strings.EqualFold(strings.TrimSpace(authOrigin), "portico") {
		return AuthenticationAuthorityHosted
	}
	return AuthenticationAuthorityLocal
}

type RequestMembershipIdentity struct {
	ServerAccountID    string
	HostedAccountID    string
	HostedMembershipID string
}

// RequestMembershipEnvelope is the server-controlled authorization ceiling
// for an account membership. A selected profile can only reduce this envelope.
type RequestMembershipEnvelope struct {
	Role                   string
	Permissions            map[string]bool
	LibraryIDs             []string
	AccessSchedule         UserAccessSchedule
	TagPolicy              UserTagPolicy
	DevicePolicy           UserDevicePolicy
	ChannelPolicy          UserChannelPolicy
	MaxContentRating       string
	MaxActiveSessions      int
	MaxActiveStreams       int
	RemoteBitrateLimitMbps int
	AccountProfilesAllowed bool
}

// RequestPrincipal is the complete authorization identity for a request.
// MembershipIdentity and MembershipEnvelope are authoritative account facts;
// ProfileID and the effective fields below are the selected viewing identity
// after profile policy has reduced that envelope.
type RequestPrincipal struct {
	AuthenticationAuthority AuthenticationAuthority
	MembershipIdentity      RequestMembershipIdentity
	MembershipEnvelope      RequestMembershipEnvelope
	AccountID               string
	ProfileID               string
	ProfileOrigin           string
	ProfileIsPrimary        bool
	DisplayName             string
	AvatarURL               string
	Preferences             UserPreferences
	Permissions             map[string]bool
	MaxContentRating        string
	MaximumAgeRating        *int
	AllowUnrated            bool
	BlockedLabels           []string
	AllowFeedback           bool
	MaxActiveSessions       int
	MaxActiveStreams        int
	RemoteBitrateLimitMbps  int
	AccountProfilesAllowed  bool
}

// ProfileRestrictions is the single portable V1 account-profile policy
// contract shared with Portico Cloud. It is intentionally narrow: values can
// only reduce the server membership's authorization envelope.
type ProfileRestrictions struct {
	Version               string   `json:"version"`
	MaximumAgeRating      *int     `json:"maximumAgeRating"`
	AllowUnrated          bool     `json:"allowUnrated"`
	BlockedLabels         []string `json:"blockedLabels"`
	AllowDownloads        bool     `json:"allowDownloads"`
	AllowLiveTV           bool     `json:"allowLiveTV"`
	AllowDVR              bool     `json:"allowDvr"`
	AllowWatchWithFriends bool     `json:"allowWatchWithFriends"`
	AllowFeedback         bool     `json:"allowFeedback"`
}

type AccountProfile struct {
	ID                string
	AccountID         string
	Origin            string
	ExternalProfileID string
	DisplayName       string
	AvatarURL         string
	IsPrimary         bool
	SortOrder         int
	Restrictions      ProfileRestrictions
	HasPIN            bool
	PINRequired       bool
	PINRevision       int64
	PolicyUpdatedAt   string
	DisabledAt        string
}

type CreateLocalProfileInput struct {
	DisplayName  string
	AvatarURL    string
	PIN          string
	Restrictions ProfileRestrictions
}

type HostedProfileSnapshot struct {
	ExternalProfileID string              `json:"id"`
	AccountID         string              `json:"accountId"`
	DisplayName       string              `json:"name"`
	Avatar            *ProfileAvatar      `json:"avatar,omitempty"`
	IsPrimary         bool                `json:"isPrimary"`
	IsAccountAdmin    bool                `json:"isAccountAdmin"`
	SortOrder         int                 `json:"sortOrder"`
	PINRequired       bool                `json:"hasPIN"`
	PINRevision       int64               `json:"pinRevision"`
	PolicyUpdatedAt   time.Time           `json:"policyUpdatedAt"`
	Restrictions      ProfileRestrictions `json:"policy"`
}

type ProfileSelectionGrant struct {
	Token              string
	AccountID          string
	ProfileID          string
	AuthProvider       string
	Purpose            string
	SourceProofID      string
	DeviceID           string
	InstallationID     string
	PINRevision        int64
	BrowserBindingHash string
	ExpiresAt          time.Time
}

func validateProfileAvatar(avatar *ProfileAvatar) (*ProfileAvatar, error) {
	if avatar == nil {
		return nil, nil
	}
	reference := strings.TrimSpace(avatar.Reference)
	if (avatar.Kind != "preset" && avatar.Kind != "custom") || reference == "" || utf8.RuneCountInString(reference) > 256 {
		return nil, errInvalidHostedProfileSnapshot
	}
	avatar.Reference = reference
	return avatar, nil
}

func profileAvatarReference(avatar *ProfileAvatar) string {
	if avatar == nil {
		return ""
	}
	return strings.TrimSpace(avatar.Reference)
}

func validLocalProfilePIN(pin string) bool {
	if len(pin) != localProfilePINLength {
		return false
	}
	for _, digit := range pin {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// localProfilePINDummyHash keeps missing and malformed credentials on the
// same cost-8 bcrypt path as real PINs without allocating work per request.
const localProfilePINDummyHash = "$2b$08$ZNSVVlfSU.wyMtr0EXeLs.wVLLWhYzyL9QPECq6.S3NxMmAv9n4/m"

func hashLocalProfilePIN(ctx context.Context, callsite kdfCallsite, pin string) (string, error) {
	if !validLocalProfilePIN(pin) {
		return "", errInvalidProfilePIN
	}
	hash, err := runKDF(ctx, callsite, kdfLaneMutation, func() ([]byte, error) {
		return bcrypt.GenerateFromPassword([]byte(pin), localProfilePINBcryptCost)
	})
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyLocalProfilePINHash(ctx context.Context, callsite kdfCallsite, encoded, pin string) (bool, error) {
	eligible := validLocalProfilePIN(pin) && validProfilePINBcryptHash(encoded, localProfilePINBcryptCost)
	hash := []byte(encoded)
	if !eligible {
		hash = []byte(localProfilePINDummyHash)
	}
	compareErr, err := runKDF(ctx, callsite, kdfLaneCompare, func() (error, error) {
		return bcrypt.CompareHashAndPassword(hash, []byte(pin)), nil
	})
	return compareErr == nil && eligible, err
}

func validProfilePINBcryptHash(encoded string, expectedCost int) bool {
	cost, err := bcrypt.Cost([]byte(encoded))
	if err != nil {
		return false
	}
	return cost == expectedCost
}

func normalizeProfileDisplayName(value string) (string, bool) {
	value = strings.TrimSpace(value)
	return value, value != "" && utf8.RuneCountInString(value) <= 80
}

func defaultProfileRestrictions() ProfileRestrictions {
	return ProfileRestrictions{
		Version: profileRestrictionsVersion, AllowUnrated: true, BlockedLabels: []string{},
		AllowDownloads: true, AllowLiveTV: true, AllowDVR: true,
		AllowWatchWithFriends: true, AllowFeedback: true,
	}
}

func normalizeProfilePolicyComparable(value string) string {
	return strings.ToLower(norm.NFC.String(strings.TrimSpace(value)))
}

func isZeroProfileRestrictions(value ProfileRestrictions) bool {
	return value.Version == "" && value.MaximumAgeRating == nil && value.BlockedLabels == nil &&
		!value.AllowUnrated && !value.AllowDownloads && !value.AllowLiveTV && !value.AllowDVR &&
		!value.AllowWatchWithFriends && !value.AllowFeedback
}

func validateProfileRestrictions(value ProfileRestrictions, allowImplicitDefault bool) (ProfileRestrictions, error) {
	if allowImplicitDefault && isZeroProfileRestrictions(value) {
		value = defaultProfileRestrictions()
	}
	if value.Version != profileRestrictionsVersion {
		return ProfileRestrictions{}, fmt.Errorf("%w: unsupported version", errInvalidProfileRestriction)
	}
	if value.MaximumAgeRating != nil && (*value.MaximumAgeRating < 0 || *value.MaximumAgeRating > 21) {
		return ProfileRestrictions{}, fmt.Errorf("%w: maximumAgeRating must be between 0 and 21", errInvalidProfileRestriction)
	}
	if len(value.BlockedLabels) > maximumBlockedProfileLabels {
		return ProfileRestrictions{}, fmt.Errorf("%w: too many blocked labels", errInvalidProfileRestriction)
	}
	labels := make([]string, 0, len(value.BlockedLabels))
	seen := map[string]struct{}{}
	for _, raw := range value.BlockedLabels {
		label := norm.NFC.String(strings.TrimSpace(raw))
		if label == "" || utf8.RuneCountInString(label) > maximumBlockedProfileLabelRunes {
			return ProfileRestrictions{}, fmt.Errorf("%w: blocked label is empty or too long", errInvalidProfileRestriction)
		}
		key := strings.ToLower(label)
		if _, exists := seen[key]; exists {
			return ProfileRestrictions{}, fmt.Errorf("%w: blocked labels must be unique", errInvalidProfileRestriction)
		}
		seen[key] = struct{}{}
		labels = append(labels, label)
	}
	sort.Slice(labels, func(i, j int) bool { return strings.ToLower(labels[i]) < strings.ToLower(labels[j]) })
	value.BlockedLabels = labels
	return value, nil
}

func encodeProfileRestrictions(value ProfileRestrictions) (string, error) {
	value, err := validateProfileRestrictions(value, true)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errInvalidProfileRestriction, err)
	}
	return string(encoded), nil
}

func decodeProfileRestrictions(value string) (ProfileRestrictions, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var restrictions ProfileRestrictions
	if err := decoder.Decode(&restrictions); err != nil {
		return ProfileRestrictions{}, fmt.Errorf("%w: %v", errInvalidProfileRestriction, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ProfileRestrictions{}, fmt.Errorf("%w: trailing content", errInvalidProfileRestriction)
	}
	return validateProfileRestrictions(restrictions, false)
}

func intersectProfilePermissions(account map[string]bool, restrictions ProfileRestrictions, profileIsPrimary bool) map[string]bool {
	effective := make(map[string]bool, len(account))
	for permission, allowed := range account {
		if !allowed {
			continue
		}
		// A subordinate viewing profile is not an alternate server
		// administrator. It inherits the account's media envelope, but never its
		// server-management authority—even when the account owns the server.
		if !profileIsPrimary && administrativeProfilePermission(permission) {
			continue
		}
		blocked := permission == "downloadMedia" && !restrictions.AllowDownloads ||
			(permission == "viewLiveTV" || permission == "playLiveTV") && !restrictions.AllowLiveTV ||
			(permission == "viewDVR" || permission == "scheduleDVR" || permission == "deleteDVRRecordings" || permission == "manageDVR") && !restrictions.AllowDVR ||
			permission == "watchWithFriends" && !restrictions.AllowWatchWithFriends
		if blocked {
			continue
		}
		effective[permission] = true
	}
	return effective
}

func administrativeProfilePermission(permission string) bool {
	switch permission {
	case "manageServer", "manageLibraries", "manageUsers":
		return true
	default:
		return false
	}
}

func stricterContentRating(accountLimit, profileLimit string) string {
	accountLimit = normalizeMaxContentRating(accountLimit)
	profileLimit = normalizeMaxContentRating(profileLimit)
	if accountLimit == "" {
		return profileLimit
	}
	if profileLimit == "" {
		return accountLimit
	}
	if contentRatingRank(profileLimit) > 0 && contentRatingRank(profileLimit) < contentRatingRank(accountLimit) {
		return profileLimit
	}
	return accountLimit
}

func (s *Server) resolveRequestPrincipalContext(ctx context.Context, accountID, profileID string) (RequestPrincipal, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	accountID = strings.TrimSpace(accountID)
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		profileID = accountID
	}
	if cached, ok, err := s.cachedRequestPrincipal(ctx, accountID, profileID); err != nil {
		return RequestPrincipal{}, err
	} else if ok {
		return cached, nil
	}

	var principal RequestPrincipal
	var accountPermissionsJSON, accountPreferencesJSON, profilePreferencesJSON, restrictionsJSON, accountRating, profileRating, authOrigin string
	var primary, profilesAllowed int
	err := s.queryUserRow(ctx, `
		SELECT u.id, p.id, p.origin, p.is_primary, p.display_name, p.avatar_url,
			COALESCE(u.auth_origin, 'local'), COALESCE(u.portico_user_id, ''), COALESCE(u.portico_membership_id, ''), u.role,
			u.permissions_json, u.preferences_json, p.preferences_json, p.restrictions_json,
			COALESCE(u.max_content_rating, ''), COALESCE(p.max_content_rating, ''),
			COALESCE(u.max_active_sessions, 0), COALESCE(u.max_active_streams, 0), COALESCE(u.remote_bitrate_limit_mbps, 0),
			COALESCE(u.allow_account_profiles, 1)
		FROM users u
		JOIN profiles p ON p.account_id = u.id
		WHERE u.id = ? AND p.id = ?
			AND COALESCE(u.disabled_at, '') = '' AND COALESCE(p.disabled_at, '') = ''`, accountID, profileID).
		Scan(&principal.AccountID, &principal.ProfileID, &principal.ProfileOrigin, &primary,
			&principal.DisplayName, &principal.AvatarURL, &authOrigin, &principal.MembershipIdentity.HostedAccountID, &principal.MembershipIdentity.HostedMembershipID, &principal.MembershipEnvelope.Role,
			&accountPermissionsJSON, &accountPreferencesJSON, &profilePreferencesJSON, &restrictionsJSON,
			&accountRating, &profileRating, &principal.MaxActiveSessions,
			&principal.MaxActiveStreams, &principal.RemoteBitrateLimitMbps, &profilesAllowed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RequestPrincipal{}, errProfileNotFound
		}
		return RequestPrincipal{}, err
	}

	principal.AuthenticationAuthority = normalizeAuthenticationAuthority(authOrigin)
	principal.MembershipIdentity.ServerAccountID = principal.AccountID
	principal.ProfileIsPrimary = primary == 1
	principal.MembershipEnvelope.AccountProfilesAllowed = profilesAllowed == 1
	principal.AccountProfilesAllowed = principal.MembershipEnvelope.AccountProfilesAllowed
	if !principal.ProfileIsPrimary && !principal.AccountProfilesAllowed {
		return RequestPrincipal{}, errProfileNotAllowed
	}

	accountPermissions := map[string]bool{}
	_ = json.Unmarshal([]byte(accountPermissionsJSON), &accountPermissions)
	restrictions, err := decodeProfileRestrictions(restrictionsJSON)
	if err != nil {
		return RequestPrincipal{}, err
	}
	principal.MembershipEnvelope.Permissions = clonePermissionMap(accountPermissions)
	applyStoredMembershipPolicies(&principal.MembershipEnvelope, accountPreferencesJSON)
	principal.MembershipEnvelope.LibraryIDs, err = s.requestPrincipalLibraryIDsContext(ctx, principal.AccountID, principal.MembershipEnvelope.Role)
	if err != nil {
		return RequestPrincipal{}, err
	}
	principal.Permissions = intersectProfilePermissions(principal.MembershipEnvelope.Permissions, restrictions, principal.ProfileIsPrimary)
	principal.Preferences = decodeUserPreferences(profilePreferencesJSON)
	principal.MaxContentRating = stricterContentRating(accountRating, profileRating)
	principal.MaximumAgeRating = restrictions.MaximumAgeRating
	principal.AllowUnrated = restrictions.AllowUnrated
	principal.BlockedLabels = append([]string(nil), restrictions.BlockedLabels...)
	principal.AllowFeedback = restrictions.AllowFeedback
	principal.MaxActiveSessions = normalizeMaxActiveSessions(principal.MaxActiveSessions)
	principal.MaxActiveStreams = normalizeMaxActiveStreams(principal.MaxActiveStreams)
	principal.RemoteBitrateLimitMbps = normalizeRemoteBitrateLimitMbps(principal.RemoteBitrateLimitMbps)
	principal.MembershipEnvelope.MaxContentRating = normalizeMaxContentRating(accountRating)
	principal.MembershipEnvelope.MaxActiveSessions = principal.MaxActiveSessions
	principal.MembershipEnvelope.MaxActiveStreams = principal.MaxActiveStreams
	principal.MembershipEnvelope.RemoteBitrateLimitMbps = principal.RemoteBitrateLimitMbps
	s.rememberRequestPrincipal(ctx, principal)
	return principal, nil
}

func resolveRequestPrincipalTx(tx *sql.Tx, accountID, profileID string) (RequestPrincipal, error) {
	var principal RequestPrincipal
	var accountPermissionsJSON, accountPreferencesJSON, profilePreferencesJSON, restrictionsJSON, accountRating, profileRating, authOrigin string
	var primary, profilesAllowed int
	err := tx.QueryRow(`
		SELECT u.id, p.id, p.origin, p.is_primary, p.display_name, p.avatar_url,
			COALESCE(u.auth_origin, 'local'), COALESCE(u.portico_user_id, ''), COALESCE(u.portico_membership_id, ''), u.role,
			u.permissions_json, u.preferences_json, p.preferences_json, p.restrictions_json,
			COALESCE(u.max_content_rating, ''), COALESCE(p.max_content_rating, ''),
			COALESCE(u.max_active_sessions, 0), COALESCE(u.max_active_streams, 0), COALESCE(u.remote_bitrate_limit_mbps, 0),
			COALESCE(u.allow_account_profiles, 1)
		FROM users u
		JOIN profiles p ON p.account_id = u.id
		WHERE u.id = ? AND p.id = ?
			AND COALESCE(u.disabled_at, '') = '' AND COALESCE(p.disabled_at, '') = ''`, accountID, profileID).
		Scan(&principal.AccountID, &principal.ProfileID, &principal.ProfileOrigin, &primary,
			&principal.DisplayName, &principal.AvatarURL, &authOrigin, &principal.MembershipIdentity.HostedAccountID, &principal.MembershipIdentity.HostedMembershipID, &principal.MembershipEnvelope.Role,
			&accountPermissionsJSON, &accountPreferencesJSON, &profilePreferencesJSON, &restrictionsJSON,
			&accountRating, &profileRating, &principal.MaxActiveSessions,
			&principal.MaxActiveStreams, &principal.RemoteBitrateLimitMbps, &profilesAllowed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RequestPrincipal{}, errProfileNotFound
		}
		return RequestPrincipal{}, err
	}
	principal.AuthenticationAuthority = normalizeAuthenticationAuthority(authOrigin)
	principal.MembershipIdentity.ServerAccountID = principal.AccountID
	principal.ProfileIsPrimary = primary == 1
	principal.MembershipEnvelope.AccountProfilesAllowed = profilesAllowed == 1
	principal.AccountProfilesAllowed = principal.MembershipEnvelope.AccountProfilesAllowed
	if !principal.ProfileIsPrimary && !principal.AccountProfilesAllowed {
		return RequestPrincipal{}, errProfileNotAllowed
	}
	accountPermissions := map[string]bool{}
	_ = json.Unmarshal([]byte(accountPermissionsJSON), &accountPermissions)
	restrictions, err := decodeProfileRestrictions(restrictionsJSON)
	if err != nil {
		return RequestPrincipal{}, err
	}
	principal.MembershipEnvelope.Permissions = clonePermissionMap(accountPermissions)
	applyStoredMembershipPolicies(&principal.MembershipEnvelope, accountPreferencesJSON)
	principal.MembershipEnvelope.LibraryIDs, err = requestPrincipalLibraryIDsTx(tx, principal.AccountID, principal.MembershipEnvelope.Role)
	if err != nil {
		return RequestPrincipal{}, err
	}
	principal.Permissions = intersectProfilePermissions(principal.MembershipEnvelope.Permissions, restrictions, principal.ProfileIsPrimary)
	principal.Preferences = decodeUserPreferences(profilePreferencesJSON)
	principal.MaxContentRating = stricterContentRating(accountRating, profileRating)
	principal.MaximumAgeRating = restrictions.MaximumAgeRating
	principal.AllowUnrated = restrictions.AllowUnrated
	principal.BlockedLabels = append([]string(nil), restrictions.BlockedLabels...)
	principal.AllowFeedback = restrictions.AllowFeedback
	principal.MaxActiveSessions = normalizeMaxActiveSessions(principal.MaxActiveSessions)
	principal.MaxActiveStreams = normalizeMaxActiveStreams(principal.MaxActiveStreams)
	principal.RemoteBitrateLimitMbps = normalizeRemoteBitrateLimitMbps(principal.RemoteBitrateLimitMbps)
	principal.MembershipEnvelope.MaxContentRating = normalizeMaxContentRating(accountRating)
	principal.MembershipEnvelope.MaxActiveSessions = principal.MaxActiveSessions
	principal.MembershipEnvelope.MaxActiveStreams = principal.MaxActiveStreams
	principal.MembershipEnvelope.RemoteBitrateLimitMbps = principal.RemoteBitrateLimitMbps
	return principal, nil
}

func clonePermissionMap(input map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(input))
	for permission, allowed := range input {
		cloned[permission] = allowed
	}
	return cloned
}

func applyStoredMembershipPolicies(envelope *RequestMembershipEnvelope, raw string) {
	if envelope == nil {
		return
	}
	envelope.AccessSchedule = decodeUserAccessSchedule(raw)
	envelope.TagPolicy = decodeUserTagPolicy(raw)
	envelope.DevicePolicy = decodeUserDevicePolicy(raw)
	envelope.ChannelPolicy = decodeUserChannelPolicy(raw)
}

func (s *Server) requestPrincipalLibraryIDsContext(ctx context.Context, accountID, role string) ([]string, error) {
	query := `SELECT library_id FROM user_library_access WHERE user_id = ? ORDER BY library_id ASC`
	args := []any{accountID}
	if strings.EqualFold(strings.TrimSpace(role), "owner") {
		query = `SELECT id FROM libraries ORDER BY id ASC`
		args = nil
	}
	rows, err := s.queryUserRead(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequestPrincipalLibraryIDs(rows)
}

func requestPrincipalLibraryIDsTx(tx *sql.Tx, accountID, role string) ([]string, error) {
	query := `SELECT library_id FROM user_library_access WHERE user_id = ? ORDER BY library_id ASC`
	args := []any{accountID}
	if strings.EqualFold(strings.TrimSpace(role), "owner") {
		query = `SELECT id FROM libraries ORDER BY id ASC`
		args = nil
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequestPrincipalLibraryIDs(rows)
}

func scanRequestPrincipalLibraryIDs(rows *sql.Rows) ([]string, error) {
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func applyRequestPrincipal(user *User, principal RequestPrincipal) {
	if user == nil {
		return
	}
	user.AccountID = principal.AccountID
	user.ProfileID = principal.ProfileID
	user.ProfileIsPrimary = principal.ProfileIsPrimary
	user.AuthOrigin = map[AuthenticationAuthority]string{AuthenticationAuthorityHosted: "portico", AuthenticationAuthorityLocal: "local"}[principal.AuthenticationAuthority]
	user.PorticoUserID = principal.MembershipIdentity.HostedAccountID
	user.PorticoMembershipID = principal.MembershipIdentity.HostedMembershipID
	user.Role = principal.MembershipEnvelope.Role
	user.LibraryIDs = append([]string(nil), principal.MembershipEnvelope.LibraryIDs...)
	user.AccessSchedule = principal.MembershipEnvelope.AccessSchedule
	user.TagPolicy = principal.MembershipEnvelope.TagPolicy
	user.DevicePolicy = principal.MembershipEnvelope.DevicePolicy
	user.ChannelPolicy = principal.MembershipEnvelope.ChannelPolicy
	user.DisplayName = principal.DisplayName
	if avatarURL := strings.TrimSpace(principal.AvatarURL); avatarURL != "" {
		user.ProfileImageURL = avatarURL
	}
	user.Permissions = principal.Permissions
	user.Preferences = principal.Preferences
	user.MaxContentRating = principal.MaxContentRating
	user.MaximumAgeRating = principal.MaximumAgeRating
	user.AllowUnrated = principal.AllowUnrated
	user.BlockedProfileLabels = append([]string(nil), principal.BlockedLabels...)
	user.AllowFeedback = principal.AllowFeedback
	user.MaxActiveSessions = principal.MaxActiveSessions
	user.MaxActiveStreams = principal.MaxActiveStreams
	user.RemoteBitrateLimitMbps = principal.RemoteBitrateLimitMbps
	user.AccountProfilesAllowed = principal.AccountProfilesAllowed
}

func viewerProfileID(user User) string {
	return strings.TrimSpace(user.ProfileID)
}

func selectedProfileMayManageAccount(user User) bool {
	profileID := strings.TrimSpace(user.ProfileID)
	accountID := accountIDForUser(user)
	return accountID != "" && profileID != "" && user.ProfileIsPrimary
}

func accountIDForUser(user User) string {
	return strings.TrimSpace(user.AccountID)
}

func (s *Server) accountIDForProfileContext(ctx context.Context, profileID string) (string, error) {
	var accountID string
	if err := s.queryUserRow(ctx, `SELECT account_id FROM profiles WHERE id = ? AND disabled_at = ''`, strings.TrimSpace(profileID)).Scan(&accountID); err != nil {
		return "", err
	}
	return accountID, nil
}

func (s *Server) accountAndProfileIDsContext(ctx context.Context, identityID string) (string, string) {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return "", ""
	}
	accountID, err := s.accountIDForProfileContext(ctx, identityID)
	if err != nil {
		return "", ""
	}
	return accountID, identityID
}

func (s *Server) createLocalProfileContext(ctx context.Context, accountID string, input CreateLocalProfileInput) (AccountProfile, error) {
	name, ok := normalizeProfileDisplayName(input.DisplayName)
	if !ok {
		return AccountProfile{}, errors.New("profile display name is required and must be at most 80 characters")
	}
	if input.PIN != "" && !validLocalProfilePIN(input.PIN) {
		return AccountProfile{}, errInvalidProfilePIN
	}
	restrictionsJSON, err := encodeProfileRestrictions(input.Restrictions)
	if err != nil {
		return AccountProfile{}, err
	}
	normalizedRestrictions, err := decodeProfileRestrictions(restrictionsJSON)
	if err != nil {
		return AccountProfile{}, err
	}
	var pinHash string
	if input.PIN != "" {
		hash, err := hashLocalProfilePIN(ctx, kdfProfilePINSetHash, input.PIN)
		if err != nil {
			return AccountProfile{}, err
		}
		pinHash = hash
	}
	profile := AccountProfile{
		ID: randomID("prof"), AccountID: strings.TrimSpace(accountID), Origin: "local",
		DisplayName: name, AvatarURL: strings.TrimSpace(input.AvatarURL), Restrictions: normalizedRestrictions,
		PINRequired: input.PIN != "",
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withUserTxTagged(ctx, []string{"profiles", "local_profile_pin_credentials"}, func(tx *sql.Tx) error {
		var origin, role, permissionsJSON, preferencesJSON, accountRating string
		var profilesAllowed int
		if err := tx.QueryRow(`
			SELECT COALESCE(auth_origin, 'local'), role, permissions_json, preferences_json, COALESCE(max_content_rating, ''), COALESCE(allow_account_profiles, 1)
			FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, profile.AccountID).
			Scan(&origin, &role, &permissionsJSON, &preferencesJSON, &accountRating, &profilesAllowed); err != nil {
			return err
		}
		if origin == "portico" {
			return errors.New("hosted account profiles must be synchronized from hosted services")
		}
		if profilesAllowed != 1 {
			return errProfileNotAllowed
		}
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM profiles WHERE account_id = ? AND disabled_at = ''`, profile.AccountID).Scan(&count); err != nil {
			return err
		}
		if count >= maxProfilesPerAccount {
			return errProfileLimit
		}
		var primaryPINCount int
		if err := tx.QueryRow(`
			SELECT COUNT(*)
			FROM profiles primary_profile
			JOIN local_profile_pin_credentials credential ON credential.profile_id = primary_profile.id
			WHERE primary_profile.account_id = ? AND primary_profile.is_primary = 1
				AND primary_profile.origin = 'local' AND primary_profile.disabled_at = ''`, profile.AccountID).Scan(&primaryPINCount); err != nil {
			return err
		}
		if primaryPINCount != 1 {
			return errPrimaryProfilePINRequired
		}
		if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM profiles WHERE account_id = ?`, profile.AccountID).Scan(&profile.SortOrder); err != nil {
			return err
		}
		_, err := tx.Exec(`
			INSERT INTO profiles (
				id, account_id, origin, external_profile_id, is_primary, sort_order, display_name, avatar_url,
				role, permissions_json, preferences_json, restrictions_json, pin_required, max_content_rating,
				max_active_sessions, remote_bitrate_limit_mbps, created_at, updated_at
			) VALUES (?, ?, 'local', '', 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`,
			profile.ID, profile.AccountID, profile.SortOrder, profile.DisplayName, profile.AvatarURL,
			role, permissionsJSON, preferencesJSON, restrictionsJSON, boolInt(pinHash != ""), accountRating, now, now)
		if err != nil {
			return err
		}
		if pinHash != "" {
			_, err = tx.Exec(`
				INSERT INTO local_profile_pin_credentials (profile_id, pin_hash, created_at, updated_at)
				VALUES (?, ?, ?, ?)`, profile.ID, pinHash, now, now)
			profile.HasPIN = err == nil
		}
		return err
	})
	return profile, err
}

func (s *Server) listAccountProfilesContext(ctx context.Context, accountID string) ([]AccountProfile, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT p.id, p.account_id, p.origin, p.external_profile_id, p.display_name, p.avatar_url,
			p.is_primary, p.sort_order, p.restrictions_json, p.pin_required, p.pin_revision, p.policy_updated_at,
			CASE WHEN c.profile_id IS NULL THEN 0 ELSE 1 END, p.disabled_at
		FROM profiles p
		LEFT JOIN local_profile_pin_credentials c ON c.profile_id = p.id
		WHERE p.account_id = ? AND p.disabled_at = ''
		ORDER BY p.sort_order, p.created_at, p.id`, strings.TrimSpace(accountID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []AccountProfile{}
	for rows.Next() {
		var profile AccountProfile
		var restrictionsJSON string
		var primary, pinRequired, hasLocalPIN int
		if err := rows.Scan(&profile.ID, &profile.AccountID, &profile.Origin, &profile.ExternalProfileID,
			&profile.DisplayName, &profile.AvatarURL, &primary, &profile.SortOrder, &restrictionsJSON,
			&pinRequired, &profile.PINRevision, &profile.PolicyUpdatedAt, &hasLocalPIN, &profile.DisabledAt); err != nil {
			return nil, err
		}
		profile.IsPrimary = primary == 1
		profile.PINRequired = pinRequired == 1
		profile.HasPIN = hasLocalPIN == 1
		profile.Restrictions, err = decodeProfileRestrictions(restrictionsJSON)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *Server) setLocalProfilePINContext(ctx context.Context, accountID, profileID, pin string) error {
	return s.setLocalProfilePINAuthorizedContext(ctx, accountID, profileID, pin, "", "")
}

func (s *Server) setLocalProfilePINAuthorizedContext(ctx context.Context, accountID, profileID, pin, expectedHash, sessionID string) error {
	if !validLocalProfilePIN(pin) {
		return errInvalidProfilePIN
	}
	hash, err := hashLocalProfilePIN(ctx, kdfProfilePINSetHash, pin)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	return s.withSecurityFenceTxTagged(ctx, []string{"profiles", "local_profile_pin_credentials"}, func(tx *sql.Tx) error {
		if expectedHash != "" || sessionID != "" {
			if err := validatePasswordSessionTx(tx, accountID, accountID, sessionID, expectedHash, now); err != nil {
				return err
			}
		}
		var origin string
		if err := tx.QueryRow(`SELECT origin FROM profiles WHERE id = ? AND account_id = ? AND disabled_at = ''`, profileID, accountID).Scan(&origin); err != nil {
			return errProfileAccountMismatch
		}
		if origin != "local" {
			return errHostedProfileLocalPIN
		}
		_, err := tx.Exec(`
			INSERT INTO local_profile_pin_credentials (profile_id, pin_hash, failed_attempts, locked_until, created_at, updated_at)
			VALUES (?, ?, 0, '', ?, ?)
			ON CONFLICT(profile_id) DO UPDATE SET
				pin_hash = excluded.pin_hash, failed_attempts = 0, locked_until = '', updated_at = excluded.updated_at`,
			profileID, hash, nowText, nowText)
		if err == nil {
			_, err = tx.Exec(`UPDATE profiles SET pin_required = 1, pin_revision = pin_revision + 1, updated_at = ? WHERE id = ? AND account_id = ?`, nowText, profileID, accountID)
		}
		return err
	})
}

func (s *Server) clearLocalProfilePINContext(ctx context.Context, accountID, profileID string) error {
	return s.clearLocalProfilePINAuthorizedContext(ctx, accountID, profileID, "", "")
}

func (s *Server) clearLocalProfilePINAuthorizedContext(ctx context.Context, accountID, profileID, expectedHash, sessionID string) error {
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	return s.withSecurityFenceTxTagged(ctx, []string{"profiles", "local_profile_pin_credentials"}, func(tx *sql.Tx) error {
		if expectedHash != "" || sessionID != "" {
			if err := validatePasswordSessionTx(tx, accountID, accountID, sessionID, expectedHash, now); err != nil {
				return err
			}
		}
		var origin string
		var primary int
		if err := tx.QueryRow(`SELECT origin, is_primary FROM profiles WHERE id = ? AND account_id = ? AND disabled_at = ''`, profileID, accountID).Scan(&origin, &primary); err != nil {
			return errProfileAccountMismatch
		}
		if origin != "local" {
			return errHostedProfileLocalPIN
		}
		if primary == 1 {
			var children int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM profiles WHERE account_id = ? AND is_primary = 0 AND disabled_at = ''`, accountID).Scan(&children); err != nil {
				return err
			}
			if children > 0 {
				return errPrimaryProfilePINInUse
			}
		}
		if _, err := tx.Exec(`DELETE FROM local_profile_pin_credentials WHERE profile_id = ?`, profileID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE profiles SET pin_required = 0, pin_revision = pin_revision + 1, updated_at = ? WHERE id = ? AND account_id = ?`, nowText, profileID, accountID)
		return err
	})
}

func (s *Server) verifyLocalProfilePINContext(ctx context.Context, accountID, profileID, pin string, now time.Time) (bool, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for attempt := 0; attempt < 3; attempt++ {
		snapshot, err := s.loadLocalProfilePINSnapshot(ctx, accountID, profileID)
		if err != nil {
			// Missing/malformed/foreign credentials still spend exactly one
			// admitted configured-cost comparison before returning their product error.
			_, kdfErr := verifyLocalProfilePINHash(ctx, kdfProfilePINAdminCompare, "", pin)
			if kdfErr != nil {
				return false, kdfErr
			}
			return false, err
		}
		evaluation, err := evaluateLocalProfilePIN(ctx, kdfProfilePINAdminCompare, snapshot, pin, now)
		if err != nil {
			return false, err
		}
		err = s.withUserTxTagged(ctx, []string{"profiles", "local_profile_pin_credentials"}, func(tx *sql.Tx) error {
			return applyLocalProfilePINEvaluationTx(tx, snapshot, evaluation, now)
		})
		if errors.Is(err, errProfilePINConcurrentChange) {
			continue
		}
		if err != nil {
			return false, err
		}
		if evaluation.mutate && !evaluation.valid {
			return false, nil
		}
		return evaluation.valid, evaluation.result
	}
	return false, errProfilePINConcurrentChange
}

var errProfilePINConcurrentChange = errors.New("profile pin changed concurrently")

type localProfilePINSnapshot struct {
	accountID, profileID, origin                         string
	pinHash, lockedUntil, nextAttemptAt, credentialStamp string
	failed, pinRequired, primary, profilesAllowed        int
	pinRevision                                          int64
}

type localProfilePINEvaluation struct {
	valid, mutate              bool
	failed                     int
	lockedUntil, nextAttemptAt string
	result                     error
}

func (s *Server) loadLocalProfilePINSnapshot(ctx context.Context, accountID, profileID string) (localProfilePINSnapshot, error) {
	var snapshot localProfilePINSnapshot
	snapshot.accountID, snapshot.profileID = strings.TrimSpace(accountID), strings.TrimSpace(profileID)
	err := s.queryUserRow(ctx, `
		SELECT p.origin, p.pin_required, p.is_primary, p.pin_revision, COALESCE(u.allow_account_profiles, 1),
		       COALESCE(c.pin_hash, ''), COALESCE(c.failed_attempts, 0), COALESCE(c.locked_until, ''),
		       COALESCE(c.next_attempt_at, ''), COALESCE(c.updated_at, '')
		FROM profiles p JOIN users u ON u.id = p.account_id
		LEFT JOIN local_profile_pin_credentials c ON c.profile_id = p.id
		WHERE p.id = ? AND p.account_id = ? AND p.disabled_at = '' AND COALESCE(u.disabled_at, '') = ''`,
		snapshot.profileID, snapshot.accountID).Scan(&snapshot.origin, &snapshot.pinRequired, &snapshot.primary,
		&snapshot.pinRevision, &snapshot.profilesAllowed, &snapshot.pinHash, &snapshot.failed, &snapshot.lockedUntil,
		&snapshot.nextAttemptAt, &snapshot.credentialStamp)
	if errors.Is(err, sql.ErrNoRows) || snapshot.pinHash == "" {
		return localProfilePINSnapshot{}, errProfilePINNotSet
	}
	if err != nil {
		return localProfilePINSnapshot{}, err
	}
	if snapshot.origin != "local" {
		return localProfilePINSnapshot{}, errHostedProfileLocalPIN
	}
	return snapshot, nil
}

func evaluateLocalProfilePIN(ctx context.Context, callsite kdfCallsite, snapshot localProfilePINSnapshot, pin string, now time.Time) (localProfilePINEvaluation, error) {
	valid, err := verifyLocalProfilePINHash(ctx, callsite, snapshot.pinHash, pin)
	if err != nil {
		return localProfilePINEvaluation{}, err
	}
	evaluation := localProfilePINEvaluation{valid: valid, failed: snapshot.failed, lockedUntil: snapshot.lockedUntil, nextAttemptAt: snapshot.nextAttemptAt}
	if locked, parseErr := time.Parse(time.RFC3339Nano, snapshot.lockedUntil); parseErr == nil && locked.After(now) {
		evaluation.valid = false
		evaluation.result = profilePINRetryError(errProfilePINLocked, locked, now)
		return evaluation, nil
	}
	if nextAttempt, parseErr := time.Parse(time.RFC3339Nano, snapshot.nextAttemptAt); parseErr == nil && nextAttempt.After(now) {
		evaluation.valid = false
		evaluation.result = profilePINRetryError(errProfilePINBackoff, nextAttempt, now)
		return evaluation, nil
	}
	if snapshot.lockedUntil != "" {
		evaluation.failed = 0
	}
	evaluation.mutate = true
	if valid {
		evaluation.failed, evaluation.lockedUntil, evaluation.nextAttemptAt = 0, "", ""
		return evaluation, nil
	}
	evaluation.failed++
	delay := localProfilePINBackoffBase << min(evaluation.failed-1, 3)
	if delay > localProfilePINBackoffMaximum {
		delay = localProfilePINBackoffMaximum
	}
	nextAttempt := now.Add(delay)
	evaluation.lockedUntil = ""
	if evaluation.failed >= localProfilePINFailureLimit {
		nextAttempt = now.Add(localProfilePINLockDuration)
		evaluation.lockedUntil = nextAttempt.Format(time.RFC3339Nano)
	}
	evaluation.nextAttemptAt = nextAttempt.Format(time.RFC3339Nano)
	evaluation.result = profilePINRetryError(errProfilePINBackoff, nextAttempt, now)
	if evaluation.lockedUntil != "" {
		evaluation.result = profilePINRetryError(errProfilePINLocked, nextAttempt, now)
	}
	return evaluation, nil
}

func validateLocalProfilePINSnapshotTx(tx *sql.Tx, snapshot localProfilePINSnapshot) error {
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM profiles p JOIN users u ON u.id = p.account_id
		JOIN local_profile_pin_credentials c ON c.profile_id = p.id
		WHERE p.id = ? AND p.account_id = ? AND p.origin = ? AND p.pin_required = ? AND p.pin_revision = ?
		  AND p.disabled_at = '' AND COALESCE(u.disabled_at, '') = '' AND COALESCE(u.allow_account_profiles, 1) = ?
		  AND c.pin_hash = ? AND c.failed_attempts = ? AND c.locked_until = ? AND c.next_attempt_at = ? AND c.updated_at = ?`,
		snapshot.profileID, snapshot.accountID, snapshot.origin, snapshot.pinRequired, snapshot.pinRevision,
		snapshot.profilesAllowed, snapshot.pinHash, snapshot.failed, snapshot.lockedUntil, snapshot.nextAttemptAt,
		snapshot.credentialStamp).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return errProfilePINConcurrentChange
	}
	return nil
}

func applyLocalProfilePINEvaluationTx(tx *sql.Tx, snapshot localProfilePINSnapshot, evaluation localProfilePINEvaluation, now time.Time) error {
	if err := validateLocalProfilePINSnapshotTx(tx, snapshot); err != nil {
		return err
	}
	if !evaluation.mutate {
		return nil
	}
	result, err := tx.Exec(`
		UPDATE local_profile_pin_credentials
		SET failed_attempts = ?, locked_until = ?, next_attempt_at = ?, updated_at = ?
		WHERE profile_id = ? AND pin_hash = ? AND failed_attempts = ? AND locked_until = ? AND next_attempt_at = ? AND updated_at = ?`,
		evaluation.failed, evaluation.lockedUntil, evaluation.nextAttemptAt, now.Format(time.RFC3339Nano),
		snapshot.profileID, snapshot.pinHash, snapshot.failed, snapshot.lockedUntil, snapshot.nextAttemptAt, snapshot.credentialStamp)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return errProfilePINConcurrentChange
	}
	return nil
}

func (s *Server) issueLocalProfileSelectionGrantContext(ctx context.Context, accountID, profileID, pin, deviceID, installationID string, now time.Time) (ProfileSelectionGrant, error) {
	return s.issueLocalProfileSelectionGrantForPurposeContext(ctx, accountID, profileID, pin, deviceID, installationID, "native", randomID("direct_pauth"), now)
}

func (s *Server) issueLocalProfileSelectionGrantForPurposeContext(ctx context.Context, accountID, profileID, pin, deviceID, installationID, purpose, sourceProofID string, now time.Time) (ProfileSelectionGrant, error) {
	accountID = strings.TrimSpace(accountID)
	profileID = strings.TrimSpace(profileID)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for attempt := 0; attempt < 3; attempt++ {
		snapshot, snapshotErr := s.loadLocalProfilePINSnapshot(ctx, accountID, profileID)
		if snapshotErr != nil {
			if !errors.Is(snapshotErr, errProfilePINNotSet) {
				return ProfileSelectionGrant{}, snapshotErr
			}
			// A profile without a credential is allowed only when its current
			// profile row says no PIN is required. Read that fact without holding
			// a write transaction; the transaction below rechecks it exactly.
			var origin string
			var pinRequired, primary, profilesAllowed int
			var pinRevision int64
			err := s.queryUserRow(ctx, `
				SELECT p.origin, p.pin_required, p.is_primary, p.pin_revision, COALESCE(u.allow_account_profiles, 1)
				FROM profiles p JOIN users u ON u.id = p.account_id
				WHERE p.id = ? AND p.account_id = ? AND p.disabled_at = '' AND COALESCE(u.disabled_at, '') = ''`, profileID, accountID).
				Scan(&origin, &pinRequired, &primary, &pinRevision, &profilesAllowed)
			if err != nil {
				return ProfileSelectionGrant{}, errProfileAccountMismatch
			}
			if origin != "local" {
				return ProfileSelectionGrant{}, errHostedProfileLocalPIN
			}
			if pinRequired == 1 {
				_, kdfErr := verifyLocalProfilePINHash(ctx, kdfProfilePINSelectCompare, "", pin)
				if kdfErr != nil {
					return ProfileSelectionGrant{}, kdfErr
				}
				return ProfileSelectionGrant{}, errProfilePINNotSet
			}
			snapshot = localProfilePINSnapshot{accountID: accountID, profileID: profileID, origin: origin, pinRequired: pinRequired, primary: primary, pinRevision: pinRevision, profilesAllowed: profilesAllowed}
		}
		var evaluation localProfilePINEvaluation
		if snapshot.pinRequired == 1 {
			var err error
			evaluation, err = evaluateLocalProfilePIN(ctx, kdfProfilePINSelectCompare, snapshot, pin, now)
			if err != nil {
				return ProfileSelectionGrant{}, err
			}
		}
		var grant ProfileSelectionGrant
		err := s.withUserTxTagged(ctx, []string{"profiles", "local_profile_pin_credentials", "profile_selection_grants"}, func(tx *sql.Tx) error {
			if snapshot.pinRequired == 1 {
				if err := applyLocalProfilePINEvaluationTx(tx, snapshot, evaluation, now); err != nil {
					return err
				}
			} else {
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM profiles p JOIN users u ON u.id = p.account_id WHERE p.id = ? AND p.account_id = ? AND p.origin = 'local' AND p.pin_required = 0 AND p.pin_revision = ? AND p.disabled_at = '' AND COALESCE(u.disabled_at, '') = '' AND COALESCE(u.allow_account_profiles, 1) = ?`, profileID, accountID, snapshot.pinRevision, snapshot.profilesAllowed).Scan(&count); err != nil || count != 1 {
					return errProfilePINConcurrentChange
				}
			}
			if snapshot.pinRequired == 1 && !evaluation.valid {
				return nil
			}
			var err error
			grant, err = s.mintProfileSelectionGrantBoundTx(tx, accountID, profileID, "local", purpose, sourceProofID, deviceID, installationID, now)
			return err
		})
		if errors.Is(err, errProfilePINConcurrentChange) {
			continue
		}
		if err != nil {
			return ProfileSelectionGrant{}, err
		}
		if snapshot.pinRequired == 1 && !evaluation.valid {
			return ProfileSelectionGrant{}, evaluation.result
		}
		return grant, nil
	}
	return ProfileSelectionGrant{}, errProfilePINConcurrentChange
}

// issueHostedProfileSelectionGrantContext exchanges a Cloud-signed selection
// envelope against current Hosted state, verifies the returned envelope again
// locally, reconciles its complete profile projection, and only then mints a
// server-local one-time grant. The Hosted envelope is never a server session
// credential and the Hosted device ID is never reused as a local device ID.
func (s *Server) issueHostedProfileSelectionGrantContext(ctx context.Context, accountID string, raw json.RawMessage, hostedDeviceID, localDeviceID, installationID string, now time.Time) (ProfileSelectionGrant, error) {
	return s.issueHostedProfileSelectionGrantForPurposeContext(ctx, accountID, raw, hostedDeviceID, localDeviceID, installationID, "native", now)
}

func (s *Server) issueHostedProfileSelectionGrantForPurposeContext(ctx context.Context, accountID string, raw json.RawMessage, hostedDeviceID, localDeviceID, installationID, purpose string, now time.Time) (ProfileSelectionGrant, error) {
	accountID = strings.TrimSpace(accountID)
	hostedDeviceID = strings.TrimSpace(hostedDeviceID)
	localDeviceID = strings.TrimSpace(localDeviceID)
	installationID = strings.TrimSpace(installationID)
	if hostedDeviceID == "" || localDeviceID == "" {
		return ProfileSelectionGrant{}, errInvalidHostedProfileSelectionAssertion
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var hostedAccountID string
	if err := s.queryUserRow(ctx, `SELECT COALESCE(portico_user_id, '') FROM users WHERE id = ? AND auth_origin = 'portico' AND COALESCE(disabled_at, '') = ''`, accountID).Scan(&hostedAccountID); err != nil {
		return ProfileSelectionGrant{}, fmt.Errorf("%w: linked Portico account lookup failed: %v", errInvalidHostedProfileSelectionAssertion, err)
	}
	if strings.TrimSpace(hostedAccountID) == "" {
		return ProfileSelectionGrant{}, fmt.Errorf("%w: linked Portico account subject is empty", errInvalidHostedProfileSelectionAssertion)
	}
	remoteSettings, err := s.remoteAccessSettings()
	if err != nil {
		return ProfileSelectionGrant{}, fmt.Errorf("%w: claimed server identity lookup failed: %v", errInvalidHostedProfileSelectionAssertion, err)
	}
	if remoteSettings.ClaimStatus != "claimed" || strings.TrimSpace(remoteSettings.ServerID) == "" {
		return ProfileSelectionGrant{}, fmt.Errorf("%w: server does not have a claimed hosted identity", errInvalidHostedProfileSelectionAssertion)
	}
	exchangedRaw, err := s.exchangeHostedProfileSelectionEnvelope(ctx, remoteSettings, raw)
	if err != nil {
		var hostedErr *hostedHTTPError
		if !errors.As(err, &hostedErr) || hostedErr.StatusCode >= http.StatusInternalServerError {
			return ProfileSelectionGrant{}, fmt.Errorf("%w: %v", errHostedProfileSelectionExchangeUnavailable, err)
		}
		return ProfileSelectionGrant{}, fmt.Errorf("%w: Hosted selection exchange rejected the envelope", errInvalidHostedProfileSelectionAssertion)
	}
	var signingKeyRef struct {
		SignatureKeyID string `json:"signatureKeyId"`
	}
	if err := json.Unmarshal(exchangedRaw, &signingKeyRef); err != nil {
		return ProfileSelectionGrant{}, fmt.Errorf("%w: Hosted selection exchange returned an invalid envelope", errInvalidHostedProfileSelectionAssertion)
	}
	if err := s.ensureHostedDocumentKey(ctx, remoteSettings.HostedBaseURL, signingKeyRef.SignatureKeyID); err != nil {
		return ProfileSelectionGrant{}, fmt.Errorf("%w: %v", errHostedProfileSelectionExchangeUnavailable, err)
	}
	envelope, payload, err := s.verifyHostedProfileSelectionEnvelope(exchangedRaw, hostedAccountID, remoteSettings.ServerID, hostedDeviceID, now)
	if err != nil {
		return ProfileSelectionGrant{}, err
	}
	if err := s.reconcileHostedProfileSelectionEnvelopeContext(ctx, accountID, envelope, now); err != nil {
		return ProfileSelectionGrant{}, err
	}
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	var grant ProfileSelectionGrant
	err = s.withUserTxTagged(ctx, []string{"profiles", "hosted_profile_snapshot_state", "hosted_profile_selection_assertion_receipts", "profile_selection_grants"}, func(tx *sql.Tx) error {
		var profileID string
		var pinRevision int64
		var primary, profilesAllowed int
		if err := tx.QueryRow(`
			SELECT p.id, p.pin_revision, p.is_primary, COALESCE(u.allow_account_profiles, 1)
			FROM profiles p
			JOIN users u ON u.id = p.account_id
			JOIN hosted_profile_snapshot_state snapshot ON snapshot.account_id = p.account_id AND snapshot.quarantined_at = ''
			WHERE p.account_id = ? AND p.origin = 'hosted' AND p.external_profile_id = ? AND p.disabled_at = ''`,
			accountID, envelope.ProfileID).Scan(&profileID, &pinRevision, &primary, &profilesAllowed); err != nil {
			return fmt.Errorf("%w: hosted profile is absent from the current server snapshot", errInvalidHostedProfileSelectionAssertion)
		}
		if profilesAllowed != 1 && primary != 1 {
			return errProfileNotAllowed
		}
		if pinRevision != envelope.PINRevision {
			return fmt.Errorf("%w: profile PIN revision does not match the current server snapshot", errInvalidHostedProfileSelectionAssertion)
		}
		var replayed int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM hosted_profile_selection_assertion_receipts WHERE assertion_id = ? OR payload_digest = ?`, envelope.AssertionID, digest).Scan(&replayed); err != nil {
			return err
		}
		if replayed != 0 {
			return errHostedProfileSelectionAssertionReplayed
		}
		if _, err := tx.Exec(`
			INSERT INTO hosted_profile_selection_assertion_receipts (
				assertion_id, payload_digest, account_id, profile_id, hosted_device_id, local_device_id, installation_id,
				pin_revision, expires_at, accepted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			envelope.AssertionID, digest, accountID, profileID, hostedDeviceID, localDeviceID, installationID,
			envelope.PINRevision, envelope.ExpiresAt, now.Format(time.RFC3339Nano)); err != nil {
			return errHostedProfileSelectionAssertionReplayed
		}
		grant, err = s.mintProfileSelectionGrantBoundTx(tx, accountID, profileID, "portico", purpose, envelope.AssertionID, localDeviceID, installationID, now)
		return err
	})
	return grant, err
}

// mintProfileSelectionGrantTx is deliberately transaction-only. Callers must
// first prove a local PIN or atomically consume a verified hosted assertion.
func (s *Server) mintProfileSelectionGrantTx(tx *sql.Tx, accountID, profileID, provider, deviceID, installationID string, now time.Time) (ProfileSelectionGrant, error) {
	return s.mintProfileSelectionGrantBoundTx(tx, accountID, profileID, provider, "native", randomID("direct_proof"), deviceID, installationID, now)
}

func (s *Server) mintProfileSelectionGrantBoundTx(tx *sql.Tx, accountID, profileID, provider, purpose, sourceProofID, deviceID, installationID string, now time.Time) (ProfileSelectionGrant, error) {
	if tx == nil {
		return ProfileSelectionGrant{}, errInvalidProfileSelectionGrant
	}
	accountID = strings.TrimSpace(accountID)
	profileID = strings.TrimSpace(profileID)
	provider = normalizeAuthProvider(provider)
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	sourceProofID = strings.TrimSpace(sourceProofID)
	deviceID = strings.TrimSpace(deviceID)
	installationID = strings.TrimSpace(installationID)
	if (purpose != "browser" && purpose != "native") || sourceProofID == "" || deviceID == "" {
		return ProfileSelectionGrant{}, errInvalidProfileSelectionGrant
	}
	if strings.TrimSpace(installationID) == "" {
		_ = tx.QueryRow(`SELECT COALESCE(installation_id, '') FROM devices WHERE id = ? AND user_id = ?`, deviceID, accountID).Scan(&installationID)
	}
	var profileOrigin string
	var pinRevision int64
	if err := tx.QueryRow(`
		SELECT p.origin, p.pin_revision
		FROM profiles p JOIN users u ON u.id = p.account_id
		WHERE p.id = ? AND p.account_id = ? AND p.disabled_at = '' AND COALESCE(u.disabled_at, '') = ''`, profileID, accountID).
		Scan(&profileOrigin, &pinRevision); err != nil {
		return ProfileSelectionGrant{}, errProfileAccountMismatch
	}
	if profileOrigin == "hosted" && provider != "portico" || profileOrigin == "local" && provider != "local" {
		return ProfileSelectionGrant{}, errInvalidProfileSelectionGrant
	}
	rawToken, err := randomNativeCredentialToken(s.nativeCredentialEntropyReader())
	if err != nil {
		return ProfileSelectionGrant{}, err
	}
	grantID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "pgrant")
	if err != nil {
		return ProfileSelectionGrant{}, err
	}
	expiresAt := now.Add(profileSelectionGrantTTL)
	_, err = tx.Exec(`
		INSERT INTO profile_selection_grants (
			id, account_id, profile_id, token_hash, auth_provider, device_id, installation_id,
			pin_revision, expires_at, consumed_at, created_at, purpose, account_authentication_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		grantID, accountID, profileID, hashToken(rawToken), provider, deviceID, installationID,
		pinRevision, expiresAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), purpose, sourceProofID)
	if err != nil {
		return ProfileSelectionGrant{}, err
	}
	return ProfileSelectionGrant{Token: rawToken, AccountID: accountID, ProfileID: profileID, AuthProvider: provider, Purpose: purpose, SourceProofID: sourceProofID,
		DeviceID: deviceID, InstallationID: installationID, PINRevision: pinRevision, ExpiresAt: expiresAt}, nil
}

func (s *Server) consumeProfileSelectionGrantTx(tx *sql.Tx, rawToken, expectedAccountID, provider, deviceID, installationID string, now time.Time) (RequestPrincipal, error) {
	return s.consumeProfileSelectionGrantForPurposeTx(tx, rawToken, expectedAccountID, provider, "native", deviceID, installationID, now)
}

func (s *Server) consumeProfileSelectionGrantForPurposeTx(tx *sql.Tx, rawToken, expectedAccountID, provider, purpose, deviceID, installationID string, now time.Time) (RequestPrincipal, error) {
	if tx == nil || strings.TrimSpace(rawToken) == "" {
		return RequestPrincipal{}, errInvalidProfileSelectionGrant
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	provider = normalizeAuthProvider(provider)
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	var grantID, accountID, profileID, storedProvider, storedPurpose, sourceProofID, storedDeviceID, storedInstallationID, expiresAt, consumedAt string
	var pinRevision int64
	err := tx.QueryRow(`
		SELECT id, account_id, profile_id, auth_provider, purpose, account_authentication_id, device_id, installation_id,
			pin_revision, expires_at, consumed_at
		FROM profile_selection_grants WHERE token_hash = ?`, hashToken(rawToken)).
		Scan(&grantID, &accountID, &profileID, &storedProvider, &storedPurpose, &sourceProofID, &storedDeviceID, &storedInstallationID, &pinRevision, &expiresAt, &consumedAt)
	if err != nil {
		return RequestPrincipal{}, errInvalidProfileSelectionGrant
	}
	if consumedAt != "" {
		return RequestPrincipal{}, errProfileSelectionGrantConsumed
	}
	expiresAtTime, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !expiresAtTime.After(now) {
		return RequestPrincipal{}, errInvalidProfileSelectionGrant
	}
	if accountID != strings.TrimSpace(expectedAccountID) || storedProvider != provider || storedPurpose != purpose || sourceProofID == "" ||
		(storedDeviceID != "" && storedDeviceID != strings.TrimSpace(deviceID)) {
		return RequestPrincipal{}, errInvalidProfileSelectionGrant
	}
	var currentPINRevision int64
	if err := tx.QueryRow(`SELECT pin_revision FROM profiles WHERE id = ? AND account_id = ? AND disabled_at = ''`, profileID, accountID).Scan(&currentPINRevision); err != nil || currentPINRevision != pinRevision {
		return RequestPrincipal{}, errInvalidProfileSelectionGrant
	}
	result, err := tx.Exec(`UPDATE profile_selection_grants SET consumed_at = ? WHERE id = ? AND consumed_at = ''`,
		now.Format(time.RFC3339Nano), grantID)
	if err != nil {
		return RequestPrincipal{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return RequestPrincipal{}, errProfileSelectionGrantConsumed
	}
	return resolveRequestPrincipalTx(tx, accountID, profileID)
}

func profileRequiresSelectionGrantTx(tx *sql.Tx, accountID, profileID string) (bool, error) {
	if tx == nil {
		return false, errInvalidProfileSelectionGrant
	}
	var pinRequired int
	if err := tx.QueryRow(`
		SELECT pin_required FROM profiles
		WHERE id = ? AND account_id = ? AND disabled_at = ''`, profileID, accountID).Scan(&pinRequired); err != nil {
		return false, err
	}
	return pinRequired == 1, nil
}
