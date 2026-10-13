# Discovery, Routing, Remote Access, and Resilience Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-NETWORKING`  
Revision: `portico.october.1`  
Status: **approved**  
Owners: Product, Security, Hosted Infrastructure, Server Networking, Client Foundations  
Imports: `PC-FOUNDATION` viewer scope, permissions, and resource lanes; `PC-GOVERNANCE` lifecycle/exception law  
Coordinates with: `PC-IDENTITY`, `PC-ONBOARDING`, `PC-PLAYBACK`, `PC-ACCESSIBILITY-LANGUAGE`  
Applies to: Hosted Services, Direct DNS, Portico Server, Web, Client Core, and every first-party client

## Scope and non-goals

This contract owns discovery, address normalization, trust, route selection, direct access, Hosted presence, certificate behavior visible to the product, retries, offline classification, and resilient server switching. It does not define the physical hosting topology, vendor-specific deployment commands, media delivery format, a Portico media relay, VPN service, or enterprise network management. Relay and paid bandwidth products are explicitly out of scope.

There are no unresolved network product decisions in this revision. ACME vendor failover is an operational choice constrained by the public-trust and security invariants here, not a client-visible product fork.

## 1. Network architecture

Portico has a Hosted control plane and direct server data planes.

```text
Client ── account, membership, signed route coordination ──> Hosted Services
Client ── API, metadata, images, events, playback, downloads ──> selected Portico Server
```

Hosted Services never relays, proxies, caches, transforms, inspects, or meters media, library metadata, artwork, subtitles, playback history, downloads, or server-local settings. Once a server-native session exists, ordinary product traffic and token refresh go directly to that server.

Hosted Services remains involved only for account session renewal, server directory/selection, membership or invitation changes, Hosted profile work, claim/certificate/route coordination, and recovery when no verified route can be used.

## 2. Public and transport domains

- `web.getportico.tv` is the canonical user-facing Hosted application, callback, device-entry, claim, and account-management domain.
- `getportico.tv` contains public information, terms, and privacy pages.
- There is one Hosted origin: browsers, native apps and user servers all use `web.getportico.tv` (its edge forwards `/v1/*` and `/auth/*` to Hosted). `api.getportico.tv` is a legacy alias that nothing in this build uses by default (2026-09-17); it must never appear in instructions, QR destinations, browser callbacks, address bars, error text, emails, or client copy.
- `direct.getportico.tv` and its descendants are server transport names, not application destinations.
- A bundled server Web app remains on the exact local server origin.

Redirects between user-facing Hosted routes preserve only opaque, single-purpose continuation tokens. Provider claims, account email, server credentials, device secrets, and private route details never enter query strings.

## 3. Route classes and order

Supported route classes are:

1. verified LAN route discovered or locally configured;
2. verified public Portico direct TLS route from a signed route document;
3. explicitly configured manual route when server policy permits it.

A route is a transport candidate for one pinned server identity, not a sign-in realm. Switching from LAN to WAN must preserve the same server-native credential, viewer scope, playback session, and server public-key fingerprint.

Default selection gives a healthy verified LAN candidate a small head start over a healthy current public candidate, while probing both as one bounded first-success race. Preference never serializes a complete LAN timeout ahead of WAN. The first candidate that cryptographically proves the selected server identity through its approved transport wins; an echoed server ID/fingerprint alone is not proof of private-key possession; every losing probe is cancelled. The last-known-good current/previous verified candidates remain eligible while permitted by policy. A different server identity is rejected even if it responds with a valid Portico API.

## 4. Manual address normalization

Address input is a URL/hostname field, not natural language:

- disable spelling correction, smart quotes/dashes, autocapitalization, and content suggestions;
- trim surrounding whitespace;
- for a bare public host, try HTTPS, then HTTP on the documented Portico port;
- if a bare recognized local hostname/IP is entered, use the documented local default port and local HTTP policy;
- preserve an explicitly supplied valid scheme and non-default port;
- remove only a trailing root slash during canonicalization;
- reject username/password components, protocol-relative values, backslashes, control characters, fragments, and unexpected paths or query parameters;
- routes advertised by Hosted require HTTPS;
- direct HTTP is accepted at any address, including public names and IPv4/IPv6 literals;
- do not silently follow a redirect to a different origin until that origin proves the expected server identity.

