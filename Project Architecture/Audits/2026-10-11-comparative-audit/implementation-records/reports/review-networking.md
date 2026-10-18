## Outcome

**Implementation follow-up complete: all ten findings are fixed with repository regressions.** The initial review below is preserved as dated evidence; the appended Fixes section is the current implementation status. Targeted tests, vet and Linux/Windows/macOS server builds passed. No push or deployment.

Completed an independent, read-only review of checkout `a093b0d` on `ehler-homelab-mini-1`, including networking merge `87e3fbd` and its commits `e03583b`, `3451f88`, and `cc46fc2`. **10 findings: 0 blocker, 2 high, 5 medium, 3 low.** No source changes, snapshots, commits, deployments or pushes.

The largest regression is that production connection-state wiring closes new HTTP/2 connections before their first request. A separate reproducible security defect lets replaying an old owner settings save restore revoked proxy trust in memory, without changing the current database settings. Ordinary proxy-chain handling, same-origin setup protection, owner certificate validation/reload and removal of the private-only HTTP restriction otherwise have substantial sound implementation behind them.

Read the programme and both lane briefs, the implementation report, relevant original network bug entries, BUG-REGISTER, FIX-LOG and V01/V06 verification entries. Existing focused tests pass. Nine additional regression assertions fail and reproduce eight of the findings below; the other two have concrete source traces. A separate real dual-stack listener check is recorded in scratch.

## Decisions I made

- This lane authorizes findings only. I wrote only this report and lane scratch artifacts. Go overlays inject scratch tests into the package under test without writing source files.
- Universal plain HTTP and open native first-owner setup are intentional programme decisions, not vulnerabilities to undo.
- Same-origin DNS rebinding is a residual trade-off of arbitrary hostname/open setup support. V01 explicitly classifies the setup decision as a deliberate trade-off. Ordinary cross-site pages are rejected; I do not claim DNS rebinding protection or current-browser exploit reproduction.
- Address-hiding TCP proxies/container gateways cannot reveal their original clients. Explicit trusted HTTP forwarding or replacement home networks, including an empty list, is the documented answer; I did not treat the lack of automatic proxy/container detection as a defect.
- The context refresh command failed on its read-only lock directory. Existing canonical project and verification guidance remained readable and was consulted.
- Publication findings use server-side captured outbound observations and read-only Hosted contract code. I did not run or alter Hosted.

## Bugs

### F1 — New HTTP/2 connections close before the first request

**Severity: high.** Location: `server/cmd/server/connection_cap.go:230-242`; installed at `server/cmd/server/main.go:1059`.

The ConnState callback closes any idle connection whose `ordinaryHeld` flag is false. Go's HTTP/2 server emits Active then Idle immediately after reading its connection preface, before reading request HEADERS. Every newly accepted HTTP/2 connection therefore looks like an unused reserve socket and is closed. This happens even below capacity, before the request path or owner authentication can classify it. It affects regular TLS clients as well as the console.

**Evidence:** scratch `TestReviewConsoleHTTP2` uses the production direct-listener, ConnContext, ConnState and HTTP2 policy with a one-connection ordinary budget and an h2-capable client. Its first console request fails with `unexpected EOF`. The runtime trace is visible in `/home/justin/.local/opt/go1.27.1/src/net/http/internal/http2/server.go:799-800`.

**Smallest sound fix:** track whether a real request has been received and do not close the initial HTTP/2 idle transition. Reclaim genuinely idle reserve connections with a bounded grace period, cancelled when another stream starts. Test production wiring with HTTP/2 both below and at capacity.

**Test gap:** `TestConsoleRemainsReachableAtConnectionLimit` disables HTTP/2 and omits ConnState. `TestMainBoundedDirectListenerPreservesHTTP2TLSAndRefresh` exercises the older bounded listener without the new reserve/state callback. Both pass here while production wiring fails.

### F2 — Replayed settings receipts restore revoked proxy trust

**Severity: high.** Location: `server/internal/httpapi/console.go:228-230`, `server/internal/httpapi/connection_policy.go:30-36`; receipt replay at `server/internal/operations/settings.go:404-405`.

