# Audio engines

## Outcome

Implemented the remaining web and native engine changes in the two assigned worktrees. The server/music lane's supplied work is preserved.

Web resampling now carries filter history and fractional phase across compatible gapless tracks. Preparing a successor does not consume the outgoing tail; committing applies it. A finite scheduling deadline releases the current track if preparation or commit is late. Tests compare the complete PCM with resampling one continuous signal, including multiple nonaligned boundaries and cancellation.

MP3 synchronization searches bounded windows, tolerates repeated tags and substantial padding, and skips tag payloads. FLAC uses validated seek-table points or bounded, CRC-confirmed byte bisection. Deep MP3/ADTS/Ogg seeks without an exact index yield to ordinary playback before downloading the prefix. AAC configuration reads explicit and implicit SBR/PS, output rates, channel layouts and raw packet sizes; MP4 packet counts no longer confuse a shortened final presentation timestamp with a full decoded AAC packet.

Browser capabilities wait for short synthetic PCM alignment probes. Successful probes admit Firefox/Vorbis and other decoder paths; mismatches retain conversion or ordinary playback. Opus always uses each file's own pre-skip. A decoder that discards more than a file's intended leading trim yields to ordinary playback. Browser conformance reporting includes admitted codecs. Audiobook Media Session metadata includes author, narrator, book and current chapter; book chapter paging follows deep seeks. Safari's optional playback audio-session API and interrupted context state are handled.

Android normalization now processes decoded PCM, including amplification, a 20 ms ramp and saturation. The listening sink refuses encoded passthrough that would bypass gain. Crossfade decks retain the low-memory buffer policy; retiring the listening owner discards the music player so the next video gets its regular configuration. Apple AAC uses raw packet conversion instead of ExtAudioFile's inferred priming, then applies the measured frame window explicitly. Packet seeks include decoder preroll. Wrapped AudioFile handles are closed correctly. Both native owners immediately fall back after the first reported optional-renderer failure, retaining session and position, and enforce comfort rewind without JavaScript.

WEBP sidecar and embedded covers are accepted as original bytes through the existing image dependency and safety bounds. Targeted Go, TypeScript, generated-media and JVM checks passed. No Apple build, full native app build, physical playback or actual browser conformance run is claimed. The server AAC conversion target remains a dependency below.

Milestone cumulative snapshot 001 exists for both trees. Final cumulative snapshots are recorded at the end of this report after all checks. No commit, push or deployment was performed.

## Decisions I made

