# Roku port — log (3 Oct 2026)

Handoff: `2026-10-03-roku-port-handoff.md`. Worktree `.worktrees/portico-react-native-roku`, branch
`roku-port` (from `frontend-rework` at `a49bb83`). Everything is under `apps/roku`. This file is
uncommitted, as the handoff asks.

**Nothing here was seen on a Roku.** There is no device. The proof that can be run is `npm run check`
(compile), `npm run package` (the zip) and `npm test` (logic, in the off-device `brs` interpreter).
Everything else is a translation to be read against the Apple source; each phase lists the Apple file,
the Roku file and the numbers to compare.

## Toolchain

| Tool | Version | Use |
|---|---|---|
| Node | 22.22.2 | Imports the shared TypeScript directly (type stripping), so the generator reads `packages/design` and `packages/i18n` without a build step |
| `brighterscript` (`bsc`) | 0.73.5 | Compiles `.bs` to `.brs`, validates every scope (`npm run check`) |
| `roku-deploy` | 3.18.4 | Zips `build/staging` into `build/portico-roku.zip` (`npm run package`) |
| `@rokucommunity/brs` | 0.47.6 | Off-device BrightScript interpreter that runs the unit tests (`npm test`) |
| `@resvg/resvg-js` | 2.6.2 | Rasterises icons, shapes, masks, scrims and brand art in the generator |

`npm install` reports 9 advisories (2 moderate, 7 high), all in `roku-deploy`'s and `brs`'s own
dependency trees (build-time tools; nothing from `node_modules` ships in the channel). Not changed.

Rooibos was not used: it runs on a device. The harness is `test/Harness.bs` plus a runner that
`scripts/test.mjs` writes at build time (one call per `sub Test_…()`), run by `brs`.

Run on the Mac. These are Node scripts of a second or two each; nothing was built on the runners.

**Reference channel:** none found. Looked in `Codex Projects/` beside `Portico/` (no "Portico (Original
Retired Project…)" folder exists there), in `__RETIRED/` and in `___Portico Backup Documentation/` for
any `*roku*` path, `manifest`, `.brs` or `.bs` file. Started fresh; nothing was taken from anywhere.

**Reference screenshots:** `Audits/tv-reference-shots/` does not exist, and none were taken (the lane
runs unattended overnight and the simulator was not driven). The translation is from the source.

## Phase 0 — toolchain, generator, design system

### Built

`scripts/generate.mjs` (`npm run generate`), outputs committed:

| Output | Source |
|---|---|
| `source/Theme.bs` | `packages/design/src/index.ts` (colours as `0xRRGGBBAA`, the 4-point scale, `typeScale.tenFoot`, radii × `tvRadiusMultiplier`, motion, layers, targets), `components.ts` collapsed to its `tv` column, `apps/native/src/ui/theme.ts` (`televisionSpace`, `televisionMetrics`, the palette aliases) |
| `source/ThemeTables.bs` | The same values as lookup tables, for names that arrive at run time (a button variant's fill token) |
| `locale/{en_US,en_CA,en_GB,en_AU}/translations.xml` | `packages/i18n/src/catalog` — every message id the channel's sources name |
| `fonts/manrope-{400,500,600,700}.ttf` | `apps/native/assets` |
| `images/icons/<id>-<size>.png` | `packages/design/src/icons.ts`, all 119 ids, drawn as `ui/Icon.tsx` draws them |
| `images/shapes/*.9.png`, `arc-*.png` | 9-patch rounded fills and rings (SceneGraph has no corner radius or border), the focus glow, the spinner arc |
| `images/masks/*.png` | Rounded masks for artwork at the card sizes |
| `images/gradients/*.png` | `componentSpec.hero.scrim` stops, the hero fallback, `ui/Canvas.tsx` Atmosphere |
| `images/brand/*`, `images/channel/*`, `manifest` | `assets/brandSymbol.ts`, `assets/wordmark.png`, `brand/appicon-1024.png`, the canvas colour |

Design system (`components/ui/`, component names prefixed `Ui`):

| Apple (`apps/native/src/ui`) | Roku (`apps/roku/components/ui`) | Compare |
|---|---|---|
| `Text.tsx` | `Text.xml/.bs` (`UiText`) | roles from `typeScale.tenFoot`: body 24/33, heading 32/40, title 44/52, display 62/68 |
| `Focusable.tsx` | `Focusable` (`UiFocusable`) | ring 4 wide, 6 outside the control, radius + 6, colour `focus`; scale 1.08; 120 ms cubic out |
| `Button.tsx` | `Button` (`UiButton`) | heights 64/64/72, padding 20/28/40, radius 14, gap 12, icons 26/30/32; transport 72 and 88; focus scale 1.05 |
| `Feedback.tsx` Ring | `Spinner` (`UiSpinner`) | 26/44/64, stroke 3, one turn per 1000 ms |
| `Artwork.tsx` | `Artwork` (`UiArtwork`) | radius 14; placeholder `raisedHigh` at 0.75; title in subheading; fade 200 ms; progress line 4 |
| `Card.tsx`, `FocusHalo.tsx` | `Card` (`UiCard`) | poster 214 × 321, square 250, landscape 356 × 200; caption 12 below, min 60; watched badge 30 with an 18 check; person 150 |
| `Shelf.tsx` | `Shelf` (`UiShelf`) | gutter 24, page inset 80, header gap 20, bottom margin 40 |
| `Grid.tsx` (WindowedGrid) | `Grid` (`UiGrid`) | 7 poster columns of 214, 6 square columns of 254, row gap 28 |
| `ListRow.tsx` (ListRow, ToggleRow, RowGroup) | `ListRow`, `RowGroup` | min heights 64/80/96, padding 20 × 12, leading box 32, icon 30, gap 20; group radius 18 |
| `Switch.tsx` | `Switch` | track 56 × 32, thumb 24, inset 4 |
| `Chip.tsx` | `Chip` | height 64, padding 20, pill |
| `Tabs.tsx` (Tabs, Segmented) | `Tabs` | tab padding 20 × 20, underline 4, focus scale 1.04; segmented 64 with radius 14/10 |
| `Field.tsx` | `Field` | height 64, radius 14, border 2 |
| `Sheet.tsx` (Sheet, Menu) | `Sheet`, `Menu` | width 760 (side 640), radius 24, inset 28, scrim 0.8, 180 ms in / 120 ms out from 0.98 |
| `Toast.tsx` | `Toast` | top 60, right 80, radius 20, padding 28 × 20, 4000 ms |
| `Page.tsx` (Page, PageHeader) | `Page`, `PageHeader` | top padding 32, bottom 80; header 28 above, 20 below |
| `Avatar.tsx` | `Avatar` | circle, initial in caption-strong |
| `Feedback.tsx` (Notice, StateView, Badge, ProgressBar) | `Notice`, `StateView`, `Badge`, `ProgressBar` | notice padding 20, radius 18, icon 28; state circle 88 with a 38 glyph |
| `Canvas.tsx` (Atmosphere, Backdrop) | `MainScene` atmosphere, `Backdrop` | scrims from `hero.scrim`; crossfade 320 ms |

Logic (`source/`), each with tests:

| File | From |
|---|---|
| `core/Messages.bs` | `packages/i18n/src/messageformat.ts` (ICU subset) |
| `core/Artwork.bs` | `client-core/src/artwork/buckets.ts`, `ui/ArtworkPipeline.ts` `sizedUri` |
| `ui/Style.bs` | `ui/theme.ts` (`alpha`, `scaled`), `ui/Icon.tsx` |
| `ui/MenuModel.bs` | `ui/Sheet.tsx` `orderMenuItems` |
| `ui/Focus.bs`, `ui/Scroll.bs` | New: what tvOS does for the Apple app (see decisions) |

### Decisions where the handoff was silent

1. **Component names carry a `Ui` prefix.** `Button`, `Label`, `Poster`, `Video`, `Group`, `Dialog`,
   `RowList` and others are built-in SceneGraph nodes, so a component cannot be named `Button`. Files
   keep the Apple names (`components/ui/Button.xml` ↔ `ui/Button.tsx`); the component is `UiButton`.
2. **The theme is BrighterScript `const`s in namespaces**, so `Theme.colors.ink` is inlined at compile
   time (no run-time cost) and a misspelt token fails the compile, as it fails `tsc` in the Apple app.
3. **The television literals of `theme.ts` are evaluated from the file, not copied.** The handoff says
   to copy them into the generator with a comment naming the source line. The generator instead cuts
   `televisionSpace`, `televisionMetrics` and the palette aliases out of `theme.ts` and evaluates them
   against the design values, so they cannot drift; the generated header names the lines.
4. **Numbers the Apple components type themselves** (not tokens: the watched badge's 30, the halo's
   shadow radius 28, the rail's 44-point head) are in `scripts/apple-literals.mjs`, each with its file
   and the exact source text. `generate` fails when that text is no longer in the file. They reach the
   channel as `Theme.apple.<name>`.
5. **`scripts/lint-literals.mjs` enforces the no-literal rule** in `components/` (part of
   `npm run check`): no hex colour anywhere, no number in component BrightScript except 0, 1, 2, 0.5,
   100 and 1000 (indices, halves, percent and millisecond conversion), and no layout attribute with a
   number in component XML. Component XML therefore declares structure and interface only; every node
   is styled from `Theme` in BrightScript.
6. **Icons are white masters tinted at run time** (`Poster.blendColor`), at the nine television sizes
   (18–48). The handoff says "in the text colours"; tinting gives every token colour (ink on a primary
   button, danger on a destructive row) from one drawing per size.
7. **Focus by geometry** (`source/ui/Focus.bs`). SceneGraph has no focus engine; tvOS has one and the
   Apple app relies on it. The channel has a small one: a control with a `focusable` field takes part,
   a direction key goes to the nearest control that way (one in line beats one that is not), and a
   `focusGroup` remembers its last control (`FocusGroup` in `ui/Focusable.tsx`). `UiPage` and `UiSheet`
   use it, so screens are laid out and need no key wiring of their own.
8. **Cards are list items, not Focusables.** A `RowList` (Shelf) or `MarkupGrid` (Grid) holds focus and
   reports it to the card as `focusPercent`; the card scales and shows its halo by it.
9. **Focus eases over `motion.quick` (120 ms).** The handoff says `motion.standard`; `ui/Focusable.tsx`
   uses `motion.quick` with cubic ease-out, and the source is the oracle.
10. **Golden fixtures** (`scripts/golden.mjs`): beyond the cases copied from `client-core/test`, the
    generator runs the TypeScript itself over inputs and the BrightScript tests replay input and
    output. For messages that is every catalogue message that takes arguments (1,564 cases).
11. **The message cache is case-sensitive.** BrightScript associative arrays are not, by default, and
    two catalogue messages differ only in case ("About {size}" and "about {size}"). The golden sweep
    found it.
12. **Roku locales:** `en_US`, `en_CA`, `en_GB`, and `en_AU` reading en-GB (`packages/i18n`
    `englishFor`: every English region except the United States and Canada spells as Britain does).
13. **"See all" is reached with Up from its row.** On Apple TV the focus engine finds it by geometry
    from the card beneath it. A Roku row keeps the focused card at the left (as Apple TV does), so the
    whole row counts as one control for the purpose and Up always reaches that row's "See all" first.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| No letter spacing (`tracking`) | A SceneGraph `Label` has none. Sizes, weights and line heights are applied |
| Line height is the font's own height plus `lineSpacing` | `Label` has no line-height field. `lineSpacing` is generated per role from Manrope's `hhea` metrics so one line is `lineHeight` tall; for the display and title roles it is negative, which a device must confirm is honoured |
| Rounded artwork uses `MaskGroup` | SceneGraph cannot clip to a radius. Masks are generated for the card sizes; a size with no mask draws square corners |
| Rounded fills, borders and rings are 9-patch images | No corner radius or border on `Rectangle` |
| The focus glow is a pre-blurred 9-patch | No shadows. Colour, opacity (0.7), radius (28) and offset (10) from `ui/FocusHalo.tsx` |
| A card's focus animation is timed by the list | `RowList`/`MarkupGrid` drive `focusPercent`; the scale (1.08), ring and glow are the same |
| Text fields open Roku's `StandardKeyboardDialog` | Rule 10. The dialog is tinted with the palette |
| Up from a row reaches "See all" before the row above | Decision 13 |

### Not done, and why

- Rail: built with the shell in phase 2 (it needs the shell's state).
- `Surface`, `Divider`, `PinEntry`, `NavBar`: `NavBar` is handheld only; the others are made when a
  screen needs them.

### Checks

- `npm run check`: clean (literal lint, then `bsc`, 0 errors).
- `npm run package`: `build/portico-roku.zip`, 1,727 KB, 1,526 files.
- `npm test`: `TESTS PASSED: 1601 checks in 20 tests`.

### Commits

- `ca99ca7` Roku: toolchain, generator and the design system

## Phase 1 — Activation, direct sign-in, profiles

### Built

Screens (`components/screens/auth/`):

| Apple (`apps/native/src/screens/auth`) | Roku | Compare |
|---|---|---|
| `AuthFrame.tsx` | `AuthFrame` | column 720 (1240 wide), top 60 + 40, bottom 56, gap 28; wordmark 44 (64 centred); title in display |
| `Activation.tsx` | `Activation` + `CodePanel` | QR 260 on a white plate (padding 20, radius 12); gap 56; code 84/96 with 10 between characters; row min height 300 |
| `QuickConnect.tsx` | `QuickConnect` + `CodePanel` | QR 240; same row; "Use a password instead" primary, secondary after a failure |
| `DirectSignIn.tsx` (with `DirectStepScreen`) | `DirectSignIn` | fields 64 high, 28 apart; primary `lg` (72) full width; steps address → code or password; challenge, new password, device approval |
| `DirectProfiles.tsx`, `ProfileAvatar.tsx` | `DirectProfiles` | face 180, tile 220, gap 40, border 3, focus scale 1.08, lock 14 |
| `PorticoLanding.tsx` (in `shell/ShellFrame.tsx`) | `PorticoLanding` | centred column max 900 beside the rail; offline rows at 0.55 |
| `SavedServerUnavailable.tsx` | `SavedServerUnavailable` | retries at 2, 4, 8, 15, 30, 60 s; ring 28 |
| `AuthFlow.tsx`, `sign-in-steps.ts` | `AuthFlow` | welcome → direct (address → code / password) → landing; Back one step at a time |
| `App.tsx` (`Root`, `Connecting`) | `MainScene` | wordmark 56, ring 44, gap 28 |

Logic (`source/core/`), each with tests:

| File | From | Tests |
|---|---|---|
| `RouteIdentity.bs` | `client-core/src/route-identity.ts`, `portico-servers.ts` (`requestPorticoChallenge`), `app/direct-connect.ts` (`serverAddress`) | golden: the TypeScript's verdict on 32 addresses, 23 hosts, 6 pins, 14 identity proofs, 15 signed-route documents, 7 challenges |
| `Ed25519.bs`, `Ed25519Constants.bs` (generated) | New (see decisions) | signatures made by Node's Ed25519 from the RFC 8032 keys, valid and tampered |
| `Bytes.bs`, `Crypto.bs`, `Time.bs`, `Validate.bs` | `routeBase64`/`routeBytes`, `currentUTCTimestamp`, `utcOrderKey`, the parsers' helpers | golden timestamps against `Date.parse` |
| `LocalSession.bs` | `client-core/src/local-session.ts` | golden: 30 raw sessions through `parseLocalSession` |
| `DirectSignIn.bs` | `client-core/src/profile-management.ts` | golden: 15 profile lists, 21 sign-in answers |
| `SetupCode.bs` | `client-core/src/setup-code.ts`, `device-authorization.ts`, `app/approval-link.ts` | validation, poll decisions, backoff |
| `Session.bs` | `app/session.tsx` with `server-connections.ts`, `direct-connect.ts` | flows against a table-driven transport |
| `Hosted.bs` | `device-authorization.ts`, `credentials.ts`, `index.ts` (`HttpHostedApi`, presence), `server-connections.ts` (`connectHostedServer`), `portico-servers.ts` (`porticoSignIn`), `app/portico-landing.ts` | flows; presence order and labels golden |
| `Errors.bs` | `client-core/src/presentation/errors.ts`, `app/errors.ts` (`signInMessage`), `app/portico-failure.ts` | golden: 329 `presentError` cases |
| `Format.bs` | `packages/i18n/src/index.ts` (`createI18n`), `client-core/src/password-strength.ts` | golden: Intl output for en-US, en-CA, en-GB |
| `Qr.bs` | New (the Apple app uses `react-native-qrcode-svg`) | golden: module grids from the `qrcode` package it is built on |
| `Api.bs`, `Http.bs`, `Store.bs` | `index.ts` (`HttpLocalApi.request`, the error envelope), `installation.ts` | through the flow tests |

### Decisions where the handoff was silent

14. **Server identity is verified on Roku as on Apple TV.** A server proves an address is its own by
    signing a fresh nonce with its Ed25519 key (`/v1/networking/identity-proof`); the account service
    signs the routes it publishes; a server signs the Portico Account challenge. Roku has no Ed25519.
    Rather than skip the checks, `source/core/Ed25519.bs` verifies signatures in BrightScript (the
    TweetNaCl algorithm, public domain; verification only, the channel holds no private key). The curve
    constants are computed in the generator, not typed. SHA-256 and SHA-512 are the device's
    (`roEVPDigest`).
15. **The plain activation-code flow.** The Apple TV app uses `protocol: tv-x25519-v1` (the grant comes
    back sealed to a key the TV holds). The account service also accepts the plain flow
    `device-authorization.ts` describes (`POST /v1/device-authorizations`, then
    `/v1/device-authorizations/token`), for any platform label, and the handoff names that file. The
    channel uses it with `platform: "roku"`. No server change was needed.
16. **Golden tests replay the TypeScript's own verdicts.** For the parsers and the identity checks,
    `scripts/golden.mjs` runs the TypeScript over valid and tampered inputs (signed with Node's
    Ed25519) and records what it decided; the BrightScript must decide the same, error code included.
17. **Strings the Apple app types itself** (not in `packages/i18n`: "Portico couldn't reach that
    server…", the direct sign-in leads, the field examples) are in `scripts/apple-strings.mjs` under
    `apple.*` ids, each pinned to its Apple file; `generate` fails when the text changes there. They are
    written into every locale as the Apple app shows them. The generator also fails on any message id
    in the channel's sources that the catalogue does not have.
18. **A flow is a Task.** HTTP cannot run on the render thread. Each step (`SessionTask`) and the code
    loop (`SetupCodeTask`) runs in a Task thread as plain sequential BrightScript; screens observe one
    session node (`components/app/AppSession`, published as `m.global.session`), as Apple screens use
    `useSession()`.
19. **Probing order for a Portico Account's server:** the signed LAN candidates first, then the others;
    an address that answers as a different server ends the attempt.
20. **Registry layout:** section `portico` (installation id, the selected server, the account's
    session, the last account server) and one section per server, `portico.server.<serverId>`, holding
    its record (address, pin, session, username, profile and server names). Nothing is encrypted.
    Roku's registry is small (16 KB per developer id): only the selected server's record is kept.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| No "Sign in with a Portico Account" button on Activation; "Sign in directly to a server" is the one button and takes focus | Rule 11: a television signs in by code or by server address. On Apple TV that button opens the account's password form with the providers. The code on the screen is the account's sign-in |
| The brand and ring stay until the remembered server has proved its identity | The Apple app opens the shell at once and confirms in the background. Here the saved token is not sent until the pinned server proves itself; in BrightScript that verification may take seconds (unmeasured: no device) |
| A code is not resumed after the channel exits | `SetupCodeService` keeps a durable journal; a Roku channel that exits cancels its code. A new one is made on return |
| A profile's PIN uses Roku's `StandardPinPadDialog` | Rule 10. The Apple TV app draws its own keypad |
| Profile pictures are not drawn; the initial on the profile's colour is | A SceneGraph `Poster` cannot send the bearer the picture needs |
| No "Watch downloads" on the saved-server screen | Roku has no downloads |
| No discovery of servers on the network | Roku has no mDNS API for channels (handoff §2) |
| Text typed with Roku's `StandardKeyboardDialog`, one dialog per field | Rule 10. Confirming a field moves into the next, as TV-AUTH-03 does |

### Not done, and why

- Joining a server from an invitation link (`JoinServer`), creating an account, password recovery: phone
  and web flows; the Apple TV app does not offer them on television either.
- Trusted-profile tokens (remembering a PIN on this device): `selectDirectProfile`'s `trust` input is
  not sent. The Apple TV chooser does not send it either.

### Checks

- `npm run check`: clean. `npm run package`: 1,712 KB.
- `npm test`: `TESTS PASSED: 4042 checks in 62 tests` (about a minute; the five Ed25519 verifications
  take most of it in the interpreter).

### Commits

- `3ac5172` Roku: the session core (identity, sign-in, codes, the Portico Account)
- `f6fc0e3` Roku: sign-in screens and the shell

## Phase 2 — Shell

### Built

| Apple | Roku | Compare |
|---|---|---|
| `shell/Rail.tsx`, `rail-expansion.ts` | `components/shell/Rail` (`ShellRail`) | 112 / 340; padding 20; top 60 + 20; head 44 (mark 34, wordmark 30); items 64 high, 4 apart, radius 14, icon 30; active tint 0.14 and a 3-wide bar; collapse after 140 ms; 180 ms |
| `shell/Shell.tsx`, `Roots.tsx` | `components/shell/Shell` | content pushed by 228 while the rail is open; page fade 180 ms |
| `shell/ShellFrame.tsx` | in `PorticoLanding` | the rail with no destinations, content max 900 |

Back: pop the stack; at a section root focus the rail; on the rail, leave. Left from the content's edge
reaches the rail; Right returns to the control that had focus. `claimFocus` refuses a screen's entry
focus while the rail has focus (`railBrowsing` in `ui/theme.ts`).

### Decisions

21. **OK on a rail item selects it and focus stays in the rail** (as on Apple TV); Right enters the
    content.
22. **Every root stays mounted once chosen** (`Roots.tsx`). Shelves outside the viewport hold no cards
    (`UiPage`), which bounds what a kept root costs.

### Differences

| Difference | Reason |
|---|---|
| Channels is not in the rail yet | It appears when the server has a source; that check arrives with phase 9 |
| No scanning dot on a library | The scan status poll (`useScanningLibraries`) is not ported yet |
| No unread badge on Settings | Notifications (the inbox) are not ported yet |

### Checks and commits

Same run as phase 1. Commit `f6fc0e3`.

## Phase 3 — Home

### Built

| Apple | Roku | Compare |
|---|---|---|
| `client-core/home.ts`, `library-content.ts` (entries) | `source/core/Home.bs`, `ContentEntry.bs` | bounded parsers; a future entry kind leaves out only that card |
| `presentation/card-caption.ts`, `card-shape.ts`, `home-hero.ts` | `source/core/CardCaption.bs` | captions, Continue Watching titles, "x left", one shape per row |
| `presentation/server-text.ts`, `content.ts` | `source/core/Cards.bs` | row titles from the catalogue by the server's code and params |
| `app/open.ts`, `app/routes.ts` | `source/core/Open.bs`, `components/app/Nav.bs` | what pressing an entry opens |
| `screens/Home.tsx` | `components/screens/Home` (`ScreenHome`) | hero 620 high, words at most 62% wide, display title 2 lines, synopsis 3 lines; Play (primary, large) then Details |
| `screens/HomeRow.tsx`, `home-row-page.ts` | `components/screens/HomeRow` | a recommendation row as a paged grid, 60 at a time |
| `screens/HomeEmpty.tsx` | in `ScreenHome` | owner and member words |
| `ui/Shelf.tsx`, `ui/Grid.tsx`, `ui/Card.tsx` | `UiShelf`, `UiGrid`, `UiCard` | RowList and MarkupGrid create cards only for what is on screen |

Parsing happens in the Task thread (`components/tasks/ApiTask` with `source/core/Readers.bs`); the
screen receives summaries and ready content nodes. Rows keep at most 100 cards (`HOME_ROW_MAX_ENTRIES`)
and page by the row's own cursor; only shelves near the viewport hold cards.

### Decisions where the handoff was silent

23. **A transient failure is retried on its own** (three quick tries, then slower), keeping the
    skeleton or the rows on screen, as the Apple app's content hook does (`source/core/Retry.bs`).
24. **Returning to Home re-reads it after 10 seconds** (the Apple app re-reads on focus).
25. **Artwork requests carry the session** through an `roHttpAgent` on the screen layer, since
    `/v1/media/…` needs the bearer token and a Poster cannot be given headers one by one.
26. **The fixtures script** (`scripts/fixtures.mjs --account <path>`) reads the development account
    from the file it is pointed at, prints nothing from it, and strips session tokens, signed cursors
    and media grants from what it saves.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| No blurred cover behind a hero whose art is a cover (music, books) | SceneGraph has no blur; the atmosphere shows instead |
| The hero's backdrop fades as the rows cover it, rather than scrolling away | A Poster behind a scrolling group is cheaper than moving a 1080p image each frame |
| "Customize Home" is not built | It is in Settings on Apple TV (phase 8), and edits `/v1/home/layout` |
| The paged row's title stays above its grid | A MarkupGrid has no header that scrolls with it |
| The hero says "Resume" (no time) when under two minutes are left | The Apple hero would say "Resume · 1m"; see question 3 |

### Not done, and why

- **Not interested** on a recommendation card: it belongs to the card's menu (phase 5).
- **Top Shelf**: tvOS only.

### Checks

- `npm run check` clean; `npm run package` 1,741 KB; `npm test`: `TESTS PASSED: 6691 checks in 70 tests`.
- Golden cases run the real `home.ts`, `card-caption.ts`, `card-shape.ts` and `home-hero.ts` over the
  development server's own Home document and synthetic entries of every kind.

### Commits

- `ae0a047` Roku: Home

## Phase 4 — Library

### Built

| Apple | Roku | Compare |
|---|---|---|
| `presentation/library-tabs.ts` | `source/core/LibraryTabs.bs` | Discover first; Movies / Shows + Recently aired / Artists, Albums, Songs, Genres, Playlists (+ Unmatched for the owner) / Books, Authors, Series; Collections |
| `client-core/browse.ts` | `source/core/Browse.bs` | capabilities, result, facets and expression parsers; the request body; anchor labels |
| `presentation/browse-toolbar.ts`, `app/browse.ts` (labels) | `source/core/BrowseToolbar.bs`, generated `BrowseLabelTables.bs` | promoted fields, three groups, field controls, range presets, applied labels, the Sort label |
| `library-content.ts` (projection) | `source/core/ContentProjection.bs` | checked against the server, library, view and entity asked for |
| `collections` (`createWindowedCollection`), `app/browse-page-source.ts` | `source/core/BrowseWindow.bs` | at most five chunks resident; a chunk is whole rows, at most 60 |
| `screens/Library.tsx` | `components/screens/Library` (`ScreenLibrary`) | the title with "Server · 1,234 movies", then one row: tabs, Sort, Filter, Jump to |
| `screens/BrowseFilters.tsx` (`BrowseBar` inline, `FieldValues`) | `components/screens/library/Filters` | side panel, two levels; values with counts, Any/Yes/No, choices, presets; at most 40 values |
| `screens/BrowseFilters.tsx` (`JumpTo`) | `components/screens/library/JumpTo` | keys at least 72 wide; offered from four anchors, by title, unfiltered |
| `screens/LibraryTabs.tsx` | in `ScreenLibrary` | genre cards (cover read when on screen), the viewer's playlists |
| `ui/ListRow.tsx` in a list | `UiRowCell`, `UiGrid` mode `rows` | the Songs tab |

**Scale.** The grid never holds the library. `BrowseWindow` keeps five neighbouring chunks (280–300
slots) whatever the total; a slot is a card once its chunk has arrived and a placeholder until then;
as focus reaches the first or last resident chunk the window moves to centre on it and the chunks that
left are dropped. Two requests run at a time, nearest to focus first. Jump to moves the window to the
server's position for a letter. Tested at 10,000,000 titles (`Test_Library_window`).

### Decisions where the handoff was silent

27. **Chunks are whole rows.** A page is 60, the grid is 7, 6 or 4 columns wide; a chunk is the
    largest whole number of rows that fits in 60 (56 for posters), so moving the window never moves
    a card sideways. The first request (made before the shape is known) asks for 60 and its first
    chunk is used.
28. **A new revision restarts from where the viewer is.** When the server refuses a range as stale,
    or a chunk reports another total or revision, the first page is read again and the window
    returns to the focused position.
29. **A failed chunk is asked for again after two seconds**; a failed first page shows the Apple
    message with Try again.
30. **A Discover row's next page is appended to that row** (to 100 cards), with the continuation
    checked against the page it continues (viewer fence, revisions, one section, a cursor that does
    not repeat). See question 4.
31. **Opened before the library list has arrived**, a library names itself from the capabilities or
    the projection's scope.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| On an engine tab the title and the tab row stay above the grid | A MarkupGrid has no header that scrolls with it. On Discover they scroll with the rows, as on Apple TV |
| No bucket re-anchoring after a stale revision on a non-title sort | Decision 28 returns to the same position instead of the same bucket |
| A category's titles (the legacy `category` route) are a paged row, not a grid | Nothing in the tabs reaches it; it keeps the page bounded |
| No "scanning" subtitle or scanning empty state | `useLibraryScan` (the scan events) is not ported |
| "Save view" is absent | It is absent on Apple TV too (the inline bar has no Save view) |
| A warning above the grid when a later page fails | Decision 29 retries instead |

### Not done, and why

- **Recommendation rows do not hide what was turned down** (`withoutNotInterested`): arrives with the
  card menu in phase 5.
- **Playlist pages** open `ScreenPlaylist`, which arrives with Saved (phase 6).

### Checks

- `npm run check` clean.
- Golden cases run the real `library-tabs.ts`, `browse.ts`, `browse-toolbar.ts` and
  `validateContentProjection` over the development server's capabilities, browse pages, facets,
  Discover and entity documents for five libraries, each also with one thing wrong (20 broken
  capabilities, 13 broken pages, 13 broken projections).
- `npm test`: `TESTS PASSED: 10985 checks in 77 tests` (about five minutes in the interpreter; the
  Library cases are 4,294 of them). `npm run package`: 1,797 KB.
- `node scripts/test.mjs <word>` runs only the tests whose name holds that word, for use while
  fixing one area; the phase's full run is the one above.

### Commits

- `18c56ca` Roku: Library (tabs, browse engine, filters, windowed grid)

## Phase 5 — Title pages

### Built

| Apple | Roku | Compare |
|---|---|---|
| `client-core/detail.ts`, `related-movies.ts`, `recommendations.ts`, `container-state.ts`, `listening.ts` | `source/core/Detail.bs` | the projection, credits pages, personal-state command and receipt, container state, related and recommendation rows |
| `client-core/show-workspace.ts` | `source/core/ShowWorkspace.bs` | show, seasons, groups, one page of episodes, next up, settings |
| `client-core/people.ts` | `source/core/People.bs` | a person's page and credits by role |
| `presentation/title-page.ts`, `detail-actions.ts`, `entry-actions.ts`; `screens/title/facts.ts` | `source/core/Title.bs` | facts lines, key crew, show people, the action row, the card menu |
| `presentation/title-files.ts`, `language.ts` | `source/core/TitleFiles.bs`, generated `LanguageTables.bs` | "4K · HEVC · Dolby Vision · MKV", one line per track |
| `screens/title/TitleHero.tsx` | `components/screens/title/Hero` (`TitleHero`) | 60 % of the screen; padding 80 / 24; gap 40; poster 240, square 280; display title, 2 lines; synopsis 2 lines at most 960 wide, "More" past 160 characters; large actions 16 apart; the focused action's name beside the row |
| `screens/Detail.tsx` | `components/screens/Detail` | cast 12 then See all; key crew, All crew; related rows of three or more; extras; Details; Files |
| `screens/Show.tsx`, `title/SeasonSwitcher.tsx`, `title/Episodes.tsx` (`EpisodeBrowser`) | `components/screens/Show`, `title/EpisodeCell` | tabs to 8 seasons, a picker beyond; rows 92 high, 6 in view, list 42 % wide, preview at most 780; number 44 wide |
| `screens/title/EpisodePanel.tsx` | `components/screens/title/EpisodePanel` | side panel 640; still, title, line, Play/Resume, Play from start, Mark watched, ⋯, synopsis, director and writers, cast with See all, files, previous / next |
| `screens/Entity.tsx`, `title/TrackList.tsx` | `components/screens/Entity` | album, disc, artist, book, author, series, collection, category |
| `screens/Person.tsx` | `components/screens/Person` | circular hero, Known for, All · Cast · Crew, credits 40 at a time |
| `player/EntryMenu.tsx` | `components/app/EntryMenu` | the remote's * key on any card |
| `screens/title/TitleDetails.tsx`, `TitleSheets.tsx` (`RateSheet`) | `components/screens/title/Common.bs`, `Rate.bs` | readable rows; genres as buttons; five stars and Clear rating |
| `app/container-state.ts` | `components/screens/title/Container.bs` | flags shown at once and put back on failure; a stale watched write reread once |

An episode's link opens its show's page with the episode's panel (the shell gained `replace`). A
personal-state change is one command with a new UUID against the revision the page was read at; the
page shows the server's receipt and then reads itself again. After a change made from a card's menu
the page on show re-reads (`NotePersonalChange`), and Home and Discover notice when returned to.

### Decisions where the handoff was silent

32. **The * key is the long press.** A card's menu, an episode's panel and a track's menu open on it.
33. **Language names come from a generated table** (the catalogue's `language.*` first, then the
    names Node's ICU gives, in English and in the language itself): a Roku has no `Intl.DisplayNames`.
34. **Calendar dates are formatted without a time zone** (an air date printed in Halifax is the day
    the server sent).
35. **The card menu opens once the title's state has been read** (a detail or container read), not
    before and then changing under the viewer.
36. **The episode list holds a slot for every episode of the season on show** and reads pages by
    the server's cursors as focus nears them (a page whose cursor is unknown is reached by reading
    the pages before it, as the Apple source does). Its size follows the season, not the library.
37. **A person's credits append as the grid nears its end**, 40 at a time; cards exist only for the
    rows on screen.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| The tagline is not italic | Manrope ships no italic face in the channel |
| A compact hero (no backdrop) has no blurred copy of the art behind it | SceneGraph has no blur |
| The title's logo is not drawn in place of its text | Not built; the text title always shows |
| An episode without a still shows its code on the artwork placeholder, not on the show's own tint | The per-show hue (`titleTintHue`) needs a shape tinted at run time for each show; the placeholder colour is the theme's |
| No Undo on "Removed from Continue Watching" and "We won't recommend…" | A toast cannot take focus on a remote; the change re-reads the page instead |
| A failed save on a show is a toast; on a movie it is the notice with Retry, as on Apple TV | The show's flags are written optimistically and put back |
| An album's, artist's or book's list longer than a page grows by "Show more" | The Apple list is windowed; here its cost follows what the viewer asked for |
| Cast cards in the episode panel are the standard 150 size (Apple: 120) | One person card size is generated |
| No "Show settings" (hide seasons, absolute numbering, ranges, newest first) for the owner | Not built; see Not done |
| No "Jump to" a season or episode number on very long shows | Not built; see Not done |
| A recommendation turned down leaves when the page re-reads, not at once | The server's answer is the list |

### Not done, and why

- **Pick before play** (version, audio and subtitle choices on the hero and in the panel): it reads
  the playback options document and belongs with the player (phase 7).
- **Add to playlist… / Add to collection…**: the sheets arrive with Saved (phase 6); the menu
  entries call the scene's `organize`.
- **Play next / Add to queue**: offered when the player has a queue (phase 7).
- **Show settings** and **Jump to** on the show page: time. Both are self-contained additions
  (`saveShowSettings`; a numeric field in a sheet).
- **Segment markers** in the detail document are passed through unvalidated until the player reads
  them (phase 7).

### Checks

- `npm run check` clean.
- Golden cases run the real `DetailService`, `ShowWorkspaceService`, `parsePersonPage`,
  `validateItemRecommendations`, `getContainerState`, `detailActions`, `entryActions`,
  `titleFileLines`, `trackName` and `facts.ts` over the development server's movie and episode
  details, a show's workspace (default and a chosen season), a person, recommendation rows and
  container states: 52 details, 36 workspaces, 16 person pages, each with one thing wrong where it
  should be refused.
- `npm test`: `TESTS PASSED: 11741 checks in 82 tests`. `npm run package`: 1,910 KB.

### Commits

- `0abc1d1` Roku: title pages (movie, show and episodes, album, artist, book, person) and the card menu

## Phase 6 — Search and Saved

### Built

| Apple | Roku | Compare |
|---|---|---|
| `client-core/search.ts`, `presentation/search-model.ts` | `source/core/Search.bs` | the nine groups in the chips' order, group failures, the Top result rule, what is searchable |
| `client-core/saved.ts`, `personal-saved.ts` | `source/core/Saved.bs` | My List, Favorites, Playlists, one playlist, history, collections, saved views |
| `screens/Search.tsx` | `components/screens/Search` | field, chips with counts, Top result (art 160), shelves per type with See all, a grid for one type, recent searches (8) |
| `screens/Saved.tsx` | `components/screens/Saved` | six tabs; sort chips over a non-empty list; grid, lists, history by day; Clear history asks first |
| `screens/title/SavedPages.tsx` | `components/screens/Playlist`, `SavedResource` | hero with a mosaic of the first four covers; Play and Shuffle; numbered entries, hidden ones keep their number |
| `player/EntryMenu.tsx` (`PlaylistSheet`, `CollectionSheet`) | `components/app/OrganizeSheet` | existing ones as rows, a name field and Create |

### Decisions where the handoff was silent

38. **A search runs when the keyboard closes.** The Roku keyboard is a dialog over the page; there
    is no typing to debounce behind it.
39. **Recent searches live in memory for the session**, per server and profile, as on Apple TV
    (the server publishes no search history yet).
40. **Grids on these pages sit inside the page**: the page scrolls until the grid fills the screen,
    then the grid's rows move under the focus. Cards exist only for the rows on screen, and pages
    are appended as focus nears the end (My List, Favorites, a collection, one search type).
41. **Adding to an existing playlist reads its revision first**, then sends one add command with a
    new operation id.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| Search does not run as letters are typed | Decision 38 |
| The Top result compares titles without folding accents | BrightScript has no Unicode normalization; "Amélie" matches itself, not "Amelie" |
| A day of History is a row of cards, not a grid; more History is behind "Show more" | One scrolling grid per page on this platform |
| A playlist's entries cannot be moved or removed | Not built (the row menu and the windowed reorder) |
| The search history preference (`search.rememberHistory`) is not read | It arrives with Settings; history is kept |
| A playlist longer than a page grows by "Show more" | As on entity pages |

### Not done, and why

- **Playlist management** (rename, reorder, remove, share, delete, pin) and **collection
  management**: the pages read and play; changing them is on the phone and web today.
- **Saved views are opened, not made**: Library's "Save view" is not on television.

### Checks

- `npm run check` clean.
- Golden cases run the real `SearchService`, `topSearchResult`, `SavedService` and
  `PersonalSavedService` over the development server's search answers (all types, and one type),
  My List, Favorites, Playlists and history, with 48 search variants and 46 saved variants.
- `npm test`: `TESTS PASSED: 11979 checks in 85 tests`. `npm run package`: 1,955 KB.

### Commits

- `f00cd1d` Roku: Search and Saved (lists, playlists, collections, history)

## Phase 7 — Player, then listening

### Built

| Apple | Roku | Compare |
|---|---|---|
| `playback-v1/types.ts`, `sessions.ts`, `queue.ts`, `timeline.ts`, `quality.ts`, `post-play.ts` | `source/core/Playback.bs`, `PlaybackReaders.bs` | options, a session and its presentation, a queue header, entries and window; the start, change, timeline, queue and advance bodies; the 202 wait; the post-play plan |
| `client-profile.ts` (`rokuDeclaredProfile`), `playback-v1/capability-profile.ts` | `source/core/ClientProfile.bs`, `ClientProfileTables.bs` (generated) | what the device decodes and shows, from `roDeviceInfo`; published at sign-in |
| `player/PlayerHost.tsx`, `VideoPlayer.tsx` | `components/player/PlayerHost` | title at the top, scrims 260 and 420, insets 60 and 80, chrome hides after 4 s |
| `player/Timeline.tsx` | the same component | track 8, knob 22 (1.5 × in focus), one focus stop 64 high, Left and Right seek |
| `player/PlayerOptions.tsx` | the same component, `UiMenu` | boxes Audio · Subtitles · Quality · Sleep, each saying its value |
| `player/SkipPrompt.tsx`, `PostPlay.tsx` | the same component | Skip intro, recap, credits; Up next with the server's countdown; Still watching? |
| `player/MiniPlayer.tsx` (`NowPlayingPill`) | the same component | top right 60 and 80, art 48, at most 520 wide |

### Decisions where the handoff was silent

42. **One player, in the scene's own layer.** A title is one session (`POST /v1/playback/sessions`);
    a list, an album or a show is the server's queue (`POST /v1/queues`, then `:advance`). The
    channel hands the Video node the URL it is given and chooses nothing.
43. **A title being prepared (202) is asked for again with the same key**, after the wait the server
    names (a quarter of a second to ten), for two minutes at most — as `sessions.ts`.
44. **The session's bearer is never sent with media.** The URL carries its own grant.
45. **Advancing does not end the old session.** `:advance` replaces it; the channel reports where
    the title stopped first.
46. **"Play now" is the viewer's choice (`next`); the countdown is `completion`**, as the Apple
    queue player sends them.
47. **Hidden controls hold no focus.** The key that brings the chrome back only brings it back.
    Back hides the chrome, then: audio goes behind the pill, video stops.
48. **Remote keys:** Play/Pause toggles (anywhere, while audio plays behind a page); Fast forward
    and Rewind skip 30 and 10 seconds; Instant Replay skips back; Up from the top of a page
    reaches the pill.
49. **What the queue moved to** is named from the entries the page handed over, else from the
    queue's own window (read around the current entry); the album's or show's artwork stays.
50. **A failure behind the pill is a toast**, and the pill goes: there is no card to show it on.
51. **The sleep timer is the channel's own timer** (15, 30, 45, 60 minutes, read from
    `PlayerOptions.tsx`); it pauses where it is.
52. **The device's profiles are published once per sign-in**, when the shell opens.
53. **Subtitles are the Video node's**, from the session's subtitle URLs; whether a subtitle is
    burned in is the server's decision (`delivery: auto`).

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| No Speed box | The Video node plays sound at one speed |
| No lyrics, Picture in Picture, Watch Together or cast controls | Not on this platform, or not built |
| No queue list in the player | Not built; Next, Previous, Play next and Add to queue work |
| Songs end into the next song with the platform's gap | No gapless or crossfade on the Video node |
| The sleep timer has no "end of chapter" or "end of book", and is not the server's listening deadline | Decision 51 |
| ~~Skip amounts are 10 and 30 seconds~~ | Resolved in phase 8 (decision 56): the viewer's `playback.skipBackSeconds` and `skipForwardSeconds` are read; 10 and 30 remain the fallback |
| ~~A marker is always offered~~ | Resolved in phase 8 (decision 56): intro, recap and credits follow ask, skip automatically or never |
| The now-playing pill has no progress ring | Not built |
| Every session says `network: local` | The channel cannot tell home from away; see question 5 |

### Not done, and why

- **Pick before play** (version, audio and subtitles on the title page): the player's own boxes
  change audio, subtitles and quality once it plays; a version cannot be chosen.
- **Chapters and trick-play thumbnails** on the timeline: the options document's chapters are
  parsed and not drawn.
- **Resuming audio after the channel restarts**, deep links (`roInput`) and voice transport.
- **Listening rules** ("Are you still listening?", autoplay off for audio): audio always goes on.

### Checks

- `npm run check` clean; `lint-literals` clean (player sizes are pinned in
  `scripts/apple-literals.mjs` with their Apple source text).
- Golden cases run the real parsers over the development server's playback options (movie,
  episode, song), a session (direct play, and after a quality limit), a queue and its advance;
  the Roku profile was accepted by the server (`PUT /v1/playback/client-profile`).
- `npm test`: `TESTS PASSED: 12124 checks in 91 tests`. `npm run package`: 1,995 KB.
- **Not seen on a Roku.** The Video node's behaviour (`playStart`, `subtitleTracks`,
  `streamFormat` for the server's `mp4` and HLS answers) is from documentation only.

### Commits

- `8eefa5b` Roku: player on Playback v1 (sessions, queue, timeline reports), chrome, post-play,
  now-playing pill; device profiles published at sign-in. It also carries the generator's
  settings tables (phase 8 groundwork).

## Phase 8 — Settings (Account pages and the Dashboard)

### Built

| Apple | Roku | Compare |
|---|---|---|
| `presentation/settings-structure.ts` (`settingsStructure`, television) | `source/core/SettingsTables.bs` (generated), `Settings.bs` (`SettingsStructureFor`) | the same headings, pages, sections and rows, for the same viewer |
| `preferences.ts`, `preferences-reader.ts`, `app/server-preferences.tsx` | `source/core/Settings.bs` | the registry, effective values, one PATCH per scope against its revision |
| `settings-structure.ts` (`readComposite`, `compositePatch`) | the same file | Quality at home and away, Playback method, HDR |
| `presentation/preference-copy.ts`, `region-choices.ts`, `settings/language-list.ts` | the same file | a stored value in words, language and time-zone choices, the ordered language list |
| `screens/Settings.tsx` (`TelevisionSettings`) | `components/screens/Settings` | pages column 560 wide, the open page beside it, a page opens as focus reaches it |
| `settings/SettingsPage.tsx` (`RowView`, `Panel`), `SettingsPages.tsx` (`WebHandoff`, `SignOutConfirm`), `settings/ResetRecommendations.tsx` | the same component | switch, value picker, language list, Sign out of this TV, Reset recommendations; QR 200 for what belongs on a phone |
| `server/Dashboard.tsx`, `server-admin/now-playing.ts`, `dashboard.ts`, `console-words.ts` | the same component, `source/core/ServerAdmin.bs` | Watching now with Stop playback, alerts with Acknowledge, health and load, tasks with their commands |

### Decisions where the handoff was silent

54. **The structure is generated by asking the shared code.** The authored `needs` are functions;
    the generator calls `settingsStructure` for every combination of what they read and records,
    for each page, section and row, the flags it depends on and the combinations it is present for.
    A golden test compares the BrightScript structure with the TypeScript one for 14 viewers.
55. **Rows this channel cannot honour are left out**: video and audiobook speed, crossfade,
    gapless, lyrics, and "Receive from other devices". They remain the profile's elsewhere.
56. **The player reads the skip preferences**: skip-back and skip-forward seconds, and intro, recap
    and credits as ask, skip automatically, or never (this removes two phase 7 differences).
57. **Text size and Reduce Motion are this installation's** (the Roku registry), read when the
    channel starts.
58. **A changed value shows at once and goes back if the server refuses it**; the refusal is a toast.
59. **The Dashboard is read again every 30 seconds** while it is open, and after each command.
60. **The device noun in "Sign out of {server} on this {device}?" is "Roku".**
61. **The words the shared code keeps as English tables** (task names and states, alert names) are
    generated from `console-words.ts`, not retyped.

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| No devices list under Profile | Not built (approve, trust, sign out, remove) |
| The More group has Switch profile only | Switch server, Notifications and Watch Together are not built; see question 6 |
| Stop playback has no message field | Not built; the stop itself is sent |
| "Open" is not offered on a session | Needs the linked-title lookup |
| Health shows current figures without the 10 m, 1 h, 24 h switch and history lines | Not built |
| The state-folder permissions alert has Acknowledge, not "Fix permissions" | Not built |
| The Dashboard refreshes on a timer, not on server events | Decision 59 |
| Language, time zone and hour cycle are saved for the profile but this channel follows the Roku's own language and clock | Roku picks translations by device locale; see question 7 |
| A larger Text size applies to pages opened after the change | Pages already open are not laid out again |
| A megabit rate that is an exact half (0.95) rounds up | JavaScript's `toFixed` rounds the nearest double |

### Not done, and why

- **Devices** (list, approve, trust, sign out, remove) and **Switch server** from Settings.
- **Notifications** and **Watch Together**: no screens on this channel yet.
- **Server pages other than the Dashboard**: television shows the Dashboard only, by the structure.

### Checks

- `npm run check` clean; `lint-literals` clean.
- Golden cases run the real `settingsStructure`, `findSettingsPage`, `parsePreferenceSnapshot`
  (the development server's registry and 8 broken documents), `preferenceValueLabel` (221 values),
  `readComposite` (17 states), `compositePatch` (27 choices), `parseAdminSession`, `healthFacts`,
  `measuredLoadTiles` and the console's words over the development server's own documents.
- `node scripts/test.mjs settings`: `TESTS PASSED: 504 checks in 4 tests`.
- `npm test`: `TESTS PASSED: 12628 checks in 95 tests`. `npm run package`: 2,040 KB.
- **Not seen on a Roku.** The two-column focus, the QR handoff and the Dashboard's refresh are from
  reading the code only. The Dashboard was read from a server with nobody watching and no tasks;
  rows for sessions and tasks were tested with made documents.

### Commits

- `8e03328` Roku: Settings (account pages from the shared structure, preferences, the owner's
  Dashboard); the player reads skip preferences; guide core. It also carries the guide's parser
  and the rail's Channels check (phase 9), which the shell and the player compile against.

## Phase 9 — Channels / Live TV

### Built

| Apple | Roku | Compare |
|---|---|---|
| `client-core/channel-guide.ts` (`parseChannelGuide`, `guideWindow`, `path`) | `source/core/Guide.bs` | the window that is on now, thirty channels a page, the same strict reading of sources, channels and programmes |
| `app/linear-availability.ts` | `components/shell/Shell` (`ShellProbeChannels`) | the rail shows Channels only where the server has sources; the last answer per server is kept |
| `screens/channels/ChannelsScreen.tsx`, `ChannelList.tsx`, `format.ts` | `components/screens/Channels` | a source picker; each channel with its number, what is on, how long is left, what follows, and progress |
| `playback-v1/channel-control.ts` (start, stop) | `components/player/PlayerHost` (`request.channel`) | `POST /v1/playback/sessions {channelId, channelGeneration, state}`; Pause/Play only; stop is a plain `DELETE` |

### Decisions where the handoff was silent

62. **The guide is the list, not the time grid.** Each channel says what is on now and next; the
    words move with the clock every 30 seconds without the list being rebuilt.
63. **The source picker lists each Live TV source by its name, then Library Channels**, from one
    request of each kind (the same request that decides whether the rail shows Channels).
64. **Unknown counts as present**, as on Apple: the rail hides Channels only after the server says
    it has no sources (or answers 404), and remembers that per server.
65. **A channel that cannot be tuned says why in a toast**; it is not hidden.
66. **Hidden channels are left out** (there is no "show hidden" on television).

### Differences from the Apple TV app

| Difference | Reason |
|---|---|
| No time grid, day picker, group chips or favourites filter | Not built; the list is the guide's List view |
| No programme sheet (details, Record, reminders) and no Recordings | Not built |
| A tuned channel joins where the stream starts, without the go-live seek | Not built; the Roku player starts a live playlist at its live edge |
| No mini guide or channel up/down inside the player | Not built |

### Not done, and why

- **Tuning was never exercised.** The development server has no Live TV source and no Library
  Channel, so no channel session could be asked for. The start body is the Apple client's; what
  the server answers (the presentation of a channel session, `409 no_tuner_available`) is read by
  the same session parser as a title, and that has not met a real answer.
- **Recording and reminders**, **the time grid**, **channel search**.

### Checks

- `npm run check` clean; `lint-literals` clean.
- Golden cases run the real `parseChannelGuide` over two made guides (live and library), the
  development server's own answers for both kinds (no sources) and 22 broken guides; `guideWindow`
  at four moments; the channel ids of `v1ChannelId`.
- The tests found two precision faults before they shipped: the hour floor and a programme's
  progress were computed in 32-bit floats and whole-number division; both now use whole seconds.
- `npm test`: `TESTS PASSED: 12688 checks in 97 tests`. `npm run package`: 2,040 KB.
- **Not seen on a Roku**, and not tuned against a server (see above).

### Commits

- `390795c` Roku: Channels (what is on now and next per source, tuning through the player); the
  rail offers Channels only where the server has them

## Round 2 — 5 Oct 2026 (Justin's answers, and what they led to)

Justin's guidance on the ten questions: recommendations accepted for 1–6, 9 and 10; for 7, the
profile's choice wins over the device (as on Apple); for 8, build lyrics and Receive from other
devices, and drop what a Roku's player cannot do. Standing rule: **build a feature out rather than
hide it**; drop UI only where the platform cannot do the thing.

### Built

| Apple | Roku | Compare |
|---|---|---|
| `auth/ProfileAvatar.tsx` (`useProfilePictures`), `identity-restrictions.ts` (`parseProfileAvatar`) | `components/screens/auth/DirectProfiles`, `source/core/DirectSignIn.bs` (`ParseProfileAvatars`) | the uploaded picture over the initial, read with the token the chooser holds; a picture that does not load leaves the initial |
| `presentation/settings-structure.ts` (two new capabilities: `playbackSpeed`, `audioEffects`) | `source/core/Settings.bs` (`RokuSettingsCapabilities`) | the rows a Roku's player cannot honour are left out by the shared structure, not by a list on the Roku side |
| `packages/i18n` (`createI18n` with the profile's locale), `region` preferences | `source/core/LocaleTables.bs` (generated), `Messages.bs` (`RegionChoice`), `Format.bs` (`RegionFor`) | the profile's Language, hour cycle and UTC apply over the device's |
| `player/Lyrics.tsx`, `client-core/lyrics.ts` | `components/player/PlayerLyrics.bs`, `source/core/Lyrics.bs` | the line being sung is bright and rests a third down; artwork 0.22 of the height beside lyrics; a line can be pressed to jump to it; Choose lyrics when several exist |
| `app/receiver.tsx` (`ReceiverMode`), `client-core/remote-playback.ts` (`ReceiverHost`), `social-playback.ts` | `components/tasks/ReceiverTask`, `components/shell/Shell`, `source/core/Receiver.bs` | registers as a receiver, asks "Let {device} play on this TV?", loads what is sent and reports readiness only once it is playing |
| `Security.tsx` (`DevicesSection`), `identity-devices.ts` | `components/screens/Settings` (devices panel), `source/core/Devices.bs` | where the account is signed in; approve, deny, trust, sign out, remove, with the same confirmations |
| `SettingsPages.tsx` (`ServersScreen`) | `components/app/AppSession` (`switchServer`), `MainScene` | another of the account's servers, or one by its address; Back returns to the server in use |
| `Notifications.tsx`, `client-core/notifications.ts`, `inbox.ts` | `components/screens/Notifications`, `source/core/Notifications.bs` | Unread, All, Archived; For you and Server for an owner; mark read, archive, Mark all read; the bell's count in Settings |
| `playback-v1/channel-control.ts` (`open`, `accept`, `tick`, `pause`, `resume`) | `components/player/PlayerHost` (channel path), `source/core/Guide.bs` (`ParseChannelSession`) | the session first, its stream once it exists, a go-live join, a read every second, pause and resume on the server |
| `delivery.ts` (`transportClassHeader`) | `source/core/Api.bs` | `X-Portico-Transport-Class: wifi` or `ethernet` on every request |

### What the questions turned out to be

- **Question 1 (profile pictures) needed no server change.** The reason logged in phase 1 was
  wrong: a node's HTTP agent carries the bearer for every image under it. Built.
- **Question 3 (the hero's Resume): the Roku is right and the Apple app has a small fault.** With
  one to two minutes left, `heroDuration` returns nothing and the Apple button reads "Resume ·
   left". The fix belongs in `home-hero.ts`; it was flagged as its own task and is not on this
  branch.
- **Question 5 (home or away) was already answered by the server.** `deliveryContext` resolves
  the network class from the address each request arrives on; a client only declares its
  transport. The channel now sends that header and no longer writes `network: local` in a
  session start. No server change.
- **Question 9 (a channel to test with) found a real fault.** A Library Channel ("Movie
  channel", from the server's first template) was created on the development server. Its
  session starts with no stream address and `linear.state: preparing`; the phase 9 player would
  have failed on it. The channel path now follows `channel-control.ts`. The session parser is
  tested against that real answer.

### Decisions where the handoff was silent

67. **The receiver reads its inbox on the poll alone** (30 s ± 20 %, as Apple without its event
    stream): a Roku request cannot be read while it is still arriving. After a phone is allowed,
    the inbox is read every second for 90 s, because what it sends follows at once.
68. **The words that differ between the catalogue's locales are a generated table** looked up
    before Roku's own translations (6 of 762 messages today). When a locale that is not English
    arrives, this becomes a whole catalogue and needs a different mechanism.
69. **The lyrics panel is one focus stop**: Up and Down move along the lines, OK jumps, Back
    returns to Pause/Play. Only the lines near the one it rests on are drawn (a song can have 4,096).
70. **Switching server keeps the sign-in in use** until another replaces it; what is playing stops.
71. **A title sent from a phone arrives as its id**: the player reads its name from the detail document.
72. **The fixture script uses one installation id** (it had been registering a new device on
    every run; the development server shows 41 of them, which the new devices list can remove).

### Differences from the Apple TV app (new or changed)

| Difference | Reason |
|---|---|
| A phone's request is noticed within about half a minute, not at once | Decision 67; a long-poll on the receiver inbox (a server change) would close it |
| The television does not advertise itself on the local network | A Roku channel cannot publish a network service; phones find it through the server's receiver list |
| A named time zone other than the device's or UTC shows device time | BrightScript has no time-zone rules; the server would have to send the zone's offset |
| A device's "This device" and "Waiting" are words in the row's meta, not badges | The list row has no badge slot |
| The Portico Account's own notices are not above the notifications list | Not built (`AccountFeed.tsx`) |
| A channel's failure says the catalogue's playback words, not `channel-control.ts`'s own English sentences | Those sentences are not in the catalogue |
| Video speed, audiobook speed, gapless, crossfade and volume leveling are not offered | A Roku's player has one speed and gives a channel no access to decoded audio (checked against Roku's Video node reference, 5 Oct) |

### Not done, and why

- **Watch Together.** Not started.
- **Lyrics search, upload and offset** (the owner's tools): the player shows and chooses lyrics only.
- **The guide's time grid and programme sheet; the player's queue list; pick before play.** Unchanged from phase 9 and phase 7.
- **Nothing was run on a Roku**, as before.

### Checks and commits

- `npm run check` clean; `lint-literals` clean.
- `npm test`: `TESTS PASSED: 12996 checks in 106 tests`. `npm run package`: 2,082 KB.
- New golden cases run the real `parseProfileAvatar`, `parseLyricsView`, `activeLyricLine`,
  `lyricSeekPosition`, `parseReceiver`, `parseReceiverGrant`, `parseHandoff`, `receiverPollDelay`,
  `parseDevices`, `parseNotificationInbox`, `parseNotificationSummary`,
  `parseNotificationBatchResult` and `parseChannelSession`, over the development server's own
  answers where it has them (lyrics view, devices, notifications, a tuned channel) and made ones.
- Commits: `467becd` profile pictures; `4b5df14` lyrics, language and clock, shared settings
  rows; `1bae6ad` receiver, devices, transport header; `cf8da76` Switch server; `8656ea7` the
  channel session flow and the notifications core; the Notifications screen after it.

### Outside `apps/roku`

- `packages/client-core/src/presentation/settings-structure.ts`: two optional capabilities
  (`playbackSpeed`, `audioEffects`) and `needs` on five rows. Absent means "can", so the web
  and Apple structures are unchanged.
- **The development server** (`~/PorticoDev`, started from `.scratch/dev-local.sh` because it was
  not running): one Library Channel was created; a channel session, a title session and a queue
  were started and stopped by the fixture script. No server code was changed.

## Round 3 — 5 Oct 2026, later (the second set of answers)

Justin's guidance: the held inbox read is wanted, and it is the user's own server's (nothing is
asked of the account service); **times are always the device's own, on every client, and no time
zone is stored**; speed stays off Roku; build Watch Together and the Portico Account's notices;
a real Roku comes later.

### Built

| Apple or shared source | Where it landed | Compare |
|---|---|---|
| (new) `GET /v1/receivers/{id}/inbox/wait` | server repo, branch `roku-port`: `server/internal/httpapi/social_routes.go`, `route_lanes.go`, `server/api/social.md`, `social.openapi.yaml` | answers at once when a grant or handoff is waiting, as soon as one arrives, or empty after `waitSeconds` (0–55, default 25); realtime lane; four held reads a viewer |
| `remote-playback.ts` (`ReceiverHost`, in place of its event stream) | `apps/roku/components/tasks/ReceiverTask`, `source/core/Receiver.bs` | the television waits on the held read and asks again at once; on a server without it (404 or 405) it falls back to the 30 s poll |
| `screens/AccountFeed.tsx`, `client-core/account-notifications.ts` | `apps/roku/components/screens/Notifications` (the account's section), `source/core/AccountNotices.bs`, `Hosted.bs` (`HostedAccountNotices`) | the account's notices above the server's inbox for a Portico Account sign-in: what each kind says, Mark all read, Older, and the three choices |
| `client-core/social-playback.ts`, `group-session.ts` (`GroupSessionService`, `GroupFollower`) | `apps/roku/source/core/Together.bs`, `components/together/TogetherHost` | the group documents, the group's clock (least-round-trip sample), the correction bands, the follower's decisions, every command's body |
| `screens/Together.tsx` | `apps/roku/components/screens/Together` | start a group or join with a code; groups you are in; what is on and where; who is here; the invitation's code; the queue; Leave and End for everyone, asked first |
| `app/together.tsx` (`TogetherProvider`), `player/GroupPill.tsx` | `apps/roku/components/player/PlayerHost` (`groupDo`, `PlayerGroupTook`, the pill) | Play, Pause and seeking become the group's while its title is on; the follower steers the player directly; the pill says who is being waited for |
| `presentation/entry-actions.ts` (`together`) | `apps/roku/source/core/Title.bs`, `components/app/EntryMenu` | Watch with group and Add to group queue, in a room that lets this viewer add |
| the time-zone decision | `settings-structure.ts` (both repos), `apps/roku/source/core/Settings.bs`, `Format.bs` | no Time zone row; the channel formats in the device's zone only |

### The time-zone decision, across the board

Done in this session by a helper working in its own worktrees, at Justin's direction:

- **Server repo**, branch `device-time-zone`, `093da7a`: `region.timeZone` and the `timeZone` type
  leave the preference registry (revision `p39.CD32.B3` → `B4`); a stored document that still has
  the key is read and loses it on the next save (new test); the web client stops reading it; the
  shared packages lose the row, `timeZoneChoices` and three strings. The maintenance schedule's
  own zone got its own string (`web.schedules.timeZone`), because it had been borrowing the
  removed label.
- **Client repo**, branch `device-time-zone`, `d2bca05`: the same shared-package edits, and the
  Apple app's settings page, `i18n` setup and guide formatting use the device's zone only.
- Verified there: Go `operations` and `httpapi` on the runner; 71 + 14 package tests; the web and
  Apple type checks; string lints. Not run: a web build, the full suites, anything in a browser
  or simulator.
- Left on purpose: the server's own zones (maintenance windows, a Library Channel's schedule, a
  profile's viewing-hours limit set by the owner) and the guide request's `timezone` parameter.
- **A finding from that work: the shared packages' canonical copy is in the server repo**
  (`apps/portico-server/packages`); the client repo holds a copy checked by
  `scripts/shared-packages.mjs`. The Roku lane's own edit to `settings-structure.ts` (the two
  device capabilities) is therefore mirrored on the server repo's `roku-port` (`e535d58`).

### Decisions where the handoff was silent

73. **The held read is its own route** (`…/inbox/wait`), in the realtime lane, so the plain read
    keeps its five-second budget. Subscribed before the first read, so nothing that arrives
    between the read and the wait is missed.
74. **The room is read every second** while it is open, in place of the group's event stream.
    The heartbeat (10 s) and the follower (1 s) are as on Apple. A held read for groups would be
    the same server change as 73, and is not built.
75. **A small drift is left for the band's hold time, then closed by a seek**, exactly as the
    Apple player (which also has no speed nudge). The follower's decisions are one function,
    tested step for step against the real `GroupFollower` with a recorded player.
76. **The room's words come from the catalogue** (`web.together.*`), not from the literals in
    `Together.tsx`: state names, readiness, member status and why a group ended are all there.
    The two "Only the host can…" sentences are pinned from `app/together.tsx`.
77. **An invitation's code is shown large, and not shared**: a television has no share sheet.
78. **A group with no name is named for who started it** (`web.together.defaultName`); Apple
    types "Watch Together".

### Differences from the Apple TV app (new)

| Difference | Reason |
|---|---|
| A change in the room (Play, Pause, a seek) reaches this television within about a second, not at once | Decision 74 |
| The clock beside what is on moves when the room is read, not every second | Drawn with the room, not on its own timer |
| No "Reconnecting to the group" notice | There is no stream to lose; a failed read keeps the room and is tried again a second later |
| Shuffle and repeat for the group are not offered | Not on the Apple screen either |

### Checks and commits

- Roku: `npm run check` clean, `lint-literals` clean; `npm test`: `TESTS PASSED: 13190 checks in
  113 tests`; `npm run package`: 2,124 KB.
- Server (`roku-port`): `scripts/runner-test.sh ./internal/httpapi ./internal/social` on the
  runner: both `ok` (httpapi 121 s), including the new `TestSocialReceiverInboxWait`.
- New golden cases run the real `parseGroupSnapshot`, `parseGroupList`, `parseGroupQueue`,
  `parseGroupTransportReceipt`, `parseGroupInvite`, `groupTargetPositionUs`, `groupCorrection`,
  `GroupFollower` (seven scenarios), `parseAccountNotificationPage` and
  `parseNotificationPreferences`, over a real group from the development server at every stage.
- Client repo `roku-port`: `0ea054e` no profile time zone; `40ec839` the held read;
  `55aab5d` the account's notices; `6929cbd` Watch Together core; then the room, screen and player.
- Server repo `roku-port`: `3e8cf59` the held inbox read; `e535d58` the settings structure mirrored.
- Helper's branches `device-time-zone`: server `093da7a`, client `d2bca05`. Nothing is pushed or merged.
- The development server on the Mac was started for fixtures and **stopped again** at the end of
  this round. It keeps one Library Channel ("Movie channel") and an ended group ("Fixtures").

### Not done, and why

- **The held read was not exercised end to end.** The development server on the Mac is the
  3 Oct build and does not have the route; building the server is the runners' job. The route is
  tested on the runner; the channel's fallback (404 → the 30 s poll) is what the development
  server exercises.
- **Watch Together was not run with two members.** The parsers and the follower are tested
  against a real one-member group (made, loaded, playing, ended) and the real follower.
- **Nothing was run on a Roku**, as before.

## Open questions after round 3 (5 Oct)

1. **A held read for groups** (the same shape as the receiver's): wanted, so a Roku follows a
   Play or a Pause at once instead of within a second?
2. **Merge order.** Three branches now touch the shared packages: `roku-port` (both repos),
   `device-time-zone` (both repos) and whatever the other port lanes carry. The server repo's
   copy is canonical; who merges, and when?
3. **A Roku.** Later, by your word; it is still where the largest unknowns are.

## Questions for Justin (3 Oct; answered 5 Oct, see round 2)

1. **Profile pictures.** A SceneGraph `Poster` cannot send the bearer a profile picture needs, so
   the chooser draws the initial on the profile's colour. Leave it, or should picture URLs carry
   their own grant as media URLs do (a server change)?
2. **Start-up.** The Apple app opens the shell at once with the remembered sign-in and confirms it
   in the background. The channel shows the ring until the remembered server has proved its
   identity. Keep the stricter order?
3. **The hero's Resume.** Under two minutes left, the Apple hero says "Resume · 1m"; the channel
   says "Resume". Match Apple, or is the bare word right?
4. **A Discover row's next page.** The channel appends it to the row; Apple's
   `LibraryContentService` replaces the page. Which is intended?
5. **Home or away.** Every session start says `network: local`, because the channel cannot tell.
   A Roku away from home would get the at-home quality limit. Should the server classify the
   network from the request, or should the channel compare the server's address with private
   ranges?
6. **Settings' More group** has Switch profile only. Are Switch server, Notifications and Watch
   Together wanted on Roku, and in that order?
7. **Language and clock.** Roku picks the channel's language from the device. Honouring the
   profile's Language, Time zone and Hour cycle would mean reading the catalogue ourselves instead
   of Roku's `translations.xml`. Wanted, or should those rows say they follow the device here?
8. **Rows the channel cannot honour** (video and audiobook speed, crossfade, gapless, lyrics,
   Receive from other devices) are hidden. Hide, or show them as the profile's settings for its
   other devices?
9. **A channel to test with.** The development server has no Live TV source and no Library
   Channel, so tuning has never met a real answer. Can the demo server get one Library Channel so
   the session answer becomes a fixture?
10. **A Roku.** Everything here is compile, package and logic tests. The first sideload will find
    what no test here can: focus feel, text metrics, the Video node. Who sideloads, and when?

## Where this stood on 3 Oct (end of the night)

- **Phases 0–9 are built and committed** on `roku-port`: `ca99ca7`, `3ac5172`, `f6fc0e3`, `ae0a047`,
  `18c56ca`, `0abc1d1`, `f00cd1d`, `8eefa5b`, `8e03328` and `390795c`. The branch is local; nothing
  was pushed.
- **Nothing outside `apps/roku` was changed.** The permission to touch the server or Hosted was not
  used. This log is the only file written elsewhere, and it is uncommitted.
- **What the development server saw from this work:** the fixture script signed in with the
  development account, read documents, published a Roku client profile and capability profile for
  that device, started one paused playback session and one queue, and stopped and deleted both. No
  library, setting or account was changed.
- **The evidence boundary:** `npm run check` (compile and the literal lint), `npm run package` (the
  zip) and `npm test` (logic in the off-device interpreter, most of it against the real
  TypeScript over the development server's own answers). Layout, focus, motion, text measurement
  and playback have not run anywhere.
- **Largest gaps, in the order I would take them:** a sideload pass; the devices list and Switch
  server; the guide's time grid and programme sheet; the player's queue list, chapters and version
  choice; playlist and collection management.
