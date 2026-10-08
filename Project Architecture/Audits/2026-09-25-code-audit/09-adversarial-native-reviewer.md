# Adversarial second review — web and shared client-core

Date: 2026-09-25. Reviewer: the Apple/React Native audit reviewer, reassigned to independently challenge `03-web-and-client-core.md`. Source paths below are relative to `/Users/justin/Documents/Codex Projects/Portico/apps/portico-server`. This document changes no application source and does not replace the first report.

## Method and outcome

I reread the implementation, callers, disposal paths, cache keys, transport adapters, and relevant limits for every WC finding. The goal was to disprove reachability or reduce overstated consequences, rather than merely verify the cited line. Static reading/search only: no application, test, build, browser, network request, exploit, media playback, or benchmark was run. Browser API semantics are reasoned from their established contracts, not newly tested or externally researched. The coordinator reports that the source inventory is unchanged despite the internal documentation repository moving to `8f412e4`.

All twelve underlying mechanisms survive review. Six findings are upheld, five need narrower wording or triggers, and one is downgraded to P3. Recommended established-finding distribution after this review: **2 P1, 9 P2, 1 P3**. This does not turn the five additional concerns into five established product defects. Two of those remain unresolved device/performance questions; the others retain the limited classifications below.

| ID | Verdict | Recommended priority | Main qualification |
|---|---|---|---|
| WC-001 | Narrow | P1 | Seed the cache through an in-place query change; ordinary full unmount is not the only or safest assumed seeding path. Full-document navigation destroys this memory. |
| WC-002 | Uphold | P2 | Actual pending candidate can have no deck; synchronous cleanup throws before reader closure. |
| WC-003 | Uphold | P2 | Cold direct-decoded packet traversal; no claim that every seek decodes every earlier sample or every playback mode is affected. |
| WC-004 | Narrow | P2 | Group watchdog bounds elapsed time, not byte volume; real browser streaming callers exist. |
| WC-005 | Uphold | P2 | Scope-stable DownloadsProvider makes the navigation trigger concrete; stale progress is not failed server work. |
| WC-006 | Uphold | P2 | Dense page positions conflict with arbitrary cached base; alignment-only tests do not falsify it. |
| WC-007 | Downgrade | P3 | Disposal is skipped, but examined work is bounded and unsubscribed; no lasting leak or old-viewer publication proved. |
| WC-008 | Narrow | P2 | Outstanding block promises are not a measured number of simultaneous network connections. |
| WC-009 | Uphold | P2 conditional | Expansion precedes box cardinality validation; crafted-file admission to this engine remains unproven. |
| WC-010 | Narrow | P2 scale limitation | Deliberate small-lineup adapter, per-window reuse and ten-second request deduplication; 200 pages applies per kind. |
| WC-011 | Narrow | P2 conditional | Byte caps are missing in examined ordinary readers; two account implementations already have 15-second deadlines. |
| WC-012 | Uphold | P1 | Insecure private-LAN document origin; not every HTTP destination from a secure page. |

## Finding-by-finding challenge

### WC-001 — Narrow; retain P1; high confidence in isolation defect

**Independent evidence:** `web/src/app/list-position.ts:21–48`; `web/src/screens/library/Library.tsx:216,341–389,398–408`; `web/src/app/browse.ts:58–68`; `packages/client-core/src/collections/windowed.ts:312–319`; `web/src/app/session.tsx:943–970,1008–1012`; `web/src/shell/ServerSwitcher.tsx:49`; `web/src/screens/auth/HostedChooser.tsx:69`.

**Falsification attempted:** Does collection disposal make it impossible to populate the cache? Does sign-out reload the document? Is the remembered key already viewer scoped? The key has only library/pivot/query; no caller clears `clearListWindows`. The sign-out/disconnect code clears viewer/session state and credentials in the existing document. A full Library unmount also disposes its live collection and clears its page map. Depending on effect cleanup ordering, that can prevent the child cleanup from saving populated pages. Therefore the original trigger “leave the page so its visible titles are saved” should not be its sole reachability argument.

**Stronger static trigger:** On the first loaded browse page (index zero), change a filter or sort while the same Library/EngineResults remains mounted. `listKey` changes, so the old child effect cleanup saves entries under the old key. `useBrowseCollection` keeps the collection for that query change and invalidates it in its subsequent effect; its disposal dependency did not change. Choosing the first page also avoids the render-time range reset causing a hole at an unloaded index. Now switch profile or sign out/in in the same document and revisit the original key. Cached real entries are recalled without viewer identity. A later empty save does not erase the existing entry. The remembered result is not replaced when live total is zero or the remembered index is unavailable.

