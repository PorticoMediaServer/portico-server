# Web UI and shared client-core — static audit

Audit date: 2026-09-25 (America/Halifax). Source: `apps/portico-server`, baseline `820dde1342aaf33dbddc69a03c859e225191de7f`. Paths below are relative to that repository unless stated otherwise. This report also applies to identical client-core copies in the React Native repository.

## Method and interpretation

Source inspection only: no application, build, test suite, browser, benchmark, network request, or exploit was run. Findings identify code-supported failure mechanisms; their frequency and measured cost remain unverified. P1 = high-priority security, availability, or core-workflow failure; P2 = substantive correctness/performance problem; P3 = lower-impact maintenance issue. A conditional security finding proves a missing defense in the client, not that an unauthenticated attacker can reach it on a normal server.

Second adversarial review is documented in `09-adversarial-native-reviewer.md`. All twelve mechanisms survived; five descriptions/triggers were narrowed and WC-007 was downgraded. Final distribution: **2 P1, 9 P2, 1 P3**.

## Findings

### WC-001 — P1 — Browse position cache crosses account/profile boundaries

**Confidence:** High. **Category:** Privacy, authorization-sensitive presentation.

**Evidence:** `web/src/app/list-position.ts:24–48`; `web/src/screens/library/Library.tsx:216,341–389,398–407`.

The module-global `windows` map retains real `ContentEntry` objects. Its key consists only of library ID, pivot, and query, without server/account/profile/session identity or a permission revision. `clearListWindows()` exists, but has no application caller. `EngineResults` renders a previously cached window before current-viewer revalidation, including on a fresh mount after sign-out and sign-in within the same document.

**Trigger:** Load the first browse page as an unrestricted viewer and change filter/sort in place, so the outgoing list-key cleanup saves the loaded entries while the collection remains mounted. Then switch to a restricted profile or another account in the same document and revisit the original library/pivot/query. The old viewer's entries are recalled without ownership checks. Ordinary full-page unmount is not a sufficient standalone seeding assumption because collection disposal can clear its pages before the save cleanup. Full-document navigation/reload destroys this module cache; cross-server exposure additionally requires same-document switching and colliding IDs. Same-server profile/account transitions do not require that collision.

The live replacement condition further requires `collection.itemAt(remembered.first) !== undefined`. If the new viewer has zero visible items, a smaller collection, or a refused request, the old window may remain instead of being replaced by the empty/error result. This aggravates the isolation defect.

**Impact:** Disclosure of cached title/metadata/history of what was browsed across a viewer boundary. The cache is bounded to five windows and 180 captured entries per window and is not disk-persistent. This is not evidence that the new viewer can retrieve protected artwork bytes or play restricted media: those requests still have server authorization.

**Recommended fix:** Include the full viewer/server/session scope in the cache key; clear old scope entries at the session owner; refuse stale-scope windows before rendering. Allow authoritative empty/error/revoked results to replace memory. Fence cleanup writes so an outgoing component cannot repopulate a cache cleared during a viewer transition.

**Verification to add:** A renders restricted entries on the loaded first page, changes filter/sort in place to seed the old key, then B revisits that key with an empty or smaller result and delayed/403 revalidation. Assert no A metadata is rendered to B, even for one frame. Repeat with same library IDs on different servers, permission changes, and logout/login in the same tab. Existing `web/test/list-position.test.ts` exercises recency/window behavior, not viewer isolation.

### WC-002 — P2 — Cancelling an audio preparation before its deck opens throws during cleanup

**Confidence:** High. **Category:** Playback lifecycle, resource cleanup.

**Evidence:** `web/src/bridge/DecodeAudioRender.ts:102–116,177–202`; `web/src/bridge/useWebPlaybackAdapter.ts:395–396,496,580`.

`prepared()` publishes a candidate with `deck: undefined as unknown as Deck`, then awaits `openDeck()`. `closePlaying()` unconditionally calls `p.deck.close()`. Clearing/replacing preparation, seeking, changing rate, stopping, switching route, or disposing while opening therefore dereferences `undefined`. The error handler also calls `closePlaying(pending)` when opening fails before assigning the deck, so an ordinary decode/fetch failure can itself throw out of the recovery path.

