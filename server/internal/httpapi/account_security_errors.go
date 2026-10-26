package httpapi

import "portico.local/apikit/apierror"

// Metadata for unchanged legacy password/factor outcomes used by the recovery
// code producer. These statuses, messages and retryability match failure().
func accountSecurityErrorDefinitions() []apierror.Definition {
	return []apierror.Definition{
		{Code: "authentication_busy", Status: 503, Message: "The request could not be completed.", Retry: apierror.SameRequest},
		{Code: "current_password_incorrect", Status: 403, Message: "The request could not be completed.", Retry: apierror.Never},
		{Code: "account_locked", Status: 429, Message: "The request could not be completed.", Retry: apierror.Never},
		{Code: "two_factor_missing", Status: 409, Message: "The request could not be completed.", Retry: apierror.Never},
		{Code: "two_factor_required", Status: 401, Message: publicErrorMessage("unauthorized"), Retry: apierror.Never},
		{Code: "two_factor_invalid", Status: 403, Message: "The request could not be completed.", Retry: apierror.Never},
	}
}
