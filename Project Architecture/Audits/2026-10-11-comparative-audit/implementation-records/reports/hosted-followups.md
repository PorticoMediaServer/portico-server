# Outcome

Hosted's copied apikit is byte-for-byte level with the named main-server/apikit, including mandatory credential scopes and revision checks. All 99 Hosted JSON routes explicitly declare a scope; OpenAPI and TypeScript route metadata are regenerated. The independent review's ten fixes remain closed under current source inspection and selected regression tests.

Hosted now delivers the newest durably recorded root-signed documents certificate on existing check-ins, including when an older replica's own signer has been fenced. An older replica cannot use revoked historical consent to add discovery standing or move custody: the membership transaction rechecks the verified signer against durable terminal revocations. The check takes a shared signing-state lock before account/server locks, matching rollout lock order; the original custody candidate is retained only if still valid after filtering; an ambiguous owner list never selects a previously unlocked account. Presence and wake acknowledgments still commit when a certificate is expired, corrupt or unavailable.

The consent question now has visible focus, and app/server names have isolated automatic text direction. The page honors Retry-After even when a proxy answers with HTML rather than JSON, handles malformed retry dates with the existing five-second floor, and displays catalogue text for network/JSON failures. No new user-facing strings were needed.

Source/test work only on personal host ehler-homelab-mini-1, in the named worktree. No push, deployment, existing database access, running-service modification, subagent or commit. Context sync failed because its lock is read-only; supplied guidance and readable canonical notes were used. PostgreSQL 18.6 binaries already extracted from the official Ubuntu package were used only to start owned, temporary socket-only clusters, each stopped by its test-command trap.

Snapshots are cumulative against HEAD. Snapshot 001 is an intermediate source capture, including a first draft of the new fixture; do not treat it as final. Snapshot 002 contains the corrected implementation and passing tests. Snapshot 003 is the final source state, supersedes both earlier captures, and is the one to apply. Snapshot directories: _program/snapshots/hosted-followups. Do not sum cumulative patches. No worktree edits follow snapshot 003.

# Decisions I made

- Hosted has dedicated account, server, DNS and signed-provider/protocol authority. Credential scope is explicit contract metadata and grants no local-server API key access to Hosted. Account and sign-in/claim continuations use session; ordinary public service/protocol entry points use open; installed server credentials use admin; the DNS feed uses read. Existing handlers enforce the same authority as before. Hosted does not gain personal-token issuance.
- A public root-signed certificate is independent of the replica's private signing authority. Store its exact signed bytes with the monotonic revision/hash, and verify its pinned root, documents purpose and current dates before returning it. The bytea column stores a signed public artifact; operational state and terminal revocations remain relational. No private key is stored there.
- A consent transaction and a root rollout must serialize. A consent that waits behind a committed revocation is refused even if an old process verified its historical signature first. Existing discovery standing is retained; new discovery consent and custody consent cannot use a revoked signer.
- Keep the review's one-second consent delay and focus on the question, never an answer. Keep the exact callback and no-registration public-client design, including cancellation navigation and best-effort system-handler refusal. Direction isolation changes display only.
- Certificate revision issuance, renewal, loading, signing and check-in advertisement are bounded to 1..9007199254740991, the exact JSON integer range shared by public JavaScript clients, which also fits the local server's signed SQLite integer. The wire field stays uint64. The old Hosted runtime accepted the full unsigned range, while the local consumer could not persist its upper half. No normal rotation cadence is affected.
- Bump Hosted's exact pre-release format to 21 because its stored signing state changed. Preserve the fresh-only rule; no upgrade or compatibility path for 20 or any earlier format.
- The account display name remains the consent page's label. It is not unique, as already disclosed in review-hosted.md. The decision is bound to the actual account and browser family; no new account-username contract field was introduced.

# Bugs

The review findings were independently re-derived from the current implementation, rather than accepted solely from the prior report:

