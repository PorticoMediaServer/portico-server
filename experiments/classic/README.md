# Portico Classic

For the Ubuntu Android phone and TV comparison, see [Android review](../ANDROID-REVIEW.md).

Portico Classic is Portico built the familiar way, done carefully. It is a media app for iPhone,
iPad and Apple TV in one React Native codebase.

The design is calm and dense:

- On Apple TV there is a rail on the left. On iPad there is a sidebar, and on iPhone a header
  and a tab bar.
- Libraries are made of rows and poster grids.
- Detail pages put Play first.
- The Portico blue glow sits behind everything and takes on the tint of whatever title is
  featured.
- Real-looking artwork is used everywhere, and nothing shows an empty box.

Every feature of the web app has a place here, including all of server administration.
[FEATURES.md](FEATURES.md) maps each web feature to its screen in this app.

## Opening it

- **Apple TV:** Portico Classic is installed on the **Portico TV** simulator. Open it from the
  home screen. Its icon is the Portico wordmark on navy (the home-screen name is Portico Classic).
- **iPhone:** it is installed on the **Portico iPhone** simulator. Open it from the home screen.

Both are Release builds with the JavaScript bundled in, so they run without Metro. The app
starts signed in as Justin (owner) on the server Lighthouse. To see the sign-in screens, sign
out from the profile menu, or open **Settings › Screen index**. The screen index lists every
screen, dialog and state, and opens it directly.

**Please check the D-pad feel by hand.** Focus movement, the rail, the Menu button and modal
focus were exercised in the simulator with an injected game-controller remote. How it feels on a
real Siri Remote (speed, parallax, long presses) still needs a person.

## Android review on Ubuntu

Classic also has a private Android phone / Android TV review build. Its launcher name is
**Portico Classic**, application ID is `tv.getportico.experiment.classic`, and launch activity is
`com.porticoclassic.MainActivity`, so it can coexist with Cinema and the main Portico app.
It retains the fixture household and simulated playback; it is not a release candidate.

