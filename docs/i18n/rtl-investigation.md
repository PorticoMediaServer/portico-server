# Right-to-left and complex-script investigation — 2026-10-18

Investigation only. No code changed. Sources are the i18n branch worktrees
(`feature/i18n-languages-20261018`, read-only) and contract
`PC-ACCESSIBILITY-LANGUAGE` §15 (RTL) / §13–14 (locale model).

Worktrees: `server/` = `packages/i18n`, `packages/design`, `web/`;
`native/` = `apps/native`, `apps/vega`; `roku/` = BrightScript channel.
Counts below were produced with ripgrep over those trees (node_modules excluded).

## Executive summary

- **Catalogue and formatting are RTL-ready in design.** The custom ICU
  MessageFormat supports plurals/selects/offsets; `requiredCategories()` in
  `translations-lib.mjs` derives each locale's plural branches from
  `Intl.PluralRules` automatically, so Arabic's six categories would be enforced
  for free. ~5,385 message keys, 137 plurals, 36 selects, zero gendered selects.
  But **no RTL locale is registered anywhere**: `translatedLocales` has no
  `ar`/`he`/`fa`/`ur` (only `th`/`hi` placeholders exist for complex scripts),
  and there is **zero `dir`/`lang` handling on any client**.
- **Web is the cheapest surface (M).** ~146 direction-sensitive style
  occurrences, concentrated and mechanical (`margin-left: auto` push-aparts,
  `text-align: left`, shelf arrows, side sheet). No `<bdi>`/bidi isolation
  anywhere. Icons that must mirror (`back`, `forward`, `next`, `previous`,
  `arrowLeft/Right`, `back10/forward10`) are centralized in
  `packages/design`. Timeline/seek must stay LTR per contract §15.2–3.
- **Native is moderate (M).** No `forceRTL`/`allowRTL` wiring at all (only a
  locale-identifier read). 168 `flexDirection: 'row'` (RN auto-flips under RTL —
  mostly free) vs 39 physical margin/padding + 59 absolute `left/right` (do not
  flip — the real work). TV D-pad handling is semantic and survives mirroring;
  an RTL switch needs an app-restart flow that does not exist today.
- **Roku is the long pole (XL) and may deserve a scope cut.** SceneGraph has no
  RTL layout: custom `StackRow`/`StackColumn` helpers and `Focus.bs` are
  physical left/right throughout; only Manrope (no Arabic/Hebrew/Thai/Devanagari
  glyphs) is bundled; channel ships only `en_*` locales. Subtitle *track*
  tables already map `ara/heb/hin/tha/fas/urd`, but on-device Arabic shaping is
  unverified.
- **Subtitles are in the best shape.** Web libass path already bundles Noto
  Arabic/Hebrew/Thai/Devanagari (+Bengali/Tamil); Android mpv font code already
  detects Arabic/Hebrew scripts. Remaining risk is Roku device caption shaping.
- **Search/sort needs targeted fixes (M).** `sorttext.Fold` strips diacritics
  (good for tashkeel/niqqud) but does not normalize alef/kaf/yeh variants; no
  Arabic article (`ال`) handling; Thai (no word spaces) defeats the
  word-prefix index — needs segmentation work.
- **Recommendation: pilot Hebrew, fast-follow Arabic.** Hebrew proves the full
  RTL pipeline (shaping, bidi, mirroring, TV) with only 3 plural categories
  (verified: `one/two/other` on ICU 78) and no dual; Arabic follows with the same
  pipeline plus its six categories and gender notes.

## 1. Web (`web/`, `packages/design`)

### 1.1 Layout: physical vs logical properties

| Pattern | Occurrences | Files |
|---|---|---|
| `margin-left` / `margin-right` | 20 | 16 files |
| `padding-left` / `padding-right` | 13 | (same set, incl. `Field`, `ListRow`, `EntryListRow`) |
| `border-left/right`, `border-top/bottom-*-radius` | 7 | `Shell`, `Overlay`, `MetadataEditor`, `Channels` |
| bare `left:` / `right:` positioning | 82 | overlays, rails, badges, toasts, sheets |
| `text-align: left/right` | 24 | cards, rows, menus, table cells |
| logical (`margin-inline`, `padding-inline`, `inset-inline`, …) | 9 | only `Shelf.module.css`, `TitleHero.module.css` |
| `scaleX(-1)` / `row-reverse` | 0 | — |
| `rotate(180deg)` | 1 | vertical chevron open-state (not directional) |

