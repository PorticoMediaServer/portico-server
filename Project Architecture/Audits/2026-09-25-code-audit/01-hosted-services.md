# Hosted Services, DNS, API toolkit and deployment — static code audit

Date: 2026-09-25. Baseline: internal repository `33ce964`. Scope: `hosted-services/{hosted,dns,apikit}`, `deployment`, `scripts`, and Cast receiver deployment integration. All paths in this report are relative to `/Users/justin/Documents/Codex Projects/Portico/portico-internal` unless marked otherwise. Line ranges identify the inspected checkout, not a live installation.

**Evidence boundary:** source reading, searches, source inventory and test-source review only. No application, test, build, service, provider request, network request, database query, migration, deployment, restore or exploitation was executed. No application code changed. Private deployment information and secret/configuration contents were not inspected. A confirmed finding means the code contains the described behavior and the stated trigger follows from its call path; it does not mean it was observed in production.

## Assessment

Second-pass review: all nine entries were challenged by the primary reviewer in `10-adversarial-primary-reviewer.md`. Conditions and recovery/Cast wording were narrowed; no core Hosted finding was rejected.

The implementation has substantial security structure: current account/family authority, operation-bound claims, constrained provider and certificate workflows, encrypted durable receipts, bounded workers, and authenticated DNS distribution. Several failures sit between those components rather than within a single validator. The highest-priority issue is that the native OAuth completion proof is optional for the caller: the older public path retains the account-session phishing condition the newer proof is designed to prevent. The DNS liveness command also opens mutable persistence while another process owns it. Reliability findings affect lost-response recovery, server discovery, repeat membership changes, and the restore drill.

| ID | Priority | Classification | Confidence | Summary |
|---|---|---|---|---|
| HS-001 | P1 | Confirmed security defect | High | Native browser OAuth can opt out of callback-possession proof |
| HS-002 | P1 | Confirmed unsafe persistence path; race-dependent impact | High | DNS healthcheck opens and repairs the daemon's live journal |
| HS-003 | P2 | Confirmed reliability defect | High | Native OAuth MFA completion cannot recover a lost challenge response |
| HS-004 | P2 | Confirmed retention/recovery defect | High | Refresh token pruning cuts off a still-valid recovery receipt |
| HS-005 | P2 | Confirmed cache invalidation defect | High | Server-list validators miss shutdown, installation and clock transitions |
| HS-006 | P2 | Confirmed discovery reliability defect | High | A second member-label rotation cuts short the first label's grace |
| HS-007 | P2 | Confirmed operational verification defect | High | Restore drill's synthetic sign-in lacks a required continuation header |
| HS-008 | P2 | Conditional deployment risk | High on code; deployment-dependent impact | Failed activation leaves the new release selected; migration can prevent binary rollback |
| HS-009 | P2 | Confirmed policy incompatibility, feature/deployment conditional | High | Hosted web CSP blocks the configured Cast sender SDK |

No P0 finding is established. Severity reflects the stated trigger and blast radius. HS-001 requires a victim to complete provider sign-in through an attacker-initiated authorization flow; an account with MFA still requires its factor. HS-002 requires overlapping mutable table use for corruption/loss; merely running one healthcheck without overlap is not proof of loss.

## Confirmed defects

### HS-001 — Native OAuth callback-possession protection is optional to an attacker

**P1 · High confidence.** Sources: `hosted-services/hosted/internal/postgres/oidc_flows.go:25–30,58–62,185–192,228–243`; `internal/postgres/oidc_login.go:120–140,203–215`; `internal/postgres/oidc_complete.go:78–100,218–258`; `internal/httpapi/account_security.go:33–65`. The latter abbreviated paths are beneath `hosted-services/hosted/`.

**Trigger and evidence chain.** A native browser-based provider-login start accepts `completion` omitted. Validation accepts both the empty string and the new `code` value; only the latter sets `identity_transactions.native_completion=true`. A non-browser caller can generate a valid random `X-Portico-Continuation` and start the flow without a session. The provider authorization URL is bound to a server-held state/nonce/PKCE verifier, but callback binding comparison is explicitly conditional on `session_mode == browser`. A victim can therefore authenticate to the legitimate provider through the attacker's native authorization URL. The verified provider assertion is stored under the attacker's transaction and binding. Both withholding account/onboarding information from polling and requiring the callback-delivered completion code are conditional on `NativeCompletion`. With the flag false, the initiating party can poll and complete. For an existing linked account without MFA, `CompleteFlow` creates and returns its session.

