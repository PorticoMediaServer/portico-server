/**
 * A small cache for a document that has no revision of its own to key on (a library's sort and
 * filter vocabulary). What was read last time is shown at once; every new use also asks the
 * server again, and listeners hear the new value only when it differs. So a tab never keeps an
 * old answer for its whole life, and no screen waits or blanks while it is checked.
 *
 * Entries belong to one scope (who is signed in): a different scope forgets everything. The
 * cache is bounded, least recently used first out. A failed check keeps the value already shown;
 * only a first read with nothing to show reports its failure.
 */
export type CacheListener<T> = {value(value: T): void; error(error: unknown): void};

type Entry<T> = {value?: T; has: boolean; flight?: Promise<void>; listeners: Set<CacheListener<T>>};

export class RevalidatingCache<T> {
  private scope = '';
  private readonly entries = new Map<string, Entry<T>>();
  private readonly max: number;
  private readonly same: (a: T, b: T) => boolean;

  constructor(options: {max?: number; same?: (a: T, b: T) => boolean} = {}) {
    this.max = Math.max(1, options.max ?? 32);
    this.same = options.same ?? ((a, b) => JSON.stringify(a) === JSON.stringify(b));
  }

  /** Shows what is known for `key` and checks it again with `load`. Returns the way to stop listening. */
  use(scope: string, key: string, load: () => Promise<T>, listener: CacheListener<T>): () => void {
    if (scope !== this.scope) {
      this.scope = scope;
      this.entries.clear();
    }
    let entry = this.entries.get(key);
    if (entry) this.entries.delete(key); else entry = {has: false, listeners: new Set()};
    this.entries.set(key, entry);
    // Entries in use are never let go; among the rest the least recently used goes first.
    for (const [old, e] of this.entries) {
      if (this.entries.size <= this.max) break;
      if (e !== entry && !e.listeners.size) this.entries.delete(old);
    }
    entry.listeners.add(listener);
    if (entry.has) listener.value(entry.value as T);
    if (!entry.flight) {
      const current = entry;
      const mine = this.scope;
      current.flight = load().then(value => {
        if (this.scope !== mine || this.entries.get(key) !== current) return;
        const changed = !current.has || !this.same(current.value as T, value);
        current.value = value;
        current.has = true;
        if (changed) for (const l of [...current.listeners]) l.value(value);
      }, error => {
        if (this.scope !== mine || this.entries.get(key) !== current) return;
        if (!current.has) {
          for (const l of [...current.listeners]) l.error(error);
          this.entries.delete(key);
        }
      }).finally(() => { current.flight = undefined; });
    }
    const held = entry;
    return () => { held.listeners.delete(listener); };
  }

  get size(): number { return this.entries.size; }
}