**Corrected wording:** “After an old viewer's browse window has been cached—for example by an in-place filter/sort change on a loaded first page—a later viewer in the same document can receive that metadata for the same library/pivot/query. Empty/refused live results may leave it displayed.” Cross-server reuse needs the same JavaScript document and colliding IDs; bundled switches using `location.assign` discard the map. Same-server profile/account reuse needs neither cross-server collision nor persistence across reload.

**Counterevidence/impact limit:** Five-window LRU and at most 180 captured entries bound volume; no disk persistence. Server-side media/art authorization is still required. Retain P1 for cross-viewer restricted metadata, not an unrestricted media-read bypass.

**Fix pitfalls/regression:** Scope both read and cleanup write by server/account/profile/family (or equivalent authorization epoch); clearing alone can race an outgoing cleanup. Explicitly test the in-place query-change seed followed by viewer switch, and distinguish full reload from same-document navigation. Empty/403 replacement must not depend on finding an item at the previous viewer's index.

### WC-002 — Uphold P2; high confidence

**Independent evidence:** `web/src/bridge/DecodeAudioRender.ts:102–116,129–133,177–202,274–285`; `web/src/bridge/useWebPlaybackAdapter.ts:380–398,493–498,577–585`.

**Falsification attempted:** Can preparation publish only after opening, or do all cleanup callers guard readiness? No. `candidate` is assigned with an explicitly cast undefined deck before awaiting `openDeck`. `closePlaying` accesses `p.deck.close()` before `reader.close()`. Clearing/replacing preparation and reset/dispose can run in this interval; adapter route-change and teardown invoke disposal. Opening failure reaches the same unsafe close in the asynchronous catch path. The adapter's constructor try/catch does not enclose every later `render.apply`/dispose.

**Corrected scope:** A timing-dependent JavaScript lifecycle exception, including an error inside opening-error recovery, when the decoded renderer has a pending candidate. It does not establish a native crash or affect all HTML/HLS playback. Keep the original priority and central wording.

**Fix pitfalls/regression:** Optional-deck guarding alone is insufficient if late open results survive cancellation. Close reader independently, make cleanup idempotent, and close a late deck/source. Hold opening unresolved and exercise replace, stop, seek, route retirement, dispose, and rejected open. No such execution occurred in this review.

### WC-003 — Uphold P2; high confidence

**Independent evidence:** `web/src/bridge/audio/track.ts:83–94,149–167`; `web/src/bridge/audio/mp4.ts:121–143`; `web/src/bridge/audio/bytes.ts:1–39,101–128`; `web/src/bridge/DecodeAudioRender.ts:129–133`.

**Falsification attempted:** Is `startAt` walking metadata only, does MP4 select an indexed start sample, or does the cache make it constant-cost? The iterator returned by `openMp4` passes start sample zero; advancing calls `reader.read` for contiguous packet data and slices each packet. `startAt` discards those packets until target/preroll. It need not decode the entire prefix, but it reads/copies it. A warm compressed-byte cache can help a short seek; its 64 MiB bound cannot retain arbitrary long prefixes and may be churned by repeated traversal. Converted sources and browser-element seeking are separate paths.

**Corrected scope:** Retain cold-cache direct-decoded M4A as the strong example. Do not say “every seek downloads the whole file” or “all earlier PCM is decoded.” Cost scales with the prefix to the target, subject to cache and format behavior. No measured latency threshold was established.

**Fix pitfalls/regression:** A sample-index jump must also preserve edit-list trim, timestamp origin, codec preroll, and accurate output position. Index lookup alone must not create a gap or feed a decoder without required predecessor packets. Count cold range bytes and validate first audible frame; use long eligible files, not only short fixtures.

### WC-004 — Narrow; retain conditional P2; high confidence

**Independent evidence:** `packages/client-core/src/sse.ts:8–38`; `web/src/app/together.tsx:20–24`; `packages/client-core/src/group-session.ts:37,140–162`; `packages/client-core/src/remote-playback.ts:195–224`; `web/src/screens/server/Diagnostics.tsx:66–110`.

