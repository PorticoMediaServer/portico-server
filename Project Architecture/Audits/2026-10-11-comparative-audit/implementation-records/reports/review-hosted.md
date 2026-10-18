# Review of the Hosted lane (independent second pass)

Reviewed: the three most recent commits touching `hosted-services/` on `audit-implementation/review-hosted` (`63405a7`, `b12b9ab`, `182e09f`), against the lane brief and the author's report (`reports/hosted.md`).
Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/internal-review-hosted`. Paths below are relative to `hosted-services/hosted/` unless they start with `deployment/`.

Method: read every non-generated changed file and the code it calls (session/refresh store, family authority check, trust package, wake queue, claims, key loading, proxy configuration, and the local server's challenge and assertion verification in `main-server`); probed the return-address validator with a throw-away table of hostile inputs; ran the author's selected PostgreSQL and unit tests on an owned, socket-only PostgreSQL 18 cluster as a baseline (all pass before any change).

Severity counts: 1 blocker, 0 high, 3 medium, 6 low.

## Findings

### F1. Sign in with Portico refuses every real server's challenge — blocker
- Where: `internal/appauth/protocol.go:64` (`Request.Validate`: `base64Bytes(q.Challenge, 32)`).
- What is wrong: the challenge an app must pass is the one the local server issues at `POST /v1/direct/portico-challenge`. That is 24 random bytes, 32 base64url characters (`main-server/server/internal/hosted/identity_assertion.go:65`; identical in every server worktree of this programme). Hosted insists on exactly 32 bytes (43 characters). With a real challenge the browser navigation to `/auth/apps/authorize` is answered `400 query_not_supported`, and `POST /v1/app-authorizations` `400 invalid_grant`. The flow cannot start against any Portico server.
- How I know: `Validate` → `base64Bytes(v, 32)` requires `len(raw) == 32`; the server produces `make([]byte, 24)`. Every test in the lane builds its challenge as `base64(make([]byte, 32))`, which is why nothing failed. Hosted's own signer (`directory.IssueIdentity`) and the server's verifier both accept 22–128 characters.
- Smallest sound fix: accept a canonical unpadded base64url challenge of 16–96 bytes (22–128 characters), the same bound the assertion issuer and the server's verifier already apply. Test with a 24-byte challenge.

### F2. An expired or fenced signing certificate now refuses every server check-in — medium
- Where: `internal/directory/server_authority.go:65-67` (`ServerHeartbeat`).
- What is wrong: the check-in was made to fail with the signer (`ValidateSigner` before the repository call) only so that it could attach the certificate. A check-in is not a signed document. Before this change a missed yearly renewal meant "servers reject Hosted's signed updates and keep their existing access" (`deployment/hosted/KEYS.md`); now every server's check-in gets 503, presence lapses for the whole fleet, wake acknowledgements stop being recorded, and every server goes into outage back-off until the operator renews.
- How I know: `ServerHeartbeat` returns `trust.ErrTrust` when the certificate is past `NotAfter`; `failure()` maps it to `503 unavailable`. Nothing in the repository call needs the signer.
- Smallest sound fix: always record the check-in; attach `signingCertificate` only while the signer is valid (the contract already declares it nullable).

### F3. Consent can be given by a repeated key press or click — medium
- Where: `internal/httpapi/appauthassets/authorize.js:30` (`if(reviewed)document.getElementById('approve').focus()`), with the buttons re-enabled in the same tick.
- What is wrong: the person presses Continue (which is described as continuing after sign-in; nothing about the app has been shown yet). The moment the review answer arrives the script enables the buttons and moves keyboard focus onto "Allow sign-in". A held Enter (key repeat fires `click` on the focused button), a second Enter or Space, or the second half of a double-click that lands on the newly drawn button approves the request before the app name, server and return address have been seen. With no app registration, that consent screen is the only thing between a lure link and a sign-in on the person's server.
- How I know: reasoned from the script: `work()` disables all buttons during the request, then `finally` re-enables them and focuses `#approve`; a `keydown` repeat on a focused, enabled `<button>` dispatches `click`.
- Smallest sound fix: when the consent appears, put focus on the consent text (so it is also what a screen reader reads) and keep Allow and Cancel disabled for a short moment. Test by running the page script against a small stand-in document.