Physical properties dominate ~146:9. The good news: the most common pattern is
`margin-left: auto` / `margin-right: auto` push-aparts (e.g.
`Shell.module.css:.railCount`, `Overlay.module.css:.menuTrailing`,
`PlayHistory.module.css:.total`), which convert trivially to
`margin-inline-start: auto`. Files most affected:
`Settings.module.css`, `StillCard.module.css`, `Overlay.module.css`,
`Card.module.css`, `TitleHero.module.css`, `ListRow.module.css`,
`Field.module.css`, `Shell.module.css`, `MetadataEditor.module.css`,
`SelectionBar.module.css`, `Player.module.css`, `Channels.module.css`,
`Server.module.css`, `Auth.module.css`, `Detail.module.css`.

Notable direction-sensitive components beyond box properties:

- `web/src/ui/Shelf.tsx` — horizontal scroller. Edge detection reads
  `el.scrollLeft`; paging does `el.scrollBy({left: dir * …})`. In RTL,
  `scrollLeft` semantics differ per browser (negative vs reverse modes) — needs
  a helper, not a one-line flip. Arrows use `back`/`forward` icons with
  `arrowLeft`/`arrowRight` gradient fades (`Shelf.module.css`).
- `web/src/ui/Overlay.module.css:.side` — side sheet docked `right: 0` with
  `slideIn` from `translateX(24px)`; must dock start-side in RTL.
- `web/src/ui/Field.module.css:.switchThumb` — toggle thumb `translateX(1px →
  19px)`; must run the other way in RTL (or use logical `inset-inline-start`).
- `web/index.html` — `<html lang="en">` hardcoded; nothing ever updates `lang`
  or `dir`. `web/src/app/i18n.ts` (`I18nProvider`, `preloadI18n`) publishes the
  catalogue but never touches `document.documentElement`. This is step 1 of any
  web phase: set `lang` + `dir` from the resolved locale, before first paint.

### 1.2 Icons that must mirror

Centralized glyphs in `packages/design/src/icons.ts` (icon identity is a
contract concept, A11Y-12.4–5, so renames need care):

- MUST mirror in RTL: `back` (›-reversed chevron, used in `Page.tsx`,
  `Settings.tsx`, `Channels.tsx` guide edges), `forward` (row chevrons in
  `Settings.tsx`, guide "later" edge), `arrowLeft`/`arrowRight` (transport map
  in `NowPlaying.tsx`, remote-control seek map in `RemoteControls.tsx`),
  `next`/`previous` (queue controls in `Player.tsx` — but see §1.3),
  `back10`/`forward10` (seek ±10 s badges in `Player.tsx`, `cast.tsx`).
- Must NOT mirror: `play`, `pause`, `volume`, `cast`, brand marks (per contract
  §15.2); vertical `chevronUp/Down/UpDown` (no horizontal meaning).
- Usage count: `ChevronLeft/Right` + `ArrowLeft/Right` appear in 11 web/design
  occurrences (mostly keyboard handling, below); `name="back"|"forward"` icon
  usages are the ~15 sites listed above.

### 1.3 Media player: do NOT mirror the timeline

Contract §15.2–3: media time stays chronological per platform convention;
previous/next/rewind/skip follow *semantic media-time direction*, not mirrored
glyphs. Findings in `web/src/player/Timeline.tsx` (126 lines):

- Bar geometry is physical: played width grows from `left`, knob/marks/ticks at
  `style={{left: pct%}}`, pointer math uses `getBoundingClientRect()` (`at()`
  is direction-agnostic — good). YouTube-style Arabic players keep the timeline
  LTR; recommendation: wrap the timeline widget in `dir="ltr"` explicitly so it
  is immune to the page's `dir`, and keep labels localized.
- Keyboard: `ArrowLeft`/`ArrowRight` hardcode −/+ step, and `Player.tsx` maps
  `ArrowLeft/j` → seek-back, `ArrowRight/l` → seek-forward. With a `dir=ltr`
  island this stays correct. `Shelf.tsx` arrow-key paging (`ArrowRight` = +1)
  is *list* navigation, not media time — that one SHOULD follow visual order
  (contract §15.7: physical movement follows the visual layout).
- Hover-time bubble and trickplay preview are `left`-positioned with clamping
  (`clampLeft`) — fine inside an LTR island.
