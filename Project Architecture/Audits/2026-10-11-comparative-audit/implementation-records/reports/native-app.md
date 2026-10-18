# Lane report: native-app

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-app`, branch `audit-implementation/native-app` (portico-react-native), 20 commits over `5c31d51`, all by Justin Ehler, nothing pushed. `packages/` is untouched.

**How far it was checked.** From `apps/native`: `npx tsc --noEmit` passes; `npx tsc -p test/tsconfig.json --noEmit` passes; `npm run lint:strings` passes (0 hard-coded strings, baseline 5); the 26 test files that touch what changed were run one at a time and pass (59 new tests: 7 new files and additions to 3). `apps/vega` type-checks (with a git-ignored `node_modules` symlink to the main checkout's, as `apps/native` already has). Every Android Kotlin source type-checks with the Kotlin 2.1.20 compiler from the Gradle cache against android-36 and the cached library jars: `_program/native-ktcheck.sh <apps/native>` (about a minute, no Gradle run). **Nothing was built with Gradle or Xcode and nothing ran on a device. The full test suite was not run. The one Apple change (an Info.plist line) was not validated.**

## 1. Outcome

1. **Open screens keep what they loaded when the sign-in is renewed.** Every 13 minutes each open screen used to throw its content away and load again. The viewer is now one stable object per person and server, and the inbox, the downloads follower and the library list are keyed on the viewer instead of the access token. (`289cb42`, `2ad757a`)
2. **History, Playlists, Collections and Saved views are windowed.** Each tab is one list that is itself the scroller; only rows near the screen exist, however far the person has scrolled. The playlist page and the collection/saved-view page work the same way. A first page shorter than the screen now loads the next one. (`6cb3221`, `e196706`)
3. **Play and Shuffle on a saved view cover the view.** They read the rest of the view first (up to 500 titles, the server's limit for one play request) and say so when the view is longer. A page that cannot be read is reported; nothing half-known plays. (`e196706`)
4. **Launch links open exactly once.** A link stays held until the shell actually takes it (a shell replaced in that instant no longer loses it); the same link delivered twice by the system opens once; a link to the page already showing does not stack it again; there is one reader of the launch link, which hands the sign-in screens theirs. (`732da4a`)
5. **A device that lost its storage key can be reset and signed in again** (Android 7–11, Fire OS). The sign-in screens are replaced by one that says "This device can't read its saved sign-in", with Reset this device (confirmed, with what goes spelled out) and Try again. After a reset the app restarts itself. (`7ee4058`)
6. **Media that can never be played says so.** "This can't be played", one sentence saying why, and only Close. Connection trouble still gets Try again. (`5406598`)
7. **Screen readers hear where they are and what a card shows.** Each screen is announced once on arrival; an error or empty state is spoken once; a sheet's title is spoken; cards and media rows end with "Watched" or "42% played"; every long-press action is also a named accessibility action. (`7cbad6e`, `fa5c70e`)
8. **Smaller fixes.** Artwork that had loaded and then fails is fetched again instead of staying blank (`bbd7a0d`). Play on always offers Link a TV (`71a5801`). A wide tablet's rail has Downloads (`47034fd`). Vega no longer offers speed, gapless, crossfade or levelling; device-class settings no longer say "This device" (`62be9e3`). A Portico Account owner can edit details (`bd034ed`). My List reaches Spotlight and Siri again (`9c38024`). The iPad lists all four orientations (`acf7cb9`). The Android TV / Fire TV Menu key opens a menu instead of pressing the control (`383326f`). Card captions have one height so grid rows line up (`d75e74a`). A typed server address gets port 32500 and the right scheme (`9900333`).

## 2. Decisions I made

1. **New strings are staged, not added.** `packages/i18n` is a copy that the next sync overwrites, and the brief forbids editing it, but the brief also says "add the strings". They live in `apps/native/src/app/new-words.ts` under their final catalogue names; `word(id)` reads the catalogue as soon as the entry exists and uses the staged English until then. The list to add is in section 7. When added, delete the entries and call `i18n.t`.
2. **Saved view playback is capped at 500 titles** and says "This view has more than 500 titles. The first 500 are playing." The server refuses query selectors today; playing a whole longer view needs a server `view` container (section 7).
3. **An invitation link that arrives signed out waits 30 minutes**, not the contract's ten, so there is time to create an account. Other held links keep ten minutes.
4. **Duplicate-delivery windows:** the same link again within 5 seconds is the same delivery; 10 seconds for the launch link (Apple re-sends it about 2 seconds after launch). A deliberate second tap after that opens normally.
5. **Key loss is reported in two cases, still never acted on automatically:** key absent with records present before Android 12 (the brief's case), and the system answering that the key entry is unrecoverable, on any version. Both need the person's confirmation; the reset re-checks first and removes nothing if the key answers.
6. **After a reset the app restarts its JavaScript** (`ReactHost.reload`) rather than patching state, so nothing read from the old records survives in memory. If that is unavailable the screen says "Close Portico and open it again to finish."
7. **Downloads are kept through a reset.** They are authorised by server-signed receipts, not the lost key. The confirmation says "Downloads stay."
8. **What counts as never playable:** server answers `not_found`, `forbidden`, `access_revoked`, `unsupported_media`, `unsupported_tuple`, `transcode_not_allowed`, `transcoding_disabled`, `unknown_source_facts`, `encoder_failure`; an engine that gave up while the server was reachable; and a file refused (403/404/410 or a local delivery error) **twice in a row** for the same title. The first refusal keeps Try again because it can be an expired stream. Live channels and downloads keep their existing messages.
9. **The failure facts are read by a second listener** on the engine's event (`src/player/failure-facts.ts`) so the player controller, which the apple-mpv lane owns, is untouched. A cleaner feed is in section 7.
10. **Back from filters keeps applying changes live** (B-21.15), as Plex does. Contract NAV-6.7 says Back restores; the contract needs correcting, not the app.
11. **Detail's Edit gate:** the shared rule still requires a direct sign-in, so the native title page passes "yes" for any owner. Remove that once the shared rule drops the requirement (section 7).
12. **Announcements are per screen and only while it is current**, which also keeps one profile's titles from being spoken after a profile switch. Loading is not announced.
13. **Server address guess:** a number or a home-only name (one word, `.local`, `.lan`, `.home`, `.internal`, `.home.arpa`) means `http://…:32500`; any other bare name means `https://`. A public IP is guessed HTTP because no certificate can exist for it; today's shared code still refuses public HTTP until the networking lane lands the "HTTP from any address" decision.

