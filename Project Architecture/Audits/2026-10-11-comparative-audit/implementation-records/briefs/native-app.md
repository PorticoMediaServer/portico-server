# Lane: native-app — the React Native apps outside the player

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-app` (branch `audit-implementation/native-app`, repository `portico-react-native`). You can commit. `apps/native/node_modules` is a symlink; do not install anything.
Audit documents: 19, 21, 22 (native parts); verification V10, G4. Also `fix-pass/lane-reports/report-native.md` and `report-native-platform.md` for what the fix pass already did and left open (Q-11, the pending-link findings).

## What to deliver
1. Every confirmed bug in documents 19, 21 and 22 that is still present and is not a player-engine bug, including: History loading or truncating instead of windowing (B-21.5); Saved View playback covering only what was loaded (B-21.11); the remaining cold-start deep-link problems (consume before dispatch, duplicate delivery, the two launch readers); screens that rebuild services on every token renewal (B-19.2).
2. **Android 7–11 and Fire OS after real key loss** (Q-11): today the person is stuck until app storage is cleared. Build the confirmed-reset path: detect it, explain it in one plain sentence, and let the person sign in again after confirming. Add the strings.
3. An honest, plain presentation when media can never be played (a permanent error), instead of retrying.
4. Screen-reader announcements on screen change, and cards that announce watched and in-progress state, across the screens the audit lists.
5. Native subtitles ignore both Portico's caption style and the system's (B-20.12): fix where it is outside the engine modules; otherwise record it for the player lane.
6. Phone, tablet and TV layouts of anything you touch must all be handled. The Vega app (`apps/vega`) shares these screens; keep it compiling in type-check terms.

## Boundaries
- `packages/*` in this repository are copies of the server repository's packages and must not be edited here. If you need a change in `client-core`, `design` or `i18n`, write the exact change needed in your report under "Needed from others"; do not work around it with a local fork.
- The **apple-mpv** lane is adding a second player engine and owns the player modules, the engine setting and the player screen's engine wiring. Stay out of `apps/native` player engine code (Swift, Kotlin and the JS player controller).
- Later lanes bring the live-refresh consumer, remembered tab and sort, downloads changes and the new capability document to native. Do not start those.
- Checks allowed: `npx tsc --noEmit`, `npx tsc -p test/tsconfig.json --noEmit`, single test files, `npm run lint:strings`. No Gradle, no Xcode.
