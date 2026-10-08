package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

// ClaimHandler is a dedicated proposed route set. Root must register it inside
// the existing origin/rate/concurrency middleware at the current schema cutover.
// Neither cookie parsing nor account-ID-only approval is implemented here.
type ClaimHandler struct {
	Web              *ClaimWeb
	Service          *claims.Service
	CaptureAuthority func(context.Context, []byte) (identity.CurrentFamilyExpectation, error)
}

func (h *ClaimHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.Service == nil || h.CaptureAuthority == nil {
		claimFailure(w, errors.New("unconfigured"))
		return
	}
	if h.web(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.URL.RawQuery != "" {
		claimFailure(w, claims.ErrInvalid)
		return
	}
	switch r.URL.Path {
	case "/v1/server-claims/approve":
		token, ok := claimBearer(r)
		if !ok {
			claimFailure(w, identity.ErrCredentials)
			return
		}
		var in claims.ApprovalInput
		if !claimDecode(w, r, &in) {
			return
		}
		authority, e := h.CaptureAuthority(ctx, security.Hash(token))
		if e != nil {
			claimFailure(w, e)
			return
		}
		result, e := h.Service.Approve(ctx, authority, in)
		claimRespond(w, result, e)
	case "/v1/server-claims/ack":
		token, ok := claimBearer(r)
		if !ok {
			claimFailure(w, identity.ErrCredentials)
			return
		}
		var in claims.Commit
		if !claimDecode(w, r, &in) {
			return
		}
		result, e := h.Service.Acknowledge(ctx, token, in)
		claimRespond(w, result, e)
	case "/v1/server-claims/nonce", "/v1/server-claims/finalize", "/v1/server-claims/result", "/v1/server-claims/cancel", "/v1/server-claims/cleanup":
		// Positive and deny-only server proof paths never accept account bearers.
		if len(r.Header.Values("Authorization")) != 0 {
			claimFailure(w, claims.ErrInvalid)
			return
		}
		var in ClaimSignedRequest
		if !claimDecode(w, r, &in) {
			return
		}
		payload, e := base64.RawURLEncoding.Strict().DecodeString(in.Payload)
		if e != nil || base64.RawURLEncoding.EncodeToString(payload) != in.Payload {
			claimFailure(w, claims.ErrInvalid)
			return
		}
		signature, e := base64.RawURLEncoding.Strict().DecodeString(in.Signature)
		if e != nil || base64.RawURLEncoding.EncodeToString(signature) != in.Signature {
			claimFailure(w, claims.ErrInvalid)
			return
		}
		switch r.URL.Path {
		case "/v1/server-claims/nonce":
			v, e := h.Service.Nonce(ctx, payload, signature)
			claimRespond(w, v, e)
		case "/v1/server-claims/finalize":
			v, e := h.Service.Finalize(ctx, payload, signature)
			claimRespond(w, v, e)
		case "/v1/server-claims/result":
			v, e := h.Service.Result(ctx, payload, signature)
			claimRespond(w, v, e)
		case "/v1/server-claims/cleanup":
			v, e := h.Service.Cleanup(ctx, payload, signature)
			claimRespond(w, v, e)
		case "/v1/server-claims/cancel":
			v, e := h.Service.Cancel(ctx, payload, signature)
			claimRespond(w, v, e)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
func claimBearer(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	return token, len(token) >= 20 && len(token) <= 128 && !strings.ContainsAny(token, " \t\r\n")
}
func claimDecode(w http.ResponseWriter, r *http.Request, out any) bool {
	kind, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || kind != "application/json" {
		claimFailure(w, claims.ErrInvalid)
		return false
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	if e != nil || len(raw) == 0 {
		claimFailure(w, claims.ErrInvalid)
		return false
	}
	// Exact struct re-encoding also rejects duplicate/unknown fields. Shared
	// consumers use this same deterministic JSON envelope rather than extra DTOs.
	if json.Unmarshal(raw, out) != nil {
		claimFailure(w, claims.ErrInvalid)
		return false
	}
	canonical, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, canonical) {
		claimFailure(w, claims.ErrInvalid)
		return false
	}
	return true
}
func claimRespond(w http.ResponseWriter, value any, e error) {
	if e != nil {
		claimFailure(w, e)
		return
	}
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > 16384 {
		claimFailure(w, errors.New("response unavailable"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
func claimFailure(w http.ResponseWriter, e error) {
	status, code := http.StatusServiceUnavailable, "unavailable"
	switch {
	case errors.Is(e, claims.ErrApprovalRequired):
		status, code = http.StatusConflict, "approval_required"
	case errors.Is(e, claims.ErrCancelled):
		status, code = http.StatusConflict, "claim_cancelled"
	case errors.Is(e, claims.ErrStale):
		status, code = http.StatusConflict, "claim_stale"
	case errors.Is(e, claims.ErrInvalid), errors.Is(e, claimweb.ErrInvalid):
		// A55: malformed claim-web input is the caller's error, not retryable.
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(e, identity.ErrCredentials):
		status, code = http.StatusUnauthorized, "credentials_required"
	case errors.Is(e, claims.ErrCapacity):
		status, code = http.StatusTooManyRequests, "unavailable"
	}
	message := "This server claim could not be completed."
	if status == http.StatusBadRequest {
		message = "Check the request and try again."
	}
	if status == http.StatusUnauthorized {
		message = "Authentication is required."
	}
	fail(w, status, code, message, status == http.StatusServiceUnavailable || status == http.StatusTooManyRequests)
}
