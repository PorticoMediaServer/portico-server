package networking

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"portico.local/server/internal/dbwork"
	"testing"
	"time"
)

func TestOwnerRecoveryEmptyLaneSleepsUntilActualWake(t *testing.T) {
	f, h, tr := handlerFixture(t)
	// Observe the same real SQLite file through the production instrumented
	// driver. No replacement schema, query stub or artificial timer is used.
	observed, err := sql.Open(dbwork.DriverName, f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observed.Close() })
	observed.SetMaxOpenConns(1)
	h.store.db = observed
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	ctx, read := dbwork.Measure(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.RunWebClaimRecovery(ctx, func(string) ClaimRequestGuard { return nil })
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("owner recovery did not stop")
		}
	}()
	waitForStatements := func(want int64) {
		t.Helper()
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for read().Statements < want {
			select {
			case <-deadline.C:
				t.Fatalf("recovery wake read %d statements, wanted %d", read().Statements, want)
			case <-tick.C:
			}
		}
	}
	// With no setup return resolver, one empty recovery pass reads saved
	// watches, expired owner captures, queued denials and the pending-work
	// predicate. Wait for that final predicate before measuring idle work.
	const emptyPassStatements = 4
	waitForStatements(emptyPassStatements)
	before := read().Statements
	if before != emptyPassStatements {
		t.Fatalf("empty recovery startup read %d statements, wanted %d", before, emptyPassStatements)
	}
	select {
	case <-time.After(31 * time.Second):
	case <-done:
		t.Fatal("empty recovery stopped without cancellation")
	}
	if after := read().Statements; after != before {
		t.Fatalf("empty recovery polled SQLite after its startup read: %d -> %d", before, after)
	}
	h.wakeClaimRecovery()
	waitForStatements(before + emptyPassStatements)
	if tr.finalizes != 0 || tr.retrieves != 0 || tr.cancels != 0 {
		t.Fatal("empty recovery performed remote claim work")
	}
}

