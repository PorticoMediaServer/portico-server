package app

import (
	"errors"
	"testing"
	"time"
)

func TestValidateHostedProfileDirectoryProjectionAllowsMissingAvatar(t *testing.T) {
	issuedAt := time.Now().UTC().Truncate(time.Second)
	profiles := []HostedProfileSnapshot{{
		ExternalProfileID: "hosted-primary-without-avatar",
		AccountID:         "hosted-account",
		DisplayName:       "Justin",
		Avatar:            &ProfileAvatar{},
		IsPrimary:         true,
		IsAccountAdmin:    true,
		SortOrder:         0,
		PolicyUpdatedAt:   issuedAt.Add(-time.Minute),
		Restrictions:      defaultProfileRestrictions(),
	}}

	if err := validateHostedProfileDirectoryProjection(profiles, "hosted-account", issuedAt); err != nil {
		t.Fatalf("valid hosted primary profile without avatar was rejected: %v", err)
	}
	if profiles[0].Avatar != nil {
		t.Fatalf("empty hosted avatar was not normalized to an absent avatar: %#v", profiles[0].Avatar)
	}
}

func TestValidateHostedProfileDirectoryProjectionRejectsMalformedNonEmptyAvatar(t *testing.T) {
	issuedAt := time.Now().UTC().Truncate(time.Second)
	profiles := []HostedProfileSnapshot{{
		ExternalProfileID: "hosted-primary-with-malformed-avatar",
		AccountID:         "hosted-account",
		DisplayName:       "Justin",
		Avatar:            &ProfileAvatar{Kind: "untrusted", Reference: "avatar-reference"},
		IsPrimary:         true,
		IsAccountAdmin:    true,
		SortOrder:         0,
		PolicyUpdatedAt:   issuedAt.Add(-time.Minute),
		Restrictions:      defaultProfileRestrictions(),
	}}

	if err := validateHostedProfileDirectoryProjection(profiles, "hosted-account", issuedAt); !errors.Is(err, errInvalidHostedProfileSnapshot) {
		t.Fatalf("malformed hosted avatar error = %v, want %v", err, errInvalidHostedProfileSnapshot)
	}
}
