# Outcome

The claim owner is ready for admission before the installed credential transaction commits. Claim installation and the first acknowledgment now wake Hosted control after commit, independently of certificate/remote management. The old delayed owner-linking path is deleted. An unclaimed control pass followed by installation/acknowledgment and immediate verified owner sign-in passes without a status wait, post-install control pass, restart, topology change or manually injected wake.

Activation now shares the account origin's HostedGate. Automatic token polls, create recovery and background cleanup obey outage deadlines; explicit create/cancel/review/decision remain interactive. The existing native TV journal authority (`<origin>#tv-x25519-v1`) binds SetupCodeService directly to that origin's gate, so the canonical package copy carries this behavior into the current native adapter. Local Quick Connect stays outside the Hosted gate. Pending/slowdown replies are ordinary protocol answers. Valid codes remain visible through outages, expired codes disappear locally, and durable redemption/cancellation receipts survive. A longer sibling outage during ambiguous create recovery retains the same request ID.

Root-certified revisions above 9007199254740991 are rejected by issuance, verification and the persistence guard before SQLite sees them. The maximum supported revision is accepted and persists correctly. No route or schema changed.

All work was in the named worktree and programme report/snapshot locations on ehler-homelab-mini-1. No commits, pushes, deployment, provider contact, full suite, native builds or helper/subagent sessions. Context sync failed on its read-only lock; supplied guidance and readable canonical project/standards/verification notes were used.

Snapshot 001 is an intermediate cumulative milestone. **Snapshot 002 is the final cumulative source state and supersedes 001**; apply only 002, not the sum. No source changes follow the final snapshot.

# Decisions I made

- Make admission ready inside installation, rather than waiting for background work. Readiness failure rolls back both credentials and the membership link; no control wake occurs on failure. Installation and acknowledgment each notify control after their own commit; acknowledgment replay does not repeat the wake.
- Preserve existing links. Claiming never reassigns an already-linked local owner or steals a Hosted identity already linked to another local member. Those existing associations retain their existing admission roles; a different claim account does not gain owner authority merely by claiming.
- Keep the 30-second cap on activation's own slowdown/retry calculation, independently of Retry-After and the shared gate. Never shorten a longer server-advertised protocol interval. Add spread beyond shared outage deadlines.
- Keep a durable consumed-grant receipt after code expiry: hide the expired code locally, and let the issuing authority answer whether a consumed grant is recoverable once the gate permits traffic. Do not discard a receipt or allocate a replacement merely because a service is temporarily unavailable.
- Bind the existing explicitly namespaced native TV journal to its Hosted origin in canonical core, requiring no native screen/adapter edits. Other generic adapters may supply an explicit gate through SetupCodeApi; local journal authorities receive none.
- Match Hosted's exact JSON revision range, rather than merely SQLite's larger signed integer range. Correctly root-signed external certificates above that bound are also refused.

# Bugs

- **Initial claim owner admission race — fixed.** Confirmed before the fix by a regression returning access_refused immediately after acknowledgment. Same-transaction readiness, post-commit wakes and regression tests now pass.
- **B-22.26 — fixed.** Confirmed before the fix by a regression reaching fetch while a sibling feature held the gate open. Adapter and native journal tests now prove no requests at 30/60/90 seconds of a 120-second outage; protocol, manual action, retry and expiry regressions pass.
- **External certificate revision overflow — fixed.** The follow-up's remaining local-server hand-off is closed at both trust and persistence boundaries. Correctly signed maximum/oversized artifacts and durable boundary tests pass.
- **B-15.12 — already fixed on this base for the earlier heartbeat/provenance hand-offs.** Nullable heartbeat signingCertificate is consumed before later admission, and learned revocations inhibit/retire affected descendants; selected existing tests pass. This lane adds the missing revision bound.
- **N-V01.1 — already fixed on this base.** Admission signer provenance and targeted descendant retirement exist; selected revocation tests preserve password families while retiring affected Portico families.

The inherited Hosted reports also enumerate findings outside this lane. Their mention is not authorization to expand beyond the declared hand-offs; one disposition per inherited ID follows:

