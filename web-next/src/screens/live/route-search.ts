export type LiveScreenSearch = Readonly<{sourceId?: string; channelId?: string; kind?: 'all' | 'live-source' | 'library-channel'; group?: string; q?: string; view?: 'guide' | 'list'; favorites?: boolean; sort?: 'number' | 'name' | 'favorites'}>;
function text(raw: Record<string, unknown>, key: string, max = 256): string | undefined {
  const value = raw[key];
  if (value === undefined) return undefined;
  if (typeof value !== 'string' || !value || value.length > max || /[\x00-\x1f\x7f]/.test(value)) throw new Error('invalid-live-route');
  return value;
}
/** A channel identity never implies a source or provenance. */
export function parseLiveScreenSearch(raw: Record<string, unknown>): LiveScreenSearch {
  const sourceId = text(raw, 'sourceId'), channelId = text(raw, 'channelId'), group = text(raw, 'group', 120), q = text(raw, 'q', 512);
  const view = (['guide', 'list'] as const).find(value => value === raw.view);
  const sort = (['number', 'name', 'favorites'] as const).find(value => value === raw.sort);
  const kind = (['all', 'live-source', 'library-channel'] as const).find(value => value === raw.kind);
  if (raw.view !== undefined && !view || raw.sort !== undefined && !sort || raw.kind !== undefined && !kind || raw.favorites !== undefined && typeof raw.favorites !== 'boolean') throw new Error('invalid-live-route');
  return {...(sourceId ? {sourceId} : {}), ...(channelId ? {channelId} : {}), ...(group ? {group} : {}), ...(q ? {q} : {}),
    ...(view ? {view} : {}), ...(sort ? {sort} : {}), ...(kind ? {kind} : {}), ...(typeof raw.favorites === 'boolean' ? {favorites: raw.favorites} : {})};
}