**Falsification attempted:** Do current callers prevent use, impose a byte cap elsewhere, or abort all parse failures? TogetherProvider supplies a real `api.routeFetch` stream; Diagnostics also calls this helper for live logs. The original observation that the main playback EventsClient lacks a streaming opener does not make this parser dead in web. GroupSession installs a 40-second silence watchdog, reset only by dispatched events, so a single unterminated event is time-bounded there. That still permits arbitrary byte volume within the watchdog interval; Diagnostics has no analogous per-read silence watchdog in this code. Neither examined group finally nor diagnostics retry catch unconditionally cancels the response when this helper throws.

**Corrected wording:** “The helper has no cumulative event byte/line bound. Callers may bound connection lifetime or silence, but short parsed data lines can grow `data[]` beyond MAX_FRAME before an event boundary or abort.” It is not necessarily literally infinite lifetime in every consumer. The selected endpoint must supply malformed/hostile bytes; no unrelated-attacker content injection chain is shown.

**Additional corroboration:** The limit checks the whole received chunk before extracting completed events, so a large coalesced chunk of valid small events can be rejected. A trailing CR is consumed immediately; a following chunk's LF becomes a blank line and can dispatch early. Counting UTF-16 string length is also different from a UTF-8 wire-byte limit.

**Fix pitfalls/regression:** Preserve streaming decoder/CRLF state; separately cap unparsed line and accumulated event bytes, and cancel on terminal failure without cancelling a deliberately shared owner's unrelated work. Keep comments/heartbeats and valid coalesced frames legal. Test fast multiline overflow, watchdog abort, split CRLF, parser error followed by retry, and multiple valid frames in one large chunk.

### WC-005 — Uphold P2; high confidence

**Independent evidence:** `packages/client-core/src/downloads-service.ts:38–45,140–183`; `web/src/app/downloads.tsx:13–24,66–78`; `web/src/screens/downloads/Downloads.tsx:43`.

**Falsification attempted:** Is the service destroyed when opening Downloads, does the ordinary poll refresh request cards, or does `refreshRequests` restart followers? DownloadsProvider owns one service above the requesting dialog/screens. The dialog starts container requests, and its View Downloads navigation reaches a screen whose `start()` synchronously increments the same generation through `refresh()`. `refreshRequests()` fetches a snapshot once; it does not invoke `followRequest`. The preparation poll updates a different collection. Finishing A also calls refresh while B's follower retains the old generation.

**Corrected scope:** Keep the established stale-observation finding. “Permanent” means that follower exits and is not automatically restarted by these paths; a manual one-off refresh may update the display. It is not proof that jobs stop, files corrupt, or native transfers fail.

**Fix pitfalls/regression:** Separate list request ordering from follower lifetime/session epoch. Avoid creating duplicate loops on every start/refresh, preserve disposal cancellation, bound tracked records, and cover A-completes/B-continues plus dialog-to-Downloads navigation.

### WC-006 — Uphold P2; high confidence

**Independent evidence:** `web/src/ui/static-collection.ts:15–26`; `packages/client-core/src/collections/windowed.ts:195,256`; `web/src/screens/library/Library.tsx:373–375,398–408`; `web/test/list-position.test.ts:57–72`.

**Falsification attempted:** Does the page store account for a leading offset or enforce aligned first-visible positions? `itemAt` indexes each stored page by absolute index modulo page size; static fetch returns a dense slice without a leading-gap representation. Arbitrary first-visible indices are accepted and saved. With base 67, the page requested at 60 begins with cached item 67 in slot 60, so lookup 67 returns item 74. Base 180 in the existing test is aligned and does not exercise the defect.

**Corrected scope:** Incorrect temporary restoration positions/content until live replacement, not corruption of the server collection. Independent of WC-001's privacy issue once a remembered window exists.

**Fix pitfalls/regression:** Do not simply clamp base down without also capturing the missing earlier entries. Align capture or model sparse offsets. Assert identity at each absolute index around page boundaries, including holes, non-divisor grid column counts, and the transition to live results.

### WC-007 — Downgrade P2 to P3; high confidence in defect, bounded demonstrated impact

**Independent evidence:** `web/src/app/content.ts:14–28,37–40`; `packages/client-core/src/library-content.ts:267,279–301`; `web/src/screens/server/Libraries.tsx:258–265`; `packages/client-core/src/library-inventory.ts:46–49`; `packages/client-core/src/metadata-repair.ts:80–85`; `packages/client-core/src/show-workspace.ts:59,67,91`; `packages/client-core/src/detail.ts:180–182,246,279–286`; `packages/client-core/src/playback-settings.ts:28–36`.

