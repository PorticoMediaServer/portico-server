# cp3-network — owner certificates, Apple local HTTP, browser access

## 1 Outcome

- **Browser access with no setup.** Any browser page on any origin can call the API with a token (API key, personal access token, session token in `Authorization`, or a media/download URL carrying its grant). The server answers its preflight and its request with `Access-Control-Allow-Origin` for that origin, and never sends `Access-Control-Allow-Credentials`. Requests from another origin *without* a token are still refused (`403 origin_not_allowed`), and first-run setup keeps its strict same-origin rule. The server's own web app works at any address a browser uses (IP, hostname, `.local`, IPv6, reverse proxy), including behind a proxy that rewrites `Host` with no forwarding trust configured: the browser's own `Sec-Fetch-Site: same-origin` now counts as same-origin. `PORTICO_ALLOWED_ORIGINS` is removed (code, demo script, API guide); nothing needs it. Public Private Network Access preflights get `Access-Control-Allow-Private-Network: true`.
- **Owner certificates on Android and Apple.** Every HTTP client in both apps now takes its TLS decision from one place (`PorticoServerTrust`, Kotlin and Objective‑C): system trust as before, otherwise the exact leaf certificate pinned for that host:port. A refused certificate's fingerprint is remembered; the shared JS fetch layer then has the server prove its Portico identity over exactly that certificate (the existing signed, nonce-bound identity proof, on a one-off client that trusts only that certificate and sends no credentials). First connection to an address, or the same server identity with a new certificate: pinned silently and the request retried. A different identity, or no provable identity: nothing is pinned; the sign-in screen shows a plain explanation and a "Trust this server" button. Connections still verify the expected server identity before any credential is sent, so a pin never replaces that check. Covered: API/control transport, event streams, ExoPlayer and AVPlayer (resource-loader trust delegate), downloads (incl. extras), subtitles, artwork/system media, audio renderer, both mpv relays (they inherit), React Native's own client (images and `fetch`; Android client factory, Apple category on `RCTHTTPRequestHandler`), the Android TV channel/top-shelf client, and the tvOS Top Shelf extension (reads the same pins from the app group).
- **Apple ATS.** `NSAllowsArbitraryLoads` is gone from the iOS, tvOS and Top Shelf Info.plists; replaced by `NSAllowsLocalNetworking` plus HTTP exceptions for the private home suffixes `home.arpa`, `internal`, `lan`. On Apple, the fetch layer refuses plain HTTP to a non-local address up front with "On Apple devices, plain HTTP works only on your home network. Use this server's https:// address." Local = loopback, RFC1918, link-local, 100.64/10 (Tailscale/overlays), IPv6 loopback/link-local/ULA, unqualified names, `.local`, `.home.arpa`, `.internal`, `.lan`.
- **Vega.** The SDK has no server-trust hook (only client certificates), so Vega cannot trust an owner's own certificate. A failed connection to an `https://` address on Vega now shows a plain line saying so and pointing to the http:// home address or the secure connection.

## 2 Decisions I made

