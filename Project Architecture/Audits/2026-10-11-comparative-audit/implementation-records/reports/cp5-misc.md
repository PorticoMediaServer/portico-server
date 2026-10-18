# cp5-misc — remaining codeable gaps

Host ehler-personal (mini-1), 16 Oct 2026. Branch `audit-implementation/cp5-misc` in `server-cp5-misc` and `native-cp5-misc`. Commits use Justin's identity and nothing was pushed. No subagents or helpers were used. Testing was lean, as the brief asked. `audit-implementation/main` was merged into the server branch (afaa5bf3, the Hosted check-in change). Native main had not moved. After the merge, apigen showed no diff. The shared packages are mirrored at server `b58069db`, and `--check` confirms they are identical.

## 1 Outcome

1. **Letter rail on playlist pages (native).** A sorted or filtered playlist is now a windowed list in the page's own scroller. Only the rows near the viewport are read and kept. Sorted by title, it gets the letter rail: beside the list on phones and tablets, and as "Jump to" in the header on TV.
   - The page's header stays mounted when the list switches between the playlist's own order and a sorted one, so TV focus stays where it was.
   - There is a new `PageRows` (a `PageList` over `getItemCount`/`getItem`). `TrackRow` and `useTrackRowHeight` are exported from `TrackWindow`.
2. **Mixed-script subtitles on Android mpv.** Every letter of a line now draws, including Arabic with Latin and lines with three scripts. There are two parts:
   - **Engine:** a small libass patch (`third_party/media-engine/patches/libass/0001-…`). Before falling back to the default font file, libass now uses any loaded font that has the glyph, closest weight and slant first.
     - I rebuilt the engine for arm64-v8a with the patch. `verify.py android` passes: 1291 checks, 0 failed.
     - I built the same patch on the host and checked it with a test program. With the patch, an "Arial" line in Latin + Arabic + Devanagari drew every letter, with 0 missing glyphs. Unpatched libass, run on the same line, reported 23 glyphs with no fallback.
   - **App (`PorticoMpvFonts`):** mpv's fonts folder (`<config>/fonts`, set as `sub-fonts-dir`) holds links to these system faces:
     - the ordinary face;
     - the face for the track's language;
     - the face for every script seen in the track's first lines.

     `sub-font` is now the ordinary face's own family name, read from its `name` table, so Latin is always drawn in it. A styled track's attached-fonts folder gets the same links. Faces over 8 MB (the CJK collections) stay as `subfont.ttf`, because libass loads every file in the folder into memory.
   - Even without the patched engine, the app part alone fixes lines that mix two scripts.
3. **Setting up a new server from a phone or tablet.**
   - **Server:** new `POST /v1/setup/app` (`start_app_setup`). It hands the one-time setup secret to an app. It refuses any request that carries `Origin` or `Sec-Fetch-Mode`, and the router refuses a cross-site preflight for it, so no web page can reach it. It uses the same 10-attempt setup limit as the browser route.
   - **client-core:** new `startAppSetup`. The claim-code flow the web kept to itself (`beginWebClaim`, `awaitWebClaim`, `safeApprovalUrl`, `claimErrorMessage`, `ClaimError`) moved to `client-core/src/claim-web.ts`, and the web re-exports it.
   - **Native, finding the server:** on the direct sign-in screen, once the address is typed, a server that says it needs setup gets an offer: "Nobody has set up this server yet… Set up this server". Signing in to such a server also opens setup instead of refusing.
   - **Native, the setup screen:** the person names the server, chooses Portico Account or accounts on the server, and creates the owner (recovery owner). The protected draft (`nativeSetupDraft`) lets an interrupted setup resume.
   - **Native, Portico Account:**
     - If the app is signed in to a Portico Account, the server is approved with the app's own account sign-in. It is the same path as Settings › Remote access › "Turn on with {account}".
     - Otherwise the account's approval page opens in the system browser. The app waits for the decision and asks again each time it comes back to the front, because the server keeps waiting by itself.
     - "Do this later" signs in and leaves the connection to Settings.
   - **TVs** still can't set up a server. They now say where it can be done: the app on a phone or tablet, or a browser.
4. **Vega "Playing on your other devices": already there; no change needed.** Vega runs the shared native screens, including the television Settings built from the settings structure (`apps/vega/src/Root.tsx` → `native/App`; only five files are overridden). cp3-nscreens made the `remoteControls` row available on every platform, so it already appears on Vega. The claim in cp3-nscreens that "Vega does not draw the settings structure" doesn't match the code. Needs a device check (§8).
5. **Other leftovers:**
   - Tests for A14 (the admin routes that read one member, invitation or device) and A25 (an account keeps at most four waiting records for sign-ins that name no installation).
   - The unused string `web.access.deviceMeta` is removed.
   - Every Android Kotlin source compiles, in both flavours (google, firetv), including cp4-leftovers' unbuilt seek-index reopen and pinned-certificate listing. The new Kotlin tests pass, along with the existing mpv support tests.

## 2 Decisions I made

