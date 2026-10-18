# Outcome
The server now lists discovered subtitle tracks as shared selectable resources immediately. First delivery fills an internal cache without creating another user-facing resource or requiring a viewer import. Scanning does not demux every subtitle before admitting the file to deeper analysis. Character decoding and cue parsing keep usable text from older and imperfect files. ASS/SSA originals, attached fonts, PGS SUP, paired VobSub and DVB packet capsules have grant-scoped range delivery. Direct playback has a real WebVTT URL. Client-core understands these delivery contracts and can select discoveries without importing them.

Sidecar association, durable local/remote directory lookup, SDH flags, HLS signalling, provider identity/language/login renewal, marker review policy and preview conditional responses are implemented. Original payloads stream from immutable files rather than allocating a whole archive per viewer. The lane implementation and limited verification are complete; results and cold acceptance steps are below. Actual device renderers and automatic capability/fallback integration belong to other lanes and are not claimed complete here.

# Decisions I made
- Execution host: personal Ubuntu `ehler-homelab-mini-1`, Tailscale `100.120.50.24`. Context refresh could not create its lock outside the writable roots; existing relevant guidance was read.
- Keep authentication, source/incarnation/evidence fencing, decoder confinement and existing resource/process bounds. Reject malformed cues individually, not whole usable files.
- Decode through the existing `golang.org/x/text v0.42.0` tables. Detection is our deterministic Unicode-first sampled script/frequency/language scorer; no new dependency. Very short ambiguous legacy samples remain heuristic.
- Keep 32 tracks, 4 MiB text input, 32 MiB bitmap input, 20,000 cues and existing size/depth/time bounds. A capacity inventory keeps a useful bounded subset. An incomplete refresh cannot remove an earlier inventory for unchanged media.
- Cache two tracks concurrently, at most 64 in-flight fills. Cancellation of the first reader does not cancel shared extraction; later readers join. Each embedded extraction has two minutes without successful input progress and a separate two-hour overall limit. Cold first delivery can still take time.
- Discovery resources are shared factual inventory. Explicit personal/shared imports and uploads retain independent identity and history. Cache filling does not bump a user-facing metadata revision.
- Preserve authored ASS/SSA bytes plus their detected encoding. Inline attachment sections unsafe for burn can still produce plain cues and opaque device originals; they never gain server burn admission.
- Deliver DVB as single-track Matroska `.mks`; PGS as `.sup`; embedded VobSub as `.idx`/`.sub` rewrapped from SPU packets without OCR or pixel conversion.
- Attempt at most 16 attached fonts, 8 MiB per font, 16 MiB total, also subject to the archive cap. Invalid, unsupported or excessive fonts are skipped and counted. Attachment names never become filesystem paths.
- Hide weak unapproved scene-boundary hypotheses from viewer prompts. Chapter-labelled and measured high-confidence markers retain their established behavior. Remove the identical intro/recap guess.
- Provider search defaults to the profile's preferred subtitle language, then English. Explicit queries remain explicit; Portuguese and Chinese regional variants survive. Username/password login renews provider tokens; repeated bad credentials stop retrying. Only official API login hosts are accepted. No owner console credential screen was added.
- Extend apikit minimally with optional response MIME metadata. Subtitle errors now use the registered public catalogue and retry policies. Pre-release contracts are regenerated without aliases or compatibility paths.

