# Lane: native-sync — bring the native apps level with what the server and shared packages now do (everything except the player)

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-sync` (branch `audit-implementation/native-sync`, repository portico-react-native). You can commit. `apps/native/node_modules` is a symlink; do not install. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/native-sync.md`.

The server repository's base branch (`/home/justin/projects/Portico/.worktrees/audit-implementation/main-server`, read-only for you) now contains a great deal of finished work, including new versions of the shared packages (`packages/client-core`, `packages/i18n`, `packages/design`, `packages/contracts`). The native repository still holds older copies. Your job is to copy the current shared packages across and make the native apps (iPhone, iPad, Apple TV, Android phone and tablet, Android TV, Fire TV, and the Vega app in `apps/vega`) consume what is new, outside the video player.

## Steps
1. **Mirror the shared packages**: in your worktree run `PORTICO_SERVER_REPO=/home/justin/projects/Portico/.worktrees/audit-implementation/main-server node scripts/shared-packages.mjs --write` and commit the result as its own commit, unchanged. A **native-playback** lane is starting at the same moment from the same base, runs the same command, and owns every player file (`apps/native/src/player/**`, the player screen, `apps/native/src/bridge/player-engines*`, `mpv-profile*`, the capability probe, the native engine modules in Swift, Objective-C and Kotlin, and the playback parts of settings). The shared package's playback code changed a great deal (one capability document, a re-plan operation, legacy playback pieces removed), so type errors **inside those player files are expected and are not yours**: do not edit them. Make `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` in `apps/native` (and the Vega type-check) clean for everything else, and list in your report the player-file errors that remain so the two lanes' work can be joined.
2. **Replace local stand-ins with the shared ones**: earlier native lanes staged strings in `apps/native/src/app/new-words.ts` and a Player engine setting and strings in `apps/native/src/player/engine-strings.ts`, `EngineSettingRow.tsx` and `engine-media.ts`. The canonical catalogue and shared settings structure now contain them: the old-name → new-key map is in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/web-app.md` (section 10). Swap them and delete the stand-ins.
3. **Consume what the reports list for native clients.** Read the "Needed from others" and "API changes" sections of these reports in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/` and do the native part of each (screens, state, strings), on phone, tablet and TV layouts:
   - `web-app.md`: remembered library tab, sort and filter needs a native storage adapter; tolerant parsing conventions; the device-class setting labels.
   - `networking.md`: the quiet plain-HTTP note on sign-in; remove any retained "require HTTPS" or "treat WAN as LAN" choices; server address entry that accepts plain HTTP public addresses and IPv6 literals.
   - `identity.md`: a "missing" state on cards and title pages; versions listed on a title with a way to pick one; nothing that assumes a title is one file.
   - `providers-matching.md`, `api-platform.md`: owner tools stay on the web; only make sure nothing on native breaks.
   - `social.md`: the client side of the bounded wait after a seek, Cast reconnection and "Play on" control.
   - `hosted.md`: TV activation obeying the shared Hosted outage gate (the exact behaviour is spelled out there).
   - `ops-packaging.md`: sending a crash and log report to the owner's server (a small "Send report" action in Settings → About, and automatic capture of a fatal JavaScript error for the next launch), using the route it added.
   - `accounts-security.md` (if present by the time you reach it): member deletion is web-only; the "outside allowed hours" state and anything else it lists for native clients.
   - `downloads.md` "Not finished" items that are outside the player and the download manager (sign-out and stored files, the note on originals a device cannot open).
   - `music-audiobooks.md`: lyrics that appear on their own; nothing else (an audio-engines lane owns the engines).
   - `native-app.md` "Not finished": the items that were waiting on shared-package changes that now exist.
4. Keep the Vega app compiling and behaving the same as the others wherever it shares these screens.

## Boundaries
- `packages/*` in this repository are copies: never edit them by hand; only the mirror script writes them. If the shared code needs a change, describe it exactly under "Needed from others".
- Player engine modules, the player screen, the capability probe and quality or engine settings belong to a later player lane (a native-playback lane). A **downloads** lane owns the download manager and download screens; an **android-mpv** lane is writing Kotlin engine code; an **audio-engines** lane owns the music engines. Stay out of all four.
- Frontend quality matters here: match the existing native screens' components, focus behaviour on TV, and wording. Every string through the catalogue.
- Checks: both type-checks, the Vega type-check, `npm run lint:strings`, and the test files for what you touch. No Gradle, no Xcode, no full suite. Leave device test notes.
