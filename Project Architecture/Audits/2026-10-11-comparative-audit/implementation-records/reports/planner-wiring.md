# Outcome

The ordinary planner now selects the richer converter outputs and persists a complete executable target with the session's media generation. It prefers original playback, remux, audio-only conversion, then video conversion. It keeps supported HEVC, bit depth and HDR range, and compares audio fidelity across transports before selecting a lossy or smaller layout. Qualified outputs include H.264, HEVC 8/10-bit, available AV1, AAC, AC-3, E-AC-3, Opus and FLAC, plus supported copied/passthrough audio. Missing encoders, decoder limits, transport limits and explicit ceilings are enforced before launch.

Original codec-changing playback uses quality mode; fixed qualities use codec-specific rates. Original subtitles beside converted streams participate in engine selection, including Automatic choosing mpv before burn-in where mpv can draw the track. Real H.264 and HEVC fragmented-MP4 tests cover planner admission, persisted targets, playlists, stable init, independently decoded media, a 12-second producer restart and target retention across generation replacement.

Live TV's independent H.264/AAC classification is removed. Its admitted engine profile uses the ordinary planner and selected target, restricted to the current MPEG-TS publisher. Prepared/download menus share the decoder inventory; optimized downloads also use the device capability document and ordinary offline planner.

Read the programme/lane briefs, all five prerequisite reports, relevant bug-register entries, and canonical project/standards/verification guidance. Linux, Windows amd64 and macOS arm64 builds, focused Go tests/vet, client-core typechecking and 19 selected TypeScript tests pass. No commits, pushes or deployments.

Compiling snapshots: `001` (Pass selected picture and sound formats to the converter), `002` (Choose compatible conversion targets for playback and live channels). Final cumulative snapshot: `_program/snapshots/planner-wiring/003`, taken after this report is finalized, subject “Wire conversion targets and verify playback planning”. No worktree changes follow it.

# Decisions I made

- Keep the existing capability document, session routes and generation-fenced target store. Add only an optional audio sample-depth ceiling where a necessary decoder fact was missing.
- Prefer a lossless output codec where accepted, including for lossy sources, to avoid another lossy encode. Track full signal preservation separately: downmixing, reduced rate/depth and floating PCM conversion are not called lossless preservation. Keep known FLAC rates through 192 kHz and supported multichannel layouts rather than imposing stereo/48 kHz.
- Preserve source depth instead of inflating 8-bit sources. Keep HDR10/HLG for accepted engine/display combinations, including an engine explicitly able to tone-map itself. Otherwise use qualified server tone mapping. No approximate-colour fallback for unreshapeable Dolby Vision profile 5.
- Copied native Dolby Vision during audio-only conversion keeps its sample entry and metadata. Compatible-base fallback strips DV metadata only with qualified filters. A profile 8 SDR base stays SDR.
- Original conversion uses quality 20 for H.264/HEVC, 28 for AV1. Existing published ceilings govern fixed rates: H.264 1/1.5/3/8/12/20 Mbps for 360/480/720/1080/1440/2160p; HEVC 70%, AV1 60%. This reconciles converter defaults with the published planner ladder.
- Record the actual qualified encoder/stages. Hardware quality modes unable to enforce an explicit cap use software. Qualified GPU filters with a software encoder request existing GPU grants; sandbox authority is unchanged.
- Bypass the experimental finite normalizer for explicit rich targets so it cannot silently rewrite them to H.264/AAC. Existing finite copy/remux behavior remains.
- Keep untouched original downloads offered. Optimized choices must be producible and playable offline by a declared engine.
- Work stayed on personal Ubuntu mini-1 (`ehler-homelab-mini-1`) in the named worktree. Context-sync could not write its read-only state lock; existing canonical guidance was read directly. No helpers or subagents.

# Bugs

- B-02.14: Fixed for ordinary playback. fMP4-only device profiles receive real H.264 and HEVC conversions. Live publication remains TS-only, below.
- B-03.16: Producer declarations were already fixed by conversion-outputs. Fixed the planner join: persisted target, encoder, advertised codec/sample entry and range agree, including native DV copy versus converted HEVC. Real producer checks verify H.264/HEVC output.
- B-11.15: Copy admission was already fixed by livetv. Replaced remaining independent live conversion classification with the ordinary planner; declared compatible TS tuples copy and incompatible tracks convert to supported targets. Android/Fire TV acceptance remains external.
- B-11.16: Already fixed by livetv and preserved. Unknown live rates do not force conversion without an applicable explicit ceiling; tests cover this. An explicit ceiling is not satisfied by pretending unknown facts fit.
- B-14.4: Recipe support was already broadened upstream. Fixed remaining publication gaps: prepared profiles/download rungs use actual producer availability; optimized download choices respect device geometry/codecs. Tests cover a 480p decoder and missing H.264 encoder. Rich prepared recipes remain a separate handoff.

