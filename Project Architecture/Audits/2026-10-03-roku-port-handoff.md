# Roku port — handoff (3 Oct 2026)

For the agent building the Portico Roku channel. Read this whole file, then `06 Clients.md`, the Apple
app's README (`apps/portico-react-native/apps/native/README.md`) and `Specs/Spec — Playback Protocol
v1.md` (§3, §4, §5, §6, §8). Two other agents work at the same time on Android/Fire OS (`apps/native`,
branch `android-port`) and Vega OS (`apps/vega`, branch `vega-port`). The previous lead (Fable) reviews
your work on return, next to the Apple TV app, which is the oracle.

## 0. What this is, and the one thing that matters

Roku has no React Native: a channel is BrightScript and SceneGraph XML. Everything else about Portico's
television client is settled — the rail, the rows, the cards, the title pages, the player chrome, the
settings — in the Apple TV app (`apps/native/src`, shared with iPhone), and Justin's instruction is:

> the goal is for the Roku client to be identical visually to the React Native clients, or as close as
> humanly possible.

So this is a **translation**, not a design job. Every screen, every spacing, every colour, every type
size and every focus behaviour comes from the Apple TV app; you reproduce it in SceneGraph. There is no
Roku device or test environment yet (one is coming), so the work is judged by (a) a channel that packages
and compiles clean, (b) a faithful translation of the design system and screens that the lead can check
line by line against the Apple source, and (c) unit-tested BrightScript for everything that is logic.

## 1. Justin's rules (non-negotiable)

1. **No subagents or helper lanes.** Do it in the lead session.
2. **Tests: logic only.** No screen tests (there is nothing to run them on). Unit tests (Rooibos or a
   plain `brs`-runner harness, whichever compiles with the toolchain you install) for the BrightScript
   that parses, decides or formats; none for SceneGraph layout. Run them once per phase.
3. **Never idle.** 4. **No AI attribution or model names** in code, files, branches or commits.
5. **Pre-release: no sacred cows**, but this lane changes nothing outside `apps/roku` (see §3).
6. **The server decides.** The channel never ranks, groups, hides or re-pages what the server
   published. Unknown row ids fall back to the server's title text. O(visible): windowed rows and
   grids, bounded pages; never a whole library in memory.
7. **Correctness over limits:** never truncate sort/filter/search/credits; a capped view has "See all".
8. **Artwork shapes** as the Apple TV app draws them (posters for film/TV/anime including episodes,
   square for music and audiobooks, landscape stills only inside a show page and Search's Episodes
   section; a row is one shape, never mixed).
9. **US English source text**, read from the same catalogue (`packages/i18n`) by a generator (§2), so
   the words are literally the same.
10. **Native text input only:** Roku's keyboard dialog, never a custom one.
11. **Sign-in on a television is by code** (the TV shows a code; a phone or computer approves it) or
    by server address with username and password. No providers.
12. **Decide and log.** Where this file and the Apple source are silent, decide the way the Apple TV app
    would, write it in the log, keep going. Do not stop to ask.

## 2. Decisions already made (apply exactly)

- **Where it lives:** `apps/portico-react-native/apps/roku` — `manifest`, `source/`, `components/`,
  `images/`, `locale/`, `scripts/`, `test/`. Channel id/name "Portico"; `ui_resolutions=fhd` so the
  channel lays out at **1920×1080**, the same points the Apple TV app uses (so every number in the Apple
  source maps one to one).
- **Toolchain:** BrighterScript (`brighterscript`, `bsc`) for compile checks and type safety, with
  `.bs` sources that compile to `.brs`; `roku-deploy` to package a sideloadable zip; the VS Code
  BrightScript extension's language server is optional. `ropm` for `rooibos` if you use it. Pin versions
  in `apps/roku/package.json`. `npm run check` must compile the channel clean; `npm run package` must
  produce the zip. There is no device: never claim something was seen on a Roku.
- **Generated, never hand-typed:** a Node script `apps/roku/scripts/generate.mjs` writes
  `source/Theme.bs` from `packages/design/src/index.ts` (every colour as `0xRRGGBBAA`, the 4-point
  spacing scale, `typeScale.tenFoot`, radii × `tvRadiusMultiplier`, motion, layers, the television
  metrics and spacing from `apps/native/src/ui/theme.ts` — copy the television values into the generator
  as constants with a comment naming their source line) and `locale/en_US/translations.ts`-equivalent
  (`locale/en_US/translations.xml`) from `packages/i18n/src/catalog/en-US.ts` for every message id the
  channel uses. The generator runs as `npm run generate`; its outputs are committed. Nothing in
  `components/` may contain a colour, font size or spacing literal.
- **Fonts:** Manrope 400/500/600/700 from `apps/native/assets/`, copied by the generator into
  `fonts/`, registered as SceneGraph fonts at the tenFoot sizes.
- **Icons:** `packages/design/src/icons.ts` are SVG paths. Roku draws PNGs: the generator rasterises
  each icon at the sizes the Apple metrics use (icon 28/36/44 at TV scale; check `televisionMetrics`)
  with `resvg`/`sharp` into `images/icons/`, in the text colours. Same icon ids, same drawings.
- **The design system first** (`components/ui/`): one SceneGraph component per Apple primitive in
  `apps/native/src/ui` — `Focusable` (focus ring colour, width and radius from `componentSpec.focus`,
  scale 1.08 by animation in `motion.standard` ms), `Button` (every variant and size from
  `componentSpec.button`), `Text` (type roles), `Card` (poster/square/landscape, the watched check, the
  progress bar, the caption rule), `Shelf` (horizontal row with the Apple gutter and page insets),
  `Grid` (windowed; see §4), `Rail` (112 collapsed / 340 expanded, the same items, the same expansion
  rule), `Tabs`, `ListRow`, `Sheet`/`Menu`/dialogs (opaque `overlaySurface`), `Chip`, `Switch`,
  `Field` (opening Roku's keyboard dialog), `Toast`, `Page`, `Artwork` (with the placeholder initials),
  `Avatar`. Each component's header comment names the Apple file it translates.
- **The model in BrightScript:** the server composes Home rows, library tabs, title pages, settings
  structure and captions; `packages/client-core` holds the parsing and a few presentation rules the
  clients share. Port only what the channel needs, file by file, keeping the names (`source/core/
  Home.bs` ↔ `client-core/src/home.ts`, `CardCaption.bs` ↔ `presentation/card-caption.ts`,
  `LibraryTabs.bs` ↔ `presentation/library-tabs.ts`, `SettingsStructure.bs` ↔
  `presentation/settings-structure.ts`, `DetailActions.bs`, `EntryActions.bs`, `Playback.bs` for v1
  sessions and queues, `ClientProfile.bs` for the capability profile, `Credentials.bs`,
  `DeviceAuthorization.bs` for the code flow). Each ported rule gets a unit test whose cases are taken
  from the TypeScript test of the same name in `packages/client-core/test` (same inputs, same expected
  strings), so the two stay provably identical.
- **Playback:** SceneGraph `Video` node, HLS (fMP4 and TS) and direct play over HTTP(S); the capability
  profile declared from `roDeviceInfo` (codecs, HDR, audio channels, model, OS version):
  `rokuDeclaredProfile` in `packages/client-core/src/client-profile.ts` (added 3 Oct; family `roku`,
  engine `roku-video`) is the table to reproduce in `ClientProfile.bs`, widened by what `roDeviceInfo`
  answers, and published the way `ClientProfilePublisher` in that file does (read it for the route and
  the fence). Subtitles
  through the Video node's tracks for embedded text; sidecar WebVTT by URL. Timeline reports per §6.
  Trickplay thumbnails per §4.4 if the Video node accepts BIF/our format; otherwise log it.
- **Listening:** music and audiobooks through the same Video node in audio mode (Roku has no gapless
  guarantee; log the behaviour), with the queue and the §8 transitions in BrightScript; the now-playing
  pill as on Apple TV.
- **Sign-in:** the Activation screen (code + QR, polling `device-authorization` as `client-core/src/
  device-authorization.ts` does) and direct sign-in by address; credentials in the registry
  (`roRegistrySection`), scoped per server as the Apple keychain records are.
- **Discovery:** Roku has no mDNS API for channels; sign-in is by address and code. Log it as the
  known difference.
- **Server management:** Dashboard only on television (as Apple TV): the Settings screen shows the
  Account pages and the Dashboard; the other Server pages are web/phone (the structure in
  `settings-structure.ts` already says which pages a TV shows).
- **Not in scope:** Roku store listing, Roku Pay, a custom keyboard, Roku's own home-screen rows,
  a device test environment (coming later), any change outside `apps/roku`.

## 3. Where things are, and staying out of the other lanes' way

- Root `/Users/justin/Documents/Codex Projects/Portico/` (Mac) or Justin's Linux clone.
  `apps/portico-react-native/apps/native/src` is the reference you translate from (read-only for you);
  `apps/portico-react-native/packages` holds `design`, `i18n`, `client-core` (the originals are in
  `apps/portico-server/packages`; identical).
- **Your worktree and branch:**
  `cd apps/portico-react-native && git worktree add "../../.worktrees/portico-react-native-roku" -b roku-port frontend-rework`.
  Work only under `apps/roku` there. Never touch `apps/native` or `apps/vega`. Commit in the worktree
  with `git -c user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…"`, at least once
  per phase. Write your log in portico-internal (`Audits/2026-10-03-roku-port-log.md`) and leave it
  uncommitted; the lead commits it.
