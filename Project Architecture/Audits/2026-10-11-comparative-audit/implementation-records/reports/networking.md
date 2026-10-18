## Outcome

The server accepts HTTP and TLS on the same port from any IPv4 or IPv6 peer. Owner home networks now decide locality everywhere in request admission, sign-in/recovery, credential network accounting and playback delivery. Trusted reverse proxies supply one verified client address, scheme and host parser. The bundled web app can write through TLS termination without an origin allow-list variable.

First-run setup remains open without a local code; both setup entry points reject ordinary cross-site browser requests and non-JSON bodies. Owners can advertise an external HTTP/HTTPS origin and configure a higher ordinary connection ceiling. Console/sign-in requests have a bounded 32-connection reserve, idle reserve sockets are reclaimed promptly, and authenticated owners can load bootstrap/preferences while ordinary connections are full. Existing authorization still runs on every route.

Owner certificates work without Hosted, reload on changed bytes within the 30-second check interval, retain a valid previous pair after a broken replacement, report whether they are actually serving, and reject expired pairs on resumed TLS sessions. Connectivity/settings routes are typed registry contracts. Discovery no longer drops interfaces by name; per-interface DNS-SD names avoid self-conflicting address records. HTTP shutdown is joined before cleanup. Targeted checks and Linux/Windows/macOS compilation passed; no deployment, push or commit was performed.

Snapshots 001, 002 and 003 are cumulative milestones; 004 is the final cumulative snapshot. Final Linux build, Windows/Darwin cross-builds, focused Go tests, vet, web/client-core TypeScript checks and parser tests passed. The tree is left unchanged after the final snapshot.

## Decisions I made

- `lanNetworks` is a complete replacement list. Defaults are loopback, RFC1918, IPv4/IPv6 link-local, unique-local IPv6, and up directly attached subnets (bounded to 64 entries). `[]` means every client is remote. The owner must explicitly add a public hairpin address or an overlay range when wanted; there is no hidden WAN-as-LAN or Tailscale rule.
- Removed `treatWanAsLan` and `secureConnectionsPolicy`, including their shared client setting entries. They cannot remain meaningful under the programme's single locality definition and never-force-TLS rule.
- Proxy headers are trusted only from the owner list. X-Forwarded-For takes precedence when present; otherwise RFC Forwarded is parsed. The full chain is bounded to 32 addresses and malformed entries invalidate it. Unknown proxied clients are remote; rate accounting falls back to the transport peer rather than an attacker-controlled key.
- A browser Origin matching the effective request origin is accepted. On an HTTP backend, an HTTPS Origin matching the exact preserved Host is also accepted, which supports first-run TLS termination before owner proxy trust exists. This does not trust forwarded client addresses or label the transport secure. Rewritten Host/proto metadata requires explicit trusted-proxy configuration. Setup ignores cross-origin API allow-lists and rejects Sec-Fetch-Site: cross-site.
- Native API clients may omit Origin, but setup still requires application/json. Open first-owner access is deliberate; same-origin DNS rebinding cannot be distinguished with the forbidden hostname allow-list, and was not runtime-tested.
- `advertiseUrl` is prioritized ahead of additional accessUrls for Hosted publication. HTTP origins are available locally; only HTTPS reaches Hosted. The existing manual-candidate publication contract is sufficient, so no Hosted shape change is required.
- The owner connection ceiling is a resource default, not a tested viewer count. The ordinary default still uses available memory; overrides are bounded to 65536 and require restart. Reserved sockets remain bounded, expire under header/idle handling, and do not bypass authentication or request-lane limits. This is protection from ordinary stream saturation, not a claim of immunity to hostile connection floods or OS resource exhaustion.
- Outbound SSRF checks, public-address checks for Hosted certificate/DNS proofs, signatures, pairing proofs, key fences and payload bounds were retained. These are not inbound viewer locality decisions. A minimal identity context change carries the already-decided home status into credential network accounting, eliminating its private-address exception without changing account/session policy.
- This work ran on personal mini-1 (ehler-homelab-mini-1). Context refresh failed because its lock directory is read-only; existing shared guidance was readable. No original checkout or private repository was edited.

