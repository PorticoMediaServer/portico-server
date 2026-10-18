# Outcome

Restored authenticated Vega control, profile/PIN requests and event streams through the existing PorticoRuntime transport interface. Sign-in/refresh/pairing requests are no longer refused before dispatch. Both HTTP and HTTPS work in the bridge; normal SDK TLS validation is unchanged. Unit tests exercise sign-in, refresh, control, remembered credentials, the real native SessionProvider and media readers. No actual device sign-in is claimed.

Known credentials are bound to their exact issuer origin. A new origin is admitted only through the existing signed, client-nonce-bound proof of the same server; the destination proof must be fresh. The shared route owner still performs pin, identity, clock and route checks before dispatch. Credentials from accepted responses and remembered records are registered; redirected responses cannot register replacement credentials.

Every observed response address is checked before headers/body/chunks are handed to a caller. Visible redirects are rejected without an app-initiated follow-up. A followed response at another origin, including an error response, retires the credentials, cancels affected queued/in-flight requests and streams, clears media/captions, starts bounded issuer-only revocation attempts independently of normal admission, and ends the live sign-in. Saved viewer records are removed/quarantined and late persistence/restore cannot republish exposed credentials. Ordinary offline failures preserve sign-in.

HLS playlists, segments and subtitle grants receive the same checks. Playlist resources must stay on the original media origin. Opaque direct media opening is removed from the capability document and engine; the server uses observable HLS, including a plain remux when supported. About has one quiet line in its existing footer, and exposure has one plain sentence beside sign-in.

Verified on personal Ubuntu ehler-homelab-mini-1: **87/87 Vega tests**, run as individual test files, **10/10 tests** in the affected native session file, both application and test TypeScript configurations, both string lints and clean diff checks. Packages remain untouched. No install, helper, subagent, commit, push, deployment or full build.

# Decisions I made

**SDK investigation.** Paths below are under apps/vega/node_modules unless absolute.

- `@amazon-devices/react-native-kepler/Libraries/Network/NativeNetworkingKepler.js`: native sendRequest accepts method, URL, request ID, headers, data, response type, incremental updates, timeout and withCredentials. It also exposes abortRequest, clearCookies and setAuthCertificate. No redirect-policy argument, pre-follow callback, native socket handle or HTTP stream reader.
- `@amazon-devices/react-native-kepler/Libraries/Network/RCTNetworking.kepler.js`: passes that interface, with request/body/tracing conversion; no redirect policy. `XMLHttpRequest.js` exposes incremental text and responseURL from received native response events.
- `@amazon-devices/react-native-kepler/Libraries/Network/fetch.js` loads `whatwg-fetch`; `whatwg-fetch/dist/fetch.umd.js` dispatches XHR and does not honour RequestInit.redirect. Adding redirect:error would make no guarantee.
- `@amazon-devices/keplerscript-netmgr-lib/README.md`: interface/Wi-Fi status, configuration and monitoring, not HTTP requests or stream sockets. `@amazon-devices/react-native-kepler/Libraries/WebSocket/NativeWebSocketModule.js` and `WebSocket.js` offer WebSocket connect/send/close, not arbitrary HTTP; the existing Portico protocol has no WebSocket sign-in/control alternative.
- `@amazon-devices/keplerscript-turbomodule-api/types/codegen.d.ts` and `/home/justin/vega/sdk/vega-sdk/main/0.24.12112/packages/VegaUIReact-TurboModuleAPI/VegaUIReact-TurboModuleAPI-1.x_vodka_libcpp.33540.0/AL2_x86_64/DEV.STD.PTHREAD/build/x86_64/include/Kepler/turbomodule/KeplerTurboModule.h` provide native module scaffolding (also present for aarch64/armv7).
- `/home/justin/vega/sdk/vega-sdk/main/0.24.12112/packages/VodkaSysrootX86_64/VodkaSysrootX86_64-4.0.49.0/AL2_x86_64/DEV.STD.PTHREAD/build/sysroot-x86_64/usr/include/sys/socket.h` provides POSIX sockets; corresponding armv7/aarch64 sysroots exist. No usable target curl/HTTP/TLS API and trust-store integration was found. The `packages/Openssl/Openssl-1.1.x.548848.0/AL2_x86_64/DEV.STD.PTHREAD/build/` libraries are host-tool libraries, not target native HTTP support. This is a finding about the installed supported interfaces, not a claim that building an entirely new native HTTP/TLS stack is theoretically impossible.
- Amazon's [Create a Turbo Module](https://developer.amazon.com/docs/vega/0.24/create-turbo-module) documents C++ module scaffolding and registration. It does not provide the missing redirect/TLS HTTP implementation. The generic React Native networking documentation URL was unavailable; local SDK code is the evidence for redirect behaviour.