- Native mirrors this: `apps/native/src/player/Timeline.tsx` positions knob,
  marks and the preview card with absolute `left`; `GuideGrid.tsx` maps TV
  remote left/right to time −/+1 h and long-press to ±2 h (semantic, survives
  mirroring).

### 1.4 Bidi isolation: missing everywhere

- Zero occurrences of `<bdi>`, `<bdo>`, `dir="auto"`, `unicode-bidi` in
  `web/src` + `packages/design` (the single `isolation: isolate` hit is CSS
  stacking isolation in `Artwork.module.css`, unrelated).
- Contract §15.4–5 requires isolation of URLs, file paths, device codes, IPs,
  codecs, timestamps-as-tokens, and first-strong isolation of user/provider
  titles (Latin title inside Arabic UI and vice versa). Every title-bearing
  surface (cards, rows, player, guide cells) needs a `<bdi>`/`dir="auto"` pass.
- Numbers/dates are in good shape: `createI18n` formatters are `Intl`-based
  (`NumberFormat`, `DateTimeFormat`, `PluralRules`, `ListFormat`); Arabic-Indic
  digits and 12/24-hour conventions come free with an `ar-*` format locale.
  `formatClock` (media counter `1:02:03`) is deliberately locale-neutral —
  correct to keep as a technical token.

## 2. Native (`apps/native`, `apps/vega`)

### 2.1 No RTL wiring exists

- `I18nManager` appears once: `apps/native/src/app/i18n.ts` reads
  `I18nManager.getConstants().localeIdentifier` on Android for *locale
  detection only*. No `forceRTL`, no `allowRTL` anywhere in `apps/native`,
  `apps/vega`, or `packages/`.
- Consequences: enabling RTL means calling `allowRTL(true)` + `forceRTL(true)`
  on first selection of an RTL locale — which requires an **app restart** (RN
  limitation). Today locale switches are live (`useI18n` memo + `setServiceI18n`
  propagation); an RTL phase must add a "restart to apply" flow (persisted
  pending-direction flag, restart prompt, cold-start `forceRTL` before first
  render). Same for tvOS/Android TV/Fire TV; Vega reuses the same screens
  through shims (`apps/vega/src/shims/*`), so it inherits whatever the shared
  code does — but Vega SDK RN RTL behavior must be verified on device.

### 2.2 Style inventory

| Pattern | Occurrences |
|---|---|
| `marginLeft/marginRight/paddingLeft/paddingRight` | 39 |
| bare `left:`/`right:` absolute positioning | 59 |
| `flexDirection: 'row'` | 168 |
| `textAlign: 'left'/'right'` | 0 (only `'center'` found, 2×) |
| `writingDirection` | 0 |

- The 168 `flexDirection: 'row'` containers **auto-flip** under RN RTL
  (`row` behaves as `row-reverse`) — layout in lists, rows, bars, sheets is
  largely free. Risk is in the exceptions: anything that positioned children
  absolutely or used physical margins to compensate (e.g.
  `ui/Shelf.tsx` `marginRight` spacer + `paddingRight`, `ui/ListRow.tsx`
  separator `marginLeft` computed from leading-box geometry,
  `screens/GuideGrid.tsx` `left`-positioned header/channel column and
  `marginLeft` tick labels, `ui/Sheet.tsx` `marginLeft: -space.sm` /
  `marginRight: -space.sm` button nudges). Each of the ~98 physical occurrences
  needs a start/end decision; most map to `marginStart/marginEnd/paddingStart/
  paddingEnd`, a few (guide grid ruler, timeline knob) are time-geometry and
  should stay physical.
- `alignSelf: 'flex-start'` (several screens) flips correctly under RTL.
- No `textAlign: left/right` and no `writingDirection` — text alignment
  currently relies on RN defaults, which follow the system RTL automatically.
  Good baseline; add explicit `writingDirection` only where mixed-content
  isolation is needed.

### 2.3 TV focus and gestures (contract §15.7)

- Platform spatial navigation (`TVFocusGuideView`, Android TV / tvOS focus
  engine) follows the *visual* layout, so physical D-pad left/right keeps
  working after mirroring — no remapping wanted. Custom handlers are semantic:
  `GuideGrid.tsx` left/right = time −/+ (media time, correct unchanged);
  `Focusable.tsx` Menu-key handling is direction-free.
