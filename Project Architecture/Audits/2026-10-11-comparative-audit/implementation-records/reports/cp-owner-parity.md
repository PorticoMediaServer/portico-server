# Lane report: cp-owner-parity — owner tools and freshness

Worktrees `server-cp-owner-parity` and `native-cp-owner-parity`, branch `audit-implementation/cp-owner-parity` in both, from `f719c2a0` / `4f0689c`, with `audit-implementation/main` merged in at the end (after licences, cp-queues-playlists, cp-web-playback and cp-polish). Nothing pushed. Ran on personal Ubuntu mini-1. **Nothing was seen on a screen or run on a device**: the evidence is source, type-checks and unit/package tests. The session was interrupted once by an app restart; work resumed from the committed and uncommitted state without loss.

## 1. Outcome

**Server**
- **List hints.** The event feed now carries `list.changed` (resource `list:<name>`) for every owner list: API keys, members' tokens, webhooks and their delivery log, members, invitations, devices, client reports, feedback, download usage, remote storage (rclone mounts and WebDAV), Live TV sources (and their refresh state), tuners, recording rules and grants, recording space, Library Channels and playback problems. A profile hears `accessTokens` (its own tokens) and `togetherGroups` (the Watch Together rooms it is in). They are SQL triggers into the existing coalescing outbox (`api_change_pending`), so every write path is covered in the same transaction; frequent writes (device last-seen, allocation leases, download progress) are excluded by `UPDATE OF` column lists.
- **Administrators hear the administration audience.** The feed gives the `admin` audience to a verified administrator as well as the owner (re-checked against the live membership); the owner's account inbox stays the owner's.
- **Lengths on administered playback.** `durationMs` (the version's length, from catalogue file facts, summed over parts) on `GET/POST /v1/admin/sessions/{id}/controls`, on each `/v1/admin/sessions` row (one batched read per page, not one per session) and on `/v1/playback/controls` rows.
- **Playback problems name their title**: `itemId`, `title`, `kind` on `/v1/admin/playback/route-failures` rows (one indexed lookup per row of a page of ≤101).
- **Tuners name whose they are**: `accountName`, `profileName` on tuner assignments.
- **Per-source import limits**: a Live TV source's settings take `importLimits` (null or per-field 0 = the server's); they can only lower the server's limits, are used on every import/refresh of that source, and are read back in the source's settings document (`effective.importLimits`).
- **Subtitle-provider credentials**: the OpenSubtitles API key, username and password live in the metadata settings document (key and password never sent back; username is), and take effect without a restart; the environment variables remain the fallback. A different (or no) account never keeps the previous password.