## 3. Bugs

**Document 19**
- B-19.1 — fixed (the three gaps left by the fix pass). `732da4a`
- B-19.2 — fixed. `289cb42`, `2ad757a`
- B-19.3 — fixed; confirmed first in the pinned React Native (`Pressability.js` calls `onPress` on key-up unless a long-press handler fired). `383326f`
- B-19.4 — fixed for Android 7–11 and Fire OS (Q-11). `7ee4058`
- B-19.5 — fixed (retry and bounded set). Not fixed: a retry that succeeds is cached under its retry address, so the next mount downloads once more.
- B-19.6 — not fixed. It is the missing error journal (F-19.1/F-19.2), not a defect; swallowing errors without one would hide them.
- B-19.7 — fixed, unvalidated (needs one archive validation). `acf7cb9`
- B-19.8 — not fixed. Speculative; needs a device that reports rotation.
- B-19.9 — not fixed. It is in `Native/PorticoPlayer.m`, which the apple-mpv lane is editing.
- B-19.10 — not fixed. Needs a "use the TV layout" override (F-19.6), a feature.
- B-19.11 — not fixed. Vega has no keychain.
- B-19.12 — not fixed, deliberately: a blind first-launch Keychain wipe risks signing people out wrongly (a background launch before first unlock reads no marker). Design: marker in UserDefaults, checked only when `isProtectedDataAvailable`, then delete Portico's Keychain services. Needs a device.
- B-19.13 — fixed for row height. The large-offset precision question is untouched (needs a very large library on a device). `d75e74a`
- B-19.14 — not fixed. Freezing covered screens changes TV focus behaviour and needs a device.
- B-19.15 — not fixed (process).
- B-19.16 — partly fixed: the artwork label and the saved-sign-in message. About 70 literals remain in `src/app/session.tsx`, `src/app/direct-connect.ts`, `src/app/browse.ts` (`fieldNames`, `quickFilterLabel`) and `DirectSignIn.tsx`; none has a catalogue entry.
- B-19.17 — not fixed. In `packages/`; needs a runtime check.
- B-19.18 — not fixed. Unconfirmed (depends on the SDK), and a blind scene migration could break a launch that works today. One Xcode 27 build settles it.
- B-19.19 — not fixed (three low leftovers).

