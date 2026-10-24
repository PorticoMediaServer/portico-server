/** Enough of IndexedDB for the vault: one object store, requests that answer in order, and a
 * transaction that completes after them. `failOpen` makes the next opens fail, as a busy or
 * blocked database does. */
export function fakeIndexedDB() {
  const data = new Map<string, unknown>();
  let failing = 0;
  const request = <T>(run: () => T) => { const r: any = {}; queueMicrotask(() => { r.result = run(); r.onsuccess?.(); }); return r; };
  const open = () => {
    const r: any = {};
    queueMicrotask(() => {
      if (failing > 0) { failing--; r.error = new Error('database busy'); r.onerror?.(); return; }
      r.result = {
        close() {},
        transaction(_name: string, _mode: string, options?: {durability?: string}) {
          const tx: any = {durability: options?.durability ?? 'default', error: null, abort() {}, objectStore: () => ({get: (k: string) => request(() => data.get(k)), put: (v: unknown, k: string) => request(() => { data.set(k, v); }), delete: (k: string) => request(() => { data.delete(k); })})};
          setTimeout(() => tx.oncomplete?.(), 0);
          return tx;
        },
      };
      r.onsuccess?.();
    });
    return r;
  };
  return {data, indexedDB: {open}, failOpen: (times: number) => { failing = times; }};
}
