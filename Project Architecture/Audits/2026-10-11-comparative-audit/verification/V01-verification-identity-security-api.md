# V01 — Verification of identity, security and API/realtime

## Summary

All **12 Tier 1** and **26 Tier 2** entries were reached. The first pass found real implementation gaps: remote sign-in policy misses the first-party route (B-01.2/B-15.1, one High defect), the release Hosted-attachment flag reads retired storage, an expired approved TV code repeatedly breaks setup pruning, and the client can delete credentials on a nonterminal 403. The report separates duplicate IDs from separate issues; verdict totals are entry counts, not unique bugs.

Several claims held only within narrower conditions. Proxy-origin failure has an explicit configuration workaround; hidden private gateways are not all exempt from HTTPS; member-device approval works with a known ID; nil/omitted response handling and Device authorization are less weak than described. Open remote first-run setup, plain state/backups, role inheritance on TV approval, capped cool-offs, and revision-fenced paging reflect recorded choices. They should not be planned as ordinary security defects. No entire entry was refuted; the Apple sign-out incident remains unsettled and its proposed single-profile cleanup explanation is contradicted by current guards.

One new independently traced **High** finding, **N-V01.1**, affects signer-compromise response: learning Hosted signer revocation does not retire families already admitted through the current server-native model. Static source supports that gap; no runtime incident, exploit, capacity failure or browser rebinding was reproduced. Tier 1: **3 Confirmed, 8 Confirmed/narrower, 1 Unsettled**. Tier 2: **14 Confirmed, 12 Confirmed/narrower**. Twenty-seven Low entries were copied without re-check.

## Verdict table

| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-01.2 | "Remote sign-in: owner only / off" bypass | Reading / High | Confirmed | Defect | High | M | No |
| B-01.8 | hostedAttached is always false in release builds | Probable / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-01.9 | Any 403 on refresh deletes the stored sign-in | Code confirmed; trigger speculative / Low–Medium | Confirmed | Defect | Medium | S | No |
| B-01.14 | Two different tests for "is this request local?" | Reading; exposure speculative / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-01.15 | Two hashing slots, no queue | Speculative / Low–Medium | Confirmed, narrower | Design risk | Low | S | No |
| B-01.23 | Related Backlog item: two Apple sign-ins ended with explicit_signout | Speculative / Unspecified in Backlog | Unsettled — needs runtime | Design risk | Medium | M | Yes (incident) |
| B-08.1 | Behind a TLS-terminating reverse proxy, the bundled web app's writes are refu… | Reading / High | Confirmed, narrower | Defect | Medium | S | No |
| B-08.6 | Every event wakes every connected client into several database reads (design … | Reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-08.14 | A full-library walk may not finish while the library is changing (needs a run… | Speculative / Medium | Confirmed, narrower | Deliberate trade-off | Medium | L | No |
| B-15.1 | The remote sign-in policy is enforced on only one of the sign-in routes | Reading / High | Confirmed | Defect | High | M | No |
| B-15.4 | An un-set-up server can be claimed by anyone who can reach it, and by a web page | Reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-15.12 | A stolen Hosted signing key vouches for any Portico Account until each server… | Reading / High | Confirmed, narrower | Design risk | High | L | No |
| B-01.1 | An access schedule signs people out instead of pausing them | Reading / Medium | Confirmed | Defect | Medium | M | No |
| B-01.3 | API keys can be created but never authenticate | Reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-01.4 | "Require approval before a device can play" does nothing | Reading / Medium | Confirmed | Defect | Medium | M | No |
| B-01.5 | One approved-but-never-collected TV code stops identity clean-up for good | Reading / Low–Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-01.6 | A TV code cannot complete when new devices need approval (latent) | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-01.7 | The approve-new-devices policy can lock the owner out, and members' devices c… | Reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-01.10 | The two "highest rating" controls disagree | Reading / Medium | Confirmed | Defect | Medium | M | No |
| B-01.12 | Erasing a person leaves their name in the owner's play history | Reading / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-08.2 | Any error a legacy handler does not recognise becomes 400 invalid_request, un… | Reading / Medium | Confirmed | Defect | Medium | M | No |
| B-08.3 | A reconnect that arrives before the server notices the old stream is dead dem… | Reading / Low today / Medium if SSE used | Confirmed, narrower | Defect | Low | S | No |
| B-08.4 | An SSE client that has not yet received an event has no cursor, so a drop los… | Reading / Low today / Medium if SSE used | Confirmed | Defect | Low | S | No |
| B-08.7 | Behind a reverse proxy, sign-in and device-code limits are shared by everyone | Reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-08.9 | Registry safety checks fail whole responses at run time | Reading / Medium | Confirmed, narrower | Design risk | Low | S | No |
| B-08.10 | The mismatch between error envelopes changes client retry behaviour | Reading / Low–Medium | Confirmed | Defect | Medium | S | Related only |
| B-08.13 | The registry's access label is not what is enforced | Reading / Medium | Confirmed, narrower | Design risk | Low | M | No |
| B-08.15 | Original files are always labelled video/mp4 | Reading / Low–Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-15.2 | One approved-but-uncollected TV code permanently stops identity maintenance | Reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-15.3 | Behind a reverse proxy, the database-backed limiter treats all clients as one | Reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-15.5 | Live credentials and two-factor seeds are stored and backed up in the clear | Reading / Medium | Confirmed | Deliberate trade-off | Medium | S | No |
| B-15.6 | Three different readings of X-Forwarded-For | Reading / Low–Medium | Confirmed | Defect | Medium | M | No |
| B-15.7 | Tokens are not bound to a device key, and the documentation says they are | Reading / Medium | Confirmed | Documentation error | Low | S | No |
| B-15.9 | The second factor can be worn down by an attacker who already has the password | Reading / Low–Medium | Confirmed | Deliberate trade-off | Low | S | No |
| B-15.10 | Two hashing slots serve every password check | Reading / Low–Medium | Confirmed, narrower | Design risk | Low | S | No |
| B-15.11 | "Private peer" can be wrong when something in front hides the client address | Reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-15.13 | Approving a code makes the other device you, and codes are advertised on the LAN | Reading / Medium | Confirmed | Deliberate trade-off | Low | S | No |
| B-15.22 | "Secure connections: required" has quiet exceptions | Reading / Low–Medium | Confirmed | Deliberate trade-off | Low | S | No |

## Tier 1 — detailed verification

### B-01.2 / B-15.1 Remote sign-in policy misses the first-party route

- **Claim:** Owner-only/off remote sign-in is bypassed through the normal direct/Portico Account sign-in routes.
- **What I traced:** `apps/portico-server/server/internal/httpapi/router.go:836` registers the legacy route and calls the policy at line 856. Repository-wide searches find no other enforcement caller. `apps/portico-server/server/internal/httpapi/direct_identity.go:66–111` issues password or verified Portico-identity sessions, checks only schedules, and returns them; `apps/portico-server/server/internal/httpapi/identity_routes.go:258–281` completes MFA with only schedule admission. `apps/portico-server/server/internal/httpapi/router.go:1282–1316` installs device context, cheap rate limiting and delivery locality, but no remote sign-in guard. `resolvePrincipal` at `apps/portico-server/server/internal/httpapi/router.go:608–654` also has none. Shared clients use `apps/portico-server/packages/client-core/src/profile-management.ts:78`.
- **Verdict:** **Confirmed** for both IDs. The default is allow (`apps/portico-server/server/internal/operations/settings.go:192`), so only owners selecting owner-only/off are affected. Profile-selection and TV collection also omit this guard; this setting intentionally leaves already established sessions alone, so those secondary paths need an explicit admission-versus-continuation rule rather than blanket revocation.
- **Class:** Defect. The correct-password/no-lockout philosophy does not justify a separately exposed control that silently fails.
- **Severity:** **High**. Remote members with valid credentials can create sessions despite an owner-selected prohibition.
- **Fix direction:** Put remote admission in a shared issuance/admission boundary for new sign-ins, cover password, Portico identity, MFA and new device grants, and preserve established-session continuation. **Size: M**; touches `apps/portico-server/server/internal/httpapi/access_admission.go`, issuing handlers, identity issuance interfaces and route tests.
- **Related:** B-01.2 and B-15.1 are exact duplicates and agree. B-01.14/B-15.11 concern which address is classified remote; repairing that alone cannot repair this wiring gap. **Backlog:** no matching item found.

### B-01.8 hostedAttached reads retired policy storage

- **Claim:** Release builds report no Hosted attachment and choose the direct sign-in/invitation UI even for a claimed server.
- **What I traced:** `apps/portico-server/server/internal/identity/pin_recovery.go:57–60,139–147` derives both flags from `SELECT EXISTS ... policy`; `apps/portico-server/server/internal/httpapi/router.go:817` exposes it. The only non-test insertion is `apps/portico-server/server/internal/hosted/legacy_policy_development.go:1,55` behind `!release`; `apps/portico-server/server/internal/hosted/current_claims.go:60` deletes old policy. `apps/portico-server/web/src/screens/auth/SignIn.tsx:38–63` treats explicit false as direct, and `apps/portico-server/web/src/screens/auth/DirectJoin.tsx:70` uses the same fallback.
- **Verdict:** **Confirmed, narrower**. The current release claim path cannot populate this retired table; a fresh/current claimed state therefore reports false. “Always” is too broad for arbitrary imported legacy databases. It changes the default and invitation branch, not whether Hosted authentication itself exists: the direct form retains the Portico Account link.
- **Class:** Defect.
- **Severity:** **Medium**, unchanged, for owners/members visiting a claimed server’s bundled page or invitation.
- **Fix direction:** Derive capability from installed current claim/auth mode with an exact definition for attached versus merely configured. **Size: S**; `apps/portico-server/server/internal/identity/pin_recovery.go`, current claim adapter, capability and web branching tests.
- **Related:** No separate matching B-index entry. **Backlog:** no matching item found.

### B-01.9 Status-only refresh refusal deletes credentials

- **Claim:** Any 403 on refresh, including an intermediary’s response, erases the stored session.
- **What I traced:** `apps/portico-server/packages/client-core/src/server-connections.ts:225–263` sends through routed `api.request`, then branches solely on caught status 401/403 at line 251. The store deletion is carefully scoped to the same family and refresh token; it does not validate provenance/error code. `apps/portico-server/server/internal/httpapi/router.go:1256–1258` is a concrete Portico 403 unrelated to revocation (origin rejection). `portico-internal/Product Contracts/identity/contract.md:177` (section 5) requires ambiguous failure preservation.
- **Verdict:** **Confirmed**. No runtime proxy failure is claimed. The conditional deletion avoids erasing a newer/replaced family, but cannot distinguish a valid authorization refusal from an HTML/JSON gateway 403.
- **Class:** Defect.
- **Severity:** **Medium** (first pass Low–Medium): a temporary routing or policy failure can force password/code sign-in even though server credentials remain valid.
- **Fix direction:** Preserve credentials unless a validated Portico terminal code proves the family ended; retain current concurrent-family fence. **Size: S**; `apps/portico-server/packages/client-core/src/server-connections.ts`, API error/route parser and refresh cases.
- **Related:** B-01.1 supplies a genuine terminal server revocation; B-22.14 concerns intermediary error provenance but does not prove this deletion. **Backlog:** no exact item; Apple unexpected sign-outs are related, not a demonstrated cause.

### B-01.14 Locality differs between recovery and connectivity

- **Claim:** Recovery/setup helpers and playback/remote policy interpret local addresses differently, potentially accepting internet recovery traffic hidden behind a private gateway.
- **What I traced:** `apps/portico-server/server/internal/httpapi/onboarding.go:18–44` uses environment proxies and a single forwarded IP, then private/loopback/link-local status. `apps/portico-server/server/internal/httpapi/request_locality.go:21–45,64–100,119–141` combines settings and environment proxies, accepts chains, and applies configured LAN CIDRs. `apps/portico-server/server/internal/httpapi/router.go:614,634` and `apps/portico-server/server/internal/httpapi/direct_identity.go:95` use the former; `apps/portico-server/server/internal/httpapi/access_admission.go:102,139` uses the latter. Recovery enforcement is real in `apps/portico-server/server/internal/identity/direct_accounts.go:230` and `apps/portico-server/server/internal/httpapi/onboarding.go:339`.
- **Verdict:** **Confirmed, narrower**. The deterministic settings/chain mismatch is proven. A hidden-address deployment has to be demonstrated, not inferred from “Docker” alone; a configured trusted gateway without a readable XFF fails remote under `remoteClient`, rather than always becoming LAN. No anonymous recovery access follows: the recovery password is still required.
- **Class:** Defect in policy consistency; topology-dependent exposure remains a design risk.
- **Severity:** **Medium**, unchanged, for claimed installations relying on a custom LAN boundary or settings-only trusted proxy.
- **Fix direction:** Share one canonical client-address parser and explicit recovery-LAN policy across recovery, cache and connectivity; retain fail-closed malformed-header handling. **Size: M**; `apps/portico-server/server/internal/httpapi/onboarding.go`, `apps/portico-server/server/internal/httpapi/request_locality.go`, `apps/portico-server/server/internal/httpapi/rate.go`, `apps/portico-server/server/internal/httpapi/secure_connections.go`, locality tests. Cheapest topology check later: inspect RemoteAddr/XFF on an isolated instance through the actual proxy/container and verify a public client is remote.
- **Related:** B-15.6/B-15.11 and B-09.24 agree on the inconsistency; B-15.11 overstates private-gateway HTTPS exemption (only loopback is exempt). **Backlog:** no matching item.

### B-01.15 Two bcrypt slots refuse rather than wait

- **Claim:** Two slow password comparisons can make honest sign-ins return busy; bogus usernames cost a hash too.
- **What I traced:** `apps/portico-server/server/internal/identity/service.go:125` creates a two-entry channel; `apps/portico-server/server/internal/identity/direct_accounts.go:102–149` pays an unknown-account bcrypt hash and uses a nonblocking channel send at lines 121–125. It checks source/account cool-offs first. `apps/portico-server/server/internal/httpapi/rate.go:39–40,76–83` adds 20/minute per address **and path**, and 1,280/minute per network; an auth lane queue exists (`apps/portico-server/server/internal/httpapi/admission.go:71`) but is not a queue for these two hashing slots.
- **Verdict:** **Confirmed, narrower**. Immediate busy is certain when both slots are held. Sustained starvation from a trickle is not established; the first-pass reference to a 1,280/minute network ceiling omits the lower per-address and cool-off guards. This is protective bounded admission, not a proven denial-of-service reproduction.
- **Class:** Design risk.
- **Severity:** **Low** (first pass Low–Medium), for sign-ins/PIN/password operations overlapping on slow hardware; existing playback does not need bcrypt.
- **Fix direction:** If measurement shows unnecessary refusals, add a short cancellable bounded hash-slot wait while preserving CPU limits and retry guidance. **Size: S**; `apps/portico-server/server/internal/identity/service.go`, `apps/portico-server/server/internal/identity/direct_accounts.go`, shared hash admission call sites. Cheapest later check: isolated slow host, sustained under-limit bogus logins plus honest logins; measure busy rate and recovery after the load stops.
- **Related:** B-15.10 is the same mechanism and agrees. **Backlog:** no exact item.

### B-01.23 Historical Apple explicit_signout

- **Claim:** The Backlog’s two unexpected simulator sign-outs might be the temporary account-session cleanup receiving a viewer session token.
- **What I traced:** `portico-internal/Project Architecture/Backlog.md:56` records the observation, without a severity or correlated requests. `apps/portico-react-native/apps/native/src/app/session.tsx:588–610` gates cleanup on `accountGrant && accountSession`; `:634–651` constructs that grant only for the profile chooser, and passes `accountGrant:false` (or no grant) for an immediate viewer result. `apps/portico-server/server/internal/identity/direct_accounts.go:294–303` explicitly returns viewer role for a single PIN-free profile.
- **Verdict:** **Unsettled — needs runtime** for the historical incident. The suggested ordinary single-profile cleanup explanation is contradicted by the current guard. That is not a refutation of the recorded incident or proof that concurrent/cancelled flows are correct.
- **Class:** Design risk / unresolved incident, not a confirmed new defect.
- **Severity:** **Medium** provisionally for unexpected loss of sign-in on Apple; the first pass/Backlog supplied no numeric severity (“as Backlog”).
- **Fix direction:** **Size: M** investigation; native session lifecycle, direct outcome parsing and server sign-out logging. Cheapest experiment later: isolated server plus Apple simulator, correlate family IDs and DELETE requests across single-profile and chooser sign-in, restart and reopen; a client DELETE of the active viewing family identifies the cause, no DELETE requires examining server revocation callers.
- **Related:** B-01.9 and B-16.2 are plausible separate credential-loss paths, not established explanations of this event. **Backlog:** yes, exact observation.

### B-08.1 Bundled web origin behind TLS termination

- **Claim:** A TLS-terminating proxy makes the bundled browser’s Origin fail the server’s same-origin check.
- **What I traced:** `apps/portico-server/server/internal/httpapi/router.go:1248–1259` runs before routing, including sign-in. `apps/portico-server/server/internal/httpapi/onboarding.go:91–96` uses backend TLS and Host only. `apps/portico-server/server/internal/httpapi/secure_connections.go:78–83` understands trusted XFP but is not used there. `apps/portico-server/server/cmd/server/main.go:551–556,946` wires explicit origins; `apps/portico-server/server/internal/operations/settings.go:192` does not supply them.
- **Verdict:** **Confirmed, narrower**. It fails for HTTPS frontend → HTTP backend when the frontend origin is not explicitly allowed; `PORTICO_ALLOWED_ORIGINS` is a real working escape hatch. HTTPS all the way to a backend whose Host matches, and direct TLS, are not affected. A rewritten backend Host also needs configured public-origin handling.
- **Class:** Defect.
- **Severity:** **Medium** (High → Medium): web writes/sign-in fail in an optional proxy topology; it fails closed and has a configuration workaround. It is still a complete blocker for that unconfigured topology.
- **Fix direction:** Use trusted transport/origin derivation and document public origins, without trusting arbitrary forwarded headers. **Size: S**; `apps/portico-server/server/internal/httpapi/onboarding.go`, outer router CORS, proxy documentation/tests.
- **Related:** B-15.8 is the same bug (listed Low/not re-checked); its lower assessment includes the workaround. Merge them at Medium. **Backlog:** no matching item.

### B-08.6 Global event wake fan-out

- **Claim:** A single event triggers authority and event SQL reads for every parked feed consumer.
- **What I traced:** `apps/portico-server/server/internal/eventfeed/feed.go:74–97` subscribes globally to three table classes and wakes all reader channels. `apps/portico-server/server/internal/httpapi/events_routes.go:112–127` reauthenticates and reads for each wake; `:37–52` adds an owner resolution; `apps/portico-server/server/internal/eventfeed/feed.go:139–175` opens a snapshot and runs bounds, device and filtered-event queries. The final authority check at `apps/portico-server/server/internal/httpapi/events_routes.go:131` occurs **when answering**, not on every irrelevant wake. `apps/portico-server/server/internal/worker/worker.go:104,129–171` coalesces shared wakes to a one-second floor; buffered subscriber channels coalesce more. Main wires the feed at `apps/portico-server/server/cmd/server/main.go:941`.
- **Verdict:** **Confirmed, narrower**. Fan-out work is proven and bounded/coalesced. The “600–800 transactions per wake” is an estimate dependent on owners and actual answers, not an established load result. Strict live authorization is intentional and must survive any optimization.
- **Class:** Design risk.
- **Severity:** **Medium**, unchanged, for hundreds of active feed clients on weak hosts during frequent writes; no measured latency/refusal threshold is established.
- **Fix direction:** Measure first, then route wakes by audience or share immutable event bounds/device binding without caching stale authorization. **Size: M**; `eventfeed`, `apps/portico-server/server/internal/httpapi/events_routes.go`, write-wake metadata. Cheapest later experiment: isolated representative host with 200 parked polls and one unrelated event/second, record read-pool waits and browsing latency against idle baseline.
- **Related:** B-08.17 covers the separate realtime admission ceiling, not this SQL cost. **Backlog:** no exact item.

### B-08.14 Revision-fenced full-library enumeration

- **Claim:** Changes during a very large paginated walk can repeatedly invalidate its continuation.
- **What I traced:** `apps/portico-server/server/internal/catalog/discovery.go:113–163` embeds and compares exact catalogue/viewer revisions; `apps/portico-server/server/internal/catalog/content.go:14–17` reads them. `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:3946–3948` increments viewer revision for progress activity insert/update/delete, so playback can invalidate the same profile’s walk. `apps/portico-server/server/internal/catalog/discovery.go:210–220` connects original browsing to the current query engine. `portico-internal/Project Architecture/02 Server.md:40` explicitly requires old revisions to return stale_continuation.
- **Verdict:** **Confirmed, narrower**. One intervening relevant revision deterministically refuses the next continuation. Permanent noncompletion requires continuing writes faster than the full traversal; quiet libraries/other profiles are not automatically affected. There is no promise of a revision-stable export snapshot in this path.
- **Class:** Deliberate trade-off (live correctness); snapshot export is a missing capability, not a broken cursor.
- **Severity:** **Medium**, unchanged as an integration limitation rather than a release defect.
- **Fix direction:** Keep current browse fences; if export is needed add an explicit bounded snapshot/job export protocol rather than silently continuing across changed restrictions. **Size: L**; catalogue enumeration, export job persistence/API, retention and cancellation. Cheapest later check: walk a known multi-page library, update progress between pages, verify 409; repeat with no writes to distinguish churn from a paging defect.
- **Related:** B-07.4 agrees and supplies the same progress-revision trigger. **Backlog:** no matching snapshot-export item.

### B-15.4 First-run ownership race and browser rebinding

- **Claim:** Any reachable client can become the first owner; a hostile page may use DNS rebinding to pass the same-origin gate.
- **What I traced:** `apps/portico-server/server/internal/httpapi/onboarding.go:132–163` deliberately permits remote first-run token retrieval, with matching/allowed Origin and JSON required; unused `localSetupHost` at `:79–90` is not a guard. `apps/portico-server/server/internal/identity/onboarding.go:361–373` refuses token retrieval once an account exists; `:83–111` checks first-owner state again inside the writer transaction. JSON and origin checks stop ordinary cross-site form/fetch setup.
- **Verdict:** **Confirmed, narrower** for reachable fresh-install takeover, which is **deliberate** (explicit Justin decision at `apps/portico-server/server/internal/httpapi/onboarding.go:141–144`). DNS rebinding is an untested residual design risk: static code accepts a matching attacker Host/Origin, but this review establishes no browser/private-network bypass. A configured-origin allowlist does not supply Host validation.
- **Class:** Deliberate trade-off; rebinding subclaim is a design risk.
- **Severity:** **Medium** residual risk (first pass Medium overall/High if exposed): an unattended uninitialized public server has a first-owner race. No initialized-server takeover is shown.
- **Fix direction:** Preserve no-code/remote setup. Document “complete setup before exposure”, and assess a Host/rebinding guard that still permits intended remote names; do not silently replace the recorded decision with local-only setup. **Size: S** for instructions/host validation policy; `apps/portico-server/server/internal/httpapi/onboarding.go`, setup docs/tests. Cheapest rebinding experiment later: disposable fresh instance and isolated two-answer DNS fixture in supported browsers, verify whether token retrieval and setup finish; blocked private-network access refutes that browser variant only.
- **Related:** No duplicate; B-01.14 is recovery after setup and is distinct. **Backlog:** no matching item.

### B-15.12 Compromised Hosted documents signer

- **Claim:** A stolen certified Hosted online signing key can forge server-specific account assertions until a server learns revocation.
- **What I traced:** `apps/portico-server/server/internal/hostedtrust/trust.go:19–22,104–134` bounds certificates to 400 days and verifies pinned-root purpose/signature/validity. `apps/portico-server/server/internal/hosted/trust.go:59–108` persists newer root-signed revocation sets and rejects remembered key IDs; `apps/portico-server/server/internal/hosted/identity_assertion.go:103–175` verifies exact server, short assertion lifetime and one-time challenge/nonce. A key holder can obtain a fresh public challenge and sign those fields, so they prevent replay, not signer compromise. `apps/portico-server/server/internal/identity/portico_accounts.go:66–101` requires an existing enabled local membership and additionally enforces server-configured TOTP. `apps/portico-server/server/internal/hosted/trust_revocation_test.go:12–37` proves learned revocation survives old certificates/restart; it does not prove prompt fleet distribution.
- **Verdict:** **Confirmed, narrower**. Existing Portico-linked membership (possibly owner), reachability, an unrevoked documents certificate and no unsatisfied server-local MFA/device/PIN restriction are required. A wake-purpose key alone cannot forge documents. No exact compromise-to-fleet-revocation latency is established. Annual signing certificate life is a recorded choice, not a cryptographic implementation bug.
- **Class:** Design risk.
- **Severity:** **High**, unchanged, conditional on compromise of a central online key, with owner impersonation possible on eligible servers.
- **Fix direction:** Define and validate a durable emergency revocation delivery/acknowledgment protocol and track the signer for admitted families; preserve outage use and cheap steady-state operation. **Size: L**; `apps/portico-server/server/internal/hosted/trust`, server-native family admission, Hosted key operations/revocation delivery. Merely polling more often would change the recorded no-chatter design.
- **Related:** No exact duplicate. New finding N-V01.1 below distinguishes rejection of future assertions from retiring already admitted current-model families. **Backlog:** no exact compromised-signer remediation item.

## Tier 2 — quick checks

Every listed Tier 2 entry was read in its source document and its cited implementation opened. The expanded entries below correct material overstatements; ordinary checks remain short. Default device registration is automatic; the identity approval policy is reachable through the owner API but no first-party policy editor was found. “No” in the Backlog column means no matching item in the dated file, not a claim about all historical audits.

### Expanded checks where the original wording changes the decision

#### B-01.5 / B-15.2 Approved uncollected TV code blocks setup pruning

- **Claim:** An expired approved code permanently stops identity cleanup.
- **What I traced:** `apps/portico-server/server/internal/identity/quick_connect.go:299` approves without token_hash; `:568–586` creates a token only while consuming. The nullable schema is `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:2743`. `PruneSetup` at `apps/portico-server/server/internal/identity/quick_connect.go:438–447` scans that NULL into string, returns an error and rolls back. The stale approved row cannot be deleted by the later non-approved-row cleanup at line 477. Maintenance discards the error at line 498.
- **Verdict:** **Confirmed, narrower** for both IDs. This repeatedly blocks **PruneSetup**, including setup receipt/code/rate-table cleanup, until the row is independently resolved. It does not halt all identity maintenance: renewal, session-history and identity-proof pruning still run at `:499–505`. Retained grants are not automatically usable forever: collection still validates expiry and family state (`:338–379`). The faulty prune can retain live plaintext credentials longer than intended and grow tables.
- **Class / severity:** **Defect / Medium**; ordinary approval followed by an abandoned TV is enough. B-01.5’s Low–Medium is resolved to Medium; B-15.2 stays Medium.
- **Fix direction:** Scan nullable hashes, expire no-family approvals by row ID, keep bounded cleanup and report maintenance failures. **Size: S**; `apps/portico-server/server/internal/identity/quick_connect.go`, maintenance tests.
- **Related / Backlog:** exact duplicates, agree on scan failure but overstate whole-maintenance scope; B-01.6 can leave the same stuck approval. Not in Backlog.

#### B-01.7 Identity approval has recovery and discovery gaps

- **Claim:** Owner approval can lock out the owner and members’ devices cannot be approved.
- **What I traced:** `apps/portico-server/server/internal/identity/devices.go:193–208` makes every new account/installation pending without owner exemption. `:282–307` lists only caller devices. `:401–454` **does** permit the owner to approve a different account’s known ID. `apps/portico-server/server/internal/identity/direct_accounts.go:281–283` returns that pending ID in the sign-in response. `apps/portico-server/server/internal/identity/recover_owner.go:34–80` resets credentials/MFA and revokes families, but does not alter device policy.
- **Verdict:** **Confirmed, narrower**. There is no complete owner discovery UI/API for member pending devices, not an inability to approve them: a member can convey the ID returned by pending sign-in. Owner recovery is trapped when no accessible approved installation remains; an approved installation can sign in again even if its prior session expired, so “no signed-in device” alone is insufficient.
- **Class / severity:** **Unfinished feature / Medium**, unchanged; latent until owner API enables approval.
- **Fix direction:** Add an administrator-scoped pending identity-device directory and a deliberate recover-owner device-policy recovery path. **Size: M**; identity device listing/approval, registration policy, recover-owner CLI and client admin Devices.
- **Related / Backlog:** B-01.4 is a separate legacy table/policy; B-01.6 shares the incomplete identity policy. No matching Backlog item.

#### B-01.12 Erasure preserves owner history names

- **Claim:** Deleting a profile/account preserves copied identifying names in owner play history despite the erasure contract.
- **What I traced:** `apps/portico-server/server/internal/identity/direct_profiles.go:478–518` clears personal state and tombstones profile names; `apps/portico-server/server/internal/hosted/claim_cleanup.go:169–211` erases linked-account personal data, but neither anonymizes `play_history`. `apps/portico-server/server/internal/persistence/migrations/0007_play_history.sql:3–18` explicitly retains copied names after account deletion. `portico-internal/Project Architecture/02 Server.md:75` records owner history forever unless the owner sets retention; `portico-internal/Product Contracts/identity/contract.md` §9 promises server erasure/anonymization.
- **Verdict:** **Confirmed, narrower**: retention works as explicitly designed, and the documents conflict about deletion’s privacy boundary. Clearing personal History is not account erasure; the latter needs a resolved policy.
- **Class / severity:** **Deliberate trade-off / Low** (Low–Medium → Low). The owner retains a private audit record; no new viewer access or external disclosure is shown. The contract inconsistency remains actionable.
- **Fix direction:** Resolve owner-history-versus-erasure wording, then optionally anonymize identifying columns while preserving plays/counts. **Size: S**; identity/Hosted erasure and history specification.
- **Related / Backlog:** B-01.18 is session retention, not this table. Backlog history enhancements do not track anonymization.

#### B-08.9 Registry rejection is intentional and narrower than “any nil”

- **Claim:** Post-handler response validation can turn a committed write into a 500 when a response is nil/too large.
- **What I traced:** `apps/portico-server/apikit/registry.go:370–389,412–424` runs serialization after the handler and rejects over 512 KiB. `:580–673` rejects serialized nil slices/maps/interfaces but explicitly permits nil pointers and skips empty omitempty fields. It cannot roll back an already committed handler.
- **Verdict:** **Confirmed, narrower**. The enforcement mechanism is intentional; no failing production response was identified. “Any reachable” nil is false for omitted fields and nullable pointers. A future handler violating its response contract is a real latent risk, not an observed endpoint defect.
- **Class / severity:** **Design risk / Low** (Medium → Low).
- **Fix direction:** Preserve response contract checks; add response conformance coverage at authoring boundaries and ensure mutation replay/reconciliation remains possible. Do not automatically invent arrays/objects for required semantic values. **Size: S**; registry conformance fixtures and relevant future handlers.
- **Related / Backlog:** B-08.13 shares overreliance on registry metadata; no exact Backlog item.

#### B-08.13 Access labels do not prove item/library admission

- **Claim:** Registry labels do not fully enforce the content restrictions they advertise.
- **What I traced:** `apps/portico-server/server/internal/httpapi/playback_v1.go:223–262` enforces owner/admin, the special direct_account scope, and **a real device binding check** for Device/ViewerItem at lines 253–255. ViewerItem item access is conditional on a path parameter literally called itemId; ViewerLibrary has no corresponding automatic library admission.
- **Verdict:** **Confirmed, narrower**. It is wrong to characterize all these rules as the same principal check: Device has additional binding. The absence of general item/body/library extraction remains real. No current unprotected content route was established. `portico-internal/Project Architecture/Specs/Spec — API v1 Style Guide.md:26` overstates how metadata makes forgotten content checks impossible.
- **Class / severity:** **Design risk / Low** (Medium → Low), with a documentation overpromise.
- **Fix direction:** Require explicit resource extraction/admission callbacks for item/library rules, or narrow the documented guarantee; keep current handler guards. **Size: M**; `apps/portico-server/server/internal/httpapi/playback_v1.go`, apikit metadata/registration validation and contract generation.
- **Related / Backlog:** B-08.9 is a different registry safeguard. No exact Backlog item.

#### B-08.15 Video originals have the wrong MIME type on local delivery

- **Claim:** Original files are always labelled video/mp4.
- **What I traced:** `apps/portico-server/server/internal/httpapi/media_original.go:18–32` relays remote responses before setting a type; `:47–52` forces local video to video/mp4 while audio uses `audioContentType`. Production video original route calls it at `apps/portico-server/server/internal/httpapi/router.go:1223`; every response has nosniff at line 1240.
- **Verdict:** **Confirmed, narrower**. Local original **video** is mislabeled for MKV/WebM/TS/AVI. Audio has a container map and remote delivery preserves its upstream response; “always” is false outside local video. Player rejection remains untested.
- **Class / severity:** **Defect / Medium** (Low–Medium → Medium), a deterministic HTTP contract error on supported local non-MP4 originals.
- **Fix direction:** Map validated video containers to MIME types and an honest unknown fallback. **Size: S**; media_original.go and delivery-header cases.
- **Related / Backlog:** B-14.12 is platform support for originals, not MIME labeling. No matching Backlog item.

#### B-15.11 Hidden client address does not exempt all private gateways from HTTPS

- **Claim:** Address-hiding proxies can make internet clients local and bypass recovery/remote/HTTPS policy.
- **What I traced:** `apps/portico-server/server/internal/httpapi/onboarding.go:18–44` accepts private untrusted peers without forwarding headers; `apps/portico-server/server/internal/httpapi/request_locality.go:80–100` accepts private peers only under its default LAN rule and fails remote when a trusted proxy omits readable XFF. `apps/portico-server/server/internal/httpapi/secure_connections.go:97–103,178–180` exempts **loopback clients**, not RFC1918 gateways generally.
- **Verdict:** **Confirmed, narrower**. An unconfigured address-hiding private gateway can defeat recovery/default-LAN classification. HTTPS exemption requires a loopback presentation (or another recorded HTTPS exception), not merely a private address. A correctly configured trusted proxy lacking XFF fails closed. Container/proxy deployment behavior remains untested.
- **Class / severity:** **Design risk / Medium**, unchanged for affected topology; passwords remain necessary.
- **Fix direction:** Canonical client-address policy, explicit address-hiding mode and owner-visible observed client locality. **Size: M**; shared locality/recovery/security helpers and connectivity UI.
- **Related / Backlog:** Merge implementation work with B-01.14/B-15.6/B-09.24; no matching Backlog item. Cheapest runtime check is the isolated actual-topology address inspection described under B-01.14.

### Remaining quick checks

| ID | Independent check and verdict | Class / severity | Fix direction and size |
|---|---|---|---|
| B-01.1 | **Confirmed**: refresh rotates, `apps/portico-server/server/internal/httpapi/identity_routes.go:223–231` calls schedule admission, `apps/portico-server/server/internal/httpapi/access_admission.go:76–82` revokes the family; shared `apps/portico-server/packages/client-core/src/server-connections.ts:251–258` deletes it. Empty timezone is UTC (`apps/portico-server/server/internal/access/enforcement.go:303`). | Defect / Medium | Refuse temporarily without revoking family, align timezone label; **M**, identity refresh/admission/client terminal codes. Related B-01.9. |
| B-01.3 | **Confirmed**: only test callers of `access.AuthenticateKey`; session-only bearer resolution in `apps/portico-server/server/internal/httpapi/access_area.go:85–99`, while web `apps/portico-server/web/src/screens/server/Access.tsx:137` issues keys. Disabled legacy auth is intentional, but exposed issuance and working-key docs are incomplete. | Unfinished feature / Medium | Finish scoped key authority or remove issuing affordance until supported; **M**, access/auth/API/UI. |
| B-01.4 | **Confirmed** for the legacy playback-approval setting: `apps/portico-server/server/internal/httpapi/access_admission.go:146–160` takes client-supplied ID; `apps/portico-server/server/internal/access/enforcement.go:96` skips empty ID; no first-party header writer found. `apps/portico-server/server/internal/access/devices.go:174–178` reassigns observed IDs. Current identity-device approval remains separate and enforced. | Defect / Medium | Use authenticated device binding and reconcile both policy/tables; **M**, access admission/devices and identity settings. |
| B-01.6 | **Confirmed**: quick grant issuance returns pending error and rolls back (`apps/portico-server/server/internal/identity/quick_connect.go:360–365,568–571`); identity registration creates pending then errors (`apps/portico-server/server/internal/identity/devices.go:193–208`). Password path commits a pending result instead (`apps/portico-server/server/internal/identity/direct_accounts.go:281–283`). | Defect / Medium, latent | Commit pending registration or treat explicit TV approval as the identity approval; **S**, quick_connect/issuing_device. Related B-01.5/7. |
| B-01.10 | **Confirmed**: account’s exact US ladder/first rating (`apps/portico-server/server/internal/access/limits.go:18`, `apps/portico-server/server/internal/access/enforcement.go:214,251–256`) differs from profile all-ratings age rule (`apps/portico-server/server/internal/catalog/restrictions.go:91–97,106–129`). Unknown-as-unrated is explicit in the ladder comment. | Defect / Medium | Unify rating semantics or clearly separate guarantees; **M**, access limits and catalogue restrictions/UI. |
| B-08.2 | **Confirmed**: unknown generic errors inherit 400 (`apps/portico-server/server/internal/httpapi/router.go:247`), logging requires ≥500 (`:556–559`); administration defaults to 500 (`apps/portico-server/server/internal/httpapi/administration.go:33`). | Defect / Medium | Name client-input errors; default unknown causes to logged 500; **M**, failure mapper and legacy input callers. |
| B-08.3 | **Confirmed, narrower**: duplicate stream cancels old and returns 409 (`apps/portico-server/server/internal/eventfeed/feed.go:118–122`); client sets Infinity (`apps/portico-server/packages/client-core/src/playback-v1/events.ts:147`). **stop/start** resets failures (`:82–87`), so object recreation is unnecessary; SSE not wired by first-party openStream options. | Defect / Low today | Retry event_stream_exists with retained cursor rather than permanent fallback; **S**, shared events client. |
| B-08.4 | **Confirmed**: no initial SSE ID (`apps/portico-server/server/internal/httpapi/events_routes.go:181–185`); no-cursor feed starts at current max (`apps/portico-server/server/internal/eventfeed/feed.go:149–152`); client cursor changes only on delivered envelope (`apps/portico-server/packages/client-core/src/playback-v1/events.ts:102`). SSE consumers may miss the initial disconnect gap; polling receives nextAfter. | Defect / Low today | Send and accept an opening baseline cursor; **S**, event stream and parser/client. |
| B-08.7 | **Confirmed, narrower**: single-XFF limiter falls back on chains (`apps/portico-server/server/internal/httpapi/rate.go:231–236`); socket-only setup subjects share proxy budgets (`apps/portico-server/server/internal/httpapi/onboarding.go:98–103`). `SetupLimit` is a ten-minute window (`apps/portico-server/server/internal/identity/quick_connect.go:123`). Direct password sign-in skips that legacy DB login counter; it still uses cheap/source limits. | Defect / Medium | Canonical trusted client address everywhere; **M**, rate/locality/setup subjects. Duplicate B-15.3/6 and B-09.24. |
| B-08.10 | **Confirmed**: general request parser reads retryable (`apps/portico-server/packages/client-core/src/index.ts:400–405`), registry writes retry (`apps/portico-server/apikit/apierror/error.go:40,128`); event errors remain legacy-shaped (`apps/portico-server/server/internal/httpapi/events_routes.go:20–23`). This establishes wrong classification, not automatic retry of every operation. | Defect / Medium (Low–Medium → Medium) | Shared retry/envelope normalization; **S**, client-core API parser/contracts. Backlog generated-contract migration is related. |
| B-15.3 | **Confirmed, narrower**: socket-only subject for legacy /sessions, Portico assertion and TV/invitation budgets; 600-second counter in `apps/portico-server/server/internal/identity/quick_connect.go:123`. First-party direct password route is guarded differently (`apps/portico-server/server/internal/httpapi/direct_identity.go:95`), so not all password logins hit the shared DB budget. | Defect / Medium | Same fix as B-08.7; **M**, setupPeerSubject and callers. |
| B-15.5 | **Confirmed**: `apps/portico-server/server/internal/identity/quick_connect.go:106–115` serializes plain JSON, `apps/portico-server/server/internal/identity/refresh.go:114–126` stores live response, `apps/portico-server/server/internal/identity/two_factor.go:423–431` reverses base64/JSON, and backup copies DB (`apps/portico-server/server/internal/backup/backup.go:398`). Explicit plain-state spec §6 and user instructions govern over conflicting encryption amendments. | Deliberate trade-off / Medium | Explain backup credential sensitivity, warn/offer permissions, preserve receipt expiry; **S**, backup/identity docs and diagnostics. No mandatory encryption change. |
| B-15.6 | **Confirmed**: environment-only/single-XFF recovery (`apps/portico-server/server/internal/httpapi/onboarding.go:18–40`), combined-proxy/right-to-left locality (`apps/portico-server/server/internal/httpapi/request_locality.go:38–45,119–141`), single-XFF cheap limiter (`apps/portico-server/server/internal/httpapi/rate.go:223–238`). | Defect / Medium (Low–Medium → Medium) | Same canonical parser fix as B-01.14/B-08.7; **M**. |
| B-15.7 | **Confirmed**: refresh proves bearer plus installation string (`apps/portico-server/server/internal/identity/refresh.go:22–24,60–61`), not possession of a device private key; `portico-internal/Project Architecture/02 Server.md:52` wrongly promises one. Tokens do have a durable device/family binding. Plain-HTTP interception is an accepted transport risk. | Documentation error / Low (conditional Medium → Low) | Correct “registers a key” guarantee; **S**, architecture/API docs. Cryptographic sender constraint would be a separate **L** capability decision. |
| B-15.9 | **Confirmed** mechanism: five attempts/challenge (`apps/portico-server/server/internal/identity/two_factor.go:329,378`), probe does not check cool-off (`apps/portico-server/server/internal/identity/credential_security.go:296–297`), failures increment it; new challenge password step checks source cool-off. No sustained attack measured. | Deliberate trade-off / Low (Low–Medium → Low) | Preserve no-lockout policy; consider source code-step pacing/security notices; **S**, factor challenge/cool-off. |
| B-15.10 | **Confirmed, narrower**: same hash-slot mechanism as full B-01.15; lower per-address limits and credential cool-off precede hashing. No starvation measurement. | Design risk / Low (Low–Medium → Low) | Measure then bounded cancellable wait; **S**, identity hash admission. |
| B-15.13 | **Confirmed** with existing primary-profile qualifier: TV issuance uses current role and demotes subordinate/viewer-only profiles (`apps/portico-server/server/internal/identity/quick_connect.go:539,563–568`); approved code cannot be collected without device proof. mDNS broadcasts code **and public key**, not poll secret (`apps/portico-react-native/apps/native/Native/PorticoNetwork.swift:173–180`). Owner-as-owner linking is recorded in `portico-internal/Project Architecture/02 Server.md:53`. | Deliberate trade-off / Low (Medium → Low) | Clear approval identity/role copy and keep security notice; **S**, approval UI/i18n. Viewer-default would change the recorded role choice. |
| B-15.22 | **Confirmed**: claim/certificate gate and availability fallback (`apps/portico-server/server/internal/httpapi/secure_connections.go:149–175`), API-only/loopback exemption (`:178–180`), 403 rather than redirect (`:190`). Request may have transmitted credentials before refusal; plain HTTP is deliberately available. | Deliberate trade-off / Low (Low–Medium → Low) | Explain effective requirement and avoid browser password submission when required HTTPS is available; **S**, connectivity/sign-in copy and transport selection. |

All remaining quick-check rows have no exact Backlog item except B-08.10’s related contract migration. Classes describe the planning consequence: a confirmed code mechanism is not automatically a confirmed product defect.

## Not re-checked (Low)

Copied from the worklist; these entries were not independently re-checked. Incidental duplicate references above do not change their coverage status.

- **B-01.11** — Some common rating spellings are read as "unrated"
- **B-01.13** — Invite-only self-registration can never succeed (latent)
- **B-01.16** — Profile PIN is checked while holding the database's single writer
- **B-01.17** — The installation id is treated as a secret but is shown and sent in URLs
- **B-01.18** — Ended session records are never deleted
- **B-01.19** — Some revocations skip the playback clean-up hook
- **B-01.20** — Refusals that answer 401 where the project's own rule says 403
- **B-01.21** — A restricted profile can do account-level things
- **B-01.22** — Left-over code from earlier identity designs
- **B-08.5** — The advertised long-poll maximum is 55 seconds; the server accepts 25
- **B-08.8** — Grid ETags are invalidated by any write anywhere and by token renewal
- **B-08.11** — Toolkit pieces the documents describe are not what the server runs
- **B-08.12** — If-None-Match, ETag and encoding nits
- **B-08.16** — Unauthenticated build fingerprint
- **B-08.17** — The realtime lane is one pool for every long-lived connection
- **B-08.18** — Slow uploads other than avatars and artwork are cut at 15 seconds
- **B-15.8** — The server's own web app is refused behind a TLS-terminating proxy unless an environment variable is set
- **B-15.14** — Remembered-account lookup is unauthenticated and keyed by a value sent in every request
- **B-15.15** — Version and build fingerprint are public
- **B-15.21** — Profile PIN attempts are counted per device, and a new sign-in is a new device
- **B-15.16** — The main CSP allows scripts from all of www.gstatic.com
- **B-15.17** — The release-feed fetch has no network policy
- **B-15.18** — The folder picker checks the path before resolving symlinks, not after
- **B-15.19** — Supply-chain loose ends
- **B-15.20** — The built-in provider credentials are a shared dependency
- **B-15.23** — TV-code and invitation routes sit outside the cheap limiter
- **B-15.24** — Small output-encoding nits

## New findings

### N-V01.1 Learned signer revocation does not retire current Portico-admitted families

- **Claim:** Learning that a Hosted documents signer was revoked rejects future assertions, but does not automatically revoke the server-native sessions already admitted by that signer. The current admission model lost the old signer-to-family invalidation path.
- **What I traced:** `apps/portico-server/server/internal/hosted/trust.go:82–95` learns revocations, then revokes only `authority='hosted'` families selected through `hosted_policy_signers`. Production Portico sign-in now issues **local** families (`apps/portico-server/server/internal/identity/portico_accounts.go:101–105` → `apps/portico-server/server/internal/identity/direct_accounts.go:299`) and stores only a boolean family admission marker (`apps/portico-server/server/internal/identity/portico_accounts.go:110–115`); `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:3228` defines that marker without signer provenance. Current `apps/portico-server/server/internal/identity/families.go:195–205`, `apps/portico-server/server/internal/identity/transaction_auth.go:39–42`, and `apps/portico-server/server/internal/identity/direct_policy.go:84–102` validate local membership/profile/epoch rather than Hosted signing-key status. `apps/portico-server/server/internal/hosted/service.go:160–163` routes local library authority to DirectAllowed. `apps/portico-server/server/internal/identity/refresh.go:80–93` extends local horizon without the Hosted verifier. Repository-wide key-revocation/admission searches found no alternate current-family invalidation.
- **Verdict:** **Confirmed by independent source trace**. Existing live local families continue after the server learns the compromised admission signer’s revocation unless another explicit session/account/device revocation intervenes. This is not just slow fleet propagation. Certificate expiry does not by itself expire a server-native family admitted earlier, and active refresh can extend the local horizon. It does not affect unissued assertions, which are correctly refused.
- **Class:** **Defect** in emergency signer-revocation coverage for the current admission model. Keeping ordinary sessions independent of Hosted outages is intentional; retaining authority after a known compromised admission signer is a different incident boundary. Justin should confirm the intended session-retirement policy before implementation.
- **Severity:** **High**, conditional on signer compromise: an attacker who has already obtained an eligible member/owner session can retain it after the server has learned revocation. Local MFA/approved-device/PIN guards discussed under B-15.12 still constrain initial issuance.
- **Fix direction:** Persist verified documents-signer provenance at issuance, inherit it through profile/TV family descendants, and atomically revoke affected Portico-admitted families when that key is learned revoked. Do not revoke unrelated direct/recovery families or require continuous Hosted calls. **Size: M**; `apps/portico-server/server/internal/hosted/trust.go`, assertion/admission adapter, identity family issuance/selection/quick-connect, migration and revocation cases.
- **Related:** B-15.12’s “until each server hears otherwise” bounds **new forgery admission**, not persistence of already issued current-model credentials. B-01.22 records remaining legacy authority branches but did not identify this consequence. Not in Backlog.
- **Cheapest later check:** Disposable current-claim fixture: sign in an enabled linked account with certified signer A, learn a root-signed revocation of A using signer B, then authenticate/refresh the first local family. Continued success confirms the source consequence; revocation would reveal a missed current-model guard. No such runtime/test execution occurred in this review.

## Disagreements worth Justin’s attention

- **Keep B-01.2/B-15.1 as one High defect.** It is a real exposed-control enforcement gap, not a no-lockout/password choice.
- **B-08.1 High → Medium**, with B-15.8 treated as its duplicate: a configurable optional proxy deployment blocker, with explicit allowed-origin workaround.
- **B-01.15/B-15.10 → Low design risk**: two slots and instant busy are real, sustained starvation is unmeasured and guarded by more than the network ceiling.
- **B-15.4, B-15.5, B-15.9, B-15.13 and B-15.22 are recorded trade-offs**, not release defects merely because the residual security risk exists. B-15.7 is chiefly a false cryptographic-binding document promise; logically device-bound sessions do exist. Broader Product Contracts encryption/transport language conflicts with the explicit open-state/HTTP direction; this audit follows Justin’s current instruction and the plain-state spec.
- **B-01.23 stays unresolved.** Current single-profile viewer cleanup is guarded; do not implement the auditor’s hypothetical fix as though that historical cause was demonstrated.
- **B-01.5/B-15.2 do not halt all identity maintenance.** Fix setup pruning promptly, without attributing every retained identity row to it. **B-01.7 does not make known-ID member approval impossible**; it lacks discoverability and robust owner recovery.
- **B-08.9/B-08.13 Medium → Low** latent risks: no failing or unprotected endpoint identified; omitted/nullable fields and Device binding were understated. Keep the safeguards and correct the guarantees.
- **B-08.14 is the specified revision fence**, not a faulty continuation. A stable large export would be a separate feature. B-01.12 is an owner-history/erasure policy conflict rather than an accidental failure to delete the owner’s audit record.
- **N-V01.1 changes incident planning:** distributing a signer revocation does not currently retire its already issued server-native families.

## Evidence boundary

Review date: 11 October 2026, on personal Ubuntu host `ehler-homelab-mini-1`. Working-tree source was reviewed as-is, not a clean-release reconstruction. HEADs observed: portico-server `82aaa54f89efdc2e029b635746627cd8c8b71960`, portico-react-native `a3d8e7cd14c16229715a07c67f43cb8364bae743`, portico-internal `19cc2a7eb6fb40ce675decd65036769c95b42c5d`. These identify checkout heads; they do not assert that every reviewed file equals its committed version.

Read the verification brief/worklist and original shared brief in full, plus domain findings, applicable identity/onboarding contracts, API style guide, plain-state backup specification, architecture/decision extracts and the full dated Backlog. Searched the B-index for related claims. Architecture and Product Contracts contain competing encryption, HTTP, owner-history and device-key statements; the report distinguishes explicit owner choices from implementation guarantees. Shared context refresh failed because its lock is outside writable roots; the existing canonical guidance was readable.

No Portico source/state changes, builds, suite runs, formatter/install commands, browser/computer-use/MCP operations, Hosted/demo access, or subagents. No throw-away runtime experiments were needed or run. Only this designated output was created/edited under Portico; source/tests were read. All 12 Tier 1 and 26 Tier 2 IDs were reached; 27 Low IDs were copied without re-check. Duplicate IDs have separate verdict-table rows and shared explanations where appropriate.

Static evidence establishes wiring, conditional logic, database shapes and client decisions. It does not establish browser DNS-rebinding exploitability, actual address presentation by a particular container/proxy, starvation/performance thresholds, real player MIME rejection, or the cause of the historical Apple incident. Cheapest follow-up experiments are specified with the affected findings. No statement here claims runtime reproduction or a completed security acceptance gate.