- Rewind 15 seconds after five minutes and 30 seconds after one hour, clamped at zero. Explicit seeks clear the pause clock. Native policy/interruption pauses also use the comfort rule. Apple uses `mach_continuous_time`; Android uses elapsed realtime, so suspension/sleep does not erase the elapsed interval. New server resume sessions retain the music lane's rule.
- Unknown normalization gain means unity. Album mode falls back to track gain. A known gain may attenuate without a peak, but cannot amplify without a measured peak. Known-peak amplification is limited by the peak and by a 32× ceiling on every engine. Normalization remains off by default. Analysis availability can still differ between tracks; no loudness is guessed.
- Gapless continuation requires equal source rate/channels and ordinary speed. Crossfades and stretched playback keep independent filters. A successor that cannot commit before the outgoing buffer's scheduling deadline does not hold back the current audio.
- Browser decoder declarations use actual short decode/alignment evidence, not merely the existence of WebCodecs. Unsupported or mismatched configurations fall back. Vorbis is conservatively declared at the probe's 44.1 kHz/stereo limits; other rates convert or play ordinarily.
- For unindexed lossy web seeks beyond 30 seconds, ordinary playback is preferable to a prefix download or an approximate frame claim. Effects and gapless are unavailable for that fallback session. FLAC bisection uses at most 32 probes, each at most 1 MiB, and bounded CRC candidate checks; an unresolved deep point falls back rather than walking the prefix.
- The AAC decoder's raw output owns trim on Apple. Core Audio's packet clock is queried after applying the cookie, including SBR output-rate scaling. Other formats retain the ExtAudioFile path with length/priming consistency guards. An inconsistent decoder yields to ordinary playback.
- The portable fixture generator preserves the original corpus. It emits edit-list-only AAC, no-LAME MP3, Vorbis, mismatched-output-rate FLAC neighbours and a checksummed MP3 with multi-packet discard. A separate optional long-file generator is supplied. HE-AAC still requires a qualified encoder/media fixture.
- Shared packages were changed canonically in the server tree and synchronized by the prescribed native script. That sync also brings the supplied server base's existing music/listening and other package changes into the native copy; those copies were not hand-edited.
- Execution host was personal Ubuntu mini-1. Context sync could not write its read-only lock path; existing canonical guidance was read. Browser inventory was empty. Cached Kotlin/Android/Media3 artifacts allowed a narrow helper compilation/JVM test despite the unavailable device/build workflow.
- Primary API references checked: [Media3 sink processors](https://developer.android.com/reference/androidx/media3/exoplayer/audio/DefaultAudioSink.Builder), [renderer sink hook](https://developer.android.com/reference/androidx/media3/exoplayer/DefaultRenderersFactory), [Apple wrapped-file lifetime](https://developer.apple.com/documentation/audiotoolbox/extaudiofilewrapaudiofileid(_:_:_:)), [raw packet conversion](https://developer.apple.com/documentation/audiotoolbox/audioconverterfillcomplexbuffer(_:_:_:_:_:_:)). No comparison-product code was copied.

## Bugs

- B-10.1 — already fixed on supplied base: book completion retains position; preserved.
- B-10.2 — already fixed on supplied base: permanent artwork has no global cache cap; preserved.
- B-10.3 — already fixed on supplied base: shuffled play-next; preserved.
- B-10.4 — already fixed on supplied base: shuffled edit/order history; preserved.
- B-10.5 — already fixed on supplied base/fix log: lyric provider pause/cursor; preserved.
- B-10.6 — already fixed on supplied base: lyric identity checks; preserved.
- B-10.7 — already fixed on supplied base: timing-based lyric identity; preserved.
- B-10.8 — already fixed on supplied base: tolerant bounded LRC; preserved.
- B-10.9 — already fixed on supplied base: packet discard summation; added a generated multi-packet fixture.
- B-10.10 — fixed in code: supplied server profile qualification plus web SBR/PS clock and raw AAC packet counting; Apple packet decoder implements the raw window. Real HE-AAC conformance remains pending.
- B-10.11 — not fixed end to end: Apple decoding is ready for AAC/MP4, but the server has no AAC converted-target recipe/selection. Those commands are expressly outside this lane; exact handoff below.
- B-10.12 — already fixed on supplied base: durable audio facts requests; preserved.
- B-10.13 — already fixed on supplied base: interactive conversion priority/seek; preserved.
- B-10.14 — already fixed on supplied base: narrator/composer evidence; web system metadata now renders the supplied credits.
- B-10.15 — already fixed on supplied base: natural part order; preserved.
- B-10.16 — already fixed on supplied base: retained usable chapters; web book metadata can page through them.
- B-10.17 — already fixed for the programme's release/album identity rules on supplied base; preserved.
- B-10.18 — already fixed on supplied base: APE/RVA2/R128 gain evidence; preserved.
- B-10.19 — fixed for remaining WEBP support. Existing case/name/parent/front-cover fixes are preserved. Byte/dimension limits remain intentional.
- B-10.20 — fixed: shared filter history, phase and cumulative output rounding across compatible committed joins; cancellation/deadline regressions pass.
- B-10.21 — fixed in code: bounded real decoder probes admit Gecko/Vorbis when output agrees. Actual Firefox conformance is not executed here; unsupported configurations retain fallback.
- B-10.22 — fixed: Android PCM amplification, peak limits and ramp; isolated real-Media3 JVM test passed.
- B-10.23 — fixed in code: supplied server padding plus web raw AAC counts and Apple raw packet trimming/seek. Apple LC/HE-AAC/edit-list acceptance is still required.
- B-10.24 — already fixed on supplied base: audio bitrate excludes covers; preserved.
- B-10.25 — already fixed on supplied base: bounded mix candidates/genre projection; preserved.
- B-10.26 — already fixed on supplied base: missing-disc fallback; preserved.
- B-10.27 — fixed policy: unity when gain is absent, no unsafe boost without a peak, album-to-track fallback. Analysis coverage can still vary; equal perceived loudness cannot be promised without measurements.
- B-10.28 — not fully fixed: portable missing-class fixtures, runtime probes, multi-packet padding, resampled joins and generated deep-seek tests are added. Real HE-AAC fixture and device/browser matrix remain unqualified; long generation is supplied but not run here.
- B-10.29 — already fixed for the directed extensions on supplied base; preserved.
- B-10.30 — fixed: bounded repeated-tag/padding synchronization, regression passed; optional decode errors retain ordinary fallback.
- B-10.31 — not present as prohibited behavior under the programme: owner TXT lyric sidecars are deliberate; supplied rip-log exclusions preserved.
- B-10.32 — fixed: author/narrator/book/chapter Media Session projection, with book chapter paging for deep positions.
- B-10.33 — already fixed on supplied base: repeat Previous/order behavior; preserved.
- B-10.34 — fixed for engine-owned fallback: supplied shared foreground fallback retained; native owners now fall back after the first reported optional-renderer error. Ordinary-player conversion escalation remains with playback owners.
- B-17.17 — fixed: FLAC index/bisection; unindexed lossy deep seeks immediately use ordinary playback. Approximate MP3 indexing is not advertised as sample exact.
- B-17.18 — fixed in code: optional Safari playback session and interrupted-context handling; lock/silent-switch/call acceptance still needs iPhone/iPad Safari.
- B-17.19 — not established/fixed by runtime evidence: bare-Web-Audio media-key integration still needs browser acceptance. No speculative silent carrier was added.
- B-20.6 — already fixed network classification preserved; native fallback still excludes Android network failures.
- B-20.7 — music owner already receives native focus/remote intents; no video-intent redesign in this lane.
- B-20.19 — optional audio deep seeks use indexed/raw decoding or ordinary fallback. Video seek failure policy remains outside this lane.
- B-20.21 — fixed: music/crossfade players are discarded on owner retirement, and crossfade buffering respects low-memory limits. Following-video acceptance remains a device test.

Other document 17/20 findings concern video, subtitles, quality, display or player screens and remain with their named lanes.

## API changes

No public route added, removed or reshaped. No registry or public contract change. Existing typed chapter, playback and media routes remain.

Internal changes only: optional `PreparedAudio.itemId` for same-album filter continuity; asynchronous demux `locate` for bounded FLAC seeking; codec `framesPerPacket` for raw AAC indexing. Canonical shared package changes were synchronized to native.

## Schema changes

None. Supplied music-lane baseline/schema changes were not altered.

## Owner or viewer settings added or wired

No new setting or screen. Existing normalization (off/track/album, default off), gapless (default on), crossfade (default zero), speed, pause intent and owner conversion/bitrate limits remain. Android now amplifies when measured gain and peak permit it. Missing-data behavior is the policy above. Audiobook comfort uses fixed server-matched listening rules; no extra switch.

## Needed from others

- **Server conversion/playback, B-10.11:** extend `server/internal/playback/audio_plan_v2.go`'s FLAC/Opus target selection with AAC-LC/MP4 for `findCap(audioDecode, "aac", "mp4")` when a bitrate limit excludes lossless and Opus is unavailable. Carry rate/channels/exact output frames/bitrate through `AudioConversion`, `server/internal/httpapi/audio_media.go`, `decoder.AnalysisSpec` and its closed recipe validation, `server/internal/decoder/analysis.go`, and `server/internal/subtitlevideo/audio_convert.go`. The latter currently allows only `audio_flac` and `audio_opus`. Preserve interactive priority, admitted source fencing, source-rate priming and efficient input seeking. Encode only the already-trimmed exact output interval. Respect the viewer's limit; do not silently increase it to an encoder minimum.
- **AAC target transport:** the Apple raw AAC path can consume AAC/MP4 using the output container's packet-table priming and the plan's exact presentation length. Qualify a progressively readable MP4 target with Core Audio, or serve a seekable completed target with accurate length/index. Do not pretend a nonseekable/truncated MP4 is a qualified streaming target. Do not reuse the original file's trim as the new encoder's priming. The capability already declares AAC/MP4; no duplicate capability document is needed.
- **Native/video lane integration:** merge only the music portions of `PorticoPlaybackController.kt`: gain sink/factory, music edges, owner entry/retirement and low-memory crossfade control. Video behavior and codecs are not redesigned. The Apple changes are in the existing Objective-C Core Audio helper/owner, not a new video module.
- **Web-player integration:** merge the small `Player.tsx`/`player/options.ts` book wiring: enable existing chapter reads for book files, pass the list to Media Session, and page chapters to the book's current position. Other changes stay inside audio bridges/modules/tests.
- **Metadata/UI:** Media Session uses supplied book-file local author/narrator evidence. If online-provider credits are projected separately, expose the selected credits in the typed book-file detail and render them through the same helper. Vega's unsupported own-engine effects remain a client-setting parity task; this lane adds no Vega decoder.
- **Documentation:** update Playback Protocol §12/§18.1/§18.8 and native audio descriptions for raw AAC ownership, runtime browser qualification, normalization absence/peak policy, ordinary unindexed seek fallback and the comfort rule. B-10.11's target stays open until the server recipe exists.

## Test notes for other environments

Executed here, as limited checks:

- Web and canonical client-core `tsc --noEmit`; native `apps/native/node_modules/.bin/tsc --noEmit -p apps/native/tsconfig.json`.
- Individual web files: audio-engine, audio-track, audio-demux, audio-capabilities, audio-aac-config, audio-seek-fixtures, audio-codec-fixtures, audio-listening-metadata, audio-render, audio-converted. They include exact PCM joins/cancellation, MP3 search bounds, high-word FLAC seek points, real generated FLAC bisection versus prefix decode, AAC final-packet padding, portable codec cases and stored probe PCM versus a qualified FFmpeg decode.
- Canonical `audio-render-v2.test.ts`; native `native-listening-v1.test.ts`.
- `GOFLAGS=-p=2 go test ./internal/localmetadata -run 'Test(WebPCover|LocalArtworkRejects)' -count=1`; targeted `go vet` for localmetadata/storage. WEBP original bytes and MIME are checked.
- Isolated Kotlin gain helper/test compiled with cached Kotlin 2.1.20, Android 36 headers and actual Media3 1.10.1 classes. JVM execution with Kotlin stdlib/Guava passed amplification, ramp, frame preservation and saturation. No Gradle invocation or native application build.
- Both trees' `git diff --check`; prescribed shared-package synchronization. Browser inventory was empty, so browser qualification is a runtime implementation with fixture/unit evidence, not an observed browser result.

Cold acceptance:

1. Generate extras with `python3 fixtures/audio-gapless/generate-portable.py --out /tmp/portico-audio-extra`. Keep the original corpus. Optional long file: add `--long-hours 2` on the qualification host. Obtain a real HE-AAC/SBR and HE-AACv2/PS fixture from a qualified encoder; its decoded rate/trim must be measured, not inferred from a filename.
2. **Apple (iPhone/iPad/Apple TV):** compile the changed Objective-C helper with the actual SDK. Run the Debug launch harness with `PORTICO_AUDIO_CONFORMANCE=<fixture directory>` and `PORTICO_AUDIO_CONFORMANCE_OUT=<temporary JSON path>`, first against the original corpus, then extras. Declared fixtures must report exact frames, best lag zero and pass tolerances. Vorbis is intentionally not declared by this Apple helper. Repeat LC/HE-AAC/edit-list seeks at the first sample, inside a packet, near end and deep in a long book. Compare with the same raw frame window of a full qualified decode. A clipped opening syllable, extra padded tail, phase shift or wrong frame count fails.
3. **Apple listening:** play contiguous tracks with a continuous tone and listen for a click/drop/overlap at every join. Pause a book near 120 seconds, suspend JavaScript while native threads continue, wait 5 minutes/1 hour and use a headset/lock-screen Play: expected starts approximately 105/90 seconds. Repeat with a timer pause and an explicit paused seek (the seek must win). New sessions must not rewind twice. Check speaker/Bluetooth/AirPlay and route changes. Try a file whose optional decoder fails: same session/position must continue ordinarily with a quiet notice.
4. **Android/Android TV/Fire TV:** compile the music controller/owner changes against Media3 1.10.1. Run `apps/native/test/audio-gain-tests.kt` with the gain source, Media3 common/exoplayer dependencies, Android headers, Kotlin stdlib and Guava on the classpath; JVM main is `tv.getportico.portico.native.Audio_gain_testsKt`. Its assertions must complete with exit zero. On devices play a quiet track carrying +6 dB and a 0.25 peak: level should rise without cracking. Try missing gain, missing peak and peak-limited positive gain. Test speaker, Bluetooth and HDMI; normalization must never bypass PCM. Repeat pause/seek/fallback tests from step 3. AAC/HE-AAC joins rely on Media3's extractor/decoder trim and require the real fixture matrix. After a music crossfade, start video: regular buffer policy/output must be restored.
5. **Browsers:** run the existing Vite conformance page on a private qualification host with `AUDIO_REPORT_DIR` set, then open `/audio-conformance.html` in current regular-profile Chrome, Firefox and Safari. `qualifiedCodecs` reports actual admissions; unsupported probes must fall back. Test Vorbis at the declared rate, MP3 with repeated tags/large padding, and unknown-priming MP3. A 44.1 kHz album on 48 kHz output must equal the continuous reference at joins. Deep FLAC must start promptly and match the full decode; deep MP3 must use ordinary playback without a prefix download. Check Media Session book credits and chapters, including >100 chapters and backward/deep seeks. On iPhone/iPad Safari lock the screen, toggle silent mode, interrupt with a call and resume; audio and hardware keys must behave consistently.
6. **Artwork:** scan an admitted book/music file with embedded WEBP and a disc album with mixed-case `Cover.WEBP` in its parent. Confirm unchanged original bytes/MIME and actual display on every first-party client. Oversized dimensions/bytes and changed/root-escaping files must still be rejected.
7. **After server AAC target integration:** set a mobile limit below a FLAC/ALAC file's bitrate. Apple must receive a qualified compressed render plan, retain normalization/gapless, start and deep-seek promptly, and remain within the chosen bitrate. A renderer-unavailable plan or ordinary fallback means B-10.11 has not passed.

## Not finished

B-10.11's server AAC target is not implemented because server conversion command construction/selection belongs to other lanes. The native AAC decoder support and exact handoff are complete, but Apple limited rendering remains unavailable until that dependency lands.

Apple SDK compilation and physical LC/HE-AAC/gapless/seek/output qualification, full Android controller/application compilation and physical playback, actual Firefox/Safari/Chrome probe and Media Session behavior, Safari background/media-key acceptance (B-17.19), and a real HE-AAC fixture remain outstanding. The long-file generator is written but the multi-hour device fixture was not generated/run here. Unindexed lossy deep web seeks intentionally use ordinary playback and lose optional effects for that session. No sample-exact own-engine claim is made for every unsupported codec/platform.

Final snapshots: `snapshots/audio-engines/002` (“Keep music and audiobook engines accurate”) and `snapshots/audio-engines-native/002` (“Keep native listening continuous and resilient”). Both are cumulative against their recorded base. Final web/native TypeScript checks, original fixture demux regression, shared-package identity check, cover regression and whitespace checks passed before these snapshots. No worktree files were changed afterward. Only this programme report was updated to record the final snapshot results.
