# Outcome
Live viewers and DVR workers share one upstream acquisition per source/channel, with separate private reader endpoints and conversion processes. Tuner admission and owner reporting count channels while retaining each viewer/recording's durable lease. The final reader keeps its allocation until the input physically closes after a three-second grace period.

Imports keep usable playlist and guide rows, report bounded diagnostics, support compressed guides, infer missing stop times, and apply selected groups/channels before programme staging. Remote guides stream to private temporary files; guides above 10,000 programmes use a private SQLite spool and publish in 128-row checkpoints. Defaults cover 50,000 channels, five million programmes and 512 MiB, with owner-adjustable bounds.

Recordings reconnect inside their planned live window, retained partial captures resume, and coverage uses programme time rather than mandatory padding. Two profiles recording the same airing use a capture leader and immutable hard links, with private catalogue ownership and independent deletion. Reliable season/episode facts publish a show/season/episode hierarchy and description. Compatible live codecs copy according to the current capability profile.

The native server build, Windows amd64 and macOS arm64 cross-builds, targeted Go checks, shared-input race check, schema check, client-core typecheck and individual TS tests pass. No deployment or push occurred. Snapshots 001–004 preserve compiling milestones. Final cumulative snapshot: _program/snapshots/livetv/005 (taken after this report is finalized).

# Decisions I made
- Share acquisition only. Reader cancellation, output endpoints, authorization and conversions remain independent. Pools are process-local, consistent with the single-server process.
- Keep the source network policy, entity rejection, bounded parsing and confined decoder requirements. A normal external XMLTV DOCTYPE remains accepted; entity definitions/internal subsets remain rejected.
- Limits are 512 MiB/50,000 channels/5,000,000 programmes by default; owners can set up to 1 GiB/100,000/10,000,000. Preview samples contain at most 100 channel names/mapping keys and ten diagnostic examples. Limit overflow skips the excess rows and reports it; oversized documents remain rejected.
- Stable fallback channel identity excludes query tokens. Repeated provider IDs receive deterministic variant suffixes. Common catch-up facts are retained; this does not add catch-up playback.
- Remote guides obey the owner's horizon; manual uploads preserve explicit programme dates. A missing stop uses the next programme start, with a bounded fallback for a final airing. Overlaps are trimmed; conflicting new/repeat flags favor repeat.
- New shared readers start at the current input chunk; they do not replay another viewer's retained input history. Shared recordings require the same exact programme/channel/source, preserving privacy and avoiding mistaken cross-airing sharing. Hard links allow each profile to delete its own path; only the final unlink releases the bytes. Reliable episode identity suppresses duplicate series recordings. Unknown episode identity is not guessed.
- Programme time outranks optional padding. Capture completeness allows the existing four-second start tolerance and counts missing input honestly. The accepted capture window is retained if a later guide temporarily loses the airing. Before capture, an explicit commitment follows a correction within 30 minutes only when the bounded candidate set contains one matching episode identity or matching title/series/subtitle/episode facts; ambiguous reruns preserve the old interval.
- Keep existing timeshift defaults (30 minutes/1 GiB) and advertise actual retained coverage. The source stream-buffer setting controls startup/live delay; it does not pretend to extend that timeshift coverage.
- Context-sync could not acquire its lock in the read-only context directory. Existing canonical project, standards and verification guidance was read on ehler-homelab-mini-1.