- Gestures: no swipe-direction-dependent navigation found in shared screens;
  the web `Shelf` scroll helper is the only scroll-physics site (see §1.1).
  Vega `snap.tsx` shim (row/page follow-focus) should be re-checked on device
  since it reimplements tvOS `snapToAlignment` semantics.

### 2.4 Hermes caveat (verify before estimating)

`packages/i18n/src/messageformat.ts` falls back to **English plural rules when
`Intl.PluralRules` is absent** (`if (!rules) return n === 1 ? 'one' : 'other'`).
Hermes' `Intl` coverage varies by build; if the shipped Hermes lacks
`PluralRules` (or lacks Arabic data), Arabic plurals silently render English
rules with zero errors. Phase 1 must assert `new Intl.PluralRules('ar')`
categories on a real Hermes build (and `DateTimeFormat('ar-EG')` digits) before
translating a single string.

## 3. Roku (`roku/`)

### 3.1 SceneGraph has no RTL layout — everything is physical

- Layout helpers are explicitly physical: `source/ui/Layout.bs`
  `StackColumn`/`StackRow` take `align = "start"|"center"|"end"` where
  start = left (`x = left + …`, `end = left + width − size`). All node
  placement is via `translation` arrays.
- Focus is a custom engine: `source/ui/Focus.bs` resolves `"left"/"right"/
  "up"/"down"` geometrically; `components/shell/Shell.bs`,
  `components/screens/Settings.bs` (`m.left`/`m.right` panes),
  `components/player/PlayerHost.bs` (timeline `direction = "left"/"right"` →
  seek ∓) all assume LTR adjacency. Mirroring Roku means touching every screen
  or adding a direction-aware layout layer — there is no `dir` to set.
- Text alignment is physical literals: `components/screens/auth/AuthFrame.bs`
  `align = "left"`, `PlayerHost.bs` `m.clockRight.align = "right"`.
  (`Label.horizAlign` accepts left/center/right; whether firmware offers a
  `natural`/bidi mode on current models must be verified on hardware.)
- Row chevrons use the `forward` glyph (`source/Theme.bs`, row/menu specs) —
  would need a mirrored asset or runtime flip per row.

### 3.2 Fonts: Manrope-only is a blocker for Roku UI locales

- `roku/fonts/` = Manrope 400/500/600/700 only; `generate.mjs` copies them in.
  Manrope has no Arabic, Hebrew, Thai, or Devanagari glyphs. Unless Roku
  firmware font fallback covers these scripts for custom `Font` URIs (doubtful
  — verify on hardware with a test channel), an RTL Roku UI needs bundled
  script fonts + a `Font` selection path in `source/Theme.bs`/`Text.bs`
  (which currently builds one `Font` per role from a single family).
- Note `components/ui/Text.bs` applies `UCase(value)` for uppercase roles:
  harmless for Arabic/Hebrew/Thai (no case), but any uppercase-styled localized
  string in those locales must be reviewed (e.g. eyebrow labels).

### 3.3 Roku locale plumbing is mechanical

- `locale/` ships `en_US/en_GB/en_CA/en_AU/translations.xml` (Qt TS format),
  generated by `scripts/generate.mjs` from the shared catalogue. Adding a
  locale = add catalogue + regenerate + verify `RegionChoice`/profile-language
  plumbing in `components/shell/Shell.bs` + `source/core/*Messages*`.
- Subtitle *language tables* already cover the scripts:
  `LanguageTables.bs` maps `ara→ar, heb→he, hin→hi, tha→th, fas→fa, urd→ur`
  with native names (`العربية`, `עברית`, `हिन्दी`, `ไทย`, `فارسی`, `اردو`).
  Those native names render in track pickers — on Roku they render in Manrope
  context (see font risk), on web/native in system fonts (fine).

## 4. ICU, catalogue, and translation tooling

- Scale: ~5,385 keys (`en-US.ts` ~1,977; `web.ts` ~2,151; `settings.ts` 626;
  channels 98; preferences 123; owner-console 209; connection-trust 13;
  secure-connection 42; channel-builder 91; owner-parity 34; subtitles 21).
- 137 `{x, plural, …}` messages, 36 `{x, select, …}` messages, **0 gender-based
  selects**. Arabic and Hebrew translators will hit agreement problems the
  source never had (verb/adjective gender, dual). Mitigation exists in tooling:
  a translation may turn a bare number into a plural (`upgrades` rule), but
  gendered sentence structure may force source-string splits — budget reviewer
  time with a native speaker, not just translator time.
