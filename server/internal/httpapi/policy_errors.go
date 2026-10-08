package httpapi

import (
	"net/http"
	"strconv"

	"portico.local/apikit/apierror"
)

func policyErrorDefinitions() []apierror.Definition {
	return []apierror.Definition{
		{Code: "metadata_unavailable", Status: 503, Message: "Metadata is temporarily unavailable.", Retry: apierror.SameRequest},
		{Code: "artwork_pending", Status: 404, Message: "Artwork is not available yet.", Retry: apierror.SameRequest},
		{Code: "artwork_gone", Status: 410, Message: "This artwork version is no longer available.", Retry: apierror.AfterRefresh},
		{Code: "method_not_allowed", Status: 405, Message: "This action is not supported.", Retry: apierror.Never},
		{Code: "web_unavailable", Status: 503, Message: "The web app is temporarily unavailable.", Retry: apierror.SameRequest},
	}
}

func policyCatalogue() *apierror.Registry {
	registry, err := apierror.New(append(v1ErrorDefinitions(), policyErrorDefinitions()...)...)
	if err != nil {
		panic(err)
	}
	return registry
}

var policyErrors = policyCatalogue()

func policyError(w http.ResponseWriter, code string) {
	seconds, _ := strconv.Atoi(w.Header().Get("Retry-After"))
	if seconds == 0 && (code == "artwork_pending" || code == "metadata_unavailable" || code == "web_unavailable") {
		seconds = 2
	}
	policyErrors.Write(w, w.Header().Get("X-Request-Id"), &apierror.Error{Code: code, RetryAfterSeconds: seconds})
}