# Bugs
- B-11.1: Already fixed on the base; preserved stream-fact fences so guide refresh does not retune.
- B-11.2: Already fixed on the base; harmless DOCTYPE remains accepted, entity definitions rejected.
- B-11.3: Fixed common XMLTV short dates, missing UTC offset, inferred stops and supported legacy text encodings.
- B-11.4: Fixed tolerant programme rows, overlap trimming, consistent new/repeat facts and safe unknown-element skipping.
- B-11.5: Fixed raised adjustable limits, streamed guide fetch/parsing, bounded spool and chunked publication.
- B-11.6: Fixed gzip by magic bytes as well as content encoding, with compressed and expanded bounds.
- B-11.7: Fixed per-row playlist tolerance, common EXTINF attributes, fallback IDs and duplicate variants.
- B-11.8: Backend fixed HDHomeRun guide mapping by XMLTV ID, number or display-name; owner form needs guide URL/mapping controls.
- B-11.9: Fixed runtime wiring for stream buffer, retry window, user agent, filters, numbering, mapping, hidden channels and horizon. Standalone guide-source creation is not implemented; attachment can reference another configured source's guide.
- B-11.10: Fixed distinct-channel reservation sweeps and soft padding yielding to programme cores; same-channel records do not compete for another tuner.
- B-11.11: Fixed upstream retries and capture continuation within the planned window, with gaps recorded.
- B-11.12: Fixed retained partial capture continuation when its live window remains open; sealed captures validate/publish normally.
- B-11.13: Fixed explicit commitments following a unique nearby correction with matching programme facts; ambiguous/missing guide data retains the accepted window.
- B-11.14: Fixed HLS master selection by highest bandwidth with bounded shared acquisition/cache; device/provider fixture acceptance remains external.
- B-11.15: Fixed admission through the advertised HLS transport tuple instead of codec-name whitelists, including HEVC/H.264/MPEG-2 and AC-3/E-AC-3/multichannel AAC when declared supported. Native Android/Fire TV acceptance remains external.
- B-11.16: Fixed unknown live frame-rate/bitrate facts no longer forcing conversion when the declared tuple is compatible and no requested quality ceiling requires those facts.
- B-11.17: Already fixed healthy recovery reset on the base; preserved.
- B-11.18: Fixed segment publication retrying transient authority/database failures for up to three seconds; explicit policy refusal remains immediate.
- B-11.19: Fixed dropping unsupported optional/low-latency and unknown URI-bearing tags; supported segment/key/map URLs remain validated and privately rewritten.
- B-11.20: Fixed completeness against the programme core, rather than mandatory full padding.
- B-11.21: Fixed storage measurement outside the capture write lock; concurrent writes are included conservatively and hard-linked objects count once.
- B-11.22: Already fixed retention-before-low-space refusal on the base; preserved.
- B-11.23: Fixed removing an unused transport hash during validation.
- B-11.24: Fixed dd_progid series recognition only for reliable EP identifiers; movie/special/generic ending-0000 identifiers do not create false series.
- B-11.25: Fixed reliable episode deduplication using external episode identity or series+season+number. Unknown identities remain distinct.
- B-11.26: Fixed the normal two-profile capture/storage path and last-owner physical deletion. Active leader cancellation does not yet transfer capture leadership seamlessly; a follower falls back to a new capture.
- B-11.27: Backend fixed: session responses expose reservationRequiredAtMs and profile-scoped channel.reservation-required events carry it for every affected shared-channel viewer. Duplicate retries do not repeat events. Client countdown rendering remains to be wired.
- B-11.28: Fixed discovered tuner descriptor reads and configured-device detection by stored locator host.
- B-11.29: Fixed owner global hidden/number overlays in directory, guide and input admission; distinguish global vs personal hide in client screens.
- B-11.30: Fixed programme rating gates at start, monitoring and media access, applying profile restrictions and the shared member rating ladder. Missing/unknown guide ratings follow the unrated policy. A narrow access.Enforcer helper was added to reuse account policy rather than duplicating it.
- B-11.31: Not present in current baseline: directory candidates already use SQL paging. Preserved it; owner overlays are joined into the query.
- B-11.32: Fixed superseded generation cleanup grace reduced from 24 hours to five minutes, with new per-generation tables pruned.
- B-11.33: Fixed two-minute refresh lease renewal every 20 seconds, with bounded gated writes and cancellation on lost fencing.
- B-11.34: Fixed time ruler alignment in quarter-hour-offset zones; Kathmandu/Chatham regression fixtures pass.
- B-11.35: Deliberate bounded timeshift policy retained; readiness/live delay is now configurable and actual span is reported. Larger viewer timeshift profiles are not added.
- A-22.37: Fixed guide reads sharing in-flight work only; completed results are no longer reused by time.

