import React, {createContext, useContext, useEffect, useLayoutEffect, useMemo, useState} from 'react';
import {catalogue, catalogueReady, createI18n, defaultI18n, loadCatalogue, resolveRegion, type DeviceRegion, type I18n, type SupportedLocale} from '@i18n';
import {useServerPreferences} from './server-preferences';
import {setUiI18n} from '../ui/i18n';
import {setErrorI18n} from './errors';
import {setServiceI18n} from '@core/presentation/index.ts';

export type {I18n, MessageId} from '@i18n';

/** The browser's languages and time zone. */
function browserRegion(): DeviceRegion {
  let timeZone: string | undefined;
  try { timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { /* no Intl */ }
  const languages = typeof navigator !== 'undefined' ? [...(navigator.languages ?? []), navigator.language].filter((l): l is string => !!l) : [];
  return {locales: languages.length ? languages : ['en-US'], timeZone};
}

const device = browserRegion();

/** The language the last signed-in profile chose on this browser (absent for Automatic). */
const LAST_LOCALE = 'portico.locale.v1';
function lastLocale(): string | undefined {
  try { return localStorage.getItem(LAST_LOCALE) ?? undefined; } catch { return undefined; }
}

/**
 * The catalogue for screens with no profile's preferences yet: signing in, setup, and the moment
 * before a profile's preferences arrive. The language this browser's last profile chose, else the
 * browser's own; a profile that leaves both choices on Automatic reads it too.
 */
let browser: I18n | undefined;
let browserFor: string | undefined;
function browserI18n(): I18n {
  const last = lastLocale();
  // Made again once its words arrive (until then a translated language reads en-US), or when the remembered language changes.
  if (!browser || browserFor !== last || (!browser.ready && catalogueReady(browser.locale))) {
    browser = createI18n(resolveRegion(last ? {locale: last} : undefined, device));
    browserFor = last;
  }
  return browser;
}

/**
 * Fetch the words the first screen needs before it renders: the browser's language, and the one
 * this browser's profile chose last time. Each translated language is its own file, loaded only
 * when read; a failure leaves en-US, which is always complete.
 */
export async function preloadI18n(): Promise<void> {
  const last = lastLocale();
  const wanted = new Set<SupportedLocale>([resolveRegion(undefined, device).uiLocale]);
  if (last) wanted.add(resolveRegion({locale: last}, device).uiLocale);
  await Promise.all([...wanted].map(loadCatalogue));
}

/** Re-render when a translated catalogue that was not here yet arrives. */
function useCatalogue(locale: SupportedLocale): boolean {
  const [ready, setReady] = useState(() => catalogueReady(locale));
  useEffect(() => {
    if (catalogueReady(locale)) { setReady(true); return; }
    setReady(false);
    let live = true;
    void loadCatalogue(locale).then(() => { if (live) setReady(catalogueReady(locale)); });
    return () => { live = false; };
  }, [locale]);
  return ready && catalogueReady(locale);
}

let active: I18n = defaultI18n;
/** How many providers are on screen. While one is, it alone decides the active catalogue. */
let providers = 0;

/** One switch for everything that speaks outside React: UI primitives, error text, shared services. */
function publish(i18n: I18n): void {
  if (active === i18n) return;
  active = i18n;
  // Screen readers pick their voice, and the browser its hyphenation, from the page's language.
  // A language with nothing translated yet reads English, and says so.
  const words = i18n.locale !== 'en-XA' && Object.keys(catalogue(i18n.locale)).length > 0;
  if (typeof document !== 'undefined') document.documentElement.lang = words ? i18n.locale : 'en';
  setUiI18n(i18n);
  setErrorI18n(i18n);
  setServiceI18n(i18n);
  // Han unification: the same code point renders with a Japanese, Simplified or
  // Traditional face depending on the page language, so the CSS font stacks
  // (`:lang(ja)` etc.) read it here.
  if (typeof document !== 'undefined' && document.documentElement.lang !== i18n.locale) document.documentElement.lang = i18n.locale;
}

/**
 * The catalogue for code outside render (a `.catch`, an event handler, a service callback): the
 * one the screen is using. A component reads `useI18n()` instead, so it re-renders when the
 * viewer's language or clock changes.
 */
export function currentI18n(): I18n {
  return active;
}

const I18nContext = createContext<I18n | undefined>(undefined);

/** The browser alone, for a profile that has answered Automatic for both choices. */
let automatic: I18n | undefined;

/**
 * The viewer's catalogue. Until the profile's preferences answer, the remembered language stands
 * in (`browserI18n`). `ready` is part of the key, not an input: the instance is made again when
 * its catalogue arrives.
 */
function viewerI18n(locale: string, hourCycle: 'auto' | 'h12' | 'h23', answered: boolean, ready: boolean): I18n {
  if (!answered) return browserI18n();
  if (locale !== 'auto' || hourCycle !== 'auto') return createI18n(resolveRegion({locale, hourCycle}, device));
  if (!automatic || (!automatic.ready && ready)) automatic = createI18n(resolveRegion(undefined, device));
  return automatic;
}

/**
 * D-I18N / X-03: the signed-in viewer's catalogue and formatters, one instance for the whole tree.
 * The server's `region.*` preferences (language, clock) win; `auto` (and anything before the
 * server answers) follows the browser. Times are always in the browser's own time zone.
 *
 * Mounted just inside the server preferences. The catalogue is published while this renders, so
 * a child that asks `currentI18n()`, a UI primitive or an error message during the same pass
 * already speaks the viewer's language; the layout effect holds it there across remounts and
 * hands the browser's catalogue back on sign-out.
 */
export function I18nProvider({children}: {children: React.ReactNode}) {
  const prefs = useServerPreferences();
  const locale = prefs.value<string>('region.locale', 'auto');
  const hourCycle = prefs.value<string>('region.hourCycle', 'auto') as 'auto' | 'h12' | 'h23';
  // The profile's own answer: the `auto` read before it arrives is only a default.
  const answered = prefs.snapshot !== undefined;
  const uiLocale = useMemo(() => {
    const last = answered ? undefined : lastLocale();
    return resolveRegion(answered ? {locale, hourCycle} : last ? {locale: last} : undefined, device).uiLocale;
  }, [locale, hourCycle, answered]);
  const ready = useCatalogue(uiLocale);
  const i18n = useMemo(() => viewerI18n(locale, hourCycle, answered, ready), [locale, hourCycle, answered, ready]);
  useEffect(() => {
    if (!answered) return;
    try { if (locale === 'auto') localStorage.removeItem(LAST_LOCALE); else localStorage.setItem(LAST_LOCALE, locale); } catch { /* storage blocked */ }
  }, [locale, answered]);
  publish(i18n);
  useLayoutEffect(() => {
    providers++;
    publish(i18n);
    return () => {
      providers--;
      if (!providers) publish(browserI18n());
    };
  }, [i18n]);
  return React.createElement(I18nContext.Provider, {value: i18n}, children);
}

/**
 * The viewer's catalogue and formatters. Under the signed-in tree this is the provider's shared
 * instance; anywhere else (the sign-in and setup screens) it follows the browser.
 */
export function useI18n(): I18n {
  const provided = useContext(I18nContext);
  // Called either way (a hook): it re-renders a signed-out screen when its words arrive.
  const last = lastLocale();
  useCatalogue(resolveRegion(last ? {locale: last} : undefined, device).uiLocale);
  if (provided) return provided;
  const i18n = browserI18n();
  // Nothing else speaks for a signed-out screen, so the first one to render sets the catalogue.
  if (!providers) publish(i18n);
  return i18n;
}