**Impact:** Interrupted playback transitions, rejected cleanup, and a reader that is not closed because the deck call throws first. It is a timing-dependent JavaScript error, not a native process crash.

**Recommended fix:** Represent opening and opened candidates as separate states or make the deck optional. Close the reader independently in a `finally`-safe idempotent cancellation path. Ensure a late `openDeck()` result is closed after cancellation.

**Verification to add:** Hold the candidate's first media read unresolved; then cancel, replace, seek, change route, and dispose. Also reject opening before a deck exists. Assert no exception/unhandled rejection and every reader/deck is closed once. Current audio-render tests await preparation settlement before cancelling and miss this interval.

### WC-003 — P2 — Direct decoded-audio seeks read the entire prefix of the file

**Confidence:** High. **Category:** Seek latency, bandwidth, CPU.

**Evidence:** `web/src/bridge/audio/track.ts:83–94,149–167`; `web/src/bridge/audio/mp4.ts:121–143`; `web/src/bridge/DecodeAudioRender.ts:129–133`.

`TrackStream.open()` calls `startAt()`, which obtains the ordinary packet iterator and advances it from frame zero until the requested position. MP4 already has sample offsets/durations, and `mp4Packets()` accepts a starting sample, but `openMp4().packets()` always passes zero. Iteration reads the actual packet bytes and copies packets before discarding them; it does not skip by the metadata table.

**Trigger:** Resume or seek far into a direct-decoded M4A/MP4 audio file with a cold cache. MP3/FLAC similarly depend on sequential packet walking. Large listening files are especially affected when eligible for this engine.

**Impact:** Seek cost grows with the bytes before the target rather than a small seek window. The 64 MiB byte LRU limits retained compressed bytes, but does not limit total traffic or eliminate repeat scans once early blocks are evicted. No absolute latency or throughput was measured.

**Recommended fix:** Expose a demuxer seek operation. Use MP4 cumulative durations/sample offsets, FLAC seek points, and suitable MP3 indexes, with bounded preroll. For formats without an index, make the fallback explicit and avoid re-downloading discarded prefixes on every seek.

**Verification to add:** Count range bytes for a cold seek near the end of a multi-hour file. Assert reads are bounded by index plus preroll, and verify exact trim/first output frame.

### WC-004 — P2 — SSE “frame limit” does not limit an accumulated event

**Confidence:** High. **Category:** Client resource exhaustion; conditional security exposure.

**Evidence:** `packages/client-core/src/sse.ts:8–38`.

`MAX_FRAME` is checked against the unparsed `buffer` only. Each parsed `data:` line is removed from `buffer` and appended to `data[]`, which has no count/byte limit. A stream of short `data:` lines without a blank event separator can grow beyond the intended event limit while every buffer check passes, until the connection or its caller aborts. When a separator arrives, `join()` creates another large allocation. The error cleanup releases the reader lock but does not itself cancel the stream.

**Trigger:** A malformed/hostile event response from a selected endpoint, or a server regression emitting an unterminated multiline event. This review does not establish a way for an unrelated remote attacker to control a trusted server's SSE body. Web playback currently constructs its main `EventsClient` without a streaming opener, but `web/src/app/together.tsx:20–24` and `web/src/screens/server/Diagnostics.tsx:66–110` are real streaming consumers. GroupSession's silence watchdog bounds elapsed time for an unterminated event, not the bytes received before it expires; Diagnostics has no comparable per-read watchdog in the inspected path.

**Recommended fix:** Track cumulative UTF-8/event bytes and line count until dispatch, bound the current line separately, reset all event accounting on dispatch, and cancel the reader on terminal parse failure. Preserve a trailing CR across chunks so split CRLF does not become two line endings. Process valid coalesced frames before applying a per-frame cap.

**Verification to add:** Many small `data:` chunks exceeding the event cap; oversized single lines; many valid events in one large network chunk; CRLF split at every byte boundary; cancellation on rejection.

### WC-005 — P2 — Ordinary download-list refresh permanently stops container request followers

**Confidence:** High. **Category:** Shared web/native download state.

**Evidence:** `packages/client-core/src/downloads-service.ts:38–45,140–183`; `web/src/screens/downloads/Downloads.tsx:43`.