**Impact.** Provider-login phishing can yield the victim's Portico session to the party that initiated the transaction. Correct nonce, signature, issuer, audience and PKCE checks do not establish that the party polling/completing is the browser user who authenticated. The source comment at `oidc_flows.go:230–234` expressly recognizes that distinction, but only enforces it for an opted-in flow.

**Counterevidence and limits.** The new code-return path has the relevant binding/code checks, browser mode compares the callback cookie binding, and MFA-enabled accounts still receive an MFA challenge. This finding does not claim MFA bypass, forged provider tokens, a browser-mode bypass or a native Apple assertion bypass. Upgrading all official clients to request `completion: code` does not close a public API opt-out.

**Test evidence.** `hosted-services/hosted/internal/postgres/oidc_login_test.go:74–154` tests the attacker-initiator condition only with `Completion: NativeCompletionCode`. Its separate legacy-compatibility test at `174–226` expressly requires details and an existing account session to be returned without a completion code. These source assertions confirm the opt-out is intentional compatibility behavior, not a hypothetical untested parser variation. Neither test was run.

**Fix.** Require a callback-possession proof for every native browser OAuth login that can produce an account session. Reject the legacy insecure mode at the server boundary or migrate it to an equally strong protocol; compatibility cannot keep the security invariant optional. Preserve separate native Apple assertion semantics and current-family fences for link/reauthentication.

**Suggested verification, not executed.** Start a native flow without the flag and confirm rejection or mandatory proof. Simulate the initiating caller and authenticating browser as independent parties; give the initiator its binding, transaction ID and authorization URL but not the callback-return secret. Poll and completion must disclose no identity/onboarding secret and mint no session. Exercise Google and browser-based Apple, new/existing accounts, MFA/non-MFA, expiration, cancellation, provider disablement, and genuine same-device completion.

### HS-002 — DNS liveness probes mutate the serving process's persistence

**P1 · High confidence on unsafe concurrency; race-dependent loss.** Sources: `hosted-services/dns/cmd/portico-dns/main.go:57–82`; `hosted-services/dns/directdns/table_state.go:115–137,322–367,370–388`; `scripts/deploy-dns-release.py:47–51`; `scripts/rebuild-dns-after-hosted-restore.py:19–28`.

**Trigger and evidence chain.** `dns-healthcheck` calls `OpenTable` against the configured/default state file before dispatching to its UDP/TCP SOA probe. `OpenTable` performs startup recovery: it removes unreferenced rotation journals, opens the referenced journal read/write, and truncates an incomplete trailing record. Meanwhile the serving daemon's `Compact` creates and adopts a new journal, releases its process-local mutation lock, and serializes/persists a snapshot that will reference that journal. During this interval, the old on-disk snapshot still references the old journal. A second process running the healthcheck can delete the daemon's new active journal as supposedly unreferenced. It can also observe a concurrent partial append as a crash tail and truncate it. Go mutexes do not coordinate these processes.

**Impact.** A liveness probe can discard newly journaled rows or cause a snapshot to reference a removed journal. Serving memory may continue to look healthy; a subsequent restart can recover an incomplete table and depend on Hosted/feed availability to repair it. If later feed batches advance the durable sequence past a deleted earlier batch, replay from that advanced sequence need not recover all lost rows. The important failure is that the healthcheck can alter state it purports only to inspect.

**Counterevidence and limits.** Normal single-owner crash recovery intentionally deletes orphaned journals and repairs incomplete tails. The separate `dns-readiness` branch exits before opening the table and does not have this problem. A startup deployment check ordinarily happens well before the daemon's periodic compaction, so the reviewed deploy script alone does not establish that the race occurs on every deploy. Overlapping use is needed, and feed reconstruction may repair some outcomes. No live race was attempted. `hosted-services/dns/directdns/journal_test.go:100–163` exercises concurrent apply/compaction through one table instance, then reopens only after churn ends. That source coverage does not model a second process opening live state.