**Falsification attempted:** Does React replacement perform cleanup so late that the timer fires before the new setup? The hook stores one pending timer in a ref; old cleanup schedules it, replacement setup clears it synchronously. The timer is not associated with an instance. This is an ordinary effect-dependency transition, not a StrictMode-only possibility. The generic hook is also used outside content: LibraryDetail includes `libraryId` in dependencies, giving an example of instance replacement without relying solely on viewer-transition mounting assumptions.

**Counterevidence:** Cleanup unsubscribes the old listener immediately. The specific content request path races a deadline and abort signal; examined inventory/metadata/detail/show services also have finite request deadlines (commonly 15 seconds). Playback settings has a seven-second default. Once settled, an unsubscribed instance can become collectible; missing `dispose()` is not proof that JavaScript retains it forever. The viewer scope hook avoids replacement on mere access-token rotation. This review did not prove old results reaching the new subscriber, an endlessly polling surviving service, or a continuing unauthorized mutation caused by this defect.

**Corrected wording:** “Replacing a `useService` instance cancels the previous instance's deferred disposal, allowing bounded obsolete work to finish rather than abort promptly.” That supports a lifecycle/efficiency P3 on current evidence. Re-escalate if a concrete long-lived resource or observable stale side effect is established.

**Fix pitfalls/regression:** Retain replay safety only for the same instance; do not clear another instance's timer. Test A-to-B replacement, true unmount, and same-instance replay separately. Do not advertise the repair as a privacy fix without a proven data-delivery chain.

### WC-008 — Narrow; retain conditional P2; high confidence

**Independent evidence:** `web/src/bridge/audio/bytes.ts:76–105,121–128`; `web/src/bridge/audio/mp4.ts:29–38`.

**Falsification attempted:** Does file size, prepared byte allowance, cache eviction, or browser scheduling bound consumed response bytes? File size clamps requested ranges, and prepared allowance accounts requested bytes; neither caps actual body bytes. `arrayBuffer()` materializes the body before 200 slicing or cache admission. The 206 path accepts body length without range validation. LRU eviction happens afterward. Sequential small-file `prefetchAll` is a useful guard, but `read` still uses Promise.all for all intersecting blocks.

**Corrected wording:** “A near-64 MiB moov read may create roughly 256 outstanding block fetch promises (possibly an extra block when unaligned). Actual concurrent network connections/streams depend on browser/protocol scheduling and were not measured.” Each promise is not necessarily an active socket. A compliant selected server delivering correct 206 responses limits each ordinary block as intended; endpoint/proxy noncompliance is required for the unbounded-body branch.

**Fix pitfalls/regression:** Do not trust Content-Length alone or reject a legitimate short final range. Validate 206 range start/end/total against the request and actual streamed bytes; bounded 200 support must be explicit. A semaphore must release on cancellation/error and avoid deadlocking shared fills. Test lying headers, streamed overflow, short EOF, ignored Range, and outstanding-fetch ceilings.

### WC-009 — Uphold conditional P2; high confidence in parser defect

**Independent evidence:** `web/src/bridge/audio/mp4.ts:29–38,72–121,148–177`; `web/src/bridge/audio/types.ts:42`.

**Falsification attempted:** Do the moov cap, box traversal, TypedArray bounds behavior, or 50-million ceiling prevent expansion? They do not. `new Uint32Array(count)` occurs before per-entry coverage validation. `new Float64Array(chunks)` uses an unbounded header count before reading entries. At the allowed 50-million count, sizes + offsets + durations account for approximately 800 million bytes if those allocations succeed; engines may reject sooner, which still interrupts playback. Box discovery verifies outer layout but not these record-count cardinalities. The integer byte helper does not throw a DataView-style bounds exception before the allocation.

**Counterevidence/wording:** A malformed input must reach this browser demuxer with a usable audio configuration; server probing and plan selection may reject candidates first. This is not a demonstrated unauthenticated web exploit or proof that a browser will allocate a particular multi-gigabyte array. Keep “allocation failure, memory pressure, or excessive loops” and the conditional classification.

**Fix pitfalls/regression:** Fixed-size `stsz` legitimately need not contain one explicit size per sample, so validate table rules, not a blanket count-times-record-size formula. Check count arithmetic, coverage, chunk/sample relationships, timescale, and an aggregate budget before allocation. Fuzz tiny valid outer structures with impossible inner counts; no fuzzing was run here.

