# Adversarial second pass — primary reviewer

Date: 2026-09-25 (local; continued across UTC midnight). Scope: all nine Hosted findings, their additional concerns, integration/coverage claims and final consolidation. This is a second source-level falsification pass, not runtime reproduction. The other three reviewers were reassigned away from the reports they authored. No application/test/build/network execution was performed.

Source roots below: `INTERNAL = portico-internal/`; `SERVER = apps/portico-server/`, under the Portico workspace. Original IDs stay stable.

## Per-finding verdicts

### HS-001 — Uphold P1; retain explicit phishing and no-MFA conditions

Tried to defeat the chain through start validation, native continuation generation/Origin restrictions, callback cookie enforcement, provider PKCE, transaction locking, and completion/onboarding/MFA gates. `hosted/internal/httpapi/account_security.go:33–65` accepts a privately generated native binding when Origin/cookie are absent; it does not require a prior session for login. `postgres/oidc_flows.go:58–62,185–192` accepts an omitted completion mode. `oidc_login.go:126–136` compares callback browser binding only for browser mode. `oidc_complete.go:78–100` requires returned proof only when the optional flag is true; known linked accounts reach the MFA/session branch at 216–258. These paths are beneath `INTERNAL/hosted-services/`.

The attacker must initiate the native transaction and induce the victim to authenticate through its genuine provider authorization URL. PKCE protects the server's exchange, but the server subsequently gives the session to the initiating binding holder. Normal official-client opt-in cannot constrain a separately issued request. MFA and native Apple assertions remain meaningful counterexamples to a universal takeover claim. The original report already states them. Fix must close the public legacy opt-out rather than only changing client defaults.

### HS-002 — Uphold unsafe path; narrow any claim of inevitable corruption

Tried the read-only command branch and persistence locks. `dns/cmd/portico-dns/main.go:57–82` opens the mutable table before healthcheck dispatch; the separate readiness command returns earlier. `directdns/table_state.go:115–137` removes unreferenced journals and truncates a partial tail. `Compact:322–367` adopts a new journal before snapshot replacement. `compactMu` and `mu` are object/process-local, so they cannot exclude a separately invoked health command. No external ownership lock was found on this route.

The append-tail hypothesis depends on seeing a partial write; the compaction/unlink hypothesis independently establishes the unsafe interleaving. A probe started when no write/compaction overlaps is not shown to lose data. The regular deploy probe runs soon after start, which may reduce overlap. Keep P1 for possible durable discovery-state loss, with race prerequisites. Feed repair can mitigate some outcomes and must not be omitted. Moving command dispatch before OpenTable is narrowly justified; a broader state lock must also account for legitimate rebuild/maintenance processes.

### HS-003 — Uphold P2

Tried the durable MFA ciphertext, polling projection, normal completed-session replay and request rollback as recovery alternatives. `postgres/oidc_complete.go:78–100` consumes the code; at 216–245 it commits the challenge without changing the transaction to completed. The retry exception requires completed, so it fails before reading the existing MFA ciphertext. `oidc_flows.go:228–271` can expose MFARequired but not the challenge secret. Code consumption and MFA creation occur in the same committed transaction; this is specifically a lost response after commit, not an ordinary validation failure which rolls back.

The repair must read back the same valid challenge under the exact proof/binding; merely allowing all consumed codes or recreating challenges would weaken the security invariant and reset attempt budgets. Existing-session receipt replay is not evidence this MFA case works.

### HS-004 — Uphold P2

Tried receipt input/output matching, family horizon checks, self-describing token routing and prune ordering. `sessions/service.go:65–85` always calls LookupRefresh before RotateRefresh. `postgres/refresh.go:42–48` needs the predecessor row; no encoded family fallback exists. Its receipt logic at 91–118 deliberately supports outage recovery until the successor is used/family horizon. `maintenance.go:69–80` independently deletes consumed predecessor rows after one day. A family/receipt can outlive that cutoff. Bounded prune batches delay the defect; they do not prevent it.

Keep the trigger precise: exact old token/request ID after committed rotation response loss, after maintenance removes that token, before the valid receipt/family expires. This is forced reauthentication, not account takeover. Retaining all token history indefinitely is not the fix; retain the specific recovery lookup for its bounded valid horizon.

### HS-005 — Uphold P2; preserve separate structural/time semantics

Searched for database triggers and helper side effects that might provide omitted invalidation. `httpapi/handler.go:108–136` returns 304 before materializing the list. `postgres/server_events.go` calls touchServerListTx only for rename; `withdrawClaimAddressesTx` (`direct_route.go:234–260`) advances DNS state, not account-list versions. `claim_cancel.go:142–186` updates installed state without bumping the list. The only relevant notification trigger fires when accounts.server_list_version is explicitly updated. `directory/server_list.go:82–126,182–198` rereads version numbers; it does not recompute clock-driven presence or compare the list body. `postgres/directory.go:15–38` does return time-derived status and presence fields.

Unconditional refresh and later endpoint/member changes can heal this; a stale response is not an authorization bypass. The acknowledgement race requires an intervening list read and no later bump that masks it. Do not fix by blindly generating a durable account version on every heartbeat or for every active client: preserve the scale contract with explicit representation/expiry semantics, separated presence validation or bounded scheduled transitions.

