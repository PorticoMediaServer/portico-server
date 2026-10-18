# Lane report: native-sync

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-sync`, branch `audit-implementation/native-sync` (portico-react-native), 18 commits over `0e4fdcf`, all by Justin Ehler, nothing pushed. `packages/` was written only by the mirror script.

**How far it was checked.** `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` in `apps/native`, and `npx tsc --noEmit` in `apps/vega` (with a git-ignored `node_modules` symlink to the main checkout's), report errors only in the playback lane's files (listed in section 7). `npm run lint:strings` passes in both apps (exit 0; one over-baseline line in `screens/Downloads.tsx`, a downloads-lane file, is a false positive of the lint on code and was left). 26 test files were run one at a time and pass: 12 new files with 54 tests, 8 tests added to 3 existing files, and the existing files next to what changed. **Nothing was built with Gradle or Xcode, nothing ran on a device, the full suite was not run. One Objective-C method and two Info.plist edits were written blind.**

## 1. Outcome

1. **Shared packages are current.** Mirrored three times from `main-server`, each as its own unchanged commit: `82d71c2` (at `c3da536`), `5f925a2` (after downloads, styled subtitles and accounts merged) and `70d7185` (at `aac183d`). `shared-packages.mjs --check` says identical at the end.
2. **Stand-ins are gone.** The 24 staged sentences and the Player setting's words come from the catalogue; `engine-strings.ts` and `EngineSettingRow.tsx` are deleted; the Player setting is the shared settings row. Device-class settings say "All TVs" or "All phones and tablets" through `settingsScopeLabel`.
3. **A library reopens the way this profile left it**: the tab, and that tab's sort and filters, kept on the device per profile and server. A link that names a tab, category or genre is followed as it is and changes nothing remembered. A remembered filter the library no longer offers is dropped. A library's sorts and filters are rechecked each time it opens.
4. **Server address.** Typing an address tries each likely one in turn (the shared rule): HTTPS, then plain HTTP on Portico's port, for a public name or address; plain HTTP first for a home one; IPv6 with or without brackets. Apple builds now allow plain HTTP to any address. The pinned identity is paired at the address that answered.
5. **Quiet plain-HTTP note** on the direct sign-in form and the TV code screen when the server says the connection is plain HTTP from outside its home networks. It never blocks.
6. **A sign-in is kept** when another server answers at its address ("A different server is answering", with Try again and Use another), and when the account is outside its allowed hours (the server's name, "doesn't let this account in at this time of day", keeps retrying). A direct server reached at a new address is remembered at that address.
7. **Titles and files.** Cards and media rows show "Unavailable" (dimmed artwork, spoken too); episodes say it in their meta line; title pages and episode panels say "Missing" with the date. A title's Files list names parts, versions and editions. A Version control joins Audio and Subtitles before play (shown once the player says it starts on the picked version; see section 7).
8. **Server pages (owner).** Missing titles (list, count, "Remove them now" with the typed number) replace held deletions, under the rule rows; API keys use the new key routes with state, last use and expiry; Home network shows where the owner is connected from; Your own certificate shows whether it is in use. The new connectivity, missing-title and background-speed settings appear through the shared list with no native code.
9. **Watch Together.** "Waiting for everyone" while the group holds; a title the viewer may not see is "a title that isn't in your libraries", not "nothing chosen"; private members are listed as "Private viewer"; Become host appears only after the host's full two-minute silence and only for someone connected and in step; Make host only for such a member; a paused or holding room loads a title paused. The title page offers Watch Together and Add to group queue.
10. **Play on.** The controller lives as long as the sign-in. After the other device takes over, a bar above the tab bar offers Play/Pause, Start over and Stop, and follows what the other device's own remote does. It is picked up again after the app was closed. The "use the other device's remote" dead end is gone.
11. **Cast.** The controller has installation-private storage for its control proof and takes back a session the system kept or resumed, without pairing again. Cast is offered only where the server names the receiver application this build was made for.
12. **TV activation obeys the shared Portico Account gate** (native and Vega): timed polls, timed retries and tidying wait while the gate is closed and are answered locally with the time left; what a person asks for goes through; "still waiting" and "slow down" never close the gate.
13. **Send a report.** Settings › About › Send a report sends the app's recent warnings and errors, and whatever ended the previous run, to the owner's server after a confirmation. An error that ends a run is kept on the device automatically for the next launch. Tokens, keys and address queries are removed as lines are recorded.
14. **Accounts.** Outside-hours, home-network-only and wrong-invitation refusals have their own sentences at sign-in; a TV code approved while the owner must still approve the device says "Waiting for the owner to approve this device" and keeps asking; the owner's-play-record note sits above the privacy switches; an account that is off says since when and why.
15. **Downloads leftovers.** Beside "Original" the download menu says when the file is one this device cannot open; signing out offers "Remove my downloads from this device" (off by default).
16. **Smaller.** Browse field and quick-filter names, and several sign-in sentences, come from the catalogue; Refresh metadata is offered only where the server can do it; the restore refusal for an unlockable folder has its sentence.

## 2. Decisions I made

1. **Mirrored again, twice**, when the server base gained downloads, accounts and the privacy note. The brief asked for one mirror at the start; without the later ones the downloads files did not type-check and the accounts items could not be done. The playback lane mirrored once, at the first point: joining needs one re-run of the mirror (as the integration method already does).
2. **Step 2 touched three lines in two player files** (`VideoPlayer.tsx`, `PlayerOptions.tsx`) and deleted the two stand-in files, as the brief's step 2 says. The playback lane made the same edits; the texts are identical except one line in `PlayerOptions.tsx` (take theirs).
3. **`SettingsPage.tsx` is shaped to join.** After seeing the playback lane's working copy I rewrote my change to be a subset of theirs in the regions both touch (imports, capabilities, the device-row block). On a conflict there, take theirs. Until joined, the "If something looks or sounds wrong" group on my branch has a heading and no rows.
4. **Apple App Transport Security is off** (`NSAllowsArbitraryLoads` true, `NSAllowsLocalNetworking` removed, iOS and tvOS). The decision is "plain HTTP from any address"; with the old setting a public host name over HTTP could never connect. App Review will ask for a reason: the app connects to servers its user names.
5. **Another server at the address ends nothing** (the web's rule). The old native behaviour sent the person to sign-in. `resumeEnding` lost its "reset" outcome.
6. **Remembered choices** follow the web: per tab; a link never changes memory; choosing a filter or sort also remembers the tab. A genre card and a row's See all inside the library count as links.
7. **The plain-HTTP note on the phone form asks when the address field is left**, not while typing: an unauthenticated read of `/v1/networking/connection` at the address a sign-in would use, with no identifier. On TV it is asked of the connected server.
8. **Version before play is hidden until the player honours it** (`pending-version.ts`). A control that changed nothing would be worse than none. The playback lane has a Version choice inside the player.
9. **Play on offers Start over, not skip or a scrubber.** The destination's session document carries no position, and the route that would list it (`GET /v1/me/devices?controllable=true`) is not on the server. Guessing a position would seek to the wrong place.
10. **The handoff id is read off the commit answer** by a wrapper round the transport, because `HandoffController` does not publish it. Remembered for 12 hours.
11. **Cast needs the server's application id to equal the build's.** A build can start only the receiver it was initialised with. With no server setting there is no Cast row (the server's rule: configuration, never a default).
12. **Activation gate classification.** A person's action counts for 10 seconds (opening the screen, Try again, Get a new code, returning to the app). Polls are never a person. A cancel after a failed accept is the app's own.
13. **Reports are sent only on request.** A crash is captured automatically and mentioned under the Send row; nothing is sent by itself. The body is plain English lines (a diagnostic record, not interface text).
14. **Strings the catalogue lacks are staged again** in `apps/native/src/app/new-words.ts` (12 entries, section 7), the mechanism the earlier lane used.
15. **Sign-out keeps downloads unless the person chooses otherwise**; only their own are removed, and the server is told for each.
16. **"Unavailable" on a media row replaces its length**, since a row has one meta slot.
17. **The outside-hours state names the server as the title** and shows the catalogue sentence; it uses the same retry schedule as an unreachable server.

## 3. Bugs

- B-13.6, B-13.8, B-13.9(a) — client side done on native (claim deadline, private members, hidden title).
- B-13.7 — native persistence done (controller storage, adoption); unverified on a device.
- B-13.18 — improved, not closed: the pause is set the moment the player has the title. A guaranteed silent first frame needs the engine to start paused (section 7).
- B-14.12 — fixed for the note; choosing mpv for such a file was already the local rule.
- B-14.14 — sign-out now offers removal; settings stay device-wide (unchanged).
- B-16.19 (native side) — fixed: capabilities are re-read on each open.
- B-19.16 — reduced: browse field names, quick filters, nine sign-in sentences, Watch Together states and readiness. Remaining literals in section 9.
- B-21.6, B-21.8, B-21.9, B-21.12, B-22.15, B-22.19, B-22.21 — not fixed: the shared or server change each waits for still does not exist (checked).
- B-22.5 — fixed on native.
- B-22.7 — native now uses the shared rule (a public IP is tried over HTTPS first, then HTTP).
- B-22.17 — fixed (`setAddress` on a verified route change).
- B-22.26 — fixed on the client (shared gate).
- D-13.13 — closed on native (no build-time fallback when the server names no application).

## 4. API changes

None on the server. Newly read by native: `GET /v1/networking/connection`, `GET /v1/admin/missing-titles`, `POST /v1/admin/missing-titles/empty`, `GET/POST/DELETE /v1/admin/api-keys`, `GET /v1/cast/configuration`, `POST /v1/diagnostics/client-logs`. The title page now also uses the existing Watch Together routes (watch with the group, add to its queue) and asks for a refresh only at the path the shared helper gives. No longer called: `/v1/admin/trash*`, `/v1/admin/access/api-keys*`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

- Player (`playerEngine`, this device): shared row, shown when the build has mpv.
- Remembered library choices: no switch.
- Sign-out: "Remove my downloads from this device", per sign-out, default off.
- Owner settings that now appear on phones through the shared list: missing-titles rule and days, external address, connection limit, own-certificate fields, background speed.
- Not wired here: `forceStereo`, `disablePassthrough`, `disableHdr` (playback lane).

## 7. Needed from others

**Joining with native-playback**
- Re-run the mirror once on the joined branch.
- Remaining type errors, all theirs: `src/bridge/capability-document.ts` (13), `client-profile.ts` (4), `local-playback.ts` (3), `device-profile.ts` (2), `audio-decode.ts` (2), `capability-publisher.ts` (1); tests `capability-profile.test.ts` (5), `local-playback.test.ts` (2); Vega `src/bridge/capability-profile.ts` (6), `src/overrides/native/bridge/client-profile.ts` (4).
- Expected conflicts: `screens/settings/SettingsPage.tsx` (take theirs in the import, capabilities and device-row hunks; keep my `settingsScopeLabel` subtitle, `PrivacyNote`, About report row), `player/PlayerOptions.tsx` (take theirs), and nothing else that I know of.
- Version before play: call `playerStartsPickedVersion()` once where the player is set up, and pass `takePendingVersion(itemId)` as `versionId` in the start's `PlaybackChoices` (`apps/native/src/app/pending-version.ts`).
- Paused start: let `engine.play` take the starting state, then `app/together.tsx` `load` can pass `playing` straight through and `paused-load.ts` can go.
- A `PlayerActions.enqueue` so the title page can offer Play next and Add to queue (`detailActions` `queue`).
- Destination session changes (pause, seek, stop sent from a phone) must reach the television's player through the session controller (social report item 6).
- I edited `src/player/Destinations.tsx` (uses the shared Play on controller; the "done" dead end removed). They had not touched it.

**Strings to add to `packages/i18n` (staged in `apps/native/src/app/new-words.ts`)**

| Name | Text |
|---|---|
| `about.report.title` | Send a report |
| `about.report.help` | Sends this app’s recent warnings and errors to the owner of {server}, to help find a problem. |
| `about.report.afterCrash` | Portico closed unexpectedly last time. A report would include what went wrong. |
| `about.report.confirmTitle` | Send a report to the owner of {server}? |
| `about.report.confirmBody` | It holds what this app logged while something went wrong. That can include the names of titles and of this device. It never includes your password or sign-in. |
| `about.report.send` | Send |
| `about.report.sent` | The report was sent to the owner of {server}. |
| `about.report.failed` | The report couldn’t be sent. Try again later. |
| `activation.failed` | Portico couldn’t get a sign-in code. Check your connection, then get a new code. |
| `downloads.quality.originalMayNotPlay` | This device can’t open this kind of file, so the original may not play here. A smaller version will. |
| `settings.signOutConfirm.removeDownloads` | Remove my downloads from this device |
| `settings.signOutConfirm.bodyRemoving` | Your downloads are removed from this device. Anything you downloaded stays on your server. |

Also wanted: device-neutral versions of `web.direct.lede` and the TV address lead; "Only the host can play and pause / move the group"; `web.together.state.paused` (the web borrows `web.downloads.state.paused`).

**`packages/client-core`**
- `HandoffSnapshot`: publish the committed handoff id (then the transport wrapper in `app/play-on.tsx` can go).
- `CastTransport`: a stable device id for the controller-storage key (two TVs with one name share a key today).
- `SetupCodeService`: surface `device_approval_pending` / `device_denied` itself; honour `retryAfterSeconds` when a code cannot be created.
- `LocalSessionRefresher`: say when a renewal is refused for hours, so a running session can show it (today only a launch does).
- Still open from the earlier lane: `detail-actions.ts` `viewer.local`, web-only rows, `pending-track-choice` file tracks, `artwork/scheduler.forget`.

**Server**
- A position (and duration) on `GET /v1/playback/sessions/{id}`, or the controllable-devices route, for Play on skip and scrub.

**Documents:** the networking contract and `06 Clients.md` should say native tries HTTP after HTTPS for public addresses and that Apple builds carry no transport restriction.

## 8. Test notes for other environments

Build first (both Android flavours, iOS, tvOS). `Native/PorticoCast.m` gained `constantsToExport`; both `Info.plist` files changed.

- **A. Address.** Type `media.example.com` for a server that answers only on `http://…:32500`: it connects after HTTPS fails (up to about 20 s). On iPhone and Apple TV this is the test that App Transport Security is off. Type an IPv6 address bare and bracketed. Type nonsense: "That isn’t a server address Portico can use".
- **B. Plain-HTTP note.** From outside the home networks over HTTP: phone form, leave the address field: one quiet line with a link. TV: the line under the code. At home or over HTTPS: nothing.
- **C. Library memory.** Pick a tab, sort and filter; leave; reopen from the rail: restored. Open a genre from a title page, then reopen from the rail: memory unchanged. Switch profile: its own. Kill and relaunch: restored.
- **D. Missing.** Remove a film's file and scan: its card is dimmed with "Unavailable", its page says Missing with the date, Server › Storage lists it; "Remove them now" needs the typed count. A two-version film lists Version 1 and 2.
- **E. Another server / hours.** Point the saved address at a different server: "A different server is answering", sign-in kept. Set allowed hours that exclude now, relaunch: the hours sentence, sign-in kept, opens by itself when the hours return.
- **F. Watch Together** (two devices): seek: badge says "Waiting for everyone"; a restricted title shows as hidden; turn activity privacy off: "Private viewer"; background the host: Become host appears at two minutes, not before; change title while paused: no sound from the new title.
- **G. Play on.** Phone to TV: the bar appears; Pause, Play, Start over, Stop act on the TV; pause with the TV remote shows within 5 s; kill and reopen the phone app: the bar returns.
- **H. Cast** (HTTPS server, `cast.applicationId` set to the build's): cast, kill the app, reopen: controls return without re-pairing and playback does not restart. Clear the server setting: no Cast row.
- **I. TV activation.** Block the account service with a 503 and `Retry-After: 120`: the code stays, no request for 120 s (proxy log), Try again sends one at once.
- **J. Report.** Settings › About › Send a report: owner sees it under client reports. Force a fatal JS error, relaunch: the row's footer mentions it; send; footer gone. Send until the account's daily amount (1 MiB) is used up: the limit sentence.
- **K. Device approval.** With "approve new devices" on, approve a TV code from a phone: the TV says it is waiting for the owner, then signs in after approval.
- **L. Sign-out.** With downloads: the switch appears with their size; on: files gone, the server's device list updated; off: files stay.
- **M. TV focus.** Missing-title rows, the Send a report row and Become host must be reachable with the remote; the plain-HTTP note is text only.

## 9. Not finished

- **Nothing was run on a device or built.** Layouts, focus and the three native edits are unverified.
- **Version before play is inert until the playback lane joins it** (two lines, section 7).
- **Play on has no skip or scrubber** (decision 9), and its bar is handheld and tablet only (TV never sends).
- **Paused Watch Together loads can still show a first frame** on a fast start (B-13.18).
- **Outside allowed hours is said at launch and sign-in only**; a session already running sees ordinary request failures until relaunch.
- **A code that cannot be created during an outage is retried locally every 5 to 30 seconds**; no request is sent, but the screen does not say how long is left.
- **Cast**: two televisions with the same name share one stored proof; Android has no Cast module at all (unchanged).
- **Owner tools kept web-only on native**: member deletion, rating-system picker, tokens, webhooks, assistants, metadata settings, needs-attention, merge and split, remove library, restart, restore progress, client-report reading.
- **Title page**: Play next, Add to queue and Delete are not offered (need the player's queue and an owner delete flow).
- **Literals left** (no catalogue entry): two leads in `DirectSignIn.tsx`, two host-only notices in `app/together.tsx`, four clean-up and profile messages in `app/session.tsx`, `tierLabels` from shared `panel-logic.ts`.
- **Tolerant parsing**: native has no document parsers of its own beyond exact credential checks; nothing was changed.
- **Lyrics**: no change needed; the panel already reads when a song opens, and the server now picks the default.
- **Vega**: type-checks; the activation gate and device-approval changes are mirrored in its override; error capture is imported in its `index.js` but was not checked against its runtime's start order.

## 10. Join follow-up (after the merge with native-playback)

Commits `228efad`, `30b8435`, `e5d9a64` on `audit-implementation/native-sync`; `f87b27e` on `audit-implementation/native-strings` (server repository, `packages/i18n` only). Both type-checks, the Vega type-check and both string lints are clean; 22 touched test files pass; every Android Kotlin source type-checks with the standalone compiler (with the `portico-mpv` module's Java source added to the script's inputs). Nothing built or run on a device.

- **Settings.** Put back on top of the player lane's `SettingsPage.tsx`: Send a report (About), the privacy note, and `settingsScopeLabel` for the scope wording. `PlayerOptions.tsx` had lost nothing of mine.
- **Version before play is live.** The player starts a title on the version picked on its page (`usePlayer.ts` `startSingle` passes `takePendingVersion(itemId)` as `versionId`). The picker now lists the title's real playback versions (`GET /v1/items/{id}/playback-options`: label, edition, a multi-part copy as one version), not the per-file offers, and the guard flag is gone. Section 9's "inert" line no longer applies.
- **Strings.** All 12 staged sentences, the player lane's 9, and 4 more (two direct sign-in leads, two host-only notices) are in the catalogue; `new-words.ts` and `player-words.ts` are deleted and every call site uses `i18n.t`. `qualityInForceLabel` now takes the full translate function.
- **Subtitle addresses** (`/v1/media/subtitles/{grant}/{resource}/{revision}…`). Seven places matched the old shape by hand and are fixed: `src/player/engine-media.ts` (delivery and font-list addresses were being dropped); `Native/PorticoSubtitleSelection.m` and `PorticoSubtitleSelection.kt` (the text cue document was refused as "not this playback's"); `PorticoMpvSupport.swift`/`.kt` `subtitleFileBelongs` and `PorticoMpvRelay.swift`/`.kt` (mpv could not load originals, fonts or HLS subtitle renditions); `PorticoMpvCore.kt` log redaction (it would have left the grant in the log). Downloads use the server's relative bundle paths and Vega uses the address as given: no change. Tests updated (`engine-media.test.ts`, `PorticoMpvTests.swift`).
- **Still open from the player lane's side:** a paused start for Watch Together (B-13.18), `PlayerActions.enqueue` for Play next on the title page, and the playback report's shared-package requests 7.1–7.4 and 7.6 (not strings; not done here).
