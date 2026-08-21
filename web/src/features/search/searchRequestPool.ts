export type SearchRequestTask = () => Promise<void>;

/**
 * Deterministic FIFO request pool shared by initial group loads and retries.
 * Active tasks finish naturally (their AbortControllers are owned by the
 * caller); clear() drops queued work so a newer search generation can replace
 * it without ever exceeding the server's per-viewer concurrency budget.
 */
export class SearchRequestPool {
  private active = 0;
  private readonly activeKeys = new Set<string>();
  private readonly queuedKeys = new Set<string>();
  private queue: Array<{ key: string; task: SearchRequestTask }> = [];

  constructor(private readonly maximumConcurrency = 2) {
    if (!Number.isInteger(maximumConcurrency) || maximumConcurrency < 1) throw new TypeError('Search request concurrency must be a positive integer.');
  }

  enqueue(key: string, task: SearchRequestTask) {
    if (!key || this.activeKeys.has(key) || this.queuedKeys.has(key)) return false;
    this.queue.push({ key, task });
    this.queuedKeys.add(key);
    this.pump();
    return true;
  }

  clear() {
    this.queue = [];
    this.queuedKeys.clear();
  }

  private pump() {
    while (this.active < this.maximumConcurrency && this.queue.length > 0) {
      const next = this.queue.shift();
      if (!next) return;
      this.queuedKeys.delete(next.key);
      this.activeKeys.add(next.key);
      this.active += 1;
      const release = () => {
        this.active -= 1;
        this.activeKeys.delete(next.key);
        this.pump();
      };
      void Promise.resolve().then(next.task).then(release, release);
    }
  }
}