# API changes

- `GET /v1/items/{itemId}/download-options` moved from handwritten mux to typed apikit registry, with `ViewerItem`, `ScopeRead` and browsing lane. Successful response shape is unchanged; errors use the registry contract. Removed duplicate legacy registration, scope/lane entries and authored OpenAPI path/schemas. Optimized options omit unplayable/unproducible recipes; existing prepared versions can remain usable without a current encoder.
- Capability documents add optional `engines.<engine>.audio[].maxBitDepth`, integer 0–64. Zero/omitted means unstated. Server and client-core validate it; Automatic selection considers it before reducing samples. Client publishers need qualified values.
- Audio rendition diagnostics optionally include `sampleRate`, `bitDepth`, `channelLayout`; client-core parses them. Retired the approximate-DV reason from server contracts/client-core and its unused catalogue label. Web prompt/helper removal remains a client handoff.
- Generated OpenAPI/TypeScript contracts refreshed with `GOFLAGS=-p=2 go run ./cmd/apigen`. Session routes/successful shapes unchanged. No auth bypass.

# Schema changes

None. Existing conversion-target storage and generation fencing carry normalized selections, including fallback and subtitle-triggered generation replacement. No schema synchronization needed.

# Owner or viewer settings added or wired

- Quality: existing named choice, default **Original**. Source geometry/quality mode when conversion is needed; fixed choices enforce current ceilings/codec rates. **Automatic** remains explicit and uses existing re-plan. Existing client settings/player screens own presentation.
- Player: existing **Automatic**, **Native**, **mpv**, default **Automatic**. Ordinary playback prefers mpv before avoidable video conversion, signal-losing audio or subtitle burn-in. Live/offline player integration remains below.
- Existing owner transcoding enablement, hardware backend/device and software settings remain enforced. Audio/HDR choices, decoder geometry/rate/depth limits and subtitle deliveries feed executable targets. No new setting or screen.

# Needed from others

- **Live publisher:** `linearbuffer` rejects `EXT-X-MAP` and publishes TS only. Add bounded fMP4 init/media publication before FLAC/Opus and other fMP4-only live tuples become selectable. Shared planning deliberately admits only published transports.
- **Live presentation/client:** work currently carries the admitted native engine profile. Probe-time Automatic/mpv switching and returning that engine require channel presentation/player integration. Session-route shapes were not changed. Live target selection is shared; complete two-engine selection is not yet implemented.
- **Prepared/download owner:** persisted recipes remain H.264 SDR/AAC. Rich offline HEVC/HDR/multichannel variants need recipe identities, target storage and packaging. Existing recipes now use common inventory/capability filtering; this does not create rich recipes.
- **Clients:** publish qualified per-codec maxBitDepth, complete offline original native/mpv choice, remove obsolete web approximate-DV warning/helper, and render optional audio diagnostics where useful.
- **Source facts:** assets.StreamDetail does not capture mastering-display/MaxCLL. Converter metadata fields exist, but planner cannot supply uncaptured facts. HDR transfer/range/depth preservation does not claim preservation of every mastering field.
- **Documents:** describe common planning/inventory and actual quality rates; retire universal H.264 SDR/stereo AAC assumptions. Preserve distinctions between lossless codec/signal, current TS live and prepared recipes, and qualified DV reshape/base fallback.

# Test notes for other environments

Verified here, with limited touched-package selections:

