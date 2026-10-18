# Music and audiobooks

## Outcome

The lane now has its own bounded audio metadata reader, wired into the retained-root storage helper, scanning and local lyric discovery. It reads ID3v2.2/2.3/2.4, ID3v1 fallback, FLAC/Vorbis, Ogg/Opus comments, MP4 metadata, APEv2, RIFF/WAV and AIFF chunks. It retains genuine multiple artists, album artists and genres, sort names, numbers and totals, recording/release/original dates, composer, compilation, MusicBrainz identifiers, ReplayGain/R128, typed embedded pictures, plain/timed lyrics, audiobook fields, and ID3/MP4 chapters. MP4 chapters include both Nero chapter atoms and referenced QuickTime text tracks. DSF's ID3 metadata pointer is also supported. No dependency was added. ffprobe's scan output omits descriptive format tags and retains stream facts, stream labels/languages and chapter fallback information.

Local lyrics appear when a song is opened. Valid local LRC lines survive odd lines; the published provenance tells the owner when lines were skipped. Sidecars outrank embedded lyrics, then synchronized lyrics outrank plain lyrics within that source. Default session selection is automatic only before the viewer has made a selection or explicitly cleared it. Curated uploads and private lyrics retain their authorization and immutable history. Bulk LRCLIB publication requires matching artist, title and duration within two seconds. Retagging keeps lyrics, and replacement timing ranges remain eligible for reacquisition.

Albums group by MusicBrainz release ID when present, otherwise album artist/title/year across folders. A release ID on only some tracks enriches one compatible existing album rather than splitting it. Compilation flags group track credits under Various Artists. Artist, album and track sort tags are projected when unambiguous. Structured genres preserve punctuation. Audiobook parts retain tag numbering; natural filename order is the fallback, and an explicit sort tag wins. Narrator falls back to composer, including multiple composers.

Audiobook completion uses time remaining and preserves the stopping position, including at the real end. New resume sessions and book selection rewind after a long absence; foreground pause/resume uses the same 15/30-second comfort rule. Shuffled play-next, removal and moves preserve played/upcoming order through bounded spans over the permutation. Growing selectors keep their insertion position. Repeat-all Previous wraps; repeats keep the same permutation.

Audio facts are requested durably in the background rather than repeatedly spending five seconds on canceled foreground decodes. Requested files precede the normal backfill. Padding across packets is summed; MP4 sample-table duration supplies AAC end padding. Codec profile and audio-only bitrate are persisted. The web's AAC-LC decoder declaration cannot admit HE-AAC. Interactive conversion runs with playback priority and seeks near the target before trimming. A shared optional-renderer failure hands the same session and position to ordinary playback.

Local artwork remains content-addressed permanent data, with no 128 MiB total cap or whole-store directory scan. Every image retains the existing byte/dimension bounds. Conventional cover names are case-insensitive; disc folders can use their album parent's cover; front-cover picture type wins. Original PNG/JPEG bytes are retained.

## Decisions I made

