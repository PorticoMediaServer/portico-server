# cp-polish — quality, accessibility, strings, infra

Worktrees: `server-cp-polish` and `native-cp-polish`, branch `audit-implementation/cp-polish` in both. Nothing pushed. Commits are listed at the end.

## 1. Outcome

Most of the lane is done in source and covered by tests. Nothing was looked at in a real browser, on a device or by a screen reader.

- **Very long web lists.** Scrolling to either end of a compressed list (millions of titles) now arrives smoothly instead of jumping. Grid cells no longer redraw every time the window moves.
- **Plain HTTP with several tabs open.** The tabs share one event stream, kept by a lease in local storage, instead of each holding its own long poll.
- **Newer server, older app.** Subtitle, lyric-candidate and player-stream lists now drop only the row the app can't read. Owner Activity shows an unknown playback state as "unknown" instead of hiding the viewer.
- **Old browsers.** The browser floor is declared: Chrome/Edge 111, Firefox 113, Safari 16.4. The build targets it, and an older browser now gets a plain, translated message instead of a blank page.
- **Screen readers.** Page announcements now handle quick navigation and two titles with the same name.
- **Accessibility in the web primitives.** Text sizes are in rem, so text follows the browser's font size. Focus, selection, switches and progress stay visible in forced-colours (high-contrast) mode.
- **React hook rules are enforced.**
  - Web: oxlint (dev only), wired into `npm run check`.
  - Apps and Vega: the eslint plugin they already had.
  - Every finding was fixed (26 on web, 10 on native and Vega), including one real conditional-hook bug in the web Home rows.
- **Infrastructure.**
  - One pinned Node release (24.21.0) for server CI, releases, security jobs and the container image, with a check that they agree.
  - Native CI checks out portico-server at the exact commit the shared packages came from, instead of a moving branch.
  - Vega's type checks, tests and string lint now run in CI.
  - New native release workflow: builds Android and Apple and drafts a GitHub release.
  - The source-map-js advisory is cleared (1.2.2).
- **Strings.** Several groups of English text moved into the catalogue:
  - the metadata editor's fallback field names;
  - the shared queue player's and subtitle service's messages;
  - native owner job, job-state and alert names (the English tables are deleted);
  - Vega's connection notices.
- **304 responses.** Typed API routes can now answer "not modified" before building the response, and revision validators are compared before the body is encoded.
- **Optional generated contracts.** All 12 declarations are retired, along with the dead client code behind them.
- **Native lists.** Cell renderers on the Person and Search grids are now stable.
- **Home row See all.** A quiet refresh re-reads every page already scrolled into (up to 5), so a title removed deep in the list disappears.

## 2. Decisions I made

1. **Hook lint on the web uses oxlint (dev dependency, exact 1.86.0).** typescript-eslint does not support the TypeScript 7 toolchain the web uses; oxlint parses TSX itself and is one dev-only package that never ships. The apps keep their existing `eslint-plugin-react-hooks`, with a hooks-only config. Deliberate exceptions keep an `eslint-disable-line … -- reason` comment.
2. **The browser floor is Chrome/Edge 111, Firefox 113, Safari/iOS 16.4.** This is what the CSS already needs (`color-mix` on almost every surface). The check script runs as a separate classic file (`/browser-check.js`), because the page's security policy forbids inline scripts. Its wording comes from the catalogue when the app is built.
3. **Plain-HTTP tab sharing uses a lease in local storage.** Timings: 8 s lease, renewed every 2 s, read back 120 ms after writing. Web Locks still serve secure pages. A tab that loses its lease stops its stream; the next tab takes over from the last event.
4. **Compressed scrolling.** Small moves stay 1:1. How far the list may drift from the proportional position is limited by how much scroll is left, so the list closes on each end at no more than twice the proportional speed.
5. **Identical headings.** Title heroes and page headers carry the *loaded* item's id (`data-page-key`). Using the route parameter would have announced the old title while the new one loads. A stale heading also stops counting as stale as soon as anything else has been on screen.
6. **Neutral values for B-16.10.**
   - Activity: unknown state or delivery reads as `unknown`.
   - Remote-source preview: unknown capacity mode reads as `defaulted` (no screen displays it).
   - The *selected* subtitle track stays strict. Post-play, the owner playback observation and listening preferences stay strict too, because they carry cross-checks or are sent back whole.
7. **Optional contracts retired, not implemented.**
   - The 9 Hosted profile routes are served by nothing. Every app manages profiles through the local server (`authority: 'local'`).
   - Remote commands were replaced by `/v1/playback/controls`. `DeviceCommandsClient` and `CommandRouter` had no callers.
   - Following the pre-release rule, I deleted the generator, the contract folder, `playback-v1/commands.ts` and the hosted branches of `ManagedProfilesClient`. Its unused `policies()` and `deletionStatus()` went too.
8. **Native release workflow.**
   - Store signing, notarization and the Vega package stay manual (credentials and the Vega SDK). The workflow says so and drafts the release rather than publishing it.
   - Android release builds still use the debug key, as `build.gradle` already states.