# Bugs
- B-04.1 — Fixed: text sidecars and embedded discoveries become selectable and eligible for automatic choice immediately, including remote discoveries whose evidence matches the session. Image selection uses the device/burn path.
- B-04.2 — Fixed: removed arbitrary eight-track scan imports; manifests rank up to 32 usable text tracks, selected first, then flags/language.
- B-04.3 — Fixed: cache/extraction failure no longer blocks subtitle inventory completion or unrelated deep analysis.
- B-04.4 — Fixed: no scan-wide 90-second extraction loop; extraction uses independent progress and overall budgets per track.
- B-04.5 — Fixed: over-capacity inventories retain up to 32 tracks instead of discarding all.
- B-04.6 — Fixed: language names/aliases, regional tags, forced/foreign/default/SDH/CC/HI and descriptive titles are recognized.
- B-04.7 — Fixed: matching sidecars attach to both same-stem video versions.
- B-04.8 — Fixed: canonical subtitle reasons use string-catalogue messages, including ambiguous association.
- B-04.9 — Fixed: BOM, UTF-8, UTF-16 and scored legacy charset decoding replaces blanket Windows-1252 fallback.
- B-04.10 — Fixed: malformed ASS events are skipped individually; three-digit fractions and tolerant SRT/VTT/SMI/TTML parsing preserve good cues. Inline attachment sections retain text/original delivery without burn admission.
- B-04.11 — Not fixed deliberately: the programme requires keeping hostile-input bounds, including 32 MiB bitmap admission. Streaming per-viewer delivery reduces allocations but does not admit larger tracks.
- B-04.12 — Fixed cancellation/restart defect: discoveries select immediately, and cache fills survive first-reader cancellation. Full-source extraction is still required for cold embedded tracks.
- B-04.13 — Not fixed here: burn canvas/aspect handling belongs to conversion-outputs; runtime geometry acceptance remains required.
- B-04.14 — Partly fixed: attached fonts are extracted and delivered with styled originals. Burn fonts-directory integration and broad fallback font coverage belong to conversion-outputs.
- B-04.15 — Already fixed on base: shared complete ISO language table; not redone.
- B-04.16 — Already fixed on base: same ISO language normalization for forced matching; not redone.
- B-04.17 — Fixed: probe and sidecar hearing-impaired facts survive inventory/revisions and reach choices, ranking and HLS.
- B-04.18 — Not fixed here: unknown-language forced/default automatic-selection policy belongs to playback-core. Flags are preserved for it.
- B-04.19 — Fixed: episode/provider/series/season identity, regional language and profile language are supplied; the web search no longer sends the UI locale.
- B-04.20 — Fixed server authentication lifecycle: username/password login, renewal, rate handling and rejected-login suppression replace startup-token environment input. Owner credential screen remains a client/owner-console task.
- B-04.21 — Fixed viewer exposure of weak unreviewed hypotheses; the underlying heuristic is not replaced by cross-episode fingerprinting.
- B-04.22 — Fixed: identical intro/recap guess is no longer emitted twice.
- B-04.23 — Not fixed here: chapter admission belongs to music-audiobooks.
- B-04.24 — Fixed server resource gap: matching pending choices can address discovered text and image resources. Client title-page choices still need integrated platform verification.
- B-04.25 — Fixed: literal curly-brace text survives; only ASS override blocks beginning with a backslash are stripped.
- B-04.26 — Not fixed here: independent overlapping-cue layout belongs to native client renderers.
- B-04.27 — Fixed: scans retain indexed subtitle directory pages for local and remote sources; queries filter association before candidate bounds. Explicit refresh uses filtered helper enumeration, so thousands of unrelated entries do not consume the subtitle candidate budget.
- B-04.28 — Not fixed here: analysis FFmpeg command tolerance belongs to conversion-outputs/library scanning. Subtitle cue tolerance is independent of it.
- B-04.29 — Fixed: trickplay stores sheets and descriptors, not duplicate per-frame JPEG artifacts.
- B-04.30 — Fixed: preview tiles and chapter images honor authorized conditional requests with 304.
- B-04.31 — Already fixed on base: Windows extraction gate removed. Touched packages cross-compile; actual Windows decoder execution remains acceptance work.
- B-04.32 — Already fixed on base for priority and per-chunk checks; additionally converted remaining document/manifest reads to read snapshots.
- B-04.33 — Fixed: removed dead duplicate subtitle-plan implementation/tables and obsolete parser/clock/style helpers.
- B-04.34 — Fixed: subtitle extraction accepts the MPEG demuxer used by VOB/MPG/MPEG sources.
- B-04.35 — Not fixed here: Complete-tier stage cost/profile design belongs to library scanning. Subtitle scan demux cost is removed.
- B-04.36 — Fixed: v1 advertises a real `.vtt` endpoint, including direct sessions; JSON documents remain separately identified.
- B-04.37 — Fixed: HLS carries selected/default/autoselect, forced and SDH signalling; Off does not default a container track.
- B-04.38 — Not fixed here: HDR tone mapping and anamorphic preview command construction belong to conversion-outputs.
- B-04.39 — Not fixed here: browser cue rebuilding belongs to client-web; the speculative flicker needs runtime observation there.
- B-04.40 — Fixed owner-facing setting description: chapter/scene suggestions and review, without promising cross-episode comparison. Private architecture/spec prose still needs reconciliation.
- N-V03.1 — Fixed: a dedicated bounded 32 MiB subtitle binary reader handles SUP/SUB while ordinary metadata reads remain bounded at 8 MiB, with the same rooted/remote fencing.