ApplySettings correctly replays the historical response to an idempotent request. The HTTP handler then incorrectly installs that historical response as the live network policy. The cache has neither a revision fence nor a subsequent freshness check. A delayed retry of an earlier owner save can permanently restore a removed trusted proxy or broader home networks until another save/restart. GET settings continues to show the newer, restrictive database document. A peer whose trust was removed can once again supply arbitrary client addresses, appear home, and choose rate-limit subjects. This requires a stale authorized owner request/retry; it is not an unauthenticated settings-write bypass.

**Evidence:** `TestReviewSettingsReplayDoesNotRestoreProxyTrust` makes three real authenticated console-settings PATCH calls: trust `203.0.113.9/32`, remove it, replay the first request with its original idempotency key and body. The database remains revision 4 with `trustedProxies=[]`, but a request from that peer with `X-Forwarded-For: 127.0.0.1` returns `Remote:false, ClientAddress:127.0.0.1`. It was remote immediately before replay.

**Smallest sound fix:** publish cached policy monotonically by document revision using an atomic compare-and-swap, including initialization, and do not apply receipt documents as current runtime state. Re-read current settings after a successful save/replay when applying runtime effects. Preserve the historical wire receipt. Cover reordered concurrent save completions as well as replay.

### F3 — Owner media streams consume the entire console reserve

**Severity: medium.** Location: `server/cmd/server/connection_cap.go:181-182`, `server/cmd/server/main.go:1060-1066`.

The owner predicate exempts every request carrying an owner session from ordinary admission, including long-running media transfers. Those connections can consume all `connectionLimit+32` sockets while the ordinary channel remains empty. The next console connection then waits in the listener backlog. This is legitimate owner-session traffic, not a hostile header-stall scenario already excluded by the implementation report.

**Evidence:** `TestReviewOwnerMediaCannotConsumeConsoleReserve` uses the reserve listener and the owner predicate seam, sends 33 owner-classified media requests with connectionLimit=1, and leaves their handlers active. A console GET times out; diagnostics show `ordinary=0 held=33`. The stubbed predicate models exactly main's unconditional owner-role exemption; actual media authentication is not bypassed or tested by this fixture.

**Smallest sound fix:** restrict owner-only reserve exemptions to the specific bootstrap/preferences reads needed to enter/render the console. Media and unrelated owner activity must acquire ordinary slots. Keep authorization on every reserved route. Add the owner-stream saturation case to the reserve test.

### F4 — Scoped IPv6 hosts fail origin and identity-proof handling

**Severity: medium.** Location: `server/internal/networkpolicy/policy.go:218`, `server/internal/httpapi/admin_connectivity.go:228-229`.

validHost rejects every percent sign, including a legitimate IPv6 zone. Origin therefore returns empty for a scoped link-local URL. Identity-proof wiring calls url.Parse on that empty string, gets no error, and replaces the request Host with empty. The proof handler then rejects the otherwise valid submitted BaseURL because its host cannot equal empty. Native clients omitting Origin are affected too; this is a server-side defect independent of browser support for scoped URLs.

**Evidence:** `TestReviewScopedIPv6Origin` reproduces rejection of Host `[fe80::1234%eth0]:32500`, derived from `http://[fe80::1234%25eth0]:32500/`. `RouteOrigin` accepts that origin, but the registered proof wrapper clears its Host before the proof's exact-host comparison. `routeURL` only encodes zones on its non-default-port branch, so port 80/443 also needs canonical encoding.

**Smallest sound fix:** specifically parse bracketed IPv6 and bounded zones, encode/decode the zone consistently for origins and URL generation, and replace the proof Host only after obtaining a nonempty valid origin. Add scoped proof and same-origin write tests; do not permit percent escapes in arbitrary DNS names.

### F5 — Settings entry points apply different runtime effects

**Severity: medium.** Locations: `server/internal/httpapi/admin_diagnostics.go:242-246`, `server/internal/httpapi/console.go:228-235`, `server/internal/httpapi/admin_connectivity.go:93-94`.

Connectivity-policy PATCH refreshes the connection cache and discovery switch but never invokes SettingsApplied. Main uses that hook to refresh delivery settings, wake Hosted publication and invalidate the custom-certificate source. Consequently the advertised immediate/next-operation application promise differs by route: the bitrate snapshot can remain stale until its five-second fallback refresh, certificate source changes wait for its 30-second cache, and publication waits for the next worker tick. Conversely full console-settings PATCH invokes SettingsApplied but never applies the discovery switch. Disabling discovery through the general settings document persists false while the advertiser continues running indefinitely.