# API changes
Typed apikit contracts now own ten methods on seven paths: GET/POST /v1/admin/live-sources; POST /v1/admin/live-sources/preview; POST /v1/admin/live-sources/remote/preview; POST /v1/admin/live-sources/remote/save; GET/PUT /v1/admin/live/settings; GET/PUT /v1/admin/live-sources/{id}/configuration; GET /v1/admin/dvr/tuners. Old mux registrations and authored YAML copies for these paths were removed. Registry-generated contracts and OpenAPI were regenerated. Existing owner-only authorization is checked before/after work.

- Source input adds restrictive per-import limits and enabled groups/channels; limits cannot exceed the saved owner budget. Remote drafts also accept enabled groups/channels before initial publication (preview body budget is 8 MiB). Source/preview includes diagnostics: skippedChannels, skippedProgrammes, adjustedProgrammes and at most ten catalogue-key examples. Preview names/mapping keys sample the first 100, with full counts separately reported.
- Live defaults add importLimits; source configuration adds enabledGroups/enabledChannels. Client-core parsers/types/services updated.
- Tuner active assignments include live/recording kind, account/profile/title and retirement state. InUse counts channels rather than assignment rows. Upcoming records and conflicts use distinct channels.
- Linear session media adds optional reservationRequiredAtMs; client-core retains it. Profile event channel.reservation-required has resource kind session, the affected session ID and data {reservationRequiredAtMs}. Existing event-client parsing accepts it; screen subscription/countdown work remains. Tuner pre-emption targets every live lease for the chosen channel, avoiding an ineffective single-viewer eviction. Initial playback admission now counts the same distinct channel union as runtime admission, including pending sessions and quarantined allocations.
- LAN confirmation error roots move from error.confirmRoots to error.current.confirmRoots in the standard typed error envelope. Client-core reads the new location. Error catalogue entries were registered.
- Registry error response schemas are absent from the current generator even when metadata declares errors. HTTP fixture tests validate the actual error envelope; the registry lane must finish error-schema generation.

# Schema changes
Edited the pre-release baseline only, with no migration/compatibility path. Added dvr_recording_shares (follower recording to capture leader), live_import_diagnostics (counts and bounded example keys per generation), and live_channel_attributes (country/catch-up facts per generation/channel), plus their indexes. Existing cleanup removes per-generation facts. Ran scripts/schema-sync.py and updated known_columns.go/known_schema.go.

# Owner or viewer settings added or wired
- importLimits: bytes/channels/programmes, default 512 MiB/50,000/5,000,000, hard maxima 1 GiB/100,000/10,000,000. Takes effect during fetch, preview and publication. New owner controls needed.
- enabledGroups/enabledChannels: arrays, empty means all. Available both on initial remote drafts and saved source configuration; an explicit empty saved selection clears an earlier draft selection. Parsed before guide staging; disabled channels have no catalogue/programme staging cost. New selection controls needed.
- streamBufferSeconds: integer 0–120, default 8; per-source nullable override; live readiness and EXT-X-START delay. Existing owner controls need verification.
- retryWindowSeconds: integer, default 60; per-source nullable override; upstream reconnect budget. Existing controls.
- userAgent: bounded string, default Portico/1.0; source fetch, guide fetch and input requests. Existing controls.
- categories/countries/keywords include/exclude filters, channelNumbering and guide mappings: now affect imported rows/current reads; existing controls.
- owner global hidden channels: affect every viewer and tuning, independent of personal hide; existing controls need clear labels.
- guideDays: default seven, affects remote guide ingestion. Existing controls.
- source logo import/overwrite settings affect imported logo values; overwrite off retains the prior channel URL across generations. The existing default logo/discovery worker policy is retained; no new artwork-download pipeline is added here.
- discoveryEnabled: existing setting continues to gate LAN discovery; descriptor enrichment is bounded to 64 devices/total five seconds.

