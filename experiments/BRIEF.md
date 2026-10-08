# Portico app experiment: brief shared by both builds

Justin wants to see whether a from-scratch Portico app for iPhone/iPad and Apple TV, designed by
you rather than evolved from existing code, comes out better than the current one. Two builds
exist side by side, each in its own folder under `experiments/`, each by a different builder:

- **classic**: the standard, Plex-like interpretation (Plex, Jellyfin, Emby, Infuse territory).
- **cinema**: the creative interpretation, taking inspiration from paid streaming services (Apple TV app,
  Netflix, Disney+, Max, Prime Video), with more latitude on layout, motion and presentation.

Both are **frontend mockups on fixture data**, but they must be **100% feature complete**. Every
feature Portico's web UI offers has a real, navigable screen or interaction. The code must be
real enough to finish: swap the fixture layer for real services and ship.

## The only hard design rules

1. **Apple TV: a left rail, or at least a left menu.** Its form is yours: a persistent rail, a
   collapsible rail, or a rail that expands on focus.

   **iPhone and iPad** don't need a left rail. The expected shape is a bottom tab bar plus other
   core, fixed elements, such as:
   - a header with the profile and server switcher and search;
   - a mini player docked above the tabs.

   Use judgement on what's fixed and where. An iPad sidebar is fine if it serves the larger screen
   better.
2. **Portico's signature blue glow.** The navy canvas has a blue projection glow falling from the top left.
   Reference: `apps/portico-react-native/apps/apple/src/ui/Canvas.tsx` (the `Atmosphere`
   component) and the colour tokens in `apps/portico-react-native/packages/design/src/index.ts`:
   - `ink` #071018: the canvas.
   - `ambient` #153749 and `raisedBright` #1d3848: the glow stops.
   - `accent` #409fd4, `accentStrong` #5cb3e2 and `accentDeep` #2f86bb: Portico blue.

   Keep the glow recognisable. You may evolve it, for example with motion, a glow that tints
   toward the focused artwork, or glow on focus.
3. **React Native, portable to Android, Android TV and Fire TV.** Use `react-native-tvos`, the
   same fork and version as the current app (`react-native: npm:react-native-tvos@0.83.10-1`, React
   19.2). Only use libraries that also support Android and Android TV: navigation, reanimated,
   svg, safe-area and similar. Write no iOS-only native modules. Platform differences go through
   `Platform`/`isTV` and layout breakpoints, not separate apps. A single codebase serves phone,
   tablet and TV.

Everything else is yours: information architecture, typography, card shapes, motion, focus
treatment, how the player looks. The existing apps have rough edges; don't copy their UI code,
though you may read them to understand the product. You're welcome to depart from their
conventions.

## Feature completeness: the web UI is the reference

The web app is the feature reference, not the current iOS/tvOS apps, which lack features. Read
`apps/portico-server/web/src`:
- the routes (grep `path:` under `app/` and `shell/`);
- every folder under `screens/`;
- `player/`;
- `shell/`;
- the dialogs and actions under `screens/shared/`.

Build an inventory. Write it to `FEATURES.md` in your project as a checklist: every web feature,
and the screen or interaction in your app that covers it. Tick every row before you finish.

The rough map (confirm and extend it from the source):

- **Getting in:**
  - Landing.
  - Portico Account sign-in (Hosted chooser) and direct server sign-in.
  - Joining a server by invitation.
  - The profile grid with PIN.
  - Linking a TV with a code or QR, and device approval.
  - Claiming a server; server setup (first run).
  - Restore; email verification; password reset.
  - Server unavailable and offline states.
  - A server switcher and a profile switcher.
- **Home:**
  - Hero or featured content.
  - Continue watching, up next and recently added.
  - Recommendations: Picks for you, Because you watched, Discover rows.
  - A row's See all page.
  - Customising Home: reorder, hide and saved views.
  - The empty state.
- **Libraries** (movies, TV, music, audiobooks, photos, home videos; include anime):
  - Discover, Browse, Collections and Categories.
  - Browse filters and sort, saving a view, alphabet jump.
  - The entity pages: genre, studio, collection, playlist and similar.
  - The no-libraries state.
- **Titles:**
  - **Movie detail:**
    - Hero, facts, play and resume.
    - Versions and editions, audio and subtitle choices.
    - Cast and crew, extras, related titles.
    - Watchlist, favorite, rating, watched and Not interested.
    - Add to playlist, the metadata editor (fix match, edit fields), and feedback.
  - **Show detail:** seasons, episodes, the episode panel or page, and next up.
  - **Person page:** credits.
- **Music:**
  - Artist, album and track lists; playlists.
  - The now-playing screen with lyrics and the queue.
  - A mini player.
- **Audiobooks:** chapters, speed and a sleep timer.
- **Photos:** a gallery and viewer with a slideshow.
- **Search:** all types, grouped results, recent searches, and TV text entry.
- **Saved:** watchlist, favorites, history, and saved resources and views.
- **Selection bar:** multi-select, then bulk actions (mark watched, add to playlist and similar).
- **Live TV:**
  - Channel list, guide grid and programme sheet.
  - Reminders; DVR (record, series recordings, recordings list).
  - The in-player mini guide and live captions.
- **Player:**
  - Video chrome: timeline with preview thumbnails, chapters, skip intro and credits prompt.
  - Audio, subtitle and quality options.
  - Post-play next-episode countdown, the queue list, and Play on (Cast, AirPlay, another Portico device).
  - The live player.
