/** Enough of IndexedDB for the vault: staged writes commit atomically; explicit abort
 * rolls them back and fires onabort even when tx.error is null. */
export function fakeIndexedDB() {
  const data = new Map<string, unknown>();
  let failing = 0, abortNextWrite = false;
  const open = () => {
    const r: any = {};
    queueMicrotask(() => {
      if (failing > 0) { failing--; r.error = new Error('database busy'); r.onerror?.(); return; }
      r.result = {
        close() {},
        transaction(_name: string, mode: string, options?: {durability?: string}) {
          const staged = new Map(data), abortAtCommit = mode === 'readwrite' && abortNextWrite;
          if (mode === 'readwrite') abortNextWrite = false;
          let aborted = false;
          const request = <T>(run: () => T) => {const req: any = {};queueMicrotask(() => {if (aborted) return;req.result = run();req.onsuccess?.();});return req;};
          const tx: any = {durability: options?.durability ?? 'default', error: null,
            abort() {if (aborted) return;aborted = true;queueMicrotask(() => tx.onabort?.());},
            objectStore: () => ({transaction: tx, get: (k: string) => request(() => staged.get(k)),
              put: (v: unknown, k: string) => request(() => {staged.set(k, v);}), delete: (k: string) => request(() => {staged.delete(k);})}),
          };
          setTimeout(() => {
            if (abortAtCommit) tx.abort();
            if (aborted) return;
            if (mode === 'readwrite') {data.clear();for (const [key, value] of staged) data.set(key, value);}
            tx.oncomplete?.();
          }, 0);
          return tx;
        },
      };
      r.onsuccess?.();
    });
    return r;
  };
  return {data, indexedDB: {open}, failOpen: (times: number) => {failing = times;}, abortWrite: () => {abortNextWrite = true;}};
}
