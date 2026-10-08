# Backlog

Known open work, as of 2 October 2026. Update this file when an item lands or a new one is found. Commits and branches named "(archive)" are in the pre-split repository bundle, `___BACKUP COPIES/Portico pre-split 2026-09-25/portico-monorepo-all-branches.bundle` (`git clone <bundle>`, then check out the branch).

## Before the next demo deploy

- **Demo:** runs portico-server 9cfe7fa (branch `frontend-rework`, deployed 3 Oct; `a13c11d` is the rollback; schema 10; its anime library moved to TMDB with that deploy, its play history starts with the first 2 Oct deploy) on the state set up 27 Sep (direct sign-in), with Movies, TV Shows, Anime, Music and Audiobooks from `/srv/portico-demo-20260919` and the "Demo IPTV" playlist source (14 channels). The pre-baseline state is `/var/lib/portico-demo-20260923.pre-e84c27f9`.
- **Browser and simulator pass of the recommendations UI.** The personal rows on Home ("More recommendations" in Customize Home), Discover personal rows, See all, title-page rows, Not interested with Undo, Reset recommendations, the For you sort, and saved views on Home. Covered so far only by component tests and type checks.
- **tvOS: Not interested reachability.** Recommendation cards expose it only through long-press. Check it with the Siri Remote and with VoiceOver/Switch Control; add a focusable More action if needed.
- **Library add form:** adding a library with local metadata only and `en` fails in the web form (seen during a browser test on 25 Sep).
- **Frontend rework (branch `frontend-rework`, 2 Oct):** not merged. Justin's open questions are numbered in `Audits/2026-10-02-frontend-rework-log.md`. Answered on 2 Oct and built: the hero (no Featured row), a linked TV's role, regional English, play history and play counts. All of the log's numbered questions were answered on 2 Oct (last section of the log: My List, one provider per library, the owner always sees plays, NFO cast with online crew); nothing is waiting on Justin except the merge.

## Recommendations

- **Title dataset release:** when the dataset's first release is published, set `enrichment.PublishedManifest` and `PublishedPublicKey` (portico-server `server/internal/enrichment`).
- **Apple:** confirm the order of a pending Not interested around a Reset (both completion orders), and whether old cards flash while a stale row restarts (the web clears them).
- Later: award rows from Wikidata; an "ask" box that maps mood words to dataset tags; music radio and mixes (a listening-similarity source is still to be chosen).

## Catalogue and metadata

