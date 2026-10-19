# ⚠️ DO NOT MERGE until Justin decides ⚠️

> **🚨 Do not merge me until you're sure! 🚨** Unreviewed machine translations, untested on every device.

This branch (`feature/i18n-languages-20261018`) is an **unreviewed, untested multi-language branch**.
It carries 25 machine-translated languages, none of them reviewed by a native speaker:

- **Complete:** French, Spanish, German and Russian.
- **About 98% complete:** 21 more languages. The missing messages are English text added on `main` after they were translated, so those messages read in English for now.
- **Registered but untranslated:** Vietnamese, Simplified Chinese and Turkish, which read English for now. The matching branches in portico-server, portico-react-native and
portico-roku belong together.

It is not ready to ship:

- Every translation is marked for native-speaker review (`packages/i18n/translations/<locale>.json`, `state: machine`).
- The languages have not been tested on devices or in a browser beyond automated checks and spot checks.
- Thai, Hindi and the CJK languages need script-specific work: fonts, line breaking, search and IME composition. That work is on `feature/i18n-cjk-20261018`, in progress.

Merging it into `main` needs a deliberate decision by Justin. Until then, keep it current by rebasing
onto `main` and translating new English strings (see `packages/i18n/TRANSLATING.md` in portico-server).
Delete this file only as part of that decision.

## Planning notes for when language testing starts

Recorded 18 October 2026. Nothing below has been tested yet. Every translation is machine output, and every
one of them needs a native-speaker review before release.

### How the translations were made
- **Source:** US English is the only source catalogue; Canadian and British English are derived from it by spelling rules.
- **Translated catalogue:** French, Spanish, German and Russian were translated by Claude (Opus). The other
  locales were translated by Muse Spark (one lane per locale). Every locale then got an adversarial review
  by DeepSeek, and the coordinator integrated the reviewed results.
- **Review marking:** every message is stored with state `machine`. A human reviewer's XLIFF marked `final`
  imports as `reviewed`. `packages/i18n/TRANSLATING.md` covers the workflow (export, import, status, check)
  and how to keep the branch current as English changes on `main`.
- **Regional variants:** fr-CA, es-419 and pt-PT are partial overlays. They store only what differs from
  fr, es and pt-BR, and everything else is inherited.

### Fonts
- **Manrope** is the canonical font. It's used wherever it has the characters: Latin, Latin Extended,
  Cyrillic (including Ukrainian), Greek and Vietnamese. The web app loads the extra Manrope subsets only
  when a page needs them. The native apps' Manrope files already include them.
- **Scripts Manrope doesn't cover** use the Noto Sans family: Noto Sans SC/TC/JP/KR, Noto Sans Thai
  (loopless) and Noto Sans Devanagari. Manrope stays first in every font stack, so Latin text inside those
  languages still renders in Manrope.
  - Native apps prefer the system fonts for these scripts.
  - Roku may need a bundled font, which has a size cost.
- **This was a pick-one-and-move-on decision.** Revisit it only if testing shows the fallback looks out of
  place: mismatched size or weight, rows that jump, or focus rings that shift.

### Text length: what to check and how to fix it
- Expect German, Finnish, Russian, Polish and Hungarian to run 30% longer than English or more. CJK text
  is much shorter but needs taller lines. Thai has no spaces between words.
- **Test with the `en-XA` pseudo-locale first.** It shows every string accented and padded about a third
  longer, in brackets. Plain English means a string bypasses the catalogue, a missing closing bracket
  means the label is clipped, and a box glyph means the font lacks that character. Then test German and
  Russian for real expansion, and Japanese for CJK layout.
- **Fix in this order:**
  1. Let it size or wrap: no fixed widths, buttons with a minimum width, headings that wrap.
  2. Reflow: stack crowded rows vertically.
  3. For genuinely tight spots (TV tabs, player controls, badges), add a separate short catalogue string
     and give translators a character limit.
  4. Use icons with accessible labels where space is tightest.
  5. Shrink text to fit only as a last resort.
- **Highest-risk screens:**
  - TV top navigation and tabs
  - player controls and skip buttons
  - settings rows with values
  - sign-in and server setup
  - title page action buttons
  - Roku labels, which have fixed geometry

### CJK, Thai and Hindi foundations
These are on `feature/i18n-cjk-20261018`, which is based on this branch. The device checklist is in that
lane's report. The work covers:
- font stacks;
- line breaking with Japanese kinsoku rules;
- search: SQLite's default tokenizer doesn't split CJK or Thai into words;
- sorting: Japanese reading order depends on metadata;
- letter-jump rails;
- IME composition, so search-as-you-type must not fire mid-composition;
- locale tags, for example zh-TW → zh-Hant.

### Right-to-left (investigation only; no work started)
The full report is `docs/i18n/rtl-investigation.md`, with an adversarial review in `docs/i18n/rtl-investigation-review.md`. In short:
- **Web:** medium. About 150 physical left/right style sites need converting to logical properties.
- **Native:** medium. `I18nManager` applies only after a restart.
- **Roku:** extra-large, or cut from scope. There's no RTL layout system, the only font is Manrope, and
  focus handling is built on physical directions.
- The video timeline stays left-to-right, per the accessibility contract.
- Suggested pilot: Hebrew first, then Arabic.

### Suggested test plan, when it's time
1. Pseudo-locale (`en-XA`) sweep of every screen on web, phone, tablet, Android TV, Apple TV, Fire TV and
   Roku. Fix clipping and hard-coded strings.
2. German and Russian sweep of the same screens, checking expansion and Cyrillic rendering.
3. Japanese sweep, checking CJK fonts, line breaking, search, sorting and IME.
4. A native-speaker review per launch language, using the XLIFF workflow, with the DeepSeek review notes
   as a starting checklist.
5. Decide which languages ship first. Everything else stays on this branch until it's reviewed.
