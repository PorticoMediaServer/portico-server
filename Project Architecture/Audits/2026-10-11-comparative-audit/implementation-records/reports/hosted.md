# Outcome

Hosted now supports public-client “Sign in with Portico” without app registration or client secrets. The consent/code exchange returns the existing server/challenge-bound identity assertion, and no Hosted account session or directory credential. The page uses the existing account web sign-in, including its password, provider and MFA paths. It identifies the server, marks the app-supplied name as unverified, and displays the exact return address.

Confirmed Hosted defects were fixed in TV approval authority, device/account lock ordering, cheap abuse admission, runtime signing authority, acceptance of revoked directory consent and signing-key change delivery. The copied API toolkit is already identical to the required canonical worktree and its CI comparison remains active.

All work is source/test work on ehler-homelab-mini-1 (personal host). No deployment, push, commit, production database access, running-service modification, or subagent was used. Context sync failed on its read-only lock; supplied guidance and readable canonical notes were used. The temporary PostgreSQL server was extracted from Ubuntu’s official package into /tmp, never installed, exposed only on an owned Unix socket, and stopped after each test command.

Snapshots: 001 records the app flow; 002 records the next cumulative milestone but captured an unused helper before build completion; that helper was removed immediately. Snapshot 003 is the final cumulative snapshot, supersedes 002, and is buildable. Snapshot directories are under _program/snapshots/hosted. Use the last cumulative snapshot as the final source state; do not sum cumulative patches.

# Decisions I made

- A third-party app receives the same `portico.identity.v1` envelope as the first-party `/v1/servers/{id}/identity` path. Hosted grants no membership, custody, local role, Hosted refresh credential or directory access. The local server still decides admission.
- S256 PKCE only. A 256-bit random authorization code lives at most 60 seconds; reviewed requests live at most five minutes. Exact return URI, verifier, account epoch and browser family are rechecked during the single-use transaction. Denial and spent-code tombstones remain until bounded pruning so retries cannot reset the review quota.
- Numeric loopback HTTP with an explicit port, hierarchical app schemes, and exact HTTPS return URLs are allowed. Credentials/fragments, reserved return parameters, dangerous built-in schemes, malformed ports and ambiguous inputs are refused. Hosted never fetches or follows a return URL. HTTPS addresses may be private because navigation is not Hosted egress.
- Existing `/account` handles sign-in in a separate tab. The consent tab remains open and the person returns there and presses Continue. This reuses every existing authentication method without a second password/SSO implementation or exporting an account bearer token. Review/decision require the canonical account origin, its current HttpOnly cookie and CSRF; media origins and bearer tokens cannot approve.
- Credentialless token exchange alone allows wildcard CORS. It accepts no bearer or browser refresh cookie and emits no cookie. Other account/directory CORS rules are unchanged.
- Q-6: TV activation obeys the shared Hosted outage gate. The 30-second cap limits activation’s own retry/slowdown calculation; it never undercuts `Retry-After` or a longer shared outage deadline. A user’s explicit action remains interactive. Required native/core wiring is spelled out below, because those trees are not writable by this lane.
- Signing key files are still installed explicitly and a process restarted for rotation. A monotonic root-certified revision is recorded durably; rollback, conflicting same-revision certificates and omitted previous terminal revocations are rejected. A new revision initiates a once-only wake sweep of at most 128 servers per transaction with durable checkpoints, using existing signed wake delivery, backoff, endpoint proof and acknowledgment. No new client polling is introduced.
- Active processes check learned signer revocations every 30 seconds against Hosted’s database. Only positively learned revocation disables their shared signer fence; database unavailability retains last verified knowledge. This is Hosted-internal checking, not server/client chatter. Certificate expiry and invalid chains are checked at every signing operation as well as readiness.
- Pre-release PostgreSQL format is now revision 20, composed during fresh baseline initialization. Old databases are refused without mutation; the old pre-history version-2 adoption bridge is deleted. No data migration or compatibility path for these changes was added. The programme’s SQLite baseline/schema-sync command does not exist in this repository and was not applied to the local server.
- The Hosted-only page has a small module-local `packages/i18n/app-sign-in.en.json` catalogue. It has one responsive column, a slate/violet palette, visible keyboard focus and plain consent text. The canonical public client catalogue remains outside this worktree; it can incorporate these keys if the web lane consolidates the page later.

# Bugs