- **Similar artists, More by, similar albums:** rebuild on a bounded candidate set: the seed's genres, capped candidates per genre, and collaborators from the seed's songs, then visibility and order. The earlier version walked the library per page view (archive: commit 658a6bd4, reverted).
- **Two background steps re-walk the library each step:** `metadata/music_artist.go` (`musicArtistStep` groups every album on every step) and `metadata/tvdb_episode_credits.go` (re-walks finished episodes). Make both incremental with a cursor or a pending-work row.
- **Missing sidecars aren't cached:** `localmetadata/sidecars.go` caches only hits, so each file re-reads every missing sidecar name. Add a per-scan negative cache per directory.
- **Audiobooks agent and per-type agent names** were built but never reviewed or merged (archive: branch `be/pages`, commits f4fee417 and d17535ea). Review on top of the current code: one agent, no new keys, no provider calls in requests, local-only honored, incremental background steps.
- **Container grids** (`catalog/content_entities.go`): TV show and collection lists sort a library's shows or collections per page with `OFFSET`. Walk the title index with a keyed cursor and take totals from maintained counts.
- **Visible counts on head changes:** collection retire/restore and book group moves may leave book-, group- and collection-visible counts stale (`compactcatalog/collection_visible_counts.go`). Queue the head's count keys in the same transaction.
- **Container personal flags:** Watchlist (shows) and Favorite (shows, albums, books, artists) now work end to end on the container personal-state route, in Saved lists and Home's Watchlist/Favorites rows (frontend rework, 2 Oct). Still open: a star **rating** on a show, album or artist, and Watchlist for an audiobook ("Want to listen").
- **A local NFO that names only actors hides the online crew (review B13).** `movie.nfo` owns the whole `credits` field (`screenField`: a non-NFO source never replaces an NFO-owned field), so a title whose NFO lists ten actors and no director never publishes the provider's director, writers or producers, and the TMDB rows written before the NFO took the field stay in `catalog_credits` as stale leftovers (the same actors twice in "See all"). Dune and Barbie on the demo data. Decide the rule: the NFO owns the cast and online crew fills what it does not name, or the NFO owns everything and the other provider's rows are removed. Either way the stale rows must go.
- **A second file of the same title becomes a second title.** `Movie (2021) - 720p.mkv` beside `Movie (2021).mp4` is catalogued as its own movie rather than a second version, so the Version control on a title page has nothing to list. Merge same-folder, same-identity files into one title's sources.
- **A replaced file is not re-read by an incremental scan.** Overwriting a media file in place (new size and modified time) left its stored facts unchanged after two incremental scans.
- **Show facts the page has room for but the catalogue lacks:** end year (for "2008–2013"), status (Continuing / Ended), a community rating and the first-aired date. The show workspace would carry them; the web hero and Details print them when present.
- **A title's dominant colour** for placeholders and the no-backdrop gradient. Clients use a stable tint from the title's id until the server publishes one (`titleTintHue`).
- **Show libraries cannot be filtered by genre, and Anime not by year** (found 2 Oct). Genres are stored for movies, songs and audiobook files only (`catalog_term_sources`), and the maintained facet counts are per episode: TV Shows has decade, year, network and studio counts (of episodes, not shows) and no genre; Anime has studio only. The clients promote Genre and Year on a show library and the picker then says "Nothing to filter by yet", on web, iPhone and tvOS. Needs: show-level genres and years in the browse index, and facet counts of shows for a show pivot.
- **Known costs, correct but worth watching:** a sparse filter over a huge pivot pages by offset after one exact count per revision; a very broad search prefix on a huge library costs its matches within the three-second budget; shuffle is linear in container size.
- Wikipedia text is English-only. Movie `logo.png` sidecars are stored but not exposed. Some entity fields (`label`, `albumType`, `overviewSource`, `country`, active years) aren't rendered by the clients yet.
- **Recently aired has no counted ordering.** The `aired` sort on episodes is an expression sort (offset paging, no blocks). Give it a counted ordering in `compactcatalog` before a multi-million-episode library uses the tab (frontend rework pass 7).
- **Sort Songs by plays.** Songs carry the viewer's play count (2 Oct) but the Plays header does not sort: a sort by plays needs the played songs first by count, then the rest by title, as one keyset. iPhone song rows do not show the count (the web hides it under 860 px too).
- **The session-based history route is unused.** The Dashboard's Viewing panel reads totals of the permanent play history since 2 Oct (`GET /v1/admin/play-history/summary`; each play records `converted`). `GET /v1/admin/playback/history` and its CSV export are no longer called by the web or Apple apps; move a CSV export onto the play history, then drop the route and `client-core` `playbackHistory`.
- **Credit cap (backend audit).** A provider record keeps at most 128 credits, cast first with key crew ranked in; raise or page it with the backend audit rather than alone (frontend-rework question 2).
- **NFO cast has no photos.** People named by a local NFO are name-only (`name:<folded name>`), so their circles show initials and their pages list only local titles. When the library's provider names the same person on the same title, link the NFO credit to the provider's person (portraits are selected per title by `provider:person id`, so the artwork step needs the link too).
- **TheTVDB libraries have no Trending feed.** Trending rows come from TMDB's and AniList's feeds; TheTVDB has none, so a library on it contributes no trend rows. (Its cast comes from TheTVDB's own record; the TMDB fallback removed on 2 Oct only covered a show whose TheTVDB record lists no characters.)
- **A title the new provider cannot find keeps its old details.** After a library's provider changes, a title with no match on the new provider stays unmatched and shows what it last had until the owner uses Fix Match. Consider saying so on the title's Matching tab.
- **Ports in flight (3 Oct):** Android/Android TV/Fire OS (`Audits/2026-10-03-android-port-handoff.md`, branch `android-port`), Vega OS (`…-vega-port-handoff.md`, `vega-port`) and Roku (`…-roku-port-handoff.md`, `roku-port`), each by its own agent in its own worktree; logs in `Audits/2026-10-03-*-port-log.md`. The lead merges the lanes and reviews against the Apple TV app. A Roku test environment is still to come.
- **Hostile-input test rotation.** `TestEveryRouteSurvivesHostileInput` gives each route one hostile path value by route index, so adding a route moves every later route to a different value (on 2 Oct that exposed three lookups that passed an integer-looking id to `pid_blob`; fixed). Try every hostile path value on every route.
- **Play history has no title search, no per-row delete and no "who watched this" on a title page.** The table has no index by title; add one with the search. Hosted-account viewers are named by their account id (as on Now Playing).
- **The old Home projection (`GET /v1/content?view=home`) is unused by the web and Apple apps** since Home moved to `/v1/home`. It lost its `featured` title on 2 Oct; remove the projection and its client-core route when no third party needs it.
- **Untagged albums in the main Albums grid.** Albums under "Unknown artist" are listed in the owner's Unmatched tab but still appear in Albums; leaving them out needs a counted ordering that excludes them (pass 7).
- **Two metadata providers under one media kind** are allowed by library settings validation (pass 6, open question 9).

## Server

- **Prepared-statement cache:** a per-connection LRU in `dbwork` cut a 10,000-movie settle from 185 s to 99 s (archive: branch `o2/stmt-cache`). Optional; long first imports are acceptable.
- **Test timing fixes** not merged (archive: branch `int/runner-timing`): show workspace cursors read the service clock; Hosted instants truncated to PostgreSQL precision; a directory revision read only after the directory settles. Check whether each still applies.
- **Native item cutover tests** (archive: branch `muse/cp2-item-tests`, test files only).
- **Planner statistics: sampling is still a query per index of up to 20,000 probes.** Fixed 2 Oct (portico-server `407cb50`): the pass stored "0 rows" for a table that was empty at the time, SQLite then planned every lookup on the filled table as a scan, and the pass's own sampling of the guide table ran for over 20 minutes on the demo after each start. Empty tables now get no entry (as ANALYZE does) and old zero entries are cleared at open; the whole pass takes 1.2 s on the demo database. Left for the backend audit: a restart in the middle still repeats the tables not yet written, and pooled read connections keep the statistics they loaded at open until they are recycled.
- **Two Apple sign-ins to the demo ended without an obvious cause** (2 Oct). Both simulator apps, signed in directly, showed "Your sign-in has ended" when they were next opened after the demo was restarted; the server logged `authorization family revoked … reason=explicit_signout` for two families at those moments and no `refresh_reuse`. A plain restart of a local server does not reproduce it. Worth a look in the backend audit: what the renewal answered while the server was busy, and why the client then signed out.
- **`runner-test.sh --full` cannot finish httpapi.** At tier all the package takes about 35 minutes alone (2,082 s, measured 2 Oct at the rollback tag and on `frontend-rework`), and the preset's `-timeout 30m` kills it. Six `TestPlaybackV1Queue…` scale tests of about 204 s each are most of the time. Either raise the preset's timeout for tier all or move those tests to the nightly scale tier.
- **`web/test/integration/setup-flow.mjs` fails** at "the checklist shows until setup is finished": `state.ready` is true as soon as the owner exists with `authMode: local`. Script and setup code are unchanged since before the rework; decide which is right.
- **Default test tier under 5 minutes.** The full server preset takes 461 s on the runner now. The remaining cost is fixture settling in `httpapi` and `catalog`, and real installs in `persistence`.

## Clients

- Apple: card size (Small · Medium · Large) is a web setting only; the Apple poster-size preference is stored but not applied; playlist editing, history clearing and recording actions not exposed; `src/app/compat.ts` can go once client-core covers the newer fields.
- Client-core: move onto the generated `packages/contracts` types.
- Apple: the Saved History tab still renders its whole list; the media tabs use `WindowedGrid`.
- Web: `web/src/bridge/profile-vault-crypto.ts` is unused; remove it.
- Web audio: MP4 seeks through its sample table, but MP3 and FLAC still decode from the start to reach a position. Add seek tables (Xing/VBRI for MP3, SEEKTABLE or a bounded bisection for FLAC).
- Cast receiver: sidecar subtitles from the presentation (`presentation.subtitles`) aren't offered as CAF text tracks yet, so Cast shows only subtitles carried in the stream.
- **iPad two-pane Settings** is still the phone layout (pass 5).
- **Apple Customize Home** reorders with arrows, not drag (pass 7).
- **Playback info on Apple has no dropped frames or buffer**: the native player does not report them to JavaScript (frontend rework pass 8).
- **Right-click on a web card opens Portico's More menu**, so the browser's own "Copy link address" is not offered there; consider a Copy link item in the More menu (pass 8).
- **Per-show settings not built**: library-level defaults, DVD ordering, grouping by years, Keep, Delete after watching (Spec — Title Pages §4; pass 7 built the four switches).

## Integrations never exercised end to end

As of the 22 September Hosted rollout; each needs one real run.

- **Google and Apple web SSO:** both provider pages load with the web.getportico.tv callbacks, but no complete sign-in has been done: callback, token exchange and onboarding, including Apple's Hide My Email.
- **Native Sign in with Apple:** align the live `oidc.json` `appleNativeClientIds` and the Apple Developer capabilities with the app's bundle ids (`tv.getportico.portico.ios`, `.tv`), then verify native sign-in on iPhone and Apple TV. Apple account-change notifications are wired (`https://web.getportico.tv/v1/identity/apple/notifications`), but no real Apple delivery has been seen.
- **Account email:** the SMTP relay (`smtp-relay.gmail.com`, sender `accounts@getportico.tv`) accepts mail, but no message has been sent to a real inbox.
- **Certificates:** no fresh claimed-server certificate issuance or renewal through Google Public CA and the DNS nodes has been run since the move to systemd.
- **Google Cast:** the receiver now plays through Playback v1 and Hosted's content security policy allows the sender SDK (25 Sep audit). Still to do: register the real application id and test pairing, load, seek, takeover and reconnect on a physical Chromecast and a Google TV.
- **Legacy domains:** `api.getportico.tv` and `cloud.getportico.tv` still route to Hosted. Check older server registrations, stored client origins and scripts before retiring them.

## Operations

- **Rename the live infrastructure identifiers** that still carry an earlier project name: on core-us-1, the units `portico-astra-hosted` and `portico-astra-web`, the user `portico-astra-hosted`, `/opt/portico-astra-hosted` and `/etc/portico-astra-hosted`; on the demo, the nginx status log name. Do it in one deploy window: rename units, user, directories and config paths, then update `deploy-hosted-release.py`, the backup units and TOPOLOGY.md.
- **Hosted backups aren't running:** BACKUPS.md describes `portico-hosted-backup.service` (pgBackRest) and the key-bundle unit, but neither unit is installed on core-us-1 (checked 26 Sep; the audit deploy fell back to a `pg_dump` in `/var/backups/portico-hosted/`). Install and enable them, then run one restore drill.
- **GitHub:** the new repositories replace the contents of the PorticoMediaServer repos (force-push) when publishing starts. `portico-roku` and `portico-temp` are retired.
- After release: revisit high availability and the broader hosting architecture.

## Planned features

- DLNA (off by default; exposes the whole server when on).
- Desktop companion app for macOS and Windows (per-user, like Plex).
- Fix Match UI (the server side exists in `internal/metadata`).
- Ports of the tvOS app: Android and Android TV, Fire TV, Vega OS, Roku.
- A paid relay for remote access without port forwarding (the only paid feature).
