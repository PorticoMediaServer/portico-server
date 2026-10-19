# Adversarial review: RTL investigation report — 2026-10-18

Reviewed: `/home/justin/projects/Portico/apps/portico-server/.scratch/reviews/2026-10-18-rtl-investigation.md`
Against: the three read-only worktrees of `feature/i18n-languages-20261018` (server/native/roku) and `portico-internal/Product Contracts/accessibility-and-language/contract.md`. Read-only; nothing edited. No sub-agents. All counts re-run with ripgrep/node over the same trees (node_modules excluded).

## Verdict

**Substantially accurate and safe to act on.** The report's structure, contract citations (A11Y-15 items 1–8 map correctly to contract lines 198–205), catalogue analysis, and ~90% of its counts and file:line examples check out. Its phased plan and effort sizing are reasonable. The platform claims are mostly sound; one (Hermes) is overstated in probability but the recommended device assertion is still right. The Hebrew-first pilot recommendation is defensible and its plural-category claim is **verified** (ICU 78.3: `one/two/other`). I found **two refuted claims, several minor count/path inaccuracies, and six genuinely missed issues** — none of which overturn the report's conclusions, but two of which (letter-spacing breadth, Roku `Language: "eng"` hardcode) should be folded into the plan before it's costed.

---

## 1. Claim-by-claim verification

### Confirmed (with evidence)