**Web**
- Every owner panel that re-read on a 30-second clock now follows its list hint; backups follow task progress (`operation.updated`), play history follows the session list (`admin.sessions`). Download usage pages past its first page ("Show more", all shown pages re-read on a hint). The resource-usage, places and dashboard measurement panels keep their timers (they are live measurements, not lists).
- Watch Together lobby follows `togetherGroups` on the freshness hub.
- Watching now: a thin progress line per row, "At m:ss of h:mm:ss", Forward stops 5 s short of the end, and **Move to…** opens a position bar (dialog with a slider) that sends an absolute seek, silently re-sent if the session moved.
- Playback problems lead with the title (with Details) instead of only the failure.
- Tuner lines say whose stream or recording it is.
- A Live TV source's settings dialog has its own import limits.
- Metadata page: "Subtitles found online" with the OpenSubtitles key, username, password and "Use the server's key"/"Remove the account".
- **No more false conflicts**: a Library Channel saved from the builder after the channel was reordered, toggled or regenerated is carried over field-by-field and re-sent silently (only a field changed on both sides is a conflict); `library_channel_changed` is now a recognised stale answer (it also makes channel reordering's existing silent retry work). Rclone mount Start/Stop/Restart/Remove re-read the mount and re-send once when only its revision moved; a command no longer offered (already done) counts as done.
- **B-18.9 administrators**: the web shows administrators the Server pages for what the server lets that tier do (below).

**Shared Settings structure (client-core)**
- Phones and tablets are declared canonically for Metadata, Integrations, Storage usage/places/download usage, Troubleshooting set-aside/client reports, General restart, Streaming playback problems and Profile tokens; People no longer has a phone-only keys tab. Card size, Show backdrops and the caption size/background are declared for every platform (the card size only where the server publishes it). Television keeps the Dashboard alone.
- `administrator` in the settings context, and an `administrators` flag on sections: Watching now; People › Accounts, Invitations, Devices; Troubleshooting › Logs, Titles set aside. No server-setting row is shown to an administrator.
- Two preferences the server already honoured but no screen offered are now rows: **Counts as started** (`playback.startedThresholdPercent`, Playback › Advanced) and **Include me in Watch Together activity** (`privacy.includeInWatchTogether`, Privacy).

**Native (iPhone, iPad, Android phones/tablets; TV and Vega where stated)**
- The local Settings adapters (`native-structure.ts`: `withOwnerPages`, `withNativeRows`) are deleted; the app draws the shared structure as is.
- Watching now: progress line, Pause/Resume, Back/Forward 30 s, Start over, **Move to…** (sheet with the player's own Timeline as the position bar), Details, Stop.
- Streaming › Playback problems (new panel): title, what failed, when, detail; menu Open the title / Forget.
- Troubleshooting › the state-folder warning panel (was listed for phones and drew nothing).
- Episode order and offset controls in the metadata editor's Match tab (same rules as the web: order picker when there is more than one, offset only for TMDB's own and alternate orders, "Use this order" on the current match, the current order named in words).
- Merge for songs, audiobook files and extras: the album page (songs), book page (files) and title page (extras) offer the owner **Merge** when two of one mergeable kind are shown; pick the two, then which stays (the same server route as titles).
- Paged owner lists (`usePagedList`) take `on:` hints and re-read every shown page in one go (Show more never collapses): API keys, members' tokens, own tokens, client reports, accounts, devices, invitations, titles needing attention, missing titles. Webhooks, Live TV sources, recording rules, recording space, Library Channels, tuners and backups follow their hints; download usage pages past page one.
- Metadata page: the OpenSubtitles key/account rows. Tuner lines name whose. A source's import limits in its settings sheet.
- Administrators: an administrator's Accounts list comes from the access list; the account sheet edits limits and viewing hours of members below them and shows devices; role, libraries, sign-in, disabling and deletion stay the owner's. **TV** (Apple TV, Android TV, Fire TV, Vega share this code): an administrator's Dashboard is Watching now alone (the rest calls owner-only routes).
- Profiles: **Forget remembered devices** works (web too): it calls the existing trust-revocation route and confirms; before it was hidden for lack of a handler.

## 2. Decisions I made

1. **One hint type, many lists**: `list.changed` with the list's name as the resource id, not one type per family. Clients already depend on `event:<type>:<id>`, so `dependsOn.list(name)` is one line per panel. Revision is the change time in ms (the hint only says "read again").
2. **Hints by trigger, not by Go call sites**, so no write path can forget one and the hint commits with the change. Columns that change on every request are excluded.
3. **Administrators receive the `admin` audience.** Its events carry no data beyond type and id; giving it to admins lets their Watching now, people and log panels stay current. The owner's account inbox is not widened.
4. **What an administrator can do = what the server already authorises for the admin tier** (Admin-access routes): sessions (list, controls, stop), access members (list, role, limits), invitations, devices and trust, logs, catalogue failures. Everything else — including `/v1/direct/members`, Play history (`/v1/admin/playback/history` is owner-only), alerts, health — stays the owner's. I did not widen any server route. An administrator does not change roles in the UI (the server lets an admin manage only tiers strictly below, so promoting to admin is refused anyway).
5. **Position bar in a dialog/sheet** rather than inline: the web list is fixed-pitch and windowed; native rows are one press target. Move to… sends an absolute position (re-sent unchanged if the session moved); Forward and Move clamp to 5 s before the end.
6. **Measurements keep their clocks** (resource usage, places, health, storage, telemetry): they change continuously and are not lists.
7. **Per-source import limits can only lower** the server's limits (the server's stay the ceiling), and are a settings override rather than something stored from the add form.
8. **OpenSubtitles credentials sit in the metadata settings document** (with TMDB/TheTVDB keys), so one page and one Save hold every provider credential; the environment remains the fallback, matching how Portico's own TMDB key works.
9. **Merging songs/files/extras on a phone is "pick two from this page"**, mirroring the web's select-two-and-merge, because those kinds have no page of their own and no search group. It uses the entries the page has loaded.
10. **Album and book (container) merges on the server were not built**: moving tracks/parts between albums or books with their disc/key structure is not small (the brief said "if small").
11. **Library/saved-resource pin controls were not built** (B-18.27 remainder): `library-pins.ts` has no consumer on any client; pinning is a viewer navigation feature with its own screens on web and native, not an owner tool. Recorded in §9.

## 3. Audit bugs and decision gaps closed