Normalization is followed by public system-info/compatibility verification and identity pinning before credentials are sent. If the input is malformed, explain the accepted examples. If the host is syntactically valid but unreachable, preserve the entered address and classify connectivity separately.

## 5. LAN discovery and trust

Native clients discover `_portico._tcp` using the platform's mDNS/Bonjour/NSD facility. Browser clients use only browser-supported local-network mechanisms and permissions; lack of browser discovery is not proof that a server is offline.

Protocol-v1 discovery records include:

- service/TXT version;
- display name;
- optional opaque server ID;
- persistent server public-key fingerprint;
- local HTTP scheme, port, and allowed path;
- observed interface/address and bounded TTL.

Normalize, validate, and deduplicate records by persistent fingerprint. Multiple interface advertisements for one fingerprint are candidates for one server. Conflicting server ID, port, or path within one fingerprint group is an identity conflict and yields no usable route.

Discovery is untrusted metadata until matched against either:

- a signed Hosted route document for the selected server; or
- a fingerprint saved during an explicit local pairing confirmation.

After an authenticated Server connection succeeds, the client durably stores the exact raw 32-byte Ed25519 Server identity public key together with its canonical fingerprint and server-native credential. The raw key comes only from the verified signed Hosted route/attachment identity or from the Server's successful native-session credential response; the client recomputes and compares the fingerprint before treating it as an offline-verification pin. A fingerprint alone is not enough because a client cannot reconstruct a public key from its digest. A restart that has credentials or a fingerprint but no durably verified raw key remains fail-closed for signed offline material until the authenticated Server supplies the complete identity again; it never infers, substitutes, or accepts a key carried by a receipt.

Stale TTL records are not connection candidates. A discovered host cannot override a pinned fingerprint, change the chosen transport without the configured address-selection flow, or cause credentials to be sent before verification.

Local Network permission denial is its own state. Clients explain how to enable it and may still use a verified public route; they do not misreport the account or server as invalid.

Local Network permission and subnet reachability govern only LAN discovery and LAN routes. They MUST NOT gate Hosted device-code entry or approval. A mobile client always permits manual entry of a TV's current eight-character activation code without discovery, a local-network permission prompt, or same-subnet reachability. Nearby TV results are explicitly labeled as local convenience candidates and remain untrusted until the Onboarding Contract's authenticated, TV-bound approval completes.

## 6. Server directory and selection

The Hosted server directory is account state, not a presence feed. Clients retrieve it:

- at account sign-in or explicit account restore;
- when the user opens or refreshes the chooser;
- after an invitation, membership, claim, transfer, or deletion event;
- after a server-directed wake or other explicit account/server change signal;
- when no selected/cached route can be recovered.

It is not polled on a per-screen, per-player, or short background interval.

Selection states are:

`account_authenticated → directory_loading → zero_servers | choose_server | reconnect_last_server`

Connection states are:

`candidate_discovery → candidate_probe → identity_verified → bootstrap_exchange → server_credentials_persisting → viewer_verification → connected`

Side states are `offline`, `server_unavailable`, `hosted_unavailable`, `local_permission_denied`, `route_stale`, `identity_mismatch`, `version_incompatible`, `membership_pending`, `membership_removed`, and `rollback_uncertain`.

No state substitutes fabricated values such as an account or server named `Portico`. A selected unavailable server must leave server chooser, account settings, and sign-out operational.

## 7. Signed route documents

Hosted Services signs canonical Ed25519 route documents containing document kind/version, audience, server ID/name, persistent public key and fingerprint, current authorization/membership projection, route candidates, endpoint generation, issue/expiry, algorithm, and key ID.