`followRequest()` captures `this.generation` and exits when it changes. Every ordinary `refresh()` increments that generation. Opening the Downloads screen calls `start()`, which invokes `refresh()` and one `refreshRequests()` call but never restarts request followers. Completing one request also calls `refresh()`, terminating every other outstanding follower. The ordinary preparation poll does not update `tracked` request progress.

**Trigger:** Start a whole-show/album/etc. download outside the Downloads screen, then open Downloads; or start two requests and let one finish. After the generation change, active request cards can stay at their last state indefinitely without another explicit refresh.

**Impact:** Stale or apparently stuck whole-container progress despite server work continuing. Native collection/enrollment defects are separately documented in NC findings; this defect is in the shared observation lifecycle.

**Recommended fix:** Separate service/session disposal epoch from list-read generation. Use one bounded request follower scheduler that survives list refresh and follows all active tracked requests; cap and retire terminal records. Resume following after navigation as required.

**Verification to add:** Active request survives `refresh()` and `start()`; completion of A does not stop B; delayed list replies cannot overwrite newer state; disposal cancels all request work.

### WC-006 — P2 — Non-page-aligned remembered windows shift items to the wrong indices

**Confidence:** High. **Category:** Browse correctness.

**Evidence:** `web/src/ui/static-collection.ts:15–26`; `web/src/screens/library/Library.tsx:398–407`; `packages/client-core/src/collections/windowed.ts` (`itemAt`); `web/test/list-position.test.ts:57–72`.

The static adapter slices cached items from a nonzero `base`, but returns a dense page that `WindowedCollection` always interprets as starting at the requested page offset. A saved first-visible index need not align with the 60-item page boundary. For example, base 67 and page start 60 return entries 67–119 as page slots 60–112, so index 67 renders entry 74; missing slots before base were not represented.

**Impact:** Wrong titles/selection positions and visual jumps during Back restoration. The existing test uses base 180, which is page aligned and cannot expose the bug.

**Recommended fix:** Save complete page-aligned windows, or support explicit sparse/page offsets in the adapter without treating unavailable leading entries as real items. Keep total and absolute indices consistent.

**Verification to add:** Bases 1, 59, 61, 67, and 179 with exact `itemAt()`/placeholder assertions; verify Back at grid row positions with column counts not dividing 60.

### WC-007 — P3 — Replacing a service skips disposal and leaves bounded obsolete work

**Confidence:** High. **Category:** Request/resource lifecycle.

**Evidence:** `web/src/app/content.ts:14–28,37–40`; `packages/client-core/src/library-content.ts:267,279–301`.

The hook's cleanup queues disposal in a timer, but the next effect unconditionally clears that timer. This is appropriate only for replay of the same instance. When dependencies create a different instance, the new effect cancels the old instance's disposal as well. Its controller is not aborted and its disposed/generation fence is not set.

**Trigger:** Dependency replacement while a `useService` consumer remains mounted. In addition to API/viewer changes, `web/src/screens/server/Libraries.tsx:258–265` replaces its service on libraryId changes. In-flight obsolete work continues until its own timeout/completion.

**Impact:** Obsolete requests and retained state survive the intended ownership boundary. The subscription is removed, so this finding alone does not prove old data is delivered into the new hook. Examined services have finite deadlines (commonly 15 seconds; playback settings seven seconds), then can become collectible. No continuing polling resource, old-viewer publication, or permanent leak was established, which is why the second pass downgraded this to P3.

**Recommended fix:** Associate deferred disposal with the exact instance; cancel it only when that same instance is reused. Dispose replaced instances unconditionally. The current app entry point does not enable StrictMode, so normal dependency transitions must take precedence over a hypothetical replay.

**Verification to add:** Replace service A with B before the deferred timer fires; assert A is disposed/aborted and B remains live. Separately verify same-instance replay if supported.

### WC-008 — P2 — Browser audio range reads buffer an unbounded response before applying the block limit

**Confidence:** High. **Category:** Memory/bandwidth bounds; conditional security exposure.

**Evidence:** `web/src/bridge/audio/bytes.ts:76–105`.