| Claim | Evidence |
|---|---|
| Web `margin-left/right`: 20 lines, 16 files | `rg -g '*.css' 'margin-(left|right)' web/src packages/design` → 20 lines, 16 files |
| Web `padding-left/right`: 13 | same pattern → 13 |
| Web `border-*`/radius row: 7 | → 7 |
| Web `text-align: left/right`: 24 | → 24 |
| Web `scaleX(-1)`/`row-reverse`: 0; `rotate(180deg)`: 1 | → 0; 1 (`web/src/player/Player.module.css:110`, vertical chevron open-state — matches "not directional") |
| `margin-left: auto` push-aparts | `Shell.module.css:71 .railCount`, `Overlay.module.css:27 .menuTrailing`, `PlayHistory.module.css:3 .total` — all `margin-left: auto` |
| `.side` sheet docked `right: 0` + `slideIn translateX(24px)` | `Overlay.module.css:39-40` |
| `.switchThumb` `translateX(1px → 19px)` | `Field.module.css:27-28` |
| `<html lang="en">` hardcoded; i18n.ts never touches `documentElement`/`dir` | `web/index.html:2`; `rg 'documentElement|dir|lang' web/src/app/i18n.ts` → only comments |
| Zero `<bdi>/<bdo>/dir="auto"/unicode-bidi`; only `isolation: isolate` is stacking in `Artwork.module.css` | 0 matches; 1 stacking match |
| `Shelf.tsx` scrollLeft edge detection + `scrollBy({left: dir * …})` | `Shelf.tsx:23,66` |
| Icons exist centrally in `packages/design/src/icons.ts` | `back/forward/back10/forward10/arrowLeft/arrowRight/next/previous` all present with semantic `meaning` strings |
| Chevron/Arrow usages: 11 in web/design | `rg 'ChevronLeft|ChevronRight|ArrowLeft|ArrowRight'` → 11 |
| back/forward icon sites ~15 | 16 `icon="back|forward|back10|forward10"` + 3 `name=` sites (Shell collapse toggle, Shelf, Page) |
| Player keyboard semantic seek | `Player.tsx:355-356` `ArrowLeft/j` → `seekBy(-skipBack)`, `ArrowRight/l` → `seekBy(skipForward)`; `next/previous` icons at `Player.tsx:820/824` |
| Shelf arrow-key paging `ArrowRight = +1` | `Shelf.tsx:58` |
| `Timeline.tsx` = 126 lines | `wc -l` → 126 |
| Native: `I18nManager` only for locale detection; no `forceRTL`/`allowRTL` anywhere | 2 occurrences in `apps/native/src/app/i18n.ts` (import + `getConstants().localeIdentifier`, line 26); 0 force/allowRTL in `apps/native`, `apps/vega`, `packages/` |
| Native: 39 margin/padding Left/Right; 168 `flexDirection: 'row'`; 0 `textAlign` left/right; 0 `writingDirection` | exact matches 39, 168, 0, 0 |
| Native examples | `ui/Shelf.tsx:43` `marginRight: gap` + `:88` `paddingRight`; `ui/ListRow.tsx:207` separator `marginLeft` computed from leading-box geometry; `GuideGrid.tsx:263/352` left-positioned channel column/header + `:323` `marginLeft` tick labels; `ui/Sheet.tsx:212/257` `±space.sm` nudges; `player/Timeline.tsx:65-86` absolute-`left` knob/marks/preview |
| GuideGrid D-pad is semantic time (survives mirroring) | `GuideGrid.tsx:174-176` left/right tap → `moveTime(∓1)`, long-press → ±2·`HOUR_MS` |
| `Focusable.tsx` Menu-key handling direction-free | `Focusable.tsx:42-52` |
| Vega `snap.tsx` shim reimplements `snapToAlignment="item"` | `apps/vega/src/shims/snap.tsx:7-11`; comment in `shims/react-native.ts:19-20` |
| Roku: `Layout.bs` physical `StackColumn`/`StackRow` (`end = left + width − size`) | `source/ui/Layout.bs` |
| Roku: `Focus.bs` geometric left/right resolution | `source/ui/Focus.bs:18-19` (`forward = direction = "right" or "down"`) |
| Roku: `AuthFrame.bs:52` `align = "left"`; `PlayerHost.bs:1232` `m.clockRight.align = "right"`; timeline left/right → `PlayerSeekBy(∓Skip)` at `1794-1811` | verified |
| Roku fonts = Manrope 400/500/600/700 only | `fonts/` listing; `generate.mjs:406-407` copies exactly those four from native assets |
| Roku locales = `en_AU/en_CA/en_GB/en_US` only | `locale/` listing |
| `LanguageTables.bs` maps `ara/heb/hin/tha/fas/urd` with native names | `source/core/LanguageTables.bs:5,13` (العربية, עברית, हिन्दी, ไทย, فارسی, اردو) |
| `Theme.bs` builds one Font per role from a single Manrope family | `Theme.bs:143-233` (every role = `pkg:/fonts/manrope-{weight}.ttf`) |
| `Text.bs` `UCase` for uppercase roles | `components/ui/Text.bs:51` |
| Catalogue key counts: en-US.ts 1,977; web.ts 2,151; channels 98; preferences 123; owner-console 209; connection-trust 13; secure-connection 42; channel-builder 91; owner-parity 34; subtitles 21 | exact per-file `key: value` line counts (settings.ts differs, see §2) |
| **137 plural messages, 36 select messages, 0 gendered selects** | distinct-key count over `catalog/en-US*.ts`: plurals 137, selects 36, `select, (male|female|he|she|gender)` = 0 |
| `requiredCategories()` enumerates 0–1000 via `Intl.PluralRules`, seeds `other` | `scripts/translations-lib.mjs:47-55` |
| `messageformat.ts` silent English fallback `return n === 1 ? 'one' : 'other'` when PluralRules absent | `packages/i18n/src/messageformat.ts:120-129` (exact quote) |
| `translatedLocales` has no ar/he/fa/ur; th/hi registered-but-empty | `packages/i18n/src/catalog/index.ts:21` — 29 locales ending `…, 'th', 'hi']`; `translated/th.ts` and `hi.ts` are 0-key files; fr/es/de/ru = 5,357 keys each |
| Hebrew plural = 3 categories on ICU 78 | **verified on this machine (node v24.21.0, ICU 78.3)**: he → `one/two/other` (n=10,20,100 → `other`; no `many`). ar → six, fa/ur/hi → two, th → one, ru → four |
| "Four live languages are all still `machine`" | `TRANSLATING.md:31-34` ("All four languages … started as machine translations on 18 October 2026; none has been reviewed yet") |
| XLIFF 1.2 round-trip + state mapping exists | `scripts/translations.mjs:5,30,49-52` |
| Web subtitle fallback fonts: Noto Arabic/Hebrew/Thai/Devanagari/Bengali/Tamil, fetched per present script | `third_party/subtitle-fonts/fonts/` (6 TTF files); `fallback-fonts.ts:17-22` ranges incl. Arabic presentation forms |
| Android mpv font code detects Arabic/Hebrew/HAN | `apps/native/android/.../PorticoMpvDevice.kt` — `Script("Arabic", [NotoNaskhArabic, NotoSansArabic, DroidNaskh, DroidKufi])`, `Script("Hebrew", …)`, language map ar/ara/fa/fas/per/ur/urd/ps/ug/ckb → Arabic, he/heb/iw/yi → Hebrew, plus HAN/HIRAGANA/KATAKANA/BOPOMOFO EAST_ASIAN set. Tested by `PorticoMpvFontsTest.kt` |
| `subtitle-surface.ts` default `align = 'center'` | `web/src/player/subtitle-surface.ts` (`n = 'center'`) |
| `base.css` `font-synthesis: none` | `web/src/ui/base.css:96` |
| Portico Sans is a Latin subset with limited `unicode-range` | `base.css` `@font-face unicode-range: U+0000-00FF, U+0131, … U+FFFD` |
| sorttext `Fold`: case-fold + NFD + ø/ł/đ/ð/æ/œ + strip `Mn` + NFC; no alef/kaf/yeh handling | `server/internal/sorttext/sort.go:22-49` |
| Article stripping only en/fr/es/de/it/pt/nl | `sort.go:12-20` |
| facettext: `IndexedSearchPrefixRunes = 12`, `NumericSearchPrefixRunes = 6`; CJK unigram/bigram in `SearchDocument` | `server/internal/facettext/prefix.go:11-12`, `search.go:28-49` |
| `mini-guide.ts` `localeCompare(…, undefined, {numeric: true})` | `packages/client-core/src/presentation/mini-guide.ts:54` |
| `SettingsRows.tsx` sorts language names by `localeCompare`; `ChannelBuilder.tsx` sorts with `i18n.locale` | `SettingsRows.tsx:85`, `server/ChannelBuilder.tsx` |
| client-core licence inventory hardcodes `'en'` | `packages/client-core/src/licences/inventory.ts:73-76,112,170,218` |
| `formatClock` locale-neutral technical token | `packages/client-core/src/presentation/content.ts` (padStart arithmetic, no locale) |
| Roku `RegionChoice` plumbing exists | `components/shell/Shell.bs:78`, `source/core/Format.bs:10` |
| Hermes on both platforms (RN 0.83 tree) | Android `gradle.properties: hermesEnabled=true`; iOS `Podfile.lock` `hermes-engine (0.14.1)`; native `react-native-tvos@0.83.10-1`; vega `react-native 0.83.0` + `@amazon-devices/react-native-kepler ~4.0.0` |
| Native locale switch is live (`useI18n` + `setServiceI18n`) | `apps/native/src/app/i18n.ts` `useEffect` calls `setActiveI18n` + `setServiceI18n` |