### F4. Anonymous callers can make Hosted scan the whole session table — medium
- Where: `internal/postgres/app_authorizations.go:52` (`SELECT … FROM session_families WHERE current_refresh_hash=$1 AND mode='browser'`).
- What is wrong: `session_families.current_refresh_hash` has no index (the ordinary refresh path finds a family through `refresh_tokens`, whose primary key is the token hash). The review and decision routes run this query for whatever cookie value the caller sends, before anything is authenticated; a non-browser client simply sets `Origin`, the cookie and a matching CSRF header itself. Each such request is a sequential scan of every session family (one per sign-in, kept 90 days). The only bound is the 60-a-minute address budget, so the cost grows with the size of the service and is available to anyone.
- How I know: schema in `internal/postgres/store.go:124` and its indexes (`session_families_account`, `session_families_expiry` only); `EXPLAIN` with sequential scans discouraged still shows `Seq Scan on session_families` (the regression test asserts this).
- Smallest sound fix: find the family through `refresh_tokens` (primary-key lookup) and keep the existing comparison with the family's current hash under lock.

### F5. An app name can take over the server's place in the consent sentence, and nothing separates app-supplied words from Portico's — low
- Where: `internal/httpapi/appauthassets/authorize.js:34` (`strings.consent.replace('{appName}',…).replace('{serverName}',…)`).
- What is wrong: the second replacement acts on the text the first one inserted. An app called `Anything {serverName}` is shown as "Allow Anything <real server name> (an unverified app name) to sign you in to {serverName}?". More generally the app name (up to 100 characters of free text) and the owner-chosen server name run straight into Portico's own words, so a name such as `your TV. Portico checked this app. Ignore:` reads as part of the page.
- How I know: `String.prototype.replace` with a string pattern replaces the first occurrence in the current string, which after the first call is inside the app name.
- Smallest sound fix: build the sentence from the catalogue string in one pass with each name in its own emphasised element, and say in one more plain sentence what allowing does.

### F6. The person is not sent to the address they were shown — low
- Where: `internal/appauth/protocol.go:163-173` (`ReturnLocation`).
- What is wrong: the return address is parsed and re-serialised: existing query parameters are re-sorted with `code` and `state` mixed in among them, `?flag` becomes `?flag=`, escapes are rewritten, and the scheme is lower-cased. The consent page shows, and the exchange later requires, the original string. The author's report says the return is "the exact destination". An app whose handler is order- or form-sensitive receives something it did not register.
- How I know: probe: `https://example.com/finish?a` → `…/finish?a=&code=…&state=…`; `itms-services:///?action=x&url=y` → `…?action=x&code=…&state=…&url=y`.
- Smallest sound fix: append `code`/`error` and `state` to the stored string unchanged (the validator already guarantees no fragment and no dangling `?`).

### F7. App-scheme return addresses can launch operating-system handlers from Portico's page — low
- Where: `internal/appauth/protocol.go:146-150` (the short refusal list).
- What is wrong: any hierarchical scheme not on a twelve-name list is accepted, and the consent page navigates to it after either button, Cancel included. Accepted today: `ms-msdt:/…`, `search-ms://…`, `ms-officecmd://…`, `itms-services:///?action=download-manifest&url=…`, `vbscript://…`, `view-source://…`, `chrome://…`, `ssh://…`, `smb://…`. Several of these have a history of doing harm when launched from a trusted-looking page; here the prompt would name `web.getportico.tv`.
- How I know: probe of `ValidRedirect` (all of the above return true).
- Smallest sound fix: refuse the known browser-internal, operating-system and network handler schemes (and the `ms-` family) while keeping ordinary app schemes such as `my-player://` working. A refusal list can never be complete; this closes the known bad ones without requiring app registration, which the owner ruled out.