A signed route candidate has exactly one of two consumer-facing quality values: `reachable` when Hosted has current identity-pinned reachability evidence, or `probe_required` when the current or single eligible previous-verified candidate remains coherent but requires a client probe. Hosted maps reported, stale, checking, DNS-synchronized, repairing, TLS-failed, HTTP-failed, failed, and unknown public or LAN evidence to `probe_required`; detailed statuses remain PostgreSQL-backed operator and repair evidence rather than a second client state machine. A current-generation Hosted-proven `identity_mismatch` candidate is omitted entirely until a newer coherent topology/observation generation replaces that evidence. Consumers rank `reachable` before `probe_required`, but both qualities still require the normal server-ID and public-key-fingerprint probe before connection.

Consumers reject unknown/colliding keys, invalid/non-canonical signatures, wrong audience/kind/server, unsupported required semantics, expired documents, implausibly future-issued documents, identity mismatches, and candidates not represented by the signed document.

Clients may retain only the current and immediately previous verified document/candidate set as bounded recovery evidence. Cached evidence is account- and server-scoped, encrypted or platform-protected where available, and never promoted beyond its validity or revocation rules.

A route probe checks a minimal public identity/compatibility endpoint with a short timeout and bounded concurrency. It verifies the authenticated transport and pinned identity as specified in the connectivity amendment below; static server ID/fingerprint strings alone cannot authenticate a candidate. Probe failures are quality evidence, not authorization decisions. Later launches race remembered identity-verified addresses first. An address may remain a discovery hint after its discovery TTL or signed route document expires; expired evidence is not current route authorization. Reuse requires fresh transport/identity verification and valid server-native credentials and policy. A stale raw discovery advertisement never becomes a remembered verified candidate. If no remembered candidate succeeds within 500 milliseconds, the client starts fresh Hosted discovery while those probes continue; a conclusive failure of every remembered candidate starts Hosted immediately. A verified cached winner before that boundary creates no Hosted route-discovery request. Bonjour/local discovery and signed public-route probing run concurrently and the first identity-confirmed route wins.

### 7.1 Server-owned membership continuity

Membership is owned by the selected local server, with changes pushed to Hosted's discovery index. There is no seven-day Hosted policy lease or daily signed membership renewal in current admission. Established local authority does not expire merely because Hosted is unavailable. Hosted identity admission, current signer revocations and explicit account departures still have their own proof/fence checks.

Check-in compares the server member sequence/digest, reconciles departures and may deliver a root-signed signing certificate. A nullable certificate is accepted when Hosted's signer is unusable; no signed authority is trusted without verification. This is current source behaviour in `internal/hosted/current_schedule.go`, not an extension of an expired signed-policy document. Historic P05 is superseded as an implementation description.

## 8. Direct DNS and certificates

Each claimed server receives an independent 96-bit random DNS namespace token unrelated to internal server ID, credentials, and public-key identity. Public hostnames encode the current address beneath `<dns-token>.direct.getportico.tv`; authoritative DNS is stateless with respect to per-server addresses.

Each server generates and retains its certificate private key and obtains a publicly trusted wildcard certificate for `*.<dns-token>.direct.getportico.tv` using DNS-01 through the delegated validation zone. WAN address changes therefore require neither certificate issuance nor mutable per-server DNS records.

The preserved operational preference is Google Public CA production; activation must verify current issuer availability and deployment configuration. Provider selection is an operational abstraction: Let's Encrypt is the required configurable fallback target, not a claim that the rebuilt adapter is already implemented, but clients rely only on ordinary public Web PKI and must not encode provider-specific assumptions. Hosted public domains likewise use Google Public CA unless an operational emergency invokes the documented fallback.

Requirements:

- CAA permits only the selected production issuer and DNS-01 policy supported by that issuer;
- certificate order/create/finalize/revoke are durable, leased, fenced, retryable jobs;
- ambiguous provider acceptance enters reconciliation, never duplicate blind issuance;
- private keys never leave the server;
- validation secrets, EAB material, ACME account keys, and DNS update credentials never enter client code, server claim payloads, logs, or diagnostics;
- Certificate Transparency monitoring alerts on unexpected issuance;
- renewal begins with enough margin and fleet jitter that provider or Hosted outage cannot create a synchronized expiry event.

Certificate status is not server login status. An expired/mismatched public certificate may make the public route unavailable while a verified LAN route continues safely.

## 9. Hosted check-in and signed wakes