- Cross-origin is decided by **token presence**, not an allow-list: `Authorization` header present, or a GET/HEAD under `/v1/media/` or `/v1/downloads/artifacts/` (grant in the path). The local server issues no cookies, so the only thing an origin rule still protects is what a token-less request can do from a page someone has open (it comes from a home address). A cookie on another origin's request is stripped before anything reads it.
- Preflights are answered for every origin (they carry no credential; the real request is checked again).
- `Sec-Fetch-Site: same-origin` is accepted as first-party everywhere except first-run setup, which keeps the existing strict Origin/Host comparison (its security test unchanged). Consequence: first-run setup through a Host-rewriting proxy with no trust configured still has to be done from the LAN address or with proxy trust set; sign-in and everything else work.
- The networking claim authorizer (owner bearer) keeps a first-party-only origin rule (own web app, built-in Portico pages, `Sec-Fetch-Site: same-origin`); a third-party page can't claim the server with Hosted.
- Trust-on-first-use is silent (no fingerprint prompt), matching Justin's "safe but simple"; it is no weaker than the plain HTTP he already allows, and Hosted-listed servers are still checked against their signed identity before credentials go out.
- A pinned certificate is accepted whatever its dates/names say (it's an exact pin, proven by the server identity). Otherwise an expired self-signed certificate would loop re-pinning.
- Pins are keyed by `host:port`, at most 64; refused fingerprints kept in memory only (32). Android stores pins in the encrypted record store (`network/certificates-v1`); Apple in the app group's defaults (`portico.certificates.v1`) so Top Shelf shares them.
- Apple local rule includes 100.64/10 (Tailscale) and `.home.arpa`/`.internal`/`.lan`; `.ts.net` names and public IPs over HTTP are refused on Apple.
- Proof failures that ask the person: `identity_mismatch`, `invalid_identity`, `invalid_response`. A server that's busy/404/unreachable is reported as unreachable instead (no question).

## 3 Audit bugs / decision gaps closed

- cp3-network item 1 (trust owner certificates) → native trust layer + JS settle flow → `test/certificate-trust.test.ts` (7 cases), `test/mpv-network-trust.test.ts` updated (relays use the app trust, no trust of their own).
- Item 2 (Apple ATS local-only) → Info.plists + `apple-plain-http.ts` → `test/apple-plain-http.test.ts` (local/remote hosts, plists have no arbitrary loads).
- Item 3 (browser access) → `browser_origin.go` → `TestAnyOriginMayCallWithAToken`, `TestAnotherOriginNeverSendsACookie`; existing tests adjusted to the decision: `TestCrossOriginClientsCanReadTheReportCadence` (any origin's preflight now answered), `TestSessionFamilyHTTPLogoutRequestSafety` "origin" case (foreign origin without a token still 403).

## 4 API changes

- CORS behaviour only (no route shapes): foreign origins + token → allowed; foreign origins without token → `403 origin_not_allowed` (unchanged code); `Access-Control-Allow-Private-Network` on PNA preflights. `PORTICO_ALLOWED_ORIGINS` removed. Guides `02-authentication.md` and `07-keys-tokens-and-scopes.md` updated. `go run ./cmd/apigen` produced no diff.
- New native module methods (`PorticoNetwork`, Android + Apple): `refusedCertificate(origin)`, `pinnedCertificate(origin)`, `pinCertificate(origin, fingerprint, serverId)`, `forgetCertificate(origin)`, `proofRequest(origin, fingerprint, body)`. Native transport rejects a refused certificate with code `certificate_untrusted`.

## 5 Schema changes

None.

## 6 Settings added

None. (`PORTICO_ALLOWED_ORIGINS` environment variable removed.)

## 7 Tests run

- `go build ./...`, `go vet ./internal/httpapi/`, gofmt → clean.
- `go test ./internal/httpapi/ -run 'TestAnyOrigin|TestAnotherOrigin|TestCrossOriginClients|TestSessionFamilyHTTPLogout…|TestSetupRangePlaybackStopAndOrigin|TestBrowserSetup…|TestPreservedProxy…|TestSetupOrigin…|TestScopedIPv6…|TestHostileHeadersAreNotTrusted|Claim'` → pass (before and after merging main).
- Native: `node --test` certificate-trust, apple-plain-http, mpv-network-trust, direct-connect, sign-in-cause, errors-present, saved-connection-store, sign-in-ended, direct-outcome, sign-in-steps → 36/36 + earlier 45/46 (the one failure was `session-intents` needing `react` before node_modules were linked; not run again).
- Native `npx tsc --noEmit`, `tsc -p test/tsconfig.json`, `npm run lint:strings`, `npm run lint:hooks` → clean. Vega `tsc --noEmit`, `lint:strings` → clean. Web `tsc --noEmit`, `lint:hooks` → clean.
- Kotlin, Objective‑C and Swift were **not compiled** (no Gradle/Xcode here).

## 8 Device test notes

Android (phone/TV/Fire TV):
1. Server with a self-signed certificate (`customCertificatePath/KeyPath`). Fresh install, type `https://<lan-ip>:32500`. Pass: signs in with no question; posters, playback (ExoPlayer and mpv), subtitles, downloads, event updates all work. Fail: "couldn't check this server's certificate" or blank images.
2. Replace the certificate with a new self-signed one (same server). Pass: next request continues silently. Then point the same address at a *different* Portico server with its own certificate: the sign-in screen shows the "certificate changed" message and "Trust this server"; nothing works until pressed.
3. A public-CA certificate (Hosted or Let's Encrypt) still works unchanged; a wrong-host public certificate goes through the proof and is pinned.
4. Compile check: `PorticoServerTrust.kt` (X509ExtendedTrustManager overrides, `OkHttpClientProvider.setOkHttpClientFactory`/`createClientBuilder`, SAM lambdas), `PorticoNetwork.kt` new methods, `PorticoTransport.kt` `const val` + `finish(code)`.

Apple (Mac checks — none of this was compiled):
1. Build iOS and tvOS: new `PorticoServerTrust.h/.m` are in both projects (IDs `A6E0…01–03`) and the bridging header; Swift calls `PorticoServerTrust.handle(_:completion:)`, `refused(at:)`, `pinned(at:)`, `pin(_:fingerprint:serverId:)`, `forget(_:)`, `prove(_:fingerprint:body:completion:)` (NS_SWIFT_NAME). Category on `RCTHTTPRequestHandler` imports `<React/RCTHTTPRequestHandler.h>`; confirm it compiles and does not collide with an existing RN method.
2. ATS: plain HTTP to 192.168.x.x, `[fd…]`, `name.local`, unqualified `nas`, `x.home.arpa` works; to a public hostname or public IP the app shows the Apple message before trying. Check the console for ATS refusals.
3. Self-signed HTTPS on a LAN address and on a **remote public hostname**: confirm custom trust (`NSURLSessionAuthChallengeUseCredential`) is honoured with ATS on (ATS may still enforce TLS 1.2/forward secrecy/key sizes; if a remote self-signed host is refused by ATS despite the pin, record it).
4. AVPlayer: playback over a pinned self-signed certificate relies on `AVAssetResourceLoaderDelegate resourceLoader:shouldWaitForResponseToAuthenticationChallenge:`; verify direct play and HLS both start. If AVFoundation doesn't ask, the native player fails while mpv works — record it.
5. Background downloads, subtitles, artwork, Now Playing artwork, audio renderer, mpv relay, Top Shelf posters over a pinned certificate.

Server/browser:
1. Open the web UI by IP, hostname, `.local`, `[IPv6]`, behind Caddy (Host preserved) and behind bare nginx `proxy_pass` (Host rewritten): sign-in and owner pages work with no env var.
2. From a page on another origin (e.g. `http://localhost:8080` tool), `fetch(server + '/v1/items', {headers: {Authorization: 'Bearer <api key>'}})` succeeds; the same without the header is 403; no cookie is ever accepted cross-origin.

## 9 Not finished

- Vega cannot trust an owner's own certificate (SDK limitation); it only explains.
- The Cast receiver page (`cast.getportico.tv`) runs in the Chromecast browser and cannot trust a self-signed server certificate; unchanged.
- The certificate question appears on the direct sign-in screens. If a certificate changes to an unprovable one mid-session or during Portico Account connection, the request fails with the plain message wherever errors are shown, but there is no "Trust this server" button outside sign-in; signing out and in again reaches it.
- No owner-facing list to view or forget pinned certificates (pins are replaced automatically when the same server proves itself).
- The query-parameter API key the cp3-api lane is adding is not recognised by my CORS token rule: `tokenCredentialPresented` in `server/internal/httpapi/browser_origin.go` should also return true when that query key is present (one line, for whoever merges cp3-api).
- Native code (Kotlin/ObjC/Swift) is uncompiled; see the Mac/Android checks.

## Commits

portico-server (`audit-implementation/cp3-network`): `def2b6d2` token-based cross-origin and same-origin detection; `b891c209` strings; `8a241ad1` merge of main.
portico-react-native (`audit-implementation/cp3-network`): `c70ff9e` mirror; `05d5371` certificate trust, Apple local HTTP, certificate question; `f365d9f` merge of main (lock re-mirrored).