- **A reference channel may exist** from the retired project ("Portico (Original Retired Project - GPT
  5.5 and 5.6)" beside `Portico/` in `Codex Projects`, and the pre-split bundle in `___BACKUP COPIES`).
  If you find a Roku channel there, read it for Roku mechanics (Video node setup, registry, the
  keyboard dialog, focus handling in SceneGraph) and nothing else: its UI predates the current design
  and its protocol predates Playback v1. If you don't find it in ten minutes, start fresh.
- **The local server** (for reading real documents while building parsers): the Android handoff §3
  describes it; `curl` with the owner test account's bearer is enough to fetch `GET /v1/home`,
  `/v1/libraries/{id}/…`, a title page and a playback plan to keep in `apps/roku/test/fixtures/`
  (strip nothing but tokens). Never print the test account.
- **The reference screenshots:** the Apple TV app in the tvOS simulator ("Portico TV"), one per screen in
  §5, kept in `portico-internal/Project Architecture/Audits/tv-reference-shots/` (shared with the Vega
  lane; if present, reuse). If you cannot take them (no Mac, no simulator), write that in the log and
  translate from the source: every number is in `apps/native/src/ui/theme.ts`, `packages/design`, and
  the screens' layout code.

## 4. Translation rules (how to read the Apple source)