func firstSwitchApproved(t *testing.T, f *sqlFixture) Intent {
	t.Helper()
	if _, err := f.db.Exec(`INSERT INTO identity_state(singleton,sign_in_kind) VALUES(1,'local'); INSERT INTO identity_moves(id,owner_id,owner_revision,from_kind,to_kind,start_over,stage,revision,people,devices,profiles,server_name,created_ms,owner_confirmed) VALUES('move','local_owner',1,'local','portico',0,'preview',1,1,1,1,'Fixture',1,1)`); err != nil {
		t.Fatal(err)
	}
	ctx := fixtureClaimContext(t)
	v, err := f.store.PreparePurpose(ctx, f.binding, f.owner, ClaimPurpose{Purpose: "switch"})
	if err != nil {
		t.Fatal(err)
	}
	facts := ApprovalFacts{TargetUsername: "approved_owner", ClaimPurpose: v.ClaimPurpose, Binding: v.Binding, Revision: 1, IssuedAt: f.now, ExpiresAt: f.now.Add(5 * time.Minute)}
	raw, _ := json.Marshal(facts)
	proof, _ := json.Marshal(struct{ Payload, Signature []byte }{raw, ed25519.Sign(f.hostedKey, raw)})
	v, err = f.store.Approve(ctx, v, proof)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func firstSwitchRetrieving(t *testing.T, f *sqlFixture, v Intent) (Intent, Cancellation) {
	t.Helper()
	ctx := fixtureClaimContext(t)
	item, err := f.store.ArmPreinstallCancellation(ctx, v, f.cancel(t, v))
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.store.BeginFinalizing(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.store.Committed(ctx, v, Commit{OperationID: v.OperationID, ServerID: v.ServerID, ClaimGeneration: "7", CredentialGeneration: "2"})
	if err != nil {
		t.Fatal(err)
	}
	return v, item
}

func TestOwnerPreinstallCancellationSurvivesExpiredApprovalAndRestart(t *testing.T) {
	for _, failure := range []string{"expired", "revoked", "lost-finalize"} {
		t.Run(failure, func(t *testing.T) {
			f := fixtureCurrentSQL(t)
			ctx := fixtureClaimContext(t)
			v := firstSwitchApproved(t, f)
			v, item := firstSwitchRetrieving(t, f, v)
			if failure == "lost-finalize" {
				// The original FINALIZE may have committed centrally even when no
				// generation was received locally. The armed proof stays intent-mode.
				if _, err := f.db.Exec(`UPDATE networking_claim_intents SET stage='finalizing',claim_generation='',credential_generation='' WHERE operation_id=?`, v.OperationID); err != nil {
					t.Fatal(err)
				}
				var err error
				v, err = f.store.Load(ctx, v.OperationID)
				if err != nil {
					t.Fatal(err)
				}
			}
			f.now = f.now.Add(6 * time.Minute)
			if failure == "revoked" {
				if _, err := f.db.Exec(`UPDATE fixture_owners SET allowed=0`); err != nil {
					t.Fatal(err)
				}
			}
			if failure != "lost-finalize" {
				if _, err := f.store.Install(ctx, v, resultFor(t, v)); err == nil {
					t.Fatal("late install accepted expired or revoked initiating authority")
				}
			}
			if _, err := f.db.Exec(`UPDATE identity_moves SET owner_confirmed=0,stage='cancelled'`); err != nil {
				t.Fatal(err)
			}
			// The dormant proof grants no Local egress before cancellation wins.
			envelope := proofEnvelope{Payload: base64.RawURLEncoding.EncodeToString(item.Proof.Payload), Signature: base64.RawURLEncoding.EncodeToString(item.Proof.Signature)}
			if err := f.store.checkServiceRequest(ctx, "/v1/server-claims/cancel", envelope); !errors.Is(err, ErrLocalServices) {
				t.Fatal("armed proof became network authority before cancellation", err)
			}
			queued, saved, err := f.store.QueuePreinstallCancellation(ctx, v.OperationID)
			if err != nil || queued.Stage != CancelPending || !bytes.Equal(saved.Proof.Payload, item.Proof.Payload) || !bytes.Equal(saved.Proof.Signature, item.Proof.Signature) {
				t.Fatal("original deny-only cancellation not queued", err)
			}
			db := openFixtureDB(t, f.path)
			cipher, err := OpenClaimCipher(ctx, db, f.dir)
			if err != nil {
				t.Fatal(err)
			}
			restarted := f.storeFor(t, db, cipher)
			again, exact, err := restarted.QueuePreinstallCancellation(ctx, v.OperationID)
			if err != nil || again.Revision != queued.Revision || !bytes.Equal(exact.Proof.Payload, item.Proof.Payload) || !bytes.Equal(exact.Proof.Signature, item.Proof.Signature) {
				t.Fatal("restart changed pending cancellation", err)
			}
			if err = restarted.Current(ctx, v); err == nil {
				t.Fatal("cancelled generation retained positive authority")
			}
			tr, err := NewHTTPTransport("https://hosted.example", restarted)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tr.CloseIdleConnections)
			calls := 0
			tr.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path != "/v1/server-claims/cancel" || r.Header.Get("Authorization") != "" {
					t.Fatal("reconciliation sent another request or a credential")
				}
				raw, _ := io.ReadAll(r.Body)
				var got proofEnvelope
				if json.Unmarshal(raw, &got) != nil || got != envelope {
					t.Fatal("reconciliation changed exact saved proof")
				}
				if calls == 1 {
					return nil, errors.New("simulated lost cancellation receipt")
				}
				body, _ := json.Marshal(map[string]string{"operationId": v.OperationID, "requestId": item.RequestID})
				return reply(200, string(body)), nil
			})
			coordinator, err := NewCoordinator(v.OperationID, "https://hosted.example", restarted, tr, modelSigner{f.localKey})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = coordinator.ReconcilePreinstallCancellation(ctx); err == nil {
				t.Fatal("lost receipt reported success")
			}
			if done, err := coordinator.ReconcilePreinstallCancellation(ctx); err != nil || done.Stage != Cancelled || calls != 2 {
				t.Fatal("exact saved cancellation could not finish after lost receipt", err)
			}
			for _, path := range []string{"/v1/server-claims/cancel", "/v1/server-claims/nonce", "/v1/server-claims/finalize", "/v1/server-claims/result", "/v1/servers/checkin"} {
				if err := restarted.checkServiceRequest(ctx, path, envelope); !errors.Is(err, ErrLocalServices) {
					t.Fatal("retired outbox granted Local networking", path, err)
				}
			}
		})
	}
}

func TestOwnerPreinstallInstallWinRetiresCancellationAtomically(t *testing.T) {
	f := fixtureCurrentSQL(t)
	ctx := fixtureClaimContext(t)
	v, item := firstSwitchRetrieving(t, f, firstSwitchApproved(t, f))
	installed, err := f.store.Install(ctx, v, resultFor(t, v))
	if err != nil || installed.Stage != Installed {
		t.Fatal(err)
	}
	if _, _, err = f.store.QueuePreinstallCancellation(ctx, v.OperationID); err == nil {
		t.Fatal("pre-install recovery cancelled a locally installed claim")
	}
	if allowed, err := f.store.queuedPreinstallCancellation(ctx, item.Proof.Payload, item.Proof.Signature); err != nil || allowed {
		t.Fatal("installed claim retained cancellation exception", err)
	}
}