**Document 21**
- B-21.1 — fixed. `71a5801`
- B-21.2 — already fixed.
- B-21.3 — fixed. `47034fd`
- B-21.4 — fixed. `9c38024`
- B-21.5 — fixed. `6cb3221`, `e196706`
- B-21.6 — not fixed. The rows are web-only in `packages/client-core` (section 7).
- B-21.7 — fixed. `62be9e3`
- B-21.8 — not fixed. Needs a server occurrence anchor (unchanged from the fix pass).
- B-21.9 — not fixed. A subtitle choice can only be carried by language or by a session resource id that does not exist before play; needs a shared change (section 7).
- B-21.10 — fixed. `62be9e3`
- B-21.11 — fixed up to 500 titles. `e196706`
- B-21.12 — fixed in the menu, show page and title page. `bd034ed`
- B-21.13 — already fixed.
- B-21.14 — fixed (repeated links coalesce). The lookup request is still not aborted, only its result dropped.
- B-21.15 — not changed, by decision 10.
- B-21.16 — fixed by a named accessibility action on every long-press control; no visible More button was added. `7cbad6e`

**Document 22** (native parts)
- B-22.7 — fixed on native. The web has the same helper. `9900333`
- B-22.4, .9, .11, .13, .18, .20, .23 — already fixed.
- B-22.15 — not fixed. Needs an operation class from the shared request layer before the three transports can reserve capacity.
- B-22.17 — not fixed. Needs `known-servers` in `packages/` to store the last verified address.
- B-22.19 — not fixed. Vega's XHR follows redirects with no hook; whether it forwards bodies needs a device check first.
- B-22.21 — not fixed. Needs a secure hint in the server's discovery record.
- B-22.5, .6, .8, .10, .14, .16, .22, .25, .26 — in `packages/`, the server or Hosted; not this lane.
- B-22.1, .2, .3, .12, .24 — not native.

**B-20.12** (caption style) — not fixed; nothing of it is outside the engine modules. For the player lane: the fixed style is in `Native/PorticoPlayer.m` (`PorticoPlayerView`, the `UILabel` and its `layoutSubviews` font size capped at 40) and `PorticoPlayerView.kt:31–83` (`TextView`, white, 12–40 dp). They should read `MACaptionAppearance` / `CaptioningManager` and Portico's `playback.subtitleSize` and `playback.subtitleBackground`; the settings rows are web-only in `settings-structure.ts:205–209`.

## 4. API changes

None. No route was added or changed.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

None added. Two changes to what Settings shows: speed, gapless, crossfade and volume levelling are hidden on Vega; quality, playback method and HDR say "All your phones and tablets" or "All your TVs" instead of "This device".

## 7. Needed from others

**Strings to add to `packages/i18n` (en-US), names as used:**

| Name | Text |
|---|---|
| `signIn.savedUnreadable` | This device couldn’t read its saved sign-in. Sign in again. |
| `signIn.reset.title` | This device can’t read its saved sign-in |
| `signIn.reset.body` | Reset this device to sign in again. |
| `signIn.reset.action` | Reset this device |
| `signIn.reset.confirmTitle` | Reset this device? |
| `signIn.reset.consequence` | Everyone is signed out of Portico on this device, and the servers and settings it remembers are forgotten. Downloads stay. Nothing on your server changes. |
| `signIn.reset.confirm` | Reset |
| `signIn.reset.failed` | This device couldn’t be reset. Try again, or clear Portico’s storage in this device’s settings. |
| `signIn.reset.working` | Starting Portico again… |
| `signIn.reset.reopen` | Close Portico and open it again to finish. |
| `player.cannotPlay.title` | This can’t be played |
| `player.cannotPlay.thisTitle` | This title |
| `player.cannotPlay.gone` | {title} is no longer on this server. |
| `player.cannotPlay.denied` | {title} can’t be played on this profile. |
| `player.cannotPlay.unsupported` | {title} can’t be played on this device, and the server couldn’t convert it. |
| `player.cannotPlay.conversionOff` | {title} needs converting to play on this device, and converting is turned off for this account. |
| `player.cannotPlay.unreadable` | {title} couldn’t be read by the server. The file may be damaged or incomplete. |
| `player.cannotPlay.fileMissing` | {title} couldn’t be sent by the server. Its file may have been moved or deleted. |
| `a11y.artwork.unavailable` | Artwork unavailable |
| `a11y.artwork.unavailableFor` | {title}, artwork unavailable |
| `saved.view.gathering` | Getting the rest of this view… |
| `saved.view.partial` | This view has more than {count, number} titles. The first {count, number} are playing. |
| `settings.scope.handhelds` | All your phones and tablets |
| `settings.scope.televisions` | All your TVs |

