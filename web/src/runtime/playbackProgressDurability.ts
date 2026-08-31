import type {
  DurablePlaybackProgressRecord,
  PlaybackProgressDurabilityAdapter,
} from "@porticomediaserver/client-core";

const DATABASE_NAME = "portico-playback-progress-v1";
const STORE_NAME = "records";

export function createBrowserPlaybackProgressDurability(
  factory: IDBFactory | null | undefined = globalThis.indexedDB,
): PlaybackProgressDurabilityAdapter {
  const memory = new Map<string, DurablePlaybackProgressRecord>();
  let database: Promise<IDBDatabase> | undefined;
  const open = () => {
    if (!factory) return undefined;
    database ??= new Promise<IDBDatabase>((resolve, reject) => {
      const request = factory.open(DATABASE_NAME, 1);
      request.onupgradeneeded = () => {
        if (!request.result.objectStoreNames.contains(STORE_NAME))
          request.result.createObjectStore(STORE_NAME, { keyPath: "key" });
      };
      request.onsuccess = () => resolve(request.result);
      request.onerror = () =>
        reject(
          request.error ??
            new Error("Playback progress storage could not be opened."),
        );
    });
    return database;
  };
  const transaction = async <T>(
    mode: IDBTransactionMode,
    run: (store: IDBObjectStore) => IDBRequest<T>,
  ): Promise<T> => {
    const db = await open();
    if (!db) throw new Error("IndexedDB is unavailable.");
    return new Promise<T>((resolve, reject) => {
      const tx = db.transaction(STORE_NAME, mode);
      const request = run(tx.objectStore(STORE_NAME));
      let result: T;
      request.onsuccess = () => {
        result = request.result;
      };
      request.onerror = () =>
        reject(request.error ?? new Error("Playback progress storage failed."));
      // An IndexedDB request can report success before its containing
      // transaction commits. Terminal authority is not durable until the
      // transaction completes, so never release Core to restore/recreate a
      // client against an uncommitted write.
      tx.oncomplete = () => resolve(result);
      tx.onerror = () =>
        reject(tx.error ?? new Error("Playback progress storage failed."));
      tx.onabort = () =>
        reject(tx.error ?? new Error("Playback progress storage was aborted."));
    });
  };
  return {
    async load() {
      if (!factory) return [...memory.values()];
      return transaction("readonly", (store) => store.getAll()) as Promise<
        DurablePlaybackProgressRecord[]
      >;
    },
    async save(record) {
      if (!factory) {
        memory.set(record.key, record);
        return;
      }
      await transaction("readwrite", (store) => store.put(record));
    },
    async remove(key) {
      if (!factory) {
        memory.delete(key);
        return;
      }
      await transaction("readwrite", (store) => store.delete(key));
    },
  };
}

export const browserPlaybackProgressDurability =
  createBrowserPlaybackProgressDurability();