### WC-010 — Narrow; retain P2 as scale/product limitation; high confidence

**Independent evidence:** `packages/client-core/src/guide/legacy-source.ts:1–14,48–64,130–174,188–210`; `web/src/app/channels.tsx:35–43,51–76`; `web/src/app/guide-reads.ts:1–37`.

**Falsification attempted:** Do per-window promise sharing, UI virtualization, source-probe reuse, or transport deduplication make reads proportional to viewport? They reduce duplicate work but do not change the underlying full cursor walk. Calls for different visible channel subsets in the same time block share a walk. The web transport deduplicates identical GET paths per API instance for ten seconds; the source probe is reused for sixty seconds. Therefore do not describe every row request or every immediate revisit as another full download. Distinct time windows still require full walks.

**Corrected wording:** “The active, explicitly documented <=100-channel stopgap loads a full lineup per uncached time window, then serves viewport slices. Existing sharing/deduplication reduces repeats but does not provide channel-scoped upstream reads.” The cap is 200 pages of 30 **per kind**; All channels can walk live and library kinds separately. Returning the first 200 pages with another cursor pending gives a silently partial result. A group/favorites/source filter may greatly reduce actual work. No measured slowdown at 100 channels is claimed.

**Counterevidence:** This was intentionally deferred architecture, not concealed completion of the scalable backend. The warning callback is optional and the live adapter supplies none; no enforcement of the small-lineup contract was found. Eight retained windows bound cache cardinality, not bytes, and evicting a pending promise does not itself abort its shared requests.

**Fix pitfalls/regression:** Do not give one scrolling consumer ownership of a shared walk's AbortController. Use reference-counted/shared cancellation or separate caller cancellation, and expose cap/truncation explicitly. Test same-window reuse separately from new-window scaling, both kinds, filtered guides, 6,001 channels in one kind, and legitimate 100-channel operation.

### WC-011 — Narrow; retain conditional P2; high confidence

**Independent evidence:** `packages/client-core/src/index.ts:345–407`; `web/src/bridge/account.ts:95–109`; `web/src/bridge/account-api.ts:13–22`; `web/src/bridge/hosted-account-api.ts:14`; `packages/client-core/src/route-identity.ts:80–88`.

**Falsification attempted:** Do route proof checks, credential renewal, endpoint validators, or account helpers universally impose byte/deadline limits? Generic HttpLocalApi readers call `text()`/`json()` after exchange without a response-body budget. Both success and error bodies can be materialized. Selected-route verification bounds its own proof response, not every later API response. Feature validators run after parsing and cannot prevent that allocation. Some service callers already provide abort/deadline ownership.

**Required deadline correction:** `BrowserAccountApi.send` in account.ts explicitly combines the supplied signal with a 15-second timeout. `accountRequest` in hosted-account-api.ts does the same. `accountApi(scope).request` in account-api.ts forwards only the supplied signal, and ordinary HttpLocalApi has no universal default deadline. All examined readers still lack a streamed body byte ceiling. Do not conflate these independent defenses or describe all account transports as timeout-free.

**Corrected wording:** “Ordinary browser JSON readers lack pre-parse response byte limits; HttpLocalApi and the scoped accountApi helper also rely on caller-supplied deadlines, while the two other examined account helpers already enforce fifteen seconds.” Impact needs a malformed/hostile selected endpoint/proxy or unusually large legitimate response. This is not proof that every ordinary response is large or every platform bypasses native transport limits.

**Fix pitfalls/regression:** Preserve native adapter budgets, endpoint-specific large legitimate responses, explicit long polls, and SSE/media contracts. Count decompressed consumed bytes, limit errors too, and avoid treating a mutation timeout as proof it did not commit. Test existing fifteen-second account cancellation remains effective after centralizing readers.

### WC-012 — Uphold P1; high confidence from source and API contracts; browser acceptance unrun

**Independent evidence:** `server/cmd/server/main.go:139–142,192–200`; `web/src/app/session.tsx:701–725`; `packages/client-core/src/server-connections.ts:69–74`; `packages/client-core/src/route-identity.ts:26–29,46–50,89–102`; `web/src/bridge/server-connections.ts:16–31,35–36`; `web/src/bridge/profile-trust.ts:5–7`.

