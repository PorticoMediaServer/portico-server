import {useEffect, useMemo} from 'react';
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
let active: I18n = defaultI18n;

/** The catalogue for code outside render (a `.catch`, a service callback): the one the screen is using. */
export function currentI18n(): I18n {
  return active;
}

/**
 * D-I18N / X-03: the viewer's catalogue and formatters. The server's `region.*` preferences
 * (language, clock) win; `auto` (and anything before the server answers) follows the browser.
 * Times are always in the browser's own time zone.
 */
export function useI18n(): I18n {
  const prefs = useServerPreferences();
  const locale = prefs.value<string>('region.locale', 'auto');
  const hourCycle = prefs.value<string>('region.hourCycle', 'auto') as 'auto' | 'h12' | 'h23';
  const i18n = useMemo(() => createI18n(resolveRegion({locale, hourCycle}, device)), [locale, hourCycle]);
  useEffect(() => { active = i18n; setUiI18n(i18n); setErrorI18n(i18n); setServiceI18n(i18n); }, [i18n]);
  return i18n;
}
