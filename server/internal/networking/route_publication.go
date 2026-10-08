package networking

import (
	"context"
	"crypto/rand"
	"encoding/base64"
)

type PublishedCandidate struct {
	BaseURL string `json:"baseUrl"`
	Class   string `json:"class"`
}
type RouteObservation struct {
	Generation    int64                `json:"generation,string"`
	RemoteEnabled bool                 `json:"remoteEnabled"`
	CGNAT         bool                 `json:"cgnat"`
	Candidates    []PublishedCandidate `json:"candidates"`
}

// proveRoute uses the existing installed-claim endpoint proof, including its
// short-lived registration and exact authority guard. It adds only a topology
// generation to the Hosted publication transaction, not to claim authority.
func (h *ClaimHandler) proveRoute(ctx context.Context, v Intent, generation int64, origin string) (EndpointReceipt, error) {
	caller, ok := h.transport.(endpointCaller)
	if !ok {
		return EndpointReceipt{}, ErrUnavailable
	}
	random := make([]byte, 32)
	if _, e := rand.Read(random); e != nil {
		return EndpointReceipt{}, e
	}
	id := base64.RawURLEncoding.EncodeToString(random)
	h.endpoints.Lock()
	if len(h.endpoints.pending) >= 2 {
		h.endpoints.Unlock()
		return EndpointReceipt{}, ErrUnavailable
	}
	h.endpoints.pending[id] = endpointPending{v, origin, ctx}
	h.endpoints.Unlock()
	defer func() { h.endpoints.Lock(); delete(h.endpoints.pending, id); h.endpoints.Unlock() }()
	var out EndpointReceipt
	q := struct {
		Generation int64 `json:"generation,string"`
		Endpoint   struct {
			BaseURL        string `json:"baseUrl"`
			RegistrationID string `json:"registrationId"`
		} `json:"endpoint"`
	}{Generation: generation}
	q.Endpoint.BaseURL = origin
	q.Endpoint.RegistrationID = id
	if e := caller.CallServer(ctx, v, ProveRoute, q, &out); e != nil {
		return out, e
	}
	if out.BaseURL != origin || out.VerifiedAt.IsZero() {
		return out, ErrInvalid
	}
	return out, h.store.Current(ctx, v)
}
