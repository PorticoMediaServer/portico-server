/** Deterministic helpers for fixture generation. */
export function hash(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export function rng(seed: number) {
  let t = seed >>> 0;
  return () => {
    t += 0x6d2b79f5;
    let r = Math.imul(t ^ (t >>> 15), 1 | t);
    r ^= r + Math.imul(r ^ (r >>> 7), 61 | r);
    return ((r ^ (r >>> 14)) >>> 0) / 4294967296;
  };
}

export function pick<T>(r: () => number, list: readonly T[]): T {
  return list[Math.floor(r() * list.length)]!;
}

export function shuffle<T>(r: () => number, list: readonly T[]): T[] {
  const a = [...list];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(r() * (i + 1));
    [a[i], a[j]] = [a[j]!, a[i]!];
  }
  return a;
}

export const delay = (ms: number) => new Promise<void>(res => setTimeout(res, ms));

/**
 * Simulated network latency: fast enough to feel instant, slow enough to show
 * loading states. `?latency=0` turns it off, `?latency=3` slows it down
 * (see src/app/devflags.ts); the factor is set once at start-up.
 */
let latencyFactor = 1;
export function setLatencyFactor(f: number) { latencyFactor = Math.max(0, f); }
export const latency = (base = 140) => (latencyFactor === 0 ? Promise.resolve() : delay((base + Math.random() * 160) * latencyFactor));

export function sortKey(title: string): string {
  return title.replace(/^(the|a|an)\s+/i, '').toLowerCase();
}

export function firstLetter(title: string): string {
  const c = sortKey(title).charAt(0).toUpperCase();
  return /[A-Z]/.test(c) ? c : '#';
}