**Fix.** Dispatch read-only health commands before opening persistence; the apex SOA probe needs no table rows. Add an exclusive interprocess state-owner lock to mutable serving/rebuild/recovery entrypoints so maintenance cannot open live storage as an independent owner. Keep health/readiness free of repair side effects.

**Suggested verification, not executed.** Force slow compaction and journal appends while invoking many independent healthcheck processes. Assert health does not open state writeably, unlink files or change bytes. Restart the owner after each interleaving and compare full table/sequence with accepted batches. Verify a second serving/rebuild process fails safely on the ownership lock.

### HS-003 — A lost native OAuth MFA challenge response cannot be replayed

**P2 · High confidence.** Sources: `hosted-services/hosted/internal/postgres/oidc_complete.go:78–100,218–245`; `hosted-services/hosted/internal/postgres/oidc_flows.go:228–271`; `hosted-services/hosted/internal/httpapi/oidc.go:200–218`.

**Trigger and evidence chain.** In a native `completion: code` login for an MFA-enabled account, `CompleteFlow` consumes the callback code and durably stores an MFA challenge, then commits and returns that challenge. The transaction remains in `verified` state until factor completion. If the HTTP response is lost, an exact retry first reaches the completion-code check. Its only already-consumed replay exception requires `r.Status == completed`, so the retry fails before the existing `mfa_challenge_cipher` recovery branch can run. Polling exposes the MFA-required state, not the challenge secret required to proceed.

**Impact.** A routine lost 202 response forces a fresh provider flow, despite the durable challenge and retry-oriented protocol. This is availability/recovery failure, not MFA bypass.

**Counterevidence.** Completed non-MFA sessions do have an explicit stored-receipt replay path; the MFA branch also stores and can deserialize the challenge, but the earlier consumed-code guard makes that branch unreachable on this retry. A response received normally can proceed.

**Fix.** Permit exact-proof readback of the already-committed MFA challenge under the original binding, transaction, provider/account epoch and challenge validity checks. Do not issue a new challenge or reset attempt budgets on retries. Treat cancelled, expired or revoked authority as terminal.

**Suggested verification, not executed.** Drop the first MFA-required completion response after commit, repeat the identical request, and require the identical live challenge. Verify different code/binding, revoked provider identity, cancelled flow, exhausted factor attempts and expired challenge cannot recover authority. Finish the recovered challenge and verify a single session family.

### HS-004 — Retention removes the lookup needed by a live refresh receipt

**P2 · High confidence.** Sources: `hosted-services/hosted/internal/postgres/maintenance.go:69–80`; `hosted-services/hosted/internal/sessions/service.go:65–85`; `hosted-services/hosted/internal/postgres/refresh.go:42–48,91–118`; `hosted-services/hosted/internal/postgres/refresh_test.go:418–456`.

**Trigger and evidence chain.** A refresh rotates successfully but its response is lost. The client retains the predecessor refresh token and request ID. The stored exact receipt is intended to remain recoverable until its successor is used or the family horizon expires; the code documents this at `refresh.go:107–113`. Maintenance nevertheless deletes every consumed refresh-token row older than one day, without excluding the predecessor referenced by the current recoverable receipt. Every refresh request first calls `LookupRefresh`, which derives the family exclusively from `refresh_tokens`. After pruning, it returns `ErrGrant` before receipt recovery can run.

**Impact.** A client returning after a multi-day outage can be forced to sign in again while its family and exact recovery receipt remain valid. The retention policy breaks the longer recovery guarantee and its test's stated intent.

**Counterevidence and test gap.** `TestRefreshReceiptOutlivesAnOutageUntilTheSuccessorIsUsed` checks that receipt expiry exceeds one day and expires the access row, but never ages the consumed token or invokes pruning. That test does not cover this cross-component interaction. Exact retries before retention, or with a successfully received successor, need not fail.

**Fix.** Retain predecessor lookup data for every still-recoverable receipt, with bounded lifetime tied to that receipt/family, or securely resolve the current receipt without the pruned row while preserving family, epoch, mode and reuse checks. Keep retention and reuse-detection semantics explicit.