- Final `GOFLAGS=-p=2 go build ./...` from server/ passes; Windows amd64 and Darwin arm64 equivalents pass.
- Vet passes for playback, playbackv1, playbackruntime, decoder, downloads, preparedmedia, httpapi and cmd/server.
- Focused Go tests pass: 108 device/source cases, every registered output's executable target, codec ladders/Original, 96 kHz/24-bit/7.1 preservation, TrueHD passthrough, DV refusal/reshape/copy/SDR base, original subtitles/Automatic, sample-depth limits, all-vendor arguments, live tuples and download filtering.
- Both real `TestPlannerFragmentedConversionsReachTheRealProducer` codec subtests run and pass. Generated 19-second MPEG-2/AAC MKV goes through capability publication/normal session creation, stored target, readiness/playlists, stable init, independent decode, absolute 12-second restart timestamps, unchanged prior fragments and fallback re-plan target retention.
- HTTP selection `Test(PlaybackV1.*|Subtitle.*|DownloadTransferSupportsRangeAndValidators|EveryRouteDeclaresACredentialScope|RestrictionMatrix.*)` passes; selected playbackv1 subtitle checks also pass.
- Client-core `npx tsc --noEmit` and `node --experimental-strip-types --test test/client-profile.test.ts test/delivery.test.ts` pass (19 tests). Apigen completed; diff check clean.
- Broader HTTP OpenAPI coverage fails on unchanged `/receiver/cast/` prefix missing from authored openapi.yaml. No casting files changed. No full Go/npm suites run.

Cold software reproduction, from server/ with the qualified development FFmpeg/ffprobe pair installed:

```
PORTICO_REQUIRE_QUALIFIED_FFMPEG=1 GOFLAGS=-p=2 go test ./internal/playback -run '^TestPlannerFragmentedConversionsReachTheRealProducer$' -count=1 -v
GOFLAGS=-p=2 go test ./internal/playback -run 'Test(PlannerOutputDeviceSourceMatrix|EveryRegisteredOutputHasAnExecutablePlannerTarget|CodecQualityLaddersAndOriginalQualityMode|LosslessRateAndPassthroughSurvivePictureConversion|SubtitleOriginalsBesideConvertedStreamsAvoidBurn|DolbyVision.*|AudioSampleDepth.*)' -count=1
GOFLAGS=-p=2 go test ./internal/decoder -run '^TestSelectedHEVCTargetArgumentsForEveryVendor$' -count=1 -v
```

Both real codec subtests must run/pass; an HEVC skip is not acceptance. Matrix tests validate declared profiles, not physical devices. Firefox uses the Chromium fixture as a declared browser-class variant; verify real Firefox declarations separately. Vendor argument tests exercise syntax on any host without qualifying GPUs.

GPU common procedure: use private test library, packaged qualified FFmpeg pair and production media executor. Supply a 19-second clip with keyframes every four seconds, HEVC Main8 SDR/Main10 HDR10/Main10 HLG, interlaced MPEG-2 and 96 kHz/24-bit 7.1 lossless audio. Publish HEVC10/fMP4/FLAC8 and SDR-only engine profiles. Select backend/device using existing owner fields; verify codec/depth/quality probes. Play Original then 1080p; seek 0→12→6→18 seconds; re-plan to 720p and back. Capture playback-info/stages, master, init and fragments. Decode each init+fragment independently and inspect codec, pixel format, transfer/primaries, channels/rate/depth, level and timestamps. Pass: target agrees, quality/caps hold, absolute clocks and independently decodable six-second cuts, stable init within generation. Black/green/purple frames, lost channels, reset timestamps, missing IDRs, stale init grants or silent AAC stereo fallback fail.

- **NVENC, Linux/Windows:** supported HEVC/Main10 NVIDIA GPU/current driver. Check hevc_nvenc, uncapped -cq and -forced-idr 1. Capped quality uses software when hardware cannot enforce cap. Qualify CUDA scaling separately from tone mapping. Repeat Linux under bubblewrap with local/pinned remote input; no expanded device/source grants.
- **QSV, Linux/Windows:** older/current Intel generations, correct driver/render device. Check hevc_qsv, -global_quality, -forced_idr 1, P010 and VPP. Interlaced inputs and unsupported Main10/quality must fall back accurately, without system-frame filters consuming hardware surfaces.
- **VAAPI, Linux:** explicit Intel/AMD render nodes under production sandbox. Check hevc_vaapi, -qp, P010, HDR metadata and IDRs. Unreadable device and HLG without mastering metadata must yield correct software fallback.
- **VideoToolbox, macOS:** Apple Silicon and Intel where available. Check hevc_videotoolbox, independently qualified -q:v, Main10 tag/profile and native HDR/HLG display. Fail encoder after published init: replacement generation/grant as needed, selected codec/range/audio retained; old grant cannot read new init.
- **AMF, Windows:** supported HEVC8/10 AMD GPU/current driver. Check hevc_amf, -qp_i, -forced_idr 1, upload and restart segments. Exercise subtitles/fonts/temp paths with spaces, commas, apostrophes and drive colons. Failed qualification must not invent hardware availability.