Each nominal 256 KiB block calls `response.arrayBuffer()` with no streamed maximum, Content-Length rejection, Content-Range validation, or response-size check. HTTP 200 is explicitly accepted and the entire body is buffered before slicing out one block. HTTP 206 can also carry more than the requested range. The cache budget applies only after allocation; oversized entries may be immediately evicted after memory has already been spent.

**Trigger:** A media endpoint/proxy ignoring Range, returning an oversized response, or supplying invalid range metadata. A large `read()` also creates one outstanding fetch promise per intersecting block; a near-64 MiB MP4 index can create roughly 256 promises (possibly one extra if unaligned). This is not a measured count of simultaneous network sockets/streams; the browser/protocol schedules those.

**Impact:** Whole-file allocations and repeated whole-file downloads for separate blocks; a misbehaving endpoint can exhaust a tab's resources. Normal compliant Go Range serving is counterevidence against this occurring on every playback.

**Recommended fix:** Stream each range response with an expected-byte ceiling, require matching Content-Range, reject unexpected 200 except a strictly bounded complete small file, and use a concurrency limiter for block fills. Abort body consumption on any mismatch.

**Verification to add:** Chunked 200/206 bodies larger than BLOCK, lying lengths/ranges, aborted reads, and a large moov read; assert byte/concurrency ceilings before allocation, not just final cache size.

### WC-009 — P2 — MP4 table counts allocate before validating the containing box

**Confidence:** High for parser defect; conditional for exploitability. **Category:** Malformed media/resource exhaustion.

**Evidence:** `web/src/bridge/audio/mp4.ts:148–177`, plus the 64 MiB moov cap at lines 35–37.

`chunks` is taken from the `stco/co64` header and directly sizes a `Float64Array`; it has no bound against the enclosing box's byte length. `count` has a 50-million ceiling but still permits about 800 MB across sizes/offsets/durations before chunk offsets. `runs` and table offsets similarly lack structural cardinality checks before loops. A small malformed box can claim a huge table; a moov byte cap does not prevent expanded allocations.

**Impact:** Allocation failure, memory pressure, or long main-thread loops when this decoder receives malformed source bytes. The server's media probing and choice of direct mode may reject some such files first; no server-to-client crafted-file chain was executed or proven.

**Recommended fix:** Validate each table's minimum header, record count against remaining bytes, monotonic sample/chunk runs, positive timescale, and exact sample coverage before allocating. Apply an aggregate memory budget and avoid materializing multiple full sample arrays where an indexed representation suffices.

**Verification to add:** Tiny boxes with huge counts, truncated tables, zero/descending chunk runs, large valid audiobooks, and allocation-budget assertions.

### WC-010 — P2 — Active guide adapter still fetches all channels per time window

**Confidence:** High; this is acknowledged temporary architecture. **Category:** Scale/performance limitation requiring product decision.

**Evidence:** `packages/client-core/src/guide/legacy-source.ts:1–14,48–64,130–174,188–210`; `web/src/app/channels.tsx:35–43,72–76`.

The adapter walks up to 200 pages of 30 channels per kind for an uncached time window, keeps all resulting programmes, then slices channels or filters programme results to the visible IDs. `channels()` loads a three-hour window for the whole lineup. Each uncached programme time block walks the applicable lineup again. The source-navigation probe also walks all live pages. Windowed DOM/GuideWindowStore does not make these upstream reads O(visible).

**Impact:** Network, parsing, and retained data scale with the full lineup; scrolling horizontally can trigger repeated full-guide walks. Eight cached windows bound window count, not programme bytes. At the 200-page cap the walk returns a partial set without a distinct truncation signal. Consumer cancellation in `legacyGuideSource` is checked after the shared walk and does not stop its requests.

**Nuance:** The module explicitly documents a <=100-channel stopgap. Same-window promises are shared, identical GETs are deduplicated for ten seconds (`web/src/app/guide-reads.ts`), and the source probe is reused for sixty seconds; source/group/favorites filters can reduce actual work. These controls prevent a claim that every row/revisit causes a fresh whole-lineup request, but do not provide channel-scoped upstream reads. The 200-page cap applies separately to live and library kinds. This is not a newly discovered violation of a completed large-lineup implementation. The live web path does not enforce that size nor provide a warning callback, so larger sources still enter it.