- **B-01.1 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.2 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.3 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.4 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.5 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.6 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.7 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.8 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.9 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.10 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.11 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.12 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.15 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.16 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.17 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.18 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.19 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.20 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.21 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.22 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-01.23 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-08.1 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-08.6 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-08.9 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-08.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-08.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-08.15 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.1 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.2 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.3 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.4 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.5 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.6 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.7 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.8 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.9 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.10 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.11 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.12 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.15 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.16 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.17 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.18 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.19 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.21 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.23 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.24 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.26 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.27 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.28 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.29 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-09.35 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-13.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-13.21 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.1 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.2 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.3 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.4 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.5 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.6 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.7 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.8 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.9 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.10 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.11 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.15 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.16 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.17 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.18 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.19 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.20 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.21 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.22 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.23 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-15.24 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-19.2 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-19.4 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-19.7 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-19.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-19.17 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-19.18 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-21.5 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-21.12 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-21.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-21.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-21.16 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.1 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.2 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.3 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.4 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.5 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.6 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.7 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.8 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.9 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.10 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.11 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.12 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.15 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.16 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.17 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.18 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.19 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.20 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.21 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.22 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.23 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.24 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-22.25 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.1 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.2 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.3 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.4 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.5 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.6 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.7 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.8 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.9 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.10 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.11 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.12 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.13 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.14 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.15 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.16 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.17 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.18 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.19 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.20 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.
- **B-25.21 — not changed here:** outside the declared server/shared-client hand-offs; the owning lane retains the status in the inherited Hosted reports.

# API changes

No method, route, request/response JSON shape, key scope or restriction classification changed; no route removed or added. No apigen run was needed.

Internal TypeScript interfaces add optional gate/request-kind hooks; HttpDeviceAuthorizationApi exposes its shared origin gate. HostedGate.run can classify a named set of non-server-error protocol answers without treating slowdown's 429 as an outage. SetupCodeApprover reports Hosted outcomes through the gate. Existing consumers remain typed, and the web source compiles.

# Schema changes

None. Existing account_portico_links and its existing membership-journal triggers are written atomically with installation. No migration, baseline edit, schema-sync or compatibility path.

# Owner or viewer settings added or wired

None. Existing claim identity, local membership, second-factor policy, code expiry and Hosted retry guidance govern the behavior.

# Needed from others

One disposition per hand-off after re-reading e2e-hosted.md, hosted.md and hosted-followups.md:

- **Owner readiness/control notification:** completed here; startup's existing UseCurrentClaims now installs both hooks, so networking_initializer.go needs no new wiring.
- **Canonical activation/shared gate:** completed here, including the adapter exercised by the integration lane and native TV's current journal namespace.
- **Native propagation:** copy canonical client-core through the established shared-package process during programme integration. No native app path was writable here. The package-level test uses the native adapter's actual namespace; physical devices remain unverified.
- **Web activation:** checked DeviceApprovalScreen's use of SetupCodeApprover; shared approver handles outcome reporting. No web screen edit was required. The web app does not use DeviceAuthorizationService for its own activation polling.
- **Heartbeat signingCertificate consumer:** already present and verified by existing selected regression; no duplicate consumer added.
- **Signer provenance/targeted retirement:** already present and verified by selected regressions; no duplicate implementation added.
- **Oversized external revision:** completed here.
- **Toolkit publication:** programme integration/publishing owner must publish the integrated canonical toolkit; this lane must never push or deploy.
- **Hosted consent catalogue/page consolidation:** remains with Hosted/web/document owners; no new local screen or user-facing string was needed here.
- **Architecture/runbooks:** document owners should reconcile Hosted format 21, shared durable check-in certificate delivery, claim admission readiness and activation deadlines. Internal documents are read-only here.
- **External-address publication:** an optional later setting, not an unfinished hand-off. Existing initializer/publication source already passes manual access URLs; no new setting was added.
- **Verified-email invitation extension:** explicitly not an unfinished Hosted requirement; no new assertion field or invitation contract added.
- **Public endpoint proof, automatic public wake transport, browser/OS callbacks and fleet/provider acceptance:** environment checks, not missing server/shared-client source identified by these reports.

