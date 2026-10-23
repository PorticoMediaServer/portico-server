import type {WebScreenSearch} from '../../route-registry';

/** Opaque identity published by ReadingClient.list/versions, never a URL. */
export function readSearch(raw: WebScreenSearch): {sourceId?: string} {
  return typeof raw.sourceId === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(raw.sourceId) ? {sourceId: raw.sourceId} : {};
}
