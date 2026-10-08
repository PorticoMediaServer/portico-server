# Portico Cinema

A from-scratch Portico client for iPhone, iPad and Apple TV (one React Native codebase on
`react-native-tvos` 0.83.10-1), built as a complete frontend on fixture data. This is the
*creative* interpretation: a personal media library presented like a premium streaming service.

## The design idea

**Your library, in the spotlight.** Portico's navy canvas and blue projection glow become a
cinema's projector beam: the light falls from the top left, breathes slowly, and picks up the
colour of whatever artwork has focus, so moving through a row subtly relights the room while
always staying recognisably Portico blue.

- **The marquee (TV Home).** The top half of the screen is always the focused title, full-bleed,
  with its name set large in a heavy, tightly tracked sans (Inter Tight). Shelves live in the lower half
  and slide up one at a time. On iPhone and iPad the marquee becomes a swipeable hero carousel.
- **Editorial typography.** Two families only: Inter Tight (600–800) for heroes, page titles
  and shelf headings, and Manrope, Portico's brand face ("Portico Sans"), for all UI and body
  text. Row titles read like an editor wrote them: "Because you watched **Charade**",
  "Starring **Cary Grant**", with the subject in Portico blue.
- **The real Portico brand.** The wordmark and Play-P symbol are Portico's official artwork
  (`assets/brand/portico`, copied from the brand set, used unmodified as white-on-dark), on the
  launch screen, sign-in, profile picker, rail and About; the app icons and tvOS top shelf are
  built from the same set.
- **Immersive title pages.** Movies, shows and episodes open on full-bleed artwork with the
  identity block anchored low; extras, cast, related titles and media details scroll up over it.
- **Focus you can see from the sofa.** Focused cards lift, get a white ring and throw a glow in
  the artwork's own colour; every button turns bright white with ink text when focused.
- **The Aisle (TV).** A slim icon rail that widens into a labelled menu over a dark gradient when
  focus enters it; focus always lands on the section you're in. Menu from content goes to the
  rail; Menu in the rail leaves the app. iPad gets the same rail as a tappable sidebar; iPhone
  uses five fixed tabs (Home, Library, Live, Saved, Search), a header with the server switcher
  and your profile, and a mini player docked above the tabs.
- **A player that feels premium.** Frames of the actual film crossfade under the chrome, the
  scrubber shows chapter frames while you scrub, intro and credits are tinted on the timeline,
  Skip intro and Up next appear on cue, and options (audio, subtitles with timing, quality,
  speed, chapters, queue, Play on) open in a single row of pills.

## What's in it

Everything in the web UI inventory is built: see [`FEATURES.md`](FEATURES.md), every row
ticked with where it lives. Highlights: sign-in (Portico Account, direct, recovery, two-step,
invitations), profiles with PIN, TV linking with QR, server claim and first-run setup, Home with
recommendation rows and Customize Home, libraries (movies, TV, anime, music, audiobooks,
photos, home videos) with Discover/Browse/Collections/Categories, filters, sort, saved views,
alphabet jump and multi-select bulk actions, movie/show/episode/person pages, the metadata editor
with fix match, music with lyrics and queue, audiobooks with chapters, speed and sleep timer,
photos with slideshow, search with TV keyboard, Saved, Live TV guide/channels/DVR with series
rules and conflicts, the live player with mini guide and captions, Watch Together, downloads,
account security/sessions/devices/invitations, settings, and all 23 server administration
sections.

## Architecture

```
src/
  App.tsx              providers, the native stack, chrome (rail / tabs / mini player), overlays
  design/              tokens, typography, Glow, Focusable, Button, Card, Row, Controls,
                       overlays (sheets, menus, confirm, prompt, toasts), states, icons
  data/
    types.ts           the domain model (items, people, live TV, downloads, together…)
    services.ts        the service interfaces every screen talks to
    admin.ts           the server-administration model and AdminService
    art.ts             resolves artwork keys ("poster/m-charade") to image sources
    hooks.ts           useServices, useQuery (cached, re-runs after mutations), useAction
    fixture/           the fixture backend: db.ts builds the catalogue, backend.ts implements
                       every service with simulated latency, adminData.ts the console state
    fixtures/          generated: content.json (catalogue) and art.generated.ts (image map)
  nav/                 routes, the Aisle rail, tab bar, Screen scaffold, Back/Menu handling,
                       deep links and the screen index
  state/               session (server, profile, role, connection) and the playback engine
  screens/             auth, home, library, title, media, browse, live, player, together,
                       downloads, settings, admin, shared
```

- **Navigation.** `@react-navigation/native-stack` (native screens, iOS edge-swipe back). Rail and
  tab destinations reset the stack to Home + section. `nav/back.ts` owns Back/Menu: overlays
  close first, then players, then the stack pops, then (TV, top level) focus moves to the rail;
  the tvOS Menu key is only captured while one of those applies, so Menu at the top exits.
- **Focus (TV).** `Focusable` wraps `Pressable` with focus state and a spring lift; placement
  styles go on the focusable view itself so tvOS always sees its real frame. Screens sit in
  `TVFocusGuideView autoFocus` so focus returns where you left it; the rail is its own guide
  whose destination is the current section, and the first card of every shelf points left at
  it. Overlays trap focus and give it back to their opener. Full-screen players only make their
  backdrop focusable while the controls are hidden, so focus never lands on something invisible.
- **Design system.** All values come from `design/tokens.ts` (Portico's colour tokens,
  `u()` scales phone sizes for TV). Platform differences go through `Platform.isTV` and layout
  breakpoints (`useLayout`: compact phone, tablet, TV) — one codebase, no separate apps.
