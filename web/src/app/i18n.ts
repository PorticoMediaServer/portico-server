import React, {createContext, useContext, useLayoutEffect, useMemo} from 'react';
import {createI18n, defaultI18n, resolveRegion, type DeviceRegion, type I18n} from '@i18n';
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

/** The catalogue that follows the browser alone: signed-out screens, and a profile that leaves both choices on Automatic. */
let browser: I18n | undefined;
function browserI18n(): I18n {
  return (browser ??= createI18n(resolveRegion(undefined, device)));
}

let active: I18n = defaultI18n;
/** How many providers are on screen. While one is, it alone decides the active catalogue. */
let providers = 0;

/** One switch for everything that speaks outside React: UI primitives, error text, shared services. */
function publish(i18n: I18n): void {
  if (active === i18n) return;
  active = i18n;
  setUiI18n(i18n);
  setErrorI18n(i18n);
  setServiceI18n(i18n);
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
  const i18n = useMemo(() => (locale === 'auto' && hourCycle === 'auto' ? browserI18n() : createI18n(resolveRegion({locale, hourCycle}, device))), [locale, hourCycle]);
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
  if (provided) return provided;
  const i18n = browserI18n();
  // Nothing else speaks for a signed-out screen, so the first one to render sets the catalogue.
  if (!providers) publish(i18n);
  return i18n;
}