Use the brief's expressly authorized weaker fallback instead of the unusable blanket refusal. No new dependency or speculative native module. Credentials remain outside Git; only synthetic test values are used.

Exposure always signs out. Refreshing inside a compromised family is not an independent replacement, and the current protocol has no atomic revoke-and-reissue endpoint without another sign-in. Local families are revoked through existing `DELETE /v1/sessions/current`; Hosted families with a remembered refresh credential use existing `POST /v1/sessions/revoke`. Attempts have five-second deadlines, a 64 KiB response bound, omit cookies, request no-store, validate the final address and do not retry. Failure still leaves the person signed out; no successful revocation is asserted merely because an attempt was started. Password/pairing requests that have not issued a session have no family the app can revoke.

Keep short-lived account access tokens on authenticated control/events and resource/playback grants on media; no account bearer is attached to HLS/subtitles. Password, refresh and explicitly trusted-profile exchanges retain their existing protocol credentials because there is no interchangeable shorter credential for them. HTTP is permitted everywhere. withCredentials=false and no-store are the strongest SDK request controls found; no independent cache-policy switch or no-replay guarantee is invented.

Keep bounds: 12 control requests active/64 waiting, existing request/response sizes, an end-to-end deadline including queue residence, four event streams, 16 KiB pieces and 1 MiB/35-minute stream rollover. Media adds 1 MiB playlist and 32 MiB segment acceptance limits. Credential bindings are hashed and bounded to 2,048; containment holds at most eight families per origin and 64 verified origin/proof entries. Normal sign-out releases its family from the containment inventory.

**Residual risk, for the owner:** Vega may send passwords, refresh tokens or playback grants to another address before the app can detect the redirect, and a chain that returns to the original address is invisible to these checks. Signing out and attempting revocation limits what can still be used, but it cannot recover a disclosed password or guarantee revocation if the connection is malicious or unavailable.

# Bugs

- **B-22.19 — Fixed to the authorized fallback standard, not strict pre-follow parity.** The functional blanket refusal is removed; origin binding, response rejection, credential retirement, revocation attempts, live/durable sign-out and regressions are implemented. SDK automatic following remains the stated residual risk.
- **B-22.18 — Already fixed for selected/pinned route ownership; fixed further here for authenticated Vega streams and exposure containment.** Cancellation and stream bounds remain.
- **B-22.21 — Fixed in Vega profile transport.** The leftover TLS/loopback restriction contradicted HTTP-from-anywhere policy; schemes, address credentials, route/method and size validation remain.
- **B-22.6 — Deliberate trade-off retained.** Signed server proof does not encrypt later plain-HTTP exchanges.
- **B-22.15 — Not redesigned here.** Vega retains 12 active/64 waiting rather than Apple/Android's reserved read/control pools; queue residence is now deadline-bounded. Hardware/load scheduling is not claimed equivalent.
- **B-19.11 — Not changed, deliberate sandboxed files.** Programme policy does not require encryption at rest; no keychain is fabricated.
- **B-21.10 — Already fixed.** Unsupported Vega speed/audio effects remain hidden; seek/rate regressions pass.
- **B-20.1 — Not present as a new Vega defect.** Receiver/output claims remain those of the existing capability document.
- **B-20.12 — Already fixed for app caption style; retained.** The device's system caption-style API remains unavailable.
- **B-25.10 — Partly fixed.** Device-free bridge coverage is now 87 tests; real Vega build/device release acceptance remains unexecuted.

Other IDs enumerated by the broad supporting reports remain with their existing owners/dispositions; this focused lane was assigned the Vega authenticated-transport blocker, not a repeat of the whole native programme.

# API changes

No server route, apikit registry, typed wire shape, legacy route or schema changed. Existing routes above are consumed unchanged. Internal Vega runtime adds optional onCredentialExposure, transportNotice and credentialExposureMessage methods; Apple/Android omit them and do not change behaviour. UI exposure callbacks receive origins only, never session secrets. The existing capability document now omits direct transport on Vega for all origins, including its fallback profile; HLS modes remain.

# Schema changes

None. Existing durable record formats are unchanged; existing selected-viewer quarantine is reused.

# Owner or viewer settings added or wired

None. Quality remains Original by default; no TLS switch, redirect switch or new screen. Existing About footer and sign-in note carry the two messages.

# Needed from others