# Test notes for other environments

Verified limited checks, all Go commands prefixed with GOFLAGS=-p=2:

- Both primary new regressions were observed failing before their fixes: immediate owner access refused, and activation reached fetch despite an open sibling gate.
- `go build ./...` from server: passes.
- `go vet ./internal/networking ./internal/hosted ./internal/hostedtrust`: passes.
- Selected networking/Hosted/trust tests matching ClaimInstallationReadiness, ClaimAcknowledgment, ClaimSQLite, MembershipPush, PorticoOwnerTransfer, PorticoSignInKeeps, ANetworkChange, NetworkChangeBursts, ADueCheckIn, ClaimCleanup, CertificateRevision, OversizedCertificate, RootRevocation, LearningASigning and SharedCertificate: pass. Installation hook test proves rollback, committed read visibility at wake, two first-transition wakes and no replay wake.
- Final selected Hosted/trust tests matching ClaimAcknowledgment, ClaimInstallationPreserves, LargestSharedCertificate, OversizedCertificate, TheCheckInCertificate and CertificateRevision: pass. Boundary checks include maximum exact revision, next revision, the signed-integer overflow boundary and maximum uint64.
- `node --experimental-strip-types --test packages/client-core/test/device-authorization.test.ts packages/client-core/test/hosted-gate.test.ts`: **25 tests pass**. Includes real HTTP-adapter fetch counts, HTTP-date 120-second floor, normal pending/slowdown, interactive versus background actions, ambiguous creation/retry extension/cancellation, longer advertised interval, local expiry, web approver isolation, native journal automatic gate binding and protected receipt retention.
- Selected `packages/client-core/test/networking-review.test.ts` with name pattern `activation|cancel|poll|approved`: **8 tests pass**, covering durable grant adoption/checkpoint fencing/cancellation and existing retry growth/jitter.
- `npx tsc --noEmit -p web/tsconfig.json`: passes.
- `git diff --check`: passes.

Routine corrections during work: the first regression-write command ran from server/ with repository-root paths and wrote nothing; the resulting Go invocation explicitly reported no tests to run and is not counted as verification. After fixing paths both regressions failed as expected. Selected cleanup tests initially exposed a fixture that assigned the claim's Hosted account to a second member while relying on delayed owner linking; its setup now explicitly removes the fixture's owner link before establishing the intended alternate association. Production cleanup logic was unchanged.

Cold reproduction on Ubuntu: use this worktree, cached Go dependencies, a writable test cache, GOFLAGS=-p=2, Node 24 with TypeScript stripping and installed workspace dependencies. Run the commands above. Go fixtures use owned temporary SQLite state, generated local keys and signed assertions; no Hosted service or provider configuration is required. Pass means acknowledgment immediately yields an authenticated owner session, failed readiness leaves no credential/link/wake, existing links do not move, and oversized revisions fail as trust errors without SQL writes.

For integrated runtime acceptance, use the existing read-only Hosted development driver from main-internal with `--server-tree` pointing at the integrated server source and owned programme scratch. Build that actual source, claim an already-idle unclaimed server, and sign its owner in immediately after acknowledgment; do not use the old driver's owner-link readback wait as proof of this fix. On native TV devices after package copy, open a valid code, cause another Hosted feature to receive HTML 503/HTTP-date Retry-After 120 seconds, and observe zero automatic polls at 30/60/90 seconds; the code remains until local expiry, manual actions remain possible, and later normal pending/approval succeeds. Test protected grant recovery and cancellation after process restart. Run on iPhone/iPad/Apple TV/Android/Android TV/Fire TV and each other shipping native adapter that exposes activation. Local Quick Connect must still work independently of Hosted's gate. Check the web review/approve flow separately with a real browser cookie and interactive navigation.

# Not finished

No assigned server/shared-client implementation item remains. Native package copying, physical-device/browser/provider/fleet acceptance and programme publication/document reconciliation are outside this worktree and were not performed. The full Hosted end-to-end driver was not rerun here; source regression fixtures and adapter-level checks establish these fixes, not external network/provider or shipping-device acceptance. No full test suite, native packaging build or deployment was run.