# Needed from others
- Client/owner lanes: render diagnostic catalogue keys/counts; add import-limit and enabled-group/channel controls; permit HDHomeRun guide URL/mappings; label live and recording tuner users; render reservation countdowns quietly; expose the guide-backed recorded episode tree while keeping profile-private ownership.
- API-platform/event lane: integrate the channel.reservation-required event with its event catalogue, reconcile typed live route administrative scopes with its new scope model, and generate typed error response schemas. Member programme ratings now reuse access.Enforcer.AdmitProgramme; merge that narrow helper with any concurrent enforcement changes.
- Identity lane: connect DVR publication to its final shared title/file attach point. This tree has no such helper; publication currently emits compactcatalog Show/Season/Episode facts with independent recording airing keys.
- Playback-core: re-point the centralized live copy check (playback.HLSTSRejections / linearPlan) to the new single capability document and Automatic/mpv selection. Avoid a second client translation.
- Platform/conversion-output lanes: Windows inherited physical locks, artifact retirement and confined decoder input/output custody remain missing in the baseline and prevent Windows Live TV/DVR acceptance. They require coordinated shared decoder/artifactlease changes beyond this lane's local wiring.
- Update architecture/product statements that describe strict rejection, 2,000-channel limits, stereo-only live copies, independent tuner acquisition or title-only recordings.

# Test notes for other environments
Completed limited checks: repeated server go build ./... with GOFLAGS=-p=2; focused parser/large-spool/gzip/security tests, shared raw readers/reconnect fixtures, allocation/conflict and DVR sharing/episode hierarchy tests, source-settings/tuner and typed HTTP tests, client-core tsc, and individual guide-read/guide-model/administration TS test files. A recording filesystem test verifies one inode, single-count measurement, independent unlink and last-owner removal. New targeted checks verify initial remote selection, fallback/variant selection, shared-channel front-door admission, one deadline event per viewer, unique guide correction, member rating refusal and audio-only conversion retaining HDR. go vet passes for the touched acquisition/buffer/DVR/media/runtime/access packages; the shared raw/HLS lifetime and grace-period rejoin tests pass under -race. Windows amd64 and macOS arm64 server cross-builds pass. Client-core tsc and explicit strict typechecks of linear-api.ts/administration.ts pass; guide-reads (8), administration (18) and guide-model (9) individual tests pass. schema-sync.py passes the Schema/KnownSchema checks. git diff --check passes. Generated contracts were refreshed after the final API shape change. Confined HLS ffprobe fixture skipped because decoder confinement is unavailable in this sandbox; this is not a successful playback test. Full suites/device builds/deployment were not run.

HDHomeRun: on Linux/macOS/Windows release candidates with working decoder custody, configure an actual device with one tuner and XMLTV guide. Start two profiles on one channel and a recording; device must report one tuner. Stop/seek one viewer: the others continue. Change one viewer to another channel: honest capacity refusal lists the users. Close everyone: tuner closes after the short grace; rapid same-channel rejoin reuses it. Refresh guide repeatedly: no retune. Discovery must show device name/model/count/firmware and configured status.

M3U provider: use 30,000+ channels and a 200+ MiB compressed guide with one bad EXTINF, duplicate IDs, rotating query tokens, harmless DOCTYPE, missing stop, unknown XML element, BOM and legacy text encoding. Preview/save/refresh must stay bounded, retain good rows, report skips, match guide IDs/numbers/names and obey selections/settings. Browse must keep the old guide available during staging. Inject an entity/internal subset: whole hostile document must fail.

Live/HLS devices: exercise native HEVC HDR, MPEG-2, AC-3/E-AC-3 and 5.1 AAC on devices that declare each supported tuple. Original should copy; unsupported tuples should convert only the required tracks. Audio-only conversion must preserve copied HDR. Exercise master variants, optional LL tags, AES-128 keys, ranges and shared segment acquisition. Retry loss beyond and within configured windows; only slow reader should lose its own bounded span.

DVR: schedule the same airing from two profiles; observe one input/capture and one physical inode across private paths. Try private library access from the other profile: deny. Remove one owner: other plays; remove final owner: bytes disappear after reader retirement. Interrupt input and restart server inside the live window: capture continues with gaps; outside the window: retained partial is honestly incomplete. Test back-to-back channels with padding and overlap priorities. Test a reliable rerun and an unknown episode. Confirm show/season/episode/description publication and start tolerance.

Large-load: import five million rows while owner/browse requests continue; check finite memory, temporary disk cleanup, renewable refresh lease and no partial published generation. Repeat on Windows/macOS once platform custody is implemented; cross-compilation alone does not establish physical retirement behavior.