### HS-006 — Uphold P2

Tried queue coalescing and server wake as counterexamples. The queue indeed merges removals before it is consumed. `postgres/member_label_rotation.go:22–50` deletes one queued entry, rotates, and wakes the server; another later removal can enqueue again. `migration_0014.go:15–25` replaces one previous slot with the formerly current label and resets 24 hours; `migration_0017.go:76–77` retains removal-driven rotation. Thus separately processed A→B→C drops A early. DNS compares only current/previous, not a retained historical grace set.

Server wake/presence refresh and DNS caches may make the practical outage uncommon or delayed. Retain the stated handover violation, not a claim that every existing TCP stream disconnects. Do not fix using an unbounded prior-label array; coalescing/epochs or explicitly bounded grace admission must preserve authority revocation and a documented handover guarantee.

### HS-007 — Uphold P2; recovery-tool failure, not backup-data corruption

Inspected the complete restore script and actual login handler. `scripts/hosted-restore-test.sh` has no curl header injection from its JSON file: it sets only Content-Type. The request defaults to native mode but `continuation(..., true)` still rejects no Origin/no binding at `httpapi/account_security.go:55–58`. The script's local trusted-proxy setting does not waive continuation. An external curl user configuration could add headers, but a mandatory correctness dependency on undocumented per-user configuration would not make the checked-in script self-contained.

The corrected tool must distinguish a 202 MFA challenge from a usable session and perform an authenticated read. This remains a drill defect; no actual restore was attempted or shown corrupt.

### HS-008 — Narrow to explicit conditional operational recovery gap; retain P2 review priority

The script intentionally activates an already reviewed release and does not promise automatic rollback. Absence of auto-rollback alone is not a security defect. The concrete concern is that `deploy-hosted-release.py:60–101` replaces live unit selection, then raises on failed validation while keeping that selection. `hosted/cmd/hosted/main.go:143–157` commits migration before validating ACME configuration; `postgres/current_schema.go:12–28` rejects a schema newer than the previous binary supports. Therefore keeping an older release directory is insufficient proof of recoverability across a version-changing migration.

Keep this entry separate from confirmed production breakage and require the failure/schema condition. Existing operator runbooks or systemd overrides may provide recovery outside the reviewed script; none were inspected. Do not propose automatic database rollback after new writes. Preflight configuration and define a tested roll-forward/backward-compatibility recovery path. The original report's conditional classification is appropriate; its heading should not imply a missing automatic rollback is universally a bug.

### HS-009 — Uphold conditional P2; cross-layer masking must be explicit

Tried default disabled Cast, server-supplied application ID, existing framework detection, and separate receiver policy. `SERVER/web/src/bridge/cast.ts:19,31–39,67–71` loads gstatic when configured and no framework is already present. `INTERNAL/deployment/hosted/web.caddy:3` restricts script-src to self; the receiver's gstatic allowance cannot relax the sender document. The main Caddyfile imports the external installed include, so source proves an incompatible template, not the live response header.

Narrow trigger to a Cast-enabled Hosted sender with this policy installed and SDK not already provided by another mechanism. Fixing it alone will not make Cast usable: SV-008 and SV-012 independently block current Hosted/receiver paths. No configured app ID means no attempted load, rather than a CSP incident. A narrow policy change must be validated against actual SDK requirements later, not replaced by a wildcard.

## Additional concerns and blanket-claim review

- The late-commit version-window concern is plausible from a fixed 10,000-version reread; no enforced commit-order bound was found. Keep it a stress/architecture question rather than an established production stale-cache incident.
- Export memory and retention throughput concern capacity; source review alone does not establish realistic account sizes or worker backlog. Keep outside counts.
- App Store certificate roles/OIDs and provider revocation policy require a scoped trust-policy review with current primary documentation. General X.509 use alone does not establish acceptance of an attacker's certificate. Do not label this a vulnerability from the available evidence.
- Retired issuer option loss is conditional on operator configuration. The deploy script reconstructs arguments, but an external override/runbook was not inspected.
- API toolkit copy equality/provenance and release gate execution are separate facts. Presence of a drift checker does not prove a release ran it.
- Public website countercheck: reread all three `getportico-tv/public/scripts/*.js` browser scripts. They filter static document nodes and toggle attributes using text-derived search; they do not write untrusted HTML or send credentials. This supports the narrow original observation, not a general website security/accessibility clearance.
- Coverage counts inventory source; they do not imply every line received two reviews. The second pass challenges every reported finding, not every original file a second time.
- Repository integrity: all 3,752 inventoried source/config hashes were unchanged at the first post-audit check. Internal HEAD changed externally from 33ce964 to 8f412e4 with only an operator README/architecture backlog diff; original baseline remains recorded. Audit files are untracked, not silently claimed ignored. No application code mutation by this audit occurred.

## Verdict summary

All seven core Hosted findings survive source-level falsification. HS-008 remains a conditional operational gap, HS-009 a conditional template incompatibility with additional current Cast blockers. The pass strengthens prerequisites and fix constraints; it provides no runtime reproduction, live deployment assessment or proof that the wider codebase is free of other issues.