- `requiredCategories()` (translations-lib.mjs) enumerates 0–1000 through the
  *build machine's* `Intl.PluralRules`, so required branches per locale are
  automatic. Verified on ICU 78: `ar` → six (zero/one/two/few/many/other),
  `he` → three (one/two/other), `fa`/`ur`/`hi` → two (one/other), `th` → one
  (other). Russian precedent (`one/few/many/other`) already exercises the
  multi-category path in CI (`translations.mjs check`).
- `translatedLocales` map currently lists French/German/Spanish/Russian plus
  registered-but-empty `pt-BR/it/nl/pl/sv/da/nb/cs/tr/uk/zh/ja/ko` and
  *planned* `fr-CA/es-419/pt-PT/el/hu/ro/fi/id/vi/th/hi`. **No `ar/he/fa/ur`
  entries at all** — adding them is a few lines plus XLIFF round-trip, but it
  is also the product decision that starts the clock on translator review
  (`machine → translated → reviewed` states; the four live languages are all
  still `machine`).
- Bidi gaps in tooling (see §1.4): no RLM/LRM/FSI handling in scripts or
  `messageformat.ts`. Recommend convention: wrap interpolated user/provider
  values in U+2066 FSI…U+2069 PDI at format time (or `<bdi>`/`dir="auto"` at
  render), plus a lint rule refusing raw `{url}`/`{path}`/`{code}` adjacent to
  localized text without isolation. Product names (`Portico…`) are already
  protected as untranslatable tokens.

## 5. Fonts and shaping

| Surface | Current font | Arabic/Hebrew/Thai/Devanagari | Shaping |
|---|---|---|---|
| Web UI | Portico Sans (Manrope variable, **Latin subset**, `unicode-range` limited) | falls back to system fonts | HarfBuzz (browser) — fine |
| Web subtitles | libass + bundled Noto Arabic/Hebrew/Thai/Devanagari/Bengali/Tamil (`third_party/subtitle-fonts`), CJK from server font | covered, fetched only when the script is present (`fallback-fonts.ts`) | libass — fine |
| Native UI (iOS/tvOS) | Manrope TTFs; system fallback | system fonts | CoreText — fine (verify) |
| Native UI (Android/TV/Fire/Vega) | Manrope TTFs; system fallback | Noto-family on most Android; verify on lean TV firmware | HarfBuzz — fine (verify) |
| Native subtitles (Android mpv) | `PorticoMpvFonts` already detects ARABIC/HEBREW/HAN (`PorticoMpvFontsTest`) | covered | libass/mpv — fine |
| Native subtitles (iOS sidecar/app-drawn) | `engine-media.ts` `sidecar` path (app draws cues) | glyphs via system fonts; shaping of app-drawn Arabic needs a check | verify |
| Roku UI | Manrope only | **no coverage — blocker** | firmware shaping unverified |

- Web `base.css` sets `font-synthesis: none`: with the Latin-only subset,
  bold Hebrew/Arabic rendered in a fallback face will *not* synthesize bold —
  acceptable, but expect weight contrast to flatten in RTL locales until/unless
  matching Noto weights are added for UI (not just subtitles).
- No `letter-spacing` on CJK-style concerns for these scripts; Arabic script
  must never have letter-spacing applied (breaks joining) — grep shows
  `letter-spacing` used for activation-code/mono styles only, but add a lint
  guard when RTL lands.

## 6. Subtitles

Already the strongest area: track-language mapping covers `ar/he/fa/ur/hi/th`
(Roku `LanguageTables.bs`; shared `language.ts` upstream), libass paths on web
and Android mpv handle shaping, and fallback fonts are bundled with a
present-script-only fetch strategy. Residual work:

1. Web sidecar/VTT path (`subtitle-surface.ts`, `VTTCue`) — browser handles
   bidi; set `vtt.align` appropriately (currently `'center'` — fine).
2. iOS/tvOS app-drawn sidecar cues — verify Arabic joining + cue alignment.
3. Roku `roVideoPlayer` device captions for Arabic/Hebrew — **highest subtitle
   risk**; needs hardware test. If firmware shaping fails, the fallback is
   burn-in (`burn` delivery already exists in `engine-media.ts` vocabulary).

## 7. Search and sort