func TestOwnerPreinstallCapturedDeadlineRefusesLateInstall(t *testing.T) {
	f := fixtureCurrentSQL(t)
	ctx := fixtureClaimContext(t)
	v, item := firstSwitchRetrieving(t, f, firstSwitchApproved(t, f))
	deadline := f.now.Add(time.Minute)
	guarded, err := guardedClaimRequest(ctx, f.store, func(_ context.Context, _ *sql.Tx) error {
		if !f.now.Before(deadline) {
			return ErrStale
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Expire the captured initiating challenge before the approval itself.
	f.now = deadline
	if _, err = f.store.Install(guarded, v, resultFor(t, v)); err == nil {
		t.Fatal("local install accepted an expired captured challenge")
	}
	queued, exact, err := f.store.QueuePreinstallCancellation(ctx, v.OperationID)
	if err != nil || queued.Stage != CancelPending || !bytes.Equal(exact.Proof.Payload, item.Proof.Payload) {
		t.Fatal("expired captured guard stranded its saved cancellation", err)
	}
}

func TestOwnerPreinstallCancellationEnvelopeAndSuccessorFence(t *testing.T) {
	f := fixtureCurrentSQL(t)
	ctx := fixtureClaimContext(t)
	v, item := firstSwitchRetrieving(t, f, firstSwitchApproved(t, f))
	if _, _, err := f.store.QueuePreinstallCancellation(ctx, v.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE identity_moves SET owner_confirmed=0,stage='cancelled'`); err != nil {
		t.Fatal(err)
	}
	exact := proofEnvelope{Payload: base64.RawURLEncoding.EncodeToString(item.Proof.Payload), Signature: base64.RawURLEncoding.EncodeToString(item.Proof.Signature)}
	for _, bad := range []proofEnvelope{{exact.Payload + "\n", exact.Signature}, {exact.Payload, exact.Signature + "\r\n"}, {exact.Payload + "=", exact.Signature}, {exact.Payload, base64.RawURLEncoding.EncodeToString(make([]byte, 64))}} {
		if err := f.store.checkServiceRequest(ctx, "/v1/server-claims/cancel", bad); err == nil {
			t.Fatal("noncanonical or substituted cancellation envelope accepted")
		}
	}
	if _, err := f.db.Exec(`UPDATE networking_claim_identity SET reset_generation=reset_generation+1,active_operation_id=NULL`); err != nil {
		t.Fatal(err)
	}
	if err := f.store.checkServiceRequest(ctx, "/v1/server-claims/cancel", exact); !errors.Is(err, ErrLocalServices) {
		t.Fatal("predecessor outbox survived successor generation", err)
	}
}

func TestOwnerPreinstallCoordinatorArmsBeforeFinalizeAndWorkerReconciles(t *testing.T) {
	f := fixtureCurrentSQL(t)
	ctx := fixtureClaimContext(t)
	v := firstSwitchApproved(t, f)
	transport := &modelTransport{now: f.now}
	transport.finalizeHook = func() error {
		var count int
		if err := f.db.QueryRow(`SELECT count(*) FROM networking_claim_cancellations WHERE operation_id=? AND mode='intent' AND claim_generation='' AND state='pending'`, v.OperationID).Scan(&count); err != nil || count != 1 {
			t.Fatal("FINALIZE escaped before durable original-key cancellation", err)
		}
		// Central FINALIZE succeeded, but local Committed/Install can no longer
		// cross the captured deadline. Recovery must not need a new signature.
		f.now = f.now.Add(6 * time.Minute)
		return nil
	}
	coordinator, err := NewCoordinator(v.OperationID, "https://hosted.example", f.store, transport, modelSigner{f.localKey})
	if err != nil {
		t.Fatal(err)
	}
	coordinator.now = func() time.Time { return f.now }
	if _, err = coordinator.Step(ctx); !errors.Is(err, ErrApprovalRequired) || transport.finalizes != 1 {
		t.Fatal("late FINALIZE response crossed the local deadline", err)
	}
	_, runner := lifecycleRunner(t)
	tr, err := NewHTTPTransport("https://hosted.example", f.store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.CloseIdleConnections)
	calls := 0
	tr.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v1/server-claims/cancel" {
			t.Fatal("recovery performed positive claim work", r.URL.Path)
		}
		var envelope proofEnvelope
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			t.Fatal("missing stored cancellation envelope")
		}
		raw, _ := base64.RawURLEncoding.DecodeString(envelope.Payload)
		var wire cancelWire
		if json.Unmarshal(raw, &wire) != nil || wire.Mode != "intent" || wire.ClaimGeneration != "" {
			t.Fatal("recovery fabricated a central generation")
		}
		body, _ := json.Marshal(map[string]string{"operationId": v.OperationID, "requestId": wire.RequestID})
		return reply(200, string(body)), nil
	})
	h, err := NewClaimHandler(f.store, runner, tr, modelSigner{f.localKey}, func(*http.Request) (ClaimRequestOwner, error) { return ClaimRequestOwner{}, ErrClaimOwnerRequired }, func() string { return "Fixture" })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE identity_moves SET owner_confirmed=0,stage='cancelled'`); err != nil {
		t.Fatal(err)
	}
	if err = h.reconcileWebClaim(context.Background(), v.OperationID, nil, false); err != nil || calls != 1 {
		t.Fatal("fresh lifecycle recovery could not deliver its stored denial", err)
	}
	loaded, err := f.store.Load(ctx, v.OperationID)
	if err != nil || loaded.Stage != Cancelled {
		t.Fatal("worker did not finish the durable cancellation", err)
	}
}