**Recommended fix:** Implement server channel-offset pages and programme reads scoped to requested channel IDs/time windows, with revision fences and abortable shared request ownership. Until then make the supported limit explicit and avoid silent truncation. Preserve the deliberate partial-kind failure behavior only if product-approved.

**Verification to add:** Compare request count/bytes for 100, 1,000, and 6,001 channels with the same viewport; cancellation and repeated cursors; assert no silent disappearance beyond a cap.

### WC-011 — P2 — General JSON transports remain unbounded before validators run

**Confidence:** High. **Category:** Defense consistency, client memory.

**Evidence:** `packages/client-core/src/index.ts:366–407`; `web/src/bridge/account.ts:103–121`; `web/src/bridge/account-api.ts:16–22`; `web/src/bridge/hosted-account-api.ts:14`.

The ordinary `HttpLocalApi.request()` and `requestRaw()` materialize whole response bodies using `json()` or `text()`; ordinary requests also have no default deadline unless the caller supplies one. Browser account helpers similarly parse bodies without streamed byte caps. However, `BrowserAccountApi` (`account.ts:95–109`) and `accountRequest` (`hosted-account-api.ts:14`) already enforce 15-second deadlines; `accountApi(scope)` (`account-api.ts:13–22`) forwards only the caller signal. Missing byte limits and missing default deadlines are separate claims. Many feature parsers validate array/string limits only after allocation. Specialized `requestConsole`, `requestLyrics`, `requestBounded`, and route-proof readers correctly implement bounded paths, but most traffic does not automatically use those readers.

**Impact:** A malformed selected server/account endpoint or proxy can consume excessive client memory before the documented bounded-parser guarantees take effect. This is a conditional defense gap, not evidence that ordinary Portico endpoints routinely emit huge bodies. Some callers independently enforce timeouts and native routes impose their own transport bounds; inspect each adapter before choosing global limits.

**Recommended fix:** Centralize streamed JSON limits with endpoint-appropriate budgets, small error-body limits, and deadlines. Preserve explicitly long-lived long-poll/SSE and media operations. Account helpers should use the same fenced, bounded transport instead of separate ad hoc fetch paths.

**Verification to add:** Oversized/chunked success and error bodies, slow/trickled responses, Content-Length lies, caller abort, and a legitimate maximum-size response per endpoint class.

### WC-012 — P1 — Advertised LAN HTTP web entrypoint depends on secure-context-only browser APIs

**Confidence:** High from source/platform requirements; browser behavior not exercised. **Category:** Installation/sign-in availability.

**Evidence:** `server/cmd/server/main.go:142,192–197`; `packages/client-core/src/route-identity.ts:26–29,46–50,89–102`; `web/src/app/session.tsx` (`signInDirect`); `web/src/bridge/server-connections.ts:17,22,30–31`; `web/src/bridge/profile-trust.ts:6–8`.

The server defaults to `0.0.0.0:32500` and tells the operator to open `http://<LAN-address>:<port>` from another device. `routeOrigin()` explicitly accepts private-network HTTP. However, direct sign-in verifies a server proof using `webRouteCrypto`, which unconditionally calls `crypto.subtle.digest/importKey/verify`. Credential persistence also requires Web Locks and `crypto.subtle`; installation identity uses `crypto.randomUUID()` and falls back to an empty ID on failure. These facilities require a secure browser context. Ordinary HTTP at a private LAN IP is not the localhost secure-context exception.

**Trigger:** Follow the server's printed LAN HTTP URL from another device with ordinary browser security settings, then sign in/connect. Proof verification or credential saving fails because the required browser capability is unavailable. HTTPS and loopback hosting are counterexamples; this is not a claim that those flows fail.

**Impact:** The documented/default cross-device web onboarding path cannot satisfy its authentication/storage prerequisites. Do not fix by weakening proof verification or storing plaintext credentials.

**Recommended fix:** Make secure hosting a supported first-run requirement with an achievable LAN bootstrap path, or design an explicitly reviewed same-origin session/proof mechanism compatible with the advertised entrypoint. Detect capability/context failure before credential entry and provide actionable setup guidance. Align server startup URLs and website instructions with the chosen supported model.