**Suggested verification, not executed.** Rotate, discard the response, age the predecessor past 24 hours, run the real retention path, and retry the exact token/request ID. Require identical tokens. Then use the successor and verify predecessor reuse still has the intended revocation behavior. Repeat with expired/revoked families and account epoch changes.

### HS-005 — Server-list ETags and watch versions omit observable lifecycle changes

**P2 · High confidence.** Sources: `hosted-services/hosted/internal/httpapi/handler.go:108–136`; `internal/directory/server_list.go:182–198,218–237`; `internal/postgres/directory.go:12–41`; `internal/postgres/server_events.go:29–46`; `internal/postgres/claim_cancel.go:174–185`; `internal/postgres/claim_commit.go:153–155`; `internal/postgres/server_operations.go:16–35`; `internal/postgres/direct_route.go:234–255`. Abbreviated paths are beneath `hosted-services/hosted/`.

**Trigger and evidence chain.** A client holds an ETag/watch version after loading its server list. The handler answers 304 from the in-memory account version before querying the representation. Several changes to that representation do not advance it:

1. The shutdown event clears `base_url`, marks the endpoint unverified, expires presence and deletes `current_server_endpoints`, but unlike the adjacent rename branch never calls `touchServerListTx`.
2. Claim finalization bumps the list while the operation is `committed_pending_install`; the list query requires `installed`. If the client reads between finalization and installation acknowledgement, it can cache a list without the server under that version. `Acknowledge` changes the operation to installed without another bump.
3. The list computes active/quiet/dormant from the current clock. Presence expiry and the 90-day dormant cutoff therefore change the response, and default-list membership, without any version update. Heartbeat timestamps/presence are also observable in the list but are not consistently represented by this structural version.

**Impact.** Conditional readers and watch-driven clients can retain a disappeared endpoint, omit a newly installed server, or keep obsolete presence indefinitely until a different versioned event or an unconditional refresh. This undermines the discovery/presence contract; it is not evidence that a stale endpoint bypasses authorization.

**Counterevidence.** The underlying list SQL is authority-scoped and evaluates current installed state. An unconditional fetch gets its current representation. Many mutations, including rename, do bump versions, and the follower has notification plus periodic recovery. Those controls cannot recover a version that was never advanced. A later member/endpoint push may incidentally repair the installation case.

**Fix.** Advance affected accounts after shutdown and after installation becomes visible. Separate structural and presence validation if needed, or incorporate the next presence transition deadline into validator/readiness decisions. A cache validator must describe the returned representation, including time-derived fields and filtering. Apply the same semantics to cheap watch responses.

**Suggested verification, not executed.** Cache a real list and compare conditional GET/watch against unconditional GET through shutdown, finalize-before-ack, ack, heartbeat reconnection, presence timeout and dormant cutoff. Require changed responses whenever representation or visibility changes. Include another account that lacks membership to verify the invalidation fix preserves boundaries.

### HS-006 — Two member-label rotations overwrite an unexpired grace label

**P2 · High confidence.** Sources: `hosted-services/hosted/internal/postgres/migration_0014.go:3–25`; `internal/postgres/member_label_rotation.go:22–50`; `internal/postgres/migration_0017.go:75–77`; `hosted-services/dns/directdns/server.go:432–452`.

**Trigger and evidence chain.** Membership removal queues rotation; the worker consumes that queue and rotates the name. Each rotation replaces `previous_member_label` with the current label and gives it 24 hours. Two separately processed removals inside that period yield A → B → C. After the second, the one previous-label slot contains B, so DNS rejects A immediately although A's original 24-hour interval has not elapsed. Pending removals can coalesce, but removals after the queue is consumed produce another rotation.

**Impact.** Remaining authorized members or a server still using A lose discovery before the promised route handover period. Reconnection or new DNS resolution can fail until discovery/check-in updates the endpoint; existing established TCP connections need not immediately fail. This can occur during normal sequential membership administration.

**Counterevidence.** The current implementation fixes the single-rotation case, wakes the server after rotation, and enforces policy independently of the label. Removed users are not granted media access by grace DNS. Faster refresh reduces the impact but does not uphold the 24-hour guarantee under repeated changes.