**Falsification attempted:** Does same-origin bundled sign-in bypass proof? Is alternate crypto injected in browser environment? Does localStorage/profile-trust fallback make sign-in work anyway? `signInDirect` inspects/pins the selected origin and calls `connectPairedServer`; browser environment supplies storage, without alternate cryptography. `serverPin` calls `crypto.subtle.digest` before credential-bearing direct sign-in. The profile-trust in-memory fallback is separate and cannot supply missing proof crypto or safe server credential persistence. The server startup instruction explicitly names private IPv4 HTTP on another device.

**Corrected precision:** The secure-context property belongs to the **document's origin**, not the media/API destination. The defect is following the advertised HTTP private-LAN web URL in ordinary browser settings. It is not established merely by passing an HTTP LAN endpoint to a client loaded from HTTPS; such a page has different browser-network restrictions that this finding does not prove. Loopback HTTP, trusted HTTPS, and exceptional enterprise/dev secure-origin overrides are counterexamples. `crypto.getRandomValues` alone is not the blocker; `subtle`, Web Locks, and the UUID use are the relevant dependencies, with proof digest the strongest early failure chain.

**Fix pitfalls/regression:** Do not restore compatibility by skipping server proof, accepting all trust, or using plaintext credential storage. Changing startup text alone is insufficient unless the supported secure LAN bootstrap is achievable. Exercise real second-device HTTP/HTTPS/loopback contexts and setup-to-sign-in-to-restore, with stock browser settings. No browser matrix was executed in this code-only pass.

## Additional concerns — explicit verdicts

The original report gives these bullets no IDs. For traceability here only, `WC-C01` through `WC-C05` map to their original order; they are not extra WC-established-finding numbers.

### WC-C01 — Audio underrun recovery: unresolved as an observed playback defect; retain concern

**Evidence:** `web/src/bridge/audio/engine.ts:76–108,123–145`; `web/src/bridge/DecodeAudioRender.ts:221–264`. Each buffer's scheduled time stays `startFrame + scheduled`; `aheadSeconds` falls to zero as context time advances. A stalled reader can then deliver several buffers with target times already past. `tick` reports starvation but does not rebase or suspend the clock. `linkDecks` has an earliest-frame guard for a successor, which does not repair scheduling within an already-started primary deck.

**Counterevidence/precision:** Ordinary prepared playback has a substantial buffer; brief stalls need not exhaust it. Browser suspension stops context time and is different from network/decode starvation while the context runs. Past start times start as soon as possible rather than automatically skipping the elapsed samples, so the source arithmetic supports overlapping catch-up as a plausible consequence. Actual overlap depends on how quickly recovery feeds those buffers. No audible result or media-element-driven recovery was measured. Keep this as an unresolved behavioral concern rather than assert every underrun produces audible corruption.

**Fix pitfall/regression:** Decide whether to pause media time, skip elapsed content, or rebase scheduling; changing only node start time breaks position, gapless edges, and progress facts. Test a running context with a reader stall exceeding decoded-ahead time, then burst recovery; distinguish intentionally paused/suspended contexts.

### WC-C02 — Cast listener retention: uphold limited P3 lifecycle concern

**Evidence:** `web/src/bridge/cast.ts:61–83,108–113`; `web/src/app/cast.tsx:19–37`; `packages/client-core/src/cast/sender.ts:93–100,135–136,231`. Successful preparation adds an anonymous context session-state listener and never retains/removes it on dispose. API/viewer changes can create new senders. Preparation can finish after disposal and still set global SDK options/register that listener.

**Counterevidence:** HTTPS origin and a configured Cast application are required; unsupported/no-SDK cases add no listener. Shared controller `publish` ignores disposed instances, `adopt` returns false when disposed, and transport-owned message/session-end callbacks have removal functions. Therefore do not claim the stale listener necessarily adopts another viewer's playback or bypasses authorization. Retained closures and redundant callbacks are the demonstrated issue; memory magnitude is unmeasured.

**Fix pitfall/regression:** Track this specific listener and a sender disposal/prepare epoch; do not remove other senders' SDK listeners or terminate a currently valid sender. Test successful prepare/dispose, disposal while config/SDK pending, and repeated viewer switches with listener counts.

### WC-C03 — Capability cache lifetime: narrow; retain low-priority lifetime/freshness concern