- **A separate app route instead of loosening the browser route.** `start_browser_setup` still requires the page's own origin. The app route requires that no origin is sent at all. Browsers always send one on a POST, and a cross-site JSON POST needs a preflight, which the router refuses for this path. So a web page can't reach either route. Anyone who can reach the server could already set it up from a browser at its address, and an app at that address can now do the same. No guard was weakened.
- **The setup offer appears only after the address is typed** (when the field is left). It reads the public `/v1/system` and stays quiet on any failure, like the plain-HTTP note.
- **The Portico Account step on the phone.** If the app is signed in to an account, approval is one tap using that account's first profile, as Settings already does. If not, the browser approval page is used, as on the web. I did not add a Portico Account sign-in inside setup.
- **The font chain comes from the ordinary face's family name plus a libass patch, not from a full Android font provider.** A provider would need NDK font APIs (API 29+, but the app supports 24) and a much larger engine change. The patch only changes behaviour when no provider gave an answer, so Apple's CoreText path is unaffected.
- **The playlist page now uses one `VirtualizedList` for both the playlist's own rows and the sorted rows.** Swapping components would have redrawn the hero.
- **The string `setup.app.notSetUp` is new.** I didn't edit `web.signIn.error.notSetUp`, because catalogue IDs are stable and the meaning changed: browser setup works from anywhere, and phones can now set up too.

## 3 Gaps closed

| Item (source) | Change | Test |
|---|---|---|
| Native letter rail on playlist pages (cp4-leftovers §9) | `SavedPages.tsx` PlaylistScreen on `PageRows` + `useGroupWindow` + `useGroupLetters`; `ui/Page.tsx` `PageRows`; `TrackWindow.tsx` exports | native `tsc`, `lint:hooks` |
| Android mpv mixed-script fonts (cp3-nplay §9, cp4-leftovers skip list) | libass patch + README; `PorticoMpvFonts` fallback folder, `familyName`, `scriptsOf`, `useAttachedFolder`; controller `sub-font`/`sub-fonts-dir` | `PorticoMpvFontsTest` (Kotlin, 2 tests) + `PorticoMpvSupportTest` (9) pass; engine arm64 build + `verify.py android`; host libass A/B program |
| First run on phones/tablets (cp3-securedefault §9, cp4-leftovers skip list) | server `start_app_setup`; client-core `startAppSetup`, `claim-web.ts`; native `ServerSetup.tsx`, `DirectSignIn` offer, session `setup`/`beginSetup`/`finishSetup`, `addressNeedsSetup`, `connectDirectServer({allowSetup})` | Go `TestAppSetupWithoutOriginAndNeverFromABrowser`; client-core `app-setup.test.ts` (3); native `direct-connect.test.ts` (+2); web `claim-connect.test.ts` (6) |
| Vega "Playing on your other devices" (cp3-nscreens §9) | none needed (see §1.4) | device check |
| A14 / A25 untested (cp3-api §9) | tests only | `TestAdministratorReadsOneMemberInvitationAndDeviceByID`, `TestUnnamedWaitingDevicesAreBoundedPerAccount` |
| `web.access.deviceMeta` unused (cp3-nscreens §9) | removed | `lint:strings` (web, native, Vega) |
| Kotlin uncompiled (cp4-leftovers §9) | compile check of every app source in both flavours | K2JVMCompiler 2.1.20 against android-36: exit 0 |

## 4 API changes

- New `POST /v1/setup/app` (`start_app_setup`, Public, ScopeSession, Security lane, default admission). The body is `{}` and the answer is `{serverId, setupToken}`, the same shape as `start_browser_setup`. Errors: `unauthorized`, `invalid_request`, `rate_limited`, `setup_already_complete`, `internal`. The route lane table and the restriction matrix both have entries for it.
- client-core:
  - new `setup-onboarding.startAppSetup`;
  - new `claim-web.ts`, holding the code moved from web `app/claim-connect.ts`, which now re-exports it.
- Behaviour only: the router answers a preflight to `/v1/setup/app` only for the server's own origin, which the handler then refuses.

## 5 Schema changes

None.

## 6 Settings added

None.

## 7 Tests run

- Go:
  - `go test ./internal/httpapi/ -run 'TestAppSetup|TestBrowserSetup|Restriction|Lane|Scope|Matrix|Generated|Contract|SetupPeer'` → ok.
  - `-run TestAdministratorReadsOne` → ok.
  - `go test ./internal/identity/ -run 'TestUnnamedWaiting|TestOwnerApprovalHolds'` → ok.
  - `go build ./...` and `go vet` on httpapi and identity → clean. gofmt clean.
  - `go run ./cmd/apigen` → no diff after the merge.
- client-core `npm run test:core` → 2024/2024. It caught hand-written paths in `claim-web.ts`; they now use the generated builders.
- Web:
  - `tsc --noEmit` → clean.
  - `lint:hooks` → 0 errors (two existing pending warnings).
  - `lint:strings` → clean.
  - `claim-connect.test.ts` → 6/6.
- Native:
  - `tsc`, test `tsc` → clean.
  - `lint:hooks` → 0 errors.
  - `lint:strings` → clean.
  - `npm test` → 701/701.