- **Together:** watch parties: start, join and the group pill.
- **Downloads** (phone and tablet): requests, progress, storage and settings.
- **Account:**
  - Profile, security (MFA, sessions and devices) and notifications.
  - Servers.
- **Settings:**
  - Playback, subtitles and languages.
  - Profiles, notifications and security.
  - Reset recommendations.
- **Server administration** (owner and admin only). The web's `/server/$section` pages include:
  - Overview, Libraries, Library settings, Access and people.
  - Now Playing, Activity, Logs.
  - Transcoding, Network, Storage.
  - Backups, Maintenance, Schedules, Diagnostics.
  - Live TV, DVR, Channels, Channel Builder.
  - Metadata, Alerts.

  Cover every section. On TV a read-mostly presentation is fine, but every section must exist and
  show a believable populated state.
- **States:** loading skeletons, empty states, errors, and a reconnecting banner.

## Fixture data

- A populated household:
  - Several profiles, one of them a kids profile.
  - Two or three servers, one of them unavailable.
  - Every media type, with enough items that grids and rows feel like a real large library.
  - Realistic metadata: synopses, years, ratings, runtimes, genres, cast, seasons and episodes,
    album tracklists, chapters, and live channels with a full day's guide.
- **Artwork must look real.** Bundle it locally so the app works with no network:
  - Posters, backdrops, episode stills, album covers, artist photos, channel logos, person
    headshots and photos.
  - Use freely licensed sources only:
    - Wikimedia Commons public-domain film posters and stills.
    - Blender Foundation open movies (CC-BY): Big Buck Bunny, Sintel, Tears of Steel, Elephants
      Dream, Spring, Cosmos Laundromat, Charge.
    - Unsplash photos via `https://picsum.photos/seed/<seed>/<w>/<h>`.
  - You may also generate artwork (typographic posters, gradients) with a script.
  - Titles can be public-domain works or invented ones. Invented titles with good generated art
    beat real titles with missing art.
  - Keep bundled assets under about 150 MB. Downscale to the sizes you actually display.
- Put all data behind a repository or service interface, not imported straight into screens.
  Replacing the fixtures with real Portico services should then be a data-layer change.
  `apps/portico-server/packages/client-core/src` shows the real service shapes, if that helps.
- Media playback can be simulated: the player chrome, timeline, options and states. Don't
  bundle video. A single short CC-BY clip is optional if it helps the player feel real.

## Build, run and simulators

- Your project lives in its own folder under `experiments/`, as its own git repository.
  - Run `git init` and commit as you go, with plain messages.
  - No AI or assistant attribution in commits (no `Co-Authored-By`, no "Generated with").
  - No AI model or agent names in code, file names or commit messages.
  - Never push.
- **Bundle identifiers and display names:**
  - classic: `tv.getportico.experiment.classic.ios` / `.tv`, shown as "Portico Classic".
  - cinema: `tv.getportico.experiment.cinema.ios` / `.tv`, shown as "Portico Cinema".
- **Justin's simulators:** "Portico iPhone" (A44FF60E-F0C1-4219-9668-8D273CFE2388, iOS 26.4) and
  "Portico TV" (C10DF26D-694F-4C65-8850-7E3E843942F8, tvOS 26.4). They hold the real Portico app.
  - **Never** erase them.
  - **Never** uninstall or modify `tv.getportico.portico.*`.
  - Don't change their settings.
- **While you develop, use your own simulators,** because the other builder works at the same
  time. Both builds share the Mac, and screenshots capture whatever app is in front.
  - Create yours with `xcrun simctl create "<classic|cinema> iPhone dev" …` and
    `"<classic|cinema> TV dev"`, on the iOS 26.4 and tvOS 26.4 runtimes.
  - Add an iPad one if you test tablet layout.
  - Delete your dev simulators when you finish.
- **Final install:**
  - Build **Release** for the simulator, with the JS bundle embedded, so the apps run without
    Metro.
  - Install on "Portico iPhone" and "Portico TV" (boot them if needed).
  - Launch once to confirm each starts, then leave them installed.
- **During development:**
  - Use a Metro port of your own: classic 8091, cinema 8092. Never 8081.
  - Put derived data under `~/Library/Caches/portico-experiment-<classic|cinema>-dd`.
  - Only stop processes you started, by PID.
  - Guard every `rm` path variable as `"${VAR:?}"`.
  - Pods: set `LANG=en_US.UTF-8` for `pod install`.
- **Make every screen reachable for review:**
  - Add a debug deep link (a URL scheme like `portico-classic://screen/<id>`) or an in-app
    "screen index" in Settings.
  - Walk every screen on both form factors and look at the screenshots yourself
    (`xcrun simctl io <udid> screenshot`, or the iOS Simulator tool if available).
  - Fix what looks wrong.
  - On TV, check focus: every control must be focusable and show focus, and the rail and Menu
    button must behave.
  - Check iPhone portrait and a large iPad layout.

## Done means

- `FEATURES.md`: every web feature ticked with where it lives.
- Both apps are installed on "Portico iPhone" and "Portico TV", launch from the home screen with
  no dev server, and show a populated library.
- A short `README.md` covering:
  - the design idea;
  - the architecture (navigation, design system, data layer);
  - how to swap in real services;
  - how to build;
  - known gaps. There should be none against the checklist; list polish you'd still do.
- Clean commits in your repository; dev simulators and derived data removed.