| Finding | Status and current evidence |
|---|---|
| F1 | Already fixed. Request.Validate accepts canonical 16–96-byte server challenges; the actual local issuer uses 24 bytes. TestServerIssuedChallengeSizes and PostgreSQL app tests pass with the real size. |
| F2 | Already fixed, extended here. ServerHeartbeat records presence/ack first; an unusable signer never makes that unsigned operation unavailable. TestCheckInSurvivesAnUnusableSigner passes. New TestCheckInDeliversLatestCertificateFromAnOlderReplica also covers a fenced old replica and expired/corrupt/oversized stored artifacts. |
| F3 | Already fixed. authorize.js focuses consent-line and both disables decisions and rejects their handlers before the one-second deadline. TestConsentIsNotGivenByARepeatedPress passes. Visible focus was missing in CSS and is now supplied. |
| F4 | Already fixed. appBrowserFamilySQL resolves the cookie through refresh_tokens' primary key and joins the family, then rechecks its current hash under lock. TestAppAuthorizationSessionLookupDoesNotScanEverySession passes on PostgreSQL with 4,000 families. |
| F5 | Already fixed. Consent is constructed in one pass with text nodes and separate strong elements; inserted names cannot become catalogue placeholders. TestConsentSentenceKeepsNamesApart passes, now also checking automatic direction on each name. Isolated text direction added here. |
| F6 | Already fixed. ReturnLocation appends the answer to the stored exact URI without parsing/reserializing it. TestReturnKeepsTheAddressAsShown and PKCE/exact-return tests pass. |
| F7 | Already fixed. ValidRedirect checks the known handler list and ms-/itms families while preserving ordinary custom schemes. TestReturnRefusesHandlerSchemes and TestRedirectAddresses pass. No claim that a refusal list can cover every OS handler. |
| F8 | Already fixed. claims.Cleanup validates the signer before repository work and again immediately before signing. TestCleanupIsNotSignedByAnUnusableSigner passes. |
| F9 | Already fixed. wake.Run checks signing availability before claiming a batch; Once checks before its claim; deliver releases a lease when signing lapses mid-job. TestUnusableSignerTakesNoWakeWork and TestExpiredSignerDoesNotDeliverAndReleasesWakeLease pass. |
| F10 | Already fixed coverage. Ambient bearer/account/media/dev credentials are refused before the exchange store; conflicting certificates at an already-held revision are rejected. TestAppExchangeRefusesEveryAmbientCredential and TestSigningRolloutCheckpointsOnceAndRejectsRollback pass. |
| H-6 | Fixed additional replica gap. Loaded certificate revocations were insufficient on an old replica. Verified ConsentSigner is internal-only; Store.PushMembers filters against hosted_signing_revocations inside the mutation. New tests cover new discovery, already-indexed custody, current-key success and a real PostgreSQL rollout lock wait. |
| H-7 | Fixed additional replica delivery gap. signing_state previously retained only the certificate hash. It now retains the public artifact; any replica returns the latest verified certificate through the existing heartbeat. Bounded wake rollout and signer fence are preserved. |
| Followup: revision range | Fixed Hosted side. Checked signer/load and offline certificate creation refuse revisions outside the shared exact range; renewal refuses exhaustion before staging. Check-in omits an oversized stored artifact. Boundary and CLI artifact-refusal tests pass. |
| Followup: proxy error | Fixed. response.json previously failed before Retry-After was recorded for HTML 503s. Retry metadata is now handled first, with a safe finite fallback. TestConsentHonoursRetryAfterOnAnHTMLProxyFailure passes using an HTTP-date floor; TestConsentCancelReturnsNoAuthorizationCode also passes. |