V03 conversion entries are outside this lane:
- B-03.1 — Already fixed per base FIX-LOG: hardware keyframe segmentation; hardware/runtime acceptance remains with conversion-outputs.
- B-03.2 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.3 — Already fixed per base FIX-LOG: native VA-API filter qualification; hardware/runtime acceptance remains with conversion-outputs.
- B-03.4 — Already fixed per base FIX-LOG: sandbox tone-mapping selection; hardware/runtime acceptance remains with conversion-outputs.
- B-03.5 — Already fixed per base FIX-LOG: remote hardware permissions/diagnostics; hardware/runtime acceptance remains with conversion-outputs.
- B-03.6 — Already fixed per base FIX-LOG: hardware circuit-breaker attribution; hardware/runtime acceptance remains with conversion-outputs.
- B-03.7 — Not present as claimed: V03 refutes the keyframe burst for the pinned FFmpeg bundle.
- B-03.8 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.9 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.10 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.11 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.12 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.13 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.14 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.15 — Already fixed per base FIX-LOG: converted audio sample rate; hardware/runtime acceptance remains with conversion-outputs.
- B-03.16 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.17 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.18 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.
- B-03.19 — Not fixed here: conversion/preparation/capability/retention/live throughput is outside this lane; runtime-dependent claims retain their V03 qualification.

# API changes
- Existing catalogue, acquisition, refresh, resource edit/delete and subtitle selection routes now have typed apikit registrations. No legacy alias was added. The old manual subtitle mux registration is removed. Upload bytes have an explicit base64 wire schema.
- Catalogue resources and playback-option subtitle streams add `deliveries`: `text`, `ass`, `pgs`, `vobsub`, `dvb`. Catalogue entries advertise formats; selected entries include grant URLs and optional `companionUrl`/`fontsUrl`.
- Subtitle selection accepts optional `rendering` using those values. Device-original selection uses `renderer: "client"`. Existing v1 `embeddedClient` selection now chooses and delivers the original. Client-core validates these fields and exposes selection without an import step.
- GET `/v1/media/subtitles/{grant}/{resource}/{revision}.vtt` is actual `text/vtt`, available for direct and HLS sessions. The unsuffixed route remains the separately labelled cue-document JSON contract. VTT validators include its window/offset; byte ranges are supported.
- GET and HEAD `/v1/media/subtitles/{grant}/{resource}/{revision}/original.ass`, `.ssa`, `.sup`, `.idx`, `.sub`, `.mks` stream original bytes with ranges and ETags. ASS/SSA use `text/x-ssa` plus detected charset; IDX uses `text/plain`; image packets/capsules use `application/octet-stream`.
- GET the same base + `/fonts` returns `{fonts:[{name,url}], skippedCount?}`; GET/HEAD `/fonts/{font}` returns font bytes with ranges. Names are labels; numeric IDs address sealed objects.
- Remote evidence is recorded before automatic choice; manifest reads reject replaced representations just as original/document reads do. Original/font/text reads require live grants, selected/manifest pins, viewer scope and exact source/evidence. Streaming checks authorization every 32 KiB; conditional responses also authenticate first. Payloads use `no-store`.
- Registered errors carry catalogue codes and retry policies, replacing manual subtitle error envelopes. Generated OpenAPI/TypeScript contracts describe actual MIME types and binary payloads.
- GET preview tile/chapter image routes now use typed registry contracts and authenticated ETag conditionals; URLs are unchanged.
- Provider search retains the existing request shape; omitted language now uses profile preferences, and omitted query adds factual item/provider identity. Client-core search defaults to omitted language.

