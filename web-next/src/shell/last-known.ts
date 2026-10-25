/** The shell facts a launch may still know when the server stops answering.
 *
 * Only what the last successful assembly already showed: the server's own name and
 * the library names. No credentials, tokens, counts or item data are kept, and a
 * stored value is re-validated on read so older or foreign shapes are ignored. */

export type LastKnownLibrary = Readonly<{id: string; name: string; kind: string}>;
export type LastKnownShell = Readonly<{serverName: string; libraries: readonly LastKnownLibrary[]}>;

const PREFIX = 'portico.shell.lastKnown.v1:';
const MAX_LIBRARIES = 40;
const MAX_TEXT = 120;

function storage(): Storage | undefined {
  try { return globalThis.localStorage; } catch { return undefined; }
}

function text(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 && value.length <= MAX_TEXT ? value : undefined;
}

function library(value: unknown): LastKnownLibrary | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const {id, name, kind} = value as Partial<LastKnownLibrary>;
  const [validId, validName, validKind] = [text(id), text(name), text(kind)];
  return validId && validName && validKind ? {id: validId, name: validName, kind: validKind} : undefined;
}

/** Remembers the last assembled shell for this server; a no-op when storage is unavailable. */
export function rememberShell(serverId: string, serverName: string, libraries: readonly LastKnownLibrary[]): void {
  const key = text(serverId), name = text(serverName);
  const store = storage();
  if (!key || !name || !store) return;
  const kept = libraries.map(library).filter((value): value is LastKnownLibrary => value !== undefined).slice(0, MAX_LIBRARIES);
  if (kept.length === 0) return;
  try { store.setItem(PREFIX + key, JSON.stringify({serverName: name, libraries: kept})); } catch { /* private or full storage */ }
}

/** The last assembled shell for this server, or undefined when none was stored. */
export function lastKnownShell(serverId: string): LastKnownShell | undefined {
  const key = text(serverId), store = storage();
  if (!key || !store) return undefined;
  let parsed: unknown;
  try { parsed = JSON.parse(store.getItem(PREFIX + key) ?? 'null'); } catch { return undefined; }
  if (!parsed || typeof parsed !== 'object') return undefined;
  const {serverName, libraries} = parsed as {serverName?: unknown; libraries?: unknown};
  const name = text(serverName);
  if (!name || !Array.isArray(libraries)) return undefined;
  const kept = libraries.map(library).filter((value): value is LastKnownLibrary => value !== undefined).slice(0, MAX_LIBRARIES);
  return kept.length > 0 ? {serverName: name, libraries: kept} : undefined;
}
