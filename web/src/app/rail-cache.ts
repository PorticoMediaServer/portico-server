import type {Library} from '@core/index.ts';
import type {ChannelSource} from '@core/guide/index.ts';

/**
 * What the rail showed last time for a signed-in viewer, kept in this browser so the shell
 * draws complete on the next load instead of assembling itself in steps (libraries, then
 * Channels, then the badge). The live reads replace it as they land; a viewer who lost a
 * library sees it disappear, never a stale page for it.
 */
export type RailCache = Readonly<{libraries: readonly Library[]; sources: readonly ChannelSource[]; owner: boolean; name?: string; at: number}>;

const KEY = 'portico.rail.v1';
const LAST = 'portico.rail.last';

function storage(): Storage | undefined {
  try { return typeof localStorage === 'undefined' ? undefined : localStorage; } catch { return undefined; }
}
function readAll(): Record<string, RailCache> {
  try { return JSON.parse(storage()?.getItem(KEY) ?? '{}') as Record<string, RailCache>; } catch { return {}; }
}

export function readRailCache(who: string | undefined): RailCache | undefined {
  if (!who) return undefined;
  const entry = readAll()[who];
  return entry && Array.isArray(entry.libraries) && Array.isArray(entry.sources) ? entry : undefined;
}

/** Merge a part of the rail into the viewer's entry (each read owns its own slice). */
export function writeRailCache(who: string | undefined, patch: Partial<Omit<RailCache, 'at'>>): void {
  const s = storage();
  if (!who || !s) return;
  try {
    const all = readAll();
    const prev = all[who] ?? {libraries: [], sources: [], owner: false, at: 0};
    // Keep the cache small: at most four viewers, newest last.
    const keep = Object.entries(all).filter(([k]) => k !== who).sort((a, b) => a[1].at - b[1].at).slice(-3);
    const next = Object.fromEntries([...keep, [who, {...prev, ...patch, at: Date.now()}]]);
    s.setItem(KEY, JSON.stringify(next));
    s.setItem(LAST, who);
  } catch { /* blocked storage: the rail just loads live */ }
}

/** The viewer who was last in the shell here, for the opening skeleton before restore finishes. */
export function lastRailCache(): RailCache | undefined {
  try { return readRailCache(storage()?.getItem(LAST) ?? undefined); } catch { return undefined; }
}

export function forgetRailCache(who: string | undefined): void {
  const s = storage();
  if (!who || !s) return;
  try {
    const all = readAll();
    delete all[who];
    s.setItem(KEY, JSON.stringify(all));
    if (s.getItem(LAST) === who) s.removeItem(LAST);
  } catch { /* nothing to forget */ }
}