- Canonical i18n owner: add `vega.connection.redirectLimit` and `vega.connection.credentialExposed` from `apps/vega/src/i18n/transport.ts` to `portico-server/packages/i18n`, then replace this small local catalogue through normal shared-package synchronization. It uses the shared ICU formatter now and supports the shipped English regional catalogues; package copies were not edited.
- Shared/client/server owners: the existing event protocol requires an account access bearer. A dedicated short-lived event grant could further narrow exposure in a future contract, but is not required for this restored implementation and was not invented locally.
- Documentation owner: replace the old Vega release-blocker language in leftovers-native/review-native-js, update Vega's direct-over-HTTPS playback claim and its old `/v1/notifications/events` report reference (current route is `/v1/events`). Update Vega README/architecture and ENGINE-CONTRACT to record observable HLS and the authorized redirect trade-off. Background files were read-only and left untouched.

# Test notes for other environments

Local environment: Ubuntu, Node 24.21.0, existing node_modules symlinks. All logs are in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/vega-transport/`.

| Check | Result |
|---|---|
| Vega individual files: transport, runtime-transport, event-stream, HLS, playback, capability-profile, subtitle-cues, hls-playlist, request-ids, debug | 87/87, zero failed/skipped/cancelled |
| Native `test/session-intents.test.ts` | 10/10, including live-viewer and delayed-restore exposure cases |
| `npx tsc --noEmit`, apps/vega and apps/native | Exit 0 |
| `npx tsc -p test/tsconfig.json --noEmit`, both apps | Exit 0 |
| `npm run lint:strings`, both apps | Exit 0; zero hard-coded strings; native glossary passes |
| `git diff --check`; `git diff HEAD -- packages` | Clean; no copied-package changes |

Existing experimental/module-type warnings and baselined logging/artwork diagnostic notices remain; no failing lint. Tests use the real bridge and SessionProvider with device/storage/network stand-ins; they establish logic, not SDK forwarding, hardware, TLS or renderer acceptance.

Cold acceptance with SDK 0.24, a Vega OS 1.2 Fire TV (virtual device secondary), integrated test server, phone and two isolated capture origins:

1. Build/install a test Vega package through the normal Vega runbook, without modifying dependencies here. Sign in by password over remote/plain HTTP and trusted HTTPS, select a PIN profile, use server Quick Connect and Hosted TV-code approval, browse, control playback and receive events. Pass: real requests leave the initial origin, screens become ready, and credentials are never blocked solely because they exist or HTTP is remote.
2. Restart and restore a remembered session, age access beyond its renewal threshold and verify refresh. Change to a same-server alternate route through a fresh signed proof. Replace the server identity at a remembered address. Pass: verified routes work; a different identity receives no remembered secret. Invalid/expired certificates fail normally; plain HTTP continues to work.
3. For login, refresh, pairing, profile proof, authenticated control, both event paths and media/subtitles, make the issuer answer 301/302/303/307/308 to the capture origin using synthetic secrets. Record SDK forwarding precisely. Pass under this fallback: the redirected answer is never admitted; existing sessions/grants are retired; playback/events stop; an issuer-only revocation attempt begins promptly; the single exposure sentence appears; restarting cannot restore the retired viewer. **Do not require zero captured secrets: this SDK cannot guarantee that.** Same-origin redirected paths also fail; visible 3xx never causes an app-issued follow-up.
4. Repeat with cross-origin final 401/500, unavailable revocation, a redirect on revocation, exposure during delayed restore/verification, and immediate app restart. Pass: no response or late candidate becomes the viewer and no success is claimed for failed revocation. Then disconnect normally without any response: pass, the saved sign-in stays available and no exposure note appears.
5. Play an HTTPS MP4 and HTTP MKV whose codecs are supported. Pass: HLS/plain remux with the original picture codec, no opaque direct opening. Check live channels, music, seek/route restoration, captions and 304 subtitle revalidation. Redirect a playlist, segment or caption document: none of its bytes are rendered. Put a cross-origin URI in a valid playlist: the app sends no request to that URI.
6. Open About: one quiet extra footer line only on Vega. Leave control requests queued, cancel before dispatch, leave streams before headers and cancel an active read. Pass: finite settlement, existing bounds, no lost UI sign-in state or orphaned stream.

# Not finished

No requested local implementation/check remains unfinished. A real pre-follow-refusing native HTTP/TLS transport is not supplied: the installed supported interface has no such seam, so this lane uses the fallback expressly authorized by the brief. Device build/sign-in, capture-endpoint forwarding, normal TLS validation, real playback and visible UI acceptance have not been run; follow the cold steps above before release. Canonical adoption of the two messages remains with its package owner.

Cumulative snapshots: 001 (SDK investigation), 002 (authenticated operation/containment), final **003** (verified implementation). No source-tree changes follow the final snapshot. No commits, pushes or deployment.