**Fix.** Retain all bounded, unexpired grace labels, or design rotation coalescing/epochs that preserve the documented minimum handover interval. Bound storage and DNS acceptance deliberately; continue immediate authorization revocation regardless of name grace. Ensure endpoint/list propagation accompanies the new label.

**Suggested verification, not executed.** Process two and several separate removals within 24 hours. Assert every still-promised grace name resolves until its own expiry, new names reach remaining members, and removed members cannot access media using either old or new hostnames. Include restart/feed reconstruction.

### HS-007 — The restore drill cannot perform its synthetic sign-in

**P2 · High confidence.** Sources: `scripts/hosted-restore-test.sh:44–53`; `hosted-services/hosted/internal/httpapi/handler.go:232–255`; `hosted-services/hosted/internal/httpapi/account_security.go:35–65`.

**Trigger and evidence chain.** After restore and readiness, the drill sends JSON to `/v1/sessions` with only `Content-Type`. Login requires `continuation(..., true)`. With neither native continuation header nor browser Origin/cookie, this branch returns 403 `continuation_required`. The private sign-in JSON file cannot add an HTTP header. Thus even otherwise-valid synthetic credentials fail this mandatory final step.

**Impact.** The checked-in end-to-end restore verification always fails at authentication under the current request contract; operators cannot use its advertised synthetic-sign-in/RTO success output as written. This is a test-tool defect and a recovery acceptance gap, not proof that backup data cannot be restored.

**Counterevidence.** Restore isolation, outbound-worker suppression, replayed WAL, bounded readiness polling and recovery-point checks are intentional controls. They do not satisfy the updated login protocol. Also, after fixing the header, merely accepting any 2xx would treat an MFA-required 202 as successful full sign-in.

**Fix.** Generate and send a private native continuation binding and a valid idempotent request ID, or implement a proper browser flow. Parse the response and verify a usable authenticated session against the restored service. Either use a documented non-MFA synthetic account or complete its factor flow; do not call a challenge a signed-in session.

**Suggested verification, not executed.** On an isolated restored fixture with outbound work disabled, run the corrected drill against current handlers, validate a follow-up authenticated read, and confirm invalid credentials/MFA-only responses fail. Verify cleanup still runs on every failure. No drill was run during this audit.

## Conditional deployment findings

### HS-008 — Conditional recovery gap after failed activation or schema-changing release

**P2 · High confidence on implementation; conditional on deployment failure/schema change.** Sources: `scripts/deploy-hosted-release.py:60–91,92–101`; `hosted-services/hosted/cmd/hosted/main.go:143–157`; `hosted-services/hosted/internal/postgres/current_schema.go:12–28`.

The script intentionally activates a previously reviewed release; lack of automatic rollback alone is not a security defect. The operational gap is the absence of a demonstrated recovery path after the following failures. The deployment script records prior release names for retention, overwrites the active systemd unit definitions and restarts, then raises if readiness/build identity fails. It does not restore previous units or service selection. Startup commits database migration before later ACME configuration validation. An otherwise reviewed new release can migrate the database and then fail later startup validation; a previous binary with a lower schema version explicitly rejects that now-newer schema. Retaining the previous directory alone is therefore not a working rollback mechanism. The same script recreates argument lists rather than preserving every active operator option; for example the application supports retired issuer configuration but the generated arguments do not carry it.

**Impact and limits.** The failure leaves service availability dependent on manual intervention. An ordinary binary swap may not recover a migrated database. This report does not claim a specific deployed unit has custom options, an actual migration is incompatible, or a failed deployment has occurred. The script says releases must be reviewed/tested first, and an operator could have an external recovery process not visible in code.

**Fix.** Define a concrete forward/backward schema compatibility window; preflight all non-mutating configuration before migration; snapshot active units/arguments and select a staged release atomically only with a known recovery path. For irreversible migration boundaries, require a reviewed roll-forward procedure or explicitly coordinated restore rather than an automatic unsafe database rollback. Preserve required operational options and include web readiness in the activation decision.