- `server/internal/sorttext` (`Fold`): Unicode case-fold + NFD + strip `Mn` +
  special-case ø/ł/đ/æ/œ. Effect on target scripts: Arabic tashkeel and Hebrew
  niqqud are mostly `Mn` → stripped (good, matches user expectations);
  **alef variants (أ إ آ → ا), alef-maqsura/ta-marbuta, and Farsi/Urdu
  kaf/yeh (ک ی vs ك ي) are NOT normalized** — searching يوسف won't match
  يوسف with a Persian yeh. Small, well-scoped addition to `Fold` (+ tests).
- Article stripping for sort keys covers `en/fr/es/de/it/pt/nl` only. Arabic
  sort conventionally ignores the `ال` (al-) prefix — add `ar` (+ `fa`/`ur`
  sharing the script) or Arabic titles sort under alef-lam.
- `facettext` search index is word-prefix based (`SearchPrefixes`, 12 runes;
  numeric 6). **Thai has no inter-word spaces**, so a Thai title becomes one
  token and only its first 12 runes are indexed — Thai search will be poor.
  CJK precedent exists (unigram/bigram expansion in `SearchDocument`); Thai
  needs either dictionary segmentation (server-side dep) or trigram indexing.
  Hebrew/Arabic (spaced, alphabetic) work with the existing machinery once
  folding is extended. Thai word segmentation is the largest search item.
- Client-side: `mini-guide.ts` uses `localeCompare(key, undefined, {numeric})`
  (locale-aware — good); `ChannelBuilder.tsx` and `SettingsRows.tsx` sort with
  the UI locale (good); `client-core` licence inventory hardcodes `'en'`
  (build/dev inventory, not user-visible sorting — no action).

## 8. Phased plan

- **Phase 0 — verify unknowns (S, 1–2 wks wall-clock, mostly device access).**
  Hermes `Intl.PluralRules`/`DateTimeFormat` Arabic coverage; Roku firmware
  Arabic/Hebrew/Thai shaping + font fallback on real hardware; Vega SDK RN
  RTL behavior; iOS sidecar cue shaping; Roku caption shaping. Go/no-go for
  Roku RTL UI scope.
- **Phase 1 — catalogue + tooling (S).** Register pilot locale(s) in
  `translatedLocales`, run XLIFF export/import, confirm plural-category
  enforcement; add bidi-isolation convention + lint (FSI/PDI or
  `<bdi>`/`dir=auto`); translator notes for gender/dual-sensitive strings;
  machine translation + native-speaker review per existing states.
- **Phase 2 — web (M).** Set `lang`/`dir` from resolved locale; logical-property
  conversion (~146 sites, codemod-able majority); mirror `back`/`forward`/arrow
  glyphs (central `icons.ts` + `scaleX(-1)`-style RTL variant, or mirrored
  assets); `dir="ltr"` islands for timeline/media-time widgets; Shelf
  scroll-helper RTL fix; `<bdi>` pass on title-bearing surfaces; `font-synthesis`
  decision; keyboard/arrow audit.
- **Phase 3 — native + Vega (M).** `allowRTL/forceRTL` + restart-to-apply flow;
  convert 39 physical box + 59 absolute occurrences to start/end (time-geometry
  stays physical); TV re-verification (focus, GuideGrid, shelves, Vega shims);
  Hermes assertion from Phase 0 as a launch gate.
- **Phase 4 — Roku (XL, or scoped out).** Direction-aware layout layer (or
  per-screen mirroring), mirrored row/timeline assets, font bundling + `Font`
  selection, locale packaging, Audio Guide re-verification. **Decision point:**
  ship Roku as subtitles/audio-only for RTL languages (track names already
  localized) and keep UI in supported LTR locales — contract allows limited
  locale sets (A11Y-14.5).
- **Phase 5 — search/sort + fonts (M).** `Fold` extensions (alef/kaf/yeh),
  Arabic article handling, Thai segmentation (the L inside this M), client
  `localeCompare` sweep.
- **Phase 6 — contract evidence (S per locale).** A11Y-19 RTL gates: screenshot,
  keyboard, screen-reader, truncation, and mixed LTR/RTL content review per
  supported RTL locale; Audio Guide pass on Roku hardware.

## 9. Effort estimates