**Evidence:** `web/src/app/browse.ts:20–38`; `web/src/app/session.tsx:45–47`. Successful promises stay in a module-global map without eviction. The key includes server/account/profile/**sessionFamilyId** plus library/pivot; token refresh alone does not add entries. Failed promises are deleted. Query and sort changes also do not create extra capability-cache keys.

**Corrected scope:** Growth is bounded by distinct sign-in families, libraries, and pivots visited during this document's lifetime, not by every access-token rotation, query, or result item. Reload destroys the map. Each entry contains capabilities rather than an entire media list. No high material memory cost or cross-viewer reuse is established. A successful cache also lacks freshness invalidation for capability changes within the same family; assess the intended revision contract before adding another defect.

**Fix pitfall/regression:** Apply a modest LRU/TTL or generation invalidation without creating repeated capability fetches per render. Verify token refresh reuse, failed-fetch deletion, library/pivot churn, and explicit server capability updates.

### WC-C04 — Decoded audio memory/CPU: unresolved performance acceptance; retain concern

**Evidence:** `web/src/bridge/DecodeAudioRender.ts:19–23,244–247,288–293`; `web/src/bridge/audio/engine.ts:63–67,93–145`; `web/src/bridge/audio/capabilities.ts:31–47`; `web/src/bridge/audio/track.ts` and `audio/decoder.ts` decoder paths.

**Bound analysis:** PCM/FLAC capability entries advertise up to 384 kHz and eight channels. Thirty seconds of Float32 output at that rate/channel count is approximately 351.6 MiB per deck; ten seconds is about 117.2 MiB. These are arithmetic scenarios, not measured allocations. The actual AudioContext may reject that rate and fall back to the device rate; at 48 kHz/eight channels thirty seconds is about 43.9 MiB. Playing and incoming decks can overlap their decode-ahead budgets, while candidate buffering uses the ten-second target. Rate/sample layout, decoder queues, temporary arrays, and implementation overhead affect peak memory.

**Counterevidence:** Time-based pumping, node release on ended, close cleanup, and the compressed-byte LRU are real bounds. They prevent a claim of memory proportional to full track duration under ordinary non-stalled playback. WebCodecs uses a browser codec implementation; it is inaccurate to say all codec work runs in JavaScript on the main thread. The JS FLAC fallback and explicit resampling/stretch computation do have synchronous JS paths. No device jank/failure threshold was measured.

**Fix pitfall/regression:** Budget output bytes and source/decoder working sets without starving high-channel playback; moving decoding to a worker does not eliminate AudioBuffer storage. Benchmark accepted rates/channels, context fallback, transitions, and JS FLAC separately before setting product limits.

### WC-C05 — Browser encrypted storage threat model: uphold limitation; no vulnerability established

**Evidence:** `web/src/bridge/server-connections.ts:6–31`; `web/src/bridge/profile-trust.ts:9–17`. The non-extractable AES-GCM key and ciphertext are separate values in one origin's IndexedDB. Code with that origin's authority can obtain the CryptoKey and ask WebCrypto to decrypt even if key export is forbidden. Therefore non-extractability is not an isolation boundary against arbitrary same-origin script.

**Counterevidence:** AES-GCM, non-extractability, randomized IVs, database-associated additional data, bounded plaintext/cipher sizes, strict transactions, and Web Locks are useful protections. No same-origin script injection/XSS chain was found in this review. Sharing an origin is not itself an exploit, and this should not become a remediation demand to remove useful at-rest encryption.

**Fix pitfall/regression:** State the storage threat model accurately and separately enforce script trust/CSP/injection defenses. A wrapper hiding the key from application components does not isolate hostile same-origin code. Any stronger isolation design must preserve safe credential refresh/transaction ownership and an explicit recovery model.

## Coverage and remaining proof boundaries

Reviewed all twelve cited implementation chains and all five concern areas, including surrounding callers/owners. Additional counterchecks covered browse effect dependencies and full-page server navigation; DownloadsProvider/dialog navigation; real Together and Diagnostics streaming consumers; generic useService consumers and their deadlines; browser account deadline variations; guide deduplication/probe/window reuse; direct renderer gating and route teardown; MP4 binary helpers and advertised audio capabilities; Cast controller disposal guards; and secure-context proof versus profile-trust fallback.

This pass did not independently repeat the entire original source inventory, certify every backend/native transport, prove a crafted MP4's server admission, quantify hostile-response feasibility, measure memory/latency, or observe browser/device behavior. Suggested regressions are proposals, not passing tests. The application and original audit report were not edited by this reviewer.