**Suggested verification, not executed.** In isolated fixtures inject failures before migration, after migration, on readiness and on web activation; demonstrate the documented recovery path and preserve the exact prior units/options. Test a real N→N+1 migration followed by attempted N recovery, including data written by N+1. Define recovery-time and data-loss limits before production acceptance.

### HS-009 — Hosted web CSP blocks the configured browser Cast sender

**P2 · High confidence; conditional on Cast configured and this policy installed.** Sources: `deployment/hosted/web.caddy:1–3`; `deployment/hosted/Caddyfile:43–55`; `/Users/justin/Documents/Codex Projects/Portico/apps/portico-server/web/src/bridge/cast.ts:18,30–40,59–70` (cross-repository integration source, independently checked and corroborated by the parent audit).

The web policy permits scripts only from `'self'`. The Cast bridge injects the Google Cast SDK from `https://www.gstatic.com/...`, and preparation awaits SDK availability before creating the sender. Therefore a Cast-enabled Hosted web client served under this checked-in policy cannot load that SDK. The receiver's separate CSP does not relax the sender page's policy.

**Counterevidence and limits.** The application ID can come from the connected server's `/v1/cast/configuration` before falling back to a build environment variable; absence of the latter alone does not establish that Cast is disabled. The issue is conditional on a usable application ID and actual installation of this Caddy include. No live response headers or receiver registration were inspected. An already-provided Cast framework skips this script load. Fixing the sender CSP alone cannot make the current Cast flow work: SV-008 and SV-012 identify separate issuance/receiver contract failures.

**Fix.** Deliberately allow the necessary official SDK script origin in the sender CSP, with the narrowest policy compatible with the integration, and check any SDK-required connections separately. Keep unrelated sources restricted.

**Suggested verification, not executed.** Serve a configured sender with the release CSP; verify SDK load and Cast session creation in a real browser, including CSP violation logs, receiver launch and playback. Repeat with Cast unconfigured and with denied/unexpected script origins.

## Additional conditional risks and open questions

These are not represented as proven production defects or separate confirmed vulnerabilities. They require workload, provider-policy or deployment evidence before promotion to prioritized defect findings.

- **Version-follower completeness under long transactions/high write rate.** `hosted-services/hosted/internal/directory/server_list.go:49–59,82–126` rereads only 10,000 sequence values behind the observed high watermark to catch late commits. There is no enforced bound proving a transaction cannot commit below that window after 10,000 other advances. A missed account stays stale until its next bump/full reload. This merits a delayed-commit stress test and a commit-ordered durable cursor or reconciliation design if the expected workload can reach the condition. The ordinary short-transaction behavior and trailing reread reduce likelihood.
- **Retention capacity is not proved for every subsystem.** `hosted-services/hosted/cmd/hosted/main.go:464–477` runs main retention until caught up but invokes OIDC/account-security pruning once per ten-minute tick. `internal/postgres/oidc_login.go:236–241` and `internal/postgres/account_security_maintenance.go:5–20` cap each statement at 100 rows, so a sustained arrival rate above ten expiring rows/minute in any such table can outpace that single-instance sweep. These paths should be load-tested separately. Bounded work protects latency only if backlog remains observable and recoverable. No production row counts, worker lag or plans were inspected.
- **Export size and memory ceiling.** `hosted-services/hosted/internal/postgres/account_export_worker.go:57–94` aggregates entire account collections/audit data into one payload. Separate background connection capacity limits database contention but does not bound payload/process memory. `scripts/deploy-hosted-release.py:75–78` caps Hosted memory at 384 MiB. An account with very large retained history needs a streaming/chunking limit and admission/size tests; ordinary accounts are not shown to exceed the cap.
- **Signing and certificate custody rotation.** Retired issuer cleanup support exists in application startup, but release automation's generated arguments do not include that configuration. Clarify how operator overlays retain it across deploys. Key backup/restore and certificate revocation must be accepted with real isolated custody material separately; no secrets were read here.
- **App Store notification trust policy.** The parser verifies a certificate chain and signed envelopes, with configured application/environment checks. A future specialist review should confirm accepted Apple certificate roles/OIDs and revocation expectations against the intended provider policy. No exploit or invalid certificate acceptance is asserted from the general-purpose X.509 usage alone.
- **Shared API toolkit drift.** `scripts/shared-apikit.sh` offers explicit synchronization/drift checking. Confirm release acceptance invokes that gate wherever the shared toolkit is duplicated. Presence of the script is not proof that every release ran it.

