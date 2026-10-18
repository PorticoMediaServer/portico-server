# cp4-leftovers — tonight's codeable leftovers

Host ehler-personal (mini-1), 15–16 Oct 2026. Branch `audit-implementation/cp4-leftovers` in `server-cp4-leftovers` (from 617dc62d) and `native-cp4-leftovers` (from a8c51ea). Committed with Justin's identity, never pushed. No subagents or helpers. Lean testing, as the brief asked. The coordinator stopped the lane at 23:50 on 15 Oct and resumed it on 16 Oct. The WIP commit `409df6f` was not offline Forced only, which was already committed as `07684e2`. It was the native half of the author merge. I split it into a mirror commit (`428ce46`) and the app change (`d2e4da6`). `audit-implementation/main` had not moved past the base in either repo, so there was nothing to merge. apigen shows no diff. The shared packages are mirrored up to server `0d68130b`; the last server commit touches Go only.

## 1 Outcome

Done (highest value first):

1. **Merged-away title ids are followed in request bodies and queries.** Before, only path ids followed them. There is one resolver at the API boundary, in apikit `RewriteRefs`.
   - It rewrites these JSON members wherever they sit in a typed or HTTP request body: `itemId`, `itemIds`, `addItemIds`, `showId`, `seasonId`, `selectedSeasonId`, `episodeId`, `entityId` and `logoItemId`, plus any member tagged `ref:"title"` (the queue selector's `container.id` and `items.ids`, and `albumIds`). The same names are rewritten in query parameters.
   - So starting playback, adding to a playlist or collection, deleting, Watch Together queues and metadata actions by an old id all reach the title that stayed. The ~380 SQL lookups were not touched.
   - Cost: a body that names none of these members is not parsed. On a server that never merged a title, one `EXISTS` probe answers, and the body passes byte for byte. A body with a duplicate member is left for the strict decoder to refuse.
2. **"Not this artist…"** (server, web entry menu, phone/tablet entry menu).
   - The album and its songs' credits move to a new artist with the same name. Identity never joins that artist back (`catalog_kept_apart`), and at least one album must stay with the old artist.
   - A later scan keeps the songs credited to the new artist: a song that names the artist the album left credits the one it went to.
   - New route: `POST /v1/admin/titles/separate-artist`.
3. **Native Downloads reads preparation progress again every 30 s** while the server is preparing something. It runs only while the screen is focused and the app is in front, which is the web's rule.
4. **Title-scoped refresh** for library lists (browse), container pages (album, artist and collection pages) and person pages, on web, phone/tablet and TV.
   - A personal hint now re-reads such a list only when it names a title the list holds in memory, or names no title.
   - A list whose filters or sort read personal state (played, favourites, watchlist, own rating, last played) still re-reads on every personal change.
   - Lists across libraries hear a no-title hint from any library through the new `personal:any:*` marker.
   - Native person pages had no live refresh at all. They now have one.
5. **"Trust this server" mid-session, and saved certificates** (phone/tablet/TV on Android and Apple).
   - A certificate question raised while signed in opens the same question the sign-in screen asks, wherever the person is. Trusting it re-reads every open screen. "Not now" isn't asked again for the same certificate this session.
   - Settings › Servers lists "Certificates you trusted" (address and fingerprint). Each one can be forgotten.
   - New native method `pinnedCertificates()` on Kotlin and ObjC/Swift.
6. **A pending Portico Account approval is picked up after a server restart.**
   - The wait saves its expected claim and a note of the owner sign-in that asked (the token digest the session store already keeps).
   - At start, the server waits on Hosted again under a guard rebuilt from that note. The guard holds only while that same owner sign-in is current, as before the restart.
   - New column `networking_web_claims.watch`.
7. **History past 10,000:** skipped, see §9.
8. **Native letter rail** on person pages sorted by title (phone/tablet beside the grid, TV in the header). Playlist pages: skipped, see §9.
9. **Forced only for downloads.** The bundle now carries a PGS or VobSub track's `lines {forced,total}`. The offline player offers Forced only where the streaming one does, and the offline command carries `forcedOnly`.
10. **MP3 seek index on Android when the track starts from 0.** The index is read in the background while the track plays. The first seek after it arrives opens the file again at the position, so the index extractor places it exactly. A queued gapless next item keeps the ordinary seek.
11. **Compressed DVB subtitle objects on the web.** These are EN 300 743 V1.6.1 coding method 2: an 8-bit indexed bitmap, stored as PNG-filtered scanlines in a zlib stream. They are decoded with our own bounded inflater and PNG unfilter (`web/src/player/subtitles/inflate.ts`). I checked the inflater against Node's zlib on 200 random streams at compression levels 0, 1, 6 and 9.
12. **Book author merge** (server, web selection, phone/tablet author page).
    - Merging two authors points the absorbed author's identity names at the author that stays, and derives its books' memberships again. Every book of either spelling ends up on one author, and a later scan keeps them there.
    - `author` added to the mergeable and container-merge kinds.
13. **Watch Together readiness reports** now answer with the room's state, revisions and counts (`ReadinessReceipt`), not the member list. Every client already ignored the body.
14. **Download menu for Dolby Vision profile 5:** only Original is offered. The optimized steps used to appear and then fail.
15. Two native `owner-structure` test expectations were stale: other lanes had added the skipped-records and devices panels on purpose. I updated them.

## 2 Decisions I made

- **One boundary resolver, by member name.** Rewriting is a no-op for any id that isn't a merged title, so a broad name list (including `entityId`) is safe. Explicit `ref:"title"` tags cover nested and odd names.
- **"Not this artist" is an album action,** and the new artist takes the old artist's name. The owner can rename it or merge it back. I chose not to fork the name from tags.
- **Title-scoped refresh** covers the titles held in memory (at most about 1,000 per list), not just those visible. A container page uses its first-page sections plus its entity id. A deeper appended page in a container page relies on its own windowed group.
- **The restart guard is tied to the exact owner sign-in** (token digest, account, profile, epoch). If the page refreshed its token during the wait, the resumed approval answers `approval_changed` and the owner presses Turn on again. That is the same rule as before a restart. I didn't weaken it.
- **Certificate "Not now"** is remembered per fingerprint for the session only.
- **Android MP3:** on the first seek after the index arrives, the file is reopened rather than playback interrupted, so the start isn't delayed.

## 3 Gaps closed (source → change → test)

| Item (source) | Change | Test |
|---|---|---|
| A8 coverage, request bodies (cp3-identity) | apikit `Refs`, `RewriteRefs`, `rewriteBody`/`rewriteQuery`; `Catalog.SurvivingTitleIDs`; httpapi `titleRefNames` | `apikit.TestRefsReachTypedBodiesHTTPBodiesAndQueries`; `catalog.TestShowMergeRunsInBatchesAndRedirectsOldIds` (extended) |
| A10 artist split (cp3-identity) | `catalog.SeparateArtist`, route, scan credit rule; client-core `separateArtist`, entry action; web panel; native ConfirmSheet | `catalog.TestOwnerSeparatesAlbumsFromAnArtist`; httpapi `TestOwnerListsEmptiesMissingTitlesAndRemovesALibrary` (probes); client-core `title-identity.test.ts` |
| Native download progress (cp3-api) | `Downloads.tsx` 30 s recheck while preparing | type-check |
| Title-scoped refresh (cp3-api) | client-core `listedTitlesWithPersonalState`, `readsPersonalState`, `titleIdsOf`, `browseFields`, windowed `residentItems`; web/native browse, content, person; dynamic `useFreshDocument` | client-core `freshness-hub.test.ts` (new case) |
| Mid-session trust + saved certificates (cp3-network) | `onCertificateQuestion`, `CertificateNotice`, Servers list, `pinnedCertificates` (Kotlin, ObjC, Swift) | native `certificate-trust.test.ts` (new case) |
| Pending approval after restart (cp3-securedefault) | `ResumeWebClaims`, `NetworkingClaimResume`, `identity.PrincipalByDigestTx`, `watch` column | `networking.TestWebClaimWatchResumesAfterRestart` |
| Letter rail, person (cp2-viewer) | `Person.tsx` with `useGroupLetters` | type-check |
| Forced only offline (cp2-native) | `DownloadTrack.Lines`, `BundleSubtitle.Lines`; client-core parse; native stored subtitle → `forcedChoice` → `forcedOnly` | client-core `downloads.test.ts`; native `offline-extras.test.ts` (new case) |
| MP3 seek from 0, Android (cp2-native, cp-followups) | `PorticoPlaybackController.kt` `unindexedMedia` | not compiled (no Gradle run) |
| Compressed DVB (cp3-web W6) | `inflate.ts`, `dvb.ts` method 2 | web `subtitle-images.test.ts` (new case); fuzz against node zlib |
| Author merge (cp2-server) | `mergeContainerTx` Author case, `authorFilesTx`; client kinds; native search group and Entity tools | `catalog.TestOwnerMergesTwoAuthors` |
| Watch Together member list per report (cp2-server) | `ReadinessReceipt`, `mutateGroupAs` | httpapi `TestSocialReadinessAggregation` (updated, asserts no `members`) |
| DV profile 5 download steps (cp3-outputs) | `Options` skips optimized rungs with no base layer | `downloads.TestDownloadMenuOffersOnlyTheOriginalForDolbyVisionWithoutABaseLayer` |

## 4 API changes

- New `POST /v1/admin/titles/separate-artist` (`separate_artist`, owner, ScopeLibrary): body `{albumIds}`, answers `{artistId}`.
- `POST /v1/groups/{id}/readiness` now answers `ReadinessReceipt {protocolVersion, serverTime, groupId, state, revision, playbackRevision, readiness}`, no longer `GroupResponse`.
- Download bundle `subtitles[].lines {forced,total}`, optional, for image tracks.
- `merge_titles` now accepts two authors (summary updated).
- Behaviour only: merged-away title ids in bodies and queries are replaced before the handler reads them.
- apikit: `Refs`, `Registry.RewriteRefs`; request fields may carry the tag `ref:"title"`.

## 5 Schema changes

- `networking_web_claims.watch TEXT NOT NULL DEFAULT ''` in the baseline; schema-sync run.

## 6 Settings added

None.

## 7 Tests run

- `go test ./apikit` → ok
- `go test ./internal/catalog/` (whole package) → ok (284 s), plus the new tests individually
- `go test ./internal/networking/ -run 'TestWebClaimWatch|TestClaimHandler'` → ok
- `go test ./internal/social/` → ok
- `go test ./internal/httpapi/ -run 'TestSocial…'` → ok
- `go test ./internal/httpapi/ -run 'TestOwnerListsEmptiesMissingTitles…|Lane|Scope|Restriction|Matrix|Generated|Contract'` → ok
- `go test ./internal/subtitles/ -run Download` → ok
- `go test ./internal/downloads/ -run TestDownloadMenu` → ok
- `go build ./...`, `go vet` on the touched packages, gofmt on touched files → clean. Two files in catalog were already unformatted and are not mine.
- `go run ./cmd/apigen` → no diff at the end.
- client-core `npm test` → 2020/2020.
- Web:
  - `tsc --noEmit`, `lint:hooks` and `lint:strings` → clean.
  - `subtitle-images.test.ts` → 14/14.
- Native:
  - `tsc` (app and tests), `lint:hooks` and `lint:strings` → clean.
  - `npm test` → 699/699 after the structure-test fix (2 stale expectations before it).
- Vega: `tsc` and `lint:strings` → clean.
- **Not compiled:** Kotlin (seek index, pinned list) and ObjC/Swift (pinned list).

## 8 Device test notes

1. **Merged ids:** merge two films, then from a client holding the old id press Play, add to a playlist, and delete. Pass: each acts on the title that stayed, with no "not found".
2. **Not this artist (phone and web):** on an album filed under a same-named artist, use the menu's "Not this artist…". Pass: a new artist page opens with that album. Rescan the library: the album and its songs' credits stay there.
3. **Title-scoped refresh:** with a library list open on one device, mark a title watched on another. Pass: the list updates only if that title is on it; a list filtered by "Unplayed" updates either way.
4. **Mid-session certificate:**
   - While signed in to a self-signed server, swap in a *different* server's certificate at the same address. Pass: the "Trust this server's certificate?" sheet appears; Trust brings screens back.
   - Settings › Servers lists the pin. Forget, then reconnect: it is proved and pinned again.
5. **Restart during approval:** start Turn on secure connection, restart the server before approving, then approve on the phone. Pass: the claim installs and the certificate is requested without pressing Turn on again.
6. **Offline Forced only:** download a Blu-ray rip with a mixed PGS track and play it in airplane mode with mpv. Pass: Forced only appears and works; it is absent for a forced-only track.
7. **Android MP3:** play an MP3 audiobook from 0 and scrub to minute 40. Pass: it lands on the exact word, the same as a resume.
8. **DVB UHD:** a UK UHD recording with progressive DVB subtitles shows them in the web player. Native mpv/FFmpeg and server burn-in were not changed (see §9).
9. **Watch Together:** a room of 3+ devices keeps the readiness pill and member list updating. The readiness POST bodies are small.
10. **DV profile 5:** the download menu for a profile 5 file shows Original only.

## 9 Not finished, and why

- **History sorted by other fields, or queued, past 10,000.** Sorting needs the viewer's whole history on every page, so cost grows with the history; it needs a per-sort index on the history rows. That is a read-path and performance change in browse_across, which cp2-reads owns tonight.
- **Native letter rail on playlist pages.** A sorted playlist there is a paged list of rows (`useGroupPages`) inside the playlist's own list. A rail needs it turned into a windowed list. Deferred.
- **More conditional-GET routes.** The remaining candidates are HTTP-registered (they set their own validators) or have no cheap complete revision, as cp3-api found. Nothing worth doing tonight.
- **Progressive DVB on native, mpv and server burn-in.** FFmpeg's DVB decoder is used there; supporting method 2 needs an engine or FFmpeg patch.
- **Android and Apple compile:** the Kotlin seek-index reopen and the pinned-certificate listing (Kotlin, ObjC, Swift) are unbuilt. Mac checks:
  - `PorticoServerTrust.allPinned` (`NS_SWIFT_NAME(allPinned())`);
  - `pinnedCertificates(_:rejecter:)` and its `RCT_EXTERN_METHOD`.
- **Skipped as out of scope or needing a product or engine change:**
  - first-run setup on phones;
  - paused-load.ts removal (playback-core);
  - mixed-script mpv fonts;
  - local people portraits and NFO cast on person pages;
  - W2/W1 page bounds (deliberate);
  - Roku snapshots (Roku is out);
  - docs refresh (docs lane).
- **No dedicated tests** for the native letter rail, the downloads recheck or `NetworkingClaimResume` (the networking test covers the resume with a fixture guard).

## Commits

**portico-server** (`audit-implementation/cp4-leftovers`): 950196a6, de3395b6, 4e0b7f72, 926e0560, d8cbc881, 8c67db64, 7578e339, 09933cb2, 656b4399, 0d68130b, 06cd8c64.

**portico-react-native** (`audit-implementation/cp4-leftovers`): 7c6a226, 71f3c4c, 83ab23a, 12c6909, d0a0cd2, 406c754, 857570f, a3c081d, 1e0d428, 07684e2, 9f30e10, 428ce46, d2e4da6, 2cb4b40.