# Not finished
- Client screen work, reservation event subscription/countdown rendering, final identity attach-point integration and new capability-document integration belong to the other lanes.
- Windows physical source lock and shared artifact/decoder retirement adapters remain unavailable; no Windows runtime acceptance is claimed.
- Active capture-leader cancellation does not hand its running process to another profile seamlessly; normal completed shared ownership is implemented.
- Capture follows its accepted window; guide extensions during an active capture are not dynamically adopted.
- A standalone XMLTV-only source creation flow and catch-up playback are not added. GuideSourceID can reuse an existing configured source's guide URI; manual uploaded-guide reuse across sources is not implemented.
- Real tuners/providers, device codecs, confinement-backed full capture/recovery and cross-platform physical retirement need the external checks above. The five-million-row load test was not run; a 10,002-row fixture proves the spill/checkpoint path, not production-scale performance.
- Manual JSON preview/save transport retains its 48 MiB body budget; remote guide ingestion is the route for hundreds-of-megabytes feeds. Playlist acquisition and private preview snapshots still hold the selected provider playlist as a bounded string; guide programmes/fetched guide bodies are streamed/spooled. Extremely large playlist-only payloads need a further streamed snapshot path if production load testing shows material memory/writer contention.

# Merge with the base

- administration.ts: kept Live TV selections, assignment owners/kinds and 100,000-row tuner budgets. Adopted the base's tolerant row/kind parsing and source deduplication. Count diagnostics use distinct channels; skipped rows never override the server's usage count. Removed trash stays removed.
- administration.test.ts: kept malformed-row/source coverage and the new assignment fields; added shared-channel readers and 201-row lists. Tuner overflow is 100,001; channel-map paging remains 200.
- foundation.go: registers both Live TV and identity routes. Every Live TV management contract declares Requires: ScopeAdmin and uses the renamed AdminScope: playback. Owner authorization remains intact.
- Followed the reviewed lanes' networking, identity, credentials, metadata, audio and client changes. Regenerated contracts/OpenAPI and schema inventories using apigen/schema-sync. Fixed keyed literals for the aliased filters and the foundation fixture's obsolete unconfigured-update expectation.
- Passed: Linux amd64/Windows amd64/macOS arm64 builds; package vet; targeted foundation, scopes, API-key, Live TV, metadata, tuner and DVR tests; 20 administration parser tests; web tsc; schema checks; diff hygiene. Grep found no conflict markers in 3,398 repository files.
- No git add, commit or snapshot. The owner must stage and commit the resolved merge files.

# Guide route follow-up after the committed merge

The guide-directory fixture mounted only hand-written live routes. Source publishing moved to the typed registry, so its POST /v1/admin/live-sources was absent from that fixture and returned 404. The fixture now supplies LiveChannels to FoundationRegistry, mounts the registry beside the remaining live routes, and sends application/json for JSON bodies. The previously failing neighbor/scope/unavailable-runtime test passes. Production keeps the single typed publishing route; no legacy route was restored.

The source-filter/logo fixture had the same missing registry and now uses the typed path with JSON content type. The concurrency fixture now exercises the remaining handler-local mutation gate through source enablement. A new registry-upload test verifies the actual two-slot HTTP upload lane, immediate overflow refusal without body reads, guide-read availability, and member rejection before JSON reads. The store refresh fixture now uses structurally malformed XML for whole-document refusal; odd well-formed programme rows remain intentionally tolerated.

Passed: all tests in livechannels, livechannels/dvr, livechannels/library, linearinput, linearbuffer, recordingaccess, recordingmedia and producerinput; all playback/playbackruntime tests matching Linear or Channel; all 53 related HTTP tests selected by area file or Live/Guide/Record/Linear/Channel/DVR/Tuner name; the separate TestEveryRouteDeclaresACredentialScope run; package vet; server go build ./...; diff hygiene. No other failures remain in that requested area. The confinement-backed HLS probe explicitly skips because decoder confinement is unavailable here; no hardware/provider acceptance is claimed.

One final snapshot uses the requested subject, Restore guide publishing and Live TV routes after combining with other work. The snapshot follows this report update; no tree changes follow it. No Git staging or commit is performed.