Restore the existing lockfile with `npm ci --legacy-peer-deps` (the React Native TV package alias
needs npm's legacy peer resolver), then build a standalone emulator APK:

```bash
cd android
JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64 \
ANDROID_HOME=/home/justin/Android/Sdk \
./gradlew assembleRelease --no-daemon --max-workers=1 \
  -PreactNativeArchitectures=x86_64 \
  '-Dorg.gradle.jvmargs=-Xmx1536m -XX:MaxMetaspaceSize=512m'
```

The development-signed APK is `android/app/build/outputs/apk/release/app-release.apk`.
It bundles JavaScript and artwork, so Metro is not needed. Local Metro remains on port 8091.
Review deep links are registered on Android, for example `portico-classic://screen/movie`;
the full list remains in **Settings › Screen index**. Android bundles the same four Manrope
weights and reuses the official Apple icon artwork for its launch icon and TV banner.
Android TV uses an app-scoped 1920-point canvas, matching the existing television design;
this leaves phone density and the emulator's system settings unchanged.

## Design

- **Brand** (`src/shell/Brand.tsx`, `assets/brand`): the real Portico Play-P symbol and wordmark,
  white on dark, from Portico's brand set. The paths are generated from the SVG masters into
  `src/shell/brandPaths.ts`. The TV rail shows the symbol and fades the letters in as it
  expands. The launch screens, splash, sign-in, About, app icons, tvOS layered icon and top
  shelf all use the same masters (`scripts/icons.py`). They are never recoloured or stretched.

- **Tokens** (`src/theme/tokens.ts`) hold Portico's palette:
  - ink `#071018`
  - ambient `#153749`
  - raised bright `#1d3848`
  - accent `#409fd4`, strong `#5cb3e2`, deep `#2f86bb`
  - status colours and profile colours

  Type is Manrope, with separate handheld and 10-foot scales.
- **Layout** (`src/theme/layout.ts`) works out the form factor (phone, tablet, TV) from
  `Platform.isTV` and the window width. It gives gutters, card sizes, rail widths and grid
  columns, so each screen is written once.
- **The glow** (`src/ui/Canvas.tsx`, `src/app/glow.tsx`) is an atmosphere layer that breathes
  slowly. It tints towards the colour of the focused or featured title.
- **Focus** (`src/ui/Focusable.tsx`): every control goes through this one wrapper. On TV it
  scales gently and draws a white focus fill or a ring with the blue glow. On touch it dims on
  press. It also handles the TV focus rules:
  - Preferred focus is a one-shot request, so a hero button doesn't pull focus back on every
    layout.
  - When a screen opens, focus goes to its preferred or top-left control, or to the selected
    tab.
  - The last control focused in the content is remembered. Pressing right on the rail returns
    there.
  - Inside modals, a focus gate gives the preferred control first claim. React Native's own
    preferred focus doesn't reach into modals.
- **TV Menu button:** it goes back through the stack, then to the rail, then out of the app.
  Inside sheets and players it closes them.

## Architecture

```
App.tsx                 providers, root stack (sign-in screens or the signed-in shell), deep links
src/theme               tokens and responsive metrics
src/ui                  design system: Text, Focusable, Button, Card, Shelf, Grid, Sheet, controls, icons, artwork
src/shell               Shell (rail / sidebar / phone chrome), profile and server switchers, banners
src/nav                 routes, navigation helpers, the screen index
src/screens/*           one folder per area (home, library, detail, music, photos, search, saved, live,
                        together, downloads, settings, account, admin, auth)
src/player              player state, video player, now playing, timeline, options, play on, mini guide
src/app                 session, preferences, toasts, glow, actions menu, selection, dialogs, editors
src/data                domain types, the service interface, and the fixture implementation
scripts/art             catalogue generator, artwork fetcher and renderer
scripts/tvremote        developer-only D-pad injector for the tvOS simulator
scripts/icons.py        app icons and tvOS brand assets
```

- **Navigation:**
  - A root native stack switches between the sign-in flow and the signed-in shell.
  - Inside the shell, one native content stack holds every destination.
  - `nav.ts` has `push`, `root` (for rail and tab destinations), `back` and `openEntry`.
  - Players are a modal above the shell. Sheets are modals too, with a focus trap on TV.
- **Data layer:**
  - Screens only talk to `PorticoClient` (`src/data/services.ts`). It has typed namespaces for
    session, libraries, home, catalogue, personal state, search, saved, live, Watch Together,
    downloads, account, notifications and preferences.
  - Changes are announced on topics. `useQuery` refetches when a topic it listens to changes;
    `useAction` wraps mutations.
  - Server administration is a separate `AdminApi` (`src/data/fixtures/admin.ts`). It returns
    each section as a page of typed blocks: stats, settings, lists, tables, charts, timelines,
    actions with forms and confirmations. `screens/admin/Blocks.tsx` renders those blocks the
    same way on every device.
- **Fixtures:**
  - `createFixtureClient()` simulates a household with latency and state:
    - three servers: Lighthouse, The Cabin, and Studio Annex, which is unreachable;
    - four profiles: Justin (owner), Maya (admin, PIN), Nora (PIN) and Theo (kids);
    - fixture PIN 1234.
  - The catalogue has:
    - about 360 movies, including 71 public-domain films and 10 Blender open movies;
    - 40 shows and 18 anime, with 1,339 episodes;
    - 50 artists, 137 albums and 1,445 tracks;
    - 30 audiobooks, 8 photo albums, 24 home videos;
    - 32 live channels with a consistent 72-hour guide.
  - Artwork is about 110 MB in `assets/art`:
    - posters, stills and portraits from Wikimedia Commons (public domain);
    - Blender Foundation posters (CC BY);
    - photographs from picsum.photos (Unsplash);
    - generated posters, covers, portraits and channel logos for invented titles.

## Swapping in real services

1. Implement `PorticoClient` from `src/data/services.ts` against the Portico server API. The
   fixture client in `src/data/fixtures/client.ts` shows the expected behaviour of each method:
   what it returns, what it rejects with, and which topic it emits after a change.
2. Implement `AdminApi`. It can map the server's admin endpoints onto the existing block types,
   or add new block types in `adminTypes.ts` and `Blocks.tsx`.
3. Replace the two `create…` calls in `src/data/DataProvider.tsx`. Nothing else imports
   fixtures, apart from artwork lookups in the player that turn into image URLs.
4. Artwork: fixtures resolve bundled `require()` images. Real services return URLs, which
   `Artwork` accepts as `{uri}` sources. Keep the placeholder behaviour for missing art.
5. Playback: the players drive a simulated clock (`PlayerProvider.tsx`). Put a video and audio
   engine behind the same `usePlayer()` actions (play, seek, set options, queue).

## Building

Requirements: Node (see `.xcode.env.local`), CocoaPods, Xcode with the tvOS and iOS simulators.

```
npm install
(cd ios && LANG=en_US.UTF-8 pod install)
(cd tvos && LANG=en_US.UTF-8 pod install)
npm start                       # Metro on port 8091
```

Debug build and run, for example on iOS:

```
cd ios
xcodebuild -workspace PorticoClassic.xcworkspace -scheme PorticoClassic -configuration Debug \
  -destination 'id=<simulator udid>' \
  -derivedDataPath ~/Library/Caches/portico-experiment-classic-dd/ios \
  CODE_SIGNING_ALLOWED=NO build
xcrun simctl install <udid> ~/Library/Caches/portico-experiment-classic-dd/ios/Build/Products/Debug-iphonesimulator/PorticoClassic.app
```

For tvOS, use `tvos/` and `Debug-appletvsimulator`. A Release configuration embeds the
JavaScript bundle and doesn't need Metro.

- Bundle ids: `tv.getportico.experiment.classic.ios` and `tv.getportico.experiment.classic.tv`.
- URL scheme: `portico-classic`.
- Artwork (only needed to change the catalogue):
  1. `python3 scripts/art/catalog.py`
  2. `python3 scripts/art/fetch.py`
  3. `python3 scripts/art/render.py`
  4. `python3 scripts/art/dedupe_backdrops.py` gives every title its own backdrop and lists the
     distinct episode stills (`src/data/fixtures/uniqueStills.json`).
  5. `python3 scripts/art/names.py`, then `python3 scripts/art/render.py albums`: distinct
     album titles and natural song titles. `render.py books` re-renders only book covers.
- Icons, launch screens and top shelf: `python3 scripts/icons.py`.

### Developer tools

- **Screen index:** Settings › Screen index, or `portico-classic://screen/<id>`. The ids are in
  `src/nav/screens.ts` and in the FEATURES.md table.
- **Command file:** development builds poll `http://localhost:8191/cmd.txt`. Writing `<id>|<anything>`
  there opens that screen on every running simulator, which avoids the URL confirmation prompt.
  Warnings are copied to the same server's request log (`src/dev/devlog.ts`).
- **TV remote injector:** `scripts/tvremote/gcremote.m` sends D-pad, select and menu presses to a
  tvOS simulator through a virtual game controller. It uses private SimulatorKit functions and is
  for development only. Build it with
  `clang -fobjc-arc -framework Foundation gcremote.m -o gcremote`, then run it as
  `gcremote <udid> right right select menu`.

## Decisions

- **Rail on Apple TV only.** iPad uses a collapsible sidebar, which suits the larger screen.
  iPhone uses a header (server switcher, downloads, notifications, profile), a bottom tab bar
  (Home, Libraries, Live TV, Saved, Search) and a mini player docked above the tabs.
- **Server administration is on every device,** for owners and administrators. It's a section
  list with a page beside it on TV and iPad, and pushed pages on iPhone.
- **Profiles, PINs and limits** can be reviewed on TV. For changing them the TV points to a phone
  or computer, as the web does.
- **Downloads** are offered on iPhone and iPad. On TV the Downloads page explains that.
- **Artwork shapes** follow Portico's rule: film, TV, anime and audiobooks are posters in every
  row and grid, episodes included (they show the show's poster), and Continue Watching too.
  Landscape stills appear only inside show and episode pages. Music is square, people and
  artists are round. Home videos and live channels stay landscape.
- **Recently added episodes** are grouped per show ("4 new episodes"), as a server groups them.
- **Home hero** rotates through featured titles. The glow follows the hero and focused cards.
- **Text fields on TV** show Portico's own field and open the system keyboard on select, instead
  of the default tvOS white pill.
- **US English** throughout.

## Status and next steps

A refinement pass (the goal: production-ready on every screen) is part way through. Work
paused at a clean checkpoint; everything builds and is committed.

### Done in the refinement pass

- **Brand:** the real Portico symbol and wordmark everywhere (rail, iPhone header, sign-in,
  splash, native launch screens, About), and new app icons, tvOS layered icon and top shelf
  from the brand masters.
- **Sign-in screens** (all twenty reviewed on iPhone, iPad and TV):
  - Sign in with Apple and Google buttons follow each provider's guidelines, with real logos.
  - Placeholder copy is gone ("(fixture)", "Fixture PIN", "Simulate approval"). TV activation
    and Quick Connect now approve by themselves after a short wait.
  - Dead controls now work: send the code again, sign in directly to another server, play a
    download while offline.
  - The invitation now comes from someone else. The claim request shows a real time, and its
    labels no longer wrap on TV.
  - Titles no longer repeat their own text, and server lines drop the repeated "Online".
  - TV skips downloads (offline and unreachable screens), spinners have room, and disabled
    buttons can't take focus.
- **Navigation and shell:**
  - iPhone header destinations (Settings, Account, Downloads, Notifications, Manage server,
    Watch Together) push onto the current tab, so the tab stays selected, and there's a Back
    button.
  - Page headers show Back automatically wherever there's somewhere to go back to.
  - The iPhone tab bar is opaque.
  - The TV rail has a single Live TV entry, soft edges where its list scrolls, and the real
    server and sign-in type under the profile.
  - The iPad sidebar respects the status bar, and iPad pages have more top room.
- **Browse:**
  - Customize Home moved to the end of Home.
  - Bigger hero title on iPad.
  - Collections are fanned poster stacks. Categories are colour tiles with a picture.
  - The A–Z bar fits the screen and sits inside the TV safe area. It only appears for long
    lists.
  - Photos and home videos no longer show film filters, and counts use the right noun.
  - Song lengths show as m:ss.
- **Sample data and artwork:**
  - 75 duplicate or blank backdrops were replaced, and no two titles share one now.
  - Episode stills never repeat within a show.
  - All 137 album titles are distinct, and song titles read naturally.
  - Audiobook covers keep titles legible.
  - Home videos are titled to match their pictures and none are dated in the future.
- **Code:** lint errors are cleared, including a hooks-order bug on the invitation screen.

### Still to do

1. Finish the screen-by-screen visual pass. Done so far: sign-in, Home, and every library
   (Movies, TV Shows, Anime, Music, Audiobooks, Photos, Home Videos). Not yet reviewed:
   - entity pages (genre, studio, collection) and Search;
   - all Titles screens (movie, show, episode, person, artist, album, audiobook, photo album,
     playlist, collection, saved view);
   - Dialogs (actions, metadata editor, feedback);
   - Playback (video, episode, live, channel problem, now playing, audiobook);
   - Live TV (guide, channels, recordings);
   - the You area (Saved, Together, Downloads, Settings and its sections, Account);
   - every Manage server section;
   - the States entries.
2. iPad in landscape, across all screens. The review tooling can now rotate the iPad (an
   XCUITest driver kept outside the repo).
3. TV focus returning to the control that opened a modal or page, after it closes.
4. Long-press: check that long-select opens the actions menu reliably on TV and that
   long-press gives the menu on touch. Consider Play/Pause as a second way in on TV.
5. Truncation of long names beside status chips (for example device names beside "Waiting for
   approval").
6. Motion review: page transitions, focus animation, hero changes, sheet presentation, and
   skeleton to content.
7. Consistent page headers. Saved, Search, Live TV, Downloads, Together and Manage server
   build their own title blocks; move them onto `PageHeader` for one rhythm.
8. Update FEATURES.md for the changes above (Live TV is one rail entry; recently added
   episodes are grouped).

### Known issues

- D-pad feel on a real Siri Remote is untested. The simulator was driven with a
  game-controller remote.
- Playback is simulated: artwork with a clock, no real video decoding.
- The top shelf is the static brand image, with no dynamic Continue Watching extension.
- A few picsum photographs still appear both as a backdrop and as an episode still, or in the
  Photos library. That's hard to notice, but they aren't all unique across the whole app.
