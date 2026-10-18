# cp3-nscreens — native screens and strings

Branch `audit-implementation/cp3-nscreens` in `server-cp3-nscreens` and `native-cp3-nscreens`; `audit-implementation/main` merged into both. Nothing pushed. No subagents or helpers were used.

## 1. Outcome

- **N3 Quiet re-reads.** When the server says a list changed underneath (library, collection/entity, show, saved lists, playlist, search), phones, tablets and TVs now read it again quietly, as the web does. The quiet re-read only runs while the app is in front, and the content on screen stays put. The "refresh" notice appears only if the list is still refused after three tries (about half a minute). The retry rule (`quietRefreshStep`) now lives in client-core, and the web uses the same copy.
- **N4 "Playing on your other devices".** This now appears on phones, tablets and TVs (Settings › This device). Each row's menu offers Pause/Resume, Back, Forward, Start over and Stop, as far as the server allows each one. A stale revision is retried silently, as on the web.
- **N5 Everyone's devices.** People › Devices on phones and tablets lists every device, newest first, with whose it is. Approve and Block work there. The line under each device uses the shared `deviceFacts` with relative times; the old raw platform code and date are gone.
- **N6 Invitations.** Native invitations now have:
  - an email field, validated. The old "name or email" label field broke the server's email check whenever a name was typed.
  - a Portico Account username field.
  - an owner-only role choice (Member/Administrator).
  - an operation id that stays the same when a send is retried.

  Pending invitations show "only for the Portico Account …", and the revoke prompt names the invitation the same way.
- **N7 Duplicate labels.** Version menus (title page and player) use the new shared `mediaVersionNames`. It builds on `versionNames`: bit rate, then size, then "Version N" tell alike copies apart. Subtitle menus (online and downloaded) use `subtitleTrackNames`. The player's subtitle pill names the track instead of showing the raw language code, and the sleep pill's "On"/"Off" now come from the catalogue.
- **Found along the way (all clients):** `quality.height` printed "1,080p" because `{height}` was formatted with digit grouping. The message is now `{height, number, ::group-off}p`, and the message formatter supports that ICU skeleton. This fixes the Files list, quality menus and version names on web, native and Vega.
- **N8 English tables.** Removed from client-core (`panel-logic.ts`): `tierLabels`, `scopeLabels`, `scopeHelp`, `scopesLabel`, `scanJobLabels`, `scanTierLabels`, `scanTierHelp`, `feedbackStatusLabel`, `feedbackCategoryLabel`, and `linearError` from `linear-api.ts`. Native now uses `roleLabel`, `scanStateName`/`scanDepthName`/`scanDepthHelp` and `feedbackStatusName`/`feedbackCategoryName`. The live-source sheet passed an already-translated error through `linearError`, which always replaced it with an English fallback; it now shows the real message.
- **N9 Owner wording.** New catalogue keys replace the borrowed ones:
  - sharing: `api.tokens.share`, `api.webhooks.shareSecret`, `api.mcp.shareAddress`, `server.reports.share`
  - webhooks and metadata: `api.webhooks.off`, `server.metadata.keyUnsaved`
  - dashboard: `server.dashboard.freeSpace`, `server.dashboard.porticoFiles`
  - viewing hours: `server.people.hoursTimeHelp`, now shown under the From and Until rows
  - merging: `title.merge.action`, plus the already existing `title.merge.find`/`title.merge.none`, now used in both merge sheets