## Coverage matrix

Counts are `.go` production/test source files inventoried in the package; they are not a claim of line-by-line review of every file. “Targeted” means substantive call-path review of the listed responsibility; “supporting” means inventory plus representative source/contract review. Test-source reading informs findings; no tests ran.

| Module/package | Source/tests | Depth and responsibilities reviewed |
|---|---:|---|
| hosted/claimweb | 1/0 | Supporting: claim-web protocol and typed handoff boundary |
| hosted/cmd/apigen | 1/0 | Supporting: generated contract entrypoint |
| hosted/cmd/hosted | 1/1 | Targeted: startup, config, migration ordering, workers, pools, readiness, shutdown, restore-drill suppression |
| hosted/cmd/web | 1/1 | Targeted: static hosting, routing/association files, immutable caching and fallback behavior |
| hosted/dev/e2eaccounts | 1/0 | Inventory/supporting only: developer account harness; not executed |
| hosted/dev/localtls | 1/0 | Inventory/supporting only: local TLS harness; not executed |
| hosted/dev/serverlistload | 1/0 | Inventory/supporting only: load harness; not executed |
| hosted/internal/appstore | 1/1 | Targeted: signed notification parsing, trust roots, account/environment association and replay boundary |
| hosted/internal/buildinfo | 1/0 | Supporting: release identity plumbing |
| hosted/internal/certificates | 7/1 | Targeted: admission/authority, ACME provider custody, worker leases, challenge propagation, cleanup/reconciliation and revocation |
| hosted/internal/claims | 5/0 | Targeted: claim request/approval/finalization authority and proof model; repository transitions followed |
| hosted/internal/contract | 2/4 | Supporting: route/schema definitions, fixture and generated-contract consistency tests |
| hosted/internal/device | 2/0 | Targeted: device-code/TV flow authority, polling, grants and receipt boundary |
| hosted/internal/directory | 7/2 | Targeted: authenticated server operation boundary, endpoint probing, member projection, version cache/watch tokens |
| hosted/internal/directroute | 1/1 | Targeted: direct route inputs and current-authority proof |
| hosted/internal/egress | 1/1 | Targeted: constrained outbound targets, address validation, DNS/dial behavior, redirects and response limits |
| hosted/internal/httpapi | 25/14 | Targeted: routing, native/browser credentials, origins/cookies, client IP trust, rate budgets, strict request decoding, owner/profile/server authority, OIDC, lifecycle/export/deletion |
| hosted/internal/identity | 10/3 | Targeted: account/session interfaces, credentials, TOTP/recovery and security intent representation |
| hosted/internal/mailoutbox | 5/1 | Targeted: durable queue, SMTP configuration, lease/retry and delivery lifecycle |
| hosted/internal/oidclogin | 2/2 | Targeted: discovery/JWKS/token verification, nonce/state/PKCE, provider notifications |
| hosted/internal/postgres | 85/38 | Deep targeted: current account/family/claim authority, refresh receipts, provider transactions, MFA/recovery, membership, devices, directory, migrations/constraints, retention, certificate state, deletion/export, audit/notifications |
| hosted/internal/security | 2/4 | Targeted: entropy/hashing, authenticated sealing and key material handling |
| hosted/internal/sessions | 2/0 | Targeted: refresh family lookup/rotation and receipt decryption/recovery |
| hosted/internal/wake | 1/1 | Targeted: bounded outbound delivery, authority refresh and retries |
| hosted/trust | 6/3 | Targeted: root/online signing authority, certificate validation and key lifecycle |
| dns/cmd/portico-dns | 1/0 | Deep targeted: command dispatch, table ownership, feed, health/readiness and rebuild |
| dns/directdns | 5/5 | Deep targeted: authoritative answers, namespace/label gates, proxy/query budgets, journal/snapshot/compaction/recovery and authenticated feed |
| dns/dns01 | 1/1 | Targeted: DNS update authentication/record shape and cleanup |
| apikit root | 2/5 | Targeted registry and strict request shape; supporting code generation/tests |
| apikit/apierror | 1/1 | Supporting: stable envelope/error status contract |
| apikit/contract | 1/0 | Supporting: contract fixture generation boundary |
| apikit/cursor | 2/1 | Targeted: MAC/context binding, expiry and rotating persistent keyring |
| apikit/idempotency | 1/0 | Targeted: request identity and bounded key validation |
| apikit/precondition | 1/1 | Targeted: conditional request syntax/version handling |
| deployment/hosted | non-Go | Targeted: Caddy/CSP/proxy boundaries, systemd generation assumptions, backup/readiness and firewall templates |
| deployment/dns | non-Go | Targeted: daemon permissions/service configuration, dnsdist/BIND role separation, authoritative/update boundaries |
| scripts | non-Go | Targeted: release build/activation, DNS activation/rebuild, backup/key backup, restore drill, shared-apikit gate; ancillary developer scripts inventoried |
| cast-receiver | integration assets/docs | Targeted: receiver deployment/hosting integration and sender CSP cross-check; actual receiver app source is audited in the parent lane |

