package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"reflect"
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
// replaces the older) and a few per client network (A47), so one client can't
// hold every stream slot.
type claimStreams struct {
	mu     sync.Mutex
	byOp   map[string]context.CancelFunc
	byPeer map[string]int
}

const claimStreamsPerPeer = 4

func (c *claimStreams) open(op, peer string, cancel context.CancelFunc) (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byOp == nil {
		c.byOp, c.byPeer = map[string]context.CancelFunc{}, map[string]int{}
	}
	if c.byPeer[peer] >= claimStreamsPerPeer {
		return nil, false
	}
	if old := c.byOp[op]; old != nil {
		old()
	}
	c.byOp[op] = cancel
	c.byPeer[peer]++
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.byPeer[peer]--
		if c.byPeer[peer] <= 0 {
			delete(c.byPeer, peer)
		}
		// Only the stream that still owns the operation clears it.
		if reflect.ValueOf(c.byOp[op]).Pointer() == reflect.ValueOf(cancel).Pointer() {
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
	if path == "/v1/server-claims/pending" || path == "/v1/server-claims/events" {
		var signed claimweb.Signed
		if !decode(w, r, &signed) {
			return true
		}
		q, e := claimweb.Verify(signed, h.Web.Audience, time.Now())
		if e != nil {
			claimFailure(w, e)
			return true
		}
		if path == "/v1/server-claims/pending" {
			p, e := h.Web.Store.RegisterWebClaim(r.Context(), q, h.Web.Keys.SealKey)
			claimRespond(w, p, e)
			return true
		}
		ctx, cancel := context.WithDeadline(r.Context(), q.ExpiresAt)
		defer cancel()
		closeStream, ok := h.Web.streams.open(q.OperationID, budgetIP(peer(r)), cancel)
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
	e = h.Web.Store.DecideWebClaim(r.Context(), a, body.Code, body.OwnerProfileID, body.ReplacesServerID, path == "/v1/server-claims/approve-code", h.Service)
	claimRespond(w, StatusCodeResponse{Code: "claim_decided", Status: "decided"}, e)
	return true
}