The server's current loop runs event-driven work plus Hosted-supplied check-in: weekly when reachable by signed wake, daily otherwise, with per-server jitter and persisted retry floors. Network observations wake local evaluation but do not force a Hosted request. Membership deltas keep the index current; a mismatched digest/sequence requests bounded full reconciliation.

Signed content-free server callbacks accelerate erasure, signer and route/index convergence. They are coordination messages, not consumer push notifications. A wake is not media authority; the server verifies current documents and exact generation before acting. Missed callbacks use check-in recovery without signing people out merely for an outage. Fleet rollover and real callback acceptance remain unverified.

Ordinary playback, screens, notifications and metadata communicate with the selected server, not Hosted.

## 10. Retry and backoff policy

Retry policy is operation-classed and cancellation-aware.

| Work | Initial behavior | Backoff | User action |
| --- | --- | --- | --- |
| Visible activation/setup code refresh | retry automatically while preserving current valid code | start about 5 s, exponential with jitter, cap 30 s | no **Try again** |
| Selected server reconnect | race remembered LAN/public candidates; hedge Hosted discovery after 500 ms without a verified winner | bounded exponential with jitter, then low-frequency background retry | optional **Choose another server**; manual retry may accelerate |
| Hosted account/control-plane recovery | preserve account/server credentials | exponential with jitter, cap appropriate to foreground/background | no sign-out; explicit retry only for a failed user mutation |
| Check-in and signed wakes | event-driven plus daily/weekly backstop | bounded retry with persisted Retry-After and jitter | settings show factual state |
| User mutation | one request plus safe idempotent retry where receipt exists | honor `Retry-After`; never blind-repeat non-idempotent work | actionable **Try again** may be offered |
| Media API/playback | server-owned recovery policy | playback has priority; route replacement preserves session | player explains only when recovery cannot continue |

All retries stop on explicit cancellation, terminal authorization, identity mismatch, unsupported contract, or a newer operation generation. `429` honors `Retry-After`. Reconnect storms are prevented with jitter, single-flight, shared circuit state, and maximum parallel probes.

## 11. Offline and failure semantics

Classify before presenting:

- **device offline:** failure evidence shows no usable path for the requested operation; a general internet-reachability flag alone does not establish that a local server is unreachable;
- **local permission denied:** OS/browser denied LAN access;
- **DNS failure:** hostname resolution failed;
- **TLS trust/identity failure:** certificate or pinned server identity mismatch; security-terminal for that candidate;
- **server unavailable:** routes were verified but no candidate responded;
- **Hosted unavailable:** control-plane unreachable while local/server operation may continue;
- **membership/authorization terminal:** authority explicitly denies current membership or credential;
- **unknown/transient:** insufficient evidence; preserve state and retry.

Messages state impact and next action. They never expose raw exception strings, internal domains, full route documents, tokens, public IP evidence, or certificate internals by default. Technical evidence belongs behind diagnostics.

A Hosted outage must not interrupt local authentication or an established server session under its current server-owned membership and credential authority. A server outage must not sign out the Portico Account. A route failure must not clear a durable credential. Ambiguous auth responses must not be treated as revocation.

## 12. Persistent connection transaction

Connecting or switching is a fenced, compensating transaction:

1. snapshot prior account/server/profile/runtime state;
2. discover and verify candidate routes in isolation;
3. check public server/API compatibility;
4. obtain exact-server bootstrap when Hosted authority is used;
5. exchange at the verified server for a server-native family;
6. verify issued scope and final `/api/auth/me` scope;
7. fence/drain the old runtime;
8. atomically persist route, pin, and credential;
9. read-verify durable state;
10. publish the new runtime and remember it as last successful server.

Failure before publication preserves the old runtime. Failure after fencing restores the complete prior snapshot only if compensation is verified. Uncertain durable state clears/quarantines candidate and previous credentials and remains fail-closed. A late older operation cannot publish over a newer choice.

## 13. Security and privacy