### F8. Claim clean-up answers are still signed without the signer check — low
- Where: `internal/claims/cleanup.go:94`.
- What is wrong: the author's report says expiry, revocation and the process fence "are checked at every signing operation". The clean-up result (which tells a server it is retired and hands it account-erasure jobs) is still signed with the raw key, so an expired or fenced process keeps issuing it, and does so after the repository call has already handed out the jobs.
- How I know: `grep` for direct `ed25519.Sign` uses outside `trust`: this is the one left.
- Smallest sound fix: check the signer before the repository call and again before signing.

### F9. A process that can no longer sign still takes wake jobs and pushes them back — low
- Where: `internal/wake/worker.go:147-151`, with `Run`/`Once` claiming unconditionally.
- What is wrong: after its key is revoked (or its certificate expires) the worker keeps leasing wake jobs, fails to sign, and reschedules each with growing delay and a spent attempt. During a key change, which is exactly when the new process is trying to wake every server, the old process takes a share of the queue and delays it; a process left running a day marks the servers unreachable.
- How I know: `Claim` increments `attempts` and sets the lease; the new signing-failure branch calls `Finish` with `RetryDelay`; `Finish` sets `wake_unreachable` after 24 hours of attempts.
- Smallest sound fix: do not claim work while the signer is invalid.

### F10. Tests that do not prove what their names say — low
- `TestAppExchangeCORSIsCredentiallessAndIsolated` (`internal/httpapi/app_authorizations_test.go:46`) only sends a preflight. Nothing that runs without PostgreSQL proves the exchange refuses a cookie, and nothing at all proves it refuses a bearer token.
- `TestSigningRolloutCheckpointsOnceAndRejectsRollback` never presents a different certificate at the same revision, which the report lists as rejected.
- Every app-authorization test uses a 32-byte challenge (see F1).
- Fix: add the missing cases (these pass on the code as written; they close the gap rather than expose a defect).

## Checked and found sound
- Code: 256 random bits, stored only as a hash, unique, 60 seconds, spent under a row lock in the same transaction that signs; twelve concurrent exchanges give one winner; a failed signature rolls the spend back. Bound to verifier (S256 only, constant-time compare), exact return address, account, account epoch, server, and the browser session family that approved; logout, password change, family expiry or server revocation between approval and exchange refuses it.
- Mix-up: no client identity exists to confuse; a code stolen by another app registered on the same scheme is useless without the verifier; an assertion names one server id and one challenge, which the server spends once and ties to the asking installation.
- The exchange returns one identity assertion (never custody), no cookie, no session, no directory data. The wildcard-CORS branch applies to exactly `POST`/`OPTIONS /v1/app-authorizations/token`; it sends no `Allow-Credentials`, the handler reads no credential, refuses a bearer or refresh cookie, and unclean paths fall back to the ordinary origin check.
- Review and decision: exact canonical `Origin`, the `__Host-` refresh cookie, double-submit CSRF, JSON only; other allowed origins and bearer tokens cannot approve; a decision must match the reviewing family and account; denial is final; server rename forces a new review.
- Page: output is escaped by `html/template`, script and style are external, CSP `default-src 'none'` with `frame-ancestors 'none'` and `X-Frame-Options: DENY`, no referrer, no-store, the request leaves the address bar after load, all dynamic text is set with `textContent`.
- Return addresses: user-info, fragments, back-slashes, spaces, non-ASCII (so IDN must be shown as punycode), percent-encoded hosts, trailing dots, zone ids, mapped IPv4, short or octal loopback forms, `localhost`, missing or padded ports, and re-encoded reserved parameter names are all refused.
- Limits: 60 a minute per address and 600 per network across the page and all three routes, ten reviews a minute per account counted in the database, rows pruned in bounded batches. The access log records route templates only.
- Lock order: account, then family, then the authorization row, the same as refresh, logout and deletion. Device poll and cancel now take the account before the device row; the approval rechecks the approving session inside its transaction.
- Signing-key state: lower revision refused, a different certificate at the same revision refused, a certificate that drops an earlier revocation refused, self-revoked certificates refused by `trust`; the sweep is checkpointed 128 servers at a time and a superseded process stops sweeping. With the database unavailable the process refuses to start (fail closed); a running process keeps its last knowledge and is fenced within 30 seconds of the database answering.
- Baseline: revision 20 is created only on an empty schema in one transaction; anything else is refused without writes.

