import type {I18n} from './index.ts';

/** A calendar fact has no time zone. Preserve its actual month/day precision
 * while using the current locale; never shift it into the device's prior day. */
export function calendarDate(i18n: Pick<I18n, 'date'>, value: string): string {
  if (!/^[0-9]{4}-[0-9]{2}(-[0-9]{2})?$/.test(value) || value.startsWith('0000')) return '';
  const month = value.length === 7;
  const date = new Date((month ? value + '-01' : value) + 'T12:00:00Z');
  if (!Number.isFinite(date.getTime()) || date.toISOString().slice(0, value.length) !== value) return '';
  return i18n.date(date, {year: 'numeric', month: 'long', ...(month ? {} : {day: 'numeric'}), timeZone: 'UTC'});
}