**Evidence:** `TestReviewConnectivitySaveWakesRuntime` saves a bitrate setting through the real route and observes zero SettingsApplied calls. `TestReviewConsoleSaveStopsDiscovery` binds an advertiser with a controlled running callback, saves `lanDiscoveryEnabled=false` through the real console route, and observes `{Enabled:false, Supported:true, State:advertising}`. Source search finds discovery Apply only at startup and in changeConnectivity; no general-settings observer stops it.

**Smallest sound fix:** centralize runtime application after both settings entry points, use current settings rather than replayed receipts, and consistently update the network cache, advertiser and SettingsApplied hook. Retain fallback refreshes for missed signals.

### F6 — Hosted observations still contain plaintext LAN routes

**Severity: medium.** Location: `server/internal/networking/remote_manager.go:411-417,576-579`.

Only manual access URLs are filtered to HTTPS. LAN candidates use HTTP whenever the Hosted listener certificate is not ready and are copied directly into the Hosted observation. A claimed server waiting for its certificate therefore still publishes plaintext routes through Hosted, contrary to the programme's TLS-only Hosted-advertisement rule and INSTALLING.md's claim that only HTTPS origins are published.

**Evidence:** `TestReviewHostedCandidatesStayTLS` captures the real remote-manager outbound observation for a claimed server with an HTTPS manual candidate but no usable Hosted certificate. It contains `{BaseURL:http://192.168.1.20:32500, Class:lan}`. Hosted `internal/directory/routes.go:86` accepts HTTP LAN candidates, and `internal/postgres/routes.go` only rewrites LAN origins whose scheme is already HTTPS, so Hosted will not repair this.

**Smallest sound fix:** exclude plaintext candidates from the Hosted observation, including LAN and developer-loopback candidates. Retain HTTP for local discovery/manual local connections. Publish secure LAN candidates once their certificate is usable; do not invent a secure route before that.

### F7 — A lone quote in Forwarded panics the parser

**Severity: low.** Location: `server/internal/networkpolicy/policy.go:161-162`.

A field value consisting of one quote satisfies both HasPrefix and HasSuffix, then is sliced as [1:0]. A trusted reverse proxy that passes the malformed header reaches this parser before authentication. Recovery contains the failure to the request, but this should be rejected as malformed input, not trigger a panic. No process-wide crash or authentication bypass is claimed.

**Evidence:** `TestReviewMalformedQuoteDoesNotPanic`, with trusted socket peer and `Forwarded: for="`, fails with `runtime error: slice bounds out of range [1:0]`.

**Smallest sound fix:** require length >=2 before stripping paired quotes and reject unmatched/empty quoted values. Add malformed one-byte fields for both address and origin consumers.

### F8 — Combined Hosted candidates exceed the contract's bound

**Severity: medium.** Location: `server/internal/networking/remote_manager.go:411-417,499-501,554`.

The topology allows up to eight LAN addresses and the access-URL provider allows eight manual URLs. Only the later public-candidate loop checks the combined twelve-candidate maximum. Five LAN addresses plus eight valid manual URLs produce thirteen candidates, and Hosted rejects the entire observation. Dual-stack/multi-interface/overlay hosts are supported, so individually valid settings can break discovery of every route in the update, including advertiseUrl.

**Evidence:** `TestReviewHostedCandidatesAreBoundedTogether` captures thirteen candidates from exactly that input. Hosted `internal/directory/routes.go:115` rejects observations longer than twelve. This capture proves the invalid outbound payload; no live Hosted request was made.

**Smallest sound fix:** build one deduplicated, prioritized candidate set with a single twelve-entry budget before publication. Keep advertiseUrl first among manual entries and reserve appropriate space for usable public/manual routes, rather than allowing LAN entries to consume everything. Test combined classes, not just the manual provider's eight-entry cap.

### F9 — Connection-limit changes are reported as already effective

**Severity: low.** Location: `server/internal/operations/settings.go:272-281`; actual application at `server/cmd/server/main.go:1050-1056`.

