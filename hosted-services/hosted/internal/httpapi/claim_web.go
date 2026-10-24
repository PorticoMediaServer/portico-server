package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"sync"
	"time"
)

type ClaimWeb struct {
	Store    *postgres.Store
	Keys     *security.Keys
	Audience string
	streams  claimStreams
}

// claimStreams keeps one live decision stream per claim operation (a newer one
// replaces the older), a few per client address (A47) and a bounded share per
// wider network (HOST01), so neither one client nor one delegation can hold
// every stream slot. Pending claims are already free to register, so without
// the network share 32 /64s inside one IPv6 /48 could occupy the whole pool.
type claimStreams struct {
	mu        sync.Mutex
	byOp      map[string]*claimStream
	byPeer    map[string]int
	byNetwork map[string]int
}

// claimStream is one open stream. Its address is its identity: a replaced
// stream's close must not clear its successor's entry, and function values
// can't be compared.
type claimStream struct {
	cancel context.CancelFunc
	closed bool
}

const (
	claimStreamsPerPeer = 4
	// One /48 (IPv6) or /24 (IPv4) gets at most 16 of the 128 claim-stream
	// slots: plenty for a household, office or campus onboarding several
	// servers at once, while at least eight separate networks are needed to
	// fill the pool.
	claimStreamsPerNetwork = 16
)

// open reserves a stream for op. The returned close is idempotent.
func (c *claimStreams) open(op, peer, network string, cancel context.CancelFunc) (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byOp == nil {
		c.byOp, c.byPeer, c.byNetwork = map[string]*claimStream{}, map[string]int{}, map[string]int{}
	}
	if c.byPeer[peer] >= claimStreamsPerPeer || c.byNetwork[network] >= claimStreamsPerNetwork {
		return nil, false
	}
	if old := c.byOp[op]; old != nil {
		old.cancel()
	}
	stream := &claimStream{cancel: cancel}
	c.byOp[op] = stream
	c.byPeer[peer]++
	c.byNetwork[network]++
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if stream.closed {
			return
		}
		stream.closed = true
		c.byPeer[peer]--
		if c.byPeer[peer] <= 0 {
			delete(c.byPeer, peer)
		}
		c.byNetwork[network]--
		if c.byNetwork[network] <= 0 {
			delete(c.byNetwork, network)
		}
		// Only the stream that still owns the operation clears it.
		if c.byOp[op] == stream {
			delete(c.byOp, op)
		}
	}, true
}

func (h *ClaimHandler) web(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if path != "/v1/server-claims/pending" && path != "/v1/server-claims/lookup" && path != "/v1/server-claims/deny" && path != "/v1/server-claims/approve-code" && path != "/v1/server-claims/events" {
		return false
	}
	if h.Web == nil {
		claimFailure(w, claimweb.ErrInvalid)
		return true
	}
	if path == "/v1/server-claims/pending" {
		var body claimweb.PendingRequest
		if !decode(w, r, &body) {
			return true
		}
		if body.SetupStart != nil {
			p, e := h.Web.Store.RegisterSetupStart(r.Context(), body.Original(), *body.SetupStart, h.Web.Audience, h.Web.Keys.SealKey)
			claimRespond(w, p, e)
			return true
		}
		if body.OwnerStart != nil {
			p, e := h.Web.Store.RegisterOwnerStartWithCount(r.Context(), body.Original(), *body.OwnerStart, body.OwnerSwitchCount, h.Web.Audience, h.Web.Keys.SealKey)
			claimRespond(w, p, e)
			return true
		}
		q, e := claimweb.Verify(body.Original(), h.Web.Audience, time.Now())
		if e != nil {
			claimFailure(w, e)
			return true
		}
		p, e := h.Web.Store.RegisterWebClaim(r.Context(), q, h.Web.Keys.SealKey)
		claimRespond(w, p, e)
		return true
	}
	if path == "/v1/server-claims/events" {
		var signed claimweb.Signed
		if !decode(w, r, &signed) {
			return true
		}
		q, e := claimweb.Verify(signed, h.Web.Audience, time.Now())
		if e != nil {
			claimFailure(w, e)
			return true
		}
		ctx, cancel := context.WithDeadline(r.Context(), q.ExpiresAt)
		defer cancel()
		closeStream, ok := h.Web.streams.open(q.OperationID, budgetIP(peer(r)), budgetNetwork(peer(r)), cancel)
		if !ok {
			w.Header().Set("Retry-After", "30")
			claimFailure(w, claims.ErrCapacity)
			return true
		}
		defer closeStream()
		_ = http.NewResponseController(w).SetWriteDeadline(q.ExpiresAt.Add(time.Second))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(" "))
		_ = http.NewResponseController(w).Flush()
		type result struct {
			raw json.RawMessage
			err error
		}
		done := make(chan result, 1)
		go func() {
			defer func() {
				if recover() != nil {
					done <- result{err: claimweb.ErrInvalid}
				}
			}()
			raw, e := h.Web.Store.WaitWebClaim(ctx, q)
			done <- result{raw, e}
		}()
		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case v := <-done:
				status := "approved"
				if v.err != nil {
					status = "closed"
				}
				_ = json.NewEncoder(w).Encode(ClaimEventResult{ApprovalEnvelope: v.raw, Status: status})
				return true
			case <-ctx.Done():
				return true
			case <-heartbeat.C:
				if _, e = w.Write([]byte(" ")); e != nil {
					return true
				}
				if e = http.NewResponseController(w).Flush(); e != nil {
					return true
				}
			}
		}
	}
	token, ok := claimBearer(r)
	if !ok {
		claimFailure(w, claimweb.ErrInvalid)
		return true
	}
	a, e := h.CaptureAuthority(r.Context(), security.Hash(token))
	if e != nil {
		claimFailure(w, e)
		return true
	}
	var body ClaimCodeRequest
	if !decode(w, r, &body) {
		return true
	}
	if len(body.Code) != 49 {
		claimFailure(w, claimweb.ErrInvalid)
		return true
	}
	if path == "/v1/server-claims/lookup" {
		p, e := h.Web.Store.LookupWebClaim(r.Context(), a, body.Code)
		claimRespond(w, p, e)
		return true
	}
	e = h.Web.Store.DecideWebClaim(r.Context(), a, body.Code, body.ReplacesServerID, path == "/v1/server-claims/approve-code", h.Service)
	claimRespond(w, StatusCodeResponse{Code: "claim_decided", Status: "decided"}, e)
	return true
}