## Notes for the owner (not defects; nothing changed)
- **Deploying this build needs an empty Hosted database.** Revision 19 and earlier are refused and the upgrade path was deleted, as the brief allows ("no data or compatibility needs preserving"). If the existing Hosted database has anything worth keeping, this is the moment to say so.
- **The design's remaining risk is persuasion, not protocol.** Any site may ask, and an `https` return address may be anyone's. A person who presses Allow gives that app their own access to that server. The page states the app name is unverified and shows the full return address. Cancel also sends the browser to the stated return address (with `error=access_denied`), which is normal for this protocol and carries no code.
- The consent page shows the account's display name only, which is not unique. Showing the username as well would need a contract field.
- Any signed-in account that knows a server's id learns that server's name from the review. Ids are 256-bit and the local server already gives its name to anyone who can reach it.
- `verifyConsent` consults the running certificate's revocation list, not the database's; a not-yet-restarted replica accepts consent signed by a key revoked in a newer certificate until it restarts. One process in production today.
- A restore drill skips both the revision record and the revocation fence, by design of that flag.
- In `DecideDevice` a device code that expires between the two clock samples is reported as invalid credentials rather than expired. Microsecond window; left.
- **The copied toolkit is behind again.** `hosted-services/apikit` matched `main-server/apikit` when the author checked; the server side gained per-route scopes afterwards (merged 03:01 on 13 October). Five files now differ. Bringing it level means declaring a scope on every Hosted route and regenerating the contract; that belongs with the API lane's change, not with this review.
- Q-6 (TV activation and the shared outage gate): Hosted's part is the 30-second cap on its own slow-down; the client wiring is not in this repository and is listed in the author's report as needed from others. That is accurate.

## Fixes

All ten findings are fixed on `audit-implementation/review-hosted`, one commit each, most severe first. Each has a repository test that failed on the reviewed code and passes now (F10 is added coverage; its cases already passed). Nothing was deployed, pushed, or run against any existing service.

| Finding | Commit | What changed | Test |
|---|---|---|---|
| F1 blocker | `66ad97f` Accept the sign-in challenge a Portico server actually issues | `appauth.Request.Validate` takes a canonical base64url challenge of 16–96 bytes (22–128 characters), the bound the issuer and the server's verifier already use. | `TestServerIssuedChallengeSizes`; the HTTP and PostgreSQL app-authorization tests now use a 24-byte challenge. |
| F2 medium | `8ee76d3` Keep server check-ins working when the signing certificate lapses | `ServerHeartbeat` always records the check-in; `signingCertificate` is null while the signer is expired or revoked. | `TestCheckInSurvivesAnUnusableSigner` |
| F3 medium | `ff1601b` Stop a repeated key press or click from approving an app sign-in | Focus goes to the consent sentence; Allow and Cancel stay disabled for one second after the consent is drawn, and the handler also ignores a decision inside that second. | `TestConsentIsNotGivenByARepeatedPress` (runs the real page script under Node against a stand-in document; on the old script the repeated press issued the decision) |
| F4 medium | `267eb48` Find the browser session for app sign-in without scanning every session | The cookie is resolved through `refresh_tokens` (primary key) joined to the family, still requiring it to be the family's current token; the locked re-check is unchanged. | `TestAppAuthorizationSessionLookupDoesNotScanEverySession` (4,000 sessions; the old query examined 4,003 rows, the new one at most a handful) |
| F5 low | `167a3b9` Show app and server names as names on the consent page | The sentence is built in one pass with each name in its own `<strong>`, shown in quotation marks; a second line states what allowing does. | `TestConsentSentenceKeepsNamesApart` |
| F6 low | `b58b9e6` Send people to the return address exactly as it was shown | `ReturnLocation` appends `code`/`error` and `state` to the stored address without rewriting it. | `TestReturnKeepsTheAddressAsShown` |
| F7 low | `aa33a86` Refuse return addresses that launch system and browser handlers | `handlerScheme` refuses browser-internal, operating-system, network/file and call schemes, plus the `ms-` and `itms` families. | `TestReturnRefusesHandlerSchemes` |
| F8 low | `f06e789` Check the signing certificate before answering a claim clean-up | Signer checked before the repository call and again before signing. | `TestCleanupIsNotSignedByAnUnusableSigner` |
| F9 low | `d9778ce` Leave wake work alone while this process cannot sign it | `Run` and `Once` claim nothing while the signer is unusable and wait the safety interval instead of spinning on due work; the release-on-failure path in `deliver` stays for a signer that lapses mid-job. | `TestUnusableSignerTakesNoWakeWork`; `TestExpiredSignerDoesNotDeliverAndReleasesWakeLease` now drives `deliver` directly |
| F10 low | `58191de` Cover the exchange's refusal of credentials and a conflicting certificate | Tests only. | `TestAppExchangeRefusesEveryAmbientCredential`; added cases in `TestSigningRolloutCheckpointsOnceAndRejectsRollback` |

