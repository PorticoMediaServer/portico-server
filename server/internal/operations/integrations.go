package operations

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/dbwork"
)

// IntegrationSettings is a projection of server_settings, never a second store.
// These limits restrict existing grants; none authorizes a member or a route.
type IntegrationSettings struct {
	WebhookRetries                                          string
	WebhookTimeoutSeconds, WebhookQueueSize, WebhookLogSize int
	WebhookHTTPSOnly                                        bool
	CredentialMaxDays                                       int
	MemberTokenScope                                        string
	TokensPerMember, APIKeysMax                             int
	KeyInAddress, AssistantsEnabled                         bool
}

func (s Settings) Integrations() IntegrationSettings {
	return IntegrationSettings{s.WebhookRetries, s.WebhookTimeoutSeconds, s.WebhookQueueSize, s.WebhookLogSize, s.WebhookHTTPSOnly, s.CredentialMaxDays, s.MemberTokenScope, s.TokensPerMember, s.APIKeysMax, s.KeyInAddress, s.AssistantsEnabled}
}

func validateIntegrations(v Settings) []string {
	fields := []string{}
	if !oneOf(v.WebhookRetries, []string{"long", "short", "none"}) {
		fields = append(fields, "values.webhookRetries")
	}
	if !oneOf(v.MemberTokenScope, []string{"same", "play", "read"}) {
		fields = append(fields, "values.memberTokenScope")
	}
	for _, check := range []struct {
		key     string
		value   int
		choices []int
	}{
		{"webhookTimeoutSeconds", v.WebhookTimeoutSeconds, []int{5, 15, 30, 60}},
		{"webhookQueueSize", v.WebhookQueueSize, []int{100, 1000, 5000}},
		{"webhookLogSize", v.WebhookLogSize, []int{50, 200, 1000}},
		{"credentialMaxDays", v.CredentialMaxDays, []int{0, 30, 90, 365}},
		{"tokensPerMember", v.TokensPerMember, []int{5, 10, 25, 50}},
		{"apiKeysMax", v.APIKeysMax, []int{10, 25, 100, 250}},
	} {
		valid := false
		for _, choice := range check.choices {
			valid = valid || choice == check.value
		}
		if !valid {
			fields = append(fields, "values."+check.key)
		}
	}
	return fields
}

// ReadIntegrationSettingsTx uses the caller's authorized/current snapshot.
// Missing schema or invalid values fail rather than opening a permissive gate.
func ReadIntegrationSettingsTx(ctx context.Context, tx *sql.Tx) (IntegrationSettings, error) {
	if tx == nil {
		return IntegrationSettings{}, errors.New("integration settings require a transaction")
	}
	v := DefaultSettings()
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(s."webhookRetries",?),COALESCE(s."webhookTimeoutSeconds",?),COALESCE(s."webhookQueueSize",?),COALESCE(s."webhookLogSize",?),COALESCE(s."webhookHTTPSOnly",?),COALESCE(s."credentialMaxDays",?),COALESCE(s."memberTokenScope",?),COALESCE(s."tokensPerMember",?),COALESCE(s."apiKeysMax",?),COALESCE(s."keyInAddress",?),COALESCE(s."assistantsEnabled",?) FROM server_settings AS s WHERE s.singleton=1`, v.WebhookRetries, v.WebhookTimeoutSeconds, v.WebhookQueueSize, v.WebhookLogSize, v.WebhookHTTPSOnly, v.CredentialMaxDays, v.MemberTokenScope, v.TokensPerMember, v.APIKeysMax, v.KeyInAddress, v.AssistantsEnabled).Scan(&v.WebhookRetries, &v.WebhookTimeoutSeconds, &v.WebhookQueueSize, &v.WebhookLogSize, &v.WebhookHTTPSOnly, &v.CredentialMaxDays, &v.MemberTokenScope, &v.TokensPerMember, &v.APIKeysMax, &v.KeyInAddress, &v.AssistantsEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IntegrationSettings{}, err
	}
	if fields := validateIntegrations(v); len(fields) > 0 {
		return IntegrationSettings{}, &ValidationError{Fields: fields}
	}
	return v.Integrations(), nil
}

func ReadIntegrationSettings(ctx context.Context, db *sql.DB) (IntegrationSettings, error) {
	snapshot, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		return IntegrationSettings{}, err
	}
	defer snapshot.Rollback()
	return ReadIntegrationSettingsTx(ctx, snapshot.Tx())
}
