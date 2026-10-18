# Outcome

Hosted and the actual local server now have a rerunnable isolated integration driver: `hosted-services/hosted/dev/e2ehosted/run.py`, with a development-only signed-wake fixture and a Node script that loads the actual shared client gate/activation adapter from the named server worktree.

The final run completed **16 passing checks and one confirmed client gap**, without any failed check. It exercised fresh Hosted database startup, account creation/password sign-in, claim installation/acknowledgment, directory listing, first-party identity admission, third-party consent/loopback callback/PKCE admission, encrypted TV enrollment/server admission, Hosted outage, persisted server retry fencing, signing rotation, revocation delivered through the actual heartbeat, and anonymous admission budgets. An earlier run also exposed an immediate server admission race after claim acknowledgment; that is documented below rather than hidden by the final driver's readback wait.

Fixed in Hosted: local consent could not use its canonical-origin cookie path because account-origin configuration admitted only the production URL. Development builds can now load a numeric-loopback HTTP account origin with an explicit port, with every external identity-provider configuration refused. Ordinary and release builds retain their existing account-origin restriction. There are unit regression tests for the configuration loader and its boundaries.

All execution was on `ehler-homelab-mini-1`. No real Hosted service, provider, DNS service, ACME issuer, deployment, push, commit, package installation, full build pipeline or full test suite was used. Only this worktree and the programme scratch/report/snapshot locations were written. The main-server tree was read and built into scratch; it was not edited. No subagents or helper models were used.