## Bugs

- B-08.1 — Fixed. Trusted scheme/host derivation and same-host HTTPS origins make bundled writes work behind TLS termination; preserved Host also works before proxy trust is configured.
- B-08.2 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.3 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.4 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.5 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.6 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.7 — Fixed. In-memory limits, database-backed setup/login/quick-code/invitation subjects, sign-in source and lane fairness share the client-address parser.
- B-08.8 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.9 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.10 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-08.11 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.12 — Fixed for gzip q=0 negotiation and strong validators on compressed bodies. Not fixed: validators still require response composition, and asset-cache policy remains with the web/API lanes.
- B-08.13 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.14 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.15 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-08.16 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.17 — Not fixed in this lane: API/event/catalogue/playback ownership. Networking request-origin and address changes do not resolve this separate issue.
- B-08.18 — Fixed for the confirmed large JSON upload routes: Live-source preview/save and client-log uploads have a separate two-request, 60-second lane/read deadline, retaining their byte bounds.
- B-09.1 — Already fixed on the base: restore/integrity routes have the separate admin-long lane. Operations owns further restore work.
- B-09.2 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.3 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.4 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-09.5 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.6 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-09.7 — Fixed. Removed the hidden 256-socket ceiling; the owner can raise the ordinary ceiling, with 32 reserved connections and independent console/sign-in admission.
- B-09.8 — Fixed. Public, overlay and global-IPv6 peers can use HTTP; connectivity status uses the authoritative home list.
- B-09.9 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.10 — Fixed by retiring the incompatible secure-connections choice. The programme accepts HTTP everywhere and never forces TLS.
- B-09.11 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.12 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.13 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.14 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-09.15 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-09.16 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-09.17 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.18 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.19 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.20 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.21 — Fixed in the server default (:32500), including empty-host topology/discovery parsing. Packaged launchers still explicitly bind IPv4; exact changes are assigned to ops-packaging below.
- B-09.22 — Fixed. run waits for HTTP Shutdown and the Hosted shutdown notice before returning into deferred database/service cleanup.
- B-09.23 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.24 — Fixed. One bounded parser consumes X-Forwarded-For or Forwarded only behind the owner-trusted peer; policy/status/rate-limit consumers agree.
- B-09.25 — Fixed in this lane: the obsolete proxy environment variable is no longer read, owner proxy settings accept addresses or ranges, and :port/localhost bind values work through net.Listen.
- B-09.26 — Not fixed in this lane for GPU drivers, container permissions, timezone or runtime packaging. Network setup documentation now explains address hiding; multicast still requires a topology that actually carries it.
- B-09.27 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-09.28 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.29 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.30 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.31 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.32 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.33 — Not present after removing HTTPS enforcement; readiness remains available over HTTP and uses the reserved connection pool.
- B-09.34 — Not fixed in this lane: operations, packaging, backup, diagnostics or playback ownership; no unrelated implementation was changed.
- B-09.35 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-15.1 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.2 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.3 — Fixed. Database-backed identity limits use the same verified client address as the in-memory limiter.
- B-15.4 — Fixed for ordinary cross-site browser setup, including the separate /v1/setup entry point: JSON plus matching browser origin, and cross-site preflight refusal. Open remote first-owner setup is deliberate. DNS rebinding that is already same-origin remains an unverified residual risk; a hostname allow-list is expressly excluded.
- B-15.5 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.6 — Fixed. One parser, a right-to-left trusted chain, one owner trust list; malformed chains are remote and rate-limited under the socket peer.
- B-15.7 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.8 — Fixed with B-08.1.
- B-15.9 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.10 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.11 — Fixed for configurable locality and observed-address reporting. A transport that hides the real address cannot reveal it: trust a forwarding HTTP proxy, remove the gateway range, or set home networks to []. No proxy/container detection was added.
- B-15.12 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.13 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.14 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.15 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.21 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.22 — Not present under the programme: mandatory-HTTPS mode and its exceptions were removed. Hosted route publication remains TLS-only.
- B-15.16 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.17 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.18 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.19 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.20 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.23 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-15.24 — Not fixed in this lane: accounts-security or the relevant provider/filesystem/release owner; the shared networking policy is available to its callers.
- B-22.1 — Already fixed on the base: signed secure-LAN alias validation exists in client-core; the focused route-identity test passed.
- B-22.2 — Already fixed in Hosted per the fix log; Hosted origin de-duplication is outside this worktree and was not exercised here.
- B-22.3 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.4 — Not fixed in this lane. The base fix log records a partial fix/residual gap; remaining client or domain work is outside networking ownership.
- B-22.5 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.6 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.7 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.8 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.9 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-22.10 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.11 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-22.12 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-22.13 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-22.14 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.15 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.16 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.17 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.18 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-22.19 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.20 — Already fixed on the base per FIX-LOG-2026-10-12.md; outside networking ownership and not re-tested in this lane.
- B-22.21 — Not present server-side after retiring mandatory HTTPS. Discovery can bootstrap HTTP while the pinned identity proof remains required.
- B-22.22 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.23 — Already fixed on the base: Report-Every-Ms is in the CORS expose list.
- B-22.24 — Fixed structurally: each interface uses a distinct stable DNS-SD suffix, so its partial address RRset cannot conflict with another interface of this server. Same-VLAN physical-interface acceptance remains outstanding; the original finding was speculative.
- B-22.25 — Not fixed in this lane: client-side recovery, storage or transport behavior; client lanes own these consumers.
- B-22.26 — Not fixed in this lane. The base fix log records a partial fix/residual gap; remaining client or domain work is outside networking ownership.
- N-V01.1 — Not fixed here: revoked Hosted signing-key session handling belongs to accounts-security and needs its own schema/policy work.