9. **Deep-list refresh is bounded at 5 pages (300 titles).** Past that, a removed title further down stays until the page is reopened, as before.

## 3. Audit bugs and decision gaps closed

| ID | What changed | Test |
|---|---|---|
| B-16.4 | `web/src/ui/compressed-scroll.ts`: continuous mapping, no snap at the ends | `web/test/compressed-scroll.test.ts` |
| B-16.5 | Stable renderers: EngineResults, TitlesTable (TitleRow takes the index, not a fresh closure), Show episodes, Person credits, saved rows. `siblings` read through a ref. Native Person and Search too | type check, web suite |
| B-16.7 | `web/src/app/tab-lease.ts`, used by `event-feed.ts` on insecure pages. The lock contract in client-core can now be lost while held (`held` signal → `event_stream_lock`) | `web/test/tab-lease.test.ts`, `client-core/test/event-feed.test.ts` (lost-lease case) |
| B-16.10 | `keepReadable` for subtitle plan/catalogue rows, lyric candidates and offer streams. Activity state/delivery and capacity mode read as neutral. (`guide/reads.ts` has no parser; the earlier note about it was mistaken) | `activity`, `subtitles`, `player-offers` tests |
| B-16.12 | `browserFloor` build/CSS target; `browser-check.js`; README section; catalogue strings | `web/test/browser-floor.test.ts`, vite build |
| B-16.14 | `onBeforeNavigate` captures the outgoing heading; `data-page-key`; stale-heading release | `web/test/page-title.test.ts` (quick-navigation and same-name cases) |
| B-16.17 | Token generator emits rem; all `font-size`/`line-height` px in web CSS converted (237 declarations); forced-colours blocks in Artwork, Card, Feedback, Field, Tabs, Overlay, TitleHero, Button, PasswordStrength and Search | `web/test/type-scale.test.ts` |
| B-16.23 | Web `lint:hooks` (oxlint) in `check`; native `lint:hooks` plus `.eslintrc` errors; all findings fixed | `lint:hooks` clean in both repos |
| B-16.25 | `.node-version` 24.21.0; workflows use `node-version-file`; Dockerfile tag names 24.21.0 (same digest as before); `scripts/check-node-version.mjs` runs in CI | script passes |
| B-25.21 | `packages/portico-server.lock.json` written by `shared-packages.mjs --write` (refuses uncommitted server packages); `--revision`; CI checks out that commit and `--check` fails in CI if it differs; native `release.yml` | `--check` passes against the recorded commit |
| B-15.19 | source-map-js 1.2.1 → 1.2.2 (fix for GHSA-68fv-2mgg-jv7q) in the server lock. Native locks don't contain it | `npm audit` showed the fix available; lock updated |
| B-19.15 | Vega CI job (typecheck, test typecheck, tests, strings). The React-internals hook driver is **not** replaced; see section 9 | — |
| B-18.22 | Metadata editor fallback labels → `web.metadata.field.*`; queue player → `queuePlayer.*`; SubtitleService → `subtitles.error.*`; native Dashboard, TV Dashboard and Schedules → `jobName`/`jobStateName`/`alertName`; English tables deleted from `console-words.ts`; Vega notices → `vega.connection.*`; AccountForm titles moved to existing `auth.*` keys. A scan found no `t('…')` id missing from the catalogue across web, client-core, native and Vega | type checks, string lints, suites |
| B-08.12 | `apikit.Conditional(ctx, tag)`; a named ETag is compared before `marshalResponse`. The asset-cache residual was already fixed (`installation.go` makes every hashed asset immutable) | `apikit/registry_reply_test.go` (two new tests) |
| Optional consumer declarations | All 12 retired (see Decisions 7) | `generated-routes`, `union-contracts` tests adjusted |
| Native compressed scrolling, memoised cells, deep-list refresh | Compression does not apply to native: VirtualizedList has no height cap like the browser's. Memoised cells done for Person and Search (Library, Home and Saved were already stable). Deep-list refresh done for the Home row See all page | `recommendation-rows.test.ts` (deep refresh) |

## 4. API changes

- New `apikit.Conditional(ctx, tag) bool` for typed GET handlers. Registry GETs whose handler names an ETag skip encoding on a match. Wire behaviour is unchanged: same 304, ETag and `Cache-Control`.
- Removed `packages/contracts/src/optional` (12 client-only declarations that had no handler) and `server/cmd/clientcontractgen`. The production OpenAPI document is unchanged; `apigen` produced no diff.
- Client-core:
  - `SharedStreamEnvironment.lock`'s work receives an optional `held` signal.
  - `ManagedProfilesClient` is local-only.
  - `playback-v1` no longer exports the command and transfer client.
  - `ActivityService` state/delivery types gain `'unknown'`.

## 5. Schema changes

None.

## 6. Settings added

None.

## 7. Tests run (final state)