- The own reader has an 8 MiB block limit, 24 MiB total read budget, bounded nesting/frame/atom/page counts, bounded field collections, eight pictures (at most 4 MiB each), twelve lyrics, and 4,096 chapters. Production scans run reads through the existing supervised helper and admitted root. File facts are checked after reading. Image and document safety limits remain.
- Genuine container list values are honored. Free-text artist punctuation is kept whole, including AC/DC; no guessed delimiter or “feat.” splitting. Known lists and sort/identifier evidence are retained for the identity lane.
- Audiobooks complete with at most 30 seconds left, capped at 5% of a short file's length. Resume is retained even after completion. Rewind is 15 seconds after five minutes and 30 seconds after one hour, clamped at zero. Explicit start positions remain explicit.
- Fallback album identity is album artist/title/year as directed. A valid release ID outranks different folders, artists and years. Different known release IDs remain separate. Existing manual album matches remain pinned. This worktree has the existing `EnsureEntityTx` attachment mechanism, not the identity lane's future hook.
- Lyric binding uses item/asset identity, duration and source range, not file size or modification time. Playback source pins still enforce size/mtime. A same-duration replacement under the same title/asset can retain lyrics; an acoustic fingerprint is not part of this lane.
- A matching `.txt` is owner-supplied lyrics. Recognizable extraction logs are excluded from automatic publication. Arbitrary prose cannot be reliably distinguished from lyrics by content.
- Shuffle repeats use a stable permutation. This avoids a repeat-lap boundary changing the last/first relationship, and Previous can return to that order. Switching shuffle off returns to source order; manual edits do not reset the played prefix while shuffle stays on.
- DSD and other newly admitted codecs use the existing bounded conversion/capability rules; no exclusive/bit-perfect output is claimed.
- No new settings screen or wall of switches. Existing defaults remain: normalization off, gapless on, crossfade off, LRCLIB off until enabled. Completion and rewind are listening rules.
- Ran on personal Ubuntu mini-1. Context sync was attempted but its lock path is read-only in this sandbox; existing canonical guidance was readable and used.
- The ID3 frame definitions were checked against the primary [ID3v2.4 frame specification](https://id3.org/id3v2.4.0-frames). No comparison-product code was copied.

## Bugs

- B-10.1 — fixed: time-remaining completion; resume survives both the threshold and ended. The earlier resume-only fix was preserved and completed.
- B-10.2 — fixed: removed the permanent artwork store's global cap and directory-size scan; per-image safety limits remain.
- B-10.3 — fixed: play-next inserts immediately after current under shuffle, including ordered client batches.
- B-10.4 — fixed: sparse permutation spans preserve played/current/upcoming order through removal and moves. Stable entry IDs survive shuffled moves.
- B-10.5 — already fixed: existing provider pause/cursor behavior retained and its targeted regression passed.
- B-10.6 — fixed: bulk candidates require title/artist/duration identity, plus timing validation.
- B-10.7 — fixed: retagging no longer changes lyric timing identity; bulk exclusion and publication dedup respect current asset/range/timing. Immutable resource timing columns are included in the transition fence.
- B-10.8 — fixed: odd LRC lines are skipped; invalid encoding, excessive sizes/expansion and other safety bounds still reject.
- B-10.9 — fixed: end discard is summed across all packets. Canned regression covers a split discard region.
- B-10.10 — fixed on the server: measured profile distinguishes HE-AAC; WebCodecs' qualified AAC-LC path requests conversion for complex profiles. Native profile/trim conformance remains a device task.
- B-10.11 — not fixed: a bitrate-limited Apple audio renderer still needs a supported compressed conversion target. The misleading reason is corrected; implementing and qualifying AAC/MP4 across conversion and the native renderer belongs to the playback/native lanes.
- B-10.12 — fixed: foreground starts enqueue persistent facts; requested files run first and are not discarded at a five-second playback budget. Facts still use a full background decode; a general packets-first/header measurement optimization is not implemented.
- B-10.13 — fixed for tested conversions: playback priority and bounded input seeking with residual sample trimming. FLAC/MP3 samples were identical; AAC was aligned and differed by at most one 16-bit unit of decoder rounding. Other codecs need qualification below.
- B-10.14 — fixed: narrator/composer fallback and plural narrator/composer values.
- B-10.15 — fixed: prior part-number parsing retained; unnumbered files have natural filename fallback in selection, queues, paging and chapter listing.
- B-10.16 — fixed: retain valid chapters, clamp usable overruns, skip bad entries and preserve bounds; report a partial timeline issue.
- B-10.17 — fixed for the directed identity rules: release identity or album artist/title/year across folders, partial release-ID enrichment, compilation grouping and broader disc hints. Different fallback years and missing compilation/album-artist evidence can still split a poorly tagged collection; no speculative collection-wide merge or manual merge UI was added.
- B-10.18 — fixed: own APEv2 reader sees MP3 ReplayGain; ID3 RVA2 and R128 forms are also read.
- B-10.19 — not fully fixed: case, common names, parent covers and front-cover type are fixed. WEBP decoding and larger-than-bound images are not added; JPEG/PNG safety limits remain intentional.
- B-10.20 — not fixed: web resampler edge history is outside this lane.
- B-10.21 — not fixed: Firefox/Vorbis decoder qualification is in the web engine/capability lane; no guessed decoder support was advertised.
- B-10.22 — not fixed: positive Android gain requires the native audio engine.
- B-10.23 — not fully fixed: server AAC end padding uses MP4 timing; Core Audio's actual priming behavior still needs native conformance.
- B-10.24 — fixed: bitrate derives from audio packets/stream facts rather than embedded cover bytes.
- B-10.25 — fixed: seeded, bounded library candidate walk starts at varying pivots and wraps; normalized genre relations replace raw tag equality. Candidate budget remains 640.
- B-10.26 — fixed: missing disc sorts as disc one in queue/selection/paging and owned listening indexes.
- B-10.27 — not fixed: unavailable loudness cannot be guessed. Normalization still depends on tags/analysis and may vary while the analysis backlog fills.
- B-10.28 — not fully fixed: added hostile-input/container/chapter/seek/profile/padding regressions and fuzzing; the full device gapless fixture matrix is not qualified.
- B-10.29 — fixed for the formats named in the finding: audio intake now also admits AIF, OGA, MKA, WMA, APE, WV, TTA, CAF, W64, AMR, DSF, DFF, MPC and MP4; closed decoder format lists were aligned. Encrypted/unknown formats and generic unsupported-file notices are not new features here.
- B-10.30 — not fixed: the web MP3 demuxer's search bound belongs to the web engine. Shared fallback now prevents this optional engine's error immediately ending a session.
- B-10.31 — not present as a prohibited behavior under the brief: matching TXT files are explicitly requested as local lyrics. Recognizable rip logs are excluded; arbitrary owner prose remains ambiguous.
- B-10.32 — not fixed: the web Media Session author projection is a web bridge change.
- B-10.33 — fixed: Previous wraps under repeat-all; repeat laps retain one permutation and order history.
- B-10.34 — fixed in shared foreground playback: renderer failure switches to the ordinary player with the same session/position. Native-owner failures and server/device conversion escalation still need their owning lanes.

## API changes

No new or removed public route and no wire-shape change. Existing typed/registered routes keep their contracts.

- `GET /v1/items/{id}/lyrics`: automatically discovers local resources and selects a default for an untouched active session. No existing viewer choice is overwritten.
- Lyrics search/publication: tolerant documents; verified bulk LRCLIB identity; stable timing-based source version. The version remains an opaque hash.
- Queue add/remove/move/advance: corrected shuffle and repeat behavior; existing request/reply shapes.
- Listening selection and playback start: book resume comfort and natural file fallback; existing shapes.
- Audio-converted media: interactive priority and efficient seek; existing headers and URL.
- Capabilities: existing `audioDecode.via` is now retained internally to apply the web decoder's qualified AAC profile limits. No new capability document or field.
- Internal storage helper operations: `inventory-audio-tags`, `inventory-audio-cover`. They are internal closed operations, not public routes.

## Schema changes

Edited the baseline directly and ran `scripts/schema-sync.py`; no compatibility migration.

- `audio_render_facts`: `codec_profile`, `bit_rate` with defaults/bounds.
- `audio_render_fact_requests`: one durable request per asset and an ordered request index.
- `queue_v1_play_spans`: bounded ranges over shuffle/source positions. An empty span represents an explicitly emptied shuffled queue; cascading queue ownership remains.
- `lyric_resources`: immutable `source_duration`, `source_start`, `source_end`; timing is written on initial insert, and the existing resource-transition trigger now fences these columns.
- Music/book listening expression indexes: missing-disc fallback is one, matching their order queries.
- Generated known schema/columns and source digest synchronized. Identity schema/core was not redesigned.

## Owner or viewer settings added or wired

No new setting. Existing LRCLIB enablement, per-library local metadata policy, playback bitrate/capability limits, normalization/gapless/crossfade choices, and source read-policy guards remain effective. Audiobook completion overrides the generic played percentage for book files; there is no new screen for completion or rewind. The global artwork-cap setting did not exist.

## Needed from others

- Playback/conversion/native: B-10.11 supported Apple compressed target; native gain amplification; native ordinary-player fallback; accurate HE-AAC/edit-list priming; broad codec deep-seek qualification and gapless fixtures. Review `decoder.AnalysisSpec`'s new source-rate/priming fields and interactive key with the conversion lane.
- Web: resampler continuity, Firefox/Vorbis qualification, MP3 synchronization search and Media Session book author. The new shared fallback is available without a screen change.
- Native: ensure pause/resume comfort is enforced while JavaScript is suspended, using the same five-minute/hour thresholds; new server resume sessions already get the comfort rule.
- Identity: integrate the music-only album-key/sort/genre changes with the new title/file attachment point. Relevant existing mechanisms here are `commitAudio`, `EnsureEntityTx`, local edition evidence, music/book query ordering and the local genre projector. Preserve manual matches and state-bearing empty entities. This lane did not add an artist-identity merge or manual album merge tool.
- Naming: reconcile broadened disc-folder hints with the canonical path parser; filename hints remain subordinate to embedded disc/track tags.
- Provider matching: richer parsed identifiers/sort/series/description evidence is available; remote audiobook providers remain your lane.
- Documentation: update Playback Protocol §12/§17.10/§18.7 and the music/library metadata contracts for automatic local lyrics, time-remaining completion, shuffle edits, frame measurement scheduling and album identity. §18.7 still needs to distinguish durable full background decoding from a future packets-first implementation. No architecture documents were edited outside this worktree.

## Test notes for other environments

Verified here with limited checks, not a full suite/build programme:

- `GOFLAGS=-p=2 go build ./...` on Linux; Windows amd64 and Darwin arm64 cross-compiles.
- Targeted `go vet` on audiotags, audiofacts, lyrics, listeningrules, assets, decoder, storage, localmetadata, catalog and playbackv1.
- Targeted Go tests for each new regression plus existing lyric pause/publication/privacy, book progress, album cleanup/grouping, selection continuation/mix bounds and queue edit behavior.
- Real audio facts for synthetic MP3/AAC/Opus/FLAC/WAV, exact seek comparison, and `PORTICO_TEST_TIER=media ... TestRealLocalAudioScanChaptersArtAndBookResume` using qualified tools. The host blocks bubblewrap user namespaces, so the existing baseline decoder protections were used; no configuration was changed.
- TypeScript `npx tsc --noEmit -p packages/client-core/tsconfig.json`; individual listening and queue-player test files (49 existing tests initially, then the new rewind/fallback regressions).
- Tag parser fuzz: 63,410 executions in two seconds with two workers, no crash. Format tests cover all requested reader families; a separate QuickTime track regression passed.
- `git diff --check` passed. Schema sync's targeted schema tests passed.

Cold acceptance procedure:

1. Use a fresh database built from this baseline. Configure separate local-only music/book libraries. Copy synthetic tag variants in the formats above into admitted roots; run a normal scan. Compare artist lists, sort names, album artist, dates, totals, identifiers, ReplayGain/R128, series/narrator/publisher/description and cover type with the tags. Try oversized/truncated metadata and a bad chapter between good chapters: playback and other metadata should remain, with a local issue/provenance note.
2. Store one multi-disc album in different folders, first without a release ID, then with the same release ID on only some tracks. It should remain one album. Different valid release IDs should remain distinct. Scan unnumbered book parts out of order and page selections/chapters; order should follow the natural filename unless explicitly tagged otherwise.
3. Put LRC/TXT beside a song and embed an alternative. Open lyrics without a search/publish step; the best local source should appear. Retag the file and rescan; lyrics should remain. Change the timing range/duration; the old resource should not be selected and a bulk run should consider acquisition again. A wrong artist/title/duration LRCLIB result must never be published automatically. Check non-owner private/library authorization and explicit “no lyrics” selection.
4. On a 12-hour book, stop 30 minutes from the end: it must remain unfinished at the exact place. Stop within 30 seconds: classification can finish but progress remains. Resume after five minutes/hour: seek 15/30 seconds back. Check both a new session and a native-owned paused session, including background/suspension.
5. Under shuffle, advance several tracks, play-next, remove an upcoming track, move another, and return Previous. Heard order/current position should remain. Test a growing large selector and an empty final-entry removal. Repeat-all Previous should wrap without a fresh permutation.
6. With qualified FFmpeg and real players, seek deep into converted MP3, FLAC, AAC, Opus, Vorbis, ALAC, WMA/APE/WV and multi-hour files. Check prompt output and sample positions against full-prefix trimming. Run the device audio-conformance fixtures for HE-AAC, edit-list-only AAC, MP3 padding spanning packets, mixed output rates and no-LAME files. Native priming/gain and Apple compressed-target failures are unresolved below; do not treat cross-compilation as device acceptance.
7. On Windows/macOS, exercise retained-root tag/art/lyric reads, case-insensitive cover names, symlinks inside/outside the admitted root, unavailable mounts and source replacement during reads. Root escape, changed-file reuse and unbounded/stuck reads must still fail safely.

## Not finished

The main tag/grouping/lyric/chapter/queue/listening work is implemented and checked. Remaining programme dependencies are explicit above: Apple bitrate-limited compressed render target; native gain/priming/fallback and suspended rewind; web resampling/codec/demux/Now Playing fixes; a normalization-availability policy; WEBP artwork; and physical device/gapless/less-common-codec qualification. Facts still perform a full background decode rather than a general packets-first/header shortcut. No generic manual album merge, ambiguous compilation detection, acoustic lyric identity, or online audiobook provider was introduced.

Snapshots 001, 002 and 003 were taken at compiled milestones. Final cumulative snapshot 004 follows final compilation and regression checks, with subject “Keep music and audiobook listening accurate”. No lane-tree change is made after that snapshot. No commit, push or deployment was performed in this sandbox.