The 139 entries below cover bug-entry headings in the requested domain/verification documents, including entries wholly outside this lane. “Already fixed according to FIX-LOG” is historical evidence, not a new runtime claim. The register and fix-pass Hosted report were checked for the Hosted findings.

- **B-01.1** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.2** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.3** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.4** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.5** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-01.6** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.7** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.8** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.9** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-01.10** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.11** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.12** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.13** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.14** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.15** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.16** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.17** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.18** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.19** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.20** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.21** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.22** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-01.23** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-08.1** — Not fixed here — outside Hosted code; belongs to API/freshness. This status does not imply the finding is still present in another implementation worktree.
- **B-08.6** — Not fixed here — outside Hosted code; belongs to API/freshness. This status does not imply the finding is still present in another implementation worktree.
- **B-08.9** — Not fixed here — outside Hosted code; belongs to API/freshness. This status does not imply the finding is still present in another implementation worktree.
- **B-08.13** — Not fixed here — outside Hosted code; belongs to API/freshness. This status does not imply the finding is still present in another implementation worktree.
- **B-08.14** — Not fixed here — outside Hosted code; belongs to API/freshness. This status does not imply the finding is still present in another implementation worktree.
- **B-08.15** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.1** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.2** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.3** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.4** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.5** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.6** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.7** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.8** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.9** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.10** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.11** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.12** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.13** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.14** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.15** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.16** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.17** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.18** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.19** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.21** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.23** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.24** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.26** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.27** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-09.28** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.29** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-09.35** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-15.1** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.2** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-15.3** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.4** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.5** — Not changed: the programme explicitly retains unencrypted local-server storage. Hosted sealing/key custody remains intact; this finding is about the local server.
- **B-15.6** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.7** — Not changed: the programme explicitly permits HTTP from any address on local servers; Hosted-advertised routes remain TLS.
- **B-15.8** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.9** — Not changed: deliberate no-lockout policy; do not introduce it. Existing Hosted abuse budgets and password backoff remain.
- **B-15.10** — Not changed here: programme retains light password rules and no lockout; local-server hashing/throttling implementation belongs to accounts/security.
- **B-15.11** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.12** — Partly fixed: runtime signer validation, terminal consent revocation, durable key-change wake sweep, rollback refusal, process signer fencing and check-in certificate delivery added. Unreachable-server consumer and current local-family retirement remain with accounts/security.
- **B-15.13** — Not changed: programme explicitly says a TV code confers the approver’s role. This lane fixes approval authority races without narrowing that role.
- **B-15.14** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.15** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.16** — Fixed in the Hosted Cast edge configuration: script-src restricted to https://www.gstatic.com/cast/, matching the current receiver SDK and server receiver CSP. Main server/web sender CSP belongs to accounts/security or web lanes. Real Cast device check remains.
- **B-15.17** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.18** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.19** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.20** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.21** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.22** — Not changed here: required-TLS local-server policy is superseded by plain HTTP acceptance; connectivity lane owns local enforcement.
- **B-15.23** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-15.24** — Not fixed here — outside Hosted code; belongs to accounts/security. This status does not imply the finding is still present in another implementation worktree.
- **B-19.2** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-19.4** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-19.7** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-19.13** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-19.17** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-19.18** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-21.5** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-21.12** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-21.13** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-21.14** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-21.16** — Not fixed here — outside Hosted code; belongs to native clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.1** — Already fixed in canonical client base; no change here. Secure-LAN producer/consumer regression remains relevant.
- **B-22.2** — Already fixed in Hosted base. Actual PostgreSQL secure-LAN route document and final-origin deduplication regressions passed here.
- **B-22.3** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.4** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.5** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.6** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.7** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.8** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.9** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.10** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.11** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.12** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.13** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.14** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.15** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.16** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.17** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.18** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.19** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.20** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.21** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.22** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.23** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-22.24** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.25** — Not fixed here — outside Hosted code; belongs to connectivity/clients. This status does not imply the finding is still present in another implementation worktree.
- **B-22.26** — Partly fixed on base (5–30-second jittered retry). Q-6 decided: shared outage gate wins. Hosted slowdown capped at 30 seconds and pre-database device admission added; client gate wiring remains outside this writable lane.
- **B-25.1** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-25.2** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.3** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-25.4** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-25.5** — Already fixed. Complete copied apikit equals the named main-server/apikit; shared-apikit.sh passes and CI checks the public canonical checkout. No gratuitous copy change.
- **B-25.6** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.7** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.8** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-25.9** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.10** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.11** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.12** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.13** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.14** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.15** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.16** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.17** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.18** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **B-25.19** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-25.20** — Already fixed according to FIX-LOG-2026-10-12; outside Hosted ownership, and not independently re-tested here.
- **B-25.21** — Not fixed here — outside Hosted code; belongs to deployment/operations. This status does not imply the finding is still present in another implementation worktree.
- **N-V01.1** — Not fixed here: local-family signer provenance/inheritance and targeted retirement are assigned to accounts/security (Q-7); its SQLite schema is outside this lane. Hosted now delivers the certificate/revocations needed by that implementation.