### Refuted

1. **"logical (`margin-inline`, `padding-inline`, `inset-inline`, …) — 9 occurrences"** — Actual count is **3** (all lines): `Shelf.module.css:8` (`scroll-padding-inline`), `TitleHero.module.css:8,10` (`margin-inline: auto`). The file list is right (only those two files); the count of 9 cannot be reproduced by any reasonable pattern (`-inline`/`-block` property names, occurrence or line count). Physical:logical ratio is ~144:3, even more lopsided than the report's 146:9.

2. **"`letter-spacing` used for activation-code/mono styles only"** — False. `letter-spacing` appears on the main typographic scale: `Text.module.css:1-10` (`display/title/heading/subheading/micro/label` at −0.03em…+0.06em), `Page.module.css:5,7,13` (eyebrow/title/sectionTitle), `TitleHero.module.css:15`, `Shelf.module.css:4`, `Feedback.module.css:29,58`, plus the mono/code cases in `Field.module.css:20-21`. Consequence: **negative/positive tracking would be applied to Arabic text on display/title/heading/label roles, which breaks letter joining.** This is a bigger, not smaller, Arabic-script risk than the report states — the lint guard must exempt *all* typographic roles for `ar/fa/ur` (and the Roku `UCase` eyebrow issue the report did note is the same class of problem on TV).