## API changes

- Added `GET /v1/networking/connection` (public, typed): `{secure, remote, plainHttpOutsideHome, clientAddress, accessUrls}`. Shared `HttpLocalApi.connection()` and strict bounded parsing are supplied. Returned addresses include advertiseUrl, additional accessUrls and a configured custom-certificate domain at the listener port.
- `GET`/`PATCH /v1/admin/connectivity/policy` moved to the typed registry. Removed secureConnectionsPolicy and treatWanAsLan; added connectionLimit (integer) and advertiseUrl (string). Other existing fields remain. PATCH keeps expectedRevision/operationId and owner-only transactional authority.
- `GET /v1/admin/connectivity/status` moved to the typed registry. Added `connection` with the public connection shape. `tls.customCertificate.serving` distinguishes a broken replacement from a lost usable certificate. Advertised IPv6 literals report their actual address/family.
- `GET`/`PATCH /v1/admin/console/settings` now have typed registry contracts for their existing scope/data envelope and complete settings write. Existing console validation and transactional authority rechecks are reused. Settings JSON has the same removed/added fields as above.
- `POST /v1/setup` and `/v1/setup/browser` are typed public registry routes, with unchanged successful setup/session and browser-resume-secret shapes. Both require JSON and same-origin browser requests; browser setup requires Origin. Browser setup now returns `setup_already_complete` (409) after initialization.
- `POST /v1/networking/identity-proof` is a typed public registry route with its existing `{baseUrl, nonce}` request and `{payload, signature}` response. Public HTTP origins and trusted external Host work; nonce/key/signature/expiry rules remain.
- No public URL was retained as a compatibility alias or removed as a feature. Duplicate manual YAML route definitions and their unused setup/settings/connectivity schemas were deleted. Generated OpenAPI and TypeScript contract artifacts were regenerated.
- Gzip negotiation now respects q=0 and uses weak ETags for compressed bodies/304s. Large Live-source and diagnostic uploads retain existing payload shapes/byte limits but get a separate two-request 60-second read/request budget.