- **Data layer.** Screens only call `useServices()` interfaces; nothing imports fixtures
  directly. Mutations bump a version so every `useQuery` re-reads.
- **Playback** is simulated by `state/player.ts` (one queue, a ticking position, sleep timer,
  shuffle/repeat, live offset); the video, now-playing and mini players all read the same store.

## Swapping in real services

1. Implement `Services` from `src/data/services.ts` (and `AdminService` from `src/data/admin.ts`)
   over Portico's HTTP APIs — `apps/portico-server/packages/client-core` already has most of
   the shapes. Keep the method names; screens need no changes.
2. Replace `resolveArt` in `src/data/art.ts` to return `{uri: serverUrl + artworkPath}`.
3. Swap `createFixtureServices()` in `App.tsx` for the real factory, and call the services'
   `subscribe` listeners from server events so lists refresh.
4. Replace the ticker in `state/player.ts` with a real player (react-native-video or the
   existing native player) feeding the same store fields.

## Fixture data and artwork

- **Catalogue** (`scripts/catalog/content/*.json`, packed by `scripts/catalog/pack.py`): 183
  films (23 real public-domain films and Blender open movies with accurate credits, plus 160
  invented), 24 TV series and 12 anime (270 episodes), 24 artists and 60 albums with lyrics,
  28 audiobooks (12 public-domain classics), 40 live channels with a rotating 3-day guide,
  30 home videos and 8 photo albums, four profiles (one kids, one with a PIN) and three servers
  (one unreachable). Invented prose was written by hand, not generated from templates;
  `scripts/catalog/check_prose.py` rejects templated text.
- **Artwork** (`scripts/art/`, ~80 MB bundled): real posters and film stills from Wikimedia
  Commons (public domain) and the Blender Foundation open movies (CC BY), Unsplash photographs
  via picsum.photos, anonymous public-domain portraits for invented people, and generated
  typographic posters, album covers, book covers and channel logos (`build_art.py`). App icons,
  the tvOS layered icon and the top shelf come from Portico's brand set via `icons.py` (only the
  rounded corner is removed, because the systems mask icons themselves).
- **Fonts** (`assets/fonts`, SIL Open Font License): Manrope and Inter Tight, registered in
  both Info.plists and bundled for Android.

## Build and run

```sh
npm install                      # .npmrc sets legacy-peer-deps for the tvOS fork
(cd ios && LANG=en_US.UTF-8 pod install)
(cd tvos && LANG=en_US.UTF-8 pod install)
npm start                        # Metro on port 8092 (Debug builds look there)
```

Debug: build the `PorticoCinema` scheme of `ios/PorticoCinema.xcworkspace` (iPhone/iPad) or
`tvos/PorticoCinema.xcworkspace` (Apple TV). Release builds embed the JS bundle and run without
Metro:

```sh
xcodebuild -workspace tvos/PorticoCinema.xcworkspace -scheme PorticoCinema -configuration Release \
  -sdk appletvsimulator -derivedDataPath ~/Library/Caches/portico-experiment-cinema-dd/tvos CODE_SIGNING_ALLOWED=NO build
```

Regenerating data: `python3 scripts/catalog/pack.py && python3 scripts/art/build_art.py`
(downloads are cached under `scripts/art/cache/`, which is not committed).

**Reviewing every screen:** Settings › About › Screen index, or
`xcrun simctl openurl <device> portico-cinema://screen/<id>` (ids are in `src/nav/screenIndex.ts`,
for example `movie`, `show`, `player`, `now-playing`, `live`, `dvr`, `admin/overview`).

The Xcode projects also contain two development-only UI-test targets (`RemoteDriverUITests`
for tvOS, `TouchDriverUITests` for iOS) that drive the simulators from a command file; they were
used to walk and screenshot every screen headlessly and are not part of the app.

## Decisions made along the way

- The brief allows a phone/tablet tab bar instead of a rail; iPhone uses tabs, iPad keeps the
  rail as a collapsible sidebar (it suits the wider screen), TV uses the Aisle.
- Card shapes follow the Portico rule: film, TV, anime and audiobooks (including episodes) are
  posters in rows and grids; landscape episode stills appear only inside show and episode
  pages; music is square with circular artist cut-outs; people are circles. Home videos and
  photo albums use landscape frames.
- Server administration is fully interactive on every form factor, not just read-only on TV.
- Downloads are hidden on Apple TV (as on the platform), and the TV sign-in leads with
  "sign in with your phone" (code + QR) over typing with the remote.
- Simulated states (reconnecting banner, channel problem, owner-stopped playback) can be
  triggered from the screen index and the live player so they can be reviewed.

## Known gaps and polish I'd still do

None against the checklist. Polish:
- D-pad feel on real hardware needs a hands-on pass. Focus was walked in the simulator with the
  remote driver on Home, the rail, libraries, title pages, search, the guide, the player and the
  admin console; long rows and the guide grid still deserve a sofa test.
- Entering the admin sidebar from content lands on the nearest item rather than the current
  section (the sidebar has no focus guide, because one fought the content for focus).
- Screens opened with the debug deep link start without focus on Apple TV until the first
  remote press; normal navigation always lands focus.
- Some invented titles reuse Unsplash photos for posters and stills; a larger curated photo
  pool would remove the occasional repeat.
- Transitions are the platform defaults plus crossfades; a shared-element poster-to-detail
  transition would add polish.
- The guide grid is a simple virtualized list; a two-axis virtualized grid would scale better
  to thousands of channels.
- The Android and Android TV builds are configured (package id, fonts, deep links, leanback)
  but were not built or run.