### Minor inaccuracies (counts/paths, no conclusion changes)

- settings.ts = **634** keys, not 626 (my line-count method; report's method unknown). Total is then **~5,393**, not ~5,385 (trivially within the "~").
- Bare `left:`/`right:` positioning in web CSS = **44 + 36 = 80 lines** (pattern `(^|[^a-zA-Z-])left\s*:` to exclude `margin-left:` etc.), plus **20 `left:` inline-style matches in TSX**. Report's 82 ≈ the CSS figure; fine either way.
- padding-left/right file set is **not** "(same set)" as margin: 9 files (ListRow, Field, MetadataEditor, PlayHistory, SelectionBar, EntryListRow, Auth, Show, Channels) vs margin's 16.
- Native absolute positioning: numeric-value style props = **57** (report 59); including variable-valued positioning (`left: side`, `left: brand.x`, Timeline `Math.max(...)`) the true set is larger (~90 `{ ,}left/right:` style-property matches). The ~59 figure is a fair lower bound.
- `textAlign: 'center'` = **3** matches (2 × `VideoPlayer.tsx:441-442`, 1 × `apps/vega/src/bridge/playback.ts:654`), not 2.
- `I18nManager` "appears once" → twice (import + `getConstants()` call), same file.
- GuideGrid tap left/right is **cell-stepping**, not "±1 h": taps move focus to the adjacent program cell; the ±2h step applies to long-press and to the no-cells-loaded path (`GuideGrid.tsx:129-146,174-176`).
- `cast.tsx` is `web/src/app/cast.tsx`, not `player/cast.tsx` (appendix).
- `subtitle-surface.ts` is `web/src/player/subtitle-surface.ts`, not `web/src/player/subtitles/subtitle-surface.ts` (appendix lists it inside `subtitles/`).
- `PorticoMpvFonts` is the *test* class; the implementation is `PorticoMpvDevice.kt` (same directory).

### Unverifiable from source (correctly flagged as such by the report)

- Roku firmware Arabic/Hebrew/Thai shaping, font fallback, and Video-node caption rendering — no source can answer this; hardware test is the right gate.
- Vega (Kepler RN fork 0.83) `I18nManager`/RTL behavior on Fire TV hardware.
- Android TV/tvOS D-pad spatial traversal under mirrored layouts — plausible (both focus engines are view-frame based, and `TVFocusGuideView` is used, e.g. `apps/native/src/ui/Sheet.tsx`), but device verification is genuinely required, as the report says.
- iOS sidecar/app-drawn cue shaping (`engine-media.ts`) and Vega's own app-drawn cue path — device check needed.

---

## 2. Corrected counts (as measured)

| Metric | Report | Measured |
|---|---|---|
| Web logical-property lines | 9 | **3** (Shelf 1, TitleHero 2) |
| Web bare `left:`/`right:` CSS | 82 | 80 (44+36); +20 `left:` inline in TSX |
| Web physical total (margin 20 + padding 13 + border 7 + bare 80 + text-align 24) | ~146 | ~144 |
| Plural messages | 137 | 137 (distinct keys) ✓ |
| Select messages | 36 | 36 ✓ (nested/duplicated openers inflate naive counts: 144/50) |
| Gendered selects | 0 | 0 ✓ |
| settings.ts keys | 626 | 634 (total ≈5,393) |
| Native margin/padding L/R | 39 | 39 ✓ |
| Native absolute L/R (numeric) | 59 | 57 (≥90 incl. variable-valued) |
| `flexDirection: 'row'` | 168 | 168 ✓ |
| `textAlign` left/right / center | 0 / 2 | 0 / 3 |
| `writingDirection` | 0 | 0 ✓ |
| `alignSelf: 'flex-start'` | "several" | 13 |
| Hebrew plural categories | one/two/other | one/two/other ✓ (ICU 78.3) |
| ar/fa/ur/hi/th categories | 6/2/2/2/1 | 6/2/2/2/1 ✓ |

---

## 3. Challenges to platform claims

- **RN `flexDirection: 'row'` auto-flip** — Correct. Yoga treats `row`/`flex-start` as direction-relative; the 168 rows are largely free, and `alignSelf: 'flex-start'` flips too. The report's caveat about absolutely-positioned/physical-margin exceptions is exactly right (verified instances in Shelf/ListRow/GuideGrid/Sheet).
- **`forceRTL` needs an app restart** — Correct per RN semantics (forceRTL alone doesn't re-layout the mounted tree reliably); and the codebase genuinely has no restart-to-apply flow, so the Phase 3 item is real.
- **Hermes Intl (report's §2.4 "if the shipped Hermes lacks PluralRules")** — **Overstated.** This tree is RN 0.83 / `react-native-tvos@0.83.10-1` / Vega RN 0.83, Hermes 0.14.1 on both platforms (Android `hermesEnabled=true`, iOS Podfile.lock). Hermes has shipped `Intl.PluralRules` for many releases; on Android it backs onto the platform ICU, so Arabic plural data is present. The residual unknowns are (a) iOS Hermes' non-ICU Intl data coverage for `ar` locale specifics (categories are CLDR-static and cheap, but `DateTimeFormat` Arabic-Indic digits/calendar are data-dependent) and (b) Vega/Kepler's fork. The Phase 0 device assertion is still cheap and worth keeping, but it should be framed as "verify `PluralRules('ar')` = six categories *and* `NumberFormat('ar-EG')`/`DateTimeFormat` digit+calendar output", not "PluralRules may be absent". Note the code's fallback (`pluralCache` null on any `Intl` absence) also degrades `DateTimeFormat`/`ListFormat`-dependent formatting if `Intl` itself is missing — same assertion covers it.
- **Roku SceneGraph RTL** — Correct as stated: SceneGraph has no direction layer; `Layout.bs`/`Focus.bs` are physical; `Label.horizAlign` has no `natural`/bidi mode to set. Also note SceneGraph's known lack of complex-script shaping means even static Arabic labels are a firmware question — the report's "verify on hardware" gate is the right framing, and the "Roku as subtitles/audio-only" scope cut (A11Y-14.5 allows limited locale sets) is a legitimate plan B. One correction: the "row chevrons use the `forward` glyph (`source/Theme.bs`, row/menu specs)" claim is confirmed (`Theme.bs:599,783`).
- **Pilot recommendation (Hebrew first)** — **Defensible; verified rationale, with two caveats.** (1) The 3-category claim is *true* on ICU 78 (checked: he 10→other), but Hebrew's `two` category exists precisely because dual/time-unit forms (שעתיים, יומיים) still inflect — the "no dual" line is optimistic; budget native-speaker review of `{count}` strings regardless. (2) The engineering argument is the strong one (identical mirroring/bidi pipeline), and the tooling auto-enforces Arabic's six categories once registered, so the marginal cost of *Arabic-first* is mostly translator/gender-review time, not code. Arabic-first would serve a far larger audience; Hebrew-first is the lower-risk way to shake down the pipeline. Either is defensible — the report's choice is fine, but the decision is a market call, not a technical one, and the report slightly oversells "minimal morphology cost". Persian/Urdu third is right (no incremental shaping cost vs Arabic).

---

## 4. Missed issues (add to the plan)

1. **Roku caption tracks are hardcoded `Language: "eng"`** — `PlayerHost.bs:535-536`: every subtitle track pushed to the Video node uses `{TrackName: …, Language: "eng", Description: trackId}`. The in-channel picker labels come from the server presentation (fine), but the device caption metadata lies, and device-level caption behavior (styling/rendering paths) keys off `Language`. This directly undercuts the "Roku subtitles-only fallback (S)" estimate: an Arabic/Hebrew subtitle track would be tagged English on-device. Small fix, but it belongs in Phase 4/fallback scope and is also a pre-existing bug for fr/es/de today.
2. **Bidi isolation is needed for *all* locales, not just RTL ones** — A11Y-15.4–5 requires first-strong isolation of unknown-direction user/provider content unconditionally. Arabic/Hebrew titles appear in English UIs for every user (and vice versa); the report ties the `<bdi>`/FSI pass to the RTL web phase and phrases it as an RTL-locale concern. The isolation pass should be scheduled as locale-independent web/native work (it also fixes the existing mixed-title case in LTR locales). The contract evidence item ("mixed LTR/RTL content review per supported RTL locale") covers this only for RTL locales.
3. **`letter-spacing` is applied by typographic role (see Refuted #2)** — Arabic joining breaks under the display/title/heading/label tracking. The Phase 2 lint guard must target role classes, not just mono/activation styles. Same class of issue as the Roku `UCase` eyebrows the report did catch.
4. **Vega has its own app-drawn subtitle cue path** — `apps/vega/src/bridge/playback.ts:654` renders cue text into React `Text` elements (fire-TV sidecar). The report covers iOS sidecar (`engine-media.ts`) but not Vega's; Arabic joining + cue alignment there is a third app-drawn shaping surface to verify in Phase 0, alongside iOS and Roku.
5. **Arabic locale calendar defaults** — `Intl.DateTimeFormat` with `ar-SA` defaults to the Islamic (Umm al-Qura) calendar; `ar-EG`/most others default to gregory. Since the product pins `locale`/`hour12` explicitly, it should also pin `calendar: 'gregory'` (or consciously accept locale defaults) when registering `ar-*`. One-line Phase 1 decision, absent from the report.
6. **Thai query side** — the report's Thai segmentation note covers indexing; queries tokenize identically (`SearchTerms` whitespace-splits, no Thai case in `search.go`), so a spaced multi-word Thai query yields tokens the index doesn't contain (index stores the whole title as one token). Segmentation work must cover both sides. Minor extension of an existing item.
7. *(Nice-to-have)* **No RTL pseudo-locale** — `pseudo.ts` gives accented English (`en-XA`) for truncation QA; an RTL pseudo-locale (à la `ar-XB`) would let the Phase 6 truncation/mirroring gates run before real translations land. Not in the report.

Not missed (verified absent): the server sends no email (no SMTP/mail templates; invitations are link/code-based), so the contract's Email row has no RTL surface today; `dir=` appears nowhere in web/src; no `intl` polyfills are pinned in native/vega.

---

## 5. Corrected estimates

The report's sizing survives contact, with these adjustments:

| Area | Report | Review |
|---|---|---|
| Web layout + icons + bidi | M | **M** (physical sites ≈144 not 146; bidi pass must be locale-independent — slightly broader, still M) |
| Native RTL | M | **M** |
| Media-player direction islands | S | S |
| Roku RTL UI | XL | **XL** (plus the `Language:"eng"` caption fix wherever Roku subtitles ship) |
| Roku subtitles-only fallback | S | **S+** (track metadata fix; hardware caption test still gates it) |
| Fonts (web/native UI) | S | S (add: letter-spacing guard across typographic roles) |
| Search/sort | S–M | S–M (add ar-SA-calendar note under catalogue; Thai both query+index) |
| Hermes gate | S (Phase 0) | S, reframed (digits/calendar on device, not PluralRules presence) |
| New: calendar pinning, RTL pseudo-locale | — | S each |

Phase structure, risk list, and the Roku scope-cut decision point all stand as written.

## 6. Bottom line

Ship this report with the two refutations fixed (logical-property count 9→3; letter-spacing inventory), the ±1h/cast.tsx/subtitle-surface/PorticoMpvFonts/padding-"same set" nits corrected, and the six missed items added (§4). The counts that matter most — catalogue scale, plural/select/gendered inventory, translatedLocales gap, physical-vs-logical dominance, Roku physicality, font/subtitle readiness, search folding gaps — are all accurate, and the Hebrew-first pilot with Arabic fast-follow remains a sound recommendation.