Final private runtime evidence: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/e2e-hosted/run-fjw1hn_s/`. `results.json` contains the individual outcomes; process/command logs, PostgreSQL data and server SQLite state permit readback. The run directory is mode 0700. Generated credentials and private keys remain only in programme scratch. All owned Hosted, TLS proxy, server, callback/proxy listeners and PostgreSQL processes were stopped; the owned socket directory was removed. The retained PostgreSQL has no postmaster.pid.

Snapshots are cumulative: 001 is the earlier implementation milestone; **002 is the final source snapshot and supersedes 001**. Use 002, not the sum of both patches. The final source tree is not changed after 002.

# Decisions I made

- Use existing devtrust setup/account helpers, a generated private Hosted root, the existing loopback TLS proxy and PostgreSQL 18.6 binaries previously extracted into `/tmp/portico-hosted-postgres`. PostgreSQL listens only on an owned private Unix socket, with host authentication rejected and no TCP listener. Each run initializes an empty database, never restores or modifies another lane's cluster.
- Test account provisioning uses `dev/e2eaccounts`, which creates the account through identity.Register in the isolated database. The actual password sign-in route is then exercised in native and browser modes, including native continuation, browser refresh/CSRF cookies and canonical Origin. This is not proof of public verified-contact registration or mail delivery.
- The production endpoint registry and egress client deliberately refuse loopback. Keep that protection. The development wake fixture reads the real server wake revision from the test database and runs the real wake.Worker with a single fixture work item and injected loopback-only HTTP client. It does not create a production endpoint, fake an endpoint proof, acquire a real queue lease or claim public-network delivery.
- Rotation uses the same root and sealing key. B renews A without revoking it. C then revokes A while retaining B. A valid B wake prompts the server's actual HTTP heartbeat; **only that heartbeat carries C's revocations**. This independently proves check-in certificate delivery rather than relying on C's wake envelope to teach the revocation.
- Claim completion is checked through installed/acknowledged state and local owner-link readback. The driver gives the owner link up to three seconds to appear; if absent, it probes actual identity admission, records the refusal as an owning-server gap, injects a signed wake and continues. It never writes a link or disables membership checks to make admission succeed.
- A failed HTTP proxy is supplied to child processes as a provider-egress guard, and the driver records attempted HTTP egress. The final run recorded none. All driver requests use direct numeric loopback connections, validate TLS through the generated CA and never automatically follow redirects; only the validated callback to its own loopback listener is visited.
- Context sync was attempted and failed on its read-only lock directory. Supplied guidance and readable canonical project/standards/verification files were loaded. The subsequently supplied guidance revision 903b1fbea30aec5db763f43747827250650b340b was incorporated and relevant standards/verification notes refreshed.

# Bugs

The runtime lane brief assigns no domain audit documents. These are the confirmed findings relevant to the performed flows:

- **Development canonical account origin — fixed.** The consent endpoints require the canonical account origin, but the existing development configuration could not name a local one. `internal/oidclogin/provider.go` now delegates exclusively in devtrust/non-release builds to the bounded, provider-free loopback validator. Actual cookie/CSRF consent passes; normal and devtrust+release loader refusal tests pass.
- **B-22.26 — not fixed here; confirmed against the real client adapter.** With the origin's shared HostedGate open after a connection failure, calling `HttpDeviceAuthorizationApi.poll` still invokes fetch once. The final runtime result records `networkRequestsWhileGateOpen: 1`. The read-only client source uses plain HttpLocalApi in create/poll/cancel/revoke and its automatic service poll does not consult the shared gate. The gate class itself correctly blocks an automatic callback, and its six selected unit tests pass; this does not establish activation adapter wiring.
- **Initial claim owner admission race — not fixed here; server ownership.** In `run-ydrlya5f`, a claim reached installed and installationAcknowledged=true and appeared in the directory, but a fresh correctly signed Hosted identity assertion received `403 access_refused`. Immediate post-run SQLite readback showed zero account_portico_links and no networking_control_schedule row. This was an immediate-admission failure; a six-hour observed outage is **not** claimed. Source confirms linking happens later in `hosted.memberStep/linkClaimOwner`, while the control loop's idle safety deadline is six hours and claim handler completion wakes certificate/remote management, not directly Hosted control. Final run's bounded owner-link wait succeeded and did not trigger its fallback. The server should close the admission race and explicitly notify control at claim installation/acknowledgment.

Several intermediate driver attempts corrected assumptions about the existing contracts: canonical claim field order, native continuation headers, directory `items`, PKCE's 400 invalid_grant response, UUID TV creation IDs, early-poll 429/Retry-After, clientRequestId binding, wake's 202 response and the 240/minute credential lookup budget. Those were driver errors, not Hosted defects. One temporary nested helper directory caused by an incorrect working directory was moved/removed before final checks and snapshots.

# API changes

None. No method, route, request/response shape, typed contract or generated consumer was added, removed or changed. The new helper is a devtrust-only executable, not an HTTP route. Existing `webOrigin` configuration gains the development-only acceptance described above.

# Schema changes

None. The real Hosted process initialized its existing format-21 baseline on an empty PostgreSQL database. The server initialized its existing SQLite baseline in fresh scratch state. No compatibility path, migration or server schema edit was added.

# Owner or viewer settings added or wired

None. The existing private OIDC configuration's `webOrigin` is used for the development canonical account page, with no providers configured. The existing allow-loopback-enrollment switch controls development cookies. This is test configuration, not a new owner/viewer setting or product screen.

# Needed from others

1. **Client B-22.26:** update canonical `packages/client-core/src/device-authorization.ts`. Share `hostedGate(baseUrl)` with the other account-service APIs; timer-driven poll and automatic retries must run as automatic work. Initial user start/cancel remains interactive. `DeviceAuthorizationService.schedule` must use a due time at least the maximum of protocol interval, Retry-After and the shared origin gate deadline. A 30-second activation slowdown cap must never shorten an outage deadline. Add an adapter-level test proving zero fetch calls while a sibling service has opened the shared gate; mirror this in native TV activation and propagate the canonical package through its established copy process. This lane did not edit client trees.
2. **Server initial admission:** in the current claim installation/acknowledgment path, arrange prompt Hosted-control notification independent of remote topology/certificate work. Wire a post-commit callback to control.NetworkChanged, and make owner admission ready atomically with installation or expose/await genuine admission readiness before a successful completion is acted on. Add a runtime regression that lets an unclaimed server become idle, installs/acks a claim, and immediately signs its owner in without an unrelated topology change, process restart or manually injected wake. Relevant files: `server/internal/networking/claim_sqlite.go`, `claim_handler.go`, `server/internal/hosted/current_schedule.go`, `membership_push.go`, and `server/cmd/server/networking_initializer.go`. All are outside this writable lane.
3. No further Hosted/server API-shape correction was required by the tested flows. Public endpoint proofs, actual automatic queue delivery over public TLS, browser rendering/OS callback behavior and fleet/provider acceptance remain separate environment checks, not source changes inferred from these passes.

# Test notes for other environments

Executed limited checks:

- Driver's final run: 16 passes, one B-22.26 gap, zero failed checks. The earlier complete rebuilt run `run-_v0sqb4v` also passed all 16 backend/shared-gate checks before the real activation-adapter gap probe was added. Final run reused those compiled Hosted/server binaries; no application source changed between that build and the final run.
- Actual devtrust Hosted/server/helper builds all pass. Main-server source HEAD observed: ce344737a44d6593a4f7cd9809f404b4aabbb024. This worktree base HEAD: 416708d966af526103164844bb9bd05d670a6147. Runtime uses the working source in the named trees, not a claim that HEAD alone describes every programme change.
- OIDC loader/validator: `go test -tags devtrust ./internal/oidclogin -run 'Test(DevelopmentAccountOrigin|LoadLocalAccountOrigin)' -count=1`; normal and `-tags 'devtrust release'` TestNormalBuildRefusesDevelopmentAccountOrigin: pass.
- `go vet -tags devtrust ./internal/oidclogin ./dev/e2ehosted/wake`: pass.
- `go test ./internal/httpapi ./internal/wake -run 'Test(App|IdentityCookie|IdentityContinuation|InjectedClientStillDelivers|DefaultClientRefusesALoopbackWakeTarget)' -count=1`: pass.
- `go test ./internal/directory -run 'TestEndpoint(ExactProofAndCommitAuthority|RejectsUnsafeOriginBeforeFetch)' -count=1`: pass, using existing injected proof fixtures without provider contact.
- `node --experimental-strip-types --test packages/client-core/test/hosted-gate.test.ts` from read-only main-server: all six pass on Node 24.21.0.
- Python AST parse, Node syntax check and git diff --check: pass. All Go commands used GOFLAGS=-p=2. No PostgreSQL test suite was invoked or silently skipped; the actual running processes and database provided integration evidence.

Cold reproduction from the lane worktree:

```sh
python3 hosted-services/hosted/dev/e2ehosted/run.py \
  --server-tree /home/justin/projects/Portico/.worktrees/audit-implementation/main-server \
  --scratch /home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/e2e-hosted \
  --pg-bin /tmp/portico-hosted-postgres/root/usr/lib/postgresql/18/bin
