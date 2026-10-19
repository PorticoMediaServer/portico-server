import {enUS, type MessageId} from './en-US.ts';
import {britishEnglish, canadianEnglish} from './regional.ts';
import {pseudoEnglish} from './pseudo.ts';

export {enUS, type MessageId};

/**
 * Locales with a catalogue. en-US is the source and the fallback for every message. The regional
 * English catalogues and the pseudo-locale are derived from it by rule; the other languages are
 * translations (translations/<locale>.json, generated into ./translated/).
 */
export const supportedLocales = ['en-US', 'en-CA', 'en-GB', 'fr', 'es', 'de', 'ru', 'en-XA'] as const;
export type SupportedLocale = (typeof supportedLocales)[number];
export const sourceLocale: SupportedLocale = 'en-US';
/** Accented, padded English for finding hard-coded words and clipped labels; never offered as a choice. */
export const pseudoLocale = 'en-XA' satisfies SupportedLocale;
/** The languages a viewer can choose. */
export const selectableLocales = supportedLocales.filter(l => l !== pseudoLocale);

export const translatedLocales = ['fr', 'es', 'de', 'ru'] as const satisfies readonly SupportedLocale[];
export type TranslatedLocale = (typeof translatedLocales)[number];
type Messages = Partial<Record<MessageId, string>>;

// A translated catalogue is a few hundred kilobytes, so each is its own module and is loaded only
// when it is read: the web splits each into its own file, and English readers never fetch one.
const loaders: Record<TranslatedLocale, () => Promise<{messages: Messages}>> = {
  fr: () => import('./translated/fr.ts'),
  es: () => import('./translated/es.ts'),
  de: () => import('./translated/de.ts'),
  ru: () => import('./translated/ru.ts'),
};
const loaded: Partial<Record<TranslatedLocale, Messages>> = {};
const loading: Partial<Record<TranslatedLocale, Promise<void>>> = {};

const isTranslated = (locale: SupportedLocale): locale is TranslatedLocale => (translatedLocales as readonly string[]).includes(locale);

/** Whether `catalogue(locale)` has its words yet. Always true for the English catalogues. */
export function catalogueReady(locale: SupportedLocale): boolean {
  return !isTranslated(locale) || loaded[locale] !== undefined;
}

/**
 * Fetch a translated catalogue. Until it arrives (and if it never does) the locale reads en-US,
 * which is always complete. Resolves at once for a catalogue already here.
 */
export function loadCatalogue(locale: SupportedLocale): Promise<void> {
  if (!isTranslated(locale) || loaded[locale]) return Promise.resolve();
  return (loading[locale] ??= loaders[locale]().then(
    module => { loaded[locale] = module.messages; },
    () => { delete loading[locale]; },
  ));
}

/** Hand over a catalogue that is already in the bundle (the apps include them all; see ./bundled.ts). */
export function provideCatalogue(locale: TranslatedLocale, messages: Messages): void {
  loaded[locale] = messages;
}

/** The messages a locale words differently from en-US; anything absent reads en-US. */
export function catalogue(locale: SupportedLocale): Messages {
  if (isTranslated(locale)) return loaded[locale] ?? {};
  return locale === 'en-CA' ? canadianEnglish() : locale === 'en-GB' ? britishEnglish() : locale === pseudoLocale ? pseudoEnglish() : enUS;
}
