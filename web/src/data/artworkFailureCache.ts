export const MAX_REMEMBERED_ARTWORK_FAILURES = 512;

const failures = new Set<string>();
const successes = new Set<string>();
const listenersBySource = new Map<string, Set<() => void>>();
let clearVersion = 0;

function notifyChange(source: string): void {
  for (const listener of listenersBySource.get(source) ?? []) listener();
}

/** Failed URLs remain settled until the active viewer scope is torn down. */
export function rememberArtworkFailure(source: string): void {
  if (!source || failures.has(source)) return;
  failures.add(source);
  while (failures.size > MAX_REMEMBERED_ARTWORK_FAILURES) {
    const oldest = failures.values().next().value as string | undefined;
    if (!oldest) break;
    failures.delete(oldest);
    notifyChange(oldest);
  }
  notifyChange(source);
}

export function hasArtworkFailure(source: string | undefined): boolean {
  return Boolean(source && failures.has(source));
}

export function forgetArtworkFailure(source: string | undefined): void {
  if (!source || !failures.delete(source)) return;
  notifyChange(source);
}

export function rememberArtworkSuccess(source: string): void {
  if (!source || successes.has(source)) return;
  successes.add(source);
  while (successes.size > MAX_REMEMBERED_ARTWORK_FAILURES * 4) {
    const oldest = successes.values().next().value as string | undefined;
    if (!oldest) break;
    successes.delete(oldest);
  }
}

export function hasArtworkSuccess(source: string | undefined): boolean {
  return Boolean(source && successes.has(source));
}

export function clearArtworkFailureCache(): void {
  if (!failures.size && !successes.size) return;
  failures.clear();
  successes.clear();
  clearVersion += 1;
  for (const listeners of listenersBySource.values()) {
    for (const listener of listeners) listener();
  }
}

export function subscribeArtworkFailureCache(source: string | undefined, listener: () => void): () => void {
  const key = source ?? '';
  let listeners = listenersBySource.get(key);
  if (!listeners) {
    listeners = new Set();
    listenersBySource.set(key, listeners);
  }
  listeners.add(listener);
  return () => {
    listeners!.delete(listener);
    if (!listeners!.size) listenersBySource.delete(key);
  };
}

export function artworkFailureCacheVersion(source: string | undefined): string {
  return `${clearVersion}:${source ? failures.has(source) : false}`;
}
