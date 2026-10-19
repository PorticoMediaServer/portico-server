import {enUS, type MessageId} from './en-US.ts';
import {britishEnglish, canadianEnglish} from './regional.ts';
import {pseudoEnglish} from './pseudo.ts';

export {enUS, type MessageId};

/**
 * Locales with a catalogue. en-US is the source and the fallback for every message. The regional
 * English catalogues and the pseudo-locale are derived from it by rule; the other languages are
 * translations (translations/<locale>.json, generated into ./translated/). A registered language
 * with few or no translations yet reads en-US for every message it lacks.
 */
export const supportedLocales = ['en-US', 'en-CA', 'en-GB', 'fr', 'fr-CA', 'es', 'es-419', 'de', 'ru', 'pt-BR', 'pt-PT', 'it', 'nl', 'pl', 'sv', 'da', 'nb', 'fi', 'cs', 'hu', 'ro', 'el', 'tr', 'uk', 'id', 'vi', 'th', 'hi', 'zh-Hans', 'zh-Hant', 'ja', 'ko', 'en-XA'] as const;
export type SupportedLocale = (typeof supportedLocales)[number];
export const sourceLocale: SupportedLocale = 'en-US';
/** Accented, padded English for finding hard-coded words and clipped labels; never offered as a choice. */
export const pseudoLocale = 'en-XA' satisfies SupportedLocale;
/** The languages a viewer can choose, each regional variant beside its base language. */
export const selectableLocales = supportedLocales.filter(l => l !== pseudoLocale);

export const translatedLocales = ['fr', 'fr-CA', 'es', 'es-419', 'de', 'ru', 'pt-BR', 'pt-PT', 'it', 'nl', 'pl', 'sv', 'da', 'nb', 'fi', 'cs', 'hu', 'ro', 'el', 'tr', 'uk', 'id', 'vi', 'th', 'hi', 'zh-Hans', 'zh-Hant', 'ja', 'ko'] as const satisfies readonly SupportedLocale[];
export type TranslatedLocale = (typeof translatedLocales)[number];
type Messages = Partial<Record<MessageId, string>>;

/**
 * Regional variants keep only the messages they word differently from their base language; every
 * other message is the base's, then en-US's: fr-CA → fr → en-US.
 */
export const variantBase: Readonly<Partial<Record<TranslatedLocale, TranslatedLocale>>> = {'fr-CA': 'fr', 'es-419': 'es', 'pt-PT': 'pt-BR'};

// A translated catalogue is a few hundred kilobytes, so each is its own module and is loaded only
// when it is read: the web splits each into its own file, and English readers never fetch one.
const loaders: Record<TranslatedLocale, () => Promise<{messages: Messages}>> = {
  fr: () => import('./translated/fr.ts'),
  es: () => import('./translated/es.ts'),
  de: () => import('./translated/de.ts'),
  ru: () => import('./translated/ru.ts'),
  'pt-BR': () => import('./translated/pt-BR.ts'),
  it: () => import('./translated/it.ts'),
  nl: () => import('./translated/nl.ts'),
  pl: () => import('./translated/pl.ts'),
  sv: () => import('./translated/sv.ts'),
  da: () => import('./translated/da.ts'),
  nb: () => import('./translated/nb.ts'),
  cs: () => import('./translated/cs.ts'),
  tr: () => import('./translated/tr.ts'),
  uk: () => import('./translated/uk.ts'),
  'zh-Hans': () => import('./translated/zh-Hans.ts'),
  'zh-Hant': () => import('./translated/zh-Hant.ts'),
  ja: () => import('./translated/ja.ts'),
  ko: () => import('./translated/ko.ts'),
  'fr-CA': () => import('./translated/fr-CA.ts'),
  'es-419': () => import('./translated/es-419.ts'),
  'pt-PT': () => import('./translated/pt-PT.ts'),
  el: () => import('./translated/el.ts'),
  hu: () => import('./translated/hu.ts'),
  ro: () => import('./translated/ro.ts'),
  fi: () => import('./translated/fi.ts'),
  id: () => import('./translated/id.ts'),
  vi: () => import('./translated/vi.ts'),
  th: () => import('./translated/th.ts'),
  hi: () => import('./translated/hi.ts'),
};
const loaded: Partial<Record<TranslatedLocale, Messages>> = {};
const loading: Partial<Record<TranslatedLocale, Promise<void>>> = {};

const isTranslated = (locale: SupportedLocale): locale is TranslatedLocale => (translatedLocales as readonly string[]).includes(locale);

/** Whether `catalogue(locale)` has its words yet (a variant's and its base's). Always true for the English catalogues. */
export function catalogueReady(locale: SupportedLocale): boolean {
  if (!isTranslated(locale)) return true;
  const base = variantBase[locale];
  return loaded[locale] !== undefined && (!base || loaded[base] !== undefined);
}

function loadOne(locale: TranslatedLocale): Promise<void> {
  if (loaded[locale]) return Promise.resolve();
  return (loading[locale] ??= loaders[locale]().then(
    module => { loaded[locale] = module.messages; },
    () => { delete loading[locale]; },
  ));
}

/**
 * Fetch a translated catalogue (and a variant's base). Until it arrives (and if it never does) the
 * locale reads en-US, which is always complete. Resolves at once for a catalogue already here.
 */
export function loadCatalogue(locale: SupportedLocale): Promise<void> {
  if (!isTranslated(locale)) return Promise.resolve();
  const base = variantBase[locale];
  return Promise.all([loadOne(locale), base ? loadOne(base) : undefined]).then(() => undefined);
}

/** Hand over a catalogue that is already in the bundle (the apps include them all; see ./bundled.ts). */
export function provideCatalogue(locale: TranslatedLocale, messages: Messages): void {
  loaded[locale] = messages;
}

const merged = new Map<TranslatedLocale, {own: Messages; base: Messages; messages: Messages}>();
/** The messages a locale words differently from en-US; anything absent reads en-US. */
export function catalogue(locale: SupportedLocale): Messages {
  if (isTranslated(locale)) {
    const own = loaded[locale] ?? {};
    const baseLocale = variantBase[locale];
    if (!baseLocale) return own;
    const base = loaded[baseLocale] ?? {};
    const made = merged.get(locale);
    if (made && made.own === own && made.base === base) return made.messages;
    const messages = {...base, ...own};
    merged.set(locale, {own, base, messages});
    return messages;
  }
  return locale === 'en-CA' ? canadianEnglish() : locale === 'en-GB' ? britishEnglish() : locale === pseudoLocale ? pseudoEnglish() : enUS;
}