- **Layout:** the Apple TV app renders at 1920×1080 points; Roku FHD is 1920×1080 pixels. Use the
  numbers as they are. Spacing names → values (television): xxs 4, xs 8, sm 12, md 20, lg 28, xl 40,
  xxl 56, xxxl 80, page 80, gutter 24. Metrics: rail 112/340, poster width 214 (height 1.5×), square
  250, landscape width per `landscapeWidth` (16:9), focus scale 1.08. Radii ×1.75. Type roles from
  `typeScale.tenFoot` (size, weight, line height, letter spacing).
- **Focus:** Roku's focus model is per-node; reproduce Apple's: a focused card scales 1.08 and shows the
  ring; rows remember their focused index; the rail takes focus on Back at a section root, Back on the
  rail exits (`apps/native/src/shell/Shell.tsx` describes the gate); a screen that finishes loading
  never steals focus from the rail (`theme.ts` says so near "railRoaming").
- **Windowing:** rows are `RowList`/`MarkupGrid` with the server's pages (`nextCursor`) loaded as the
  focus nears the end, never the whole row; grids page the same way (`WindowedGrid` in
  `apps/native/src/ui/Grid.tsx` is the behaviour to match: placeholders with stable keys, pages
  requested by the visible range).
- **Captions and marks:** `CardCaption.bs` from `card-caption.ts`; a watched check, a progress bar, no
  unwatched mark.
- **Words:** every string through the generated catalogue by message id; the same ids the Apple screens
  pass to `t()`.
- **Player chrome:** `apps/native/src/player/VideoPlayer.tsx`, `Timeline.tsx`, `PlayerOptions.tsx`:
  Pause/Play is the only filled control; option boxes Audio · Subtitles · Quality · Speed · Sleep; chrome
  hides after a few seconds; left/right seeks on the timeline.