connectionLimit is correctly declared a restart setting, but the settings document reports the newly requested value as Effective and omits it from RestartFields. Only temporaryDirectory has an applied startup projection. Changing the connection limit after startup leaves the listener unchanged while API status claims the new limit is effective and supplies no pending-restart field. This is an observable settings contract defect; requiring restart itself is a permissible decision.

**Evidence:** source trace: the listener's capacity is constructed once from startup settings, no setter/resizer exists, Store has only AppliedTemporaryDirectory, and projectAppliedSettings constructs RestartFields solely from that directory.

**Smallest sound fix:** capture the startup connection-limit value, expose it in Effective, and include connectionLimit in RestartFields when Requested differs. Merge restart fields rather than replacing the list. Add a document projection check.

### F10 — Device registration records the proxy rather than the client

**Severity: low.** Location: `server/internal/httpapi/device_contract.go:29`, persisted at `server/internal/identity/devices.go:197-199`.

The POST /v1/devices route still passes r.RemoteAddr directly to RegisterDevice. Device registration and refresh consequently store the proxy's IP, while sign-in accounting, connection status and request fairness use the verified client IP. The registration upsert overwrites an existing device's ip with this socket address. This leaves the implementation report's single-client-address claim incomplete and misleads device diagnostics; it does not bypass approval.

**Evidence:** reasoned trace through the quoted route, registerDeviceTx's upsert and clientIP(peer). The source search found this remaining inbound raw-RemoteAddr consumer after confirming the policy/rate consumers use networkpolicy.Client.

**Smallest sound fix:** pass d.signInPeer(r), which already uses the bounded shared parser and safe fallback. Verify a trusted-proxy registration records the same IP as the connection document.

### Original network bug dispositions

These are independent review dispositions, not claims that this lane fixed source.

- B-08.1 — Original proxy-origin failure fixed for preserved Host and owner-trusted host/proto; F4/F7 affect edge cases.
- B-08.7 — Original multi-hop/source-limit inconsistency fixed; F2 can restore stale trust, F10 leaves device display accounting inconsistent.
- B-08.12 — gzip q=0 and compressed strong validators fixed by source review; full composition before 304 and asset-cache residuals remain outside this review's scope.
- B-08.18 — Upload routes have separate 60-second read/request budget and capacity two; test checks configuration only, not a slow wire upload.
- B-09.7 — Hidden 256 cap removed and override wired; F1/F3 mean console reachability is not sound, F9 misreports applied limit.
- B-09.8 — Private-only HTTP restriction removed; status/request locality use owner networks. F4 remains for scoped URLs.
- B-09.10 — Retired insecurely contradictory mandatory-TLS setting; superseded by programme decision.
- B-09.21 — Server wildcard default fixed; current Docker/systemd/launchd/Windows service defaults are IPv6 wildcard too after operations integration. Linux dual-stack is checked separately; scoped proof F4 remains.
- B-09.22 — Shutdown completion is now joined before deferred cleanup. Static verification, not a new end-to-end shutdown run.
- B-09.24 — Shared proxy parsing implemented; F2/F7/F10 remain.
- B-09.25 — Proxy environment startup parser no longer used; bare proxy addresses accepted in settings and wildcard/localhost bind accepted.
- B-09.26 — Networking topology documentation covers hidden addresses and multicast limitations. Hardware, permissions and timezone portions were not independently reviewed in this networking lane.
- B-09.33 — Mandatory HTTPS conflict removed. HTTP readiness route can use reserved paths; F1/F3 compromise transport availability generally.
- B-15.3 — Database-backed subjects now use the verified client address; F2 remains.
- B-15.4 — Ordinary cross-site setup blocked by JSON/origin checks and denied cross-site preflight. Open native claiming and same-origin rebinding remain recorded trade-offs, not newly claimed fixes.
- B-15.6 — Shared parser replaces divergent rate/locality/setup readings; F2/F7/F10 remain.
- B-15.8 — Same disposition as B-08.1.
- B-15.11 — Explicit empty home list and trusted-proxy configuration work; defaults still necessarily classify a hidden private gateway as home until owner configuration. The report documents that correctly.
- B-15.22 — Superseded by deliberate universal HTTP; Hosted TLS-only promise remains incomplete in F6.
- B-22.1 — Base fix visible in client route identity alias validation; client acceptance not rerun in this server review.
- B-22.21 — Mandatory-HTTPS bootstrap conflict removed. Signature/nonce/key/host fences remain, with scoped proof failure F4.
- B-22.23 — Report-Every-Ms is in the outer CORS expose list; already fixed on base.
- B-22.24 — Stable distinct interface suffixes remove the identified self-conflicting-name structure; same-VLAN multicast acceptance remains physical-test work.
- N-V01.1 — Hosted signer-revocation/session retirement belongs to accounts-security; not independently reviewed here.

