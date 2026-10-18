# V07 — Verification of live TV, DVR, music and audiobooks

## Summary

All 27 Tier 1 entries and all 23 Tier 2 entries were checked. Tier 1: **6 Confirmed, 19 Confirmed, narrower, 2 Unsettled — needs runtime**. Tier 2: **17 Confirmed, 6 Confirmed, narrower**. No complete finding was refuted, although several categorical statements and implied impacts were disproved. The 19 Low entries are copied below without re-checking.

The most consequential source confirmations are the audiobook resume reset (B-10.1), durable local-artwork budget (B-10.2), guide-triggered live retunes (B-11.1), harmless DOCTYPE rejection (B-11.2), and missing recording continuation after an input interruption (B-11.11). HDHomeRun guide support exists in the server API; the first-party setup and extras integration are incomplete (B-11.8/9). One-time recordings survive corrections with stable provider IDs; interval-only feeds remain vulnerable (B-11.13). Direct decoded-audio errors bypass the shared route-escalation mechanism (B-10.34).

Several first-pass “bugs” implement explicit choices: padded conflict losers fail rather than record partially, each profile-owned recording has an allocation, restart recovery preserves partials, and completion describes the padded capture window. Strict import/manifest rejection also has deliberate integrity/network-boundary reasons. I lowered B-11.3/4/5/7 from High to Medium because scope is feed-dependent, prior valid guides survive refresh failure, and some represent compatibility/policy work rather than broad data loss. HLS variant behavior (B-11.14) and real refresh-lease overruns (B-11.33) still require targeted runtime checks; no frequency, performance, hardware or playback result is claimed here.

All code paths below are relative to /home/justin/projects/Portico. Document shorthand: **Playback Protocol** = portico-internal/Project Architecture/Specs/Spec — Playback Protocol v1.md; **Channels spec** = portico-internal/Project Architecture/Specs/Spec — Channels and Guide.md; **live-tv-and-dvr contract** = portico-internal/Product Contracts/live-tv-and-dvr/contract.md; **architecture-amendments.md** and **decisions.md** = portico-internal/Product Contracts/; **Backlog.md** = portico-internal/Project Architecture/Backlog.md. S/M/L mean hours/days/a week or more. Severity measures residual user impact even for deliberate policies; a trade-off row is not a confirmed defect.

## Verdict table

| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
| --- | --- | --- | --- | --- | --- | --- | --- |
| B-10.1 | An audiobook file is marked finished, and its resume point reset to zero, at 95% | Confirmed by reading / High for audiobooks | Confirmed | Defect | High | S | No |
| B-10.2 | All local artwork shares one 128 MiB folder; after that, covers are silently dropped | Confirmed by reading (I found no pruning of this folder other than the owner's maintenance action) / High; it contradicts the 5–10 million item goal | Confirmed, narrower | Defect | High | M | No |
| B-10.8 | One unexpected line makes a whole LRC file invalid | Confirmed by reading; how often it bites is Probable / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-10.10 | HE-AAC is planned as plain "AAC" and will not decode on the web engine | Probable / Medium (with B-10.34 it is a playback failure, not a quality loss) | Confirmed, narrower | Defect | Medium | M | No |
| B-10.12 | Long files cannot be measured in the 5-second play-time budget, and each attempt is thrown away | Probable (decode speed is an estimate) / Medium on first import, Low afterwards | Confirmed, narrower | Defect | Medium | M | No |
| B-10.15 | Audiobook parts without a track tag or a leading number play in catalogue order, not file-name order | Probable (I did not confirm the scan's visiting order) / Medium | Confirmed | Defect | Medium | M | No |
| B-10.19 | Covers with other names, capitalisation, size or location are missed | Confirmed by reading for names and limits; Probable for case sensitivity (I saw no case folding in the read path) / Medium | Confirmed, narrower | Missing feature mislabelled as a bug | Medium | M | No |
| B-10.28 | The gapless fixtures leave whole classes of file untested (design risk) | Not stated / Medium as a process gap | Confirmed, narrower | Design risk | Medium | M | No |
| B-10.29 | Files with unlisted extensions vanish without a word | Confirmed by reading for the gate; Probable for the lack of notice / Medium | Confirmed | Missing feature mislabelled as a bug | Medium | M | No |
| B-10.34 | When the device's engine cannot play a direct plan, playback fails instead of falling back | Probable (I did not trace what fail leads to in every client) / Medium; it turns each decode gap (B-10.10, B-10.30, damaged files) into "this song will not play" | Confirmed, narrower | Defect | Medium | M | No |
| B-11.1 | Every guide refresh retunes every live viewer of that source | Confirmed by reading (the retune) / High | Confirmed | Defect | High | M | No |
| B-11.2 | A standard XMLTV file with a DOCTYPE line is rejected | Confirmed by reading / High | Confirmed | Defect | High | S | No |
| B-11.3 | Valid XMLTV time forms, a missing stop, or a non-UTF-8 encoding reject the whole guide | Confirmed by reading / High | Confirmed | Missing feature mislabelled as a bug | Medium | M | No |
| B-11.4 | One odd programme rejects the whole guide | Confirmed by reading / High for real-world guides | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-11.5 | Size limits fail the whole source | Confirmed by reading / High for IPTV line-ups, low for an antenna | Confirmed, narrower | Design risk | Medium | M | No |
| B-11.7 | One imperfect playlist entry rejects the whole playlist | Confirmed by reading / High | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-11.8 | HDHomeRun sources cannot have a guide, so they cannot record | Confirmed by reading / High (the most common home tuner gets channel names only) | Confirmed, narrower | Unfinished feature | High | M | No |
| B-11.10 | A few minutes of padding overlap fails an entire recording | Confirmed by reading / High where a limit is set (every HDHomeRun using the discovered count, every IPTV source with an owner limit) | Confirmed, narrower | Deliberate trade-off | High | M | No |
| B-11.11 | The first upstream error ends a recording for good | Confirmed by reading / High | Confirmed, narrower | Unfinished feature | High | L | No |
| B-11.13 | A one-time recording is lost if the provider corrects the start time | Confirmed by reading / High | Confirmed, narrower | Deliberate trade-off | High | M | No |
| B-11.14 | HLS sources with several quality variants: recordings take all of them, live takes the first listed | Probable (it rests on FFmpeg's stream mapping for HLS, which I did not run) / High for recordings of such channels (several times the disk and bandwidth), Medium for live quality | Unsettled — needs runtime | Design risk | High | M | No |
| B-11.15 | On Android and Fire TV, live channels are probably always converted | Probable — needs a runtime check on an Android device / Medium to High on those platforms (CPU per viewer, large segments, short timeshift) | Confirmed, narrower | Defect | Medium | S | No |
| B-11.16 | An unknown frame rate forces a conversion | Speculative — needs a runtime check with real sources / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-11.18 | The segment writer has no tolerance for a slow database | Speculative — needs a load test / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-11.19 | The HLS gateway refuses playlists with common extra tags | Probable for the rule; which providers trigger it needs real playlists / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-11.20 | "Complete" is judged against the padded window with a four-second tolerance | Probable — needs timing on real sources / Low to Medium (trust in the label) | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-11.33 | A slow import can outlive its refresh lease | Speculative — needs a timing test with a guide near the limits / Low to Medium | Unsettled — needs runtime | Design risk | Medium | S | No |
| B-10.3 | "Play next" on a shuffled queue adds to the very end | Confirmed by reading / Medium (a visible everyday surprise) | Confirmed | Deliberate trade-off | Medium | S | No |
| B-10.4 | Removing or moving one entry in a shuffled queue reshuffles everything and forgets what was played | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-10.5 | A library-wide lyrics run that is rate-limited once races through the rest of the library as "missing" | Confirmed by reading / Medium (the feature quietly does nothing for large libraries; no data is damaged and a rerun retries) | Confirmed, narrower | Defect | Medium | S | No |
| B-10.6 | The bulk lyrics run publishes the first search hit without checking it is the same song | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-10.7 | Retagging a file hides its lyrics, and the bulk run will not fetch them again | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-10.11 | A bitrate limit on an Apple device turns the audio plan off | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-10.13 | A listener's conversion runs as background work and restarts from the file's beginning on every seek | Confirmed by reading / Low to Medium (weak boxes, long files) | Confirmed | Defect | Medium | M | Related only: web MP3/FLAC seek tables (Backlog.md:67); not this conversion path. |
| B-10.14 | Narrators are read only from a narrator tag | Confirmed by reading / Medium for audiobook libraries | Confirmed, narrower | Missing feature mislabelled as a bug | Medium | S | Related only: unmerged Audiobooks agent (Backlog.md); no explicit composer alias item. |
| B-10.16 | One bad chapter discards all chapters | Confirmed by reading / Low to Medium | Confirmed | Deliberate trade-off | Medium | S | No |
| B-10.17 | Common album layouts split into several albums | Confirmed by reading / Medium (the most common complaint about tag-keyed music servers) | Confirmed, narrower | Design risk | Medium | M | Related only: “Untagged albums in the main Albums grid”; these tagged split cases are not listed. |
| B-10.21 | In Firefox every lossy track is converted to FLAC | Confirmed by reading / Medium as a design risk | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-11.6 | Gzip guide files are not decompressed | Confirmed by reading / Medium (most public guide feeds are .gz) | Confirmed, narrower | Missing feature mislabelled as a bug | Medium | S | No |
| B-11.9 | Owner settings that are saved but do nothing | Confirmed by reading / Medium (misleading; and it hides the absence of the mapping feature in B-11.8) | Confirmed | Unfinished feature | Medium | M | No |
| B-11.12 | A server restart during a recording loses the rest of the programme | Confirmed by reading / Medium | Confirmed | Deliberate trade-off | Medium | L | No |
| B-11.17 | Live recovery stops after five failures and the counter does not reset | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-11.21 | Measuring storage stalls every running capture | Confirmed by reading (the locking); the size of the stall is a runtime question / Medium, latent | Confirmed | Design risk | Medium | M | No |
| B-11.22 | A full disk at start time fails the recording before retention can help | Confirmed by reading / Low to Medium | Confirmed | Defect | Medium | S | No |
| B-11.25 | An "all episodes" rule records every airing of every episode | Confirmed by reading / Medium | Confirmed | Missing feature mislabelled as a bug | Medium | M | No |
| B-11.26 | Two profiles recording the same programme use two tuners and store it twice | Confirmed by reading / Medium on a tuner-limited source | Confirmed | Deliberate trade-off | Medium | L | No |
| B-11.27 | The pre-emption countdown is not delivered | Confirmed by reading / Medium (the contract makes the countdown a condition of pre-emption) | Confirmed | Unfinished feature | Medium | M | No |
| B-11.29 | "Hide channel" is offered to owners but hides only for the owner's own profile | Confirmed by reading / Low to Medium | Confirmed | Defect | Low | S | No |
| B-11.30 | A programme the profile may not see can still be watched | Confirmed by reading / Medium for a parental-control promise; arguably intended, since ratings in guides are unreliable and channel lists exist | Confirmed | Defect | Medium | M | No |
| B-11.32 | Hourly refresh keeps up to a day of full guide copies | Confirmed by reading / Low to Medium (database size and backup time) | Confirmed | Deliberate trade-off | Low | S | No |

## Tier 1 — detailed verification

### B-10.1 An audiobook file is marked finished, and its resume point reset to zero, at 95%

- **Claim:** Audiobook completion resets resume at the shared threshold.
- **What I traced:** apps/portico-server/server/internal/catalog/personal_activity.go:27,45,57,66,73,112,141; apps/portico-server/server/internal/operations/preferences.go:178; apps/portico-server/server/cmd/server/main.go:447 wires PersonalProgress. The book path is not protected by the video-only credits rule. apps/portico-server/server/internal/playback/service.go:627 invokes the hook instead of the raw-position fallback; apps/portico-server/server/internal/playback/personal_progress.go:71 creates the per-book resume row.
- **Verdict:** **Confirmed** — At the default 95% threshold a canonical Part report becomes completed, clears progress and writes zero to book_resume. Paused history, noncanonical playback and explicit personal-state fences avoid the write; none is an audiobook completion guard. The generic threshold is documented in Playback Protocol §6 and architecture-amendments.md:109, but neither explicitly approves destroying a still-listening book position. Treat this as a media-specific resume defect, with the shared-threshold policy requiring reconciliation.
- **Class:** Defect.
- **Severity:** **High** — A listener stopping in the final 36 minutes of a twelve-hour file loses automatic resume; explicit seeking still works.
- **Fix direction:** Separate book completion from clearing its resume position; retain the last observed file position until true end/next part. Touch catalog/personal_activity.go, book resume readers and completion preferences. (**Size:** S).
- **Related:** B-02.8 shares the personal-progress routine but concerns rewatch classification; B-14.11 concerns offline multipart resume and is separate.
- **Backlog:** No.

### B-10.2 All local artwork shares one 128 MiB folder; after that, covers are silently dropped

- **Claim:** Local artwork intake stops at a shared 128 MiB budget.
- **What I traced:** apps/portico-server/server/cmd/server/main.go:435; apps/portico-server/server/internal/ingestion/analysis.go:361; apps/portico-server/server/internal/localmetadata/audio.go:55,60,64,101,123,126,136,156; apps/portico-server/server/internal/localmetadata/sidecars.go:109; apps/portico-server/server/internal/administration/maintenance.go:36.
- **Verdict:** **Confirmed, narrower** — Unique originals occupy the permanent serving folder; no automatic eviction or reclaim path was found. Existing identical images still succeed by digest. “Silently” is too broad: audio sidecar storage failures set invalid_local_cover and embedded failures set invalid_embedded_cover, although both misdescribe budget exhaustion. Artwork from online providers has its own store; not all artwork is under this budget.
- **Class:** Defect.
- **Severity:** **High** — Libraries relying on hundreds/thousands of distinct local images eventually lose new artwork, regardless of available disk.
- **Fix direction:** Separate durable originals from bounded derived caches, track references/size incrementally, expose a truthful capacity issue and reclaim unreferenced originals. Touch localmetadata, catalog local artwork and maintenance. (**Size:** M).
- **Related:** B-06.4 is the same defect and agrees; merge into one item.
- **Backlog:** No.

### B-10.8 One unexpected line makes a whole LRC file invalid

- **Claim:** Strict LRC parsing rejects unfamiliar untimed lines.
- **What I traced:** apps/portico-server/server/internal/lyrics/document.go:49,51,118,130,144,172,222; apps/portico-server/server/internal/lyrics/service.go acquisition calls Parse/validateTiming.
- **Verdict:** **Confirmed, narrower** — Unknown headers, untimed prose, colon fractions and duplicate offsets fail the document. Blank lines are skipped, and known metadata is accepted. The explicit comment at document.go:172 forbids mixed timed/untimed prose: strictness is intentional, not proof of an accidental parser failure. Extending harmless metadata compatibility is a missing compatibility slice, while retaining byte/time bounds is sensible.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium** — People importing common extended LRC dialects cannot publish that lyric; the media remains playable.
- **Fix direction:** Accept/ignore safe metadata headers and documented alternate timestamp fractions; quarantine malformed timing with diagnostics. Touch lyrics/document.go and acquisition error presentation. (**Size:** S).
- **Related:** No separate duplicate found in B-index.txt.
- **Backlog:** No.

### B-10.10 HE-AAC is planned as plain "AAC" and will not decode on the web engine

- **Claim:** AAC admission misses HE-AAC configuration and output rate.
- **What I traced:** apps/portico-server/server/internal/audiofacts/facts.go:25,58,100; apps/portico-server/server/internal/playback/audio_plan_v2.go:99,165; apps/portico-server/web/src/bridge/audio/capabilities.ts:44; apps/portico-server/web/src/bridge/audio/mp4.ts:85; apps/portico-server/web/src/bridge/audio/track.ts:83.
- **Verdict:** **Confirmed, narrower** — For an HE-AAC MP4 whose first AudioSpecificConfig rate is the core rate while FFmpeg facts report the SBR output rate, direct admission passes and TrackStream.open throws before decoding. AAC-LC capability probes cannot exclude that file. This does not establish failure of every HE-AAC signaling variant, browser or native engine.
- **Class:** Defect.
- **Severity:** **Medium** — Web listeners to the affected HE-AAC files get an error instead of playback.
- **Fix direction:** Retain AAC profile/configuration evidence, parse SBR/PS rates correctly and test exact configurations before direct admission; allow bounded conversion fallback. Touch audiofacts, playback audio plan and web MP4 demux. (**Size:** M).
- **Related:** B-10.34 amplifies this failure; B-10.28 lacks coverage for it.
- **Backlog:** No.

### B-10.12 Long files cannot be measured in the 5-second play-time budget, and each attempt is thrown away

- **Claim:** On-demand audio measurement decodes the whole file within five seconds.
- **What I traced:** apps/portico-server/server/internal/playbackv1/audio.go:15,39; apps/portico-server/server/internal/audiofacts/facts.go:54,57,89; apps/portico-server/server/internal/audiofacts/store.go:75,82,112,200,223; apps/portico-server/server/internal/playback/audio_plan_v2.go:150. apps/portico-server/server/internal/httpapi/playback_v1_routes.go:59 wires MeasureAudio; apps/portico-server/server/internal/subtitlevideo/audio_facts.go:45 runs audiofacts.Args through the streaming probe.
- **Verdict:** **Confirmed, narrower** — The request includes -show_frames and sums decoded nb_samples, so this is not the packet-only operation promised by Playback Protocol §18.7. A deadline cancellation is deliberately not persisted and later play can repeat it. The exact file duration that exceeds five seconds depends on codec/hardware; a long file does not mathematically guarantee timeout. Background backfill can resolve it; ordinary playback is retained with the unavailable audio-effects plan.
- **Class:** Defect.
- **Severity:** **Medium** — New/unmeasured long books can incur repeated startup work and temporarily lose audio effects; claimed days of delay are unmeasured.
- **Fix direction:** Use packet-only facts where exactness permits, persist a bounded deferred-measurement state, and avoid repeating expensive interactive attempts. Touch audiofacts, decoder audio measurement and playbackv1/audio.go. (**Size:** M).
- **Related:** B-10.28 lacks long-file fixtures; B-10.13 and B-17.17 are different full-prefix decode costs.
- **Backlog:** No.

### B-10.15 Audiobook parts without a track tag or a leading number play in catalogue order, not file-name order

- **Claim:** Untagged audiobook parts use catalogue order.
- **What I traced:** apps/portico-server/server/internal/catalog/audio_repository.go:93,270; apps/portico-server/server/internal/playbackv1/selectors.go:154; apps/portico-server/server/internal/catalog/audio_queries.go:243; apps/portico-server/server/internal/ingestion/inventory.go:94,133; apps/portico-server/server/internal/storage/inventory_page_linux.go:14,25,54; apps/portico-server/server/internal/ingestion/analysis.go:50.
- **Verdict:** **Confirmed** — Part number parsing recognizes only a leading number or track tag. Local inventory reads filesystem directory order through getdents rather than a natural filename sort, then analysis orders inventory object IDs; no production filename fallback exists in the book selector. Even a fortuitously correct first scan cannot keep a later-added earlier part in order.
- **Class:** Defect.
- **Severity:** **Medium** — Multi-file books without complete track/disc tags can play chapters out of order.
- **Fix direction:** Maintain a natural filename ordering key as a fallback after explicit part tags, consistently in detail and queue selectors. Touch catalog audio ingestion, audio queries and playbackv1 selectors. (**Size:** M).
- **Related:** B-10.26 is the separate missing-disc ordering case; Q-24.21 asks for structural acceptance.
- **Backlog:** No.

### B-10.19 Covers with other names, capitalisation, size or location are missed

- **Claim:** Local audio covers use a narrow naming and location vocabulary.
- **What I traced:** apps/portico-server/server/internal/localmetadata/audio.go:55,56,66,78,110; apps/portico-server/server/internal/storage/client.go:363; apps/portico-server/server/internal/ingestion/analysis.go:361.
- **Verdict:** **Confirmed, narrower** — Exact leaf paths find four lowercase JPG/PNG names, with no parent search. On a case-sensitive filesystem alternate capitalization is missed; on case-insensitive storage it may work. Size/dimension bounds are explicit defensive policy, and embedded extraction scales to 1024 pixels before storage. The first-picture selection and folder-wide cover semantics remain as claimed. There is no recorded promise of recognizing every named variant.
- **Class:** Missing feature mislabelled as a bug.
- **Severity:** **Medium** — Owners with front.jpg, mixed-case names or disc-folder parent art need renaming or embedded art; oversized sidecars are refused.
- **Fix direction:** Add a bounded case-folded candidate discovery with parent-disc-folder support and normalize oversized originals safely. Touch localmetadata/audio.go and storage small-file discovery. (**Size:** M).
- **Related:** B-10.2 is the independent permanent budget defect; B-06.4 duplicates that budget.
- **Backlog:** No.

### B-10.28 The gapless fixtures leave whole classes of file untested (design risk)

- **Claim:** Conformance fixtures omit important audio classes.
- **What I traced:** apps/portico-server/fixtures/audio-gapless/generate.py and album.json; apps/portico-server/server/internal/audiofacts/facts_test.go:63,86; apps/portico-server/web/test/audio-demux.test.ts:6; apps/portico-server/web/test/audio-track.test.ts:43; Playback Protocol §18.8.
- **Verdict:** **Confirmed, narrower** — The committed corpus has FLAC, LAME MP3, iTunSMPB AAC, ALAC, WAV and Opus; no edit-list-only AAC, HE-AAC, Vorbis, no-LAME MP3 or multi-hour fixture. The FFmpeg AAC case at facts_test.go:86 explicitly adds iTunSMPB, so it does not cover edit-list-only AAC. audio-track.test.ts:39 has synthetic resampler checks, which narrow the claim of no resampling tests, but do not test a complete mismatched-rate album join through each engine. This is a coverage gap, not evidence every uncovered class fails.
- **Class:** Design risk.
- **Severity:** **Medium** — Uncovered files may expose trim/admission/long-file failures across clients; this is a verification gap, not evidence all such files fail.
- **Fix direction:** Extend generated corpus and cross-engine acceptance to the missing cases, keeping long-file tests bounded and separate from unit tests. Touch fixtures/audio-gapless, audiofacts tests and client conformance harnesses. (**Size:** M).
- **Related:** B-10.10/12/23 and B-25.9/10 agree on gaps but are not all one bug.
- **Backlog:** No.

### B-10.29 Files with unlisted extensions vanish without a word

- **Claim:** Unlisted audio extensions are skipped before inventory records.
- **What I traced:** apps/portico-server/server/internal/assets/assets.go:285; apps/portico-server/server/internal/catalog/inventory.go:42,141; apps/portico-server/server/internal/ingestion/inventory.go:144; apps/portico-server/server/internal/catalog/inventory_admin.go:90.
- **Verdict:** **Confirmed** — The active inventory gate continues before cataloguing unsupported names. The owner inventory has an unsupported filter, but it cannot report files skipped before an inventory row exists. A broad FFmpeg format whitelist is a sandbox permission list, not a supported-extension promise; format support is missing rather than proven broken for those formats.
- **Class:** Missing feature mislabelled as a bug.
- **Severity:** **Medium** — Owners importing otherwise decodable .aif/.oga/.mka/.wma/.ape/.wv/.mp4 audio receive no per-file reason.
- **Fix direction:** Publish bounded unsupported-media diagnostics and add format/extensions only with tested probing/delivery. Touch assets support gate, catalog inventory and owner inventory presentation. (**Size:** M).
- **Related:** No separate duplicate found in B-index.txt.
- **Backlog:** No.

### B-10.34 When the device's engine cannot play a direct plan, playback fails instead of falling back

- **Claim:** A direct web audio decode failure has no automatic delivery fallback.
- **What I traced:** apps/portico-server/web/src/bridge/DecodeAudioRender.ts:123,162,174,245; apps/portico-server/packages/client-core/src/playback-v1/v1-queue-player.ts:603,724; apps/portico-server/packages/client-core/src/playback-v1/audio-render.ts plan eligibility. apps/portico-server/packages/client-core/src/index.ts:1453 has a separate engineFailed escalation method, but this audio path calls fail directly (:1469); apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:394 selects the decoded-audio engine.
- **Verdict:** **Confirmed, narrower** — Current-track demux/decode failures call playback.fail. Retry restarts the same entry without changing decode capabilities or requesting converted audio, so the same bad direct tuple can recur. The failed prepared-next transition test in W test/audio-render.test.ts:52 is an advance fallback, not a direct decoder fallback. Scope is established for web; every native engine failure path was not traced.
- **Class:** Defect.
- **Severity:** **Medium** — Web users with a measured but unsupported/malformed direct tuple cannot recover through ordinary retry.
- **Fix direction:** Report failed tuple/configuration to the planner and retry once with a converted or ordinary supported plan, preserving position and owner limits. Touch web DecodeAudioRender, shared queue recovery and server audio planning. (**Size:** M).
- **Related:** B-10.10/30 are triggers; B-20.6 concerns video failure classification, not this exact path.
- **Backlog:** No.

### B-11.1 Every guide refresh retunes every live viewer of that source

- **Claim:** Publishing any guide generation retunes live viewers.
- **What I traced:** apps/portico-server/server/internal/livechannels/remote.go:450; apps/portico-server/server/internal/livechannels/store.go:252; apps/portico-server/server/internal/playbackruntime/linear_resolver.go:115; apps/portico-server/server/internal/playback/channel_sessions.go:279,305,320; apps/portico-server/server/internal/playbackruntime/linear.go:335,590,717. apps/portico-server/server/cmd/server/main.go:620 wires this runtime.
- **Verdict:** **Confirmed** — Refresh creates a fresh generation even for identical bytes. Current-generation resolution changes the session selection, stops its producer and reacquires/probes upstream. The playback session itself survives; retained buffer may hide some interruption. No semantic-equality fast path was found. Actual visible stall duration is not established.
- **Class:** Defect.
- **Severity:** **High** — All live viewers on a refreshed source retune together, including ordinary six-hour automatic refresh and hourly owner selections.
- **Fix direction:** Fence playback on stream/configuration identity separately from guide-generation identity; update programme metadata without retiring unchanged upstream input. Touch livechannels publication, playback channel sessions and linear runtime. (**Size:** M).
- **Related:** B-11.17 is related recovery behavior, not a duplicate; Q-24.24 asks for runtime isolation.
- **Backlog:** No.

### B-11.2 A standard XMLTV file with a DOCTYPE line is rejected

- **Claim:** A harmless XMLTV DOCTYPE is rejected.
- **What I traced:** apps/portico-server/server/internal/livechannels/parse.go:139,160,355,371; apps/portico-server/server/internal/livechannels/adapters.go:193; apps/portico-server/server/internal/livechannels/store_test.go:179.
- **Verdict:** **Confirmed** — Both XML preflight and parser reject every xml.Directive, including the ordinary external-DTD declaration. Go tokenization does not itself fetch that DTD. The hostile-entity test justifies rejecting unsafe declarations, not rejecting the benign XMLTV declaration. There is no adapter normalization before parse.
- **Class:** Defect.
- **Severity:** **High** — Any source delivering that declaration cannot initially import, or retains a stale prior guide on refresh failure.
- **Fix direction:** Recognize a safe XMLTV DOCTYPE without fetching external entities, retaining rejection of internal entity declarations. Touch livechannels/parse.go and importer fixtures. (**Size:** S).
- **Related:** B-11.3/4 are distinct XMLTV rejection causes; merge implementation work, not evidence.
- **Backlog:** No.

### B-11.3 Valid XMLTV time forms, a missing stop, or a non-UTF-8 encoding reject the whole guide

- **Claim:** XMLTV ingest supports only full zoned times and UTF-8-compatible XML.
- **What I traced:** apps/portico-server/server/internal/livechannels/parse.go:145,222,355; apps/portico-server/server/internal/livechannels/adapters.go:193; XML decoders have no CharsetReader.
- **Verdict:** **Confirmed** — Short/zoneless timestamps, missing stop and declared legacy charsets cannot pass. No caller expands them. This confirms the support gap, but inferring an absent stop requires explicit policy: a partial timestamp or a final open-ended programme cannot always yield a trustworthy DVR interval. Legacy charset normalization is a separate compatibility slice. Source-wide invalidation is shared with B-11.4.
- **Class:** Missing feature mislabelled as a bug.
- **Severity:** **Medium** — Owners of such feeds have no initial guide; prevalence across intended feeds was not measured.
- **Fix direction:** Normalize documented timestamp forms and a bounded charset vocabulary; infer stops only from valid channel sequence evidence and expose unresolved intervals. Touch livechannels guide parser/adapters. (**Size:** M).
- **Related:** B-11.2/4 agree on narrow XMLTV vocabulary; B-11.6 is compressed-file handling.
- **Backlog:** No.

### B-11.4 One odd programme rejects the whole guide

- **Claim:** A malformed or contradictory programme can invalidate a guide.
- **What I traced:** apps/portico-server/server/internal/livechannels/parse.go:18,224,227,266,287,320; portico-internal/Product Contracts/live-tv-and-dvr/contract.md:73 (reject or quarantine invalid intervals); apps/portico-server/server/internal/livechannels/store_test.go:130 preserves the old guide.
- **Verdict:** **Confirmed, narrower** — Overlaps, invalid title/control characters, excessive duration and contradictory new/repeat evidence reject the document. The code explicitly validates even unmapped bad timestamps. Harmless invalid icons are ignored, invalid descriptions dropped, and unfamiliar nested tags skipped: not every odd field rejects. Fail-closed atomic import is deliberate; quarantine would also satisfy the contract. The claimed frequency of overlaps is unverified.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium** — One bad airing blocks an initial import or guide refresh, while an existing valid guide remains available.
- **Fix direction:** Keep atomic publication but quarantine bad programme rows with counts/reasons; expose incomplete guide coverage. Touch livechannels parsing/publication and owner diagnostics. (**Size:** M).
- **Related:** B-11.3/7 share strict-source validation; no distinct cross-document duplicate found.
- **Backlog:** No.

### B-11.5 Size limits fail the whole source

- **Claim:** Fixed ingest bounds can exclude large channel lineups.
- **What I traced:** apps/portico-server/server/internal/livechannels/model.go:20; apps/portico-server/server/internal/livechannels/parse.go:33,116,295,366; apps/portico-server/server/internal/livechannels/adapters.go:34,53,96,159,180; apps/portico-server/packages/client-core/src/linear-api.ts:19. portico-internal/Product Contracts/live-tv-and-dvr/contract.md:263 requires bounds and :85 requires supplied seven-day guide range.
- **Verdict:** **Confirmed, narrower** — The bounds are intentional and enforced at multiple layers; rejection does not erase the prior generation. A count of 500 channels alone does not establish 100,000 programmes: seven days at hourly airings is 84,000; at half-hourly airings it is 168,000. A guide can hit the byte or XML-node bound first. This is a compatibility/scale design risk, not proof arbitrary limits should disappear.
- **Class:** Design risk.
- **Severity:** **Medium** — Large IPTV lineups cannot be imported completely under current bounds.
- **Fix direction:** Use streaming bounded staging with per-operation budgets and validated scale targets; synchronize client preview bounds and diagnostics. Touch livechannels adapters/parser/staging and client-core linear API. (**Size:** M).
- **Related:** B-11.31 explains a downstream cost of raising channel caps.
- **Backlog:** No.

### B-11.7 One imperfect playlist entry rejects the whole playlist

- **Claim:** Strict playlist parsing rejects a whole source on an incompatible entry.
- **What I traced:** apps/portico-server/server/internal/livechannels/parse.go:21,37,49,87,96,101; apps/portico-server/server/internal/livechannels/adapters.go:113,168; apps/portico-server/server/internal/livechannels/store_test.go:176.
- **Verdict:** **Confirmed, narrower** — Missing identity, conflicting duplicates, userinfo URLs/fragments, BOM and oversized lines fail source validation. Exact duplicates collapse; ordinary comment lines are skipped. Xtream names containing quotes are explicitly refused before synthesized playlist creation. Identity ambiguity and URL safety rejection are intentional safeguards, while missing-id/BOM/name compatibility can improve without weakening network policy.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium** — Owners with one incompatible row cannot import a new source or refresh the old one.
- **Fix direction:** Quarantine incompatible rows with owner diagnostics, strip safe BOMs, derive stable nonambiguous identities where justified, and retain network-policy checks. Touch livechannels playlist parser/adapters. (**Size:** M).
- **Related:** B-11.4 uses the same fail-whole-import policy for programme rows.
- **Backlog:** No.

### B-11.8 HDHomeRun sources cannot have a guide, so they cannot record

- **Claim:** First-party HDHomeRun setup leaves guide mapping unusable.
- **What I traced:** apps/portico-server/web/src/screens/server/LiveTV.tsx:117; apps/portico-server/packages/client-core/src/linear-api.ts:11; apps/portico-server/server/internal/livechannels/remote.go:32,37,96; apps/portico-server/server/internal/livechannels/adapters.go:64,129,173,179; apps/portico-server/server/internal/livechannels/parse.go:123; apps/portico-server/server/internal/livechannels/dvr/model.go:64. Contract live-tv-and-dvr §4.1 requires owner mapping.
- **Verdict:** **Confirmed, narrower** — The UI hides GuideURL and starts with empty mappings. However the actual remote API accepts GuideURL and ChannelMapping, the adapter fetches that guide for every kind, and parse applies those mappings. An XMLTV feed could also deliberately use hdhr-<number> IDs. Thus “HDHomeRun cannot have a guide even through API” is false. The real shipped UI omission is serious; the admin extras map does not feed ingest (B-11.9).
- **Class:** Unfinished feature.
- **Severity:** **High** — Ordinary owners adding a home tuner through first-party setup get no programme-backed DVR; a correctly authored API configuration can work.
- **Fix direction:** Expose guide URL and validated channel mapping in preview/save/edit and connect extras to the ingest authority. Touch LiveTV, LiveSourceExtras, client-core linear API and administration integration. (**Size:** M).
- **Related:** B-11.9 agrees on unwired settings; merge guide/mapping implementation work.
- **Backlog:** No.

### B-11.10 A few minutes of padding overlap fails an entire recording

- **Claim:** Any losing padded interval fails the entire DVR intent.
- **What I traced:** apps/portico-server/server/internal/livechannels/dvr/worker.go:341; apps/portico-server/server/internal/livechannels/dvr/conflicts.go:13,19,30,48; apps/portico-server/server/internal/livechannels/conflicts.go:27; apps/portico-server/server/internal/livechannels/dvr/store.go:39; apps/portico-server/packages/client-core/src/linear-api.ts:11. Contract live-tv-and-dvr:114,167,181 specifies padded protected reservations and failed losers.
- **Verdict:** **Confirmed, narrower** — At known capacity a losing subinterval causes terminal tuner-conflict rather than a late/trimmed start. Unknown capacity does not trigger this computed conflict. This is the written protected-reservation policy, not a scheduler failing to enforce it. The first pass also overstates “every HDHomeRun”: the draft defaults useDiscoveredCapacity=false; discovered count must be selected/confirmed before it becomes this cap. Editing padding resolves the example.
- **Class:** Deliberate trade-off.
- **Severity:** **High** — Owners enforcing one-tuner/connection limits can lose back-to-back recordings because default padding overlaps.
- **Fix direction:** Product decision: make padding soft while programme time remains protected, or explain conflicts/default padding clearly. If adopting soft padding, touch DVR conflicts, reservations, claim logic and rule/record UI. (**Size:** M).
- **Related:** B-11.26 compounds tuner usage but follows an independent one-allocation-per-recording decision.
- **Backlog:** No.

### B-11.11 The first upstream error ends a recording for good

- **Claim:** Recording capture does not reconnect after an upstream interruption.
- **What I traced:** apps/portico-server/server/internal/linearinput/gateway.go:200; apps/portico-server/server/internal/decoder/linear.go:32; apps/portico-server/server/internal/recordingmedia/driver.go:215,263,304; apps/portico-server/server/internal/livechannels/dvr/worker.go:451; apps/portico-server/server/internal/livechannels/dvr/publication.go:78.
- **Verdict:** **Confirmed, narrower** — The raw input pump terminates on its first read error; recording FFmpeg has no explicit reconnect policy. Playable output is validated and published as terminal incomplete, or recovery validates existing bytes, rather than reopening the source for remaining airtime. HLS itself may retry requests inside FFmpeg, so “first upstream error” is too broad across transports. No Portico-level capture reconnect loop was found.
- **Class:** Unfinished feature.
- **Severity:** **High** — A transient raw-source outage can lose the remainder of a scheduled programme; partial bytes are preserved.
- **Fix direction:** Add bounded reconnect/append capture with fenced ownership and explicit gap accounting, retaining incomplete status when bytes were missed. Touch recordingmedia, linearinput and DVR worker/checkpoint model. (**Size:** L).
- **Related:** B-11.12 shares partial-publication recovery but concerns process restart.
- **Backlog:** No.

### B-11.13 A one-time recording is lost if the provider corrects the start time

- **Claim:** A moved interval-only airing strands a one-time recording.
- **What I traced:** apps/portico-server/server/internal/livechannels/parse.go:279; apps/portico-server/server/internal/livechannels/occurrence.go:13,22,37; apps/portico-server/server/internal/livechannels/dvr/scheduled.go:64,71,87,132; apps/portico-server/server/internal/livechannels/dvr/worker.go:296. apps/portico-server/server/internal/livechannels/refresh_failure_test.go:40 expressly expects correction identity to survive.
- **Verdict:** **Confirmed, narrower** — Without an explicit programme id, identity includes start time; a time move removes the old identity, yielding waiting-guide then missed-window failure. With a still-future explicit provider id and noncontradictory episode evidence, programmeIdentityTx retains ID and scheduled reconciliation updates times. Refusing title/time guessing is deliberately contracted in live-tv-and-dvr §4.1/4.3; missing guide lineage remains an unresolved mapping, not permission to silently record a different airing.
- **Class:** Deliberate trade-off.
- **Severity:** **High** — One-time DVR requests against interval-only feeds may capture nothing after schedule correction; stable-ID feeds are protected.
- **Fix direction:** Expose the unresolved occurrence and a user-confirmed rebind/record-original-window choice, using strong additional provider evidence when available. Touch livechannels occurrence evidence, DVR reconciliation and recording issue UI. (**Size:** M).
- **Related:** B-11.4 is guide validation, not this reconciliation problem.
- **Backlog:** No.

### B-11.14 HLS sources with several quality variants: recordings take all of them, live takes the first listed

- **Claim:** HLS variant mapping may record every quality and choose arbitrary live quality.
- **What I traced:** apps/portico-server/server/internal/decoder/linear.go:36 maps every video/audio/subtitle stream; apps/portico-server/server/internal/decoder/linear_hls.go:105 maps first video/audio; apps/portico-server/server/internal/linearinput/manifest.go:50 rewrites but does not select variants; apps/portico-server/server/internal/linearinput/gateway.go:309 caps concurrent requests.
- **Verdict:** **Unsettled — needs runtime** — No gateway/caller selects a programme or variant first. The stream mapping asymmetry is certain; exact FFmpeg HLS exposure, HTTP request counts and output bitrate were not executed and can vary with the master and linked audio renditions. Cheapest later experiment: a local synthetic two-variant HLS master in a disposable acceptance environment, run the exact capture and live argument vectors and probe output/count child requests; multiple capture video streams confirms amplification, first-listed live stream confirms selection. This was not run under this standard-library-only review allowance.
- **Class:** Design risk.
- **Severity:** **High** — If the predicted mapping occurs, affected HLS recordings consume multiple qualities and may fail the four-request bound.
- **Fix direction:** Select one supported variant/programme and its linked tracks consistently before capture/live planning. Touch linearinput manifest selection, decoder linear copy/HLS and probe selection. (**Size:** M).
- **Related:** B-11.15/16 are independent live encoding decisions.
- **Backlog:** No.

### B-11.15 On Android and Fire TV, live channels are probably always converted

- **Claim:** Unknown live bitrate can become the Android decoder maximum encode target.
- **What I traced:** apps/portico-react-native/apps/native/android/app/src/main/java/tv/getportico/portico/native/PorticoCapabilities.kt:146,162; apps/portico-server/packages/client-core/src/client-profile.ts:495; apps/portico-server/packages/client-core/src/playback-v1/capability-profile.ts:46; apps/portico-server/server/internal/playbackv1/capabilities.go:240; apps/portico-server/server/internal/playbackruntime/linear_plan.go:185; apps/portico-server/server/internal/decoder/linear_hls.go:73.
- **Verdict:** **Confirmed, narrower** — The probed decoder maximum survives publication/translation. When both video and format bitrate are unknown, it forces conversion and uses that maximum as target; an explicit smaller quality cap narrows it. A value over 200 Mb/s is rejected by the argument builder. “Always on Android/Fire TV” is unproven: format bitrate can be known, native probing can be unavailable and caps differ by hardware. Cheapest platform check: collect capability JSON and one compatible live probe on a physical Android/Fire TV device, inspect chosen encoding and output bitrate.
- **Class:** Defect.
- **Severity:** **Medium** — Affected devices on unknown-bitrate sources spend unnecessary server CPU/bandwidth, and a target above the builder bound prevents start unless capped.
- **Fix direction:** Separate decode ceilings from encode targets; choose a bounded source/quality-based target for required conversion and clamp/validate before launching. Touch playbackruntime/linear_plan.go and its capability/plan tests. (**Size:** S).
- **Related:** B-20.8 adds a separate MPEG-2 capability-name mismatch; B-20.14 concerns software decoder declarations.
- **Backlog:** No.

### B-11.16 An unknown frame rate forces a conversion

- **Claim:** Unknown live frame rate triggers conservative conversion.
- **What I traced:** apps/portico-server/server/internal/playbackruntime/linear_plan.go:97,143,148,181; apps/portico-server/server/internal/decoder/linear_hls.go:93.
- **Verdict:** **Confirmed, narrower** — Given an absent/zero/unparseable average frame rate, MaxFrameRate becomes a cap and ConvertVideo is true, even for otherwise compatible H.264. The code comment explicitly bounds the encoded tuple; refusing unknown decode evidence is conservative intent. Frequency of 0/0 on real supported sources remains unmeasured. Cheapest later check: feed a known-compatible captured probe with avg_frame_rate=0/0 through the planner and inspect ConvertVideo; then compare a real provider probe.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium** — Only unknown-rate sources incur this extra conversion; it is not a blanket live conversion rule.
- **Fix direction:** Use trustworthy r_frame_rate or bounded packet timing as fallback evidence before requiring conversion; retain caps for truly unknown tuples. Touch linear_plan and bounded probing. (**Size:** S).
- **Related:** B-11.15 can independently force conversion in the same plan.
- **Backlog:** No.

### B-11.18 The segment writer has no tolerance for a slow database

- **Claim:** One transient authority error rejects a segment upload as retired.
- **What I traced:** apps/portico-server/server/internal/playbackruntime/linear.go:737; apps/portico-server/server/internal/linearbuffer/buffer.go:345,354; apps/portico-server/server/internal/playback/channel_sessions.go:279; apps/portico-server/server/internal/playbackruntime/linear.go:623.
- **Verdict:** **Confirmed, narrower** — Any returned check error becomes HTTP 410 on upload; this path does not apply the monitor’s transient-error patience. A merely slow successful query is not automatically a rejection. Whether FFmpeg stops immediately or retries after that response is runtime-dependent, and database error frequency under load is unmeasured. Cheapest later experiment: inject one transient check error into a disposable Publisher with exact FFmpeg uploader and observe recovery/exit; 410 is source-certain, interruption requires that response path.
- **Class:** Defect.
- **Severity:** **Medium** — Live viewers under a transient DB/read failure can suffer avoidable producer retirement; no data-loss frequency is established.
- **Fix direction:** Distinguish authority refusal from transient infrastructure failure, retry within a bounded upload deadline and return a retryable response when safe. Touch linearbuffer Publisher and channel authority error mapping. (**Size:** S).
- **Related:** B-11.17 limits recovery if these failures repeat; Q-24.1 discusses load but is not the same defect.
- **Backlog:** No.

### B-11.19 The HLS gateway refuses playlists with common extra tags

- **Claim:** HLS gateway refuses unsupported URI-bearing extension tags.
- **What I traced:** apps/portico-server/server/internal/linearinput/manifest.go:55,57,98; apps/portico-server/server/internal/linearinput/gateway.go manifest rewriting path.
- **Verdict:** **Confirmed, narrower** — Listed low-latency tags and unknown lines containing URI=/URL= fail manifest validation. DATERANGE without such text passes; specific ad feeds failing was not verified. The explicit comment protects against a future FFmpeg expanding network access. That boundary is intentional; dropping ignorable unknown lines would preserve it. Low-latency-only feeds may require actual support, not merely dropping PART lines.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium** — Providers emitting these tags cannot play/record through this gateway; actual provider reach is unknown.
- **Fix direction:** Strip safely ignorable extension metadata before FFmpeg; add explicit bounded rewriting/support for media-bearing tags only when required. Touch linearinput/manifest.go and fixtures. (**Size:** S).
- **Related:** B-11.14 concerns variant selection through the same gateway, not extension safety.
- **Backlog:** No.

### B-11.20 "Complete" is judged against the padded window with a four-second tolerance

- **Claim:** Completion requires padded-window coverage with four-second tolerance.
- **What I traced:** apps/portico-server/server/internal/recordingmedia/driver.go:437,445; apps/portico-server/server/internal/livechannels/allocator.go:143; portico-internal/Product Contracts/architecture-amendments.md:89 and live-tv-and-dvr §6.1/10.
- **Verdict:** **Confirmed, narrower** — Completion checks padded start/end, observed first/last bytes, gaps, producer success, boundary and media duration. It is not just a symmetric four-second duration test: up to eight seconds extra duration is accepted. A startup delay over four seconds can mark a fully covered programme incomplete, but that is honest about the requested padded window. How often startup crosses the tolerance needs measurement. Cheapest check: a delayed synthetic source with 60-second prepadding, start delivery five seconds late, verify all programme time exists and result remains incomplete.
- **Class:** Deliberate trade-off.
- **Severity:** **Low** — Some recordings receive a conservative incomplete label despite full unpadded programme content; playback remains available.
- **Fix direction:** Retain capture truth but distinguish missing padding from missing programme in UI/reason fields. Touch recordingmedia result and DVR description/presentation. (**Size:** S).
- **Related:** B-11.27 misses the pre-emption countdown and can introduce lateness independently.
- **Backlog:** No.

### B-11.33 A slow import can outlive its refresh lease

- **Claim:** Large refreshes may exceed their fixed two-minute lease.
- **What I traced:** apps/portico-server/server/internal/livechannels/remote.go:413,427,439,450,476; apps/portico-server/server/internal/livechannels/store.go:282,313; apps/portico-server/server/internal/livechannels/remote_large_guide_test.go:17.
- **Verdict:** **Unsettled — needs runtime** — No lease renewal exists between initial claim and final publication; each staging transaction reauthorizes. A 75-second fetch leaves under 45 seconds for parsing/staging. Thus crossing the deadline deterministically prevents publication, but a near-limit guide crossing it on supported hardware is not established. The existing large-guide test is 864 programmes and the owner preview/save path, not a refresh-expiry/load test. Cheapest later experiment: delayed disposable fetch plus near-limit guide with controlled slow staging, inspect active generation and degraded status; refusal at 120 seconds confirms the risk.
- **Class:** Design risk.
- **Severity:** **Medium** — A busy/slow server may repeatedly fail guide refresh while retaining an increasingly stale prior generation.
- **Fix direction:** Renew the exact refresh token during bounded staging, fencing revision and cancellation; alternatively budget the entire operation consistently. Touch livechannels remote refresh/staging. (**Size:** S).
- **Related:** B-11.32 explains how repeated generations consume storage; no duplicate lease claim in B-index.
- **Backlog:** No.

## Tier 2 — quick checks

Six entries received fuller checks because the original wording needed correction. They remain Tier 2 for coverage/counts.

### B-10.3 "Play next" on a shuffled queue adds to the very end

**Confirmed · Deliberate trade-off · Medium · Size S.** apps/portico-server/server/internal/playbackv1/queue_commands.go:744 appends while shuffled, including the apps/portico-server/packages/client-core/src/playback-v1/v1-queue-player.ts:511 play-next action. Playback Protocol §17.10 explicitly requires the unshuffled tail; the misleading action label is the defect within a deliberate queue policy. Fix direction: Either label the action Add to queue under shuffle or amend the queue policy to reserve immediate next entries; touches queue commands and client action text. Related: No separate duplicate found. Backlog: No.

### B-10.4 Removing or moving one entry in a shuffled queue reshuffles everything and forgets what was played

**Confirmed · Defect · Medium · Size M.** apps/portico-server/server/internal/playbackv1/queue_commands.go:838,844,939 resets the shuffle domain to the whole remaining queue and current to zero after remove/move. No played-prefix preservation exists in this mutation path. Fix direction: Preserve effective played/upcoming order through mutation; touches queue layout and commands. Related: No separate duplicate found. Backlog: No.

### B-10.5 A library-wide lyrics run that is rate-limited once races through the rest of the library as "missing"

- **Claim:** Provider throttling is counted as missing lyrics instead of deferred work.
- **What I traced:** apps/portico-server/server/internal/lyrics/lrclib.go:40,68; apps/portico-server/server/internal/lyrics/bulk.go:151,215,307; apps/portico-server/server/internal/dbwork/background_pacing.go:17. apps/portico-server/server/cmd/server/main.go:946 exposes LyricsBulk.
- **Verdict:** **Confirmed, narrower** — The bulk cursor advances after ErrUnavailable and those songs are reported missing. The whole remainder does not necessarily finish within the pause: work is batched, yields under foreground pressure, and provider calls resume once the deadline is close enough. The defect is every transient refusal permanently consuming that song within this run, not a guaranteed instant loss of every remaining library entry.
- **Class:** Defect.
- **Severity:** **Medium** — Large bulk runs can skip many songs during one provider outage while reporting success; rerunning retries missing songs.
- **Fix direction:** Return a retryable/deferred outcome carrying provider deadline, keep the cursor on pending work and report outages distinctly. Touch lyrics/bulk.go and LRCLIB pacing. (**Size:** S).
- **Related:** B-10.6 is a separate provider-match defect.
- **Backlog:** No.

### B-10.6 The bulk lyrics run publishes the first search hit without checking it is the same song

- **Claim:** Bulk lyric publication checks timing but not song identity.
- **What I traced:** apps/portico-server/server/internal/lyrics/bulk.go:298,307,313,317,327; apps/portico-server/server/internal/lyrics/lrclib.go:130,155; apps/portico-server/server/internal/lyrics/bulk_test.go:98 exercises an accepted provider candidate.
- **Verdict:** **Confirmed, narrower** — The choice is not literally always the first hit: it prefers the first timing-valid synced result, falling back to the first timing-valid plain result. Neither branch compares title/artist/album/duration identity to the requested song; the response does not retain duration. This correction does not remove the central wrong-lyrics risk.
- **Class:** Defect.
- **Severity:** **Medium** — Songs with common titles, covers or edits can receive another song’s library-wide lyrics.
- **Fix direction:** Use exact-match signatures and confidence checks before bulk publication, preserving user review for ambiguous results. Touch lyrics provider DTO/search and bulk matching. (**Size:** M).
- **Related:** B-10.8 concerns parsing, not wrong identity.
- **Backlog:** No.

### B-10.7 Retagging a file hides its lyrics, and the bulk run will not fetch them again

**Confirmed · Defect · Medium · Size S.** apps/portico-server/server/internal/lyrics/service.go:155 hashes size/mtime and :277 filters by current source_version; apps/portico-server/server/internal/lyrics/bulk.go:80,357 tests any undeleted library resource without that version. Retagging therefore hides lyrics and prevents bulk reacquisition; no rebind/delete path was found. Fix direction: Use current source version consistently in pending/publish dedup; preserve old revisions for explicit review. Touch lyrics bulk and source-change handling. Related: No separate duplicate found. Backlog: No.

### B-10.11 A bitrate limit on an Apple device turns the audio plan off

**Confirmed · Unfinished feature · Medium · Size M.** apps/portico-react-native/apps/native/src/bridge/audio-decode.ts:14 declares no Ogg Opus; apps/portico-server/server/internal/playback/audio_plan_v2.go:163,191,205,225 yields unavailable effects when the active bitrate limit excludes direct and FLAC. Ordinary playback remains available; the reason wrongly says FLAC cannot decode. Fix direction: Provide an exact bounded AAC or supported compressed route on Apple; correct unavailable reason. Touch server audio plan/conversion and Apple audio renderer. Related: No separate duplicate found. Backlog: No.

### B-10.13 A listener's conversion runs as background work and restarts from the file's beginning on every seek

**Confirmed · Defect · Medium · Size M.** apps/portico-server/server/internal/subtitlevideo/audio_convert.go:24 uses AnalysisDecoder; apps/portico-server/server/internal/decoder/analysis.go:62 trims after full-prefix decode and :127 sets background=true; apps/portico-server/server/internal/mediaexec/shim_unix.go:47 lowers priority. This is the interactive conversion path, contrary to portico-internal/Project Architecture/04 Playback and Media.md:18. Fix direction: Separate interactive conversion priority and implement indexed seek with exact trim correction. Touch decoder analysis/conversion and subtitlevideo. Related: B-17.17 is the related web direct MP3/FLAC prefix-read cost; different implementation. Backlog: Related only: web MP3/FLAC seek tables (Backlog.md:67); not this conversion path..

### B-10.14 Narrators are read only from a narrator tag

- **Claim:** Common composer-to-narrator fallback is missing.
- **What I traced:** apps/portico-server/server/internal/catalog/audio_repository.go:467,528; apps/portico-server/server/internal/catalog/audio_evidence.go:155,210; apps/portico-server/server/internal/assets/audio_tags.go:67; apps/portico-server/server/internal/localmetadata/evidence.go:153,253,316; apps/portico-server/server/internal/catalog/audio_evidence_test.go:27.
- **Verdict:** **Confirmed, narrower** — Composer does not map to narrator. However “narrators read only from narrator” is too broad: plural narrators is accepted into LocalBookMetadata, OPF supports nrt creators/contributors, JSON supports narrator arrays, and previous book-wide evidence is retained. The first pass acknowledged OPF but missed the plural/JSON/evidence path. Comment/grouping/mvnm still lack the claimed aliases.
- **Class:** Missing feature mislabelled as a bug.
- **Severity:** **Medium** — Audiobook files tagged only with composer lack a narrator unless other local evidence supplies one.
- **Fix direction:** For audiobook libraries, use composer as lower-priority narrator evidence and add documented aliases without changing music semantics. Touch assets tag aliases and localmetadata/catalog evidence. (**Size:** S).
- **Related:** Q-24.21 concerns broader book structure.
- **Backlog:** Related only: unmerged Audiobooks agent (Backlog.md); no explicit composer alias item..

### B-10.16 One bad chapter discards all chapters

**Confirmed · Deliberate trade-off · Medium · Size S.** apps/portico-server/server/internal/assets/assets.go:234 rejects the complete chapter timeline on invalid intervals/overlaps or a 4097th chapter. It already clamps a final end within 0.1 seconds of duration (:243), so minor overruns inside that tolerance survive. Fix direction: Quarantine individual bad chapter intervals with diagnostics instead of dropping a useful timeline; touches assets chapter parsing. Related: B-04.23 is the same parser and agrees; merge fixes. Backlog: No.

### B-10.17 Common album layouts split into several albums

- **Claim:** Local album identity can split otherwise related tracks.
- **What I traced:** apps/portico-server/server/internal/catalog/audio_repository.go:252,326,340,346,380,414; apps/portico-server/server/internal/catalog/audio_evidence.go edition evidence; tagged compilation fixture in apps/portico-server/server/internal/ingestion/audio_test.go:68.
- **Verdict:** **Confirmed, narrower** — Year, album artist and folder participate in fallback identity, so inconsistent values split fresh local albums. But missing album-artist is not sufficient on its own: compilation=1/true substitutes Various Artists. Exact Disc/CD folders collapse to parent, and manual MusicBrainz album matches preserve existing album identity. The first pass’s grouping risk survives for uncorrected local tags, not every compilation or existing manual match.
- **Class:** Design risk.
- **Severity:** **Medium** — Locally grouped compilations/remasters/disc layouts with inconsistent evidence can fragment browse/queue albums.
- **Fix direction:** Use stable release evidence or bounded folder-level grouping with explicit ambiguity/edition handling; keep manual matches authoritative. Touch catalog audio album identity and metadata match UX. (**Size:** M).
- **Related:** Q-24.21 asks for album structural acceptance.
- **Backlog:** Related only: “Untagged albums in the main Albums grid”; these tagged split cases are not listed..

### B-10.21 In Firefox every lossy track is converted to FLAC

- **Claim:** Gecko cannot directly decode lossy tracks through the effects engine.
- **What I traced:** apps/portico-server/web/src/bridge/audio/track.ts:23,28,40; apps/portico-server/web/src/bridge/audio/capabilities.ts:31,39; apps/portico-server/server/internal/playback/audio_plan_v2.go:185,191,205; apps/portico-server/web/test/audio-capabilities.test.ts:8.
- **Verdict:** **Confirmed, narrower** — Gecko’s conformance table intentionally has no WebCodecs families, so AAC/MP3/Opus/Vorbis are not declared for direct decoded-audio playback. FLAC conversion is selected when allowed and not bitrate-limited. “Every lossy track is converted to FLAC” is false under conversion-disabled/active-limit cases, where the effects plan is unavailable and ordinary playback remains. This is the effects path, not proof Firefox cannot play MP3 normally.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium** — Gecko users can incur FLAC bandwidth/CPU or lose effects; owner limits are still respected.
- **Fix direction:** Add verified Gecko decoder/conformance support or a supported bounded compressed conversion; touch web audio capability/decoder tables and server audio plan. (**Size:** M).
- **Related:** B-10.11 is the related missing limited-conversion route.
- **Backlog:** No.

### B-11.6 Gzip guide files are not decompressed

- **Claim:** Gzip guide files without an HTTP gzip encoding are not decoded.
- **What I traced:** apps/portico-server/server/internal/livechannels/adapters.go:18,37,39,52; apps/portico-server/server/internal/remotemedia/client.go:97; source fetch passes resulting bytes to parse.
- **Verdict:** **Confirmed, narrower** — HTTP Content-Encoding:gzip is explicitly decompressed with independent compressed/expanded bounds. A .gz file served as a compressed resource with absent/identity Content-Encoding is not sniffed or decompressed; it reaches XML parsing as binary data. The source entry’s body distinguishes this, but the title alone implies no gzip support at all.
- **Class:** Missing feature mislabelled as a bug.
- **Severity:** **Medium** — Owners of compressed guide-file endpoints without a transport encoding header cannot import them.
- **Fix direction:** Detect gzip resource bytes/content type in bounded fetch, retaining both byte budgets and avoiding double decoding. Touch livechannels/adapters.go. (**Size:** S).
- **Related:** B-11.3 concerns charset/time forms rather than compression.
- **Backlog:** No.

### B-11.9 Owner settings that are saved but do nothing

**Confirmed · Unfinished feature · Medium · Size M.** apps/portico-server/server/internal/administration/live_sources.go:17,77,338,439 persists settings/map overrides; only its own admin projection reads numbering/map fields. Production livechannels/linear fetch/planning does not consume them; apps/portico-server/server/internal/remotemedia/client.go:98 hardcodes User-Agent. Logo import and discovery are wired exceptions. Fix direction: Connect operational settings to the canonical fetch/ingest/playback models or disable unfinished controls. Touch administration live settings, livechannels adapters/directory and linear runtime. Related: B-11.8 shares the unwired guide mapping. Backlog: No.

### B-11.12 A server restart during a recording loses the rest of the programme

**Confirmed · Deliberate trade-off · Medium · Size L.** apps/portico-server/server/internal/livechannels/dvr/worker.go:253,451 calls Recover for interrupted capture; apps/portico-server/server/internal/recordingmedia/driver.go:319,344 validates existing bytes and returns recovered-partial, never retunes. The restart contract preserves partials and architecture-amendments.md:89 forbids pretending missed live bytes were recovered. Fix direction: Optional improvement: resume nonexpired airtime into a fenced partial with explicit missing intervals; touches DVR recovery, recordingmedia and publication. Related: B-11.11 shares no reconnect/continuation behavior. Backlog: No.

### B-11.17 Live recovery stops after five failures and the counter does not reset

**Confirmed · Defect · Medium · Size S.** apps/portico-server/server/internal/playbackruntime/linear.go:330,368,416 accumulates producer failures with no healthy-period reset. Selection/retry revision changes reset it (:349,353); linearSelectionKey at :1088 excludes ordinary live programme changes, so five failures within one unchanged source generation exhaust recovery. Fix direction: Reset a consecutive-failure budget after sustained healthy output; retain bounded retries for continuous failure. Touch linear runtime recovery. Related: No separate duplicate found. Backlog: No.

### B-11.21 Measuring storage stalls every running capture

**Confirmed · Design risk · Medium · Size M.** apps/portico-server/server/internal/recordingmedia/storage.go:31 holds writeMu through WalkDir/create/sync health probing; :126 needs the same lock for every capture write. apps/portico-server/server/internal/livechannels/dvr/storage.go:99 and recordingmedia/driver.go:480 call Measure. The stall mechanism is definite; duration or dropped bytes is unmeasured. Fix direction: Measure without holding the write lock through filesystem traversal, reconcile counters briefly and keep floor checks safe; touches recordingmedia storage. Related: Q-24.1 is a different DB scan-contention question. Backlog: No.

### B-11.22 A full disk at start time fails the recording before retention can help

**Confirmed · Defect · Medium · Size S.** apps/portico-server/server/internal/livechannels/dvr/worker.go:181,326 terminally fails a due claim on storage floor/cap before its later maintenance sequence (:115) runs retention/deletion. Retention can help only when eligible safe candidates exist; no emergency deletion is implied. Fix direction: Run eligible retention/deletion before terminal rejection and recheck once, preserving Keep/reader protections; touches DVR worker/storage. Related: B-11.21 is independent filesystem measurement contention. Backlog: No.

### B-11.25 An "all episodes" rule records every airing of every episode

**Confirmed · Missing feature mislabelled as a bug · Medium · Size M.** apps/portico-server/server/internal/livechannels/dvr/reconcile.go:148,184 schedules each matching programme ID; RuleConfig.Matches at model.go:127 never compares episode identity. Per-owner programme uniqueness prevents repeated scheduling of one airing, not rerun dedup. Fix direction: Add explicit unique-episode versus all-airings rule semantics using reliable episode evidence; touches DVR matching, retained episode index and rule editor. Related: No separate duplicate found. Backlog: No.

### B-11.26 Two profiles recording the same programme use two tuners and store it twice

**Confirmed · Deliberate trade-off · Medium · Size L.** apps/portico-server/server/internal/livechannels/dvr/worker.go:352 acquires by recording ID; model.go owner tuple and baseline schema 0001_baseline.sql:3046 deduplicate only per owner/source/programme. Contract live-tv-and-dvr §6 explicitly gives every recording one allocation until safe sharing is added. Fix direction: Optional shared capture with separate profile-owned access/retention references; touches allocator, recordingmedia artifacts and DVR ownership. Related: B-11.10 compounds conflicts; policy choices agree. Backlog: No.

### B-11.27 The pre-emption countdown is not delivered

**Confirmed · Unfinished feature · Medium · Size M.** apps/portico-server/server/internal/livechannels/allocator.go:71,152,195 stores PreemptAt/deadline; searches found no playback event/DTO/client countdown consumer. DVR worker’s tuner-preemption-countdown reason is internal retry state. Contract live-tv-and-dvr §6.1 requires a typed event and visible countdown. Fix direction: Publish the exact reservation deadline/event in channel session state and render it on clients before fenced pre-emption; touches allocator, playback DTO/events and client-core/channel players. Related: No separate duplicate found. Backlog: No.

### B-11.29 "Hide channel" is offered to owners but hides only for the owner's own profile

**Confirmed · Defect · Low · Size S.** apps/portico-server/server/internal/livechannels/preferences.go:20,66,74 stores per-profile hidden state as contracted; apps/portico-server/web/src/screens/channels/Channels.tsx:239,287 restricts Hide/Show hidden to owner. The server behavior is correct, but members cannot control their own hidden list; an implied global-hide promise is not established by label alone. Fix direction: Offer personal Hide/Show hidden to permitted viewers; distinguish any owner global-management action. Touch Channels.tsx and shared/native menu policy. Related: No separate duplicate found. Backlog: No.

### B-11.30 A programme the profile may not see can still be watched

**Confirmed · Defect · Medium · Size M.** apps/portico-server/server/internal/httpapi/live_channels.go:374 masks programme metadata by RatingAllowed; apps/portico-server/server/internal/playbackruntime/linear_resolver.go:92,120,154 tunes live without current-programme rating validation, whereas :174 checks library-channel item visibility. AllowLiveTV/source membership remain enforced; no recorded exception removes programme restrictions (contract live-tv-and-dvr:265). Running CheckTx at linear_resolver.go:207 also delegates to liveSelection without a rating check. Fix direction: Apply current programme restrictions at tune, running checks and programme boundaries with truthful blocked state; touches live resolver/contentaccess and channel session projection. Related: B-01.10/11 concern rating configuration/vocabulary, distinct from this enforcement gap. Backlog: No.

### B-11.32 Hourly refresh keeps up to a day of full guide copies

**Confirmed · Deliberate trade-off · Low · Size S.** apps/portico-server/server/internal/livechannels/generation_retention.go:12,22,34 retains generations younger than 24h by creation time, then deletes superseded rows in bounded batches. Hourly imports can therefore retain roughly a day of full copies; this is bounded grace, not unbounded growth, and created_at differs from time superseded. Fix direction: Use a measured request/cursor grace or pinned-generation reference policy; touches generation retention and guide continuation rules. Related: No separate duplicate found. Backlog: No.

## Not re-checked (Low)

These IDs/titles are copied from the worklist; no second-review verdict is supplied.

- **B-10.9** — End padding longer than one frame is under-counted
- **B-10.18** — ReplayGain stored in APEv2 tags on MP3 files is not seen
- **B-10.20** — On the web, a gapless join between tracks that need resampling is two filtered edges
- **B-10.22** — Android cannot raise quiet tracks
- **B-10.23** — AAC with only an edit list: the end is not trimmed, and Apple may cut the start
- **B-10.24** — Embedded cover art inflates the "bitrate" used for the limit test
- **B-10.25** — The instant mix pads from the same 192 songs every time
- **B-10.26** — Tracks with no disc number sort before disc 1
- **B-10.27** — Normalisation comes and goes while analysis catches up
- **B-10.30** — The web MP3 reader gives up if audio does not start within 8 KB after the ID3 tag
- **B-10.31** — Any <track name>.txt is offered as lyrics
- **B-10.32** — The lock-screen entry for an audiobook has no author
- **B-10.33** — Small queue edge cases
- **B-11.23** — Each finished recording is hashed twice, one result discarded
- **B-11.24** — dd_progid gives movies and one-off programmes a "series"
- **B-11.28** — HDHomeRun discovery returns addresses only, and "already configured" is never true
- **B-11.31** — The channel list sorts the whole line-up for every page
- **B-11.34** — Time ruler labels in zones offset by 45 minutes
- **B-11.35** — The default timeshift window is shorter than it sounds at broadcast bitrates

## New findings

None. Additional impacts and counterexamples found while tracing are incorporated into the existing IDs rather than counted twice.

## Disagreements worth Justin's attention

- **B-11.8:** “Cannot have a guide even through the API” is disproved. Explicit GuideURL/mappings work structurally for HDHomeRun. Plan the missing setup/edit integration, not a new server guide subsystem.
- **B-11.13:** A corrected start does not automatically destroy a programme identity. The explicit provider-ID path and correction test preserve it. The high-impact residual is interval-only guide identity and the absence of a user-confirmed resolution.
- **B-11.10/26:** Padded conflicts and separate capture allocations are contract choices. Soft padding or shared capture require a product-policy change; do not fix them as accidental scheduler violations. The stock HDHomeRun draft also does not automatically enforce discovered capacity.
- **B-10.1:** The generic threshold is specified, but resetting a still-listening book resume position is not explicitly justified. Preserve resume independently from classification. If Justin intends the shared threshold to discard audiobook positions too, this becomes a deliberate trade-off with the stated High residual impact.
- **B-10.2:** Budget exhaustion is real and duplicates B-06.4. It is not uniformly silent: audio metadata issues are set, with misleading invalid-cover wording.
- **B-10.5/6/14/17/21 and B-11.6:** Provider calls can resume after cooldown; synced candidates take precedence over plain results; plural/JSON/OPF narrators exist; compilation tags/manual album matching avoid some splits; Gecko effects can become unavailable instead of converting; HTTP gzip decoding exists. Preserve these counterexamples in implementation planning.
- **B-11.3/4/5/7:** High → Medium. Unsupported timestamp/charset forms, strict row validation and fixed safety bounds deserve compatibility work, but prevalence and broad impact were overstated or unestablished. Failed refresh retains the old guide.
- **B-11.15:** Medium–High → Medium, conditional on unknown source bitrate and the device capability. Decoder maxima are unsuitable encode targets, but “always converted” is not established.
- **B-11.20/29/32:** Low–Medium → Low. These concern conservative completion copy, member access to personal hiding, and bounded retained guide copies. They do not establish lost programme media, global-hide enforcement failure or unbounded guide growth.
- **B-10.13/16, B-11.22/33:** Low–Medium → Medium (selection of the upper end of the original range). Interactive full-prefix conversion, loss of chapter navigation, terminal storage rejection and potential repeated refresh expiry have concrete affected workflows. B-11.33 remains unmeasured.
- **B-11.30:** Retained at Medium as a genuine enforcement gap. Unreliable upstream ratings are not a documented exception to the existing profile restriction; first-party guide masking is not a playback fence.

## Evidence boundary

This was read-only static review of the current Ubuntu working trees on ehler-homelab-mini-1. Only this deliverable under Portico was written; temporary review scripts/data were written under /tmp. No Portico server, build, tests, formatter, installation, browser, computer-use/MCP action, hosted endpoint or comparison-repository code was executed. No throw-away behavior experiment was needed or run. Tests cited here were read, not executed; a fixture/test expectation is not a reproduced result.

The shared context-sync command was attempted as instructed but could not create its lock outside the writable sandbox. Existing project and verification guidance were read directly. The supplied audit briefs governed access and scope; no private authentication/state files were read.

The severity/size estimates assume ordinary pre-release use and the stated owner-choice philosophy. Backlog status means the checked Backlog.md contains no exact item unless a related entry is expressly noted; it is a dated list, not evidence a fix cannot exist elsewhere. B-index.txt was searched for duplicates and related impacts; B-06.4 and B-04.23 should be merged with B-10.2 and B-10.16 respectively.

Runtime uncertainty remains for exact HE-AAC signaling/browser combinations, long-file timeout thresholds, actual capture/retune interruption, HLS variant/FFmpeg behavior, native capability values, database pressure, padded-start labeling frequency and near-limit refresh time. Cheapest follow-ups are specified in the relevant Tier 1 entries. Nothing here establishes provider prevalence, real-device acceptance or 5–10 million-item performance. **Unreached Tier 1/Tier 2 IDs: none.**
