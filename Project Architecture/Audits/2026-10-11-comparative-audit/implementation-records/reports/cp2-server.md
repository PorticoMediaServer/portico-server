# Lane report: cp2-server — container merges, WebDAV retry, Watch Together scale, Complete scan cost

Worktrees `server-cp2-server` and `native-cp2-server`, branch `audit-implementation/cp2-server` in both, from `6ae3ccf4` / `ea762a7`. `audit-implementation/main` was merged at the end in both (it had no new commits at that time: "Already up to date"). Nothing pushed. Ran on personal Ubuntu mini-1. Testing was kept narrow on the coordinator's instruction (type-checks, builds, the tests of the code changed, one before/after run per measurement). Nothing was seen on a screen or run on a device.

## 1. Outcome

- **Whole albums, books and artists merge.** The existing "these two are one" (`POST /v1/admin/titles/merge`) now takes two albums, two books or two artists of one library. The one that goes hands its children to the one that stays: a song (or book file) the stayer already has at the same disc and number *and with the same name* becomes one song with two versions; the rest move as they are (a deluxe edition's bonus tracks join the album). Merging artists moves their albums (an album the stayer already has under the same name, year and a compatible edition merges in turn) and every song credit. Everyone's state for the container moves with it: favourites, finished marks, plays, a book's resume point (the stayer's wins where both exist), collections, lists, saved entries, series/groups a book is in. An album artist left with nothing goes (retired if it has state of its own). Every name the absorbed container was known by now finds the stayer, and a rescan keeps the files where the merge put them (albums: scanning a merged album's files now looks the album up by its name in the identity ledger first and does not split it off by edition or rewrite its key).
- **Owner UI.** Web: select two albums, two books or two artists in a library grid → Merge (the existing selection bar action, now offered for these kinds) with a container-specific explanation. Phones/tablets: the album, book and artist pages get a **Merge** row (same sheet as films/shows: search the other by name in the library, choose which stays, Merge); after the merge the page that stays opens.
- **WebDAV commands re-read and resend silently.** Removing a share that was edited elsewhere is re-read and removed at its new generation; a share already removed elsewhere counts as done. Editing a share that was changed elsewhere carries the owner's edit onto the share as it is now, field by field (name, "unencrypted local", connection); only a field both changed is a conflict. In client-core (`RemoteSourceService`), so web and native both have it. The rclone mount silent retry moved from the web screen into client-core (`sendMountCommand`) and native now uses it too (native mounts had no retry before).
- **B-13.10 Watch Together at the room ceiling (32 members).** Benchmarked a full room where every member reports readiness and catches up on events once a second. Fixed what grew with the square of the room: every report used to be re-broadcast to everyone (N reports × N readers) even when nothing anyone sees changed, each report materialised the whole member list to count readiness, and each catch-up read did one privacy query per event. Now an unchanged report (same readiness, same room summary, member still fresh) is not re-broadcast, counts come from one aggregate query, and a catch-up reads the room's privacy once. Per-member cost is now roughly flat (table in §3).
- **B-04.35 Complete scan cost.** Measured Default vs Complete per file on generated media, then implemented shared decoding: the audio is decoded once (48 kHz stereo) for waveform, fingerprint and loudness (loudness is now measured in Go, EBU R 128 / BS.1770-4, instead of a second ffmpeg pass), and when trickplay and scene detection are both due, the video is decoded once at the scene rate and tile size, serving both. Complete is ~2.5–2.8× cheaper per video file (table in §3). Scan work stays in the background analysis worker, off the write gate, bounded as before.

## 2. Decisions I made

1. **Container merge matches a child by disc, number *and name*,** not number alone: track 5 of a deluxe edition is not track 5 of the standard one. Same-number different-name children are kept side by side.
2. **Provider matches don't block a container merge.** Two editions/printings are routinely matched to different MusicBrainz/provider releases; the one that stays keeps its own match and the other's conflicting identities are dropped. (Film/show/episode merges still refuse conflicting provider identities, unchanged.)
3. **Split/undo, consistent with title merge:** title merges have no undo, so container merges have none either (stated in the confirmation text: "the two stay merged"). A version that should not have joined can still be split off with the existing Split on the title's files (it stays on the album/book it is in). Recorded honestly in §9.
4. **One merge moves at most 5,000 children** (songs, files, albums or credited songs); beyond that it is refused with "these are too large to merge in one step" (`ErrMergeTooLarge`, a request-input error) to keep the write short.
5. **Durability by the identity ledger, not by pinning.** The merge does not lock MusicBrainz matching or add per-file manual links (which would freeze tag updates); the album that stays answers to both names.
6. **WebDAV edit retry resends the same request once**, including a newly typed password, because a conflict is a definite refusal (nothing was applied) and it is the same explicit request; nothing is stored or queued. Ambiguous outcomes still never resend credentials.
7. **Readiness re-broadcast rule:** a report is broadcast when the member's readiness or playback epoch changed, the room summary changed, or the member's previous report was stale (>30 s). Positions of other members on screen therefore update when something changes rather than every report; snapshots carry current positions. A new `social_groups.readiness_summary` column holds the last summary told to the room.
8. **Loudness is measured on a 48 kHz stereo downmix** (one decode for every audio analysis). Multichannel films are measured as the stereo the decoder downmixes to, and mono is measured as dual mono, not as the 5.1 or single-channel measurement ffmpeg's ebur128 made; true peak uses a short 4× interpolator (within about 0.5 dB). Waveform/fingerprint read an 11025 Hz mono stream downmixed in Go (4th-order low-pass + interpolation) instead of ffmpeg's resampler, so fingerprint values differ from before (pre-release; results are recomputed per algorithm).
9. **Shared video decode only when the trickplay interval is a whole number of scene samples** (the usual case: 1 s scenes, whole-second trickplay interval); otherwise, or when only one of the two is due, each runs its own decode as before. Scene pictures in the shared path are the mean luma of the tile's cells rather than ffmpeg's 32×18 scale (heuristic detector, owner-reviewed markers).
10. Deleted the superseded decoder recipes `pcm` and `loudness` (pre-release rule); `pcm_shared` replaces both.

## 3. Audit bugs closed and decision gaps closed

| ID / gap | What changed | Test |
|---|---|---|
| cp-owner-parity decision 10 (album/book/artist merge) | `catalog/container_merge.go`; `MergeTitlesTx` takes Album/Book/Artist; commitAudio honours merged album names; client-core `MERGEABLE_KINDS` + `CONTAINER_MERGE_KINDS`, `mergeExplanation`; web SelectionBar; native Entity page Merge row | Go `TestAlbumMergeMovesTracksAndHoldsAcrossScans`, `TestArtistMergeMovesAlbumsAndCredits`, `TestBookMergeMovesFilesAndResume` (each rescans after the merge); native `owner-rules` |
| cp-owner-parity §9 WebDAV silent retry | `RemoteSourceService.configure(id,input,base)` / `remove` re-read and resend; `rebaseDavEdit`; `alreadyDone`; `sendMountCommand` shared | client-core `remote-sources.test.ts` (2 new tests), web `storage-commands.test.ts` |
| B-13.10 | readiness not re-broadcast when unchanged; `readinessSummary` aggregate; `roomPrivacy` once per catch-up | `BenchmarkRoomRound` (before/after below); social package tests |
| B-04.35 | `mediaanalysis/shared_audio.go`, `shared_video.go`, `RunOne` runs due stages of one group together; decoder `pcm_shared` | `TestSharedAudioMeasuresReferenceLoudness` (997 Hz at −20 dBFS → −20 LUFS, true peak 0.1), mediaanalysis and decoder package tests; `TestScanCostPerFile` (opt-in measurement) |

**B-13.10 — one room round (every member reports once and catches up), µs per member-report** (`go test ./internal/social -run '^$' -bench RoomRound -benchtime 30x`, i5-10500T):

| Members | Before | After |
|---|---|---|
| 4 | 1342 | 1150 |
| 8 | 1197 | 990 |
| 16 | 1621 | 946 |
| 32 (ceiling) | 2272 | 1333 |

What remains growing is the full member list each reporter gets back in its response (the group snapshot contract): O(N) per report, not O(N²) per member.

**B-04.35 — per-file cost, one run** (`PORTICO_SCAN_COST=1 go test ./internal/mediaanalysis -run ScanCost -v`; generated media served over HTTP as the bridge serves it; the machine was busy with other lanes, so absolute times are noisy, the ratios are the point):

| File | Default (probe) | Complete before | Complete after |
|---|---|---|---|
| Film, 3 min 1080p H.264 + 5.1 AAC | 0.08–0.13 s | 27.8 s quiet run / 53.2 s busy run (trickplay 26.7, scenes 20.4, waveform 0.7, loudness 5.4) | 19.4 s busy run (one video decode 17.1, one audio decode 2.2) |
| Episode, 2 min 720p H.264 + stereo AAC | 0.06–0.5 s | 28.3 s busy (trickplay 17.2, scenes 8.4, waveform 0.8, loudness 2.0) | 10.6 s busy (video 9.3, audio 1.2) |
| Music, 4 min FLAC | 0.05–0.08 s | 4.8 s busy (waveform 0.8, loudness 4.0) | 2.3 s busy |

Per-file cost of Complete still scales with the file's length (it decodes the whole file once per kind instead of four times); Default stays a header read.

## 4. API changes

- `POST /v1/admin/titles/merge` (`merge_titles`): same shape; now accepts two albums, two books or two artists; may answer a request-input error when a merge would move more than 5,000 children. Summary text updated; contracts regenerated (`go run ./cmd/apigen`).
- No other route changed. Readiness events are now sent only when something changed (same event shape).
- client-core: `RemoteState` gained `errorCode` and `alreadyDone`; `RemoteSourceService.configure` takes an optional `base`; new `rebaseDavEdit`, `sendMountCommand`, `CONTAINER_MERGE_KINDS`, `mergeExplanation`; `davCommandSucceeded` accepts `alreadyDone`.

## 5. Schema changes (baseline only)

- `social_groups.readiness_summary TEXT NOT NULL DEFAULT ''`. `scripts/schema-sync.py` run. Development databases need recreation.

## 6. Settings added

None. New string: `web.merge.containerBody`.

## 7. Tests run

- Go: `go test ./internal/catalog -run 'Merge|Audio|Album|Book'` ok; `go test ./internal/httpapi -run Title` ok; `go test ./internal/social` ok (+ benchmark); `go test ./internal/mediaanalysis ./internal/decoder` ok; `go vet` on touched packages clean; `go build ./...` and `GOOS=windows go build ./...` ok; apigen re-run gives no diff; schema-sync ok.
- client-core: `remote-sources.test.ts`, `title-identity.test.ts` → 15/15.
- Web: `lint:hooks` (oxlint), `lint:strings` (after fixing a glossary hit), `tsc --noEmit` clean; `storage-commands.test.ts` 6/6. Full `npm run check` / `npm test` not run (orchestrator).
- Native: `npm ci --legacy-peer-deps` (plain `npm ci` hits an ERESOLVE on react-native-qrcode-svg), `tsc --noEmit` clean, `lint:hooks` clean, `owner-*.test.ts` 41/41 (one expectation updated: albums/artists/books now have a merge search group). Full native suite, Vega not run (no Vega change).
- Shared packages mirrored with `--write`; lock records server `9e3ba767`.
- GOCACHE deleted at the end.

## 8. Device test notes

1. **Album merge (web).** Music library with the same album twice (e.g. "Record" and "Record (Deluxe)", same artist). Select both album cards → Merge → keep "Record". Pass: one album; the shared tracks show two versions each in their Files list; bonus tracks are on it; a favourite set on the deluxe one is on the kept one. Rescan the library: still one album. Fail: a second album reappears after the scan.
2. **Artist merge (web and phone).** Two spellings of one artist ("The Band", "Band, The"). Phone: open one artist page → Merge → search the other → keep one → Merge: the kept artist page opens with both artists' albums and songs; the other is gone from Artists; rescan keeps one artist.
3. **Book merge (phone).** Two copies of one audiobook; start listening to the one you will merge away; merge it into the other: its files are on the kept book and Resume continues where you were (if the kept book had no resume point of its own).
4. **Too large:** merging two artists with >5,000 songs between them is refused with a plain message.
5. **WebDAV.** Two browsers on Server › Storage. In B rename a share; in A (dialog opened before the rename) toggle "unencrypted local" and Save: saved silently, B's name kept. In B and A both rename it differently: A gets the "source changed" message. In B remove a share; in A remove the same share: done, no error. Same on a phone. rclone mount Stop on a phone right after it stopped elsewhere: no error.
6. **Watch Together.** A room with as many devices as you have (ideally 5+): playback stays in sync, the readiness pill and member list still update when someone buffers/recovers; Play waits for everyone as before. Members' positions in the list update on changes rather than every second.
7. **Complete scan.** A film library on Complete: trickplay scrubbing previews, intro/credits candidates and loudness values appear as before; scan wall time per film roughly halves or better. Compare a loudness value with another tool (e.g. ffmpeg `ebur128` on a stereo file): within ~0.5 LU for stereo content; multichannel films will read like their stereo downmix.

## 9. Not finished

- **No undo of a container merge** (as for titles). Splitting a merged song off leaves it on the same album; there is no "move this song back to its own album".
- **Book merges are not durable against scans in every case:** files of a merged book are found again through the identity ledger (tested once), but `saveLocalBookMetadata` still rewrites the kept book's local metadata from whichever file was scanned last (pre-existing behaviour for books whose files disagree).
- **Merging authors** (book authors as browse entities) not built; authors follow the books' author text.
- **TV/Vega** have no merge UI (owner administration is phone/tablet only, by decision).
- **Watch Together:** the per-report response still carries the full member list (O(N) per report, contract-bound). No real-device room at 32 members was run.
- **Shared video decode** applies only when trickplay's interval is a whole number of scene samples and both stages are due in the same pass; chapter images still decode one frame per chapter separately. Shared-path scene detection and loudness were checked by unit tests and one synthetic run, not on real films; detector thresholds were not re-tuned for the cell-mean scene picture.
- Web/native screens not seen on a screen.

## Commits

portico-server (`audit-implementation/cp2-server`): `4e3c2c06`, `556a5b9c`, `fda6bb59`, `92bcdcab`, `9e3ba767`.

portico-react-native (`audit-implementation/cp2-server`): `69d4e0f`, `25f058e`.