- **N11 Fade-in.** Large windowed grids (library, person, saved) now fade in a card that a re-read put where another card was. Cards reached by scrolling, and titles that only moved down a place, do not fade. A new list (another generation or store) starts again, and at most 60 cards fade in one short while. In shelves and in-page grids, more than 60 new cards now fade as long as most of the old cards stayed (a scan's worth). Only a list that lost most of its cards counts as a different list.
- **N19 Cast.** `CastTransport` has an optional `deviceId`, and the pairing proof is keyed by it when present. Android (`CastDevice.deviceId`), Apple (`GCKDevice.deviceID`) and web (`Receiver.label`) now report it. Two TVs with the same name keep separate proofs.

## 2. Decisions I made

- N6: the native "Who it's for" field became an email field, matching the web and the server's validation. A name typed there made the invitation fail. Removed `invite.label`/`invite.labelHelp`; added `invite.emailInvalid`.
- N4 on TV: the panel is offered on television too (the structure row is no longer web-only). TVs control other devices through a focusable row menu. Vega does not draw the settings structure, so Vega has no panel.
- N11: the different-list rule for shelves became "more than 60 new and most of the old gone", instead of "more than 60 new".
- 1080p: fixed in the catalogue message, not at each call site, so the other lanes' player files didn't need edits.
- `web.access.deviceMeta` is now unused by every client. I left it for cp3-web's unused-string pass (W9) to avoid a catalogue conflict.

## 3. Audit bugs and decision gaps closed

| ID | What changed | Test |
|---|---|---|
| N3 | native `useQuietRefresh` (`app/quiet-refresh.ts`) in Library, Entity, Show, Saved (both lists), SavedPages playlist, Search; rule moved to `client-core/src/quiet-refresh.ts` | `client-core/test/quiet-refresh.test.ts` (moved from web) |
| N4 | `screens/settings/RemoteControls.tsx`; structure row on all platforms | `settings-console-pages.test.ts` (phone and TV include it) |
| N5 | `AccountDevices` without an account = everyone's; `server.devices` on phone | `settings-console-pages.test.ts` |
| N6 | `InviteSheet.tsx` email + Portico username + role; Invitations rows | native type-check |
| N7 | `mediaVersionNames`; native menus and pill | `track-version-names.test.ts` (new case) |
| N8 | English tables deleted, catalogue helpers used | type-checks; `linear-lan.test.ts` |
| N9 | 11 new keys, native uses them | string lints |
| N11 | `WindowArrivals` + `WindowedGrid`; shelf rule | `native/test/arrivals.test.ts` (8 tests) |
| N19 | `deviceId` through Cast transports | `cast-sender.test.ts` (new case) |
| (found) | `quality.height` "1,080p" → "1080p"; `::group-off` | `i18n.test.ts` (new case) |
| (found) | live-source sheet showed English fallback over real error | — |

## 4. API changes

None to HTTP routes. In client-core:
- added `mediaVersionNames` and the `NamedMediaVersion` type, plus `quiet-refresh.ts`
- `CastTransport.deviceId?` is new
- removed the English tables listed in §1 and `linearError`

## 5. Schema changes

None.

## 6. Settings added

None. The "Playing on your other devices" and People › Devices panels now appear on more platforms.

## 7. Tests run

- client-core: `tsc --noEmit` passes. `node --test` on settings-console-pages, linear-lan, track-version-names, i18n, quiet-refresh and cast-sender: 53/53 pass after the merge.
- web: `tsc --noEmit` passes; `lint:hooks` (oxlint) passes; `lint:strings` is clean.
- native: `tsc --noEmit` (app and tests) passes before and after the merge; `test/arrivals.test.ts` 8/8; `lint:hooks` passes; `lint:strings` shows 0 hard-coded strings. Vega `lint:strings` shows 0. After the merge, `shared-packages.mjs --check` reports the mirror identical to the server.
- Vega: `tsc --noEmit` passes.
- `go run ./cmd/apigen` after the merge: no changes. The shared-packages mirror was re-written after the merge and its lock is committed.

## 8. Device test notes

1. **Quiet re-read.** Open a library, collection or show on a phone and a TV. Rescan from another device so the list's continuation goes stale.
   - Pass: no warning flashes and the list updates in place.
   - With the server refusing for 30 s or more, the warning with Refresh appears.
   - Background the app during the tries: nothing is read until it's back in front.
2. **Other devices.** Play something on a TV, open Settings › This device on a phone. The row shows the device and title. Pause, Forward 10 s and Stop each act on the TV, and the row updates within 10 s. On a TV, the same panel is focusable and its menu works with the remote.
3. **People › Devices** (phone or tablet, as owner): every device is listed with its owner's name and "last seen …". Approve and Block work, and an administrator sees the list too.
4. **Invite.** Type a name instead of an email: the field shows the error and Create is disabled. Fill a Portico username, create, and the invitation says "only for the Portico Account …". The owner can choose Administrator.
5. **Version menu.** A film with two 1080p MKV copies shows "… · 8.0 Mbps" and "… · 12 Mbps" (or "Version 1/2"), and heights read "1080p", not "1,080p". Two unnamed English SRT tracks read "English · SRT (1)" and "(2)". The subtitle pill shows "English", not "en".
6. **Fade-in.** With a library grid open, add a title so it sorts at the top: its card fades in (on TV from 35%). Scrolling to new pages does not fade, and focus never moves.
7. **Cast** (Mac/Android): pair with TV A, then with a second TV of the same name; reopening the app reattaches to the right one. Check that `GCKDevice.deviceID` compiles in `PorticoCast.m`.

## 9. Not finished

- No device or emulator runs (as instructed). The RN hook `useQuietRefresh` and the grid fade have no render tests. Their pure rules are tested.
- `PorticoCast.m` can't be compiled here (Mac check). `PorticoCast.kt` was not built (no Gradle run).
- Vega has no "Playing on your other devices" panel: it does not draw the settings structure.
- `web.access.deviceMeta` is left unused (see §2).

## Commits

- portico-server:
  - `9656affd` Phones and tablets get everyone's devices and remote controls; owner words come from the catalogue
  - `aed6a4ff` Version menus tell alike copies apart; 1080p not 1,080p
  - `9b0c656b` Shared quiet re-read rule; Cast proofs per television
  - merge of main
- portico-react-native:
  - `7b154e7` Devices, remote controls, invitations, owner words
  - `56bb1ea` Version and subtitle menu names
  - `9c0c4e8` Quiet re-reads, grid fade-in, Cast per television
  - `ab90f66` merge of main with re-mirrored shared packages