Also run qualified Vulkan/libplacebo DV reshape or OpenCL tone-map with software encoding: existing GPU grants must be requested for hardware stages, absent for CPU-only graphs. Corrupt source failing both hardware/software must not count as successful software recovery or charge the hardware failure circuit.

Devices/receivers: Apple TV 4K native/mpv, Apple TV HD, iPhone, Android with/without HEVC hardware, Fire TV, Chromecast-class hardware, Chrome/Safari/Firefox and Vega. Exercise the nine matrix source classes: H.264 SDR, HEVC10 HDR10/TrueHD7.1, DV5/7/8, AV1, interlaced MPEG-2/AC-3, VC-1/DTS-HD, HLG. Verify direct/remux first, supported lossless/multichannel before stereo, Automatic mpv, HDR-preserving audio-only conversion and honest DV5 refusal. Pair passthrough with declared receiver limits and inspect actual format/layout; muxability alone is insufficient.

Subtitles: styled ASS and PGS/VobSub originals remain side resources even during picture conversion on drawing engines. Non-drawing native plus drawing mpv should choose mpv in Automatic. Force Native for high-quality burn-in, embedded fonts, offsets/repeated seeks and correct HDR picture range. Repeat live H.264/HEVC TS/multichannel Dolby and device-limited download menus. fMP4-only live should currently refuse rather than promise unsupported publication.

Windows/macOS were cross-builds only. GPU/browser/device/receiver/tuner runtime and genuine confinement acceptance did not run. Host denies unprivileged user namespaces; real producer used existing baseline executor. No security/process bounds weakened.

# Not finished

Planner/converter wiring, ordinary playback verification, shared live target selection and existing download filtering are complete within this lane. Remaining cross-lane work: live fMP4 publisher/two-engine presentation, rich prepared recipes/packaging, offline original player integration, qualified audio-depth declarations, web DV prompt removal and mastering-metadata capture. Experimental finite pipeline remains a separate legacy normalizer, bypassed for explicit rich targets. Real hardware/device/receiver/sandbox acceptance and unrelated casting OpenAPI coverage remain unverified/unfixed. No other implementation work is pending in this worktree.

# Merge with the base

Read networking, identity, api-platform, providers-matching, music-audiobooks, web-app, web-player and naming reports. Context refresh still cannot write its read-only lock. This follow-up supersedes the earlier final-snapshot freeze; no snapshot, staging or commit is performed for the merge.

- `server/api/downloads.openapi.yaml`: followed the base's deletion. Download contracts now come entirely from typed routes; no duplicate manual document remains.
- `downloads/options.go`: kept device-document/offline-planner filtering, source availability reasons and retained-copy handling. Adapted new-work checks to the base's `preparedmedia.Producible` API.
- `httpapi/downloads_routes.go`: retained the base's complete typed route migration, grant renewal/bundles/original subtitles and explicit credential scopes. Kept one download-options route with `ViewerItem`, `{itemId}`, page-sized cost and Read scope; removed my duplicate from download_requests.go and restored its browsing entry in route_lanes.go. Base error mapping is reused.
- `playbackv1/options.go`: use the base's canonical `subtitles.RenderingFormat` helper to determine original styled/image delivery beside converted streams, preserving my Automatic engine decision.
- `playbackv1/sessions.go`: same original-delivery helper during start/re-plan, keeping base subtitle selection/memory and presentation behavior.
- `preparedmedia/types.go`: retained v3 bounded-fragment identities, older recipe lookup and shared `recipeFor` translation. Consolidated producer checks in the base's `decoder.PreparedProducible`, adding my qualified converter-inventory check; adapted Submit and removed duplicate CanPrepare/ProfileProducible APIs.

Validation complete: apigen and schema-sync passed (including the schema test). Final Linux, Windows amd64 and Darwin arm64 `GOFLAGS=-p=2 go build ./...` pass; vet passes on downloads, preparedmedia, playbackv1, decoder and httpapi. Focused tests pass for download menus/bundles/recipes, prepared submission/source/publication, subtitle originals/re-plans, quality/options, vendor arguments, real planner H.264/HEVC fMP4 output, production router construction, scopes, lane coverage and restriction checks. Web `npx --no-install tsc --noEmit` passes. Whole-tree marker scan and diff check are clean. The optional broader `TestOpenAPICoversRegisteredRoutes` still fails only on the unchanged `/receiver/cast/` authored-document gap recorded above; casting was not changed. The lane-table failure found during validation was fixed and its test rerun successfully. Working files are resolved; Git index intentionally remains unmerged until Justin stages the merge. No staging, commit or snapshot was attempted.