- **Settings:** `settings-structure.ts` lists the pages and rows a television shows; `SettingsPage.tsx`
  and `ServerPage.tsx` draw them from authored rows (`ServerForms.tsx`): translate the row kinds, not
  each page by hand.

## 5. The screens, in order (each a phase checkpoint)

1. Activation (code + QR) and direct sign-in; profile choice (`DirectProfiles`).
2. Shell: rail (collapsed/expanded), section roots, Back policy.
3. Home: hero (Continue Watching's first item), rows, cards, captions, "See all" → `HomeRow`.
4. Library: tabs (Discover first), Browse grid with sort and filters (`BrowseFilters`), Recently aired.
5. Title pages: movie (`Detail` + `title/*`), show (`Show`, episodes, seasons), person, entity
   (album/book/artist), with the action row (Play · My List · Favorite · Watched · ⋯) and pick-before-play.
6. Search (Roku keyboard dialog), Saved (My List, Favorites, Playlists, Collections, History).
7. Player (video), then listening (music/audiobook) with the pill.
8. Settings: Account pages and the Dashboard.
9. Channels/Live TV (guide grid) if the server has sources; otherwise hidden.

For each: a SceneGraph screen under `components/screens/`, the BrightScript it needs under
`source/core/`, the fixtures it parses under `test/fixtures/`, and a log entry with the Apple file it
was translated from and any difference.

## 6. If nothing can be run

There is no Roku. That is expected. The compile check, the package step and the unit tests are the
only executable proof; the rest is the faithfulness of the translation. Write the log so the lead can
check it: for each screen, the Apple file, the Roku component, and the three or four numbers (widths,
gaps, sizes) the lead can compare in a minute.

## 7. The log (required)

`portico-internal/Project Architecture/Audits/2026-10-03-roku-port-log.md`, uncommitted. First: the
toolchain versions and whether any reference channel was found and what was taken from it. Then per
phase: **Built** (Apple file → Roku component), **Decisions where this handoff was silent**,
**Differences from the Apple TV app** (each with the reason), **Not done, and why**, **Checks** (compile,
package, tests), **Commits**. Questions for Justin numbered at the end; do not wait for answers.

---

## Prompt to paste

```
You are continuing work on Portico, a self-hosted media server with a Go backend, a web app, and one
React Native app (react-native-tvos) for iPhone, iPad and Apple TV. Your task is to build the Portico
Roku channel (BrightScript + SceneGraph, compiled with BrighterScript) as a faithful translation of the
Apple TV app: the goal, in Justin's words, is for the Roku client to be identical visually to the React
Native clients, or as close as humanly possible. There is no Roku device or test environment yet; the
work is judged by a clean compile and package, a design system and screens generated and translated
from the shared design tokens, catalogue and Apple source, and unit-tested BrightScript logic whose
test cases mirror client-core's.

Read, in this order, and follow exactly:
1. portico-internal/Project Architecture/Audits/2026-10-03-roku-port-handoff.md.
2. apps/portico-react-native/apps/native/README.md, then apps/native/src/ui/theme.ts,
   packages/design/src/index.ts and components.ts (the numbers and colours).
3. portico-internal/Project Architecture/06 Clients.md and Specs/Spec — Playback Protocol v1.md
   (§3, §4, §5, §6, §8).

Rules you must not break: no subagents or helper lanes; unit tests for logic only, once per phase; never
idle; no AI attribution or model names anywhere; nothing hand-typed that the generator can produce
(colours, spacing, type, strings); no colour, size or spacing literal in a component; the server
decides and rows are windowed; US English source text through the shared catalogue; decide and log
where the handoff is silent rather than asking.

Work in your own worktree and branch: cd apps/portico-react-native && git worktree add
"../../.worktrees/portico-react-native-roku" -b roku-port frontend-rework. Work only under apps/roku
there; never touch apps/native or apps/vega (other agents). Commit per phase with: git -c
user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…". Write the log at
portico-internal/Project Architecture/Audits/2026-10-03-roku-port-log.md but do not commit in
portico-internal. Start with the toolchain, the generator and the design system, then the screens in
the handoff's §5 order.
```