## Security and reliability controls verified in source

These observations explain why nearby hypotheses were not promoted into findings; they are not runtime certification.

- Session refresh is serialized by family authority, checks account epoch/mode/revocation, seals exact responses and detects predecessor reuse. The issue is interaction with retention, not absent rotation or a plain-text credential table.
- Sensitive account actions use intent-specific recent proof/current-family expectations rather than trusting an account ID from JSON. Deletion rechecks its preview/current authority and explicit confirmation; export retrieval is owner-bound. No general cross-account IDOR was established in the inspected paths.
- Browser credential handling constrains origins and uses HttpOnly continuation cookies; native bindings cannot override a browser cookie/origin. This defends normal browser requests but cannot make a native OAuth opt-out safe.
- Provider verification includes state/nonce and issuer/audience/signature checks, with separate subject-link ownership and revocation checks. Account MFA uses bounded clock windows/replay state and cryptographic recovery values. Provider email is not by itself a universal account-link authority.
- Claim installation/endpoint/server operations carry account, claim generation, credential generation and committed-token identity. Current installed authority is checked in repository operations rather than only at the initial HTTP authentication step.
- Endpoint/wake network work uses constrained egress and bounded clients instead of unconstrained request URLs. Certificate issuance has persisted operations, leases, provider receipt/custody handling and cleanup/revocation paths. An uncertain external issuance is not automatically treated as permission to create unrelated fresh issuance.
- DNS feed sequencing and authentication, atomically published in-memory tables, write-ahead persistence, fsync/rename snapshots and startup recovery are deliberately implemented. The cross-process healthcheck hole does not negate those single-owner controls.
- Migration history and schema identity are checked and serialized rather than blindly rerunning DDL on restart. This protects consistency but makes rollback compatibility an explicit operational requirement.
- Worker pools, retry/backoff, bounded query/HTTP concurrency and cleanup exist. Static review cannot establish query plans, fleet capacity, memory under realistic payloads or whether configured alerts are received.

## Verification priorities and limitations

First close HS-001 at the server protocol boundary and remove mutable DNS initialization from health probes. Then add cross-component regression coverage for MFA/refresh recovery and list validators; these are gaps that unit tests of each component can miss. Correct the restore tool before relying on its recovery acceptance output. Resolve repeat label handover and test configured Cast against the actual release CSP. Separately rehearse release failure and schema recovery in an isolated environment.

Unreviewed or unproven here: live deployment configuration; secret custody and file modes on hosts; DNS delegation, resolver behavior, certificate provider acceptance and real propagation; database contents/query plans/RLS policy in a running instance; actual account/provider/device UI completion; abuse capacity/load/chaos behavior; dependency vulnerabilities against a current advisory database; real backup restore, RPO/RTO and binary rollback; every line of generated code, all ancillary developer scripts, third-party dependency implementation and every permutation in the test suite. Production claims must not be inferred from this static report or from previously green tests/deployments.