### Decisions I made
- Challenge size is the server's to choose; Hosted bounds it (16–96 bytes) rather than pinning it.
- A failed signer never refuses a check-in. It does still refuse every signed document (identity assertions, routes, erasures, claim approvals and clean-up, wakes), and readiness still fails.
- One second before the consent buttons accept anything. Long enough to defeat key repeat and a double-click, short enough not to be noticed by someone reading.
- New catalogue string `meaning` in `packages/i18n/app-sign-in.en.json`: "The app will be able to use this server as you. Allow this only if you started the sign-in yourself." It is the one place the page says what is being granted. Reword freely.
- App schemes stay open (no reverse-domain requirement), because the owner wants any app to be able to use this without registration. The refusal list is therefore best effort by nature.
- No API shape, route, schema or generated contract changed (`go run ./cmd/apigen` leaves the tree clean).

### What I ran
- `GOFLAGS=-p=2 go build ./...` and `go vet` on every touched package: clean.
- Unit packages in full: `internal/appauth`, `internal/httpapi`, `internal/security`, `internal/directory`, `internal/wake`, `internal/claims`: pass.
- PostgreSQL-backed: the whole of `internal/postgres` and `internal/contract`, plus `internal/httpapi`, on an owned temporary PostgreSQL 18.6 cluster (the binaries the author extracted under `/tmp/portico-hosted-postgres`, data directory in my scratch space, Unix socket only under `/run/user/1000`, no TCP listener, removed afterwards): pass (`internal/postgres` 263 s). No database test was skipped.
- The two page-script tests need `node` on the path and skip without it (Node 24 here; GitHub's runners have it).

### Not done
- **The consent page was not looked at in a real browser.** The script is tested under Node with a stand-in document, and the handler tests cover headers and escaping. The quotation marks around names, the focus ring on the sentence and the one-second delay have not been seen on a screen, and no real cookie, provider sign-in or app-scheme hand-off was exercised. The author's browser acceptance note in `reports/hosted.md` still stands; add to it: press and hold Enter on Continue and confirm nothing is approved; try an app named `X {serverName}`; confirm the return lands on the address exactly as typed.
- **`hosted-services/apikit` is behind `main-server/apikit`** (five files) because the server toolkit changed after the Hosted work was snapshotted. Not touched here; see the note above. `scripts/shared-apikit.sh` and CI will report it until the copy is refreshed and Hosted's routes declare scopes.
- The items under "Notes for the owner" are unchanged by design.
- Still outside this repository, as the author's report says: the client side of Q-6, the local server reading `signingCertificate` from check-ins (it must now also accept null), and retiring local sessions admitted under a revoked signer.

### Correction to the author's report
- `reports/hosted.md`, outside-developer flow, step 2: the challenge returned by the local server is 24 bytes (32 characters), and is passed through unchanged. The 32-byte value in that step is the app's own nonce. With F1 fixed the described flow is otherwise accurate; step 5's "exact destination" is now literally true.
- The statements that the signer is checked "at every signing operation" and that the copied toolkit "equals" the server's are true as of these fixes for the first, and no longer true for the second.
