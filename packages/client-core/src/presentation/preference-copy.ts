/**
 * Human copy for the server's preference registry, shared by every client (X-05). The server
 * publishes keys, types, defaults and domains; the client owns only the words, which live in the
 * catalogue (`pref.*`, `packages/i18n/src/catalog/en-US/preferences.ts`) and follow the viewer's
 * language (`setServiceI18n`). Copy that depends on the device has `.web`/`.phone`/`.tv` variants,
 * so an Apple TV never says "browser" or "hover". Unknown keys fall back to the key in sentence case.
 */
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';
import {serviceI18n} from './service-text.ts';

export type CopyPlatform = 'web' | 'phone' | 'tv';

type Copy = {label: string; help?: string; values?: Record<string, string>};

/** A catalogue message if it exists (`pref.*` IDs are derived from registry keys). */
function text(id: string, values?: MessageValues): string | undefined {
  const i18n = serviceI18n();
  return i18n.has(id) ? i18n.t(id as MessageId, values) : undefined;
}
/** The platform variant (`id.tv`) or the shared message (`id`). */
const forPlatform = (id: string, platform: CopyPlatform) => text(`${id}.${platform}`) ?? text(id);

/** Keys the settings page never renders because another surface owns them. */
export const managedElsewhere: ReadonlySet<string> = new Set(['home.rowOrder', 'home.hiddenRowIds', 'navigation.pinnedLibraryIds', 'navigation.sidebarCollapsed']);

export function preferenceGroupTitle(group: string, platform: CopyPlatform): {title: string; description?: string} {
  const title = text(`pref.group.${group}.title`);
  if (!title) return {title: sentenceFromKey(group)};
  const description = forPlatform(`pref.group.${group}.description`, platform);
  return description ? {title, description} : {title};
}

export function preferenceLabel(key: string, platform: CopyPlatform): {label: string; help?: string} {
  if (/^quality\.[a-z]+\.allowHDR$/.test(key)) return {label: text('pref.label.allowHDR')!};
  const label = text(`pref.label.${key}`);
  if (!label) return {label: sentenceFromKey(key.split('.').pop() ?? key)};
  const help = forPlatform(`pref.help.${key}`, platform);
  return help ? {label, help} : {label};
}

/** A read-only record over catalogue IDs `prefix + key`, for the fixed key sets below. */
function catalogueRecord(prefix: string, keys: readonly string[]): Readonly<Record<string, string>> {
  return new Proxy({} as Record<string, string>, {
    get: (_t, p) => (typeof p === 'string' ? text(prefix + p) : undefined),
    has: (_t, p) => typeof p === 'string' && keys.includes(p),
    ownKeys: () => [...keys],
    getOwnPropertyDescriptor: (_t, p) => (typeof p === 'string' && keys.includes(p) ? {enumerable: true, configurable: true, value: text(prefix + p)} : undefined),
  });
}

/** Quality modes for every network (`quality.<network>.mode`). */
export const qualityModeLabels: Readonly<Record<string, string>> = catalogueRecord('pref.qualityMode.', ['off', 'automatic', 'original', 'high', 'standard', 'data-saver']);

/** Copy for an enumerated value; falls back to the value in sentence case with units. */
export function preferenceValueLabel(key: string, value: string | number): string {
  const known = text(`pref.value.${key}.${String(value)}`);
  if (known) return known;
  if (/^quality\.[a-z]+\.mode$/.test(key) && qualityModeLabels[String(value)]) return qualityModeLabels[String(value)]!;
  // Heights are labels, not quantities: "1080p", never "1,080p".
  if (/^quality\.[a-z]+\.maxVideoHeight$/.test(key) && typeof value === 'number') return value >= 4320 ? text('pref.unit.8k')! : value >= 2160 ? text('pref.unit.4k')! : text('pref.unit.height', {value: String(value)})!;
  if (typeof value === 'number') {
    if (key.endsWith('Seconds')) return text('pref.unit.seconds', {count: value})!;
    if (key.endsWith('Episodes')) return text('pref.unit.episodes', {count: value})!;
    if (key.endsWith('Percent')) return text('pref.unit.percent', {value})!;
    if (key.endsWith('Speed')) return value === 1 ? text('pref.unit.speedNormal')! : text('pref.unit.speed', {value})!;
    if (key.endsWith('Height')) return text('pref.unit.height', {value: String(value)})!;
    if (key.endsWith('Mbps')) return text('pref.unit.mbps', {value})!;
    if (key.endsWith('Kbps')) return text('pref.unit.kbps', {value})!;
    return String(value);
  }
  return sentenceFromKey(value);
}

/** Network classes for the per-network quality rows. */
export const qualityNetworkLabels: Readonly<Record<string, string>> = catalogueRecord('pref.network.', ['local', 'wifi', 'cellular', 'unknown']);

/** The device noun used in copy: "this browser" on web, "this device" elsewhere. */
export function deviceNoun(platform: CopyPlatform): string {
  return text(platform === 'web' ? 'pref.device.web' : 'pref.device.other')!;
}

function sentenceFromKey(key: string): string {
  const words = key.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/[_-]+/g, ' ').trim().toLowerCase();
  return words.replace(/^\w/, c => c.toUpperCase());
}

export type {Copy as PreferenceCopy};