| ID / gap | What changed | Test |
|---|---|---|
| B-18.9 (Administrator role grants nothing) | Shared structure `administrator` + `administrators` sections; web `session.administrator`, Server pages for admins, administrator Accounts panel; native the same, TV Watching now only; feed audience for admins | client-core `settings-console-pages` (administrator test), native `owner-structure`, Go `TestListHintsReachOwnerAndAdministratorsOnly` |
| B-18.27 (dormant/inert controls) | Native `server.stateFolder` and `server.playbackProblems` now drawn; phone declarations canonical (adapters deleted); registry-only `playback.startedThresholdPercent` and `privacy.includeInWatchTogether` now have rows; Forget remembered devices wired on web and native. Per-library provider API-key field was already removed by the console lane. Pins remain (§9). | native `owner-structure` ("every panel the phone structure names is one this app draws"), structure catalogue test |
| Native owner gaps (playback problems, Watching-now transport, episode order, merge songs/files/extras, paged lists quiet) | §1 Native | native `owner-rules`, `owner-structure`, suite |
| Web owner gaps (channel builder/mount false conflict, import-limit readback, tuner names, subtitle credentials) | §1 Web, Server | Go metadata/livechannels/administration tests, web suite |
| Admin session duration, playback-failure title identity | §1 Server | Go `TestAdministratorControls…`, `TestFailureMemoryNamesTheTitle`; client-core `owner-parity.test.ts` |
| List-change events; 30-second clock removed; Watch Together on the hub; download usage beyond page 1 | §1 | Go list-hint test; client-core `dependsOn.list` test |

## 4. API changes

- Event feed: new `list.changed` hints (see `server/api/guide/06-events.md`); administrators now receive `admin`-audience hints (`admin.sessions`, `operation.updated`, `list.changed`).
- `durationMs?` on `SessionControls` (admin controls GET/POST, remote controls rows) and on `AdminSession` rows.
- `itemId?`, `title?`, `kind?` on playback route-failure rows.
- `accountName`, `profileName` on tuner assignments.
- Live source settings: `importLimits: {bytes,channels,programmes} | null`; `effective.importLimits` reflects it.
- Metadata settings: response `openSubtitlesKeyConfigured`, `openSubtitlesUsername`, `openSubtitlesPasswordConfigured`; request `openSubtitlesApiKey?`, `openSubtitlesUsername?`, `openSubtitlesPassword?`.
- Contracts regenerated (`go run ./cmd/apigen`), also after the merge.

## 5. Schema changes (baseline only)

- `metadata_runtime_settings`: `opensubtitles_api_key`, `opensubtitles_username`, `opensubtitles_password`.
- ~75 `*_list_*` triggers appending to `api_change_pending` (end of `0001_baseline.sql`), and four Watch Together triggers. `scripts/schema-sync.py` run (also after the merge). Development databases need recreation.

## 6. Settings added or wired

| Setting | Where |
|---|---|
| OpenSubtitles API key / username / password (server, metadata document) | Server › Metadata (web, phone/tablet) |
| Live source `importLimits` (per source, lower only) | Live TV › a source's settings (web, phone/tablet) |
| `playback.startedThresholdPercent` (existing, 1–25 %, default 5) | Playback › Advanced (all clients) |
| `privacy.includeInWatchTogether` (existing, default on) | Privacy (all clients) |
| Card size, Show backdrops, caption size/background | now declared for phone and TV in the shared structure (were native adapters) |

## 7. Tests run

- Go (touched packages): `go test ./internal/social ./internal/apievents ./internal/eventfeed ./internal/playback ./internal/playbackv1 ./internal/administration ./internal/metadata ./internal/subtitles ./internal/livechannels ./internal/persistence` → all ok. `go test ./internal/httpapi` → one failure, `TestListeningHTTPPreferenceEnvelopeAndCAS` ("explicit reviewed helper binary required": an environment requirement of that test, unrelated to this lane); all others pass. After the merge: focused httpapi/apievents/eventfeed/social/metadata runs ok, `go vet` on touched packages clean, `go build ./...` and `GOOS=windows go build ./...` ok.
- New Go tests: list-hint audiences, controls/list duration, failure-memory title, OpenSubtitles provider credentials, metadata settings credentials, `SourceImportLimits`.
- client-core: `npm run test:core` → 1968/1968 after the merge (one timing-sensitive playback test flaked once under load before the merge and passed alone). New `test/owner-parity.test.ts`; updated `settings-console-pages`, `live-words`.
- Web (after merge): oxlint hook lint clean, `tokens`, `lint:design --strict`, preference-consumer check (38 keys), `lint:strings`, `tsc --noEmit`, `npm test` → 581/581.
- Native (after merge): `tsc` app and tests clean, `lint:hooks` clean, `lint:strings` 0 over baseline, `npm test` → 673/673. Vega: `tsc` app and tests clean, `npm test` → 87/87. Shared packages `--check`: identical to the server branch head.
- No lockfile changed, so licence lists were not regenerated.