Also wanted in the catalogue: the remaining B-19.16 literals, and `playbackUserMessage` in `client-core/src/playback-messages.ts`, which is hard-coded English.

**`packages/client-core`**
- `presentation/detail-actions.ts:105`: drop `&& !!viewer?.local` from `canEdit` (B-21.12); then native `Detail.tsx` passes the real authority again.
- `presentation/settings-structure.ts`: remove `platforms: ['web']` from `search.rememberHistory` and `notifications.badges`, which native already obeys (B-21.6); and from the subtitle appearance section once native draws captions in that style (B-20.12).
- `pending-track-choice.ts`: let `subtitle` name a file track (stream index, or title plus forced), matched against the session's plan in `advancePendingChoice`. Then `PlaybackChoices.tsx` can list unknown-language tracks and tell SDH and forced apart (B-21.9).
- `PlaybackSnapshot`: carry the failure's code and whether it is final (`fail(intentId, message, {failure})`), so the player card stops matching on sentences and `failure-facts.ts` can go.
- `known-servers.ts`: store the last verified address beside the logical one (B-22.17).
- A request class (control or read) on the shared request path, for B-22.15.
- `artwork/scheduler.ts`: a `forget(key)` so a failed reload can go back through the scheduler.

**Server**
- playbackv1: a `view` container (or the query selector) so a saved view plays as one request. Then add `'view'` to `KINDS` in `apps/native/src/app/container-play.ts`, pass `{kind: 'view', id}` from `SavedResourceScreen`, and delete `view-play.ts` and `useGatherView`.
- playbackv1: a playlist occurrence anchor (B-21.8).
- Discovery record: a secure address hint (B-22.21).

**Player lanes (apple-mpv, android-mpv, native-playback)**
- B-20.12 as described in section 3.
- If a second engine reports failures on another event than `PorticoPlaybackFact`, `src/player/failure-facts.ts` must listen to it too, or the "never playable" card falls back to "Playback stopped / Try again" for that engine.
- I edited two files near their area, both in small, separate places: `src/player/VideoPlayer.tsx` (the error card, one ref and a guard on the remote's play key) and `src/player/PlayerHost.tsx` (one effect).

**Web lane:** the same bare-address guess (B-22.7) is in `web/src/app/session.tsx:166–172`.

**Documents:** NAV-6.7 (filters and Back); NAV-11 (invitation lifetime of 30 minutes); `06 Clients.md` does not mention Vega.

## 8. Test notes for other environments

Build first: the Android app (both flavours) and the iOS and tvOS targets. Step zero on Android is that it builds; the Kotlin was type-checked but never linked.

**A. Renewed sign-in (any device, 15 minutes).** Open Home, a library tab, a title page, Saved › History, and start a download. Leave the app in front for 14 minutes. Pass: no screen returns to loading, TV focus does not move, the download continues, the library list does not flicker. Fail: any reload at about 13 minutes.

**B. Saved lists (TV and phone; an account with 300+ history entries and a 500+ entry playlist).** Scroll History to the end. Pass: smooth; day headings correct; one card per title; Clear history works; pull to refresh works; memory does not climb with distance. On TV: focus moves between tabs, cards and back to the rail; Back returns to the card you left. Repeat on Playlists, Collections, Saved views, a playlist page (long-press or Menu reorders and removes) and a collection page. Check a tab whose first page is shorter than the screen still loads the next.

**C. Saved view playback.** A view of about 120 titles: open it, do not scroll, press Play. Pass: "Getting the rest of this view…", then playback; Up Next holds all 120. Shuffle the same. A view of 600: the "more than 500" message appears and 500 play. Turn the network off mid-read: an error, nothing plays. Leave the page mid-read: nothing plays later.

**D. Launch links.** Use the fix-pass note for cold-start links (`report-native.md`), then: (1) Apple, cold launch from a Top Shelf card or a `portico://item/…` link: the page opens once, not twice. (2) Send the same link twice within 5 seconds while running: one page. Send it again after 10 seconds from another page: it opens. (3) Open a link to the title already showing: no second copy. (4) Signed out, open a server invitation link, then sign in to the Portico Account within 30 minutes: the join screen appears; after 31 minutes it does not. (5) Provider sign-in that returns by universal link, cold: sign-in completes.

**E. Key loss (disposable Android 7–11 device or emulator, and a Fire TV).** Sign in; save a server, a profile and a download. Force-stop. With a debug hook or `run-as`, delete only the Keystore alias `tv.getportico.portico.records.v1`, leaving `no_backup/portico-records/`. Launch. Pass: within about 2 seconds the screen "This device can’t read its saved sign-in" with Reset this device and Try again; no sign-in form; on TV no blank activation screen. Try again: the app restarts and shows the same screen. Reset this device › Reset: "Starting Portico again…", the app restarts on the sign-in screen with the sign-in-ended note; sign in; relaunch; still signed in; the download is still listed and plays. Negative controls: (1) make the Keystore unavailable for the first read only (instrumentation): no reset screen, saved sign-in intact. (2) With the reset screen showing, restore the key, press Reset › Reset: the app restarts **signed in**, records byte-identical. (3) Flip one byte in a record file: no reset screen. (4) Kill the app mid-reset: next launch finishes the reset and never shows old data. On Android 12+: unchanged automatic recovery. Also confirm `ReactHost.reload` works in a release build; if it does not, "Close Portico and open it again to finish." must appear.

**F. Never playable.** (1) Delete a file on the server and play its title: "This can’t be played", "… is no longer on this server" or "… couldn’t be sent…", only Close; on TV Close has focus and the remote's play button does nothing. (2) A profile restricted from a title: the "this profile" sentence. (3) A file this device cannot decode with conversion disabled: the conversion sentence. (4) Pull the network during playback: "Reconnecting", and Try again after give-up, never the new card. (5) Let a paused stream expire, resume: the first failure shows Try again, and Try again plays. (6) Live TV and a download failing: old messages.

**G. Screen readers.** VoiceOver on iPhone and Apple TV, TalkBack on Android and Android TV. Each screen's name is spoken once on arrival and on coming back; pull to refresh does not repeat it; a covered screen stays silent. Cards read "Title, caption, Watched" or "…, 42% played". With a long-press control focused, the Actions rotor (VoiceOver) or actions menu (TalkBack) offers "More for …" and it opens the menu; double tap still opens the title. A playlist entry can be reordered that way. Opening a sheet speaks its title. An empty watchlist is spoken once. Check nothing is spoken twice on iOS (the name is queued behind the system's own reading).