**Verification to add:** Real browser matrix for localhost HTTP, private IPv4/IPv6 HTTP, `.local` HTTP, and trusted HTTPS on a second device; verify setup, direct sign-in, proof, protected persistence, refresh and restoration. This specific acceptance test remains unrun per audit scope.

## Additional concerns for review, not counted as established findings

- **Audio underrun recovery:** `audio/engine.ts:137–145` schedules every recovered buffer at its original start time, and `DecodeAudioRender.tick()` reports starvation without pausing/rebasing the AudioContext clock. After a stall longer than the decoded buffer, several recovered nodes may be scheduled in the past and play together. Review desired no-skip versus catch-up semantics and test an actual stalled audio clock before assigning final severity. Current arithmetic strongly warrants a regression test; no audible behavior was observed here.
- **Cast listener retention:** `bridge/cast.ts:72–83` adds an anonymous SDK session-state listener that `dispose()` cannot remove. Disposed controllers refuse adoption, limiting behavioral impact, but listeners retain closures/API instances across repeated viewer changes. Store/remove the callback and guard late `prepare()` completion.
- **Capability cache lifetime:** `app/browse.ts:20–38` scopes keys by server/account/profile/session family plus library/pivot and has no size/TTL eviction. Access-token rotation, query and sort do not add keys; failures are removed and reload clears the map. No material memory cost or cross-viewer reuse is established; assess lifetime/freshness before prioritizing it.
- **Decoded audio memory/CPU:** A fixed 30-second decode-ahead per deck plus 10-second preparation is large for high-rate/multichannel PCM; The JS FLAC fallback and explicit resampling/time-stretch paths perform synchronous JS work; WebCodecs uses browser codec implementations, so this is not a claim that all decoding runs in main-thread JavaScript. Actual output rate may fall back to the device AudioContext rate. Bounds exist in time, but measured memory/jank at the declared capabilities are unverified. A worker and byte-aware budget deserve evaluation, not an unmeasured claim that current devices fail.
- **Browser encrypted storage threat model:** AES keys and ciphertext share the same origin's IndexedDB. Non-extractable keys improve storage handling but do not make tokens inaccessible to arbitrary same-origin script, which can invoke decryption. No XSS injection path was established in this review; do not describe this storage as an XSS-proof boundary.

## Coverage and counterevidence

| Area | Static review performed | Limits |
|---|---|---|
| Entry, routing, gate and session ownership | App entry/router, sign-in/restore/profile transition controller, account/session bridges, server route origins/proof, secure stores | No provider/browser/device sign-in executed; full race-state space not proven |
| Shared auth/credentials | Credential service structure, refresh owner, disposal, route transport and request handling | No cryptographic implementation certification or token replay exercise |
| Browser playback | HLS setup/resource keys/route recovery, decode renderer, packet/byte readers, MP4/FLAC/PCM/WebCodecs helpers, audio deck scheduling, player/provider event ownership | No media corpus, decoded-output comparison, GPU/audio-device testing or measured timing |
| Lists/content | Windowed collection, grid/static adapters, Browse/Home/content hooks, cache/position lifecycle, selection, source/guide adapters | Screen CSS and every visual/accessibility state not rendered; not every parser permutation enumerated |
| Downloads | Shared preparation/container follower lifecycle and web callers | No server job run, browser download or offline device acceptance |
| Artwork | Shared scheduler policy, browser request bounds, object-URL pin/eviction lifecycle | Encoded byte bounds do not constitute decoded GPU-memory measurements |
| Administration/settings | Route/owner gate, feature service structure, console/lyrics/certificate/support bounded transport paths | Screens are not authorization boundaries; backend authorization handled in server report; not every form combination verified |
| Shared data and contracts | Response-boundary pattern review across client-core, serializers, presentation helpers and test references | Generated contract output, all message catalog entries, and every helper were not independently proven line by line |
| Tests/tooling | Relevant source tests read to identify coverage gaps; manifest/build boundary inspection | No test, lint, typecheck or build executed |

Useful protections observed include redirect refusal and origin validation on selected-server transports; signed route proof checks; viewer/session-generation fences in many feature services; bounded console, artwork, lyrics and diagnostic readers; bounded page residency and artwork queues; and browser rendering through React text rather than raw server HTML. These reduce scope of several concerns above. They are not a security sign-off.
