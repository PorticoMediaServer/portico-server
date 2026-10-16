import type {ServerEvent} from './playback-v1/types.ts';
import type {EventsClient} from './playback-v1/events.ts';

/** Counters are decimal strings; compare them as BigInt, never floating point. */
export type FreshnessCounters = Readonly<{directory?: string; content?: string; contentOrder?: string; personal?: string; observation?: string}>;
/** `titles`, on a personal or progress hint, names the one title it was about, then its parent and grandparent; absent, any title in the library may have moved. */
export type FreshnessChange = Readonly<{type: 'library.changed' | 'libraries.changed' | 'personal.changed' | 'progress.changed' | 'settings.changed'; resourceId: string; revision?: string; counters: FreshnessCounters; titles?: readonly string[]}>;
const types = new Set(['library.changed', 'libraries.changed', 'personal.changed', 'progress.changed', 'settings.changed']);
export function parseFreshnessChange(event: ServerEvent): FreshnessChange | undefined {
  if (!types.has(event.type) || !event.resource?.id) return undefined;
  const data = event.data && typeof event.data === 'object' ? event.data as Record<string, unknown> : {};
  const counters: Record<string, string> = {};
  for (const key of ['directory', 'content', 'contentOrder', 'personal', 'observation']) {
    const value = data[key];
    if (typeof value === 'string' && /^\d{1,20}$/.test(value)) counters[key] = value;
  }
  const titles = Array.isArray(data.titles) && data.titles.length > 0 && data.titles.length <= 3 && data.titles.every(t => typeof t === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(t)) ? Object.freeze([...data.titles as string[]]) : undefined;
  return {type: event.type as FreshnessChange['type'], resourceId: event.resource.id, revision:event.revision, counters, ...(titles ? {titles} : {})};
}

/** Features share the existing device feed; this helper opens no connection. */
export function subscribeFreshness(events: EventsClient, listener: (change: FreshnessChange) => void): () => void {
  const releases = [...types].map(type => events.on(type, event => {const change = parseFreshnessChange(event); if (change) listener(change);}));
  return () => {for (const release of releases) release();};
}

export type CatalogueFailure = Readonly<{domain: number; entityId?: string; title?: string; reason: string; attempts: number; failedAt: number}>;
