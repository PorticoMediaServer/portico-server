import {defaultI18n, type I18n} from '@i18n';

/**
 * The catalogue the UI primitives speak (labels like "Close", "Scroll back"). The app sets it from
 * the viewer's region (`app/i18n.ts`), so `ui/` never depends on `app/`.
 */
let active: I18n = defaultI18n;
export function uiI18n(): I18n {
  return active;
}
export function setUiI18n(i18n: I18n): void {
  active = i18n;
}