# Schema changes
- Edited pre-release baseline directly and ran schema-sync; no compatibility migration.
- Subtitle revisions admit zero-size internal caches. `subtitle_resources.discovered` distinguishes inventory resources from independent imports/uploads; removing discoveries retires only their inventory resources.
- Added `subtitle_original_revisions` for authored archive references/skipped-font counts and `subtitle_original_files` for normalized streaming data, companion, font and text-projection object references. Metadata revisions copy references; GC counts all references.
- Hearing-impaired facts survive inventory/render revisions.
- Subtitle inventory entries have normalized directory/name keys and association indexes. Local scans commit directory pages as remote scans do.
- Removed unused playback subtitle plan/track tables and their rebase entry.

# Owner or viewer settings added or wired
- Existing preferred subtitle languages: profile language list, existing default; now also determines provider search language. Existing preference screens can continue editing it.
- Device subtitle rendering choice: API/client-core enum `text|ass|pgs|vobsub|dvb`; omitted choice retains established text/burn behavior. No new screen in this lane. Capability-driven automatic choice is playback-core integration.
- OpenSubtitles credentials: optional owner-held `PORTICO_OPENSUBTITLES_USERNAME` and `PORTICO_OPENSUBTITLES_PASSWORD`, unset by default; used for login/renewal. Existing API key enables anonymous use. `PORTICO_OPENSUBTITLES_TOKEN` is no longer read. No credential screen exists in this lane. The provider's official API contract was consulted: https://stoplight.io/api/v1/projects/opensubtitles/opensubtitles-api/nodes/open_api.json.
- Existing segment-detection setting retains its type/default and now accurately describes chapter/scene suggestions needing review.

# Needed from others
- Playback-core: integrate capability `subtitleRendering` into Automatic/Native/mpv delivery choices and server fallback; settle unknown forced/default selection policy. Server delivery enums are exactly `text`, `ass`, `pgs`, `vobsub`, `dvb`.
- Client lanes: libass/WASM libass for ASS/SSA with charset-aware authored script decoding and attached fonts; PGS SUP, VobSub paired files and DVB MKS demux/rendering. Preserve independent overlapping cue placement, offsets, pause/seeks and source-relative timing. Hide unsupported renderers. Verify title-page pending choices, upload/search/error screens and native/web cue refresh.
- Conversion-outputs: consume archived fonts for burn, broad glyph fallback, high-quality image geometry, HDR/anamorphic previews, tolerant analysis decoder policy. The internal OriginalAsset contract preserves fonts/authored assets; public reads use streaming files.
- Library scanning: Complete-tier cost/stage profile and labels; subtitle scan stage now completes inventory without extracting bytes.
- Music-audiobooks: tolerant chapter admission/start-origin normalization.
- Downloads: copy all selected originals/companions/fonts and metadata into offline bundles; preparation may not depend on live grants after download. Existing offline plans need this new delivery integration.
- Owner-console/client-web: provider credential screen if desired; web cue mapping flicker acceptance. The minimal web search call change must merge with the client lane.
- Documentation/integration: reconcile private subtitle/player specs that prohibit on-device styled/image rendering, promise JSON as WebVTT or promise cross-episode detection. Public playback guide includes these delivery contracts. Merge the localized catalog/ingestion/schema/registry/settings/string changes with their owning lanes.