- Never send credentials to an unverified route or follow cross-origin redirects with credentials.
- SSRF boundaries reject public/manual URLs that target disallowed local, metadata-service, multicast, broadcast, unspecified, or credential-bearing endpoints outside the explicit LAN policy.
- Forwarded headers are trusted only from configured proxy CIDRs.
- LAN/public classification uses the actual selected route and trusted proxy model, not attacker-supplied headers or discovery labels.
- Network diagnostics redact account IDs, tokens, route secrets, private email, exact media URLs, and private LAN addresses by default.
- Route failure reporting is bounded and contains no media request path or credential.
- Direct DNS has no application database, Redis, Hosted environment, or credential access.

## 14. Conformance

Connection snapshots validate against `connection-state.schema.json`. Every implementation runs `conformance.json`, including manual URL normalization, spoofed discovery, stale TTL, conflicting advertisements, signed-route expiry, LAN permission denial, route failover during playback, no-server and multi-server account states, Hosted outage, lost credential-write acknowledgement, late candidate completion, refresh classification, push duplication, fleet polling limits, and certificate-provider independence.

Minimum applicability matrix:

| Rule family | Server | Hosted/DNS | Web | Client Core | Apple | Android | Roku | Cast |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Direct data path | required | no relay | required | required | required | required | required | required |
| LAN discovery | advertise | not applicable | browser-capability | normalize/verify | required | required | required | receiver-capability |
| Signed public routes | verify identity | issue/sign | required | required | required | required | required | required where direct |
| Manual address | receive/verify | not applicable | required | required | required | required | required | sender-owned |
| Presence/push | required | required | account events | helpers | wake consumer | wake consumer | wake consumer | not applicable |
| Retry/offline | required | required | required | required | required | required | required | required |
| Certificates | key/order client | coordination/DNS | Web PKI | CA-independent | CA-independent | CA-independent | CA-independent | CA-independent |

Platform adaptations may use native Bonjour/NSD, local-network permission APIs, HTTP stacks, and secure storage, but cannot trust discovery without identity proof, send credentials before verification, turn a Hosted outage into product outage, or implement a media relay.

Approved exceptions use the full governance record with `Contract: PC-NETWORKING#<section>` and conformance evidence.

See `plex-jellyfin-precedent-review.md` for the official mature-product evidence considered and the precedents Portico deliberately rejects. That review is informative and cannot override this contract.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## Home networks and server transport — October decisions

`lanNetworks` is the complete owner home-network list. Defaults include loopback, private/link-local/unique-local ranges and bounded directly attached subnets. An empty list makes all peers remote. No Tailscale, proxy or WAN-as-LAN detection creates another locality rule. Home classification governs sign-in, delivery and network accounting.

Plain HTTP and TLS share the server port. TLS is encouraged, never forced. Remove mandatory-HTTPS policy. A quiet note may explain HTTP outside home networks; no unsafe-choice detection or blocking flow is added. Trust forwarded headers only from configured proxies, with bounded validated chains. The owner's hostname reverse proxy and certificate pair are supported. Certificate reload retains a valid old pair after an invalid replacement and reports actual serving state.

IPv4, IPv6 and overlays remain supported; CGNAT is not refused. Connectivity still requires a working path. Sub-path hosting is excluded; relay is future work. Docker bridge multicast does not establish LAN discovery. Scoped IPv6 URL handling has regression coverage but needs browser/device acceptance.

Hosted candidates remain TLS-only even though locally entered addresses may be HTTP. There is no confidentiality claim for HTTP. Owner connection-limit overrides require restart and expose requested/effective values. A bounded reserve protects necessary sign-in/console reads; keys and owner media use ordinary capacity. Sustained capacity and actual proxy/container/network acceptance remain unproven.

Hosted server callbacks are signed coordination wakes, not consumer push. TV activation and background requests must honour the shared per-origin outage floor, Retry-After and positive jitter; a 5–30-second protocol retry cap must not shorten a longer shared outage floor.

## October scope and evidence

The appended review fixes supersede the initial ten findings. Runtime HTTP/2/proxy-policy/scoped-IPv6 regressions pass; actual proxy/container/browser discovery, IPv6 zones and capacity acceptance remain open. Never call setup DNS-rebinding protected.

Sources: [networking](../../../_program/reports/networking.md), [review-networking](../../../_program/reports/review-networking.md), [native-sync](../../../_program/reports/native-sync.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