**H. Menu key (Fire TV remote).** Focus Sign out or Play and press Menu: nothing happens. Focus a card: its menu opens once, on release. Hold Menu: still once. In a debug build Menu must not open the developer menu.

**I. Others.** Wide iPad: Downloads in the rail, selected state follows, rotating to narrow keeps Downloads current. Play on with Local Network denied: Link a TV is offered and works while playback continues. Vega: Settings shows no speed, gapless, crossfade or levelling. Portico Account owner on a phone: Edit details appears and saves. iPhone: add a title to My List, refresh Home, find it in Spotlight. Book library with long titles at the largest text size: jump to a late letter and land on it. Server address: type `192.168.x.x` alone and connect on port 32500. iPad archive: validate for ITMS-90474. Artwork: browse, stop the server, scroll back to evicted images (placeholders with titles, not blanks), restart the server (images return within 20 seconds).

## 9. Not finished

- **Nothing was run on a device, and Gradle, Xcode and the full test suite were not run.** Everything above is type-checked and unit-tested only.
- **24 strings are staged in `new-words.ts`, not in the catalogue** (section 7). They show in English until added.
- **B-20.12 caption style: not done** (engine modules).
- **Saved view playback stops at 500 titles**; the whole-view fix is a server piece.
- **B-21.9, B-21.8, B-21.6, B-22.15, B-22.17, B-22.19, B-22.21: not done**, each waiting on a shared-package or server change listed in section 7.
- **B-19.6, .8, .9, .10, .11, .12, .14, .15, .17, .18, .19: not done**, for the reasons in section 3. B-19.18 (Apple scenes) is the one that could block a launch on a new SDK and needs one build to settle.
- **B-19.16: about 70 English literals remain** outside the catalogue.
- **The "never playable" card depends on matching four fixed sentences and nine codes** from the shared package, and on a second engine using the same event. If either changes, it degrades to the old "Playback stopped / Try again", not to a wrong message.
- **Key loss on Apple and Vega:** no equivalent state exists on Apple; Vega was not examined for one.
- **The mini player and TV pill** do not show the "never playable" wording; only the full player does.
- **If the My List read for Spotlight fails on a fresh launch**, Continue Watching is not re-indexed until a later Home refresh succeeds (the previous index is left in place).