Confirmed findings from the Hosted code review:

| Finding | What was confirmed | Change and evidence |
|---|---|---|
| H-1 Cheap credential/TV admission | Account/server wrappers authenticate in PostgreSQL before their per-principal limit; public device create/poll/cancel also lacked an in-memory admission guard. Anonymous invalid credentials/code traffic therefore reaches SQL repeatedly. | Bounded address and wider-network sketches now run before database lookup, shared across varying route IDs. Existing per-account/server/database limits remain. Every new rejection carries Retry-After. App authorization has a shared 60/minute address and 600/minute network budget across all request IDs; its static assets also have cheap admission. Unit test varies server IDs and proves the budget does not reset. |
| H-2 TV approval session race | The HTTP wrapper authenticates, then the repository approves by account ID and reads its current epoch. Logout/revocation between those steps could approve with the new epoch using retired authority. | Approval captures the presented access token’s family tuple and applies the existing strict current-family check inside its transaction, before device locks and again at expiry boundaries. Revoked/expired-after-lock-wait regression passes on PostgreSQL. |
| H-3 Device/deletion lock crossing | Poll/cancel held a device row before taking account/family locks, while deletion takes account first then deletes that device row. | All approved-device operations lock account first. A pending-to-approved transition releases locks and retries once, with account binding rechecked. Existing concurrent consumption/cancellation and approval tests pass. |
| H-4 Unbounded slowdown | Repeated early device polls add five seconds without a maximum. | The protocol slowdown is capped at 30 seconds; shared outage and explicit retry floors remain separate. Existing slowdown regression passes. |
| H-5 Runtime signer validity | Load/readiness checked the certificate, but document and wake signers still used Ed25519 directly after expiry. | Central checked signing now validates root, certificate dates, key/certificate pairing, self-revocation and process fence. Claims check before approval signing; directory documents/erasures and wakes use it. Failed wake signing releases its leased job for retry. Unit tests cover valid, expired, revoked and absent signers. |
| H-6 Revoked historical directory consent | `verifyConsent` checked an old certificate at issue time but never compared its signer with the current root-certified revocation list. | Revoked signers cannot add a new indexed member or accept custody through historical consent. Rootless verification fallback is removed. Regression verifies that a historically valid signature is refused by the current revocation state, while the current signer succeeds. |
| H-7 Key-change delivery | Key rotation/revocation created no deliberate event to notify idle reachable servers, and ordinary check-ins carried no documents certificate. | Monotonic durable rollout state and terminal revocation table; checkpointed existing signed-wake/ack queue; current certificate added to check-in. PostgreSQL test covers 261 server rows, a 128-row first checkpoint, completion, idempotent repeat, rollback, forgotten revocation, obsolete-worker refusal and old-process signer fencing. Unreachable consumers are an explicit external dependency below. |
| H-8 Pre-release schema adoption | The base silently adopted old production-era version-2 formats although this programme has no compatibility requirement. | Adoption bridge removed; revision 20 required for restart, with required new table presence checked. Empty dry-run rolls back; old/history-less formats are rejected unchanged. PostgreSQL tests pass. |
| H-9 Hosted Cast CSP | Hosted’s static Cast edge allowed scripts from all of gstatic, broader than the receiver SDK path. | Edge script-src narrowed to /cast/, matching the current receiver and local server receiver policy. Source check only; real Cast execution remains outstanding. |

Review boundaries and retained safeguards: claim nonce/proof code retains signatures, audience, expiry, account/family fences, current installed authority and atomic credential delivery; certificate issuance retains exact CSR SAN/public-key validation, namespace ownership, durable account/server/CA quotas, leases, quarantine/reconciliation and owned TXT cleanup; egress retains public HTTPS validation, pinned resolved addresses, redirect refusal and response bounds; direct-address/DNS feed retains authenticated access and generation/label handling. No new external provider call, mail, notification, relay, media or metadata flow was introduced. This review and targeted tests are not an exhaustive proof of every Hosted execution path.