## 8. Device test notes

Server with owner, an administrator, a member; a film library, a music library, an audiobook library, Live TV; two clients A and B.

1. **List hints.** A on Server › Integrations › API keys (web, then phone). In B create a key: it appears in A within a few seconds with no spinner and no list collapse; open "Show more" first and repeat — the extra pages stay open. Repeat for webhooks (send a test: the delivery log moves by itself), People accounts/invitations/devices, client reports, Live TV sources (refresh a guide), tuners (start a channel), Library Channels, remote storage. Fail: a list that changes only after ~30 s or only on pull.
2. **Member tokens.** As a member, Settings › Profile › tokens open on A; create one on B: it appears on A.
3. **Administrator.** Sign in as an administrator. Web and phone: Server shows Dashboard (Watching now only), People (Accounts, Invitations, Devices) and Troubleshooting (Logs, Titles set aside); nothing else. Open a member: limits and viewing hours save; role, libraries, sign-in and Delete are not offered; another administrator or the owner opens read-only. Invite someone as a member: works. Pause a member's playback from Watching now: works. On a TV as administrator: the Dashboard is Watching now only. Fail: any page that errors for lack of permission.
4. **Watching now.** Play a 2-hour film elsewhere. Web and phone: the row shows "At 0:05 of 2:00:00" and a progress line. Move to… → drag to 1:30:00 → Move here: the other device jumps there. Forward near the end stops 5 s before it. Channel sessions: no Move to….
5. **Playback problems.** Make a file fail (e.g. HEVC on a browser without HEVC at Original): Streaming lists the film's title; Details opens it; Forget removes it. Same on a phone.
6. **Channel builder.** Open a Library Channel in the builder on A; on B reorder the channels or switch that channel off; save on A: saved, no conflict; B's change is kept. Change the channel's name on both: A gets the conflict message.
7. **Mounts.** Start an rclone mount and immediately Stop it from another browser; Stop/Start/Remove from the first: no "storage changed" message unless the mount was really removed.
8. **Episode order (phone/tablet).** An anime show numbered 1–24 split into two provider seasons: Edit metadata › Match shows Episode order (if several) and the offset field; set the offset, Use this order: episodes line up; the current match line names the order.
9. **Merge on a phone.** On an album with a duplicated track, Merge → pick both → keep one: one track with two versions. Same for a book's duplicated file and a film's duplicated extra.
10. **Subtitles found online.** Metadata page: paste an OpenSubtitles key and account, Save; search subtitles for a title: results appear without a server restart; "Use the server's key" falls back to the environment's.
11. **Import limits.** In a source's settings set "Most channels" to 50, Save, refresh the source: at most 50 channels; reopen: 50 shown; clear it: the server's again.
12. **Watch Together.** Lobby open on A; join a room on B with the same profile: A's lobby lists it within seconds; end it: it goes.
13. **Forget remembered devices.** On a PIN-protected profile, remember a device, then Forget remembered devices from another: the first asks for the PIN again.

## 9. Not finished

- **Library/saved-resource pin controls** (`library-pins.ts` has no consumer anywhere): a viewer navigation feature for web and native rail/sidebar; not built here.
- **Album, book and artist merges on the server** (moving children between containers) were not built (§2.10). Native merge covers songs, book files and extras within one page's loaded entries only.
- **Forgot PIN** in the native profile menu is still hidden (no native handler); out of this lane's scope.
- **Administrator Play history**: the panel's route is owner-only, so administrators don't see it even though an admin-gated `/v1/admin/play-history` exists; switching the panel to it was not done.
- **Remote-storage WebDAV commands** keep their own receipt/ambiguity handling; only rclone mount commands gained the silent re-send.
- Native layouts (Move to… sheet with the Timeline inside a sheet, merge-two sheet, metadata subtitle rows) were not seen on a screen.
- Native intermediate commit `6363011` references panels added in the next commit; the branch head compiles and tests clean.

## Commits

portico-server (`audit-implementation/cp-owner-parity`): `5da74e9b`, `8c401287`, `46ad7172`, `6d7af205`, `7deefc1d`, merge `04570edc`.

portico-react-native (`audit-implementation/cp-owner-parity`): `257d776`, `6363011`, `f8a44c7`, `8bcf218`, `a170851`, `d4490db`, `3e3c36b`, `0375329`, merge `30aa026`.
