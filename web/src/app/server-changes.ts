/**
 * "Something about this server's structure changed" (a library or a channel source added,
 * removed or renamed). The rail's libraries and channel sources reload on it, so they never
 * wait for a page reload. Fired after every successful console action (`admin/console.ts`
 * `useAction`), which covers create and delete for libraries, live sources and channels.
 */
const listeners = new Set<() => void>();
export function serverChanged(): void { for (const fn of [...listeners]) fn(); }
export function onServerChanged(fn: () => void): () => void { listeners.add(fn); return () => { listeners.delete(fn); }; }