- `web`: `npm test` → 560/560. `tsc --noEmit` clean. `lint:design --strict`, `lint:strings` and preference consumers clean. oxlint hooks clean. `vite build` OK.
- `npm run test:cast` → 20/20. `packages/contracts` tests → 30/30. `npm run build --prefix packages/contracts` OK.
- `packages/client-core`: `npm test` → 1947/1947 (four tests retired with the dormant protocols; new ones added).
- `apikit`: `go test ./...` → ok.
- `server`: `go test ./internal/httpapi/` → two failures, neither from this lane:
  - `TestListeningHTTPPreferenceEnvelopeAndCAS` needs the reviewed helper binary (`PORTICO_PLAYBACK_TEST_HELPER`), which CI builds.
  - `TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot` is timing-sensitive under load and passed 3/3 when run alone.
- `server`: `go run ./cmd/apigen` → no diff. `scripts/check-node-version.mjs` → passes.
- Native: `tsc` (app and tests) clean. `npm test` → 665/665. `lint:hooks` clean. `lint:strings` → 0 hard-coded.
- Vega: `tsc` (app and tests) clean. `npm test` → 87/87. `lint:strings` clean.
- `shared-packages.mjs --check` → identical at the recorded commit `525f23be`.
- The lane's GOCACHE and the scratch oxlint install were deleted.

## 8. Device test notes

- **Plain HTTP, two or more tabs** (`http://<LAN IP>:32500`, same viewer):
  - The server log or devtools should show one `/v1/events` stream and no `waitSeconds` polls from the second tab.
  - Close the holding tab: the other takes over within about a second and misses no update.
  - Leave one tab hidden for 10 minutes, then bring it back.
- **Huge library** (or `/dev` WindowedLab with millions of items): drag the scrollbar near the end, then use the wheel to reach the last row. There should be no jump; Home and End land exactly on the first and last rows.
- **Screen reader** (NVDA/VoiceOver):
  - Click Home → a library → a film within half a second: only the film is announced.
  - Open two shows with the same name one after the other: both are announced.
- **Forced colours** (Windows High Contrast, Edge): focus a poster card, check chips/tabs/segments, switches, checkboxes, watched bars, the menu highlight and the search field focus.
- **Browser font size "Very large"**: text scales; look for clipping in fixed-height rows.
- **Old browser** (Safari 15 or Chrome 100): the plain "too old" message appears, not an empty page.
- **Native owner Dashboard and TV Dashboard**: job, state and alert names read as before (now from the catalogue).
- **CI, once pushed**: native `ci.yml` resolves the server revision from the lock (it must be a pushed commit). `release.yml` is untested. The Android and Apple jobs build the media engine first, which can take hours.

## 9. Not finished

- **The hook lint needs `npm ci` before `npm run check` works.** `oxlint` was added to `web/package.json` and the lock, but not installed into the shared `node_modules` (installing there is outside this lane). `npm run check` fails at `lint:hooks` until someone runs `npm ci`.
- **The lock revision will conflict when lanes merge.** Every lane that mirrors writes its own server commit into `packages/portico-server.lock.json`. Re-run `shared-packages.mjs --write` after the final server merge; that commit must be pushed before native CI can fetch it.
- **B-19.15: the hook test driver still imitates React through its internal dispatcher.** There is no renderer in the native tree, and adding `react-test-renderer` (deprecated) or `react-reconciler` would be a new dependency. It still fails loudly if the seam disappears.
- **Typed routes don't use `Conditional` yet.** The mechanism is in; adopting it needs a cheap, complete per-route revision.
- **Saved lists that have been paged still refresh only their current page.** Deep refresh was done for the Home row See all page only.
- **Native release workflow.** It has never run. It assumes the media-engine build scripts work on GitHub runners; the licences lane is changing their paths. Store signing and the Vega package stay manual.
- **Forced colours and font scaling haven't been looked at.**
  - Inline `fontSize` styles in TSX (a few placeholders and badges) are still in px.
  - Native font scaling was already on, except on TV; I didn't audit fixed-height native rows.
- **Untouched B-16.10 parsers.** Lyric provenance origin, post-play reason and the playback observation lifecycle stay strict on purpose (Decisions 6).

## Commits

**server-cp-polish** (from `f719c2a0`):
`d6950a72` compressed scrolling and stable renderers ·
`b4d3b45e` plain-HTTP shared stream lease ·
`8ea61959` browser floor ·
`277a4a6c` navigation announcements ·
`9e5ecc2e` rem text and forced colours ·
`5751f399` hook lint, its fixes, source-map-js ·
`759d899d` Node pin ·
`7a6c852e` conditional responses ·
`d82e791c` parser salvage ·
`1c135bac` catalogue strings, English console tables retired ·
`4f4e480b` optional contracts retired ·
`725cd158` deep-list refresh ·
`5affcb45` README hook-lint note ·
`525f23be` union decoder tests.

**native-cp-polish** (from `4f0689c`):
`4d82d83` hook lint and fixes ·
`bf7d170` server revision lock and mirror ·
`2c7fe81` Node pin, Vega CI, release workflow ·
`0cb81ea` mirror ·
`c2f739a` catalogue names in owner screens, Vega notices, Library lint fix ·
`c61036d` stable Person and Search renderers ·
`570e482` mirror ·
`1d0cdf5` lock revision `525f23be`.
