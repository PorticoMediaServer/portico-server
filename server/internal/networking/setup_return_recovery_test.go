package networking

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/serverreturn"
)

// The recovery trace uses real installed credentials, TLS/certificate validation,
// route fences and resolver transport. Capture/identity callbacks are controlled
// fixtures; signed capture and atomic owner admission have separate identity tests.
func TestSetupReturnRecoveryUsesOnlyCurrentUnexpiredInstalledCapture(t *testing.T) {
	for _, mode := range []string{"current", "expired", "not acknowledged", "wrong note operation", "wrong note generation", "guard refused", "manual route", "route changes during resolver", "already bound"} {
		t.Run(mode, func(t *testing.T) {
			f, h, ctx, _ := managedHTTPSFixture(t)
			if _, err := f.db.Exec(`UPDATE networking_claim_intents SET purpose='setup',current_owner_account_id='' WHERE stage='installed'`); err != nil {
				t.Fatal(err)
			}
			v, err := f.store.InstalledIntent(ctx)
			if err != nil {
				t.Fatal(err)
			}
			note := webClaimResume{Expected: ClaimExpected{OperationID: v.OperationID, LocalGeneration: v.LocalGeneration}, Owner: "captured-setup-owner"}
			if mode == "wrong note operation" {
				note.Expected.OperationID = "different-operation"
			}
			if mode == "wrong note generation" {
				note.Expected.LocalGeneration++
			}
			raw, _ := json.Marshal(note)
			now := time.Now().UnixMilli()
			expiry := now + 60000
			if mode == "expired" {
				expiry = now - 1
			}
			if _, err = f.db.Exec(`INSERT INTO networking_web_claims(operation_id,request,watch) VALUES(?,'{}',?)`, v.OperationID, string(raw)); err != nil {
				t.Fatal(err)
			}
			if _, err = f.db.Exec(`INSERT INTO identity_setup_completions(operation_id,setup_request_id,setup_revision,setup_proof,phase,claim_hash,claim_payload,claim_signature,start_payload,start_signature,state_hash,code_challenge,installation_id,server_challenge_hash,created_ms,expires_ms) VALUES(?,'setup-request',1,'captured-proof','prepared',?,X'01',zeroblob(64),X'01',zeroblob(64),'state',?,'installation','challenge',?,?)`, v.OperationID, strings.Repeat("h", 43), strings.Repeat("c", 43), now-60000, expiry); err != nil {
				t.Fatal(err)
			}
			if mode == "not acknowledged" {
				if _, err = f.db.Exec(`UPDATE networking_claim_intents SET installation_acknowledged=0`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "manual route" {
				if _, err = f.db.Exec(`UPDATE networking_remote_state SET status='{}'`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "already bound" {
				if _, err = f.db.Exec(`UPDATE identity_setup_completions SET phase='installed',registration_id='existing-binding'`); err != nil {
					t.Fatal(err)
				}
			}
			read, err := f.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			want, _, routeErr := h.setupReturnRouteTx(ctx, read, v)
			read.Rollback()
			if routeErr != nil && mode != "manual route" && mode != "not acknowledged" {
				t.Fatal(routeErr)
			}
			resolves, binds, calls := 0, 0, 0
			h.transport.(*HTTPTransport).client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
				resolves++
				if r.URL.Path != "/v1/servers/"+v.ServerID+"/return-registration" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					t.Fatal("not an installed resolver request")
				}
				if mode == "route changes during resolver" {
					if _, err = f.db.Exec(`UPDATE networking_remote_state SET generation=generation+1`); err != nil {
						t.Fatal(err)
					}
				}
				raw, _ := json.Marshal(want)
				return reply(200, string(raw)), nil
			})
			guard := func(context.Context, *sql.Tx) error {
				if mode == "guard refused" {
					return ErrClaimOwnerRequired
				}
				return nil
			}
			h.UseSetupReturnResolver(func(ctx context.Context, owner, operation string) error {
				calls++
				if owner != note.Owner || operation != v.OperationID {
					t.Fatal("capture substituted")
				}
				_, _, err := h.BindSetupReturn(ctx, guard, func(ctx context.Context, tx *sql.Tx, r serverreturn.Registration, generation int64) (bool, error) {
					binds++
					if r != want {
						t.Fatal("resolver registration substituted")
					}
					_, err := tx.ExecContext(ctx, `UPDATE identity_setup_completions SET phase='installed',registration_id=?,registration_origin=?,route_generation=? WHERE operation_id=?`, r.ID, r.Origin, generation, operation)
					return true, err
				})
				return err
			})
			h.resumeSetupReturnBinding(context.Background(), func(owner string) ClaimRequestGuard {
				if owner != note.Owner {
					t.Fatal("owner substituted")
				}
				return guard
			})
			if mode == "current" {
				if calls != 1 || resolves != 1 || binds != 1 {
					t.Fatal("current capture not bound", calls, resolves, binds)
				}
				h.resumeSetupReturnBinding(context.Background(), func(string) ClaimRequestGuard { return guard })
				if calls != 1 {
					t.Fatal("bound capture retried")
				}
			} else if binds != 0 {
				t.Fatal("refused capture bound", mode)
			}
			if mode != "current" && mode != "manual route" && mode != "route changes during resolver" && (calls != 0 || resolves != 0) {
				t.Fatal("retired capture reached resolver", mode, calls, resolves)
			}
			var after int64
			if err = f.db.QueryRow(`SELECT expires_ms FROM identity_setup_completions`).Scan(&after); err != nil || after != expiry {
				t.Fatal("expiry changed", err)
			}
		})
	}
}