## Schema changes

None. Settings remain in the existing owner settings document; no migration or schema-sync was needed.

## Owner or viewer settings added or wired

| Setting | Type/default | Runtime effect | Screen status |
| --- | --- | --- | --- |
| lanNetworks (Home networks) | string list; standard local ranges plus attached subnets | Hot, sole inbound locality authority; [] makes all remote | Existing connectivity screen; client lanes must update help/empty-list behavior |
| trustedProxies | string list; [] | Hot, trusted address/host/scheme headers and all rate/fairness subjects | Existing screen |
| advertiseUrl (External address) | string; empty | Hot, local connection document; HTTPS manual Hosted candidate prioritized | New screen row needed |
| accessUrls | string list; [] | Hot; local clients accept HTTP/HTTPS origins; Hosted filters HTTPS | Existing screen/help update needed |
| connectionLimit | integer; 0 = existing memory-based default; max 65536 | Restart; ordinary connection capacity plus fixed console reserve | New numeric screen row needed |
| customCertificatePath / customCertificateKeyPath / customCertificateDomain | strings; empty, configured together | Hot source, PEM file reload and live/resumed TLS validity | Existing screen; serving state display needed |
| advertisedInterface | string; empty | Explicit interface preference; automatic no longer filters names | Existing screen/help update needed |
| lanDiscoveryEnabled | boolean; true | Hot advertisement lifecycle; home-network changes refresh discovery topology | Existing screen |
| remoteBitrateLimitKbps / uploadCapacityKbps | integers; 0 | Existing caps now use the same owner home decision as sign-in | Existing screens |
| remoteSignInPolicy | enumeration; allow | Existing request locality seam is unified; all-route policy enforcement belongs to accounts-security | Existing screen |

Retired secureConnectionsPolicy and treatWanAsLan. Added catalogue IDs for the HTTP note, external address, connection limit and setup-already-complete wording; no viewer preference was added.

## Needed from others

- Client lanes: use HttpLocalApi.connection() on sign-in and show `web.connectivity.httpOutsideHome` as one quiet line only when plainHttpOutsideHome is true. Render connectionLimit/advertiseUrl; show the status connection clientAddress/locality and custom certificate serving flag. Update LAN/help copy and remove any independently retained required-HTTPS/WAN-as-LAN choices in native/Vega screens. Shared server-side/client-core entries are already changed here.
- Ops-packaging: change explicit `PORTICO_BIND=0.0.0.0:32500` to `PORTICO_BIND=:32500` in Dockerfile, packaging/linux/portico-media-server.service, packaging/macos/tv.getportico.server.service.plist, and packaging/windows/installer.nsi. These files are owned by that lane and were not changed here. State explicitly that bridged Docker multicast is not LAN discovery; use a topology that carries multicast, or explicit connection addresses.
- Accounts-security: preserve/use Dependencies.RequestIsRemote or its existing privateSetupPeer wrapper for every sign-in/recovery flow. Credential network accounting now consumes networkpolicy.Home(ctx), populated by the router; other account changes must not reintroduce IsPrivate-based classification. Public fingerprint/session-policy findings remain their ownership.
- Hosted: no new wire contract is needed. Verify existing Class=manual HTTPS candidates remain accepted for arbitrary owner domains, including behind CGNAT, and de-duplicated after secure-LAN normalization. Server publication filters out plaintext and retains claim/proof fences.
- Documentation owners: Product Contracts/architecture-amendments.md's private-only HTTP rule and the old networking/link/security contract references to mandatory HTTPS and WAN-as-LAN conflict with the programme. Update them to universal HTTP, owner home networks and TLS-only Hosted directory routes. INSTALLING.md and the server connectivity API guide are updated here.
- First-party address-entry/native transport lanes: finish device/platform acceptance for cleartext public addresses and IPv6 scoped-link-local URLs. This lane normalizes scoped socket peers and retains encoded zones in server-generated literal URLs; browser URL APIs and device discovery/transport support still require platform testing.