```

Requires Go with cached dependencies, PostgreSQL 17/18 binaries, psql/createdb, Node capable of TypeScript stripping and Python cryptography (46.0.5 was available; nothing was installed). The default command builds the needed individual executables into scratch. `--skip-build` reuses existing scratch binaries. Each invocation retains its own 0700 run directory and tears down only its owned processes and cluster. No ports, keys, passwords or account IDs need to be supplied; all are generated. The Unix socket uses an owned short `/tmp/portico-e2e-socket-*` directory to stay below Unix path-length limits; credentials and persistent state remain under programme scratch.

Pass evidence includes:

- Empty database account count 0, then 1 after development provisioning, actual native/browser password sessions.
- Claimed installation acknowledged; correct server listed; signed challenge bound to nonce, installation and server key; real assertion admits a session and GET /v1/me succeeds.
- Consent HTML and CSP served; canonical cookie/CSRF review and Allow endpoints; exact existing callback query/state retained; wrong verifier and spent code refused; exchange yields only an assertion; the server admits it.
- TV's initial too-early poll produces 429; Retry-After honored; review/approval; X25519/HKDF/AES-GCM decrypts the grant bound to issuer/client request/recipient; its account token obtains a server assertion and session.
- With Hosted and its TLS proxy stopped, GET /v1/me works for admitted/direct sessions. A signed wake causes a failed control attempt and a persisted retry_at; two additional wakes leave attempts/retry_at unchanged before the deadline. Actual shared gate blocks automatic fetch, while the real activation adapter separately records the known bypass.
- B reaches local trust revision 2 and A/B sessions survive. C's heartbeat reaches revision 3 and stores A's terminal revocation. First-party, third-party and TV sessions admitted by A return 401; B, direct and fresh C sessions work. Hosted wake_state acknowledges the actual wake revision.
- Anonymous claim, password/registration, device credential and app exchange budgets produce 429 and positive Retry-After. The password/registration/claim budget is shared; the test does not claim five independent quotas. No provider HTTP egress was attempted.

Browser and physical-device testing should additionally verify consent rendering/accessibility and actual OS loopback navigation, native TV timing with a shared gate already open, and grant decryption through the shipping player/client. Public TLS/proxy endpoint proof and automatic signed-wake delivery need an isolated publicly routable fixture; preserve private/loopback egress denial rather than disabling it. Actual Apple/Google authentication, DNS, ACME and mail were intentionally unconfigured and uncontacted. Decoder sandbox/Dolby Vision capability notices appeared at server startup; no media, playback, tuner or hardware acceptance is claimed by this sign-in lane.

# Not finished

No Hosted implementation item discovered by this lane remains unfinished. The real client activation bypass (B-22.26) and the observed immediate server owner-admission race require their owning lanes, as specified above. Final backend passes include an explicit owner-link readback wait and injected signed-wake transport; they do not prove immediate claim-to-admission readiness, all client automatic callers honoring the gate, public endpoint proof or automatic public wake transport. Visual/browser/physical-device/provider/fleet/deployment acceptance was not exercised. No full suites or deployments were run.
