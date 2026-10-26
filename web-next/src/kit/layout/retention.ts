/** Paint retention only. Incoming data is always rendered in full; the bound is
 * shared by every overlapping outgoing cohort, never applied to live rows. */
export type LayoutExit<T> = Readonly<{
  key: string; item: T; token: object; admittedAt: number; delay: number;
  deadline: number; cohortDeadline: number; retireAt: number; reduced: boolean;
}>;
export type LayoutRetention<T> = Readonly<{
  live: readonly T[]; present: readonly T[]; exits: ReadonlyMap<string, LayoutExit<T>>; scope?: unknown;
}>;
export type LayoutRetentionPolicy = Readonly<{
  maxRetained?: number; staggerMs?: number; visibleOnly?: boolean; scope?: unknown;
  /** Original caller context only, before the withdrawal commit makes rows inert. */
  beforeLeave?: (remaining: ReadonlySet<string>) => void;
}>;

export function retainLayout<T>(previous: LayoutRetention<T>, incoming: readonly T[], keyOf: (item: T) => string,
  now: number, reduced: boolean, exitMs: number, reducedMs: number, policy?: LayoutRetentionPolicy,
  visible?: ReadonlySet<string>): LayoutRetention<T> {
  if (previous.scope !== policy?.scope) return {live: incoming, present: incoming, exits: new Map(), scope: policy?.scope};
  const ids = new Set(incoming.map(keyOf)), exits = new Map<string, LayoutExit<T>>();
  const max = policy?.maxRetained === undefined ? Infinity : Number.isFinite(policy.maxRetained) ? Math.max(0, Math.floor(policy.maxRetained)) : 0;
  const stagger = Number.isFinite(policy?.staggerMs ?? 0) ? Math.max(0, policy?.staggerMs ?? 0) : 0;
  // Existing admissions keep their original token/delay/deadline. Reduction may
  // shorten effective retention, but no update can extend the admitted cohort deadline.
  for (const [key, exit] of previous.exits) {
    if (ids.has(key) || exit.retireAt <= now || exits.size >= max) continue;
    exits.set(key, reduced && !exit.reduced ? Object.freeze({...exit, reduced: true, retireAt: Math.min(exit.retireAt, now + reducedMs)}) : exit);
  }
  const admitted: {key: string; item: T}[] = [];
  for (const item of previous.live) {
    const key = keyOf(item);
    if (ids.has(key) || previous.exits.has(key) || exits.size + admitted.length >= max || policy?.visibleOnly && !visible?.has(key)) continue;
    admitted.push({key, item});
  }
  // A cohort keeps its invisible layout slots until its final stagger finishes.
  // Earlier faded rows cannot make later outgoing rows jump up underneath paint.
  const cohortDeadline = now + (reduced ? reducedMs : exitMs + Math.max(0, admitted.length - 1) * stagger);
  admitted.forEach(({key, item}, index) => {
    const delay = reduced ? 0 : index * stagger, deadline = now + delay + (reduced ? reducedMs : exitMs);
    exits.set(key, Object.freeze({key, item, token: Object.freeze({}), admittedAt: now, delay, deadline, cohortDeadline, retireAt: cohortDeadline, reduced}));
  });
  const positions = new Map(previous.present.map((item, index) => [keyOf(item), index]));
  const present = [...incoming];
  for (const exit of exits.values()) present.splice(Math.min(positions.get(exit.key) ?? present.length, present.length), 0, exit.item);
  return {live: incoming, present, exits, scope: policy?.scope};
}