- Vega: `tsc`, test `tsc` and `lint:strings` → clean (existing "pending entry stale" info lines only).
- Kotlin:
  - All app sources plus tests compile (google flavour), and all sources compile (firetv flavour). I used the scratch K2JVMCompiler recipe with a JUnit stub, not Gradle.
  - `PorticoMpvFontsTest` and `PorticoMpvSupportTest` → 11/11 with the stub runner.
  - `familyName` checked on real fonts: DejaVu Sans, Noto Sans CJK JP (TTC), Noto Sans Arabic.
- Media engine:
  - `build-android.sh --abis arm64-v8a` → ok. It ends with `verify.py`, and a separate `verify.py android --abis arm64-v8a` run also passed: 1291 checks, 0 failed.
  - The patch string is present in `libass.so`.
  - The output is kept at `_program/scratch/cp5-misc/engine-out/android` (157 MB, arm64 only). The work folder and GOCACHE are deleted.
- Not run: Gradle, emulators, devices, the Mac build, or the full Go suites.

## 8 Device test notes

1. **Playlist letter rail (phone, tablet, TV):** open a long music playlist and sort it by title.
   - Pass: rows appear as you scroll, and the rail is at the right edge (on TV: "Jump to" under Sort/Filter).
   - Jump to "M": the list lands on the first M title.
   - Choosing Default order brings back the playlist's own rows, with reorder and remove menus. The hero is not redrawn, and on TV focus stays on the Sort button.
2. **Android mpv mixed scripts** (needs an engine build with the patch; arm64 output above):
   - Play an Arabic SRT that has Latin names in mpv. Pass: both scripts draw, with no boxes.
   - Repeat with an English-tagged track whose lines include Hindi or Thai. Pass: the track reopens once and the letters draw.
   - Repeat with an ASS file that has attached fonts. Pass: its fonts still win where they have the glyph.
   - logcat shows `fontselect: Using a loaded font with glyph`.
3. **Phone first run:** start a fresh server, then in the app choose sign in to a server and type its address.
   - Pass: "Nobody has set up this server yet" appears with "Set up this server".
   - Create the owner with "Keep accounts on this server". Pass: you are signed in as owner.
   - Repeat while signed in to a Portico Account in the app. Pass: the app shows "Connecting {server} to {account}", then signs in. Settings › Remote access › Secure connection shows Starting, then On.
   - Repeat signed out of the account. Pass: the browser opens the approval page; approve, return to the app, and it signs in by itself.
   - Kill the app during setup and reopen it. Pass: setup resumes and signs in.
   - On a TV, enter a fresh server's address. Pass: "Set it up from the Portico app on a phone or tablet, or open its address in a browser."
4. **Apple:** setup over plain HTTP must work on the local network (ATS local-only rule). A remote `http://` address should give the existing Apple message.
5. **Vega:** open Settings › This device while the same profile plays on another device. Pass: "Playing on your other devices" lists it, and Pause/Stop work.

**Mac checks:** I changed no Swift or ObjC.
- Rebuild the Apple engine with `third_party/media-engine/build-apple.sh` (the libass patch is now in the recipe) and run `verify.py apple`. Expect no change in behaviour, because CoreText answers fallback requests first.
- Run the iPhone/iPad setup flow from note 3.

## 9 Not finished

- **The engine is rebuilt for arm64-v8a only.** armeabi-v7a, x86 and x86_64 need `build-android.sh` (all ABIs) before a release build or an emulator build. Apple needs `build-apple.sh` on the Mac.
- **Font coverage:** CJK collections over 8 MB are not in the fallback folder. A line mixing CJK with Arabic, where the track's default face is neither, can still box one script. Faces are chosen from the first 8 lines of a track only.
- **Setup:**
  - There is no Portico Account sign-in inside setup. Signed out, approval uses the browser.
  - After the owner is created, Back abandons the account step. The person then signs in with the owner they created.
  - No render test for the setup screen.
- **Skipped, with reasons:**
  - History sorts and queues beyond 10,000, and every perf item from cp2-reads: owned by cp5-reads.
  - The counted-block trigger generator: same baseline triggers cp5-reads is editing.
  - Hosted milliseconds: cp5-hosted.
  - `paused-load.ts` removal: a playback-core change through the queue owner. Two lanes declined it; it works and has a test.
  - Show-level NFO cast on person pages: a catalogue read-path and counts change in the area cp5-reads is optimising.
  - Progressive DVB on native/mpv/burn-in: needs an FFmpeg patch.
  - Vega owner certificates and the Cast receiver: platform limits.
  - mpv live, hardware-encoder mastering metadata: need devices.
  - Roku snapshots: Roku is out.
  - A15 prune test: the sweep is inline in `cmd/server/maintenance.go`; testing it means refactoring the cadence logic, which isn't worth it for one bounded DELETE.

## Commits

**portico-server** (`audit-implementation/cp5-misc`): 7dfed9d8, 5d558d05, 024b52bf, 935aa2c6 (merge of main), b58069db.

**portico-react-native** (`audit-implementation/cp5-misc`): 2dd31c8, c31e426, b360ce9, e33899e, 4e7f6d8, 2aca4d6, 04bb59e.
