import {KnownDirectServers, keyValueKnownServerStorage} from '@core/known-servers.ts';

/**
 * Servers this browser has signed in to directly, for the server switcher (client-core
 * `KnownDirectServers`; no credentials, just id, name, address and the last profile).
 */
const memory = new Map<string, string>();
const store = (() => {
  try { if (typeof localStorage !== 'undefined') { localStorage.getItem('portico.knownServers'); return localStorage; } } catch { /* blocked storage */ }
  return {getItem: (k: string) => memory.get(k) ?? null, setItem: (k: string, v: string) => { memory.set(k, v); }};
})();
export const knownServers = new KnownDirectServers(keyValueKnownServerStorage(store));