# Test notes for other environments
Verified on Ubuntu, final check 2026-10-13 05:39 UTC:
- `GOFLAGS=-p=2 go build ./...` from `server` passed on the final tree.
- Focused Go regex tests passed in subtitles, subtitlevideo, storage, httpapi, playback, mediaanalysis and decoder. Coverage includes charset scripts/UTF-16, tolerant parsing/skipped cues, immediate discovery/auto-selection, shared cache cancellation, exact authored ASS/text projection, metadata revisions, source/grant revocation and ranges, inventory capacity/filtering, provider query/login renewal, font attachment extraction and PGS/VobSub packet fixtures.
- After the final remote-evidence correction, focused subtitles/playback/httpapi tests passed again, including matching versus replaced remote discovery and manifest evidence. Selected and manifest reads require matching remote evidence.
- Independent progress tests passed: successful reads renew idle time, failed reads do not, overall time remains bounded. HLS selected/forced/SDH signalling test passed.
- `go vet` passed for subtitles, subtitlevideo, storage, httpapi, playbackv1, mediaanalysis and decoder; final changed subtitles/playback also passed.
- Touched packages cross-compiled for Windows amd64 and Darwin arm64 (subtitles, subtitlevideo, storage, decoder, httpapi, playbackv1, mediaanalysis, ingestion). Final subtitles/playback changes were cross-compiled again. This proves compilation, not physical decoder/device behavior.
- `npx tsc --noEmit` in web passed. Client-core `subtitles.test.ts` passed 15 tests; `subtitle-presentation.test.ts` passed 5. They cover delivery URLs/companions, skipped counts, literal braces, cancellation/CAS/privacy and avoiding replay of same-generation historical presentations.
- Apikit binary/MIME generation test passed. `go run ./cmd/apigen` regenerated OpenAPI and TypeScript contracts, and route/access/coverage tests passed. `python3 scripts/schema-sync.py` passed, including its persistence check. `git diff --check` passed.
- Real FFmpeg fixture tests preserved attached font bytes and returned embedded PGS SUP. VobSub fixture packets/palette were preserved and ffprobe recognized DVD subtitles. No real-disc pixel comparison, device glyph test or live provider account was used.
- No full repository test suite, device build, Docker build, deployment or Git commit was performed. Namespace confinement was unavailable on this host for burn acceptance; fixture execution is not proof of confined deployment readiness.

Cumulative snapshots: 001 “Make discovered subtitles selectable and decode legacy text”; 002 “Deliver subtitle originals with playback grants and retain track flags”; 003 “Stream subtitle files without loading full archives per viewer”; final 004 “Finish subtitle delivery contracts and source-fenced selection”. The final snapshot is taken after this report and final validation; no tree changes follow it.