Other bug IDs in the broad original documents concern accounts, playback, general API/client behavior, storage or packaging. Their implementation-report dispositions were not accepted as independent verification or counted as networking findings here.

## API changes

None made by this review. Checked the typed registry registrations for connection, connectivity policy/status, console settings, setup and identity proof. Route authorization still runs; JSON setup admission is retained; route proofs retain strict nonce, current identity/key, signature and expiry fences. Findings F2/F4/F5/F9/F10 concern runtime behavior of these contracts, not new proposed route shapes.

## Schema changes

None made or needed for the proposed fixes. This review did not run schema-sync.

## Owner or viewer settings added or wired

None added by this review. Observed implementation:

| Setting | Observed behavior |
| --- | --- |
| Home networks / lanNetworks | Replacement list; defaults include standard local and attached networks; [] makes all remote. Cache can regress on old receipts (F2). |
| trustedProxies | Empty default; hot shared client/host/scheme policy. F2/F7 need fixing; device IP uses socket peer (F10). |
| advertiseUrl / accessUrls | Local HTTP/HTTPS origins; manual Hosted entries filter HTTPS. Combined publication needs F6/F8, runtime hook F5. |
| connectionLimit | 0 selects memory-based default; owner override <=65536 applied at startup. Restart metadata incorrect (F9); reserve F1/F3. |
| customCertificatePath/key/domain | Validated together, hot source with fallback refresh; content digests and last valid pair implemented. Policy PATCH misses immediate source invalidation (F5). |
| lanDiscoveryEnabled | True default; hot through connectivity PATCH, not full console-settings PATCH (F5). |
| remoteBitrateLimitKbps | Shared locality is used; policy PATCH misses immediate delivery snapshot refresh (F5). |
| remoteSignInPolicy | Networking supplies the shared locality seam; complete sign-in/session enforcement remains accounts-security's scope. |

## Needed from others

- Networking/server implementation owners: fix F1–F9 before treating the networking report as complete. Keep fixes within the existing contract rather than weakening authentication, proxy trust or bounds.
- Accounts/device route owner: F10 needs the shared verified-address seam on explicit registration.
- Hosted owner: confirm TLS-only handling after F6 and the common twelve-candidate budget after F8. The current Hosted contract accepts plaintext LAN entries; no new wire shape is required for server-side filtering/capping.
- Client lanes: continue sign-in HTTP note, external-address/connection-limit screens, actual request-address display and certificate serving-state display. Read RestartFields accurately once F9 is fixed.
- Document owners: correct claims that only HTTPS reaches Hosted and that every settings save invokes runtime application until F5/F6 are resolved. Do not describe setup as DNS-rebinding protected.

## Test notes for other environments