## Test notes for other environments

Verified here, on Ubuntu personal mini-1 with GOFLAGS=-p=2 and the shared programme GOCACHE:

- `go build ./...` (server module).
- Targeted networking, networkpolicy, connectivity, HTTP API and cmd/server tests: public/private/overlay/IPv6 listener traffic, TLS/h2 behavior, proxy chains/Forwarded, empty home networks, setup JSON/origin rules, trusted/preserved Host writes, owner settings round-trip, route/lane/OpenAPI coverage, console access at connection capacity, bounded shutdown, certificate reload/expiry/resumption, per-interface names, HTTPS-only publication, gzip negotiation and IPv6 report parsing.
- `go vet` on internal/networkpolicy, networking, connectivity, hostlimits, httpapi and cmd/server.
- Windows and Darwin cmd/server cross-builds with CGO_ENABLED=0. Outputs are only in /tmp, not the worktree.
- `npx tsc --noEmit -p web/tsconfig.json` and the client-core tsconfig. Focused route-identity and server-administration Node test files passed.

Cold acceptance instructions:

1. Windows/macOS/Linux: run the packaged server with :32500 on a host with IPv4/IPv6. From another machine, GET /v1/server and perform a pinned identity proof through both address families. Plain HTTP from an overlay/global address must reach normal authorization; neither family may silently disappear. Validate localhost and explicit-IP overrides. Packaging defaults must first be integrated.
2. Reverse proxy: disposable fresh state, own hostname, TLS frontend/plain backend. Preserve Host and do browser setup without an origin variable. Then configure the exact trusted proxy peer and test rewritten Host, X-Forwarded-* and RFC Forwarded. Two real clients must have different connection addresses and login/device-code buckets. A direct untrusted peer spoofing headers must remain its socket address. Another site's form/fetch/preflight must not create an owner.
3. Docker bridge/rootless/tunnel: inspect status.connection before policy changes. Configure forwarding trust when possible; otherwise set home networks to [] or remove the hidden gateway range. Recovery/remote streaming policy must consistently report remote. Do not expect original addresses from a TCP tunnel that does not send them. Multicast discovery must be separately tested on actual container/network modes.
4. Owner certificates, with and without a Hosted claim: use a trusted PEM chain for the configured domain; request TLS status and connect through SNI. Atomically replace the PEM pair; the next check within 30 seconds must use the new bytes even with preserved timestamp/size. Corrupt it: status is error but serving remains true for the previous valid pair. Advance beyond expiry/reconnect using a saved TLS session: expired material must fail and HTTP/console must remain usable.
5. Capacity: set a small connectionLimit and restart. Hold that many ordinary authenticated media connections. Open the console cold (including sign-in/bootstrap) through HTTP and TLS; it must reach its normal authorization and render requests while another ordinary request gets retryable busy. Header stalls/reserve floods are bounded, not a guarantee of DDoS capacity. Test HTTP/2 clients, OS descriptor limits and real target load separately.
6. Discovery: two interfaces on the same VLAN and separate IPv4/IPv6 LANs, then an interface change. Check unique per-interface DNS names, no self-withdrawal loop, grouping by server identity, scope handling on real Apple/Android/Vega clients and multicast caches. Standards conformance/full simultaneous-probe behavior was not claimed from unit tests.
7. Uploads: deliver a permitted Live-source JSON body over 15 seconds but within 60 seconds. It must finish normally without occupying ordinary console slots; an oversized body remains rejected, a stalled 60-second upload ends, and only two uploads run at once.

## Not finished

No known unfinished server implementation within the networking lane's owned scope. Client screens, packaged launcher defaults and physical proxy/container/discovery/device acceptance remain with the named lanes. DNS-rebinding/private-network browser behavior, hostile-reserve saturation and 200-browser/100-stream capacity are not runtime-qualified here. Other domains' bugs are recorded individually above rather than claimed fixed.