Cold acceptance instructions:
1. On Linux/macOS/Windows with the configured confined FFmpeg/FFprobe bundle, scan a multi-language file with more than 32 tracks plus matching SRT/ASS/SMI/TTML/SUP/VobSub sidecars. Expect up to 32 usable tracks immediately, no viewer import state, forced/default/SDH facts retained, and malformed cues skipped while good ones play. A single empty track must not block trickplay/other analysis. Scan two same-stem video versions and a flat folder with over 4,096 unrelated entries; matching sidecars must survive.
2. Use a 40 GB remux on a slow NAS. Select a cold embedded track, cancel its first delivery request, then retry. Expect one continuing shared extraction, independent per-track idle budgets, no file-analysis failure and eventual delivery. Stop source progress for two minutes: extraction must cancel. Keep progress moving: it may exceed 90 seconds, but not the two-hour outer bound. Owner console responsiveness under concurrent fills requires runtime observation.
3. Start direct playback and follow advertised WebVTT and ASS/image/font URLs. Check GET, HEAD, Range, 416 and If-None-Match behavior, exact ASS authored bytes and paired VobSub links. Stop the session, revoke access or replace the physical file: every subsequent/conditional read must fail, and an active stream must stop on its next checked chunk. Another profile/source/resource must not be addressable through that grant. Repeat remote-source evidence replacement and inaccessible root cases.
4. Render multilingual ASS with attached TTF/OTF/collection fonts on every target device and burn fallback. Compare styles, positioning, CJK/Thai/Indic glyphs, font overrides and offsets to a trusted player. Font attempt/size bounds must skip bad/excess attachments and report counts. Device rendering and container glyph coverage were not verified here.
5. Compare authored SUP and rewrapped VobSub from real DVD/Blu-ray media, including nonzero media origins, cropped video, canvas sizes, multiple cues and seeks. Fixtures preserve packets and ffprobe recognizes the VobSub result; real visual/timing acceptance remains required. Verify DVB MKS demuxing on target engines.
6. Try the listed legacy encodings, UTF-16 LE/BE with/without BOM, malformed ASS events, missing SRT separators, literal `{LAUGHS}`, SMI and TTML. Expected: correct fixture scripts/language, usable cues retained and no markup execution. Very short unknown-language legacy text may need a future explicit owner encoding override.
7. With a real provider account, search episodes sharing titles such as Pilot, pt-BR and zh-TW tracks; expire the token and retry. Expect series/season/episode/provider identity, region retained and one login renewal. Wrong credentials must not trigger repeated login attempts. Mock transport tests cover this; no live account was used.
8. Review weak luminance markers: viewers should see no prompt before approval; chapter/high-confidence markers still work. Preview tile/chapter ETags should return authenticated 304 after expiry. Confirm trickplay creates no duplicate per-frame image artifacts.

# Not finished
- Actual on-device engines/screens, capability-driven automatic device/burn fallback, offline bundle delivery and conversion font/geometry integration are owned by other lanes. The programme-wide feature is not complete until those are integrated.
- The existing 32 MiB bitmap and 32-track limits are retained by instruction; larger tracks remain unsupported, and excess tracks retain a bounded subset.
- Real Windows/macOS execution, NAS timing/load, confined decoder acceptance, real DVD/DVB visual timing, device glyph/layout, live provider credentials and HDR/preview behavior require the environments above. This host's namespace sandbox was unavailable for a burn test; baseline FFmpeg fixture tests do not prove sandbox deployment readiness.
- Chapter admission, Complete scan redesign and client cue refresh are explicit handoffs above. No cross-episode intro detector was invented.


# Merge with the base

Completed the follow-up merge after reading the eight named lane reports. No staging, commit or snapshot was performed; the five files remain unmerged in the index until Justin stages their resolved contents.

- `apikit/generate.go`: keep subtitle/image MIME metadata and the base's `x-scope` together.
- `apikit/registry.go`: adopt `AdminScope`, mandatory `Requires`, scope gating and path-aware access validation; retain `ResponseContentType` and MIME validation.
- `foundation.go`: retain all subtitle/preview, credential, platform, webhook and MCP registrations; gate scopes and freeze after registration.
- `policy_errors.go`: combine subtitle, connection, credential and webhook errors with the existing catalogue.
- `route_lanes.go`: preserve every base admission value, including tools/uploads, and add subtitle original/font/VTT/selection entries. Adapt tile matching to `.jpg`. Removed trash, raw item deletion and obsolete API-key routes stay removed.

Subtitle metadata declares read for reads, playback for selection, library for acquisition/edits, and open for Public grant payloads, whose existing live grant/source checks still authorize delivery. Subtitle parameters use `{itemId}` for the base's item check, without changing URL spellings. Scope/lane tables and restriction probes agree. Preserve the base's HTTP/trusted-proxy policy, missing-title model, provider changes and web player path. Update the foundation fixture to expect unavailable with the base's default release feed and no fetcher; extend restriction classifications for landed routes and test real subtitle credential reads/refusals. No removed setting or helper was restored.