Additional HTML/script/rate review: html/template escapes the hidden JSON as text; script/style remain external; frame-ancestors none plus X-Frame-Options DENY, base-uri/form-action none, no-store, no-referrer and nosniff remain. Dynamic content uses textContent/text nodes, not HTML. The page removes request query state from history. Cookie/CSRF review and decision remain restricted to the canonical account Origin, with exact family/account binding. Credentialless exchange alone has wildcard CORS and emits no cookie. Handler concurrency and body/time bounds remain intact.

The shared cheap budget remains 60 requests/minute/address (IPv6 /64) and 600/minute/network (IPv6 /48, IPv4 /24), across the page/review/decision/exchange, with Retry-After. It is a bounded process-local count-min sketch; multiple replicas each have their own address budget. The database's ten reviews/minute/account remains shared and serialized on the account row, including deny/spent tombstones. TestAppAuthorizationAdmissionIsSharedByEveryEntryPoint proves API route changes and the page cannot reset the same address budget. Static assets retain their separate cheap credential admission. OPTIONS and early origin/query refusals make no database call. No new lockout or intrusive user restrictions were added.

Per inherited bug ID (outside-area entries are not new programme-wide verification claims):

- B-01.1 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.2 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.3 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.4 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.5 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.6 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.7 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.8 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.9 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.10 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.11 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.12 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.15 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.16 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.17 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.18 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.19 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.20 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.21 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.22 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-01.23 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-08.1 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-08.6 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-08.9 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-08.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-08.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-08.15 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.1 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.2 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.3 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.4 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.5 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.6 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.7 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.8 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.9 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.10 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.11 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.12 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.15 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.16 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.17 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.18 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.19 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.21 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.23 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.24 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.26 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.27 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.28 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.29 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-09.35 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-13.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-13.21 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.1 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.2 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.3 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.4 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.5 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.6 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.7 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.8 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.9 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.10 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.11 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.12 — Fixed Hosted delivery/old-replica consent gap here; local revocation/provenance gates already implemented in main-server; fleet latency unverified.

- B-15.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.15 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.16 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.17 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.18 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.19 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.20 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.21 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.22 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.23 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-15.24 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-19.2 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-19.4 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-19.7 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-19.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-19.17 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-19.18 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-21.5 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-21.12 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-21.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-21.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-21.16 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.1 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.2 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.3 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.4 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.5 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.6 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.7 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.8 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.9 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.10 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.11 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.12 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.15 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.16 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.17 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.18 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.19 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.20 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.21 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.22 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.23 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.24 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.25 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-22.26 — Already fixed for Hosted slowdown/admission; not fixed here for client shared-gate wiring, outside this worktree.

- B-25.1 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.2 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.3 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.4 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.5 — Fixed here: copied toolkit and 99 scopes updated; complete canonical comparison and CI gate preserved.

- B-25.6 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.7 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.8 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.9 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.10 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.11 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.12 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.13 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.14 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.15 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.16 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.17 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.18 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.19 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.20 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- B-25.21 — Not fixed here: outside Hosted followup ownership; retain the owning lane's status in hosted.md/accounts-security.md and their reviews.

- N-V01.1 — Local provenance and future admission gates already fixed in main-server; Hosted certificate delivery aligned and hardened here; fleet acceptance remains external.

# API changes

- All 99 existing JSON route definitions gain explicit Requires; generated OpenAPI gains x-scope and TypeScript routes gain requires. No method, path, JSON request field, JSON response field or status is changed by this lane. No route removed.
- POST /v1/servers/{id}/heartbeat retains signingCertificate: Certificate|null. It now returns the latest durably recorded root-certified documents artifact across replicas, instead of only the responding process's own certificate. An invalid/expired artifact is null; presence and acknowledgment remain available.
- directory.MemberStanding.ConsentSigner is verified internal provenance with json:"-", never accepted from a caller or emitted. No public contract change.
- No packages/client-core changes are needed for these metadata/internal-behavior changes; that package is outside this worktree. Existing generated files are current. Hosted's non-JSON browser page/assets remain under the existing callback convention.

# Schema changes

