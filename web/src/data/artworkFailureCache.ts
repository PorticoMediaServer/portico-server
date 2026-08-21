export const ARTWORK_FAILURE_TTL_MS = 30_000;
export const MAX_REMEMBERED_ARTWORK_FAILURES = 512;

type ArtworkFailureRecord = {
  expiresAt: number;
  timer?: ReturnType<typeof setTimeout>;
};

const failures = new Map<string, ArtworkFailureRecord>();
const listeners = new Set<() => void>();
let version = 0;

function notifyChange(): void {
  version += 1;
  for (const listener of listeners) listener();
}

function expireArtworkFailure(source: string, record: ArtworkFailureRecord): void {
  if (failures.get(source) !== record) return;
  failures.delete(source);
  notifyChange();
}

export function rememberArtworkFailure(source: string): number {
  if (!source) return 0;
  const previous = failures.get(source);
  if (previous?.timer) clearTimeout(previous.timer);
  const record: ArtworkFailureRecord = { expiresAt: Date.now() + ARTWORK_FAILURE_TTL_MS };
  record.timer = setTimeout(() => expireArtworkFailure(source, record), ARTWORK_FAILURE_TTL_MS);
  failures.delete(source);
  failures.set(source, record);
  while (failures.size > MAX_REMEMBERED_ARTWORK_FAILURES) {
    const oldest = failures.keys().next().value as string | undefined;
    if (!oldest) break;
    const oldestRecord = failures.get(oldest);
    if (oldestRecord?.timer) clearTimeout(oldestRecord.timer);
    failures.delete(oldest);
  }
  notifyChange();
  return record.expiresAt;
}

export function artworkFailureExpiresAt(source: string | undefined): number {
  if (!source) return 0;
  const expiresAt = failures.get(source)?.expiresAt ?? 0;
  return expiresAt > Date.now() ? expiresAt : 0;
}

export function clearArtworkFailureCache(): void {
  if (!failures.size) return;
  for (const record of failures.values()) {
    if (record.timer) clearTimeout(record.timer);
  }
  failures.clear();
  notifyChange();
}

export function subscribeArtworkFailureCache(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function artworkFailureCacheVersion(): number {
  return version;
}

export function tagsMayRefreshArtwork(tags: Iterable<string>): boolean {
  for (const tag of tags) {
    if (tag === '*' || tag === 'media' || tag === 'metadata' || tag === 'library-items' || tag === 'artwork') return true;
  }
  return false;
}