| Area | Size | Notes |
|---|---|---|
| Catalogue/tooling (register locale, XLIFF, lint, FSI convention) | S | automation already exists; translator review is calendar time |
| Web layout + icons + bidi isolation | M | ~146 mechanical sites + ~15 icon sites + title-surface pass |
| Media-player direction islands (web + native) | S | mostly `dir="ltr"` scoping + already-semantic handlers |
| Native RTL (forceRTL, restart flow, start/end) | M | + TV/Vega device verification |
| Roku RTL UI | XL | layout layer + fonts + hardware verification; candidate for scope cut |
| Roku subtitles-only fallback | S | tables already map the languages |
| Fonts (web/native UI) | S | system fallback suffices; weigh Noto UI weights vs flattening |
| Roku fonts | M–XL | bundle + `Font` path, pending Phase 0 firmware findings |
| Subtitles shaping verification | S | iOS sidecar + Roku captions are the only unknowns |
| Search/sort (folding, articles) | S–M | small |
| Thai segmentation | M–L | only if Thai ships; independent of RTL pilot |
| Per-locale evidence (screenshots, SR, keyboard, mixed-content) | S | per RTL locale, per contract §19 |

## 10. Risks

1. **Roku hardware truth is unknown.** Firmware shaping, font fallback, and
   caption rendering for Arabic/Hebrew cannot be determined from source; do not
   commit to Roku RTL UI before a hardware pass (Phase 0).
2. **Silent plural fallback on Hermes.** `messageformat.ts` degrades to English
   rules without `Intl.PluralRules`; gate the pilot on a runtime assertion.
3. **Gendered agreement.** Zero gendered selects in source is good structure but
   means Arabic/Hebrew reviewers will find sentences that must be restructured,
   not just translated — budget source-string changes, not words.
4. **Scope creep into Thai/Hindi.** They are LTR and lower-risk, but Thai
   segmentation and Devanagari font QA are separate workstreams; keep them out
   of the RTL pilot except where shared (fonts, search folding).
5. **Timeline convention.** Deliberately keep media time LTR (contract §15.2–3,
   platform convention); document it so reviewers don't file it as an RTL bug.
6. **Translator states.** All four live languages are still `machine`; adding
   RTL locales before the review pipeline is proven risks shipping two
   half-reviewed batches. Pilot one locale end-to-end first.

## 11. Recommendation: pilot Hebrew, fast-follow Arabic

- **Hebrew first.** Full RTL + shaping + bidi proving ground, but minimal
  morphology cost: 3 plural categories (`one/two/other`, verified on ICU 78),
  no dual, and a smaller initial translation surface to shake down the
  `dir`/mirror/bidi pipeline and the Phase 6 evidence gates.
- **Arabic second** using the identical pipeline, adding its six categories
  (tooling enforces them), gender-note review, and `ال` sort handling.
- **Persian/Urdu third** (same script family, `one/other` plurals).
- **Thai/Hindi alongside or after**: LTR, so they don't need the mirror work,
  but need segmentation (Thai) and font QA; schedule independently.

## Appendix: key files

- Contract: `portico-internal/Product Contracts/accessibility-and-language/contract.md` §13–15, §18–19
- Catalogue: `server/packages/i18n/src/catalog/{index,en-US,translated}.ts`,
  `server/packages/i18n/src/{index,messageformat}.ts`,
  `server/packages/i18n/scripts/translations{,-lib}.mjs`, `TRANSLATING.md`
- Web: `web/index.html`, `web/src/app/i18n.ts`, `web/src/ui/{Shelf,Shelf.module,
  Overlay.module,Field.module}.tsx/css`, `web/src/player/Timeline.tsx`,
  `web/src/shell/Shell.module.css`, `packages/design/src/icons.ts`
- Native: `apps/native/src/app/i18n.ts`, `apps/native/src/ui/{Shelf,ListRow,
  Sheet,Text}.tsx`, `apps/native/src/player/Timeline.tsx`,
  `apps/native/src/screens/channels/GuideGrid.tsx`, `apps/vega/src/shims/`
- Roku: `source/ui/{Layout,Focus,Style}.bs`, `components/ui/Text.bs`,
  `components/player/PlayerHost.bs`, `source/core/LanguageTables.bs`,
  `scripts/generate.mjs`, `locale/en_*/translations.xml`, `fonts/`
- Subtitles: `web/src/player/subtitles/{fallback-fonts,vendor,subtitle-surface}.ts`,
  `third_party/subtitle-fonts/fonts/`, Android `PorticoMpvFonts*`
- Search/sort: `server/internal/{sorttext,facettext}`,
  `server/internal/catalog/search_prefix*.go`,
  `packages/client-core/src/presentation/mini-guide.ts`
