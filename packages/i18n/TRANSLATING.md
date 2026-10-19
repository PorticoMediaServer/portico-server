# Translating Portico

The source catalogue is US English (`src/catalog/en-US.ts` and `src/catalog/en-US/*.ts`). Canadian and
British English are derived from it by spelling rules (`src/catalog/regional.ts`). Every other
language is a translation. French, Spanish, German and Russian are fully translated. Portuguese
(Brazil) `pt-BR`, Italian `it`, Dutch `nl`, Polish `pl`, Swedish `sv`, Danish `da`, Norwegian Bokmål `nb`,
Czech `cs`, Turkish `tr`, Ukrainian `uk`, Chinese `zh-Hans` and `zh-Hant`, Japanese `ja`, Korean `ko`,
Greek `el`, Hungarian `hu`, Romanian `ro`, Finnish `fi`, Indonesian `id`, Vietnamese `vi`, Thai `th` and
Hindi `hi` are registered with no translations yet: they read en-US until their translations are
imported. Chinese resolves by script (`zh-TW`, `zh-HK` and `zh-MO` read `zh-Hant`) and any Norwegian
reads `nb`.

Three **regional variants** keep only the messages they word differently from a base language:
Canadian French `fr-CA` (base `fr`), Latin American Spanish `es-419` (base `es`) and European
Portuguese `pt-PT` (base `pt-BR`). A message reads the variant's own words, then the base's, then
en-US. `fr-CA` is chosen for French in Canada, `es-419` for Spanish anywhere but Spain, `pt-PT` for
Portuguese anywhere but Brazil. `status` counts a message the variant does not override as
**inherited**, not missing. `export` for a variant is always the whole catalogue, prefilled with the
base's words; on import, a target left as the base's words is not stored, so only real differences
are kept.

| File | What it is |
| --- | --- |
| `translations/<locale>.json` | The record: each message's text, the English it was translated from (`source`) and its `state` |
| `src/catalog/translated/<locale>.ts` | Generated from the record; what the apps load. Never edit it by hand |
| `scripts/translations.mjs` | Status, export, import, build and check |
| `scripts/translations-lib.mjs` | The rules a translation must pass, shared with the tests |

`state` says how far a translation can be trusted: `machine` (machine translation; needs a native
speaker's review), `translated` (a person translated it; not reviewed) or `reviewed`. All four languages
French, Spanish, German and Russian started as machine translations on 18 October 2026; none has been
reviewed yet.

## Commands

Run from the repository root (Node 24):

```sh
node packages/i18n/scripts/translations.mjs status            # completeness per locale
node packages/i18n/scripts/translations.mjs export fr > fr.xlf  # what needs work: missing, stale, unusable, machine
node packages/i18n/scripts/translations.mjs export fr --all > fr.xlf
node packages/i18n/scripts/translations.mjs import fr fr.xlf    # check, merge and rebuild
node packages/i18n/scripts/translations.mjs check               # CI: every translation usable, generated files current
```

The exchange format is **XLIFF 1.2**. Every translation tool (Weblate, Crowdin, Lokalise, Phrase,
memoQ, Trados, Poedit, OmegaT) reads and writes it; it carries the English beside the translation,
the developer's context as notes, and a review state per message, which is exactly what a review pass
needs. Machine translations export as `needs-review-translation`; a reviewer marks each one `final`
(or `signed-off`), and import records those as `reviewed`. `translated` imports as `translated`;
units with no target, or `new`/`needs-translation`, are skipped. `--state reviewed` overrides the
file's states, for a reviewer who sends back plain targets.

Import refuses a translation that does not parse, drops or invents an argument, changes a select's keys,
loses an exact plural branch (`=0`), lacks one of the language's plural categories (Russian needs
`one`, `few`, `many` and `other`), drops a product name (Portico, Portico Account, Portico Services),
changes the leading or trailing spaces, or uses the ASCII apostrophe (`'` is ICU quoting; use `’`).
A translation may turn a bare number the English counts with into a plural, so the noun agrees.
A partial file is fine: what it lacks keeps reading en-US, and `check` only fails on a translation that
is unusable against today's English. The plural categories a language must give are those
`Intl.PluralRules` uses for whole numbers: `one`/`other` for most, `one`/`few`/`many`/`other` for
Polish, Russian and Ukrainian, `one`/`few`/`other` for Czech and Romanian, and `other` alone for
Chinese, Japanese, Korean, Indonesian, Vietnamese and Thai. Import names any unit that lacks one.

## How messages fall back

A message a language lacks reads US English (with English plural rules), and so does one whose translation no longer fits the English
(its arguments changed). A translation made from older English that still fits is used and reported as
**stale** until it is retranslated. Language names (`locale.*`) are written in their own language and
are never translated.

## Keeping a translated branch current

When the English changes on `main`, rebase, then:

1. `translations.mjs status` shows what is missing or stale.
2. `translations.mjs export <locale>` gives a translator (or a translation pass) exactly that work.
3. `translations.mjs import <locale> <file>` merges it back; commit `translations/` and the
   regenerated `src/catalog/translated/` together.
4. `translations.mjs check` and `npm run test:core` must pass. Copy the shared packages to the native
   repository and regenerate the Roku strings as usual.

## Pseudo-locale

`en-XA` is accented English padded about a third longer, inside brackets: `[Ŕéçéñţļý áððéð îñ Movies
öñé ţŵö]`. Words that show plain English bypass the catalogue; a label missing its closing bracket is
clipped; a box glyph is a font without the character. It is never offered as a language. To read it,
set the profile's `region.locale` to `en-XA` through the API, or the browser's or device's first
language to `en-XA`.