Fresh-only PostgreSQL format 21. hosted_signing_state adds certificate bytea NOT NULL DEFAULT empty bytes, storing the exact signed public certificate atomically with existing revision, certificate_hash, pending and after_server. Terminal revocations remain in hosted_signing_revocations. The current baseline records numbered format history through 21 in one initialization transaction. Earlier/current-missing-history schemas are refused without mutation. TestHostedRejectsThePreviousSigningDeliverySchema specifically covers 20; existing old-schema/dry-run/restart checks also pass. No SQLite schema touched; this repository has no server baseline/schema-sync script.

# Owner or viewer settings added or wired

None. Existing account cookie, canonical Origin, signing/root/sealing files, TLS-advertised routes, wake queue/backoff and check-in cadence remain the inputs. No third-party client registration, secret, push notification, email or extra local-server polling was added.

# Needed from others

- Canonical toolkit publication/integration: CI still recursively compares the public portico-server default branch with the entire copy. It has not been weakened or pinned around these changes. Publish the integrated canonical apikit before expecting the Hosted CI comparison to pass against GitHub. Local comparison with named main-server passes.
- Client Q-6 / B-22.26 work remains outside this lane: initial explicit/manual actions interactive, timer polls automatic; due time is max(protocol due, Retry-After, shared gate). The client's shared outage floor must not be capped to Hosted's 30-second protocol slowdown.
- Current main-server source already consumes nullable signingCertificate in current_schedule.go, verifies pinned-root documents purpose/dates, persists monotonic revocations, and retires/inhibits revoked-signer descendants. The former consumer/provenance requests in hosted.md/review-hosted.md are implemented there. This is source alignment, not a fleet integration test. Certificate/envelope field shapes match. Numeric range mismatch found and closed for Hosted-produced/advertised artifacts: Hosted issuance is now limited to the exact JSON range, within SQLite's signed range. Local hostedtrust still formally accepts uint64 root-signed revisions above SQLite's range, and learnRevocations passes that value directly to SQL; an externally root-issued oversized certificate could therefore cause a storage error. The server lane should reject that range before persistence if supporting certificates issued outside the Hosted ceremony. No normal Hosted artifact can now trigger it. Check-ins are normally weekly when wakeable and daily when not, with server jitter; missed/unreachable delivery is bounded by that existing cadence, not 30 seconds. The 30-second fence is Hosted-internal only.
- Local admission still chooses membership/role, MFA, device approval and remote sign-in policy. The app exchange returns only a server/challenge-bound identity assertion; it changes none of those rules. Pending factors, TV approvals and Cast provenance gates are the accounts/security implementation, read-only here.
- Invitation binding remains by Portico username. A verified-email assertion was mentioned as a possible extension in accounts-security.md, not an unfinished Hosted requirement; no email claim was added or promised.
- Existing architecture/runbook descriptions of revision 20 and process-only check-in certificates need to describe format 21 and shared durable delivery when the owner reconciles programme documentation. The source lane's browser/Cast/ACME/DNS/provider/device and client acceptance remains outstanding. No documents were created beyond this requested report.

Deploy consequence for the owner:

