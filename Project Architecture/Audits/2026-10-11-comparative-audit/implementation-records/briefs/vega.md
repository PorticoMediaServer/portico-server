# Lane: vega — the Vega (Fire TV) app at parity with the other television apps

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-vega` (branch `audit-implementation/vega`, repository portico-react-native). You can commit. Do not install packages. Nothing can be built for or run on a Vega device here. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/vega.md`.

The Vega app (`apps/vega`) is a thin variant of the React Native app (`apps/native`) for Amazon's Vega operating system on newer Fire TV devices: it shares `packages/client-core`, the screens and most behaviour, with Vega-specific native pieces and an older React Native version (audit document 19, entries A-19.4 and nearby, explains the constraints; document 21 covers TV experience). Justin's rule: every first-party client has every feature; where a platform truly cannot do something, hide it there cleanly.

Today a great deal landed in the shared packages and in `apps/native`: a rebuilt playback path with one capability document and re-plan, quality with Automatic, a second player engine (mpv, which does not exist on Vega), remembered library tab and sort, missing and versioned titles, downloads changes, Watch Together states, Cast and "Play on" controls, TV activation behind the Hosted outage gate, Send a report, the plain-HTTP note, new settings rows, many new strings. The reports are in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/` (native-playback.md, native-sync.md, native-app.md, apple-mpv.md, android-mpv.md, downloads.md, web-app.md, playback-core.md).

## What to deliver
1. **A parity inventory**: every screen, setting row and player ability the Android TV / Apple TV app now has, against what the Vega app shows and does. Put the table in your report.
2. **Close every gap that is code**: wire the Vega app to the shared pieces it is missing, so it has the same screens and behaviour; make its player use the same shared playback service and session controller as the others, publish an honest single capability document for the Vega player (native profile only), follow the plan, re-plan in place for quality, audio and subtitle changes, the quality menu with Original, Automatic and fixed qualities, text subtitles with caption style, remembered tracks, chapters if available.
3. **Hide what Vega cannot do**, by capability and not by scattered platform checks: the mpv engine and the Player engine setting, anything else its platform lacks (downloads if TV builds have none, Picture in Picture, and so on). Styled and image subtitles on Vega are burned in by the server: make sure its capability document says so.
4. Vega type-checks clean (`apps/vega` has its own check; find the command the other lanes used in their reports), and anything shared you touch still type-checks in `apps/native` (`npx tsc --noEmit`, `npx tsc -p test/tsconfig.json --noEmit`).
5. Device test notes for a Vega Fire TV: the first-run path, sign-in by code, browse, play (direct, remux, converted), quality change, subtitles, Watch Together join, a report sent.

## Boundaries
- `packages/*` are copies of the server repository's packages: never edit them here. List needed shared changes exactly under "Needed from others".
- Do not change `apps/native` behaviour for the other platforms; shared code you must touch stays platform-neutral.
- Match the existing TV screens exactly (focus, layout, wording through the catalogue).