Verified after resolution:
- No repository conflict markers found by recursive grep; `git diff --check` clean.
- `GOFLAGS=-p=2 go build ./...` passed on Linux, Windows amd64 and Darwin arm64.
- `go vet` passed for apikit, httpapi and subtitles.
- Targeted apikit registry/scope/access/MIME generation tests passed. Combined httpapi foundation, route/lane/scope/OpenAPI, policy, subtitle/grant, credential, restriction-matrix, webhook and MCP tests passed. Focused subtitle cache/discovery/manifest/source tests passed.
- `go run ./cmd/apigen` and `python3 scripts/schema-sync.py` regenerated artifacts successfully; the schema persistence check passed. Generated files were not hand-merged.
- `npx tsc --noEmit` in web passed. No full test suite, deployment or native execution was performed.


# Subtitle addresses and production startup

The merged Live TV media pattern and the former nested subtitle-original/font patterns were ambiguous to Go's ServeMux. All subtitle payloads now share the reserved namespace `/v1/media/subtitles/{grant}/{resource}/{revision}`: JSON at the base, WebVTT at `.vtt`, originals at `/original.ass|ssa|sup|idx|sub|mks`, font list at `/fonts` and font bytes at `/fonts/{font}`. The earlier grant-first shapes are removed. Live streams remain under `/v1/media/linear/...`. Grant checks, source pins/evidence, range support and read bounds are unchanged.

Server plans, HLS text manifests and font lists use the shared `subtitles.MediaBaseURL` builder. Registry routes, admission/scope/restriction classifications, client-core validators/readers, fake-server fixtures, web subtitle fixtures and public API documentation are updated together. Real clients use the original/companion/fonts/document addresses returned by the server; they do not derive these addresses from video URLs. Validators construct expected paths only for validation, and test fixtures deliberately construct sample addresses. Native repository copies are not edited.

Download originals/companions/fonts already use server-supplied bundle-relative paths under the download grant. They do not carry live playback addresses, so their transport shape is retained; a new client-core regression covers original/font files and rejects a live grant path in an offline bundle.

New httpapi guards construct the real production router with optional media route families enabled and mount every declared production pattern, including optional networking patterns, on a ServeMux. They check every `/v1/media` route lookup, using `linear` as wildcard values to exercise the old collision. Both guards passed; no further media pattern ambiguity was found. Contracts are regenerated. The requested checks were run; final results and limitations follow. One snapshot closes this follow-up, with no tree changes afterward.


Verification checkpoint after application restart: the client-core subtitle/download selection passed 80 tests, web subtitle renderer/image/surface files passed 26 tests, TypeScript passed, and the updated server binary compiled. Router construction/production-pattern guards and focused playback/subtitle tests pass. The larger HTTP selection ran 55 passing tests before its four-minute limit, but exposed unrelated failures: tuner-exhaustion returned 201 instead of refusal, and OpenAPI coverage lacks the unchanged `GET /receiver/cast/` route. It timed out in `TestPlaybackV1ArtistPlaysInOneRequestQuickly`. Those areas were not changed by this fix. The requested first-run startup test now reaches readiness rather than panicking, but measured 2.814 s during concurrent media tests and 2.065 s in a later run against a 2 s budget; second-run startup passed at 1.083 s and 1.041 s. The timing threshold is retained. Networking startup/owner/restart and main dependency wiring passed. Focused final HTTP subtitle/playback/scope/router checks subsequently passed (7.140 s), including originals, byte ranges, HEAD, font-list addresses and grant revocation. The updated HLS namespace test, subtitle source/manifest tests, playback replan test and targeted vet passed. `GOFLAGS=-p=2 go build ./...` passed on the final tree. Contract generation, client-core tests, web renderer tests, TypeScript and diff checks passed. No conflict markers remain.


Final delivery: snapshot 005, “Keep subtitle file addresses distinct from live stream addresses so the server starts”. This is the only snapshot taken for the startup-address follow-up. The route-overlap defect and every in-scope producer/consumer are fixed; the initial-start timing miss, tuner-exhaustion failure, Cast documentation gap and artist-playback timeout are retained as exact remaining verification issues, not hidden or weakened. No staging, commit, deployment or other-repository edit was performed. No tree changes follow this snapshot.