# API changes

| Route | Old shape | New shape |
|---|---|---|
| POST /v1/app-authorizations | Absent | Canonical browser-cookie/CSRF request `{serverId, challenge, redirectUri, state, appName, codeChallenge}`; 201 `{id, accountId, accountName, serverName, appName, redirectUri, expiresAt}`. |
| POST /v1/app-authorizations/{id}/decision | Absent | Canonical cookie/CSRF `{accountId, approve}`; 200 `{location}` containing code+state or access_denied+state. Family/account must equal the reviewed request. |
| POST /v1/app-authorizations/token | Absent | Credentialless `{code, verifier, redirectUri}`; 200 `{assertion}` and nothing broader. Invalid, expired, spent or mistargeted codes use one invalid_grant refusal. |
| POST /v1/servers/{id}/heartbeat | Existing heartbeat fields | Adds `signingCertificate`, the current root-signed documents certificate. Generated Hosted contract updated. Local-server consuming struct is outside this lane. |
| POST /v1/device-authorizations/decision | Existing body | Same wire body, now atomically checks the approving session family. |
| Device create/token/cancel and authenticated account/server routes | Existing wire shapes | Same shapes, new cheap admission and Retry-After on 429. Early-poll slowdown cannot exceed 30 seconds. |

All new/changed JSON API shapes are declared in the apikit contract and generated OpenAPI/TypeScript files. New browser navigation `/auth/apps/authorize` and its external JS/CSS assets are UI endpoints under Hosted’s existing non-API callback/assets convention, outside the JSON registry. The navigation accepts exactly the six request query fields above, rejects duplicates/unknown/malformed parameters, and removes them from browser history once loaded. Source Caddy matcher sends `/auth/apps/*` to Hosted. No JSON API route was removed.

Exact outside-developer flow:

1. Know the chosen local server’s address and server ID. Generate and keep an installation ID, a fresh 32-byte nonce, a high-entropy 43–128-character PKCE verifier and an unpredictable state. Keep verifier/state private in the app.
2. Call the local server’s existing `POST /v1/direct/portico-challenge` with `{nonce}` and the same `X-Portico-Installation-ID` used for subsequent admission. Verify the returned server-signed proof, server ID/public-key pin, nonce and challenge exactly as first-party `requestPorticoChallenge` does. The local challenge lasts five minutes; restart the flow if it expires.
3. Open `https://web.getportico.tv/auth/apps/authorize` with URL-encoded `serverId`, `challenge`, exact `redirectUri`, `state`, `appName`, and `codeChallenge=base64url(SHA256(verifier))` without padding. S256 is the only method; there is no client ID, registration or secret. Return address examples: `http://127.0.0.1:49152/finish`, `http://[::1]:49152/finish`, `my-player://signin/finish`, `tv.example.player:/signin`, `https://player.example/finish`. Original query values are retained except reserved code/state/error names, which are forbidden in the supplied URI.
4. The person signs in through the existing `/account` page if needed, returns to the consent tab, presses Continue, reviews account/server/unverified app name/return address and chooses Allow sign-in or Cancel. No authentication token is exposed to the app or its return address.
5. The return is the exact destination with `code` and original `state`, or `error=access_denied` and state. Validate state before doing anything. Within 60 seconds, POST JSON `{code, verifier, redirectUri}` to `/v1/app-authorizations/token`. Browser requests use `credentials: 'omit'`; never send a Hosted cookie or bearer. This one endpoint permits public-client CORS and exposes Retry-After. Honor rate/outage guidance.
6. Present `response.assertion` as `porticoIdentity` to the local server’s existing `POST /v1/direct/sign-in`, using the same installation ID. Use its ordinary local account/profile/MFA/admission flow and resulting local session. Hosted cannot make a non-member a member. Do not substitute this identity envelope for a local session.

The local server needs **no new route or admission logic for this app sign-in flow**. It does need the signing-certificate check-in consumption described under Needed from others for the separate revocation improvement. The exchange emits no directory/routes, so an app must already know the selected server’s address rather than acquiring an account-wide Hosted discovery grant.

# Schema changes

PostgreSQL current format 20, fresh-only baseline additions:

- `app_authorizations`: normalized account/family/server bindings, immutable request fields, reviewed server name, S256 challenge, hash-only code, expiries and created time. Foreign-key account/family/server cascades and account/expiry indexes. Maintenance deletes expired rows in the existing bounded batch.
- `hosted_signing_state`: singleton monotonic root revision, certificate hash, pending sweep flag and server cursor.
- `hosted_signing_revocations`: terminal root-certified key IDs.
- Revision/history 20 is recorded atomically during fresh baseline initialization. Existing 20 restart performs no DDL. Older/missing-history formats are refused without writes, including dry-run. The old adoption bridge is deleted, with its tests replaced by explicit unchanged-state rejection.

No local-server SQLite schema was touched. No production database was reset or migrated.

# Owner or viewer settings added or wired

No new owner/viewer switch. Signing rotation uses existing key files/operator workflow. App name and return URI are per-request app inputs, visibly unverified, not account preferences. Existing HTTP acceptance and Hosted TLS route rules are unchanged.

# Needed from others

- **Connectivity/native — Q-6 / B-22.26:** canonical `packages/client-core/src/hosted-gate.ts` and SetupCodeService are in main-server, and native setup-codes adapter is in the native worktree. Do not route local-server setup through HostedGate. For Hosted activation, classify the initial explicit create/manual retry/cancel as interactive and timer-driven retries/token polls/background cancellation as automatic. Run actual Hosted requests through the per-origin shared gate and report outcomes. Pending/slow_down protocol answers remain normal handled protocol responses. Schedule no network request before `max(protocol due time, Retry-After floor, shared gate notBefore)`, with positive jitter beyond a common floor. Its own exponential retry stays 5–30 seconds; do not clamp the shared gate to 30. Preserve the current code and expiry through outages; expire locally and regenerate only when appropriate. Test two features sharing a gate: a 120-second/HTTP-date outage floor must prevent TV network polls at 30/60/90 seconds, manual action remains possible, and authorization_pending does not end sign-in. This wiring could not be written in the only authorized worktree.
- **Accounts/security — heartbeat consumer:** add the non-UI `signingCertificate` field to local server `internal/hosted/current_schedule.go`’s response struct. Root-verify its documents purpose, dates and revision; atomically persist monotonic revocations using the existing trust machinery before subsequent admission. A plain certificate is root-signed, not an unsigned server authority grant; verify it before acting. Reachable servers already learn the same certificate through the existing signed wake verifier. Unreachable servers need this consumer on their existing daily check-in; no additional polling is needed.
- **Accounts/security — Q-7 / N-V01.1:** persist verified admission signer provenance through local/profile/TV families; retire only affected Portico-admitted descendants on learned terminal root revocation. Direct/recovery families and normal Hosted-outage independence remain. This requires local SQLite/identity code unavailable for writes here.
- **Web/string owners:** this lane ships a functional module-local Hosted catalogue/page. If consolidating into the canonical public web/i18n build, import the Hosted-only keys and preserve canonical-cookie consent, unverified naming, explicit review, exact return binding and CSP. No new local/media screen is needed. Browser visual/device acceptance is still required.
- **Documents:** `Project Architecture/07 Hosted Services.md` needs the new app flow, revision-20 fresh baseline, checked signing and durable rollover wake/check-in path. Its existing mailoutbox explanation and Networking Contract’s historic membership/presence model need reconciliation by their owners with programme decisions. `deployment/hosted/KEYS.md`, restore/deploy rehearsal notes and old migration guidance must stop promising adoption/upgrades for this pre-release format. No new planning document was created in the repository.
- **Possible external-address publication:** this already fits authenticated `POST /v1/servers/{id}/routes` with a `class:'manual'` HTTPS origin, followed by `/routes/prove` (or `/endpoint`) to prove the same installed server key through its proxy/domain. RouteSets preserves manual origins instead of rewriting them to Portico names. Current authority/generation, egress/TLS, response bounds and identity proof must remain. A later owner-console setting would publish through this existing path; no new public claim, DNS namespace or Portico-issued certificate for that private domain is necessary. No such setting was added without the later request.

# Test notes for other environments

Verified here, limited to touched packages/selected tests:

- Hosted `GOFLAGS=-p=2 go build ./...` passes with `GOMODCACHE=/tmp/portico-hosted-modcache`.
- Targeted `go vet` passes for appauth, httpapi, postgres, security, directory, device, wake, claims, contract, packages/i18n and cmd/hosted.
- Validator/PKCE vector, callback query, canonical-origin/CSRF, credentialless CORS isolation, cheap credential admission, escaped page/CSP/assets, checked signer, revoked consent and wake lease tests pass. JavaScript `node --check` passes.
- Selected PostgreSQL/contract tests pass on owned PostgreSQL 18.6: app scope and single use, wrong verifier/altered URI, 12 concurrent exchanges with exactly one winner, signing rollback, changed family/account, denial/quota, code/request expiry, family/account/server revocation, HTTP consent/exchange, TV approval and device races, refresh input/output/reuse, claim middleware, secure-LAN document/deduplication, signing rollout checkpoints/rollback/fence, registry/golden/generated contracts, current schema initialization/restart/rejection/dry-run. Latest combined selection completed in about 21 seconds for postgres and 1.3 seconds for contract. No full module/repository suite was run.
- Recursive apikit comparison passes against `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server`; CI still checks the published canonical server checkout. `git diff --check` passes.
- Browser inventory through cua was empty; no connected browser existed. HTML response/escaping/asset tests are evidence of output, not visual rendering, real cookie behavior or provider acceptance.

Cold isolated-database procedure: on an isolated host with PostgreSQL 17 or 18 server tools, create an owned empty cluster in /tmp with an owned Unix socket, empty listen_addresses and an unused port. Create `portico_hosted_test_lane`, put its DSN in a mode-0600 file, set `PORTICO_TEST_DATABASE_URL_FILE`, `GOFLAGS=-p=2` and a writable Go module/build cache. From hosted-services/hosted run:

```sh
GOFLAGS=-p=2 go test ./internal/postgres ./internal/contract -run 'Test(AppAuthorization|SigningRollout|Device|ConcurrentDevice|RefreshRotation|ConcurrentSameRefresh|BrowserRefreshCookie|CurrentHostedClaimMiddleware|SecureLANRouteDocument|RouteSets|Contract|Generated|Golden|Definitions|HostedRejects|MigrateDryRun|CurrentIdentitySchema)' -count=1
GOFLAGS=-p=2 go test ./internal/appauth ./internal/httpapi ./internal/security ./internal/directory ./internal/wake -run 'Test(App|Credential|PKCE|Redirect|Signer|KeysSetup|RevokedSigner|ExpiredSigner|InjectedClient|DefaultClient)' -count=1
```

Every named database test must run rather than SKIP; one concurrent exchange succeeds and all others get invalid_grant; wrong PKCE/URI does not consume the code; spent/expired/revoked authority fails; exact same key revision enqueues no new sweep; old schema rejection leaves its catalogue unchanged. Stop only that owned cluster afterward. Do not point at an existing production/test service. A temporary `/tmp/portico-hosted-db-tests.sh` here encapsulates that owned-cluster procedure; it is not a repository deliverable.

Browser/provider/device acceptance: use an isolated revision-20 Hosted instance with dev/root keys, a disposable local-server member and loopback cookie policy only where explicitly enabled. Open the app authorization URL; verify desktop and narrow/mobile layouts, keyboard focus, screen-reader consent and long names/return URLs; exercise signed-out password/provider/MFA, existing signed-in cookie, switching accounts/families after review, Cancel, exact loopback/app-scheme/HTTPS callbacks, state mismatch, PKCE mismatch, replay, code expiry and Retry-After. Confirm the receiver app gets only an identity assertion and admission still follows local membership/MFA/profile rules. For key rotation, certify revision B revoking A with the same offline root/sealing key, start B on the isolated Hosted database, ensure one bounded wake sweep, existing reachable server learns B through signed wake and acknowledgments drain; after the other lane’s consumer, an unreachable server learns B on its existing check-in. Verify local-family targeted retirement separately. Test Cast SDK loading under the narrowed edge CSP on a real Cast device. No real provider, certificate authority, DNS deployment, proxy, TV or app-scheme OS handoff was exercised here.

# Not finished

- Q-6’s shared client/native outage-gate wiring cannot be edited in this lane’s authorized worktree. Decision, required behavior and acceptance test are concrete above; Hosted alone cannot implement a client-wide circuit.
- The local-server check-in `signingCertificate` consumer and Q-7 signer-provenance/session retirement need accounts/security. Reachable signed-wake propagation is written using the existing verifier, but end-to-end fleet behavior and unreachable-server revocation are not established until that work lands.
- Browser visual/provider/cookie and actual OS callback/TV/Cast/ACME/DNS/proxy acceptance remain unverified. No browser was connected; no production service was touched.
- Non-JSON browser UI/asset endpoints follow Hosted’s existing callback convention rather than JSON apikit route registration. All changed JSON API routes are registered and generated.

No other Hosted source implementation item is knowingly left as a stub. These external integration and acceptance gaps are not claims of programme-wide completion.
