package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

func signInReturnFixture(t *testing.T, f returnFixture) returnFixture {
	t.Helper()
	f.start.Purpose = "sign-in"
	f.accountToken = "sign-in-return-other-account-access-fixture"
	if _, e := f.store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,'strict-other','strict-other-family',clock_timestamp()+interval '15 minutes')`, security.Hash(f.accountToken)); e != nil {
		t.Fatal(e)
	}
	return f
}

func TestRegisteredReturnSignInNeedsNoMembershipAndCannotClaim(t *testing.T) {
	f := signInReturnFixture(t, registeredReturnFixture(t))
	if _, e := f.store.DB.Exec(`INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,is_primary) VALUES('return-contact','strict-other','return@example.test',clock_timestamp(),'challenge',true)`); e != nil {
		t.Fatal(e)
	}
	started := f.begin(t)
	returnCall(t, f.handler, "/v1/server-returns/"+started.TransactionID+"/complete", "claim-owner-access-token-fixture", struct{}{}, 401)
	q := f.complete(t, started.TransactionID)
	for _, change := range []func(*serverreturn.Exchange){
		func(q *serverreturn.Exchange) { q.Purpose = "setup" },
		func(q *serverreturn.Exchange) { q.InstallationID = "another-installation-0000000000000000" },
		func(q *serverreturn.Exchange) { q.ServerChallenge = "another-challenge-000000000000000000" },
		func(q *serverreturn.Exchange) { q.RegistrationID = "ret_other" },
		func(q *serverreturn.Exchange) { q.Verifier = "another-verifier-0000000000000000000" },
	} {
		bad := q
		change(&bad)
		returnCall(t, f.handler, "/v1/server-returns/exchange", "", bad, 401)
	}
	raw := returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 200)
	var grant serverreturn.Grant
	if e := json.Unmarshal(raw, &grant); e != nil {
		t.Fatal(e)
	}
	payload, e := grant.Assertion.Verify(f.keys.RootPublicKey, "documents", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var assertion serverreturn.Assertion
	if e = json.Unmarshal(payload, &assertion); e != nil {
		t.Fatal(e)
	}
	canonical, _ := json.Marshal(assertion)
	if !bytes.Equal(payload, canonical) || assertion.AccountID != "strict-other" || assertion.Username != "strict-other" || assertion.DisplayName != "Fixture" || assertion.Email != "return@example.test" || assertion.Purpose != "sign-in" || assertion.CurrentOwnerAccountID != "" || assertion.PreviousOwnerRole != "" || assertion.ClaimOperationID != f.input.OperationID || assertion.ClaimGeneration != 1 || assertion.CredentialGeneration != 1 || assertion.Challenge != q.ServerChallenge || assertion.InstallationID != q.InstallationID {
		t.Fatalf("identity/immutable tuple: %s", payload)
	}
	returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 401)
	var members, operations int
	var owner string
	if e = f.store.DB.QueryRow(`SELECT (SELECT count(*) FROM server_members WHERE account_id='strict-other'),(SELECT count(*) FROM claim_operations),owner_account_id FROM servers WHERE id=$1`, f.input.ServerID).Scan(&members, &operations, &owner); e != nil || members != 0 || operations != 1 || owner != "strict-owner" {
		t.Fatal("sign-in changed membership/claim", members, operations, owner, e)
	}
	in := f.input
	in.Purpose = "sign-in"
	if in.Valid() || serverreturn.Binding("sign-in", "", "") || serverreturn.Binding("sign-in", "strict-owner", "admin") || serverreturn.ReturnBinding("sign-in", "strict-owner", "") {
		t.Fatal("sign-in widened owner claim authority")
	}
	// The real returned envelope is forwarded unchanged by the admitted server.
	// Identity issuance itself made no index row; only this authenticated push
	// may project Server membership into Hosted discovery.
	sequence := int64(0)
	push := func(consent json.RawMessage, skipped int) {
		t.Helper()
		sequence++
		q := directory.MemberPush{Through: sequence, Full: true, Members: []directory.MemberStanding{{AccountID: "strict-owner", Role: "owner", Active: true}, {AccountID: "strict-other", Role: "member", Active: true, Consent: consent}}}
		raw := returnCall(t, f.handler, "/v1/servers/"+f.input.ServerID+"/members", f.serverToken, q, 200)
		var out directory.MemberPushResult
		if e := json.Unmarshal(raw, &out); e != nil || len(out.Skipped) != skipped {
			t.Fatal("return consent projection", string(raw), e)
		}
	}
	for _, change := range []func(*serverreturn.Assertion){
		func(a *serverreturn.Assertion) { a.AccountID = "strict-owner" },
		func(a *serverreturn.Assertion) { a.ServerID = "srv_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBA" },
		func(a *serverreturn.Assertion) { a.Purpose = "custody" },
		func(a *serverreturn.Assertion) { a.CurrentOwnerAccountID = "strict-owner" },
		func(a *serverreturn.Assertion) { a.PreviousOwnerRole = "admin" },
		func(a *serverreturn.Assertion) { a.ClaimGeneration = 0 },
		func(a *serverreturn.Assertion) { a.ExpiresAt = a.IssuedAt },
		func(a *serverreturn.Assertion) { a.ExpiresAt = a.IssuedAt.Add(2 * time.Minute) },
		func(a *serverreturn.Assertion) {
			a.IssuedAt = time.Now().Add(6 * time.Minute)
			a.ExpiresAt = a.IssuedAt.Add(time.Minute)
		},
	} {
		bad := assertion
		change(&bad)
		payload, _ := json.Marshal(bad)
		signed, e := f.keys.Sign(payload)
		if e != nil {
			t.Fatal(e)
		}
		consent, _ := json.Marshal(signed)
		push(consent, 1)
	}
	// Even genuinely signed noncanonical payloads cannot widen the wire.
	for _, payload := range [][]byte{append([]byte(" "), canonical...), append(append([]byte(nil), canonical[:len(canonical)-1]...), []byte(`,"unknown":true}`)...)} {
		signed, e := f.keys.Sign(payload)
		if e != nil {
			t.Fatal(e)
		}
		consent, _ := json.Marshal(signed)
		push(consent, 1)
	}
	consent, _ := json.Marshal(grant.Assertion)
	push(consent, 0)
	listed, e := f.directory.List(context.Background(), "strict-other")
	if e != nil || len(listed) != 1 || listed[0].ID != f.input.ServerID || listed[0].Role != "member" {
		t.Fatal("signed return did not reach role index", listed, e)
	}
	// A later departure cannot be undone with old signed return bytes.
	if e = f.directory.LeaveServer(context.Background(), "strict-other", f.input.ServerID); e != nil {
		t.Fatal(e)
	}
	push(consent, 1)
}

func TestRegisteredReturnConsentRechecksCurrentSignerRevocation(t *testing.T) {
	f := signInReturnFixture(t, registeredReturnFixture(t))
	started := f.begin(t)
	q := f.complete(t, started.TransactionID)
	raw := returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 200)
	var grant serverreturn.Grant
	if e := json.Unmarshal(raw, &grant); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.Exec(`INSERT INTO hosted_signing_revocations(key_id) VALUES($1)`, f.keys.KeyID); e != nil {
		t.Fatal(e)
	}
	consent, _ := json.Marshal(grant.Assertion)
	p := directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: "strict-owner", Role: "owner", Active: true}, {AccountID: "strict-other", Role: "member", Active: true, Consent: consent}}}
	raw = returnCall(t, f.handler, "/v1/servers/"+f.input.ServerID+"/members", f.serverToken, p, 200)
	var out directory.MemberPushResult
	if e := json.Unmarshal(raw, &out); e != nil || len(out.Skipped) != 1 {
		t.Fatal("revoked return consent indexed", string(raw), e)
	}
	listed, e := f.directory.List(context.Background(), "strict-other")
	if e != nil || len(listed) != 0 {
		t.Fatal("revoked signer leaked into discovery", listed, e)
	}
}

func TestRegisteredReturnSignInRechecksInstalledAuthority(t *testing.T) {
	for name, mutation := range map[string]string{
		"key":        `UPDATE servers SET public_key=translate(rtrim(encode(decode(repeat('01',32),'hex'),'base64'),'='),'+/','-_')`,
		"claim":      `UPDATE servers SET claim_generation=claim_generation+1`,
		"credential": `UPDATE servers SET credential_generation=credential_generation+1`,
		"route":      `UPDATE networking_route_state SET generation=generation+1`,
		"origin":     `UPDATE networking_route_candidates SET origin='https://other.example:32500'`,
		"family":     `UPDATE session_families SET revoked_at=clock_timestamp() WHERE id='strict-other-family'`,
	} {
		t.Run(name, func(t *testing.T) {
			f := signInReturnFixture(t, registeredReturnFixture(t))
			started := f.begin(t)
			q := f.complete(t, started.TransactionID)
			if _, e := f.store.DB.Exec(mutation); e != nil {
				t.Fatal(e)
			}
			returnCall(t, f.handler, "/v1/server-returns/exchange", "", q, 401)
			var consumed bool
			if e := f.store.DB.QueryRowContext(context.Background(), `SELECT consumed_at IS NOT NULL FROM server_returns WHERE id=$1`, started.TransactionID).Scan(&consumed); e != nil || consumed {
				t.Fatal("invalid return consumed", consumed, e)
			}
		})
	}
}
