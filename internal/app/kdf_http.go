package app

import (
	"errors"
	"net/http"
	"strconv"
)

func writeKDFUnavailable(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, errKDFUnavailable) && !errors.Is(err, errKDFCancelled) {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int(kdfRetryAfter.Seconds()))))
	writeProductError(w, http.StatusServiceUnavailable, "credential_verification_unavailable", "Credential verification is temporarily busy. Try again shortly.")
	return true
}