1. This build needs an **empty Hosted database**. It refuses format 20 as well as 19 and earlier. The original Hosted lane removed the adoption/upgrade bridge under the pre-release rule; this followup adds stored certificate delivery and provisions exact format 21 only. It will not migrate existing accounts/claims/sessions. Preserve the old database and files intact, and provision a separate empty database rather than deleting the existing one.
2. Build/package the integrated Hosted branch with the matching canonical toolkit and correct compiled public root; ship its embedded consent assets and generated contracts. Preserve the existing edge route for /auth/apps/*, canonical account origin/cookies, exact trusted-proxy configuration and TLS hostname. Validate initialization/readiness and the test flow on the separate database before switching traffic. No deploy has been performed here.
3. Switching to an empty database means Hosted accounts, provider links, browser/TV sign-ins, installed server claims/credentials, discovery membership, certificate namespace/order state and DNS address state are absent. Recreate accounts and re-claim/reconcile servers and provider/DNS/certificate state under the existing operator procedures; existing server credentials cannot authenticate against empty claim tables. Local-server libraries and direct/password access are not migrated by Hosted. Do not assume a binary-only restart keeps Hosted access working.
4. **New or renamed runtime environment variables: none. New secrets: none.** Existing names involved: PORTICO_APPLE_TEAM_ID (web association), PORTICO_DEV_LOOPBACK_ROUTE (development only). Existing file/flag names whose contents or destination must be checked for this deploy: database-url-file; key-file; key-file.certificate.json; key-file.seal.json; root-public-key-file; claim-audience; tv-issuer; device-verification-uri; origins; trusted-proxy-cidrs; oidc-config-file; direct-dns-key-file; acme-config-file; appstore-config-file. These are names only, not values. PORTICO_SERVER_REPO is the existing toolkit-comparison input; PORTICO_TEST_DATABASE_URL_FILE is the existing isolated-test input. No app client ID/secret is required. Ordinary rotation changes the root-signed certificate's revision/revocations and online signing key, while preserving the offline root and sealing key; do not place the offline root private key online.
5. Rollback: stop the new release's workers and traffic, point the retained prior release at its retained matching database and signing/sealing/config files, then restore the previous traffic target and reconcile external DNS/certificate changes made since cutover. Never point an older binary at format 21 or restore a format-20 database underneath this binary. New-database changes after cutover are not merged into the retained database by rollback. If a newer root certificate has revoked an old online signer, returning to that revoked key or an older revocation list is not a valid rollback: servers' learned revocations are permanent. Retain/use the newest valid root-certified signer and revocation list with the compatible prior release if supported, otherwise forward-fix the current release. A restored database's signer state must not forget revocations learned after its backup. The restore-drill flag intentionally bypasses rollout/fence workers; it is not proof of production revocation delivery.

# Test notes for other environments

Checks performed (limited package selections, no repository/full test suite):

- Hosted: GOFLAGS=-p=2 go build ./... passes; go vet ./internal/httpapi ./internal/directory ./internal/postgres ./internal/contract ./internal/security passes.
- Toolkit: go test . with TestEveryRoute/Registry/TypedChanges/AccessLabels/GenerateWith/Generated/RawRoutes/Contextual selection passes. This covers mandatory scopes, admission ordering and new revision fences. Copy comparison passes byte for byte; git diff --check clean.
- Hosted unit/script/contract selection across internal/httpapi, appauth, directory, wake, claims and contract passes. Redirect/PKCE/server challenge, all app consent/CORS/cookie/escaping/rate regressions, all review signer regressions, served/declaration matching and generation checked. Node available; page-script tests ran rather than skipped. node --check passes. HTTP tests were rerun after the final script changes.
- Security: selected signer/key ceremony/setup/rotation/certification/check-in tests pass, including the maximum exact revision and refusal of the next revision without writing a certificate.
- PostgreSQL selection across internal/postgres and internal/contract passes on owned PostgreSQL 18.6; final selection completed in 19.0 seconds for postgres and 0.7 seconds for contract. Selected TestCheckInDelivers, OlderReplica, SigningRollout, AppAuthorization, MemberPush, MemberConsent, OwnerChange, CurrentHostedRestart, CurrentIdentitySchemaRequires, HostedRejects, MigrateDryRun, Generated, Definitions, ContractDeclares and Golden. All selected PostgreSQL-backed tests ran; none was unavailable or skipped. A final additional PostgreSQL selection for new delivery/consent/previous-format/ambiguous-custody cases passes in 3.9 seconds; no selected database test skipped. Other PostgreSQL suites (provider/App Store/export/ACME/burst/scale/etc.) were intentionally not run under the programme's limited-testing rule.
- First attempts corrected during work: a relative edit command ran from the module instead of worktree root and wrote nothing; a Go test issued from the repository root found no module and ran nothing; an initial toolkit test selector matched no tests, then the actual scope/revision tests were selected. A new custody fixture initially retained two owners and correctly refused transfer; it was corrected to exercise a real transfer, and filtering now invalidates revoked custody candidates without selecting an unlocked account from an originally ambiguous list. A shared-route limiter fixture initially omitted the canonical Origin allowlist, so early Origin denials never reached admission; corrected without changing origin security. Final selected checks above pass.

Cold isolated PostgreSQL reproduction: use PostgreSQL 17/18 binaries; initialize an owned empty /tmp cluster with auth-host=reject, listen_addresses empty, a private Unix socket and unused port. Create a database whose name contains test; put its DSN in a mode-0600 file and set PORTICO_TEST_DATABASE_URL_FILE. From hosted-services/hosted run:

```
GOFLAGS=-p=2 go test ./internal/postgres ./internal/contract -run 'Test(CheckInDelivers|OlderReplica|SigningRollout|AppAuthorization|MemberPush|MemberConsent|OwnerChange|CurrentHostedRestart|CurrentIdentitySchemaRequires|HostedRejects|MigrateDryRun|Generated|Definitions|ContractDeclares|Golden)' -count=1
GOFLAGS=-p=2 go test ./internal/httpapi ./internal/appauth ./internal/directory ./internal/wake ./internal/claims -run 'Test(App|Consent|ServerIssued|Return|Redirect|PKCE|CheckIn|CleanupIs|UnusableSigner|ExpiredSigner)' -count=1
```

Pass means no selected database test says SKIP; current certificates work, revoked consent cannot gain new discovery/custody, a queued consent sees the earlier committed rollout, a fenced old replica delivers certificate B and records the wake ack, null/unusable certificate preserves presence, old format is refused unchanged, PKCE/code/family fences hold, and one concurrent exchange wins. Stop only the owned cluster afterward. A /tmp/portico-hosted-followups-db-tests.sh helper was used here; it is not a repository artifact.

Browser/provider/device acceptance: CUA inventory has no apps or browsers, so no actual browser rendering, screen reader, real account cookie, provider/MFA or OS callback test was possible. On an isolated format-21 instance, exercise password/provider/MFA sign-in and signed-in cookies; desktop/mobile width and long/RTL app/server names; held Enter/double-click at Continue; Tab to Allow/Cancel only after review; visible question focus; Cancel yields only access_denied; exact HTTPS/loopback/custom callbacks, state/PKCE mismatch, expiry and replay. Make a proxy return HTML 503 with a ten-second Retry-After date: an immediate second Continue must send no request, and one after the floor must work. Automated Node stand-in tests do not prove screen-reader or physical key behavior.

Fleet acceptance: admit sessions/TV/factor/Cast descendants under A; install root-certified B revoking A on an isolated Hosted instance; run old and new Hosted replicas against that same format-21 database. Pass: only B performs signed wakes, old replica still returns B on check-in, local server learns terminal revocation before later admission, all A descendants/pending credentials refuse, password/B sessions survive, ack drains and steady state does not repeat the sweep. An unreachable server learns on its existing check-in after reachability returns. Verify TLS/proxy/OS callback/Cast/ACME/DNS behavior separately; no real external provider calls or deployment were exercised.

# Not finished

No known Hosted source implementation item remains from this lane. Local-server handling of externally root-issued revisions beyond its persistence range is the exact residual contract hardening noted above, outside this writable lane. External client shared-outage wiring, canonical toolkit publication, documentation reconciliation and real browser/provider/device/fleet/deploy/rollback acceptance remain. The consent label remains a non-unique display name by the recorded decision. Multi-replica address admission remains process-local; the account quota is database-wide. No full test suites, packaging pipelines or deployment were run. Source alignment and repository tests do not establish production fleet revocation latency.