Scratch directory: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-networking/`. Exact regression test source and overlay mapping are retained there. They use loopback/disposable SQLite/temp certificates, not a running Portico installation.

Reproduce all nine failing assertions from this checkout's `server/`:

```sh
GOFLAGS=-p=2 go test -overlay=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-networking/overlay.json ./internal/networkpolicy ./internal/networking ./internal/httpapi ./cmd/server -run '^TestReview' -count=1 -v
```

Output: `all-reproductions.txt`. The wildcard test was added afterward and run separately; its output is `dual-stack.txt`. Initial split reproductions also remain for convenient triage. Expected failure messages are quoted in the findings. After fixes, assertions should pass; preserve the ordinary/TLS/origin/security fences rather than weakening the tests.

Existing focused tests passed in all four packages. Exact selection and output are in `existing-tests.txt`: shared client parsing, empty home networks, certificate matching/rotation/corrupt replacements/expiry/resumption callback, manual access URLs, setup origin/JSON, proxy writes, upload lane constants and current console/h2 tests. No full suite/build was run. The reserve and combined publication regressions explicitly demonstrate holes in those passing tests.

Cold acceptance after fixes:

1. **HTTP/2 and capacity:** run packaged server with a small connectionLimit and usable owner certificate. Negotiate h2 from a normal client; first console and ordinary requests must succeed. Hold ordinary media streams up to limit, including owner-session streams; a fresh console connection must reach normal authentication, while excess media gets retryable busy. Genuine idle reserve connections must eventually release without killing the initial h2 handshake or an active stream.
2. **Proxy policy replay:** trust a test peer, remove trust, replay the historical full settings write. Settings GET and connection classification must retain the newer revision and real peer. A forged home XFF from the removed peer must stay remote. Two genuine clients behind a trusted proxy must keep distinct rate subjects. Malformed Forwarded fields must fail safely without panic.
3. **IPv6:** on Linux, macOS and Windows, check packaged wildcard bind accepts both families. From a real IPv6 link-local peer, use its proper interface zone and complete identity proof before credentials, then a write. Invalid DNS-host percent escapes must remain rejected. No real Mac/Windows/scoped-network acceptance is claimed here.
4. **Runtime settings:** change certificates, bitrate and advertised origin through connectivity-policy PATCH and observe next-operation/immediate effects; disable discovery through the full console-settings document and observe advertisement shutdown. Both entry points should apply the same current committed settings. Change connectionLimit and verify Requested/new versus Effective/startup and pending restart, then restart and verify the new capacity.
5. **Hosted:** with a current claim and pending/broken Hosted certificate but valid manual HTTPS domain, capture the outgoing route set. No HTTP route may be included. Exercise eight LAN plus eight manual candidates: the single final set must be <=12 and retain the preferred external address; existing proofs/signatures/authority checks remain mandatory.
6. **Certificates/discovery:** atomically replace valid PEM pairs with/without Hosted; corrupt or mismatch the new pair and verify previous valid material continues serving with error status. Reject expired resumed sessions. Test two interfaces on one VLAN and real multicast caches. Existing callback/unit tests do not establish physical discovery or real resumed-handshake acceptance.
7. **Slow uploads:** send permitted Live-source JSON over more than 15 but less than 60 seconds, ensure it succeeds and leaves ordinary/console capacity free; oversized bodies stay rejected and stalled uploads terminate. The existing test only checks the configured lane/budget.

## Not finished

The review is complete; no implementation fixes were authorized in this findings-only lane. F1–F10 remain in the reviewed checkout. Physical browser DNS-rebinding/PNA behavior, Windows/macOS listening, real scoped IPv6 discovery/transport, same-VLAN multicast, container address-hiding modes, sustained viewer capacity and slow wire uploads were not qualified here.

### Verdict per networking scope item

| Scope | Verdict | What was checked / remaining fixes |
| --- | --- | --- |
| 1. Plain HTTP from any address; HTTP/TLS sharing and connection note | **Sound with listed fixes** | Classifier has no peer restriction, note derives shared locality, protocol detector preserves tls.Conn. HTTP transport sound; h2 TLS wiring fails F1. |
| 2. One home-network authority | **Sound with listed fixes** | Empty-list/default/overlay/mapped/scoped peers, request/recovery/delivery/rate seams traced. Live policy must not regress on receipt replay (F2). |
| 3. Trusted reverse proxies and same-origin writes | **Sound with listed fixes** | Untrusted headers ignored; rightmost-untrusted chain and preserved/trusted host origin tests pass. F2/F4/F7/F10 remain. |
| 4. First-run setup protection | **Sound for authorized scope** | JSON plus origin/cross-site restrictions and first-owner transaction checked; no new bypass found for ordinary cross-site pages. Arbitrary non-browser first-owner access and already-same-origin DNS rebinding remain explicit trade-offs; no host allow-list introduced. |
| 5. IPv4/IPv6 listening and zones | **Sound with listed fixes** | Wildcard dual-stack check on Ubuntu; current packaging uses IPv6 wildcard. Zone-bearing peer classification sound; scoped origin/proof fails F4. Other OS runtime acceptance outstanding. |
| 6. Owner certificates with/without Hosted | **Sound with listed fixes** | Certificate/key/domain/time validation, content digest reload, old valid pair retention and resumed validity callback checked; isolated focused tests pass. Full h2 connection F1 and immediate policy-save effects F5 remain. |
| 7. Owner external address and Hosted TLS publication | **Sound with listed fixes** | Local document and manual HTTPS priority/filtering implemented. F5/F6/F8 prevent full claim. No new Hosted contract shape necessary. |
| 8. Owner connection ceiling and console reserve | **Not sound** | Override is constructed at startup and ordinary HTTP/1 saturation has reserve. New h2 is killed (F1); owner media consumes all reserve (F3); applied-value reporting wrong (F9). |
| 9. Remaining network-layer fixes | **Sound with listed fixes** | Gzip negotiation/weak validators, upload budget, shutdown join, interface-name removal and distinct discovery names checked. General settings discovery switch needs F5; physical behavior limitations remain as stated. |

## Fixes

Fix work started on branch audit-implementation/review-networking, base 05fd02e, under the owner’s new source-edit authorization. The original review above remains dated evidence; this section records the final implementation disposition. Context refresh still fails on its read-only lock; canonical guidance is readable.

- F1 / F2 — Initial repository regressions confirmed the failures before implementation.

- F1 — Fixed. Reserve idle cleanup now uses a two-second grace period and cancels stale timers on active/closed transitions, preserving HTTP/2's pre-request idle state. `TestConsoleHTTP2FirstRequest` failed with unexpected EOF before the fix; now passes and verifies eventual idle reclamation.
- F2 — Fixed. Connection policy cache rejects older/equal revision publications with atomic compare-and-swap. `TestSettingsReplayDoesNotRestoreProxyTrust` failed before the fix; now a historical owner-save replay leaves the revoked proxy remote. `TestConnectionPolicyRejectsOlderSaveCompletion` covers delayed completion ordering. Current sign-in policy fields from intervening lane merges are preserved.
- High group validation: focused cmd/server and httpapi tests passed; vet passed for both packages; cmd/server build passed. Snapshot 001: Keep HTTP/2 connections open and preserve current proxy trust. Logs: scratch/high-before.txt and high-after.txt.

- F3 — Fixed. Owner reserve exemptions are limited to GET bootstrap, preferences and console registry; media must acquire ordinary connection slots. `TestOwnerMediaUsesOrdinaryConnections` failed before the fix with a second owner media request returning 200; now excess media gets 503 while all required console reads remain reachable. Focused cmd/server tests, vet and server build passed. Snapshot 002: Keep owner media within the ordinary connection ceiling.

- F4 — Fixed. Bracketed IPv6 hosts accept bounded interface zones, preserve zone case, and encode percent delimiters in origins at all ports. Proof wiring no longer clears Host for an empty parsed origin. Endpoint comparisons remove client-local zones (native clients strip them from HTTP Host) while retaining exact address/port checks and the full scoped BaseURL in the signed payload. `TestScopedIPv6Origin`, `TestScopedRouteURLs`, and `TestScopedIPv6IdentityProof` failed before the fix and now pass; the latter verifies the pinned signature and rejects wrong addresses/ports.
- F5 — Fixed. Both complete and partial owner settings saves serialize runtime refresh from the latest committed document, update discovery and policy, and invoke SettingsApplied. Delivery snapshot refresh now finishes before the request context ends. `TestConnectivitySaveAppliesRuntimeSettings` and `TestConsoleSaveStopsDiscovery` failed before and pass after. Historical wire receipts are unchanged; they cannot drive runtime state.
- F6 — Fixed. Automatic LAN routes enter Hosted sets only when the TLS listener is usable; final candidate selection also refuses every HTTP candidate. Retired the obsolete development plaintext loopback publication switch. Local HTTP discovery and arbitrary-address HTTP service are untouched. `TestHostedCandidatesStayTLS` failed before and passes after.
- F8 — Fixed. Manual addresses retain owner order ahead of automatic public/LAN routes; one final deduplicated set is capped at twelve. `TestHostedCandidatesAreBoundedTogether` uses a usable certificate so TLS filtering cannot conceal the budget defect: thirteen valid HTTPS candidates failed before, now twelve are published with all eight manual entries retained. `TestPublishedCandidatesDeduplicateAndKeepManualPriority` also passes.
- Medium group validation: scoped/proxy/setup settings and route-publication tests passed; vet passed for networkpolicy, networking, httpapi and cmd/server; server build passed. `GOFLAGS=-p=2 go run ./cmd/apigen` completed from server/; existing route key scopes remain declared and no generated contract shape changed. Snapshot 003: Support scoped IPv6 and apply consistent network settings and routes. Logs: scratch/medium-before.txt and medium-after.txt.


- F7 — Fixed. Forwarded fields with a lone quote or unmatched quotes are rejected before unquoting. `TestForwardedMalformedQuotes` failed with panics before the fix and now passes for address, host and scheme fields without granting client/origin authority.
- F9 — Fixed. The listener records its startup connection-limit setting. Settings retain the new value in Requested, keep the startup value in Effective and include connectionLimit in RestartFields until restart, preserving other pending restart fields. Partial settings edits merge Requested so they cannot erase a pending limit. `TestConnectionLimitAppliesOnlyAfterRestart` failed before projection/wiring and now passes, including simulated restart adoption and other pending fields. `TestConnectivityChangePreservesPendingConnectionLimit` also passes. Zero still means the automatic memory-based ceiling.
- F10 — Fixed. Device registration uses the same verified client-address seam as sign-in and network accounting. `TestDeviceRegistrationUsesVerifiedClientAddress` configures the actual owner trusted-proxy setting and verifies stored/returned IP on registration, refresh and an untrusted forged-header request. The real test fails against the original route via a source-only overlay (scratch/device-before.txt) and passes against the fix. The initial fixture had put trust only on Dependencies, which the owner settings supersede; that fixture was corrected rather than changing trust semantics.
- All ten findings now have repository regression coverage and fixes. Final targeted tests passed in cmd/server, networkpolicy, networking, httpapi and operations; output is scratch/final-tests.txt. Vet passed for all five packages and the Linux server build passed. The route generator ran again after the device handler change; every changed route keeps its declared key scope. No public wire shape or schema changed, so no client parser or schema-sync changes were required.
- Resumed after the host application restart by checking the current branch/worktree, this report and snapshots 001–003. The source and test artifacts were intact; no completed work was repeated. Platform cross-builds and the remaining snapshots were completed as recorded below.

- Low group validation and snapshot: all targeted tests, five-package vet, route generation and Linux server compilation passed. Snapshot 004: Report pending connection limits and record verified device addresses safely. Before/after evidence is retained in scratch/low-before.txt, device-before.txt and low-after.txt.


### Final verification and disposition

All F1–F10 are fixed; no findings were withdrawn or left open. The source is confined to connection admission, network policy/origins, owner settings runtime projection/application, route publication and the device-address call site. Other lanes’ merged account/conversion code and unrelated work were preserved.

- `GOFLAGS=-p=2 go vet ./internal/networkpolicy ./internal/networking ./internal/httpapi ./internal/operations ./cmd/server` — passed.
- `GOFLAGS=-p=2 go run ./cmd/apigen` — passed after route-handler changes; generated files were unchanged because route shapes/key scopes were unchanged.
- Targeted regression and adjacent behavior tests in those five packages — passed; the exact named tests and outcomes are in scratch/final-tests.txt. No whole suite was run.
- Linux cmd/server build, Windows amd64 and Darwin arm64 cross-builds — passed. Cross-builds used CGO_ENABLED=0 and GOFLAGS=-p=2; all binaries are under /tmp, outside the worktree.
- `git diff --check` — passed. No schema change, commit, push or deployment.

Snapshot 005 is the final cumulative source snapshot, subject “Complete network connection, policy and publication fixes.” The worktree is left unchanged after that snapshot. Snapshots 001–005 and this report are ready for programme integration.

No implementation work remains for these ten findings. Physical Windows/macOS execution, scoped-network interfaces, browser zone support, real reverse-proxy/container deployments, same-VLAN multicast and sustained-load acceptance remain the environment checks described above; cross-compilation and disposable regression fixtures do not claim that acceptance. The deliberately open HTTP/native setup and same-origin DNS-rebinding trade-offs remain unchanged.
