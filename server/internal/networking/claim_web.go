package networking

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"portico.local/server/internal/claimweb"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/supervise"
	"strconv"
	"time"
)

type localWebClaim struct {
	Signed    claimweb.Signed `json:"signed"`
	Code      string          `json:"code"`
	URL       string          `json:"url"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

func (h *ClaimHandler) beginWebClaim(ctx context.Context, v Intent, address string) (localWebClaim, error) {
	var out localWebClaim
	transport, ok := h.transport.(*HTTPTransport)
	if !ok {
		return out, ErrUnavailable
	}
	if v.Stage != Prepared {
		return out, ErrStale
	}
	// Public proof and code survive a UI remount; private signing material never leaves its store.
	var saved string
	if e := h.store.db.QueryRowContext(ctx, `SELECT request FROM networking_web_claims WHERE operation_id=?`, v.OperationID).Scan(&saved); e == nil {
		if json.Unmarshal([]byte(saved), &out) == nil && time.Now().Before(out.ExpiresAt) {
			return out, nil
		}
		// The earlier request expired unanswered: sign a fresh one for the same
		// operation (Hosted lets a newer request replace an undecided one). Answering
		// stale here left a server that could never connect again (demo, 24 Sep).
		if _, e = dbwork.ExecWrite(ctx, h.store.db, dbwork.ClassInteractive, `DELETE FROM networking_web_claims WHERE operation_id=?`, v.OperationID); e != nil {
			return localWebClaim{}, e
		}
		out = localWebClaim{}
	}
	now := time.Now().UTC()
	account := v.AccountID
	if account == UnboundAccount {
		account = ""
	}
	q := claimweb.Request{Kind: "portico.claim.web.v1", Audience: transport.origin, OperationID: v.OperationID, ServerID: v.ServerID, AccountID: account, PublicKey: base64.RawURLEncoding.EncodeToString(v.PublicKey), LocalGeneration: v.LocalGeneration, Name: h.name(), Address: address, IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	raw, _ := json.Marshal(q)
	signature, e := h.signer.Sign(ctx, v.Binding, raw)
	if e != nil {
		return out, e
	}
	out.Signed = claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(signature)}
	if _, e = claimweb.Verify(out.Signed, transport.origin, now); e != nil {
		return out, ErrInvalid
	}
	var pending claimweb.Pending
	if e = transport.call(ctx, "pending", out.Signed, &pending, nil); e != nil {
		return out, e
	}
	fragment := url.Values{"code": {pending.Code}, "server": {q.Name}, "return": {address}}
	out.Code = pending.Code
	out.URL = transport.origin + "/claim#" + fragment.Encode()
	out.ExpiresAt = q.ExpiresAt
	encoded, _ := json.Marshal(out)
	_, e = dbwork.ExecWrite(ctx, h.store.db, dbwork.ClassInteractive, `INSERT INTO networking_web_claims(operation_id,request) VALUES(?,?) ON CONFLICT(operation_id) DO NOTHING`, v.OperationID, string(encoded))
	return out, e
}
func (h *ClaimHandler) awaitWebClaim(w http.ResponseWriter, r *http.Request) {
	actor, e := h.authorize(r)
	if e != nil || actor.Guard == nil {
		h.failure(w, r, ErrClaimOwnerRequired)
		return
	}
	var body struct {
		Expected ClaimExpected `json:"expected"`
	}
	if e = readClaimBody(w, r, &body); e != nil {
		h.failure(w, r, e)
		return
	}
	if _, loaded := h.webWaiters.LoadOrStore(body.Expected.OperationID, true); loaded {
		h.failure(w, r, stepFailure("approval already awaited", ErrUnavailable))
		return
	}
	defer h.webWaiters.Delete(body.Expected.OperationID)
	var saved string
	var local localWebClaim
	if e = h.store.db.QueryRowContext(r.Context(), `SELECT request FROM networking_web_claims WHERE operation_id=?`, body.Expected.OperationID).Scan(&saved); e != nil || json.Unmarshal([]byte(saved), &local) != nil {
		h.failure(w, r, ErrStale)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), local.ExpiresAt)
	defer cancel()
	// Capture locally authorized intent without retaining a lifecycle lease while
	// a person decides in the web browser. Reset/cancel remains immediately usable.
	e = h.runner.Do(ctx, func(ctx context.Context) error {
		guarded, e := guardedClaimRequest(ctx, h.store, actor.Guard)
		if e != nil {
			return e
		}
		_, e = h.expected(guarded, body.Expected)
		return e
	})
	if e != nil {
		h.failure(w, r, e)
		return
	}
	t, ok := h.transport.(*HTTPTransport)
	if !ok {
		h.failure(w, r, stepFailure("hosted transport", ErrUnavailable))
		return
	}
	raw, _ := json.Marshal(local.Signed)
	request, e := http.NewRequestWithContext(ctx, "POST", t.origin+"/v1/server-claims/events", bytes.NewReader(raw))
	if e != nil {
		h.failure(w, r, e)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	client := *t.client
	client.Timeout = 0
	response, e := client.Do(request)
	if e != nil {
		h.failure(w, r, stepFailure("hosted approval events", e))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		h.failure(w, r, stepFailureDetail("hosted approval events", "HTTP "+strconv.Itoa(response.StatusCode), ErrUnavailable))
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(local.ExpiresAt.Add(time.Second))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(" "))
	_ = http.NewResponseController(w).Flush()
	type decision struct {
		Status   string          `json:"status"`
		Envelope json.RawMessage `json:"approvalEnvelope"`
	}
	type received struct {
		decision decision
		err      error
	}
	done := make(chan received, 1)
	supervise.Go("networking.claim.web-decision", func() {
		var d decision
		e := json.NewDecoder(io.LimitReader(response.Body, 32<<10)).Decode(&d)
		done <- received{d, e}
	})
	timer := time.NewTicker(25 * time.Second)
	defer timer.Stop()
	var d decision
wait:
	for {
		select {
		case v := <-done:
			if v.err != nil || v.decision.Status != "approved" {
				_ = json.NewEncoder(w).Encode(map[string]string{"state": "approval_closed"})
				return
			}
			d = v.decision
			break wait
		case <-ctx.Done():
			// A63: the headers are out, so the answer is always JSON.
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "approval_closed"})
			return
		case <-timer.C:
			if _, e = w.Write([]byte(" ")); e != nil {
				return
			}
			if e = http.NewResponseController(w).Flush(); e != nil {
				return
			}
		}
	}
	var status ClaimStatus
	e = h.runner.Do(ctx, func(ctx context.Context) error {
		ctx, e = guardedClaimRequest(ctx, h.store, actor.Guard)
		if e != nil {
			return e
		}
		v, e := h.expected(ctx, body.Expected)
		if e != nil {
			return e
		}
		if _, e = h.store.Approve(ctx, v, d.Envelope); e != nil {
			return e
		}
		coordinator, e := NewCoordinator(v.OperationID, h.store.audience, h.store, h.transport, h.signer)
		if e != nil {
			return e
		}
		for range 4 {
			next, e := coordinator.Step(ctx)
			if e != nil {
				return e
			}
			if next.Stage == Installed && next.InstallationAcknowledged {
				status, e = h.snapshot(ctx)
				return e
			}
		}
		return ErrUnavailable
	})
	if e != nil {
		log.Printf("Claim approval from the web could not be applied: %v", e)
		_ = json.NewEncoder(w).Encode(map[string]string{"state": "approval_changed"})
		return
	}
	if h.certificates != nil {
		h.certificates.Wake()
	}
	if h.remote != nil {
		h.remote.Invalidate()
	}
	_ = json.NewEncoder(w).Encode(status)
}
